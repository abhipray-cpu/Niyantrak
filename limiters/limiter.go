// Package limiters provides rate limiting implementations
package limiters

import (
	"context"
	"errors"
	"time"

	obstypes "github.com/abhipray-cpu/niyantrak/observability/types"
)

// Common error definitions used across all limiters
var (
	// ErrLimitExceeded indicates the rate limit has been exceeded
	ErrLimitExceeded = errors.New("rate limit exceeded")

	// ErrInvalidKey indicates an invalid or empty key
	ErrInvalidKey = errors.New("invalid key")

	// ErrInvalidConfig indicates invalid configuration
	ErrInvalidConfig = errors.New("invalid configuration")

	// ErrLimiterClosed indicates the limiter has been closed
	ErrLimiterClosed = errors.New("limiter closed")

	// ErrKeyNotAssigned indicates a key has not been assigned to a tier/tenant
	ErrKeyNotAssigned = errors.New("key not assigned")

	// ErrInvalidTier indicates an invalid tier
	ErrInvalidTier = errors.New("invalid tier")

	// ErrInvalidTenant indicates an invalid tenant
	ErrInvalidTenant = errors.New("invalid tenant")

	// ErrOperationNotFound indicates an operation is not defined
	ErrOperationNotFound = errors.New("operation not found")

	// ErrLimitNotFound indicates a limit is not found in composite
	ErrLimitNotFound = errors.New("limit not found")
)

// LimitResult represents the result of a rate limit check
type LimitResult struct {
	// Allowed indicates if the request is allowed
	Allowed bool

	// Remaining is the number of requests remaining in the current window
	Remaining int

	// Limit is the total limit for the window
	Limit int

	// ResetAt indicates when the limit will reset
	ResetAt time.Time

	// RetryAfter is how long to wait before retrying (if denied)
	RetryAfter time.Duration

	// Error is any error that occurred during the check
	Error error
}

// Limiter is the base interface for all rate limiters
type Limiter interface {
	// Allow checks if a single request is allowed
	Allow(ctx context.Context, key string) *LimitResult

	// AllowN checks if N requests are allowed
	AllowN(ctx context.Context, key string, n int) *LimitResult

	// Reset clears the state for a key
	Reset(ctx context.Context, key string) error

	// GetStats returns statistics for a key
	GetStats(ctx context.Context, key string) interface{}

	// Close cleans up resources
	Close() error

	// SetLimit updates the limit for a specific key
	SetLimit(ctx context.Context, key string, limit int, window time.Duration) error

	// Type returns the limiter type name
	Type() string
}

// BasicLimiter provides simple per-key rate limiting
type BasicLimiter interface {
	Limiter
}

// TierBasedLimiter provides subscription tier-based rate limiting
type TierBasedLimiter interface {
	Limiter

	// SetTierLimit configures limits for a specific tier
	SetTierLimit(ctx context.Context, tier string, limit int, window time.Duration) error

	// GetTierLimit retrieves the limit configuration for a tier
	GetTierLimit(ctx context.Context, tier string) (int, time.Duration, error)

	// AssignKeyToTier assigns a key to a specific tier
	AssignKeyToTier(ctx context.Context, key string, tier string) error

	// GetKeyTier retrieves which tier a key belongs to
	GetKeyTier(ctx context.Context, key string) (string, error)

	// ListTiers returns all configured tiers
	ListTiers(ctx context.Context) ([]string, error)
}

// TenantBasedLimiter provides multi-tenancy rate limiting
type TenantBasedLimiter interface {
	Limiter

	// SetTenantLimit configures limits for a specific tenant
	SetTenantLimit(ctx context.Context, tenantID string, limit int, window time.Duration) error

	// GetTenantLimit retrieves the limit configuration for a tenant
	GetTenantLimit(ctx context.Context, tenantID string) (int, time.Duration, error)

	// AssignKeyToTenant assigns a key to a specific tenant
	AssignKeyToTenant(ctx context.Context, key string, tenantID string) error

	// GetKeyTenant retrieves which tenant a key belongs to
	GetKeyTenant(ctx context.Context, key string) (string, error)

	// GetTenantStats returns aggregated stats for all keys in a tenant
	GetTenantStats(ctx context.Context, tenantID string) *TenantStats

	// ListTenants returns all configured tenants
	ListTenants(ctx context.Context) ([]string, error)
}

