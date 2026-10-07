package sms

import (
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// maxDLRBodySize limits the maximum allowed body size for POST callbacks (1 MiB).
const maxDLRBodySize = 1 << 20

// HandleSMSAPIDLR returns an HTTP handler for SMSAPI delivery report (DLR) callbacks.
//
// In accordance with SMSAPI documentation, callback requests arrive via HTTP GET
// or POST (application/x-www-form-urlencoded).
//
// Authentication:
//   - Requires expectedToken to be configured. If empty, returns HTTP 503 Service Unavailable.
//   - Compares token from path ({token}), query param (?token=), or POST form against expectedToken
//     using subtle.ConstantTimeCompare.
//
// On success:
//   - Responds with HTTP 200 OK and body "OK" (SMSAPIDLRAckResponse).
//
// On error:
//   - HTTP 405 Method Not Allowed for unsupported HTTP verbs.
//   - HTTP 503 Service Unavailable if expectedToken is not configured.
//   - HTTP 401 Unauthorized if the token does not match.
//   - HTTP 400 Bad Request if parameters are malformed or missing required keys.
//   - HTTP 500 Internal Server Error if consumers fail to store reports, signaling
//     SMSAPI to retry delivering the callback later.
func HandleSMSAPIDLR(consumer providers.DeliveryReportConsumer, expectedToken string, logger *slog.Logger) http.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		tok := strings.TrimSpace(expectedToken)
		if tok == "" {
			logger.Warn("smsapi dlr rejected: SMSAPI_DLR_TOKEN is not configured")
			http.Error(w, "webhook disabled: token not configured", http.StatusServiceUnavailable)
			return
		}

		reqToken := r.PathValue("token")
		if reqToken == "" {
			reqToken = r.URL.Query().Get("token")
		}

		var values url.Values
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, maxDLRBodySize)
			if err := r.ParseForm(); err != nil {
				var maxErr *http.MaxBytesError
				if errors.As(err, &maxErr) {
					logger.Warn("smsapi dlr request body too large", "err", err)
					http.Error(w, "request entity too large", http.StatusRequestEntityTooLarge)
					return
				}
				logger.Warn("smsapi dlr parse form failed", "err", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			values = r.Form
			if reqToken == "" {
				reqToken = r.Form.Get("token")
			}
		} else {
			values = r.URL.Query()
		}

		if subtle.ConstantTimeCompare([]byte(reqToken), []byte(tok)) != 1 {
			logger.Warn("smsapi dlr unauthorized: token mismatch")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		reports, err := ParseSMSAPIDLR(values)
		if err != nil {
			logger.Warn("smsapi dlr parse failed", "err", err)
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}

		if consumer == nil {
			logger.Error("smsapi dlr consumer is nil")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if err := consumer.ConsumeDeliveryReports(r.Context(), reports); err != nil {
			logger.Error("smsapi dlr consume failed", "err", err, "count", len(reports))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(SMSAPIDLRAckResponse))
	}
}
