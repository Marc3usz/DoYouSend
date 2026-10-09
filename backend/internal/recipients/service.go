package recipients

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Errors of the recipient service. Match them with errors.Is.
var (
	ErrNotFound       = errors.New("recipient not found")
	ErrRecipientInUse = errors.New("recipient appears in the batch history")
	ErrInvalidInput   = errors.New("invalid input")
	ErrInvalidParams  = errors.New("invalid request parameters")
)

// ValidationError carries every field problem of one rejected request. It
// matches ErrInvalidInput with errors.Is.
type ValidationError struct {
	Fields []FieldError
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

// ParamError carries the problems of a request's query or path parameters
// (filters, paging, the recipient ID), as opposed to the submitted fields of
// a ValidationError: the contract answers them with invalid_request. It
// matches ErrInvalidParams with errors.Is.
type ParamError struct {
	Fields []FieldError
}

func (e *ParamError) Error() string {
	msgs := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		msgs[i] = f.Error()
	}
	return "invalid request parameters: " + strings.Join(msgs, "; ")
}

func (e *ParamError) Is(target error) bool {
	return target == ErrInvalidParams
}

// List limits (openapi.yaml GET /recipients).
const (
	DefaultPageSize = 50
	MaxPageSize     = 200
	MaxQueryLength  = 100 // runes
)

// Filter narrows the recipients the store returns.
type Filter struct {
	// Query matches a fragment of the name, e-mail or phone, ignoring case.
	Query string
	// Type, when set, keeps only recipients of this type.
	Type Type
	// Class, when set, keeps only recipients assigned to this normalized class.
	Class string
}

// ListQuery is a page request of GET /recipients.
type ListQuery struct {
	Filter
	// Issue, when set, keeps only recipients who cannot be reached on it.
	Issue  Channel
	Limit  int
	Offset int
}

// Page is one page of recipients plus the number of all matches.
type Page struct {
	Items []Recipient
	Total int
}

// Details is a recipient with the custom groups they belong to.
type Details struct {
	Recipient
	GroupIDs []string
}

// Input is what an administrator submits when adding or editing a recipient.
type Input struct {
	FirstName string
	LastName  string
	Email     string
	Phone     string
	Type      Type
	// Classes replace the recipient's classes; nil keeps them unchanged on
	// Update and means no class on Create.
	Classes *[]string
}

// normalize applies the same clean-up as a file import (recordToRecipient):
// trimmed fields, a phone number in E.164 where it can be derived and
// normalized class names.
func (in Input) normalize() Recipient {
	r := Recipient{
		FirstName: strings.TrimSpace(in.FirstName),
		LastName:  strings.TrimSpace(in.LastName),
		Email:     strings.TrimSpace(in.Email),
		Phone:     strings.TrimSpace(in.Phone),
		Type:      Type(strings.TrimSpace(string(in.Type))),
	}
	if r.Phone != "" {
		r.Phone = NormalizePhone(r.Phone)
	}
	if in.Classes != nil {
		r.Classes = NormalizeClasses(*in.Classes)
	}
	return r
}

// RecipientStore is the storage the Service needs; PGStore implements it.
type RecipientStore interface {
	// ListRecipients returns every recipient matching f, sorted by last name,
	// first name and ID.
	ListRecipients(ctx context.Context, f Filter) ([]Recipient, error)
	// GetRecipient returns one recipient or fails with ErrNotFound.
	GetRecipient(ctx context.Context, id string) (Recipient, error)
	// RecipientGroupIDs returns the custom groups of a recipient, by name.
	RecipientGroupIDs(ctx context.Context, id string) ([]string, error)
	// FindContacts is Store.FindContacts.
	FindContacts(ctx context.Context, emails, phones []string) (ExistingContacts, error)
	// CreateRecipient stores r and returns it with ID and timestamps set. A
	// taken contact fails with *DuplicateContactError.
	CreateRecipient(ctx context.Context, r Recipient) (Recipient, error)
	// UpdateRecipient replaces the data of r.ID and returns the stored row.
	// It fails with ErrNotFound or *DuplicateContactError.
	UpdateRecipient(ctx context.Context, r Recipient) (Recipient, error)
	// DeleteRecipient removes a recipient and their group memberships. It
	// fails with ErrNotFound, or ErrRecipientInUse when a batch refers to them.
	DeleteRecipient(ctx context.Context, id string) error
}

// Service is the manual management of the recipient base (description.md:
// "dodawać i edytować odbiorców ręcznie").
type Service struct {
	store RecipientStore
}

// NewService returns a Service backed by store.
func NewService(store RecipientStore) *Service {
	return &Service{store: store}
}

// List returns one page of recipients. Filtering by contact issue happens
// here, with the same rules Validate uses, rather than in SQL: a school has
// a few thousand recipients at most, and a second copy of the e-mail rule in
// SQL would drift from the Go one.
func (s *Service) List(ctx context.Context, q ListQuery) (Page, error) {
	q, err := checkListQuery(q)
	if err != nil {
		return Page{}, err
	}
	all, err := s.store.ListRecipients(ctx, q.Filter)
	if err != nil {
		return Page{}, fmt.Errorf("list recipients: %w", err)
	}
	if q.Issue != "" {
		kept := all[:0]
		for _, r := range all {
			if r.HasIssue(q.Issue) {
				kept = append(kept, r)
			}
		}
		all = kept
	}

	page := Page{Total: len(all), Items: []Recipient{}}
	if q.Offset < len(all) {
		page.Items = all[q.Offset:min(q.Offset+q.Limit, len(all))]
	}
	return page, nil
}

