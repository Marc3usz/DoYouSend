//go:build integration

package recipients

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database/dbtest"
)

// Fictional recipients only (CLAUDE.md rule 2).
func pgFixture() []Recipient {
	return []Recipient{
		{FirstName: "Jan", LastName: "Kowalski", Email: "Jan.Kowalski@example.test", Phone: "+48500100101", Type: TypeParent},
		{FirstName: "Maria", LastName: "Kowalska", Email: "maria.kowalska@example.test", Type: TypeParent},
		{FirstName: "Lena", LastName: "Nowak", Phone: "+48500100103", Type: TypeStudent},
	}
}

func newPGStore(t *testing.T) *PGStore {
	t.Helper()
	s := NewPGStore(dbtest.New(t))
	if err := s.CreateRecipients(context.Background(), pgFixture()); err != nil {
		t.Fatalf("CreateRecipients(fixture) error = %v", err)
	}
	return s
}

func names(rs []Recipient) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.FirstName + " " + r.LastName
	}
	return out
}

func TestPGStoreRecipientsByType(t *testing.T) {
	s := newPGStore(t)
	ctx := context.Background()

	parents, err := s.RecipientsByType(ctx, TypeParent)
	if err != nil {
		t.Fatalf("RecipientsByType() error = %v", err)
	}
	if want := []string{"Maria Kowalska", "Jan Kowalski"}; !reflect.DeepEqual(names(parents), want) {
		t.Errorf("parents = %v, want %v (by last name)", names(parents), want)
	}
	maria := parents[0]
	if maria.Phone != "" || maria.Email != "maria.kowalska@example.test" || maria.ID == "" || maria.CreatedAt.IsZero() {
		t.Errorf("Maria = %+v, want NULL phone read as \"\", ID and CreatedAt set", maria)
	}
}

func TestPGStoreRecipientsByIDs(t *testing.T) {
	s := newPGStore(t)
	ctx := context.Background()
	students, err := s.RecipientsByType(ctx, TypeStudent)
	if err != nil || len(students) != 1 {
		t.Fatalf("RecipientsByType(student) = %v, %v; want one", students, err)
	}
	lena := students[0]

	got, err := s.RecipientsByIDs(ctx, []string{
		strings.ToUpper(lena.ID),
		"11111111-1111-4111-8111-0000000000ff", // unknown
		"not-a-uuid",
	})
	if err != nil {
		t.Fatalf("RecipientsByIDs() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != lena.ID || got[0].Email != "" {
		t.Errorf("RecipientsByIDs() = %+v, want only Lena (unknown and malformed IDs left out)", got)
	}

	if got, err := s.RecipientsByIDs(ctx, nil); err != nil || len(got) != 0 {
		t.Errorf("RecipientsByIDs(nil) = %v, %v; want nothing", got, err)
	}
}

func TestPGStoreFindContacts(t *testing.T) {
	s := newPGStore(t)
	ctx := context.Background()

	got, err := s.FindContacts(ctx,
		[]string{"jan.kowalski@example.test", "nikt@example.test"},
		[]string{"+48500100103", "+48500100199"})
	if err != nil {
		t.Fatalf("FindContacts() error = %v", err)
	}
	if len(got.Emails) != 1 || got.Emails["jan.kowalski@example.test"] == "" {
		t.Errorf("Emails = %v, want Jan's stored mixed-case address matched", got.Emails)
	}
	// Jan also has a phone number, but it was not asked for.
	if len(got.Phones) != 1 || got.Phones["+48500100103"] == "" {
		t.Errorf("Phones = %v, want only Lena's number", got.Phones)
	}

	if got, err := s.FindContacts(ctx, nil, []string{"+48500100101"}); err != nil || len(got.Phones) != 1 || len(got.Emails) != 0 {
		t.Errorf("FindContacts(no e-mails) = %+v, %v; want Jan's phone only", got, err)
	}
}

func TestPGStoreCreateRecipientsAllOrNothing(t *testing.T) {
	tests := []struct {
		name      string
		recipient Recipient
		field     string
	}{
		{
			name:      "e-mail taken, other letter case",
			recipient: Recipient{FirstName: "Inny", LastName: "Jan", Email: "JAN.KOWALSKI@example.test", Type: TypeParent},
			field:     "email",
		},
		{
			name:      "phone taken",
			recipient: Recipient{FirstName: "Inna", LastName: "Lena", Phone: "+48500100103", Type: TypeParent},
			field:     "phone",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newPGStore(t)
			ctx := context.Background()

			fresh := Recipient{FirstName: "Oskar", LastName: "Adamski", Email: "oskar.adamski@example.test", Type: TypeStudent}
			err := s.CreateRecipients(ctx, []Recipient{fresh, tt.recipient})

			var dup *DuplicateContactError
			if !errors.As(err, &dup) || dup.Field != tt.field || !errors.Is(err, ErrDuplicateContact) {
				t.Fatalf("CreateRecipients() error = %v, want *DuplicateContactError on %s", err, tt.field)
			}
			students, err := s.RecipientsByType(ctx, TypeStudent)
			if err != nil {
				t.Fatalf("RecipientsByType() error = %v", err)
			}
			if want := []string{"Lena Nowak"}; !reflect.DeepEqual(names(students), want) {
				t.Errorf("students = %v, want %v (Oskar rolled back)", names(students), want)
			}
		})
	}
}

