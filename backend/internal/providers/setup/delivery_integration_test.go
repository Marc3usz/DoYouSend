//go:build integration

package setup_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Marc3usz/DoYouSend/backend/internal/delivery"
	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database/dbtest"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers/setup"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers/sms"
	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

func TestDeliveryPipeline_EndToEnd_Integration(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()

	// 1. Initialize providers and dispatcher via setup
	provs, err := setup.New(setup.Config{
		EmailProvider: "mailpit",
		SMSProvider:   "fake",
		DryRun:        true,
	}, nil)
	if err != nil {
		t.Fatalf("setup.New = %v", err)
	}

	dispatcher, err := provs.Dispatcher(delivery.DefaultRetryPolicy(), nil)
	if err != nil {
		t.Fatalf("provs.Dispatcher = %v", err)
	}

	// 2. Insert DB fixtures
	senderID := "11111111-1111-1111-1111-111111111111"
	recipientID := "22222222-2222-2222-2222-222222222222"
	batchID := "33333333-3333-3333-3333-333333333333"
	batchRecipientID := "44444444-4444-4444-4444-444444444444"
	deliverySMSID := "55555555-5555-5555-5555-555555555551"
	deliveryEmailID := "55555555-5555-5555-5555-555555555552"

	setupFixtures(t, pool, senderID, recipientID, batchID, batchRecipientID, deliverySMSID, deliveryEmailID)

	// 3. Plan and dispatch messages
	recipientsList := []groups.Resolved{
		{
			Recipient: recipients.Recipient{
				ID:        recipientID,
				FirstName: "Jan",
				LastName:  "Kowalski",
				Email:     "jan@example.test",
				Phone:     "+48500100101",
				Type:      recipients.TypeParent,
			},
		},
	}

	plan := delivery.NewPlan("Zebranie", "Wiadomosc do {{imie}}", recipientsList)
	dispatchedPlan, err := dispatcher.Dispatch(ctx, plan)
	if err != nil {
		t.Fatalf("dispatcher.Dispatch = %v", err)
	}

	var smsMsgID, emailMsgID string
	for _, r := range dispatchedPlan.Recipients {
		for _, d := range r.Deliveries {
			if d.Channel == providers.ChannelSMS {
				smsMsgID = d.ProviderMessageID
				if d.Status != delivery.StatusSent || smsMsgID == "" {
					t.Errorf("SMS delivery status = %v, msgID = %q; want sent with non-empty ID", d.Status, smsMsgID)
				}
			}
			if d.Channel == providers.ChannelEmail {
				emailMsgID = d.ProviderMessageID
				if d.Status != delivery.StatusSent || emailMsgID == "" {
					t.Errorf("Email delivery status = %v, msgID = %q; want sent with non-empty ID", d.Status, emailMsgID)
				}
			}
		}
	}

	// 4. Update deliveries in database with provider message IDs (simulating outcome saving)
	_, err = pool.Exec(ctx, `
		UPDATE deliveries SET status = 'sent', provider_message_id = $1 WHERE id = $2
	`, smsMsgID, deliverySMSID)
	if err != nil {
		t.Fatalf("update sms delivery: %v", err)
	}

	_, err = pool.Exec(ctx, `
		UPDATE deliveries SET status = 'sent', provider_message_id = $1 WHERE id = $2
	`, emailMsgID, deliveryEmailID)
	if err != nil {
		t.Fatalf("update email delivery: %v", err)
	}

	// 5. Simulate incoming delivery reports via PGDeliveryReportConsumer
	reportConsumer := providers.NewPGDeliveryReportConsumer(pool)

	smsReports := []providers.DeliveryReport{
		{
			Channel:           providers.ChannelSMS,
			ProviderMessageID: smsMsgID,
			Status:            providers.StatusDelivered,
		},
	}
	if err := reportConsumer.ConsumeDeliveryReports(ctx, smsReports); err != nil {
		t.Fatalf("consume SMS delivery report: %v", err)
	}

	emailReports := []providers.DeliveryReport{
		{
			Channel:           providers.ChannelEmail,
			ProviderMessageID: emailMsgID,
			Status:            providers.StatusDelivered,
		},
	}
	if err := reportConsumer.ConsumeDeliveryReports(ctx, emailReports); err != nil {
		t.Fatalf("consume Email delivery report: %v", err)
	}

	// 6. Verify deliveries table status in DB
	var smsStatus, emailStatus string
	err = pool.QueryRow(ctx, `SELECT status FROM deliveries WHERE id = $1`, deliverySMSID).Scan(&smsStatus)
	if err != nil {
		t.Fatalf("query sms status: %v", err)
	}
	if smsStatus != "delivered" {
		t.Errorf("sms status = %q, want delivered", smsStatus)
	}

	err = pool.QueryRow(ctx, `SELECT status FROM deliveries WHERE id = $1`, deliveryEmailID).Scan(&emailStatus)
	if err != nil {
		t.Fatalf("query email status: %v", err)
	}
	if emailStatus != "delivered" {
		t.Errorf("email status = %q, want delivered", emailStatus)
	}

	// 7. Verify SMS usage statistics aggregation
	usageStore := sms.NewPGUsageStore(pool)
	counts, err := usageStore.GetUsage(ctx, sms.UsageFilter{})
	if err != nil {
		t.Fatalf("usageStore.GetUsage = %v", err)
	}

	if counts.TotalMessages != 1 {
		t.Errorf("TotalMessages = %d, want 1", counts.TotalMessages)
	}
	if counts.DeliveredMessages != 1 {
		t.Errorf("DeliveredMessages = %d, want 1", counts.DeliveredMessages)
	}
	if counts.BilledParts != 1 {
		t.Errorf("BilledParts = %d, want 1", counts.BilledParts)
	}

	stats := sms.CalculateUsageStats(counts, 80)
	if stats.BilledCostMilli != 80 {
		t.Errorf("BilledCostMilli = %d, want 80", stats.BilledCostMilli)
	}
	if stats.DeliveredCostMilli != 80 {
		t.Errorf("DeliveredCostMilli = %d, want 80", stats.DeliveredCostMilli)
	}
}

func setupFixtures(t *testing.T, pool *pgxpool.Pool, senderID, recipientID, batchID, batchRecipientID, deliverySMSID, deliveryEmailID string) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, full_name, role, password_hash)
		VALUES ($1, 'sender@example.test', 'Sender', 'sender', 'hash')
	`, senderID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO recipients (id, first_name, last_name, email, phone, type)
		VALUES ($1, 'Jan', 'Kowalski', 'jan@example.test', '+48500100101', 'parent')
	`, recipientID)
	if err != nil {
		t.Fatalf("insert recipient: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO message_batches (id, subject, body, status, created_by, created_at, confirmed_at)
		VALUES ($1, 'Zebranie', 'Wiadomosc do {{imie}}', 'done', $2, now(), now())
	`, batchID, senderID)
	if err != nil {
		t.Fatalf("insert message_batches: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO batch_recipients (id, batch_id, recipient_id, rendered_body)
		VALUES ($1, $2, $3, 'Wiadomosc do Jan')
	`, batchRecipientID, batchID, recipientID)
	if err != nil {
		t.Fatalf("insert batch_recipients: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO deliveries (id, batch_recipient_id, channel, status, parts)
		VALUES 
			($1, $3, 'sms', 'pending', 1),
			($2, $3, 'email', 'pending', 1)
	`, deliverySMSID, deliveryEmailID, batchRecipientID)
	if err != nil {
		t.Fatalf("insert deliveries: %v", err)
	}
}
