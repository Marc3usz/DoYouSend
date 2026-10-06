package groups

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

// GroupInfo is a group as the group list shows it.
type GroupInfo struct {
	Group
	MemberCount int
}

// MembershipChange reports what AddMembers or RemoveMembers did. Unchanged
// counts IDs that were already in (or already out of) the group.
type MembershipChange struct {
	Changed   int
	Unchanged int
}

// Service manages custom groups and their members. Built-in groups are listed
// and readable through it, but every change to them fails with ErrSystemGroup.
type Service struct {
	store Store
	dir   Directory
}

// NewService returns a Service storing groups in store and checking member
// IDs against dir.
func NewService(store Store, dir Directory) *Service {
	return &Service{store: store, dir: dir}
}

// List returns the built-in groups first, then custom groups by name, each
// with its current number of members.
func (s *Service) List(ctx context.Context) ([]GroupInfo, error) {
	custom, err := s.store.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	slices.SortFunc(custom, func(a, b Group) int {
		if c := compareNames(a.Name, b.Name); c != 0 {
			return c
		}
		return compareNames(a.ID, b.ID)
	})

	ids := make([]string, len(custom))
	for i, g := range custom {
		ids[i] = g.ID
	}
	var members map[string][]string
	if len(ids) > 0 {
		members, err = s.store.Members(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("count members of %d groups: %w", len(ids), err)
		}
	}

	out := make([]GroupInfo, 0, len(custom)+2)
	for _, g := range SystemGroups() {
		rs, err := s.dir.RecipientsByType(ctx, g.Rule.Type)
		if err != nil {
			return nil, fmt.Errorf("count members of group %s: %w", g.ID, err)
		}
		out = append(out, GroupInfo{Group: g, MemberCount: countMatching(rs, *g.Rule)})
	}
	for _, g := range custom {
		out = append(out, GroupInfo{Group: g, MemberCount: len(members[g.ID])})
	}
	return out, nil
}

// Get returns one group, built-in or custom.
func (s *Service) Get(ctx context.Context, id string) (Group, error) {
	id, err := checkID(id)
	if err != nil {
		return Group{}, err
	}
	if g, ok := systemGroup(id); ok {
		return g, nil
	}
	return s.custom(ctx, id)
}

// Info returns one group with its current number of members.
func (s *Service) Info(ctx context.Context, id string) (GroupInfo, error) {
	g, err := s.Get(ctx, id)
	if err != nil {
		return GroupInfo{}, err
	}
	if g.IsSystem() {
		rs, err := s.dir.RecipientsByType(ctx, g.Rule.Type)
		if err != nil {
			return GroupInfo{}, fmt.Errorf("count members of group %s: %w", g.ID, err)
		}
		return GroupInfo{Group: g, MemberCount: countMatching(rs, *g.Rule)}, nil
	}
	members, err := s.store.Members(ctx, []string{g.ID})
	if err != nil {
		return GroupInfo{}, fmt.Errorf("count members of group %s: %w", g.ID, err)
	}
	return GroupInfo{Group: g, MemberCount: len(members[g.ID])}, nil
}

// Create stores a new custom group.
func (s *Service) Create(ctx context.Context, in GroupInput) (Group, error) {
	in, err := s.checkInput(in)
	if err != nil {
		return Group{}, err
	}
	g, err := s.store.CreateGroup(ctx, Group{Name: in.Name, Description: in.Description, Kind: KindCustom})
	if err != nil {
		return Group{}, fmt.Errorf("create group: %w", err)
	}
	return g, nil
}

// Update renames a custom group or changes its description.
func (s *Service) Update(ctx context.Context, id string, in GroupInput) (Group, error) {
	g, err := s.editable(ctx, id)
	if err != nil {
		return Group{}, err
	}
	in, err = s.checkInput(in)
	if err != nil {
		return Group{}, err
	}
	g.Name, g.Description = in.Name, in.Description
	if err := s.store.UpdateGroup(ctx, g); err != nil {
		return Group{}, fmt.Errorf("update group %s: %w", g.ID, err)
	}
	return g, nil
}

// Delete removes a custom group. Its members stay in the recipient base; only
// the membership is gone. Batches already sent keep the group ID in their
// history (message_batches.selected_groups).
func (s *Service) Delete(ctx context.Context, id string) error {
	g, err := s.editable(ctx, id)
	if err != nil {
		return err
	}
	if err := s.store.DeleteGroup(ctx, g.ID); err != nil {
		return fmt.Errorf("delete group %s: %w", g.ID, err)
	}
	return nil
}

// Members returns the recipients of a group sorted by last name, first name.
func (s *Service) Members(ctx context.Context, id string) ([]recipients.Recipient, error) {
	g, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	var rs []recipients.Recipient
	if g.IsSystem() {
		all, err := s.dir.RecipientsByType(ctx, g.Rule.Type)
		if err != nil {
			return nil, fmt.Errorf("list members of group %s: %w", g.ID, err)
		}
		for _, r := range all {
			if g.Rule.Matches(r) {
				rs = append(rs, r)
			}
		}
	} else {
		members, err := s.store.Members(ctx, []string{g.ID})
		if err != nil {
			return nil, fmt.Errorf("list members of group %s: %w", g.ID, err)
		}
		if ids := members[g.ID]; len(ids) > 0 {
			rs, err = s.dir.RecipientsByIDs(ctx, ids)
			if err != nil {
				return nil, fmt.Errorf("look up members of group %s: %w", g.ID, err)
			}
		}
	}

	slices.SortFunc(rs, func(a, b recipients.Recipient) int {
		return compareResolved(Resolved{Recipient: a}, Resolved{Recipient: b})
	})
	return rs, nil
}

