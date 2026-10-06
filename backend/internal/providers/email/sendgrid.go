package email

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// sendgridAPIURL is the production endpoint. Tests override it via SendGridConfig.BaseURL.
const sendgridAPIURL = "https://api.sendgrid.com"

// SendGridConfig holds the credentials and settings for the SendGrid adapter.
// Values come from the environment via setup.ConfigFromEnv.
type SendGridConfig struct {
	// APIKey is the SendGrid API key (Bearer token). Required.
	APIKey string

	// From is the sender address, e.g. "Szkola <no-reply@example.test>"
	// or just "no-reply@example.test". Parsed with net/mail.ParseAddress.
	From string

	// Sandbox enables SendGrid sandbox mode: the API validates the request
	// but does not deliver the message (returns 200 OK, not 202).
	// Useful for integration tests.
	Sandbox bool

	// BaseURL overrides the API endpoint. Empty means the production URL.
	// Tests set this to an httptest.Server URL.
	BaseURL string

	// HTTPClient is the client used for API calls. If nil, a client with a
	// 15-second timeout is created.
	HTTPClient *http.Client
}

// SendGrid sends e-mails through the SendGrid Web API v3.
//
// It implements providers.Provider and is safe for concurrent use.
// Click tracking and open tracking are disabled in every request so the
// message body stays identical to the SMS body (CLAUDE.md, rule 4).
type SendGrid struct {
	cfg      SendGridConfig
	client   *http.Client
	url      string
	fromAddr sgAddress // parsed once in NewSendGrid
	logger   *slog.Logger
}

// NewSendGrid creates a SendGrid email provider.
// If logger is nil, slog.Default() is used.
//
// Returns an error if From is not a valid RFC 5322 address.
func NewSendGrid(cfg SendGridConfig, logger *slog.Logger) (*SendGrid, error) {
	if logger == nil {
		logger = slog.Default()
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	base := cfg.BaseURL
	if base == "" {
		base = sendgridAPIURL
	}

	from, err := parseFromAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("parse sender address: %w", err)
	}

	return &SendGrid{
		cfg:      cfg,
		client:   client,
		url:      base + "/v3/mail/send",
		fromAddr: from,
		logger:   logger,
	}, nil
}

// Channel returns providers.ChannelEmail.
func (sg *SendGrid) Channel() providers.Channel {
	return providers.ChannelEmail
}

// Send delivers a single e-mail to a single recipient via the SendGrid
// v3 mail/send endpoint.
//
// Error classification (ADR-0008):
//   - 4xx (except 429) → permanent (bad content, bad key, no permission)
//   - 429              → transient (rate limit; Retry-After in error message)
//   - 5xx              → transient (server error)
//   - context cancel   → transient
func (sg *SendGrid) Send(ctx context.Context, msg providers.Message) (providers.Result, error) {
	if ctx.Err() != nil {
		return providers.Result{}, providers.TransientError(
			fmt.Errorf("send email to recipient %s: %w", msg.RecipientID, ctx.Err()),
		)
	}

	if strings.TrimSpace(msg.To) == "" {
		return providers.Result{}, providers.PermanentError(
			fmt.Errorf("send email to recipient %s: %w", msg.RecipientID, providers.ErrInvalidRecipient),
		)
	}

	body, err := sg.buildRequestBody(msg)
	if err != nil {
		return providers.Result{}, providers.PermanentError(
			fmt.Errorf("send email to recipient %s: build request: %w", msg.RecipientID, err),
		)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sg.url, bytes.NewReader(body))
	if err != nil {
		return providers.Result{}, providers.TransientError(
			fmt.Errorf("send email to recipient %s: create request: %w", msg.RecipientID, err),
		)
	}
	req.Header.Set("Authorization", "Bearer "+sg.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := sg.client.Do(req)
	if err != nil {
		return providers.Result{}, providers.TransientError(
			fmt.Errorf("send email to recipient %s: %w", msg.RecipientID, err),
		)
	}
	defer resp.Body.Close()

	// Limit how much of the response body we read to avoid unbounded memory use.
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	// SendGrid returns 202 Accepted on normal send, 200 OK in sandbox mode.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		msgID := resp.Header.Get("X-Message-Id")
		// In sandbox mode SendGrid does not return X-Message-Id; generate a
		// synthetic one so Result always carries a non-empty ID.
		if msgID == "" {
			msgID = syntheticMessageID()
		}
		sg.logger.Info("email sent via sendgrid",
			"recipient_id", msg.RecipientID,
			"provider_message_id", msgID,
			"sandbox", sg.cfg.Sandbox,
		)
		return providers.Result{ProviderMessageID: msgID}, nil
	}

	detail := string(respBody)
	return providers.Result{}, classifyHTTPError(msg.RecipientID, resp.StatusCode, detail, resp.Header)
}

