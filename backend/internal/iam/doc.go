// Package iam owns users, roles and the audit trail.
//
// Scope (see description.md "Bezpieczenstwo" and "Panel administracyjny"):
//   - authentication and sessions,
//   - roles: administrator, sender (director / authorised employee),
//   - authorisation checks for admin-only operations,
//   - audit log: who composed and who confirmed each batch.
//
// Owner: Marc3usz (domain D, ADR-0012).
package iam
