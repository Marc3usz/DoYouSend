//go:build integration

package email

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
	br6 := "44444444-4444-4444-4444-444444444446"

	_, err = pool.Exec(ctx, `
		INSERT INTO batch_recipients (id, batch_id, recipient_id, rendered_body)
		VALUES 
			($1, $7, '22222222-2222-2222-2222-222222222221', 'body1'),
			($2, $7, '22222222-2222-2222-2222-222222222222', 'body1'),
			($3, $7, '22222222-2222-2222-2222-222222222223', 'body1'),
			($4, $7, '22222222-2222-2222-2222-222222222224', 'body1'),
			($5, $8, '22222222-2222-2222-2222-222222222221', 'body2'),
			($6, $8, '22222222-2222-2222-2222-222222222222', 'body2')
	`, br1, br2, br3, br4, br5, br6, batch1ID, batch2ID)
	if err != nil {
		t.Fatalf("insert batch_recipients fixtures: %v", err)
	}

	// Insert deliveries respecting UNIQUE (batch_recipient_id, channel)
	_, err = pool.Exec(ctx, `
		INSERT INTO deliveries (id, batch_recipient_id, channel, status, parts, provider_message_id)
		VALUES 
			('55555555-5555-5555-5555-555555555551', $1, 'email', 'delivered', 1, 'email-1'),
			('55555555-5555-5555-5555-555555555552', $2, 'email', 'sent', 1, 'email-2'),
			('55555555-5555-5555-5555-555555555553', $3, 'email', 'failed', 1, 'email-3'),
			('55555555-5555-5555-5555-555555555554', $4, 'email', 'pending', 1, 'email-4'),
			('55555555-5555-5555-5555-555555555555', $4, 'sms', 'delivered', 2, 'sms-5'),
			('55555555-5555-5555-5555-555555555556', $5, 'email', 'delivered', 1, 'email-6'),
			('55555555-5555-5555-5555-555555555557', $6, 'email', 'failed', 1, NULL)
	`, br1, br2, br3, br4, br5, br6)
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

	// 1. Overall stats (no filter)
	stats, err := store.GetUsage(ctx, UsageFilter{})
	if err != nil {
		t.Fatalf("GetUsage without filter failed: %v", err)
	}

	if stats.TotalMessages != 6 {
		t.Errorf("TotalMessages = %d, want 6", stats.TotalMessages)
	}
	if stats.DeliveredMessages != 2 {
		t.Errorf("DeliveredMessages = %d, want 2", stats.DeliveredMessages)
	}
	if stats.SentMessages != 1 {
		t.Errorf("SentMessages = %d, want 1", stats.SentMessages)
	}
	if stats.FailedMessages != 2 {
		t.Errorf("FailedMessages = %d, want 2", stats.FailedMessages)
	}
	if stats.InFlightMessages != 1 {
		t.Errorf("InFlightMessages = %d, want 1", stats.InFlightMessages)
	}

	// 2. Filter by BatchID (Batch 1 only)
	b1Stats, err := store.GetUsage(ctx, UsageFilter{BatchID: &batch1ID})
	if err != nil {
		t.Fatalf("GetUsage for batch 1 failed: %v", err)
	}
	if b1Stats.TotalMessages != 4 {
		t.Errorf("Batch 1 TotalMessages = %d, want 4", b1Stats.TotalMessages)
	}
	if b1Stats.DeliveredMessages != 1 {
		t.Errorf("Batch 1 DeliveredMessages = %d, want 1", b1Stats.DeliveredMessages)
	}
	if b1Stats.SentMessages != 1 {
		t.Errorf("Batch 1 SentMessages = %d, want 1", b1Stats.SentMessages)
	}
	if b1Stats.FailedMessages != 1 {
		t.Errorf("Batch 1 FailedMessages = %d, want 1", b1Stats.FailedMessages)
	}
	if b1Stats.InFlightMessages != 1 {
		t.Errorf("Batch 1 InFlightMessages = %d, want 1", b1Stats.InFlightMessages)
	}

	// 3. Filter by time range (Batch 2 only: 2026-10-04 to 2026-10-06)
	from := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	b2Stats, err := store.GetUsage(ctx, UsageFilter{From: &from, To: &to})
	if err != nil {
		t.Fatalf("GetUsage with time range failed: %v", err)
	}
	if b2Stats.TotalMessages != 2 {
		t.Errorf("Batch 2 TotalMessages = %d, want 2", b2Stats.TotalMessages)
	}
	if b2Stats.DeliveredMessages != 1 {
		t.Errorf("Batch 2 DeliveredMessages = %d, want 1", b2Stats.DeliveredMessages)
	}
	if b2Stats.FailedMessages != 1 {
		t.Errorf("Batch 2 FailedMessages = %d, want 1", b2Stats.FailedMessages)
	}
}