// classifyHTTPError maps SendGrid HTTP status codes to permanent or transient errors.
//
// Classification (ADR-0008):
//   - 429        → transient (rate limit), includes Retry-After if present
//   - other 4xx  → permanent (bad request, auth, forbidden, etc.)
//   - 5xx        → transient (server error)
//   - other      → permanent (unexpected, no point retrying)
func classifyHTTPError(recipientID string, status int, detail string, header http.Header) error {
	err := fmt.Errorf("send email to recipient %s: sendgrid returned HTTP %d: %s", recipientID, status, detail)

	switch {
	case status == 429:
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

// parseFromAddress splits an RFC 5322 address (e.g. "Szkola <no-reply@example.test>")
// into the email and optional display name that SendGrid expects in separate fields.
func parseFromAddress(from string) (sgAddress, error) {
	if from == "" {
		return sgAddress{}, fmt.Errorf("sender address is empty")
	}
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return sgAddress{}, err
	}
	return sgAddress{Email: addr.Address, Name: addr.Name}, nil
}

// syntheticMessageID generates a placeholder message ID when the API does not
// return one (sandbox mode).
func syntheticMessageID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("sandbox-%d-%s", time.Now().UnixNano(), hex.EncodeToString(b[:]))
}

// --- SendGrid v3 mail/send request body ---

type sgMailSend struct {
	Personalizations []sgPersonalization `json:"personalizations"`
	From             sgAddress           `json:"from"`
	Subject          string              `json:"subject"`
	Content          []sgContent         `json:"content"`
	TrackingSettings sgTracking          `json:"tracking_settings"`
	MailSettings     *sgMailSettings     `json:"mail_settings,omitempty"`
	CustomArgs       map[string]string   `json:"custom_args,omitempty"`
}

type sgPersonalization struct {
	To []sgAddress `json:"to"`
}

type sgAddress struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type sgContent struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type sgTracking struct {
	ClickTracking sgToggle `json:"click_tracking"`
	OpenTracking  sgToggle `json:"open_tracking"`
}

type sgToggle struct {
	Enable bool `json:"enable"`
}

type sgMailSettings struct {
	SandboxMode sgToggle `json:"sandbox_mode"`
}

func (sg *SendGrid) buildRequestBody(msg providers.Message) ([]byte, error) {
	payload := sgMailSend{
		Personalizations: []sgPersonalization{
			{To: []sgAddress{{Email: msg.To}}},
		},
		From:    sg.fromAddr,
		Subject: msg.Subject,
		Content: []sgContent{
			{Type: "text/plain", Value: msg.Body},
		},
		TrackingSettings: sgTracking{
			ClickTracking: sgToggle{Enable: false},
			OpenTracking:  sgToggle{Enable: false},
		},
		CustomArgs: map[string]string{
			"recipient_id": msg.RecipientID,
		},
	}

	if sg.cfg.Sandbox {
		payload.MailSettings = &sgMailSettings{
			SandboxMode: sgToggle{Enable: true},
		}
	}

	return json.Marshal(payload)
}
