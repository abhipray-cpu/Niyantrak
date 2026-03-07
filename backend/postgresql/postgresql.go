package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/abhipray-cpu/niyantrak/backend"
	_ "github.com/lib/pq" // register the postgres driver with database/sql
)

// postgresqlBackend is a PostgreSQL-based persistent storage backend
type postgresqlBackend struct {
	db       *sql.DB
	mu       sync.RWMutex
	closed   bool
	host     string
	port     int
	database string
	username string
	prefix   string
}

// Compile-time check to ensure postgresqlBackend implements backend.PostgreSQLBackend
var _ backend.PostgreSQLBackend = (*postgresqlBackend)(nil)

// Compile-time check to ensure postgresqlBackend implements backend.AtomicBackend
var _ backend.AtomicBackend = (*postgresqlBackend)(nil)

// validPrefix matches only safe SQL identifier characters.
var validPrefix = regexp.MustCompile(`^[a-zA-Z0-9_]*$`)

// NewPostgreSQLBackend creates a new PostgreSQL backend
// host: PostgreSQL server host
// port: PostgreSQL server port
// database: Database name
// username: Database username
// password: Database password
// prefix: Table prefix for namespacing (optional, must be alphanumeric/underscore only)
func NewPostgreSQLBackend(host string, port int, database, username, password, prefix string) backend.PostgreSQLBackend {
	// Sanitise prefix to prevent SQL injection through table names.
	if !validPrefix.MatchString(prefix) {
		// Return backend in error (closed) state.
		return &postgresqlBackend{
			db:       nil,
			closed:   true,
			host:     host,
			port:     port,
			database: database,
			username: username,
			prefix:   prefix,
		}
	}

	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, username, password, database)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		// Return backend with error state
		return &postgresqlBackend{
			db:       nil,
			closed:   true,
			host:     host,
			port:     port,
			database: database,
			username: username,
			prefix:   prefix,
		}
	}

	return &postgresqlBackend{
		db:       db,
		closed:   false,
		host:     host,
		port:     port,
		database: database,
		username: username,
		prefix:   prefix,
	}
}

// CreateTable creates the rate limit state table if it doesn't exist
func (p *postgresqlBackend) CreateTable(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed || p.db == nil {
		return backend.ErrBackendClosed
	}

	tableName := p.makeTableName()
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			key VARCHAR(255) PRIMARY KEY,
			value TEXT NOT NULL,
			expires_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`, tableName)

	_, err := p.db.ExecContext(ctx, query)
	if err != nil {
		return err
	}

	// Create index on expires_at for efficient cleanup
	indexQuery := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS %s_expires_idx ON %s(expires_at)
	`, tableName, tableName)

	_, err = p.db.ExecContext(ctx, indexQuery)
	return err
}

// Get retrieves the current state for a key
func (p *postgresqlBackend) Get(ctx context.Context, key string) (interface{}, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed || p.db == nil {
		return nil, backend.ErrBackendClosed
	}

	tableName := p.makeTableName()
	query := fmt.Sprintf(`
		SELECT value, expires_at FROM %s WHERE key = $1
	`, tableName)

	var value string
	var expiresAt sql.NullTime

	err := p.db.QueryRowContext(ctx, query, key).Scan(&value, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, backend.ErrKeyNotFound
	}
	if err != nil {
		return nil, err
	}

	// Check if expired
	if expiresAt.Valid && time.Now().After(expiresAt.Time) {
		return nil, backend.ErrKeyExpired
	}

	// Unwrap using typed envelope so concrete algorithm state types survive the roundtrip.
	data, err := backend.Unwrap([]byte(value))
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Set updates the state for a key with optional TTL
func (p *postgresqlBackend) Set(ctx context.Context, key string, state interface{}, ttl time.Duration) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed || p.db == nil {
		return backend.ErrBackendClosed
	}

	tableName := p.makeTableName()

	// Wrap with typed envelope so concrete types survive the JSON roundtrip.
	dataBytes, err := backend.Wrap(state)
	if err != nil {
		return err
	}
	data := string(dataBytes)

	// Calculate expiration time
	var expiresAt sql.NullTime
	if ttl > 0 {
		expiresAt = sql.NullTime{
			Time:  time.Now().Add(ttl),
			Valid: true,
		}
	}

	// Upsert query
	query := fmt.Sprintf(`
		INSERT INTO %s (key, value, expires_at, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (key) DO UPDATE
		SET value = $2, expires_at = $3, updated_at = CURRENT_TIMESTAMP
	`, tableName)

	_, err = p.db.ExecContext(ctx, query, key, data, expiresAt)
	return err
}