func checkListQuery(q ListQuery) (ListQuery, error) {
	var errs []FieldError
	q.Query = strings.TrimSpace(q.Query)
	if utf8.RuneCountInString(q.Query) > MaxQueryLength {
		errs = append(errs, FieldError{Field: "q", Message: fmt.Sprintf("longer than %d characters", MaxQueryLength)})
	}
	if q.Type != "" && q.Type != TypeParent && q.Type != TypeStudent {
		errs = append(errs, FieldError{Field: "type", Message: "type must be 'parent' or 'student'"})
	}
	if q.Class = NormalizeClass(q.Class); q.Class != "" && !ValidClass(q.Class) {
		errs = append(errs, FieldError{Field: "class", Message: "not a valid class name, expected e.g. 3A"})
	}
	if q.Issue != "" && q.Issue != ChannelEmail && q.Issue != ChannelSMS {
		errs = append(errs, FieldError{Field: "issue", Message: "issue must be 'email' or 'sms'"})
	}
	switch {
	case q.Limit == 0:
		q.Limit = DefaultPageSize
	case q.Limit < 0 || q.Limit > MaxPageSize:
		errs = append(errs, FieldError{Field: "limit", Message: fmt.Sprintf("limit must be between 1 and %d", MaxPageSize)})
	}
	if q.Offset < 0 {
		errs = append(errs, FieldError{Field: "offset", Message: "offset must not be negative"})
	}
	if len(errs) > 0 {
		return ListQuery{}, &ParamError{Fields: errs}
	}
	return q, nil
}

// Get returns one recipient with their custom groups.
func (s *Service) Get(ctx context.Context, id string) (Details, error) {
	id, err := checkID(id)
	if err != nil {
		return Details{}, err
	}
	r, err := s.store.GetRecipient(ctx, id)
	if err != nil {
		return Details{}, fmt.Errorf("get recipient %s: %w", id, err)
	}
	groupIDs, err := s.store.RecipientGroupIDs(ctx, id)
	if err != nil {
		return Details{}, fmt.Errorf("groups of recipient %s: %w", id, err)
	}
	return Details{Recipient: r, GroupIDs: groupIDs}, nil
}

// Create adds a recipient. A contact used by someone else fails with
// *DuplicateContactError naming that person, so the UI can link to them.
func (s *Service) Create(ctx context.Context, in Input) (Recipient, error) {
	r, err := s.checkInput(ctx, "", in)
	if err != nil {
		return Recipient{}, err
	}
	created, err := s.store.CreateRecipient(ctx, r)
	if err != nil {
		return Recipient{}, fmt.Errorf("create recipient: %w", err)
	}
	return created, nil
}

// Update replaces a recipient's data.
func (s *Service) Update(ctx context.Context, id string, in Input) (Recipient, error) {
	id, err := checkID(id)
	if err != nil {
		return Recipient{}, err
	}
	// Check existence first: a 404 is more useful than field errors for a
	// recipient someone else has just deleted.
	current, err := s.store.GetRecipient(ctx, id)
	if err != nil {
		return Recipient{}, fmt.Errorf("get recipient %s: %w", id, err)
	}
	if in.Classes == nil {
		in.Classes = &current.Classes
	}
	r, err := s.checkInput(ctx, id, in)
	if err != nil {
		return Recipient{}, err
	}
	r.ID = id
	updated, err := s.store.UpdateRecipient(ctx, r)
	if err != nil {
		return Recipient{}, fmt.Errorf("update recipient %s: %w", id, err)
	}
	return updated, nil
}

// Delete removes a recipient. Someone already messaged cannot be deleted:
// the history must keep showing who received a batch.
func (s *Service) Delete(ctx context.Context, id string) error {
	id, err := checkID(id)
	if err != nil {
		return err
	}
	if err := s.store.DeleteRecipient(ctx, id); err != nil {
		return fmt.Errorf("delete recipient %s: %w", id, err)
	}
	return nil
}

// checkInput normalizes and validates in, then makes sure no other recipient
// than selfID uses its e-mail or phone. The unique indexes still guard
// against a concurrent request; this check exists to name the other person.
func (s *Service) checkInput(ctx context.Context, selfID string, in Input) (Recipient, error) {
	r := in.normalize()
	if errs := r.Validate(); len(errs) > 0 {
		return Recipient{}, &ValidationError{Fields: errs}
	}

	var emails, phones []string
	if r.Email != "" {
		emails = []string{normalizeEmail(r.Email)}
	}
	if r.Phone != "" {
		phones = []string{r.Phone}
	}
	existing, err := s.store.FindContacts(ctx, emails, phones)
	if err != nil {
		return Recipient{}, fmt.Errorf("look up contacts: %w", err)
	}
	if owner, ok := existing.Emails[normalizeEmail(r.Email)]; ok && owner != selfID {
		return Recipient{}, &DuplicateContactError{Field: "email", ExistingID: owner}
	}
	if owner, ok := existing.Phones[r.Phone]; ok && owner != selfID {
		return Recipient{}, &DuplicateContactError{Field: "phone", ExistingID: owner}
	}
	return r, nil
}

func checkID(id string) (string, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if !idPattern.MatchString(id) {
		return "", &ParamError{Fields: []FieldError{{Field: "id", Message: "not a valid ID"}}}
	}
	return id, nil
}
