package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

const testKey = "00000000-0000-4000-8000-00000000c0de"

func keyedDraft() Draft {
	d := validDraft()
	d.IdempotencyKey = testKey
	return d
}

func TestServiceStartDispatchesInBackground(t *testing.T) {
	store := NewMemStore()
	sms := &scriptedSender{channel: providers.ChannelSMS}
	svc := newTestService(t, twoRecipients(), store, sms)

	got, replayed, err := svc.Start(context.Background(), keyedDraft())
	if err != nil || replayed {
		t.Fatalf("Start() = replayed %v, %v", replayed, err)
	}
	if got.Status != BatchRunning || got.Counts.Recipients != 2 {
		t.Errorf("Start() batch = %s, %+v; want running with 2 recipients", got.Status, got.Counts)
	}

	svc.Wait()
	stored, _ := store.Batch(context.Background(), got.ID)
	if stored.Status != BatchDone || len(sms.sent) != 2 || stored.Counts.Failed != 0 {
		t.Errorf("after Wait: status %s, sms sent %d, counts %+v; want done and 2", stored.Status, len(sms.sent), stored.Counts)
	}
}

func TestServiceIdempotencyKey(t *testing.T) {
	store := NewMemStore()
	sms := &scriptedSender{channel: providers.ChannelSMS}
	svc := newTestService(t, twoRecipients(), store, sms)
	ctx := context.Background()

	first, err := svc.Send(ctx, keyedDraft())
	if err != nil {
		t.Fatal(err)
	}

	// The same confirmation again, IDs in another order and case: nothing is sent.
	again := keyedDraft()
	again.IdempotencyKey = " " + testKey + " "
	again.Selection = groups.Selection{GroupIDs: []string{groupID2, groupID, groupID}}
	got, replayed, err := svc.Start(ctx, again)
	svc.Wait()
	if err != nil || !replayed || got.ID != first.ID {
		t.Errorf("repeated Start() = %s replayed %v, %v; want %s replayed", got.ID, replayed, err, first.ID)
	}
	if len(store.batches) != 1 || len(sms.sent) != 2 {
		t.Errorf("%d batches, %d sms; want the first batch only", len(store.batches), len(sms.sent))
	}

	changed := keyedDraft()
	changed.Body = "Inna tresc"
	if _, err := svc.Send(ctx, changed); !errors.Is(err, ErrIdempotencyConflict) {
		t.Errorf("Send(other body) error = %v; want ErrIdempotencyConflict", err)
	}

	otherUser := keyedDraft()
	otherUser.CreatedBy = "00000000-0000-4000-8000-0000000000ab"
	if _, err := svc.Send(ctx, otherUser); err != nil {
		t.Errorf("Send(same key, other sender) error = %v; keys are per sender", err)
	}

	bad := keyedDraft()
	bad.IdempotencyKey = "abc"
	if _, err := svc.Send(ctx, bad); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("Send(bad key) error = %v; want ErrInvalidKey", err)
	}
}

// raceStore reports the key as new, then loses the insert to a concurrent request.
type raceStore struct {
	*MemStore
	winner  Batch
	lookups int
}

func (s *raceStore) BatchByKey(ctx context.Context, by, key string) (Batch, error) {
	s.lookups++
	if s.lookups == 1 {
		return Batch{}, ErrBatchNotFound
	}
	return s.winner, nil
}

func (s *raceStore) CreateBatch(context.Context, Batch) (Batch, error) {
	return Batch{}, ErrDuplicateKey
}

func TestServiceIdempotencyRace(t *testing.T) {
	hash, _ := requestHash(keyedDraft())
	store := &raceStore{MemStore: NewMemStore(), winner: Batch{ID: "winner", RequestHash: hash}}
	sms := &scriptedSender{channel: providers.ChannelSMS}
	svc := newTestService(t, twoRecipients(), store, sms)

	got, replayed, err := svc.Start(context.Background(), keyedDraft())
	svc.Wait()
	if err != nil || !replayed || got.ID != "winner" || len(sms.sent) != 0 {
		t.Errorf("Start() = %s replayed %v, %v, sent %d; want the winner, nothing sent", got.ID, replayed, err, len(sms.sent))
	}
}

func TestMemStoreListBatches(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()
	base := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	var ids []string
	for i, by := range []string{userID, "other", userID} {
		s.now = func() time.Time { return base.Add(time.Duration(i) * time.Minute) }
		b, err := s.CreateBatch(ctx, Batch{CreatedBy: by, Status: BatchRunning, Plan: NewPlan("S", "x", twoRecipients().Recipients)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, b.ID)
	}

	all, total, _ := s.ListBatches(ctx, BatchFilter{Limit: 2})
	if total != 3 || len(all) != 2 || all[0].ID != ids[2] || all[1].ID != ids[1] {
		t.Errorf("ListBatches(all) = %v, total %d", all, total)
	}
	if len(all[0].Plan.Recipients) != 0 || all[0].Counts.Recipients != 2 {
		t.Errorf("summary = %+v; want counts without the plan", all[0])
	}
	mine, total, _ := s.ListBatches(ctx, BatchFilter{CreatedBy: userID, Limit: 5, Offset: 1})
	if total != 2 || len(mine) != 1 || mine[0].ID != ids[0] {
		t.Errorf("ListBatches(mine) = %v, total %d", mine, total)
	}
	none, total, _ := s.ListBatches(ctx, BatchFilter{Status: BatchDone, Limit: 5, Offset: 9})
	if total != 0 || len(none) != 0 {
		t.Errorf("ListBatches(done) = %v, total %d", none, total)
	}
}

func TestPlanCounts(t *testing.T) {
	p := NewPlan("S", "{{imie}}", []groups.Resolved{
		resolved(id1, "Anna", "anna@example.test", "+48500100101"),
		resolved(id2, "", "", "+48500100102"), // render fails
		resolved(id3, "Ola", "", ""),          // unreachable
	})
	want := Counts{Recipients: 3, Partial: 1, Failed: 2}
	if got := p.Counts(); got != want {
		t.Errorf("Counts() = %+v, want %+v", got, want)
	}
}

func TestServiceStartAfterWaitIsRefused(t *testing.T) {
	store := NewMemStore()
	sms := &scriptedSender{channel: providers.ChannelSMS}
	svc := newTestService(t, twoRecipients(), store, sms)
	svc.Wait()

	if _, _, err := svc.Start(context.Background(), keyedDraft()); !errors.Is(err, ErrShuttingDown) {
		t.Errorf("Start() after Wait error = %v; want ErrShuttingDown", err)
	}
	if len(store.batches) != 0 || len(sms.sent) != 0 {
		t.Error("a batch was recorded or sent while shutting down")
	}
}
