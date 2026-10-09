package delivery

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
	"github.com/Marc3usz/DoYouSend/backend/internal/messaging"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
)

// History page sizes (GET /batches limit in docs/api/openapi.yaml).
const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// Caller is the signed-in user a request runs as.
type Caller struct {
	ID       string
	FullName string
	// IsAdmin users see every batch; others only their own.
	IsAdmin bool
}

// CallerFunc returns the signed-in user of a request. main adapts iam.UserFrom to it,
// so delivery does not depend on iam.
type CallerFunc func(ctx context.Context) (Caller, bool)

// Handler serves the batch endpoints of docs/api/openapi.yaml.
type Handler struct {
	svc    *Service
	store  Store
	sender CallerFunc
	logger *slog.Logger
}

// NewHandler returns a Handler sending through svc and reading history from store.
func NewHandler(svc *Service, store Store, sender CallerFunc, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, store: store, sender: sender, logger: logger}
}

// Register adds the batch routes to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/batches", h.create)
	mux.HandleFunc("GET /api/batches", h.list)
	mux.HandleFunc("GET /api/batches/{id}", h.get)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	user, ok := h.user(w, r)
	if !ok {
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if !groups.IsValidID(key) {
		invalidInput(w, "Idempotency-Key", "a new UUID is required for every confirmation")
		return
	}
	var req batchRequestJSON
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_request", Message: "expected a JSON batch request"})
		return
	}

	batch, replayed, err := h.svc.Start(r.Context(), Draft{
		Subject:        req.Subject,
		Body:           req.Body,
		Selection:      req.Selection.toSelection(),
		CreatedBy:      user.ID,
		CreatedByName:  user.FullName,
		IdempotencyKey: key,
	})
	if err != nil {
		h.sendError(w, err)
		return
	}
	status := http.StatusAccepted
	if replayed {
		status = http.StatusOK
	}
	httpx.JSON(w, status, toDetailJSON(batch))
}

// sendError maps a refused draft to the error codes of POST /batches.
func (h *Handler) sendError(w http.ResponseWriter, err error) {
	var changed *SelectionChangedError
	var invalid *groups.ValidationError
	switch {
	case errors.Is(err, ErrEmptySubject):
		invalidInput(w, "subject", "the e-mail subject is empty")
	case errors.Is(err, messaging.ErrEmptyBody):
		invalidInput(w, "body", "the message is empty")
	case errors.Is(err, ErrUnknownPlaceholders):
		invalidInput(w, "body", strings.TrimPrefix(err.Error(), "send batch: "))
	case errors.As(err, &invalid):
		out := httpx.ErrorBody{Code: "invalid_input", Message: "invalid recipient selection"}
		for _, f := range invalid.Fields {
			out.Fields = append(out.Fields, httpx.FieldError{Field: "selection." + f.Field, Message: f.Message})
		}
		httpx.Error(w, http.StatusBadRequest, out)
	case errors.Is(err, groups.ErrEmptySelection):
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "empty_selection", Message: groups.ErrEmptySelection.Error()})
	case errors.As(err, &changed):
		ids := append(append([]string{}, changed.UnknownGroupIDs...), changed.UnknownRecipientIDs...)
		httpx.Error(w, http.StatusConflict, httpx.ErrorBody{Code: "selection_changed", Message: ErrSelectionChanged.Error(), IDs: ids})
	case errors.Is(err, ErrNoRecipients):
		httpx.Error(w, http.StatusUnprocessableEntity, httpx.ErrorBody{Code: "no_recipients", Message: ErrNoRecipients.Error()})
	case errors.Is(err, ErrInvalidKey):
		invalidInput(w, "Idempotency-Key", "a new UUID is required for every confirmation")
	case errors.Is(err, ErrShuttingDown):
		httpx.Error(w, http.StatusServiceUnavailable, httpx.ErrorBody{Code: "internal", Message: ErrShuttingDown.Error()})
	case errors.Is(err, ErrIdempotencyConflict):
		httpx.Error(w, http.StatusConflict, httpx.ErrorBody{Code: "idempotency_conflict", Message: ErrIdempotencyConflict.Error()})
	default:
		h.logger.Error("send batch", "err", err)
		internalError(w)
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	user, ok := h.user(w, r)
	if !ok {
		return
	}
	f, field, ok := parseFilter(r)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_request", Message: "invalid query parameter " + field})
		return
	}
	if !user.IsAdmin {
		f.CreatedBy = user.ID
	}
	batches, total, err := h.store.ListBatches(r.Context(), f)
	if err != nil {
		h.logger.Error("list batches", "err", err)
		internalError(w)
		return
	}
	page := batchPageJSON{Items: make([]batchSummaryJSON, 0, len(batches)), Total: total}
	for _, b := range batches {
		page.Items = append(page.Items, toSummaryJSON(b))
	}
	httpx.JSON(w, http.StatusOK, page)
}

