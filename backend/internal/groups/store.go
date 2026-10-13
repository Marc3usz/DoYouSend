package groups

import (
	"context"

	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

// Store is the group storage this package needs. It holds custom groups and
// their members only: built-in groups live in code (system.go) and compute
// their members from the Directory. The Postgres implementation lands together
// with the storage layer; MemStore stands in until then.
type Store interface {
	// ListGroups returns every custom group, in no particular order. Rows of
	// built-in groups (groups.is_system, ADR-0007) must be left out: they come
	// from SystemGroups, and listing them here would show them twice.
	ListGroups(ctx context.Context) ([]Group, error)
	// FindGroups returns the custom groups among ids. Unknown IDs and built-in
	// groups are left out, not reported as an error.
	FindGroups(ctx context.Context, ids []string) ([]Group, error)
	// CreateGroup stores g, assigning its ID and CreatedAt, and returns the
	// stored group. It fails with ErrNameTaken when another custom group has
	// the same name (compared case-insensitively).
	CreateGroup(ctx context.Context, g Group) (Group, error)
	// UpdateGroup replaces the name and description of the group with g.ID.
	// It fails with ErrNotFound or ErrNameTaken.
	UpdateGroup(ctx context.Context, g Group) error
	// DeleteGroup removes a group and its memberships. It fails with ErrNotFound.
	DeleteGroup(ctx context.Context, id string) error
	// Members returns the member recipient IDs of each of groupIDs. A group
	// without members, or an unknown group, maps to no entry.
	Members(ctx context.Context, groupIDs []string) (map[string][]string, error)
	// AddMembers adds recipientIDs to a group, ignoring those already in it,
	// and returns how many were added. It fails with ErrNotFound.
	AddMembers(ctx context.Context, groupID string, recipientIDs []string) (int, error)
	// RemoveMembers removes recipientIDs from a group, ignoring those not in
	// it, and returns how many were removed. It fails with ErrNotFound.
	RemoveMembers(ctx context.Context, groupID string, recipientIDs []string) (int, error)
}

// Directory is the read-only view of the recipient base this package needs.
// It is defined here by its consumer; package recipients provides the storage.
type Directory interface {
	// RecipientsByIDs returns the recipients among ids. Unknown IDs are left
	// out, not reported as an error.
	RecipientsByIDs(ctx context.Context, ids []string) ([]recipients.Recipient, error)
	// RecipientsByType returns every recipient of type t.
	RecipientsByType(ctx context.Context, t recipients.Type) ([]recipients.Recipient, error)
	// Classes returns every class some recipient is assigned to (ADR-0009).
	Classes(ctx context.Context) ([]string, error)
}
