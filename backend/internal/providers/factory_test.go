package providers

import (
	"context"
	"errors"
	"testing"
)

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

	if p.Email.Channel() != ChannelEmail {
		t.Errorf("Email.Channel() = %v, want %v", p.Email.Channel(), ChannelEmail)
	}
	if p.SMS.Channel() != ChannelSMS {
		t.Errorf("SMS.Channel() = %v, want %v", p.SMS.Channel(), ChannelSMS)
	}
}

func TestNew_UnsupportedEmailProvider(t *testing.T) {
	t.Parallel()

	cfg := Config{
		EmailProvider: "sendgrid",
		SMSProvider:   "fake",
	}

	_, err := New(cfg, nil)
	if err == nil {
		t.Fatal("expected error for unsupported email provider")
	}
	if !errors.Is(err, ErrUnsupportedProvider) {
		t.Errorf("expected ErrUnsupportedProvider, got: %v", err)
	}
}

func TestNew_UnsupportedSMSProvider(t *testing.T) {
	t.Parallel()

	cfg := Config{
		EmailProvider: "mailpit",
		SMTPHost:      "localhost",
		SMTPPort:      "1025",
		EmailFrom:     "test@example.test",
		SMSProvider:   "twilio",
	}

	_, err := New(cfg, nil)
	if err == nil {
		t.Fatal("expected error for unsupported SMS provider")
	}
	if !errors.Is(err, ErrUnsupportedProvider) {
		t.Errorf("expected ErrUnsupportedProvider, got: %v", err)
	}
}

func TestNew_DryRunWrapsProviders(t *testing.T) {
	t.Parallel()

	cfg := Config{
		EmailProvider: "mailpit",
		SMTPHost:      "unreachable.invalid",
		SMTPPort:      "9999",
		EmailFrom:     "test@example.test",
		SMSProvider:   "fake",
		DryRun:        true,
	}

	p, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// When DryRun is true, sending through Email should succeed without
	// touching the network — the host is unreachable on purpose.
	res, err := p.Email.Send(context.Background(), Message{
		RecipientID: "r-dry-email",
		To:          "parent@example.test",
		Subject:     "Test",
		Body:        "body",
	})
	if err != nil {
		t.Fatalf("Email.Send in dry run should succeed, got: %v", err)
	}
	if res.ProviderMessageID == "" {
		t.Error("expected non-empty ProviderMessageID from dry-run email")
	}

	// Same for SMS.
	res, err = p.SMS.Send(context.Background(), Message{
		RecipientID: "r-dry-sms",
		To:          "+48500100101",
		Body:        "body",
	})
	if err != nil {
		t.Fatalf("SMS.Send in dry run should succeed, got: %v", err)
	}
	if res.ProviderMessageID == "" {
		t.Error("expected non-empty ProviderMessageID from dry-run sms")
	}
}

func TestNew_NoDryRunDoesNotWrap(t *testing.T) {
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

	// SMS fake provider should record the message when DryRun=false.
	res, err := p.SMS.Send(context.Background(), Message{
		RecipientID: "r-real-sms",
		To:          "+48500100102",
		Body:        "real message",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ProviderMessageID == "" {
		t.Error("expected non-empty ProviderMessageID")
	}
}
