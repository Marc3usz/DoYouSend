package recipients

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database"
)

// ErrDuplicateContact is returned when an e-mail address or phone number is
// already used by another recipient (the unique indexes of 0001_init.sql).
var ErrDuplicateContact = errors.New("contact is already used by another recipient")

// DuplicateContactError says which contact collided. It matches
// ErrDuplicateContact with errors.Is.
type DuplicateContactError struct {
	// Field is "email" or "phone".
	Field string
	// ExistingID is the recipient already using the contact, when known. A
	// collision caught only by the unique index (a concurrent write) has none.
	ExistingID string
}

func (e *DuplicateContactError) Error() string {
	return e.Field + ": " + ErrDuplicateContact.Error()
}

func (e *DuplicateContactError) Is(target error) bool {
	return target == ErrDuplicateContact
}

// PGStore keeps recipients in Postgres. It implements Store (the importer's
// storage) and the read-only directory package groups needs.
type PGStore struct {
	pool *pgxpool.Pool
}

// NewPGStore returns a PGStore using pool.
func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

// recipientColumns lists the columns scanRecipient reads, in its order.
// Missing contacts are NULL in the table and "" in Recipient. The classes come
// from recipient_classes; in a RETURNING clause they are the ones stored before
// the statement, so writers set Recipient.Classes themselves.
const recipientColumns = `id::text, first_name, last_name, coalesce(email, ''), coalesce(phone, ''), type::text,
	coalesce((SELECT array_agg(c.class_name) FROM recipient_classes c WHERE c.recipient_id = recipients.id), '{}'),
	created_at, updated_at`

// FindContacts implements Store.
func (s *PGStore) FindContacts(ctx context.Context, emails, phones []string) (ExistingContacts, error) {
	out := ExistingContacts{Emails: make(map[string]string), Phones: make(map[string]string)}
	if len(emails) == 0 && len(phones) == 0 {
		return out, nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id::text, lower(email), phone FROM recipients
		WHERE lower(email) = ANY($1::text[]) OR phone = ANY($2::text[])`,
		nonNil(emails), nonNil(phones))
	if err != nil {
		return ExistingContacts{}, fmt.Errorf("query contacts: %w", err)
	}
	defer rows.Close()

	wantEmail := toSet(emails)
	wantPhone := toSet(phones)
	for rows.Next() {
		var id string
		var email, phone *string
		if err := rows.Scan(&id, &email, &phone); err != nil {
			return ExistingContacts{}, fmt.Errorf("scan contact: %w", err)
		}
		// A row matched on one value may also carry the other; report only
		// what was asked for.
		if email != nil && wantEmail[*email] {
			out.Emails[*email] = id
		}
		if phone != nil && wantPhone[*phone] {
			out.Phones[*phone] = id
		}
	}
	if err := rows.Err(); err != nil {
		return ExistingContacts{}, fmt.Errorf("read contacts: %w", err)
	}
	return out, nil
}

// CreateRecipients implements Store. All rows go in one transaction; a unique
// violation (a contact taken meanwhile) fails with *DuplicateContactError.
func (s *PGStore) CreateRecipients(ctx context.Context, rs []Recipient) error {
	if len(rs) == 0 {
		return nil
	}
	first := make([]string, len(rs))
	last := make([]string, len(rs))
	emails := make([]*string, len(rs))
	phones := make([]*string, len(rs))
	types := make([]string, len(rs))
	classes := make([]string, len(rs))
	for i, r := range rs {
		first[i], last[i], types[i] = r.FirstName, r.LastName, string(r.Type)
		emails[i], phones[i] = nullable(r.Email), nullable(r.Phone)
		classes[i] = strings.Join(r.Classes, ",")
	}
	// One statement is one transaction: all rows and their classes are stored
	// or none is. Inserted rows are matched back to their classes by e-mail and
	// phone, a pair no two imported rows share (the unique indexes would
	// refuse them anyway).
	_, err := s.pool.Exec(ctx, `
		WITH input AS (
			SELECT * FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::recipient_type[], $6::text[])
				AS t(first_name, last_name, email, phone, type, classes)
		), inserted AS (
			INSERT INTO recipients (first_name, last_name, email, phone, type)
			SELECT first_name, last_name, email, phone, type FROM input
			RETURNING id, email, phone
		)
		INSERT INTO recipient_classes (recipient_id, class_name)
		SELECT inserted.id, class_name
		FROM inserted
		JOIN input ON input.email IS NOT DISTINCT FROM inserted.email AND input.phone IS NOT DISTINCT FROM inserted.phone
		CROSS JOIN LATERAL unnest(string_to_array(nullif(input.classes, ''), ',')) AS class_name`,
		first, last, emails, phones, types, classes)
	if err != nil {
		return fmt.Errorf("insert %d recipients: %w", len(rs), contactError(err))
	}
	return nil
}

// RecipientsByIDs returns the recipients among ids. Unknown and malformed IDs
// are left out, not reported as an error.
func (s *PGStore) RecipientsByIDs(ctx context.Context, ids []string) ([]Recipient, error) {
	ids = validIDs(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	return s.query(ctx, `SELECT `+recipientColumns+` FROM recipients WHERE id = ANY($1::uuid[])`, ids)
}

// RecipientsByType returns every recipient of type t.
func (s *PGStore) RecipientsByType(ctx context.Context, t Type) ([]Recipient, error) {
	return s.query(ctx, `SELECT `+recipientColumns+` FROM recipients WHERE type = $1::recipient_type ORDER BY lower(last_name), lower(first_name), id`, string(t))
}

// Classes returns every class some recipient is assigned to, in
// CompareClasses order. Each one has its class groups (ADR-0009).
func (s *PGStore) Classes(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT class_name FROM recipient_classes`)
	if err != nil {
		return nil, fmt.Errorf("query classes: %w", err)
	}
	classes, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("read classes: %w", err)
	}
	return NormalizeClasses(classes), nil
}

