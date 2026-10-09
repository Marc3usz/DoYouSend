package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
	"github.com/Marc3usz/DoYouSend/backend/internal/messaging"
)

// saveTimeout bounds recording the outcome after dispatch.
const saveTimeout = 10 * time.Second

var (
	ErrEmptySubject = errors.New("e-mail subject is empty")
	ErrInvalidUser  = errors.New("sender user ID is not valid")
	// ErrUnknownPlaceholders means the body uses a placeholder no recipient can fill.
	ErrUnknownPlaceholders = errors.New("body uses unknown placeholders")
	// ErrSelectionChanged means a selected group or person no longer exists.
	ErrSelectionChanged = errors.New("selection refers to groups or recipients that no longer exist")
	// ErrNoRecipients means the selection resolved to nobody.
	ErrNoRecipients = errors.New("selection has no recipients")
	// ErrInvalidKey means the idempotency key is not a UUID.
	ErrInvalidKey = errors.New("idempotency key is not a valid UUID")
	// ErrIdempotencyConflict means the idempotency key was already used for a
	// different message or selection.
	ErrIdempotencyConflict = errors.New("idempotency key reused with a different request")
	// ErrShuttingDown means the server is stopping and takes no new batches.
	ErrShuttingDown = errors.New("server is shutting down")
)

// SelectionChangedError lists what disappeared since the sender made the selection.
// The batch is refused rather than sent to a smaller list without the sender knowing.
// It matches ErrSelectionChanged with errors.Is.
type SelectionChangedError struct {
	UnknownGroupIDs     []string
	UnknownRecipientIDs []string
}

func (e *SelectionChangedError) Error() string {
	return fmt.Sprintf("%v: %d groups, %d recipients", ErrSelectionChanged,
		len(e.UnknownGroupIDs), len(e.UnknownRecipientIDs))
}

func (e *SelectionChangedError) Is(target error) bool { return target == ErrSelectionChanged }

// Resolver expands a selection into the final recipient list. *groups.Resolver satisfies it.
type Resolver interface {
	Resolve(ctx context.Context, sel groups.Selection) (groups.Resolution, error)
}

// GroupLookup returns one group by ID. *groups.Service satisfies it.
type GroupLookup interface {
	Get(ctx context.Context, id string) (groups.Group, error)
}

// PlanDispatcher sends a plan. *Dispatcher satisfies it.
type PlanDispatcher interface {
	Dispatch(ctx context.Context, plan Plan) (Plan, error)
}

// Draft is the message the sender confirms in the composer.
type Draft struct {
	Subject   string
	Body      string
	Selection groups.Selection
	// CreatedBy is the ID of the signed-in user confirming the send.
	CreatedBy     string
	CreatedByName string
	// IdempotencyKey, when set, makes a repeated confirmation return the batch it
	// created the first time instead of sending again.
	IdempotencyKey string
}

// Service turns a confirmed draft into a sent, recorded batch.
type Service struct {
	resolver          Resolver
	groups            GroupLookup
	dispatcher        PlanDispatcher
	store             Store
	pricePerPartMilli int64
	logger            *slog.Logger
	// running tracks Start calls and the dispatches they leave in the background, for
	// Wait. closing, guarded by mu, stops new ones once Wait has begun.
	mu      sync.Mutex
	closing bool
	running sync.WaitGroup
}

// NewService returns a Service pricing every SMS part at pricePerPartMilli thousandths
// of a złoty.
func NewService(resolver Resolver, lookup GroupLookup, dispatcher PlanDispatcher, store Store,
	pricePerPartMilli int64, logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		resolver: resolver, groups: lookup, dispatcher: dispatcher, store: store,
		pricePerPartMilli: pricePerPartMilli, logger: logger,
	}
}

