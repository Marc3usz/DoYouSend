package delivery

import (
	"context"
	"sync"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// recordingAuditor keeps every batch it was told about.
type recordingAuditor struct {
	mu      sync.Mutex
	batches []Batch
}

func (a *recordingAuditor) BatchConfirmed(_ context.Context, b Batch) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.batches = append(a.batches, b)
}

func TestServiceAuditsEachNewBatchOnce(t *testing.T) {
	svc := newTestService(t, twoRecipients(), NewMemStore(), &scriptedSender{channel: providers.ChannelSMS})
	audit := &recordingAuditor{}
	svc.SetAuditor(audit)
	ctx := context.Background()

	d := keyedDraft()
	first, err := svc.Send(ctx, d)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	// The same confirmation again (double click, retried request): logged already.
	if _, err := svc.Send(ctx, d); err != nil {
		t.Fatalf("repeated Send() error = %v", err)
	}
	// A refused draft never becomes a batch, so there is nothing to log.
	bad := validDraft()
	bad.Subject = " "
	if _, err := svc.Send(ctx, bad); err == nil {
		t.Fatal("Send(blank subject) error = nil")
	}

	if len(audit.batches) != 1 {
		t.Fatalf("audited %d batches, want 1", len(audit.batches))
	}
	got := audit.batches[0]
	if got.ID != first.ID || got.CreatedBy != userID || got.Counts.Recipients != 2 || len(got.Groups) != 2 {
		t.Errorf("audited %+v; want batch %s by %s with 2 recipients and 2 groups", got, first.ID, userID)
	}
}

func TestServiceWithoutAuditor(t *testing.T) {
	svc := newTestService(t, twoRecipients(), NewMemStore(), &scriptedSender{channel: providers.ChannelSMS})
	if _, err := svc.Send(context.Background(), validDraft()); err != nil {
		t.Fatalf("Send() without an auditor error = %v", err)
	}
}
