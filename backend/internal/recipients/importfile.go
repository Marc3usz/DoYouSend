package recipients

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"
)

// Limits for a single import file. A school has at most a few thousand
// parents and students, so a bigger file is far more likely the wrong file
// than a real roster; the limits also bound the memory one upload can use.
const (
	MaxImportFileSize = 5 << 20 // bytes, as uploaded
	MaxImportRows     = 5000    // non-empty data rows, header excluded
	MaxImportColumns  = 100     // per row, trailing empty cells not counted
)

// Errors that make a whole import file unusable, as opposed to one bad row.
// Match them with errors.Is; the message may carry extra detail.
var (
	ErrEmptyFile         = errors.New("file has no header row")
	ErrMissingColumns    = errors.New("missing required columns")
	ErrUnsupportedFormat = errors.New("unsupported file format, expected CSV or XLSX")
	ErrFileTooLarge      = fmt.Errorf("file is larger than %d MB", MaxImportFileSize>>20)
	ErrTooManyRows       = fmt.Errorf("file has more than %d data rows", MaxImportRows)
	ErrUnclosedQuote     = errors.New("a quote in the file is never closed")
	ErrInvalidHeader     = errors.New("invalid header row")
)

// ImportReport is the per-row outcome of an import file (description.md:
// "System musi wskazywac bledne, niepelne albo powtarzajace sie dane").
// Row numbers are 1-based row numbers of the file, header included, so they
// match what the administrator sees in a spreadsheet.
//
// Every non-empty data row lands in exactly one of the three lists.
type ImportReport struct {
	Valid      []ImportedRow
	Invalid    []InvalidRow
	Duplicates []DuplicateRow
}

// ImportedRow is a row that passed validation. Recipient holds trimmed values
// and a normalized (E.164) phone number, ready to be persisted.
type ImportedRow struct {
	Row       int
	Recipient Recipient
}

// InvalidRow is a row rejected by Recipient.Validate or by the file reader
// (e.g. broken CSV quoting, text that is not UTF-8). Errors lists every
// problem found, not just the first.
type InvalidRow struct {
	Row    int
	Errors []FieldError
}

// DuplicateRow is a valid row whose e-mail or phone number is already taken,
// either by an earlier valid row of the same file (DuplicateOf is that row)
// or by a stored recipient (DuplicateOf is 0, ExistingID is its ID).
// Field says which value collided; "email" is checked before "phone".
type DuplicateRow struct {
	Row         int
	Field       string
	DuplicateOf int
	ExistingID  string
}

// sourceRow is one non-empty row as read from the file, before validation.
// Err is set instead of Fields when the row itself could not be read.
type sourceRow struct {
	Row    int
	Fields []string
	Err    *FieldError
}

// newSourceRow bounds a raw record before it is kept for the whole parse.
// Trailing empty cells are dropped, since excelize pads a row up to its last
// used column: a single stray cell in column XFD would otherwise make every
// row 16384 strings long. A row still wider than MaxImportColumns is
// reported instead of kept.
func newSourceRow(row int, fields []string) sourceRow {
	end := len(fields)
	for end > 0 && strings.TrimSpace(fields[end-1]) == "" {
		end--
	}
	if end > MaxImportColumns {
		return sourceRow{Row: row, Err: &FieldError{Field: "row", Message: fmt.Sprintf("row has more than %d filled columns", MaxImportColumns)}}
	}
	// Clone, so the padded backing array is not kept alive.
	return sourceRow{Row: row, Fields: slices.Clone(fields[:end])}
}

// parsedRow is one data row after validation; Errors is nil for a valid row.
type parsedRow struct {
	Row       int
	Recipient Recipient
	Errors    []FieldError
}

var (
	zipMagic = []byte("PK\x03\x04")
	// oleMagic starts an OLE2 compound file: a legacy .xls, but also any
	// password-protected .xlsx.
	oleMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
)

// ParseFile reads an import file and reports every row, checking duplicates
// only inside the file. It needs no storage, so it can serve a preview before
// the database exists; Importer.Check also checks stored recipients.
func ParseFile(r io.Reader) (ImportReport, error) {
	rows, err := parseFile(r)
	if err != nil {
		return ImportReport{}, err
	}
	return classify(rows, ExistingContacts{}), nil
}

// parseFile reads an import file, detects its format from the content (not
// the file name) and validates every data row. A bad row never makes it
// fail; an error means the file as a whole is unusable.
func parseFile(r io.Reader) ([]parsedRow, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxImportFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read import file: %w", err)
	}
	if len(data) > MaxImportFileSize {
		return nil, ErrFileTooLarge
	}

	var rows []sourceRow
	switch {
	case bytes.HasPrefix(data, zipMagic):
		rows, err = readXLSX(data)
	case bytes.HasPrefix(data, oleMagic):
		return nil, fmt.Errorf("%w: legacy .xls or password-protected workbook, save it as .xlsx without a password", ErrUnsupportedFormat)
	default:
		rows, err = readCSV(data)
	}
	if err != nil {
		return nil, err
	}
	return parseRows(rows)
}

// parseRows maps the header and validates each data row.
func parseRows(rows []sourceRow) ([]parsedRow, error) {
	if len(rows) == 0 {
		return nil, ErrEmptyFile
	}
	header := rows[0]
	if header.Err != nil {
		return nil, fmt.Errorf("%w %d: %s", ErrInvalidHeader, header.Row, header.Err.Message)
	}
	if errs := checkEncoding(header.Fields); errs != nil {
		return nil, fmt.Errorf("%w %d: %s", ErrInvalidHeader, header.Row, errs[0].Message)
	}
	cols, err := mapColumns(header.Fields)
	if err != nil {
		return nil, err
	}

	parsed := make([]parsedRow, 0, len(rows)-1)
	for _, src := range rows[1:] {
		p := parsedRow{Row: src.Row}
		switch {
		case src.Err != nil:
			p.Errors = []FieldError{*src.Err}
		default:
			p.Errors = checkEncoding(src.Fields)
			if p.Errors == nil {
				p.Recipient = recordToRecipient(src.Fields, cols)
				p.Errors = p.Recipient.Validate()
			}
		}
		parsed = append(parsed, p)
	}
	return parsed, nil
}

