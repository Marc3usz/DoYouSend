package email

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UsageFilter specifies optional criteria for aggregating email usage.
type UsageFilter struct {
	// BatchID filters deliveries belonging to a specific batch UUID.
	BatchID *string

	// From filters deliveries whose batch was created or confirmed at or after this timestamp.
	From *time.Time

	// To filters deliveries whose batch was created or confirmed before or at this timestamp.
	To *time.Time

	// ToExclusive specifies whether the To bound is strictly exclusive (< To).
	ToExclusive bool
}

// UsageStats presents email usage statistics.
type UsageStats struct {
	TotalMessages     int `json:"totalMessages"`
	DeliveredMessages int `json:"deliveredMessages"`
	SentMessages      int `json:"sentMessages"`
	FailedMessages    int `json:"failedMessages"`
	InFlightMessages  int `json:"inFlightMessages"`
}

// UsageStore supplies aggregated email usage metrics.
type UsageStore interface {
	GetUsage(ctx context.Context, filter UsageFilter) (UsageStats, error)
}

// PGUsageStore aggregates email usage metrics directly from the deliveries and message_batches tables in PostgreSQL.
type PGUsageStore struct {
	pool *pgxpool.Pool
}

// NewPGUsageStore creates a PGUsageStore.
func NewPGUsageStore(pool *pgxpool.Pool) *PGUsageStore {
	return &PGUsageStore{pool: pool}
}

// GetUsage executes an aggregation query joining deliveries, batch_recipients and message_batches.
func (s *PGUsageStore) GetUsage(ctx context.Context, filter UsageFilter) (UsageStats, error) {
	if s == nil || s.pool == nil {
		return UsageStats{}, errors.New("database pool is not configured")
	}

	query := `
		SELECT
			COUNT(*)::int AS total_messages,
			COUNT(*) FILTER (WHERE d.status = 'delivered')::int AS delivered_messages,
			COUNT(*) FILTER (WHERE d.status = 'sent')::int AS sent_messages,
			COUNT(*) FILTER (WHERE d.status = 'failed')::int AS failed_messages,
			COUNT(*) FILTER (WHERE d.status IN ('pending', 'sending'))::int AS in_flight_messages
		FROM deliveries d
		JOIN batch_recipients br ON br.id = d.batch_recipient_id
		JOIN message_batches mb ON mb.id = br.batch_id
	`

	conditions := []string{"d.channel = 'email'"}
	var args []any
	argIdx := 1

	if filter.BatchID != nil && strings.TrimSpace(*filter.BatchID) != "" {
		conditions = append(conditions, fmt.Sprintf("br.batch_id = $%d", argIdx))
		args = append(args, strings.TrimSpace(*filter.BatchID))
		argIdx++
	}

	if filter.From != nil {
		conditions = append(conditions, fmt.Sprintf("COALESCE(mb.confirmed_at, mb.created_at) >= $%d", argIdx))
		args = append(args, *filter.From)
		argIdx++
	}

	if filter.To != nil {
		op := "<="
		if filter.ToExclusive {
			op = "<"
		}
		conditions = append(conditions, fmt.Sprintf("COALESCE(mb.confirmed_at, mb.created_at) %s $%d", op, argIdx))
		args = append(args, *filter.To)
		argIdx++
	}

	query += " WHERE " + strings.Join(conditions, " AND ")

	var stats UsageStats
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&stats.TotalMessages,
		&stats.DeliveredMessages,
		&stats.SentMessages,
		&stats.FailedMessages,
		&stats.InFlightMessages,
	)
	if err != nil {
		return UsageStats{}, fmt.Errorf("query email usage stats: %w", err)
	}

	return stats, nil
}
