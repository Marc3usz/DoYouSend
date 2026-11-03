package recipients

import (
	"context"
	"errors"
	"testing"
)

func TestImporterCheckTable(t *testing.T) {
	header := []string{"Imię", "Nazwisko", "E-mail", "Telefon", "Typ"}
	tests := []struct {
		name        string
		table       [][]string
		wantErr     error
		wantValid   []int
		wantInvalid []int
		wantDup     []int
	}{
		{
			name: "row numbers count the header as row 1, blank rows skipped",
			table: [][]string{
				header,
				{"Anna", "Testowa", "anna.testowa@example.test", "+48 500 100 101", "rodzic"},
				{"", "", "", "", ""},
				{"Piotr", "", "piotr.testowy@example.test", "", "uczeń"},
				{"Ola", "Testowa", "ANNA.testowa@example.test", "", "rodzic"},
			},
			wantValid:   []int{2},
			wantInvalid: []int{4},
			wantDup:     []int{5},
		},
		{
			name:    "empty table",
			table:   nil,
			wantErr: ErrEmptyFile,
		},
		{
			name:    "missing required columns",
			table:   [][]string{{"Imię", "Nazwisko"}, {"Anna", "Testowa"}},
			wantErr: ErrMissingColumns,
		},
		{
			name:    "too many rows",
			table:   tooManyRows(header),
			wantErr: ErrTooManyRows,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := NewImporter(&fakeStore{}).CheckTable(context.Background(), tt.table)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("CheckTable() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("CheckTable() error = %v", err)
			}
			if got := validRows(report); !equalInts(got, tt.wantValid) {
				t.Errorf("valid rows = %v, want %v", got, tt.wantValid)
			}
			var invalid, dup []int
			for _, r := range report.Invalid {
				invalid = append(invalid, r.Row)
			}
			for _, r := range report.Duplicates {
				dup = append(dup, r.Row)
			}
			if !equalInts(invalid, tt.wantInvalid) {
				t.Errorf("invalid rows = %v, want %v", invalid, tt.wantInvalid)
			}
			if !equalInts(dup, tt.wantDup) {
				t.Errorf("duplicate rows = %v, want %v", dup, tt.wantDup)
			}
		})
	}
}

func TestImporterImportTable(t *testing.T) {
	store := &fakeStore{}
	table := [][]string{
		{"first_name", "last_name", "email", "phone", "type"},
		{"Anna", "Testowa", "anna.testowa@example.test", "", "parent"},
		{"Bez", "", "", "", "parent"},
	}
	report, err := NewImporter(store).ImportTable(context.Background(), table)
	if err != nil {
		t.Fatalf("ImportTable() error = %v", err)
	}
	if len(store.created) != 1 || store.created[0].Email != "anna.testowa@example.test" {
		t.Errorf("created = %+v, want only the valid row", store.created)
	}
	if len(report.Invalid) != 1 || report.Invalid[0].Row != 3 {
		t.Errorf("Invalid = %+v, want row 3", report.Invalid)
	}
}

func tooManyRows(header []string) [][]string {
	table := [][]string{header}
	for range MaxImportRows + 1 {
		table = append(table, []string{"Anna", "Testowa", "", "", "rodzic"})
	}
	return table
}

func validRows(r ImportReport) []int {
	var out []int
	for _, v := range r.Valid {
		out = append(out, v.Row)
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
