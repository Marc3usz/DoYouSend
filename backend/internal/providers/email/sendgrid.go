package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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

	// From is the sender address, e.g. "Szkola <no-reply@example.test>".
	From string

	// Sandbox enables SendGrid sandbox mode: the API validates the request
	// but does not deliver the message. Useful for integration tests.
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
	cfg    SendGridConfig
	client *http.Client
	url    string
	logger *slog.Logger
}

// NewSendGrid creates a SendGrid email provider.
// If logger is nil, slog.Default() is used.
func NewSendGrid(cfg SendGridConfig, logger *slog.Logger) *SendGrid {
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
	return &SendGrid{
		cfg:    cfg,
		client: client,
		url:    base + "/v3/mail/send",
		logger: logger,
	}
}

// Channel returns providers.ChannelEmail.
func (sg *SendGrid) Channel() providers.Channel {
	return providers.ChannelEmail
}

// Send delivers a single e-mail to a single recipient via the SendGrid
// v3 mail/send endpoint.
//
// Error classification (ADR-0008):
//   - 400, 401, 403, 413 → permanent (bad content, bad key, no permission)
//   - 429, 5xx            → transient (rate limit, server error)
//   - context cancellation → transient
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

	// SendGrid returns 202 Accepted on success.
	if resp.StatusCode == http.StatusAccepted {
		msgID := resp.Header.Get("X-Message-Id")
		sg.logger.Info("email sent via sendgrid",
			"recipient_id", msg.RecipientID,
			"provider_message_id", msgID,
		)
		return providers.Result{ProviderMessageID: msgID}, nil
	}

	detail := string(respBody)
	return providers.Result{}, sg.classifyHTTPError(msg.RecipientID, resp.StatusCode, detail)
}

// classifyHTTPError maps SendGrid HTTP status codes to permanent or transient errors.
func (sg *SendGrid) classifyHTTPError(recipientID string, status int, detail string) error {
	err := fmt.Errorf("send email to recipient %s: sendgrid returned HTTP %d: %s", recipientID, status, detail)

	switch {
	case status == 400, status == 401, status == 403, status == 413:
		return providers.PermanentError(err)
	case status == 429:
		return providers.TransientError(err)
	case status >= 500:
		return providers.TransientError(err)
	default:
		// Unknown status codes are treated as transient to allow retry.
		return providers.TransientError(err)
	}
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
		From:    sgAddress{Email: sg.cfg.From},
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