// Send checks the draft, resolves its recipients, records the batch and dispatches it.
// Before anything is recorded it refuses a draft that would fail as a whole: no
// subject, an empty body, an unknown placeholder, a selection that changed, or nobody
// to send to. After that, problems are per recipient and end up in the batch.
//
// Once recorded, the batch is sent to the end even if ctx is cancelled (the sender
// closing the tab must not leave half the school unmessaged, CLAUDE.md rule 5);
// ctx still carries its values. The returned batch holds the outcome. An error together
// with a batch that has an ID means the batch was recorded but its outcome was not saved.
// A repeated idempotency key returns the batch recorded the first time and sends nothing.
func (s *Service) Send(ctx context.Context, d Draft) (Batch, error) {
	batch, replayed, err := s.record(ctx, d)
	if err != nil || replayed {
		return batch, err
	}
	return s.dispatch(context.WithoutCancel(ctx), batch)
}

// Start does what Send does but returns as soon as the batch is recorded, with its
// deliveries still pending; the dispatch goes on in the background (POST /batches
// answers 202). replayed reports a repeated idempotency key: the batch is the one
// recorded the first time and nothing is sent again. Wait blocks until every
// background dispatch has finished.
func (s *Service) Start(ctx context.Context, d Draft) (Batch, bool, error) {
	if !s.enter() {
		return Batch{}, false, fmt.Errorf("send batch: %w", ErrShuttingDown)
	}
	batch, replayed, err := s.record(ctx, d)
	if err != nil || replayed {
		s.running.Done()
		return batch, replayed, err
	}
	go func(b Batch) {
		defer s.running.Done()
		if _, err := s.dispatch(context.WithoutCancel(ctx), b); err != nil {
			s.logger.Error("batch dispatch", "batch_id", b.ID, "err", err)
		}
	}(batch)
	return batch, false, nil
}

// enter registers a Start call, unless Wait has begun.
func (s *Service) enter() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return false
	}
	s.running.Add(1)
	return true
}

// Wait refuses new Start calls and blocks until every batch already started has been
// dispatched and saved.
func (s *Service) Wait() {
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()
	s.running.Wait()
}

// record checks the draft and saves the batch, or finds the one recorded under the
// same idempotency key.
func (s *Service) record(ctx context.Context, d Draft) (Batch, bool, error) {
	d.IdempotencyKey = strings.ToLower(strings.TrimSpace(d.IdempotencyKey))
	if err := checkDraft(d); err != nil {
		return Batch{}, false, fmt.Errorf("send batch: %w", err)
	}
	hash, err := requestHash(d)
	if err != nil {
		return Batch{}, false, fmt.Errorf("send batch: %w", err)
	}
	if d.IdempotencyKey != "" {
		if b, err := s.replay(ctx, d, hash); !errors.Is(err, ErrBatchNotFound) {
			return b, err == nil, err
		}
	}

	res, err := s.resolver.Resolve(ctx, d.Selection)
	if err != nil {
		return Batch{}, false, fmt.Errorf("send batch: %w", err)
	}
	if len(res.UnknownGroupIDs) > 0 || len(res.UnknownRecipientIDs) > 0 {
		return Batch{}, false, fmt.Errorf("send batch: %w", &SelectionChangedError{
			UnknownGroupIDs: res.UnknownGroupIDs, UnknownRecipientIDs: res.UnknownRecipientIDs,
		})
	}
	if len(res.Recipients) == 0 {
		return Batch{}, false, fmt.Errorf("send batch: %w", ErrNoRecipients)
	}
	refs, err := s.groupRefs(ctx, d.Selection.Normalize().GroupIDs)
	if err != nil {
		return Batch{}, false, fmt.Errorf("send batch: %w", err)
	}

	plan := NewPlan(d.Subject, d.Body, res.Recipients)
	parts := plan.smsParts()
	batch, err := s.store.CreateBatch(ctx, Batch{
		Subject:        d.Subject,
		Body:           d.Body,
		CreatedBy:      d.CreatedBy,
		CreatedByName:  d.CreatedByName,
		IdempotencyKey: d.IdempotencyKey,
		RequestHash:    hash,
		Groups:         refs,
		RecipientIDs:   d.Selection.Normalize().RecipientIDs,
		Status:         BatchRunning,
		SMSParts:       parts,
		CostMilli:      int64(parts) * s.pricePerPartMilli,
		Plan:           plan,
	})
	if errors.Is(err, ErrDuplicateKey) {
		// A concurrent request with the same key won the race.
		b, err := s.replay(ctx, d, hash)
		return b, err == nil, err
	}
	if err != nil {
		return Batch{}, false, fmt.Errorf("send batch: record: %w", err)
	}
	return batch, false, nil
}

