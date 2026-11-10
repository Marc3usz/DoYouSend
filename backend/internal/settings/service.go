package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	netmail "net/mail"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/messaging"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers/setup"
)

// Source says where the effective value of a setting comes from.
type Source string

// Sources, from strongest to weakest.
const (
	SourcePanel   Source = "panel"
	SourceEnv     Source = "env"
	SourceDefault Source = "default"
)

// Setting is a definition with its effective value.
type Setting struct {
	Def
	// Value is the effective value. For secrets it is the plaintext and must
	// never be written to a response or a log.
	Value  string
	Source Source
	// Unreadable is a panel secret that cannot be decrypted (the key
	// changed); the setting falls back to the environment until re-entered.
	Unreadable bool
}

// Set reports whether the setting has a non-empty value.
func (s Setting) Set() bool { return s.Value != "" }

// Snapshot is the effective configuration at one moment.
type Snapshot struct {
	byKey map[string]Setting
	// DryRunLocked is DRY_RUN_LOCK=true: the panel cannot turn DRY_RUN off.
	DryRunLocked bool
}

// Settings returns every setting in display order.
func (s Snapshot) Settings() []Setting {
	out := make([]Setting, 0, len(defs))
	for _, d := range defs {
		out = append(out, s.byKey[d.Key])
	}
	return out
}

// Value returns the effective value of key ("" when unknown).
func (s Snapshot) Value(key string) string { return s.byKey[key].Value }

func (s Snapshot) flag(key string) bool { return s.Value(key) == "true" }

// DryRun reports whether real sending is suppressed.
func (s Snapshot) DryRun() bool { return s.DryRunLocked || s.flag(KeyDryRun) }

// SendsExternally reports whether messages leave the machine: DRY_RUN is off
// and at least one channel uses a real operator.
func (s Snapshot) SendsExternally() bool {
	return !s.DryRun() && (s.Value(KeyEmailProvider) == "sendgrid" || s.Value(KeySMSProvider) == "smsapi")
}

// SMSPriceMilli is the SMS part price in thousandths of a złoty. Values are
// validated before they are stored, so a parse error falls back to 0.
func (s Snapshot) SMSPriceMilli() int64 {
	p, _ := messaging.ParsePrice(s.Value(KeySMSPricePerPart))
	return p
}

// ProviderConfig maps the snapshot onto the provider factory configuration.
func (s Snapshot) ProviderConfig() setup.Config {
	return setup.Config{
		EmailProvider:            s.Value(KeyEmailProvider),
		SMTPHost:                 s.Value(KeySMTPHost),
		SMTPPort:                 s.Value(KeySMTPPort),
		SMTPUsername:             s.Value(KeySMTPUsername),
		SMTPPassword:             s.Value(KeySMTPPassword),
		EmailFrom:                s.Value(KeyEmailFrom),
		SendGridAPIKey:           s.Value(KeySendGridAPIKey),
		SendGridSandbox:          s.flag(KeySendGridSandbox),
		SendGridRegion:           s.Value(KeySendGridRegion),
		SendGridWebhookPublicKey: s.Value(KeySendGridWebhookKey),
		SenderAddress:            s.Value(KeySenderAddress),
		SenderCity:               s.Value(KeySenderCity),
		SenderZip:                s.Value(KeySenderZip),
		SenderCountry:            s.Value(KeySenderCountry),
		SMSProvider:              s.Value(KeySMSProvider),
		SMSAPIKey:                s.Value(KeySMSAPIKey),
		SMSSenderName:            s.Value(KeySMSSenderName),
		SMSAPITestMode:           s.flag(KeySMSTestMode),
		DryRun:                   s.DryRun(),
	}
}

// Row is one stored setting.
type Row struct {
	Key       string
	Value     string // ciphertext when Encrypted
	Encrypted bool
	UpdatedAt time.Time
	UpdatedBy string // user ID, empty when unknown
}

