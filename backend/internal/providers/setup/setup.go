// Package setup wires the concrete provider implementations to the abstract
// providers.Provider interface. It lives in its own package to break the
// import cycle: providers ← providers/email and providers ← providers/sms
// both depend on the providers package for the interface and error types,
// so the factory that imports the concrete implementations cannot live in
// the providers package itself.
//
// Usage (typically in cmd/api/main.go):
//
//	cfg := setup.ConfigFromEnv()
//	p, err := setup.New(cfg, logger)
//	// p.Email and p.SMS implement providers.Provider
package setup

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers/email"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers/sms"
)

// ErrUnsupportedProvider is returned when a configured provider name has no
// matching implementation.
var ErrUnsupportedProvider = errors.New("unsupported provider")

// Config holds everything the factory needs to build the email and SMS providers.
// Values are typically loaded from environment variables via ConfigFromEnv.
type Config struct {
	// EmailProvider selects the email adapter: "mailpit" (default).
	EmailProvider string

	// SMTP settings used by the mailpit adapter.
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	EmailFrom    string

	// SendGrid settings (used when EmailProvider == "sendgrid").
	SendGridAPIKey  string
	SendGridSandbox bool

	// SMSProvider selects the SMS adapter: "fake" (default).
	SMSProvider string

	// DryRun prevents any real outbound delivery when true.
	// Enforced by wrapping every provider with providers.WrapDryRun.
	DryRun bool
}

// ConfigFromEnv reads provider-related environment variables with safe defaults
// matching .env.example. No secrets are hardcoded — only placeholder defaults
// that keep the system inert (DRY_RUN=true, fake SMS, local Mailpit).
func ConfigFromEnv() Config {
	return Config{
		EmailProvider:   envOr("EMAIL_PROVIDER", "mailpit"),
		SMTPHost:        envOr("SMTP_HOST", "localhost"),
		SMTPPort:        envOr("SMTP_PORT", "1025"),
		SMTPUsername:    envOr("SMTP_USERNAME", ""),
		SMTPPassword:    envOr("SMTP_PASSWORD", ""),
		EmailFrom:       envOr("EMAIL_FROM", "DoYouSend <no-reply@example.test>"),
		SendGridAPIKey:  envOr("SENDGRID_API_KEY", ""),
		SendGridSandbox: envOr("SENDGRID_SANDBOX", "false") == "true",
		SMSProvider:     envOr("SMS_PROVIDER", "fake"),
		DryRun:          envOr("DRY_RUN", "true") == "true",
	}
}

// Providers groups the initialised email and SMS providers, ready to be
// injected into the delivery worker or the API server.
type Providers struct {
	Email providers.Provider
	SMS   providers.Provider
}

// New builds a Providers pair from the given Config.
//
// It selects the concrete adapter based on Config.EmailProvider and
// Config.SMSProvider, then wraps both in WrapDryRun when Config.DryRun is true.
//
// Returns ErrUnsupportedProvider if a provider name has no implementation.
func New(cfg Config, logger *slog.Logger) (*Providers, error) {
	if logger == nil {
		logger = slog.Default()
	}

	emailProvider, err := newEmailProvider(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("init email provider %q: %w", cfg.EmailProvider, err)
	}

	smsProvider, err := newSMSProvider(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("init sms provider %q: %w", cfg.SMSProvider, err)
	}

	if cfg.DryRun {
		logger.Info("dry run enabled: wrapping all providers",
			"email_provider", cfg.EmailProvider,
			"sms_provider", cfg.SMSProvider,
		)
		emailProvider = providers.WrapDryRun(emailProvider, logger)
		smsProvider = providers.WrapDryRun(smsProvider, logger)
	}

	return &Providers{
		Email: emailProvider,
		SMS:   smsProvider,
	}, nil
}

// newEmailProvider creates the email provider selected by cfg.EmailProvider.
func newEmailProvider(cfg Config, logger *slog.Logger) (providers.Provider, error) {
	switch cfg.EmailProvider {
	case "mailpit":
		return email.NewMailpit(email.MailpitConfig{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			From:     cfg.EmailFrom,
			Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword,
			DryRun:   cfg.DryRun,
		}, logger), nil

	case "sendgrid":
		return email.NewSendGrid(email.SendGridConfig{
			APIKey:  cfg.SendGridAPIKey,
			From:    cfg.EmailFrom,
			Sandbox: cfg.SendGridSandbox,
		}, logger), nil

	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedProvider, cfg.EmailProvider)
	}
}

// newSMSProvider creates the SMS provider selected by cfg.SMSProvider.
func newSMSProvider(cfg Config, logger *slog.Logger) (providers.Provider, error) {
	switch cfg.SMSProvider {
	case "fake":
		return sms.NewFake(logger), nil

	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedProvider, cfg.SMSProvider)
	}
}

// envOr reads an environment variable or returns the fallback.
func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
