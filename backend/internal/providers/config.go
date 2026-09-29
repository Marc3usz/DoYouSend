package providers

import (
	"errors"
	"os"
)

// ErrUnsupportedProvider is returned when a configured provider name has no
// matching implementation in this package.
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

	// SMSProvider selects the SMS adapter: "fake" (default).
	SMSProvider string

	// DryRun prevents any real outbound delivery when true.
	// Enforced by wrapping every provider with WrapDryRun.
	DryRun bool
}

// ConfigFromEnv reads provider-related environment variables with safe defaults
// matching .env.example. No secrets are hardcoded — only placeholder defaults
// that keep the system inert (DRY_RUN=true, fake SMS, local Mailpit).
func ConfigFromEnv() Config {
	return Config{
		EmailProvider: envOr("EMAIL_PROVIDER", "mailpit"),
		SMTPHost:      envOr("SMTP_HOST", "localhost"),
		SMTPPort:      envOr("SMTP_PORT", "1025"),
		SMTPUsername:  envOr("SMTP_USERNAME", ""),
		SMTPPassword:  envOr("SMTP_PASSWORD", ""),
		EmailFrom:     envOr("EMAIL_FROM", "DoYouSend <no-reply@example.test>"),
		SMSProvider:   envOr("SMS_PROVIDER", "fake"),
		DryRun:        envOr("DRY_RUN", "true") == "true",
	}
}

// envOr reads an environment variable or returns the fallback.
func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
