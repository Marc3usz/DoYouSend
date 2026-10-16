package email

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

func TestSendGrid_SendSuccess(t *testing.T) {
	t.Parallel()

	var receivedReq sgMailSend
	var receivedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.URL.Path != "/v3/mail/send" {
			t.Errorf("Path = %q, want %q", r.URL.Path, "/v3/mail/send")
		}

		receivedAuth = r.Header.Get("Authorization")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &receivedReq); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}

		w.Header().Set("X-Message-Id", "sg-msg-12345")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	// Use RFC 5322 display name to test proper parsing into Email and Name fields.
	sg, err := NewSendGrid(SendGridConfig{
		APIKey:     "test-key-abc",
		From:       "Szkola Testowa <no-reply@example.test>",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	}, nil)
	if err != nil {
		t.Fatalf("unexpected NewSendGrid error: %v", err)
	}

	msg := providers.Message{
		RecipientID: "rec-001",
		To:          "recipient@example.test",
		Subject:     "Wazne ogloszenie",
		Body:        "Tresc wiadomosci identyczna z SMS.",
	}

	res, err := sg.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected Send error: %v", err)
	}

	if res.ProviderMessageID != "sg-msg-12345" {
		t.Errorf("ProviderMessageID = %q, want %q", res.ProviderMessageID, "sg-msg-12345")
	}

	// Verify authorization header
	if receivedAuth != "Bearer test-key-abc" {
		t.Errorf("Authorization = %q, want %q", receivedAuth, "Bearer test-key-abc")
	}

	// Verify recipient and sender
	if len(receivedReq.Personalizations) != 1 || len(receivedReq.Personalizations[0].To) != 1 {
		t.Fatalf("unexpected personalizations count: %+v", receivedReq.Personalizations)
	}
	if gotTo := receivedReq.Personalizations[0].To[0].Email; gotTo != "recipient@example.test" {
		t.Errorf("To = %q, want %q", gotTo, "recipient@example.test")
	}
	if receivedReq.From.Email != "no-reply@example.test" {
		t.Errorf("From.Email = %q, want %q", receivedReq.From.Email, "no-reply@example.test")
	}
	if receivedReq.From.Name != "Szkola Testowa" {
		t.Errorf("From.Name = %q, want %q", receivedReq.From.Name, "Szkola Testowa")
	}
	if receivedReq.Subject != "Wazne ogloszenie" {
		t.Errorf("Subject = %q, want %q", receivedReq.Subject, "Wazne ogloszenie")
	}

	// Verify content is plain text
	if len(receivedReq.Content) != 1 {
		t.Fatalf("Content count = %d, want 1", len(receivedReq.Content))
	}
	if receivedReq.Content[0].Type != "text/plain" {
		t.Errorf("Content.Type = %q, want text/plain", receivedReq.Content[0].Type)
	}
	if receivedReq.Content[0].Value != msg.Body {
		t.Errorf("Content.Value = %q, want %q", receivedReq.Content[0].Value, msg.Body)
	}

	// Invariant 4: click tracking and open tracking MUST be disabled
	if receivedReq.TrackingSettings.ClickTracking.Enable {
		t.Error("click_tracking must be disabled to preserve link identicality")
	}
	if receivedReq.TrackingSettings.OpenTracking.Enable {
		t.Error("open_tracking must be disabled")
	}

	// Verify custom args
	if receivedReq.CustomArgs["recipient_id"] != "rec-001" {
		t.Errorf("custom_args[recipient_id] = %q, want %q",
			receivedReq.CustomArgs["recipient_id"], "rec-001")
	}
}

