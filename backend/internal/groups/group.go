package groups

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

// Limits for administrator input. They keep names readable in the group picker
// and bound what one request can store.
const (
	MaxNameLength        = 100 // runes
	MaxDescriptionLength = 500 // runes
)

// Errors returned by this package. Match them with errors.Is; the message may
// carry extra detail such as the offending ID.
var (
	ErrNotFound         = errors.New("group not found")
	ErrSystemGroup      = errors.New("built-in group cannot be changed")
	ErrNameTaken        = errors.New("group name is already taken")
	ErrInvalidInput     = errors.New("invalid input")
	ErrUnknownRecipient = errors.New("unknown recipient")
)

// Kind tells custom groups (edited by an administrator) from built-in ones
// (computed from recipient data, read-only).
type Kind string

const (
	KindCustom Kind = "custom"
	KindSystem Kind = "system"
)

// Group mirrors a row of the groups table. Rule is set only for built-in groups
// and says which recipients belong to them; custom groups list their members in
// group_members instead.
type Group struct {
	ID          string
	Name        string
	Description string
	Kind        Kind
	Rule        *SystemRule
	CreatedAt   time.Time
}

// IsSystem reports whether g is a built-in, read-only group.
func (g Group) IsSystem() bool {
	return g.Kind == KindSystem
}

// GroupInput is what an administrator submits when creating or editing a
// custom group.
type GroupInput struct {
	Name        string
	Description string
}

// Normalize trims both fields and collapses runs of whitespace in the name, so
// "Rada  rodzicow " and "Rada rodzicow" are recognised as the same name.
func (in GroupInput) Normalize() GroupInput {
	return GroupInput{
		Name:        strings.Join(strings.Fields(in.Name), " "),
		Description: strings.TrimSpace(in.Description),
	}
}

// Validate checks normalized input and returns every violation at once, like
// recipients.Recipient.Validate, so a form can mark all bad fields together.
func (in GroupInput) Validate() []recipients.FieldError {
	var errs []recipients.FieldError

	switch n := utf8.RuneCountInString(in.Name); {
	case n == 0:
		errs = append(errs, recipients.FieldError{Field: "name", Message: "name is required"})
	case n > MaxNameLength:
		errs = append(errs, recipients.FieldError{Field: "name", Message: "name is longer than 100 characters"})
	}
	if hasControlChars(in.Name) {
		errs = append(errs, recipients.FieldError{Field: "name", Message: "name contains control characters"})
	}

	if utf8.RuneCountInString(in.Description) > MaxDescriptionLength {
		errs = append(errs, recipients.FieldError{Field: "description", Message: "description is longer than 500 characters"})
	}
	if hasControlChars(strings.NewReplacer("\n", "", "\r", "", "\t", "").Replace(in.Description)) {
		errs = append(errs, recipients.FieldError{Field: "description", Message: "description contains control characters"})
	}

	return errs
}

func hasControlChars(s string) bool {
	return strings.ContainsFunc(s, unicode.IsControl)
}

// ValidationError carries every field problem of one rejected input. It matches
// ErrInvalidInput with errors.Is, so callers can map it to 400 without a type
// switch and still show Fields when they want the detail.
type ValidationError struct {
	Fields []recipients.FieldError
}

func (e *ValidationError) Error() string {
	msgs := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		msgs[i] = f.Error()
	}
	return "invalid input: " + strings.Join(msgs, "; ")
}

func (e *ValidationError) Is(target error) bool {
	return target == ErrInvalidInput
}

// UnknownRecipientsError lists recipient IDs that do not exist. It matches
// ErrUnknownRecipient with errors.Is.
type UnknownRecipientsError struct {
	IDs []string
}

func (e *UnknownRecipientsError) Error() string {
	return "unknown recipient: " + strings.Join(e.IDs, ", ")
}

func (e *UnknownRecipientsError) Is(target error) bool {
	return target == ErrUnknownRecipient
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsValidID reports whether id is a UUID in its canonical textual form, the
// only form the groups and recipients tables use.
func IsValidID(id string) bool {
	return uuidPattern.MatchString(id)
}

// canonicalID lower-cases a valid UUID so the same ID typed in upper case by a
// client does not count as a different recipient during deduplication.
func canonicalID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

// nameKey is the form group names are compared in: names differing only in
// letter case or spacing are the same name.
func nameKey(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}
