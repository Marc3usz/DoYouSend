package setup

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

func TestConfigFromEnv_Defaults(t *testing.T) {
	t.Parallel()

	cfg := ConfigFromEnv()

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"EmailProvider", cfg.EmailProvider, "mailpit"},
		{"SMTPHost", cfg.SMTPHost, "localhost"},
		{"SMTPPort", cfg.SMTPPort, "1025"},
		{"SMTPUsername", cfg.SMTPUsername, ""},
		{"SMTPPassword", cfg.SMTPPassword, ""},
		{"SendGridAPIKey", cfg.SendGridAPIKey, ""},
		{"SendGridWebhookPublicKey", cfg.SendGridWebhookPublicKey, ""},
		{"SMSProvider", cfg.SMSProvider, "fake"},
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}

	if cfg.SendGridSandbox {
		t.Error("SendGridSandbox should default to false")
	}
	if !cfg.DryRun {
		t.Error("DryRun should default to true")
	}
}

func TestConfigFromEnv_OverridesFromEnv(t *testing.T) {
	// Not parallel — modifies environment.
	t.Setenv("EMAIL_PROVIDER", "sendgrid")
	t.Setenv("SMTP_HOST", "smtp.example.test")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMS_PROVIDER", "smsapi")
	t.Setenv("SENDGRID_API_KEY", "test-sg-key")
	t.Setenv("SENDGRID_SANDBOX", "true")
	t.Setenv("SENDGRID_WEBHOOK_PUBLIC_KEY", "test-webhook-key")
	t.Setenv("DRY_RUN", "false")

	cfg := ConfigFromEnv()

	if cfg.EmailProvider != "sendgrid" {
		t.Errorf("EmailProvider = %q, want %q", cfg.EmailProvider, "sendgrid")
	}
	if cfg.SMTPHost != "smtp.example.test" {
		t.Errorf("SMTPHost = %q, want %q", cfg.SMTPHost, "smtp.example.test")
	}
	if cfg.SMTPPort != "587" {
		t.Errorf("SMTPPort = %q, want %q", cfg.SMTPPort, "587")
	}
	if cfg.SMSProvider != "smsapi" {
		t.Errorf("SMSProvider = %q, want %q", cfg.SMSProvider, "smsapi")
	}
	if cfg.SendGridAPIKey != "test-sg-key" {
		t.Errorf("SendGridAPIKey = %q, want %q", cfg.SendGridAPIKey, "test-sg-key")
	}
	if cfg.SendGridWebhookPublicKey != "test-webhook-key" {
		t.Errorf("SendGridWebhookPublicKey = %q, want %q", cfg.SendGridWebhookPublicKey, "test-webhook-key")
	}
	if !cfg.SendGridSandbox {
		t.Error("SendGridSandbox should be true when SENDGRID_SANDBOX=true")
	}
	if cfg.DryRun {
		t.Error("DryRun should be false when DRY_RUN=false")
	}
}

func TestEnvOr(t *testing.T) {
	t.Parallel()

	key := "TEST_PROVIDER_UNSET_XYZ"
	os.Unsetenv(key)

	if got := envOr(key, "default"); got != "default" {
		t.Errorf("envOr(%q, %q) = %q, want %q", key, "default", got, "default")
	}
}

func TestNew_DefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := Config{
		EmailProvider: "mailpit",
		SMTPHost:      "localhost",
		SMTPPort:      "1025",
		EmailFrom:     "test@example.test",
		SMSProvider:   "fake",
		DryRun:        false,
	}

	p, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.Email == nil {
		t.Fatal("Email provider is nil")
	}
	if p.SMS == nil {
		t.Fatal("SMS provider is nil")
	}

	if p.Email.Channel() != providers.ChannelEmail {
		t.Errorf("Email.Channel() = %v, want %v", p.Email.Channel(), providers.ChannelEmail)
	}
	if p.SMS.Channel() != providers.ChannelSMS {
		t.Errorf("SMS.Channel() = %v, want %v", p.SMS.Channel(), providers.ChannelSMS)
	}
}

