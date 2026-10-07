package delivery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// Retry defaults: a transient failure is tried up to 3 times, waiting 1s, then 2s.
const (
	DefaultMaxAttempts = 3
	DefaultBaseBackoff = time.Second
)

// ErrDuplicateChannel is returned when two senders serve the same channel.
var ErrDuplicateChannel = errors.New("more than one sender for a channel")

// errNoSender marks a delivery whose channel has no configured sender.
var errNoSender = errors.New("no sender configured for channel")

// Sender sends one message on one channel. providers.Provider satisfies it.
type Sender interface {
	Send(ctx context.Context, msg providers.Message) (providers.Result, error)
	Channel() providers.Channel
}

// RetryPolicy decides how often a transient failure is retried.
type RetryPolicy struct {
	// MaxAttempts is the total number of tries per delivery, at least 1.
	MaxAttempts int
	// BaseBackoff is the wait before the second try; it doubles before each next one.
	BaseBackoff time.Duration
}

// DefaultRetryPolicy returns the policy used unless configured otherwise.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: DefaultMaxAttempts, BaseBackoff: DefaultBaseBackoff}
}

func (p RetryPolicy) backoff(attempt int) time.Duration {
	return p.BaseBackoff << (attempt - 1)
}

// Dispatcher sends a Plan, one recipient and one channel at a time.
type Dispatcher struct {
	senders map[providers.Channel]Sender
	policy  RetryPolicy
	logger  *slog.Logger
	// sleep waits between retries; tests replace it to run instantly.
	sleep func(ctx context.Context, d time.Duration) error
}

// NewDispatcher returns a Dispatcher sending through senders, at most one per channel.
func NewDispatcher(senders []Sender, policy RetryPolicy, logger *slog.Logger) (*Dispatcher, error) {
	byChannel := make(map[providers.Channel]Sender, len(senders))
	for _, s := range senders {
		if _, dup := byChannel[s.Channel()]; dup {
			return nil, fmt.Errorf("new dispatcher: %w: %s", ErrDuplicateChannel, s.Channel())
		}
		byChannel[s.Channel()] = s
	}
	if policy.MaxAttempts < 1 {
		policy.MaxAttempts = 1
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{senders: byChannel, policy: policy, logger: logger, sleep: sleepCtx}, nil
}

// Dispatch sends every pending delivery of plan and returns the updated copy; plan
// itself is not changed. Each delivery ends sent or failed on its own: one failure
// never stops the others (CLAUDE.md rule 5). Transient errors are retried under the
// RetryPolicy; permanent and unclassified errors are not, because a retry of a message
// the gateway may have accepted would reach the person twice.
//
// When ctx is cancelled, Dispatch stops and returns the plan so far together with the
// context error. Deliveries not yet tried stay pending and can be dispatched again. A
// delivery interrupted while its provider call was in flight is left sending: the
// gateway may already have accepted it, so it waits for a delivery report or a person
// instead of being sent again.
func (d *Dispatcher) Dispatch(ctx context.Context, plan Plan) (Plan, error) {
	out := plan.clone()
	for i := range out.Recipients {
		r := &out.Recipients[i]
		for j := range r.Deliveries {
			if r.Deliveries[j].Status != StatusPending {
				continue
			}
			r.Deliveries[j] = d.send(ctx, out.Subject, r.RecipientID, r.Body, r.Deliveries[j])
			if err := ctx.Err(); err != nil {
				return out, fmt.Errorf("dispatch batch: %w", err)
			}
		}
	}
	return out, nil
}

// send tries one delivery until it succeeds, fails for good or runs out of attempts.
func (d *Dispatcher) send(ctx context.Context, subject, recipientID, body string, del Delivery) Delivery {
	sender, ok := d.senders[del.Channel]
	if !ok {
		return failed(del, fmt.Errorf("%w: %s", errNoSender, del.Channel))
	}
	msg := providers.Message{RecipientID: recipientID, To: del.To, Subject: subject, Body: body}

	for {
		del.Attempts++
		res, err := sender.Send(ctx, msg)
		if err == nil {
			del.Status = StatusSent
			del.ProviderMessageID = res.ProviderMessageID
			del.Error = ""
			return del
		}
		if ctx.Err() != nil {
			// Outcome unknown: the gateway may have the message already.
			del.Status = StatusSending
			del.Error = describe(err)
			return del
		}
		if !providers.IsTransient(err) || del.Attempts >= d.policy.MaxAttempts {
			d.logger.Warn("delivery failed",
				"recipient_id", recipientID, "channel", del.Channel,
				"attempts", del.Attempts, "reason", describe(err))
			return failed(del, err)
		}
		if err := d.sleep(ctx, d.policy.backoff(del.Attempts)); err != nil {
			// The last try failed for sure; nothing is in flight, so it may be sent again.
			del.Error = describe(err)
			return del
		}
	}
}

func failed(del Delivery, err error) Delivery {
	del.Status = StatusFailed
	del.Error = describe(err)
	return del
}

// knownCauses are the errors whose text is safe to store and show: none of them
// carries an address, a phone number or a gateway's free-text reply.
var knownCauses = []error{
	providers.ErrInvalidRecipient,
	providers.ErrGatewayTimeout,
	providers.ErrGatewayUnavailable,
	errNoSender,
	context.Canceled,
	context.DeadlineExceeded,
}

// describe turns a send error into the short reason stored in Delivery.Error. The raw
// text is not kept, because a gateway reply may quote the address or the number.
func describe(err error) string {
	kind := "unclassified"
	switch {
	case providers.IsPermanent(err):
		kind = "permanent"
	case providers.IsTransient(err):
		kind = "transient"
	}
	for _, cause := range knownCauses {
		if errors.Is(err, cause) {
			return kind + ": " + cause.Error()
		}
	}
	return kind + " provider error"
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
