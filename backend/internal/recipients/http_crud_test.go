package recipients

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func crudServer(store *memRecipients) *http.ServeMux {
	mux := http.NewServeMux()
	NewHandler(NewService(store), slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)
	return mux
}

func do(t *testing.T, mux *http.ServeMux, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var out map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s %s: response is not a JSON object: %q", method, path, rec.Body.String())
		}
	}
	return rec, out
}

func TestHandlerList(t *testing.T) {
	mux := crudServer(serviceFixture())

	rec, body := do(t, mux, http.MethodGet, "/api/recipients?issue=sms&limit=1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %v", rec.Code, body)
	}
	if body["total"] != 2.0 {
		t.Errorf("total = %v, want 2", body["total"])
	}
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items = %v, want one", items)
	}
	maria, _ := items[0].(map[string]any)
	if maria["phone"] != nil || maria["email"] != "maria.kowalska@example.test" {
		t.Errorf("item = %v, want null phone", maria)
	}
	if _, ok := maria["groupIds"]; ok {
		t.Error("list items must not carry groupIds")
	}
	issues, _ := maria["issues"].([]any)
	if len(issues) != 1 || issues[0].(map[string]any)["channel"] != "sms" || issues[0].(map[string]any)["reason"] != "missing" {
		t.Errorf("issues = %v, want sms missing", issues)
	}
}

func TestHandlerLookup(t *testing.T) {
	mux := crudServer(serviceFixture())

	rec, body := do(t, mux, http.MethodGet, "/api/recipients/lookup?q=MARIA", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %v", rec.Code, body)
	}
	items, _ := body["items"].([]any)
	if body["total"] != 1.0 || len(items) != 1 {
		t.Fatalf("lookup = %v, want Maria only", body)
	}
	maria, _ := items[0].(map[string]any)
	for _, field := range []string{"email", "phone", "createdAt", "updatedAt", "groupIds"} {
		if _, ok := maria[field]; ok {
			t.Errorf("lookup item carries %q: %v", field, maria)
		}
	}
	issues, _ := maria["issues"].([]any)
	if maria["firstName"] != "Maria" || maria["type"] != "parent" || len(issues) != 1 {
		t.Errorf("item = %v, want Maria, parent, the missing phone", maria)
	}
	if classes, ok := maria["classes"].([]any); !ok || len(classes) != 0 {
		t.Errorf("classes = %v, want []", maria["classes"])
	}

	// Searching by contact data must not reveal whose it is.
	rec, _ = do(t, mux, http.MethodGet, "/api/recipients/lookup?q=maria.kowalska%40example.test", "")
	if got := strings.TrimSpace(rec.Body.String()); got != `{"items":[],"total":0}` {
		t.Errorf("lookup by e-mail = %s, want nothing", got)
	}

	rec, body = do(t, mux, http.MethodGet, "/api/recipients/lookup?limit=abc", "")
	if rec.Code != http.StatusBadRequest || body["code"] != "invalid_request" {
		t.Errorf("bad limit = %d %v, want 400", rec.Code, body)
	}
}

func TestHandlerListEmptyIsArray(t *testing.T) {
	rec, _ := do(t, crudServer(serviceFixture()), http.MethodGet, "/api/recipients?q=nikt", "")
	if got := strings.TrimSpace(rec.Body.String()); got != `{"items":[],"total":0}` {
		t.Errorf("body = %s, want empty items as []", got)
	}
}

