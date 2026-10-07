package delivery

import (
	"context"
	"errors"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
	"github.com/Marc3usz/DoYouSend/backend/internal/messaging"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

const (
	userID   = "00000000-0000-4000-8000-0000000000aa"
	groupID  = "00000000-0000-4000-8000-0000000000bb"
	groupID2 = "00000000-0000-4000-8000-0000000000cc"
)

type fakeResolver struct {
	res groups.Resolution
	err error
}

func (f fakeResolver) Resolve(context.Context, groups.Selection) (groups.Resolution, error) {
	return f.res, f.err
}

type fakeGroups map[string]string

func (f fakeGroups) Get(_ context.Context, id string) (groups.Group, error) {
	name, ok := f[id]
	if !ok {
		return groups.Group{}, groups.ErrNotFound
	}
	return groups.Group{ID: id, Name: name}, nil
}

type failingStore struct {
	*MemStore
	createErr, saveErr error
}

func (s failingStore) CreateBatch(ctx context.Context, b Batch) (Batch, error) {
	if s.createErr != nil {
		return Batch{}, s.createErr
	}
	return s.MemStore.CreateBatch(ctx, b)
}

func (s failingStore) SaveOutcome(ctx context.Context, id string, p Plan) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	return s.MemStore.SaveOutcome(ctx, id, p)
}

func twoRecipients() groups.Resolution {
	return groups.Resolution{Recipients: []groups.Resolved{
		resolved(id1, "Anna", "anna@example.test", "+48500100101"),
		resolved(id2, "Jan", "", "+48500100102"),
	}}
}

func validDraft() Draft {
	return Draft{
		Subject:   "Zebranie",
		Body:      "Czesc {{imie}}",
		Selection: groups.Selection{GroupIDs: []string{groupID, groupID2}},
		CreatedBy: userID,
	}
}

func newTestService(t *testing.T, res groups.Resolution, store Store, sms *scriptedSender) *Service {
	t.Helper()
	email := &scriptedSender{channel: providers.ChannelEmail}
	d, _ := newTestDispatcher(t, DefaultRetryPolicy(), email, sms)
	names := fakeGroups{groupID: "Rodzice uczniów klasy 3A", groupID2: "Wszyscy rodzice"}
	return NewService(fakeResolver{res: res}, names, d, store, 65, nil)
}

func TestServiceSendRecordsBatch(t *testing.T) {
	store := NewMemStore()
	sms := &scriptedSender{channel: providers.ChannelSMS, errs: map[string][]error{id2: {errPermanent}}}
	svc := newTestService(t, twoRecipients(), store, sms)

	got, err := svc.Send(context.Background(), validDraft())
	if err != nil {
		t.Fatal(err)
	}

	if got.ID == "" || got.Status != BatchDoneWithErrors {
		t.Errorf("batch id=%q status=%s, want an ID and done_with_errors", got.ID, got.Status)
	}
	if got.SMSParts != 2 || got.CostMilli != 130 {
		t.Errorf("SMSParts=%d CostMilli=%d, want 2 and 130", got.SMSParts, got.CostMilli)
	}
	wantGroups := []GroupRef{{groupID, "Rodzice uczniów klasy 3A"}, {groupID2, "Wszyscy rodzice"}}
	if len(got.Groups) != 2 || got.Groups[0] != wantGroups[0] || got.Groups[1] != wantGroups[1] {
		t.Errorf("Groups = %v, want %v", got.Groups, wantGroups)
	}

	stored, err := store.Batch(context.Background(), got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != BatchDoneWithErrors || stored.FinishedAt.IsZero() || stored.CreatedBy != userID {
		t.Errorf("stored batch = %+v", stored)
	}
	if d := stored.Plan.Recipients[1].Deliveries[0]; d.Status != StatusFailed || d.Error == "" {
		t.Errorf("stored failed delivery = %+v", d)
	}
	if d := stored.Plan.Recipients[0].Deliveries[1]; d.Status != StatusSent || d.ProviderMessageID == "" {
		t.Errorf("stored sent delivery = %+v", d)
	}
}

func TestServiceSendRefusesBeforeRecording(t *testing.T) {
	changed := twoRecipients()
	changed.UnknownGroupIDs = []string{groupID2}

	tests := []struct {
		name  string
		draft func(Draft) Draft
		res   groups.Resolution
		want  error
	}{
		{"no subject", func(d Draft) Draft { d.Subject = " "; return d }, twoRecipients(), ErrEmptySubject},
		{"no body", func(d Draft) Draft { d.Body = ""; return d }, twoRecipients(), messaging.ErrEmptyBody},
		{"bad user", func(d Draft) Draft { d.CreatedBy = "x"; return d }, twoRecipients(), ErrInvalidUser},
		{"unknown placeholder", func(d Draft) Draft { d.Body = "{{klasa}}"; return d }, twoRecipients(), ErrUnknownPlaceholders},
		{"group gone", func(d Draft) Draft { return d }, changed, ErrSelectionChanged},
		{"nobody", func(d Draft) Draft { return d }, groups.Resolution{}, ErrNoRecipients},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemStore()
			sms := &scriptedSender{channel: providers.ChannelSMS}
			svc := newTestService(t, tt.res, store, sms)

			_, err := svc.Send(context.Background(), tt.draft(validDraft()))

			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
			if len(store.batches) != 0 || len(sms.sent) != 0 {
				t.Errorf("recorded %d batches, sent %d sms; want nothing", len(store.batches), len(sms.sent))
			}
		})
	}
}

