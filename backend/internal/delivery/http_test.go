package delivery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

const (
	adminID    = "00000000-0000-4000-8000-0000000000ad"
	batchJSON  = `{"subject":"Zebranie","body":"Czesc {{imie}}","selection":{"groupIds":["` + groupID + `","` + groupID2 + `"]}}`
	headerKey  = "Idempotency-Key"
	callerHdr  = "X-Test-Caller"
	callerNone = "none"
)

type httpEnv struct {
	mux   *http.ServeMux
	svc   *Service
	store *MemStore
	sms   *scriptedSender
}

// newHTTPEnv serves the batch routes. The caller is the sender (userID) unless the
// test header names "admin" or "none".
func newHTTPEnv(t *testing.T, res groups.Resolution) httpEnv {
	t.Helper()
	store := NewMemStore()
	sms := &scriptedSender{channel: providers.ChannelSMS, errs: map[string][]error{id2: {errPermanent}}}
	svc := newTestService(t, res, store, sms)
	caller := func(ctx context.Context) (Caller, bool) {
		switch ctx.Value(callerKey{}) {
		case "admin":
			return Caller{ID: adminID, FullName: "Admin Testowy", IsAdmin: true}, true
		case callerNone:
			return Caller{}, false
		}
		return Caller{ID: userID, FullName: "Nadawca Testowy"}, true
	}
	mux := http.NewServeMux()
	NewHandler(svc, store, caller, nil).Register(mux)
	return httpEnv{mux: mux, svc: svc, store: store, sms: sms}
}

type callerKey struct{}

func (e httpEnv) do(t *testing.T, method, target, body string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	if c := req.Header.Get(callerHdr); c != "" {
		req = req.WithContext(context.WithValue(req.Context(), callerKey{}, c))
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	e.svc.Wait()
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return v
}

func TestHTTPCreateBatch(t *testing.T) {
	env := newHTTPEnv(t, twoRecipients())
	key := map[string]string{headerKey: testKey}

	rec := env.do(t, http.MethodPost, "/api/batches", batchJSON, key)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST = %d %s, want 202", rec.Code, rec.Body)
	}
	got := decode[batchDetailJSON](t, rec)
	if got.ID == "" || got.Status != BatchRunning || got.CreatedBy.FullName != "Nadawca Testowy" ||
		len(got.Groups) != 2 || got.RecipientCount != 2 || len(got.Recipients) != 2 || got.Body != "Czesc {{imie}}" {
		t.Errorf("POST body = %+v", got)
	}
	if r := got.Recipients[0]; r.FirstName != "Anna" || r.RenderedBody == nil || *r.RenderedBody != "Czesc Anna" || len(r.Deliveries) != 2 {
		t.Errorf("recipient = %+v", r)
	}
	if strings.Contains(rec.Body.String(), "example.test") || strings.Contains(rec.Body.String(), "+48") {
		t.Error("response carries contact data")
	}

	again := env.do(t, http.MethodPost, "/api/batches", batchJSON, key)
	if again.Code != http.StatusOK || decode[batchDetailJSON](t, again).ID != got.ID || len(env.sms.sent) != 2 {
		t.Errorf("repeat = %d %s, sms sent %d; want 200 with the same batch, nothing resent", again.Code, again.Body, len(env.sms.sent))
	}

	detail := decode[batchDetailJSON](t, env.do(t, http.MethodGet, "/api/batches/"+got.ID, "", nil))
	if detail.Status != BatchDoneWithErrors || detail.FailedCount != 1 || detail.FinishedAt == nil {
		t.Errorf("GET after dispatch = %s failed %d finished %v", detail.Status, detail.FailedCount, detail.FinishedAt)
	}
	if d := detail.Recipients[1].Deliveries[0]; d.Status != StatusFailed || d.Error == nil || d.Attempts != 1 {
		t.Errorf("failed delivery = %+v", d)
	}
}

