package email

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

type mockConsumer struct {
	reports []providers.DeliveryReport
	err     error
}

func (m *mockConsumer) ConsumeDeliveryReports(_ context.Context, reports []providers.DeliveryReport) error {
	if m.err != nil {
		return m.err
	}
	m.reports = append(m.reports, reports...)
	return nil
}

func generateECDSAKeyPair(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ecdsa key: %v", err)
	}

	derKey, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	pubKeyBase64 := base64.StdEncoding.EncodeToString(derKey)
	return privKey, pubKeyBase64
}

func signPayload(t *testing.T, privKey *ecdsa.PrivateKey, timestamp string, payload []byte) string {
	t.Helper()
	h := sha256.New()
	h.Write([]byte(timestamp))
	h.Write(payload)
	digest := h.Sum(nil)

	sigBytes, err := ecdsa.SignASN1(rand.Reader, privKey, digest)
	if err != nil {
		t.Fatalf("sign digest: %v", err)
	}
	return base64.StdEncoding.EncodeToString(sigBytes)
}

func TestHandleSendGridEvents_SuccessWithoutKey(t *testing.T) {
	t.Parallel()

	consumer := &mockConsumer{}
	handler := HandleSendGridEvents(consumer, "", slog.Default())

	payload := `[{"event":"delivered","sg_message_id":"sg-msg-1.filter","timestamp":1631525653}]`
	req := httptest.NewRequest(http.MethodPost, "/providers/email/events", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != `{"status":"ok"}` {
		t.Fatalf("body = %q, want %q", rr.Body.String(), `{"status":"ok"}`)
	}

	if len(consumer.reports) != 1 {
		t.Fatalf("got %d reports, want 1", len(consumer.reports))
	}
	r := consumer.reports[0]
	if r.ProviderMessageID != "sg-msg-1" {
		t.Errorf("ProviderMessageID = %q, want 'sg-msg-1'", r.ProviderMessageID)
	}
	if r.Status != providers.StatusDelivered {
		t.Errorf("Status = %q, want %q", r.Status, providers.StatusDelivered)
	}
	if r.Channel != providers.ChannelEmail {
		t.Errorf("Channel = %q, want %q", r.Channel, providers.ChannelEmail)
	}
}

func TestHandleSendGridEvents_SuccessWithSignedKey(t *testing.T) {
	t.Parallel()

	privKey, pubKey := generateECDSAKeyPair(t)
	consumer := &mockConsumer{}
	handler := HandleSendGridEvents(consumer, pubKey, slog.Default())

	payload := []byte(`[{"event":"delivered","sg_message_id":"sg-msg-signed.filter","timestamp":1631525653}]`)
	timestamp := "1631525653"
	sig := signPayload(t, privKey, timestamp, payload)

	req := httptest.NewRequest(http.MethodPost, "/providers/email/events", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerSendGridSignature, sig)
	req.Header.Set(headerSendGridTimestamp, timestamp)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != `{"status":"ok"}` {
		t.Fatalf("body = %q, want %q", rr.Body.String(), `{"status":"ok"}`)
	}
	if len(consumer.reports) != 1 || consumer.reports[0].ProviderMessageID != "sg-msg-signed" {
		t.Fatalf("unexpected reports: %+v", consumer.reports)
	}
}

func TestHandleSendGridEvents_InvalidSignature(t *testing.T) {
	t.Parallel()

	_, pubKey := generateECDSAKeyPair(t)
	consumer := &mockConsumer{}
	handler := HandleSendGridEvents(consumer, pubKey, slog.Default())

	payload := []byte(`[{"event":"delivered","sg_message_id":"msg-1"}]`)
	req := httptest.NewRequest(http.MethodPost, "/providers/email/events", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerSendGridSignature, base64.StdEncoding.EncodeToString([]byte("bad-signature")))
	req.Header.Set(headerSendGridTimestamp, "1631525653")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandleSendGridEvents_MissingSignatureHeaders(t *testing.T) {
	t.Parallel()

	_, pubKey := generateECDSAKeyPair(t)
	consumer := &mockConsumer{}
	handler := HandleSendGridEvents(consumer, pubKey, slog.Default())

	payload := []byte(`[{"event":"delivered","sg_message_id":"msg-1"}]`)
	req := httptest.NewRequest(http.MethodPost, "/providers/email/events", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandleSendGridEvents_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	consumer := &mockConsumer{}
	handler := HandleSendGridEvents(consumer, "", slog.Default())

	for _, method := range []string{http.MethodGet, http.MethodDelete, http.MethodPut, http.MethodPatch} {
		req := httptest.NewRequest(method, "/providers/email/events", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("[%s] got status %d, want %d", method, rr.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestHandleSendGridEvents_PayloadTooLarge(t *testing.T) {
	t.Parallel()

	consumer := &mockConsumer{}
	handler := HandleSendGridEvents(consumer, "", slog.Default())

	// > 2 MiB payload
	largePayload := make([]byte, (2<<20)+1024)
	req := httptest.NewRequest(http.MethodPost, "/providers/email/events", bytes.NewReader(largePayload))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got status %d, want %d", rr.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestHandleSendGridEvents_InvalidJSON(t *testing.T) {
	t.Parallel()

	consumer := &mockConsumer{}
	handler := HandleSendGridEvents(consumer, "", slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/providers/email/events", strings.NewReader("not-json"))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleSendGridEvents_InformationalEventsOnly(t *testing.T) {
	t.Parallel()

	consumer := &mockConsumer{}
	handler := HandleSendGridEvents(consumer, "", slog.Default())

	payload := `[{"event":"open","sg_message_id":"msg-1"},{"event":"click","sg_message_id":"msg-1"}]`
	req := httptest.NewRequest(http.MethodPost, "/providers/email/events", strings.NewReader(payload))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusOK)
	}
	if len(consumer.reports) != 0 {
		t.Fatalf("consumer received %d reports, want 0", len(consumer.reports))
	}
}

func TestHandleSendGridEvents_ConsumerError(t *testing.T) {
	t.Parallel()

	consumer := &mockConsumer{err: errors.New("db failure")}
	handler := HandleSendGridEvents(consumer, "", slog.Default())

	payload := `[{"event":"delivered","sg_message_id":"msg-1"}]`
	req := httptest.NewRequest(http.MethodPost, "/providers/email/events", strings.NewReader(payload))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandleSendGridEvents_NilConsumer(t *testing.T) {
	t.Parallel()

	handler := HandleSendGridEvents(nil, "", slog.Default())

	payload := `[{"event":"delivered","sg_message_id":"msg-1"}]`
	req := httptest.NewRequest(http.MethodPost, "/providers/email/events", strings.NewReader(payload))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandleSendGridEvents_LogsDoNotLeakData(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	consumer := &mockConsumer{err: errors.New("storage error")}
	handler := HandleSendGridEvents(consumer, "", logger)

	payload := `[{"event":"delivered","sg_message_id":"secret-id","email":"secret-user@example.test"}]`
	req := httptest.NewRequest(http.MethodPost, "/providers/email/events", strings.NewReader(payload))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	logOutput := buf.String()
	if strings.Contains(logOutput, "secret-user@example.test") {
		t.Errorf("recipient email leaked into logs: %s", logOutput)
	}
}
