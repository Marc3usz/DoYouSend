package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
)

// Handler serves GET and PATCH /api/admin/settings (docs/api/openapi.yaml).
// The /api/admin prefix is administrator-only (iam policy).
type Handler struct {
	svc    *Service
	actor  func(ctx context.Context) string
	logger *slog.Logger
}

// NewHandler returns a Handler; actor returns the signed-in user's ID.
func NewHandler(svc *Service, actor func(ctx context.Context) string, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, actor: actor, logger: logger}
}

// Register adds the routes to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/settings", h.get)
	mux.HandleFunc("PATCH /api/admin/settings", h.patch)
}

func (h *Handler) get(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, toJSON(h.svc.Current()))
}

func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		// Values maps a setting key to its new value; null drops the panel value.
		Values             map[string]json.RawMessage `json:"values"`
		ConfirmLiveSending bool                       `json:"confirmLiveSending"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_request", Message: "expected a JSON SettingsChange"})
		return
	}
	changes := make([]Change, 0, len(body.Values))
	var fields []httpx.FieldError
	for key, raw := range body.Values {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			changes = append(changes, Change{Key: key})
			continue
		}
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			fields = append(fields, httpx.FieldError{Field: key, Message: "must be a string or null"})
			continue
		}
		changes = append(changes, Change{Key: key, Value: &v})
	}
	if len(fields) > 0 {
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_input", Message: "invalid input", Fields: fields})
		return
	}
	// Map order is random; a stable order keeps error lists and audit details stable.
	sort.Slice(changes, func(i, j int) bool { return changes[i].Key < changes[j].Key })

	snap, err := h.svc.Update(r.Context(), h.actor(r.Context()), changes, body.ConfirmLiveSending)
	var invalid *ValidationError
	switch {
	case err == nil:
		httpx.JSON(w, http.StatusOK, toJSON(snap))
	case errors.As(err, &invalid):
		fields := make([]httpx.FieldError, len(invalid.Fields))
		for i, f := range invalid.Fields {
			fields[i] = httpx.FieldError(f)
		}
		httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_input", Message: "invalid input", Fields: fields})
	case errors.Is(err, ErrConfirmationRequired):
		httpx.Error(w, http.StatusConflict, httpx.ErrorBody{Code: "confirmation_required", Message: err.Error()})
	case errors.Is(err, ErrDryRunLocked):
		httpx.Error(w, http.StatusConflict, httpx.ErrorBody{Code: "dry_run_locked", Message: err.Error()})
	case errors.Is(err, ErrBatchesRunning):
		httpx.Error(w, http.StatusConflict, httpx.ErrorBody{Code: "batches_running", Message: err.Error()})
	default:
		h.logger.Error("save settings", "err", err)
		httpx.Error(w, http.StatusInternalServerError, httpx.ErrorBody{Code: "internal", Message: "internal error"})
	}
}

// JSON shapes of docs/api/openapi.yaml (Settings, SettingItem).
type (
	settingsJSON struct {
		DryRun          bool          `json:"dryRun"`
		DryRunLocked    bool          `json:"dryRunLocked"`
		SendsExternally bool          `json:"sendsExternally"`
		Items           []settingJSON `json:"items"`
	}
	settingJSON struct {
		Key     string   `json:"key"`
		Group   string   `json:"group"`
		Kind    Kind     `json:"kind"`
		Options []string `json:"options,omitempty"`
		// Value is null for secrets: only Set says whether one is stored.
		Value      *string `json:"value"`
		Set        bool    `json:"set"`
		Source     Source  `json:"source"`
		EnvVar     string  `json:"envVar"`
		Unreadable bool    `json:"unreadable"`
	}
)

func toJSON(s Snapshot) settingsJSON {
	out := settingsJSON{DryRun: s.DryRun(), DryRunLocked: s.DryRunLocked, SendsExternally: s.SendsExternally()}
	for _, st := range s.Settings() {
		item := settingJSON{
			Key: st.Key, Group: st.Group, Kind: st.Kind, Options: st.Options,
			Set: st.Set(), Source: st.Source, EnvVar: st.Env, Unreadable: st.Unreadable,
		}
		if st.Kind != KindSecret {
			v := st.Value
			item.Value = &v
		}
		out.Items = append(out.Items, item)
	}
	return out
}
