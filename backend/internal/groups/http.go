package groups

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

// Handler serves the group endpoints of docs/api/openapi.yaml, including
// POST /groups/resolve (shape per ADR-0007).
//
// TODO(iam): group management needs the admin role and resolve the sender
// role once the iam middleware lands (DEV D). Until then the API is meant for
// local, fictional data only (CLAUDE.md rules 2 and 3).
type Handler struct {
	svc      *Service
	resolver *Resolver
	logger   *slog.Logger
}

// NewHandler returns a Handler using svc for group management and resolver
// for POST /groups/resolve.
func NewHandler(svc *Service, resolver *Resolver, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, resolver: resolver, logger: logger}
}

// Register adds the routes to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/groups", h.list)
	mux.HandleFunc("POST /api/groups", h.create)
	mux.HandleFunc("POST /api/groups/resolve", h.resolve)
	mux.HandleFunc("GET /api/groups/{id}", h.get)
	mux.HandleFunc("PUT /api/groups/{id}", h.update)
	mux.HandleFunc("DELETE /api/groups/{id}", h.delete)
	mux.HandleFunc("GET /api/groups/{id}/members", h.members)
	mux.HandleFunc("POST /api/groups/{id}/members", h.addMembers)
	mux.HandleFunc("POST /api/groups/{id}/members/remove", h.removeMembers)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.List(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	out := make([]groupJSON, len(list))
	for i, g := range list {
		out[i] = toGroupJSON(g)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	g, err := h.svc.Info(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toGroupJSON(g))
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeGroupInput(w, r)
	if !ok {
		return
	}
	g, err := h.svc.Create(r.Context(), in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/groups/"+g.ID)
	httpx.JSON(w, http.StatusCreated, toGroupJSON(GroupInfo{Group: g}))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	in, ok := decodeGroupInput(w, r)
	if !ok {
		return
	}
	if _, err := h.svc.Update(r.Context(), id, in); err != nil {
		h.writeError(w, r, err)
		return
	}
	g, err := h.svc.Info(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toGroupJSON(g))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) members(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	rs, err := h.svc.Members(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	out := make([]recipients.RecipientJSON, len(rs))
	for i, rec := range rs {
		out[i] = recipients.NewRecipientJSON(rec)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) addMembers(w http.ResponseWriter, r *http.Request) {
	h.changeMembers(w, r, h.svc.AddMembers)
}

func (h *Handler) removeMembers(w http.ResponseWriter, r *http.Request) {
	h.changeMembers(w, r, h.svc.RemoveMembers)
}

func (h *Handler) changeMembers(w http.ResponseWriter, r *http.Request, change func(ctx context.Context, groupID string, ids []string) (MembershipChange, error)) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		RecipientIDs []string `json:"recipientIds"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_request", Message: "expected a JSON MemberIds"})
		return
	}
	c, err := change(r.Context(), id, body.RecipientIDs)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, membershipJSON{Changed: c.Changed, Unchanged: c.Unchanged})
}

func (h *Handler) resolve(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GroupIDs             []string `json:"groupIds"`
		RecipientIDs         []string `json:"recipientIds"`
		ExcludedRecipientIDs []string `json:"excludedRecipientIds"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_request", Message: "expected a JSON RecipientSelection"})
		return
	}
	res, err := h.resolver.Resolve(r.Context(), Selection{
		GroupIDs:             body.GroupIDs,
		RecipientIDs:         body.RecipientIDs,
		ExcludedRecipientIDs: body.ExcludedRecipientIDs,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toResolutionJSON(res))
}

// pathID checks the {id} path value. A malformed ID is a bad request
// parameter (invalid_request), not a field error of the submitted body.
func pathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := canonicalID(r.PathValue("id"))
	if !IsValidID(id) {
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{
			Code:    "invalid_request",
			Message: "invalid request parameters",
			Fields:  []httpx.FieldError{{Field: "id", Message: "not a valid ID"}},
		})
		return "", false
	}
	return id, true
}

func decodeGroupInput(w http.ResponseWriter, r *http.Request) (GroupInput, bool) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_request", Message: "expected a JSON GroupInput"})
		return GroupInput{}, false
	}
	return GroupInput{Name: body.Name, Description: body.Description}, true
}

