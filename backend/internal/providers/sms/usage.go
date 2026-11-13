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
	BilledParts       int
	DeliveredMessages int
	DeliveredParts    int
	SentMessages      int
	SentParts         int
	FailedMessages    int
	FailedParts       int
	InFlightMessages  int
	InFlightParts     int
	// SimulatedMessages and SimulatedParts were "sent" under DRY_RUN, by the fake
	// provider or in SMSAPI test mode: nothing left the system and nothing is billed.
	SimulatedMessages int
	SimulatedParts    int
	// CostsKnown means the store priced every part at the price recorded on its
	// batch (the three costs below), so a later price change does not rewrite the
	// past. Otherwise CalculateUsageStats prices all parts at the current price.
	CostsKnown         bool
	PlannedCostMilli   int64
	BilledCostMilli    int64
	DeliveredCostMilli int64
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
	SimulatedMessages  int   `json:"simulatedMessages"`
	SimulatedParts     int   `json:"simulatedParts"`
}

// CalculateUsageStats combines raw delivery counts with a unit price to produce complete statistics.
// Planned cost reflects all planned SMS parts.
// Billed cost reflects parts charged by the provider (delivered + sent + failed with provider_message_id).
func CalculateUsageStats(counts RawUsageCounts, pricePerPartMilli int64) UsageStats {
	plannedCost := int64(counts.TotalParts) * pricePerPartMilli
	billedParts := counts.BilledParts
	if billedParts == 0 && (counts.DeliveredParts > 0 || counts.SentParts > 0) {
		billedParts = counts.DeliveredParts + counts.SentParts
	}
	billedCost := int64(billedParts) * pricePerPartMilli
	deliveredCost := int64(counts.DeliveredParts) * pricePerPartMilli
	if counts.CostsKnown {
		plannedCost, billedCost, deliveredCost = counts.PlannedCostMilli, counts.BilledCostMilli, counts.DeliveredCostMilli
	}

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
		SimulatedMessages:  counts.SimulatedMessages,
		SimulatedParts:     counts.SimulatedParts,
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

	// u.billed: the operator charges for the parts (accepted, even if later failed),
	// and the message really left the system (not DRY_RUN, fake or SMSAPI test mode).
	// u.price: the price of one part recorded on the batch, in thousandths of PLN.
	query := `
		SELECT
			COUNT(*)::int AS total_messages,
			COALESCE(SUM(d.parts), 0)::int AS total_parts,
			COALESCE(SUM(d.parts) FILTER (WHERE u.billed), 0)::int AS billed_parts,
			COUNT(*) FILTER (WHERE d.status = 'delivered')::int AS delivered_messages,
			COALESCE(SUM(d.parts) FILTER (WHERE d.status = 'delivered'), 0)::int AS delivered_parts,
			COUNT(*) FILTER (WHERE d.status = 'sent')::int AS sent_messages,
			COALESCE(SUM(d.parts) FILTER (WHERE d.status = 'sent'), 0)::int AS sent_parts,
			COUNT(*) FILTER (WHERE d.status = 'failed')::int AS failed_messages,
			COALESCE(SUM(d.parts) FILTER (WHERE d.status = 'failed'), 0)::int AS failed_parts,
			COUNT(*) FILTER (WHERE d.status IN ('pending', 'sending'))::int AS in_flight_messages,
			COALESCE(SUM(d.parts) FILTER (WHERE d.status IN ('pending', 'sending')), 0)::int AS in_flight_parts,
			COUNT(*) FILTER (WHERE u.simulated)::int AS simulated_messages,
			COALESCE(SUM(d.parts) FILTER (WHERE u.simulated), 0)::int AS simulated_parts,
			round(COALESCE(SUM(d.parts * u.price), 0))::bigint AS planned_cost,
			round(COALESCE(SUM(d.parts * u.price) FILTER (WHERE u.billed), 0))::bigint AS billed_cost,
			round(COALESCE(SUM(d.parts * u.price) FILTER (WHERE d.status = 'delivered' AND NOT u.simulated), 0))::bigint AS delivered_cost
		FROM deliveries d
		JOIN batch_recipients br ON br.id = d.batch_recipient_id
		JOIN message_batches mb ON mb.id = br.batch_id
		CROSS JOIN LATERAL (
			SELECT
				CASE WHEN mb.sms_part_count > 0 THEN mb.estimated_cost * 1000 / mb.sms_part_count ELSE 0 END AS price,
				coalesce(d.provider_message_id LIKE 'dryrun-%' OR d.provider_message_id LIKE 'fake-%'
					OR d.provider_message_id LIKE 'smsapi-test-%', false) AS simulated
		) p
		CROSS JOIN LATERAL (
			SELECT p.price, p.simulated,
				NOT p.simulated AND (d.status IN ('sent', 'delivered')
					OR (d.status = 'failed' AND d.provider_message_id IS NOT NULL)) AS billed
		) u
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
	}

	query += " WHERE " + strings.Join(conditions, " AND ")

	var counts RawUsageCounts
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&counts.TotalMessages,
		&counts.TotalParts,
		&counts.BilledParts,
		&counts.DeliveredMessages,
		&counts.DeliveredParts,
		&counts.SentMessages,
		&counts.SentParts,
		&counts.FailedMessages,
		&counts.FailedParts,
		&counts.InFlightMessages,
		&counts.InFlightParts,
		&counts.SimulatedMessages,
		&counts.SimulatedParts,
		&counts.PlannedCostMilli,
		&counts.BilledCostMilli,
		&counts.DeliveredCostMilli,
	)
	if err != nil {
		return RawUsageCounts{}, fmt.Errorf("query sms usage stats: %w", err)
	}
	counts.CostsKnown = true

	return counts, nil
}
