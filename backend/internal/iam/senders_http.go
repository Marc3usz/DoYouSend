package iam

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
)

// SenderHandler serves the sender-identity endpoints of docs/api/openapi.yaml
// (ADR-0016). All of them sit under /api/users and /api/admin, so Middleware
// lets only administrators in.
type SenderHandler struct {
	senders *Senders
	logger  *slog.Logger
}

// NewSenderHandler returns a SenderHandler using senders.
func NewSenderHandler(senders *Senders, logger *slog.Logger) *SenderHandler {
	return &SenderHandler{senders: senders, logger: logger}
}

// Register adds the routes to mux.
func (h *SenderHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/senders", h.list)
	mux.HandleFunc("GET /api/users/{id}/sender", h.status)
	mux.HandleFunc("POST /api/users/{id}/sender", h.request)
	mux.HandleFunc("POST /api/users/{id}/sender/resend", h.resend)
	mux.HandleFunc("DELETE /api/users/{id}/sender", h.remove)
}

func (h *SenderHandler) list(w http.ResponseWriter, r *http.Request) {
	ids, err := h.senders.List(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	opts := h.senders.Options()
	out := senderListJSON{Provider: opts.Provider, Live: opts.Live, Items: make([]senderJSON, len(ids))}
	for i, id := range ids {
		out.Items[i] = toSenderJSON(id)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *SenderHandler) status(w http.ResponseWriter, r *http.Request) {
	id, err := h.senders.Status(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toSenderJSON(id))
}

func (h *SenderHandler) request(w http.ResponseWriter, r *http.Request) {
	actor, _ := UserFrom(r.Context())
	id, err := h.senders.Request(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toSenderJSON(id))
}

func (h *SenderHandler) resend(w http.ResponseWriter, r *http.Request) {
	actor, _ := UserFrom(r.Context())
	if err := h.senders.Resend(r.Context(), actor, r.PathValue("id")); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SenderHandler) remove(w http.ResponseWriter, r *http.Request) {
	actor, _ := UserFrom(r.Context())
	if err := h.senders.Remove(r.Context(), actor, r.PathValue("id")); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeError maps a sender-flow error to its status and stable code.
func (h *SenderHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid *ValidationError
	var rejected *SenderRejectedError
	switch {
	case errors.As(err, &invalid):
		fields := make([]httpx.FieldError, len(invalid.Fields))
		for i, f := range invalid.Fields {
			fields[i] = httpx.FieldError(f)
		}
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_input", Message: "invalid input", Fields: fields})
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, httpx.ErrorBody{Code: "not_found", Message: ErrNotFound.Error()})
	case errors.Is(err, ErrSenderNotConfigured):
		httpx.Error(w, http.StatusNotFound, httpx.ErrorBody{Code: "sender_not_configured", Message: ErrSenderNotConfigured.Error()})
	case errors.Is(err, ErrSenderExists):
		httpx.Error(w, http.StatusConflict, httpx.ErrorBody{Code: "sender_exists", Message: ErrSenderExists.Error()})
	case errors.Is(err, ErrSenderAlreadyVerified):
		httpx.Error(w, http.StatusConflict, httpx.ErrorBody{Code: "sender_already_verified", Message: ErrSenderAlreadyVerified.Error()})
	case errors.Is(err, ErrSenderGone):
		httpx.Error(w, http.StatusConflict, httpx.ErrorBody{Code: "sender_missing", Message: ErrSenderGone.Error()})
	case errors.As(err, &rejected):
		httpx.Error(w, http.StatusUnprocessableEntity, httpx.ErrorBody{Code: "sender_rejected", Message: rejected.Message})
	default:
		// Provider down, bad API key, rate limit or a database error: the
		// details go to the log, never to the response.
		h.logger.Error("sender identity request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		httpx.Error(w, http.StatusBadGateway, httpx.ErrorBody{Code: "provider_error", Message: "the e-mail provider could not handle the request"})
	}
}

// JSON shapes of docs/api/openapi.yaml (SenderIdentity, SenderIdentityList).
type (
	senderJSON struct {
		UserID     string       `json:"userId"`
		Provider   string       `json:"provider"`
		FromEmail  string       `json:"fromEmail"`
		FromName   string       `json:"fromName"`
		Status     SenderStatus `json:"status"`
		CreatedAt  time.Time    `json:"createdAt"`
		CheckedAt  *time.Time   `json:"checkedAt"`
		VerifiedAt *time.Time   `json:"verifiedAt"`
	}
	senderListJSON struct {
		Provider string       `json:"provider"`
		Live     bool         `json:"live"`
		Items    []senderJSON `json:"items"`
	}
)

func toSenderJSON(s SenderIdentity) senderJSON {
	return senderJSON{
		UserID: s.UserID, Provider: s.Provider, FromEmail: s.FromEmail, FromName: s.FromName,
		Status: s.Status, CreatedAt: s.CreatedAt, CheckedAt: optionalTime(s.CheckedAt), VerifiedAt: optionalTime(s.VerifiedAt),
	}
}

func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
