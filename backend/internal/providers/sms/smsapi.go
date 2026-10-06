package sms

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// DefaultSMSAPIBaseURL is the production SMSAPI endpoint (ADR-0003).
const DefaultSMSAPIBaseURL = "https://api.smsapi.pl"

// SMSAPIConfig holds configuration details for the SMSAPI REST adapter.
// Values come from the environment via setup.ConfigFromEnv.
type SMSAPIConfig struct {
	// APIKey is the OAuth token / API key (Bearer token). Required.
	APIKey string

	// SenderName is the registered alphanumeric sender name (field "from").
	// If empty, SMSAPI uses default ECO/2Way or account default.
	SenderName string

	// TestMode simulates sending without charging or dispatching to GSM network
	// (SMSAPI parameter test=1). Useful for integration tests and sandbox.
	TestMode bool

	// BaseURL overrides the default endpoint (https://api.smsapi.pl).
	// Useful for unit tests with httptest.Server or fallback endpoint.
	BaseURL string

	// HTTPClient is the client used for API calls. If nil, a client with a
	// 15-second timeout is created.
	HTTPClient *http.Client
}

// SMSAPI delivers SMS messages via the SMSAPI REST API (POST /sms.do).
//
// It implements providers.Provider and is safe for concurrent use.
// No vendor SDKs are used (pure net/http).
type SMSAPI struct {
	cfg    SMSAPIConfig
	client *http.Client
	url    string
	logger *slog.Logger
}

// NewSMSAPI creates an SMSAPI SMS provider.
// If logger is nil, slog.Default() is used.
//
// Returns an error if APIKey is empty.
func NewSMSAPI(cfg SMSAPIConfig, logger *slog.Logger) (*SMSAPI, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("SMSAPI APIKey is required")
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	base := cfg.BaseURL
	if base == "" {
		base = DefaultSMSAPIBaseURL
	}
	base = strings.TrimRight(base, "/")

	return &SMSAPI{
		cfg:    cfg,
		client: client,
		url:    base + "/sms.do",
		logger: logger,
	}, nil
}

// Channel returns providers.ChannelSMS.
func (s *SMSAPI) Channel() providers.Channel {
	return providers.ChannelSMS
}

// Send delivers a single SMS to a single recipient via SMSAPI POST /sms.do.
//
// Error classification (ADR-0003):
//   - Permanent errors: invalid recipient, authentication failure (11, 12),
//     invalid sender name (14), empty/bad message (18, 26), 4xx HTTP statuses.
//   - Transient errors: gateway rate limit (105, 429), insufficient credits (103),
//     internal server errors (201, 202, 5xx), timeouts, network errors.
func (s *SMSAPI) Send(ctx context.Context, msg providers.Message) (providers.Result, error) {
	if ctx.Err() != nil {
		return providers.Result{}, providers.TransientError(
			fmt.Errorf("send sms to recipient %s: %w", msg.RecipientID, ctx.Err()),
		)
	}

	trimmedTo := strings.TrimSpace(msg.To)
	if trimmedTo == "" {
		return providers.Result{}, providers.PermanentError(
			fmt.Errorf("send sms to recipient %s: %w", msg.RecipientID, providers.ErrInvalidRecipient),
		)
	}

	if strings.ContainsAny(trimmedTo, "\r\n") {
		return providers.Result{}, providers.PermanentError(
			fmt.Errorf("send sms to recipient %s: phone number contains newline", msg.RecipientID),
		)
	}

	// Prepare form data
	form := url.Values{}
	// Normalize recipient number: strip leading plus for SMSAPI compatibility
	form.Set("to", strings.TrimPrefix(trimmedTo, "+"))
	form.Set("message", msg.Body)
	form.Set("format", "json")
	form.Set("encoding", "utf-8")
	form.Set("details", "1")

	if s.cfg.SenderName != "" {
		form.Set("from", s.cfg.SenderName)
	}
	if s.cfg.TestMode {
		form.Set("test", "1")
	}
	if msg.RecipientID != "" {
		form.Set("param1", msg.RecipientID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, strings.NewReader(form.Encode()))
	if err != nil {
		return providers.Result{}, providers.TransientError(
			fmt.Errorf("send sms to recipient %s: create request: %w", msg.RecipientID, err),
		)
	}

	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return providers.Result{}, providers.TransientError(
			fmt.Errorf("send sms to recipient %s: %w", msg.RecipientID, err),
		)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return providers.Result{}, providers.TransientError(
			fmt.Errorf("send sms to recipient %s: read response: %w", msg.RecipientID, err),
		)
	}

	var apiResp smsapiResponse
	jsonErr := json.Unmarshal(respBody, &apiResp)

	// If SMSAPI returned an error object in JSON
	if jsonErr == nil && apiResp.Error != 0 {
		return providers.Result{}, classifySMSAPIError(msg.RecipientID, apiResp.Error, apiResp.Message, resp.StatusCode)
	}

	// Non-2xx HTTP status without a parsed JSON error
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return providers.Result{}, classifyHTTPStatus(msg.RecipientID, resp.StatusCode, string(respBody), resp.Header)
	}

	// Successful response
	if jsonErr == nil && len(apiResp.List) > 0 {
		msgID := apiResp.List[0].ID
		if msgID == "" && s.cfg.TestMode {
			msgID = syntheticSMSMessageID()
		}
		// Never log full phone number or message content (backend/CLAUDE.md)
		s.logger.Info("sms sent via smsapi",
			"recipient_id", msg.RecipientID,
			"provider_message_id", msgID,
			"test_mode", s.cfg.TestMode,
		)
		return providers.Result{ProviderMessageID: msgID}, nil
	}

	// In test mode, SMSAPI might return count without list items
	if s.cfg.TestMode {
		msgID := syntheticSMSMessageID()
		s.logger.Info("sms sent via smsapi test mode",
			"recipient_id", msg.RecipientID,
			"provider_message_id", msgID,
		)
		return providers.Result{ProviderMessageID: msgID}, nil
	}

	return providers.Result{}, providers.TransientError(
		fmt.Errorf("send sms to recipient %s: empty or unexpected response from smsapi: %s", msg.RecipientID, string(respBody)),
	)
}

