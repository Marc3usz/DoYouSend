package iam

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Session and login limits (ADR-0010).
const (
	SessionTTL = 12 * time.Hour
	// A user who fails MaxFailedLogins times within FailedLoginWindow must
	// wait until the oldest failure leaves the window.
	MaxFailedLogins   = 5
	FailedLoginWindow = 15 * time.Minute
	// Paging of GET /audit.
	DefaultAuditPage = 50
	MaxAuditPage     = 200
	maxFullName      = 200 // runes
)

// dummyHash is verified when a login names an unknown e-mail, so that the
// answer takes as long as for a wrong password and does not reveal which
// addresses have accounts.
const dummyHash = "pbkdf2-sha256$600000$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

// Service is authentication, the user administration and the audit trail.
type Service struct {
	store Store
	now   func() time.Time
	// hash is HashPassword; tests swap in a cheap variant.
	hash func(password string) (string, error)

	mu       sync.Mutex
	failures map[string][]time.Time // normalized e-mail -> recent failed logins
}

// NewService returns a Service backed by store.
func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now, hash: HashPassword, failures: make(map[string][]time.Time)}
}

// Session is a successful login: the token for the cookie and its expiry.
type Session struct {
	User      User
	Token     string
	ExpiresAt time.Time
}

// Login checks the credentials and opens a session. Every failure looks the
// same to the caller (ErrInvalidCredentials), whether the e-mail is unknown,
// the password wrong or the account disabled; repeated failures for one
// e-mail answer ErrTooManyAttempts for a while.
func (s *Service) Login(ctx context.Context, email, password string) (Session, error) {
	email = normalizeEmail(email)
	if s.throttled(email) {
		return Session{}, ErrTooManyAttempts
	}

	user, hash, err := s.store.UserByEmail(ctx, email)
	switch {
	case errors.Is(err, ErrNotFound):
		_, _ = VerifyPassword(password, dummyHash)
		return Session{}, s.failLogin(ctx, email, "")
	case err != nil:
		return Session{}, fmt.Errorf("look up user: %w", err)
	}
	ok, err := VerifyPassword(password, hash)
	if err != nil && !errors.Is(err, errMalformedHash) {
		return Session{}, fmt.Errorf("verify password: %w", err)
	}
	if !ok || user.Disabled {
		return Session{}, s.failLogin(ctx, email, user.ID)
	}

	token, tokenHash, err := newToken()
	if err != nil {
		return Session{}, err
	}
	now := s.now()
	expires := now.Add(SessionTTL)
	if err := s.store.CreateSession(ctx, tokenHash, user.ID, expires); err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	if err := s.store.TouchLogin(ctx, user.ID, now); err != nil {
		return Session{}, fmt.Errorf("record login: %w", err)
	}
	user.LastLoginAt = now
	s.clearFailures(email)
	s.audit(ctx, user.ID, ActionLogin, "user", user.ID, nil)
	return Session{User: user, Token: token, ExpiresAt: expires}, nil
}

// failLogin records a failed attempt and returns the error for the caller.
func (s *Service) failLogin(ctx context.Context, email, userID string) error {
	s.mu.Lock()
	s.failures[email] = append(s.recentFailures(email), s.now())
	s.mu.Unlock()
	s.audit(ctx, userID, ActionLoginFailed, "user", userID, nil)
	return ErrInvalidCredentials
}

func (s *Service) throttled(email string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	recent := s.recentFailures(email)
	if len(recent) == 0 {
		delete(s.failures, email)
	} else {
		s.failures[email] = recent
	}
	return len(recent) >= MaxFailedLogins
}

// recentFailures returns the failures of email still inside the window. The
// caller holds s.mu.
func (s *Service) recentFailures(email string) []time.Time {
	cutoff := s.now().Add(-FailedLoginWindow)
	var recent []time.Time
	for _, t := range s.failures[email] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	return recent
}

func (s *Service) clearFailures(email string) {
	s.mu.Lock()
	delete(s.failures, email)
	s.mu.Unlock()
}

