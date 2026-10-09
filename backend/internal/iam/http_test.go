package iam

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testServer is the iam handler behind the middleware, next to a stand-in
// route of another domain for each access level.
func testServer(t *testing.T) (http.Handler, *Service, User, User) {
	t.Helper()
	svc, _, _, admin, sender := testService(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	NewHandler(svc, CookieOptions{Secure: true}, SendingConfig{DryRun: true, SMSProvider: "fake", SMSPricePerPartMilli: 80}, logger).Register(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("POST /api/messages/preview", func(w http.ResponseWriter, r *http.Request) {
		u, _ := UserFrom(r.Context())
		_, _ = io.WriteString(w, u.ID)
	})
	mux.HandleFunc("GET /api/recipients", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return Middleware(svc, logger, mux), svc, admin, sender
}

func request(t *testing.T, h http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func login(t *testing.T, h http.Handler, email, password string) *http.Cookie {
	t.Helper()
	rec := request(t, h, http.MethodPost, "/api/auth/login", `{"email":"`+email+`","password":"`+password+`"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s = %d %s", email, rec.Code, rec.Body)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookie {
			return c
		}
	}
	t.Fatal("login set no session cookie")
	return nil
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not a JSON object: %v: %s", err, rec.Body)
	}
	return out
}

func TestLoginSetsAHardenedCookie(t *testing.T) {
	h, _, admin, _ := testServer(t)
	c := login(t, h, "admin@example.test", adminPassword)
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.MaxAge <= 0 {
		t.Errorf("cookie = %+v, want HttpOnly, Secure, SameSite=Lax, Path=/ and an expiry", c)
	}

	rec := request(t, h, http.MethodGet, "/api/auth/me", "", c)
	if got := decode(t, rec); rec.Code != http.StatusOK || got["id"] != admin.ID || got["role"] != "admin" {
		t.Errorf("me = %d %v", rec.Code, got)
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Errorf("me exposes a password field: %s", rec.Body)
	}
}

func TestMiddlewareEnforcesAccess(t *testing.T) {
	h, _, _, sender := testServer(t)
	senderCookie := login(t, h, "dyrektor@example.test", senderPassword)
	adminCookie := login(t, h, "admin@example.test", adminPassword)

	tests := []struct {
		name         string
		method, path string
		cookie       *http.Cookie
		want         int
		wantCode     string
	}{
		{"public route without a session", http.MethodGet, "/healthz", nil, http.StatusOK, ""},
		{"user route without a session", http.MethodPost, "/api/messages/preview", nil, http.StatusUnauthorized, "unauthenticated"},
		{"user route with a forged cookie", http.MethodPost, "/api/messages/preview", &http.Cookie{Name: SessionCookie, Value: "x"}, http.StatusUnauthorized, "unauthenticated"},
		{"user route for a sender", http.MethodPost, "/api/messages/preview", senderCookie, http.StatusOK, ""},
		{"admin route for a sender", http.MethodGet, "/api/recipients", senderCookie, http.StatusForbidden, "forbidden"},
		{"admin route for an admin", http.MethodGet, "/api/recipients", adminCookie, http.StatusOK, ""},
		{"dot segments do not dodge the admin rule", http.MethodGet, "/api/messages/../recipients", senderCookie, http.StatusForbidden, "forbidden"},
		{"user list for a sender", http.MethodGet, "/api/users", senderCookie, http.StatusForbidden, "forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := request(t, h, tt.method, tt.path, "", tt.cookie)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body)
			}
			if tt.wantCode != "" && decode(t, rec)["code"] != tt.wantCode {
				t.Errorf("body = %s, want code %s", rec.Body, tt.wantCode)
			}
		})
	}

	// The handler sees who called it.
	if rec := request(t, h, http.MethodPost, "/api/messages/preview", "", senderCookie); rec.Body.String() != sender.ID {
		t.Errorf("handler saw user %q, want the sender %q", rec.Body, sender.ID)
	}
}

func TestLoginErrors(t *testing.T) {
	h, _, _, _ := testServer(t)
	for name, tc := range map[string]struct {
		body     string
		want     int
		wantCode string
	}{
		"malformed":      {`{"email":`, http.StatusBadRequest, "invalid_request"},
		"wrong password": {`{"email":"admin@example.test","password":"zle-haslo-zle-haslo"}`, http.StatusUnauthorized, "invalid_credentials"},
	} {
		rec := request(t, h, http.MethodPost, "/api/auth/login", tc.body, nil)
		if rec.Code != tc.want || decode(t, rec)["code"] != tc.wantCode {
			t.Errorf("%s: = %d %s, want %d %s", name, rec.Code, rec.Body, tc.want, tc.wantCode)
		}
	}
	for i := 0; i < MaxFailedLogins; i++ {
		request(t, h, http.MethodPost, "/api/auth/login", `{"email":"dyrektor@example.test","password":"zle-haslo-zle-haslo"}`, nil)
	}
	rec := request(t, h, http.MethodPost, "/api/auth/login", `{"email":"dyrektor@example.test","password":"`+senderPassword+`"}`, nil)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Errorf("throttled login = %d (Retry-After %q), want 429 with Retry-After", rec.Code, rec.Header().Get("Retry-After"))
	}
}

func TestLogoutClearsTheCookie(t *testing.T) {
	h, _, _, _ := testServer(t)
	c := login(t, h, "admin@example.test", adminPassword)

	rec := request(t, h, http.MethodPost, "/api/auth/logout", "", c)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d %s", rec.Code, rec.Body)
	}
	cleared := rec.Result().Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Errorf("logout cookies = %+v, want the session cookie expired", cleared)
	}
	if rec := request(t, h, http.MethodGet, "/api/auth/me", "", c); rec.Code != http.StatusUnauthorized {
		t.Errorf("me after logout = %d, want 401", rec.Code)
	}
}

func TestUserAdministrationOverHTTP(t *testing.T) {
	h, _, admin, sender := testServer(t)
	c := login(t, h, "admin@example.test", adminPassword)

	rec := request(t, h, http.MethodPost, "/api/users", `{"email":"sekretariat@example.test","fullName":"Ewa Biurowa","role":"sender","password":"haslo-sekretariatu"}`, c)
	created := decode(t, rec)
	if rec.Code != http.StatusCreated || created["email"] != "sekretariat@example.test" || created["lastLoginAt"] != nil {
		t.Fatalf("create = %d %v", rec.Code, created)
	}

	rec = request(t, h, http.MethodPost, "/api/users", `{"email":"sekretariat@example.test","fullName":"X","role":"sender","password":"haslo-sekretariatu"}`, c)
	if rec.Code != http.StatusConflict || decode(t, rec)["code"] != "email_taken" {
		t.Errorf("duplicate create = %d %s", rec.Code, rec.Body)
	}

	rec = request(t, h, http.MethodGet, "/api/users", "", c)
	var users []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil || len(users) != 3 {
		t.Errorf("list = %d %s, want 3 users", rec.Code, rec.Body)
	}

	rec = request(t, h, http.MethodPatch, "/api/users/"+admin.ID, `{"role":"sender"}`, c)
	if rec.Code != http.StatusConflict || decode(t, rec)["code"] != "self_lockout" {
		t.Errorf("demote self = %d %s, want 409 self_lockout", rec.Code, rec.Body)
	}
	rec = request(t, h, http.MethodPatch, "/api/users/"+sender.ID, `{"disabled":true}`, c)
	if got := decode(t, rec); rec.Code != http.StatusOK || got["disabled"] != true {
		t.Errorf("disable = %d %v", rec.Code, got)
	}
	rec = request(t, h, http.MethodPut, "/api/users/"+sender.ID+"/password", `{"password":"krotkie"}`, c)
	if rec.Code != http.StatusBadRequest || decode(t, rec)["code"] != "invalid_input" {
		t.Errorf("weak password = %d %s", rec.Code, rec.Body)
	}
	rec = request(t, h, http.MethodPut, "/api/users/"+sender.ID+"/password", `{"password":"nowe-haslo-dyrektora"}`, c)
	if rec.Code != http.StatusNoContent {
		t.Errorf("reset password = %d %s", rec.Code, rec.Body)
	}

	rec = request(t, h, http.MethodGet, "/api/audit?limit=2", "", c)
	page := decode(t, rec)
	if items, _ := page["items"].([]any); rec.Code != http.StatusOK || len(items) != 2 || page["total"].(float64) < 4 {
		t.Errorf("audit = %d %v", rec.Code, page)
	}
	if rec := request(t, h, http.MethodGet, "/api/audit?limit=x", "", c); rec.Code != http.StatusBadRequest {
		t.Errorf("audit?limit=x = %d, want 400", rec.Code)
	}
}

func TestSendingConfigHasNoSecrets(t *testing.T) {
	h, _, _, _ := testServer(t)
	c := login(t, h, "admin@example.test", adminPassword)
	rec := request(t, h, http.MethodGet, "/api/admin/config", "", c)
	got := decode(t, rec)
	sms, _ := got["sms"].(map[string]any)
	if rec.Code != http.StatusOK || got["dryRun"] != true || sms["provider"] != "fake" || sms["pricePerPartMilli"] != float64(80) {
		t.Errorf("config = %d %v", rec.Code, got)
	}
	for _, key := range []string{"apiKey", "secret", "password", "token"} {
		if strings.Contains(strings.ToLower(rec.Body.String()), strings.ToLower(key)+`"`) {
			t.Errorf("config response has a %q field: %s", key, rec.Body)
		}
	}
}