// Store keeps the values saved in the panel; PGStore implements it.
type Store interface {
	ListSettings(ctx context.Context) ([]Row, error)
	// SaveSettings upserts set and deletes the keys in remove, atomically.
	SaveSettings(ctx context.Context, set []Row, remove []string) error
}

// Applier builds and activates providers; setup.Live implements it.
type Applier interface {
	Prepare(cfg setup.Config) (*setup.Prepared, error)
	Activate(p *setup.Prepared)
}

// Auditor records a change in the audit log; iam.Service implements it.
type Auditor interface {
	Record(ctx context.Context, userID, action, entity, entityID string, details map[string]string)
}

// ActionSettingsChanged is the audit action of a saved change; entity "settings".
const ActionSettingsChanged = "settings_changed"

// Errors of Update. Match them with errors.Is.
var (
	ErrInvalidInput = errors.New("invalid settings")
	// ErrConfirmationRequired: the change makes messages leave the machine
	// and the request did not confirm it.
	ErrConfirmationRequired = errors.New("enabling real sending must be confirmed")
	// ErrDryRunLocked: DRY_RUN_LOCK=true forbids turning DRY_RUN off.
	ErrDryRunLocked = errors.New("DRY_RUN is locked on by the server (DRY_RUN_LOCK)")
	// ErrBatchesRunning: the change would switch to real sending, or to other
	// providers, while a batch is being sent.
	ErrBatchesRunning = errors.New("a batch is being sent; turn DRY_RUN off or change providers once it finishes")
)

// FieldError is a problem with one setting.
type FieldError struct {
	Field   string
	Message string
}

// ValidationError lists every rejected setting; it matches ErrInvalidInput.
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string {
	msgs := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		msgs[i] = f.Field + ": " + f.Message
	}
	return "invalid settings: " + strings.Join(msgs, "; ")
}

// Is makes errors.Is(err, ErrInvalidInput) match.
func (e *ValidationError) Is(target error) bool { return target == ErrInvalidInput }

// Options configure a Service.
type Options struct {
	// LookupEnv reads the environment; os.LookupEnv in production.
	LookupEnv func(string) (string, bool)
	// DryRunLocked forbids turning DRY_RUN off from the panel.
	DryRunLocked bool
	Logger       *slog.Logger
	Now          func() time.Time
}

// Service reads and changes the settings.
type Service struct {
	store   Store
	cipher  *Cipher
	opts    Options
	applier Applier
	auditor Auditor
	busy    func() int

	mu        sync.RWMutex
	current   Snapshot
	listeners []func(Snapshot)
}

// NewService returns a Service. Call Load before Current.
func NewService(store Store, c *Cipher, opts Options) *Service {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Service{store: store, cipher: c, opts: opts}
}

// SetApplier sets what builds providers from a candidate configuration.
// Without one, Update stores values without checking that providers start.
func (s *Service) SetApplier(a Applier) { s.applier = a }

// SetBusy sets what reports how many batches are being sent right now. While it
// is above zero, Update refuses to turn DRY_RUN off or to change providers with
// DRY_RUN off, so the rest of a test batch cannot leave for real (ADR-0018).
// Turning DRY_RUN on is always allowed.
func (s *Service) SetBusy(busy func() int) { s.busy = busy }

// SetAuditor sets where saved changes are recorded.
func (s *Service) SetAuditor(a Auditor) { s.auditor = a }

// OnChange registers f to run with every snapshot Update activates.
func (s *Service) OnChange(f func(Snapshot)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, f)
}

// Load reads the stored values and makes them current. It does not build
// providers; the caller does that with the returned snapshot.
func (s *Service) Load(ctx context.Context) (Snapshot, error) {
	rows, err := s.store.ListSettings(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load settings: %w", err)
	}
	snap := s.resolve(rows)
	s.mu.Lock()
	s.current = snap
	s.mu.Unlock()
	return snap, nil
}

