//go:build integration

package groups

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database/dbtest"
	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

type pgEnv struct {
	pool   *pgxpool.Pool
	store  *PGStore
	dir    *recipients.PGStore
	byName map[string]string // first name -> recipient ID
}

// newPGEnv returns stores over a migrated schema holding the package's
// fictional fixture recipients (helpers_test.go), with database-assigned IDs.
func newPGEnv(t *testing.T) pgEnv {
	t.Helper()
	pool := dbtest.New(t)
	env := pgEnv{pool: pool, store: NewPGStore(pool), dir: recipients.NewPGStore(pool), byName: map[string]string{}}
	ctx := context.Background()

	var valid []recipients.Recipient
	for _, r := range fixtureRecipients() {
		if r.FirstName == "Oskar" {
			continue // invalid contact data on purpose; not storable through CreateRecipients
		}
		valid = append(valid, r)
	}
	if err := env.dir.CreateRecipients(ctx, valid); err != nil {
		t.Fatalf("CreateRecipients() error = %v", err)
	}
	for _, typ := range []recipients.Type{recipients.TypeParent, recipients.TypeStudent} {
		rs, err := env.dir.RecipientsByType(ctx, typ)
		if err != nil {
			t.Fatalf("RecipientsByType() error = %v", err)
		}
		for _, r := range rs {
			env.byName[r.FirstName] = r.ID
		}
	}
	return env
}

func TestMigrationInsertsSystemGroups(t *testing.T) {
	env := newPGEnv(t)
	for _, g := range SystemGroups() {
		var name string
		var isSystem bool
		err := env.pool.QueryRow(context.Background(), `SELECT name, is_system FROM groups WHERE id = $1`, g.ID).Scan(&name, &isSystem)
		if err != nil || name != g.Name || !isSystem {
			t.Errorf("row %s = (%q, %v), %v; want (%q, true)", g.ID, name, isSystem, err, g.Name)
		}
	}
}

func TestPGStoreHidesSystemGroups(t *testing.T) {
	env := newPGEnv(t)
	ctx := context.Background()

	list, err := env.store.ListGroups(ctx)
	if err != nil || len(list) != 0 {
		t.Errorf("ListGroups() = %v, %v; want no groups (built-in rows hidden)", list, err)
	}
	if found, err := env.store.FindGroups(ctx, []string{AllParentsID}); err != nil || len(found) != 0 {
		t.Errorf("FindGroups(system) = %v, %v; want nothing", found, err)
	}
	checks := map[string]error{
		"UpdateGroup": env.store.UpdateGroup(ctx, Group{ID: AllParentsID, Name: "Zmieniona"}),
		"DeleteGroup": env.store.DeleteGroup(ctx, AllStudentsID),
		"AddMembers": func() error {
			_, err := env.store.AddMembers(ctx, AllParentsID, []string{env.byName["Jan"]})
			return err
		}(),
		"RemoveMembers": func() error {
			_, err := env.store.RemoveMembers(ctx, AllParentsID, []string{env.byName["Jan"]})
			return err
		}(),
	}
	for name, err := range checks {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s(system group) error = %v, want ErrNotFound", name, err)
		}
	}
}

func TestPGStoreGroupLifecycle(t *testing.T) {
	env := newPGEnv(t)
	ctx := context.Background()

	g, err := env.store.CreateGroup(ctx, Group{Name: "Rada rodziców", Description: "Spotkania co miesiąc"})
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}
	if !IsValidID(g.ID) || g.CreatedAt.IsZero() || g.Kind != KindCustom {
		t.Errorf("CreateGroup() = %+v, want ID, CreatedAt and custom kind", g)
	}

	tests := []struct {
		name string
		err  error
	}{
		{"same name", func() error { _, err := env.store.CreateGroup(ctx, Group{Name: "Rada rodziców"}); return err }()},
		{"other letter case", func() error { _, err := env.store.CreateGroup(ctx, Group{Name: "RADA RODZICÓW"}); return err }()},
	}
	for _, tt := range tests {
		if !errors.Is(tt.err, ErrNameTaken) {
			t.Errorf("CreateGroup(%s) error = %v, want ErrNameTaken", tt.name, tt.err)
		}
	}

	other, err := env.store.CreateGroup(ctx, Group{Name: "Wycieczka"})
	if err != nil {
		t.Fatalf("CreateGroup(Wycieczka) error = %v", err)
	}
	if err := env.store.UpdateGroup(ctx, Group{ID: other.ID, Name: "rada rodziców"}); !errors.Is(err, ErrNameTaken) {
		t.Errorf("UpdateGroup(taken name) error = %v, want ErrNameTaken", err)
	}
	if err := env.store.UpdateGroup(ctx, Group{ID: strings.ToUpper(other.ID), Name: "Wycieczka 2026", Description: ""}); err != nil {
		t.Errorf("UpdateGroup() error = %v", err)
	}

	found, err := env.store.FindGroups(ctx, []string{other.ID, "nie-uuid", idMissingGroup})
	if err != nil || len(found) != 1 || found[0].Name != "Wycieczka 2026" || found[0].Description != "" {
		t.Errorf("FindGroups() = %+v, %v; want the renamed group only", found, err)
	}

	if err := env.store.DeleteGroup(ctx, other.ID); err != nil {
		t.Errorf("DeleteGroup() error = %v", err)
	}
	if err := env.store.DeleteGroup(ctx, other.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteGroup(again) error = %v, want ErrNotFound", err)
	}
	if list, err := env.store.ListGroups(ctx); err != nil || len(list) != 1 || list[0].ID != g.ID {
		t.Errorf("ListGroups() = %+v, %v; want only %s", list, err, g.ID)
	}
}