func TestSendGrid_SandboxMode(t *testing.T) {
	t.Parallel()

	var receivedReq sgMailSend

	// SendGrid returns 200 OK without X-Message-Id header in sandbox mode.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedReq)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sg, err := NewSendGrid(SendGridConfig{
		APIKey:     "test-key",
		From:       "sender@example.test",
		Sandbox:    true,
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	}, nil)
	if err != nil {
		t.Fatalf("unexpected NewSendGrid error: %v", err)
	}

	res, err := sg.Send(context.Background(), providers.Message{
		RecipientID: "rec-002",
		To:          "sandbox@example.test",
		Subject:     "Test",
		Body:        "Body",
	})
	if err != nil {
		t.Fatalf("unexpected Send error: %v", err)
	}

	if receivedReq.MailSettings == nil || !receivedReq.MailSettings.SandboxMode.Enable {
		t.Error("mail_settings.sandbox_mode.enable should be true")
	}

	// Verify synthetic message ID was generated so Result has non-empty ID.
	if res.ProviderMessageID == "" {
		t.Error("expected non-empty synthetic ProviderMessageID in sandbox mode")
	}
	if !strings.HasPrefix(res.ProviderMessageID, "sandbox-") {
		t.Errorf("ProviderMessageID = %q, expected prefix 'sandbox-'", res.ProviderMessageID)
	}
}

func TestSendGrid_NewSendGrid_InvalidFrom(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		from string
	}{
		{"empty From", ""},
		{"malformed From", "no-at-sign"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewSendGrid(SendGridConfig{
				APIKey: "test-key",
				From:   tc.from,
			}, nil)
			if err == nil {
				t.Fatalf("expected error for invalid From %q, got nil", tc.from)
			}
		})
	}
}

func TestSendGrid_InvalidRecipient(t *testing.T) {
	t.Parallel()

	sg, err := NewSendGrid(SendGridConfig{APIKey: "key", From: "test@example.test"}, nil)
	if err != nil {
		t.Fatalf("unexpected NewSendGrid error: %v", err)
	}

	cases := []struct {
		name string
		to   string
	}{
		{name: "empty To", to: ""},
		{name: "whitespace To", to: "   "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := sg.Send(context.Background(), providers.Message{
				RecipientID: "rec-003",
				To:          tc.to,
			})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !providers.IsPermanent(err) {
				t.Errorf("error = %v, want permanent error", err)
			}
			if !errors.Is(err, providers.ErrInvalidRecipient) {
				t.Errorf("error = %v, want ErrInvalidRecipient", err)
			}
		})
	}
}

