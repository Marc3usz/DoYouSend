package recipients

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseCSV(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  ImportReport
	}{
		{
			name: "valid rows with english header",
			input: "first_name,last_name,email,phone,type\n" +
				"Jan,Kowalski,jan.kowalski@example.test,+48500100101,parent\n" +
				"Lena,Nowak,,500 100 106,student\n",
			want: ImportReport{Valid: []ImportedRow{
				{Row: 2, Recipient: Recipient{FirstName: "Jan", LastName: "Kowalski", Email: "jan.kowalski@example.test", Phone: "+48500100101", Type: TypeParent}},
				{Row: 3, Recipient: Recipient{FirstName: "Lena", LastName: "Nowak", Phone: "+48500100106", Type: TypeStudent}},
			}},
		},
		{
			name: "polish header, semicolon delimiter, BOM, extra column, trimmed values",
			input: "\xEF\xBB\xBFImię;Nazwisko;E-mail;Telefon;Typ;Klasa\n" +
				" Zofia ; Wiśniewska ;;+48 500-100-104; Rodzic ;3A\n" +
				"Kacper;Kowalski;kacper.kowalski@example.test;;uczeń;3A\n",
			want: ImportReport{Valid: []ImportedRow{
				{Row: 2, Recipient: Recipient{FirstName: "Zofia", LastName: "Wiśniewska", Phone: "+48500100104", Type: TypeParent}},
				{Row: 3, Recipient: Recipient{FirstName: "Kacper", LastName: "Kowalski", Email: "kacper.kowalski@example.test", Type: TypeStudent}},
			}},
		},
		{
			name: "invalid rows are reported with every error and do not stop the import",
			input: "first_name,last_name,email,phone,type\n" +
				",Kowalski,not-an-email,+48500100101,teacher\n" +
				"Maria,Kowalska\n" +
				"Tomasz,Nowak,tomasz.nowak@example.test,,parent\n",
			want: ImportReport{
				Valid: []ImportedRow{
					{Row: 4, Recipient: Recipient{FirstName: "Tomasz", LastName: "Nowak", Email: "tomasz.nowak@example.test", Type: TypeParent}},
				},
				Invalid: []InvalidRow{
					{Row: 2, Errors: []FieldError{
						{Field: "first_name", Message: "first name is required"},
						{Field: "type", Message: "type must be 'parent' or 'student'"},
						{Field: "email", Message: ErrInvalidEmail.Error()},
					}},
					{Row: 3, Errors: []FieldError{
						{Field: "type", Message: "type must be 'parent' or 'student'"},
						{Field: "contact", Message: "recipient must have an e-mail or a phone number"},
					}},
				},
			},
		},
		{
			name: "duplicates by case-insensitive e-mail and by normalized phone keep the first row",
			input: "first_name,last_name,email,phone,type\n" +
				"Jan,Kowalski,jan.kowalski@example.test,+48500100101,parent\n" +
				"Jan,Kowalski,JAN.Kowalski@example.test,,parent\n" +
				"Janusz,Kowalski,janusz@example.test,500100101,parent\n",
			want: ImportReport{
				Valid: []ImportedRow{
					{Row: 2, Recipient: Recipient{FirstName: "Jan", LastName: "Kowalski", Email: "jan.kowalski@example.test", Phone: "+48500100101", Type: TypeParent}},
				},
				Duplicates: []DuplicateRow{
					{Row: 3, Field: "email", DuplicateOf: 2},
					{Row: 4, Field: "phone", DuplicateOf: 2},
				},
			},
		},
		{
			name: "an invalid row does not claim its e-mail for later rows",
			input: "first_name,last_name,email,phone,type\n" +
				"Jan,,jan.kowalski@example.test,,parent\n" +
				"Jan,Kowalski,jan.kowalski@example.test,,parent\n",
			want: ImportReport{
				Valid: []ImportedRow{
					{Row: 3, Recipient: Recipient{FirstName: "Jan", LastName: "Kowalski", Email: "jan.kowalski@example.test", Type: TypeParent}},
				},
				Invalid: []InvalidRow{
					{Row: 2, Errors: []FieldError{{Field: "last_name", Message: "last name is required"}}},
				},
			},
		},
		{
			name: "row numbers follow file lines across blank lines and quoted newlines",
			input: "first_name,last_name,email,phone,type\n" +
				"\n" +
				"\"Anna\nMaria\",Testowa,anna@example.test,,parent\n" +
				"Piotr,Przykladowy,piotr@example.test,,parent\n",
			want: ImportReport{Valid: []ImportedRow{
				{Row: 3, Recipient: Recipient{FirstName: "Anna\nMaria", LastName: "Testowa", Email: "anna@example.test", Type: TypeParent}},
				{Row: 5, Recipient: Recipient{FirstName: "Piotr", LastName: "Przykladowy", Email: "piotr@example.test", Type: TypeParent}},
			}},
		},
		{
			name: "non-UTF-8 row is reported instead of validated",
			input: "first_name,last_name,email,phone,type\n" +
				"Zofia,Wi\x9cniewska,zofia@example.test,,parent\n",
			want: ImportReport{Invalid: []InvalidRow{
				{Row: 2, Errors: []FieldError{{Field: "encoding", Message: "row is not valid UTF-8, save the file as \"CSV UTF-8\""}}},
			}},
		},
		{
			name: "a row with broken quoting is reported and the import continues",
			input: "first_name,last_name,email,phone,type\n" +
				"Jan \"Janek\",Kowalski,jan.kowalski@example.test,,parent\n" +
				"Maria,Kowalska,maria.kowalska@example.test,,parent\n",
			want: ImportReport{
				Valid: []ImportedRow{
					{Row: 3, Recipient: Recipient{FirstName: "Maria", LastName: "Kowalska", Email: "maria.kowalska@example.test", Type: TypeParent}},
				},
				Invalid: []InvalidRow{
					{Row: 2, Errors: []FieldError{{Field: "csv", Message: `bare " in non-quoted-field`}}},
				},
			},
		},
		{
			name: "an unclosed quote swallows the rest of the file into one invalid row",
			input: "first_name,last_name,email,phone,type\n" +
				"\"Jan,Kowalski,a@example.test,,parent\n" +
				"Maria,Kowalska,maria.kowalska@example.test,,parent\n",
			want: ImportReport{Invalid: []InvalidRow{
				{Row: 2, Errors: []FieldError{{Field: "csv", Message: `extraneous or missing " in quoted-field`}}},
			}},
		},
		{
			name:  "header only",
			input: "first_name,last_name,email,phone,type\n",
			want:  ImportReport{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCSV(strings.NewReader(tc.input))
			if err != nil {
				t.Fatalf("ParseCSV() error = %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseCSV() =\n%+v\nwant\n%+v", got, tc.want)
			}
		})
	}
}

func TestParseCSVFileErrors(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantErrIs error
		wantInMsg string
	}{
		{"empty file", "", nil, "file is empty"},
		{"missing columns", "first_name,last_name,email\n", ErrMissingColumns, "phone, type"},
		{"column given twice", "imie,first_name,last_name,email,phone,type\n", nil, "more than once"},
		{"malformed header", "first_name,\"last_name,email,phone,type\n", nil, "read csv header"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseCSV(strings.NewReader(tc.input))
			if err == nil {
				t.Fatal("ParseCSV() error = nil, want error")
			}
			if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
				t.Errorf("ParseCSV() error = %v, want errors.Is %v", err, tc.wantErrIs)
			}
			if !strings.Contains(err.Error(), tc.wantInMsg) {
				t.Errorf("ParseCSV() error = %q, want it to contain %q", err, tc.wantInMsg)
			}
		})
	}
}
