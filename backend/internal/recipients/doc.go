// Package recipients owns the recipient base: first/last name, e-mail, phone number,
// recipient type (parent/student) and the data-quality rules around them.
//
// Scope (see description.md "Baza odbiorcow"):
//   - manual CRUD for recipients,
//   - CSV/XLSX import with a per-row report,
//   - validation of e-mail addresses and phone numbers,
//   - detection of incomplete and duplicated records.
//
// Owner: DEV A. Out of scope: group membership (package groups), sending (package delivery).
package recipients