func TestImporterWithPGStore(t *testing.T) {
	s := NewPGStore(dbtest.New(t))
	im := NewImporter(s)
	ctx := context.Background()
	const file = "first_name,last_name,email,phone,type\n" +
		"Jan,Kowalski,jan.kowalski@example.test,500 100 101,parent\n" +
		"Lena,Nowak,,500 100 103,student\n"

	report, err := im.Import(ctx, strings.NewReader(file))
	if err != nil || len(report.Valid) != 2 {
		t.Fatalf("Import() = %+v, %v; want 2 rows stored", report, err)
	}

	// The same file again: every row now collides with a stored recipient.
	report, err = im.Check(ctx, strings.NewReader(file))
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(report.Valid) != 0 || len(report.Duplicates) != 2 {
		t.Fatalf("Check() = %+v, want both rows reported as duplicates", report)
	}
	for _, d := range report.Duplicates {
		if d.ExistingID == "" {
			t.Errorf("duplicate %+v has no ExistingID", d)
		}
	}
}

func TestPGStoreListRecipients(t *testing.T) {
	s := newPGStore(t)
	ctx := context.Background()

	tests := []struct {
		name   string
		filter Filter
		want   []string
	}{
		{"all", Filter{}, []string{"Maria Kowalska", "Jan Kowalski", "Lena Nowak"}},
		{"type", Filter{Type: TypeStudent}, []string{"Lena Nowak"}},
		{"last name fragment, any case", Filter{Query: "KOWAL"}, []string{"Maria Kowalska", "Jan Kowalski"}},
		{"first and last name", Filter{Query: "jan kowalski"}, []string{"Jan Kowalski"}},
		{"last and first name", Filter{Query: "Nowak Lena"}, []string{"Lena Nowak"}},
		{"e-mail", Filter{Query: "maria.kowalska@"}, []string{"Maria Kowalska"}},
		{"phone with spaces", Filter{Query: "500 100 103"}, []string{"Lena Nowak"}},
		{"LIKE wildcards are literal", Filter{Query: "%"}, nil},
		{"only dashes does not match every phone", Filter{Query: " - - "}, nil},
		{"query and type", Filter{Query: "kowal", Type: TypeStudent}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.ListRecipients(ctx, tt.filter)
			if err != nil {
				t.Fatalf("ListRecipients() error = %v", err)
			}
			if g := names(got); !reflect.DeepEqual(g, append([]string{}, tt.want...)) {
				t.Errorf("ListRecipients() = %v, want %v", g, tt.want)
			}
		})
	}
}