// Authenticate returns the user of a session token, or ErrUnauthenticated.
func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrUnauthenticated
	}
	u, err := s.store.SessionUser(ctx, hashToken(token), s.now())
	if errors.Is(err, ErrNotFound) {
		return User{}, ErrUnauthenticated
	}
	if err != nil {
		return User{}, fmt.Errorf("look up session: %w", err)
	}
	return u, nil
}

// Logout ends the session of token. Ending an unknown session is not an error.
func (s *Service) Logout(ctx context.Context, user User, token string) error {
	if err := s.store.DeleteSession(ctx, hashToken(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	s.audit(ctx, user.ID, ActionLogout, "user", user.ID, nil)
	return nil
}

// ListUsers returns every user, administrators first, then by name.
func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

// NewUser is what an administrator submits to create an account.
type NewUser struct {
	Email    string
	FullName string
	Role     Role
	Password string
}

// CreateUser creates an account on behalf of actor.
func (s *Service) CreateUser(ctx context.Context, actor User, in NewUser) (User, error) {
	in.Email, in.FullName = normalizeEmail(in.Email), strings.TrimSpace(in.FullName)
	var errs []FieldError
	if !emailPattern.MatchString(in.Email) {
		errs = append(errs, FieldError{Field: "email", Message: "not a valid e-mail address"})
	}
	errs = append(errs, checkName(in.FullName)...)
	if !in.Role.Valid() {
		errs = append(errs, FieldError{Field: "role", Message: "role must be 'admin' or 'sender'"})
	}
	if err := CheckPasswordPolicy(in.Password); err != nil {
		errs = append(errs, FieldError{Field: "password", Message: err.Error()})
	}
	if len(errs) > 0 {
		return User{}, &ValidationError{Fields: errs}
	}

	hash, err := s.hash(in.Password)
	if err != nil {
		return User{}, err
	}
	created, err := s.store.CreateUser(ctx, User{Email: in.Email, FullName: in.FullName, Role: in.Role}, hash)
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	s.audit(ctx, actor.ID, ActionUserCreated, "user", created.ID, map[string]string{"role": string(created.Role)})
	return created, nil
}

// UserChange is a partial update of an account; nil fields stay as they are.
type UserChange struct {
	FullName *string
	Role     *Role
	Disabled *bool
}

// UpdateUser changes an account on behalf of actor. Administrators cannot
// demote or disable themselves, and the last active administrator cannot be
// demoted or disabled by anyone, so the panel always stays reachable.
func (s *Service) UpdateUser(ctx context.Context, actor User, id string, ch UserChange) (User, error) {
	id, err := checkID(id)
	if err != nil {
		return User{}, err
	}
	current, err := s.store.GetUser(ctx, id)
	if err != nil {
		return User{}, fmt.Errorf("get user %s: %w", id, err)
	}

	next := current
	var errs []FieldError
	if ch.FullName != nil {
		next.FullName = strings.TrimSpace(*ch.FullName)
		errs = append(errs, checkName(next.FullName)...)
	}
	if ch.Role != nil {
		if !ch.Role.Valid() {
			errs = append(errs, FieldError{Field: "role", Message: "role must be 'admin' or 'sender'"})
		}
		next.Role = *ch.Role
	}
	if ch.Disabled != nil {
		next.Disabled = *ch.Disabled
	}
	if len(errs) > 0 {
		return User{}, &ValidationError{Fields: errs}
	}

	losesAdmin := current.IsAdmin() && !current.Disabled && (!next.IsAdmin() || next.Disabled)
	if losesAdmin {
		if current.ID == actor.ID {
			return User{}, ErrSelfLockout
		}
		admins, err := s.store.CountActiveAdmins(ctx)
		if err != nil {
			return User{}, fmt.Errorf("count admins: %w", err)
		}
		if admins <= 1 {
			return User{}, ErrLastAdmin
		}
	}

	updated, err := s.store.UpdateUser(ctx, next)
	if err != nil {
		return User{}, fmt.Errorf("update user %s: %w", id, err)
	}
	if next.Disabled && !current.Disabled {
		if err := s.store.DeleteUserSessions(ctx, id); err != nil {
			return User{}, fmt.Errorf("end sessions of %s: %w", id, err)
		}
	}
	s.audit(ctx, actor.ID, ActionUserUpdated, "user", id, changeDetails(current, updated))
	return updated, nil
}

// ResetPassword sets a new password for user id on behalf of actor and ends
// every session of that user.
func (s *Service) ResetPassword(ctx context.Context, actor User, id, password string) error {
	id, err := checkID(id)
	if err != nil {
		return err
	}
	if err := CheckPasswordPolicy(password); err != nil {
		return &ValidationError{Fields: []FieldError{{Field: "password", Message: err.Error()}}}
	}
	if _, err := s.store.GetUser(ctx, id); err != nil {
		return fmt.Errorf("get user %s: %w", id, err)
	}
	hash, err := s.hash(password)
	if err != nil {
		return err
	}
	if err := s.store.SetPassword(ctx, id, hash); err != nil {
		return fmt.Errorf("set password of %s: %w", id, err)
	}
	if err := s.store.DeleteUserSessions(ctx, id); err != nil {
		return fmt.Errorf("end sessions of %s: %w", id, err)
	}
	action := ActionPasswordReset
	if id == actor.ID {
		action = ActionPasswordChange
	}
	s.audit(ctx, actor.ID, action, "user", id, nil)
	return nil
}

// AuditPage is one page of the audit log.
type AuditPage struct {
	Items []AuditEntry
	Total int
}

// ListAudit returns a page of the audit log, newest first.
func (s *Service) ListAudit(ctx context.Context, limit, offset int) (AuditPage, error) {
	var errs []FieldError
	switch {
	case limit == 0:
		limit = DefaultAuditPage
	case limit < 0 || limit > MaxAuditPage:
		errs = append(errs, FieldError{Field: "limit", Message: fmt.Sprintf("limit must be between 1 and %d", MaxAuditPage)})
	}
	if offset < 0 {
		errs = append(errs, FieldError{Field: "offset", Message: "offset must not be negative"})
	}
	if len(errs) > 0 {
		return AuditPage{}, &ValidationError{Fields: errs}
	}
	items, total, err := s.store.ListAudit(ctx, limit, offset)
	if err != nil {
		return AuditPage{}, fmt.Errorf("list audit log: %w", err)
	}
	return AuditPage{Items: items, Total: total}, nil
}

// audit records an action. A failed audit write must not undo the action the
// user already completed, so it is not returned; the store error is lost
// only if the database is down, in which case the action failed too.
func (s *Service) audit(ctx context.Context, userID, action, entity, entityID string, details map[string]string) {
	_ = s.store.AddAudit(ctx, AuditEntry{UserID: userID, Action: action, Entity: entity, EntityID: entityID, Details: details})
}

// changeDetails lists what an update changed, for the audit log. The name is
// left out on purpose: the log is about permissions, not personal data.
func changeDetails(before, after User) map[string]string {
	d := map[string]string{}
	if before.Role != after.Role {
		d["role"] = string(after.Role)
	}
	if before.Disabled != after.Disabled {
		d["disabled"] = fmt.Sprint(after.Disabled)
	}
	if before.FullName != after.FullName {
		d["fullName"] = "changed"
	}
	return d
}

func checkName(name string) []FieldError {
	if name == "" {
		return []FieldError{{Field: "fullName", Message: "full name is required"}}
	}
	if utf8.RuneCountInString(name) > maxFullName {
		return []FieldError{{Field: "fullName", Message: fmt.Sprintf("longer than %d characters", maxFullName)}}
	}
	return nil
}

func checkID(id string) (string, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if !idPattern.MatchString(id) {
		return "", ErrNotFound
	}
	return id, nil
}

// newToken returns a random session token and the hash that is stored. Only
// the hash reaches the database, so a leaked sessions table opens nothing.
func newToken() (string, []byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	return token, hashToken(token), nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
