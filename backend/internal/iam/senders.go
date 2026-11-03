package iam

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Sender identities (ADR-0016): an administrator asks the e-mail provider to
// verify a user's own address as a sender ("Single Sender Verification" in
// SendGrid). The provider e-mails a confirmation link to that address; the
// identity becomes verified once its owner clicks it.

// SenderStatus is the last known state of a sender identity.
type SenderStatus string

// Sender identity states.
const (
	SenderPending  SenderStatus = "pending"  // waiting for the owner to click the link
	SenderVerified SenderStatus = "verified" // the provider accepts it as From
	SenderMissing  SenderStatus = "missing"  // the provider no longer knows it
)

// SenderIdentity mirrors a row of user_sender_identities.
type SenderIdentity struct {
	UserID     string
	Provider   string
	ProviderID string
	FromEmail  string
	FromName   string
	Status     SenderStatus
	CreatedAt  time.Time
	CheckedAt  time.Time // zero when never checked
	VerifiedAt time.Time // zero while not verified
}

// SenderRequest is what the provider needs to create an identity.
type SenderRequest struct {
	Nickname  string
	FromEmail string
	FromName  string
	ReplyTo   string
}

// ProviderSender is an identity as the provider reports it.
type ProviderSender struct {
	ID       string
	Verified bool
}

// SenderVerifier is the e-mail provider's sender-identity API, defined here by
// its consumer. The adapter maps "unknown identity" to ErrSenderGone and a
// refused request to a *SenderRejectedError.
type SenderVerifier interface {
	CreateSender(ctx context.Context, in SenderRequest) (ProviderSender, error)
	GetSender(ctx context.Context, id string) (ProviderSender, error)
	ResendVerification(ctx context.Context, id string) error
	DeleteSender(ctx context.Context, id string) error
}

// SenderStore keeps sender identities; PGStore implements it.
type SenderStore interface {
	// GetSenderIdentity returns the user's identity or ErrSenderNotConfigured.
	GetSenderIdentity(ctx context.Context, userID string) (SenderIdentity, error)
	ListSenderIdentities(ctx context.Context) ([]SenderIdentity, error)
	// CreateSenderIdentity fails with ErrSenderExists when the user has one.
	CreateSenderIdentity(ctx context.Context, s SenderIdentity) (SenderIdentity, error)
	// SaveSenderStatus stores a checked status; verified_at keeps the first
	// verification time.
	SaveSenderStatus(ctx context.Context, userID string, status SenderStatus, checkedAt time.Time) (SenderIdentity, error)
	DeleteSenderIdentity(ctx context.Context, userID string) error
}

// Errors of the sender-identity flow. Match them with errors.Is.
var (
	ErrSenderNotConfigured   = errors.New("the user has no sender identity")
	ErrSenderExists          = errors.New("the user already has a sender identity")
	ErrSenderAlreadyVerified = errors.New("the sender identity is already verified")
	// ErrSenderGone is returned by a SenderVerifier for an identity the
	// provider does not know (anymore).
	ErrSenderGone = errors.New("the provider does not know this sender identity")
)

// SenderRejectedError is the provider refusing a request (e.g. an address
// that is already a sender in the account). Message is the provider's text,
// shown to the administrator; it never holds credentials.
type SenderRejectedError struct{ Message string }

func (e *SenderRejectedError) Error() string { return "sender identity rejected: " + e.Message }

// Audit actions of the sender-identity flow; entity "user".
const (
	ActionSenderRequested = "sender_verification_requested"
	ActionSenderResent    = "sender_verification_resent"
	ActionSenderRemoved   = "sender_identity_removed"
)

// SenderOptions configure Senders.
type SenderOptions struct {
	// Provider names the verifier ("sendgrid" or "fake"); identities created
	// by another provider are never sent to this one.
	Provider string
	// Live is false when the verifier is the in-memory fake (DRY_RUN): no
	// confirmation e-mail goes out. Shown in the panel.
	Live bool
	// NicknamePrefix starts the provider-side nickname, e.g. "DoYouSend".
	NicknamePrefix string
}

// Senders manages users' sender identities on behalf of administrators.
type Senders struct {
	svc      *Service
	store    SenderStore
	verifier SenderVerifier
	opts     SenderOptions
}

// NewSenders returns Senders; svc provides users, the clock and the audit log.
func NewSenders(svc *Service, store SenderStore, verifier SenderVerifier, opts SenderOptions) *Senders {
	if opts.NicknamePrefix == "" {
		opts.NicknamePrefix = "DoYouSend"
	}
	return &Senders{svc: svc, store: store, verifier: verifier, opts: opts}
}

// Options returns the configuration, for the panel.
func (s *Senders) Options() SenderOptions { return s.opts }