func TestPGStoreRecipientCRUD(t *testing.T) {
	s := newPGStore(t)
	ctx := context.Background()

	created, err := s.CreateRecipient(ctx, Recipient{FirstName: "Oskar", LastName: "Adamski", Phone: "+48500100120", Type: TypeStudent})
	if err != nil || created.ID == "" || created.Email != "" || created.CreatedAt.IsZero() {
		t.Fatalf("CreateRecipient() = %+v, %v", created, err)
	}
	if _, err := s.CreateRecipient(ctx, Recipient{FirstName: "X", LastName: "Y", Phone: "+48500100120", Type: TypeParent}); !errors.Is(err, ErrDuplicateContact) {
		t.Errorf("CreateRecipient(taken phone) error = %v, want ErrDuplicateContact", err)
	}

	created.Email, created.Phone, created.LastName = "oskar.adamski@example.test", "", "Adamski-Nowak"
	updated, err := s.UpdateRecipient(ctx, created)
	if err != nil {
		t.Fatalf("UpdateRecipient() error = %v", err)
	}
	if updated.Phone != "" || updated.Email != "oskar.adamski@example.test" || !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Errorf("UpdateRecipient() = %+v, want phone cleared, updated_at bumped", updated)
	}
	updated.Email = "JAN.KOWALSKI@example.test"
	if _, err := s.UpdateRecipient(ctx, updated); !errors.Is(err, ErrDuplicateContact) {
		t.Errorf("UpdateRecipient(taken e-mail) error = %v, want ErrDuplicateContact", err)
	}
	missing := "11111111-1111-4111-8111-0000000000ff"
	if _, err := s.UpdateRecipient(ctx, Recipient{ID: missing, FirstName: "A", LastName: "B", Phone: "+48500100199", Type: TypeParent}); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateRecipient(unknown) error = %v, want ErrNotFound", err)
	}

	if got, err := s.GetRecipient(ctx, strings.ToUpper(created.ID)); err != nil || got.LastName != "Adamski-Nowak" {
		t.Errorf("GetRecipient() = %+v, %v", got, err)
	}
	if err := s.DeleteRecipient(ctx, created.ID); err != nil {
		t.Errorf("DeleteRecipient() error = %v", err)
	}
	for name, err := range map[string]error{
		"GetRecipient":    func() error { _, err := s.GetRecipient(ctx, created.ID); return err }(),
		"DeleteRecipient": s.DeleteRecipient(ctx, created.ID),
		"GetRecipient(malformed)": func() error {
			_, err := s.GetRecipient(ctx, "nope")
			return err
		}(),
	} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s error = %v, want ErrNotFound", name, err)
		}
	}
}

func TestPGStoreDeleteRecipientInBatchHistory(t *testing.T) {
	pool := dbtest.New(t)
	s := NewPGStore(pool)
	ctx := context.Background()
	jan, err := s.CreateRecipient(ctx, pgFixture()[0])
	if err != nil {
		t.Fatalf("CreateRecipient() error = %v", err)
	}
	_, err = pool.Exec(ctx, `
		WITH u AS (INSERT INTO users (email, full_name, role, password_hash)
		           VALUES ('dyrektor@example.test', 'Anna Testowa', 'sender', 'x') RETURNING id),
		     b AS (INSERT INTO message_batches (subject, body, created_by) SELECT 'Zebranie', 'Tresc', id FROM u RETURNING id)
		INSERT INTO batch_recipients (batch_id, recipient_id, rendered_body) SELECT id, $1, 'Tresc' FROM b`, jan.ID)
	if err != nil {
		t.Fatalf("insert batch: %v", err)
	}

	if err := s.DeleteRecipient(ctx, jan.ID); !errors.Is(err, ErrRecipientInUse) {
		t.Errorf("DeleteRecipient(in history) error = %v, want ErrRecipientInUse", err)
	}
	if _, err := s.GetRecipient(ctx, jan.ID); err != nil {
		t.Errorf("recipient gone after refused delete: %v", err)
	}
}

func TestPGStoreRecipientGroupIDs(t *testing.T) {
	pool := dbtest.New(t)
	s := NewPGStore(pool)
	ctx := context.Background()
	jan, err := s.CreateRecipient(ctx, pgFixture()[0])
	if err != nil {
		t.Fatalf("CreateRecipient() error = %v", err)
	}
	var zebra, alfa string
	err = pool.QueryRow(ctx, `
		WITH z AS (INSERT INTO groups (name) VALUES ('zebra') RETURNING id),
		     a AS (INSERT INTO groups (name) VALUES ('Alfa') RETURNING id)
		SELECT (SELECT id::text FROM z), (SELECT id::text FROM a)`).Scan(&zebra, &alfa)
	if err != nil {
		t.Fatalf("insert groups: %v", err)
	}
	// A stray membership of a built-in group (as old seeds wrote) is ignored.
	_, err = pool.Exec(ctx, `INSERT INTO group_members (group_id, recipient_id)
		VALUES ($1, $3), ($2, $3), ('00000000-0000-4000-a000-000000000001', $3)`, zebra, alfa, jan.ID)
	if err != nil {
		t.Fatalf("insert members: %v", err)
	}

	got, err := s.RecipientGroupIDs(ctx, jan.ID)
	if err != nil || !reflect.DeepEqual(got, []string{alfa, zebra}) {
		t.Errorf("RecipientGroupIDs() = %v, %v; want [Alfa zebra] by name, custom only", got, err)
	}
}
