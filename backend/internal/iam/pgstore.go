package iam

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database"
)

// PGStore keeps users, sessions and the audit log in Postgres.
type PGStore struct {
	pool *pgxpool.Pool
}

// NewPGStore returns a PGStore using pool.
func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

// userColumns lists the columns scanUser reads, in its order.
const userColumns = `u.id::text, u.email, u.full_name, u.role::text, u.disabled_at IS NOT NULL, u.created_at, u.last_login_at`

func scanUser(row pgx.CollectableRow) (User, error) {
	var u User
	var role string
	var lastLogin *time.Time
	err := row.Scan(&u.ID, &u.Email, &u.FullName, &role, &u.Disabled, &u.CreatedAt, &lastLogin)
	u.Role = Role(role)
	if lastLogin != nil {
		u.LastLoginAt = *lastLogin
	}
	return u, err
}

func (s *PGStore) queryUsers(ctx context.Context, sql string, args ...any) ([]User, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query users: %w", err)
	}
	users, err := pgx.CollectRows(rows, scanUser)
	if err != nil {
		return nil, fmt.Errorf("read users: %w", err)
	}
	return users, nil
}

func (s *PGStore) oneUser(ctx context.Context, sql string, args ...any) (User, error) {
	users, err := s.queryUsers(ctx, sql, args...)
	if err != nil {
		return User{}, err
	}
	if len(users) == 0 {
		return User{}, ErrNotFound
	}
	return users[0], nil
}

// ListUsers implements Store.
func (s *PGStore) ListUsers(ctx context.Context) ([]User, error) {
	return s.queryUsers(ctx, `SELECT `+userColumns+` FROM users u
		ORDER BY u.role <> 'admin', u.disabled_at IS NOT NULL, lower(u.full_name), u.id`)
}

// GetUser implements Store.
func (s *PGStore) GetUser(ctx context.Context, id string) (User, error) {
	return s.oneUser(ctx, `SELECT `+userColumns+` FROM users u WHERE u.id = $1`, id)
}

// UserByEmail implements Store.
func (s *PGStore) UserByEmail(ctx context.Context, email string) (User, string, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+userColumns+`, u.password_hash FROM users u WHERE lower(u.email) = $1`, email)
	if err != nil {
		return User{}, "", fmt.Errorf("query user: %w", err)
	}
	var hash string
	u, err := pgx.CollectExactlyOneRow(rows, func(row pgx.CollectableRow) (User, error) {
		var u User
		var role string
		var lastLogin *time.Time
		err := row.Scan(&u.ID, &u.Email, &u.FullName, &role, &u.Disabled, &u.CreatedAt, &lastLogin, &hash)
		u.Role = Role(role)
		if lastLogin != nil {
			u.LastLoginAt = *lastLogin
		}
		return u, err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrNotFound
	}
	if err != nil {
		return User{}, "", fmt.Errorf("read user: %w", err)
	}
	return u, hash, nil
}

// CreateUser implements Store.
func (s *PGStore) CreateUser(ctx context.Context, u User, passwordHash string) (User, error) {
	created, err := s.oneUser(ctx, `
		INSERT INTO users AS u (email, full_name, role, password_hash)
		VALUES ($1, $2, $3::user_role, $4)
		RETURNING `+userColumns, u.Email, u.FullName, string(u.Role), passwordHash)
	if _, dup := database.UniqueViolation(err); dup {
		return User{}, ErrEmailTaken
	}
	return created, err
}

// UpdateUser implements Store. disabled_at keeps the time of the first
// disabling while the account stays disabled.
func (s *PGStore) UpdateUser(ctx context.Context, u User) (User, error) {
	return s.oneUser(ctx, `
		UPDATE users AS u SET full_name = $2, role = $3::user_role,
			disabled_at = CASE WHEN $4 THEN coalesce(u.disabled_at, now()) END
		WHERE u.id = $1
		RETURNING `+userColumns, u.ID, u.FullName, string(u.Role), u.Disabled)
}

// SetPassword implements Store.
func (s *PGStore) SetPassword(ctx context.Context, id, passwordHash string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, id, passwordHash)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CountActiveAdmins implements Store.
func (s *PGStore) CountActiveAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE role = 'admin' AND disabled_at IS NULL`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count admins: %w", err)
	}
	return n, nil
}

// TouchLogin implements Store.
func (s *PGStore) TouchLogin(ctx context.Context, id string, at time.Time) error {
	if _, err := s.pool.Exec(ctx, `UPDATE users SET last_login_at = $2 WHERE id = $1`, id, at); err != nil {
		return fmt.Errorf("update last login: %w", err)
	}
	return nil
}

// CreateSession implements Store. Expired sessions are swept on the way, so
// the table does not grow without bound.
func (s *PGStore) CreateSession(ctx context.Context, tokenHash []byte, userID string, expiresAt time.Time) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`); err != nil {
			return fmt.Errorf("sweep sessions: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
			tokenHash, userID, expiresAt); err != nil {
			return fmt.Errorf("insert session: %w", err)
		}
		return nil
	})
}

// SessionUser implements Store.
func (s *PGStore) SessionUser(ctx context.Context, tokenHash []byte, now time.Time) (User, error) {
	return s.oneUser(ctx, `SELECT `+userColumns+` FROM sessions ss JOIN users u ON u.id = ss.user_id
		WHERE ss.token_hash = $1 AND ss.expires_at > $2 AND u.disabled_at IS NULL`, tokenHash, now)
}

// DeleteSession implements Store.
func (s *PGStore) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteUserSessions implements Store.
func (s *PGStore) DeleteUserSessions(ctx context.Context, userID string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete sessions: %w", err)
	}
	return nil
}

// AddAudit implements Store.
func (s *PGStore) AddAudit(ctx context.Context, e AuditEntry) error {
	var details map[string]string
	if len(e.Details) > 0 {
		details = e.Details
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_log (user_id, action, entity, entity_id, details)
		VALUES (nullif($1, '')::uuid, $2, nullif($3, ''), nullif($4, ''), $5)`,
		e.UserID, e.Action, e.Entity, e.EntityID, details)
	if err != nil {
		return fmt.Errorf("insert audit entry: %w", err)
	}
	return nil
}

// ListAudit implements Store. Details written by other domains may hold
// non-string values; they are shown as their JSON text.
func (s *PGStore) ListAudit(ctx context.Context, limit, offset int) ([]AuditEntry, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM audit_log`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit log: %w", err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, coalesce(a.user_id::text, ''), coalesce(u.full_name, ''), a.action,
			coalesce(a.entity, ''), coalesce(a.entity_id, ''),
			coalesce((SELECT jsonb_object_agg(k, CASE WHEN jsonb_typeof(v) = 'string' THEN v #>> '{}' ELSE v::text END)
				FROM jsonb_each(CASE WHEN jsonb_typeof(a.details) = 'object' THEN a.details ELSE '{}' END) AS d(k, v)), '{}'),
			a.created_at
		FROM audit_log a LEFT JOIN users u ON u.id = a.user_id
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query audit log: %w", err)
	}
	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (AuditEntry, error) {
		var e AuditEntry
		err := row.Scan(&e.ID, &e.UserID, &e.UserName, &e.Action, &e.Entity, &e.EntityID, &e.Details, &e.CreatedAt)
		return e, err
	})
	if err != nil {
		return nil, 0, fmt.Errorf("read audit log: %w", err)
	}
	return entries, total, nil
}

var _ Store = (*PGStore)(nil)
