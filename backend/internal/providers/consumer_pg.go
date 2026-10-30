package providers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGDeliveryReportConsumer updates delivery statuses in PostgreSQL.
// It enforces the forward-only lifecycle: once a delivery is marked as
// 'delivered' or 'failed', subsequent events cannot overwrite it.
// Transitions are strictly forward: pending -> sending -> sent -> delivered / failed.
//
// The forward-only lifecycle rule mirrors delivery.Advance in internal/delivery/status.go.
//
// If a delivery report arrives before the delivery has been assigned a provider_message_id
// (for example, while a large batch is still being dispatched), the report is buffered into
// the unmatched_delivery_reports table and matched automatically once provider_message_id
// is recorded (Issue #42, ADR-0011).
type PGDeliveryReportConsumer struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewPGDeliveryReportConsumer constructs a new consumer backed by PostgreSQL.
func NewPGDeliveryReportConsumer(pool *pgxpool.Pool, logger *slog.Logger) *PGDeliveryReportConsumer {
	if logger == nil {
		logger = slog.Default()
	}
	return &PGDeliveryReportConsumer{pool: pool, logger: logger}
}

// ConsumeDeliveryReports persists a batch of DeliveryReport updates to the deliveries table.
// If reports is empty, it returns nil immediately without acquiring a database connection.
// Status updates are applied forward-only and scoped to the exact channel:
// terminal statuses ('delivered', 'failed') are preserved even if an out-of-order event arrives.
//
// If a report does not match any row in deliveries by provider_message_id, it is buffered
// in unmatched_delivery_reports so it is not lost while batch dispatch is in progress.
// Furthermore, if a report updates a delivery to 'failed' on a finished batch, the parent
// batch status in message_batches is updated from 'done' to 'done_with_errors'.
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

	const insertUnmatchedQuery = `
		INSERT INTO unmatched_delivery_reports (provider_message_id, channel, status, error, created_at, updated_at)
		VALUES ($1, $2, $3::channel_status, $4, now(), now())
		ON CONFLICT (provider_message_id, channel) DO UPDATE
		SET status = EXCLUDED.status,
		    error = EXCLUDED.error,
		    updated_at = now()
		WHERE (unmatched_delivery_reports.status = 'pending' AND EXCLUDED.status IN ('sending', 'sent', 'delivered', 'failed'))
		   OR (unmatched_delivery_reports.status = 'sending' AND EXCLUDED.status IN ('sent', 'delivered', 'failed'))
		   OR (unmatched_delivery_reports.status = 'sent'    AND EXCLUDED.status IN ('delivered', 'failed'))
	`

	const resolveRaceQuery = `
		WITH matched AS (
			DELETE FROM unmatched_delivery_reports u
			USING deliveries d
			WHERE d.provider_message_id = u.provider_message_id
			  AND d.channel = u.channel
			  AND u.provider_message_id = $1
			  AND u.channel = $2
			RETURNING d.id AS delivery_id, u.status, u.error
		)
		UPDATE deliveries d
		SET status = m.status,
		    error = m.error,
		    updated_at = now()
		FROM matched m
		WHERE d.id = m.delivery_id
		  AND (
		    (d.status = 'pending' AND m.status IN ('sending', 'sent', 'delivered', 'failed')) OR
		    (d.status = 'sending' AND m.status IN ('sent', 'delivered', 'failed')) OR
		    (d.status = 'sent'    AND m.status IN ('delivered', 'failed'))
		  )
	`

	var lockMsgIDs, lockChannels []string
	seen := make(map[string]bool)
	for _, r := range reports {
		if r.ProviderMessageID == "" {
			continue
		}
		k := r.ProviderMessageID + ":" + string(r.Channel)
		if !seen[k] {
			seen[k] = true
			lockMsgIDs = append(lockMsgIDs, r.ProviderMessageID)
			lockChannels = append(lockChannels, string(r.Channel))
		}
	}

	if len(lockMsgIDs) > 0 {
		// Acquire transaction advisory locks ordered deterministically to prevent deadlocks
		// with concurrent delivery report consumers and dispatcher's SaveDelivery/SaveOutcome (Review #44, ADR-0011).
		_, err := tx.Exec(ctx, `
			SELECT pg_advisory_xact_lock(hashtext(m || ':' || c))
			FROM unnest($1::text[], $2::text[]) AS t(m, c)
			WHERE m IS NOT NULL AND m <> ''
			ORDER BY hashtext(m || ':' || c)`,
			lockMsgIDs, lockChannels)
		if err != nil {
			return fmt.Errorf("lock deliveries for reports: %w", err)
		}
	}

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

		tag, err := tx.Exec(ctx, updateQuery, string(r.Status), errText, r.ProviderMessageID, string(r.Channel))
		if err != nil {
			return fmt.Errorf("update delivery status for provider_message_id %s: %w", r.ProviderMessageID, err)
		}

		if tag.RowsAffected() == 0 {
			// Check if the delivery exists with this provider_message_id.
			var exists bool
			checkErr := tx.QueryRow(ctx, `SELECT true FROM deliveries WHERE provider_message_id = $1 AND channel = $2`,
				r.ProviderMessageID, string(r.Channel)).Scan(&exists)
			if errors.Is(checkErr, pgx.ErrNoRows) {
				// No delivery with this provider_message_id in database yet (e.g. batch is still dispatching).
				// Buffer report in unmatched_delivery_reports table.
				c.logger.Debug("delivery report buffered as unmatched",
					"provider_message_id", r.ProviderMessageID,
					"channel", r.Channel,
					"status", r.Status)

				if _, err := tx.Exec(ctx, insertUnmatchedQuery, r.ProviderMessageID, string(r.Channel), string(r.Status), errText); err != nil {
					return fmt.Errorf("insert unmatched report for %s: %w", r.ProviderMessageID, err)
				}

				// Resolve concurrent race if deliveries was committed right before this insert:
				if _, err := tx.Exec(ctx, resolveRaceQuery, r.ProviderMessageID, string(r.Channel)); err != nil {
					return fmt.Errorf("resolve race for unmatched report %s: %w", r.ProviderMessageID, err)
				}
			} else if checkErr != nil {
				return fmt.Errorf("check delivery existence for provider_message_id %s: %w", r.ProviderMessageID, checkErr)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delivery status updates: %w", err)
	}
	return nil
}

