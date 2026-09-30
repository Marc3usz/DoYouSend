package groups

import (
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

func TestSystemGroups(t *testing.T) {
	groups := SystemGroups()
	seen := make(map[string]bool)
	for _, g := range groups {
		if !g.IsSystem() || g.Rule == nil {
			t.Errorf("%s: want a system group with a rule, got %+v", g.Name, g)
		}
		if !IsValidID(g.ID) || g.ID != canonicalID(g.ID) {
			t.Errorf("%s: ID %q is not a canonical UUID", g.Name, g.ID)
		}
		if errs := (GroupInput{Name: g.Name, Description: g.Description}).Validate(); len(errs) > 0 {
			t.Errorf("%s: built-in group fails its own validation: %v", g.Name, errs)
		}
		if seen[g.ID] || seen[nameKey(g.Name)] {
			t.Errorf("%s: repeated ID or name", g.Name)
		}
		seen[g.ID], seen[nameKey(g.Name)] = true, true
	}

	// Callers get their own copy.
	groups[0].Name = "changed"
	groups[0].Rule.Type = recipients.TypeStudent
	if again := SystemGroups(); again[0].Name == "changed" || again[0].Rule.Type != recipients.TypeParent {
		t.Errorf("SystemGroups() shares state between calls")
	}
}

func TestSystemRuleMatches(t *testing.T) {
	parent := recipients.Recipient{Type: recipients.TypeParent}
	student := recipients.Recipient{Type: recipients.TypeStudent}

	tests := []struct {
		id      string
		parent  bool
		student bool
	}{
		{id: AllParentsID, parent: true},
		{id: AllStudentsID, student: true},
	}
	for _, tt := range tests {
		g, ok := systemGroup(tt.id)
		if !ok {
			t.Fatalf("systemGroup(%s) not found", tt.id)
		}
		if g.Rule.Matches(parent) != tt.parent || g.Rule.Matches(student) != tt.student {
			t.Errorf("%s: Matches(parent) = %v, Matches(student) = %v", g.Name, g.Rule.Matches(parent), g.Rule.Matches(student))
		}
	}
	if _, ok := systemGroup(idMissingGroup); ok {
		t.Errorf("systemGroup(unknown) found a group")
	}
}

func TestIsSystemName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"Wszyscy rodzice", true},
		{"  wszyscy   UCZNIOWIE ", true},
		{"Wszyscy rodzice 3A", false},
		{"Rodzice", false},
	}
	for _, tt := range tests {
		if got := isSystemName(tt.name); got != tt.want {
			t.Errorf("isSystemName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
