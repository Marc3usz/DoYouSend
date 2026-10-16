package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// PGStore is the Postgres Store: a batch is one message_batches row with a
// batch_recipients row per person and a deliveries row per channel.
type PGStore struct {
	pool *pgxpool.Pool
}

// NewPGStore returns a PGStore using pool.
func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

// CreateBatch implements Store. The whole plan is written in one transaction, so a
// batch is either recorded with every recipient and delivery or not at all.
func (s *PGStore) CreateBatch(ctx context.Context, b Batch) (Batch, error) {
	groupsJSON, err := json.Marshal(nonNil(b.Groups))
	if err != nil {
		return Batch{}, fmt.Errorf("create batch: encode groups: %w", err)
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// The sender confirms in the same request that creates the batch (POST /batches),
		// so created_by and confirmed_by are the same person.
		err := tx.QueryRow(ctx, `
			INSERT INTO message_batches (subject, body, status, created_by, confirmed_by,
				selected_groups, selected_recipient_ids, recipient_count, sms_part_count,
				estimated_cost, confirmed_at, idempotency_key, request_hash)
			VALUES ($1, $2, $3::text::batch_status, $4, $4, $5, $6::uuid[], $7, $8, $9::bigint / 1000.0, now(),
				nullif($10, '')::uuid, nullif($11, ''))
			RETURNING id::text, created_at, (SELECT full_name FROM users WHERE id = created_by)`,
			b.Subject, b.Body, string(b.Status), b.CreatedBy, groupsJSON, nonNil(b.RecipientIDs),
			len(b.Plan.Recipients), b.SMSParts, b.CostMilli, b.IdempotencyKey, b.RequestHash,
		).Scan(&b.ID, &b.CreatedAt, &b.CreatedByName)
		if constraint, ok := database.UniqueViolation(err); ok && constraint == "message_batches_idempotency_key" {
			return ErrDuplicateKey
		}
		if err != nil {
			return fmt.Errorf("insert batch: %w", err)
		}
		return insertPlan(ctx, tx, b.ID, b.Plan)
	})
	if err != nil {
		return Batch{}, fmt.Errorf("create batch: %w", err)
	}
	b.Plan = b.Plan.clone()
	b.Counts = b.Plan.Counts()
	b.Groups = append([]GroupRef(nil), b.Groups...)
	b.RecipientIDs = append([]string(nil), b.RecipientIDs...)
	return b, nil
}

