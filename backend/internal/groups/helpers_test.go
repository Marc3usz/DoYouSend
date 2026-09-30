package groups

import (
	"context"
	"errors"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

// Fictional recipients (CLAUDE.md rule 2): @example.test and +48 500 100 1NN.
const (
	idJan    = "11111111-1111-4111-8111-000000000001"
	idMaria  = "11111111-1111-4111-8111-000000000002"
	idLena   = "11111111-1111-4111-8111-000000000003"
	idOskar  = "11111111-1111-4111-8111-000000000004"
	idZofia  = "11111111-1111-4111-8111-000000000005"
	idNobody = "11111111-1111-4111-8111-0000000000ff"

	idMissingGroup = "22222222-2222-4222-8222-0000000000ff"
)

func fixtureRecipients() []recipients.Recipient {
	return []recipients.Recipient{
		{ID: idJan, FirstName: "Jan", LastName: "Kowalski", Email: "jan.kowalski@example.test", Phone: "+48500100101", Type: recipients.TypeParent},
		{ID: idMaria, FirstName: "Maria", LastName: "Kowalska", Email: "maria.kowalska@example.test", Type: recipients.TypeParent},
		{ID: idLena, FirstName: "Lena", LastName: "Nowak", Phone: "+48500100103", Type: recipients.TypeStudent},
		{ID: idOskar, FirstName: "Oskar", LastName: "Adamski", Email: "oskar.adamski@example", Phone: "500 100 104", Type: recipients.TypeStudent},
		{ID: idZofia, FirstName: "Zofia", LastName: "Wiśniewska", Email: "zofia.wisniewska@example.test", Phone: "+48500100105", Type: recipients.TypeParent},
	}
}

// fakeDirectory is an in-memory Directory that counts calls and can fail.
type fakeDirectory struct {
	byID map[string]recipients.Recipient
	err  error

	byIDCalls   int
	byTypeCalls map[recipients.Type]int
}

func newFakeDirectory(rs ...recipients.Recipient) *fakeDirectory {
	d := &fakeDirectory{byID: make(map[string]recipients.Recipient), byTypeCalls: make(map[recipients.Type]int)}
	for _, r := range rs {
		d.byID[r.ID] = r
	}
	return d
}

func (d *fakeDirectory) RecipientsByIDs(_ context.Context, ids []string) ([]recipients.Recipient, error) {
	d.byIDCalls++
	if d.err != nil {
		return nil, d.err
	}
	var out []recipients.Recipient
	for _, id := range ids {
		if r, ok := d.byID[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func (d *fakeDirectory) RecipientsByType(_ context.Context, t recipients.Type) ([]recipients.Recipient, error) {
	d.byTypeCalls[t]++
	if d.err != nil {
		return nil, d.err
	}
	var out []recipients.Recipient
	for _, r := range fixtureOrder(d) {
		if r.Type == t {
			out = append(out, r)
		}
	}
	return out, nil
}

// fixtureOrder returns the directory's recipients in fixture order, so
// results do not depend on map iteration.
func fixtureOrder(d *fakeDirectory) []recipients.Recipient {
	var out []recipients.Recipient
	for _, r := range fixtureRecipients() {
		if stored, ok := d.byID[r.ID]; ok {
			out = append(out, stored)
		}
	}
	return out
}

// failingStore wraps a Store and fails the named method.
type failingStore struct {
	Store
	method string
	err    error
}

func (s failingStore) fail(method string) error {
	if s.method == method {
		return s.err
	}
	return nil
}

func (s failingStore) ListGroups(ctx context.Context) ([]Group, error) {
	if err := s.fail("ListGroups"); err != nil {
		return nil, err
	}
	return s.Store.ListGroups(ctx)
}

func (s failingStore) FindGroups(ctx context.Context, ids []string) ([]Group, error) {
	if err := s.fail("FindGroups"); err != nil {
		return nil, err
	}
	return s.Store.FindGroups(ctx, ids)
}

func (s failingStore) Members(ctx context.Context, ids []string) (map[string][]string, error) {
	if err := s.fail("Members"); err != nil {
		return nil, err
	}
	return s.Store.Members(ctx, ids)
}

func (s failingStore) AddMembers(ctx context.Context, id string, ids []string) (int, error) {
	if err := s.fail("AddMembers"); err != nil {
		return 0, err
	}
	return s.Store.AddMembers(ctx, id, ids)
}

var errBoom = errors.New("boom")

// mustCreate creates a custom group with members, failing the test on error.
func mustCreate(t *testing.T, store Store, name string, members ...string) Group {
	t.Helper()
	g, err := store.CreateGroup(context.Background(), Group{Name: name})
	if err != nil {
		t.Fatalf("CreateGroup(%q) error = %v", name, err)
	}
	if len(members) > 0 {
		if _, err := store.AddMembers(context.Background(), g.ID, members); err != nil {
			t.Fatalf("AddMembers(%q) error = %v", name, err)
		}
	}
	return g
}
