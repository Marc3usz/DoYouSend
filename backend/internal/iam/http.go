package iam

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
)

// SendingConfig is the sending setup shown to administrators (description.md
// "konfigurowania operatora e-mail i SMS"). It holds no secret: credentials
// are reported only as set or not. Changing it means editing the environment
// and, for a real operator, the supervisor's approval (CLAUDE.md rule 3).
type SendingConfig struct {
	DryRun               bool
	EmailProvider        string
	EmailFrom            string
	EmailSandbox         bool
	EmailCredentialsSet  bool
	SMSProvider          string
	SMSSenderName        string
	SMSTestMode          bool
	SMSCredentialsSet    bool
	SMSPricePerPartMilli int64
	SMSReportsWebhook    bool
	EmailEventsWebhook   bool
}

// Handler serves the auth, user and admin endpoints of docs/api/openapi.yaml.
// Access itself is decided by Middleware; handlers only read the user.
type Handler struct {
	svc     *Service
	cookies CookieOptions
	config  SendingConfig
	logger  *slog.Logger
}

// NewHandler returns a Handler using svc.
func NewHandler(svc *Service, cookies CookieOptions, config SendingConfig, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, cookies: cookies, config: config, logger: logger}
}

// Register adds the routes to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.HandleFunc("POST /api/auth/logout", h.logout)
	mux.HandleFunc("GET /api/auth/me", h.me)
	mux.HandleFunc("GET /api/users", h.listUsers)
	mux.HandleFunc("POST /api/users", h.createUser)
	mux.HandleFunc("PATCH /api/users/{id}", h.updateUser)
	mux.HandleFunc("PUT /api/users/{id}/password", h.resetPassword)
	mux.HandleFunc("GET /api/audit", h.listAudit)
	mux.HandleFunc("GET /api/admin/config", h.sendingConfig)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_request", Message: "expected a JSON LoginRequest"})
		return
	}
	session, err := h.svc.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	http.SetCookie(w, h.cookies.session(session.Token, session.ExpiresAt))
	httpx.JSON(w, http.StatusOK, toUserJSON(session.User))
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFrom(r.Context())
	if err := h.svc.Logout(r.Context(), user, sessionToken(r)); err != nil {
		h.writeError(w, r, err)
		return
	}
	http.SetCookie(w, h.cookies.cleared())
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFrom(r.Context())
	httpx.JSON(w, http.StatusOK, toUserJSON(user))
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.ListUsers(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	out := make([]userJSON, len(users))
	for i, u := range users {
		out[i] = toUserJSON(u)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		FullName string `json:"fullName"`
		Role     Role   `json:"role"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_request", Message: "expected a JSON UserInput"})
		return
	}
	actor, _ := UserFrom(r.Context())
	created, err := h.svc.CreateUser(r.Context(), actor, NewUser(body))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toUserJSON(created))
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FullName *string `json:"fullName"`
		Role     *Role   `json:"role"`
		Disabled *bool   `json:"disabled"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_request", Message: "expected a JSON UserChange"})
		return
	}
	actor, _ := UserFrom(r.Context())
	updated, err := h.svc.UpdateUser(r.Context(), actor, r.PathValue("id"), UserChange(body))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toUserJSON(updated))
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_request", Message: "expected a JSON PasswordInput"})
		return
	}
	actor, _ := UserFrom(r.Context())
	if err := h.svc.ResetPassword(r.Context(), actor, r.PathValue("id"), body.Password); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err1 := optionalInt(q.Get("limit"))
	offset, err2 := optionalInt(q.Get("offset"))
	if err1 != nil || err2 != nil {
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_request", Message: "limit and offset must be integers"})
		return
	}
	page, err := h.svc.ListAudit(r.Context(), limit, offset)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	out := auditPageJSON{Items: make([]auditJSON, len(page.Items)), Total: page.Total}
	for i, e := range page.Items {
		out.Items[i] = toAuditJSON(e)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) sendingConfig(w http.ResponseWriter, _ *http.Request) {
	c := h.config
	httpx.JSON(w, http.StatusOK, sendingConfigJSON{
		DryRun: c.DryRun,
		Email: emailConfigJSON{
			Provider: c.EmailProvider, From: c.EmailFrom, Sandbox: c.EmailSandbox,
			CredentialsSet: c.EmailCredentialsSet, EventsWebhook: c.EmailEventsWebhook,
		},
		SMS: smsConfigJSON{
			Provider: c.SMSProvider, SenderName: c.SMSSenderName, TestMode: c.SMSTestMode,
			CredentialsSet: c.SMSCredentialsSet, ReportsWebhook: c.SMSReportsWebhook,
			PricePerPartMilli: c.SMSPricePerPartMilli,
		},
	})
}

