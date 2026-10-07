//go:build integration

package sms

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database/dbtest"
)

func setupUsageFixtures(t *testing.T, pool *pgxpool.Pool) (string, string) {
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

	// Insert test recipients
	_, err = pool.Exec(ctx, `
		INSERT INTO recipients (id, first_name, last_name, email, phone, type)
		VALUES 
			('22222222-2222-2222-2222-222222222221', 'Jan', 'Kowalski', 'jan@example.test', '+48500100101', 'parent'),
			('22222222-2222-2222-2222-222222222222', 'Anna', 'Nowak', 'anna@example.test', '+48500100102', 'parent'),
			('22222222-2222-2222-2222-222222222223', 'Piotr', 'Wisniewski', 'piotr@example.test', '+48500100103', 'student'),
			('22222222-2222-2222-2222-222222222224', 'Ewa', 'Dabrowska', 'ewa@example.test', '+48500100104', 'student')
	`)
	if err != nil {
		t.Fatalf("insert recipient fixtures: %v", err)
	}

	batch1ID := "33333333-3333-3333-3333-333333333331"
	batch2ID := "33333333-3333-3333-3333-333333333332"

	// Insert test message_batches
	_, err = pool.Exec(ctx, `
		INSERT INTO message_batches (id, subject, body, status, created_by, created_at, confirmed_at)
		VALUES 
			($1, 'Subject 1', 'body1', 'done', '11111111-1111-1111-1111-111111111111', '2026-10-01 10:00:00+00', '2026-10-01 10:05:00+00'),
			($2, 'Subject 2', 'body2', 'done', '11111111-1111-1111-1111-111111111111', '2026-10-05 12:00:00+00', '2026-10-05 12:01:00+00')
	`, batch1ID, batch2ID)
	if err != nil {
		t.Fatalf("insert message_batches fixtures: %v", err)
	}

	// Insert batch_recipients
	br1 := "44444444-4444-4444-4444-444444444441"
	br2 := "44444444-4444-4444-4444-444444444442"
	br3 := "44444444-4444-4444-4444-444444444443"
	br4 := "44444444-4444-4444-4444-444444444444"
	br5 := "44444444-4444-4444-4444-444444444445"

	_, err = pool.Exec(ctx, `
		INSERT INTO batch_recipients (id, batch_id, recipient_id, rendered_body)
		VALUES 
			($1, $6, '22222222-2222-2222-2222-222222222221', 'body1'),
			($2, $6, '22222222-2222-2222-2222-222222222222', 'body1'),
			($3, $6, '22222222-2222-2222-2222-222222222223', 'body1'),
			($4, $6, '22222222-2222-2222-2222-222222222224', 'body1'),
			($5, $7, '22222222-2222-2222-2222-222222222221', 'body2')
	`, br1, br2, br3, br4, br5, batch1ID, batch2ID)
	if err != nil {
		t.Fatalf("insert batch_recipients fixtures: %v", err)
	}

	// Insert deliveries respecting UNIQUE (batch_recipient_id, channel)
	_, err = pool.Exec(ctx, `
		INSERT INTO deliveries (id, batch_recipient_id, channel, status, parts, provider_message_id)
		VALUES 
			('55555555-5555-5555-5555-555555555551', $1, 'sms', 'delivered', 2, 'sms-1'),
			('55555555-5555-5555-5555-555555555552', $2, 'sms', 'sent', 3, 'sms-2'),
			('55555555-5555-5555-5555-555555555553', $3, 'sms', 'failed', 1, 'sms-3'),
			('55555555-5555-5555-5555-555555555554', $4, 'sms', 'pending', 1, 'sms-4'),
			('55555555-5555-5555-5555-555555555555', $4, 'email', 'delivered', 1, 'email-5'),
			('55555555-5555-5555-5555-555555555556', $5, 'sms', 'delivered', 2, 'sms-6')
	`, br1, br2, br3, br4, br5)
	if err != nil {
		t.Fatalf("insert deliveries fixtures: %v", err)
	}

	return batch1ID, batch2ID
}

func TestPGUsageStore_Aggregation(t *testing.T) {
	pool := dbtest.New(t)
	batch1ID, _ := setupUsageFixtures(t, pool)

	store := NewPGUsageStore(pool)
	ctx := context.Background()

	// 1. All batches aggregate
	counts, err := store.GetUsage(ctx, UsageFilter{})
	if err != nil {
		t.Fatalf("GetUsage(all): %v", err)
	}

	// 5 SMS messages total (email is ignored)
	if counts.TotalMessages != 5 {
		t.Errorf("TotalMessages = %d, want 5", counts.TotalMessages)
	}
	// Parts: 2 (delivered) + 3 (sent) + 1 (failed) + 1 (pending) + 2 (delivered) = 9
	if counts.TotalParts != 9 {
		t.Errorf("TotalParts = %d, want 9", counts.TotalParts)
	}
	if counts.DeliveredMessages != 2 {
		t.Errorf("DeliveredMessages = %d, want 2", counts.DeliveredMessages)
	}
	if counts.DeliveredParts != 4 {
		t.Errorf("DeliveredParts = %d, want 4", counts.DeliveredParts)
	}
	if counts.SentMessages != 1 || counts.SentParts != 3 {
		t.Errorf("Sent = (%d, %d), want (1, 3)", counts.SentMessages, counts.SentParts)
	}
	if counts.FailedMessages != 1 || counts.FailedParts != 1 {
		t.Errorf("Failed = (%d, %d), want (1, 1)", counts.FailedMessages, counts.FailedParts)
	}
	if counts.InFlightMessages != 1 || counts.InFlightParts != 1 {
		t.Errorf("InFlight = (%d, %d), want (1, 1)", counts.InFlightMessages, counts.InFlightParts)
	}

	// 2. Filter by Batch 1 only
	batchCounts, err := store.GetUsage(ctx, UsageFilter{BatchID: &batch1ID})
	if err != nil {
		t.Fatalf("GetUsage(batch1): %v", err)
	}
	if batchCounts.TotalMessages != 4 {
		t.Errorf("batch1 TotalMessages = %d, want 4", batchCounts.TotalMessages)
	}
	if batchCounts.TotalParts != 7 {
		t.Errorf("batch1 TotalParts = %d, want 7", batchCounts.TotalParts)
	}
	if batchCounts.DeliveredMessages != 1 || batchCounts.DeliveredParts != 2 {
		t.Errorf("batch1 Delivered = (%d, %d), want (1, 2)", batchCounts.DeliveredMessages, batchCounts.DeliveredParts)
	}

	// 3. Filter by date range (batch1 was created on 2026-10-01, batch2 on 2026-10-05)
	from := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	rangeCounts, err := store.GetUsage(ctx, UsageFilter{From: &from, To: &to, ToExclusive: true})
	if err != nil {
		t.Fatalf("GetUsage(range): %v", err)
	}
	// Only batch2 falls in this range (1 message, 2 parts)
	if rangeCounts.TotalMessages != 1 || rangeCounts.TotalParts != 2 {
		t.Errorf("range counts = %+v, want 1 message and 2 parts", rangeCounts)
	}

	// 4. Filter by future date range
	future := time.Now().Add(24 * time.Hour)
	futureCounts, err := store.GetUsage(ctx, UsageFilter{From: &future})
	if err != nil {
		t.Fatalf("GetUsage(future): %v", err)
	}
	if futureCounts.TotalMessages != 0 || futureCounts.TotalParts != 0 {
		t.Errorf("future counts = %+v, want 0", futureCounts)
	}
}
