package sms

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// TestAPI_AmbiguousOutcomesAreNotTransient: once SMSAPI may have received the
// request, a failure must not be retried automatically (the SMS could arrive
// twice). Only a connection that never opened is transient.
func TestAPI_AmbiguousOutcomesAreNotTransient(t *testing.T) {
	t.Parallel()

	msg := providers.Message{RecipientID: "r-1", To: "+48500100101", Body: "Tresc"}

	t.Run("connection refused is transient", func(t *testing.T) {
		t.Parallel()
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := l.Addr().String()
		_ = l.Close() // nothing listens there any more
		p, err := NewAPI(APIConfig{APIKey: "k", BaseURL: "http://" + addr}, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Send(context.Background(), msg)
		if !providers.IsTransient(err) {
			t.Errorf("err = %v, want transient", err)
		}
	})

	t.Run("timeout after the request was sent is unclassified", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(300 * time.Millisecond)
		}))
		defer srv.Close()
		client := srv.Client()
		client.Timeout = 50 * time.Millisecond
		p, err := NewAPI(APIConfig{APIKey: "k", BaseURL: srv.URL, HTTPClient: client}, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Send(context.Background(), msg)
		if err == nil || providers.IsTransient(err) || providers.IsPermanent(err) {
			t.Errorf("err = %v, want an unclassified error", err)
		}
	})

	t.Run("unreadable success is unclassified", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not json"))
		}))
		defer srv.Close()
		p, err := NewAPI(APIConfig{APIKey: "k", BaseURL: srv.URL, HTTPClient: srv.Client()}, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Send(context.Background(), msg)
		if err == nil || providers.IsTransient(err) || providers.IsPermanent(err) {
			t.Errorf("err = %v, want an unclassified error", err)
		}
	})
}