func TestServiceSendSelectionChangedListsIDs(t *testing.T) {
	res := twoRecipients()
	res.UnknownRecipientIDs = []string{id3}
	svc := newTestService(t, res, NewMemStore(), &scriptedSender{channel: providers.ChannelSMS})

	_, err := svc.Send(context.Background(), validDraft())

	var sce *SelectionChangedError
	if !errors.As(err, &sce) || len(sce.UnknownRecipientIDs) != 1 || sce.UnknownRecipientIDs[0] != id3 {
		t.Errorf("err = %v, want SelectionChangedError naming %s", err, id3)
	}
}

func TestServiceSendStoreFailures(t *testing.T) {
	boom := errors.New("db down")

	t.Run("create", func(t *testing.T) {
		sms := &scriptedSender{channel: providers.ChannelSMS}
		svc := newTestService(t, twoRecipients(), failingStore{MemStore: NewMemStore(), createErr: boom}, sms)
		if _, err := svc.Send(context.Background(), validDraft()); !errors.Is(err, boom) {
			t.Errorf("err = %v", err)
		}
		if len(sms.sent) != 0 {
			t.Error("sent messages for a batch that was never recorded")
		}
	})

	t.Run("save outcome", func(t *testing.T) {
		svc := newTestService(t, twoRecipients(), failingStore{MemStore: NewMemStore(), saveErr: boom},
			&scriptedSender{channel: providers.ChannelSMS})
		got, err := svc.Send(context.Background(), validDraft())
		if !errors.Is(err, boom) || got.ID == "" {
			t.Errorf("batch id=%q err=%v, want the recorded batch and the save error", got.ID, err)
		}
	})
}

func TestServiceSendFinishesAfterCallerCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := NewMemStore()
	sms := &scriptedSender{channel: providers.ChannelSMS, onSend: cancel}
	svc := newTestService(t, twoRecipients(), store, sms)

	got, err := svc.Send(ctx, validDraft())

	if err != nil {
		t.Fatalf("err = %v, want the batch finished despite the cancel", err)
	}
	stored, _ := store.Batch(context.Background(), got.ID)
	if stored.Status != BatchDone || len(sms.sent) != 2 {
		t.Errorf("stored status = %s, sms sent = %d; want done and 2", stored.Status, len(sms.sent))
	}
}

func TestServiceSendRecordsDispatchError(t *testing.T) {
	boom := errors.New("queue down")
	store := NewMemStore()
	svc := NewService(fakeResolver{res: twoRecipients()}, fakeGroups{groupID: "A", groupID2: "B"},
		dispatchFunc(func(_ context.Context, p Plan) (Plan, error) { return p, boom }), store, 0, nil)

	got, err := svc.Send(context.Background(), validDraft())

	if !errors.Is(err, boom) || got.ID == "" {
		t.Fatalf("batch id=%q err=%v", got.ID, err)
	}
	if stored, _ := store.Batch(context.Background(), got.ID); stored.Status != BatchRunning {
		t.Errorf("stored status = %s, want running", stored.Status)
	}
}

type dispatchFunc func(context.Context, Plan) (Plan, error)