func TestNew_SendGridEmailProvider(t *testing.T) {
	t.Parallel()

	cfg := Config{
		EmailProvider:  "sendgrid",
		SendGridAPIKey: "test-api-key",
		EmailFrom:      "test@example.test",
		SMSProvider:    "fake",
		DryRun:         false,
	}

	p, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.Email == nil {
		t.Fatal("Email provider is nil")
	}
	if p.Email.Channel() != providers.ChannelEmail {
		t.Errorf("Email.Channel() = %v, want %v", p.Email.Channel(), providers.ChannelEmail)
	}
}

func TestNew_SendGridMissingAPIKey(t *testing.T) {
	t.Parallel()

	// Review comment 7: when EMAIL_PROVIDER=sendgrid but SENDGRID_API_KEY is empty,
	// New should return an error immediately at startup.
	cfg := Config{
		EmailProvider:  "sendgrid",
		SendGridAPIKey: "",
		EmailFrom:      "test@example.test",
		SMSProvider:    "fake",
	}

	_, err := New(cfg, nil)
	if err == nil {
		t.Fatal("expected error when SENDGRID_API_KEY is empty, got nil")
	}
}

func TestNew_SendGridInvalidFrom(t *testing.T) {
	t.Parallel()

	cfg := Config{
		EmailProvider:  "sendgrid",
		SendGridAPIKey: "test-api-key",
		EmailFrom:      "invalid-from",
		SMSProvider:    "fake",
	}

	_, err := New(cfg, nil)
	if err == nil {
		t.Fatal("expected error when EmailFrom is invalid, got nil")
	}
}

func TestNew_UnsupportedEmailProvider(t *testing.T) {
	t.Parallel()

	_, err := New(Config{EmailProvider: "nonexistent", SMSProvider: "fake"}, nil)
	if err == nil {
		t.Fatal("expected error for unsupported email provider")
	}
	if !errors.Is(err, ErrUnsupportedProvider) {
		t.Errorf("expected ErrUnsupportedProvider, got: %v", err)
	}
}

func TestNew_UnsupportedSMSProvider(t *testing.T) {
	t.Parallel()

	_, err := New(Config{EmailProvider: "mailpit", SMSProvider: "nonexistent"}, nil)
	if err == nil {
		t.Fatal("expected error for unsupported SMS provider")
	}
	if !errors.Is(err, ErrUnsupportedProvider) {
		t.Errorf("expected ErrUnsupportedProvider, got: %v", err)
	}
}

func TestNew_DryRunWrapsBothProviders(t *testing.T) {
	t.Parallel()

	cfg := Config{
		EmailProvider: "mailpit",
		SMTPHost:      "localhost",
		SMTPPort:      "1025",
		EmailFrom:     "test@example.test",
		SMSProvider:   "fake",
		DryRun:        true,
	}

	p, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// In DryRun mode sending an email or SMS must succeed without network
	// transmission and return ErrDryRun.
	emailRes, err := p.Email.Send(context.Background(), providers.Message{
		RecipientID: "rec-dry-email",
		To:          "dry@example.test",
		Subject:     "Dry",
		Body:        "Body",
	})
	if err != nil {
		t.Fatalf("unexpected dry-run email error: %v", err)
	}
	if emailRes.ProviderMessageID == "" {
		t.Error("expected non-empty dry-run ProviderMessageID for email")
	}

	smsRes, err := p.SMS.Send(context.Background(), providers.Message{
		RecipientID: "rec-dry-sms",
		To:          "+48500100101",
		Body:        "Body",
	})
	if err != nil {
		t.Fatalf("unexpected dry-run SMS error: %v", err)
	}
	if smsRes.ProviderMessageID == "" {
		t.Error("expected non-empty dry-run ProviderMessageID for SMS")
	}
}