// PruneUnmatchedReports removes reports that have remained unmatched in the buffer
// longer than maxAge (defaults to 7 days if <= 0). It returns the number of deleted rows.
func (c *PGDeliveryReportConsumer) PruneUnmatchedReports(ctx context.Context, maxAge time.Duration) (int64, error) {
	if c == nil || c.pool == nil {
		return 0, errors.New("database pool is not configured")
	}
	if maxAge <= 0 {
		maxAge = 7 * 24 * time.Hour
	}
	tag, err := c.pool.Exec(ctx, `
		DELETE FROM unmatched_delivery_reports
		WHERE created_at < now() - ($1 * interval '1 second')
	`, int64(maxAge.Seconds()))
	if err != nil {
		return 0, fmt.Errorf("prune unmatched reports: %w", err)
	}
	return tag.RowsAffected(), nil
}

// StartCleaner runs PruneUnmatchedReports immediately on startup, and then periodically
// every interval until ctx is cancelled. If interval <= 0, 24 hours is used.
// If maxAge <= 0, 7 days is used.
func (c *PGDeliveryReportConsumer) StartCleaner(ctx context.Context, interval time.Duration, maxAge time.Duration) {
	if c == nil || c.pool == nil {
		return
	}
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	if maxAge <= 0 {
		maxAge = 7 * 24 * time.Hour
	}

	go func() {
		if deleted, err := c.PruneUnmatchedReports(ctx, maxAge); err != nil {
			c.logger.Warn("initial prune of unmatched delivery reports failed", "err", err)
		} else if deleted > 0 {
			c.logger.Info("pruned unmatched delivery reports on startup", "deleted", deleted)
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if deleted, err := c.PruneUnmatchedReports(ctx, maxAge); err != nil {
					c.logger.Warn("periodic prune of unmatched delivery reports failed", "err", err)
				} else if deleted > 0 {
					c.logger.Info("periodic prune of unmatched delivery reports", "deleted", deleted)
				}
			}
		}
	}()
}