// Current returns the snapshot in effect.
func (s *Service) Current() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// EnvOnly is the configuration from the environment and defaults alone. The
// API falls back to it, with DRY_RUN forced on, when the stored panel
// configuration cannot start.
func (s *Service) EnvOnly() Snapshot { return s.resolve(nil) }

// resolve computes the effective values: panel row, then environment, then default.
func (s *Service) resolve(rows []Row) Snapshot {
	stored := make(map[string]Row, len(rows))
	for _, r := range rows {
		stored[r.Key] = r
	}
	snap := Snapshot{byKey: make(map[string]Setting, len(defs)), DryRunLocked: s.opts.DryRunLocked}
	for _, d := range defs {
		st := Setting{Def: d, Value: d.Default, Source: SourceDefault}
		if v, ok := s.lookupEnv(d.Env); ok && strings.TrimSpace(v) != "" {
			// An environment value the panel would reject keeps the default.
			if nv, msg := normalize(d, normalizeEnv(d, v)); msg == "" {
				st.Value, st.Source = nv, SourceEnv
			} else {
				s.opts.Logger.Warn("ignoring an invalid environment value; using the default", "env", d.Env, "reason", msg)
			}
		}
		if r, ok := stored[d.Key]; ok {
			v, err := s.open(r)
			if err != nil {
				st.Unreadable = true
				s.opts.Logger.Warn("stored setting cannot be decrypted; falling back to the environment", "key", d.Key)
			} else {
				st.Value, st.Source = v, SourcePanel
			}
		}
		snap.byKey[d.Key] = st
	}
	return snap
}

func (s *Service) lookupEnv(key string) (string, bool) {
	if s.opts.LookupEnv == nil {
		return "", false
	}
	return s.opts.LookupEnv(key)
}

// normalizeEnv reads booleans with strconv.ParseBool, as config.Load does for
// DRY_RUN ("1", "TRUE" and "true" are all true). A value it cannot read falls
// back to the default, so a typo in DRY_RUN never turns real sending on.
func normalizeEnv(d Def, v string) string {
	v = strings.TrimSpace(v)
	if d.Kind == KindBool {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return d.Default
		}
		return strconv.FormatBool(b)
	}
	return v
}

func (s *Service) open(r Row) (string, error) {
	if !r.Encrypted {
		return r.Value, nil
	}
	if s.cipher == nil {
		return "", ErrUnreadableSecret
	}
	return s.cipher.Open(r.Key, r.Value)
}

// Change is one requested change: a new value, or nil to drop the panel value
// and fall back to the environment or the default.
type Change struct {
	Key   string
	Value *string
}