// TenantStats represents aggregated statistics for a tenant
type TenantStats struct {
	// TenantID is the unique tenant identifier
	TenantID string

	// TotalKeys is the number of unique keys for this tenant
	TotalKeys int

	// TotalRequests across all keys
	TotalRequests int64

	// AllowedCount across all keys
	AllowedCount int64

	// DeniedCount across all keys
	DeniedCount int64

	// CurrentRate is the aggregate requests per second
	CurrentRate float64

	// LastUpdated when stats were calculated
	LastUpdated time.Time
}

// CostBasedLimiter provides weighted token-based rate limiting
type CostBasedLimiter interface {
	Limiter

	// AllowWithCost checks if a request with specific cost is allowed
	// Cost represents the number of tokens consumed
	AllowWithCost(ctx context.Context, key string, cost int) *LimitResult

	// SetOperationCost defines the cost for a specific operation type
	SetOperationCost(ctx context.Context, operation string, cost int) error

	// GetOperationCost retrieves the cost for an operation
	GetOperationCost(ctx context.Context, operation string) (int, error)

	// GetRemainingBudget returns remaining tokens for a key
	GetRemainingBudget(ctx context.Context, key string) (int, error)

	// ListOperations returns all defined operations and their costs
	ListOperations(ctx context.Context) (map[string]int, error)
}

// CompositeLimiter provides multiple simultaneous rate limits
type CompositeLimiter interface {
	Limiter

	// AddLimit adds a new limit to the composite
	// All limits must be satisfied for a request to be allowed
	AddLimit(ctx context.Context, name string, limit int, window time.Duration) error

	// RemoveLimit removes a limit from the composite
	RemoveLimit(ctx context.Context, name string) error

	// GetLimits returns all configured limits
	GetLimits(ctx context.Context) ([]LimitConfig, error)

	// CheckAll checks all limits and returns which ones are exceeded
	CheckAll(ctx context.Context, key string) ([]LimitStatus, error)

	// GetHierarchy returns the hierarchy/relationship between limits
	GetHierarchy(ctx context.Context) *LimitHierarchy
}

// LimitConfig represents a single limit configuration
type LimitConfig struct {
	// Name is a unique identifier for this limit
	Name string

	// Limit is the maximum number of requests
	Limit int

	// Window is the time window for the limit
	Window time.Duration

	// Algorithm is the rate limiting algorithm to use
	Algorithm string

	// Priority for hierarchical composite limits (lower = higher priority)
	Priority int

	// Description of what this limit controls
	Description string
}

// LimitStatus represents the status of a single limit check
type LimitStatus struct {
	// Name of the limit
	Name string

	// Allowed indicates if this specific limit allows the request
	Allowed bool

	// Remaining requests in this limit
	Remaining int

	// Limit is the total limit
	Limit int

	// ResetAt indicates when this limit resets
	ResetAt time.Time

	// Progress is percentage of limit used (0-100)
	Progress int
}

// LimitHierarchy represents relationships between limits in composite limiters
type LimitHierarchy struct {
	// Name of the limiter
	Name string

	// Limits ordered by priority
	Limits []LimitConfig

	// Relationships between limits (e.g., "minute limit < hour limit")
	Relationships []string

	// ConflictingLimits indicates if any limits conflict
	ConflictingLimits bool
}

// BasicConfig represents configuration for BasicLimiter
type BasicConfig struct {
	// AlgorithmName is the name of the algorithm to use
	AlgorithmName string

	// AlgorithmConfig is the configuration for the algorithm
	AlgorithmConfig interface{}

	// DefaultLimit is the default limit for new keys
	DefaultLimit int

	// DefaultWindow is the default window for new keys
	DefaultWindow time.Duration

	// KeyTTL is how long to keep state for a key without access
	// Set to 0 for no expiration
	KeyTTL time.Duration

	// Observability contains optional observability configuration
	// If not set, NoOp implementations are used (zero overhead)
	Observability ObservabilityConfig

	// DynamicLimits contains optional dynamic limit management
	// If not set, dynamic limits are disabled
	DynamicLimits DynamicLimitConfig

	// Failover contains optional failover mechanism
	// If not set, failover is disabled
	Failover FailoverConfig
}