// ListRecipients implements RecipientStore. Query is matched against the
// first and last name (in both orders), the e-mail and the phone number, the
// latter also with spaces and dashes removed ("500 100" finds +48500100101).
func (s *PGStore) ListRecipients(ctx context.Context, f Filter) ([]Recipient, error) {
	pattern, phonePattern := "", ""
	if q := strings.TrimSpace(f.Query); q != "" {
		pattern = "%" + escapeLike(q) + "%"
		// A query of only dashes and spaces leaves nothing to look for in
		// phone numbers; "%%" would match every one of them.
		if digits := strings.NewReplacer(" ", "", "-", "").Replace(q); digits != "" {
			phonePattern = "%" + escapeLike(digits) + "%"
		}
	}
	return s.query(ctx, `SELECT `+recipientColumns+` FROM recipients
		WHERE ($1 = '' OR type::text = $1)
		  AND ($2 = '' OR first_name ILIKE $2 OR last_name ILIKE $2
		       OR first_name || ' ' || last_name ILIKE $2 OR last_name || ' ' || first_name ILIKE $2
		       OR email ILIKE $2 OR ($3 <> '' AND phone LIKE $3))
		  AND ($4 = '' OR EXISTS (SELECT 1 FROM recipient_classes c WHERE c.recipient_id = recipients.id AND c.class_name = $4))
		ORDER BY lower(last_name), lower(first_name), id`,
		string(f.Type), pattern, phonePattern, f.Class)
}

// GetRecipient implements RecipientStore.
func (s *PGStore) GetRecipient(ctx context.Context, id string) (Recipient, error) {
	rs, err := s.RecipientsByIDs(ctx, []string{id})
	if err != nil {
		return Recipient{}, err
	}
	if len(rs) == 0 {
		return Recipient{}, ErrNotFound
	}
	return rs[0], nil
}

// RecipientGroupIDs implements RecipientStore. Built-in groups are left out:
// their membership follows from the recipient type (ADR-0007).
func (s *PGStore) RecipientGroupIDs(ctx context.Context, id string) ([]string, error) {
	ids := validIDs([]string{id})
	if len(ids) == 0 {
		return []string{}, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT g.id::text FROM group_members m JOIN groups g ON g.id = m.group_id
		WHERE m.recipient_id = $1 AND NOT g.is_system
		ORDER BY lower(g.name), g.id`, ids[0])
	if err != nil {
		return nil, fmt.Errorf("query groups of recipient: %w", err)
	}
	groupIDs, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("read groups of recipient: %w", err)
	}
	return groupIDs, nil
}

// CreateRecipient implements RecipientStore. The recipient and their classes
// are stored in one transaction.
func (s *PGStore) CreateRecipient(ctx context.Context, r Recipient) (Recipient, error) {
	var created Recipient
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			INSERT INTO recipients (first_name, last_name, email, phone, type)
			VALUES ($1, $2, $3, $4, $5::recipient_type)
			RETURNING `+recipientColumns,
			r.FirstName, r.LastName, nullable(r.Email), nullable(r.Phone), string(r.Type))
		if err != nil {
			return err
		}
		if created, err = pgx.CollectExactlyOneRow(rows, scanRecipient); err != nil {
			return contactError(err)
		}
		created.Classes, err = replaceClasses(ctx, tx, created.ID, r.Classes)
		return err
	})
	if err != nil {
		return Recipient{}, fmt.Errorf("insert recipient: %w", err)
	}
	return created, nil
}

