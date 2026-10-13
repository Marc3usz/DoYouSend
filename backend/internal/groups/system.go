package groups

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

// Well-known IDs of the built-in groups. They are fixed so a selection saved in
// message_batches.selected_groups keeps pointing at the same group across
// deployments; the rows carrying them are inserted by a migration (ADR-0007).
const (
	AllParentsID  = "00000000-0000-4000-a000-000000000001"
	AllStudentsID = "00000000-0000-4000-a000-000000000002"
)

// ClassNamespace is the UUIDv5 namespace of class group IDs (ADR-0009). It must
// never change after the first send: every class group ID, and with it every
// saved selection, is derived from it.
const ClassNamespace = "733b2535-1e27-428a-a23d-c275f4eab3fa"

// Audience says whom a class group covers.
type Audience string

const (
	AudienceStudents Audience = "students"
	AudienceParents  Audience = "parents"
)

// Names of class groups; the prefixes are reserved for them.
const (
	studentsOfClassPrefix = "Uczniowie klasy "
	parentsOfClassPrefix  = "Rodzice uczniów klasy "
)

// SystemRule says which recipients belong to a built-in group. Membership is
// computed on every read, so a recipient added, retyped or moved to another
// class by an import is in the right built-in groups at once, with nothing to
// keep in sync.
type SystemRule struct {
	// Type selects every recipient of this type.
	Type recipients.Type
	// Class, when set, keeps only recipients assigned to this class (a class
	// group, ADR-0009).
	Class string
}

// Matches reports whether rec belongs to a group with this rule.
func (r SystemRule) Matches(rec recipients.Recipient) bool {
	return rec.Type == r.Type && (r.Class == "" || slices.Contains(rec.Classes, r.Class))
}

// Audience returns whom a class group covers, or "" for other groups.
func (r SystemRule) Audience() Audience {
	switch {
	case r.Class == "":
		return ""
	case r.Type == recipients.TypeStudent:
		return AudienceStudents
	default:
		return AudienceParents
	}
}

// SystemGroups returns the fixed built-in groups in the order the group picker
// shows them. The slice is new on every call; callers may modify it.
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

// ClassGroups returns the two groups of each class: its students, then the
// parents of its students, classes in recipients.CompareClasses order.
func ClassGroups(classes []string) []Group {
	sorted := slices.Clone(classes)
	slices.SortFunc(sorted, recipients.CompareClasses)
	out := make([]Group, 0, 2*len(sorted))
	for _, c := range slices.Compact(sorted) {
		out = append(out,
			Group{
				ID:          classGroupID(AudienceStudents, c),
				Name:        studentsOfClassPrefix + c,
				Description: "Uczniowie przypisani do klasy " + c + ".",
				Kind:        KindSystem,
				Rule:        &SystemRule{Type: recipients.TypeStudent, Class: c},
			},
			Group{
				ID:          classGroupID(AudienceParents, c),
				Name:        parentsOfClassPrefix + c,
				Description: "Rodzice z klasą " + c + " (klasa ich dziecka).",
				Kind:        KindSystem,
				Rule:        &SystemRule{Type: recipients.TypeParent, Class: c},
			},
		)
	}
	return out
}

// classGroupID is the stable ID of a class group: UUIDv5 (RFC 9562) of
// "students:3A" or "parents:3A" in ClassNamespace.
func classGroupID(a Audience, class string) string {
	ns := uuidBytes(ClassNamespace)
	h := sha1.New()
	h.Write(ns[:])
	h.Write([]byte(string(a) + ":" + class))
	sum := h.Sum(nil)
	sum[6] = sum[6]&0x0f | 0x50 // version 5
	sum[8] = sum[8]&0x3f | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

// uuidBytes parses a canonical UUID constant that is known to be valid.
func uuidBytes(id string) [16]byte {
	var out [16]byte
	_, _ = hex.Decode(out[:], []byte(strings.ReplaceAll(id, "-", "")))
	return out
}

// systemGroup returns the fixed built-in group with the given (canonical) ID.
func systemGroup(id string) (Group, bool) {
	for _, g := range SystemGroups() {
		if g.ID == id {
			return g, true
		}
	}
	return Group{}, false
}

// builtIn returns the fixed or class group with the given (canonical) ID. A
// class group exists while some recipient is assigned to its class.
func builtIn(ctx context.Context, dir Directory, id string) (Group, bool, error) {
	if g, ok := systemGroup(id); ok {
		return g, true, nil
	}
	classes, err := dir.Classes(ctx)
	if err != nil {
		return Group{}, false, fmt.Errorf("list classes: %w", err)
	}
	for _, g := range ClassGroups(classes) {
		if g.ID == id {
			return g, true, nil
		}
	}
	return Group{}, false, nil
}

// isSystemName reports whether name collides with a built-in group's name, or
// looks like a class group, so an administrator cannot create a second,
// editable "Wszyscy rodzice" or "Uczniowie klasy 3A".
func isSystemName(name string) bool {
	key := nameKey(name)
	for _, g := range SystemGroups() {
		if nameKey(g.Name) == key {
			return true
		}
	}
	for _, prefix := range []string{studentsOfClassPrefix, parentsOfClassPrefix} {
		if strings.HasPrefix(key, nameKey(prefix)+" ") {
			return true
		}
	}
	return false
}
