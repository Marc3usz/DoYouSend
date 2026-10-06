package recipients

import "strings"

// Channel is a delivery channel, named as in the channel enum of the schema.
type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelSMS   Channel = "sms"
)

// IssueReason says why a channel cannot be used for a recipient.
type IssueReason string

const (
	// IssueMissing: the recipient has no e-mail address or no phone number.
	IssueMissing IssueReason = "missing"
	// IssueInvalid: the stored value would be rejected by Validate, e.g. a
	// phone number saved before normalization existed.
	IssueInvalid IssueReason = "invalid"
)

// ContactIssue is one channel a recipient cannot be reached on, and why.
// description.md requires showing these before sending, so the sender can fix
// the data, exclude the person, or send on the remaining channel only.
type ContactIssue struct {
	Channel Channel
	Reason  IssueReason
}

// ContactIssues checks stored contact data with the rules Validate applies on
// input, in the order e-mail, SMS. Stored values are expected to be normalized
// already, so a phone number not in E.164 is reported as invalid rather than
// silently fixed: delivery sends exactly what is stored.
func (r Recipient) ContactIssues() []ContactIssue {
	var issues []ContactIssue
	switch {
	case strings.TrimSpace(r.Email) == "":
		issues = append(issues, ContactIssue{Channel: ChannelEmail, Reason: IssueMissing})
	case ValidateEmail(r.Email) != nil:
		issues = append(issues, ContactIssue{Channel: ChannelEmail, Reason: IssueInvalid})
	}
	switch {
	case strings.TrimSpace(r.Phone) == "":
		issues = append(issues, ContactIssue{Channel: ChannelSMS, Reason: IssueMissing})
	case ValidatePhone(r.Phone) != nil:
		issues = append(issues, ContactIssue{Channel: ChannelSMS, Reason: IssueInvalid})
	}
	return issues
}

// HasIssue reports whether channel c cannot be used for r.
func (r Recipient) HasIssue(c Channel) bool {
	for _, is := range r.ContactIssues() {
		if is.Channel == c {
			return true
		}
	}
	return false
}