// TierLimit defines rate limit settings for a tier.
type TierLimit struct {
	Limit  int
	Window time.Duration
}

// TierConfig represents configuration for TierBasedLimiter
type TierConfig struct {
	BasicConfig

	// Tiers is the initial tier configuration
	// Map of tier name to rate limit settings
	Tiers map[string]TierLimit

	// DefaultTier is the tier assigned to keys without explicit tier
	DefaultTier string

	// PersistMappings, when true, stores key→tier assignments in the
	// backend so that all distributed instances share them. Default false
	// keeps mappings in-memory only (single-instance mode).
	PersistMappings bool
}

// TenantLimit defines rate limit settings for a tenant.
type TenantLimit struct {
	Limit  int
	Window time.Duration
}

// TenantConfig represents configuration for TenantBasedLimiter
type TenantConfig struct {
	BasicConfig

	// Tenants is the initial tenant configuration
	// Map of tenant ID to rate limit settings
	Tenants map[string]TenantLimit

	// DefaultTenant is the tenant assigned to keys without explicit tenant
	DefaultTenant string

	// PersistMappings, when true, stores key→tenant assignments in the
	// backend so that all distributed instances share them. Default false
	// keeps mappings in-memory only (single-instance mode).
	PersistMappings bool
}

// CostConfig represents configuration for CostBasedLimiter
type CostConfig struct {
	BasicConfig

	// Operations is the initial operation cost configuration
	// Map of operation name to cost
	Operations map[string]int

	// DefaultCost is the cost for operations not in the Operations map
	DefaultCost int
}

// CompositeConfig represents configuration for CompositeLimiter
type CompositeConfig struct {
	// Limits are the individual limits to combine
	Limits []LimitConfig

	// Name of the composite limiter
	Name string

	// Observability contains optional observability configuration
	// If not set, NoOp implementations are used (zero overhead)
	Observability ObservabilityConfig

	// DynamicLimits contains optional dynamic limit management
	// If not set, dynamic limits are disabled
	DynamicLimits DynamicLimitConfig

	// Failover contains optional failover mechanism
	// If not set, failover is disabled
	Failover FailoverConfig
}

// ObservabilityConfig contains optional observability integrations.
// All fields are optional and default to no-op implementations for zero overhead.
type ObservabilityConfig struct {
	// Logger for rate limit events (optional)
	// Default: NoOpLogger (zero overhead)
	Logger obstypes.Logger

	// Metrics collector for rate limit decisions (optional)
	// Default: NoOpMetrics (zero overhead)
	Metrics obstypes.Metrics

	// Tracer for distributed tracing (optional)
	// Default: NoOpTracer (zero overhead)
	Tracer obstypes.Tracer

	// EnableTracing controls whether spans are created for decisions
	// Only used if Tracer is not nil
	// Default: false
	EnableTracing bool

	// LogLevel controls minimum log level to emit
	// Valid values: "debug", "info", "warn", "error"
	// Default: "info"
	LogLevel string
}

// DynamicLimitConfig contains optional dynamic limit management configuration.
// All fields are optional and dynamic limits are disabled if not set.
type DynamicLimitConfig struct {
	// Manager provides dynamic limit management (optional)
	// If not set, dynamic limits are disabled
	// Should be *features.DynamicLimitManager
	Manager interface{}

	// EnableDynamicLimits controls whether dynamic limits are enabled
	// Only used if Manager is not nil
	// Default: false
	EnableDynamicLimits bool
}

// FailoverConfig contains failover mechanism configuration
type FailoverConfig struct {
	// Handler provides failover management (optional)
	// If not set, failover is disabled
	// Should be *features.FailoverHandler
	Handler interface{}

	// EnableFailover controls whether failover mechanism is enabled
	// Only used if Handler is not nil
	// Default: false
	EnableFailover bool
}
