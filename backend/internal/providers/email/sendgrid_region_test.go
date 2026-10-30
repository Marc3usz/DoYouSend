package email

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

func TestResolveHost(t *testing.T) {
	tests := []struct {
		name    string
		cfg     SendGridConfig
		want    string
		wantErr bool
	}{
		{"empty region is global", SendGridConfig{}, "https://api.sendgrid.com", false},
		{"global", SendGridConfig{Region: RegionGlobal}, "https://api.sendgrid.com", false},
		{"eu", SendGridConfig{Region: RegionEU}, "https://api.eu.sendgrid.com", false},
		{"base url wins over region", SendGridConfig{Region: RegionEU, BaseURL: "http://127.0.0.1:1"}, "http://127.0.0.1:1", false},
		{"unknown region", SendGridConfig{Region: "us"}, "", true},
		{"region is case sensitive", SendGridConfig{Region: "EU"}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveHost(tc.cfg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("host = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewSendGridRejectsUnknownRegion(t *testing.T) {
	_, err := NewSendGrid(SendGridConfig{APIKey: "k", From: "no-reply@example.test", Region: "us"}, nil)
	if err == nil || !strings.Contains(err.Error(), "region") {
		t.Fatalf("err = %v, want region error", err)
	}
}

func TestNewSendGridEURegionUsesEUHostForCredits(t *testing.T) {
	sg, err := NewSendGrid(SendGridConfig{APIKey: "k", From: "no-reply@example.test", Region: RegionEU}, nil)
	if err != nil {
		t.Fatalf("NewSendGrid: %v", err)
	}
	if want := "https://api.eu.sendgrid.com/v3/user/credits"; sg.creditsURL != want {
		t.Errorf("creditsURL = %q, want %q", sg.creditsURL, want)
	}
	if sg.host != "https://api.eu.sendgrid.com" {
		t.Errorf("host = %q", sg.host)
	}
}

// The SDK's package-level client has no timeout and is shared state; the adapter
// must send through the *http.Client it was given (ADR-0014).
func TestSendUsesInjectedHTTPClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Message-Id", "abc123")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	used := false
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		used = true
		return http.DefaultTransport.RoundTrip(r)
	})}
	sg, err := NewSendGrid(SendGridConfig{APIKey: "k", From: "no-reply@example.test", BaseURL: srv.URL, HTTPClient: client}, nil)
	if err != nil {
		t.Fatalf("NewSendGrid: %v", err)
	}
	res, err := sg.Send(context.Background(), providers.Message{RecipientID: "r1", To: "jan@example.test", Subject: "s", Body: "b"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !used {
		t.Error("injected HTTP client was not used")
	}
	if res.ProviderMessageID != "abc123" {
		t.Errorf("ProviderMessageID = %q, want abc123", res.ProviderMessageID)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
