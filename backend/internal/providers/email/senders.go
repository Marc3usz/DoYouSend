package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sender identities (SendGrid "Single Sender Verification", ADR-0016).
//
// SendGrid only sends mail whose From address is a verified sender identity or
// belongs to an authenticated domain. Before the school authenticates its domain,
// every staff member who sends in their own name needs a single sender: SendGrid
// mails a confirmation link to that address and the identity becomes verified
// once the owner clicks it. Nothing here sends a message to recipients.

// Errors of the sender-identity API. Match them with errors.Is.
var (
	// ErrSenderNotFound means the provider has no identity with that ID.
	ErrSenderNotFound = errors.New("sender identity not found")
	// ErrSenderRejected means the provider refused the request itself
	// (a 4xx other than 404 and 429): bad address, duplicate, missing field.
	ErrSenderRejected = errors.New("sender identity rejected by provider")
)

// SenderAPIError carries what the provider said about a refused request.
// Messages come from the provider and hold no credentials.
type SenderAPIError struct {
	Status   int
	Messages []string
	kind     error
}

func (e *SenderAPIError) Error() string {
	msg := strings.Join(e.Messages, "; ")
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	return fmt.Sprintf("sendgrid verified senders: HTTP %d: %s", e.Status, msg)
}

// Unwrap lets errors.Is match ErrSenderNotFound / ErrSenderRejected.
func (e *SenderAPIError) Unwrap() error { return e.kind }

// SenderIdentity is one sender identity as the provider reports it.
type SenderIdentity struct {
	ID        string
	Nickname  string
	FromEmail string
	FromName  string
	ReplyTo   string
	Verified  bool
	// Locked identities cannot be edited (SendGrid locks them while in use).
	Locked bool
}

// NewSenderIdentity is what is needed to request a sender identity. SendGrid
// requires Nickname, FromEmail and ReplyTo; the postal address is optional in
// the API but shown in the verification e-mail, so it comes from the school's
// configuration (SENDGRID_SENDER_*).
type NewSenderIdentity struct {
	Nickname    string
	FromEmail   string
	FromName    string
	ReplyTo     string
	ReplyToName string
	Address     string
	City        string
	Zip         string
	Country     string
}

// SenderVerifier manages sender identities at the e-mail provider.
type SenderVerifier interface {
	CreateSender(ctx context.Context, in NewSenderIdentity) (SenderIdentity, error)
	GetSender(ctx context.Context, id string) (SenderIdentity, error)
	ResendVerification(ctx context.Context, id string) error
	DeleteSender(ctx context.Context, id string) error
}

// SendGridSenders calls the SendGrid Verified Senders API
// (https://www.twilio.com/docs/sendgrid/api-reference/sender-verification).
// It is safe for concurrent use.
type SendGridSenders struct {
	apiKey string
	host   string
	client *http.Client
	logger *slog.Logger
}

// NewSendGridSenders returns a client for the Verified Senders API. It uses the
// APIKey, Region, BaseURL and HTTPClient of cfg; the key needs the
// "Sender Verification" (verified_senders) scope.
func NewSendGridSenders(cfg SendGridConfig, logger *slog.Logger) (*SendGridSenders, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("sendgrid verified senders: API key is empty")
	}
	if logger == nil {
		logger = slog.Default()
	}
	host, err := resolveHost(cfg)
	if err != nil {
		return nil, err
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &SendGridSenders{apiKey: cfg.APIKey, host: strings.TrimRight(host, "/"), client: client, logger: logger}, nil
}

// sgSender is the JSON shape of a sender identity in the SendGrid API.
type sgSender struct {
	ID          int64  `json:"id,omitempty"`
	Nickname    string `json:"nickname"`
	FromEmail   string `json:"from_email"`
	FromName    string `json:"from_name,omitempty"`
	ReplyTo     string `json:"reply_to"`
	ReplyToName string `json:"reply_to_name,omitempty"`
	Address     string `json:"address,omitempty"`
	City        string `json:"city,omitempty"`
	Zip         string `json:"zip,omitempty"`
	Country     string `json:"country,omitempty"`
	Verified    bool   `json:"verified,omitempty"`
	Locked      bool   `json:"locked,omitempty"`
}

