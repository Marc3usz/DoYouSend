package recipients

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// ExistingContacts says which looked-up e-mails and phone numbers already
// belong to a stored recipient. Keys are normalized values (lower-case
// e-mail, E.164 phone); values are the owning recipient's ID.
type ExistingContacts struct {
	Emails map[string]string
	Phones map[string]string
}

// Store is the storage the Importer needs. It is defined here by its consumer;
// the Postgres implementation lands together with the storage layer.
type Store interface {
	// FindContacts reports which of emails (lower-case) and phones (E.164)
	// are already used by stored recipients.
	FindContacts(ctx context.Context, emails, phones []string) (ExistingContacts, error)
	// CreateRecipients stores all given recipients in one transaction:
	// either every one of them is created or none is.
	CreateRecipients(ctx context.Context, rs []Recipient) error
}

// ErrStorage marks an import that failed on the database rather than on the
// file, so the HTTP layer answers 500 and not a file error.
var ErrStorage = errors.New("recipient storage failed")

// Importer turns an uploaded CSV or XLSX file into recipients. Check is the
// dry run the administrator reviews; Import stores the rows Check accepts.
type Importer struct {
	store Store
}

// NewImporter returns an Importer backed by store.
func NewImporter(store Store) *Importer {
	return &Importer{store: store}
}

// Check parses the file and reports every row, including duplicates of
// recipients already stored, without writing anything. An error means the
// file as a whole is unusable (see the Err* values) or the lookup failed.
func (im *Importer) Check(ctx context.Context, r io.Reader) (ImportReport, error) {
	rows, err := parseFile(r)
	if err != nil {
		return ImportReport{}, err
	}
	return im.classifyStored(ctx, rows)
}

// Import runs Check and stores the rows it reports as valid. Invalid and
// duplicate rows are left out, not fatal: the report says which and why.
// Storing is all-or-nothing, so after an error nothing was imported and the
// same file can simply be uploaded again.
func (im *Importer) Import(ctx context.Context, r io.Reader) (ImportReport, error) {
	rows, err := parseFile(r)
	if err != nil {
		return ImportReport{}, err
	}

	// A contact stored by someone else between the lookup and the insert
	// makes the unique index refuse the whole batch. One more lookup turns
	// that row into a reported duplicate instead of failing the import.
	for attempt := 1; ; attempt++ {
		report, err := im.classifyStored(ctx, rows)
		if err != nil {
			return ImportReport{}, err
		}
		if len(report.Valid) == 0 {
			return report, nil
		}

		rs := make([]Recipient, len(report.Valid))
		for i, row := range report.Valid {
			rs[i] = row.Recipient
		}
		err = im.store.CreateRecipients(ctx, rs)
		if err == nil {
			return report, nil
		}
		if attempt == 1 && errors.Is(err, ErrDuplicateContact) {
			continue
		}
		return ImportReport{}, fmt.Errorf("%w: store %d imported recipients: %w", ErrStorage, len(rs), err)
	}
}

// classifyStored reports parsed rows, marking those whose contact a stored
// recipient already uses.
func (im *Importer) classifyStored(ctx context.Context, rows []parsedRow) (ImportReport, error) {
	// Each value is looked up once, however many rows repeat it.
	var emails, phones []string
	seen := make(map[string]bool)
	for _, p := range rows {
		if len(p.Errors) > 0 {
			continue
		}
		if email := normalizeEmail(p.Recipient.Email); email != "" && !seen["email:"+email] {
			seen["email:"+email] = true
			emails = append(emails, email)
		}
		if phone := p.Recipient.Phone; phone != "" && !seen["phone:"+phone] {
			seen["phone:"+phone] = true
			phones = append(phones, phone)
		}
	}

	var existing ExistingContacts
	if len(emails) > 0 || len(phones) > 0 {
		var err error
		existing, err = im.store.FindContacts(ctx, emails, phones)
		if err != nil {
			return ImportReport{}, fmt.Errorf("%w: look up existing contacts: %w", ErrStorage, err)
		}
	}
	return classify(rows, existing), nil
}
