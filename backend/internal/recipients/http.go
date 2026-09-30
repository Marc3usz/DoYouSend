package recipients

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
)

// maxImportBody caps the whole multipart request: the file limit plus room for
// part headers and boundaries. ParseFile enforces the file limit itself.
const maxImportBody = MaxImportFileSize + 64<<10

// HandleCheckImport serves POST /api/recipients/import/check
// (docs/api/openapi.yaml): the per-row report of the uploaded file, without
// storing anything. It needs no authentication while it only uses ParseFile,
// because the response holds nothing but the uploaded data; once it checks
// stored recipients it must move behind the admin role.
func HandleCheckImport(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxImportBody)
		mr, err := r.MultipartReader()
		if err != nil {
			writeFileError(w, http.StatusBadRequest, "invalid_request", "expected multipart/form-data with a file field")
			return
		}

		for {
			part, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				writeFileError(w, http.StatusBadRequest, "missing_file", "the request has no file field")
				return
			}
			if err != nil {
				writeImportError(w, logger, err)
				return
			}
			if part.FormName() != "file" {
				continue // NextPart skips the unread rest of this part
			}

			report, err := ParseFile(part)
			if err != nil {
				writeImportError(w, logger, err)
				return
			}
			httpx.JSON(w, http.StatusOK, toReportJSON(report))
			return
		}
	}
}

// writeImportError maps a whole-file error to its status and stable code.
func writeImportError(w http.ResponseWriter, logger *slog.Logger, err error) {
	var tooBig *http.MaxBytesError
	switch {
	case errors.Is(err, ErrFileTooLarge), errors.As(err, &tooBig):
		writeFileError(w, http.StatusRequestEntityTooLarge, "file_too_large", ErrFileTooLarge.Error())
	case errors.Is(err, ErrEmptyFile):
		writeFileError(w, http.StatusUnprocessableEntity, "empty_file", err.Error())
	case errors.Is(err, ErrMissingColumns):
		writeFileError(w, http.StatusUnprocessableEntity, "missing_columns", err.Error())
	case errors.Is(err, ErrInvalidHeader):
		writeFileError(w, http.StatusUnprocessableEntity, "invalid_header", err.Error())
	case errors.Is(err, ErrUnsupportedFormat):
		writeFileError(w, http.StatusUnprocessableEntity, "unsupported_format", err.Error())
	case errors.Is(err, ErrTooManyRows):
		writeFileError(w, http.StatusUnprocessableEntity, "too_many_rows", err.Error())
	case errors.Is(err, ErrUnclosedQuote):
		writeFileError(w, http.StatusUnprocessableEntity, "unclosed_quote", err.Error())
	default:
		// Reading the request failed (e.g. the client went away mid-upload).
		// The error never carries file contents, so it is safe to log.
		logger.Warn("recipients import check: read upload", "err", err)
		writeFileError(w, http.StatusBadRequest, "invalid_request", "the upload could not be read")
	}
}

func writeFileError(w http.ResponseWriter, status int, code, message string) {
	httpx.JSON(w, status, fileErrorJSON{Code: code, Message: message})
}

// JSON shapes from docs/api/openapi.yaml. Kept apart from the domain types so
// the wire format does not change when the domain does.
type (
	fileErrorJSON struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	reportJSON struct {
		Valid      []importedRowJSON  `json:"valid"`
		Invalid    []invalidRowJSON   `json:"invalid"`
		Duplicates []duplicateRowJSON `json:"duplicates"`
	}
	importedRowJSON struct {
		Row       int                   `json:"row"`
		Recipient importedRecipientJSON `json:"recipient"`
	}
	importedRecipientJSON struct {
		FirstName string  `json:"firstName"`
		LastName  string  `json:"lastName"`
		Email     *string `json:"email"`
		Phone     *string `json:"phone"`
		Type      Type    `json:"type"`
	}
	invalidRowJSON struct {
		Row    int              `json:"row"`
		Errors []fieldErrorJSON `json:"errors"`
	}
	fieldErrorJSON struct {
		Field   string `json:"field"`
		Message string `json:"message"`
	}
	duplicateRowJSON struct {
		Row                 int    `json:"row"`
		Field               string `json:"field"`
		DuplicateOfRow      int    `json:"duplicateOfRow,omitempty"`
		ExistingRecipientID string `json:"existingRecipientId,omitempty"`
	}
)

// toReportJSON converts a report; empty lists encode as [], not null.
func toReportJSON(rep ImportReport) reportJSON {
	out := reportJSON{
		Valid:      make([]importedRowJSON, 0, len(rep.Valid)),
		Invalid:    make([]invalidRowJSON, 0, len(rep.Invalid)),
		Duplicates: make([]duplicateRowJSON, 0, len(rep.Duplicates)),
	}
	for _, v := range rep.Valid {
		out.Valid = append(out.Valid, importedRowJSON{Row: v.Row, Recipient: importedRecipientJSON{
			FirstName: v.Recipient.FirstName,
			LastName:  v.Recipient.LastName,
			Email:     nullable(v.Recipient.Email),
			Phone:     nullable(v.Recipient.Phone),
			Type:      v.Recipient.Type,
		}})
	}
	for _, inv := range rep.Invalid {
		errs := make([]fieldErrorJSON, 0, len(inv.Errors))
		for _, e := range inv.Errors {
			errs = append(errs, fieldErrorJSON(e))
		}
		out.Invalid = append(out.Invalid, invalidRowJSON{Row: inv.Row, Errors: errs})
	}
	for _, d := range rep.Duplicates {
		out.Duplicates = append(out.Duplicates, duplicateRowJSON{
			Row: d.Row, Field: d.Field, DuplicateOfRow: d.DuplicateOf, ExistingRecipientID: d.ExistingID,
		})
	}
	return out
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
