package senders

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

const (
	userA = "00000000-0000-4000-8000-0000000000a1"
	userB = "00000000-0000-4000-8000-0000000000b2"
	admin = "00000000-0000-4000-8000-0000000000ad"
)

// memStore is an in-memory Store for the service tests.
type memStore struct {
	next      int
	domains   map[string]Domain
	addresses map[string]Address
	users     map[string]string // id -> full name, active users only
	singles   map[string]bool   // email -> verified
	failNext  error
}

func newMemStore() *memStore {
	return &memStore{
		domains: map[string]Domain{}, addresses: map[string]Address{}, singles: map[string]bool{},
		users: map[string]string{userA: "Anna Nowak", userB: "Jan Kowalski", admin: "Admin"},
	}
}

func (m *memStore) id() string {
	m.next++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", m.next)
}

func (m *memStore) ListDomains(context.Context) ([]Domain, error) {
	var out []Domain
	for _, d := range m.domains {
		out = append(out, d)
	}
	return out, nil
}

func (m *memStore) GetDomain(_ context.Context, id string) (Domain, error) {
	d, ok := m.domains[id]
	if !ok {
		return Domain{}, ErrNotFound
	}
	return d, nil
}

func (m *memStore) CreateDomain(_ context.Context, d Domain, _ string) (Domain, error) {
	if m.failNext != nil {
		err := m.failNext
		m.failNext = nil
		return Domain{}, err
	}
	for _, x := range m.domains {
		if x.Domain == d.Domain {
			return Domain{}, ErrDomainExists
		}
	}
	d.ID = m.id()
	m.domains[d.ID] = d
	return d, nil
}

func (m *memStore) SaveDomainCheck(_ context.Context, id string, status DomainStatus, records []DNSRecord, at time.Time) (Domain, error) {
	d, ok := m.domains[id]
	if !ok {
		return Domain{}, ErrNotFound
	}
	d.Status, d.Records, d.CheckedAt = status, records, at
	if status == DomainValid && d.ValidatedAt.IsZero() {
		d.ValidatedAt = at
	}
	m.domains[id] = d
	return d, nil
}

func (m *memStore) DeleteDomain(_ context.Context, id string) error {
	if _, ok := m.domains[id]; !ok {
		return ErrNotFound
	}
	delete(m.domains, id)
	return nil
}

func (m *memStore) ListAddresses(context.Context) ([]Address, error) {
	var out []Address
	for _, a := range m.addresses {
		out = append(out, a)
	}
	return out, nil
}

func (m *memStore) GetAddress(_ context.Context, id string) (Address, error) {
	a, ok := m.addresses[id]
	if !ok {
		return Address{}, ErrNotFound
	}
	return a, nil
}

func (m *memStore) refs(ids []string) ([]UserRef, error) {
	refs := make([]UserRef, 0, len(ids))
	for _, id := range ids {
		name, ok := m.users[id]
		if !ok {
			return nil, ErrUnknownUsers
		}
		refs = append(refs, UserRef{ID: id, FullName: name})
	}
	return refs, nil
}

func (m *memStore) CreateAddress(_ context.Context, a Address, userIDs []string, _ string) (Address, error) {
	for _, x := range m.addresses {
		if x.Email == a.Email {
			return Address{}, ErrAddressExists
		}
	}
	refs, err := m.refs(userIDs)
	if err != nil {
		return Address{}, err
	}
	a.ID, a.Users = m.id(), refs
	m.addresses[a.ID] = a
	return a, nil
}

func (m *memStore) UpdateAddress(_ context.Context, id string, name *string, userIDs []string, setUsers bool, _ string) (Address, error) {
	a, ok := m.addresses[id]
	if !ok {
		return Address{}, ErrNotFound
	}
	if name != nil {
		a.Name = *name
	}
	if setUsers {
		refs, err := m.refs(userIDs)
		if err != nil {
			return Address{}, err
		}
		a.Users = refs
	}
	m.addresses[id] = a
	return a, nil
}

func (m *memStore) DeleteAddress(_ context.Context, id string) error {
	if _, ok := m.addresses[id]; !ok {
		return ErrNotFound
	}
	delete(m.addresses, id)
	return nil
}

