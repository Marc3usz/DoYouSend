package delivery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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
	CreatedBy string
}

// Service turns a confirmed draft into a sent, recorded batch.
type Service struct {
	resolver          Resolver
	groups            GroupLookup
	dispatcher        PlanDispatcher
	store             Store
	pricePerPartMilli int64
	logger            *slog.Logger
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
func (s *Service) Send(ctx context.Context, d Draft) (Batch, error) {
	if err := checkDraft(d); err != nil {
		return Batch{}, fmt.Errorf("send batch: %w", err)
	}
	res, err := s.resolver.Resolve(ctx, d.Selection)
	if err != nil {
		return Batch{}, fmt.Errorf("send batch: %w", err)
	}
	if len(res.UnknownGroupIDs) > 0 || len(res.UnknownRecipientIDs) > 0 {
		return Batch{}, fmt.Errorf("send batch: %w", &SelectionChangedError{
			UnknownGroupIDs: res.UnknownGroupIDs, UnknownRecipientIDs: res.UnknownRecipientIDs,
		})
	}
	if len(res.Recipients) == 0 {
		return Batch{}, fmt.Errorf("send batch: %w", ErrNoRecipients)
	}
	refs, err := s.groupRefs(ctx, d.Selection.Normalize().GroupIDs)
	if err != nil {
		return Batch{}, fmt.Errorf("send batch: %w", err)
	}

	plan := NewPlan(d.Subject, d.Body, res.Recipients)
	parts := plan.smsParts()
	batch, err := s.store.CreateBatch(ctx, Batch{
		Subject:      d.Subject,
		Body:         d.Body,
		CreatedBy:    d.CreatedBy,
		Groups:       refs,
		RecipientIDs: d.Selection.Normalize().RecipientIDs,
		Status:       BatchRunning,
		SMSParts:     parts,
		CostMilli:    int64(parts) * s.pricePerPartMilli,
		Plan:         plan,
	})
	if err != nil {
		return Batch{}, fmt.Errorf("send batch: record: %w", err)
	}

	detached := context.WithoutCancel(ctx)
	sent, dispatchErr := s.dispatcher.Dispatch(detached, batch.Plan)
	batch.Plan = sent
	batch.Status = sent.Status()

	saveCtx, cancel := context.WithTimeout(detached, saveTimeout)
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