func (f dispatchFunc) Dispatch(ctx context.Context, p Plan) (Plan, error) { return f(ctx, p) }

func TestServiceSendUnknownGroupName(t *testing.T) {
	d, _ := newTestDispatcher(t, DefaultRetryPolicy())
	svc := NewService(fakeResolver{res: twoRecipients()}, fakeGroups{groupID: "A"}, d, NewMemStore(), 0, nil)
	_, err := svc.Send(context.Background(), validDraft())
	var sce *SelectionChangedError
	if !errors.As(err, &sce) || len(sce.UnknownGroupIDs) != 1 || sce.UnknownGroupIDs[0] != groupID2 {
		t.Errorf("err = %v, want SelectionChangedError naming %s", err, groupID2)
	}
}

func TestMemStore(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()
	if _, err := s.Batch(ctx, "nope"); !errors.Is(err, ErrBatchNotFound) {
		t.Errorf("Batch(nope) err = %v", err)
	}
	if err := s.SaveOutcome(ctx, "nope", Plan{}); !errors.Is(err, ErrBatchNotFound) {
		t.Errorf("SaveOutcome(nope) err = %v", err)
	}

	plan := NewPlan("S", "x", twoRecipients().Recipients)
	b, err := s.CreateBatch(ctx, Batch{Plan: plan, Groups: []GroupRef{{ID: groupID, Name: "G"}}})
	if err != nil || !groups.IsValidID(b.ID) || b.CreatedAt.IsZero() {
		t.Fatalf("CreateBatch = %+v, %v", b, err)
	}

	b.Groups[0].Name = "changed by caller"
	b.Plan.Recipients[0].Deliveries[0].Status = StatusFailed
	stored, _ := s.Batch(ctx, b.ID)
	if stored.Groups[0].Name != "G" || stored.Plan.Recipients[0].Deliveries[0].Status != StatusPending {
		t.Error("caller's changes leaked into the store")
	}

	// An outcome for a recipient the batch does not have is ignored.
	extra := NewPlan("S", "x", []groups.Resolved{resolved(id3, "C", "", "+48500100103")})
	extra.Recipients[0].Deliveries[0].Status = StatusSent
	if err := s.SaveOutcome(ctx, b.ID, extra); err != nil {
		t.Fatal(err)
	}
	stored, _ = s.Batch(ctx, b.ID)
	if len(stored.Plan.Recipients) != 2 || !stored.FinishedAt.IsZero() || stored.Status != BatchRunning {
		t.Errorf("stored = %+v", stored)
	}
}

func TestMemStoreSaveOutcomeOnlyMovesForward(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()
	b, _ := s.CreateBatch(ctx, Batch{Plan: NewPlan("S", "x", []groups.Resolved{resolved(id1, "A", "", "+48500100101")})})

	withStatus := func(st Status, msgID string) Plan {
		p := b.Plan.clone()
		p.Recipients[0].Deliveries[0].Status = st
		p.Recipients[0].Deliveries[0].ProviderMessageID = msgID
		return p
	}
	if err := s.SaveOutcome(ctx, b.ID, withStatus(StatusSent, "m1")); err != nil {
		t.Fatal(err)
	}
	first, _ := s.Batch(ctx, b.ID)
	if first.Status != BatchDone || first.FinishedAt.IsZero() {
		t.Fatalf("after sent: %s finished=%v", first.Status, first.FinishedAt)
	}

	// A stale save from an earlier state must not undo the sent delivery.
	if err := s.SaveOutcome(ctx, b.ID, withStatus(StatusPending, "")); err != nil {
		t.Fatal(err)
	}
	again, _ := s.Batch(ctx, b.ID)
	d := again.Plan.Recipients[0].Deliveries[0]
	if d.Status != StatusSent || d.ProviderMessageID != "m1" || !again.FinishedAt.Equal(first.FinishedAt) {
		t.Errorf("after stale save: %+v finished=%v", d, again.FinishedAt)
	}
}

func TestSMSPartsSkipsFailedDeliveries(t *testing.T) {
	p := NewPlan("S", "{{imie}}", []groups.Resolved{
		resolved(id1, "", "", "+48500100101"),
		resolved(id2, "Jan", "", "+48500100102"),
	})
	if got := p.smsParts(); got != 1 {
		t.Errorf("smsParts = %d, want 1 (render failure costs nothing)", got)
	}
}
