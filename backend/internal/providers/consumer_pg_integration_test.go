//go:build integration

package providers

import (
	"context"
	"testing"
	"time"

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

	consumer := NewPGDeliveryReportConsumer(pool, nil)
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

	// 7. Unknown provider message ID should not error, but buffer into unmatched_delivery_reports
	reports = []DeliveryReport{
		{ProviderMessageID: "unknown-id", Channel: ChannelSMS, Status: StatusDelivered},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume unknown ID report: %v", err)
	}

	var unmatchedCount int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM unmatched_delivery_reports WHERE provider_message_id = 'unknown-id'`).Scan(&unmatchedCount)
	if err != nil {
		t.Fatalf("query unmatched report: %v", err)
	}
	if unmatchedCount != 1 {
		t.Errorf("unmatchedCount = %d, want 1", unmatchedCount)
	}
}

func TestPGDeliveryReportConsumer_UnmatchedReportsBufferedAndReconciled(t *testing.T) {
	pool := dbtest.New(t)
	setupDeliveryFixtures(t, pool)

	consumer := NewPGDeliveryReportConsumer(pool, nil)
	ctx := context.Background()

	// 1. Report arrives BEFORE delivery has provider_message_id in DB (e.g. during dispatch)
	earlyID := "early-dispatch-msg-id"
	reports := []DeliveryReport{
		{ProviderMessageID: earlyID, Channel: ChannelSMS, Status: StatusDelivered},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume early report: %v", err)
	}

	// 2. Verify report is buffered in unmatched_delivery_reports
	var bufferedStatus string
	err := pool.QueryRow(ctx, `SELECT status FROM unmatched_delivery_reports WHERE provider_message_id = $1 AND channel = 'sms'`, earlyID).Scan(&bufferedStatus)
	if err != nil {
		t.Fatalf("query buffered report: %v", err)
	}
	if bufferedStatus != "delivered" {
		t.Errorf("bufferedStatus = %q, want delivered", bufferedStatus)
	}

	// 3. Update delivery with this provider_message_id (simulating dispatch outcome save)
	existingDeliveryID := "55555555-5555-5555-5555-555555555551"
	_, err = pool.Exec(ctx, `
		UPDATE deliveries
		SET provider_message_id = $1, status = 'sent'
		WHERE id = $2
	`, earlyID, existingDeliveryID)
	if err != nil {
		t.Fatalf("update delivery with provider_message_id: %v", err)
	}

	// 4. Trigger should have applied the buffered report and upgraded status to delivered
	var deliveryStatus string
	err = pool.QueryRow(ctx, `SELECT status FROM deliveries WHERE id = $1`, existingDeliveryID).Scan(&deliveryStatus)
	if err != nil {
		t.Fatalf("query delivery status: %v", err)
	}
	if deliveryStatus != "delivered" {
		t.Errorf("deliveryStatus = %q, want delivered (trigger should have reconciled buffered report)", deliveryStatus)
	}

	// 5. Buffer table entry should now be deleted
	var remainingCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM unmatched_delivery_reports WHERE provider_message_id = $1`, earlyID).Scan(&remainingCount)
	if err != nil {
		t.Fatalf("query remaining unmatched reports: %v", err)
	}
	if remainingCount != 0 {
		t.Errorf("remainingCount = %d, want 0 after reconciliation", remainingCount)
	}
}

