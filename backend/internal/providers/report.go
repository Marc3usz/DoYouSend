package providers

import (
	"errors"
	"time"
)

// DeliveryStatus mirrors the channel_status enum in the database
// (migrations/0001_init.sql). The delivery worker writes these values
// to deliveries.status based on reports from the provider.
type DeliveryStatus string

const (
	StatusPending   DeliveryStatus = "pending"
	StatusSending   DeliveryStatus = "sending"
	StatusSent      DeliveryStatus = "sent"
	StatusDelivered DeliveryStatus = "delivered"
	StatusFailed    DeliveryStatus = "failed"
)

// DeliveryReport is the canonical representation of a delivery status
// notification received from an external provider (e.g. an SMSAPI webhook
// or an SMTP DSN). The delivery worker uses it to update
// deliveries.status and deliveries.error.
//
// Provider-specific parsers (e.g. sms.ParseSMSAPIDLR) convert the raw
// payload into this struct so that the delivery package never depends on
// a vendor format directly (backend/CLAUDE.md: interfaces, not SDKs).
type DeliveryReport struct {
	// ProviderMessageID is the external identifier assigned when the
	// message was sent (Result.ProviderMessageID). It is the join key
	// between the sent message and the status callback.
	ProviderMessageID string

	// Channel identifies which channel delivered (or failed to deliver)
	// the message.
	Channel Channel

	// Status is the canonical status to write to deliveries.status.
	Status DeliveryStatus

	// ErrorMessage is a human-readable reason when Status is StatusFailed.
	// Empty on success. Stored in deliveries.error.
	ErrorMessage string

	// Timestamp is when the provider observed the status change.
	// Zero if the provider does not supply one.
	Timestamp time.Time
}

// ErrMalformedReport is returned when a provider-specific payload cannot
// be parsed into a DeliveryReport (missing fields, bad JSON, etc.).
var ErrMalformedReport = errors.New("malformed delivery report")
