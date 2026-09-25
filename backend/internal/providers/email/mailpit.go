// Package email implements the e-mail channel. Locally it talks to Mailpit, so no message
// ever leaves the developer's machine. Owner: DEV C (MichalK252).
package email

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"

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
	DryRun   bool   // DRY_RUN flag from config
}

// Mailpit sends e-mails via SMTP to a local Mailpit instance.
// In production this struct can be replaced by a different implementation
// behind the same providers.Provider interface.
//
// Safe for concurrent use.
type Mailpit struct {
	cfg    MailpitConfig
	logger *slog.Logger
}

// NewMailpit creates a Mailpit email provider.
// If logger is nil, slog.Default() is used.
func NewMailpit(cfg MailpitConfig, logger *slog.Logger) *Mailpit {
	if logger == nil {
		logger = slog.Default()
	}
	return &Mailpit{
		cfg:    cfg,
		logger: logger,
	}
}

// Send delivers an e-mail via SMTP with context-aware timeout and header validation.
//
// Errors are classified as:
//   - permanent: empty To, CRLF in headers (injection attempt), SMTP 5xx,
//   - transient: connection timeout, connection refused, SMTP 4xx.
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

	// Guard against header injection via carriage returns or newlines.
	if strings.ContainsAny(msg.To, "\r\n") {
		return providers.Result{}, providers.PermanentError(
			fmt.Errorf("send email to recipient %s: recipient address contains newline", msg.RecipientID),
		)
	}
	if strings.ContainsAny(msg.Subject, "\r\n") {
		return providers.Result{}, providers.PermanentError(
			fmt.Errorf("send email to recipient %s: subject contains newline", msg.RecipientID),
		)
	}

	msgID := generateMessageID(m.cfg.Host)

	// Short-circuit if DRY_RUN is active so no network transmission happens.
	if m.cfg.DryRun {
		m.logger.Info("dry run: email suppressed",
			"recipient_id", msg.RecipientID,
			"provider_message_id", msgID,
		)
		return providers.Result{ProviderMessageID: msgID}, nil
	}

	body := buildRFC822(m.cfg.From, msg.To, msg.Subject, msg.Body, msgID)

	addr := net.JoinHostPort(m.cfg.Host, m.cfg.Port)

	// Connect with context support so hanging connections don't block indefinitely.
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return providers.Result{}, classifySMTPError(msg.RecipientID, err)
	}
	defer conn.Close()

	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	}

	// Ensure immediate unblocking if the context is cancelled during the SMTP exchange.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		return providers.Result{}, classifySMTPError(msg.RecipientID, err)
	}
	defer client.Close()

	if m.cfg.Username != "" {
		auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return providers.Result{}, classifySMTPError(msg.RecipientID, err)
			}
		}
	}

	if err := client.Mail(m.cfg.From); err != nil {
		return providers.Result{}, classifySMTPError(msg.RecipientID, err)
	}
	if err := client.Rcpt(msg.To); err != nil {
		return providers.Result{}, classifySMTPError(msg.RecipientID, err)
	}

	w, err := client.Data()
	if err != nil {
		return providers.Result{}, classifySMTPError(msg.RecipientID, err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		_ = w.Close()
		return providers.Result{}, classifySMTPError(msg.RecipientID, err)
	}
	if err := w.Close(); err != nil {
		return providers.Result{}, classifySMTPError(msg.RecipientID, err)
	}

	_ = client.Quit()

	// Never log the full e-mail address or body — only the recipient ID (backend/CLAUDE.md).
	m.logger.Info("email sent via mailpit",
		"recipient_id", msg.RecipientID,
		"provider_message_id", msgID,
	)

	return providers.Result{ProviderMessageID: msgID}, nil
}

// Channel returns providers.ChannelEmail.
func (m *Mailpit) Channel() providers.Channel {
	return providers.ChannelEmail
}

// buildRFC822 constructs an RFC 822/5322 compliant message with Q-encoded subject
// and a dedicated Message-ID header.
func buildRFC822(from, to, subject, body, msgID string) string {
	encodedSubject := mime.QEncoding.Encode("utf-8", subject)

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", encodedSubject)
	fmt.Fprintf(&b, "Message-ID: %s\r\n", msgID)
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return b.String()
}

// generateMessageID produces a globally unique RFC 5322 Message-ID header.
func generateMessageID(host string) string {
	if host == "" {
		host = "localhost"
	}
	var randBytes [8]byte
	_, _ = rand.Read(randBytes[:])
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), hex.EncodeToString(randBytes[:]), host)
}

// classifySMTPError maps SMTP and network errors to permanent or transient errors.
func classifySMTPError(recipientID string, err error) error {
	wrap := func(err error) error {
		return fmt.Errorf("send email to recipient %s: %w", recipientID, err)
	}

	// Protocol-level SMTP response codes from textproto.
	var tpErr *textproto.Error
	if errors.As(err, &tpErr) {
		if tpErr.Code >= 500 && tpErr.Code < 600 {
			return providers.PermanentError(wrap(err))
		}
		if tpErr.Code >= 400 && tpErr.Code < 500 {
			return providers.TransientError(wrap(err))
		}
	}

	// Network errors (timeouts, connection refused) are transient.
	var netErr net.Error
	if errors.As(err, &netErr) {
		return providers.TransientError(wrap(err))
	}

	// Fallback to error text inspect if wrapped as raw text.
	errStr := err.Error()
	if len(errStr) >= 3 && errStr[0] == '5' {
		return providers.PermanentError(wrap(err))
	}
	if len(errStr) >= 3 && errStr[0] == '4' {
		return providers.TransientError(wrap(err))
	}

	// Default: treat unknown errors as transient to allow retry.
	return providers.TransientError(wrap(err))
}
