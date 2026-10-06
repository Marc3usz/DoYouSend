package recipients

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// memRecipients is an in-memory RecipientStore for service and HTTP tests.
type memRecipients struct {
	rs        []Recipient // sorted by last name, as the Postgres store returns them
	groups    map[string][]string
	inUse     map[string]bool
	nextID    int
	listCalls int
	err       error
}

func newMemRecipients(rs ...Recipient) *memRecipients {
	m := &memRecipients{groups: map[string][]string{}, inUse: map[string]bool{}}
	for _, r := range rs {
		if _, err := m.CreateRecipient(context.Background(), r); err != nil {
			panic(err)
		}
	}
	return m
}

func (m *memRecipients) ListRecipients(_ context.Context, f Filter) ([]Recipient, error) {
	m.listCalls++
	if m.err != nil {
		return nil, m.err
	}
	var out []Recipient
	for _, r := range m.rs {
		if f.Type != "" && r.Type != f.Type {
			continue
		}
		if q := strings.ToLower(f.Query); q != "" && !strings.Contains(strings.ToLower(r.FirstName+" "+r.LastName+" "+r.Email+" "+r.Phone), q) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (m *memRecipients) GetRecipient(_ context.Context, id string) (Recipient, error) {
	if m.err != nil {
		return Recipient{}, m.err
	}
	for _, r := range m.rs {
		if r.ID == id {
			return r, nil
		}
	}
	return Recipient{}, ErrNotFound
}

func (m *memRecipients) RecipientGroupIDs(_ context.Context, id string) ([]string, error) {
	return m.groups[id], nil
}

func (m *memRecipients) FindContacts(_ context.Context, emails, phones []string) (ExistingContacts, error) {
	out := ExistingContacts{Emails: map[string]string{}, Phones: map[string]string{}}
	for _, r := range m.rs {
		if e := normalizeEmail(r.Email); e != "" && slices.Contains(emails, e) {
			out.Emails[e] = r.ID
		}
		if r.Phone != "" && slices.Contains(phones, r.Phone) {
			out.Phones[r.Phone] = r.ID
		}
	}
	return out, nil
}

func (m *memRecipients) CreateRecipient(_ context.Context, r Recipient) (Recipient, error) {
	if m.err != nil {
		return Recipient{}, m.err
	}
	m.nextID++
	r.ID = fmt.Sprintf("00000000-0000-4000-8000-%012d", m.nextID)
	m.rs = append(m.rs, r)
	m.sort()
	return r, nil
}

func (m *memRecipients) UpdateRecipient(_ context.Context, r Recipient) (Recipient, error) {
	for i := range m.rs {
		if m.rs[i].ID == r.ID {
			m.rs[i] = r
			m.sort()
			return r, nil
		}
	}
	return Recipient{}, ErrNotFound
}

func (m *memRecipients) DeleteRecipient(_ context.Context, id string) error {
	if m.inUse[id] {
		return ErrRecipientInUse
	}
	for i := range m.rs {
		if m.rs[i].ID == id {
			m.rs = slices.Delete(m.rs, i, i+1)
			return nil
		}
	}
	return ErrNotFound
}

func (m *memRecipients) sort() {
	slices.SortStableFunc(m.rs, func(a, b Recipient) int {
		return strings.Compare(strings.ToLower(a.LastName+" "+a.FirstName), strings.ToLower(b.LastName+" "+b.FirstName))
	})
}

func (m *memRecipients) id(firstName string) string {
	for _, r := range m.rs {
		if r.FirstName == firstName {
			return r.ID
		}
	}
	panic("no fixture recipient " + firstName)
}

// Fictional recipients only (CLAUDE.md rule 2).
func serviceFixture() *memRecipients {
	return newMemRecipients(
		Recipient{FirstName: "Jan", LastName: "Kowalski", Email: "jan.kowalski@example.test", Phone: "+48500100101", Type: TypeParent},
		Recipient{FirstName: "Maria", LastName: "Kowalska", Email: "maria.kowalska@example.test", Type: TypeParent},
		Recipient{FirstName: "Lena", LastName: "Nowak", Phone: "+48500100103", Type: TypeStudent},
		Recipient{FirstName: "Pawel", LastName: "Lewandowski", Email: "pawel.lewandowski@example.test", Phone: "500-100", Type: TypeParent},
	)
}

func firstNames(rs []Recipient) []string {
	out := []string{}
	for _, r := range rs {
		out = append(out, r.FirstName)
	}
	return out
}

func TestServiceList(t *testing.T) {
	tests := []struct {
		name  string
		query ListQuery
		want  []string
		total int
	}{
		{"all, sorted", ListQuery{}, []string{"Maria", "Jan", "Pawel", "Lena"}, 4},
		{"by type", ListQuery{Filter: Filter{Type: TypeStudent}}, []string{"Lena"}, 1},
		{"by query, trimmed", ListQuery{Filter: Filter{Query: "  kowal "}}, []string{"Maria", "Jan"}, 2},
		{"missing or invalid e-mail", ListQuery{Issue: ChannelEmail}, []string{"Lena"}, 1},
		{"missing or invalid phone", ListQuery{Issue: ChannelSMS}, []string{"Maria", "Pawel"}, 2},
		{"page", ListQuery{Limit: 2, Offset: 1}, []string{"Jan", "Pawel"}, 4},
		{"offset past the end", ListQuery{Offset: 10}, []string{}, 4},
		{"issue with type", ListQuery{Filter: Filter{Type: TypeParent}, Issue: ChannelSMS, Limit: 1}, []string{"Maria"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := NewService(serviceFixture()).List(context.Background(), tt.query)
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if got := firstNames(page.Items); !reflect.DeepEqual(got, tt.want) || page.Total != tt.total {
				t.Errorf("List() = %v (total %d), want %v (total %d)", got, page.Total, tt.want, tt.total)
			}
		})
	}
}

func TestServiceListRejectsBadQuery(t *testing.T) {
	tests := []struct {
		name  string
		query ListQuery
		field string
	}{
		{"type", ListQuery{Filter: Filter{Type: "teacher"}}, "type"},
		{"issue", ListQuery{Issue: "fax"}, "issue"},
		{"limit too big", ListQuery{Limit: MaxPageSize + 1}, "limit"},
		{"negative limit", ListQuery{Limit: -1}, "limit"},
		{"negative offset", ListQuery{Offset: -1}, "offset"},
		{"long query", ListQuery{Filter: Filter{Query: strings.Repeat("ą", MaxQueryLength+1)}}, "q"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := serviceFixture()
			_, err := NewService(store).List(context.Background(), tt.query)
			var invalid *ParamError
			if !errors.As(err, &invalid) || invalid.Fields[0].Field != tt.field || !errors.Is(err, ErrInvalidParams) {
				t.Fatalf("List() error = %v, want ParamError on %s", err, tt.field)
			}
			if store.listCalls != 0 {
				t.Errorf("store queried %d times for a rejected query", store.listCalls)
			}
		})
	}
}

func TestServiceCreate(t *testing.T) {
	store := serviceFixture()
	svc := NewService(store)

	got, err := svc.Create(context.Background(), Input{
		FirstName: "  Oskar ", LastName: "Adamski", Email: " oskar.adamski@example.test ", Phone: "500 100 120", Type: TypeStudent,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	want := Recipient{ID: got.ID, FirstName: "Oskar", LastName: "Adamski", Email: "oskar.adamski@example.test", Phone: "+48500100120", Type: TypeStudent}
	if !reflect.DeepEqual(got, want) || got.ID == "" {
		t.Errorf("Create() = %+v, want %+v (trimmed, phone in E.164)", got, want)
	}
}

func TestServiceCreateRejects(t *testing.T) {
	store := serviceFixture()
	tests := []struct {
		name     string
		in       Input
		dupField string // set for a duplicate, otherwise a validation error is expected
		dupOf    string
		fields   []string
	}{
		{"nothing filled", Input{}, "", "", []string{"first_name", "last_name", "type", "contact"}},
		{"bad e-mail and phone", Input{FirstName: "A", LastName: "B", Email: "a@b", Phone: "12", Type: TypeParent}, "", "", []string{"email", "phone"}},
		{"e-mail taken, other case", Input{FirstName: "A", LastName: "B", Email: "JAN.Kowalski@example.test", Type: TypeParent}, "email", "Jan", nil},
		{"phone taken, loose format", Input{FirstName: "A", LastName: "B", Phone: "+48 500 100 103", Type: TypeParent}, "phone", "Lena", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := len(store.rs)
			_, err := NewService(store).Create(context.Background(), tt.in)
			if tt.dupField != "" {
				var dup *DuplicateContactError
				if !errors.As(err, &dup) || dup.Field != tt.dupField || dup.ExistingID != store.id(tt.dupOf) {
					t.Fatalf("Create() error = %#v, want duplicate %s of %s", err, tt.dupField, tt.dupOf)
				}
			} else {
				var invalid *ValidationError
				if !errors.As(err, &invalid) {
					t.Fatalf("Create() error = %v, want *ValidationError", err)
				}
				var got []string
				for _, f := range invalid.Fields {
					got = append(got, f.Field)
				}
				if !reflect.DeepEqual(got, tt.fields) {
					t.Errorf("fields = %v, want %v", got, tt.fields)
				}
			}
			if len(store.rs) != before {
				t.Error("a rejected recipient was stored")
			}
		})
	}
}

func TestServiceUpdate(t *testing.T) {
	store := serviceFixture()
	svc := NewService(store)
	ctx := context.Background()
	jan := store.id("Jan")

	// Keeping one's own contacts is not a duplicate.
	got, err := svc.Update(ctx, strings.ToUpper(jan), Input{FirstName: "Jan", LastName: "Kowalski-Nowak", Email: "jan.kowalski@example.test", Phone: "500100101", Type: TypeParent})
	if err != nil {
		t.Fatalf("Update(own contacts) error = %v", err)
	}
	if got.ID != jan || got.LastName != "Kowalski-Nowak" || got.Phone != "+48500100101" {
		t.Errorf("Update() = %+v", got)
	}

	_, err = svc.Update(ctx, jan, Input{FirstName: "Jan", LastName: "K", Email: "maria.kowalska@example.test", Type: TypeParent})
	if !errors.Is(err, ErrDuplicateContact) {
		t.Errorf("Update(Maria's e-mail) error = %v, want ErrDuplicateContact", err)
	}

	missing := "00000000-0000-4000-8000-0000000000ff"
	if _, err := svc.Update(ctx, missing, Input{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update(unknown) error = %v, want ErrNotFound before validation", err)
	}
	if _, err := svc.Update(ctx, "nope", Input{}); !errors.Is(err, ErrInvalidParams) {
		t.Errorf("Update(bad id) error = %v, want ErrInvalidParams", err)
	}
}

func TestServiceGetAndDelete(t *testing.T) {
	store := serviceFixture()
	svc := NewService(store)
	ctx := context.Background()
	jan, lena := store.id("Jan"), store.id("Lena")
	store.groups[jan] = []string{"22222222-2222-4222-8222-000000000001"}
	store.inUse[lena] = true

	d, err := svc.Get(ctx, jan)
	if err != nil || d.FirstName != "Jan" || !reflect.DeepEqual(d.GroupIDs, store.groups[jan]) {
		t.Errorf("Get() = %+v, %v", d, err)
	}
	if err := svc.Delete(ctx, lena); !errors.Is(err, ErrRecipientInUse) {
		t.Errorf("Delete(in use) error = %v, want ErrRecipientInUse", err)
	}
	if err := svc.Delete(ctx, jan); err != nil {
		t.Errorf("Delete() error = %v", err)
	}
	if _, err := svc.Get(ctx, jan); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(deleted) error = %v, want ErrNotFound", err)
	}
}
