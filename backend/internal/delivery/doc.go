// Package delivery owns dispatching a confirmed batch and tracking its outcome.
//
// Scope (see description.md "Wysylka wiadomosci" and "Statusy wysylki"):
//   - queueing one job per recipient per channel,
//   - per-recipient, per-channel status: pending, sending, sent, delivered, failed,
//   - batch status: draft, scheduled, running, done, done_with_errors, cancelled,
//   - partial sends when a recipient lacks one of the contact details,
//   - retry of failed messages, delivery-report ingestion,
//   - history of every batch: author, time, groups, final recipient list, errors.
//
// Invariants: one recipient is never messaged twice in one batch; a single recipient's
// failure never aborts the batch; a sent message is immutable (copy it to reuse it).
//
// Owner: DEV B. Uses providers through an interface - never calls a vendor SDK directly.
package delivery