// UpdateRecipient implements RecipientStore.
func (s *PGStore) UpdateRecipient(ctx context.Context, r Recipient) (Recipient, error) {
	ids := validIDs([]string{r.ID})
	if len(ids) == 0 {
		return Recipient{}, ErrNotFound
	}
	var updated Recipient
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			UPDATE recipients
			SET first_name = $2, last_name = $3, email = $4, phone = $5, type = $6::recipient_type, updated_at = now()
			WHERE id = $1
			RETURNING `+recipientColumns,
			ids[0], r.FirstName, r.LastName, nullable(r.Email), nullable(r.Phone), string(r.Type))
		if err != nil {
			return err
		}
		if updated, err = pgx.CollectExactlyOneRow(rows, scanRecipient); err != nil {
			return contactError(err)
		}
		updated.Classes, err = replaceClasses(ctx, tx, updated.ID, r.Classes)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Recipient{}, ErrNotFound
	}
	if err != nil {
		return Recipient{}, fmt.Errorf("update recipient: %w", err)
	}
	return updated, nil
}

// replaceClasses makes classes the only classes of recipient id and returns
// them in the order recipients are read with.
func replaceClasses(ctx context.Context, tx pgx.Tx, id string, classes []string) ([]string, error) {
	if _, err := tx.Exec(ctx, `DELETE FROM recipient_classes WHERE recipient_id = $1`, id); err != nil {
		return nil, fmt.Errorf("clear classes: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO recipient_classes (recipient_id, class_name)
		SELECT $1, unnest($2::text[])`, id, nonNil(classes)); err != nil {
		return nil, fmt.Errorf("store classes: %w", err)
	}
	return NormalizeClasses(classes), nil
}

// DeleteRecipient implements RecipientStore. Group memberships go with the
// recipient (ON DELETE CASCADE); a batch_recipients row keeps them.
func (s *PGStore) DeleteRecipient(ctx context.Context, id string) error {
	ids := validIDs([]string{id})
	if len(ids) == 0 {
		return ErrNotFound
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM recipients WHERE id = $1`, ids[0])
	if _, fk := database.ForeignKeyViolation(err); fk {
		return ErrRecipientInUse
	}
	if err != nil {
		return fmt.Errorf("delete recipient: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// escapeLike makes s match literally inside a LIKE pattern (backslash is the
// default escape character).
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
}

func (s *PGStore) query(ctx context.Context, sql string, args ...any) ([]Recipient, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query recipients: %w", err)
	}
	rs, err := pgx.CollectRows(rows, scanRecipient)
	if err != nil {
		return nil, fmt.Errorf("read recipients: %w", err)
	}
	return rs, nil
}

func scanRecipient(row pgx.CollectableRow) (Recipient, error) {
	var r Recipient
	var typ string
	err := row.Scan(&r.ID, &r.FirstName, &r.LastName, &r.Email, &r.Phone, &typ, &r.Classes, &r.CreatedAt, &r.UpdatedAt)
	r.Type = Type(typ)
	r.Classes = NormalizeClasses(r.Classes)
	return r, err
}

// contactError turns a violation of the recipients unique indexes into
// *DuplicateContactError and leaves any other error as it is.
func contactError(err error) error {
	constraint, ok := database.UniqueViolation(err)
	if !ok {
		return err
	}
	switch constraint {
	case "recipients_email_key":
		return &DuplicateContactError{Field: "email"}
	case "recipients_phone_key":
		return &DuplicateContactError{Field: "phone"}
	default:
		return err
	}
}

var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// validIDs lower-cases ids and drops those that are not UUIDs, which Postgres
// would otherwise reject for the whole query.
func validIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id = strings.ToLower(strings.TrimSpace(id)); idPattern.MatchString(id) {
			out = append(out, id)
		}
	}
	return out
}

func toSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}

// nonNil keeps a nil slice from being sent as SQL NULL, for which = ANY(...)
// is NULL rather than false.
func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

var (
	_ Store          = (*PGStore)(nil)
	_ RecipientStore = (*PGStore)(nil)
)