func TestSendGrid_ErrorClassification(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		statusCode     int
		responseHeader http.Header
		responseBody   string
		wantPermanent  bool
		wantTransient  bool
		wantSubstring  string
	}{
		{
			name:          "400 Bad Request is permanent",
			statusCode:    http.StatusBadRequest,
			responseBody:  `{"errors":[{"message":"The from address does not match"}]}`,
			wantPermanent: true,
		},
		{
			name:          "401 Unauthorized is permanent",
			statusCode:    http.StatusUnauthorized,
			responseBody:  `{"errors":[{"message":"The provided authorization grant is invalid"}]}`,
			wantPermanent: true,
		},
		{
			name:          "403 Forbidden is permanent",
			statusCode:    http.StatusForbidden,
			responseBody:  `{"errors":[{"message":"Access denied"}]}`,
			wantPermanent: true,
		},
		{
			name:          "404 Not Found is permanent",
			statusCode:    http.StatusNotFound,
			responseBody:  `{"errors":[{"message":"Endpoint not found"}]}`,
			wantPermanent: true,
		},
		{
			name:          "405 Method Not Allowed is permanent",
			statusCode:    http.StatusMethodNotAllowed,
			responseBody:  `{"errors":[{"message":"Method not allowed"}]}`,
			wantPermanent: true,
		},
		{
			name:          "413 Request Entity Too Large is permanent",
			statusCode:    http.StatusRequestEntityTooLarge,
			responseBody:  `{"errors":[{"message":"Payload too large"}]}`,
			wantPermanent: true,
		},
		{
			name:          "422 Unprocessable Entity is permanent",
			statusCode:    http.StatusUnprocessableEntity,
			responseBody:  `{"errors":[{"message":"Unprocessable entity"}]}`,
			wantPermanent: true,
		},
		{
			name:           "429 Too Many Requests with Retry-After is transient",
			statusCode:     http.StatusTooManyRequests,
			responseHeader: http.Header{"Retry-After": []string{"60"}},
			responseBody:   `{"errors":[{"message":"Rate limit exceeded"}]}`,
			wantTransient:  true,
			wantSubstring:  "Retry-After: 60",
		},
		{
			name:          "500 Internal Server Error is transient",
			statusCode:    http.StatusInternalServerError,
			responseBody:  `{"errors":[{"message":"Internal error"}]}`,
			wantTransient: true,
		},
		{
			name:          "503 Service Unavailable is transient",
			statusCode:    http.StatusServiceUnavailable,
			responseBody:  `{"errors":[{"message":"Service unavailable"}]}`,
			wantTransient: true,
		},
		{
			name:          "301 Moved Permanently is permanent",
			statusCode:    http.StatusMovedPermanently,
			responseBody:  `Moved`,
			wantPermanent: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for k, vs := range tc.responseHeader {
					for _, v := range vs {
						w.Header().Add(k, v)
					}
				}
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.responseBody))
			}))
			defer srv.Close()

			sg, err := NewSendGrid(SendGridConfig{
				APIKey:     "test-key",
				From:       "sender@example.test",
				BaseURL:    srv.URL,
				HTTPClient: srv.Client(),
			}, nil)
			if err != nil {
				t.Fatalf("unexpected NewSendGrid error: %v", err)
			}

			_, sendErr := sg.Send(context.Background(), providers.Message{
				RecipientID: "rec-err",
				To:          "user@example.test",
				Subject:     "Subject",
				Body:        "Body",
			})
			if sendErr == nil {
				t.Fatalf("expected error for HTTP %d, got nil", tc.statusCode)
			}

			if tc.wantPermanent && !providers.IsPermanent(sendErr) {
				t.Errorf("HTTP %d: error = %v, want permanent", tc.statusCode, sendErr)
			}
			if tc.wantTransient && !providers.IsTransient(sendErr) {
				t.Errorf("HTTP %d: error = %v, want transient", tc.statusCode, sendErr)
			}
			if tc.wantSubstring != "" && !strings.Contains(sendErr.Error(), tc.wantSubstring) {
				t.Errorf("HTTP %d: error = %v, want substring %q", tc.statusCode, sendErr, tc.wantSubstring)
			}
		})
	}
}

