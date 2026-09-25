package recipients

import (
	"strings"
	"time"
)

// Type distinguishes who a recipient is in the school context.
type Type string

const (
	TypeParent  Type = "parent"
	TypeStudent Type = "student"
)

// Recipient mirrors a row of the recipients table.
type Recipient struct {
	ID        string
	FirstName string
	LastName  string
	Email     string
	Phone     string
	Type      Type
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Validate checks a Recipient against the data-quality rules from description.md:
// a valid e-mail and/or phone number, and at least one contact channel present.
// It returns all violations at once (not just the first) so a CSV import report
// can show a complete picture per row.
func (r Recipient) Validate() []FieldError {
	var errs []FieldError

	if strings.TrimSpace(r.FirstName) == "" {
		errs = append(errs, FieldError{Field: "first_name", Message: "first name is required"})
	}
	if strings.TrimSpace(r.LastName) == "" {
		errs = append(errs, FieldError{Field: "last_name", Message: "last name is required"})
	}
	if r.Type != TypeParent && r.Type != TypeStudent {
		errs = append(errs, FieldError{Field: "type", Message: "type must be 'parent' or 'student'"})
	}

	if r.Email != "" {
		if err := ValidateEmail(r.Email); err != nil {
			errs = append(errs, FieldError{Field: "email", Message: err.Error()})
		}
	}
	// Validate the normalized form: callers may still hold loose CSV/UI input
	// ("500 100 101", "+48 500 100 101") at this point, since normalization
	// only needs to happen once, right before a value is persisted.
	if r.Phone != "" {
		if err := ValidatePhone(NormalizePhone(r.Phone)); err != nil {
			errs = append(errs, FieldError{Field: "phone", Message: err.Error()})
		}
	}
	if r.Email == "" && r.Phone == "" {
		errs = append(errs, FieldError{Field: "contact", Message: "recipient must have an e-mail or a phone number"})
	}

	return errs
}

// FieldError attaches a validation message to the offending field, so callers
// (CSV import, HTTP handlers) can report exactly what is wrong with which row.
type FieldError struct {
	Field   string
	Message string
}

func (e FieldError) Error() string {
	return e.Field + ": " + e.Message
}
