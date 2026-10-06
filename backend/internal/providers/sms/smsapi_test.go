package sms

import (
	"context"
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
		errorCode     int
		errorMsg      string
		wantPermanent bool
		wantTransient bool
	}{
		{
			name:          "Error 11 invalid login is permanent",
			errorCode:     11,
			errorMsg:      "Message not sent, invalid login credentials",
			wantPermanent: true,
		},
		{
			name:          "Error 13 invalid recipient number is permanent",
			errorCode:     13,
			errorMsg:      "No correct phone numbers",
			wantPermanent: true,
		},
		{
			name:          "Error 14 invalid sender name is permanent",
			errorCode:     14,
			errorMsg:      "Wrong sender name",
			wantPermanent: true,
		},
		{
			name:          "Error 18 empty message is permanent",
			errorCode:     18,
			errorMsg:      "Message text is empty",
			wantPermanent: true,
		},
		{
			name:          "Error 101 invalid param is permanent",
			errorCode:     101,
			errorMsg:      "Invalid param",
			wantPermanent: true,
		},
		{
			name:          "Error 103 insufficient points is transient",
			errorCode:     103,
			errorMsg:      "Insufficient points on account",
			wantTransient: true,
		},
		{
			name:          "Error 105 rate limit is transient",
			errorCode:     105,
			errorMsg:      "Rate limit exceeded",
			wantTransient: true,
		},
		{
			name:          "Error 201 internal system error is transient",
			errorCode:     201,
			errorMsg:      "Internal system error",
			wantTransient: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(
					`{"error":` + string(rune('0'+tc.errorCode/100)) +
						`,"message":"` + tc.errorMsg + `"}`))
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

			// Direct classification test
			classErr := classifySMSAPIError("rec-test", tc.errorCode, tc.errorMsg, http.StatusBadRequest)
			if tc.wantPermanent && !providers.IsPermanent(classErr) {
				t.Errorf("classifySMSAPIError(%d) = %v, want permanent", tc.errorCode, classErr)
			}
			if tc.wantTransient && !providers.IsTransient(classErr) {
				t.Errorf("classifySMSAPIError(%d) = %v, want transient", tc.errorCode, classErr)
			}
			_ = p
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
