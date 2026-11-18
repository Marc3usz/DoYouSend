package delivery

import (
	"context"
	"errors"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// Sentinel errors for batch operations.
var (
	// ErrBatchNotFound is returned when a batch ID matches no stored batch.
	ErrBatchNotFound = errors.New("batch not found")
	// ErrDuplicateKey is returned by CreateBatch when the sender already has a batch
	// with the same idempotency key.
	ErrDuplicateKey = errors.New("idempotency key already used")
	// ErrBatchRunning is returned when attempting to retry a batch that is currently running.
	ErrBatchRunning = errors.New("batch is already running")
	// ErrNoFailedDeliveries is returned when a batch has no failed deliveries to retry.
	ErrNoFailedDeliveries = errors.New("batch has no failed deliveries to retry")
)

// GroupRef is a selected group as it was named when the batch was sent. History keeps
// the name because a class group disappears once its class is empty (ADR-0009), and a
// custom group can be renamed or deleted. Stored in message_batches.selected_groups.
type GroupRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Batch is one confirmed send: what was sent, by whom, to whom, and how it went
// (message_batches with its batch_recipients and deliveries).
type Batch struct {
	ID string
	// Subject is the e-mail subject; Body is the template before personalisation.
	Subject   string
	Body      string
	CreatedBy string
	// CreatedByName is the sender's full name. PGStore reads the current one from users.
	CreatedByName string
	// IdempotencyKey is the client's Idempotency-Key, empty when there was none;
	// RequestHash fingerprints the request it came with (see requestHash).
	IdempotencyKey string
	RequestHash    string
	Groups         []GroupRef
	// RecipientIDs are the people picked by hand, outside any group.
	RecipientIDs []string
	// ExcludedRecipientIDs are the people explicitly excluded from this send.
	ExcludedRecipientIDs []string
	Status               BatchStatus
	// Channels are the channels the sender chose (ADR-0019): both, or one of them.
	Channels []providers.Channel
	// SMSParts is the number of SMS parts the batch was planned with; CostMilli their
	// price in thousandths of a złoty (see messaging.ParsePrice).
	SMSParts  int
	CostMilli int64
	// Plan carries the e-mail From and Reply-To chosen for the batch (ADR-0021).
	Plan Plan
	// Counts summarises Plan; ListBatches fills it without loading the plan.
	Counts     Counts
	CreatedAt  time.Time
	FinishedAt time.Time
	// RetryCount is how many times the failed deliveries were sent again, the
	// last time at LastRetryAt.
	RetryCount  int
	LastRetryAt time.Time
}

// Counts is the history summary of a batch (BatchSummary in openapi.yaml).
type Counts struct {
	Recipients int
	// Partial recipients were reachable on one channel only.
	Partial int
	// Failed recipients had a failed delivery or no usable channel at all.
	Failed int
}

// BatchFilter selects a page of history, newest first.
type BatchFilter struct {
	// CreatedBy limits the history to one sender; empty means everyone's batches.
	CreatedBy string
	// Status limits the history to one batch status; empty means any.
	Status BatchStatus
	Limit  int
	Offset int
}

// Store keeps batches. A sent batch is immutable (backend/CLAUDE.md invariant 4):
// after CreateBatch only the delivery outcomes and the batch status change.
type Store interface {
	// CreateBatch saves b with its whole plan and returns it with ID and CreatedAt set.
	// A key the sender already used fails with ErrDuplicateKey and saves nothing.
	CreateBatch(ctx context.Context, b Batch) (Batch, error)
	// SaveOutcome records the dispatched plan's delivery outcomes, matched to the stored
	// deliveries by recipient and channel; nothing else of the batch changes. A status
	// only moves forward (Advance): a late or repeated save never undoes a newer one.
	// The batch status is derived from the merged deliveries, and FinishedAt is set
	// once, when the batch first finishes.
	SaveOutcome(ctx context.Context, batchID string, plan Plan) error
	// SaveDelivery records an incremental delivery progress update for one recipient and
	// channel, enforcing the forward-only Advance lifecycle.
	SaveDelivery(ctx context.Context, batchID string, recipientID string, d Delivery) error
	// Batch returns one batch with its plan and Counts, or ErrBatchNotFound.
	Batch(ctx context.Context, id string) (Batch, error)
	// BatchByKey returns the sender's batch created with an idempotency key, or
	// ErrBatchNotFound.
	BatchByKey(ctx context.Context, createdBy, key string) (Batch, error)
	// ListBatches returns one page of batches without their plans, newest first,
	// and how many batches match f in total.
	ListBatches(ctx context.Context, f BatchFilter) ([]Batch, int, error)
	// ResetFailedDeliveries puts the failed deliveries of batchID that a retry sends
	// again (Plan.RetryPlan) back to pending, refreshing their address from the
	// recipient's current contact, counts the retry, marks the batch running and
	// returns it ready to dispatch. ErrBatchRunning while it is being sent,
	// ErrNoFailedDeliveries when there is nothing to send again.
	ResetFailedDeliveries(ctx context.Context, batchID string, includeUncertain bool) (Batch, error)
	// RunningBatchIDs returns the batches whose status is running, oldest first.
	RunningBatchIDs(ctx context.Context) ([]string, error)
	// InterruptSending marks the deliveries of batchID still in sending as failed
	// with an uncertain reason: the process sending them stopped, and the gateway
	// may or may not have the message.
	InterruptSending(ctx context.Context, batchID string) error
}

// Counts summarises the plan for the history list.
func (p Plan) Counts() Counts {
	c := Counts{Recipients: len(p.Recipients)}
	for _, r := range p.Recipients {
		if r.Partial {
			c.Partial++
		}
		failed := r.Unreachable
		for _, d := range r.Deliveries {
			failed = failed || d.Status == StatusFailed
		}
		if failed {
			c.Failed++
		}
	}
	return c
}

// smsParts sums the SMS parts the plan will send: deliveries that failed while
// planning (the body did not render) cost nothing.
func (p Plan) smsParts() int {
	total := 0
	for _, r := range p.Recipients {
		for _, d := range r.Deliveries {
			if d.Channel == providers.ChannelSMS && d.Status != StatusFailed {
				total += d.Parts
			}
		}
	}
	return total
}
