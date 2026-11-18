//go:build integration

package senders

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database/dbtest"
)

func TestPGStoreSenders(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	store := NewPGStore(pool)

	ids := map[string]string{}
	for _, u := range []struct{ key, email, name, disabled string }{
		{"anna", "anna@szkola.example.test", "Anna Nowak", ""},
		{"jan", "jan@szkola.example.test", "Jan Kowalski", ""},
		{"off", "off@szkola.example.test", "Konto Zablokowane", "now()"},
	} {
		var id string
		err := pool.QueryRow(ctx, `
			INSERT INTO users (email, full_name, role, password_hash, disabled_at)
			VALUES ($1, $2, 'sender', 'x', CASE WHEN $3 = '' THEN NULL ELSE now() END) RETURNING id::text`,
			u.email, u.name, u.disabled).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		ids[u.key] = id
	}

	// Domains.
	now := time.Now().UTC().Truncate(time.Second)
	d, err := store.CreateDomain(ctx, Domain{
		Domain: "szkola.example.test", Provider: "sendgrid", ProviderID: "42", Status: DomainPending,
		Records: []DNSRecord{{Type: "cname", Host: "em42.szkola.example.test", Data: "u42.wl.sendgrid.net"}}, CreatedAt: now,
	}, ids["anna"])
	if err != nil {
		t.Fatalf("CreateDomain() error = %v", err)
	}
	if len(d.Records) != 1 || d.Records[0].Host != "em42.szkola.example.test" || !d.ValidatedAt.IsZero() {
		t.Errorf("created domain = %+v", d)
	}
	if _, err := store.CreateDomain(ctx, Domain{Domain: "szkola.example.test", Provider: "sendgrid", ProviderID: "43", Status: DomainPending, CreatedAt: now}, ""); !errors.Is(err, ErrDomainExists) {
		t.Errorf("duplicate domain err = %v, want ErrDomainExists", err)
	}
	if ok, _ := store.DomainValid(ctx, "sendgrid", "szkola.example.test"); ok {
		t.Error("pending domain reported valid")
	}
	d, err = store.SaveDomainCheck(ctx, d.ID, DomainValid, []DNSRecord{{Type: "cname", Host: "h", Data: "d", Valid: true}}, now)
	if err != nil || d.Status != DomainValid || !d.ValidatedAt.Equal(now) || !d.Records[0].Valid {
		t.Fatalf("SaveDomainCheck() = %+v, %v", d, err)
	}
	if ok, _ := store.DomainValid(ctx, "sendgrid", "SZKOLA.example.test"); !ok {
		t.Error("valid domain not found (case-insensitive)")
	}
	if ok, _ := store.DomainValid(ctx, "fake", "szkola.example.test"); ok {
		t.Error("domain of another provider counted as valid")
	}

	// Single senders (ADR-0016 table).
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_sender_identities (user_id, provider, provider_sender_id, from_email, from_name, status)
		VALUES ($1, 'sendgrid', '7', 'Jan@szkola.example.test', 'Jan', 'pending')`, ids["jan"]); err != nil {
		t.Fatal(err)
	}
	if exists, verified, err := store.SingleSender(ctx, "sendgrid", "jan@szkola.example.test"); err != nil || !exists || verified {
		t.Errorf("SingleSender(pending) = %v, %v, %v; want exists, not verified", exists, verified, err)
	}

	// Delegated addresses.
	a, err := store.CreateAddress(ctx, Address{Email: "sekretariat@szkola.example.test", Name: "Sekretariat"}, []string{ids["anna"], ids["jan"]}, ids["anna"])
	if err != nil {
		t.Fatalf("CreateAddress() error = %v", err)
	}
	if len(a.Users) != 2 || a.Users[0].FullName != "Anna Nowak" {
		t.Errorf("address users = %+v, want both, sorted by name", a.Users)
	}
	if _, err := store.CreateAddress(ctx, Address{Email: "sekretariat@szkola.example.test", Name: "X"}, nil, ""); !errors.Is(err, ErrAddressExists) {
		t.Errorf("duplicate address err = %v, want ErrAddressExists", err)
	}
	if _, err := store.CreateAddress(ctx, Address{Email: "dyrekcja@szkola.example.test", Name: "Dyrekcja"}, []string{ids["off"]}, ""); !errors.Is(err, ErrUnknownUsers) {
		t.Errorf("delegating to a disabled user: err = %v, want ErrUnknownUsers", err)
	}
	if as, _ := store.ListAddresses(ctx); len(as) != 1 {
		t.Errorf("a refused address was stored: %+v", as)
	}

	mine, err := store.AddressesOf(ctx, ids["jan"])
	if err != nil || len(mine) != 1 || mine[0].ID != a.ID {
		t.Errorf("AddressesOf(jan) = %+v, %v", mine, err)
	}
	name := "Sekretariat szkoły"
	a, err = store.UpdateAddress(ctx, a.ID, &name, []string{ids["anna"]}, true, ids["anna"])
	if err != nil || a.Name != name || len(a.Users) != 1 {
		t.Fatalf("UpdateAddress() = %+v, %v", a, err)
	}
	if mine, _ := store.AddressesOf(ctx, ids["jan"]); len(mine) != 0 {
		t.Errorf("Jan still has the address after the delegation moved: %+v", mine)
	}
	a, err = store.UpdateAddress(ctx, a.ID, nil, nil, false, "")
	if err != nil || a.Name != name || len(a.Users) != 1 {
		t.Errorf("UpdateAddress(no changes) = %+v, %v; want everything kept", a, err)
	}

	if err := store.DeleteAddress(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetAddress(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAddress(deleted) err = %v, want ErrNotFound", err)
	}
	if err := store.DeleteDomain(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"not-a-uuid", "00000000-0000-4000-8000-000000000000"} {
		if _, err := store.GetDomain(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetDomain(%q) err = %v, want ErrNotFound", id, err)
		}
	}
}
