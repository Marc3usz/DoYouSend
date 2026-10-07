package delivery

import "github.com/Marc3usz/DoYouSend/backend/internal/providers"

// Status is the state of one delivery: one recipient on one channel
// (deliveries.status, channel_status enum in 0001_init.sql).
type Status = providers.DeliveryStatus

const (
	StatusPending   = providers.StatusPending
	StatusSending   = providers.StatusSending
	StatusSent      = providers.StatusSent
	StatusDelivered = providers.StatusDelivered
	StatusFailed    = providers.StatusFailed
)

// BatchStatus mirrors the batch_status enum in 0001_init.sql.
type BatchStatus string

const (
	BatchDraft          BatchStatus = "draft"
	BatchScheduled      BatchStatus = "scheduled"
	BatchRunning        BatchStatus = "running"
	BatchDone           BatchStatus = "done"
	BatchDoneWithErrors BatchStatus = "done_with_errors"
	BatchCancelled      BatchStatus = "cancelled"
)

// statusRank orders the statuses a delivery moves through. Delivered and failed
// share the top rank: both are final.
var statusRank = map[Status]int{
	StatusPending:   0,
	StatusSending:   1,
	StatusSent:      2,
	StatusDelivered: 3,
	StatusFailed:    3,
}

// IsFinal reports whether s can no longer change through sending or reports.
func IsFinal(s Status) bool {
	return s == StatusDelivered || s == StatusFailed
}

// Advance returns the status a delivery in current should have after an update to
// next, e.g. from a delivery report. Updates only move forward
// (pending -> sending -> sent -> delivered / failed); a late or duplicate report
// never downgrades a delivery, and a final status never changes
// (providers.DeliveryReport contract). Unknown statuses are ignored.
func Advance(current, next Status) Status {
	cur, okCur := statusRank[current]
	nxt, okNext := statusRank[next]
	if !okCur || !okNext || IsFinal(current) || nxt <= cur {
		return current
	}
	return next
}
