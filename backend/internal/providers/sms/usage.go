package sms

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UsageFilter specifies optional criteria for aggregating SMS usage.
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

// RawUsageCounts holds aggregate message and part numbers from storage.
type RawUsageCounts struct {
	TotalMessages     int
	TotalParts        int
	DeliveredMessages int
	DeliveredParts    int
	SentMessages      int
	SentParts         int
	FailedMessages    int
	FailedParts       int
	InFlightMessages  int
	InFlightParts     int
}

// UsageStats presents SMS usage statistics including costs in thousandths of PLN (milli-PLN).
type UsageStats struct {
	PricePerPartMilli  int64 `json:"pricePerPartMilli"`
	TotalMessages      int   `json:"totalMessages"`
	TotalParts         int   `json:"totalParts"`
	PlannedCostMilli   int64 `json:"plannedCostMilli"`
	BilledCostMilli    int64 `json:"billedCostMilli"`
	DeliveredMessages  int   `json:"deliveredMessages"`
	DeliveredParts     int   `json:"deliveredParts"`
	DeliveredCostMilli int64 `json:"deliveredCostMilli"`
	SentMessages       int   `json:"sentMessages"`
	SentParts          int   `json:"sentParts"`
	FailedMessages     int   `json:"failedMessages"`
	FailedParts        int   `json:"failedParts"`
	InFlightMessages   int   `json:"inFlightMessages"`
	InFlightParts      int   `json:"inFlightParts"`
}

// CalculateUsageStats combines raw delivery counts with a unit price to produce complete statistics.
// Planned cost reflects all planned SMS parts.
// Billed cost reflects parts that were handed over to the network (sent + delivered).
func CalculateUsageStats(counts RawUsageCounts, pricePerPartMilli int64) UsageStats {
	plannedCost := int64(counts.TotalParts) * pricePerPartMilli
	billedCost := int64(counts.DeliveredParts+counts.SentParts) * pricePerPartMilli
	deliveredCost := int64(counts.DeliveredParts) * pricePerPartMilli

	return UsageStats{
		PricePerPartMilli:  pricePerPartMilli,
		TotalMessages:      counts.TotalMessages,
		TotalParts:         counts.TotalParts,
		PlannedCostMilli:   plannedCost,
		BilledCostMilli:    billedCost,
		DeliveredMessages:  counts.DeliveredMessages,
		DeliveredParts:     counts.DeliveredParts,
		DeliveredCostMilli: deliveredCost,
		SentMessages:       counts.SentMessages,
		SentParts:          counts.SentParts,
		FailedMessages:     counts.FailedMessages,
		FailedParts:        counts.FailedParts,
		InFlightMessages:   counts.InFlightMessages,
		InFlightParts:      counts.InFlightParts,
	}
}

// UsageStore supplies aggregated SMS usage metrics.
type UsageStore interface {
	GetUsage(ctx context.Context, filter UsageFilter) (RawUsageCounts, error)
}

// PGUsageStore aggregates SMS usage metrics directly from the deliveries and message_batches tables in PostgreSQL.
type PGUsageStore struct {
	pool *pgxpool.Pool
}

// NewPGUsageStore creates a PGUsageStore.
func NewPGUsageStore(pool *pgxpool.Pool) *PGUsageStore {
	return &PGUsageStore{pool: pool}
}

// GetUsage executes an aggregation query joining deliveries, batch_recipients and message_batches.
func (s *PGUsageStore) GetUsage(ctx context.Context, filter UsageFilter) (RawUsageCounts, error) {
	if s == nil || s.pool == nil {
		return RawUsageCounts{}, errors.New("database pool is not configured")
	}

	query := `
		SELECT
			COUNT(*)::int AS total_messages,
			COALESCE(SUM(d.parts), 0)::int AS total_parts,
			COUNT(*) FILTER (WHERE d.status = 'delivered')::int AS delivered_messages,
			COALESCE(SUM(d.parts) FILTER (WHERE d.status = 'delivered'), 0)::int AS delivered_parts,
			COUNT(*) FILTER (WHERE d.status = 'sent')::int AS sent_messages,
			COALESCE(SUM(d.parts) FILTER (WHERE d.status = 'sent'), 0)::int AS sent_parts,
			COUNT(*) FILTER (WHERE d.status = 'failed')::int AS failed_messages,
			COALESCE(SUM(d.parts) FILTER (WHERE d.status = 'failed'), 0)::int AS failed_parts,
			COUNT(*) FILTER (WHERE d.status IN ('pending', 'sending'))::int AS in_flight_messages,
			COALESCE(SUM(d.parts) FILTER (WHERE d.status IN ('pending', 'sending')), 0)::int AS in_flight_parts
		FROM deliveries d
		JOIN batch_recipients br ON br.id = d.batch_recipient_id
		JOIN message_batches mb ON mb.id = br.batch_id
	`

	conditions := []string{"d.channel = 'sms'"}
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

	var counts RawUsageCounts
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&counts.TotalMessages,
		&counts.TotalParts,
		&counts.DeliveredMessages,
		&counts.DeliveredParts,
		&counts.SentMessages,
		&counts.SentParts,
		&counts.FailedMessages,
		&counts.FailedParts,
		&counts.InFlightMessages,
		&counts.InFlightParts,
	)
	if err != nil {
		return RawUsageCounts{}, fmt.Errorf("query sms usage stats: %w", err)
	}

	return counts, nil
}
