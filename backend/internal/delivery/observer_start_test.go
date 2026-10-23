package delivery

import (
	"context"
	"sync"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// countingStore is a MemStore that counts per-delivery saves.
type countingStore struct {
	*MemStore
	mu    sync.Mutex
	saves int
}

func (s *countingStore) SaveDelivery(ctx context.Context, batchID, recipientID string, d Delivery) error {
	s.mu.Lock()
	s.saves++
	s.mu.Unlock()
	return s.MemStore.SaveDelivery(ctx, batchID, recipientID, d)
}

// POST /batches goes through Start, not Send: it must save each delivery as it
// goes too (ADR-0011), not only the outcome at the end.
func TestServiceStartSavesEachDelivery(t *testing.T) {
	store := &countingStore{MemStore: NewMemStore()}
	svc := newTestService(t, twoRecipients(), store, &scriptedSender{channel: providers.ChannelSMS})

	batch, _, err := svc.Start(context.Background(), keyedDraft())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	svc.Wait()

	deliveries := 0
	for _, r := range batch.Plan.Recipients {
		deliveries += len(r.Deliveries)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	// Each delivery is saved twice: as sending before the gateway call, then its outcome.
	if store.saves != 2*deliveries {
		t.Errorf("SaveDelivery calls = %d, want %d for %d deliveries", store.saves, 2*deliveries, deliveries)
	}
}