func TestHandlerErrors(t *testing.T) {
	store := serviceFixture()
	store.inUse[store.id("Lena")] = true
	mux := crudServer(store)
	missing := "00000000-0000-4000-8000-0000000000ff"

	tests := []struct {
		name, method, path, body string
		status                   int
		code                     string
	}{
		{"bad limit", http.MethodGet, "/api/recipients?limit=abc", "", 400, "invalid_request"},
		{"bad type", http.MethodGet, "/api/recipients?type=teacher", "", 400, "invalid_request"},
		{"bad id", http.MethodGet, "/api/recipients/xyz", "", 400, "invalid_request"},
		{"unknown id", http.MethodGet, "/api/recipients/" + missing, "", 404, "not_found"},
		{"not JSON", http.MethodPost, "/api/recipients", "{", 400, "invalid_request"},
		{"two JSON values", http.MethodPost, "/api/recipients", "{}{}", 400, "invalid_request"},
		{"invalid fields", http.MethodPost, "/api/recipients", `{"firstName":"A"}`, 400, "invalid_input"},
		{"duplicate", http.MethodPost, "/api/recipients", `{"firstName":"A","lastName":"B","email":"jan.kowalski@example.test","type":"parent"}`, 409, "duplicate_contact"},
		{"update unknown", http.MethodPut, "/api/recipients/" + missing, `{}`, 404, "not_found"},
		{"delete in use", http.MethodDelete, "/api/recipients/" + store.id("Lena"), "", 409, "recipient_in_use"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, body := do(t, mux, tt.method, tt.path, tt.body)
			if rec.Code != tt.status || body["code"] != tt.code {
				t.Errorf("= %d %v, want %d %s", rec.Code, body, tt.status, tt.code)
			}
		})
	}
}

func TestHandlerDuplicateNamesExistingRecipient(t *testing.T) {
	store := serviceFixture()
	_, body := do(t, crudServer(store), http.MethodPost, "/api/recipients",
		`{"firstName":"A","lastName":"B","phone":"500 100 103","type":"student"}`)
	ids, _ := body["ids"].([]any)
	fields, _ := body["fields"].([]any)
	if len(ids) != 1 || ids[0] != store.id("Lena") || len(fields) != 1 || fields[0].(map[string]any)["field"] != "phone" {
		t.Errorf("body = %v, want phone field and Lena's ID", body)
	}
	if strings.Contains(strings.ToLower(body["message"].(string)), "500") {
		t.Errorf("message %q leaks the phone number", body["message"])
	}
}