func (m *memStore) AddressesOf(_ context.Context, userID string) ([]Address, error) {
	var out []Address
	for _, a := range m.addresses {
		if slices.ContainsFunc(a.Users, func(u UserRef) bool { return u.ID == userID }) {
			out = append(out, a)
		}
	}
	slices.SortFunc(out, func(x, y Address) int { return compare(x.Email, y.Email) })
	return out, nil
}

func compare(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func (m *memStore) DomainValid(_ context.Context, provider, domain string) (bool, error) {
	for _, d := range m.domains {
		if d.Provider == provider && d.Domain == domain && d.Status == DomainValid {
			return true, nil
		}
	}
	return false, nil
}

func (m *memStore) SingleSender(_ context.Context, _ string, email string) (bool, bool, error) {
	verified, ok := m.singles[email]
	return ok, verified, nil
}

// fakeAPI is a DomainAPI that validates on the first check.
type fakeAPI struct {
	created, deleted []string
	gone             bool
}

func (f *fakeAPI) CreateDomain(_ context.Context, domain string) (ProviderDomain, error) {
	f.created = append(f.created, domain)
	return ProviderDomain{ID: "p-" + domain, Records: []DNSRecord{{Type: "cname", Host: "em1." + domain, Data: "u1.wl.sendgrid.net"}}}, nil
}

func (f *fakeAPI) GetDomain(_ context.Context, id string) (ProviderDomain, error) {
	return ProviderDomain{ID: id}, nil
}

func (f *fakeAPI) ValidateDomain(_ context.Context, id string) (ProviderDomain, error) {
	if f.gone {
		return ProviderDomain{}, ErrDomainGone
	}
	return ProviderDomain{ID: id, Valid: true, Records: []DNSRecord{{Type: "cname", Host: "em1", Data: "x", Valid: true}}}, nil
}

func (f *fakeAPI) DeleteDomain(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type auditLog struct{ actions []string }

func (a *auditLog) Record(_ context.Context, _, action, _, _ string, _ map[string]string) {
	a.actions = append(a.actions, action)
}

func newService(cfg *Config) (*Service, *memStore, *fakeAPI, *auditLog) {
	store, api, audit := newMemStore(), &fakeAPI{}, &auditLog{}
	svc := NewService(store, Options{
		API:     func() DomainAPI { return api },
		Config:  func() Config { return *cfg },
		Auditor: audit,
		Now:     func() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) },
	})
	return svc, store, api, audit
}

func liveConfig() *Config {
	return &Config{
		Provider: "sendgrid", Live: true, PreferOwn: true,
		Shared:  providers.Address{Email: "no-reply@szkola.example.test", Name: "Szkoła"},
		ReplyTo: providers.Address{Email: "sekretariat@szkola.example.test"},
	}
}

var anna = User{ID: userA, Email: "Anna.Nowak@szkola.example.test", FullName: "Anna Nowak"}

func TestOptionsWithoutLiveOperatorEverythingIsUsable(t *testing.T) {
	cfg := liveConfig()
	cfg.Live, cfg.Provider = false, "fake"
	svc, _, _, _ := newService(cfg)

	opts, def, err := svc.Options(context.Background(), anna)
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 2 || opts[0].ID != IDShared || opts[1].ID != IDOwn {
		t.Fatalf("options = %+v, want shared and own", opts)
	}
	if !opts[1].Usable || opts[1].Address.Email != "anna.nowak@szkola.example.test" || opts[1].Address.Name != "Anna Nowak" {
		t.Errorf("own option = %+v, want usable, lower-cased address and the user's name", opts[1])
	}
	if def != IDOwn {
		t.Errorf("default = %q, want own", def)
	}

	cfg.PreferOwn = false
	if _, def, _ := svc.Options(context.Background(), anna); def != IDShared {
		t.Errorf("default with PreferOwn off = %q, want shared", def)
	}
}

