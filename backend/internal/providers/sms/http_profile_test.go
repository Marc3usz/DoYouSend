package sms_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers/sms"
)

type mockProfiler struct {
	profile sms.Profile
	err     error
}

func (m *mockProfiler) GetProfile(ctx context.Context) (sms.Profile, error) {
	if m.err != nil {
		return sms.Profile{}, m.err
	}
	return m.profile, nil
}

func TestHandleProfile_MethodNotAllowed(t *testing.T) {
	handler := sms.HandleProfile(&mockProfiler{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/providers/sms/profile", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestHandleProfile_NilProfiler(t *testing.T) {
	handler := sms.HandleProfile(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/providers/sms/profile", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected status %d, got %d", http.StatusNotImplemented, rec.Code)
	}
}

func TestHandleProfile_ProviderError(t *testing.T) {
	mock := &mockProfiler{err: errors.New("upstream timeout")}
	handler := sms.HandleProfile(mock, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/providers/sms/profile", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, rec.Code)
	}
}

func TestHandleProfile_Success(t *testing.T) {
	mock := &mockProfiler{
		profile: sms.Profile{
			Points:   125.5,
			Name:     "Test Account",
			UserType: "developer",
		},
	}
	handler := sms.HandleProfile(mock, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/providers/sms/profile", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var resp sms.ProfileResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Points != 125.5 {
		t.Errorf("expected points 125.5, got %f", resp.Points)
	}
	if resp.Name != "Test Account" {
		t.Errorf("expected name 'Test Account', got %q", resp.Name)
	}
	if resp.UserType != "developer" {
		t.Errorf("expected userType 'developer', got %q", resp.UserType)
	}
}

func TestHandleProfile_FakeSMS(t *testing.T) {
	fake := sms.NewFake(nil)
	handler := sms.HandleProfile(fake, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/providers/sms/profile", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var resp sms.ProfileResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Points != 500.0 {
		t.Errorf("expected default fake points 500.0, got %f", resp.Points)
	}
	if resp.Name != "Fake SMS Sandbox" {
		t.Errorf("expected fake name 'Fake SMS Sandbox', got %q", resp.Name)
	}
	if resp.UserType != "developer" {
		t.Errorf("expected userType 'developer', got %q", resp.UserType)
	}
}