func TestPGStoreMembership(t *testing.T) {
	env := newPGEnv(t)
	ctx := context.Background()
	g, err := env.store.CreateGroup(ctx, Group{Name: "Rodzice 3A"})
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}
	jan, maria, lena := env.byName["Jan"], env.byName["Maria"], env.byName["Lena"]

	tests := []struct {
		name    string
		op      func() (int, error)
		want    int
		members []string
	}{
		{"add two", func() (int, error) { return env.store.AddMembers(ctx, g.ID, []string{jan, maria}) }, 2, []string{jan, maria}},
		{"add again, one new, repeated", func() (int, error) {
			return env.store.AddMembers(ctx, g.ID, []string{strings.ToUpper(jan), lena, lena})
		}, 1, []string{jan, maria, lena}},
		{"remove one and a non-member", func() (int, error) {
			return env.store.RemoveMembers(ctx, g.ID, []string{maria, idNobody})
		}, 1, []string{jan, lena}},
	}
	for _, tt := range tests {
		n, err := tt.op()
		if err != nil || n != tt.want {
			t.Fatalf("%s: = %d, %v; want %d", tt.name, n, err, tt.want)
		}
		got, err := env.store.Members(ctx, []string{g.ID, AllParentsID})
		if err != nil {
			t.Fatalf("%s: Members() error = %v", tt.name, err)
		}
		want := sortedCopy(tt.members)
		if !reflect.DeepEqual(got, map[string][]string{g.ID: want}) {
			t.Errorf("%s: Members() = %v, want %v (by recipient ID, built-in group absent)", tt.name, got, want)
		}
	}

	if _, err := env.store.AddMembers(ctx, g.ID, []string{idNobody}); !errors.Is(err, ErrUnknownRecipient) {
		t.Errorf("AddMembers(unknown recipient) error = %v, want ErrUnknownRecipient", err)
	}
	if _, err := env.store.AddMembers(ctx, idMissingGroup, []string{jan}); !errors.Is(err, ErrNotFound) {
		t.Errorf("AddMembers(unknown group) error = %v, want ErrNotFound", err)
	}
}

// TestResolveWithPGStores runs the real Resolver over Postgres: a person in a
// built-in group, a custom group and picked by hand still appears once.
func TestResolveWithPGStores(t *testing.T) {
	env := newPGEnv(t)
	ctx := context.Background()
	g, err := env.store.CreateGroup(ctx, Group{Name: "Rodzice 3A"})
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}
	jan, lena := env.byName["Jan"], env.byName["Lena"]
	if _, err := env.store.AddMembers(ctx, g.ID, []string{jan, lena}); err != nil {
		t.Fatalf("AddMembers() error = %v", err)
	}

	res, err := NewResolver(env.store, env.dir).Resolve(ctx, Selection{
		GroupIDs:     []string{AllParentsID, g.ID},
		RecipientIDs: []string{jan},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	var got []string
	for _, r := range res.Recipients {
		got = append(got, r.Recipient.FirstName)
	}
	if want := []string{"Maria", "Jan", "Lena", "Zofia"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Resolve() recipients = %v, want %v", got, want)
	}
	if res.MergedDuplicates != 2 {
		t.Errorf("MergedDuplicates = %d, want 2 (Jan via custom group and by hand)", res.MergedDuplicates)
	}
}

func sortedCopy(ids []string) []string {
	out := slices.Clone(ids)
	slices.Sort(out)
	return out
}

func TestClassGroupsWithPostgres(t *testing.T) {
	env := newPGEnv(t)
	ctx := context.Background()
	for name, classes := range map[string][]string{"Lena": {"3A"}, "Jan": {"1B", "3A"}, "Zofia": {"3A"}} {
		r, err := env.dir.GetRecipient(ctx, env.byName[name])
		if err != nil {
			t.Fatal(err)
		}
		r.Classes = classes
		if _, err := env.dir.UpdateRecipient(ctx, r); err != nil {
			t.Fatalf("UpdateRecipient(%s) error = %v", name, err)
		}
	}

	list, err := NewService(env.store, env.dir).List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	counts := map[string]int{}
	for _, g := range list {
		counts[g.Name] = g.MemberCount
	}
	for name, want := range map[string]int{
		"Uczniowie klasy 3A": 1, "Rodzice uczniów klasy 3A": 2,
		"Uczniowie klasy 1B": 0, "Rodzice uczniów klasy 1B": 1,
	} {
		if counts[name] != want {
			t.Errorf("%s has %d members, want %d (all: %v)", name, counts[name], want, counts)
		}
	}

	res, err := NewResolver(env.store, env.dir).Resolve(ctx, Selection{GroupIDs: []string{
		classGroupID(AudienceParents, "3A"), classGroupID(AudienceParents, "1B"),
	}})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(res.Recipients) != 2 || res.MergedDuplicates != 1 {
		t.Errorf("Resolve(parents of 3A and 1B) = %d recipients, %d merged, want Jan once and Zofia", len(res.Recipients), res.MergedDuplicates)
	}
}