func TestLiveOperatorNeedsAuthenticatedDomainOrVerifiedSender(t *testing.T) {
	cfg := liveConfig()
	svc, store, _, _ := newService(cfg)
	ctx := context.Background()

	opts, def, _ := svc.Options(ctx, anna)
	if own := opts[1]; own.Usable || own.Reason != ReasonDomainNotAuthenticated {
		t.Errorf("own option before any setup = %+v, want unusable, domain_not_authenticated", own)
	}
	if def != IDShared {
		t.Errorf("default = %q, want shared while own is unusable", def)
	}
	if _, _, err := svc.ResolveSender(ctx, anna, IDOwn); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("ResolveSender(own) err = %v, want ErrNotAllowed", err)
	}
	from, replyTo, err := svc.ResolveSender(ctx, anna, "")
	if err != nil || from != cfg.Shared || replyTo != cfg.ReplyTo {
		t.Errorf("ResolveSender(default) = %+v, %+v, %v; want the shared sender with its reply-to", from, replyTo, err)
	}

	// A pending single sender is "not verified"; a verified one is usable.
	store.singles["anna.nowak@szkola.example.test"] = false
	if opts, _, _ := svc.Options(ctx, anna); opts[1].Reason != ReasonNotVerified {
		t.Errorf("reason with a pending single sender = %q, want not_verified", opts[1].Reason)
	}
	store.singles["anna.nowak@szkola.example.test"] = true
	if opts, def, _ := svc.Options(ctx, anna); !opts[1].Usable || def != IDOwn {
		t.Errorf("verified single sender: own = %+v, default %q; want usable and default", opts[1], def)
	}
	delete(store.singles, "anna.nowak@szkola.example.test")

	// Authenticating the domain makes every address in it usable automatically.
	d, err := svc.AddDomain(ctx, admin, "Szkola.Example.Test.")
	if err != nil {
		t.Fatalf("AddDomain() error = %v", err)
	}
	if d.Domain != "szkola.example.test" || d.Status != DomainPending || len(d.Records) != 1 {
		t.Errorf("added domain = %+v, want normalized, pending, with the DNS records", d)
	}
	if opts, _, _ := svc.Options(ctx, anna); opts[1].Usable {
		t.Error("own address usable before the DNS records were validated")
	}
	if d, err = svc.CheckDomain(ctx, d.ID); err != nil || d.Status != DomainValid || d.ValidatedAt.IsZero() {
		t.Fatalf("CheckDomain() = %+v, %v; want valid", d, err)
	}
	from, replyTo, err = svc.ResolveSender(ctx, anna, "")
	if err != nil || from.Email != "anna.nowak@szkola.example.test" || !replyTo.IsZero() {
		t.Errorf("ResolveSender(default) after domain auth = %+v, %+v, %v; want own address, no reply-to", from, replyTo, err)
	}
}

