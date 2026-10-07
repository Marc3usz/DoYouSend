package email

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// maxSendGridBodySize limits the maximum allowed body size for SendGrid event webhooks (2 MiB).
const maxSendGridBodySize = 2 << 20

// Header names specified by SendGrid Signed Event Webhook.
const (
	headerSendGridSignature = "X-Twilio-Email-Event-Webhook-Signature"
	headerSendGridTimestamp = "X-Twilio-Email-Event-Webhook-Timestamp"
)

// HandleSendGridEvents returns an HTTP handler for SendGrid Event Webhook callbacks (ADR-0008).
//
// Verification:
//   - If webhookPublicKey is non-empty, verifies the ECDSA SHA-256 signature using the
//     X-Twilio-Email-Event-Webhook-Signature and X-Twilio-Email-Event-Webhook-Timestamp headers.
//   - If webhookPublicKey is empty (e.g. in local development or test environments without keys),
//     signature verification is bypassed.
//
// On success:
//   - Responds with HTTP 200 OK and JSON body `{"status":"ok"}`.
//
// On error:
//   - HTTP 405 Method Not Allowed if the request method is not POST.
//   - HTTP 413 Request Entity Too Large if the body exceeds 2 MiB.
//   - HTTP 401 Unauthorized if the cryptographic signature is invalid or missing headers.
//   - HTTP 400 Bad Request if the payload cannot be parsed as a valid SendGrid event batch.
//   - HTTP 500 Internal Server Error if consumers fail to store reports, causing SendGrid
//     to retry delivery.
func HandleSendGridEvents(consumer providers.DeliveryReportConsumer, webhookPublicKey string, logger *slog.Logger) http.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSendGridBodySize))
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				logger.Warn("sendgrid webhook payload too large", "err", err)
				http.Error(w, "request entity too large", http.StatusRequestEntityTooLarge)
				return
			}
			logger.Warn("read sendgrid webhook payload failed", "err", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		pubKey := strings.TrimSpace(webhookPublicKey)
		if pubKey != "" {
			sig := r.Header.Get(headerSendGridSignature)
			ts := r.Header.Get(headerSendGridTimestamp)
			if err := VerifySendGridWebhookSignature(pubKey, body, sig, ts); err != nil {
				logger.Warn("sendgrid webhook signature verification failed", "err", err)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

		reports, err := ParseSendGridEvents(bytes.NewReader(body), logger)
		if err != nil {
			logger.Warn("sendgrid webhook parse failed", "err", err)
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}

		if consumer == nil {
			logger.Error("sendgrid events consumer is nil")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if len(reports) > 0 {
			if err := consumer.ConsumeDeliveryReports(r.Context(), reports); err != nil {
				logger.Error("sendgrid events consume failed", "err", err, "count", len(reports))
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}