func TestSendGrid_ContextCancelled(t *testing.T) {
	t.Parallel()

	sg, err := NewSendGrid(SendGridConfig{APIKey: "key", From: "sender@example.test"}, nil)
	if err != nil {
		t.Fatalf("unexpected NewSendGrid error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled immediately

	_, err = sg.Send(ctx, providers.Message{
		RecipientID: "rec-cancel",
		To:          "user@example.test",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if !providers.IsTransient(err) {
		t.Errorf("error = %v, want transient error", err)
	}
}

func TestSendGrid_Channel(t *testing.T) {
	t.Parallel()

	sg, err := NewSendGrid(SendGridConfig{APIKey: "key", From: "sender@example.test"}, nil)
	if err != nil {
		t.Fatalf("unexpected NewSendGrid error: %v", err)
	}
	if sg.Channel() != providers.ChannelEmail {
		t.Errorf("Channel() = %v, want %v", sg.Channel(), providers.ChannelEmail)
	}
}

func TestSendGrid_GetProfile_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Method = %q, want %q", r.Method, http.MethodGet)
		}
		if r.URL.Path != "/v3/user/credits" {
			t.Errorf("Path = %q, want %q", r.URL.Path, "/v3/user/credits")
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-api-key" {
			t.Errorf("Authorization = %q, want Bearer test-api-key", auth)
		}
		if accept := r.Header.Get("Accept"); accept != "application/json" {
			t.Errorf("Accept = %q, want application/json", accept)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"remain": 95,
			"total": 100,
			"overage": 0,
			"used": 5,
			"reset_frequency": "day",
			"next_reset": "2026-10-17"
		}`))
	}))
	defer srv.Close()

	sg, err := NewSendGrid(SendGridConfig{
		APIKey:     "test-api-key",
		From:       "sender@example.test",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	}, nil)
	if err != nil {
		t.Fatalf("unexpected NewSendGrid error: %v", err)
	}

	profile, err := sg.GetProfile(context.Background())
	if err != nil {
		t.Fatalf("unexpected GetProfile error: %v", err)
	}

	if profile.Provider != "sendgrid" {
		t.Errorf("Provider = %q, want 'sendgrid'", profile.Provider)
	}
	if profile.RemainCredits == nil || *profile.RemainCredits != 95 {
		t.Errorf("RemainCredits = %v, want 95", profile.RemainCredits)
	}
	if profile.TotalCredits == nil || *profile.TotalCredits != 100 {
		t.Errorf("TotalCredits = %v, want 100", profile.TotalCredits)
	}
	if profile.UsedCredits == nil || *profile.UsedCredits != 5 {
		t.Errorf("UsedCredits = %v, want 5", profile.UsedCredits)
	}
	if profile.ResetFrequency != "day" {
		t.Errorf("ResetFrequency = %q, want 'day'", profile.ResetFrequency)
	}
	if profile.NextReset != "2026-10-17" {
		t.Errorf("NextReset = %q, want '2026-10-17'", profile.NextReset)
	}
}

func TestSendGrid_GetProfile_APIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{
			"errors": [
				{"message": "The provided authorization grant is invalid, expired, or revoked", "field": null}
			]
		}`))
	}))
	defer srv.Close()

	sg, err := NewSendGrid(SendGridConfig{
		APIKey:     "invalid-key",
		From:       "sender@example.test",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	}, nil)
	if err != nil {
		t.Fatalf("unexpected NewSendGrid error: %v", err)
	}

	_, err = sg.GetProfile(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "The provided authorization grant is invalid") {
		t.Errorf("error = %v, want containing API error message", err)
	}
}

func TestSendGrid_GetProfile_5xxNoJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("Bad Gateway"))
	}))
	defer srv.Close()

	sg, err := NewSendGrid(SendGridConfig{
		APIKey:     "test-key",
		From:       "sender@example.test",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	}, nil)
	if err != nil {
		t.Fatalf("unexpected NewSendGrid error: %v", err)
	}

	_, err = sg.GetProfile(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("error = %v, want mentioning status 502", err)
	}
}

func TestSendGrid_GetProfile_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"remain": `))
	}))
	defer srv.Close()

	sg, err := NewSendGrid(SendGridConfig{
		APIKey:     "test-key",
		From:       "sender@example.test",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	}, nil)
	if err != nil {
		t.Fatalf("unexpected NewSendGrid error: %v", err)
	}

	_, err = sg.GetProfile(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "decode response") {
		t.Errorf("error = %v, want containing 'decode response'", err)
	}
}

func TestSendGrid_GetProfile_MissingRemain(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"total": 100, "used": 0}`))
	}))
	defer srv.Close()

	sg, err := NewSendGrid(SendGridConfig{
		APIKey:     "test-key",
		From:       "sender@example.test",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	}, nil)
	if err != nil {
		t.Fatalf("unexpected NewSendGrid error: %v", err)
	}

	_, err = sg.GetProfile(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "missing remain field") {
		t.Errorf("error = %v, want containing 'missing remain field'", err)
	}
}

func TestSendGrid_GetProfile_ContextCancelled(t *testing.T) {
	t.Parallel()

	sg, err := NewSendGrid(SendGridConfig{
		APIKey: "test-key",
		From:   "sender@example.test",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected NewSendGrid error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = sg.GetProfile(ctx)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}
