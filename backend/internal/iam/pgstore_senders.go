package iam

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database"
)

// senderColumns lists the columns scanSender reads, in its order.
const senderColumns = `user_id::text, provider, provider_sender_id, from_email, from_name, status,
	created_at, checked_at, verified_at`

func scanSender(row pgx.CollectableRow) (SenderIdentity, error) {
	var s SenderIdentity
	var status string
	var checked, verified *time.Time
	err := row.Scan(&s.UserID, &s.Provider, &s.ProviderID, &s.FromEmail, &s.FromName, &status,
		&s.CreatedAt, &checked, &verified)
	s.Status = SenderStatus(status)
	if checked != nil {
		s.CheckedAt = *checked
	}
	if verified != nil {
		s.VerifiedAt = *verified
	}
	return s, err
}

func (s *PGStore) querySenders(ctx context.Context, sql string, args ...any) ([]SenderIdentity, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query sender identities: %w", err)
	}
	out, err := pgx.CollectRows(rows, scanSender)
	if err != nil {
		return nil, fmt.Errorf("read sender identities: %w", err)
	}
	return out, nil
}

func (s *PGStore) oneSender(ctx context.Context, sql string, args ...any) (SenderIdentity, error) {
	out, err := s.querySenders(ctx, sql, args...)
	if err != nil {
		return SenderIdentity{}, err
	}
	if len(out) == 0 {
		return SenderIdentity{}, ErrSenderNotConfigured
	}
	return out[0], nil
}

// GetSenderIdentity implements SenderStore.
func (s *PGStore) GetSenderIdentity(ctx context.Context, userID string) (SenderIdentity, error) {
	return s.oneSender(ctx, `SELECT `+senderColumns+` FROM user_sender_identities WHERE user_id = $1`, userID)
}

// ListSenderIdentities implements SenderStore.
func (s *PGStore) ListSenderIdentities(ctx context.Context) ([]SenderIdentity, error) {
	return s.querySenders(ctx, `SELECT `+senderColumns+` FROM user_sender_identities ORDER BY created_at, user_id`)
}

// CreateSenderIdentity implements SenderStore.
func (s *PGStore) CreateSenderIdentity(ctx context.Context, in SenderIdentity) (SenderIdentity, error) {
	out, err := s.oneSender(ctx, `
		INSERT INTO user_sender_identities
			(user_id, provider, provider_sender_id, from_email, from_name, status, created_at, verified_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, CASE WHEN $6 = 'verified' THEN $7::timestamptz END)
		RETURNING `+senderColumns,
		in.UserID, in.Provider, in.ProviderID, in.FromEmail, in.FromName, string(in.Status), in.CreatedAt)
	if _, dup := database.UniqueViolation(err); dup {
		return SenderIdentity{}, ErrSenderExists
	}
	if _, fk := database.ForeignKeyViolation(err); fk {
		return SenderIdentity{}, ErrNotFound
	}
	return out, err
}

// SaveSenderStatus implements SenderStore.
func (s *PGStore) SaveSenderStatus(ctx context.Context, userID string, status SenderStatus, checkedAt time.Time) (SenderIdentity, error) {
	return s.oneSender(ctx, `
		UPDATE user_sender_identities SET status = $2, checked_at = $3,
			verified_at = CASE WHEN $2 = 'verified' THEN coalesce(verified_at, $3::timestamptz) END
		WHERE user_id = $1
		RETURNING `+senderColumns, userID, string(status), checkedAt)
}

// DeleteSenderIdentity implements SenderStore.
func (s *PGStore) DeleteSenderIdentity(ctx context.Context, userID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM user_sender_identities WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete sender identity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSenderNotConfigured
	}
	return nil
}
