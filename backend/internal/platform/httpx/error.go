package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ErrorBody is the ErrorBody schema of docs/api/openapi.yaml: a stable code the
// UI maps to a Polish text, an English diagnostic message, and optional
// details. Message must never carry an e-mail address, phone number or
// message body.
type ErrorBody struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Fields  []FieldError `json:"fields,omitempty"`
	IDs     []string     `json:"ids,omitempty"`
}

// FieldError points at one invalid input field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error writes body with the given status.
func Error(w http.ResponseWriter, status int, body ErrorBody) {
	JSON(w, status, body)
}

// MaxJSONBody caps a JSON request body. Requests of this API are small forms
// and ID lists; the largest is POST /groups/resolve with three lists of up to
// 5000 UUIDs each (about 600 KB).
const MaxJSONBody = 1 << 20

// ErrInvalidJSON is returned by DecodeJSON for a body that is not one JSON
// value of the expected shape, or is larger than MaxJSONBody.
var ErrInvalidJSON = errors.New("invalid JSON body")

// DecodeJSON reads one JSON value from the request body into v. Unknown
// fields are ignored, so an older client keeps working when fields are added.
func DecodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxJSONBody))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidJSON, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: unexpected data after the JSON value", ErrInvalidJSON)
	}
	return nil
}