// replay returns the batch recorded under the draft's idempotency key, provided it
// came from the same request; ErrBatchNotFound when the key is new.
func (s *Service) replay(ctx context.Context, d Draft, hash string) (Batch, error) {
	b, err := s.store.BatchByKey(ctx, d.CreatedBy, d.IdempotencyKey)
	if err != nil {
		return Batch{}, fmt.Errorf("send batch: find key: %w", err)
	}
	if b.RequestHash != hash {
		return Batch{}, fmt.Errorf("send batch %s: %w", b.ID, ErrIdempotencyConflict)
	}
	return b, nil
}

// requestHash fingerprints what a sender confirmed, to tell a repeated request from a
// key reused for another message. Selection order and repeats do not matter.
func requestHash(d Draft) (string, error) {
	sel := d.Selection.Normalize()
	data, err := json.Marshal([]any{d.Subject, d.Body,
		slices.Sorted(slices.Values(sel.GroupIDs)),
		slices.Sorted(slices.Values(sel.RecipientIDs)),
		slices.Sorted(slices.Values(sel.ExcludedRecipientIDs))})
	if err != nil {
		return "", fmt.Errorf("hash request: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// dispatch sends a recorded batch and saves the outcome. ctx must not be cancelled
// by the caller going away.
func (s *Service) dispatch(ctx context.Context, batch Batch) (Batch, error) {
	sent, dispatchErr := s.dispatcher.Dispatch(ctx, batch.Plan)
	batch.Plan = sent
	batch.Status = sent.Status()
	batch.Counts = sent.Counts()

	saveCtx, cancel := context.WithTimeout(ctx, saveTimeout)
	defer cancel()
	if err := s.store.SaveOutcome(saveCtx, batch.ID, sent); err != nil {
		s.logger.Error("batch outcome not saved", "batch_id", batch.ID, "err", err)
		return batch, fmt.Errorf("send batch %s: save outcome: %w", batch.ID, errors.Join(err, dispatchErr))
	}
	if dispatchErr != nil {
		return batch, fmt.Errorf("send batch %s: %w", batch.ID, dispatchErr)
	}
	s.logger.Info("batch sent", "batch_id", batch.ID, "status", batch.Status,
		"recipients", len(sent.Recipients))
	return batch, nil
}

func checkDraft(d Draft) error {
	switch {
	case strings.TrimSpace(d.Subject) == "":
		return ErrEmptySubject
	case strings.TrimSpace(d.Body) == "":
		return messaging.ErrEmptyBody
	case !groups.IsValidID(d.CreatedBy):
		return ErrInvalidUser
	case d.IdempotencyKey != "" && !groups.IsValidID(d.IdempotencyKey):
		return ErrInvalidKey
	}
	if unknown := messaging.UnknownPlaceholders(d.Body); len(unknown) > 0 {
		return fmt.Errorf("%w: %s", ErrUnknownPlaceholders, strings.Join(unknown, ", "))
	}
	return nil
}

// groupRefs snapshots the names of the selected groups, in selection order.
func (s *Service) groupRefs(ctx context.Context, ids []string) ([]GroupRef, error) {
	refs := make([]GroupRef, 0, len(ids))
	for _, id := range ids {
		g, err := s.groups.Get(ctx, id)
		if errors.Is(err, groups.ErrNotFound) {
			// Deleted after Resolve: same answer as if Resolve had missed it.
			return nil, &SelectionChangedError{UnknownGroupIDs: []string{id}}
		}
		if err != nil {
			return nil, fmt.Errorf("name group %s: %w", id, err)
		}
		refs = append(refs, GroupRef{ID: id, Name: g.Name})
	}
	return refs, nil
}
