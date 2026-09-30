package sms

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// SMSAPI DLR (Delivery Report) webhook payload.
//
// SMSAPI sends a POST request to the configured callback URL with a JSON body
// containing the delivery status. See https://www.smsapi.pl/docs (Raport doręczeń).
//
// Example payload:
//
//	{
//	  "id": "abc123",
//	  "status": "DELIVERED",
//	  "date": 1727600000
//	}

// smsapiDLRPayload is the raw JSON structure of an SMSAPI DLR webhook callback.
type smsapiDLRPayload struct {
	// ID is the SMSAPI message identifier, stored as ProviderMessageID when
	// the message was sent.
	ID string `json:"id"`

	// Status is the SMSAPI delivery status string:
	// DELIVERED, UNDELIVERED, EXPIRED, REJECTED, UNKNOWN, QUEUE, SENT.
	Status string `json:"status"`

	// Date is the Unix timestamp when SMSAPI observed the status change.
	// Zero when the status is intermediate (QUEUE, SENT).
	Date int64 `json:"date"`
}

// smsapiStatusMap maps SMSAPI status strings to canonical DeliveryStatus values.
// Statuses not in this map are treated as intermediate/informational and produce
// StatusSent (the message is still in flight).
var smsapiStatusMap = map[string]providers.DeliveryStatus{
	"DELIVERED":   providers.StatusDelivered,
	"UNDELIVERED": providers.StatusFailed,
	"EXPIRED":     providers.StatusFailed,
	"REJECTED":    providers.StatusFailed,
	"SENT":        providers.StatusSent,
	"QUEUE":       providers.StatusSending,
}

// smsapiErrorMessages provides human-readable error reasons for failed statuses.
var smsapiErrorMessages = map[string]string{
	"UNDELIVERED": "message could not be delivered to the handset",
	"EXPIRED":     "delivery timed out, the message expired before reaching the handset",
	"REJECTED":    "message rejected by the operator or the recipient number is invalid",
}

// ParseSMSAPIDLR parses an SMSAPI DLR webhook body into a canonical
// DeliveryReport. The body should be a JSON object with at least "id" and
// "status" fields.
//
// Returns ErrMalformedReport if the JSON is invalid or required fields are missing.
func ParseSMSAPIDLR(body io.Reader) (providers.DeliveryReport, error) {
	var p smsapiDLRPayload
	if err := json.NewDecoder(body).Decode(&p); err != nil {
		return providers.DeliveryReport{}, fmt.Errorf(
			"%w: invalid JSON: %w", providers.ErrMalformedReport, err,
		)
	}

	if p.ID == "" {
		return providers.DeliveryReport{}, fmt.Errorf(
			"%w: missing required field \"id\"", providers.ErrMalformedReport,
		)
	}
	if p.Status == "" {
		return providers.DeliveryReport{}, fmt.Errorf(
			"%w: missing required field \"status\"", providers.ErrMalformedReport,
		)
	}

	status, ok := smsapiStatusMap[p.Status]
	if !ok {
		// Unknown status — treat as "still in flight" rather than failing.
		status = providers.StatusSent
	}

	var ts time.Time
	if p.Date > 0 {
		ts = time.Unix(p.Date, 0).UTC()
	}

	return providers.DeliveryReport{
		ProviderMessageID: p.ID,
		Channel:           providers.ChannelSMS,
		Status:            status,
		ErrorMessage:      smsapiErrorMessages[p.Status],
		Timestamp:         ts,
	}, nil
}