// writeError maps a service error to its status and stable code. Unexpected
// errors are logged (they carry IDs, never contact data) and hidden.
func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid *ValidationError
	var unknown *UnknownRecipientsError
	switch {
	case errors.As(err, &invalid):
		fields := make([]httpx.FieldError, len(invalid.Fields))
		for i, f := range invalid.Fields {
			fields[i] = httpx.FieldError{Field: f.Field, Message: f.Message}
		}
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_input", Message: "invalid input", Fields: fields})
	case errors.Is(err, ErrEmptySelection):
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "empty_selection", Message: ErrEmptySelection.Error()})
	case errors.As(err, &unknown):
		httpx.Error(w, http.StatusUnprocessableEntity, httpx.ErrorBody{Code: "unknown_recipient", Message: ErrUnknownRecipient.Error(), IDs: unknown.IDs})
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, httpx.ErrorBody{Code: "not_found", Message: ErrNotFound.Error()})
	case errors.Is(err, ErrSystemGroup):
		httpx.Error(w, http.StatusConflict, httpx.ErrorBody{Code: "system_group", Message: ErrSystemGroup.Error()})
	case errors.Is(err, ErrNameTaken):
		httpx.Error(w, http.StatusConflict, httpx.ErrorBody{
			Code: "name_taken", Message: ErrNameTaken.Error(),
			Fields: []httpx.FieldError{{Field: "name", Message: ErrNameTaken.Error()}},
		})
	default:
		h.logger.Error("groups request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		httpx.Error(w, http.StatusInternalServerError, httpx.ErrorBody{Code: "internal", Message: "internal error"})
	}
}

// JSON shapes of docs/api/openapi.yaml (Group, MembershipChange, Resolution,
// ResolvedRecipient), kept apart from the domain types. Lists are never null.
type (
	groupJSON struct {
		ID          string     `json:"id"`
		Name        string     `json:"name"`
		Description string     `json:"description"`
		Kind        Kind       `json:"kind"`
		MemberCount int        `json:"memberCount"`
		CreatedAt   *time.Time `json:"createdAt"`
	}
	membershipJSON struct {
		Changed   int `json:"changed"`
		Unchanged int `json:"unchanged"`
	}
	resolvedJSON struct {
		Recipient   recipients.RecipientJSON `json:"recipient"`
		ViaGroupIDs []string                 `json:"viaGroupIds"`
		Direct      bool                     `json:"direct"`
		Channels    []Channel                `json:"channels"`
	}
	resolutionJSON struct {
		Recipients          []resolvedJSON `json:"recipients"`
		UnknownGroupIDs     []string       `json:"unknownGroupIds"`
		UnknownRecipientIDs []string       `json:"unknownRecipientIds"`
		ExcludedIDs         []string       `json:"excludedIds"`
		MergedDuplicates    int            `json:"mergedDuplicates"`
		Summary             summaryJSON    `json:"summary"`
	}
	summaryJSON struct {
		Total       int `json:"total"`
		Complete    int `json:"complete"`
		Partial     int `json:"partial"`
		Unreachable int `json:"unreachable"`
	}
)

func toGroupJSON(g GroupInfo) groupJSON {
	out := groupJSON{
		ID:          g.ID,
		Name:        g.Name,
		Description: g.Description,
		Kind:        g.Kind,
		MemberCount: g.MemberCount,
	}
	// Built-in groups live in code and have no creation time (null).
	if !g.IsSystem() && !g.CreatedAt.IsZero() {
		createdAt := g.CreatedAt
		out.CreatedAt = &createdAt
	}
	return out
}

func toResolutionJSON(res Resolution) resolutionJSON {
	out := resolutionJSON{
		Recipients:          make([]resolvedJSON, len(res.Recipients)),
		UnknownGroupIDs:     nonNil(res.UnknownGroupIDs),
		UnknownRecipientIDs: nonNil(res.UnknownRecipientIDs),
		ExcludedIDs:         nonNil(res.ExcludedIDs),
		MergedDuplicates:    res.MergedDuplicates,
	}
	for i, r := range res.Recipients {
		out.Recipients[i] = resolvedJSON{
			Recipient:   recipients.NewRecipientJSON(r.Recipient),
			ViaGroupIDs: nonNil(r.ViaGroupIDs),
			Direct:      r.Direct,
			Channels:    nonNil(r.Channels()),
		}
		switch {
		case r.Unreachable():
			out.Summary.Unreachable++
		case r.Partial():
			out.Summary.Partial++
		default:
			out.Summary.Complete++
		}
	}
	out.Summary.Total = len(res.Recipients)
	return out
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
