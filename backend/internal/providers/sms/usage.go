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
	BatchID	*string

	// From filters deliveries updated at or after this timestamp.
	From	*time.Time

	// To filters deliveries updated at or before this timestamp.
	To	*time.Time
}

// RawUsageCounts holds aggregate message and part numbers from storage.
type RawUsageCounts struct {
	TotalMessages		int
	TotalParts		int
	DeliveredMessages	int
	DeliveredParts		int
	SentMessages		int
	SentParts		int
	FailedMessages		int
	FailedParts		int
	InFlightMessages	int
	InFlightParts		int
}

// UsageStats presents human-readable and machine-readable SMS statistics
// including costs in thousandths of PLN and formatted PLN currency strings.
type UsageStats struct {
	PricePerPartMilli	int64	`json:"pricePerPartMilli"`
	PricePerPartPLN		string	`json:"pricePerPartPln"`
	TotalMessages		int	`json:"totalMessages"`
	TotalParts		int	`json:"totalParts"`
	TotalCostMilli		int64	`json:"totalCostMilli"`
	TotalCostPLN		string	`json:"totalCostPln"`
	DeliveredMessages	int	`json:"deliveredMessages"`
	DeliveredParts		int	`json:"deliveredParts"`
	DeliveredCostMilli	int64	`json:"deliveredCostMilli"`
	DeliveredCostPLN	string	`json:"deliveredCostPln"`
	SentMessages		int	`json:"sentMessages"`
	SentParts		int	`json:"sentParts"`
	FailedMessages		int	`json:"failedMessages"`
	FailedParts		int	`json:"failedParts"`
	InFlightMessages	int	`json:"inFlightMessages"`
	InFlightParts		int	`json:"inFlightParts"`
}

// FormatPLN formats a milli-currency value (e.g. 80 -> "0.08", 24000 -> "24.00").
func FormatPLN(milli int64) string {
	sign := ""
	if milli < 0 {
		sign = "-"
		milli = -milli
	}
	whole := milli / 1000
	fraction := milli % 1000
	// Show two decimal places when last digit is zero, else three
	if fraction%10 == 0 {
		return fmt.Sprintf("%s%d.%02d", sign, whole, fraction/10)
	}
	return fmt.Sprintf("%s%d.%03d", sign, whole, fraction)
}

// CalculateUsageStats combines raw delivery counts with a unit price to produce complete statistics.
func CalculateUsageStats(counts RawUsageCounts, pricePerPartMilli int64) UsageStats {
	totalCost := int64(counts.TotalParts) * pricePerPartMilli
	deliveredCost := int64(counts.DeliveredParts) * pricePerPartMilli

	return UsageStats{
		PricePerPartMilli:	pricePerPartMilli,
		PricePerPartPLN:	FormatPLN(pricePerPartMilli),
		TotalMessages:		counts.TotalMessages,
		TotalParts:		counts.TotalParts,
		TotalCostMilli:		totalCost,
		TotalCostPLN:		FormatPLN(totalCost),
		DeliveredMessages:	counts.DeliveredMessages,
		DeliveredParts:		counts.DeliveredParts,
		DeliveredCostMilli:	deliveredCost,
		DeliveredCostPLN:	FormatPLN(deliveredCost),
		SentMessages:		counts.SentMessages,
		SentParts:		counts.SentParts,
		FailedMessages:		counts.FailedMessages,
		FailedParts:		counts.FailedParts,
		InFlightMessages:	counts.InFlightMessages,
		InFlightParts:		counts.InFlightParts,
	}
}

// UsageStore supplies aggregated SMS usage metrics.
type UsageStore interface {
	GetUsage(ctx context.Context, filter UsageFilter) (RawUsageCounts, error)
}

// PGUsageStore aggregates SMS usage metrics directly from the deliveries table in PostgreSQL.
type PGUsageStore struct {
	pool *pgxpool.Pool
}

// NewPGUsageStore creates a PGUsageStore.
func NewPGUsageStore(pool *pgxpool.Pool) *PGUsageStore {
	return &PGUsageStore{pool: pool}
}

// GetUsage executes an aggregation query on the deliveries table with optional batch and time range filters.
func (s *PGUsageStore) GetUsage(ctx context.Context, filter UsageFilter) (RawUsageCounts, error) {
	if s == nil || s.pool == nil {
		return RawUsageCounts{}, errors.New("database pool is not configured")
	}

	var conditions []string
	var args []any
	argIdx := 1

	conditions = append(conditions, "d.channel = 'sms'")

	var query string
	if filter.BatchID != nil && strings.TrimSpace(*filter.BatchID) != "" {
		query = `
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
		`
		conditions = append(conditions, fmt.Sprintf("br.batch_id = $%d", argIdx))
		args = append(args, strings.TrimSpace(*filter.BatchID))
		argIdx++
	} else {
		query = `
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
		`
	}

	if filter.From != nil {
		conditions = append(conditions, fmt.Sprintf("d.updated_at >= $%d", argIdx))
		args = append(args, *filter.From)
		argIdx++
	}
	if filter.To != nil {
		conditions = append(conditions, fmt.Sprintf("d.updated_at <= $%d", argIdx))
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
