package senders

import (
	"context"
	"errors"
	"fmt"
	netmail "net/mail"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// Config is the part of the sending settings senders depends on. It is read
// on every call, so changes in the settings panel apply at once.
type Config struct {
	// Provider names the operator whose checks apply ("sendgrid", or "fake"
	// under DRY_RUN and with Mailpit). Domains are stored per provider.
	Provider string
	// Live is true when the operator itself refuses a From address it does not
	// know (SendGrid with DRY_RUN off). Otherwise every allowed address is usable.
	Live bool
	// Shared is EMAIL_FROM; ReplyTo the reply address of mail sent from it.
	Shared  providers.Address
	ReplyTo providers.Address
	// PreferOwn makes the user's own address the default whenever it is usable.
	PreferOwn bool
}

// Options configure a Service.
type Options struct {
	// API returns the operator's domain API for the current settings.
	API     func() DomainAPI
	Config  func() Config
	Auditor Auditor
	Now     func() time.Time
}

// Service manages sender domains and delegated addresses and answers which
// addresses a user may send from.
type Service struct {
	store Store
	opts  Options
}

// NewService returns a Service.
func NewService(store Store, opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Service{store: store, opts: opts}
}

// Options returns the addresses u may send from, shared first, then their own,
// then delegated ones by address, and the ID of the default one.
func (s *Service) Options(ctx context.Context, u User) ([]Option, string, error) {
	cfg := s.opts.Config()
	out := []Option{{ID: IDShared, Kind: KindShared, Address: cfg.Shared, Usable: true}}

	if own := strings.ToLower(strings.TrimSpace(u.Email)); own != "" && !strings.EqualFold(own, cfg.Shared.Email) {
		usable, reason, err := s.usable(ctx, cfg, own)
		if err != nil {
			return nil, "", err
		}
		out = append(out, Option{
			ID: IDOwn, Kind: KindOwn, Address: providers.Address{Email: own, Name: u.FullName},
			Usable: usable, Reason: reason,
		})
	}

	delegated, err := s.store.AddressesOf(ctx, u.ID)
	if err != nil {
		return nil, "", fmt.Errorf("sender options: %w", err)
	}
	for _, a := range delegated {
		usable, reason, err := s.usable(ctx, cfg, a.Email)
		if err != nil {
			return nil, "", err
		}
		out = append(out, Option{
			ID: a.ID, Kind: KindDelegated, Address: providers.Address{Email: a.Email, Name: a.Name},
			Usable: usable, Reason: reason,
		})
	}

	def := IDShared
	if cfg.PreferOwn {
		if i := slices.IndexFunc(out, func(o Option) bool { return o.ID == IDOwn }); i >= 0 && out[i].Usable {
			def = IDOwn
		}
	}
	return out, def, nil
}

// ResolveSender returns the From and Reply-To for a batch u sends with the
// option senderID (empty: the default one). An option u does not have, or one
// the operator would refuse right now, fails with ErrNotAllowed.
func (s *Service) ResolveSender(ctx context.Context, u User, senderID string) (from, replyTo providers.Address, err error) {
	opts, def, err := s.Options(ctx, u)
	if err != nil {
		return providers.Address{}, providers.Address{}, err
	}
	id := strings.ToLower(strings.TrimSpace(senderID))
	if id == "" {
		id = def
	}
	i := slices.IndexFunc(opts, func(o Option) bool { return o.ID == id })
	if i < 0 || !opts[i].Usable {
		return providers.Address{}, providers.Address{}, fmt.Errorf("sender %q: %w", senderID, ErrNotAllowed)
	}
	if opts[i].Kind == KindShared {
		return opts[i].Address, s.opts.Config().ReplyTo, nil
	}
	// Replies to a personal or delegated address go to that address.
	return opts[i].Address, providers.Address{}, nil
}

// usable reports whether the operator accepts email as From right now.
func (s *Service) usable(ctx context.Context, cfg Config, email string) (bool, Reason, error) {
	if !cfg.Live {
		return true, ReasonNone, nil
	}
	_, domain, ok := strings.Cut(email, "@")
	if !ok {
		return false, ReasonDomainNotAuthenticated, nil
	}
	valid, err := s.store.DomainValid(ctx, cfg.Provider, strings.ToLower(domain))
	if err != nil {
		return false, ReasonNone, fmt.Errorf("check sender domain: %w", err)
	}
	if valid {
		return true, ReasonNone, nil
	}
	exists, verified, err := s.store.SingleSender(ctx, cfg.Provider, email)
	if err != nil {
		return false, ReasonNone, fmt.Errorf("check single sender: %w", err)
	}
	switch {
	case verified:
		return true, ReasonNone, nil
	case exists:
		return false, ReasonNotVerified, nil
	default:
		return false, ReasonDomainNotAuthenticated, nil
	}
}

// AddressUsable reports whether a delegated address can send right now, for
// the administrator's list.
func (s *Service) AddressUsable(ctx context.Context, email string) (bool, Reason, error) {
	return s.usable(ctx, s.opts.Config(), email)
}

// Live reports whether the current operator checks sender addresses, for the panel.
func (s *Service) Live() bool { return s.opts.Config().Live }

// --- Domains ---------------------------------------------------------------

// hostname is a DNS name with at least two labels, e.g. szkola.example.test.
var hostname = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{0,61}[a-z0-9]$`)

// ListDomains returns every domain as last checked; it calls no operator.
func (s *Service) ListDomains(ctx context.Context) ([]Domain, error) {
	ds, err := s.store.ListDomains(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sender domains: %w", err)
	}
	return ds, nil
}

// AddDomain asks the operator to authenticate domain and stores the DNS
// records the school must add.
func (s *Service) AddDomain(ctx context.Context, actorID, domain string) (Domain, error) {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if len(domain) > 253 || !hostname.MatchString(domain) {
		return Domain{}, invalid("domain", "must be a domain name like szkola.example.test")
	}
	// Checked before the operator, which would refuse a duplicate with a less
	// helpful error and must not get a second copy of a domain we already track.
	existing, err := s.store.ListDomains(ctx)
	if err != nil {
		return Domain{}, fmt.Errorf("list sender domains: %w", err)
	}
	if slices.ContainsFunc(existing, func(d Domain) bool { return d.Domain == domain }) {
		return Domain{}, ErrDomainExists
	}
	cfg := s.opts.Config()
	pd, err := s.opts.API().CreateDomain(ctx, domain)
	if err != nil {
		return Domain{}, fmt.Errorf("authenticate domain: %w", err)
	}
	status := DomainPending
	if pd.Valid {
		status = DomainValid
	}
	now := s.opts.Now()
	d, err := s.store.CreateDomain(ctx, Domain{
		Domain: domain, Provider: cfg.Provider, ProviderID: pd.ID, Status: status,
		Records: pd.Records, CreatedAt: now, CheckedAt: now,
	}, actorID)
	if err != nil {
		// Do not leave a domain at the operator that nothing points to.
		if delErr := s.opts.API().DeleteDomain(ctx, pd.ID); delErr != nil && !errors.Is(delErr, ErrDomainGone) {
			err = errors.Join(err, fmt.Errorf("roll back operator domain %s: %w", pd.ID, delErr))
		}
		return Domain{}, fmt.Errorf("store sender domain: %w", err)
	}
	s.audit(ctx, actorID, ActionDomainAdded, entityDomain, d.ID, map[string]string{"domain": domain, "provider": cfg.Provider})
	return d, nil
}

// CheckDomain asks the operator to validate the DNS records now and stores
// the answer. A domain added under another operator is only reported as missing.
func (s *Service) CheckDomain(ctx context.Context, id string) (Domain, error) {
	d, err := s.store.GetDomain(ctx, id)
	if err != nil {
		return Domain{}, fmt.Errorf("load sender domain: %w", err)
	}
	if d.Provider != s.opts.Config().Provider {
		return s.store.SaveDomainCheck(ctx, d.ID, DomainMissing, d.Records, s.opts.Now())
	}
	pd, err := s.opts.API().ValidateDomain(ctx, d.ProviderID)
	switch {
	case errors.Is(err, ErrDomainGone):
		return s.store.SaveDomainCheck(ctx, d.ID, DomainMissing, d.Records, s.opts.Now())
	case err != nil:
		return Domain{}, fmt.Errorf("validate domain: %w", err)
	}
	status := DomainPending
	if pd.Valid {
		status = DomainValid
	}
	records := pd.Records
	if len(records) == 0 {
		records = d.Records
	}
	return s.store.SaveDomainCheck(ctx, d.ID, status, records, s.opts.Now())
}

// RemoveDomain deletes the domain at the operator (when it is ours and still
// there) and locally.
func (s *Service) RemoveDomain(ctx context.Context, actorID, id string) error {
	d, err := s.store.GetDomain(ctx, id)
	if err != nil {
		return fmt.Errorf("load sender domain: %w", err)
	}
	if d.Provider == s.opts.Config().Provider {
		if err := s.opts.API().DeleteDomain(ctx, d.ProviderID); err != nil && !errors.Is(err, ErrDomainGone) {
			return fmt.Errorf("delete operator domain: %w", err)
		}
	}
	if err := s.store.DeleteDomain(ctx, d.ID); err != nil {
		return fmt.Errorf("delete sender domain: %w", err)
	}
	s.audit(ctx, actorID, ActionDomainRemoved, entityDomain, d.ID, map[string]string{"domain": d.Domain})
	return nil
}

// --- Delegated addresses -----------------------------------------------------

// ListAddresses returns every delegated address with its users.
func (s *Service) ListAddresses(ctx context.Context) ([]Address, error) {
	as, err := s.store.ListAddresses(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sender addresses: %w", err)
	}
	return as, nil
}

// CreateAddress adds a mailbox and delegates it to userIDs.
func (s *Service) CreateAddress(ctx context.Context, actorID, email, name string, userIDs []string) (Address, error) {
	email, name, err := checkAddress(email, name)
	if err != nil {
		return Address{}, err
	}
	ids, err := checkUserIDs(userIDs)
	if err != nil {
		return Address{}, err
	}
	a, err := s.store.CreateAddress(ctx, Address{Email: email, Name: name}, ids, actorID)
	if err != nil {
		return Address{}, s.addressError(err)
	}
	s.audit(ctx, actorID, ActionAddressCreated, entityAddress, a.ID, map[string]string{"users": fmt.Sprint(len(ids))})
	return a, nil
}

// UpdateAddress renames the mailbox and/or replaces the users it is delegated
// to; nil leaves a field unchanged.
func (s *Service) UpdateAddress(ctx context.Context, actorID, id string, name *string, userIDs *[]string) (Address, error) {
	var newName *string
	if name != nil {
		n, err := checkName(*name)
		if err != nil {
			return Address{}, err
		}
		newName = &n
	}
	var ids []string
	if userIDs != nil {
		var err error
		if ids, err = checkUserIDs(*userIDs); err != nil {
			return Address{}, err
		}
	}
	a, err := s.store.UpdateAddress(ctx, id, newName, ids, userIDs != nil, actorID)
	if err != nil {
		return Address{}, s.addressError(err)
	}
	details := map[string]string{"users": fmt.Sprint(len(a.Users))}
	if name != nil {
		details["renamed"] = "true"
	}
	s.audit(ctx, actorID, ActionAddressUpdated, entityAddress, a.ID, details)
	return a, nil
}

// DeleteAddress removes the mailbox; nobody can send from it any more.
func (s *Service) DeleteAddress(ctx context.Context, actorID, id string) error {
	if err := s.store.DeleteAddress(ctx, id); err != nil {
		return fmt.Errorf("delete sender address: %w", err)
	}
	s.audit(ctx, actorID, ActionAddressRemoved, entityAddress, id, nil)
	return nil
}

func (s *Service) addressError(err error) error {
	switch {
	case errors.Is(err, ErrUnknownUsers):
		return invalid("userIds", "every ID must be an active user")
	case errors.Is(err, ErrAddressExists), errors.Is(err, ErrNotFound):
		return err
	default:
		return fmt.Errorf("save sender address: %w", err)
	}
}

func (s *Service) audit(ctx context.Context, actorID, action, entity, entityID string, details map[string]string) {
	if s.opts.Auditor != nil {
		s.opts.Auditor.Record(ctx, actorID, action, entity, entityID, details)
	}
}

// checkAddress accepts a bare address (no display name) and a display name.
func checkAddress(email, name string) (string, string, error) {
	var fields []FieldError
	e := strings.ToLower(strings.TrimSpace(email))
	if a, err := netmail.ParseAddress(e); err != nil || a.Address != e || a.Name != "" || len(e) > 254 {
		fields = append(fields, FieldError{"email", "must be one address like sekretariat@example.test"})
	}
	n, err := checkName(name)
	if err != nil {
		var v *ValidationError
		if errors.As(err, &v) {
			fields = append(fields, v.Fields...)
		}
	}
	if len(fields) > 0 {
		return "", "", &ValidationError{Fields: fields}
	}
	return e, n, nil
}

func checkName(name string) (string, error) {
	n := strings.TrimSpace(name)
	if n == "" || utf8.RuneCountInString(n) > 100 || strings.ContainsAny(n, "\r\n\"<>") {
		return "", invalid("name", "must be 1-100 characters without quotes, angle brackets or line breaks")
	}
	return n, nil
}

// uuidLike matches the canonical UUID form the database stores.
var uuidLike = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func checkUserIDs(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, id := range in {
		id = strings.ToLower(strings.TrimSpace(id))
		if !uuidLike.MatchString(id) {
			return nil, invalid("userIds", "every ID must be a user ID")
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out, nil
}

// ParseAddress turns an RFC 5322 setting value ("Szkoła <a@example.test>")
// into an Address; an empty or unparsable value gives the zero Address.
func ParseAddress(v string) providers.Address {
	a, err := netmail.ParseAddress(strings.TrimSpace(v))
	if err != nil {
		return providers.Address{}
	}
	return providers.Address{Email: strings.ToLower(a.Address), Name: a.Name}
}
