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
)

func newTestSenders(t *testing.T, h http.HandlerFunc) *SendGridSenders {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewSendGridSenders(SendGridConfig{APIKey: "SG.test-key", BaseURL: srv.URL, HTTPClient: srv.Client()}, nil)
	if err != nil {
		t.Fatalf("NewSendGridSenders: %v", err)
	}
	return c
}

func TestNewSendGridSendersRequiresKey(t *testing.T) {
	if _, err := NewSendGridSenders(SendGridConfig{}, nil); err == nil {
		t.Fatal("want error for empty API key")
	}
	if _, err := NewSendGridSenders(SendGridConfig{APIKey: "k", Region: "mars"}, nil); err == nil {
		t.Fatal("want error for unknown region")
	}
}

func TestSendGridSendersCreate(t *testing.T) {
	var got map[string]any
	c := newTestSenders(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v3/verified_senders" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if a := r.Header.Get("Authorization"); a != "Bearer SG.test-key" {
			t.Errorf("Authorization = %q", a)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1234,"nickname":"DoYouSend: Anna Nowak","from_email":"anna.nowak@example.test",
			"from_name":"Anna Nowak","reply_to":"anna.nowak@example.test","verified":false,"locked":false}`))
	})

	s, err := c.CreateSender(context.Background(), NewSenderIdentity{
		Nickname: "DoYouSend: Anna Nowak", FromEmail: "anna.nowak@example.test", FromName: "Anna Nowak",
		ReplyTo: "anna.nowak@example.test", Address: "ul. Przykładowa 1", City: "Testowo", Country: "Polska",
	})
	if err != nil {
		t.Fatalf("CreateSender: %v", err)
	}
	if s.ID != "1234" || s.Verified || s.FromEmail != "anna.nowak@example.test" {
		t.Errorf("identity = %+v", s)
	}
	for k, want := range map[string]string{
		"nickname": "DoYouSend: Anna Nowak", "from_email": "anna.nowak@example.test",
		"reply_to": "anna.nowak@example.test", "address": "ul. Przykładowa 1", "country": "Polska",
	} {
		if got[k] != want {
			t.Errorf("body[%s] = %v, want %q", k, got[k], want)
		}
	}
	if _, ok := got["zip"]; ok {
		t.Error("empty zip must be omitted")
	}
}

func TestSendGridSendersCreateTruncatesToAPILimits(t *testing.T) {
	var got map[string]string
	c := newTestSenders(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	})
	_, err := c.CreateSender(context.Background(), NewSenderIdentity{
		Nickname: strings.Repeat("ż", 150), FromEmail: "a@example.test", ReplyTo: "a@example.test", Zip: "00-000-0000-0",
	})
	if err != nil {
		t.Fatalf("CreateSender: %v", err)
	}
	if n := len([]rune(got["nickname"])); n != maxNickname {
		t.Errorf("nickname runes = %d, want %d", n, maxNickname)
	}
	if len(got["zip"]) > maxZip {
		t.Errorf("zip = %q, longer than %d", got["zip"], maxZip)
	}
}

func TestSendGridSendersErrors(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantIs     error
		wantInText string
	}{
		{"duplicate", 400, `{"errors":[{"field":"from_email","message":"already exists"}]}`, ErrSenderRejected, "from_email: already exists"},
		{"not found", 404, `{"errors":[{"message":"resource not found"}]}`, ErrSenderNotFound, "resource not found"},
		{"bad key is not the request's fault", 401, `{"errors":[{"message":"authorization required"}]}`, nil, "HTTP 401"},
		{"rate limited", 429, ``, nil, "HTTP 429"},
		{"server error", 500, `oops`, nil, "HTTP 500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestSenders(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			_, err := c.CreateSender(context.Background(), NewSenderIdentity{Nickname: "n", FromEmail: "a@example.test", ReplyTo: "a@example.test"})
			if err == nil {
				t.Fatal("want error")
			}
			for _, sentinel := range []error{ErrSenderRejected, ErrSenderNotFound} {
				wantMatch := tt.wantIs != nil && errors.Is(tt.wantIs, sentinel)
				if errors.Is(err, sentinel) != wantMatch {
					t.Errorf("errors.Is(err, %v) = %v", sentinel, errors.Is(err, sentinel))
				}
			}
			if !strings.Contains(err.Error(), tt.wantInText) {
				t.Errorf("error %q does not contain %q", err, tt.wantInText)
			}
			if strings.Contains(err.Error(), "SG.test-key") {
				t.Error("error leaks the API key")
			}
		})
	}
}

func TestSendGridSendersGetResendDelete(t *testing.T) {
	var calls []string
	c := newTestSenders(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"results":[{"id":77,"from_email":"a@example.test","verified":true}]}`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	ctx := context.Background()

	s, err := c.GetSender(ctx, "77")
	if err != nil || !s.Verified || s.ID != "77" {
		t.Fatalf("GetSender = %+v, %v", s, err)
	}
	if _, err := c.GetSender(ctx, "78"); !errors.Is(err, ErrSenderNotFound) {
		t.Errorf("GetSender(missing) err = %v, want ErrSenderNotFound", err)
	}
	if err := c.ResendVerification(ctx, "77"); err != nil {
		t.Errorf("ResendVerification: %v", err)
	}
	if err := c.DeleteSender(ctx, "77"); err != nil {
		t.Errorf("DeleteSender: %v", err)
	}
	if err := c.DeleteSender(ctx, "abc"); !errors.Is(err, ErrSenderNotFound) {
		t.Errorf("DeleteSender(bad id) err = %v, want ErrSenderNotFound", err)
	}

	want := []string{
		"GET /v3/verified_senders?id=77",
		"GET /v3/verified_senders?id=78",
		"POST /v3/verified_senders/resend/77",
		"DELETE /v3/verified_senders/77",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestFakeSenders(t *testing.T) {
	ctx := context.Background()
	f := NewFakeSenders(nil)
	f.ConfirmAfter = 1
	in := NewSenderIdentity{Nickname: "n", FromEmail: "a@example.test", ReplyTo: "a@example.test"}

	s, err := f.CreateSender(ctx, in)
	if err != nil || s.Verified {
		t.Fatalf("CreateSender = %+v, %v", s, err)
	}
	if _, err := f.CreateSender(ctx, in); !errors.Is(err, ErrSenderRejected) {
		t.Errorf("duplicate err = %v, want ErrSenderRejected", err)
	}
	if got, _ := f.GetSender(ctx, s.ID); got.Verified {
		t.Error("first check must still be pending with ConfirmAfter=1")
	}
	if got, _ := f.GetSender(ctx, s.ID); !got.Verified {
		t.Error("second check must be verified")
	}
	if err := f.DeleteSender(ctx, s.ID); err != nil {
		t.Errorf("DeleteSender: %v", err)
	}
	if err := f.ResendVerification(ctx, "77"); !errors.Is(err, ErrSenderNotFound) {
		t.Errorf("Resend(foreign id) err = %v, want ErrSenderNotFound", err)
	}

	// After an API restart the fake forgets its identities; its own IDs stay usable.
	restarted := NewFakeSenders(nil)
	if got, err := restarted.GetSender(ctx, s.ID); err != nil || !got.Verified {
		t.Errorf("GetSender after restart = %+v, %v, want verified", got, err)
	}
	if err := restarted.DeleteSender(ctx, s.ID); err != nil {
		t.Errorf("DeleteSender after restart: %v", err)
	}
}
