// Package database owns the Postgres connection pool and transaction helpers.
// Shared plumbing - coordinate before editing.
//
// TODO(platform): wire pgxpool here once the first domain package needs storage.
package database
