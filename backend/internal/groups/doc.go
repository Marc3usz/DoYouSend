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
// Layout:
//   - group.go     - the Group model and validation of administrator input,
//   - system.go    - built-in groups, computed from recipient data instead of stored members,
//   - selection.go - what a sender picks (groups, single people, exclusions),
//   - resolve.go   - Resolver: selection -> deduplicated list with contact problems flagged,
//   - service.go   - Service: custom group CRUD and membership,
//   - store.go     - storage interfaces this package consumes,
//   - memstore.go  - in-memory Store until the Postgres one lands.
//
// Class-based built-in groups (parents/students of class 3A) are not implemented yet: the
// schema has no class and no parent-student link. See docs/adr/0007-ksztalt-resolve.md.
//
// Owner: DEV A.
package groups
