// Package senders decides which e-mail addresses a user may send from
// (ADR-0021) and manages what makes an address usable with the e-mail
// operator:
//
//   - the shared address from the sending settings (EMAIL_FROM),
//   - the user's own login address, usable automatically once its domain is
//     authenticated with the operator (SendGrid Domain Authentication) or the
//     address is a verified single sender (ADR-0016),
//   - shared mailboxes (sekretariat@...) an administrator delegates to users.
//
// The composer offers the options of the signed-in user, and delivery asks
// ResolveSender for the From of a batch, so nobody sends from an address that
// was not given to them. Only the e-mail From and Reply-To change: the body
// stays byte-identical on both channels (CLAUDE.md rule 4).
//
// Owner: Marc3usz (domains C and D, ADR-0020).
package senders

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// Kind is where a sender option comes from.
type Kind string

// Sender option kinds.
const (
	KindShared    Kind = "shared"    // EMAIL_FROM from the sending settings
	KindOwn       Kind = "own"       // the user's login address
	KindDelegated Kind = "delegated" // a mailbox an administrator gave the user
)

// Fixed option IDs; a delegated option's ID is its address ID.
const (
	IDShared = "shared"
	IDOwn    = "own"
)

// Reason says why an address cannot be used as sender right now.
type Reason string

// Reasons an option is not usable.
const (
	ReasonNone                   Reason = ""
	ReasonDomainNotAuthenticated Reason = "domain_not_authenticated"
	ReasonNotVerified            Reason = "not_verified" // a single sender exists but is not confirmed yet
)

// Option is one address a user may pick as the e-mail sender.
type Option struct {
	ID      string
	Kind    Kind
	Address providers.Address
	Usable  bool
	Reason  Reason
}

// User is the signed-in user, as far as senders needs to know.
type User struct {
	ID       string
	Email    string
	FullName string
}

// DomainStatus is the last known state of an authenticated domain.
type DomainStatus string

// Domain states.
const (
	DomainPending DomainStatus = "pending" // DNS records not confirmed by the operator yet
	DomainValid   DomainStatus = "valid"   // every address in the domain may send
	DomainMissing DomainStatus = "missing" // the operator no longer knows it
)

// DNSRecord is one record the school adds at its DNS provider.
type DNSRecord struct {
	Type  string `json:"type"` // cname, txt or mx
	Host  string `json:"host"`
	Data  string `json:"data"`
	Valid bool   `json:"valid"`
}

// Domain is a row of sender_domains.
type Domain struct {
	ID          string
	Domain      string
	Provider    string
	ProviderID  string
	Status      DomainStatus
	Records     []DNSRecord
	CreatedAt   time.Time
	CheckedAt   time.Time // zero when never checked
	ValidatedAt time.Time // zero while not valid
}

// UserRef names a user an address is delegated to.
type UserRef struct {
	ID       string
	FullName string
}

// Address is a delegated mailbox (sender_addresses) with its users.
type Address struct {
	ID        string
	Email     string
	Name      string
	Users     []UserRef
	CreatedAt time.Time
}

// ProviderDomain is a domain as the operator reports it.
type ProviderDomain struct {
	ID      string
	Valid   bool
	Records []DNSRecord
}

// DomainAPI is the operator's domain-authentication API, defined here by its
// consumer. The adapter maps "unknown domain" to ErrDomainGone and a refused
// request to a *RejectedError.
type DomainAPI interface {
	CreateDomain(ctx context.Context, domain string) (ProviderDomain, error)
	GetDomain(ctx context.Context, id string) (ProviderDomain, error)
	// ValidateDomain asks the operator to check the DNS records now.
	ValidateDomain(ctx context.Context, id string) (ProviderDomain, error)
	DeleteDomain(ctx context.Context, id string) error
}

// Store keeps domains and delegated addresses; PGStore implements it.
type Store interface {
	ListDomains(ctx context.Context) ([]Domain, error)
	// GetDomain returns ErrNotFound for an unknown ID.
	GetDomain(ctx context.Context, id string) (Domain, error)
	// CreateDomain fails with ErrDomainExists for a domain already added.
	CreateDomain(ctx context.Context, d Domain, actorID string) (Domain, error)
	SaveDomainCheck(ctx context.Context, id string, status DomainStatus, records []DNSRecord, checkedAt time.Time) (Domain, error)
	DeleteDomain(ctx context.Context, id string) error

	ListAddresses(ctx context.Context) ([]Address, error)
	GetAddress(ctx context.Context, id string) (Address, error)
	// CreateAddress fails with ErrAddressExists for an address already added and
	// with ErrUnknownUsers when a user ID is not an active user.
	CreateAddress(ctx context.Context, a Address, userIDs []string, actorID string) (Address, error)
	// UpdateAddress changes the name and/or replaces the users; nil leaves a field.
	UpdateAddress(ctx context.Context, id string, name *string, userIDs []string, setUsers bool, actorID string) (Address, error)
	DeleteAddress(ctx context.Context, id string) error
	// AddressesOf returns the addresses delegated to userID.
	AddressesOf(ctx context.Context, userID string) ([]Address, error)

	// DomainValid reports whether domain is authenticated with provider.
	DomainValid(ctx context.Context, provider, domain string) (bool, error)
	// SingleSender reports whether email has a single-sender identity with
	// provider (ADR-0016), and whether it is verified.
	SingleSender(ctx context.Context, provider, email string) (exists, verified bool, err error)
}

// Auditor records an action in the audit log; iam.Service implements it.
type Auditor interface {
	Record(ctx context.Context, userID, action, entity, entityID string, details map[string]string)
}

// Audit actions; entity "sender_domain" or "sender_address".
const (
	ActionDomainAdded    = "sender_domain_added"
	ActionDomainRemoved  = "sender_domain_removed"
	ActionAddressCreated = "sender_address_added"
	ActionAddressUpdated = "sender_address_updated"
	ActionAddressRemoved = "sender_address_removed"

	entityDomain  = "sender_domain"
	entityAddress = "sender_address"
)

// Errors. Match them with errors.Is.
var (
	ErrNotFound      = errors.New("not found")
	ErrDomainExists  = errors.New("the domain is already added")
	ErrAddressExists = errors.New("the address is already added")
	ErrUnknownUsers  = errors.New("some user IDs are not active users")
	// ErrNotAllowed: the user may not send from the chosen address, or it is
	// not usable with the operator right now.
	ErrNotAllowed = errors.New("sender address not allowed")
	// ErrDomainGone is returned by a DomainAPI for a domain the operator does
	// not know (anymore).
	ErrDomainGone = errors.New("the operator does not know this domain")
	ErrInvalid    = errors.New("invalid input")
)

// RejectedError is the operator refusing a request. Message is the operator's
// text, shown to the administrator; it never holds credentials.
type RejectedError struct{ Message string }

func (e *RejectedError) Error() string { return "rejected by the e-mail operator: " + e.Message }

// FieldError is a problem with one input field.
type FieldError struct{ Field, Message string }

// ValidationError lists rejected fields; it matches ErrInvalid.
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string {
	parts := []string{"invalid input"}
	for _, f := range e.Fields {
		parts = append(parts, f.Field+": "+f.Message)
	}
	return strings.Join(parts, "; ")
}

// Is makes errors.Is(err, ErrInvalid) match.
func (e *ValidationError) Is(target error) bool { return target == ErrInvalid }

func invalid(field, message string) error {
	return &ValidationError{Fields: []FieldError{{Field: field, Message: message}}}
}
