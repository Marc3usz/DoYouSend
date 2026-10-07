package delivery

import (
	"fmt"

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
	Subject    string
	Recipients []PlannedRecipient
}

// NewPlan renders body for every resolved recipient and prepares one pending delivery
// per usable channel. A recipient listed twice is planned once (CLAUDE.md rule 6).
// A body that does not render for someone fails only that person's deliveries; the
// rest of the batch is planned as usual (CLAUDE.md rule 5).
func NewPlan(subject, body string, list []groups.Resolved) Plan {
	plan := Plan{Subject: subject, Recipients: make([]PlannedRecipient, 0, len(list))}
	seen := make(map[string]bool, len(list))
	for _, r := range list {
		if seen[r.Recipient.ID] {
			continue
		}
		seen[r.Recipient.ID] = true
		plan.Recipients = append(plan.Recipients, planRecipient(body, r))
	}
	return plan
}

func planRecipient(body string, r groups.Resolved) PlannedRecipient {
	pr := PlannedRecipient{
		RecipientID: r.Recipient.ID,
		Partial:     r.Partial(),
		Unreachable: r.Unreachable(),
	}

	rendered, renderErr := messaging.Render(body, messaging.RecipientFields(r.Recipient.FirstName, r.Recipient.LastName))
	if renderErr == nil && rendered == "" {
		renderErr = messaging.ErrEmptyBody
	}
	if renderErr == nil {
		pr.Body = rendered
	}

	for _, c := range r.Channels() {
		d := Delivery{Channel: providers.Channel(c), Status: StatusPending, Parts: 1}
		switch c {
		case groups.ChannelEmail:
			d.To = r.Recipient.Email
		case groups.ChannelSMS:
			d.To = r.Recipient.Phone
			d.Parts = messaging.MeasureSMS(pr.Body).Parts
		}
		if renderErr != nil {
			d.Status = StatusFailed
			d.Error = fmt.Sprintf("render body: %v", renderErr)
		}
		pr.Deliveries = append(pr.Deliveries, d)
	}
	return pr
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
	out := Plan{Subject: p.Subject, Recipients: make([]PlannedRecipient, len(p.Recipients))}
	for i, r := range p.Recipients {
		r.Deliveries = append([]Delivery(nil), r.Deliveries...)
		out.Recipients[i] = r
	}
	return out
}