// insertPlan writes the recipients and deliveries of a plan with one statement each,
// whatever the size of the school.
func insertPlan(ctx context.Context, tx pgx.Tx, batchID string, plan Plan) error {
	n := len(plan.Recipients)
	if n == 0 {
		return nil
	}
	ids, bodies := make([]string, n), make([]string, n)
	emails, phones := make([]string, n), make([]string, n)
	partial, positions := make([]bool, n), make([]int, n)
	for i, r := range plan.Recipients {
		ids[i], bodies[i], partial[i], positions[i] = r.RecipientID, r.Body, r.Partial, i
		for _, d := range r.Deliveries {
			switch d.Channel {
			case providers.ChannelEmail:
				emails[i] = d.To
			case providers.ChannelSMS:
				phones[i] = d.To
			}
		}
	}
	rows, err := tx.Query(ctx, `
		INSERT INTO batch_recipients (batch_id, recipient_id, rendered_body, email_snapshot,
			phone_snapshot, is_partial, position)
		SELECT $1, r.id, r.body, nullif(r.email, ''), nullif(r.phone, ''), r.partial, r.position
		FROM unnest($2::uuid[], $3::text[], $4::text[], $5::text[], $6::bool[], $7::int[])
			AS r(id, body, email, phone, partial, position)
		RETURNING id::text, recipient_id::text`,
		batchID, ids, bodies, emails, phones, partial, positions)
	if err != nil {
		return fmt.Errorf("insert recipients: %w", err)
	}
	rowIDs := make(map[string]string, n)
	for rows.Next() {
		var rowID, recipientID string
		if err := rows.Scan(&rowID, &recipientID); err != nil {
			rows.Close()
			return fmt.Errorf("scan recipient row: %w", err)
		}
		rowIDs[recipientID] = rowID
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("insert recipients: %w", err)
	}

	var cols deliveryColumns
	for _, r := range plan.Recipients {
		for _, d := range r.Deliveries {
			cols.add(rowIDs[strings.ToLower(r.RecipientID)], d)
		}
	}
	if len(cols.keys) == 0 {
		return nil
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO deliveries (batch_recipient_id, channel, status, parts, attempts,
			provider_message_id, error)
		SELECT d.row_id, d.channel::channel, d.status::channel_status, d.parts, d.attempts,
			nullif(d.message_id, ''), nullif(d.error, '')
		FROM unnest($1::uuid[], $2::text[], $3::text[], $4::int[], $5::int[], $6::text[], $7::text[])
			AS d(row_id, channel, status, parts, attempts, message_id, error)`,
		cols.keys, cols.channels, cols.statuses, cols.parts, cols.attempts, cols.messageIDs, cols.errors)
	if err != nil {
		return fmt.Errorf("insert deliveries: %w", err)
	}
	return nil
}

// deliveryColumns holds deliveries column by column, the shape unnest takes. keys is
// the batch_recipients row ID on insert and the recipient ID on update.
type deliveryColumns struct {
	keys, channels, statuses, messageIDs, errors []string
	parts, attempts                              []int
}

func (c *deliveryColumns) add(key string, d Delivery) {
	c.keys = append(c.keys, key)
	c.channels = append(c.channels, string(d.Channel))
	c.statuses = append(c.statuses, string(d.Status))
	c.parts = append(c.parts, d.Parts)
	c.attempts = append(c.attempts, d.Attempts)
	c.messageIDs = append(c.messageIDs, d.ProviderMessageID)
	c.errors = append(c.errors, d.Error)
}

// SaveOutcome implements Store. A delivery takes the outcome only when Advance allows
// it: a delivery report that arrived between dispatch and this save (e.g. delivered)
// is not overwritten by the older sent. The enum lists statuses in the order they
// move through, so "later" is a plain comparison; delivered and failed are final.
func (s *PGStore) SaveOutcome(ctx context.Context, batchID string, plan Plan) error {
	id := strings.ToLower(batchID)
	if !groups.IsValidID(id) {
		return fmt.Errorf("save outcome of batch %s: %w", batchID, ErrBatchNotFound)
	}
	var cols deliveryColumns
	for _, r := range plan.Recipients {
		for _, d := range r.Deliveries {
			cols.add(r.RecipientID, d)
		}
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Lock the batch so two saves derive its status from the same deliveries.
		var exists bool
		err := tx.QueryRow(ctx, `SELECT true FROM message_batches WHERE id = $1 FOR UPDATE`, id).Scan(&exists)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrBatchNotFound
		}
		if err != nil {
			return fmt.Errorf("lock batch: %w", err)
		}
		if len(cols.keys) > 0 {
			_, err = tx.Exec(ctx, `
				UPDATE deliveries d
				SET status = o.status::channel_status, attempts = o.attempts,
					provider_message_id = nullif(o.message_id, ''), error = nullif(o.error, ''),
					updated_at = now()
				FROM unnest($2::uuid[], $3::text[], $4::text[], $5::int[], $6::text[], $7::text[])
						AS o(recipient_id, channel, status, attempts, message_id, error),
					batch_recipients br
				WHERE br.batch_id = $1 AND br.recipient_id = o.recipient_id
					AND d.batch_recipient_id = br.id AND d.channel = o.channel::channel
					AND (d.status = o.status::channel_status
						OR (d.status NOT IN ('delivered', 'failed') AND d.status < o.status::channel_status))`,
				id, cols.keys, cols.channels, cols.statuses, cols.attempts, cols.messageIDs, cols.errors)
			if err != nil {
				return fmt.Errorf("update deliveries: %w", err)
			}
		}
		merged, err := loadPlan(ctx, tx, id)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE message_batches
			SET status = $2::text::batch_status,
				finished_at = CASE WHEN $2 <> 'running' THEN coalesce(finished_at, now()) ELSE finished_at END
			WHERE id = $1`,
			id, string(merged.Status()))
		if err != nil {
			return fmt.Errorf("update batch status: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("save outcome of batch %s: %w", batchID, err)
	}
	return nil
}

// Batch implements Store.
func (s *PGStore) Batch(ctx context.Context, id string) (Batch, error) {
	id = strings.ToLower(id)
	if !groups.IsValidID(id) {
		return Batch{}, fmt.Errorf("batch %s: %w", id, ErrBatchNotFound)
	}
	var (
		b          Batch
		groupsJSON []byte
		status     string
		finishedAt *time.Time
	)
	// One snapshot for the batch row and its plan, so a SaveOutcome committing in
	// between never shows a running batch whose deliveries have all finished.
	snapshot := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	err := pgx.BeginTxFunc(ctx, s.pool, snapshot, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT b.id::text, b.subject, b.body, b.created_by::text, u.full_name, b.selected_groups,
				b.selected_recipient_ids::text[], b.status::text, b.sms_part_count,
				(b.estimated_cost * 1000)::bigint, b.created_at, b.finished_at,
				coalesce(b.idempotency_key::text, ''), coalesce(b.request_hash, '')
			FROM message_batches b JOIN users u ON u.id = b.created_by
			WHERE b.id = $1`, id,
		).Scan(&b.ID, &b.Subject, &b.Body, &b.CreatedBy, &b.CreatedByName, &groupsJSON, &b.RecipientIDs,
			&status, &b.SMSParts, &b.CostMilli, &b.CreatedAt, &finishedAt, &b.IdempotencyKey, &b.RequestHash)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrBatchNotFound
		}
		if err != nil {
			return fmt.Errorf("query batch: %w", err)
		}
		b.Plan, err = loadPlan(ctx, tx, id)
		return err
	})
	if err != nil {
		return Batch{}, fmt.Errorf("batch %s: %w", id, err)
	}
	if err := json.Unmarshal(groupsJSON, &b.Groups); err != nil {
		return Batch{}, fmt.Errorf("batch %s: decode groups: %w", id, err)
	}
	b.Status = BatchStatus(status)
	b.Counts = b.Plan.Counts()
	if finishedAt != nil {
		b.FinishedAt = *finishedAt
	}
	return b, nil
}

// BatchByKey implements Store.
func (s *PGStore) BatchByKey(ctx context.Context, createdBy, key string) (Batch, error) {
	createdBy, key = strings.ToLower(createdBy), strings.ToLower(key)
	if !groups.IsValidID(createdBy) || !groups.IsValidID(key) {
		return Batch{}, fmt.Errorf("batch with key %s: %w", key, ErrBatchNotFound)
	}
	var id string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text FROM message_batches WHERE created_by = $1 AND idempotency_key = $2`,
		createdBy, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Batch{}, fmt.Errorf("batch with key %s: %w", key, ErrBatchNotFound)
	}
	if err != nil {
		return Batch{}, fmt.Errorf("batch with key %s: %w", key, err)
	}
	return s.Batch(ctx, id)
}

// ListBatches implements Store. The counts come from the recipient and delivery rows
// in the same query, so the list never loads whole plans.
func (s *PGStore) ListBatches(ctx context.Context, f BatchFilter) ([]Batch, int, error) {
	createdBy := strings.ToLower(f.CreatedBy)
	if createdBy != "" && !groups.IsValidID(createdBy) {
		return nil, 0, nil
	}
	const where = `WHERE ($1 = '' OR b.created_by::text = $1) AND ($2 = '' OR b.status::text = $2)`
	var total int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM message_batches b `+where,
		createdBy, string(f.Status)).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count batches: %w", err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT b.id::text, b.subject, b.created_by::text, u.full_name, b.selected_groups,
			b.status::text, b.sms_part_count, (b.estimated_cost * 1000)::bigint,
			b.created_at, b.finished_at, c.recipients, c.partial, c.failed
		FROM message_batches b
		JOIN users u ON u.id = b.created_by
		CROSS JOIN LATERAL (
			SELECT count(*) AS recipients,
				count(*) FILTER (WHERE br.is_partial) AS partial,
				count(*) FILTER (WHERE NOT EXISTS (
					SELECT 1 FROM deliveries d WHERE d.batch_recipient_id = br.id
				) OR EXISTS (
					SELECT 1 FROM deliveries d
					WHERE d.batch_recipient_id = br.id AND d.status = 'failed'
				)) AS failed
			FROM batch_recipients br WHERE br.batch_id = b.id
		) c
		`+where+`
		ORDER BY b.created_at DESC, b.id DESC
		LIMIT $3 OFFSET $4`,
		createdBy, string(f.Status), f.Limit, f.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list batches: %w", err)
	}
	defer rows.Close()
	var out []Batch
	for rows.Next() {
		var (
			b          Batch
			groupsJSON []byte
			status     string
			finishedAt *time.Time
		)
		if err := rows.Scan(&b.ID, &b.Subject, &b.CreatedBy, &b.CreatedByName, &groupsJSON,
			&status, &b.SMSParts, &b.CostMilli, &b.CreatedAt, &finishedAt,
			&b.Counts.Recipients, &b.Counts.Partial, &b.Counts.Failed); err != nil {
			return nil, 0, fmt.Errorf("scan batch: %w", err)
		}
		if err := json.Unmarshal(groupsJSON, &b.Groups); err != nil {
			return nil, 0, fmt.Errorf("batch %s: decode groups: %w", b.ID, err)
		}
		b.Status = BatchStatus(status)
		if finishedAt != nil {
			b.FinishedAt = *finishedAt
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read batches: %w", err)
	}
	return out, total, nil
}

// loadPlan reads the recipients of a batch in plan order, each with its deliveries
// e-mail before SMS (the channel enum order). A recipient without deliveries had no
// usable channel when the batch was planned.
func loadPlan(ctx context.Context, tx pgx.Tx, batchID string) (Plan, error) {
	plan := Plan{}
	err := tx.QueryRow(ctx, `SELECT subject FROM message_batches WHERE id = $1`, batchID).Scan(&plan.Subject)
	if err != nil {
		return Plan{}, fmt.Errorf("query plan subject: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT br.recipient_id::text, r.first_name, r.last_name, br.rendered_body, br.is_partial,
			coalesce(br.email_snapshot, ''), coalesce(br.phone_snapshot, ''),
			d.channel::text, d.status::text, d.parts, d.attempts,
			coalesce(d.provider_message_id, ''), coalesce(d.error, '')
		FROM batch_recipients br
		JOIN recipients r ON r.id = br.recipient_id
		LEFT JOIN deliveries d ON d.batch_recipient_id = br.id
		WHERE br.batch_id = $1
		ORDER BY br.position, br.recipient_id, d.channel`, batchID)
	if err != nil {
		return Plan{}, fmt.Errorf("query plan: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			r                      PlannedRecipient
			email, phone           string
			channel, status        *string
			parts, attempts        *int
			messageID, deliveryErr string
		)
		if err := rows.Scan(&r.RecipientID, &r.FirstName, &r.LastName, &r.Body, &r.Partial, &email, &phone,
			&channel, &status, &parts, &attempts, &messageID, &deliveryErr); err != nil {
			return Plan{}, fmt.Errorf("scan plan row: %w", err)
		}
		last := len(plan.Recipients) - 1
		if last < 0 || plan.Recipients[last].RecipientID != r.RecipientID {
			r.Unreachable = channel == nil
			plan.Recipients = append(plan.Recipients, r)
			last++
		}
		if channel == nil {
			continue
		}
		d := Delivery{
			Channel: providers.Channel(*channel), Status: Status(*status),
			Parts: *parts, Attempts: *attempts, ProviderMessageID: messageID, Error: deliveryErr,
		}
		if d.Channel == providers.ChannelSMS {
			d.To = phone
		} else {
			d.To = email
		}
		plan.Recipients[last].Deliveries = append(plan.Recipients[last].Deliveries, d)
	}
	if err := rows.Err(); err != nil {
		return Plan{}, fmt.Errorf("read plan: %w", err)
	}
	return plan, nil
}

// nonNil turns a nil slice into an empty one, so it is stored as [] / '{}', not null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
