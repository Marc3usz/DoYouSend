package providers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// dryRunProvider wraps an existing Provider and intercepts Send calls,
// preventing outbound network transmissions while returning a simulated success.
type dryRunProvider struct {
	inner  Provider
	logger *slog.Logger
}

// WrapDryRun returns a Provider that suppresses external delivery.
// When Send is called, it validates basic recipient constraints, logs the suppression,
// and returns a simulated ProviderMessageID without calling the underlying provider.
func WrapDryRun(inner Provider, logger *slog.Logger) Provider {
	if logger == nil {
		logger = slog.Default()
	}
	return &dryRunProvider{
		inner:  inner,
		logger: logger,
	}
}

func (d *dryRunProvider) Send(ctx context.Context, msg Message) (Result, error) {
	if ctx.Err() != nil {
		return Result{}, TransientError(
			fmt.Errorf("send %s to recipient %s: %w", d.inner.Channel(), msg.RecipientID, ctx.Err()),
		)
	}

	if msg.To == "" || strings.ContainsAny(msg.To, "\r\n") {
		return Result{}, PermanentError(
			fmt.Errorf("send %s to recipient %s: %w", d.inner.Channel(), msg.RecipientID, ErrInvalidRecipient),
		)
	}

	id := fmt.Sprintf("dryrun-%s-%s", d.inner.Channel(), msg.RecipientID)

	d.logger.Info("dry run: message suppressed",
		"channel", d.inner.Channel(),
		"recipient_id", msg.RecipientID,
		"provider_message_id", id,
	)

	return Result{ProviderMessageID: id}, nil
}

func (d *dryRunProvider) Channel() Channel {
	return d.inner.Channel()
}
