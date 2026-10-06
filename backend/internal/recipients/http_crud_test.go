package recipients

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
