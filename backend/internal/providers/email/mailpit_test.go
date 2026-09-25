package email

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

func TestMailpit_Channel(t *testing.T) {
	t.Parallel()

	m := NewMailpit(MailpitConfig{}, slog.Default())
	if m.Channel() != providers.ChannelEmail {
		t.Errorf("Channel() = %v, want %v", m.Channel(), providers.ChannelEmail)
	}
}

func TestMailpit_EmptyTo(t *testing.T) {
	t.Parallel()

	m := NewMailpit(MailpitConfig{Host: "localhost", Port: "1025"}, slog.Default())

	_, err := m.Send(context.Background(), providers.Message{
		RecipientID: "r-010",
		To:          "",
		Subject:     "Test",
		Body:        "test body",
	})

	if err == nil {
		t.Fatal("expected error for empty To")
	}
	if !providers.IsPermanent(err) {
		t.Errorf("empty To should be permanent error, got: %v", err)
	}
}

func TestMailpit_CancelledContext(t *testing.T) {
	t.Parallel()

	m := NewMailpit(MailpitConfig{Host: "localhost", Port: "1025"}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := m.Send(ctx, providers.Message{
		RecipientID: "r-011",
		To:          "parent@example.test",
		Subject:     "Test",
		Body:        "test body",
	})

	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !providers.IsTransient(err) {
		t.Errorf("cancelled context should be transient, got: %v", err)
	}
}

func TestBuildRFC822(t *testing.T) {
	t.Parallel()

	got := buildRFC822(
		"Szkola <no-reply@example.test>",
		"parent@example.test",
		"Zebranie",
		"Zapraszamy na zebranie",
	)

	tests := []struct {
		name     string
		contains string
	}{
		{"has From header", "From: Szkola <no-reply@example.test>"},
		{"has To header", "To: parent@example.test"},
		{"has Subject header", "Subject: Zebranie"},
		{"has Content-Type", "Content-Type: text/plain; charset=utf-8"},
		{"has body", "Zapraszamy na zebranie"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(got, tt.contains) {
				t.Errorf("RFC822 message missing %q:\n%s", tt.contains, got)
			}
		})
	}
}
