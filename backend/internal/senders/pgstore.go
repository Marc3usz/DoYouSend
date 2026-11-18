package senders

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database"
)

// PGStore is the Postgres Store (migration 0012). It also reads users (names
// of delegated users) and user_sender_identities (ADR-0016), both owned by iam.
type PGStore struct{ pool *pgxpool.Pool }

// NewPGStore returns a PGStore using pool.
func NewPGStore(pool *pgxpool.Pool) *PGStore { return &PGStore{pool: pool} }

const domainColumns = `id::text, domain, provider, provider_domain_id, status, dns_records,
	created_at, checked_at, validated_at`

func scanDomain(row pgx.CollectableRow) (Domain, error) {
	var (
		d                  Domain
		status             string
		records            []byte
		checked, validated *time.Time
	)
	if err := row.Scan(&d.ID, &d.Domain, &d.Provider, &d.ProviderID, &status, &records,
		&d.CreatedAt, &checked, &validated); err != nil {
		return Domain{}, err
	}
	d.Status = DomainStatus(status)
	if err := json.Unmarshal(records, &d.Records); err != nil {
		return Domain{}, fmt.Errorf("decode dns records: %w", err)
	}
	if checked != nil {
		d.CheckedAt = *checked
	}
	if validated != nil {
		d.ValidatedAt = *validated
	}
	return d, nil
}

func (s *PGStore) queryDomains(ctx context.Context, sql string, args ...any) ([]Domain, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query sender domains: %w", err)
	}
	ds, err := pgx.CollectRows(rows, scanDomain)
	if err != nil {
		return nil, fmt.Errorf("read sender domains: %w", err)
	}
	return ds, nil
}

func (s *PGStore) oneDomain(ctx context.Context, sql string, args ...any) (Domain, error) {
	ds, err := s.queryDomains(ctx, sql, args...)
	if err != nil {
		return Domain{}, err
	}
	if len(ds) == 0 {
		return Domain{}, ErrNotFound
	}
	return ds[0], nil
}

// ListDomains implements Store.
func (s *PGStore) ListDomains(ctx context.Context) ([]Domain, error) {
	return s.queryDomains(ctx, `SELECT `+domainColumns+` FROM sender_domains ORDER BY domain`)
}

// GetDomain implements Store.
func (s *PGStore) GetDomain(ctx context.Context, id string) (Domain, error) {
	if !uuidLike.MatchString(strings.ToLower(id)) {
		return Domain{}, ErrNotFound
	}
	return s.oneDomain(ctx, `SELECT `+domainColumns+` FROM sender_domains WHERE id = $1`, id)
}

// CreateDomain implements Store.
func (s *PGStore) CreateDomain(ctx context.Context, d Domain, actorID string) (Domain, error) {
	records, err := json.Marshal(nonNil(d.Records))
	if err != nil {
		return Domain{}, fmt.Errorf("encode dns records: %w", err)
	}
	out, err := s.oneDomain(ctx, `
		INSERT INTO sender_domains (domain, provider, provider_domain_id, status, dns_records,
			created_by, created_at, checked_at, validated_at)
		VALUES ($1, $2, $3, $4, $5, nullif($6, '')::uuid, $7, $7, CASE WHEN $4 = 'valid' THEN $7::timestamptz END)
		RETURNING `+domainColumns,
		d.Domain, d.Provider, d.ProviderID, string(d.Status), records, actorID, d.CreatedAt)
	if _, ok := database.UniqueViolation(err); ok {
		return Domain{}, ErrDomainExists
	}
	return out, err
}

// SaveDomainCheck implements Store. validated_at keeps the first time the domain was valid.
func (s *PGStore) SaveDomainCheck(ctx context.Context, id string, status DomainStatus, records []DNSRecord, checkedAt time.Time) (Domain, error) {
	raw, err := json.Marshal(nonNil(records))
	if err != nil {
		return Domain{}, fmt.Errorf("encode dns records: %w", err)
	}
	return s.oneDomain(ctx, `
		UPDATE sender_domains
		SET status = $2, dns_records = $3, checked_at = $4,
			validated_at = CASE WHEN $2 = 'valid' THEN coalesce(validated_at, $4) ELSE validated_at END
		WHERE id = $1
		RETURNING `+domainColumns, id, string(status), raw, checkedAt)
}

// DeleteDomain implements Store.
func (s *PGStore) DeleteDomain(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM sender_domains WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete sender domain: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DomainValid implements Store.
func (s *PGStore) DomainValid(ctx context.Context, provider, domain string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM sender_domains WHERE provider = $1 AND domain = $2 AND status = 'valid')`,
		provider, strings.ToLower(domain)).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("query sender domain: %w", err)
	}
	return ok, nil
}

// SingleSender implements Store.
func (s *PGStore) SingleSender(ctx context.Context, provider, email string) (exists, verified bool, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT count(*) > 0, coalesce(bool_or(status = 'verified'), false)
		FROM user_sender_identities WHERE provider = $1 AND lower(from_email) = lower($2)`,
		provider, email).Scan(&exists, &verified)
	if err != nil {
		return false, false, fmt.Errorf("query single sender: %w", err)
	}
	return exists, verified, nil
}