// writeError maps a service error to its status and stable code.
func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid *ValidationError
	switch {
	case errors.As(err, &invalid):
		fields := make([]httpx.FieldError, len(invalid.Fields))
		for i, f := range invalid.Fields {
			fields[i] = httpx.FieldError(f)
		}
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_input", Message: "invalid input", Fields: fields})
	case errors.Is(err, ErrInvalidCredentials):
		httpx.Error(w, http.StatusUnauthorized, httpx.ErrorBody{Code: "invalid_credentials", Message: ErrInvalidCredentials.Error()})
	case errors.Is(err, ErrTooManyAttempts):
		w.Header().Set("Retry-After", strconv.Itoa(int(FailedLoginWindow.Seconds())))
		httpx.Error(w, http.StatusTooManyRequests, httpx.ErrorBody{Code: "too_many_attempts", Message: ErrTooManyAttempts.Error()})
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, httpx.ErrorBody{Code: "not_found", Message: ErrNotFound.Error()})
	case errors.Is(err, ErrEmailTaken):
		httpx.Error(w, http.StatusConflict, httpx.ErrorBody{Code: "email_taken", Message: ErrEmailTaken.Error(),
			Fields: []httpx.FieldError{{Field: "email", Message: ErrEmailTaken.Error()}}})
	case errors.Is(err, ErrLastAdmin):
		httpx.Error(w, http.StatusConflict, httpx.ErrorBody{Code: "last_admin", Message: ErrLastAdmin.Error()})
	case errors.Is(err, ErrSelfLockout):
		httpx.Error(w, http.StatusConflict, httpx.ErrorBody{Code: "self_lockout", Message: ErrSelfLockout.Error()})
	default:
		h.logger.Error("iam request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		httpx.Error(w, http.StatusInternalServerError, httpx.ErrorBody{Code: "internal", Message: "internal error"})
	}
}

func optionalInt(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	return strconv.Atoi(raw)
}

// JSON shapes of docs/api/openapi.yaml (User, AuditEntry, SendingConfig).
type (
	userJSON struct {
		ID          string     `json:"id"`
		Email       string     `json:"email"`
		FullName    string     `json:"fullName"`
		Role        Role       `json:"role"`
		Disabled    bool       `json:"disabled"`
		CreatedAt   time.Time  `json:"createdAt"`
		LastLoginAt *time.Time `json:"lastLoginAt"`
	}
	auditJSON struct {
		ID        int64             `json:"id"`
		UserID    *string           `json:"userId"`
		UserName  *string           `json:"userName"`
		Action    string            `json:"action"`
		Entity    *string           `json:"entity"`
		EntityID  *string           `json:"entityId"`
		Details   map[string]string `json:"details"`
		CreatedAt time.Time         `json:"createdAt"`
	}
	auditPageJSON struct {
		Items []auditJSON `json:"items"`
		Total int         `json:"total"`
	}
	sendingConfigJSON struct {
		DryRun bool            `json:"dryRun"`
		Email  emailConfigJSON `json:"email"`
		SMS    smsConfigJSON   `json:"sms"`
	}
	emailConfigJSON struct {
		Provider       string `json:"provider"`
		From           string `json:"from"`
		Sandbox        bool   `json:"sandbox"`
		CredentialsSet bool   `json:"credentialsSet"`
		EventsWebhook  bool   `json:"eventsWebhook"`
	}
	smsConfigJSON struct {
		Provider          string `json:"provider"`
		SenderName        string `json:"senderName"`
		TestMode          bool   `json:"testMode"`
		CredentialsSet    bool   `json:"credentialsSet"`
		ReportsWebhook    bool   `json:"reportsWebhook"`
		PricePerPartMilli int64  `json:"pricePerPartMilli"`
	}
)

func toUserJSON(u User) userJSON {
	out := userJSON{ID: u.ID, Email: u.Email, FullName: u.FullName, Role: u.Role, Disabled: u.Disabled, CreatedAt: u.CreatedAt}
	if !u.LastLoginAt.IsZero() {
		t := u.LastLoginAt
		out.LastLoginAt = &t
	}
	return out
}

func toAuditJSON(e AuditEntry) auditJSON {
	details := e.Details
	if details == nil {
		details = map[string]string{}
	}
	return auditJSON{
		ID: e.ID, UserID: optional(e.UserID), UserName: optional(e.UserName), Action: e.Action,
		Entity: optional(e.Entity), EntityID: optional(e.EntityID), Details: details, CreatedAt: e.CreatedAt,
	}
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
