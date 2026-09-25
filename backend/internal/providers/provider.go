// Package providers defines the channel abstractions that delivery depends on, so the
// e-mail service and the SMS gateway can be swapped without touching business logic.
//
// Implementations live in the subpackages:
//   - providers/email: SMTP (Mailpit locally) and the production e-mail service,
//   - providers/sms:   the SMS gateway and a "fake" gateway that only records messages.
//
// Owner: DEV C (MichalK252). The fake/local implementations must stay usable with DRY_RUN=true.
package providers

import (
	"context"
	"errors"
	"fmt"
)

// Sentinel errors shared across provider implementations.
var (
	// ErrInvalidRecipient signals a malformed or non-existent address/phone.
	ErrInvalidRecipient = errors.New("invalid recipient")

	// ErrGatewayTimeout signals that the external service did not respond in time.
	ErrGatewayTimeout = errors.New("gateway timeout")

	// ErrGatewayUnavailable signals a 5xx or connection-refused from the gateway.
	ErrGatewayUnavailable = errors.New("gateway unavailable")

	// ErrDryRun is returned (informational) when DRY_RUN prevents actual sending.
	ErrDryRun = errors.New("dry run: message recorded but not sent")
)

// Channel identifies the delivery channel.
type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelSMS   Channel = "sms"
)

// Message holds the data a Provider needs to send one message to one recipient.
// The Body is identical for both email and SMS (project invariant).
type Message struct {
	// RecipientID is the recipient's UUID — used for logging. Never log the
	// full address or phone number (see backend/CLAUDE.md).
	RecipientID string

	// To is the destination: an e-mail address for email providers,
	// an E.164 phone number for SMS providers.
	To string

	// Subject is the e-mail subject line. SMS providers ignore it.
	Subject string

	// Body is the message content, identical for both channels.
	Body string
}

// Result is returned by Provider.Send on success.
type Result struct {
	// ProviderMessageID is the external identifier assigned by the provider.
	// Stored in deliveries.provider_message_id for status tracking.
	ProviderMessageID string
}

// Provider is the interface that every channel adapter must implement.
// The delivery package depends on this interface — never on a vendor SDK directly.
//
// Implementations must:
//   - honour DRY_RUN: when enabled, record the message without sending it
//     (can be enforced via WrapDryRun or internal adapter checks),
//   - classify errors as permanent or transient using the helpers in this package,
//   - be safe for concurrent use from the delivery worker.
type Provider interface {
	// Send delivers a single message to a single recipient.
	// It returns a Result with the provider's message ID on success.
	//
	// Errors must be wrapped with PermanentError or TransientError so that
	// the delivery worker can decide whether to retry.
	Send(ctx context.Context, msg Message) (Result, error)

	// Channel returns which channel this provider serves.
	Channel() Channel
}

// --- Error classification ---
//
// The delivery worker needs to know whether a failed send should be retried.
// Permanent errors (bad address, invalid number) are never retried.
// Transient errors (timeout, 5xx from the gateway) are retried.

// permanentError wraps an error that must not be retried.
type permanentError struct {
	err error
}

func (e *permanentError) Error() string { return fmt.Sprintf("permanent: %s", e.err) }
func (e *permanentError) Unwrap() error { return e.err }

// transientError wraps an error that qualifies for retry.
type transientError struct {
	err error
}

func (e *transientError) Error() string { return fmt.Sprintf("transient: %s", e.err) }
func (e *transientError) Unwrap() error { return e.err }

// PermanentError wraps err to mark it as non-retryable.
// Example: invalid recipient address, rejected by the gateway.
func PermanentError(err error) error {
	return &permanentError{err: err}
}

// TransientError wraps err to mark it as retryable.
// Example: network timeout, HTTP 500/503 from the gateway.
func TransientError(err error) error {
	return &transientError{err: err}
}

// IsPermanent reports whether err (or any error in its chain) is a permanent,
// non-retryable error.
func IsPermanent(err error) bool {
	var pe *permanentError
	return errors.As(err, &pe)
}

// IsTransient reports whether err (or any error in its chain) is a transient,
// retryable error.
func IsTransient(err error) bool {
	var te *transientError
	return errors.As(err, &te)
}
