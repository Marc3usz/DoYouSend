package sms

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

func TestSMSAPI_SendSuccess(t *testing.T) {
	t.Parallel()

	var receivedAuth string
	var receivedContentType string
	var receivedForm url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.URL.Path != "/sms.do" {
			t.Errorf("Path = %q, want %q", r.URL.Path, "/sms.do")
		}

		receivedAuth = r.Header.Get("Authorization")
		receivedContentType = r.Header.Get("Content-Type")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse form: %v", err)
		}
		receivedForm = form

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"count": 1,
			"list": [
				{
					"id": "1631525653123456789",
					"points": 0.08,
					"number": "48500100101",
					"date_sent": 1631525653,
					"submitted_number": "48500100101",
					"status": "QUEUE"
				}
			]
		}`))
	}))
	defer srv.Close()

	p, err := NewSMSAPI(SMSAPIConfig{
		APIKey:     "test-token-123",
		SenderName: "SZKOLA-TEST",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	}, nil)
	if err != nil {
		t.Fatalf("unexpected NewSMSAPI error: %v", err)
	}

	msg := providers.Message{
		RecipientID: "rec-001",
		To:          "+48500100101",
		Body:        "Witaj w szkole!",
	}

	res, err := p.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected Send error: %v", err)
	}

	if res.ProviderMessageID != "1631525653123456789" {
		t.Errorf("ProviderMessageID = %q, want %q", res.ProviderMessageID, "1631525653123456789")
	}

	if receivedAuth != "Bearer test-token-123" {
		t.Errorf("Authorization = %q, want %q", receivedAuth, "Bearer test-token-123")
	}
	if !strings.HasPrefix(receivedContentType, "application/x-www-form-urlencoded") {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", receivedContentType)
	}

	// Verify form fields
	if gotTo := receivedForm.Get("to"); gotTo != "48500100101" {
		t.Errorf("form[to] = %q, want 48500100101", gotTo)
	}
	if gotMsg := receivedForm.Get("message"); gotMsg != "Witaj w szkole!" {
		t.Errorf("form[message] = %q, want 'Witaj w szkole!'", gotMsg)
	}
	if gotFrom := receivedForm.Get("from"); gotFrom != "SZKOLA-TEST" {
		t.Errorf("form[from] = %q, want SZKOLA-TEST", gotFrom)
	}
	if gotFormat := receivedForm.Get("format"); gotFormat != "json" {
		t.Errorf("form[format] = %q, want json", gotFormat)
	}
	if gotParam1 := receivedForm.Get("param1"); gotParam1 != "rec-001" {
		t.Errorf("form[param1] = %q, want rec-001", gotParam1)
	}
	// Verify details=1 is NOT passed to avoid echoing sensitive data back in response
	if receivedForm.Has("details") {
		t.Errorf("form should not set details to avoid echoing sensitive data")
	}
}

func TestSMSAPI_MaxParts(t *testing.T) {
	t.Parallel()

	var receivedForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedForm, _ = url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"count": 1, "list": [{"id": "msg-parts", "status": "QUEUE"}]}`))
	}))
	defer srv.Close()

	p, err := NewSMSAPI(SMSAPIConfig{
		APIKey:     "test-key",
		BaseURL:    srv.URL,
		MaxParts:   4,
		HTTPClient: srv.Client(),
	}, nil)
	if err != nil {
		t.Fatalf("unexpected NewSMSAPI error: %v", err)
	}

	_, err = p.Send(context.Background(), providers.Message{
		RecipientID: "rec-parts",
		To:          "48500100101",
		Body:        "Długa treść",
	})
	if err != nil {
		t.Fatalf("unexpected Send error: %v", err)
	}

	if got := receivedForm.Get("max_parts"); got != "4" {
		t.Errorf("form[max_parts] = %q, want 4", got)
	}
}

