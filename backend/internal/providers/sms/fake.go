// Package sms implements the SMS channel, including part counting agreed with package
// messaging and ingestion of delivery reports. The "fake" implementation records messages
// instead of sending them and is the default. Owner: DEV C (MichalK252).
package sms

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// Fake is an SMS provider that records messages in memory instead of sending them.
// It is the default provider when SMS_PROVIDER=fake or DRY_RUN=true.
//
// Safe for concurrent use.
type Fake struct {
	mu       sync.Mutex
	messages []providers.Message
	logger   *slog.Logger
}

// NewFake creates a Fake SMS provider. All "sent" messages are kept in memory
// and can be inspected via Sent().
func NewFake(logger *slog.Logger) *Fake {
	return &Fake{
		logger: logger,
	}
}

// Send records the message without sending it. It always succeeds unless the
// context is cancelled or the recipient address is empty.
func (f *Fake) Send(ctx context.Context, msg providers.Message) (providers.Result, error) {
	if ctx.Err() != nil {
		return providers.Result{}, providers.TransientError(
			fmt.Errorf("send sms to recipient %s: %w", msg.RecipientID, ctx.Err()),
		)
	}

	if msg.To == "" {
		return providers.Result{}, providers.PermanentError(
			fmt.Errorf("send sms to recipient %s: %w", msg.RecipientID, providers.ErrInvalidRecipient),
		)
	}

	f.mu.Lock()
	f.messages = append(f.messages, msg)
	idx := len(f.messages)
	f.mu.Unlock()

	id := fmt.Sprintf("fake-sms-%d", idx)

	// Never log the full phone number or body — only the recipient ID (backend/CLAUDE.md).
	f.logger.Info("fake sms recorded",
		"recipient_id", msg.RecipientID,
		"provider_message_id", id,
	)

	return providers.Result{ProviderMessageID: id}, nil
}

// Channel returns providers.ChannelSMS.
func (f *Fake) Channel() providers.Channel {
	return providers.ChannelSMS
}

// Sent returns a copy of all recorded messages. Useful for tests and inspection.
func (f *Fake) Sent() []providers.Message {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]providers.Message, len(f.messages))
	copy(out, f.messages)
	return out
}

// Reset clears all recorded messages. Useful between test runs.
func (f *Fake) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.messages = nil
}
