package recipients

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
)

// readCSV reads CSV text into rows, skipping blank ones. Both ',' and ';'
// are accepted as delimiters (Excel in a Polish locale saves CSV with ';'),
// detected from the first line. A UTF-8 byte order mark is ignored.
func readCSV(data []byte) ([]sourceRow, error) {
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
	cr := csv.NewReader(bytes.NewReader(data))
	cr.Comma = detectDelimiter(data)
	cr.FieldsPerRecord = -1 // a short row is reported per row, not as a file error

	lastLine := bytes.Count(data, []byte("\n"))
	if !bytes.HasSuffix(data, []byte("\n")) {
		lastLine++
	}

	var rows []sourceRow
	for {
		record, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		// A syntax error (e.g. a stray quote) normally belongs to one row and
		// csv.Reader resumes after it. The exception is a quoted field that is
		// never closed: it swallows every line up to the end of the file, and
		// reporting one bad row would silently drop all rows after it.
		var pe *csv.ParseError
		switch {
		case errors.As(err, &pe) && errors.Is(pe.Err, csv.ErrQuote) && pe.Line > pe.StartLine && pe.Line >= lastLine:
			return nil, fmt.Errorf("%w: the quote opened in row %d swallows the rest of the file", ErrUnclosedQuote, pe.StartLine)
		case errors.As(err, &pe):
			rows = append(rows, sourceRow{Row: pe.StartLine, Err: &FieldError{Field: "row", Message: pe.Err.Error()}})
		case err != nil:
			return nil, fmt.Errorf("read csv: %w", err)
		case isEmptyRecord(record):
			continue
		default:
			line, _ := cr.FieldPos(0)
			rows = append(rows, newSourceRow(line, record))
		}
		if len(rows) > MaxImportRows+1 {
			return nil, ErrTooManyRows
		}
	}
	return rows, nil
}

// detectDelimiter picks ';' when the first line contains more semicolons
// than commas, ',' otherwise.
func detectDelimiter(data []byte) rune {
	line := data
	if i := bytes.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	if bytes.Count(line, []byte(";")) > bytes.Count(line, []byte(",")) {
		return ';'
	}
	return ','
}
