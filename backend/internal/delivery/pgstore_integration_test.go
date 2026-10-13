//go:build integration

package delivery

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database/dbtest"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

const (
	pgBoth  = "00000000-0000-4000-8000-000000000001"
	pgEmail = "00000000-0000-4000-8000-000000000002"
	pgSMS   = "00000000-0000-4000-8000-000000000003"
	pgNone  = "00000000-0000-4000-8000-000000000004"
)

// newPGStore returns a PGStore over a migrated schema holding the sender and the
// fictional recipients the test plans use.
func newPGStore(t *testing.T) (*PGStore, *pgxpool.Pool) {
	t.Helper()
	pool := dbtest.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, full_name, role, password_hash)
		VALUES ($1, 'nadawca@example.test', 'Nadawca Testowy', 'sender', 'x')`, userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	for i, id := range []string{pgBoth, pgEmail, pgSMS, pgNone} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO recipients (id, first_name, last_name, phone, type)
			VALUES ($1, 'Osoba', 'Testowa', $2, 'parent')`, id, "+4850010010"+string(rune('1'+i))); err != nil {
			t.Fatalf("insert recipient %s: %v", id, err)
		}
	}
	return NewPGStore(pool), pool
}

func testPlan() Plan {
	return NewPlan("Zebranie", "Czesc {{imie}}", []groups.Resolved{
		resolved(pgBoth, "Anna", "anna@example.test", "+48500100101"),
		resolved(pgEmail, "Jan", "jan@example.test", ""),
		resolved(pgSMS, "", "", "+48500100103"), // no first name: deliveries fail at planning
		resolved(pgNone, "Ewa", "", ""),
	})
}

func testBatch() Batch {
	plan := testPlan()
	parts := plan.smsParts()
	return Batch{
		Subject:      "Zebranie",
		Body:         "Czesc {{imie}}",
		CreatedBy:    userID,
		Groups:       []GroupRef{{ID: groupID, Name: "Rodzice uczniów klasy 3A"}},
		RecipientIDs: []string{pgEmail},
		Status:       BatchRunning,
		SMSParts:     parts,
		CostMilli:    int64(parts) * 65, // 0.065 zł a part: needs the third decimal
		Plan:         plan,
	}
}

// outcome returns plan with every pending delivery set to status.
func outcome(plan Plan, status Status) Plan {
	out := plan.clone()
	for i, r := range out.Recipients {
		for j, d := range r.Deliveries {
			if d.Status == StatusPending {
				d.Status, d.Attempts, d.ProviderMessageID = status, 1, "msg-"+r.RecipientID+"-"+string(d.Channel)
				out.Recipients[i].Deliveries[j] = d
			}
		}
	}
	return out
}

func TestPGStoreCreateAndReadBatch(t *testing.T) {
	store, _ := newPGStore(t)
	ctx := context.Background()
	want := testBatch()

	created, err := store.CreateBatch(ctx, want)
	if err != nil {
		t.Fatalf("CreateBatch() error = %v", err)
	}
	if !groups.IsValidID(created.ID) || created.CreatedAt.IsZero() {
		t.Fatalf("CreateBatch() = ID %q, CreatedAt %v; want both set", created.ID, created.CreatedAt)
	}

	got, err := store.Batch(ctx, created.ID)
	if err != nil {
		t.Fatalf("Batch() error = %v", err)
	}
	want.ID, want.CreatedAt = created.ID, got.CreatedAt
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Batch() =\n%+v\nwant\n%+v", got, want)
	}
	if !got.Plan.Recipients[3].Unreachable || got.Plan.Recipients[2].Body != "" {
		t.Errorf("unreachable / unrendered recipients not kept: %+v", got.Plan.Recipients)
	}
}

