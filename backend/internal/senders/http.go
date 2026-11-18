package senders

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
)

// Handler serves the sender endpoints of docs/api/openapi.yaml (ADR-0021).
// /api/me/senders is open to every signed-in user; /api/admin/* only to
// administrators (iam policy).
type Handler struct {
	svc    *Service
	user   func(ctx context.Context) (User, bool)
	logger *slog.Logger
}

// NewHandler returns a Handler; user returns the signed-in user.
func NewHandler(svc *Service, user func(ctx context.Context) (User, bool), logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, user: user, logger: logger}
}

// Register adds the routes to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/me/senders", h.myOptions)
	mux.HandleFunc("GET /api/admin/sender-domains", h.listDomains)
	mux.HandleFunc("POST /api/admin/sender-domains", h.addDomain)
	mux.HandleFunc("POST /api/admin/sender-domains/{id}/validate", h.checkDomain)
	mux.HandleFunc("DELETE /api/admin/sender-domains/{id}", h.removeDomain)
	mux.HandleFunc("GET /api/admin/sender-addresses", h.listAddresses)
	mux.HandleFunc("POST /api/admin/sender-addresses", h.createAddress)
	mux.HandleFunc("PATCH /api/admin/sender-addresses/{id}", h.updateAddress)
	mux.HandleFunc("DELETE /api/admin/sender-addresses/{id}", h.deleteAddress)
}

// JSON shapes of docs/api/openapi.yaml.
type (
	optionJSON struct {
		ID     string  `json:"id"`
		Email  string  `json:"email"`
		Name   string  `json:"name"`
		Kind   Kind    `json:"kind"`
		Usable bool    `json:"usable"`
		Reason *Reason `json:"reason"`
	}
	domainJSON struct {
		ID          string       `json:"id"`
		Domain      string       `json:"domain"`
		Status      DomainStatus `json:"status"`
		DNSRecords  []DNSRecord  `json:"dnsRecords"`
		CreatedAt   time.Time    `json:"createdAt"`
		CheckedAt   *time.Time   `json:"checkedAt"`
		ValidatedAt *time.Time   `json:"validatedAt"`
	}
	userRefJSON struct {
		ID       string `json:"id"`
		FullName string `json:"fullName"`
	}
	addressJSON struct {
		ID        string        `json:"id"`
		Email     string        `json:"email"`
		Name      string        `json:"name"`
		Users     []userRefJSON `json:"users"`
		Usable    bool          `json:"usable"`
		Reason    *Reason       `json:"reason"`
		CreatedAt time.Time     `json:"createdAt"`
	}
)

