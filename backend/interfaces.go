package backend

import (
	"context"
	"errors"
	"time"
)

// Common error definitions used across all backends
var (
	// ErrKeyNotFound indicates the requested key does not exist
	ErrKeyNotFound = errors.New("key not found")

	// ErrKeyExpired indicates the key has expired
	ErrKeyExpired = errors.New("key expired")

	// ErrBackendClosed indicates the backend connection is closed
	ErrBackendClosed = errors.New("backend closed")
)

// Backend represents a storage backend for rate limiting state
type Backend interface {
	// Get retrieves the current state for a key
	// Returns ErrKeyNotFound if the key doesn't exist
	Get(ctx context.Context, key string) (interface{}, error)

	// Set updates the state for a key with optional TTL (time-to-live)
	// A TTL of 0 means no expiration
	Set(ctx context.Context, key string, state interface{}, ttl time.Duration) error

	// IncrementAndGet atomically increments the value for a key and returns new value
	// Useful for fixed window counters and other atomic operations
	IncrementAndGet(ctx context.Context, key string, ttl time.Duration) (int64, error)

	// Delete removes a key from the backend
	Delete(ctx context.Context, key string) error

	// Close cleans up resources and should be called before shutdown
	Close() error

	// Ping checks if backend is healthy
	Ping(ctx context.Context) error

	// Type returns the backend type name
	Type() string
}

// UpdateFunc is the callback passed to AtomicBackend.Update.
// It receives the current state (nil if key doesn't exist) and returns:
//   - newState: the state to persist back
//   - result:   an arbitrary value returned to the caller (e.g. algorithm result)
//   - err:      if non-nil, the update is aborted and no write occurs
type UpdateFunc func(currentState interface{}) (newState interface{}, result interface{}, err error)

// AtomicBackend is an optional interface that backends may implement to
// provide atomic read-modify-write operations. When available, limiters
// use Update instead of separate Get→compute→Set calls, eliminating the
// race window between read and write.
type AtomicBackend interface {
	// Update atomically reads the current state for key, passes it to fn,
	// and writes back the returned newState. The entire operation is
	// serialised per-key so no concurrent Update for the same key can
	// interleave.
	//
	// If the key does not exist, fn is called with currentState == nil.
	// ttl is applied to the written key (0 means no expiration).
	//
	// The result value returned by fn is forwarded to the caller unchanged.
	Update(ctx context.Context, key string, ttl time.Duration, fn UpdateFunc) (result interface{}, err error)
}

// AtomicUpdate is a helper that uses AtomicBackend.Update when the backend
// supports it, otherwise falls back to a non-atomic Get→fn→Set sequence.
func AtomicUpdate(ctx context.Context, b Backend, key string, ttl time.Duration, fn UpdateFunc) (interface{}, error) {
	if ab, ok := b.(AtomicBackend); ok {
		return ab.Update(ctx, key, ttl, fn)
	}

	// Fallback: non-atomic Get → fn → Set
	state, err := b.Get(ctx, key)
	if err != nil && err != ErrKeyNotFound {
		return nil, err
	}
	if err == ErrKeyNotFound {
		state = nil
	}

	newState, result, fnErr := fn(state)
	if fnErr != nil {
		return nil, fnErr
	}

	if err := b.Set(ctx, key, newState, ttl); err != nil {
		return nil, err
	}
	return result, nil
}

// MemoryBackend is an in-memory storage backend
type MemoryBackend interface {
	Backend

	// GetSize returns number of stored keys
	GetSize(ctx context.Context) (int, error)

	// Clear removes all keys
	Clear(ctx context.Context) error

	// GetMemoryUsage returns approximate memory usage in bytes
	GetMemoryUsage(ctx context.Context) (int64, error)

	// SetMaxSize sets maximum number of keys to store
	// Returns error if current size exceeds new max
	SetMaxSize(ctx context.Context, maxSize int) error
}

// RedisBackend is a Redis-based distributed backend
type RedisBackend interface {
	Backend

	// GetConnection returns the underlying Redis connection info
	GetConnection(ctx context.Context) *ConnectionInfo

	// CheckCluster checks if connected to Redis cluster
	CheckCluster(ctx context.Context) (bool, error)

	// GetReplication checks Redis replication status
	GetReplication(ctx context.Context) *ReplicationStatus

	// Flush flushes all data from current database
	Flush(ctx context.Context) error

	// Scan scans keys matching pattern
	Scan(ctx context.Context, pattern string, count int) ([]string, error)
}

// PostgreSQLBackend is a PostgreSQL-based persistent backend
type PostgreSQLBackend interface {
	Backend

	// CreateTable creates the rate limit state table if it doesn't exist
	CreateTable(ctx context.Context) error

	// Migrate runs any pending migrations
	Migrate(ctx context.Context) error

	// GetSchema returns the current database schema
	GetSchema(ctx context.Context) string

	// CleanupExpired removes expired entries
	CleanupExpired(ctx context.Context) (int64, error)

	// Vacuum optimizes the database
	Vacuum(ctx context.Context) error

	// GetStats returns database statistics
	GetStats(ctx context.Context) *DatabaseStats

	// Transaction runs a function in a database transaction
	Transaction(ctx context.Context, fn func(Backend) error) error
}

// CustomBackend is a user-defined storage backend
type CustomBackend interface {
	Backend

	// GetMetadata returns custom metadata about the backend
	GetMetadata(ctx context.Context) map[string]interface{}

	// Execute executes a custom operation
	Execute(ctx context.Context, operation string, args map[string]interface{}) (interface{}, error)
}

// ConnectionInfo represents backend connection information
type ConnectionInfo struct {
	// Host of the backend
	Host string

	// Port of the backend
	Port int

	// Database name
	Database string

	// Username if applicable
	Username string

	// IsConnected indicates connection status
	IsConnected bool

	// ConnectionTime when connection was established
	ConnectionTime time.Time

	// Version of the backend server
	Version string
}

// ReplicationStatus represents Redis replication status
type ReplicationStatus struct {
	// Role of the instance (master, slave)
	Role string

	// ConnectedSlaves number of connected slaves
	ConnectedSlaves int

	// Offset of replication
	Offset int64

	// BacklogSize of replication backlog
	BacklogSize int64

	// Status string representation
	Status string
}

// DatabaseStats represents PostgreSQL database statistics
type DatabaseStats struct {
	// TotalRows total number of rows in rate limit table
	TotalRows int64

	// IndexSize size of indices in bytes
	IndexSize int64

	// TableSize size of table in bytes
	TableSize int64

	// DeadTuples number of dead tuples
	DeadTuples int64

	// LastVacuum when table was last vacuumed
	LastVacuum time.Time

	// LastAnalyze when table was last analyzed
	LastAnalyze time.Time
}

// BackendFactory creates backend instances
type BackendFactory interface {
	// Create creates a new backend instance with the given config
	Create(config interface{}) (Backend, error)

	// Type returns the backend type this factory creates
	Type() string
}

// BackendRegistry manages backend factories
type BackendRegistry interface {
	// Register registers a backend factory
	Register(factory BackendFactory) error

	// Get retrieves a registered backend factory by type
	Get(backendType string) (BackendFactory, error)

	// List returns all registered backend types
	List() []string

	// Create creates a new backend instance
	Create(backendType string, config interface{}) (Backend, error)
}
