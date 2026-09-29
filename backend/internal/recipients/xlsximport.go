package recipients

import (
	"archive/zip"
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// maxXLSXUnzippedSize bounds how far an XLSX file (a zip archive) may expand,
// so a small "zip bomb" upload cannot exhaust memory. Keeping the XML limit
// equal to it also stops excelize from spilling worksheets to temp files.
const maxXLSXUnzippedSize = 64 << 20

// maxXLSXScannedRows bounds how far down the worksheet readXLSX looks, blank
// rows included. excelize visits every row number up to the last one present,
// so one stray cell in row 1048576 would otherwise cost a million iterations
// for a file of a few kilobytes. It leaves room for the header, MaxImportRows
// data rows and as many blank rows between them.
const maxXLSXScannedRows = 2*MaxImportRows + 1

// readXLSX reads the first worksheet of an XLSX workbook into rows, skipping
// blank ones (see docs/adr/0006-import-xlsx-excelize.md). Cell values are
// read raw, without number formats, so a phone typed as a number stays
// "500100101" rather than a formatted "500 100 101".
func readXLSX(data []byte) ([]sourceRow, error) {
	if err := checkUnzippedSize(data); err != nil {
		return nil, err
	}
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		UnzipSizeLimit:    maxXLSXUnzippedSize,
		UnzipXMLSizeLimit: maxXLSXUnzippedSize,
	})
	// The zip itself was readable (checkUnzippedSize), so a failure here means
	// the archive is not a workbook, e.g. a .docx; excelize reports that as
	// anything from ErrWorkbookFileFormat to a bare io.EOF.
	if err != nil {
		return nil, fmt.Errorf("%w: not a valid XLSX workbook: %w", ErrUnsupportedFormat, err)
	}
	defer func() { _ = f.Close() }()

	// Some zips that are not workbooks still open, just with no worksheets.
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("%w: the file has no worksheets", ErrUnsupportedFormat)
	}
	it, err := f.Rows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("read worksheet %q: %w", sheets[0], err)
	}
	defer func() { _ = it.Close() }()

	var rows []sourceRow
	// Next yields every row number in order, including rows absent from the
	// file, so the counter is the row number shown in the spreadsheet.
	for n := 1; it.Next(); n++ {
		if n > maxXLSXScannedRows {
			return nil, fmt.Errorf("%w: the worksheet has content below row %d, delete everything that is not part of the list", ErrTooManyRows, maxXLSXScannedRows)
		}
		cells, err := it.Columns(excelize.Options{RawCellValue: true})
		switch {
		case err != nil:
			rows = append(rows, sourceRow{Row: n, Err: &FieldError{Field: "row", Message: "row could not be read: " + err.Error()}})
		case isEmptyRecord(cells):
			continue
		default:
			rows = append(rows, newSourceRow(n, cells))
		}
		if len(rows) > MaxImportRows+1 {
			return nil, ErrTooManyRows
		}
	}
	if err := it.Error(); err != nil {
		return nil, fmt.Errorf("read worksheet %q: %w", sheets[0], err)
	}
	return rows, nil
}

// checkUnzippedSize rejects an archive whose entries declare more than
// maxXLSXUnzippedSize in total. excelize enforces the same limit, but only
// with an untyped error; archive/zip refuses to inflate past a declared size,
// so the declared sizes can be trusted.
func checkUnzippedSize(data []byte) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnsupportedFormat, err)
	}
	var total uint64
	for _, zf := range zr.File {
		if zf.UncompressedSize64 > maxXLSXUnzippedSize-total {
			return fmt.Errorf("%w: it unpacks to more than %d MB", ErrFileTooLarge, maxXLSXUnzippedSize>>20)
		}
		total += zf.UncompressedSize64
	}
	return nil
}