// classifySMSAPIError maps numerical SMSAPI error codes to permanent or transient errors.
// Reference: https://www.smsapi.pl/docs/#17-kody-bledow
func classifySMSAPIError(recipientID string, code int, message string, httpStatus int) error {
	err := fmt.Errorf("send sms to recipient %s: smsapi error %d: %s", recipientID, code, message)

	switch code {
	// Permanent errors:
	// 11: Invalid login or token
	// 12: Account disabled
	// 13: Invalid recipient phone number
	// 14: Invalid sender name
	// 18: Empty or invalid message
	// 26: Message body too long
	// 30: Invalid UDH
	// 101: Invalid parameter or option
	// 104: Test account restriction / recipient not allowed
	case 11, 12, 13, 14, 18, 26, 30, 101, 104:
		return providers.PermanentError(err)

	// Transient errors:
	// 103: Insufficient points/credits (prepaid exhausted - retryable once topped up)
	// 105: Rate limit exceeded
	// 201: Internal SMSAPI system error
	// 202: SMSAPI gateway unavailable
	case 103, 105, 201, 202:
		return providers.TransientError(err)

	default:
		if httpStatus >= 400 && httpStatus < 500 && httpStatus != http.StatusTooManyRequests {
			return providers.PermanentError(err)
		}
		return providers.TransientError(err)
	}
}

// classifyHTTPStatus maps HTTP status codes to permanent or transient errors.
func classifyHTTPStatus(recipientID string, status int, body string, header http.Header) error {
	err := fmt.Errorf("send sms to recipient %s: smsapi returned HTTP %d: %s", recipientID, status, body)

	switch {
	case status == http.StatusTooManyRequests:
		if ra := header.Get("Retry-After"); ra != "" {
			err = fmt.Errorf("%w (Retry-After: %s)", err, ra)
		}
		return providers.TransientError(err)
	case status >= 400 && status < 500:
		return providers.PermanentError(err)
	case status >= 500:
		return providers.TransientError(err)
	default:
		return providers.PermanentError(err)
	}
}

func syntheticSMSMessageID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("smsapi-test-%d-%s", time.Now().UnixNano(), hex.EncodeToString(b[:]))
}

// --- SMSAPI JSON response structures ---

type smsapiResponse struct {
	Count   int             `json:"count"`
	List    []smsapiMsgItem `json:"list"`
	Error   int             `json:"error"`
	Message string          `json:"message"`
}

type smsapiMsgItem struct {
	ID              string  `json:"id"`
	Points          float64 `json:"points"`
	Number          string  `json:"number"`
	DateSent        int64   `json:"date_sent"`
	SubmittedNumber string  `json:"submitted_number"`
	Status          string  `json:"status"`
}
