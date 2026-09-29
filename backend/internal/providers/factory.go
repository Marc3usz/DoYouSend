package providers

import (
	"fmt"
	"log/slog"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers/email"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers/sms"
)

// Providers groups the initialised email and SMS providers, ready to be
// injected into the delivery worker or the API server.
type Providers struct {
	Email Provider
	SMS   Provider
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
		emailProvider = WrapDryRun(emailProvider, logger)
		smsProvider = WrapDryRun(smsProvider, logger)
	}

	return &Providers{
		Email: emailProvider,
		SMS:   smsProvider,
	}, nil
}

// newEmailProvider creates the email provider selected by cfg.EmailProvider.
func newEmailProvider(cfg Config, logger *slog.Logger) (Provider, error) {
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

	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedProvider, cfg.EmailProvider)
	}
}

// newSMSProvider creates the SMS provider selected by cfg.SMSProvider.
func newSMSProvider(cfg Config, logger *slog.Logger) (Provider, error) {
	switch cfg.SMSProvider {
	case "fake":
		return sms.NewFake(logger), nil

	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedProvider, cfg.SMSProvider)
	}
}
