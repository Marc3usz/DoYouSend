// Package database owns the Postgres connection pool and the helpers every
// domain store shares. Shared plumbing - coordinate before editing.
package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNoURL is returned by Open when DATABASE_URL is empty.
var ErrNoURL = errors.New("DATABASE_URL is not set")

// Open connects to Postgres at url and checks the connection with a ping, so a
// wrong URL fails at startup rather than on the first request.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	if url == "" {
		return nil, ErrNoURL
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		// The URL may carry a password: never echo it back.
		return nil, errors.New("parse DATABASE_URL: invalid connection string")
	}
	return OpenConfig(ctx, cfg)
}

// OpenConfig is Open for an already parsed configuration (tests use it to set
// a search_path).
func OpenConfig(ctx context.Context, cfg *pgxpool.Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// SQLSTATE codes the stores map to domain errors.
const (
	codeUniqueViolation     = "23505"
	codeForeignKeyViolation = "23503"
)

// UniqueViolation reports whether err is a unique-constraint violation and, if
// so, the name of the violated constraint or index.
func UniqueViolation(err error) (constraint string, ok bool) {
	return violation(err, codeUniqueViolation)
}

// ForeignKeyViolation reports whether err is a foreign-key violation and, if
// so, the name of the violated constraint.
func ForeignKeyViolation(err error) (constraint string, ok bool) {
	return violation(err, codeForeignKeyViolation)
}

func violation(err error, code string) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == code {
		return pgErr.ConstraintName, true
	}
	return "", false
}
