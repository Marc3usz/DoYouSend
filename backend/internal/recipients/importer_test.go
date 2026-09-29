package recipients

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakeStore is an in-memory Store that records what the Importer asked for.
type fakeStore struct {
	existing ExistingContacts
	findErr  error
	saveErr  error

	lookedUpEmails, lookedUpPhones []string
	created                        []Recipient
	saveCalls                      int
}

func (s *fakeStore) FindContacts(_ context.Context, emails, phones []string) (ExistingContacts, error) {
	s.lookedUpEmails, s.lookedUpPhones = emails, phones
	return s.existing, s.findErr
}

func (s *fakeStore) CreateRecipients(_ context.Context, rs []Recipient) error {
	s.saveCalls++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.created = append(s.created, rs...)
	return nil
}

const importerFile = "first_name,last_name,email,phone,type\n" +
	"Jan,Kowalski,Jan.Kowalski@example.test,,parent\n" + // already stored
	"Maria,Kowalska,maria.kowalska@example.test,500 100 102,parent\n" +
	"Lena,,lena.nowak@example.test,,student\n" // invalid

func TestImporterCheck(t *testing.T) {
	store := &fakeStore{existing: ExistingContacts{Emails: map[string]string{"jan.kowalski@example.test": "id-jan"}}}

	report, err := NewImporter(store).Check(context.Background(), strings.NewReader(importerFile))
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if want := []string{"jan.kowalski@example.test", "maria.kowalska@example.test"}; !reflect.DeepEqual(store.lookedUpEmails, want) {
		t.Errorf("looked up e-mails %v, want %v (normalized, valid rows only)", store.lookedUpEmails, want)
	}
	if want := []string{"+48500100102"}; !reflect.DeepEqual(store.lookedUpPhones, want) {
		t.Errorf("looked up phones %v, want %v", store.lookedUpPhones, want)
	}
	if want := []DuplicateRow{{Row: 2, Field: "email", ExistingID: "id-jan"}}; !reflect.DeepEqual(report.Duplicates, want) {
		t.Errorf("Duplicates = %+v, want %+v", report.Duplicates, want)
	}
	if len(report.Valid) != 1 || report.Valid[0].Row != 3 || len(report.Invalid) != 1 || report.Invalid[0].Row != 4 {
		t.Errorf("report = %+v, want row 3 valid and row 4 invalid", report)
	}
	if store.saveCalls != 0 {
		t.Errorf("Check() stored recipients, want a dry run")
	}
}

func TestImporterImport(t *testing.T) {
	store := &fakeStore{existing: ExistingContacts{Emails: map[string]string{"jan.kowalski@example.test": "id-jan"}}}

	report, err := NewImporter(store).Import(context.Background(), strings.NewReader(importerFile))
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	want := []Recipient{{FirstName: "Maria", LastName: "Kowalska", Email: "maria.kowalska@example.test", Phone: "+48500100102", Type: TypeParent}}
	if !reflect.DeepEqual(store.created, want) {
		t.Errorf("created %+v, want %+v", store.created, want)
	}
	if len(report.Valid) != 1 || len(report.Duplicates) != 1 || len(report.Invalid) != 1 {
		t.Errorf("report = %+v, want 1 valid, 1 duplicate, 1 invalid", report)
	}
}

func TestImporterNothingToStore(t *testing.T) {
	store := &fakeStore{}
	input := "first_name,last_name,email,phone,type\n,,,,parent\n"

	report, err := NewImporter(store).Import(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if store.saveCalls != 0 || store.lookedUpEmails != nil {
		t.Errorf("store was called (%d saves, lookup %v), want no calls for a file without valid rows", store.saveCalls, store.lookedUpEmails)
	}
	if len(report.Invalid) != 1 {
		t.Errorf("Invalid = %+v, want the one row", report.Invalid)
	}
}

func TestImporterErrors(t *testing.T) {
	errDB := errors.New("connection refused")
	cases := []struct {
		name    string
		store   *fakeStore
		input   string
		wantErr error
	}{
		{"unusable file", &fakeStore{}, "name,surname\n", ErrMissingColumns},
		{"lookup fails", &fakeStore{findErr: errDB}, importerFile, errDB},
		{"storing fails", &fakeStore{saveErr: errDB}, importerFile, errDB},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report, err := NewImporter(tc.store).Import(context.Background(), strings.NewReader(tc.input))
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Import() error = %v, want errors.Is %v", err, tc.wantErr)
			}
			if !reflect.DeepEqual(report, ImportReport{}) {
				t.Errorf("Import() report = %+v, want empty on error", report)
			}
		})
	}
}
