// Package groups owns recipient groups and the resolution of a selection into a final
// recipient list.
//
// Scope (see description.md "Grupy odbiorcow"):
//   - built-in groups (all parents, all students, parents/students of a class),
//   - custom groups created by an administrator,
//   - many-to-many membership,
//   - Resolve(selection) -> deduplicated recipient list; a person picked through several
//     groups must appear exactly once.
//
// Owner: DEV A.
package groups
