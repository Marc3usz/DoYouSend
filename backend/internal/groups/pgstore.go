package groups

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database"
	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

// PGStore is the Postgres Store. It only ever touches custom groups: rows of
// built-in groups (is_system, migration 0002) are invisible to every method,
// so they cannot be renamed, deleted or given members through it.
type PGStore struct {
	pool *pgxpool.Pool
}

// NewPGStore returns a PGStore using pool.
func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

const groupColumns = `id::text, name, coalesce(description, ''), created_at`

// ListGroups implements Store.
func (s *PGStore) ListGroups(ctx context.Context) ([]Group, error) {
	return s.queryGroups(ctx, `SELECT `+groupColumns+` FROM groups WHERE NOT is_system`)
}

// FindGroups implements Store.
func (s *PGStore) FindGroups(ctx context.Context, ids []string) ([]Group, error) {
	ids = validIDs(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	return s.queryGroups(ctx, `SELECT `+groupColumns+` FROM groups WHERE NOT is_system AND id = ANY($1::uuid[])`, ids)
}

// CreateGroup implements Store.
func (s *PGStore) CreateGroup(ctx context.Context, g Group) (Group, error) {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO groups (name, description, is_system) VALUES ($1, nullif($2, ''), false)
		RETURNING id::text, created_at`,
		g.Name, g.Description).Scan(&g.ID, &g.CreatedAt)
	if err != nil {
		return Group{}, fmt.Errorf("insert group: %w", nameError(g.Name, err))
	}
	g.Kind = KindCustom
	g.Rule = nil
	return g, nil
}

// UpdateGroup implements Store.
func (s *PGStore) UpdateGroup(ctx context.Context, g Group) error {
	id := canonicalID(g.ID)
	if !IsValidID(id) {
		return fmt.Errorf("group %s: %w", g.ID, ErrNotFound)
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE groups SET name = $2, description = nullif($3, '')
		WHERE id = $1 AND NOT is_system`,
		id, g.Name, g.Description)
	if err != nil {
		return fmt.Errorf("update group %s: %w", id, nameError(g.Name, err))
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("group %s: %w", id, ErrNotFound)
	}
	return nil
}

// DeleteGroup implements Store. Memberships go with it (ON DELETE CASCADE).
func (s *PGStore) DeleteGroup(ctx context.Context, id string) error {
	id = canonicalID(id)
	if !IsValidID(id) {
		return fmt.Errorf("group %s: %w", id, ErrNotFound)
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM groups WHERE id = $1 AND NOT is_system`, id)
	if err != nil {
		return fmt.Errorf("delete group %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("group %s: %w", id, ErrNotFound)
	}
	return nil
}

// Members implements Store. Member IDs are ordered by recipient ID: the table
// keeps no insertion order, and callers sort for display anyway.
func (s *PGStore) Members(ctx context.Context, groupIDs []string) (map[string][]string, error) {
	out := make(map[string][]string)
	ids := validIDs(groupIDs)
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT m.group_id::text, m.recipient_id::text
		FROM group_members m JOIN groups g ON g.id = m.group_id
		WHERE NOT g.is_system AND m.group_id = ANY($1::uuid[])
		ORDER BY m.group_id, m.recipient_id`, ids)
	if err != nil {
		return nil, fmt.Errorf("query members: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var groupID, recipientID string
		if err := rows.Scan(&groupID, &recipientID); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		out[groupID] = append(out[groupID], recipientID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read members: %w", err)
	}
	return out, nil
}

// AddMembers implements Store. A recipient ID with no recipient fails the whole
// call with ErrUnknownRecipient (the foreign key); the Service checks IDs
// first, so this only happens when a recipient is deleted meanwhile.
func (s *PGStore) AddMembers(ctx context.Context, groupID string, recipientIDs []string) (int, error) {
	var added int
	err := s.withCustomGroup(ctx, groupID, func(tx pgx.Tx, id string) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO group_members (group_id, recipient_id)
			SELECT $1, r FROM unnest($2::uuid[]) AS r
			ON CONFLICT DO NOTHING`, id, validIDs(recipientIDs))
		if _, fk := database.ForeignKeyViolation(err); fk {
			return fmt.Errorf("add members to group %s: %w", id, ErrUnknownRecipient)
		}
		if err != nil {
			return fmt.Errorf("add members to group %s: %w", id, err)
		}
		added = int(tag.RowsAffected())
		return nil
	})
	return added, err
}

// RemoveMembers implements Store.
func (s *PGStore) RemoveMembers(ctx context.Context, groupID string, recipientIDs []string) (int, error) {
	var removed int
	err := s.withCustomGroup(ctx, groupID, func(tx pgx.Tx, id string) error {
		tag, err := tx.Exec(ctx, `
			DELETE FROM group_members WHERE group_id = $1 AND recipient_id = ANY($2::uuid[])`,
			id, validIDs(recipientIDs))
		if err != nil {
			return fmt.Errorf("remove members from group %s: %w", id, err)
		}
		removed = int(tag.RowsAffected())
		return nil
	})
	return removed, err
}

// withCustomGroup runs fn in a transaction holding a lock on the custom group
// groupID, so the group cannot be deleted halfway through a membership change.
// It fails with ErrNotFound for an unknown or built-in group.
func (s *PGStore) withCustomGroup(ctx context.Context, groupID string, fn func(tx pgx.Tx, id string) error) error {
	id := canonicalID(groupID)
	if !IsValidID(id) {
		return fmt.Errorf("group %s: %w", groupID, ErrNotFound)
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var found string
		err := tx.QueryRow(ctx, `SELECT id::text FROM groups WHERE id = $1 AND NOT is_system FOR UPDATE`, id).Scan(&found)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("group %s: %w", id, ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("lock group %s: %w", id, err)
		}
		return fn(tx, id)
	})
}

func (s *PGStore) queryGroups(ctx context.Context, sql string, args ...any) ([]Group, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query groups: %w", err)
	}
	gs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Group, error) {
		g := Group{Kind: KindCustom}
		err := row.Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt)
		return g, err
	})
	if err != nil {
		return nil, fmt.Errorf("read groups: %w", err)
	}
	return gs, nil
}

// nameError turns a violation of either unique index on groups.name into
// ErrNameTaken. The service has already rejected the names of built-in groups.
func nameError(name string, err error) error {
	if _, ok := database.UniqueViolation(err); ok {
		return fmt.Errorf("group name %q: %w", name, ErrNameTaken)
	}
	return err
}

// validIDs canonicalizes ids and drops repeats and malformed IDs, which
// Postgres would otherwise reject for the whole query.
func validIDs(ids []string) []string {
	out := []string{}
	for _, id := range uniqueIDs(ids) {
		if IsValidID(id) {
			out = append(out, id)
		}
	}
	return out
}

// Compile-time checks that the Postgres stores fit what this package consumes.
var (
	_ Store     = (*PGStore)(nil)
	_ Directory = (*recipients.PGStore)(nil)
)
