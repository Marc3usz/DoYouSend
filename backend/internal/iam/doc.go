// Package iam owns users, roles and the audit trail.
//
// Scope (see description.md "Bezpieczenstwo" and "Panel administracyjny"):
//   - authentication and sessions,
//   - roles: administrator, sender (director / authorised employee),
//   - authorisation checks for admin-only operations,
//   - audit log: who composed and who confirmed each batch.
//
// Owner: DEV C.
package iam