func TestPGDeliveryReportConsumer_FailedReportUpdatesBatchToDoneWithErrors(t *testing.T) {
	pool := dbtest.New(t)
	setupDeliveryFixtures(t, pool)

	consumer := NewPGDeliveryReportConsumer(pool, nil)
	ctx := context.Background()

	// 1. Mark batch as 'done' and delivery as 'sent'
	batchID := "33333333-3333-3333-3333-333333333333"
	_, err := pool.Exec(ctx, `UPDATE message_batches SET status = 'done' WHERE id = $1`, batchID)
	if err != nil {
		t.Fatalf("update batch to done: %v", err)
	}
	_, err = pool.Exec(ctx, `UPDATE deliveries SET status = 'sent' WHERE provider_message_id = 'common-msg-id' AND channel = 'sms'`)
	if err != nil {
		t.Fatalf("update delivery to sent: %v", err)
	}

	// 2. Late failed report arrives after batch completion
	reports := []DeliveryReport{
		{
			ProviderMessageID: "common-msg-id",
			Channel:           ChannelSMS,
			Status:            StatusFailed,
			ErrorMessage:      "rejected: phone unreachable",
		},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume late failed report: %v", err)
	}

	// 3. Verify delivery status became failed
	smsStatus, errText := getDeliveryStatusAndError(t, pool, "sms", "common-msg-id")
	if smsStatus != "failed" {
		t.Errorf("sms status = %q, want failed", smsStatus)
	}
	if errText == nil || *errText != "rejected: phone unreachable" {
		t.Errorf("error = %v, want 'rejected: phone unreachable'", errText)
	}

	// 4. Verify batch status in message_batches changed from 'done' to 'done_with_errors'
	var batchStatus string
	err = pool.QueryRow(ctx, `SELECT status FROM message_batches WHERE id = $1`, batchID).Scan(&batchStatus)
	if err != nil {
		t.Fatalf("query batch status: %v", err)
	}
	if batchStatus != "done_with_errors" {
		t.Errorf("batchStatus = %q, want done_with_errors", batchStatus)
	}
}

func TestPGDeliveryReportConsumer_LateFailedOnDeliveredDoesNotChangeDoneBatch(t *testing.T) {
	pool := dbtest.New(t)
	setupDeliveryFixtures(t, pool)

	consumer := NewPGDeliveryReportConsumer(pool, nil)
	ctx := context.Background()

	batchID := "33333333-3333-3333-3333-333333333333"
	_, err := pool.Exec(ctx, `UPDATE message_batches SET status = 'done' WHERE id = $1`, batchID)
	if err != nil {
		t.Fatalf("update batch to done: %v", err)
	}
	_, err = pool.Exec(ctx, `UPDATE deliveries SET status = 'delivered' WHERE provider_message_id = 'common-msg-id' AND channel = 'sms'`)
	if err != nil {
		t.Fatalf("update delivery to delivered: %v", err)
	}

	// Late failed report for an already-delivered message (e.g. spurious bounce)
	reports := []DeliveryReport{
		{
			ProviderMessageID: "common-msg-id",
			Channel:           ChannelSMS,
			Status:            StatusFailed,
			ErrorMessage:      "late bounce",
		},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume late failed report: %v", err)
	}

	// Delivery should remain 'delivered' (forward-only lifecycle)
	smsStatus, errText := getDeliveryStatusAndError(t, pool, "sms", "common-msg-id")
	if smsStatus != "delivered" {
		t.Errorf("sms status = %q, want delivered", smsStatus)
	}
	if errText != nil {
		t.Errorf("expected nil error for delivered status, got %v", *errText)
	}

	// Batch status should remain 'done', not regress to 'done_with_errors'
	var batchStatus string
	err = pool.QueryRow(ctx, `SELECT status FROM message_batches WHERE id = $1`, batchID).Scan(&batchStatus)
	if err != nil {
		t.Fatalf("query batch status: %v", err)
	}
	if batchStatus != "done" {
		t.Errorf("batchStatus = %q, want 'done' (should not change to done_with_errors)", batchStatus)
	}
}