func (s sgSender) identity() SenderIdentity {
	return SenderIdentity{
		ID: strconv.FormatInt(s.ID, 10), Nickname: s.Nickname, FromEmail: s.FromEmail,
		FromName: s.FromName, ReplyTo: s.ReplyTo, Verified: s.Verified, Locked: s.Locked,
	}
}

// SendGrid field limits (POST /v3/verified_senders).
const (
	maxNickname = 100
	maxEmail    = 256
	maxName     = 256
	maxAddress  = 100
	maxCity     = 150
	maxZip      = 10
	maxCountry  = 100
)

// CreateSender implements SenderVerifier with POST /v3/verified_senders.
// SendGrid answers 201 and e-mails the confirmation link to FromEmail.
func (c *SendGridSenders) CreateSender(ctx context.Context, in NewSenderIdentity) (SenderIdentity, error) {
	body := sgSender{
		Nickname:    truncateRunes(in.Nickname, maxNickname),
		FromEmail:   in.FromEmail,
		FromName:    truncateRunes(in.FromName, maxName),
		ReplyTo:     in.ReplyTo,
		ReplyToName: truncateRunes(in.ReplyToName, maxName),
		Address:     truncateRunes(in.Address, maxAddress),
		City:        truncateRunes(in.City, maxCity),
		Zip:         truncateRunes(in.Zip, maxZip),
		Country:     truncateRunes(in.Country, maxCountry),
	}
	if body.Nickname == "" || body.FromEmail == "" || body.ReplyTo == "" ||
		len(body.FromEmail) > maxEmail || len(body.ReplyTo) > maxEmail {
		return SenderIdentity{}, fmt.Errorf("create sender: %w: nickname, from_email and reply_to are required", ErrSenderRejected)
	}
	var out sgSender
	if err := c.do(ctx, http.MethodPost, "/v3/verified_senders", body, &out); err != nil {
		return SenderIdentity{}, fmt.Errorf("create sender: %w", err)
	}
	c.logger.Info("sendgrid sender identity created", "sender_id", out.ID, "verified", out.Verified)
	return out.identity(), nil
}

// GetSender implements SenderVerifier with GET /v3/verified_senders?id=.
func (c *SendGridSenders) GetSender(ctx context.Context, id string) (SenderIdentity, error) {
	n, err := parseSenderID(id)
	if err != nil {
		return SenderIdentity{}, err
	}
	var out struct {
		Results []sgSender `json:"results"`
	}
	q := url.Values{"id": {strconv.FormatInt(n, 10)}}
	if err := c.do(ctx, http.MethodGet, "/v3/verified_senders?"+q.Encode(), nil, &out); err != nil {
		return SenderIdentity{}, fmt.Errorf("get sender %d: %w", n, err)
	}
	for _, s := range out.Results {
		if s.ID == n {
			return s.identity(), nil
		}
	}
	return SenderIdentity{}, fmt.Errorf("get sender %d: %w", n, ErrSenderNotFound)
}

// ResendVerification implements SenderVerifier with
// POST /v3/verified_senders/resend/{id}.
func (c *SendGridSenders) ResendVerification(ctx context.Context, id string) error {
	n, err := parseSenderID(id)
	if err != nil {
		return err
	}
	if err := c.do(ctx, http.MethodPost, "/v3/verified_senders/resend/"+strconv.FormatInt(n, 10), nil, nil); err != nil {
		return fmt.Errorf("resend verification for sender %d: %w", n, err)
	}
	return nil
}

// DeleteSender implements SenderVerifier with DELETE /v3/verified_senders/{id}.
func (c *SendGridSenders) DeleteSender(ctx context.Context, id string) error {
	n, err := parseSenderID(id)
	if err != nil {
		return err
	}
	if err := c.do(ctx, http.MethodDelete, "/v3/verified_senders/"+strconv.FormatInt(n, 10), nil, nil); err != nil {
		return fmt.Errorf("delete sender %d: %w", n, err)
	}
	return nil
}

