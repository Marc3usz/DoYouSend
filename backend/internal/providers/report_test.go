package providers

import (
	"testing"
	"time"
)

func TestDeliveryStatusValues(t *testing.T) {
	t.Parallel()

	// Verify that status constants match the channel_status enum in 0001_init.sql.
	expected := map[DeliveryStatus]string{
		StatusPending:   "pending",
		StatusSending:   "sending",
		StatusSent:      "sent",
		StatusDelivered: "delivered",
		StatusFailed:    "failed",
	}

	for status, want := range expected {
		if string(status) != want {
			t.Errorf("Status%s = %q, want %q", want, status, want)
		}
	}
}

func TestDeliveryReportZeroTimestamp(t *testing.T) {
	t.Parallel()

	r := DeliveryReport{
		ProviderMessageID: "test-123",
		Channel:           ChannelSMS,
		Status:            StatusDelivered,
	}

	if !r.Timestamp.IsZero() {
		t.Errorf("zero-value Timestamp should be zero, got %v", r.Timestamp)
	}
}

func TestDeliveryReportWithTimestamp(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	r := DeliveryReport{
		ProviderMessageID: "test-456",
		Channel:           ChannelEmail,
		Status:            StatusFailed,
		ErrorMessage:      "mailbox full",
		Timestamp:         ts,
	}

	if r.Timestamp != ts {
		t.Errorf("Timestamp = %v, want %v", r.Timestamp, ts)
	}
	if r.ErrorMessage != "mailbox full" {
		t.Errorf("ErrorMessage = %q, want %q", r.ErrorMessage, "mailbox full")
	}
}
