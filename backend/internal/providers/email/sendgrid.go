package email

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
	netmail "net/mail"
	"strings"
	"time"

	"github.com/sendgrid/rest"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// Data-residency regions understood by SendGridConfig.Region (ADR-0014).
// maxErrorDetail caps the provider response text copied into error messages.
const maxErrorDetail = 4096

const (
	RegionGlobal = "global" // api.sendgrid.com
	RegionEU     = "eu"     // api.eu.sendgrid.com
)

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

	// Region selects the SendGrid data-residency host: RegionGlobal (default when
	// empty) or RegionEU. Any other value is rejected by NewSendGrid.
	Region string

	// BaseURL overrides the API host and wins over Region. It exists for tests,
	// which set it to an httptest.Server URL.
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
	cfg        SendGridConfig
	client     *rest.Client // wraps our own *http.Client; never sendgrid.DefaultClient (ADR-0014)
	host       string
	creditsURL string
	fromAddr   *mail.Email // parsed once in NewSendGrid
	logger     *slog.Logger
}

// NewSendGrid creates a SendGrid email provider.
// If logger is nil, slog.Default() is used.
//
// Returns an error if From is not a valid RFC 5322 address.
func NewSendGrid(cfg SendGridConfig, logger *slog.Logger) (*SendGrid, error) {
	if logger == nil {
		logger = slog.Default()
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	base, err := resolveHost(cfg)
	if err != nil {
		return nil, err
	}

	from, err := parseFromAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("parse sender address: %w", err)
	}

	return &SendGrid{
		cfg:        cfg,
		client:     &rest.Client{HTTPClient: httpClient},
		host:       base,
		creditsURL: base + "/v3/user/credits",
		fromAddr:   from,
		logger:     logger,
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

	req := sg.buildRequest(msg)

	resp, err := sg.client.SendWithContext(ctx, req)
	if err != nil {
		return providers.Result{}, providers.TransientError(
			fmt.Errorf("send email to recipient %s: %w", msg.RecipientID, err),
		)
	}
	header := http.Header(resp.Headers)

	// SendGrid returns 202 Accepted on normal send, 200 OK in sandbox mode.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		msgID := header.Get("X-Message-Id")
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

	// rest.Client reads the whole response; cap what ends up in the error text.
	detail := resp.Body
	if len(detail) > maxErrorDetail {
		detail = detail[:maxErrorDetail]
	}
	return providers.Result{}, classifyHTTPError(msg.RecipientID, resp.StatusCode, detail, header)
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
		retryAfter, _ := providers.ParseRetryAfter(header.Get("Retry-After"), time.Now())
		if ra := header.Get("Retry-After"); ra != "" {
			err = fmt.Errorf("%w (Retry-After: %s)", err, ra)
		}
		return providers.RateLimitError(err, retryAfter)
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
func parseFromAddress(from string) (*mail.Email, error) {
	if from == "" {
		return nil, fmt.Errorf("sender address is empty")
	}
	addr, err := netmail.ParseAddress(from)
	if err != nil {
		return nil, err
	}
	return mail.NewEmail(addr.Name, addr.Address), nil
}

// syntheticMessageID generates a placeholder message ID when the API does not
// return one (sandbox mode).
func syntheticMessageID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("sandbox-%d-%s", time.Now().UnixNano(), hex.EncodeToString(b[:]))
}

// --- SendGrid v3 mail/send request ---

// resolveHost returns the API host for cfg: BaseURL when set (tests), otherwise the
// host of the selected data-residency region, taken from the SDK (ADR-0014).
func resolveHost(cfg SendGridConfig) (string, error) {
	if cfg.BaseURL != "" {
		return cfg.BaseURL, nil
	}
	region := cfg.Region
	if region == "" {
		region = RegionGlobal
	}
	req, err := sendgrid.SetDataResidency(sendgrid.GetRequest("", "", ""), region)
	if err != nil {
		return "", fmt.Errorf("sendgrid region %q: %w", cfg.Region, err)
	}
	return req.BaseURL, nil
}

// buildRequest assembles the mail/send request with the SDK types. The body is the
// rendered body untouched, as text/plain, with click and open tracking off so links
// stay identical to the SMS (CLAUDE.md, rule 4).
func (sg *SendGrid) buildRequest(msg providers.Message) rest.Request {
	p := mail.NewPersonalization()
	p.AddTos(mail.NewEmail("", msg.To))

	m := mail.NewV3Mail().
		SetFrom(sg.fromAddr).
		AddPersonalizations(p).
		AddContent(mail.NewContent("text/plain", msg.Body)).
		SetTrackingSettings(&mail.TrackingSettings{
			ClickTracking: &mail.ClickTrackingSetting{Enable: boolPtr(false)},
			OpenTracking:  &mail.OpenTrackingSetting{Enable: boolPtr(false)},
		}).
		SetCustomArg("recipient_id", msg.RecipientID)
	m.Subject = msg.Subject

	if sg.cfg.Sandbox {
		m.SetMailSettings(&mail.MailSettings{SandboxMode: mail.NewSetting(true)})
	}

	req := sendgrid.GetRequest(sg.cfg.APIKey, "/v3/mail/send", sg.host)
	req.Method = http.MethodPost
	req.Body = mail.GetRequestBody(m)
	return req
}

func boolPtr(b bool) *bool { return &b }

// GetProfile fetches the SendGrid account credit limits from GET /v3/user/credits.
func (sg *SendGrid) GetProfile(ctx context.Context) (Profile, error) {
	if ctx.Err() != nil {
		return Profile{}, ctx.Err()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sg.creditsURL, nil)
	if err != nil {
		return Profile{}, fmt.Errorf("sendgrid get profile: create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+sg.cfg.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := sg.client.HTTPClient.Do(req)
	if err != nil {
		return Profile{}, fmt.Errorf("sendgrid get profile: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return Profile{}, fmt.Errorf("sendgrid get profile: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Errors []struct {
				Message string `json:"message"`
				Field   string `json:"field"`
			} `json:"errors"`
		}
		if json.Unmarshal(body, &apiErr) == nil && len(apiErr.Errors) > 0 {
			return Profile{}, fmt.Errorf("sendgrid get profile: api error: %s", apiErr.Errors[0].Message)
		}
		return Profile{}, fmt.Errorf("sendgrid get profile: unexpected status %d", resp.StatusCode)
	}

	var data struct {
		Remain         *int64 `json:"remain"`
		Total          *int64 `json:"total"`
		Used           *int64 `json:"used"`
		ResetFrequency string `json:"reset_frequency"`
		NextReset      string `json:"next_reset"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return Profile{}, fmt.Errorf("sendgrid get profile: decode response: %w", err)
	}

	if data.Remain == nil {
		return Profile{}, errors.New("sendgrid get profile: missing remain field in response")
	}

	return Profile{
		Provider:       "sendgrid",
		RemainCredits:  data.Remain,
		TotalCredits:   data.Total,
		UsedCredits:    data.Used,
		ResetFrequency: data.ResetFrequency,
		NextReset:      data.NextReset,
	}, nil
}
