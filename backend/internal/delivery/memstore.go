package delivery

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// MemStore keeps batches in memory. It is meant for tests and for running the API
// without a database; everything is lost on restart.
type MemStore struct {
	mu      sync.Mutex
	batches map[string]Batch
	now     func() time.Time
}

// NewMemStore returns an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{batches: make(map[string]Batch), now: time.Now}
}

// CreateBatch implements Store.
func (s *MemStore) CreateBatch(_ context.Context, b Batch) (Batch, error) {
	id, err := newID()
	if err != nil {
		return Batch{}, err
	}
	b = cloneBatch(b)
	b.ID = id
	b.CreatedAt = s.now()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.batches[id] = b
	return cloneBatch(b), nil
}

// SaveOutcome implements Store.
func (s *MemStore) SaveOutcome(_ context.Context, batchID string, plan Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.batches[batchID]
	if !ok {
		return fmt.Errorf("save outcome of batch %s: %w", batchID, ErrBatchNotFound)
	}

	type key struct {
		recipientID string
		channel     providers.Channel
	}
	outcome := make(map[key]Delivery)
	for _, r := range plan.Recipients {
		for _, d := range r.Deliveries {
			outcome[key{r.RecipientID, d.Channel}] = d
		}
	}

	updated := b.Plan.clone()
	for i, r := range updated.Recipients {
		for j, d := range r.Deliveries {
			o, ok := outcome[key{r.RecipientID, d.Channel}]
			if !ok || Advance(d.Status, o.Status) != o.Status {
				continue
			}
			d.Status, d.Attempts, d.ProviderMessageID, d.Error = o.Status, o.Attempts, o.ProviderMessageID, o.Error
			updated.Recipients[i].Deliveries[j] = d
		}
	}
	b.Plan = updated
	b.Status = updated.Status()
	if b.Status != BatchRunning && b.FinishedAt.IsZero() {
		b.FinishedAt = s.now()
	}
	s.batches[batchID] = b
	return nil
}

// Batch implements Store.
func (s *MemStore) Batch(_ context.Context, id string) (Batch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.batches[id]
	if !ok {
		return Batch{}, fmt.Errorf("batch %s: %w", id, ErrBatchNotFound)
	}
	return cloneBatch(b), nil
}

func cloneBatch(b Batch) Batch {
	b.Groups = append([]GroupRef(nil), b.Groups...)
	b.RecipientIDs = append([]string(nil), b.RecipientIDs...)
	b.Plan = b.Plan.clone()
	return b
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate batch id: %w", err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