func parseFilter(r *http.Request) (f BatchFilter, badField string, ok bool) {
	q := r.URL.Query()
	f.Limit = defaultPageSize
	if v := q.Get("status"); v != "" {
		f.Status = BatchStatus(v)
		if !knownBatchStatus[f.Status] {
			return f, "status", false
		}
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxPageSize {
			return f, "limit", false
		}
		f.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return f, "offset", false
		}
		f.Offset = n
	}
	return f, "", true
}

var knownBatchStatus = map[BatchStatus]bool{
	BatchDraft: true, BatchScheduled: true, BatchRunning: true,
	BatchDone: true, BatchDoneWithErrors: true, BatchCancelled: true,
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	user, ok := h.user(w, r)
	if !ok {
		return
	}
	b, err := h.store.Batch(r.Context(), r.PathValue("id"))
	// A sender learns nothing about other people's batches, not even that they exist.
	if errors.Is(err, ErrBatchNotFound) || err == nil && !user.IsAdmin && b.CreatedBy != user.ID {
		httpx.Error(w, http.StatusNotFound, httpx.ErrorBody{Code: "not_found", Message: ErrBatchNotFound.Error()})
		return
	}
	if err != nil {
		h.logger.Error("get batch", "err", err)
		internalError(w)
		return
	}
	httpx.JSON(w, http.StatusOK, toDetailJSON(b))
}

// user returns the signed-in sender. The iam middleware lets no anonymous request
// through to these routes, so a missing user is a wiring bug.
func (h *Handler) user(w http.ResponseWriter, r *http.Request) (Caller, bool) {
	u, ok := h.sender(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, httpx.ErrorBody{Code: "unauthenticated", Message: "not signed in"})
	}
	return u, ok
}

func invalidInput(w http.ResponseWriter, field, msg string) {
	httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{
		Code: "invalid_input", Message: "invalid batch request",
		Fields: []httpx.FieldError{{Field: field, Message: msg}},
	})
}

func internalError(w http.ResponseWriter) {
	httpx.Error(w, http.StatusInternalServerError, httpx.ErrorBody{Code: "internal", Message: "internal error"})
}

