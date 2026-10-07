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
		INSERT INTO batches (id, title, sms_body, email_body, email_subject, status, channels, created_by)
		VALUES ('33333333-3333-3333-3333-333333333333', 'Test Batch', 'body', 'body', 'subj', 'running', '{sms,email}', '11111111-1111-1111-1111-111111111111')
	`)
	if err != nil {
		t.Fatalf("insert batch fixture: %v", err)
	}

	// Insert batch_recipient
	_, err = pool.Exec(ctx, `
		INSERT INTO batch_recipients (id, batch_id, recipient_id, rendered_body)
		VALUES ('44444444-4444-4444-4444-444444444444', '33333333-3333-3333-3333-333333333333', '22222222-2222-2222-2222-222222222222', 'body')
	`)
	if err != nil {
		t.Fatalf("insert batch_recipient fixture: %v", err)
	}

	// Insert deliveries
	_, err = pool.Exec(ctx, `
		INSERT INTO deliveries (id, batch_recipient_id, channel, status, provider_message_id)
		VALUES 
			('55555555-5555-5555-5555-555555555551', '44444444-4444-4444-4444-444444444444', 'sms', 'pending', 'sms-msg-1'),
			('55555555-5555-5555-5555-555555555552', '44444444-4444-4444-4444-444444444444', 'email', 'pending', 'email-msg-2')
	`)
	if err != nil {
		t.Fatalf("insert deliveries fixtures: %v", err)
	}
}

func getDeliveryStatusAndError(t *testing.T, pool *pgxpool.Pool, providerMessageID string) (string, *string) {
	t.Helper()
	var status string
	var errText *string
	err := pool.QueryRow(context.Background(), `
		SELECT status, error
		FROM deliveries
		WHERE provider_message_id = $1
	`, providerMessageID).Scan(&status, &errText)
	if err != nil {
		t.Fatalf("getDeliveryStatusAndError query failed for %s: %v", providerMessageID, err)
	}
	return status, errText
}

func TestPGDeliveryReportConsumer_ForwardOnlyTransitions(t *testing.T) {
	pool := dbtest.New(t)
	setupDeliveryFixtures(t, pool)

	consumer := NewPGDeliveryReportConsumer(pool)
	ctx := context.Background()

	// 1. Update sms-msg-1: pending -> sent
	reports := []DeliveryReport{
		{ProviderMessageID: "sms-msg-1", Channel: ChannelSMS, Status: StatusSent},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume sent report: %v", err)
	}
	st, errMsg := getDeliveryStatusAndError(t, pool, "sms-msg-1")
	if st != "sent" {
		t.Errorf("status = %q, want 'sent'", st)
	}
	if errMsg != nil {
		t.Errorf("error = %v, want nil", errMsg)
	}

	// 2. Update sms-msg-1: sent -> delivered
	reports = []DeliveryReport{
		{ProviderMessageID: "sms-msg-1", Channel: ChannelSMS, Status: StatusDelivered},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume delivered report: %v", err)
	}
	st, errMsg = getDeliveryStatusAndError(t, pool, "sms-msg-1")
	if st != "delivered" {
		t.Errorf("status = %q, want 'delivered'", st)
	}

	// 3. Late/out-of-order event for sms-msg-1: sent should NOT downgrade delivered!
	reports = []DeliveryReport{
		{ProviderMessageID: "sms-msg-1", Channel: ChannelSMS, Status: StatusSent},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume late sent report: %v", err)
	}
	st, errMsg = getDeliveryStatusAndError(t, pool, "sms-msg-1")
	if st != "delivered" {
		t.Errorf("status = %q, want 'delivered' (must not downgrade terminal state)", st)
	}

	// 4. Update email-msg-2: pending -> failed with error message
	reports = []DeliveryReport{
		{
			ProviderMessageID: "email-msg-2",
			Channel:           ChannelEmail,
			Status:            StatusFailed,
			ErrorMessage:      "550 User unknown",
		},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume failed report: %v", err)
	}
	st, errMsg = getDeliveryStatusAndError(t, pool, "email-msg-2")
	if st != "failed" {
		t.Errorf("status = %q, want 'failed'", st)
	}
	if errMsg == nil || *errMsg != "550 User unknown" {
		t.Errorf("error = %v, want '550 User unknown'", errMsg)
	}

	// 5. Late delivered event for email-msg-2: delivered must NOT overwrite terminal failed!
	reports = []DeliveryReport{
		{ProviderMessageID: "email-msg-2", Channel: ChannelEmail, Status: StatusDelivered},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume late delivered report: %v", err)
	}
	st, errMsg = getDeliveryStatusAndError(t, pool, "email-msg-2")
	if st != "failed" {
		t.Errorf("status = %q, want 'failed' (must not overwrite terminal failed)", st)
	}
	if errMsg == nil || *errMsg != "550 User unknown" {
		t.Errorf("error = %v, want '550 User unknown'", errMsg)
	}

	// 6. Unknown provider message ID should not error
	reports = []DeliveryReport{
		{ProviderMessageID: "unknown-id", Channel: ChannelSMS, Status: StatusDelivered},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume unknown ID report: %v", err)
	}
}