func (h *Handler) myOptions(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, httpx.ErrorBody{Code: "unauthenticated", Message: "not signed in"})
		return
	}
	opts, def, err := h.svc.Options(r.Context(), u)
	if err != nil {
		h.internal(w, "sender options", err)
		return
	}
	out := struct {
		DefaultID string       `json:"defaultId"`
		Items     []optionJSON `json:"items"`
	}{DefaultID: def, Items: make([]optionJSON, len(opts))}
	for i, o := range opts {
		out.Items[i] = optionJSON{
			ID: o.ID, Email: o.Address.Email, Name: o.Address.Name, Kind: o.Kind,
			Usable: o.Usable, Reason: reasonPtr(o.Reason),
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) listDomains(w http.ResponseWriter, r *http.Request) {
	ds, err := h.svc.ListDomains(r.Context())
	if err != nil {
		h.internal(w, "list sender domains", err)
		return
	}
	out := struct {
		Live  bool         `json:"live"`
		Items []domainJSON `json:"items"`
	}{Live: h.svc.Live(), Items: make([]domainJSON, len(ds))}
	for i, d := range ds {
		out.Items[i] = toDomainJSON(d)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) addDomain(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Domain string `json:"domain"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		badRequest(w)
		return
	}
	d, err := h.svc.AddDomain(r.Context(), h.actor(r), body.Domain)
	if err != nil {
		h.writeError(w, "add sender domain", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toDomainJSON(d))
}

func (h *Handler) checkDomain(w http.ResponseWriter, r *http.Request) {
	d, err := h.svc.CheckDomain(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeError(w, "validate sender domain", err)
		return
	}
	httpx.JSON(w, http.StatusOK, toDomainJSON(d))
}

func (h *Handler) removeDomain(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RemoveDomain(r.Context(), h.actor(r), r.PathValue("id")); err != nil {
		h.writeError(w, "remove sender domain", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listAddresses(w http.ResponseWriter, r *http.Request) {
	as, err := h.svc.ListAddresses(r.Context())
	if err != nil {
		h.internal(w, "list sender addresses", err)
		return
	}
	out := struct {
		Items []addressJSON `json:"items"`
	}{Items: make([]addressJSON, 0, len(as))}
	for _, a := range as {
		aj, err := h.toAddressJSON(r.Context(), a)
		if err != nil {
			h.internal(w, "sender address status", err)
			return
		}
		out.Items = append(out.Items, aj)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) createAddress(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email   string   `json:"email"`
		Name    string   `json:"name"`
		UserIDs []string `json:"userIds"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		badRequest(w)
		return
	}
	a, err := h.svc.CreateAddress(r.Context(), h.actor(r), body.Email, body.Name, body.UserIDs)
	if err != nil {
		h.writeError(w, "create sender address", err)
		return
	}
	h.writeAddress(w, r, http.StatusCreated, a)
}

func (h *Handler) updateAddress(w http.ResponseWriter, r *http.Request) {
	// json.RawMessage tells a missing field (unchanged) from a present one.
	var body struct {
		Name    *string         `json:"name"`
		UserIDs json.RawMessage `json:"userIds"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		badRequest(w)
		return
	}
	var userIDs *[]string
	if len(body.UserIDs) > 0 && string(body.UserIDs) != "null" {
		var ids []string
		if err := json.Unmarshal(body.UserIDs, &ids); err != nil {
			fieldError(w, &ValidationError{Fields: []FieldError{{"userIds", "must be a list of user IDs"}}})
			return
		}
		userIDs = &ids
	}
	a, err := h.svc.UpdateAddress(r.Context(), h.actor(r), r.PathValue("id"), body.Name, userIDs)
	if err != nil {
		h.writeError(w, "update sender address", err)
		return
	}
	h.writeAddress(w, r, http.StatusOK, a)
}

func (h *Handler) deleteAddress(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteAddress(r.Context(), h.actor(r), r.PathValue("id")); err != nil {
		h.writeError(w, "delete sender address", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) writeAddress(w http.ResponseWriter, r *http.Request, status int, a Address) {
	aj, err := h.toAddressJSON(r.Context(), a)
	if err != nil {
		h.internal(w, "sender address status", err)
		return
	}
	httpx.JSON(w, status, aj)
}

func (h *Handler) toAddressJSON(ctx context.Context, a Address) (addressJSON, error) {
	usable, reason, err := h.svc.AddressUsable(ctx, a.Email)
	if err != nil {
		return addressJSON{}, err
	}
	out := addressJSON{
		ID: a.ID, Email: a.Email, Name: a.Name, Users: make([]userRefJSON, len(a.Users)),
		Usable: usable, Reason: reasonPtr(reason), CreatedAt: a.CreatedAt,
	}
	for i, u := range a.Users {
		out.Users[i] = userRefJSON(u)
	}
	return out, nil
}

func (h *Handler) actor(r *http.Request) string {
	u, _ := h.user(r.Context())
	return u.ID
}

func (h *Handler) writeError(w http.ResponseWriter, op string, err error) {
	var v *ValidationError
	var rejected *RejectedError
	switch {
	case errors.As(err, &v):
		fieldError(w, v)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, httpx.ErrorBody{Code: "not_found", Message: "not found"})
	case errors.Is(err, ErrDomainExists):
		httpx.Error(w, http.StatusConflict, httpx.ErrorBody{Code: "domain_exists", Message: err.Error()})
	case errors.Is(err, ErrAddressExists):
		httpx.Error(w, http.StatusConflict, httpx.ErrorBody{Code: "address_exists", Message: err.Error()})
	case errors.As(err, &rejected):
		httpx.Error(w, http.StatusBadGateway, httpx.ErrorBody{Code: "provider_rejected", Message: rejected.Message})
	default:
		h.internal(w, op, err)
	}
}

func (h *Handler) internal(w http.ResponseWriter, op string, err error) {
	h.logger.Error(op, "err", err)
	httpx.Error(w, http.StatusInternalServerError, httpx.ErrorBody{Code: "internal", Message: "internal error"})
}

func badRequest(w http.ResponseWriter) {
	httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_request", Message: "expected a JSON body"})
}

func fieldError(w http.ResponseWriter, v *ValidationError) {
	out := httpx.ErrorBody{Code: "invalid_input", Message: "invalid input"}
	for _, f := range v.Fields {
		out.Fields = append(out.Fields, httpx.FieldError{Field: f.Field, Message: f.Message})
	}
	httpx.Error(w, http.StatusBadRequest, out)
}

func toDomainJSON(d Domain) domainJSON {
	out := domainJSON{
		ID: d.ID, Domain: d.Domain, Status: d.Status, DNSRecords: nonNil(d.Records), CreatedAt: d.CreatedAt,
	}
	if !d.CheckedAt.IsZero() {
		t := d.CheckedAt
		out.CheckedAt = &t
	}
	if !d.ValidatedAt.IsZero() {
		t := d.ValidatedAt
		out.ValidatedAt = &t
	}
	return out
}

func reasonPtr(r Reason) *Reason {
	if r == ReasonNone {
		return nil
	}
	return &r
}
