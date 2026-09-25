// Package sms implements the SMS channel. SMS part counting is owned and calculated
// by package messaging (via messaging.MeasureSMS), while delivery and status reporting
// is handled per recipient here. The "fake" implementation records messages in memory
// instead of sending them and is the default. Owner: DEV C (MichalK252).
package sms

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// DefaultMaxMessages defines the maximum number of recorded messages kept in memory
// before older ones are evicted in a ring-buffer fashion.
const DefaultMaxMessages = 1000

// Fake is an SMS provider that records messages in memory instead of sending them.
// It is the default provider when SMS_PROVIDER=fake or DRY_RUN=true.
//
// Safe for concurrent use.
type Fake struct {
	mu          sync.Mutex
	messages    []providers.Message
	maxMessages int
	counter     uint64
	logger      *slog.Logger
}

// NewFake creates a Fake SMS provider with the default in-memory limit.
// If logger is nil, slog.Default() is used.
func NewFake(logger *slog.Logger) *Fake {
	return NewFakeWithLimit(DefaultMaxMessages, logger)
}

// NewFakeWithLimit creates a Fake SMS provider with a custom capacity limit.
func NewFakeWithLimit(maxMessages int, logger *slog.Logger) *Fake {
	if logger == nil {
		logger = slog.Default()
	}
	if maxMessages <= 0 {
		maxMessages = DefaultMaxMessages
	}
	return &Fake{
		maxMessages: maxMessages,
		logger:      logger,
	}
}

// Send records the message without sending it. It always succeeds unless the
// context is cancelled or the recipient address is invalid/empty.
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

	if strings.ContainsAny(msg.To, "\r\n") {
		return providers.Result{}, providers.PermanentError(
			fmt.Errorf("send sms to recipient %s: phone number contains newline", msg.RecipientID),
		)
	}

	f.mu.Lock()
	f.counter++
	id := fmt.Sprintf("fake-sms-%d", f.counter)

	if len(f.messages) >= f.maxMessages {
		// Evict oldest message to prevent unbounded memory growth.
		f.messages = f.messages[1:]
	}
	f.messages = append(f.messages, msg)
	f.mu.Unlock()

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

// Reset clears all recorded messages and resets the counter. Useful between test runs.
func (f *Fake) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.messages = nil
	f.counter = 0
}
