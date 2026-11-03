package recipients

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
)

// Handler serves the recipient CRUD endpoints of docs/api/openapi.yaml.
// iam.Middleware lets only administrators through, except for GET
// /recipients/lookup and PATCH /recipients/{id}/contact, which any signed-in
// user may call (iam/policy.go).
type Handler struct {
	svc     *Service
	logger  *slog.Logger
	auditor ContactAuditor
}

// ContactAuditor records who fixed a recipient's contact (ADR-0013). It gets
// the channel names only, never the values.
type ContactAuditor interface {
	ContactFixed(ctx context.Context, recipientID string, channels []Channel)
}

// WithAuditor makes the handler report every contact fix to a.
func (h *Handler) WithAuditor(a ContactAuditor) *Handler {
	h.auditor = a
	return h
}

// NewHandler returns a Handler using svc.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Register adds the routes to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/recipients", h.list)
	mux.HandleFunc("GET /api/recipients/lookup", h.lookup)
	mux.HandleFunc("GET /api/recipients/stats", h.stats)
	mux.HandleFunc("POST /api/recipients", h.create)
	mux.HandleFunc("GET /api/recipients/{id}", h.get)
	mux.HandleFunc("PUT /api/recipients/{id}", h.update)
	mux.HandleFunc("PATCH /api/recipients/{id}/contact", h.fixContact)
	mux.HandleFunc("DELETE /api/recipients/{id}", h.delete)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lq := ListQuery{
		Filter: Filter{Query: q.Get("q"), Type: Type(q.Get("type")), Class: q.Get("class")},
		Issue:  Channel(q.Get("issue")),
	}
	var errs []FieldError
	lq.Limit, errs = intParam(q.Get("limit"), "limit", errs)
	lq.Offset, errs = intParam(q.Get("offset"), "offset", errs)
	if len(errs) > 0 {
		h.writeError(w, r, &ParamError{Fields: errs})
		return
	}

	page, err := h.svc.List(r.Context(), lq)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	out := pageJSON{Items: make([]RecipientJSON, len(page.Items)), Total: page.Total}
	for i, rec := range page.Items {
		out.Items[i] = toRecipientJSON(rec, nil)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// lookup serves the composer's recipient search to every signed-in user, so it
// leaves out the e-mail and phone (docs/api/openapi.yaml, RecipientLookup).
func (h *Handler) lookup(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lq := ListQuery{Filter: Filter{Query: q.Get("q"), Type: Type(q.Get("type")), Class: q.Get("class")}}
	var errs []FieldError
	lq.Limit, errs = intParam(q.Get("limit"), "limit", errs)
	lq.Offset, errs = intParam(q.Get("offset"), "offset", errs)
	if len(errs) > 0 {
		h.writeError(w, r, &ParamError{Fields: errs})
		return
	}

	page, err := h.svc.Lookup(r.Context(), lq)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	out := lookupPageJSON{Items: make([]lookupJSON, len(page.Items)), Total: page.Total}
	for i, rec := range page.Items {
		out.Items[i] = toLookupJSON(rec)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	d, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	groupIDs := d.GroupIDs
	if groupIDs == nil {
		groupIDs = []string{}
	}
	httpx.JSON(w, http.StatusOK, toRecipientJSON(d.Recipient, &groupIDs))
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	in, ok := h.decodeInput(w, r)
	if !ok {
		return
	}
	rec, err := h.svc.Create(r.Context(), in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/recipients/"+rec.ID)
	httpx.JSON(w, http.StatusCreated, toRecipientJSON(rec, nil))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	in, ok := h.decodeInput(w, r)
	if !ok {
		return
	}
	rec, err := h.svc.Update(r.Context(), r.PathValue("id"), in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toRecipientJSON(rec, nil))
}

// fixContact lets a sender fill in a missing contact (ADR-0013). It answers
// with the lookup shape, so it reveals no contact data, and hides who else
// uses a duplicate value.
func (h *Handler) fixContact(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email *string `json:"email"`
		Phone *string `json:"phone"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_request", Message: "expected a JSON RecipientContactFix"})
		return
	}
	rec, fixed, err := h.svc.FixContact(r.Context(), r.PathValue("id"), ContactFix{Email: body.Email, Phone: body.Phone})
	if err != nil {
		var dup *DuplicateContactError
		if errors.As(err, &dup) {
			err = &DuplicateContactError{Field: dup.Field}
		}
		h.writeError(w, r, err)
		return
	}
	if h.auditor != nil {
		h.auditor.ContactFixed(r.Context(), rec.ID, fixed)
	}
	httpx.JSON(w, http.StatusOK, toLookupJSON(rec))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) decodeInput(w http.ResponseWriter, r *http.Request) (Input, bool) {
	var body inputJSON
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_request", Message: "expected a JSON RecipientInput"})
		return Input{}, false
	}
	return Input{
		FirstName: body.FirstName,
		LastName:  body.LastName,
		Email:     deref(body.Email),
		Phone:     deref(body.Phone),
		Type:      body.Type,
		Classes:   body.Classes,
	}, true
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Stats(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, statsJSON(st))
}

// writeError maps a service error to its status and stable code. Unexpected
// errors are logged (they carry IDs, never contact data) and hidden.
func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid *ValidationError
	var badParams *ParamError
	var dup *DuplicateContactError
	switch {
	case errors.As(err, &badParams):
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{
			Code: "invalid_request", Message: "invalid request parameters", Fields: toFieldErrorsJSON(badParams.Fields),
		})
	case errors.As(err, &invalid):
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{
			Code: "invalid_input", Message: "invalid input", Fields: toFieldErrorsJSON(invalid.Fields),
		})
	case errors.As(err, &dup):
		body := httpx.ErrorBody{
			Code:    "duplicate_contact",
			Message: dup.Error(),
			Fields:  []httpx.FieldError{{Field: dup.Field, Message: ErrDuplicateContact.Error()}},
		}
		if dup.ExistingID != "" {
			body.IDs = []string{dup.ExistingID}
		}
		httpx.Error(w, http.StatusConflict, body)
	case errors.Is(err, ErrContactNotFixable):
		httpx.Error(w, http.StatusConflict, httpx.ErrorBody{Code: "contact_not_fixable", Message: ErrContactNotFixable.Error()})
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, httpx.ErrorBody{Code: "not_found", Message: ErrNotFound.Error()})
	case errors.Is(err, ErrRecipientInUse):
		httpx.Error(w, http.StatusConflict, httpx.ErrorBody{Code: "recipient_in_use", Message: ErrRecipientInUse.Error()})
	default:
		h.logger.Error("recipients request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		httpx.Error(w, http.StatusInternalServerError, httpx.ErrorBody{Code: "internal", Message: "internal error"})
	}
}

// intParam parses an optional non-negative integer query parameter; a bad
// value is added to errs. Range checks are the service's job.
func intParam(raw, field string, errs []FieldError) (int, []FieldError) {
	if raw == "" {
		return 0, errs
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, append(errs, FieldError{Field: field, Message: "not an integer"})
	}
	return n, errs
}

// JSON shapes of docs/api/openapi.yaml (Recipient, RecipientInput,
// RecipientPage), kept apart from the domain types.
type (
	// RecipientJSON is the Recipient schema. Package groups returns it too
	// (group members, resolved recipients), so both stay one shape.
	RecipientJSON struct {
		ID        string      `json:"id"`
		FirstName string      `json:"firstName"`
		LastName  string      `json:"lastName"`
		Email     *string     `json:"email"`
		Phone     *string     `json:"phone"`
		Type      Type        `json:"type"`
		Classes   []string    `json:"classes"`
		GroupIDs  *[]string   `json:"groupIds,omitempty"`
		Issues    []issueJSON `json:"issues"`
		CreatedAt time.Time   `json:"createdAt"`
		UpdatedAt time.Time   `json:"updatedAt"`
	}
	issueJSON struct {
		Channel Channel     `json:"channel"`
		Reason  IssueReason `json:"reason"`
	}
	pageJSON struct {
		Items []RecipientJSON `json:"items"`
		Total int             `json:"total"`
	}
	// lookupJSON is the RecipientLookup schema: a recipient without contact data.
	lookupJSON struct {
		ID        string      `json:"id"`
		FirstName string      `json:"firstName"`
		LastName  string      `json:"lastName"`
		Type      Type        `json:"type"`
		Classes   []string    `json:"classes"`
		Issues    []issueJSON `json:"issues"`
	}
	lookupPageJSON struct {
		Items []lookupJSON `json:"items"`
		Total int          `json:"total"`
	}
	statsJSON struct {
		Total        int `json:"total"`
		Parents      int `json:"parents"`
		Students     int `json:"students"`
		WithoutEmail int `json:"withoutEmail"`
		WithoutPhone int `json:"withoutPhone"`
		Unreachable  int `json:"unreachable"`
	}
	inputJSON struct {
		FirstName string  `json:"firstName"`
		LastName  string  `json:"lastName"`
		Email     *string `json:"email"`
		Phone     *string `json:"phone"`
		Type      Type    `json:"type"`
		// Classes is a pointer so that an omitted field keeps the classes on PUT.
		Classes *[]string `json:"classes"`
	}
)

// NewRecipientJSON returns r in the Recipient schema, without groupIds.
func NewRecipientJSON(r Recipient) RecipientJSON {
	return toRecipientJSON(r, nil)
}

func toRecipientJSON(r Recipient, groupIDs *[]string) RecipientJSON {
	issues := r.ContactIssues()
	out := RecipientJSON{
		ID:        r.ID,
		FirstName: r.FirstName,
		LastName:  r.LastName,
		Email:     nullable(r.Email),
		Phone:     nullable(r.Phone),
		Type:      r.Type,
		Classes:   nonNil(r.Classes),
		GroupIDs:  groupIDs,
		Issues:    make([]issueJSON, len(issues)),
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
	for i, is := range issues {
		out.Issues[i] = issueJSON(is)
	}
	return out
}

func toLookupJSON(r Recipient) lookupJSON {
	full := toRecipientJSON(r, nil)
	return lookupJSON{
		ID: full.ID, FirstName: full.FirstName, LastName: full.LastName,
		Type: full.Type, Classes: full.Classes, Issues: full.Issues,
	}
}

func toFieldErrorsJSON(errs []FieldError) []httpx.FieldError {
	out := make([]httpx.FieldError, len(errs))
	for i, e := range errs {
		out[i] = httpx.FieldError(e)
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
