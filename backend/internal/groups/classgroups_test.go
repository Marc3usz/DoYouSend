package groups

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

// classFixture adds classes to the fixture (ADR-0009): Lena is a student of
// 3A, Oskar of 1B; Jan is a parent with children in both classes, Zofia a
// parent in 3A. Maria has no class.
func classFixture() []recipients.Recipient {
	rs := fixtureRecipients()
	classes := map[string][]string{idLena: {"3A"}, idOskar: {"1B"}, idJan: {"1B", "3A"}, idZofia: {"3A"}}
	for i := range rs {
		rs[i].Classes = classes[rs[i].ID]
	}
	return rs
}

func TestClassGroupIDsAreUUIDv5(t *testing.T) {
	// Expected values computed independently with Python's uuid.uuid5. If this
	// test fails, saved selections and the history would stop resolving.
	for _, tt := range []struct {
		audience Audience
		class    string
		want     string
	}{
		{AudienceStudents, "3A", "eb338490-695b-50b8-9d27-f5a07709a346"},
		{AudienceParents, "3A", "3794ada9-f860-5faf-a77d-4029bb2bbc1b"},
	} {
		if got := classGroupID(tt.audience, tt.class); got != tt.want {
			t.Errorf("classGroupID(%s, %s) = %s, want %s", tt.audience, tt.class, got, tt.want)
		}
	}
}

func TestClassGroupsOrder(t *testing.T) {
	var names []string
	for _, g := range ClassGroups([]string{"10A", "1B", "2A", "1B"}) {
		names = append(names, g.Name)
		if !g.IsSystem() || g.Rule == nil || g.Rule.Class == "" {
			t.Errorf("%s is not a built-in class group: %+v", g.Name, g)
		}
	}
	want := []string{
		"Uczniowie klasy 1B", "Rodzice uczniów klasy 1B",
		"Uczniowie klasy 2A", "Rodzice uczniów klasy 2A",
		"Uczniowie klasy 10A", "Rodzice uczniów klasy 10A",
	}
	if !slices.Equal(names, want) {
		t.Errorf("ClassGroups() = %q, want %q", names, want)
	}
}

func TestServiceListsClassGroups(t *testing.T) {
	store := NewMemStore()
	svc := NewService(store, newFakeDirectory(classFixture()...))
	mustCreate(t, store, "Rada rodziców")

	list, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	type row struct {
		name  string
		count int
	}
	var got []row
	for _, g := range list {
		got = append(got, row{g.Name, g.MemberCount})
	}
	want := []row{
		{"Wszyscy rodzice", 3}, {"Wszyscy uczniowie", 2},
		{"Uczniowie klasy 1B", 1}, {"Rodzice uczniów klasy 1B", 1},
		{"Uczniowie klasy 3A", 1}, {"Rodzice uczniów klasy 3A", 2},
		{"Rada rodziców", 0},
	}
	if !slices.Equal(got, want) {
		t.Errorf("List() = %v, want %v", got, want)
	}
}

func TestResolveClassGroups(t *testing.T) {
	store := NewMemStore()
	dir := newFakeDirectory(classFixture()...)
	rv := NewResolver(store, dir)
	parents3A, parents1B := classGroupID(AudienceParents, "3A"), classGroupID(AudienceParents, "1B")

	res, err := rv.Resolve(context.Background(), Selection{GroupIDs: []string{parents3A, parents1B}})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	var ids []string
	for _, r := range res.Recipients {
		ids = append(ids, r.Recipient.ID)
	}
	// Jan has children in both classes: messaged once (CLAUDE.md rule 6).
	if want := []string{idJan, idZofia}; !slices.Equal(ids, want) {
		t.Errorf("recipients = %v, want %v", ids, want)
	}
	if res.MergedDuplicates != 1 || len(res.UnknownGroupIDs) != 0 {
		t.Errorf("merged = %d, unknown = %v, want 1 and none", res.MergedDuplicates, res.UnknownGroupIDs)
	}

	// A class nobody is in any more is an unknown group, not an empty one.
	gone := classGroupID(AudienceStudents, "7C")
	res, err = rv.Resolve(context.Background(), Selection{GroupIDs: []string{gone}})
	if err != nil || !slices.Equal(res.UnknownGroupIDs, []string{gone}) {
		t.Errorf("Resolve(empty class) = %+v, %v, want it reported unknown", res, err)
	}
}

func TestClassGroupsAreReadOnly(t *testing.T) {
	store := NewMemStore()
	svc := NewService(store, newFakeDirectory(classFixture()...))
	ctx := context.Background()
	id := classGroupID(AudienceStudents, "3A")

	g, err := svc.Get(ctx, id)
	if err != nil || g.Name != "Uczniowie klasy 3A" {
		t.Fatalf("Get(class group) = %+v, %v", g, err)
	}
	members, err := svc.Members(ctx, id)
	if err != nil || len(members) != 1 || members[0].ID != idLena {
		t.Errorf("Members(3A students) = %v, %v, want Lena", recipientIDs(members), err)
	}
	if _, err := svc.Update(ctx, id, GroupInput{Name: "Inna"}); !errors.Is(err, ErrSystemGroup) {
		t.Errorf("Update(class group) error = %v, want ErrSystemGroup", err)
	}
	if _, err := svc.AddMembers(ctx, id, []string{idMaria}); !errors.Is(err, ErrSystemGroup) {
		t.Errorf("AddMembers(class group) error = %v, want ErrSystemGroup", err)
	}
	for _, name := range []string{"Uczniowie klasy 4B", "rodzice  UCZNIÓW klasy 1A"} {
		if _, err := svc.Create(ctx, GroupInput{Name: name}); !errors.Is(err, ErrNameTaken) {
			t.Errorf("Create(%q) error = %v, want ErrNameTaken (reserved for class groups)", name, err)
		}
	}
	if _, err := svc.Create(ctx, GroupInput{Name: "Uczniowie klasy sportowej"}); !errors.Is(err, ErrNameTaken) {
		// A name starting with the prefix is reserved even if it is not a class.
		t.Errorf("Create(prefix) error = %v, want ErrNameTaken", err)
	}
	if _, err := svc.Create(ctx, GroupInput{Name: "Uczniowie klas trzecich"}); err != nil {
		t.Errorf("Create(similar but not reserved) error = %v", err)
	}
}

func TestHandlerListShowsClassAndAudience(t *testing.T) {
	store := NewMemStore()
	dir := newFakeDirectory(classFixture()...)
	mux := http.NewServeMux()
	NewHandler(NewService(store, dir), NewResolver(store, dir), slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)

	var list []struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		Kind      string  `json:"kind"`
		ClassName *string `json:"className"`
		Audience  *string `json:"audience"`
	}
	if code := do(t, mux, http.MethodGet, "/api/groups", "", &list); code != http.StatusOK {
		t.Fatalf("GET /groups = %d", code)
	}
	if list[0].ClassName != nil || list[0].Audience != nil {
		t.Errorf("Wszyscy rodzice has className/audience %v/%v, want null", list[0].ClassName, list[0].Audience)
	}
	g := list[2]
	if g.Name != "Uczniowie klasy 1B" || g.Kind != "system" || g.ClassName == nil || *g.ClassName != "1B" || g.Audience == nil || *g.Audience != "students" {
		t.Errorf("first class group = %+v", g)
	}
}