// Request creates a sender identity for the user's login address and makes
// the provider send the confirmation link to it.
func (s *Senders) Request(ctx context.Context, actor User, userID string) (SenderIdentity, error) {
	user, err := s.user(ctx, userID)
	if err != nil {
		return SenderIdentity{}, err
	}
	if user.Disabled {
		return SenderIdentity{}, &ValidationError{Fields: []FieldError{{Field: "user", Message: "the account is disabled"}}}
	}
	switch _, err := s.store.GetSenderIdentity(ctx, user.ID); {
	case err == nil:
		return SenderIdentity{}, ErrSenderExists
	case !errors.Is(err, ErrSenderNotConfigured):
		return SenderIdentity{}, fmt.Errorf("load sender identity: %w", err)
	}

	ps, err := s.verifier.CreateSender(ctx, SenderRequest{
		// The short ID keeps nicknames of namesakes apart in the provider's UI.
		Nickname:  fmt.Sprintf("%s: %s (%s)", s.opts.NicknamePrefix, user.FullName, user.ID[:8]),
		FromEmail: user.Email,
		FromName:  user.FullName,
		ReplyTo:   user.Email,
	})
	if err != nil {
		return SenderIdentity{}, fmt.Errorf("create sender identity: %w", err)
	}
	status := SenderPending
	if ps.Verified {
		status = SenderVerified
	}
	now := s.svc.now()
	created, err := s.store.CreateSenderIdentity(ctx, SenderIdentity{
		UserID: user.ID, Provider: s.opts.Provider, ProviderID: ps.ID,
		FromEmail: user.Email, FromName: user.FullName, Status: status, CreatedAt: now,
	})
	if err != nil {
		// Do not leave an identity at the provider that nothing points to.
		if delErr := s.verifier.DeleteSender(ctx, ps.ID); delErr != nil && !errors.Is(delErr, ErrSenderGone) {
			err = errors.Join(err, fmt.Errorf("roll back provider identity %s: %w", ps.ID, delErr))
		}
		return SenderIdentity{}, fmt.Errorf("store sender identity: %w", err)
	}
	s.svc.audit(ctx, actor.ID, ActionSenderRequested, "user", user.ID, map[string]string{"provider": s.opts.Provider})
	return created, nil
}

// Status returns the user's identity. A pending identity is checked with the
// provider first, so the answer is current; a verified one stays verified.
func (s *Senders) Status(ctx context.Context, userID string) (SenderIdentity, error) {
	id, err := s.identity(ctx, userID)
	if err != nil {
		return SenderIdentity{}, err
	}
	if id.Status == SenderVerified || !s.ours(id) {
		return id, nil
	}
	status := SenderPending
	ps, err := s.verifier.GetSender(ctx, id.ProviderID)
	switch {
	case errors.Is(err, ErrSenderGone):
		status = SenderMissing
	case err != nil:
		return SenderIdentity{}, fmt.Errorf("check sender identity: %w", err)
	case ps.Verified:
		status = SenderVerified
	}
	return s.store.SaveSenderStatus(ctx, id.UserID, status, s.svc.now())
}

// List returns every stored identity as last checked; it calls no provider.
func (s *Senders) List(ctx context.Context) ([]SenderIdentity, error) {
	ids, err := s.store.ListSenderIdentities(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sender identities: %w", err)
	}
	return ids, nil
}

// Resend asks the provider to e-mail the confirmation link again.
func (s *Senders) Resend(ctx context.Context, actor User, userID string) error {
	id, err := s.identity(ctx, userID)
	if err != nil {
		return err
	}
	if id.Status == SenderVerified {
		return ErrSenderAlreadyVerified
	}
	if !s.ours(id) {
		return &SenderRejectedError{Message: fmt.Sprintf("identity was created with provider %q; remove it and request it again", id.Provider)}
	}
	if err := s.verifier.ResendVerification(ctx, id.ProviderID); err != nil {
		if errors.Is(err, ErrSenderGone) {
			_, _ = s.store.SaveSenderStatus(ctx, id.UserID, SenderMissing, s.svc.now())
		}
		return fmt.Errorf("resend sender verification: %w", err)
	}
	s.svc.audit(ctx, actor.ID, ActionSenderResent, "user", id.UserID, nil)
	return nil
}

// Remove deletes the identity at the provider (when it is ours and still
// there) and locally.
func (s *Senders) Remove(ctx context.Context, actor User, userID string) error {
	id, err := s.identity(ctx, userID)
	if err != nil {
		return err
	}
	if s.ours(id) {
		if err := s.verifier.DeleteSender(ctx, id.ProviderID); err != nil && !errors.Is(err, ErrSenderGone) {
			return fmt.Errorf("delete provider sender identity: %w", err)
		}
	}
	if err := s.store.DeleteSenderIdentity(ctx, id.UserID); err != nil {
		return fmt.Errorf("delete sender identity: %w", err)
	}
	s.svc.audit(ctx, actor.ID, ActionSenderRemoved, "user", id.UserID, map[string]string{"provider": id.Provider})
	return nil
}

func (s *Senders) ours(id SenderIdentity) bool { return id.Provider == s.opts.Provider }

func (s *Senders) user(ctx context.Context, userID string) (User, error) {
	id, err := checkID(userID)
	if err != nil {
		return User{}, err
	}
	u, err := s.svc.store.GetUser(ctx, id)
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

func (s *Senders) identity(ctx context.Context, userID string) (SenderIdentity, error) {
	id, err := checkID(userID)
	if err != nil {
		return SenderIdentity{}, err
	}
	si, err := s.store.GetSenderIdentity(ctx, id)
	if err != nil {
		return SenderIdentity{}, fmt.Errorf("load sender identity: %w", err)
	}
	return si, nil
}