// classify sorts validated rows into the report. The first valid row using an
// e-mail or phone number keeps it; a later row using it again, or a row using
// one that existing already holds, is a duplicate. Invalid rows never take a
// value, so a corrected copy further down the file is still imported.
func classify(rows []parsedRow, existing ExistingContacts) ImportReport {
	type owner struct {
		row int
		id  string
	}
	emailOwner := make(map[string]owner, len(existing.Emails)+len(rows))
	phoneOwner := make(map[string]owner, len(existing.Phones)+len(rows))
	for email, id := range existing.Emails {
		emailOwner[normalizeEmail(email)] = owner{id: id}
	}
	// Normalize what the store returns too: stored rows predating
	// NormalizePhone must still collide with the normalized file values.
	for phone, id := range existing.Phones {
		phoneOwner[NormalizePhone(phone)] = owner{id: id}
	}

	var report ImportReport
	duplicate := func(row int, field string, o owner) {
		report.Duplicates = append(report.Duplicates, DuplicateRow{Row: row, Field: field, DuplicateOf: o.row, ExistingID: o.id})
	}
	for _, p := range rows {
		if len(p.Errors) > 0 {
			report.Invalid = append(report.Invalid, InvalidRow{Row: p.Row, Errors: p.Errors})
			continue
		}
		email, phone := normalizeEmail(p.Recipient.Email), p.Recipient.Phone
		if o, ok := emailOwner[email]; ok && email != "" {
			duplicate(p.Row, "email", o)
			continue
		}
		if o, ok := phoneOwner[phone]; ok && phone != "" {
			duplicate(p.Row, "phone", o)
			continue
		}
		if email != "" {
			emailOwner[email] = owner{row: p.Row}
		}
		if phone != "" {
			phoneOwner[phone] = owner{row: p.Row}
		}
		report.Valid = append(report.Valid, ImportedRow{Row: p.Row, Recipient: p.Recipient})
	}
	return report
}

// columnAliases maps accepted header names (case-insensitive, runs of spaces
// collapsed) to the canonical column. Polish names and spelled-out variants
// are accepted because school exports use them.
var columnAliases = map[string]string{
	"first_name": "first_name", "first name": "first_name", "imie": "first_name", "imię": "first_name",
	"last_name": "last_name", "last name": "last_name", "nazwisko": "last_name",
	"email": "email", "e-mail": "email", "adres email": "email", "adres e-mail": "email",
	"phone": "phone", "telefon": "phone", "numer telefonu": "phone", "nr telefonu": "phone",
	"type": "type", "typ": "type",
}

var requiredColumns = []string{"first_name", "last_name", "email", "phone", "type"}

// typeAliases maps accepted values of the type column (case-insensitive).
var typeAliases = map[string]Type{
	"parent": TypeParent, "rodzic": TypeParent,
	"student": TypeStudent, "uczen": TypeStudent, "uczeń": TypeStudent,
}

// mapColumns returns the index of each canonical column in the header.
// Columns it does not know are ignored.
func mapColumns(header []string) (map[string]int, error) {
	cols := make(map[string]int)
	for i, name := range header {
		canonical, ok := columnAliases[strings.Join(strings.Fields(strings.ToLower(name)), " ")]
		if !ok {
			continue
		}
		if _, dup := cols[canonical]; dup {
			return nil, fmt.Errorf("%w: column %q appears more than once", ErrInvalidHeader, canonical)
		}
		cols[canonical] = i
	}

	var missing []string
	for _, c := range requiredColumns {
		if _, ok := cols[c]; !ok {
			missing = append(missing, c)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrMissingColumns, strings.Join(missing, ", "))
	}
	return cols, nil
}

// checkEncoding rejects rows that are not valid UTF-8, which usually means a
// CSV saved as Windows-1250. Validating such a row would report misleading
// errors on mangled names.
func checkEncoding(record []string) []FieldError {
	for _, f := range record {
		if !utf8.ValidString(f) {
			return []FieldError{{Field: "encoding", Message: "row is not valid UTF-8, save the file as \"CSV UTF-8\""}}
		}
	}
	return nil
}

// isEmptyRecord reports whether every field is blank, as in the ";;;;" rows
// spreadsheets leave at the end of an export. Such rows are skipped.
func isEmptyRecord(record []string) bool {
	for _, f := range record {
		if strings.TrimSpace(f) != "" {
			return false
		}
	}
	return true
}

// recordToRecipient builds a Recipient from one record. Missing trailing
// fields read as empty, so Validate reports them like any other empty value.
// An unrecognised type is left as typed and rejected by Validate.
func recordToRecipient(record []string, cols map[string]int) Recipient {
	field := func(name string) string {
		i := cols[name]
		if i >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[i])
	}

	typ := Type(field("type"))
	if t, ok := typeAliases[strings.ToLower(string(typ))]; ok {
		typ = t
	}
	phone := field("phone")
	if phone != "" {
		phone = NormalizePhone(phone)
	}
	return Recipient{
		FirstName: field("first_name"),
		LastName:  field("last_name"),
		Email:     field("email"),
		Phone:     phone,
		Type:      typ,
	}
}