// do sends one request and decodes a 2xx JSON answer into out (when non-nil).
// Errors never contain the API key.
func (c *SendGridSenders) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.host+path, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("sendgrid verified senders: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return senderAPIError(resp.StatusCode, raw)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// senderAPIError turns a non-2xx answer into a SenderAPIError. SendGrid sends
// {"errors":[{"field":"from_email","message":"..."}]}.
func senderAPIError(status int, raw []byte) error {
	var parsed struct {
		Errors []struct {
			Field   string `json:"field"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	e := &SenderAPIError{Status: status}
	if json.Unmarshal(raw, &parsed) == nil {
		for _, pe := range parsed.Errors {
			msg := truncateRunes(pe.Message, 300)
			if pe.Field != "" {
				msg = pe.Field + ": " + msg
			}
			e.Messages = append(e.Messages, msg)
		}
	}
	switch {
	case status == http.StatusNotFound:
		e.kind = ErrSenderNotFound
	case status >= 400 && status < 500 && status != http.StatusTooManyRequests &&
		status != http.StatusUnauthorized && status != http.StatusForbidden:
		e.kind = ErrSenderRejected
	}
	return e
}

func parseSenderID(id string) (int64, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("sender id %q: %w", id, ErrSenderNotFound)
	}
	return n, nil
}

func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// fakeSenderPrefix starts every ID FakeSenders hands out.
const fakeSenderPrefix = "fake-"

// FakeSenders is an in-memory SenderVerifier for DRY_RUN and local
// development: it calls no API and sends no confirmation e-mail. New
// identities start unverified; ConfirmAfter lets the next status check report
// them verified, so the panel can show both states locally. An unknown
// "fake-" ID (created before an API restart) counts as a confirmed identity,
// so restarting never turns stored test identities into missing ones.
type FakeSenders struct {
	mu     sync.Mutex
	next   int64
	byID   map[string]SenderIdentity
	checks map[string]int
	// ConfirmAfter is how many GetSender calls report an identity as still
	// pending before it turns verified. Zero verifies on the first check.
	ConfirmAfter int
	logger       *slog.Logger
}

// NewFakeSenders returns an empty FakeSenders.
func NewFakeSenders(logger *slog.Logger) *FakeSenders {
	if logger == nil {
		logger = slog.Default()
	}
	return &FakeSenders{byID: make(map[string]SenderIdentity), checks: make(map[string]int), logger: logger}
}

// CreateSender implements SenderVerifier.
func (f *FakeSenders) CreateSender(_ context.Context, in NewSenderIdentity) (SenderIdentity, error) {
	if in.Nickname == "" || in.FromEmail == "" || in.ReplyTo == "" {
		return SenderIdentity{}, fmt.Errorf("create sender: %w: nickname, from_email and reply_to are required", ErrSenderRejected)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.byID {
		if strings.EqualFold(s.FromEmail, in.FromEmail) {
			return SenderIdentity{}, fmt.Errorf("create sender: %w", &SenderAPIError{
				Status: http.StatusBadRequest, Messages: []string{"from_email: already exists"}, kind: ErrSenderRejected,
			})
		}
	}
	f.next++
	s := SenderIdentity{
		ID: fakeSenderPrefix + strconv.FormatInt(f.next, 10), Nickname: in.Nickname,
		FromEmail: in.FromEmail, FromName: in.FromName, ReplyTo: in.ReplyTo,
	}
	f.byID[s.ID] = s
	f.logger.Info("fake sender identity created (no e-mail sent)", "sender_id", s.ID)
	return s, nil
}

// GetSender implements SenderVerifier.
func (f *FakeSenders) GetSender(_ context.Context, id string) (SenderIdentity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.byID[id]
	if !ok {
		if strings.HasPrefix(id, fakeSenderPrefix) {
			return SenderIdentity{ID: id, Verified: true}, nil
		}
		return SenderIdentity{}, fmt.Errorf("get sender %s: %w", id, ErrSenderNotFound)
	}
	if !s.Verified {
		if f.checks[id] >= f.ConfirmAfter {
			s.Verified = true
			f.byID[id] = s
		}
		f.checks[id]++
	}
	return s, nil
}

// ResendVerification implements SenderVerifier.
func (f *FakeSenders) ResendVerification(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byID[id]; !ok && !strings.HasPrefix(id, fakeSenderPrefix) {
		return fmt.Errorf("resend verification for sender %s: %w", id, ErrSenderNotFound)
	}
	return nil
}

// DeleteSender implements SenderVerifier.
func (f *FakeSenders) DeleteSender(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byID[id]; !ok && !strings.HasPrefix(id, fakeSenderPrefix) {
		return fmt.Errorf("delete sender %s: %w", id, ErrSenderNotFound)
	}
	delete(f.byID, id)
	delete(f.checks, id)
	return nil
}
