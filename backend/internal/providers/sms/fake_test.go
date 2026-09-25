package sms

import (
	"context"
	"log/slog"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

func TestFake_Send(t *testing.T) {
	t.Parallel()

	logger := slog.Default()

	tests := []struct {
		name        string
		msg         providers.Message
		wantErr     bool
		isPermanent bool
	}{
		{
			name: "valid message is recorded",
			msg: providers.Message{
				RecipientID: "r-001",
				To:          "+48500100101",
				Body:        "Zebranie 15 października o 17:00",
			},
			wantErr: false,
		},
		{
			name: "empty To is a permanent error",
			msg: providers.Message{
				RecipientID: "r-002",
				To:          "",
				Body:        "test",
			},
			wantErr:     true,
			isPermanent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := NewFake(logger)
			result, err := f.Send(context.Background(), tt.msg)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.isPermanent && !providers.IsPermanent(err) {
					t.Errorf("expected permanent error, got: %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.ProviderMessageID == "" {
				t.Error("expected non-empty ProviderMessageID")
			}

			sent := f.Sent()
			if len(sent) != 1 {
				t.Fatalf("expected 1 recorded message, got %d", len(sent))
			}
			if sent[0].RecipientID != tt.msg.RecipientID {
				t.Errorf("recorded recipient = %s, want %s", sent[0].RecipientID, tt.msg.RecipientID)
			}
		})
	}
}

func TestFake_CancelledContext(t *testing.T) {
	t.Parallel()

	f := NewFake(slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := f.Send(ctx, providers.Message{
		RecipientID: "r-003",
		To:          "+48500100102",
		Body:        "test",
	})

	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !providers.IsTransient(err) {
		t.Errorf("cancelled context should be transient, got: %v", err)
	}
}

func TestFake_Reset(t *testing.T) {
	t.Parallel()

	f := NewFake(slog.Default())

	_, _ = f.Send(context.Background(), providers.Message{
		RecipientID: "r-004",
		To:          "+48500100103",
		Body:        "test",
	})

	if len(f.Sent()) != 1 {
		t.Fatal("expected 1 message before reset")
	}

	f.Reset()

	if len(f.Sent()) != 0 {
		t.Fatal("expected 0 messages after reset")
	}
}

func TestFake_Channel(t *testing.T) {
	t.Parallel()

	f := NewFake(slog.Default())
	if f.Channel() != providers.ChannelSMS {
		t.Errorf("Channel() = %v, want %v", f.Channel(), providers.ChannelSMS)
	}
}
