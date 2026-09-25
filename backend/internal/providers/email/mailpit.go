// Package email implements the e-mail channel. Locally it talks to Mailpit, so no message
// ever leaves the developer's machine. Owner: DEV C (MichalK252).
package email

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"sync/atomic"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// MailpitConfig holds the SMTP connection details for Mailpit.
// Values come from the environment via platform/config (see .env.example).
type MailpitConfig struct {
	Host     string // SMTP_HOST, default "localhost"
	Port     string // SMTP_PORT, default "1025"
	From     string // EMAIL_FROM, e.g. "Szkola Testowa <no-reply@example.test>"
	Username string // SMTP_USERNAME (empty for Mailpit)
	Password string // SMTP_PASSWORD (empty for Mailpit)
}

// Mailpit sends e-mails via SMTP to a local Mailpit instance.
// In production this struct can be replaced by a different implementation
// behind the same providers.Provider interface.
//
// Safe for concurrent use.
type Mailpit struct {
	cfg    MailpitConfig
	logger *slog.Logger
	seq    atomic.Int64
}

// NewMailpit creates a Mailpit email provider.
func NewMailpit(cfg MailpitConfig, logger *slog.Logger) *Mailpit {
	return &Mailpit{
		cfg:    cfg,
		logger: logger,
	}
}

// Send delivers an e-mail via SMTP.
//
// Errors are classified as:
//   - permanent: empty To, malformed address,
//   - transient: connection refused, timeout, SMTP 4xx.
func (m *Mailpit) Send(ctx context.Context, msg providers.Message) (providers.Result, error) {
	if ctx.Err() != nil {
		return providers.Result{}, providers.TransientError(
			fmt.Errorf("send email to recipient %s: %w", msg.RecipientID, ctx.Err()),
		)
	}

	if msg.To == "" {
		return providers.Result{}, providers.PermanentError(
			fmt.Errorf("send email to recipient %s: %w", msg.RecipientID, providers.ErrInvalidRecipient),
		)
	}

	body := buildRFC822(m.cfg.From, msg.To, msg.Subject, msg.Body)

	addr := net.JoinHostPort(m.cfg.Host, m.cfg.Port)
	var auth smtp.Auth
	if m.cfg.Username != "" {
		auth = smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	}

	if err := smtp.SendMail(addr, auth, m.cfg.From, []string{msg.To}, []byte(body)); err != nil {
		return providers.Result{}, classifySMTPError(msg.RecipientID, err)
	}

	id := fmt.Sprintf("mailpit-%d", m.seq.Add(1))

	// Never log the full e-mail address or body — only the recipient ID.
	m.logger.Info("email sent via mailpit",
		"recipient_id", msg.RecipientID,
		"provider_message_id", id,
	)

	return providers.Result{ProviderMessageID: id}, nil
}

// Channel returns providers.ChannelEmail.
func (m *Mailpit) Channel() providers.Channel {
	return providers.ChannelEmail
}

// buildRFC822 constructs a minimal RFC 822 message.
func buildRFC822(from, to, subject, body string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return b.String()
}

// classifySMTPError maps SMTP errors to permanent or transient.
func classifySMTPError(recipientID string, err error) error {
	wrap := func(err error) error {
		return fmt.Errorf("send email to recipient %s: %w", recipientID, err)
	}

	// Network errors (dial timeout, connection refused) are transient.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return providers.TransientError(wrap(err))
	}

	errStr := err.Error()

	// SMTP 5xx codes are permanent (bad mailbox, rejected address).
	if len(errStr) >= 3 && errStr[0] == '5' {
		return providers.PermanentError(wrap(err))
	}

	// SMTP 4xx codes are transient (try again later, greylisting).
	if len(errStr) >= 3 && errStr[0] == '4' {
		return providers.TransientError(wrap(err))
	}

	// Default: treat unknown errors as transient to allow retry.
	return providers.TransientError(wrap(err))
}
