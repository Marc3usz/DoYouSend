package providers

import (
	"os"
	"testing"
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
		{"SMSProvider", cfg.SMSProvider, "fake"},
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
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
	if cfg.DryRun {
		t.Error("DryRun should be false when DRY_RUN=false")
	}
}

func TestEnvOr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		key      string
		envVal   string
		fallback string
		want     string
	}{
		{"uses fallback when unset", "TEST_PROVIDER_UNSET_XYZ", "", "default", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Ensure the key is NOT set.
			os.Unsetenv(tt.key)
			if got := envOr(tt.key, tt.fallback); got != tt.want {
				t.Errorf("envOr(%q, %q) = %q, want %q", tt.key, tt.fallback, got, tt.want)
			}
		})
	}
}