func TestDelegatedAddresses(t *testing.T) {
	cfg := liveConfig()
	svc, _, _, audit := newService(cfg)
	ctx := context.Background()

	a, err := svc.CreateAddress(ctx, admin, " Sekretariat@Szkola.Example.Test ", "Sekretariat", []string{userA, userA})
	if err != nil {
		t.Fatalf("CreateAddress() error = %v", err)
	}
	if a.Email != "sekretariat@szkola.example.test" || len(a.Users) != 1 {
		t.Errorf("address = %+v, want normalized email and one user", a)
	}
	if _, err := svc.CreateAddress(ctx, admin, "sekretariat@szkola.example.test", "Inny", nil); !errors.Is(err, ErrAddressExists) {
		t.Errorf("duplicate address err = %v, want ErrAddressExists", err)
	}

	// Jan has no delegation: the address is not his to pick.
	jan := User{ID: userB, Email: "jan@inna.example.test", FullName: "Jan Kowalski"}
	if _, _, err := svc.ResolveSender(ctx, jan, a.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("ResolveSender for a user without delegation: err = %v, want ErrNotAllowed", err)
	}
	// Anna has it, but the operator would refuse it until the domain is valid.
	if _, _, err := svc.ResolveSender(ctx, anna, a.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("ResolveSender before domain auth: err = %v, want ErrNotAllowed", err)
	}
	d, _ := svc.AddDomain(ctx, admin, "szkola.example.test")
	_, _ = svc.CheckDomain(ctx, d.ID)
	from, replyTo, err := svc.ResolveSender(ctx, anna, a.ID)
	if err != nil || from.Email != a.Email || from.Name != "Sekretariat" || !replyTo.IsZero() {
		t.Errorf("ResolveSender(delegated) = %+v, %+v, %v", from, replyTo, err)
	}

	// Moving the delegation to Jan takes it away from Anna.
	ids := []string{userB}
	if _, err := svc.UpdateAddress(ctx, admin, a.ID, nil, &ids); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ResolveSender(ctx, anna, a.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("after the delegation moved: err = %v, want ErrNotAllowed", err)
	}
	if _, _, err := svc.ResolveSender(ctx, jan, a.ID); err != nil {
		t.Errorf("Jan after delegation: %v", err)
	}

	bad := []string{"00000000-0000-4000-8000-00000000dead"}
	if _, err := svc.UpdateAddress(ctx, admin, a.ID, nil, &bad); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown user: err = %v, want ErrInvalid", err)
	}
	if err := svc.DeleteAddress(ctx, admin, a.ID); err != nil {
		t.Fatal(err)
	}
	want := []string{ActionAddressCreated, ActionDomainAdded, ActionAddressUpdated, ActionAddressRemoved}
	if !slices.Equal(audit.actions, want) {
		t.Errorf("audit = %v, want %v", audit.actions, want)
	}
}

func TestInputValidation(t *testing.T) {
	svc, _, api, _ := newService(liveConfig())
	ctx := context.Background()
	for _, domain := range []string{"", "localhost", "szkoła.pl", "-a.example.test", "a..example.test"} {
		if _, err := svc.AddDomain(ctx, admin, domain); !errors.Is(err, ErrInvalid) {
			t.Errorf("AddDomain(%q) err = %v, want ErrInvalid", domain, err)
		}
	}
	if len(api.created) != 0 {
		t.Errorf("invalid domains reached the operator: %v", api.created)
	}
	for _, tt := range []struct{ email, name string }{
		{"Sekretariat <s@example.test>", "S"},
		{"s@example.test", ""},
		{"s@example.test", "Zły\r\nBcc: x"},
		{"not-an-address", "S"},
	} {
		if _, err := svc.CreateAddress(ctx, admin, tt.email, tt.name, nil); !errors.Is(err, ErrInvalid) {
			t.Errorf("CreateAddress(%q, %q) err = %v, want ErrInvalid", tt.email, tt.name, err)
		}
	}
}

func TestAddDomainRollsBackWhenNotStored(t *testing.T) {
	svc, store, api, _ := newService(liveConfig())
	store.failNext = errors.New("db down")
	if _, err := svc.AddDomain(context.Background(), admin, "szkola.example.test"); err == nil {
		t.Fatal("AddDomain() error = nil, want the store error")
	}
	if !slices.Equal(api.deleted, []string{"p-szkola.example.test"}) {
		t.Errorf("deleted at operator = %v, want the domain just created", api.deleted)
	}
}

func TestCheckDomainGoneAndOtherProvider(t *testing.T) {
	cfg := liveConfig()
	svc, _, api, _ := newService(cfg)
	ctx := context.Background()
	d, _ := svc.AddDomain(ctx, admin, "szkola.example.test")

	api.gone = true
	if got, err := svc.CheckDomain(ctx, d.ID); err != nil || got.Status != DomainMissing {
		t.Errorf("CheckDomain(gone) = %+v, %v; want missing", got, err)
	}

	// A domain added under the fake operator is not sent to the real one.
	api.gone = false
	cfg.Provider = "other"
	if got, err := svc.CheckDomain(ctx, d.ID); err != nil || got.Status != DomainMissing {
		t.Errorf("CheckDomain(other provider) = %+v, %v; want missing", got, err)
	}
	if err := svc.RemoveDomain(ctx, admin, d.ID); err != nil || len(api.deleted) != 0 {
		t.Errorf("RemoveDomain(other provider) = %v, deleted %v; want local removal only", err, api.deleted)
	}
}