func TestPGStoreBatchWithoutGroupsOrHandPicked(t *testing.T) {
	store, _ := newPGStore(t)
	ctx := context.Background()
	b := testBatch()
	b.Groups, b.RecipientIDs = nil, nil

	created, err := store.CreateBatch(ctx, b)
	if err != nil {
		t.Fatalf("CreateBatch() error = %v", err)
	}
	got, err := store.Batch(ctx, created.ID)
	if err != nil {
		t.Fatalf("Batch() error = %v", err)
	}
	if len(got.Groups) != 0 || len(got.RecipientIDs) != 0 {
		t.Errorf("Batch() groups %v, recipient IDs %v; want none", got.Groups, got.RecipientIDs)
	}
}

func TestPGStoreCreateBatchIsAtomic(t *testing.T) {
	store, pool := newPGStore(t)
	ctx := context.Background()
	b := testBatch()
	b.Plan.Recipients[1].RecipientID = "00000000-0000-4000-8000-0000000000ff" // no such recipient

	if _, err := store.CreateBatch(ctx, b); err == nil {
		t.Fatal("CreateBatch() error = nil; want the foreign key violation")
	}
	var batches int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM message_batches`).Scan(&batches); err != nil || batches != 0 {
		t.Errorf("message_batches rows = %d, %v; want 0 after a failed create", batches, err)
	}
}

func TestPGStoreSaveOutcome(t *testing.T) {
	store, _ := newPGStore(t)
	ctx := context.Background()
	created, err := store.CreateBatch(ctx, testBatch())
	if err != nil {
		t.Fatalf("CreateBatch() error = %v", err)
	}

	sent := outcome(created.Plan, StatusSent)
	if err := store.SaveOutcome(ctx, created.ID, sent); err != nil {
		t.Fatalf("SaveOutcome() error = %v", err)
	}
	got, err := store.Batch(ctx, created.ID)
	if err != nil {
		t.Fatalf("Batch() error = %v", err)
	}
	if !reflect.DeepEqual(got.Plan, sent) {
		t.Errorf("plan after save =\n%+v\nwant\n%+v", got.Plan, sent)
	}
	// One recipient was unreachable and one could not be rendered.
	if got.Status != BatchDoneWithErrors || got.FinishedAt.IsZero() {
		t.Errorf("batch = %s finished %v; want done_with_errors with FinishedAt", got.Status, got.FinishedAt)
	}

	// A repeated save keeps the moment the batch first finished.
	if err := store.SaveOutcome(ctx, created.ID, sent); err != nil {
		t.Fatalf("second SaveOutcome() error = %v", err)
	}
	again, _ := store.Batch(ctx, created.ID)
	if !again.FinishedAt.Equal(got.FinishedAt) {
		t.Errorf("FinishedAt moved from %v to %v", got.FinishedAt, again.FinishedAt)
	}
}

func TestPGStoreSaveOutcomeStaysRunning(t *testing.T) {
	store, _ := newPGStore(t)
	ctx := context.Background()
	created, err := store.CreateBatch(ctx, testBatch())
	if err != nil {
		t.Fatalf("CreateBatch() error = %v", err)
	}
	// Interrupted mid-call: the outcome is unknown until a delivery report comes.
	if err := store.SaveOutcome(ctx, created.ID, outcome(created.Plan, StatusSending)); err != nil {
		t.Fatalf("SaveOutcome() error = %v", err)
	}
	got, _ := store.Batch(ctx, created.ID)
	if got.Status != BatchRunning || !got.FinishedAt.IsZero() {
		t.Errorf("batch = %s finished %v; want running, not finished", got.Status, got.FinishedAt)
	}
}

func TestPGStoreSaveOutcomeKeepsNewerReport(t *testing.T) {
	store, pool := newPGStore(t)
	ctx := context.Background()
	created, err := store.CreateBatch(ctx, testBatch())
	if err != nil {
		t.Fatalf("CreateBatch() error = %v", err)
	}
	// A delivery report lands before the dispatcher's outcome is saved.
	if _, err := pool.Exec(ctx, `
		UPDATE deliveries d SET status = 'delivered', provider_message_id = 'dlr-1'
		FROM batch_recipients br
		WHERE d.batch_recipient_id = br.id AND br.recipient_id = $1 AND d.channel = 'sms'`, pgBoth); err != nil {
		t.Fatalf("simulate report: %v", err)
	}

	if err := store.SaveOutcome(ctx, created.ID, outcome(created.Plan, StatusSent)); err != nil {
		t.Fatalf("SaveOutcome() error = %v", err)
	}
	got, _ := store.Batch(ctx, created.ID)
	for _, d := range got.Plan.Recipients[0].Deliveries {
		switch d.Channel {
		case providers.ChannelSMS:
			if d.Status != StatusDelivered || d.ProviderMessageID != "dlr-1" {
				t.Errorf("sms = %s %q; want the delivered report kept", d.Status, d.ProviderMessageID)
			}
		case providers.ChannelEmail:
			if d.Status != StatusSent {
				t.Errorf("email = %s; want sent", d.Status)
			}
		}
	}
}

func TestPGStoreUnknownBatch(t *testing.T) {
	store, _ := newPGStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, id := range []string{"00000000-0000-4000-8000-0000000000ee", "not-a-uuid"} {
		if _, err := store.Batch(ctx, id); !errors.Is(err, ErrBatchNotFound) {
			t.Errorf("Batch(%q) error = %v; want ErrBatchNotFound", id, err)
		}
		if err := store.SaveOutcome(ctx, id, Plan{}); !errors.Is(err, ErrBatchNotFound) {
			t.Errorf("SaveOutcome(%q) error = %v; want ErrBatchNotFound", id, err)
		}
	}
}

func TestPGStoreWithService(t *testing.T) {
	store, _ := newPGStore(t)
	sms := &scriptedSender{channel: providers.ChannelSMS}
	res := groups.Resolution{Recipients: []groups.Resolved{
		resolved(pgBoth, "Anna", "anna@example.test", "+48500100101"),
		resolved(pgSMS, "Ola", "", "+48500100103"),
	}}
	svc := newTestService(t, res, store, sms)

	sent, err := svc.Send(context.Background(), validDraft())
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	got, err := store.Batch(context.Background(), sent.ID)
	if err != nil {
		t.Fatalf("Batch() error = %v", err)
	}
	if got.Status != BatchDone || len(got.Groups) != 2 || got.CostMilli != int64(got.SMSParts)*65 {
		t.Errorf("stored batch = %s, groups %v, cost %d for %d parts", got.Status, got.Groups, got.CostMilli, got.SMSParts)
	}
}

func TestPGStoreInterruptedDispatchLeavesProgress(t *testing.T) {
	store, pool := newPGStore(t)
	ctx := context.Background()

	// Batch with two recipients: pgSMS and pgEmail
	res := groups.Resolution{Recipients: []groups.Resolved{
		resolved(pgSMS, "Ola", "", "+48500100103"),
		resolved(pgEmail, "Jan", "jan@example.test", ""),
	}}
	plan := NewPlan("Zebranie", "Czesc {{imie}}", res.Recipients)
	created, err := store.CreateBatch(ctx, Batch{
		Subject:   "Zebranie",
		Body:      "Czesc {{imie}}",
		CreatedBy: userID,
		Status:    BatchRunning,
		Plan:      plan,
		CostMilli: 65,
	})
	if err != nil {
		t.Fatalf("CreateBatch error = %v", err)
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sms := &scriptedSender{
		channel: providers.ChannelSMS,
		onSend: func() {
			// Interrupt dispatch right after first message is dispatched
			cancel()
		},
	}
	email := &scriptedSender{channel: providers.ChannelEmail}
	d, _ := newTestDispatcher(t, DefaultRetryPolicy(), sms, email)

	obs := DeliveryObserverFunc(func(obsCtx context.Context, recipientID string, del Delivery) error {
		return store.SaveDelivery(context.WithoutCancel(obsCtx), created.ID, recipientID, del)
	})

	_, _ = d.DispatchWithObserver(cancelCtx, created.Plan, obs)

	// Check DB: first delivery must be sent with provider_message_id, second must remain pending with NULL
	var smsStatus string
	var smsMsgID *string
	err = pool.QueryRow(ctx, `
		SELECT d.status, d.provider_message_id
		FROM deliveries d
		JOIN batch_recipients br ON br.id = d.batch_recipient_id
		WHERE br.batch_id = $1 AND br.recipient_id = $2 AND d.channel = 'sms'`,
		created.ID, pgSMS).Scan(&smsStatus, &smsMsgID)
	if err != nil {
		t.Fatalf("query sms delivery: %v", err)
	}
	if smsStatus != "sent" || smsMsgID == nil || *smsMsgID == "" {
		t.Errorf("first delivery status = %q, msgID = %v; want 'sent' with provider_message_id", smsStatus, smsMsgID)
	}

	var emailStatus string
	var emailMsgID *string
	err = pool.QueryRow(ctx, `
		SELECT d.status, d.provider_message_id
		FROM deliveries d
		JOIN batch_recipients br ON br.id = d.batch_recipient_id
		WHERE br.batch_id = $1 AND br.recipient_id = $2 AND d.channel = 'email'`,
		created.ID, pgEmail).Scan(&emailStatus, &emailMsgID)
	if err != nil {
		t.Fatalf("query email delivery: %v", err)
	}
	if emailStatus != "pending" || emailMsgID != nil {
		t.Errorf("second delivery status = %q, msgID = %v; want 'pending' with nil provider_message_id", emailStatus, emailMsgID)
	}
}

func TestPGStore_DLRDuringDispatchEndsDelivered(t *testing.T) {
	store, pool := newPGStore(t)
	ctx := context.Background()

	res := groups.Resolution{Recipients: []groups.Resolved{
		resolved(pgSMS, "Ola", "", "+48500100103"),
	}}
	plan := NewPlan("Zebranie", "Czesc {{imie}}", res.Recipients)
	created, err := store.CreateBatch(ctx, Batch{
		Subject:   "Zebranie",
		Body:      "Czesc {{imie}}",
		CreatedBy: userID,
		Status:    BatchRunning,
		Plan:      plan,
		CostMilli: 65,
	})
	if err != nil {
		t.Fatalf("CreateBatch error = %v", err)
	}

	consumer := providers.NewPGDeliveryReportConsumer(pool)
	expectedMsgID := "sms-" + pgSMS

	sms := &scriptedSender{
		channel: providers.ChannelSMS,
		onSend: func() {
			// Simulate DLR callback arriving in the middle of provider call / dispatch
			report := providers.DeliveryReport{
				ProviderMessageID: expectedMsgID,
				Channel:           providers.ChannelSMS,
				Status:            providers.StatusDelivered,
			}
			if err := consumer.ConsumeDeliveryReports(ctx, []providers.DeliveryReport{report}); err != nil {
				t.Fatalf("ConsumeDeliveryReports during dispatch failed: %v", err)
			}
		},
	}
	d, _ := newTestDispatcher(t, DefaultRetryPolicy(), sms)

	obs := DeliveryObserverFunc(func(obsCtx context.Context, recipientID string, del Delivery) error {
		return store.SaveDelivery(obsCtx, created.ID, recipientID, del)
	})

	dispatched, err := d.DispatchWithObserver(ctx, created.Plan, obs)
	if err != nil {
		t.Fatalf("DispatchWithObserver error = %v", err)
	}
	if err := store.SaveOutcome(ctx, created.ID, dispatched); err != nil {
		t.Fatalf("SaveOutcome error = %v", err)
	}

	// Verify in DB that the delivery ended as 'delivered'
	var finalStatus string
	err = pool.QueryRow(ctx, `
		SELECT d.status
		FROM deliveries d
		JOIN batch_recipients br ON br.id = d.batch_recipient_id
		WHERE br.batch_id = $1 AND br.recipient_id = $2 AND d.channel = 'sms'`,
		created.ID, pgSMS).Scan(&finalStatus)
	if err != nil {
		t.Fatalf("query delivery status: %v", err)
	}
	if finalStatus != "delivered" {
		t.Errorf("delivery finalStatus = %q, want 'delivered'", finalStatus)
	}
}
