package recipients

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// ImportReport is the per-row outcome of parsing a recipients CSV file
// (description.md: "System musi wskazywac bledne, niepelne albo powtarzajace
// sie dane"). Row numbers are 1-based line numbers in the file, header
// included, so they match what the administrator sees in a spreadsheet.
//
// Every data row lands in exactly one of the three lists.
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

// InvalidRow is a row rejected by Recipient.Validate or by the parser itself
// (e.g. text that is not UTF-8). Errors lists every problem found, not just
// the first.
type InvalidRow struct {
	Row    int
	Errors []FieldError
}

// DuplicateRow is a valid row whose e-mail or phone number was already used
// by an earlier valid row of the same file. Only the first occurrence is kept
// in Valid; Field says which value collided ("email" or "phone", e-mail is
// checked first) and DuplicateOf points at the row that won.
//
// Collisions with recipients already stored in the database are not detected
// here — that needs the storage layer.
type DuplicateRow struct {
	Row         int
	Field       string
	DuplicateOf int
}

// ErrMissingColumns is returned when the header lacks a required column.
var ErrMissingColumns = errors.New("missing required columns")

// columnAliases maps accepted header names (case-insensitive) to the canonical
// column. Polish names are accepted because school exports use them.
var columnAliases = map[string]string{
	"first_name": "first_name", "imie": "first_name", "imię": "first_name",
	"last_name": "last_name", "nazwisko": "last_name",
	"email": "email", "e-mail": "email",
	"phone": "phone", "telefon": "phone",
	"type": "type", "typ": "type",
}

var requiredColumns = []string{"first_name", "last_name", "email", "phone", "type"}

// typeAliases maps accepted values of the type column (case-insensitive).
var typeAliases = map[string]Type{
	"parent": TypeParent, "rodzic": TypeParent,
	"student": TypeStudent, "uczen": TypeStudent, "uczeń": TypeStudent,
}

// ParseCSV reads a recipients CSV file and classifies every data row as
// valid, invalid or a duplicate of an earlier row. A bad row never aborts the
// import, including a row with broken CSV quoting; an error is returned only
// when the file as a whole is unusable (unreadable, bad header).
//
// Both ',' and ';' are accepted as delimiters (Excel in a Polish locale saves
// CSV with ';'), detected from the header line. A UTF-8 byte order mark is
// ignored. Columns not listed in the header aliases are ignored.
func ParseCSV(r io.Reader) (ImportReport, error) {
	br := bufio.NewReader(r)
	if bom, err := br.Peek(3); err == nil && bytes.Equal(bom, []byte("\xEF\xBB\xBF")) {
		_, _ = br.Discard(3)
	}

	cr := csv.NewReader(br)
	cr.Comma = detectDelimiter(br)
	cr.FieldsPerRecord = -1 // a short row is reported per row, not as a file error

	header, err := cr.Read()
	if errors.Is(err, io.EOF) {
		return ImportReport{}, fmt.Errorf("read csv header: file is empty")
	}
	if err != nil {
		return ImportReport{}, fmt.Errorf("read csv header: %w", err)
	}
	cols, err := mapColumns(header)
	if err != nil {
		return ImportReport{}, err
	}

	var (
		report     ImportReport
		emailOwner = make(map[string]int)
		phoneOwner = make(map[string]int)
	)
	for {
		record, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		// A syntax error (e.g. a stray quote) belongs to one row; csv.Reader
		// resumes on the next line, so only I/O errors abort the import.
		var pe *csv.ParseError
		if errors.As(err, &pe) {
			report.Invalid = append(report.Invalid, InvalidRow{
				Row:    pe.StartLine,
				Errors: []FieldError{{Field: "csv", Message: pe.Err.Error()}},
			})
			continue
		}
		if err != nil {
			return ImportReport{}, fmt.Errorf("read csv: %w", err)
		}
		row, _ := cr.FieldPos(0)

		if fieldErrs := checkEncoding(record); fieldErrs != nil {
			report.Invalid = append(report.Invalid, InvalidRow{Row: row, Errors: fieldErrs})
			continue
		}

		rec := recordToRecipient(record, cols)
		if fieldErrs := rec.Validate(); len(fieldErrs) > 0 {
			report.Invalid = append(report.Invalid, InvalidRow{Row: row, Errors: fieldErrs})
			continue
		}

		emailKey := normalizeEmail(rec.Email)
		if first, ok := emailOwner[emailKey]; ok && emailKey != "" {
			report.Duplicates = append(report.Duplicates, DuplicateRow{Row: row, Field: "email", DuplicateOf: first})
			continue
		}
		if first, ok := phoneOwner[rec.Phone]; ok && rec.Phone != "" {
			report.Duplicates = append(report.Duplicates, DuplicateRow{Row: row, Field: "phone", DuplicateOf: first})
			continue
		}
		if emailKey != "" {
			emailOwner[emailKey] = row
		}
		if rec.Phone != "" {
			phoneOwner[rec.Phone] = row
		}
		report.Valid = append(report.Valid, ImportedRow{Row: row, Recipient: rec})
	}
	return report, nil
}

// detectDelimiter picks ';' when the header line contains more semicolons
// than commas, ',' otherwise. It only peeks, so the header is still read by
// the csv.Reader afterwards.
func detectDelimiter(br *bufio.Reader) rune {
	line, _ := br.Peek(br.Size())
	if i := bytes.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	if bytes.Count(line, []byte(";")) > bytes.Count(line, []byte(",")) {
		return ';'
	}
	return ','
}

// mapColumns returns the index of each canonical column in the header.
func mapColumns(header []string) (map[string]int, error) {
	cols := make(map[string]int)
	for i, name := range header {
		canonical, ok := columnAliases[strings.ToLower(strings.TrimSpace(name))]
		if !ok {
			continue
		}
		if _, dup := cols[canonical]; dup {
			return nil, fmt.Errorf("column %q appears more than once in the header", canonical)
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

// checkEncoding rejects rows that are not valid UTF-8, which usually means the
// file was saved as Windows-1250. Validating such a row would report
// misleading errors on mangled names.
func checkEncoding(record []string) []FieldError {
	for _, f := range record {
		if !utf8.ValidString(f) {
			return []FieldError{{Field: "encoding", Message: "row is not valid UTF-8, save the file as \"CSV UTF-8\""}}
		}
	}
	return nil
}

// recordToRecipient builds a Recipient from one CSV record. Missing trailing
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
