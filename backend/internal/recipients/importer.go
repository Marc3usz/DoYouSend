package recipients

import (
	"context"
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

	var emails, phones []string
	for _, p := range rows {
		if len(p.Errors) > 0 {
			continue
		}
		if email := normalizeEmail(p.Recipient.Email); email != "" {
			emails = append(emails, email)
		}
		if p.Recipient.Phone != "" {
			phones = append(phones, p.Recipient.Phone)
		}
	}

	var existing ExistingContacts
	if len(emails) > 0 || len(phones) > 0 {
		existing, err = im.store.FindContacts(ctx, emails, phones)
		if err != nil {
			return ImportReport{}, fmt.Errorf("look up existing contacts: %w", err)
		}
	}
	return classify(rows, existing), nil
}

// Import runs Check and stores the rows it reports as valid. Invalid and
// duplicate rows are left out, not fatal: the report says which and why.
// Storing is all-or-nothing, so after an error nothing was imported and the
// same file can simply be uploaded again.
func (im *Importer) Import(ctx context.Context, r io.Reader) (ImportReport, error) {
	report, err := im.Check(ctx, r)
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
	if err := im.store.CreateRecipients(ctx, rs); err != nil {
		return ImportReport{}, fmt.Errorf("store %d imported recipients: %w", len(rs), err)
	}
	return report, nil
}