// addressSelect reads addresses with their users as a JSON array, so one query
// returns everything; where filters sender_addresses a.
const addressSelect = `
	SELECT a.id::text, a.email, a.name, a.created_at,
		coalesce((
			SELECT json_agg(json_build_object('id', u.id::text, 'fullName', u.full_name) ORDER BY u.full_name, u.id)
			FROM sender_address_users au JOIN users u ON u.id = au.user_id
			WHERE au.address_id = a.id
		), '[]'::json)
	FROM sender_addresses a `

func scanAddress(row pgx.CollectableRow) (Address, error) {
	var (
		a     Address
		users []byte
	)
	if err := row.Scan(&a.ID, &a.Email, &a.Name, &a.CreatedAt, &users); err != nil {
		return Address{}, err
	}
	var refs []struct {
		ID       string `json:"id"`
		FullName string `json:"fullName"`
	}
	if err := json.Unmarshal(users, &refs); err != nil {
		return Address{}, fmt.Errorf("decode address users: %w", err)
	}
	a.Users = make([]UserRef, len(refs))
	for i, r := range refs {
		a.Users[i] = UserRef{ID: r.ID, FullName: r.FullName}
	}
	return a, nil
}

func queryAddresses(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, sql string, args ...any) ([]Address, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query sender addresses: %w", err)
	}
	as, err := pgx.CollectRows(rows, scanAddress)
	if err != nil {
		return nil, fmt.Errorf("read sender addresses: %w", err)
	}
	return as, nil
}

// ListAddresses implements Store.
func (s *PGStore) ListAddresses(ctx context.Context) ([]Address, error) {
	return queryAddresses(ctx, s.pool, addressSelect+`ORDER BY a.email`)
}

// GetAddress implements Store.
func (s *PGStore) GetAddress(ctx context.Context, id string) (Address, error) {
	return getAddress(ctx, s.pool, id)
}

func getAddress(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, id string) (Address, error) {
	if !uuidLike.MatchString(strings.ToLower(id)) {
		return Address{}, ErrNotFound
	}
	as, err := queryAddresses(ctx, q, addressSelect+`WHERE a.id = $1`, id)
	if err != nil {
		return Address{}, err
	}
	if len(as) == 0 {
		return Address{}, ErrNotFound
	}
	return as[0], nil
}

// AddressesOf implements Store.
func (s *PGStore) AddressesOf(ctx context.Context, userID string) ([]Address, error) {
	if !uuidLike.MatchString(strings.ToLower(userID)) {
		return nil, nil
	}
	return queryAddresses(ctx, s.pool, addressSelect+`
		WHERE EXISTS (SELECT 1 FROM sender_address_users au WHERE au.address_id = a.id AND au.user_id = $1)
		ORDER BY a.email`, userID)
}

// CreateAddress implements Store.
func (s *PGStore) CreateAddress(ctx context.Context, a Address, userIDs []string, actorID string) (Address, error) {
	var out Address
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO sender_addresses (email, name, created_by) VALUES ($1, $2, nullif($3, '')::uuid)
			RETURNING id::text`, a.Email, a.Name, actorID).Scan(&id)
		if _, ok := database.UniqueViolation(err); ok {
			return ErrAddressExists
		}
		if err != nil {
			return fmt.Errorf("insert sender address: %w", err)
		}
		if err := setUsers(ctx, tx, id, userIDs, actorID); err != nil {
			return err
		}
		out, err = getAddress(ctx, tx, id)
		return err
	})
	return out, err
}

// UpdateAddress implements Store.
func (s *PGStore) UpdateAddress(ctx context.Context, id string, name *string, userIDs []string, replaceUsers bool, actorID string) (Address, error) {
	if !uuidLike.MatchString(strings.ToLower(id)) {
		return Address{}, ErrNotFound
	}
	var out Address
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE sender_addresses SET name = coalesce($2, name), updated_at = now() WHERE id = $1`, id, name)
		if err != nil {
			return fmt.Errorf("update sender address: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if replaceUsers {
			if err := setUsers(ctx, tx, id, userIDs, actorID); err != nil {
				return err
			}
		}
		out, err = getAddress(ctx, tx, id)
		return err
	})
	return out, err
}

// setUsers replaces the users of an address; every ID must be an active user.
func setUsers(ctx context.Context, tx pgx.Tx, addressID string, userIDs []string, actorID string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM sender_address_users WHERE address_id = $1`, addressID); err != nil {
		return fmt.Errorf("clear address users: %w", err)
	}
	if len(userIDs) == 0 {
		return nil
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO sender_address_users (address_id, user_id, granted_by)
		SELECT $1, u.id, nullif($3, '')::uuid FROM users u
		WHERE u.id = ANY($2::uuid[]) AND u.disabled_at IS NULL`, addressID, userIDs, actorID)
	if err != nil {
		return fmt.Errorf("insert address users: %w", err)
	}
	if int(tag.RowsAffected()) != len(userIDs) {
		return ErrUnknownUsers
	}
	return nil
}

// DeleteAddress implements Store.
func (s *PGStore) DeleteAddress(ctx context.Context, id string) error {
	if !uuidLike.MatchString(strings.ToLower(id)) {
		return ErrNotFound
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM sender_addresses WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete sender address: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// nonNil turns a nil slice into an empty one, so it is stored as [], not null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