// AddMembers adds recipients to a custom group. It is all-or-nothing: if any
// ID is malformed or unknown, nothing is added and the error lists them all
// (*ValidationError or *UnknownRecipientsError).
func (s *Service) AddMembers(ctx context.Context, groupID string, recipientIDs []string) (MembershipChange, error) {
	g, ids, err := s.membershipRequest(ctx, groupID, recipientIDs)
	if err != nil {
		return MembershipChange{}, err
	}

	found, err := s.dir.RecipientsByIDs(ctx, ids)
	if err != nil {
		return MembershipChange{}, fmt.Errorf("look up %d recipients: %w", len(ids), err)
	}
	if unknown := missingIDs(ids, found); len(unknown) > 0 {
		return MembershipChange{}, fmt.Errorf("add members to group %s: %w", g.ID, &UnknownRecipientsError{IDs: unknown})
	}

	n, err := s.store.AddMembers(ctx, g.ID, ids)
	if err != nil {
		return MembershipChange{}, fmt.Errorf("add %d members to group %s: %w", len(ids), g.ID, err)
	}
	return MembershipChange{Changed: n, Unchanged: len(ids) - n}, nil
}

// RemoveMembers removes recipients from a custom group. IDs that are not
// members (including recipients deleted meanwhile) are counted as unchanged.
func (s *Service) RemoveMembers(ctx context.Context, groupID string, recipientIDs []string) (MembershipChange, error) {
	g, ids, err := s.membershipRequest(ctx, groupID, recipientIDs)
	if err != nil {
		return MembershipChange{}, err
	}
	n, err := s.store.RemoveMembers(ctx, g.ID, ids)
	if err != nil {
		return MembershipChange{}, fmt.Errorf("remove %d members from group %s: %w", len(ids), g.ID, err)
	}
	return MembershipChange{Changed: n, Unchanged: len(ids) - n}, nil
}

func (s *Service) membershipRequest(ctx context.Context, groupID string, recipientIDs []string) (Group, []string, error) {
	g, err := s.editable(ctx, groupID)
	if err != nil {
		return Group{}, nil, err
	}
	ids := uniqueIDs(recipientIDs)
	if len(ids) == 0 {
		return Group{}, nil, &ValidationError{Fields: []recipients.FieldError{{Field: "recipientIds", Message: "at least one recipient is required"}}}
	}
	if errs := validateIDs("recipientIds", ids); len(errs) > 0 {
		return Group{}, nil, &ValidationError{Fields: errs}
	}
	return g, ids, nil
}

// editable returns the custom group with id, or ErrSystemGroup / ErrNotFound.
func (s *Service) editable(ctx context.Context, id string) (Group, error) {
	id, err := checkID(id)
	if err != nil {
		return Group{}, err
	}
	if _, ok := systemGroup(id); ok {
		return Group{}, fmt.Errorf("group %s: %w", id, ErrSystemGroup)
	}
	return s.custom(ctx, id)
}

func (s *Service) custom(ctx context.Context, id string) (Group, error) {
	found, err := s.store.FindGroups(ctx, []string{id})
	if err != nil {
		return Group{}, fmt.Errorf("find group %s: %w", id, err)
	}
	for _, g := range found {
		if canonicalID(g.ID) == id {
			return g, nil
		}
	}
	return Group{}, fmt.Errorf("group %s: %w", id, ErrNotFound)
}

// checkInput normalizes and validates in. Names of built-in groups are
// reserved, so the picker never shows two groups with the same name.
func (s *Service) checkInput(in GroupInput) (GroupInput, error) {
	in = in.Normalize()
	if errs := in.Validate(); len(errs) > 0 {
		return GroupInput{}, &ValidationError{Fields: errs}
	}
	if isSystemName(in.Name) {
		return GroupInput{}, fmt.Errorf("group name %q: %w", in.Name, ErrNameTaken)
	}
	return in, nil
}

func checkID(id string) (string, error) {
	id = canonicalID(id)
	if !IsValidID(id) {
		return "", &ValidationError{Fields: []recipients.FieldError{{Field: "id", Message: "not a valid ID"}}}
	}
	return id, nil
}

// missingIDs returns the ids that have no recipient in found, in ids order.
func missingIDs(ids []string, found []recipients.Recipient) []string {
	have := make(map[string]bool, len(found))
	for _, r := range found {
		have[canonicalID(r.ID)] = true
	}
	var out []string
	for _, id := range ids {
		if !have[id] {
			out = append(out, id)
		}
	}
	return out
}

func countMatching(rs []recipients.Recipient, rule SystemRule) int {
	n := 0
	for _, r := range rs {
		if rule.Matches(r) {
			n++
		}
	}
	return n
}

func compareNames(a, b string) int {
	return strings.Compare(nameKey(a), nameKey(b))
}
