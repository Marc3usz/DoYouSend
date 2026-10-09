// Package iam owns users, roles and the audit trail.
//
// Scope (see description.md "Bezpieczenstwo" and "Panel administracyjny"):
//   - authentication and sessions,
//   - roles: administrator, sender (director / authorised employee),
//   - authorisation checks for admin-only operations,
//   - audit log: who composed and who confirmed each batch.
//
// Owner: DEV D (averithefox). First version written by DEV A on DEV D's behalf
// (ADR-0010); review responsibility stays with DEV D.
package iam
