package delivery

import (
	"context"
	"errors"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// ErrBatchNotFound is returned when a batch ID matches no stored batch.
var ErrBatchNotFound = errors.New("batch not found")

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
	Groups    []GroupRef
	// RecipientIDs are the people picked by hand, outside any group.
	RecipientIDs []string
	Status       BatchStatus
	// SMSParts is the number of SMS parts the batch was planned with; CostMilli their
	// price in thousandths of a złoty (see messaging.ParsePrice).
	SMSParts   int
	CostMilli  int64
	Plan       Plan
	CreatedAt  time.Time
	FinishedAt time.Time
}

// Store keeps batches. A sent batch is immutable (backend/CLAUDE.md invariant 4):
// after CreateBatch only the delivery outcomes and the batch status change.
type Store interface {
	// CreateBatch saves b with its whole plan and returns it with ID and CreatedAt set.
	CreateBatch(ctx context.Context, b Batch) (Batch, error)
	// SaveOutcome records the dispatched plan's delivery outcomes, matched to the stored
	// deliveries by recipient and channel; nothing else of the batch changes. A status
	// only moves forward (Advance): a late or repeated save never undoes a newer one.
	// The batch status is derived from the merged deliveries, and FinishedAt is set
	// once, when the batch first finishes.
	SaveOutcome(ctx context.Context, batchID string, plan Plan) error
	// Batch returns one batch, or ErrBatchNotFound.
	Batch(ctx context.Context, id string) (Batch, error)
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
