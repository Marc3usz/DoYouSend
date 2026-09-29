package recipients

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/xuri/excelize/v2"
)

// buildXLSX writes rows to the first worksheet of a new workbook, starting at
// row 1. A nil row is left out, a nil value leaves its cell empty. Test
// workbooks are generated rather than committed, so every fixture stays
// readable in review.
func buildXLSX(t *testing.T, rows [][]any, extraSheets ...string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	for i, row := range rows {
		if row == nil {
			continue
		}
		cell, err := excelize.CoordinatesToCellName(1, i+1)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.SetSheetRow("Sheet1", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range extraSheets {
		if _, err := f.NewSheet(name); err != nil {
			t.Fatal(err)
		}
		if err := f.SetSheetRow(name, "A1", &[]any{"first_name", "last_name", "email", "phone", "type"}); err != nil {
			t.Fatal(err)
		}
		if err := f.SetSheetRow(name, "A2", &[]any{"Ukryty", "Arkusz", "ukryty@example.test", nil, "parent"}); err != nil {
			t.Fatal(err)
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReadXLSXReport(t *testing.T) {
	header := []any{"Imię", "Nazwisko", "E-mail", "Telefon", "Typ", "Klasa"}
	cases := []struct {
		name  string
		rows  [][]any
		extra []string
		want  ImportReport
	}{
		{
			name: "polish header, phone typed as a number, empty cell mid-row",
			rows: [][]any{
				header,
				{"Zofia", "Wiśniewska", nil, 500100104, "rodzic", "3A"},
				{"Kacper", "Kowalski", "kacper.kowalski@example.test", "+48 500 100 105", "Uczeń", "3A"},
			},
			want: ImportReport{Valid: []ImportedRow{
				{Row: 2, Recipient: Recipient{FirstName: "Zofia", LastName: "Wiśniewska", Phone: "+48500100104", Type: TypeParent}},
				{Row: 3, Recipient: Recipient{FirstName: "Kacper", LastName: "Kowalski", Email: "kacper.kowalski@example.test", Phone: "+48500100105", Type: TypeStudent}},
			}},
		},
		{
			name: "row numbers match the spreadsheet across empty rows",
			rows: [][]any{
				header,
				nil,
				{"Jan", "Kowalski", "jan.kowalski@example.test", nil, "rodzic"},
				{nil, nil, nil, nil, nil},
				{"Lena", "Nowak", "lena.nowak@example.test", nil, "uczeń"},
			},
			want: ImportReport{Valid: []ImportedRow{
				{Row: 3, Recipient: Recipient{FirstName: "Jan", LastName: "Kowalski", Email: "jan.kowalski@example.test", Type: TypeParent}},
				{Row: 5, Recipient: Recipient{FirstName: "Lena", LastName: "Nowak", Email: "lena.nowak@example.test", Type: TypeStudent}},
			}},
		},
		{
			name: "invalid rows and duplicates are reported like in CSV",
			rows: [][]any{
				header,
				{"Jan", "Kowalski", "jan.kowalski@example.test", nil, "rodzic"},
				{"Jan", nil, "jan@example", nil, "rodzic"},
				{"Janek", "Kowalski", "JAN.KOWALSKI@example.test", nil, "rodzic"},
			},
			want: ImportReport{
				Valid: []ImportedRow{
					{Row: 2, Recipient: Recipient{FirstName: "Jan", LastName: "Kowalski", Email: "jan.kowalski@example.test", Type: TypeParent}},
				},
				Invalid: []InvalidRow{
					{Row: 3, Errors: []FieldError{
						{Field: "last_name", Message: "last name is required"},
						{Field: "email", Message: ErrInvalidEmail.Error()},
					}},
				},
				Duplicates: []DuplicateRow{{Row: 4, Field: "email", DuplicateOf: 2}},
			},
		},
		{
			name:  "only the first worksheet is read",
			rows:  [][]any{header, {"Jan", "Kowalski", "jan.kowalski@example.test", nil, "rodzic"}},
			extra: []string{"Drugi"},
			want: ImportReport{Valid: []ImportedRow{
				{Row: 2, Recipient: Recipient{FirstName: "Jan", LastName: "Kowalski", Email: "jan.kowalski@example.test", Type: TypeParent}},
			}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseReport(buildXLSX(t, tc.rows, tc.extra...))
			if err != nil {
				t.Fatalf("parseReport() error = %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseReport() =\n%+v\nwant\n%+v", got, tc.want)
			}
		})
	}
}

func TestReadXLSXRowLimit(t *testing.T) {
	rows := [][]any{{"first_name", "last_name", "email", "phone", "type"}}
	for range MaxImportRows + 1 {
		rows = append(rows, []any{"Jan", "Kowalski", nil, "+48500100101", "parent"})
	}
	if _, err := parseReport(buildXLSX(t, rows)); !errors.Is(err, ErrTooManyRows) {
		t.Errorf("parseReport() error = %v, want %v", err, ErrTooManyRows)
	}
}

func TestReadXLSXWideRows(t *testing.T) {
	// One stray cell far to the right used to pad every row to 16384 cells.
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := f.SetSheetRow("Sheet1", "A1", &[]any{"first_name", "last_name", "email", "phone", "type"}); err != nil {
		t.Fatal(err)
	}
	if err := f.SetSheetRow("Sheet1", "A2", &[]any{"Jan", "Kowalski", "jan.kowalski@example.test", nil, "parent"}); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "XFD3", "x"); err != nil {
		t.Fatal(err)
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}

	rows, err := parseFile(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parseFile() error = %v", err)
	}
	want := []parsedRow{
		{Row: 2, Recipient: Recipient{FirstName: "Jan", LastName: "Kowalski", Email: "jan.kowalski@example.test", Type: TypeParent}},
		{Row: 3, Errors: []FieldError{{Field: "row", Message: "row has more than 100 filled columns"}}},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("parseFile() =\n%+v\nwant\n%+v", rows, want)
	}
}

func TestReadXLSXScanLimit(t *testing.T) {
	build := func(strayRow int) []byte {
		f := excelize.NewFile()
		defer func() { _ = f.Close() }()
		if err := f.SetSheetRow("Sheet1", "A1", &[]any{"first_name", "last_name", "email", "phone", "type"}); err != nil {
			t.Fatal(err)
		}
		if err := f.SetSheetRow("Sheet1", "A2", &[]any{"Jan", "Kowalski", "jan.kowalski@example.test", nil, "parent"}); err != nil {
			t.Fatal(err)
		}
		cell, err := excelize.CoordinatesToCellName(1, strayRow)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.SetCellValue("Sheet1", cell, "x"); err != nil {
			t.Fatal(err)
		}
		buf, err := f.WriteToBuffer()
		if err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}

	report, err := parseReport(build(maxXLSXScannedRows))
	if err != nil {
		t.Fatalf("stray cell in row %d: error = %v, want nil", maxXLSXScannedRows, err)
	}
	if len(report.Valid) != 1 || len(report.Invalid) != 1 || report.Invalid[0].Row != maxXLSXScannedRows {
		t.Errorf("stray cell in row %d: report = %+v, want row 2 valid and the stray row invalid", maxXLSXScannedRows, report)
	}

	_, err = parseReport(build(maxXLSXScannedRows + 1))
	if !errors.Is(err, ErrTooManyRows) {
		t.Errorf("stray cell in row %d: error = %v, want %v", maxXLSXScannedRows+1, err, ErrTooManyRows)
	}
}
