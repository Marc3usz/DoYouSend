package settings

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGStore keeps panel settings in the app_settings table.
type PGStore struct{ pool *pgxpool.Pool }

// NewPGStore returns a PGStore using pool.
func NewPGStore(pool *pgxpool.Pool) *PGStore { return &PGStore{pool: pool} }

// ListSettings implements Store.
func (s *PGStore) ListSettings(ctx context.Context) ([]Row, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT key, value, encrypted, updated_at, coalesce(updated_by::text, '') FROM app_settings ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("query settings: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Row, error) {
		var r Row
		err := row.Scan(&r.Key, &r.Value, &r.Encrypted, &r.UpdatedAt, &r.UpdatedBy)
		return r, err
	})
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}
	return out, nil
}

// SaveSettings implements Store.
func (s *PGStore) SaveSettings(ctx context.Context, set []Row, remove []string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if len(remove) > 0 {
			if _, err := tx.Exec(ctx, `DELETE FROM app_settings WHERE key = ANY($1)`, remove); err != nil {
				return fmt.Errorf("delete settings: %w", err)
			}
		}
		for _, r := range set {
			_, err := tx.Exec(ctx, `
				INSERT INTO app_settings (key, value, encrypted, updated_at, updated_by)
				VALUES ($1, $2, $3, $4, nullif($5, '')::uuid)
				ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, encrypted = EXCLUDED.encrypted,
					updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by`,
				r.Key, r.Value, r.Encrypted, r.UpdatedAt, r.UpdatedBy)
			if err != nil {
				return fmt.Errorf("save setting %s: %w", r.Key, err)
			}
		}
		return nil
	})
}