func TestHTTPCreateBatchErrors(t *testing.T) {
	changed := twoRecipients()
	changed.UnknownRecipientIDs = []string{id3}
	key := map[string]string{headerKey: testKey}

	tests := []struct {
		name      string
		res       groups.Resolution
		body      string
		header    map[string]string
		wantCode  int
		wantErr   string
		wantField string
	}{
		{"no key", twoRecipients(), batchJSON, nil, 400, "invalid_input", headerKey},
		{"bad key", twoRecipients(), batchJSON, map[string]string{headerKey: "abc"}, 400, "invalid_input", headerKey},
		{"bad json", twoRecipients(), `{"subject":`, key, 400, "invalid_request", ""},
		{"no subject", twoRecipients(), strings.Replace(batchJSON, "Zebranie", " ", 1), key, 400, "invalid_input", "subject"},
		{"no body", twoRecipients(), strings.Replace(batchJSON, "Czesc {{imie}}", "", 1), key, 400, "invalid_input", "body"},
		{"unknown placeholder", twoRecipients(), strings.Replace(batchJSON, "{{imie}}", "{{klasa}}", 1), key, 400, "invalid_input", "body"},
		{"selection changed", changed, batchJSON, key, 409, "selection_changed", ""},
		{"nobody", groups.Resolution{}, batchJSON, key, 422, "no_recipients", ""},
		{"signed out", twoRecipients(), batchJSON, map[string]string{headerKey: testKey, callerHdr: callerNone}, 401, "unauthenticated", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newHTTPEnv(t, tt.res)
			rec := env.do(t, http.MethodPost, "/api/batches", tt.body, tt.header)
			got := decode[httpx.ErrorBody](t, rec)
			if rec.Code != tt.wantCode || got.Code != tt.wantErr {
				t.Fatalf("POST = %d %+v, want %d %s", rec.Code, got, tt.wantCode, tt.wantErr)
			}
			if tt.wantField != "" && (len(got.Fields) != 1 || got.Fields[0].Field != tt.wantField) {
				t.Errorf("fields = %+v, want %s", got.Fields, tt.wantField)
			}
			if tt.name == "selection changed" && (len(got.IDs) != 1 || got.IDs[0] != id3) {
				t.Errorf("ids = %v, want %s", got.IDs, id3)
			}
			if len(env.store.batches) != 0 {
				t.Error("a refused request recorded a batch")
			}
		})
	}
}

func TestHTTPCreateBatchKeyConflict(t *testing.T) {
	env := newHTTPEnv(t, twoRecipients())
	key := map[string]string{headerKey: testKey}
	env.do(t, http.MethodPost, "/api/batches", batchJSON, key)

	rec := env.do(t, http.MethodPost, "/api/batches", strings.Replace(batchJSON, "Zebranie", "Wycieczka", 1), key)
	if got := decode[httpx.ErrorBody](t, rec); rec.Code != http.StatusConflict || got.Code != "idempotency_conflict" {
		t.Errorf("POST other body, same key = %d %+v; want 409 idempotency_conflict", rec.Code, got)
	}
}

func TestHTTPListAndGetAccess(t *testing.T) {
	env := newHTTPEnv(t, twoRecipients())
	mine := decode[batchDetailJSON](t, env.do(t, http.MethodPost, "/api/batches", batchJSON, map[string]string{headerKey: testKey}))
	admins := decode[batchDetailJSON](t, env.do(t, http.MethodPost, "/api/batches", batchJSON,
		map[string]string{headerKey: testKey, callerHdr: "admin"}))

	page := decode[batchPageJSON](t, env.do(t, http.MethodGet, "/api/batches", "", nil))
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != mine.ID {
		t.Errorf("sender's list = %+v; want only their batch", page)
	}
	page = decode[batchPageJSON](t, env.do(t, http.MethodGet, "/api/batches?limit=1", "", map[string]string{callerHdr: "admin"}))
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].ID != admins.ID {
		t.Errorf("admin's list = %+v; want both batches, newest first, one per page", page)
	}
	// Only the first batch hit the scripted SMS failure.
	page = decode[batchPageJSON](t, env.do(t, http.MethodGet, "/api/batches?status=done_with_errors", "", map[string]string{callerHdr: "admin"}))
	if page.Total != 1 || page.Items[0].ID != mine.ID {
		t.Errorf("admin's done_with_errors = %+v; want the sender's batch", page)
	}
	if empty := env.do(t, http.MethodGet, "/api/batches?status=draft", "", nil); !strings.Contains(empty.Body.String(), `"items":[]`) {
		t.Errorf("empty page = %s; want items: []", empty.Body)
	}

	if rec := env.do(t, http.MethodGet, "/api/batches/"+admins.ID, "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("sender GET admin's batch = %d; want 404", rec.Code)
	}
	if rec := env.do(t, http.MethodGet, "/api/batches/"+mine.ID, "", map[string]string{callerHdr: "admin"}); rec.Code != http.StatusOK {
		t.Errorf("admin GET sender's batch = %d; want 200", rec.Code)
	}
	if rec := env.do(t, http.MethodGet, "/api/batches/nope", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET unknown = %d; want 404", rec.Code)
	}

	for _, q := range []string{"status=sent", "limit=0", "limit=201", "offset=-1", "limit=x"} {
		if rec := env.do(t, http.MethodGet, "/api/batches?"+q, "", nil); rec.Code != http.StatusBadRequest {
			t.Errorf("GET ?%s = %d; want 400", q, rec.Code)
		}
	}
}
