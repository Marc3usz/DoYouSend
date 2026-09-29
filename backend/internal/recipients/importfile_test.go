package recipients

import (
	"archive/zip"
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// parseReport is parseFile + classify without a store: duplicates are only
// detected inside the file.
func parseReport(data []byte) (ImportReport, error) {
	rows, err := parseFile(bytes.NewReader(data))
	if err != nil {
		return ImportReport{}, err
	}
	return classify(rows, ExistingContacts{}), nil
}

func TestParseFileErrors(t *testing.T) {
	cases := []struct {
		name      string
		input     []byte
		wantErrIs error
		wantInMsg string
	}{
		{"empty file", nil, ErrEmptyFile, ""},
		{"only blank records", []byte(";;;;\n\n;;;;\n"), ErrEmptyFile, ""},
		{"missing columns", []byte("first_name,last_name,email\n"), ErrMissingColumns, "phone, type"},
		{"column given twice", []byte("imie,first_name,last_name,email,phone,type\n"), nil, "more than once"},
		{"malformed header", []byte("first_name,\"last_name,email,phone,type\n"), nil, "header row 1"},
		{"header not UTF-8", []byte("Imi\xea;Nazwisko;E-mail;Telefon;Typ\n"), nil, "UTF-8"},
		{"unclosed quote swallowing the rest", []byte("first_name,last_name,email,phone,type\n\"Jan,Kowalski,a@example.test,,parent\nMaria,Kowalska,m@example.test,,parent\n"), ErrUnclosedQuote, "row 2"},
		{"unclosed quote without trailing newline", []byte("first_name,last_name,email,phone,type\n\"Jan,Kowalski,a@example.test,,parent\nMaria,Kowalska,m@example.test,,parent"), ErrUnclosedQuote, "row 2"},
		{"header too wide", []byte(strings.Repeat("x,", MaxImportColumns) + "first_name,last_name,email,phone,type\n"), nil, "more than 100"},
		{"too large", bytes.Repeat([]byte("a"), MaxImportFileSize+1), ErrFileTooLarge, ""},
		{"legacy xls", []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1, 0, 0}, ErrUnsupportedFormat, ".xls"},
		{"corrupted zip", []byte("PK\x03\x04 definitely not a zip"), ErrUnsupportedFormat, ""},
		{"zip that is not a workbook", zipOf(t, "word/document.xml", "<document/>"), ErrUnsupportedFormat, ""},
		{"zip bomb", zipDeclaring(t, maxXLSXUnzippedSize+1), ErrFileTooLarge, "unpacks"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseReport(tc.input)
			if err == nil {
				t.Fatal("parseReport() error = nil, want error")
			}
			if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
				t.Errorf("parseReport() error = %v, want errors.Is %v", err, tc.wantErrIs)
			}
			if !strings.Contains(err.Error(), tc.wantInMsg) {
				t.Errorf("parseReport() error = %q, want it to contain %q", err, tc.wantInMsg)
			}
		})
	}
}

func TestClassifyAgainstExisting(t *testing.T) {
	rows := []parsedRow{
		{Row: 2, Recipient: Recipient{FirstName: "Jan", LastName: "Kowalski", Email: "Jan.Kowalski@example.test", Type: TypeParent}},
		{Row: 3, Recipient: Recipient{FirstName: "Zofia", LastName: "Wisniewska", Phone: "+48500100104", Type: TypeParent}},
		{Row: 4, Recipient: Recipient{FirstName: "Lena", LastName: "Nowak", Email: "lena.nowak@example.test", Type: TypeStudent}},
		{Row: 5, Recipient: Recipient{FirstName: "Lena", LastName: "Nowak", Email: "lena.nowak@example.test", Type: TypeStudent}},
	}
	existing := ExistingContacts{
		Emails: map[string]string{"jan.kowalski@example.test": "id-jan"},
		Phones: map[string]string{"+48 500 100 104": "id-zofia"}, // stored before normalization
	}

	got := classify(rows, existing)

	wantDups := []DuplicateRow{
		{Row: 2, Field: "email", ExistingID: "id-jan"},
		{Row: 3, Field: "phone", ExistingID: "id-zofia"},
		{Row: 5, Field: "email", DuplicateOf: 4},
	}
	if !reflect.DeepEqual(got.Duplicates, wantDups) {
		t.Errorf("Duplicates = %+v, want %+v", got.Duplicates, wantDups)
	}
	if len(got.Valid) != 1 || got.Valid[0].Row != 4 {
		t.Errorf("Valid = %+v, want only row 4", got.Valid)
	}
}

// zipOf builds a zip archive holding a single file.
func zipOf(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// zipDeclaring builds a tiny zip whose only entry claims to unpack to size
// bytes — the shape of a zip bomb, without spending the memory on one.
func zipDeclaring(t *testing.T, size uint64) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateRaw(&zip.FileHeader{Name: "xl/worksheets/sheet1.xml", Method: zip.Deflate, UncompressedSize64: size})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte{0x03, 0x00}); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