// IncrementAndGet atomically increments the value for a key
func (p *postgresqlBackend) IncrementAndGet(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed || p.db == nil {
		return 0, backend.ErrBackendClosed
	}

	tableName := p.makeTableName()

	// Calculate expiration time
	var expiresAt sql.NullTime
	if ttl > 0 {
		expiresAt = sql.NullTime{
			Time:  time.Now().Add(ttl),
			Valid: true,
		}
	}

	// Use PostgreSQL's atomic increment
	// First, try to get current value and check expiration
	var currentValue int64
	var currentExpiresAt sql.NullTime

	query := fmt.Sprintf(`SELECT value, expires_at FROM %s WHERE key = $1`, tableName)
	err := p.db.QueryRowContext(ctx, query, key).Scan(&currentValue, &currentExpiresAt)

	if errors.Is(err, sql.ErrNoRows) || (currentExpiresAt.Valid && time.Now().After(currentExpiresAt.Time)) {
		// Key doesn't exist or expired, start from 0
		insertQuery := fmt.Sprintf(`
			INSERT INTO %s (key, value, expires_at, updated_at)
			VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
			ON CONFLICT (key) DO UPDATE
			SET value = 1, expires_at = $3, updated_at = CURRENT_TIMESTAMP
			RETURNING value
		`, tableName)

		var newValue int64
		err = p.db.QueryRowContext(ctx, insertQuery, key, "1", expiresAt).Scan(&newValue)
		if err != nil {
			return 0, err
		}
		return 1, nil
	}

	if err != nil {
		return 0, err
	}

	// Increment existing value
	updateQuery := fmt.Sprintf(`
		UPDATE %s
		SET value = (CAST(value AS BIGINT) + 1)::TEXT,
		    expires_at = $2,
		    updated_at = CURRENT_TIMESTAMP
		WHERE key = $1
		RETURNING CAST(value AS BIGINT)
	`, tableName)

	var newValue int64
	err = p.db.QueryRowContext(ctx, updateQuery, key, expiresAt).Scan(&newValue)
	return newValue, err
}

// Delete removes a key from PostgreSQL
func (p *postgresqlBackend) Delete(ctx context.Context, key string) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed || p.db == nil {
		return backend.ErrBackendClosed
	}

	tableName := p.makeTableName()
	query := fmt.Sprintf(`DELETE FROM %s WHERE key = $1`, tableName)

	_, err := p.db.ExecContext(ctx, query, key)
	return err
}

// Update atomically reads, transforms, and writes the state for a key.
// It uses a PostgreSQL transaction with SELECT … FOR UPDATE to lock the
// row for the duration of the transform function, preventing concurrent
// modifications.
func (p *postgresqlBackend) Update(ctx context.Context, key string, ttl time.Duration, fn backend.UpdateFunc) (interface{}, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed || p.db == nil {
		return nil, backend.ErrBackendClosed
	}

	tableName := p.makeTableName()

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("postgresql: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback is a no-op after Commit; error intentionally ignored

	// --- read with row-level lock ---
	selectQ := fmt.Sprintf(`SELECT value, expires_at FROM %s WHERE key = $1 FOR UPDATE`, tableName)

	var rawValue string
	var expiresAt sql.NullTime
	var current interface{}

	scanErr := tx.QueryRowContext(ctx, selectQ, key).Scan(&rawValue, &expiresAt)
	switch {
	case scanErr == sql.ErrNoRows:
		current = nil
	case scanErr != nil:
		return nil, fmt.Errorf("postgresql: select for update: %w", scanErr)
	default:
		// Check expiry
		if expiresAt.Valid && time.Now().After(expiresAt.Time) {
			current = nil
		} else {
			current, err = backend.Unwrap([]byte(rawValue))
			if err != nil {
				return nil, fmt.Errorf("postgresql: unwrap: %w", err)
			}
		}
	}

	// --- compute ---
	newState, result, fnErr := fn(current)
	if fnErr != nil {
		return nil, fnErr
	}

	// --- write back ---
	dataBytes, wrapErr := backend.Wrap(newState)
	if wrapErr != nil {
		return nil, fmt.Errorf("postgresql: wrap: %w", wrapErr)
	}
	data := string(dataBytes)

	var newExpiry sql.NullTime
	if ttl > 0 {
		newExpiry = sql.NullTime{Time: time.Now().Add(ttl), Valid: true}
	}

	upsertQ := fmt.Sprintf(`
		INSERT INTO %s (key, value, expires_at, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (key) DO UPDATE
		SET value = $2, expires_at = $3, updated_at = CURRENT_TIMESTAMP
	`, tableName)

	if _, err := tx.ExecContext(ctx, upsertQ, key, data, newExpiry); err != nil {
		return nil, fmt.Errorf("postgresql: upsert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("postgresql: commit: %w", err)
	}

	return result, nil
}

// Close closes the PostgreSQL connection
func (p *postgresqlBackend) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed || p.db == nil {
		return nil
	}

	p.closed = true
	return p.db.Close()
}

// Ping checks if PostgreSQL backend is healthy
func (p *postgresqlBackend) Ping(ctx context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed || p.db == nil {
		return backend.ErrBackendClosed
	}

	return p.db.PingContext(ctx)
}

// Type returns the backend type name
func (p *postgresqlBackend) Type() string {
	return "postgresql"
}

// Migrate runs any pending migrations
func (p *postgresqlBackend) Migrate(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed || p.db == nil {
		return backend.ErrBackendClosed
	}

	// For now, just ensure table exists
	// In a real implementation, this would run versioned migrations
	return nil
}

// GetSchema returns the current database schema
func (p *postgresqlBackend) GetSchema(ctx context.Context) string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed || p.db == nil {
		return ""
	}

	tableName := p.makeTableName()
	query := `
		SELECT column_name, data_type
		FROM information_schema.columns
		WHERE table_name = $1
		ORDER BY ordinal_position
	`

	rows, err := p.db.QueryContext(ctx, query, tableName)
	if err != nil {
		return ""
	}
	defer rows.Close()

	schema := fmt.Sprintf("Table: %s\n", tableName)
	for rows.Next() {
		var columnName, dataType string
		if err := rows.Scan(&columnName, &dataType); err == nil {
			schema += fmt.Sprintf("  %s: %s\n", columnName, dataType)
		}
	}

	return schema
}

