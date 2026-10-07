//go:build integration

package providers

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database/dbtest"
)

func setupDeliveryFixtures(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// Insert test user
	_, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, full_name, role, password_hash)
		VALUES ('11111111-1111-1111-1111-111111111111', 'sender@example.test', 'Test Sender', 'sender', 'hash')
	`)
	if err != nil {
		t.Fatalf("insert user fixture: %v", err)
	}

	// Insert test recipient
	_, err = pool.Exec(ctx, `
		INSERT INTO recipients (id, first_name, last_name, email, phone, type)
		VALUES ('22222222-2222-2222-2222-222222222222', 'Jan', 'Kowalski', 'jan@example.test', '+48500100101', 'parent')
	`)
	if err != nil {
		t.Fatalf("insert recipient fixture: %v", err)
	}

	// Insert test batch
	_, err = pool.Exec(ctx, `
		INSERT INTO message_batches (id, subject, body, status, created_by)
		VALUES ('33333333-3333-3333-3333-333333333333', 'subj', 'body', 'running', '11111111-1111-1111-1111-111111111111')
	`)
	if err != nil {
		t.Fatalf("insert message_batches fixture: %v", err)
	}

	// Insert batch_recipient
	_, err = pool.Exec(ctx, `
		INSERT INTO batch_recipients (id, batch_id, recipient_id, rendered_body)
		VALUES ('44444444-4444-4444-4444-444444444444', '33333333-3333-3333-3333-333333333333', '22222222-2222-2222-2222-222222222222', 'body')
	`)
	if err != nil {
		t.Fatalf("insert batch_recipient fixture: %v", err)
	}

	// Insert deliveries (with same ID on both channels to verify channel isolation)
	_, err = pool.Exec(ctx, `
		INSERT INTO deliveries (id, batch_recipient_id, channel, status, provider_message_id)
		VALUES 
			('55555555-5555-5555-5555-555555555551', '44444444-4444-4444-4444-444444444444', 'sms', 'pending', 'common-msg-id'),
			('55555555-5555-5555-5555-555555555552', '44444444-4444-4444-4444-444444444444', 'email', 'pending', 'common-msg-id')
	`)
	if err != nil {
		t.Fatalf("insert deliveries fixtures: %v", err)
	}
}

func getDeliveryStatusAndError(t *testing.T, pool *pgxpool.Pool, channel string, providerMessageID string) (string, *string) {
	t.Helper()
	var status string
	var errText *string
	err := pool.QueryRow(context.Background(), `
		SELECT status, error
		FROM deliveries
		WHERE channel = $1 AND provider_message_id = $2
	`, channel, providerMessageID).Scan(&status, &errText)
	if err != nil {
		t.Fatalf("getDeliveryStatusAndError query failed for channel %s, id %s: %v", channel, providerMessageID, err)
	}
	return status, errText
}

func TestPGDeliveryReportConsumer_ForwardOnlyTransitions(t *testing.T) {
	pool := dbtest.New(t)
	setupDeliveryFixtures(t, pool)

	consumer := NewPGDeliveryReportConsumer(pool)
	ctx := context.Background()

	// 1. Channel isolation: update SMS common-msg-id to sent. Email must remain pending!
	reports := []DeliveryReport{
		{ProviderMessageID: "common-msg-id", Channel: ChannelSMS, Status: StatusSent},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume sent report: %v", err)
	}
	smsStatus, _ := getDeliveryStatusAndError(t, pool, "sms", "common-msg-id")
	if smsStatus != "sent" {
		t.Errorf("sms status = %q, want 'sent'", smsStatus)
	}
	emailStatus, _ := getDeliveryStatusAndError(t, pool, "email", "common-msg-id")
	if emailStatus != "pending" {
		t.Errorf("email status = %q, want 'pending' (channel isolation violated)", emailStatus)
	}

	// 2. Out-of-order intermediate event: pending/sending must NOT downgrade sent!
	reports = []DeliveryReport{
		{ProviderMessageID: "common-msg-id", Channel: ChannelSMS, Status: StatusSending},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume sending report: %v", err)
	}
	smsStatus, _ = getDeliveryStatusAndError(t, pool, "sms", "common-msg-id")
	if smsStatus != "sent" {
		t.Errorf("sms status = %q, want 'sent' (downgraded to sending)", smsStatus)
	}

	// 3. Update SMS: sent -> delivered
	reports = []DeliveryReport{
		{ProviderMessageID: "common-msg-id", Channel: ChannelSMS, Status: StatusDelivered},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume delivered report: %v", err)
	}
	smsStatus, _ = getDeliveryStatusAndError(t, pool, "sms", "common-msg-id")
	if smsStatus != "delivered" {
		t.Errorf("sms status = %q, want 'delivered'", smsStatus)
	}

	// 4. Stale event for delivered message: sent must NOT downgrade delivered!
	reports = []DeliveryReport{
		{ProviderMessageID: "common-msg-id", Channel: ChannelSMS, Status: StatusSent},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume late sent report: %v", err)
	}
	smsStatus, _ = getDeliveryStatusAndError(t, pool, "sms", "common-msg-id")
	if smsStatus != "delivered" {
		t.Errorf("sms status = %q, want 'delivered' (must not downgrade terminal state)", smsStatus)
	}

	// 5. Update email: pending -> failed with error message
	reports = []DeliveryReport{
		{
			ProviderMessageID: "common-msg-id",
			Channel:           ChannelEmail,
			Status:            StatusFailed,
			ErrorMessage:      "bounce: mailbox rejected the message",
		},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume failed report: %v", err)
	}
	emailStatus, emailErr := getDeliveryStatusAndError(t, pool, "email", "common-msg-id")
	if emailStatus != "failed" {
		t.Errorf("email status = %q, want 'failed'", emailStatus)
	}
	if emailErr == nil || *emailErr != "bounce: mailbox rejected the message" {
		t.Errorf("email error = %v, want 'bounce: mailbox rejected the message'", emailErr)
	}

	// 6. Late delivered event for email: delivered must NOT overwrite terminal failed!
	reports = []DeliveryReport{
		{ProviderMessageID: "common-msg-id", Channel: ChannelEmail, Status: StatusDelivered},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume late delivered report: %v", err)
	}
	emailStatus, _ = getDeliveryStatusAndError(t, pool, "email", "common-msg-id")
	if emailStatus != "failed" {
		t.Errorf("email status = %q, want 'failed' (must not overwrite terminal failed)", emailStatus)
	}

	// 7. Unknown provider message ID should not error
	reports = []DeliveryReport{
		{ProviderMessageID: "unknown-id", Channel: ChannelSMS, Status: StatusDelivered},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume unknown ID report: %v", err)
	}
}
