package iam

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

// Role is what a user may do (description.md "Uzytkownicy systemu",
// docs/bezpieczenstwo.md "Uprawnienia").
type Role string

const (
	// RoleAdmin manages users, recipients, groups and the sending setup.
	RoleAdmin Role = "admin"
	// RoleSender composes and sends messages (the director or another
	// authorised employee).
	RoleSender Role = "sender"
)

// Valid reports whether r is a known role.
func (r Role) Valid() bool { return r == RoleAdmin || r == RoleSender }

// User mirrors a row of the users table. The password hash never leaves the
// package.
type User struct {
	ID          string
	Email       string
	FullName    string
	Role        Role
	Disabled    bool
	CreatedAt   time.Time
	LastLoginAt time.Time // zero when the user never logged in
}

// IsAdmin reports whether u may use the administration panel.
func (u User) IsAdmin() bool { return u.Role == RoleAdmin }

// Errors of the iam package. Match them with errors.Is.
var (
	ErrNotFound           = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid e-mail or password")
	ErrTooManyAttempts    = errors.New("too many failed login attempts")
	ErrUnauthenticated    = errors.New("not logged in")
	ErrForbidden          = errors.New("not allowed for this role")
	ErrEmailTaken         = errors.New("e-mail is already used by another user")
	// ErrLastAdmin protects against locking everybody out of the panel.
	ErrLastAdmin = errors.New("the last active administrator cannot be demoted or disabled")
	// ErrSelfLockout keeps administrators from disabling or demoting themselves.
	ErrSelfLockout  = errors.New("you cannot disable or demote your own account")
	ErrInvalidInput = errors.New("invalid input")
)

// FieldError attaches a validation message to a field of the request.
type FieldError struct {
	Field   string
	Message string
}

// ValidationError carries every field problem of one rejected request. It
// matches ErrInvalidInput with errors.Is.
type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	msgs := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		msgs[i] = f.Field + ": " + f.Message
	}
	return "invalid input: " + strings.Join(msgs, "; ")
}

func (e *ValidationError) Is(target error) bool { return target == ErrInvalidInput }

// emailPattern is deliberately loose: staff accounts are created by an
// administrator, who sees the address they type.
var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// normalizeEmail is the form e-mails are stored and compared in.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// AuditEntry is one row of audit_log: who did what to which entity.
type AuditEntry struct {
	ID        int64
	UserID    string // empty for actions without a known user (a failed login)
	UserName  string // full name of UserID at read time
	Action    string
	Entity    string
	EntityID  string
	Details   map[string]string
	CreatedAt time.Time
}

// Audit actions written by this package. Other domains add their own (e.g.
// batch_confirmed), the panel shows unknown ones as they are.
const (
	ActionLogin          = "login"
	ActionLoginFailed    = "login_failed"
	ActionLogout         = "logout"
	ActionUserCreated    = "user_created"
	ActionUserUpdated    = "user_updated"
	ActionPasswordReset  = "password_reset"
	ActionPasswordChange = "password_changed"
)

// Store is the storage the Service needs; PGStore implements it.
type Store interface {
	ListUsers(ctx context.Context) ([]User, error)
	GetUser(ctx context.Context, id string) (User, error)
	// UserByEmail returns the user and their password hash, or ErrNotFound.
	UserByEmail(ctx context.Context, email string) (User, string, error)
	// CreateUser stores u with passwordHash; a taken e-mail fails with ErrEmailTaken.
	CreateUser(ctx context.Context, u User, passwordHash string) (User, error)
	// UpdateUser stores the name, role and disabled flag of u.ID.
	UpdateUser(ctx context.Context, u User) (User, error)
	SetPassword(ctx context.Context, id, passwordHash string) error
	// CountActiveAdmins returns how many enabled administrators there are.
	CountActiveAdmins(ctx context.Context) (int, error)
	TouchLogin(ctx context.Context, id string, at time.Time) error

	CreateSession(ctx context.Context, tokenHash []byte, userID string, expiresAt time.Time) error
	// SessionUser returns the enabled user of an unexpired session, or ErrNotFound.
	SessionUser(ctx context.Context, tokenHash []byte, now time.Time) (User, error)
	DeleteSession(ctx context.Context, tokenHash []byte) error
	// DeleteUserSessions logs a user out everywhere.
	DeleteUserSessions(ctx context.Context, userID string) error

	AddAudit(ctx context.Context, e AuditEntry) error
	// ListAudit returns a page of entries, newest first, and the total count.
	ListAudit(ctx context.Context, limit, offset int) ([]AuditEntry, int, error)
}
