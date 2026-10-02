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
