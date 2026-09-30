package groups

import "github.com/Marc3usz/DoYouSend/backend/internal/recipients"

// Well-known IDs of the built-in groups. They are fixed so a selection saved in
// message_batches.selected_groups keeps pointing at the same group across
// deployments; the rows carrying them are inserted by a migration (ADR-0007).
const (
	AllParentsID  = "00000000-0000-4000-a000-000000000001"
	AllStudentsID = "00000000-0000-4000-a000-000000000002"
)

// SystemRule says which recipients belong to a built-in group. Membership is
// computed on every read, so a recipient added or retyped by an import is in
// the right built-in groups at once, with nothing to keep in sync.
type SystemRule struct {
	// Type selects every recipient of this type.
	Type recipients.Type
}

// Matches reports whether rec belongs to a group with this rule.
func (r SystemRule) Matches(rec recipients.Recipient) bool {
	return rec.Type == r.Type
}

// SystemGroups returns the built-in groups in the order the group picker shows
// them. The slice is new on every call; callers may modify it.
func SystemGroups() []Group {
	return []Group{
		{
			ID:          AllParentsID,
			Name:        "Wszyscy rodzice",
			Description: "Każdy odbiorca typu rodzic.",
			Kind:        KindSystem,
			Rule:        &SystemRule{Type: recipients.TypeParent},
		},
		{
			ID:          AllStudentsID,
			Name:        "Wszyscy uczniowie",
			Description: "Każdy odbiorca typu uczeń.",
			Kind:        KindSystem,
			Rule:        &SystemRule{Type: recipients.TypeStudent},
		},
	}
}

// systemGroup returns the built-in group with the given (canonical) ID.
func systemGroup(id string) (Group, bool) {
	for _, g := range SystemGroups() {
		if g.ID == id {
			return g, true
		}
	}
	return Group{}, false
}

// isSystemName reports whether name collides with a built-in group's name, so
// an administrator cannot create a second, editable "Wszyscy rodzice".
func isSystemName(name string) bool {
	key := nameKey(name)
	for _, g := range SystemGroups() {
		if nameKey(g.Name) == key {
			return true
		}
	}
	return false
}
