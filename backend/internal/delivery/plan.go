package delivery

import (
	"fmt"
	"slices"

	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
	"github.com/Marc3usz/DoYouSend/backend/internal/messaging"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// Delivery is one recipient on one channel (a deliveries row).
type Delivery struct {
	Channel providers.Channel
	// To is the e-mail address or E.164 phone number. Never log it.
	To     string
	Status Status
	// Parts is the number of SMS messages, 1 for e-mail.
	Parts             int
	Attempts          int
	ProviderMessageID string
	// Error says why the delivery failed, as a short reason built by describe or by
	// rendering; it never contains the address, the number or the body.
	Error string
}

// PlannedRecipient is one person of a batch (a batch_recipients row) with the body
// rendered for them and one Delivery per usable channel.
type PlannedRecipient struct {
	RecipientID string
	// FirstName and LastName are for the history. PGStore reads the current ones.
	FirstName, LastName string
	// Body is sent byte for byte over every channel (CLAUDE.md rule 4).
	Body string
	// Partial is true when only one channel can be used (batch_recipients.is_partial).
	Partial bool
	// Unreachable is true when no channel can be used; Deliveries is then empty.
	Unreachable bool
	Deliveries  []Delivery
}

// Plan is a batch ready to dispatch: everything to send, nothing sent yet.
type Plan struct {
	// Subject is the e-mail subject; SMS has none.
	Subject string
	// From and ReplyTo are the e-mail sender chosen for the batch (ADR-0021).
	// Zero values leave the provider's configured sender.
	From, ReplyTo providers.Address
	Recipients    []PlannedRecipient
}

// NewPlan renders body for every resolved recipient and prepares one pending delivery
// per usable channel among targetChannels (defaulting to both e-mail and SMS).
// A recipient listed twice is planned once (CLAUDE.md rule 6).
// A body that does not render for someone fails only that person's deliveries; the
// rest of the batch is planned as usual (CLAUDE.md rule 5).
func NewPlan(subject, body string, list []groups.Resolved, targetChannels ...providers.Channel) Plan {
	targets := targetChannels
	if len(targets) == 0 {
		targets = []providers.Channel{providers.ChannelEmail, providers.ChannelSMS}
	}
	plan := Plan{Subject: subject, Recipients: make([]PlannedRecipient, 0, len(list))}
	seen := make(map[string]bool, len(list))
	for _, r := range list {
		if seen[r.Recipient.ID] {
			continue
		}
		seen[r.Recipient.ID] = true
		plan.Recipients = append(plan.Recipients, planRecipient(body, r, targets))
	}
	return plan
}

func planRecipient(body string, r groups.Resolved, targets []providers.Channel) PlannedRecipient {
	hasEmail := slices.Contains(targets, providers.ChannelEmail) && r.CanReach(groups.ChannelEmail)
	hasPhone := slices.Contains(targets, providers.ChannelSMS) && r.CanReach(groups.ChannelSMS)

	usableCount := 0
	if hasEmail {
		usableCount++
	}
	if hasPhone {
		usableCount++
	}

	isPartial := len(targets) > 1 && usableCount == 1
	isUnreachable := usableCount == 0

	pr := PlannedRecipient{
		RecipientID: r.Recipient.ID,
		FirstName:   r.Recipient.FirstName,
		LastName:    r.Recipient.LastName,
		Partial:     isPartial,
		Unreachable: isUnreachable,
	}

	rendered, renderErr := messaging.Render(body, messaging.RecipientFields(r.Recipient.FirstName, r.Recipient.LastName))
	if renderErr == nil && rendered == "" {
		renderErr = messaging.ErrEmptyBody
	}
	if renderErr == nil {
		pr.Body = rendered
	}

	for _, c := range targets {
		if (c == providers.ChannelEmail && !hasEmail) || (c == providers.ChannelSMS && !hasPhone) {
			continue
		}
		d := Delivery{Channel: c, Status: StatusPending, Parts: 1}
		switch c {
		case providers.ChannelEmail:
			d.To = r.Recipient.Email
		case providers.ChannelSMS:
			d.To = r.Recipient.Phone
			d.Parts = messaging.MeasureSMS(pr.Body).Parts
		}
		if renderErr != nil {
			d.Status = StatusFailed
			d.Error = fmt.Sprintf("%s: %v", reasonRender, renderErr)
		}
		pr.Deliveries = append(pr.Deliveries, d)
	}
	return pr
}

// limitSMSParts fails, before anything is sent, every SMS longer than maxParts
// parts (0: no limit): the operator would refuse it anyway (SMSAPI max_parts),
// and the body is never shortened (CLAUDE.md rule 4). The e-mail still goes.
func (p *Plan) limitSMSParts(maxParts int) {
	if maxParts <= 0 {
		return
	}
	for i := range p.Recipients {
		for j := range p.Recipients[i].Deliveries {
			d := &p.Recipients[i].Deliveries[j]
			if d.Channel == providers.ChannelSMS && d.Status == StatusPending && d.Parts > maxParts {
				d.Status = StatusFailed
				d.Error = fmt.Sprintf("%s: %d parts, at most %d allowed", reasonTooLong, d.Parts, maxParts)
			}
		}
	}
}

// Status summarises the batch: running while any delivery is pending or sending (its
// outcome unknown), then done, or done_with_errors when a delivery failed or someone
// was unreachable.
// Reports arriving later (sent -> delivered) do not reopen a finished batch.
func (p Plan) Status() BatchStatus {
	status := BatchDone
	for _, r := range p.Recipients {
		if r.Unreachable {
			status = BatchDoneWithErrors
		}
		for _, d := range r.Deliveries {
			switch d.Status {
			case StatusPending, StatusSending:
				return BatchRunning
			case StatusFailed:
				status = BatchDoneWithErrors
			}
		}
	}
	return status
}

// clone returns a deep copy, so dispatching never changes the caller's plan.
func (p Plan) clone() Plan {
	out := Plan{Subject: p.Subject, From: p.From, ReplyTo: p.ReplyTo, Recipients: make([]PlannedRecipient, len(p.Recipients))}
	for i, r := range p.Recipients {
		r.Deliveries = append([]Delivery(nil), r.Deliveries...)
		out.Recipients[i] = r
	}
	return out
}

// RetryPlan returns a copy of p where the failed deliveries a retry may send again
// (see RetryClassOf) are back in StatusPending, ready to be dispatched. Uncertain
// ones, whose message the gateway may already have, are included only with
// includeUncertain. Sent and delivered deliveries are never touched, so a retry
// sends nobody a message twice and costs nothing twice (CLAUDE.md rule 5).
// Attempts and the last error stay, so the history shows what happened; the error
// is cleared once the delivery succeeds. It returns the new plan and how many
// deliveries were reset.
func (p Plan) RetryPlan(includeUncertain bool) (Plan, int) {
	out := p.clone()
	retried := 0
	for i := range out.Recipients {
		for j := range out.Recipients[i].Deliveries {
			d := &out.Recipients[i].Deliveries[j]
			if d.Status == StatusFailed && retries(d.Error, includeUncertain) {
				// The old message ID goes: a late report about the previous try
				// must not overwrite the new one.
				d.Status, d.ProviderMessageID = StatusPending, ""
				retried++
			}
		}
	}
	return out, retried
}

// retries reports whether a retry sends a failed delivery with this reason again.
func retries(reason string, includeUncertain bool) bool {
	switch RetryClassOf(reason) {
	case RetrySafe:
		return true
	case RetryUncertain:
		return includeUncertain
	default:
		return false
	}
}

// RetryCounts returns how many failed deliveries a plain retry sends again, and how
// many more it sends when asked to include the uncertain ones.
func (p Plan) RetryCounts() (retryable, uncertain int) {
	for _, r := range p.Recipients {
		for _, d := range r.Deliveries {
			if d.Status != StatusFailed {
				continue
			}
			switch RetryClassOf(d.Error) {
			case RetrySafe:
				retryable++
			case RetryUncertain:
				uncertain++
			}
		}
	}
	return retryable, uncertain
}