// JSON shapes from docs/api/openapi.yaml, kept apart from the domain types so the wire
// format does not change when the domain does. Lists encode as [], not null.
type (
	batchRequestJSON struct {
		Subject   string        `json:"subject"`
		Body      string        `json:"body"`
		Selection selectionJSON `json:"selection"`
	}
	selectionJSON struct {
		GroupIDs             []string `json:"groupIds"`
		RecipientIDs         []string `json:"recipientIds"`
		ExcludedRecipientIDs []string `json:"excludedRecipientIds"`
	}
	userRefJSON struct {
		ID       string `json:"id"`
		FullName string `json:"fullName"`
	}
	groupRefJSON struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	batchSummaryJSON struct {
		ID             string         `json:"id"`
		Subject        string         `json:"subject"`
		Status         BatchStatus    `json:"status"`
		CreatedBy      userRefJSON    `json:"createdBy"`
		CreatedAt      time.Time      `json:"createdAt"`
		FinishedAt     *time.Time     `json:"finishedAt"`
		Groups         []groupRefJSON `json:"groups"`
		RecipientCount int            `json:"recipientCount"`
		PartialCount   int            `json:"partialCount"`
		FailedCount    int            `json:"failedCount"`
		TotalSMSParts  int            `json:"totalSmsParts"`
		CostMilli      int64          `json:"costMilli"`
	}
	batchPageJSON struct {
		Items []batchSummaryJSON `json:"items"`
		Total int                `json:"total"`
	}
	batchDetailJSON struct {
		batchSummaryJSON
		Body         string               `json:"body"`
		RecipientIDs []string             `json:"recipientIds"`
		Recipients   []batchRecipientJSON `json:"recipients"`
	}
	batchRecipientJSON struct {
		RecipientID  string         `json:"recipientId"`
		FirstName    string         `json:"firstName"`
		LastName     string         `json:"lastName"`
		RenderedBody *string        `json:"renderedBody"`
		Partial      bool           `json:"partial"`
		Unreachable  bool           `json:"unreachable"`
		Deliveries   []deliveryJSON `json:"deliveries"`
	}
	// deliveryJSON has no address or number: history shows who and how, not contacts.
	deliveryJSON struct {
		Channel  string  `json:"channel"`
		Status   Status  `json:"status"`
		Parts    int     `json:"parts"`
		Attempts int     `json:"attempts"`
		Error    *string `json:"error"`
	}
)

func (s selectionJSON) toSelection() groups.Selection {
	return groups.Selection{
		GroupIDs:             s.GroupIDs,
		RecipientIDs:         s.RecipientIDs,
		ExcludedRecipientIDs: s.ExcludedRecipientIDs,
	}
}

func toSummaryJSON(b Batch) batchSummaryJSON {
	out := batchSummaryJSON{
		ID:             b.ID,
		Subject:        b.Subject,
		Status:         b.Status,
		CreatedBy:      userRefJSON{ID: b.CreatedBy, FullName: b.CreatedByName},
		CreatedAt:      b.CreatedAt,
		Groups:         make([]groupRefJSON, 0, len(b.Groups)),
		RecipientCount: b.Counts.Recipients,
		PartialCount:   b.Counts.Partial,
		FailedCount:    b.Counts.Failed,
		TotalSMSParts:  b.SMSParts,
		CostMilli:      b.CostMilli,
	}
	if !b.FinishedAt.IsZero() {
		finished := b.FinishedAt
		out.FinishedAt = &finished
	}
	for _, g := range b.Groups {
		out.Groups = append(out.Groups, groupRefJSON(g))
	}
	return out
}

func toDetailJSON(b Batch) batchDetailJSON {
	out := batchDetailJSON{
		batchSummaryJSON: toSummaryJSON(b),
		Body:             b.Body,
		RecipientIDs:     append([]string{}, b.RecipientIDs...),
		Recipients:       make([]batchRecipientJSON, 0, len(b.Plan.Recipients)),
	}
	for _, r := range b.Plan.Recipients {
		rj := batchRecipientJSON{
			RecipientID: r.RecipientID, FirstName: r.FirstName, LastName: r.LastName,
			Partial: r.Partial, Unreachable: r.Unreachable,
			Deliveries: make([]deliveryJSON, 0, len(r.Deliveries)),
		}
		if r.Body != "" {
			body := r.Body
			rj.RenderedBody = &body
		}
		for _, d := range r.Deliveries {
			dj := deliveryJSON{Channel: string(d.Channel), Status: d.Status, Parts: d.Parts, Attempts: d.Attempts}
			if d.Error != "" {
				msg := d.Error
				dj.Error = &msg
			}
			rj.Deliveries = append(rj.Deliveries, dj)
		}
		out.Recipients = append(out.Recipients, rj)
	}
	return out
}