func TestSMSAPI_TestMode(t *testing.T) {
	t.Parallel()

	var receivedForm url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedForm, _ = url.ParseQuery(string(body))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"count": 1,
			"list": [
				{
					"id": "test-msg-id-123",
					"points": 0.08,
					"status": "QUEUE"
				}
			]
		}`))
	}))
	defer srv.Close()

	p, err := NewSMSAPI(SMSAPIConfig{
		APIKey:   "test-token",
		TestMode: true,
		BaseURL:  srv.URL,
	}, nil)
	if err != nil {
		t.Fatalf("unexpected NewSMSAPI error: %v", err)
	}

	res, err := p.Send(context.Background(), providers.Message{
		RecipientID: "rec-test",
		To:          "48500100101",
		Body:        "Test body",
	})
	if err != nil {
		t.Fatalf("unexpected Send error: %v", err)
	}

	if receivedForm.Get("test") != "1" {
		t.Errorf("form[test] = %q, want '1'", receivedForm.Get("test"))
	}
	if res.ProviderMessageID != "test-msg-id-123" {
		t.Errorf("ProviderMessageID = %q, want test-msg-id-123", res.ProviderMessageID)
	}
}

func TestSMSAPI_NewSMSAPI_MissingAPIKey(t *testing.T) {
	t.Parallel()

	_, err := NewSMSAPI(SMSAPIConfig{APIKey: ""}, nil)
	if err == nil {
		t.Fatal("expected error when APIKey is empty, got nil")
	}
}

func TestSMSAPI_InvalidRecipient(t *testing.T) {
	t.Parallel()

	p, err := NewSMSAPI(SMSAPIConfig{APIKey: "key"}, nil)
	if err != nil {
		t.Fatalf("unexpected NewSMSAPI error: %v", err)
	}

	cases := []struct {
		name string
		to   string
	}{
		{"empty To", ""},
		{"whitespace To", "   "},
		{"newline in To", "48500100101\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := p.Send(context.Background(), providers.Message{
				RecipientID: "rec-002",
				To:          tc.to,
				Body:        "Body",
			})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !providers.IsPermanent(err) {
				t.Errorf("error = %v, want permanent error", err)
			}
		})
	}
}

func TestSMSAPI_ErrorClassification_JSON(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		httpStatus    int
		errorCode     int
		errorMsg      string
		wantPermanent bool
		wantTransient bool
	}{
		// Permanent errors tested with HTTP 200 (SMSAPI returns error JSON even with 200 OK)
		{
			name:          "Error 11 message too long or empty with HTTP 200 is permanent",
			httpStatus:    http.StatusOK,
			errorCode:     11,
			errorMsg:      "Message too long or empty",
			wantPermanent: true,
		},
		{
			name:          "Error 12 max parts exceeded with HTTP 200 is permanent",
			httpStatus:    http.StatusOK,
			errorCode:     12,
			errorMsg:      "Exceeded max_parts limit",
			wantPermanent: true,
		},
		{
			name:          "Error 13 invalid recipient or blacklisted with HTTP 200 is permanent",
			httpStatus:    http.StatusOK,
			errorCode:     13,
			errorMsg:      "No correct phone numbers",
			wantPermanent: true,
		},
		{
			name:          "Error 14 invalid sender name with HTTP 400 is permanent",
			httpStatus:    http.StatusBadRequest,
			errorCode:     14,
			errorMsg:      "Wrong sender name",
			wantPermanent: true,
		},
		{
			name:          "Error 18 invalid params count with HTTP 400 is permanent",
			httpStatus:    http.StatusBadRequest,
			errorCode:     18,
			errorMsg:      "Invalid number of parameters",
			wantPermanent: true,
		},
		{
			name:          "Error 26 subject too long with HTTP 400 is permanent",
			httpStatus:    http.StatusBadRequest,
			errorCode:     26,
			errorMsg:      "Subject too long",
			wantPermanent: true,
		},
		{
			name:          "Error 101 invalid OAuth token with HTTP 401 is permanent",
			httpStatus:    http.StatusUnauthorized,
			errorCode:     101,
			errorMsg:      "Invalid authorization info",
			wantPermanent: true,
		},
		{
			name:          "Error 102 invalid credentials with HTTP 401 is permanent",
			httpStatus:    http.StatusUnauthorized,
			errorCode:     102,
			errorMsg:      "Invalid login or password",
			wantPermanent: true,
		},
		{
			name:          "Error 105 IP filter whitelist error with HTTP 200 is permanent",
			httpStatus:    http.StatusOK,
			errorCode:     105,
			errorMsg:      "Wrong IP address",
			wantPermanent: true,
		},
		{
			name:          "Unknown error code 9999 with HTTP 200 defaults to permanent",
			httpStatus:    http.StatusOK,
			errorCode:     9999,
			errorMsg:      "Some future unlisted error",
			wantPermanent: true,
		},
		// Transient errors
		{
			name:          "Error 8 system reference error with HTTP 200 is transient",
			httpStatus:    http.StatusOK,
			errorCode:     8,
			errorMsg:      "Error in reference",
			wantTransient: true,
		},
		{
			name:          "Error 103 insufficient points with HTTP 200 is transient",
			httpStatus:    http.StatusOK,
			errorCode:     103,
			errorMsg:      "Insufficient points on account",
			wantTransient: true,
		},
		{
			name:          "Error 201 internal system error with HTTP 500 is transient",
			httpStatus:    http.StatusInternalServerError,
			errorCode:     201,
			errorMsg:      "Internal system error",
			wantTransient: true,
		},
		{
			name:          "Error 202 rate limit / too many requests with HTTP 200 is transient",
			httpStatus:    http.StatusOK,
			errorCode:     202,
			errorMsg:      "Too many simultaneous requests",
			wantTransient: true,
		},
		{
			name:          "Error 999 internal system error with HTTP 500 is transient",
			httpStatus:    http.StatusInternalServerError,
			errorCode:     999,
			errorMsg:      "Internal system error",
			wantTransient: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jsonPayload, err := json.Marshal(map[string]any{
				"error":   tc.errorCode,
				"message": tc.errorMsg,
			})
			if err != nil {
				t.Fatalf("marshal error JSON: %v", err)
			}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.httpStatus)
				_, _ = w.Write(jsonPayload)
			}))
			defer srv.Close()

			p, err := NewSMSAPI(SMSAPIConfig{
				APIKey:     "test-key",
				BaseURL:    srv.URL,
				HTTPClient: srv.Client(),
			}, nil)
			if err != nil {
				t.Fatalf("unexpected NewSMSAPI error: %v", err)
			}

			_, sendErr := p.Send(context.Background(), providers.Message{
				RecipientID: "rec-test",
				To:          "48500100101",
				Body:        "Wiadomość testowa",
			})
			if sendErr == nil {
				t.Fatalf("expected error from Send, got nil")
			}

			if tc.wantPermanent && !providers.IsPermanent(sendErr) {
				t.Errorf("Send() error = %v, want permanent", sendErr)
			}
			if tc.wantTransient && !providers.IsTransient(sendErr) {
				t.Errorf("Send() error = %v, want transient", sendErr)
			}

			wantSub := fmt.Sprintf("smsapi error %d: %s", tc.errorCode, tc.errorMsg)
			if !strings.Contains(sendErr.Error(), wantSub) {
				t.Errorf("Send() error = %v, want substring %q", sendErr, wantSub)
			}

			// Invariant: errors must never leak phone numbers or message body
			if strings.Contains(sendErr.Error(), "48500100101") || strings.Contains(sendErr.Error(), "Wiadomość testowa") {
				t.Errorf("Send() error leaked sensitive content: %v", sendErr)
			}
		})
	}
}

func TestSMSAPI_ErrorClassification_HTTPStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		statusCode    int
		header        http.Header
		body          string
		wantPermanent bool
		wantTransient bool
		wantSubstring string
	}{
		{
			name:          "400 Bad Request is permanent",
			statusCode:    http.StatusBadRequest,
			body:          "Bad Request",
			wantPermanent: true,
		},
		{
			name:          "401 Unauthorized is permanent",
			statusCode:    http.StatusUnauthorized,
			body:          "Unauthorized",
			wantPermanent: true,
		},
		{
			name:          "403 Forbidden is permanent",
			statusCode:    http.StatusForbidden,
			body:          "Forbidden",
			wantPermanent: true,
		},
		{
			name:          "429 Too Many Requests with Retry-After is transient",
			statusCode:    http.StatusTooManyRequests,
			header:        http.Header{"Retry-After": []string{"30"}},
			body:          "Too Many Requests",
			wantTransient: true,
			wantSubstring: "Retry-After: 30",
		},
		{
			name:          "500 Internal Server Error is transient",
			statusCode:    http.StatusInternalServerError,
			body:          "Internal Error",
			wantTransient: true,
		},
		{
			name:          "503 Service Unavailable is transient",
			statusCode:    http.StatusServiceUnavailable,
			body:          "Service Unavailable",
			wantTransient: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for k, vs := range tc.header {
					for _, v := range vs {
						w.Header().Add(k, v)
					}
				}
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			p, err := NewSMSAPI(SMSAPIConfig{
				APIKey:     "test-key",
				BaseURL:    srv.URL,
				HTTPClient: srv.Client(),
			}, nil)
			if err != nil {
				t.Fatalf("unexpected NewSMSAPI error: %v", err)
			}

			_, sendErr := p.Send(context.Background(), providers.Message{
				RecipientID: "rec-err",
				To:          "48500100101",
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
			// Invariant: errors must not leak raw response bodies
			if tc.body != "" && strings.Contains(sendErr.Error(), tc.body) {
				t.Errorf("HTTP %d: error leaked body %q: %v", tc.statusCode, tc.body, sendErr)
			}
		})
	}
}

func TestSMSAPI_ContextCancelled(t *testing.T) {
	t.Parallel()

	p, err := NewSMSAPI(SMSAPIConfig{APIKey: "key"}, nil)
	if err != nil {
		t.Fatalf("unexpected NewSMSAPI error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled immediately

	_, err = p.Send(ctx, providers.Message{
		RecipientID: "rec-cancel",
		To:          "48500100101",
		Body:        "Body",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if !providers.IsTransient(err) {
		t.Errorf("error = %v, want transient error", err)
	}
}

func TestSMSAPI_Channel(t *testing.T) {
	t.Parallel()

	p, err := NewSMSAPI(SMSAPIConfig{APIKey: "key"}, nil)
	if err != nil {
		t.Fatalf("unexpected NewSMSAPI error: %v", err)
	}
	if p.Channel() != providers.ChannelSMS {
		t.Errorf("Channel() = %v, want %v", p.Channel(), providers.ChannelSMS)
	}
}
