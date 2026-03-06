// Package postgresql provides a PostgreSQL-backed rate limiter storage backend.
//
// State is stored in a dedicated table with row-level locking via
// SELECT … FOR UPDATE for atomic updates. The table-name prefix is validated
// against ^[a-zA-Z0-9_]*$ at construction time to prevent SQL injection.
package postgresql