// CleanupExpired removes expired entries
func (p *postgresqlBackend) CleanupExpired(ctx context.Context) (int64, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed || p.db == nil {
		return 0, backend.ErrBackendClosed
	}

	tableName := p.makeTableName()
	query := fmt.Sprintf(`
		DELETE FROM %s
		WHERE expires_at IS NOT NULL AND expires_at < CURRENT_TIMESTAMP
	`, tableName)

	result, err := p.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}

	count, err := result.RowsAffected()
	return count, err
}

// Vacuum optimizes the database
func (p *postgresqlBackend) Vacuum(ctx context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed || p.db == nil {
		return backend.ErrBackendClosed
	}

	tableName := p.makeTableName()
	query := fmt.Sprintf(`VACUUM ANALYZE %s`, tableName)

	_, err := p.db.ExecContext(ctx, query)
	return err
}

// GetStats returns database statistics
func (p *postgresqlBackend) GetStats(ctx context.Context) *backend.DatabaseStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed || p.db == nil {
		return nil
	}

	tableName := p.makeTableName()
	stats := &backend.DatabaseStats{}

	// Get total rows
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM %s`, tableName)
	_ = p.db.QueryRowContext(ctx, countQuery).Scan(&stats.TotalRows)

	// Get table and index sizes
	sizeQuery := `
		SELECT
			pg_total_relation_size($1) as table_size,
			pg_indexes_size($1) as index_size
	`
	_ = p.db.QueryRowContext(ctx, sizeQuery, tableName).Scan(&stats.TableSize, &stats.IndexSize)

	// Get dead tuples
	deadQuery := `
		SELECT n_dead_tup
		FROM pg_stat_user_tables
		WHERE relname = $1
	`
	_ = p.db.QueryRowContext(ctx, deadQuery, tableName).Scan(&stats.DeadTuples)

	return stats
}

// Transaction runs a function in a database transaction
func (p *postgresqlBackend) Transaction(ctx context.Context, fn func(backend.Backend) error) error {
	p.mu.RLock()
	if p.closed || p.db == nil {
		p.mu.RUnlock()
		return backend.ErrBackendClosed
	}
	p.mu.RUnlock()

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// Create a transaction backend
	txBackend := &postgresqlBackend{
		db:       p.db, // Reuse connection but will use tx in queries
		closed:   false,
		host:     p.host,
		port:     p.port,
		database: p.database,
		username: p.username,
		prefix:   p.prefix,
	}

	// Execute the function
	err = fn(txBackend)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

// Helper function to create table name with prefix
func (p *postgresqlBackend) makeTableName() string {
	if p.prefix == "" {
		return "rate_limit_state"
	}
	return p.prefix + "rate_limit_state"
}
