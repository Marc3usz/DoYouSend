package providers

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGDeliveryReportConsumer updates delivery statuses in PostgreSQL.
// It enforces the forward-only lifecycle: once a delivery is marked as
// 'delivered' or 'failed', subsequent events cannot overwrite it.
// Transitions are strictly forward: pending -> sending -> sent -> delivered / failed.
//
// The forward-only lifecycle rule mirrors delivery.Advance in internal/delivery/store.go.
type PGDeliveryReportConsumer struct {
	pool *pgxpool.Pool
}

// NewPGDeliveryReportConsumer constructs a new consumer backed by PostgreSQL.
func NewPGDeliveryReportConsumer(pool *pgxpool.Pool) *PGDeliveryReportConsumer {
	return &PGDeliveryReportConsumer{pool: pool}
}

// ConsumeDeliveryReports persists a batch of DeliveryReport updates to the deliveries table.
// If reports is empty, it returns nil immediately without acquiring a database connection.
// Status updates are applied forward-only and scoped to the exact channel:
// terminal statuses ('delivered', 'failed') are preserved even if an out-of-order event arrives.
func (c *PGDeliveryReportConsumer) ConsumeDeliveryReports(ctx context.Context, reports []DeliveryReport) error {
	if len(reports) == 0 {
		return nil
	}
	if c == nil || c.pool == nil {
		return errors.New("database pool is not configured")
	}

	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Cast $1 to channel_status enum to prevent Postgres from inferring $1 as text due to string literals in IN.
	const updateQuery = `
		UPDATE deliveries
		SET status = $1::channel_status, error = $2, updated_at = now()
		WHERE provider_message_id = $3
		  AND channel = $4
		  AND (
		    (status = 'pending' AND $1::channel_status IN ('sending', 'sent', 'delivered', 'failed')) OR
		    (status = 'sending' AND $1::channel_status IN ('sent', 'delivered', 'failed')) OR
		    (status = 'sent'    AND $1::channel_status IN ('delivered', 'failed'))
		  )
	`

	for _, r := range reports {
		if r.ProviderMessageID == "" {
			continue
		}

		var errText *string
		if r.Status == StatusFailed {
			if r.ErrorMessage != "" {
				errText = &r.ErrorMessage
			} else {
				fallback := "delivery failed"
				errText = &fallback
			}
		}

		if _, err := tx.Exec(ctx, updateQuery, string(r.Status), errText, r.ProviderMessageID, string(r.Channel)); err != nil {
			return fmt.Errorf("update delivery status for provider_message_id %s: %w", r.ProviderMessageID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delivery status updates: %w", err)
	}
	return nil
}