// Update validates and saves changes made by actorID, rebuilds the providers
// and makes the result current. Nothing is saved when anything fails: a
// rejected value, a configuration the providers cannot start with, or real
// sending that was not confirmed.
func (s *Service) Update(ctx context.Context, actorID string, changes []Change, confirmLive bool) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.store.ListSettings(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load settings: %w", err)
	}
	before := s.resolve(rows)

	byKey := make(map[string]Row, len(rows))
	for _, r := range rows {
		byKey[r.Key] = r
	}
	now := s.opts.Now()
	var set []Row
	var remove, changed []string
	var fields []FieldError
	for _, ch := range changes {
		d, ok := defByKey(ch.Key)
		if !ok {
			fields = append(fields, FieldError{Field: ch.Key, Message: "unknown setting"})
			continue
		}
		if slices.Contains(changed, d.Key) {
			fields = append(fields, FieldError{Field: d.Key, Message: "changed twice in one request"})
			continue
		}
		changed = append(changed, d.Key)
		if ch.Value == nil {
			delete(byKey, d.Key)
			remove = append(remove, d.Key)
			continue
		}
		v, msg := normalize(d, *ch.Value)
		if msg != "" {
			fields = append(fields, FieldError{Field: d.Key, Message: msg})
			continue
		}
		if d.Key == KeyDryRun && v != "true" && s.opts.DryRunLocked {
			return Snapshot{}, ErrDryRunLocked
		}
		row := Row{Key: d.Key, Value: v, UpdatedAt: now, UpdatedBy: actorID}
		if d.Kind == KindSecret {
			if s.cipher == nil {
				return Snapshot{}, errors.New("no settings encryption key configured")
			}
			if row.Value, err = s.cipher.Seal(d.Key, v); err != nil {
				return Snapshot{}, err
			}
			row.Encrypted = true
		}
		byKey[d.Key] = row
		set = append(set, row)
	}
	if len(fields) > 0 {
		return Snapshot{}, &ValidationError{Fields: fields}
	}

	next := make([]Row, 0, len(byKey))
	for _, r := range byKey {
		next = append(next, r)
	}
	after := s.resolve(next)
	if switchesSending(before, after) && s.busy != nil && s.busy() > 0 {
		return Snapshot{}, ErrBatchesRunning
	}
	if after.SendsExternally() && !before.SendsExternally() && !confirmLive {
		return Snapshot{}, ErrConfirmationRequired
	}

	var prepared *setup.Prepared
	if s.applier != nil {
		prepared, err = s.applier.Prepare(after.ProviderConfig())
		if err != nil {
			return Snapshot{}, &ValidationError{Fields: []FieldError{{Field: "config", Message: err.Error()}}}
		}
	}
	if err := s.store.SaveSettings(ctx, set, remove); err != nil {
		return Snapshot{}, fmt.Errorf("save settings: %w", err)
	}
	if prepared != nil {
		s.applier.Activate(prepared)
	}
	s.current = after
	for _, f := range s.listeners {
		f(after)
	}

	if s.auditor != nil && len(changed) > 0 {
		slices.Sort(changed)
		details := map[string]string{"keys": strings.Join(changed, ",")}
		if before.DryRun() != after.DryRun() {
			details["dry_run"] = strconv.FormatBool(after.DryRun())
		}
		for _, k := range []string{KeyEmailProvider, KeySMSProvider} {
			if before.Value(k) != after.Value(k) {
				details[k] = after.Value(k)
			}
		}
		s.auditor.Record(ctx, actorID, ActionSettingsChanged, "settings", "", details)
	}
	s.opts.Logger.Info("settings changed", "keys", strings.Join(changed, ","), "dryRun", after.DryRun())
	return after, nil
}

// switchesSending reports whether after sends through other providers than
// before: DRY_RUN turned off, or a provider changed while DRY_RUN is off.
func switchesSending(before, after Snapshot) bool {
	if after.DryRun() {
		return false
	}
	return before.DryRun() ||
		before.Value(KeyEmailProvider) != after.Value(KeyEmailProvider) ||
		before.Value(KeySMSProvider) != after.Value(KeySMSProvider)
}

// normalize checks a value of d and returns its stored form, or a message.
func normalize(d Def, raw string) (string, string) {
	v := strings.TrimSpace(raw)
	switch d.Kind {
	case KindBool:
		if v != "true" && v != "false" {
			return "", "must be true or false"
		}
	case KindEnum:
		if !slices.Contains(d.Options, v) {
			return "", "must be one of: " + strings.Join(d.Options, ", ")
		}
	case KindPrice:
		if _, err := messaging.ParsePrice(v); err != nil {
			return "", "must be a price like 0.08 (dot, at most 3 decimals)"
		}
	case KindPort:
		if n, err := strconv.Atoi(v); err != nil || n < 1 || n > 65535 {
			return "", "must be a port number between 1 and 65535"
		}
	case KindEmail:
		if _, err := netmail.ParseAddress(v); err != nil {
			return "", "must be an address like \"Szkola <no-reply@example.test>\""
		}
	case KindSecret:
		if v == "" {
			return "", "empty; to remove the value send null"
		}
	}
	if len(v) > 4096 {
		return "", "longer than 4096 characters"
	}
	if d.Normalize != nil && v != "" {
		nv, err := d.Normalize(v)
		if err != nil {
			return "", err.Error()
		}
		v = nv
	}
	return v, ""
}
