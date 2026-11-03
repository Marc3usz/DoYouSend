package email_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers/email"
)

type mockProfiler struct {
	profile email.Profile
	err     error
}

func (m *mockProfiler) GetProfile(_ context.Context) (email.Profile, error) {
	if m.err != nil {
		return email.Profile{}, m.err
	}
	return m.profile, nil
}

func TestHandleProfile_MethodNotAllowed(t *testing.T) {
	handler := email.HandleProfile(&mockProfiler{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/providers/email/profile", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestHandleProfile_NilProfiler(t *testing.T) {
	handler := email.HandleProfile(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/providers/email/profile", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected status %d, got %d", http.StatusNotImplemented, rec.Code)
	}
}

func TestHandleProfile_ProviderError(t *testing.T) {
	mock := &mockProfiler{err: errors.New("sendgrid timeout")}
	handler := email.HandleProfile(mock, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/providers/email/profile", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, rec.Code)
	}
}

func TestHandleProfile_Success(t *testing.T) {
	remain := int64(85)
	total := int64(100)
	used := int64(15)
	mock := &mockProfiler{
		profile: email.Profile{
			Provider:       "sendgrid",
			RemainCredits:  &remain,
			TotalCredits:   &total,
			UsedCredits:    &used,
			ResetFrequency: "day",
			NextReset:      "2026-10-18",
		},
	}
	handler := email.HandleProfile(mock, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/providers/email/profile", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var resp email.ProfileResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Provider != "sendgrid" {
		t.Errorf("expected provider 'sendgrid', got %q", resp.Provider)
	}
	if resp.RemainCredits == nil || *resp.RemainCredits != 85 {
		t.Errorf("expected remainCredits 85, got %v", resp.RemainCredits)
	}
	if resp.TotalCredits == nil || *resp.TotalCredits != 100 {
		t.Errorf("expected totalCredits 100, got %v", resp.TotalCredits)
	}
	if resp.UsedCredits == nil || *resp.UsedCredits != 15 {
		t.Errorf("expected usedCredits 15, got %v", resp.UsedCredits)
	}
	if resp.ResetFrequency != "day" {
		t.Errorf("expected resetFrequency 'day', got %q", resp.ResetFrequency)
	}
	if resp.NextReset != "2026-10-18" {
		t.Errorf("expected nextReset '2026-10-18', got %q", resp.NextReset)
	}
}

func TestHandleProfile_Mailpit(t *testing.T) {
	mailpit := email.NewMailpit(email.MailpitConfig{
		Host: "localhost",
		Port: "1025",
	}, nil)
	handler := email.HandleProfile(mailpit, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/providers/email/profile", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var resp email.ProfileResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Provider != "mailpit" {
		t.Errorf("expected provider 'mailpit', got %q", resp.Provider)
	}
	if resp.RemainCredits != nil {
		t.Errorf("expected nil remainCredits for mailpit, got %v", resp.RemainCredits)
	}
}
