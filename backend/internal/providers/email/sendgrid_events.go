package email

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// sendgridEvent mirrors the JSON structure sent by SendGrid Event Webhook.
// See: https://docs.sendgrid.com/for-developers/tracking-events/event
type sendgridEvent struct {
	Email       string `json:"email"`
	Timestamp   int64  `json:"timestamp"`
	Event       string `json:"event"`
	SGMessageID string `json:"sg_message_id"`
	Reason      string `json:"reason"`
	Response    string `json:"response"`
	Status      string `json:"status"`
}

// ParseSendGridEvents parses an HTTP request body containing a JSON array of SendGrid
// webhook events into canonical DeliveryReport structs (ADR-0008).
//
// Event mapping:
//   - "delivered"          → StatusDelivered
//   - "bounce", "dropped"  → StatusFailed (reason stored in ErrorMessage)
//   - "processed", "deferred" → StatusSent
//   - "open", "click", "spamreport", "unsubscribe" and unrecognised events
//     are engagement or informational events, not delivery statuses, and are ignored.
//
// ProviderMessageID is extracted from sg_message_id by taking the part before the
// first dot (which matches the X-Message-Id header returned by POST /v3/mail/send).
//
// Returns ErrMalformedReport if the JSON is invalid or an actionable event is missing
// sg_message_id.
func ParseSendGridEvents(body io.Reader) ([]providers.DeliveryReport, error) {
	var events []sendgridEvent
	if err := json.NewDecoder(body).Decode(&events); err != nil {
		return nil, fmt.Errorf("%w: invalid SendGrid events JSON: %w", providers.ErrMalformedReport, err)
	}

	var reports []providers.DeliveryReport
	for i, e := range events {
		status, isActionable := mapSendGridEvent(e.Event)
		if !isActionable {
			// Engagement/informational events (open, click, unsubscribe, spamreport)
			// do not update delivery status.
			continue
		}

		rawID := strings.TrimSpace(e.SGMessageID)
		if rawID == "" {
			return nil, fmt.Errorf("%w: event at index %d (%s) missing sg_message_id",
				providers.ErrMalformedReport, i, e.Event)
		}

		// SendGrid sg_message_id format: "<x-message-id>.<filter-info>".
		// Extract prefix before first dot to match X-Message-Id.
		msgID := rawID
		if idx := strings.IndexByte(rawID, '.'); idx != -1 {
			msgID = rawID[:idx]
		}

		var ts time.Time
		if e.Timestamp > 0 {
			ts = time.Unix(e.Timestamp, 0).UTC()
		}

		var errMsg string
		if status == providers.StatusFailed {
			errMsg = formatFailureReason(e)
		}

		reports = append(reports, providers.DeliveryReport{
			ProviderMessageID: msgID,
			Channel:           providers.ChannelEmail,
			Status:            status,
			ErrorMessage:      errMsg,
			Timestamp:         ts,
		})
	}

	return reports, nil
}

func mapSendGridEvent(event string) (providers.DeliveryStatus, bool) {
	switch strings.ToLower(event) {
	case "delivered":
		return providers.StatusDelivered, true
	case "bounce", "dropped":
		return providers.StatusFailed, true
	case "processed", "deferred":
		return providers.StatusSent, true
	default:
		// open, click, spamreport, unsubscribe, group_unsubscribe, group_resubscribe, unknown
		return "", false
	}
}

func formatFailureReason(e sendgridEvent) string {
	reason := strings.TrimSpace(e.Reason)
	response := strings.TrimSpace(e.Response)

	if reason != "" && response != "" {
		return fmt.Sprintf("%s (%s)", reason, response)
	}
	if reason != "" {
		return reason
	}
	if response != "" {
		return response
	}
	return fmt.Sprintf("email %s by SendGrid", e.Event)
}