func TestHandlerCreateGetUpdateDelete(t *testing.T) {
	store := serviceFixture()
	mux := crudServer(store)

	rec, created := do(t, mux, http.MethodPost, "/api/recipients",
		`{"firstName":"Oskar","lastName":"Adamski","email":"oskar.adamski@example.test","phone":null,"type":"student","id":"ignored"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %v", rec.Code, created)
	}
	id, _ := created["id"].(string)
	if rec.Header().Get("Location") != "/api/recipients/"+id || id == "ignored" {
		t.Errorf("Location = %q, id = %q", rec.Header().Get("Location"), id)
	}

	rec, got := do(t, mux, http.MethodGet, "/api/recipients/"+id, "")
	if rec.Code != http.StatusOK || got["groupIds"] == nil {
		t.Errorf("get = %d %v, want groupIds as []", rec.Code, got)
	}

	rec, updated := do(t, mux, http.MethodPut, "/api/recipients/"+id,
		`{"firstName":"Oskar","lastName":"Adamski","email":"","phone":"500 100 120","type":"student"}`)
	if rec.Code != http.StatusOK || updated["email"] != nil || updated["phone"] != "+48500100120" {
		t.Errorf("update = %d %v, want e-mail cleared and phone normalized", rec.Code, updated)
	}

	rec, _ = do(t, mux, http.MethodDelete, "/api/recipients/"+id, "")
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Errorf("delete = %d %q", rec.Code, rec.Body.String())
	}
}

func TestHandlerHidesStoreErrors(t *testing.T) {
	store := serviceFixture()
	store.err = errors.New("connection refused to db.internal")
	rec, body := do(t, crudServer(store), http.MethodGet, "/api/recipients", "")
	if rec.Code != http.StatusInternalServerError || body["code"] != "internal" || strings.Contains(rec.Body.String(), "db.internal") {
		t.Errorf("= %d %s, want a generic 500", rec.Code, rec.Body.String())
	}
}

func TestHandlerStats(t *testing.T) {
	mux := crudServer(serviceFixture())

	rec, body := do(t, mux, http.MethodGet, "/api/recipients/stats", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %v", rec.Code, body)
	}
	want := map[string]any{
		"total": 4.0, "parents": 3.0, "students": 1.0,
		"withoutEmail": 1.0, "withoutPhone": 2.0, "unreachable": 0.0,
	}
	if len(body) != len(want) {
		t.Errorf("body = %v, want exactly %d fields", body, len(want))
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("%s = %v, want %v", k, body[k], v)
		}
	}
	if body["email"] != nil || body["items"] != nil {
		t.Errorf("stats must not carry recipient data: %v", body)
	}
}

func TestHandlerStatsIsNotARecipientID(t *testing.T) {
	// "stats" must never be read as the {id} of GET /api/recipients/{id}.
	rec, _ := do(t, crudServer(serviceFixture()), http.MethodGet, "/api/recipients/stats", "")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (not 400/404 from the {id} route)", rec.Code)
	}
}

func TestHandlerStatsStoreError(t *testing.T) {
	store := serviceFixture()
	store.err = errors.New("boom")
	rec, body := do(t, crudServer(store), http.MethodGet, "/api/recipients/stats", "")
	if rec.Code != http.StatusInternalServerError || body["code"] != "internal" {
		t.Errorf("status = %d, body %v, want 500 internal", rec.Code, body)
	}
}

type recordedFix struct {
	id       string
	channels []Channel
}

type fakeContactAuditor struct{ got []recordedFix }

func (a *fakeContactAuditor) ContactFixed(_ context.Context, id string, channels []Channel) {
	a.got = append(a.got, recordedFix{id, channels})
}

func TestHandlerFixContact(t *testing.T) {
	store := serviceFixture()
	audit := &fakeContactAuditor{}
	mux := http.NewServeMux()
	NewHandler(NewService(store), slog.New(slog.NewTextHandler(io.Discard, nil))).WithAuditor(audit).Register(mux)
	maria, lena := store.id("Maria"), store.id("Lena")

	rec, body := do(t, mux, http.MethodPatch, "/api/recipients/"+maria+"/contact", `{"phone":"500100120"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %v", rec.Code, body)
	}
	if _, ok := body["phone"]; ok {
		t.Errorf("response must not carry contact data: %v", body)
	}
	if issues, _ := body["issues"].([]any); len(issues) != 0 {
		t.Errorf("issues = %v, want none after the fix", issues)
	}
	if want := []recordedFix{{maria, []Channel{ChannelSMS}}}; !reflect.DeepEqual(audit.got, want) {
		t.Errorf("audit = %+v, want %+v", audit.got, want)
	}

	tests := []struct {
		name, id, body string
		status         int
		code           string
	}{
		{"working channel", maria, `{"email":"x@example.test"}`, http.StatusConflict, "contact_not_fixable"},
		{"duplicate", lena, `{"email":"jan.kowalski@example.test"}`, http.StatusConflict, "duplicate_contact"},
		{"nothing given", lena, `{}`, http.StatusBadRequest, "invalid_input"},
		{"not JSON", lena, `[]`, http.StatusBadRequest, "invalid_request"},
		{"unknown", "00000000-0000-4000-8000-0000000000ff", `{"email":"a@example.test"}`, http.StatusNotFound, "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, body := do(t, mux, http.MethodPatch, "/api/recipients/"+tt.id+"/contact", tt.body)
			if rec.Code != tt.status || body["code"] != tt.code {
				t.Fatalf("got %d %v, want %d %s", rec.Code, body["code"], tt.status, tt.code)
			}
			if _, ok := body["ids"]; ok {
				t.Errorf("error must not name other recipients: %v", body)
			}
		})
	}
	if len(audit.got) != 1 {
		t.Errorf("failed fixes must not be audited: %+v", audit.got)
	}
}