func TestPGDeliveryReportConsumer_BufferedFailedReportReconciled(t *testing.T) {
	pool := dbtest.New(t)
	setupDeliveryFixtures(t, pool)

	consumer := NewPGDeliveryReportConsumer(pool, nil)
	ctx := context.Background()

	earlyFailedID := "early-failed-msg-id"
	reports := []DeliveryReport{
		{
			ProviderMessageID: earlyFailedID,
			Channel:           ChannelSMS,
			Status:            StatusFailed,
			ErrorMessage:      "carrier reject",
		},
	}
	if err := consumer.ConsumeDeliveryReports(ctx, reports); err != nil {
		t.Fatalf("consume early failed report: %v", err)
	}

	// Verify report is buffered
	var bufferedStatus string
	var bufferedErr *string
	err := pool.QueryRow(ctx, `
		SELECT status, error FROM unmatched_delivery_reports WHERE provider_message_id = $1 AND channel = 'sms'`,
		earlyFailedID).Scan(&bufferedStatus, &bufferedErr)
	if err != nil {
		t.Fatalf("query buffered report: %v", err)
	}
	if bufferedStatus != "failed" || bufferedErr == nil || *bufferedErr != "carrier reject" {
		t.Errorf("bufferedStatus = %q, err = %v; want failed, 'carrier reject'", bufferedStatus, bufferedErr)
	}

	// Associate delivery with provider_message_id
	existingDeliveryID := "55555555-5555-5555-5555-555555555551"
	_, err = pool.Exec(ctx, `
		UPDATE deliveries
		SET provider_message_id = $1, status = 'sent'
		WHERE id = $2
	`, earlyFailedID, existingDeliveryID)
	if err != nil {
		t.Fatalf("update delivery with provider_message_id: %v", err)
	}

	// Trigger should have reconciled buffered failed status and error
	var deliveryStatus string
	var deliveryErr *string
	err = pool.QueryRow(ctx, `SELECT status, error FROM deliveries WHERE id = $1`, existingDeliveryID).Scan(&deliveryStatus, &deliveryErr)
	if err != nil {
		t.Fatalf("query delivery status: %v", err)
	}
	if deliveryStatus != "failed" || deliveryErr == nil || *deliveryErr != "carrier reject" {
		t.Errorf("deliveryStatus = %q, err = %v; want failed, 'carrier reject'", deliveryStatus, deliveryErr)
	}

	// Buffer entry should be deleted
	var count int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM unmatched_delivery_reports WHERE provider_message_id = $1`, earlyFailedID).Scan(&count)
	if err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != 0 {
		t.Errorf("unmatched count = %d, want 0", count)
	}
}

func TestPGDeliveryReportConsumer_PruneUnmatchedReports(t *testing.T) {
	pool := dbtest.New(t)
	setupDeliveryFixtures(t, pool)

	consumer := NewPGDeliveryReportConsumer(pool, nil)
	ctx := context.Background()

	// Insert an old unmatched report (10 days old)
	_, err := pool.Exec(ctx, `
		INSERT INTO unmatched_delivery_reports (provider_message_id, channel, status, created_at, updated_at)
		VALUES ('old-unmatched-id', 'sms', 'delivered', now() - interval '10 days', now() - interval '10 days')
	`)
	if err != nil {
		t.Fatalf("insert old unmatched report: %v", err)
	}

	// Insert a recent unmatched report (1 hour old)
	_, err = pool.Exec(ctx, `
		INSERT INTO unmatched_delivery_reports (provider_message_id, channel, status, created_at, updated_at)
		VALUES ('recent-unmatched-id', 'sms', 'delivered', now() - interval '1 hour', now() - interval '1 hour')
	`)
	if err != nil {
		t.Fatalf("insert recent unmatched report: %v", err)
	}

	// Prune older than 7 days
	deleted, err := consumer.PruneUnmatchedReports(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("PruneUnmatchedReports error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	// Old should be gone, recent should remain
	var recentCount, oldCount int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM unmatched_delivery_reports WHERE provider_message_id = 'old-unmatched-id'`).Scan(&oldCount)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM unmatched_delivery_reports WHERE provider_message_id = 'recent-unmatched-id'`).Scan(&recentCount)
	if oldCount != 0 {
		t.Errorf("oldCount = %d, want 0", oldCount)
	}
	if recentCount != 1 {
		t.Errorf("recentCount = %d, want 1", recentCount)
	}
}
