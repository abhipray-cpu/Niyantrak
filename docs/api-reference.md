# Niyantrak API Reference

---
## `github.com/abhipray-cpu/niyantrak`

package niyantrak // import "github.com/abhipray-cpu/niyantrak"

Package niyantrak provides a comprehensive rate limiting library for Go
applications.

Niyantrak (Sanskrit: नियंत्रक - "Controller") offers multiple rate limiting
algorithms, pluggable storage backends, middleware support, and built-in
observability.

# Core Features

- 5 Algorithms: Token Bucket, Leaky Bucket, Fixed Window, Sliding Window,
GCRA - Multiple Backends: In-memory, Redis, PostgreSQL, custom - Middleware:
HTTP and gRPC support with standard rate limit headers - Observability:
Prometheus metrics and structured logging - Production-Ready: Thread-safe,
low latency, comprehensive testing

# Quick Start

Create a basic rate limiter:

    import (
    	"context"
    	"log"
    	"github.com/abhipray-cpu/niyantrak"
    	"github.com/abhipray-cpu/niyantrak/limiters/basic"
    	"github.com/abhipray-cpu/niyantrak/backend/memory"
    	"github.com/abhipray-cpu/niyantrak/algorithm"
    )

    func main() {
    	// Create memory backend
    	backend := memory.NewMemoryBackend()
    	defer backend.Close()

    	// Create algorithm
    	algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
    		Capacity:     100,
    		RefillRate:   10,
    		RefillPeriod: time.Second,
    	})

    	// Create limiter
    	limiter, err := basic.NewBasicLimiter(algo, backend, limiters.BasicConfig{
    		DefaultLimit:  100,
    		DefaultWindow: time.Second,
    	})
    	if err != nil {
    		log.Fatal(err)
    	}
    	defer limiter.Close()

    	// Check rate limit
    	ctx := context.Background()
    	result := limiter.Allow(ctx, "user:123")
    	if !result.Allowed {
    		log.Printf("Rate limited! Retry after %v", result.RetryAfter)
    		return
    	}

    	log.Printf("Request allowed. Remaining: %d/%d", result.Remaining, result.Limit)
    }

# Architecture

The library uses a layered architecture:

    ┌─────────────────────────────────┐
    │     Application Code            │
    ├─────────────────────────────────┤
    │   HTTP/gRPC Middleware          │
    ├─────────────────────────────────┤
    │   Limiter Interface             │
    │ (Basic/Tier/Tenant/Cost/Composite) │
    ├─────────────────────────────────┤
    │   Algorithm Layer               │
    │ (Token Bucket, Leaky Bucket, etc) │
    ├─────────────────────────────────┤
    │   Backend Storage               │
    │ (Memory, Redis, PostgreSQL)     │
    └─────────────────────────────────┘

# Algorithms

- TokenBucket: Allows burst traffic while maintaining average rate -
LeakyBucket: Smooths traffic to a constant rate - FixedWindow: Resets counter at
fixed time boundaries - SlidingWindow: More accurate with sliding time windows -
GCRA: Generic Cell Rate Algorithm for precise rate limiting

# Backends

- Memory: In-memory storage (single-instance, no persistence) - Redis:
Distributed storage for multi-instance deployments - PostgreSQL: Persistent
storage with audit trail - Custom: Implement Backend interface for custom
storage

# Limiter Types

- BasicLimiter: Simple per-key rate limiting - TierBasedLimiter: Subscription
tier-based limits - TenantBasedLimiter: Multi-tenancy rate limiting -
CostBasedLimiter: Operation cost-based limiting - CompositeLimiter: Combine
multiple limiters

VARIABLES

var (
	// ErrLimitExceeded indicates the rate limit has been exceeded
	ErrLimitExceeded = limiters.ErrLimitExceeded

	// ErrInvalidKey indicates an invalid or empty key
	ErrInvalidKey = limiters.ErrInvalidKey

	// ErrInvalidConfig indicates invalid configuration
	ErrInvalidConfig = limiters.ErrInvalidConfig

	// ErrLimiterClosed indicates the limiter has been closed
	ErrLimiterClosed = limiters.ErrLimiterClosed

	// ErrKeyNotAssigned indicates a key has not been assigned to a tier/tenant
	ErrKeyNotAssigned = limiters.ErrKeyNotAssigned

	// ErrInvalidTier indicates an invalid tier
	ErrInvalidTier = limiters.ErrInvalidTier

	// ErrInvalidTenant indicates an invalid tenant
	ErrInvalidTenant = limiters.ErrInvalidTenant

	// ErrOperationNotFound indicates an operation is not defined
	ErrOperationNotFound = limiters.ErrOperationNotFound

	// ErrLimitNotFound indicates a limit is not found in composite
	ErrLimitNotFound = limiters.ErrLimitNotFound

	// ErrKeyNotFound indicates the requested key does not exist
	ErrKeyNotFound = backend.ErrKeyNotFound

	// ErrBackendClosed indicates the backend connection is closed
	ErrBackendClosed = backend.ErrBackendClosed
)

TYPES

type Algorithm = algorithm.Algorithm
    Algorithm represents a rate limiting algorithm implementation

type AlgorithmType int
    AlgorithmType identifies the rate limiting algorithm to use.

const (
	// TokenBucket implements the token bucket algorithm.
	// Recommended for general-purpose rate limiting.
	TokenBucket AlgorithmType = iota

	// LeakyBucket implements the leaky bucket algorithm.
	// Smooths out bursts by processing requests at a fixed rate.
	LeakyBucket

	// FixedWindow implements the fixed window algorithm.
	// Simplest algorithm — counts requests in fixed time windows.
	FixedWindow

	// SlidingWindow implements the sliding window algorithm.
	// More accurate than fixed window, avoids boundary spikes.
	SlidingWindow

	// GCRA implements the Generic Cell Rate Algorithm.
	// Precise, memory-efficient algorithm used in telecom.
	GCRA
)
type Backend = backend.Backend
    Backend represents a storage backend for rate limiting state

type BasicConfig = limiters.BasicConfig
    BasicConfig holds configuration for creating a BasicLimiter

type BasicLimiter = limiters.BasicLimiter
    BasicLimiter provides simple per-key rate limiting

func New(opts ...Option) (BasicLimiter, error)
    New creates a BasicLimiter using functional options.

    This is the recommended entry point for simple rate limiting. For
    advanced scenarios (tier-based, tenant-based, composite), use the limiter
    sub-packages directly.

        limiter, err := niyantrak.New(
            niyantrak.WithAlgorithm(niyantrak.TokenBucket),
            niyantrak.WithTokenBucketConfig(niyantrak.TokenBucketConfig{
                Capacity:     100,
                RefillRate:   100,
                RefillPeriod: time.Minute,
            }),
            niyantrak.WithMemoryBackend(),
        )

type CompositeConfig = limiters.CompositeConfig
    CompositeConfig holds configuration for creating a CompositeLimiter

type CompositeLimiter = limiters.CompositeLimiter
    CompositeLimiter combines multiple limiters for complex scenarios

func NewComposite(cfg limiters.CompositeConfig, opts ...Option) (CompositeLimiter, error)
    NewComposite creates a CompositeLimiter using the builder's algorithm
    options and a CompositeConfig.

type CostBasedLimiter = limiters.CostBasedLimiter
    CostBasedLimiter provides operation cost-based rate limiting

func NewCostBased(cfg limiters.CostConfig, opts ...Option) (CostBasedLimiter, error)
    NewCostBased creates a CostBasedLimiter using the builder's algorithm
    options and a CostConfig.

type CostConfig = limiters.CostConfig
    CostConfig holds configuration for creating a CostBasedLimiter

type CustomBackend = backend.CustomBackend
    CustomBackend allows implementing custom storage backends

type CustomMiddleware = middleware.CustomMiddleware
    CustomMiddleware provides a base interface for custom middleware

type DynamicLimitConfig = limiters.DynamicLimitConfig
    DynamicLimitConfig holds dynamic limit configuration

type DynamicLimitController = features.DynamicLimitController
    DynamicLimitController allows runtime adjustment of rate limits

type FailoverConfig = limiters.FailoverConfig
    FailoverConfig holds failover configuration

type FailoverHandler = features.FailoverHandler
    FailoverHandler manages graceful degradation when backends fail

type FailoverStatus = features.FailoverStatus
    FailoverStatus represents current failover state

type FixedWindowConfig = algorithm.FixedWindowConfig
    FixedWindowConfig configures the Fixed Window algorithm

type GCRAConfig = algorithm.GCRAConfig
    GCRAConfig configures the GCRA algorithm

type GRPCKeyExtractor = middleware.GRPCKeyExtractor
    GRPCKeyExtractor extracts a rate limit key from gRPC context

type GRPCMiddleware = middleware.GRPCMiddleware
    GRPCMiddleware provides gRPC integration for rate limiting

type GRPCOptions = middleware.GRPCOptions
    GRPCOptions contains gRPC middleware options

type HTTPMiddleware = middleware.HTTPMiddleware
    HTTPMiddleware provides HTTP integration for rate limiting

type HTTPOptions = middleware.HTTPOptions
    HTTPOptions contains HTTP middleware options

type HeaderFormatter = middleware.HeaderFormatter
    HeaderFormatter formats rate limit response headers

type HeaderNames = middleware.HeaderNames
    HeaderNames specifies custom header names

type KeyExtractor = middleware.KeyExtractor
    KeyExtractor extracts a rate limit key from an HTTP request

type KeyExtractorBuilder = middleware.KeyExtractorBuilder
    KeyExtractorBuilder helps build custom key extractors

type LeakyBucketConfig = algorithm.LeakyBucketConfig
    LeakyBucketConfig configures the Leaky Bucket algorithm

type LimitConfig = limiters.LimitConfig
    LimitConfig represents a single rate limit rule

type LimitResult = limiters.LimitResult
    LimitResult represents the result of a rate limit check

type Limiter = limiters.Limiter
    Limiter is the base interface for all rate limiters

type MemoryBackend = backend.MemoryBackend
    MemoryBackend is an in-memory storage backend

type ObservabilityConfig = limiters.ObservabilityConfig
    ObservabilityConfig holds observability settings

type Option func(*options)
    Option configures the rate limiter created by New.

func WithAlgorithm(t AlgorithmType) Option
    WithAlgorithm selects the rate limiting algorithm.

func WithBackend(b Backend) Option
    WithBackend uses a custom or pre-configured storage backend. Use this to
    pass a Redis, PostgreSQL, or custom backend.

func WithFixedWindowConfig(cfg FixedWindowConfig) Option
    WithFixedWindowConfig sets the configuration for the Fixed Window algorithm.

func WithGCRAConfig(cfg GCRAConfig) Option
    WithGCRAConfig sets the configuration for the GCRA algorithm.

func WithKeyTTL(d time.Duration) Option
    WithKeyTTL sets how long rate limit state is kept per key.

func WithLeakyBucketConfig(cfg LeakyBucketConfig) Option
    WithLeakyBucketConfig sets the configuration for the Leaky Bucket algorithm.

func WithLimit(limit int) Option
    WithLimit sets the default rate limit (requests per window).

func WithMemoryBackend() Option
    WithMemoryBackend uses an in-memory storage backend. Best for
    single-instance applications and development.

func WithSlidingWindowConfig(cfg SlidingWindowConfig) Option
    WithSlidingWindowConfig sets the configuration for the Sliding Window
    algorithm.

func WithTokenBucketConfig(cfg TokenBucketConfig) Option
    WithTokenBucketConfig sets the configuration for the Token Bucket algorithm.

func WithWindow(d time.Duration) Option
    WithWindow sets the default time window for rate limiting.

type PostgreSQLBackend = backend.PostgreSQLBackend
    PostgreSQLBackend is a PostgreSQL-based persistent backend

type RateLimitHandler = middleware.RateLimitHandler
    RateLimitHandler handles rate limit exceeded responses

type RedisBackend = backend.RedisBackend
    RedisBackend is a Redis-based distributed backend

type SlidingWindowConfig = algorithm.SlidingWindowConfig
    SlidingWindowConfig configures the Sliding Window algorithm

type TenantBasedLimiter = limiters.TenantBasedLimiter
    TenantBasedLimiter provides multi-tenancy rate limiting

func NewTenantBased(cfg limiters.TenantConfig, opts ...Option) (TenantBasedLimiter, error)
    NewTenantBased creates a TenantBasedLimiter using the builder's algorithm
    options and a TenantConfig.

type TenantConfig = limiters.TenantConfig
    TenantConfig holds configuration for creating a TenantBasedLimiter

type TenantStats = limiters.TenantStats
    TenantStats represents aggregated statistics for a tenant

type TierBasedLimiter = limiters.TierBasedLimiter
    TierBasedLimiter provides subscription tier-based rate limiting

func NewTierBased(cfg limiters.TierConfig, opts ...Option) (TierBasedLimiter, error)
    NewTierBased creates a TierBasedLimiter using the builder's algorithm
    options and a TierConfig.

        limiter, err := niyantrak.NewTierBased(
            niyantrak.TierConfig{ ... },
            niyantrak.WithAlgorithm(niyantrak.TokenBucket),
            niyantrak.WithMemoryBackend(),
        )

type TierConfig = limiters.TierConfig
    TierConfig holds configuration for creating a TierBasedLimiter

type TokenBucketConfig = algorithm.TokenBucketConfig
    TokenBucketConfig configures the Token Bucket algorithm


---
## `github.com/abhipray-cpu/niyantrak/algorithm`

package algorithm // import "github.com/abhipray-cpu/niyantrak/algorithm"

Package algorithm provides rate limiting algorithm interfaces

TYPES

type Algorithm interface {
	// Allow checks if a request is allowed based on current state
	// Returns updated state and result
	Allow(ctx context.Context, state interface{}, cost int) (interface{}, interface{}, error)

	// Reset resets the state for this algorithm
	Reset(ctx context.Context) (interface{}, error)

	// GetStats calculates current statistics from state
	GetStats(ctx context.Context, state interface{}) interface{}

	// ValidateConfig validates algorithm-specific configuration
	ValidateConfig(config interface{}) error

	// Name returns the algorithm name
	Name() string

	// Description returns a human-readable description
	Description() string
}
    Algorithm represents a rate limiting algorithm implementation

func NewFixedWindow(config FixedWindowConfig) Algorithm
    NewFixedWindow creates a new Fixed Window algorithm instance

func NewFixedWindowWithClock(config FixedWindowConfig, clock Clock) Algorithm
    NewFixedWindowWithClock creates a new Fixed Window algorithm instance with a
    custom clock. If clock is nil, time.Now is used.

func NewGCRA(config GCRAConfig) Algorithm
    NewGCRA creates a new GCRA algorithm instance

func NewGCRAWithClock(config GCRAConfig, clock Clock) Algorithm
    NewGCRAWithClock creates a new GCRA algorithm instance with a custom clock.
    If clock is nil, time.Now is used.

func NewLeakyBucket(config LeakyBucketConfig) Algorithm
    NewLeakyBucket creates a new Leaky Bucket algorithm instance

func NewLeakyBucketWithClock(config LeakyBucketConfig, clock Clock) Algorithm
    NewLeakyBucketWithClock creates a new Leaky Bucket algorithm instance with a
    custom clock. If clock is nil, time.Now is used.

func NewSlidingWindow(config SlidingWindowConfig) Algorithm
    NewSlidingWindow creates a new Sliding Window algorithm instance

func NewSlidingWindowWithClock(config SlidingWindowConfig, clock Clock) Algorithm
    NewSlidingWindowWithClock creates a new Sliding Window algorithm instance
    with a custom clock. If clock is nil, time.Now is used.

func NewTokenBucket(config TokenBucketConfig) Algorithm
    NewTokenBucket creates a new Token Bucket algorithm instance

func NewTokenBucketWithClock(config TokenBucketConfig, clock Clock) Algorithm
    NewTokenBucketWithClock creates a new Token Bucket algorithm instance with a
    custom clock. If clock is nil, time.Now is used.

type AlgorithmFactory interface {
	// Create creates a new algorithm instance with the given config
	Create(config interface{}) (Algorithm, error)

	// Type returns the algorithm type this factory creates
	Type() string
}
    AlgorithmFactory creates algorithm instances

type AlgorithmRegistry interface {
	// Register registers an algorithm factory
	Register(factory AlgorithmFactory) error

	// Get retrieves a registered algorithm factory by type
	Get(algorithmType string) (AlgorithmFactory, error)

	// List returns all registered algorithm types
	List() []string

	// Create creates a new algorithm instance
	Create(algorithmType string, config interface{}) (Algorithm, error)
}
    AlgorithmRegistry manages algorithm factories

type AlgorithmResult interface {
	IsAllowed() bool
	GetRemaining() interface{}
	GetResetTime() interface{}
}
    AlgorithmResult represents a generic result from any algorithm

type AlgorithmStats interface {
	GetCurrentState() interface{}
	GetConfiguration() interface{}
}
    AlgorithmStats represents generic statistics from any algorithm

type Clock func() time.Time
    Clock is a function that returns the current time. Algorithms accept an
    optional Clock for deterministic testing. When nil, time.Now is used.

type FixedWindowConfig struct {
	// Limit is the maximum number of requests per window
	Limit int

	// Window is the duration of each time window
	Window time.Duration

	// WindowAlignment aligns windows to clock boundaries (minute, hour, etc.)
	WindowAlignment string
}
    FixedWindowConfig configures the Fixed Window algorithm

type FixedWindowResult struct {
	// Allowed indicates if the request is allowed
	Allowed bool

	// RequestCount is the current request count in this window
	RequestCount int

	// Limit is the maximum requests allowed per window
	Limit int

	// Remaining is the number of requests remaining in this window
	Remaining int

	// ResetTime is when the current window ends and counter resets
	ResetTime time.Time

	// RetryAfter is how long to wait before retrying (if denied)
	RetryAfter time.Duration
}
    FixedWindowResult represents the result of a fixed window check

type FixedWindowState struct {
	// WindowStart is the start time of the current window
	WindowStart time.Time

	// RequestCount is the number of requests in the current window
	RequestCount int
}
    FixedWindowState represents the state of a fixed window

type FixedWindowStats struct {
	// CurrentRequestCount is the number of requests in current window
	CurrentRequestCount int

	// Limit is the maximum requests allowed per window
	Limit int

	// WindowSize is the duration of each window
	WindowSize time.Duration

	// WindowStart is when the current window started
	WindowStart time.Time

	// WindowEnd is when the current window ends
	WindowEnd time.Time

	// TimeInWindow is how long we've been in current window
	TimeInWindow time.Duration

	// TimeUntilReset is time until the window resets
	TimeUntilReset time.Duration
}
    FixedWindowStats represents statistics for the fixed window

type GCRAConfig struct {
	// Limit is the maximum number of requests per period
	Limit int

	// Period is the time period for the limit
	Period time.Duration

	// BurstSize allows temporary bursts above the rate
	BurstSize int

	// MaxBurst is the maximum burst allowed
	MaxBurst int
}
    GCRAConfig configures the GCRA (Generic Cell Rate Algorithm)

type GCRAResult struct {
	// Allowed indicates if the request is allowed
	Allowed bool

	// TAT is the theoretical arrival time after this request
	TAT time.Time

	// TimeToAct is when the request can be processed (for conforming requests)
	TimeToAct time.Time

	// RetryAfter is how long to wait before retrying (if denied)
	RetryAfter time.Duration

	// Limit is the emission interval (time between requests)
	Limit time.Duration
}
    GCRAResult represents the result of a GCRA check

type GCRAState struct {
	// TAT (Theoretical Arrival Time) is the virtual scheduled time
	TAT time.Time

	// LastUpdate is when the state was last updated
	LastUpdate time.Time
}
    GCRAState represents the state of a GCRA limiter

type GCRAStats struct {
	// TAT is the current theoretical arrival time
	TAT time.Time

	// Period is the time between allowed requests
	Period time.Duration

	// BurstSize is the maximum burst size
	BurstSize int

	// DelayTolerance is the maximum delay variation tolerance
	DelayTolerance time.Duration

	// NextAllowedTime is when the next request will be allowed
	NextAllowedTime time.Time

	// IsThrottled indicates if currently in throttled state
	IsThrottled bool
}
    GCRAStats represents statistics for GCRA

type LeakyBucketConfig struct {
	// Capacity is the maximum number of requests in the queue
	Capacity int

	// LeakRate is how many requests leak out per LeakPeriod
	LeakRate int

	// LeakPeriod is the time period for leaking requests
	LeakPeriod time.Duration

	// QueueBehavior determines behavior when queue is full
	// "drop" = drop new requests, "reject" = return error
	QueueBehavior string
}
    LeakyBucketConfig configures the Leaky Bucket algorithm

type LeakyBucketResult struct {
	// Allowed indicates if the request is allowed
	Allowed bool

	// QueueSize is the current queue size after this request
	QueueSize int

	// QueueCapacity is the maximum queue capacity
	QueueCapacity int

	// ResetTime is when the queue will be empty
	ResetTime time.Time

	// RetryAfter is how long to wait before retrying (if denied)
	RetryAfter time.Duration
}
    LeakyBucketResult represents the result of a leaky bucket check

type LeakyBucketState struct {
	// QueueSize is the current number of requests in the queue
	QueueSize int

	// LastLeakTime is when requests were last leaked
	LastLeakTime time.Time
}
    LeakyBucketState represents the state of a leaky bucket

type LeakyBucketStats struct {
	// CurrentQueueSize is the current number of requests in queue
	CurrentQueueSize int

	// Capacity is the maximum queue capacity
	Capacity int

	// LeakRate is requests processed per leak period
	LeakRate int

	// LeakPeriod is the time between leak operations
	LeakPeriod time.Duration

	// LastLeakTime is when requests were last leaked
	LastLeakTime time.Time

	// NextLeakTime is when requests will next be leaked
	NextLeakTime time.Time

	// EstimatedWaitTime is estimated time for queue to empty
	EstimatedWaitTime time.Duration
}
    LeakyBucketStats represents statistics for the leaky bucket

type SlidingWindowConfig struct {
	// Limit is the maximum number of requests per window
	Limit int

	// Window is the duration of the sliding window
	Window time.Duration

	// Precision is how granular the sliding window is
	// Lower precision = less memory, slightly less accurate
	Precision time.Duration

	// BucketCount is number of buckets used internally
	BucketCount int
}
    SlidingWindowConfig configures the Sliding Window algorithm

type SlidingWindowResult struct {
	// Allowed indicates if the request is allowed
	Allowed bool

	// RequestCount is the estimated number of requests in the sliding window
	RequestCount int

	// Limit is the maximum requests allowed per window
	Limit int

	// Remaining is the number of requests remaining
	Remaining int

	// OldestTimestamp is the start of the effective window (now - window)
	OldestTimestamp time.Time

	// RetryAfter is how long to wait before retrying (if denied)
	RetryAfter time.Duration
}
    SlidingWindowResult represents the result of a sliding window check

type SlidingWindowState struct {
	// CurrentCount is the number of requests in the current fixed window
	CurrentCount int

	// PreviousCount is the number of requests in the previous fixed window
	PreviousCount int

	// CurrentWindowStart is the start time of the current fixed window
	CurrentWindowStart time.Time

	// Timestamps is kept for backward compatibility with serialized state.
	// New code no longer writes to this field. On read, if CurrentWindowStart
	// is zero and Timestamps is non-empty, the state is migrated automatically.
	Timestamps []time.Time `json:"Timestamps,omitempty"`

	// LastCleanup is kept for backward compatibility with serialized state.
	LastCleanup time.Time `json:"LastCleanup,omitempty"`
}
    SlidingWindowState represents the state of a sliding window using two-window
    counter approximation. This uses O(1) memory instead of O(n) per key.

type SlidingWindowStats struct {
	// CurrentRequestCount is the estimated number of requests in the sliding window
	CurrentRequestCount int

	// Limit is the maximum requests allowed per window
	Limit int

	// Window is the duration of the sliding window
	Window time.Duration

	// OldestTimestamp is the start of the effective window
	OldestTimestamp time.Time

	// NewestTimestamp is now (latest possible request time)
	NewestTimestamp time.Time

	// WindowUtilization is the percentage of the limit used (0-100)
	WindowUtilization float64
}
    SlidingWindowStats represents statistics for the sliding window

type TokenBucketConfig struct {
	// Capacity is the maximum number of tokens in the bucket
	Capacity int

	// RefillRate is how many tokens are added per RefillPeriod
	RefillRate int

	// RefillPeriod is the time period for refilling tokens
	RefillPeriod time.Duration

	// InitialTokens is the number of tokens when bucket is created
	// If 0, starts with Capacity tokens
	InitialTokens int
}
    TokenBucketConfig configures the Token Bucket algorithm

type TokenBucketResult struct {
	// Allowed indicates if the request is allowed
	Allowed bool

	// RemainingTokens is the number of tokens left after this request
	RemainingTokens float64

	// ResetTime is when the bucket will have tokens again
	ResetTime time.Time

	// RetryAfter is how long to wait before retrying (if denied)
	RetryAfter time.Duration
}
    TokenBucketResult represents the result of a token bucket check

type TokenBucketState struct {
	// Tokens is the current number of tokens in the bucket
	Tokens float64

	// LastRefillTime is when tokens were last refilled
	LastRefillTime time.Time
}
    TokenBucketState represents the state of a token bucket

type TokenBucketStats struct {
	// CurrentTokens is the current number of tokens
	CurrentTokens float64

	// Capacity is the maximum number of tokens
	Capacity float64

	// RefillRate is tokens added per refill period
	RefillRate float64

	// RefillPeriod is the time between refills
	RefillPeriod time.Duration

	// LastRefillTime is when tokens were last refilled
	LastRefillTime time.Time

	// NextRefillTime is when tokens will next be refilled
	NextRefillTime time.Time
}
    TokenBucketStats represents statistics for the token bucket


---
## `github.com/abhipray-cpu/niyantrak/backend`

package backend // import "github.com/abhipray-cpu/niyantrak/backend"

Package backend defines storage interfaces and helpers for rate limiter state.

It provides the Backend and AtomicBackend interfaces that all storage
implementations must satisfy, plus the Envelope type for typed JSON
serialization used by Redis and PostgreSQL backends.

Concrete implementations live in sub-packages:
  - github.com/abhipray-cpu/niyantrak/backend/memory — in-process with optional
    GC
  - github.com/abhipray-cpu/niyantrak/backend/redis — Redis with Lua CAS
  - github.com/abhipray-cpu/niyantrak/backend/postgresql — PostgreSQL with row
    locking
  - github.com/abhipray-cpu/niyantrak/backend/custom — user-supplied
    implementation

VARIABLES

var (
	// ErrKeyNotFound indicates the requested key does not exist
	ErrKeyNotFound = errors.New("key not found")

	// ErrKeyExpired indicates the key has expired
	ErrKeyExpired = errors.New("key expired")

	// ErrBackendClosed indicates the backend connection is closed
	ErrBackendClosed = errors.New("backend closed")
)
    Common error definitions used across all backends


FUNCTIONS

func AtomicUpdate(ctx context.Context, b Backend, key string, ttl time.Duration, fn UpdateFunc) (interface{}, error)
    AtomicUpdate is a helper that uses AtomicBackend.Update when the backend
    supports it, otherwise falls back to a non-atomic Get→fn→Set sequence.

func RegisterType(ptr interface{})
    RegisterType registers a concrete type so that JSON-backed backends can
    reconstruct it on Get(). Typically called in an init() block.

        backend.RegisterType((*algorithm.TokenBucketState)(nil))

func Unwrap(raw []byte) (interface{}, error)
    Unwrap decodes bytes produced by Wrap back into the original concrete type.
    If the bytes don't look like an envelope (no _type field), they are
    unmarshalled into a plain interface{} as a fallback.

func Wrap(value interface{}) ([]byte, error)
    Wrap creates an Envelope for value, encoding its type name and JSON payload.
    For primitive types (string, int64, etc.) it returns the raw JSON bytes
    directly without an envelope, so IncrementAndGet values remain simple.


TYPES

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
    AtomicBackend is an optional interface that backends may implement to
    provide atomic read-modify-write operations. When available, limiters use
    Update instead of separate Get→compute→Set calls, eliminating the race
    window between read and write.

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
    Backend represents a storage backend for rate limiting state

type BackendFactory interface {
	// Create creates a new backend instance with the given config
	Create(config interface{}) (Backend, error)

	// Type returns the backend type this factory creates
	Type() string
}
    BackendFactory creates backend instances

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
    BackendRegistry manages backend factories

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
    ConnectionInfo represents backend connection information

type CustomBackend interface {
	Backend

	// GetMetadata returns custom metadata about the backend
	GetMetadata(ctx context.Context) map[string]interface{}

	// Execute executes a custom operation
	Execute(ctx context.Context, operation string, args map[string]interface{}) (interface{}, error)
}
    CustomBackend is a user-defined storage backend

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
    DatabaseStats represents PostgreSQL database statistics

type Envelope struct {
	Type string          `json:"_type"`
	Data json.RawMessage `json:"data"`
}
    Envelope wraps a value with its type name so JSON roundtrips preserve
    the concrete Go type. Redis and PostgreSQL backends use this to
    serialize/deserialize algorithm state correctly.

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
    MemoryBackend is an in-memory storage backend

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
    PostgreSQLBackend is a PostgreSQL-based persistent backend

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
    RedisBackend is a Redis-based distributed backend

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
    ReplicationStatus represents Redis replication status

type UpdateFunc func(currentState interface{}) (newState interface{}, result interface{}, err error)
    UpdateFunc is the callback passed to AtomicBackend.Update. It receives the
    current state (nil if key doesn't exist) and returns:
      - newState: the state to persist back
      - result: an arbitrary value returned to the caller (e.g. algorithm
        result)
      - err: if non-nil, the update is aborted and no write occurs


---
## `github.com/abhipray-cpu/niyantrak/backend/custom`

package custom // import "github.com/abhipray-cpu/niyantrak/backend/custom"

Package custom provides a backend adapter that delegates to user-supplied
functions, allowing any storage system to be plugged into Niyantrak.

FUNCTIONS

func NewCustomBackend(name string, config map[string]interface{}) backend.CustomBackend
    NewCustomBackend creates a new custom backend name: Name of the custom
    backend instance config: Optional configuration map


---
## `github.com/abhipray-cpu/niyantrak/backend/memory`

package memory // import "github.com/abhipray-cpu/niyantrak/backend/memory"

Package memory provides an in-process rate limiter backend using sync.RWMutex.

Use NewMemoryBackend for a simple store where expired entries are cleaned
lazily on read, or NewMemoryBackendWithGC to add a periodic background
garbage-collection goroutine that prevents unbounded growth under write-heavy
workloads.

The memory backend implements both
github.com/abhipray-cpu/niyantrak/backend.Backend and
github.com/abhipray-cpu/niyantrak/backend.AtomicBackend.

VARIABLES

var (
	ErrMaxSizeExceeded = errors.New("max size exceeded")
)
    Local error definition for memory-specific error


FUNCTIONS

func NewMemoryBackend() backend.MemoryBackend
    NewMemoryBackend creates a new memory backend with unlimited size

func NewMemoryBackendWithGC(gcInterval time.Duration) backend.MemoryBackend
    NewMemoryBackendWithGC creates a memory backend that periodically sweeps
    expired keys. The GC goroutine runs every gcInterval and is stopped
    automatically when Close() is called.

    For long-running processes this prevents unbounded memory growth from keys
    that expire but are never read again (lazy expiry alone won't reclaim them).


---
## `github.com/abhipray-cpu/niyantrak/backend/postgresql`

package postgresql // import "github.com/abhipray-cpu/niyantrak/backend/postgresql"

Package postgresql provides a PostgreSQL-backed rate limiter storage backend.

State is stored in a dedicated table with row-level locking via SELECT …
FOR UPDATE for atomic updates. The table-name prefix is validated against
^[a-zA-Z0-9_]*$ at construction time to prevent SQL injection.

FUNCTIONS

func NewPostgreSQLBackend(host string, port int, database, username, password, prefix string) backend.PostgreSQLBackend
    NewPostgreSQLBackend creates a new PostgreSQL backend host: PostgreSQL
    server host port: PostgreSQL server port database: Database name username:
    Database username password: Database password prefix: Table prefix for
    namespacing (optional, must be alphanumeric/underscore only)


---
## `github.com/abhipray-cpu/niyantrak/backend/redis`

package redis // import "github.com/abhipray-cpu/niyantrak/backend/redis"

Package redis provides a Redis-backed rate limiter storage backend.

It uses a Lua Compare-And-Swap (CAS) script for atomic read-modify-write
operations, avoiding WATCH/MULTI/EXEC and working correctly with Redis Cluster.
The backend accepts a github.com/redis/go-redis/v9.UniversalClient,
transparently supporting Standalone, Sentinel, and Cluster topologies.

Constructors:
  - NewRedisBackend — connect by address, DB index, and key prefix
  - NewRedisBackendFromOptions — connect with full RedisOptions
  - NewRedisBackendFromClient — wrap an existing UniversalClient

FUNCTIONS

func NewRedisBackend(addr string, db int, prefix string) backend.RedisBackend
    NewRedisBackend creates a new Redis backend with simple parameters.
    For advanced configuration (Cluster, Sentinel, timeouts) use
    NewRedisBackendFromOptions.

func NewRedisBackendFromClient(client goredis.UniversalClient, prefix string) backend.RedisBackend
    NewRedisBackendFromClient creates a Redis backend from a pre-configured
    UniversalClient. This is useful when you need full control over the client
    configuration (e.g. custom Dialer for Docker/Cluster address remapping).

func NewRedisBackendFromOptions(opts RedisOptions) backend.RedisBackend
    NewRedisBackendFromOptions creates a Redis backend from full options.


TYPES

type RedisOptions struct {
	// Addrs is a list of host:port addresses.
	// Single element  → standalone Redis.
	// Multiple        → Redis Cluster or Sentinel depending on MasterName.
	Addrs []string

	// DB selects a database (standalone/Sentinel only; Cluster always uses 0).
	DB int

	// MasterName, when set, enables Sentinel mode.
	MasterName string

	// Password for Redis AUTH.
	Password string

	// Prefix is prepended to every key for namespacing.
	Prefix string

	// DialTimeout is the maximum time to establish a connection.
	DialTimeout time.Duration

	// ReadTimeout is the maximum time to wait for a response.
	ReadTimeout time.Duration

	// WriteTimeout is the maximum time to wait for a write to complete.
	WriteTimeout time.Duration

	// PoolSize is the maximum number of socket connections.
	PoolSize int

	// MinIdleConns is the minimum number of idle connections to keep.
	MinIdleConns int

	// MaxRetries is the maximum number of command retries before giving up.
	MaxRetries int
}
    RedisOptions configures a Redis backend with full control over connection
    pooling, timeouts, and topology (single-node, Sentinel, Cluster).


---
## `github.com/abhipray-cpu/niyantrak/examples/01_basic_memory`



---
## `github.com/abhipray-cpu/niyantrak/examples/02_basic_redis`



---
## `github.com/abhipray-cpu/niyantrak/examples/03_tier_based`



---
## `github.com/abhipray-cpu/niyantrak/examples/04_tenant_based`



---
## `github.com/abhipray-cpu/niyantrak/examples/05_cost_based`



---
## `github.com/abhipray-cpu/niyantrak/examples/06_composite`



---
## `github.com/abhipray-cpu/niyantrak/features`

package features // import "github.com/abhipray-cpu/niyantrak/features"

Package features provides runtime extensions for rate limiters: dynamic limit
adjustment and backend failover.

  - DynamicLimitManager — change limits at runtime based on external signals
    (load, time of day, admin controls).
  - FailoverHandler — automatically switch to a fallback when the primary
    backend is unavailable, with periodic health-check recovery. Strategies:
    FailOpen, FailClosed, LocalFallback.

TYPES

type DynamicLimitConfig struct {
	// ReloadInterval is how often to check for configuration changes
	ReloadInterval time.Duration

	// ConfigSource is the source of dynamic configuration (file, database, etc.)
	ConfigSource string

	// AllowOnlineUpdates indicates if limits can be updated without reload
	AllowOnlineUpdates bool

	// GracefulSwitching if true, gradually switch to new limits
	GracefulSwitching bool

	// SwitchingPeriod is the duration over which to switch limits
	SwitchingPeriod time.Duration
}
    DynamicLimitConfig holds configuration for dynamic limits

type DynamicLimitController interface {
	// UpdateLimit changes the limit for a specific key at runtime
	// Returns error if the update fails
	UpdateLimit(ctx context.Context, key string, newLimit int, window time.Duration) error

	// UpdateLimitByTier changes limits for all keys in a specific tier
	// Useful for subscription tier changes
	UpdateLimitByTier(ctx context.Context, tier string, newLimit int, window time.Duration) error

	// UpdateLimitByTenant changes limits for all keys under a tenant
	// Useful for multi-tenant applications
	UpdateLimitByTenant(ctx context.Context, tenantID string, newLimit int, window time.Duration) error

	// GetCurrentLimit retrieves the current limit for a key
	GetCurrentLimit(ctx context.Context, key string) (int, time.Duration, error)

	// ReloadConfig reloads configuration from external source (file, database, etc.)
	ReloadConfig(ctx context.Context) error
}
    DynamicLimitController allows runtime adjustment of rate limits

type DynamicLimitManager struct {
	// Has unexported fields.
}
    DynamicLimitManager implements DynamicLimitController

func NewDynamicLimitManager(cfg DynamicLimitManagerConfig) *DynamicLimitManager
    NewDynamicLimitManager creates a new DynamicLimitManager

func (m *DynamicLimitManager) AddUpdateHook(hook func(key string, config *LimitConfig))
    AddUpdateHook registers a callback to be triggered when limits are updated

func (m *DynamicLimitManager) GetAllLimits() map[string]*LimitConfig
    GetAllLimits returns all configured limits

func (m *DynamicLimitManager) GetCurrentLimit(ctx context.Context, key string) (int, time.Duration, error)
    GetCurrentLimit retrieves the current limit for a key

func (m *DynamicLimitManager) GetDefaultLimit() *LimitConfig
    GetDefaultLimit returns the default limit configuration

func (m *DynamicLimitManager) GetTenantLimit(ctx context.Context, tenant string) (*LimitConfig, error)
    GetTenantLimit retrieves the current limit for a tenant

func (m *DynamicLimitManager) GetTierLimit(ctx context.Context, tier string) (*LimitConfig, error)
    GetTierLimit retrieves the current limit for a tier

func (m *DynamicLimitManager) ReloadConfig(ctx context.Context) error
    ReloadConfig reloads the entire configuration

func (m *DynamicLimitManager) UpdateDefaultLimit(ctx context.Context, limit int64, window time.Duration) error
    UpdateDefaultLimit updates the default limit

func (m *DynamicLimitManager) UpdateLimit(ctx context.Context, key string, newLimit int, window time.Duration) error
    UpdateLimit updates the limit for a specific key

func (m *DynamicLimitManager) UpdateLimitByTenant(ctx context.Context, tenantID string, newLimit int, window time.Duration) error
    UpdateLimitByTenant updates the limit for a specific tenant

func (m *DynamicLimitManager) UpdateLimitByTier(ctx context.Context, tier string, newLimit int, window time.Duration) error
    UpdateLimitByTier updates the limit for a specific tier

type DynamicLimitManagerConfig struct {
	DefaultLimit  int64
	DefaultWindow time.Duration
	Logger        obstypes.Logger
	Metrics       obstypes.Metrics
	Tracer        obstypes.Tracer
}
    DynamicLimitManagerConfig configures the DynamicLimitManager

type FailoverConfig struct {
	// EnableFallback enables fallback mechanism
	EnableFallback bool

	// FallbackBackendType is the type of fallback (memory, local cache, etc.)
	FallbackBackendType string

	// HealthCheckInterval is how often to check backend health
	HealthCheckInterval interface{} // time.Duration

	// FailureThreshold is consecutive failures before switching
	FailureThreshold int

	// AutoRecovery if true, automatically switch back when primary recovers
	AutoRecovery bool

	// RecoveryCheckInterval is how often to check for recovery
	RecoveryCheckInterval interface{} // time.Duration
}
    FailoverConfig holds failover configuration

type FailoverHandler interface {
	// OnBackendFailure is called when primary backend operation fails
	// Should return a decision whether to allow/deny the request
	OnBackendFailure(ctx context.Context, key string, err error) interface{}

	// GetFallbackBackend returns a fallback backend if primary fails
	// Returns nil if no fallback is available
	GetFallbackBackend() interface{}

	// IsHealthy checks if the primary backend is healthy
	IsHealthy(ctx context.Context) bool

	// SwitchToFallback switches to fallback mode
	SwitchToFallback(ctx context.Context) error

	// SwitchToPrimary switches back to primary backend
	SwitchToPrimary(ctx context.Context) error

	// GetFallbackStatus returns current failover status
	GetFallbackStatus(ctx context.Context) *FailoverStatus
}
    FailoverHandler manages graceful degradation when backends fail

func NewFailoverManager(
	primaryBackend backend.Backend,
	fallbackBackend backend.Backend,
	config FailoverConfig,
	logger obstypes.Logger,
	metricsCollector obstypes.Metrics,
	tracer obstypes.Tracer,
) (FailoverHandler, error)
    NewFailoverManager creates a new failover handler with primary and fallback
    backends

type FailoverStatus struct {
	// IsFallbackActive indicates if we're using fallback
	IsFallbackActive bool

	// FailureReason is why we switched to fallback (if active)
	FailureReason string

	// SwitchedAt is when the switch happened
	SwitchedAt interface{} // time.Time

	// FailureCount is number of consecutive failures
	FailureCount int

	// LastHealthCheck is when we last checked health
	LastHealthCheck interface{} // time.Time
}
    FailoverStatus represents current failover state

type LimitConfig struct {
	Limit  int64
	Window time.Duration
}
    LimitConfig represents a dynamic limit configuration


---
## `github.com/abhipray-cpu/niyantrak/integration`

package integration // import "github.com/abhipray-cpu/niyantrak/integration"

Package integration contains integration tests for Niyantrak. Run with: go test
-tags integration -v -count=1 ./integration/...

---
## `github.com/abhipray-cpu/niyantrak/limiters`

package limiters // import "github.com/abhipray-cpu/niyantrak/limiters"

Package limiters provides rate limiting implementations

VARIABLES

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
    Common error definitions used across all limiters


TYPES

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
    BasicConfig represents configuration for BasicLimiter

type BasicLimiter interface {
	Limiter
}
    BasicLimiter provides simple per-key rate limiting

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
    CompositeConfig represents configuration for CompositeLimiter

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
    CompositeLimiter provides multiple simultaneous rate limits

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
    CostBasedLimiter provides weighted token-based rate limiting

type CostConfig struct {
	BasicConfig

	// Operations is the initial operation cost configuration
	// Map of operation name to cost
	Operations map[string]int

	// DefaultCost is the cost for operations not in the Operations map
	DefaultCost int
}
    CostConfig represents configuration for CostBasedLimiter

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
    DynamicLimitConfig contains optional dynamic limit management configuration.
    All fields are optional and dynamic limits are disabled if not set.

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
    FailoverConfig contains failover mechanism configuration

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
    LimitConfig represents a single limit configuration

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
    LimitHierarchy represents relationships between limits in composite limiters

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
    LimitResult represents the result of a rate limit check

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
    LimitStatus represents the status of a single limit check

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
    Limiter is the base interface for all rate limiters

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
    ObservabilityConfig contains optional observability integrations. All fields
    are optional and default to no-op implementations for zero overhead.

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
    TenantBasedLimiter provides multi-tenancy rate limiting

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
    TenantConfig represents configuration for TenantBasedLimiter

type TenantLimit struct {
	Limit  int
	Window time.Duration
}
    TenantLimit defines rate limit settings for a tenant.

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
    TenantStats represents aggregated statistics for a tenant

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
    TierBasedLimiter provides subscription tier-based rate limiting

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
    TierConfig represents configuration for TierBasedLimiter

type TierLimit struct {
	Limit  int
	Window time.Duration
}
    TierLimit defines rate limit settings for a tier.


---
## `github.com/abhipray-cpu/niyantrak/limiters/basic`

package basic // import "github.com/abhipray-cpu/niyantrak/limiters/basic"

Package basic provides the default single-key rate limiter implementation.

A BasicLimiter combines one
github.com/abhipray-cpu/niyantrak/algorithm.Algorithm with one
github.com/abhipray-cpu/niyantrak/backend.Backend to perform per-key rate
limiting via [Allow] and [AllowN].

FUNCTIONS

func NewBasicLimiter(
	algo algorithm.Algorithm,
	backend backend.Backend,
	cfg limiters.BasicConfig,
) (limiters.BasicLimiter, error)
    NewBasicLimiter creates a new basic rate limiter


---
## `github.com/abhipray-cpu/niyantrak/limiters/composite`

package composite // import "github.com/abhipray-cpu/niyantrak/limiters/composite"

Package composite provides a composite rate limiter combining multiple
independent rate limits

FUNCTIONS

func NewCompositeLimiter(algo algorithm.Algorithm, be backend.Backend, cfg limiters.CompositeConfig) (limiters.CompositeLimiter, error)
    NewCompositeLimiter creates a new composite rate limiter


---
## `github.com/abhipray-cpu/niyantrak/limiters/cost`

package cost // import "github.com/abhipray-cpu/niyantrak/limiters/cost"

Package cost provides a cost-aware rate limiter where different operations
consume different amounts of quota.

Use [AllowWithCost] to deduct a named operation's cost, and [SetOperationCost]
to define or update costs at runtime.

FUNCTIONS

func NewCostBasedLimiter(
	algo algorithm.Algorithm,
	backend backend.Backend,
	cfg limiters.CostConfig,
) (limiters.CostBasedLimiter, error)
    NewCostBasedLimiter creates a new cost-based rate limiter


---
## `github.com/abhipray-cpu/niyantrak/limiters/tenant`

package tenant // import "github.com/abhipray-cpu/niyantrak/limiters/tenant"

Package tenant provides a multi-tenant rate limiter that assigns keys to tenant
identifiers, each with independent rate limits and statistics.

When PersistMappings is enabled, key→tenant assignments are stored in the
backend under a __tenant_mapping: prefix for distributed consistency.

FUNCTIONS

func NewTenantBasedLimiter(
	algo algorithm.Algorithm,
	backendBackend backend.Backend,
	cfg limiters.TenantConfig,
) (limiters.TenantBasedLimiter, error)
    NewTenantBasedLimiter creates a new tenant-based rate limiter


---
## `github.com/abhipray-cpu/niyantrak/limiters/tier`

package tier // import "github.com/abhipray-cpu/niyantrak/limiters/tier"

Package tier provides a tier-based rate limiter that assigns keys to named tiers
(e.g. "free", "pro", "enterprise"), each with its own rate limit.

When PersistMappings is enabled, key→tier assignments are stored in the backend
under a __tier_mapping: prefix so that all instances in a distributed deployment
share a consistent view.

FUNCTIONS

func NewTierBasedLimiter(
	algo algorithm.Algorithm,
	backend backend.Backend,
	cfg limiters.TierConfig,
) (limiters.TierBasedLimiter, error)
    NewTierBasedLimiter creates a new tier-based rate limiter


---
## `github.com/abhipray-cpu/niyantrak/middleware`

package middleware // import "github.com/abhipray-cpu/niyantrak/middleware"

Package middleware defines the common types and key-extraction helpers shared by
HTTP and gRPC rate-limiting middleware.

Concrete handlers and interceptors live in sub-packages:
  - github.com/abhipray-cpu/niyantrak/middleware/http — net/http handlers
  - github.com/abhipray-cpu/niyantrak/middleware/grpc — gRPC interceptors
  - github.com/abhipray-cpu/niyantrak/middleware/adapters — Gin, Chi, Echo,
    Fiber

TYPES

type CustomMiddleware interface {
	// Apply applies rate limiting to a generic handler
	Apply(handler interface{}, limiter interface{}, options map[string]interface{}) interface{}

	// GetName returns the middleware name
	GetName() string

	// GetSupported returns list of supported integration types
	GetSupported() []string
}
    CustomMiddleware provides a base interface for custom middleware

type GRPCKeyExtractor func(context.Context) (string, error)
    GRPCKeyExtractor extracts a rate limit key from gRPC context

type GRPCMiddleware interface {
	// UnaryInterceptor returns a gRPC unary server interceptor
	UnaryInterceptor(limiter interface{}, options *GRPCOptions) interface{}

	// StreamInterceptor returns a gRPC stream server interceptor
	StreamInterceptor(limiter interface{}, options *GRPCOptions) interface{}

	// GetKeyExtractor returns the default key extractor
	GetKeyExtractor() GRPCKeyExtractor
}
    GRPCMiddleware provides gRPC integration for rate limiting

type GRPCOptions struct {
	// Extractor extracts rate limit key from gRPC context
	Extractor GRPCKeyExtractor

	// MetadataKey is the key to store rate limit info in gRPC metadata
	MetadataKey string

	// SkipMethods is list of gRPC methods to skip rate limiting
	// Format: "/service.Service/Method"
	SkipMethods []string

	// EnableMetadata includes rate limit info in response metadata
	EnableMetadata bool

	// CustomStatusCode is custom gRPC status code for limit exceeded
	CustomStatusCode string
}
    GRPCOptions contains gRPC middleware options

type HTTPMiddleware interface {
	// Wrap wraps an http.Handler with rate limiting
	Wrap(handler http.Handler, limiter interface{}, options *HTTPOptions) http.Handler

	// WrapFunc wraps an http.HandlerFunc with rate limiting
	WrapFunc(handler http.HandlerFunc, limiter interface{}, options *HTTPOptions) http.HandlerFunc

	// GetKeyExtractor returns the default key extractor
	GetKeyExtractor() KeyExtractor
}
    HTTPMiddleware provides HTTP integration for rate limiting

type HTTPOptions struct {
	// Extractor extracts rate limit key from request
	Extractor KeyExtractor

	// Handler handles rate limit exceeded responses
	Handler RateLimitHandler

	// HeaderFormatter formats rate limit response headers
	HeaderFormatter HeaderFormatter

	// SkipPaths is list of paths to skip rate limiting
	SkipPaths []string

	// SkipMethods is list of HTTP methods to skip
	SkipMethods []string

	// CustomKeyHeader is header to use as key (X-API-Key, etc.)
	CustomKeyHeader string

	// EnableDetailedHeaders includes detailed rate limit info
	EnableDetailedHeaders bool
}
    HTTPOptions contains HTTP middleware options

type HeaderFormatter interface {
	// FormatHeaders adds rate limit headers to the response
	FormatHeaders(w http.ResponseWriter, result interface{})

	// GetHeaderNames returns custom header names to use
	GetHeaderNames() *HeaderNames
}
    HeaderFormatter formats rate limit headers for HTTP responses

type HeaderNames struct {
	// Limit header name (default: X-RateLimit-Limit)
	Limit string

	// Remaining header name (default: X-RateLimit-Remaining)
	Remaining string

	// Reset header name (default: X-RateLimit-Reset)
	Reset string

	// RetryAfter header name (default: Retry-After)
	RetryAfter string
}
    HeaderNames specifies custom header names

type KeyExtractor func(*http.Request) (string, error)
    KeyExtractor extracts a rate limit key from an HTTP request

type KeyExtractorBuilder interface {
	// FromHeader extracts key from HTTP header
	FromHeader(headerName string) KeyExtractor

	// FromQuery extracts key from query parameter
	FromQuery(paramName string) KeyExtractor

	// FromPath extracts key from URL path segment
	FromPath(segment int) KeyExtractor

	// FromBody extracts key from request body (JSON field)
	FromBody(fieldName string) KeyExtractor

	// Composite combines multiple extractors
	Composite(extractors ...KeyExtractor) KeyExtractor

	// WithFallback provides fallback extractor if primary fails
	WithFallback(primary KeyExtractor, fallback KeyExtractor) KeyExtractor
}
    KeyExtractorBuilder helps build custom key extractors

type RateLimitHandler interface {
	// HandleExceeded is called when rate limit is exceeded
	HandleExceeded(w http.ResponseWriter, r *http.Request, result interface{})

	// HandleError is called when rate limit check fails
	HandleError(w http.ResponseWriter, r *http.Request, err error)
}
    RateLimitHandler handles rate limit exceeded responses

type ResponseWriter interface {
	// Write writes response data
	Write(b []byte) (int, error)

	// WriteHeader writes HTTP status code
	WriteHeader(statusCode int)

	// Header returns response headers
	Header() http.Header

	// WithRateLimit adds rate limit information
	WithRateLimit(result interface{}) ResponseWriter
}
    ResponseWriter wraps http.ResponseWriter for rate limiting


---
## `github.com/abhipray-cpu/niyantrak/middleware/adapters`

package adapters // import "github.com/abhipray-cpu/niyantrak/middleware/adapters"

Package adapters provides rate-limiting middleware for popular Go HTTP
frameworks: Gin, Chi, Echo, and Fiber.

Each adapter wraps a github.com/abhipray-cpu/niyantrak/limiters.Limiter and
returns the framework's native handler/middleware type.

FUNCTIONS

func ChiRateLimiter(limiter interface{}, options ChiOptions) func(http.Handler) http.Handler
    ChiRateLimiter creates a Chi middleware for rate limiting

func EchoRateLimiter(limiter interface{}, options EchoOptions) echo.MiddlewareFunc
    EchoRateLimiter creates an Echo middleware for rate limiting

func FiberRateLimiter(limiter interface{}, options FiberOptions) fiber.Handler
    FiberRateLimiter creates a Fiber middleware for rate limiting

func GinRateLimiter(limiter interface{}, options GinOptions) gin.HandlerFunc
    GinRateLimiter creates a Gin middleware for rate limiting


TYPES

type ChiOptions struct {
	// KeyExtractor extracts the rate limit key from the HTTP request
	// If nil, defaults to extracting from X-API-Key header or client IP
	KeyExtractor func(*http.Request) string

	// OnRateLimitExceeded is called when rate limit is exceeded
	// If nil, returns 429 with JSON error
	OnRateLimitExceeded func(http.ResponseWriter, *http.Request)

	// OnError is called when an error occurs during rate limiting
	// If nil, returns 400 with JSON error
	OnError func(http.ResponseWriter, *http.Request, error)

	// SkipPaths contains paths that should skip rate limiting
	SkipPaths []string

	// IncludeHeaders determines if rate limit headers should be included
	// Default: true (set to &false to disable)
	IncludeHeaders *bool

	// AbortOnError determines if the request should be aborted on error
	// Default: false (continues to handler)
	AbortOnError bool
}
    ChiOptions contains configuration options for Chi rate limiter

type EchoOptions struct {
	// KeyExtractor extracts the rate limit key from the Echo context
	// If nil, defaults to extracting from X-API-Key header or client IP
	KeyExtractor func(echo.Context) string

	// OnRateLimitExceeded is called when rate limit is exceeded
	// If nil, returns 429 with JSON error
	OnRateLimitExceeded func(echo.Context) error

	// OnError is called when an error occurs during rate limiting
	// If nil, returns 400 with JSON error
	OnError func(echo.Context, error) error

	// SkipPaths contains paths that should skip rate limiting
	SkipPaths []string

	// IncludeHeaders determines if rate limit headers should be included
	// Default: true (set to &false to disable)
	IncludeHeaders *bool

	// AbortOnError determines if the request should be aborted on error
	// Default: false (continues to handler)
	AbortOnError bool
}
    EchoOptions contains configuration options for Echo rate limiter

type FiberOptions struct {
	// KeyExtractor extracts the rate limit key from the Fiber context
	// If nil, defaults to extracting from X-API-Key header or client IP
	KeyExtractor func(*fiber.Ctx) string

	// OnRateLimitExceeded is called when rate limit is exceeded
	// If nil, returns 429 with JSON error
	OnRateLimitExceeded func(*fiber.Ctx) error

	// OnError is called when an error occurs during rate limiting
	// If nil, returns 400 with JSON error
	OnError func(*fiber.Ctx, error) error

	// SkipPaths contains paths that should skip rate limiting
	SkipPaths []string

	// IncludeHeaders determines if rate limit headers should be included
	// Default: true (set to &false to disable)
	IncludeHeaders *bool

	// AbortOnError determines if the request should be aborted on error
	// Default: false (continues to handler)
	AbortOnError bool
}
    FiberOptions contains configuration options for Fiber rate limiter

type GinOptions struct {
	// KeyExtractor extracts the rate limit key from the Gin context
	// If nil, defaults to extracting from X-API-Key header or client IP
	KeyExtractor func(*gin.Context) string

	// OnRateLimitExceeded is called when rate limit is exceeded
	// If nil, returns 429 with JSON error
	OnRateLimitExceeded func(*gin.Context)

	// OnError is called when an error occurs during rate limiting
	// If nil, returns 400 with JSON error
	OnError func(*gin.Context, error)

	// SkipPaths contains paths that should skip rate limiting
	SkipPaths []string

	// IncludeHeaders determines if rate limit headers should be included
	// Use pointer to distinguish between not set (nil = default true) and explicitly false
	IncludeHeaders *bool

	// AbortOnError determines if the request should be aborted on error
	// Default: false (continues to handler)
	AbortOnError bool
}
    GinOptions contains configuration options for Gin rate limiter


---
## `github.com/abhipray-cpu/niyantrak/middleware/grpc`

package grpcmiddleware // import "github.com/abhipray-cpu/niyantrak/middleware/grpc"

Package grpcmiddleware provides gRPC unary and streaming interceptors that
enforce rate limits.

Denied RPCs return codes.ResourceExhausted with google.rpc.RetryInfo and
google.rpc.QuotaFailure error details, enabling well-behaved clients to
implement programmatic retry logic.

FUNCTIONS

func New() middleware.GRPCMiddleware
    New creates a new gRPC middleware instance


---
## `github.com/abhipray-cpu/niyantrak/middleware/http`

package http // import "github.com/abhipray-cpu/niyantrak/middleware/http"

Package http provides net/http middleware that enforces rate limits and
sets standard response headers (X-RateLimit-Limit, X-RateLimit-Remaining,
X-RateLimit-Reset, Retry-After).

Denied requests receive a 429 Too Many Requests status.

FUNCTIONS

func New() middleware.HTTPMiddleware
    New creates a new HTTP middleware instance

func NewCustomHeaderFormatter(names *middleware.HeaderNames) middleware.HeaderFormatter
    NewCustomHeaderFormatter creates a formatter with custom header names

func NewDefaultHeaderFormatter() middleware.HeaderFormatter
    NewDefaultHeaderFormatter creates a new default header formatter

func NewDefaultRateLimitHandler() middleware.RateLimitHandler
    NewDefaultRateLimitHandler creates a new default rate limit handler


---
## `github.com/abhipray-cpu/niyantrak/mocks`

package mocks // import "github.com/abhipray-cpu/niyantrak/mocks"

Package mocks is a generated GoMock package.

Package mocks is a generated GoMock package.

Package mocks is a generated GoMock package.

Package mocks is a generated GoMock package.

Package mocks is a generated GoMock package.

Package mocks is a generated GoMock package.

TYPES

type MockAlgorithm struct {
	// Has unexported fields.
}
    MockAlgorithm is a mock of Algorithm interface.

func NewMockAlgorithm(ctrl *gomock.Controller) *MockAlgorithm
    NewMockAlgorithm creates a new mock instance.

func (m *MockAlgorithm) Allow(ctx context.Context, state interface{}, cost int) (interface{}, interface{}, error)
    Allow mocks base method.

func (m *MockAlgorithm) Description() string
    Description mocks base method.

func (m *MockAlgorithm) EXPECT() *MockAlgorithmMockRecorder
    EXPECT returns an object that allows the caller to indicate expected use.

func (m *MockAlgorithm) GetStats(ctx context.Context, state interface{}) interface{}
    GetStats mocks base method.

func (m *MockAlgorithm) Name() string
    Name mocks base method.

func (m *MockAlgorithm) Reset(ctx context.Context) (interface{}, error)
    Reset mocks base method.

func (m *MockAlgorithm) ValidateConfig(config interface{}) error
    ValidateConfig mocks base method.

type MockAlgorithmFactory struct {
	// Has unexported fields.
}
    MockAlgorithmFactory is a mock of AlgorithmFactory interface.

func NewMockAlgorithmFactory(ctrl *gomock.Controller) *MockAlgorithmFactory
    NewMockAlgorithmFactory creates a new mock instance.

func (m *MockAlgorithmFactory) Create(config interface{}) (algorithm.Algorithm, error)
    Create mocks base method.

func (m *MockAlgorithmFactory) EXPECT() *MockAlgorithmFactoryMockRecorder
    EXPECT returns an object that allows the caller to indicate expected use.

func (m *MockAlgorithmFactory) Type() string
    Type mocks base method.

type MockAlgorithmFactoryMockRecorder struct {
	// Has unexported fields.
}
    MockAlgorithmFactoryMockRecorder is the mock recorder for
    MockAlgorithmFactory.

func (mr *MockAlgorithmFactoryMockRecorder) Create(config interface{}) *gomock.Call
    Create indicates an expected call of Create.

func (mr *MockAlgorithmFactoryMockRecorder) Type() *gomock.Call
    Type indicates an expected call of Type.

type MockAlgorithmMockRecorder struct {
	// Has unexported fields.
}
    MockAlgorithmMockRecorder is the mock recorder for MockAlgorithm.

func (mr *MockAlgorithmMockRecorder) Allow(ctx, state, cost interface{}) *gomock.Call
    Allow indicates an expected call of Allow.

func (mr *MockAlgorithmMockRecorder) Description() *gomock.Call
    Description indicates an expected call of Description.

func (mr *MockAlgorithmMockRecorder) GetStats(ctx, state interface{}) *gomock.Call
    GetStats indicates an expected call of GetStats.

func (mr *MockAlgorithmMockRecorder) Name() *gomock.Call
    Name indicates an expected call of Name.

func (mr *MockAlgorithmMockRecorder) Reset(ctx interface{}) *gomock.Call
    Reset indicates an expected call of Reset.

func (mr *MockAlgorithmMockRecorder) ValidateConfig(config interface{}) *gomock.Call
    ValidateConfig indicates an expected call of ValidateConfig.

type MockAlgorithmRegistry struct {
	// Has unexported fields.
}
    MockAlgorithmRegistry is a mock of AlgorithmRegistry interface.

func NewMockAlgorithmRegistry(ctrl *gomock.Controller) *MockAlgorithmRegistry
    NewMockAlgorithmRegistry creates a new mock instance.

func (m *MockAlgorithmRegistry) Create(algorithmType string, config interface{}) (algorithm.Algorithm, error)
    Create mocks base method.

func (m *MockAlgorithmRegistry) EXPECT() *MockAlgorithmRegistryMockRecorder
    EXPECT returns an object that allows the caller to indicate expected use.

func (m *MockAlgorithmRegistry) Get(algorithmType string) (algorithm.AlgorithmFactory, error)
    Get mocks base method.

func (m *MockAlgorithmRegistry) List() []string
    List mocks base method.

func (m *MockAlgorithmRegistry) Register(factory algorithm.AlgorithmFactory) error
    Register mocks base method.

type MockAlgorithmRegistryMockRecorder struct {
	// Has unexported fields.
}
    MockAlgorithmRegistryMockRecorder is the mock recorder for
    MockAlgorithmRegistry.

func (mr *MockAlgorithmRegistryMockRecorder) Create(algorithmType, config interface{}) *gomock.Call
    Create indicates an expected call of Create.

func (mr *MockAlgorithmRegistryMockRecorder) Get(algorithmType interface{}) *gomock.Call
    Get indicates an expected call of Get.

func (mr *MockAlgorithmRegistryMockRecorder) List() *gomock.Call
    List indicates an expected call of List.

func (mr *MockAlgorithmRegistryMockRecorder) Register(factory interface{}) *gomock.Call
    Register indicates an expected call of Register.

type MockBackend struct {
	// Has unexported fields.
}
    MockBackend is a mock of Backend interface.

func NewMockBackend(ctrl *gomock.Controller) *MockBackend
    NewMockBackend creates a new mock instance.

func (m *MockBackend) Close() error
    Close mocks base method.

func (m *MockBackend) Delete(ctx context.Context, key string) error
    Delete mocks base method.

func (m *MockBackend) EXPECT() *MockBackendMockRecorder
    EXPECT returns an object that allows the caller to indicate expected use.

func (m *MockBackend) Get(ctx context.Context, key string) (interface{}, error)
    Get mocks base method.

func (m *MockBackend) IncrementAndGet(ctx context.Context, key string, ttl time.Duration) (int64, error)
    IncrementAndGet mocks base method.

func (m *MockBackend) Ping(ctx context.Context) error
    Ping mocks base method.

func (m *MockBackend) Set(ctx context.Context, key string, state interface{}, ttl time.Duration) error
    Set mocks base method.

func (m *MockBackend) Type() string
    Type mocks base method.

type MockBackendFactory struct {
	// Has unexported fields.
}
    MockBackendFactory is a mock of BackendFactory interface.

func NewMockBackendFactory(ctrl *gomock.Controller) *MockBackendFactory
    NewMockBackendFactory creates a new mock instance.

func (m *MockBackendFactory) Create(config interface{}) (backend.Backend, error)
    Create mocks base method.

func (m *MockBackendFactory) EXPECT() *MockBackendFactoryMockRecorder
    EXPECT returns an object that allows the caller to indicate expected use.

func (m *MockBackendFactory) Type() string
    Type mocks base method.

type MockBackendFactoryMockRecorder struct {
	// Has unexported fields.
}
    MockBackendFactoryMockRecorder is the mock recorder for MockBackendFactory.

func (mr *MockBackendFactoryMockRecorder) Create(config interface{}) *gomock.Call
    Create indicates an expected call of Create.

func (mr *MockBackendFactoryMockRecorder) Type() *gomock.Call
    Type indicates an expected call of Type.

type MockBackendMockRecorder struct {
	// Has unexported fields.
}
    MockBackendMockRecorder is the mock recorder for MockBackend.

func (mr *MockBackendMockRecorder) Close() *gomock.Call
    Close indicates an expected call of Close.

func (mr *MockBackendMockRecorder) Delete(ctx, key interface{}) *gomock.Call
    Delete indicates an expected call of Delete.

func (mr *MockBackendMockRecorder) Get(ctx, key interface{}) *gomock.Call
    Get indicates an expected call of Get.

func (mr *MockBackendMockRecorder) IncrementAndGet(ctx, key, ttl interface{}) *gomock.Call
    IncrementAndGet indicates an expected call of IncrementAndGet.

func (mr *MockBackendMockRecorder) Ping(ctx interface{}) *gomock.Call
    Ping indicates an expected call of Ping.

func (mr *MockBackendMockRecorder) Set(ctx, key, state, ttl interface{}) *gomock.Call
    Set indicates an expected call of Set.

func (mr *MockBackendMockRecorder) Type() *gomock.Call
    Type indicates an expected call of Type.

type MockBackendRegistry struct {
	// Has unexported fields.
}
    MockBackendRegistry is a mock of BackendRegistry interface.

func NewMockBackendRegistry(ctrl *gomock.Controller) *MockBackendRegistry
    NewMockBackendRegistry creates a new mock instance.

func (m *MockBackendRegistry) Create(backendType string, config interface{}) (backend.Backend, error)
    Create mocks base method.

func (m *MockBackendRegistry) EXPECT() *MockBackendRegistryMockRecorder
    EXPECT returns an object that allows the caller to indicate expected use.

func (m *MockBackendRegistry) Get(backendType string) (backend.BackendFactory, error)
    Get mocks base method.

func (m *MockBackendRegistry) List() []string
    List mocks base method.

func (m *MockBackendRegistry) Register(factory backend.BackendFactory) error
    Register mocks base method.

type MockBackendRegistryMockRecorder struct {
	// Has unexported fields.
}
    MockBackendRegistryMockRecorder is the mock recorder for
    MockBackendRegistry.

func (mr *MockBackendRegistryMockRecorder) Create(backendType, config interface{}) *gomock.Call
    Create indicates an expected call of Create.

func (mr *MockBackendRegistryMockRecorder) Get(backendType interface{}) *gomock.Call
    Get indicates an expected call of Get.

func (mr *MockBackendRegistryMockRecorder) List() *gomock.Call
    List indicates an expected call of List.

func (mr *MockBackendRegistryMockRecorder) Register(factory interface{}) *gomock.Call
    Register indicates an expected call of Register.

type MockCustomBackend struct {
	// Has unexported fields.
}
    MockCustomBackend is a mock of CustomBackend interface.

func NewMockCustomBackend(ctrl *gomock.Controller) *MockCustomBackend
    NewMockCustomBackend creates a new mock instance.

func (m *MockCustomBackend) Close() error
    Close mocks base method.

func (m *MockCustomBackend) Delete(ctx context.Context, key string) error
    Delete mocks base method.

func (m *MockCustomBackend) EXPECT() *MockCustomBackendMockRecorder
    EXPECT returns an object that allows the caller to indicate expected use.

func (m *MockCustomBackend) Execute(ctx context.Context, operation string, args map[string]interface{}) (interface{}, error)
    Execute mocks base method.

func (m *MockCustomBackend) Get(ctx context.Context, key string) (interface{}, error)
    Get mocks base method.

func (m *MockCustomBackend) GetMetadata(ctx context.Context) map[string]interface{}
    GetMetadata mocks base method.

func (m *MockCustomBackend) IncrementAndGet(ctx context.Context, key string, ttl time.Duration) (int64, error)
    IncrementAndGet mocks base method.

func (m *MockCustomBackend) Ping(ctx context.Context) error
    Ping mocks base method.

func (m *MockCustomBackend) Set(ctx context.Context, key string, state interface{}, ttl time.Duration) error
    Set mocks base method.

func (m *MockCustomBackend) Type() string
    Type mocks base method.

type MockCustomBackendMockRecorder struct {
	// Has unexported fields.
}
    MockCustomBackendMockRecorder is the mock recorder for MockCustomBackend.

func (mr *MockCustomBackendMockRecorder) Close() *gomock.Call
    Close indicates an expected call of Close.

func (mr *MockCustomBackendMockRecorder) Delete(ctx, key interface{}) *gomock.Call
    Delete indicates an expected call of Delete.

func (mr *MockCustomBackendMockRecorder) Execute(ctx, operation, args interface{}) *gomock.Call
    Execute indicates an expected call of Execute.

func (mr *MockCustomBackendMockRecorder) Get(ctx, key interface{}) *gomock.Call
    Get indicates an expected call of Get.

func (mr *MockCustomBackendMockRecorder) GetMetadata(ctx interface{}) *gomock.Call
    GetMetadata indicates an expected call of GetMetadata.

func (mr *MockCustomBackendMockRecorder) IncrementAndGet(ctx, key, ttl interface{}) *gomock.Call
    IncrementAndGet indicates an expected call of IncrementAndGet.

func (mr *MockCustomBackendMockRecorder) Ping(ctx interface{}) *gomock.Call
    Ping indicates an expected call of Ping.

func (mr *MockCustomBackendMockRecorder) Set(ctx, key, state, ttl interface{}) *gomock.Call
    Set indicates an expected call of Set.

func (mr *MockCustomBackendMockRecorder) Type() *gomock.Call
    Type indicates an expected call of Type.

type MockDynamicLimitController struct {
	// Has unexported fields.
}
    MockDynamicLimitController is a mock of DynamicLimitController interface.

func NewMockDynamicLimitController(ctrl *gomock.Controller) *MockDynamicLimitController
    NewMockDynamicLimitController creates a new mock instance.

func (m *MockDynamicLimitController) EXPECT() *MockDynamicLimitControllerMockRecorder
    EXPECT returns an object that allows the caller to indicate expected use.

func (m *MockDynamicLimitController) GetCurrentLimit(ctx context.Context, key string) (int, time.Duration, error)
    GetCurrentLimit mocks base method.

func (m *MockDynamicLimitController) ReloadConfig(ctx context.Context) error
    ReloadConfig mocks base method.

func (m *MockDynamicLimitController) UpdateLimit(ctx context.Context, key string, newLimit int, window time.Duration) error
    UpdateLimit mocks base method.

func (m *MockDynamicLimitController) UpdateLimitByTenant(ctx context.Context, tenantID string, newLimit int, window time.Duration) error
    UpdateLimitByTenant mocks base method.

func (m *MockDynamicLimitController) UpdateLimitByTier(ctx context.Context, tier string, newLimit int, window time.Duration) error
    UpdateLimitByTier mocks base method.

type MockDynamicLimitControllerMockRecorder struct {
	// Has unexported fields.
}
    MockDynamicLimitControllerMockRecorder is the mock recorder for
    MockDynamicLimitController.

func (mr *MockDynamicLimitControllerMockRecorder) GetCurrentLimit(ctx, key interface{}) *gomock.Call
    GetCurrentLimit indicates an expected call of GetCurrentLimit.

func (mr *MockDynamicLimitControllerMockRecorder) ReloadConfig(ctx interface{}) *gomock.Call
    ReloadConfig indicates an expected call of ReloadConfig.

func (mr *MockDynamicLimitControllerMockRecorder) UpdateLimit(ctx, key, newLimit, window interface{}) *gomock.Call
    UpdateLimit indicates an expected call of UpdateLimit.

func (mr *MockDynamicLimitControllerMockRecorder) UpdateLimitByTenant(ctx, tenantID, newLimit, window interface{}) *gomock.Call
    UpdateLimitByTenant indicates an expected call of UpdateLimitByTenant.

func (mr *MockDynamicLimitControllerMockRecorder) UpdateLimitByTier(ctx, tier, newLimit, window interface{}) *gomock.Call
    UpdateLimitByTier indicates an expected call of UpdateLimitByTier.

type MockLogger struct {
	// Has unexported fields.
}
    MockLogger is a mock of Logger interface.

func NewMockLogger(ctrl *gomock.Controller) *MockLogger
    NewMockLogger creates a new mock instance.

func (m *MockLogger) Debug(message string, args ...interface{})
    Debug mocks base method.

func (m *MockLogger) EXPECT() *MockLoggerMockRecorder
    EXPECT returns an object that allows the caller to indicate expected use.

func (m *MockLogger) Error(message string, args ...interface{})
    Error mocks base method.

func (m *MockLogger) Info(message string, args ...interface{})
    Info mocks base method.

func (m *MockLogger) Warn(message string, args ...interface{})
    Warn mocks base method.

type MockLoggerMockRecorder struct {
	// Has unexported fields.
}
    MockLoggerMockRecorder is the mock recorder for MockLogger.

func (mr *MockLoggerMockRecorder) Debug(message interface{}, args ...interface{}) *gomock.Call
    Debug indicates an expected call of Debug.

func (mr *MockLoggerMockRecorder) Error(message interface{}, args ...interface{}) *gomock.Call
    Error indicates an expected call of Error.

func (mr *MockLoggerMockRecorder) Info(message interface{}, args ...interface{}) *gomock.Call
    Info indicates an expected call of Info.

func (mr *MockLoggerMockRecorder) Warn(message interface{}, args ...interface{}) *gomock.Call
    Warn indicates an expected call of Warn.

type MockMemoryBackend struct {
	// Has unexported fields.
}
    MockMemoryBackend is a mock of MemoryBackend interface.

func NewMockMemoryBackend(ctrl *gomock.Controller) *MockMemoryBackend
    NewMockMemoryBackend creates a new mock instance.

func (m *MockMemoryBackend) Clear(ctx context.Context) error
    Clear mocks base method.

func (m *MockMemoryBackend) Close() error
    Close mocks base method.

func (m *MockMemoryBackend) Delete(ctx context.Context, key string) error
    Delete mocks base method.

func (m *MockMemoryBackend) EXPECT() *MockMemoryBackendMockRecorder
    EXPECT returns an object that allows the caller to indicate expected use.

func (m *MockMemoryBackend) Get(ctx context.Context, key string) (interface{}, error)
    Get mocks base method.

func (m *MockMemoryBackend) GetMemoryUsage(ctx context.Context) (int64, error)
    GetMemoryUsage mocks base method.

func (m *MockMemoryBackend) GetSize(ctx context.Context) (int, error)
    GetSize mocks base method.

func (m *MockMemoryBackend) IncrementAndGet(ctx context.Context, key string, ttl time.Duration) (int64, error)
    IncrementAndGet mocks base method.

func (m *MockMemoryBackend) Ping(ctx context.Context) error
    Ping mocks base method.

func (m *MockMemoryBackend) Set(ctx context.Context, key string, state interface{}, ttl time.Duration) error
    Set mocks base method.

func (m *MockMemoryBackend) SetMaxSize(ctx context.Context, maxSize int) error
    SetMaxSize mocks base method.

func (m *MockMemoryBackend) Type() string
    Type mocks base method.

type MockMemoryBackendMockRecorder struct {
	// Has unexported fields.
}
    MockMemoryBackendMockRecorder is the mock recorder for MockMemoryBackend.

func (mr *MockMemoryBackendMockRecorder) Clear(ctx interface{}) *gomock.Call
    Clear indicates an expected call of Clear.

func (mr *MockMemoryBackendMockRecorder) Close() *gomock.Call
    Close indicates an expected call of Close.

func (mr *MockMemoryBackendMockRecorder) Delete(ctx, key interface{}) *gomock.Call
    Delete indicates an expected call of Delete.

func (mr *MockMemoryBackendMockRecorder) Get(ctx, key interface{}) *gomock.Call
    Get indicates an expected call of Get.

func (mr *MockMemoryBackendMockRecorder) GetMemoryUsage(ctx interface{}) *gomock.Call
    GetMemoryUsage indicates an expected call of GetMemoryUsage.

func (mr *MockMemoryBackendMockRecorder) GetSize(ctx interface{}) *gomock.Call
    GetSize indicates an expected call of GetSize.

func (mr *MockMemoryBackendMockRecorder) IncrementAndGet(ctx, key, ttl interface{}) *gomock.Call
    IncrementAndGet indicates an expected call of IncrementAndGet.

func (mr *MockMemoryBackendMockRecorder) Ping(ctx interface{}) *gomock.Call
    Ping indicates an expected call of Ping.

func (mr *MockMemoryBackendMockRecorder) Set(ctx, key, state, ttl interface{}) *gomock.Call
    Set indicates an expected call of Set.

func (mr *MockMemoryBackendMockRecorder) SetMaxSize(ctx, maxSize interface{}) *gomock.Call
    SetMaxSize indicates an expected call of SetMaxSize.

func (mr *MockMemoryBackendMockRecorder) Type() *gomock.Call
    Type indicates an expected call of Type.

type MockMetrics struct {
	// Has unexported fields.
}
    MockMetrics is a mock of Metrics interface.

func NewMockMetrics(ctrl *gomock.Controller) *MockMetrics
    NewMockMetrics creates a new mock instance.

func (m *MockMetrics) EXPECT() *MockMetricsMockRecorder
    EXPECT returns an object that allows the caller to indicate expected use.

func (m *MockMetrics) GetMetrics() interface{}
    GetMetrics mocks base method.

func (m *MockMetrics) RecordDecisionLatency(key string, latencyNs int64)
    RecordDecisionLatency mocks base method.

func (m *MockMetrics) RecordRequest(key string, allowed bool, limit int64)
    RecordRequest mocks base method.

type MockMetricsMockRecorder struct {
	// Has unexported fields.
}
    MockMetricsMockRecorder is the mock recorder for MockMetrics.

func (mr *MockMetricsMockRecorder) GetMetrics() *gomock.Call
    GetMetrics indicates an expected call of GetMetrics.

func (mr *MockMetricsMockRecorder) RecordDecisionLatency(key, latencyNs interface{}) *gomock.Call
    RecordDecisionLatency indicates an expected call of RecordDecisionLatency.

func (mr *MockMetricsMockRecorder) RecordRequest(key, allowed, limit interface{}) *gomock.Call
    RecordRequest indicates an expected call of RecordRequest.

type MockPostgreSQLBackend struct {
	// Has unexported fields.
}
    MockPostgreSQLBackend is a mock of PostgreSQLBackend interface.

func NewMockPostgreSQLBackend(ctrl *gomock.Controller) *MockPostgreSQLBackend
    NewMockPostgreSQLBackend creates a new mock instance.

func (m *MockPostgreSQLBackend) CleanupExpired(ctx context.Context) (int64, error)
    CleanupExpired mocks base method.

func (m *MockPostgreSQLBackend) Close() error
    Close mocks base method.

func (m *MockPostgreSQLBackend) CreateTable(ctx context.Context) error
    CreateTable mocks base method.

func (m *MockPostgreSQLBackend) Delete(ctx context.Context, key string) error
    Delete mocks base method.

func (m *MockPostgreSQLBackend) EXPECT() *MockPostgreSQLBackendMockRecorder
    EXPECT returns an object that allows the caller to indicate expected use.

func (m *MockPostgreSQLBackend) Get(ctx context.Context, key string) (interface{}, error)
    Get mocks base method.

func (m *MockPostgreSQLBackend) GetSchema(ctx context.Context) string
    GetSchema mocks base method.

func (m *MockPostgreSQLBackend) GetStats(ctx context.Context) *backend.DatabaseStats
    GetStats mocks base method.

func (m *MockPostgreSQLBackend) IncrementAndGet(ctx context.Context, key string, ttl time.Duration) (int64, error)
    IncrementAndGet mocks base method.

func (m *MockPostgreSQLBackend) Migrate(ctx context.Context) error
    Migrate mocks base method.

func (m *MockPostgreSQLBackend) Ping(ctx context.Context) error
    Ping mocks base method.

func (m *MockPostgreSQLBackend) Set(ctx context.Context, key string, state interface{}, ttl time.Duration) error
    Set mocks base method.

func (m *MockPostgreSQLBackend) Transaction(ctx context.Context, fn func(backend.Backend) error) error
    Transaction mocks base method.

func (m *MockPostgreSQLBackend) Type() string
    Type mocks base method.

func (m *MockPostgreSQLBackend) Vacuum(ctx context.Context) error
    Vacuum mocks base method.

type MockPostgreSQLBackendMockRecorder struct {
	// Has unexported fields.
}
    MockPostgreSQLBackendMockRecorder is the mock recorder for
    MockPostgreSQLBackend.

func (mr *MockPostgreSQLBackendMockRecorder) CleanupExpired(ctx interface{}) *gomock.Call
    CleanupExpired indicates an expected call of CleanupExpired.

func (mr *MockPostgreSQLBackendMockRecorder) Close() *gomock.Call
    Close indicates an expected call of Close.

func (mr *MockPostgreSQLBackendMockRecorder) CreateTable(ctx interface{}) *gomock.Call
    CreateTable indicates an expected call of CreateTable.

func (mr *MockPostgreSQLBackendMockRecorder) Delete(ctx, key interface{}) *gomock.Call
    Delete indicates an expected call of Delete.

func (mr *MockPostgreSQLBackendMockRecorder) Get(ctx, key interface{}) *gomock.Call
    Get indicates an expected call of Get.

func (mr *MockPostgreSQLBackendMockRecorder) GetSchema(ctx interface{}) *gomock.Call
    GetSchema indicates an expected call of GetSchema.

func (mr *MockPostgreSQLBackendMockRecorder) GetStats(ctx interface{}) *gomock.Call
    GetStats indicates an expected call of GetStats.

func (mr *MockPostgreSQLBackendMockRecorder) IncrementAndGet(ctx, key, ttl interface{}) *gomock.Call
    IncrementAndGet indicates an expected call of IncrementAndGet.

func (mr *MockPostgreSQLBackendMockRecorder) Migrate(ctx interface{}) *gomock.Call
    Migrate indicates an expected call of Migrate.

func (mr *MockPostgreSQLBackendMockRecorder) Ping(ctx interface{}) *gomock.Call
    Ping indicates an expected call of Ping.

func (mr *MockPostgreSQLBackendMockRecorder) Set(ctx, key, state, ttl interface{}) *gomock.Call
    Set indicates an expected call of Set.

func (mr *MockPostgreSQLBackendMockRecorder) Transaction(ctx, fn interface{}) *gomock.Call
    Transaction indicates an expected call of Transaction.

func (mr *MockPostgreSQLBackendMockRecorder) Type() *gomock.Call
    Type indicates an expected call of Type.

func (mr *MockPostgreSQLBackendMockRecorder) Vacuum(ctx interface{}) *gomock.Call
    Vacuum indicates an expected call of Vacuum.

type MockRedisBackend struct {
	// Has unexported fields.
}
    MockRedisBackend is a mock of RedisBackend interface.

func NewMockRedisBackend(ctrl *gomock.Controller) *MockRedisBackend
    NewMockRedisBackend creates a new mock instance.

func (m *MockRedisBackend) CheckCluster(ctx context.Context) (bool, error)
    CheckCluster mocks base method.

func (m *MockRedisBackend) Close() error
    Close mocks base method.

func (m *MockRedisBackend) Delete(ctx context.Context, key string) error
    Delete mocks base method.

func (m *MockRedisBackend) EXPECT() *MockRedisBackendMockRecorder
    EXPECT returns an object that allows the caller to indicate expected use.

func (m *MockRedisBackend) Flush(ctx context.Context) error
    Flush mocks base method.

func (m *MockRedisBackend) Get(ctx context.Context, key string) (interface{}, error)
    Get mocks base method.

func (m *MockRedisBackend) GetConnection(ctx context.Context) *backend.ConnectionInfo
    GetConnection mocks base method.

func (m *MockRedisBackend) GetReplication(ctx context.Context) *backend.ReplicationStatus
    GetReplication mocks base method.

func (m *MockRedisBackend) IncrementAndGet(ctx context.Context, key string, ttl time.Duration) (int64, error)
    IncrementAndGet mocks base method.

func (m *MockRedisBackend) Ping(ctx context.Context) error
    Ping mocks base method.

func (m *MockRedisBackend) Scan(ctx context.Context, pattern string, count int) ([]string, error)
    Scan mocks base method.

func (m *MockRedisBackend) Set(ctx context.Context, key string, state interface{}, ttl time.Duration) error
    Set mocks base method.

func (m *MockRedisBackend) Type() string
    Type mocks base method.

type MockRedisBackendMockRecorder struct {
	// Has unexported fields.
}
    MockRedisBackendMockRecorder is the mock recorder for MockRedisBackend.

func (mr *MockRedisBackendMockRecorder) CheckCluster(ctx interface{}) *gomock.Call
    CheckCluster indicates an expected call of CheckCluster.

func (mr *MockRedisBackendMockRecorder) Close() *gomock.Call
    Close indicates an expected call of Close.

func (mr *MockRedisBackendMockRecorder) Delete(ctx, key interface{}) *gomock.Call
    Delete indicates an expected call of Delete.

func (mr *MockRedisBackendMockRecorder) Flush(ctx interface{}) *gomock.Call
    Flush indicates an expected call of Flush.

func (mr *MockRedisBackendMockRecorder) Get(ctx, key interface{}) *gomock.Call
    Get indicates an expected call of Get.

func (mr *MockRedisBackendMockRecorder) GetConnection(ctx interface{}) *gomock.Call
    GetConnection indicates an expected call of GetConnection.

func (mr *MockRedisBackendMockRecorder) GetReplication(ctx interface{}) *gomock.Call
    GetReplication indicates an expected call of GetReplication.

func (mr *MockRedisBackendMockRecorder) IncrementAndGet(ctx, key, ttl interface{}) *gomock.Call
    IncrementAndGet indicates an expected call of IncrementAndGet.

func (mr *MockRedisBackendMockRecorder) Ping(ctx interface{}) *gomock.Call
    Ping indicates an expected call of Ping.

func (mr *MockRedisBackendMockRecorder) Scan(ctx, pattern, count interface{}) *gomock.Call
    Scan indicates an expected call of Scan.

func (mr *MockRedisBackendMockRecorder) Set(ctx, key, state, ttl interface{}) *gomock.Call
    Set indicates an expected call of Set.

func (mr *MockRedisBackendMockRecorder) Type() *gomock.Call
    Type indicates an expected call of Type.

type MockTracer struct {
	// Has unexported fields.
}
    MockTracer is a mock of Tracer interface.

func NewMockTracer(ctrl *gomock.Controller) *MockTracer
    NewMockTracer creates a new mock instance.

func (m *MockTracer) AddAttribute(span *obstypes.SpanContext, key string, value interface{})
    AddAttribute mocks base method.

func (m *MockTracer) AddEvent(span *obstypes.SpanContext, event string, args ...interface{})
    AddEvent mocks base method.

func (m *MockTracer) EXPECT() *MockTracerMockRecorder
    EXPECT returns an object that allows the caller to indicate expected use.

func (m *MockTracer) EndSpan(span *obstypes.SpanContext)
    EndSpan mocks base method.

func (m *MockTracer) SetError(span *obstypes.SpanContext, message string)
    SetError mocks base method.

func (m *MockTracer) StartSpan(ctx context.Context, name string) *obstypes.SpanContext
    StartSpan mocks base method.

type MockTracerMockRecorder struct {
	// Has unexported fields.
}
    MockTracerMockRecorder is the mock recorder for MockTracer.

func (mr *MockTracerMockRecorder) AddAttribute(span, key, value interface{}) *gomock.Call
    AddAttribute indicates an expected call of AddAttribute.

func (mr *MockTracerMockRecorder) AddEvent(span, event interface{}, args ...interface{}) *gomock.Call
    AddEvent indicates an expected call of AddEvent.

func (mr *MockTracerMockRecorder) EndSpan(span interface{}) *gomock.Call
    EndSpan indicates an expected call of EndSpan.

func (mr *MockTracerMockRecorder) SetError(span, message interface{}) *gomock.Call
    SetError indicates an expected call of SetError.

func (mr *MockTracerMockRecorder) StartSpan(ctx, name interface{}) *gomock.Call
    StartSpan indicates an expected call of StartSpan.


---
## `github.com/abhipray-cpu/niyantrak/observability/logging`

package logging // import "github.com/abhipray-cpu/niyantrak/observability/logging"

Package logging provides a Zerolog-based adapter for the
github.com/abhipray-cpu/niyantrak/observability/types.Logger interface.

TYPES

type Logger interface {
	// Debug logs a debug-level message with key-value pairs
	Debug(message string, args ...interface{})

	// Info logs an info-level message with key-value pairs
	Info(message string, args ...interface{})

	// Warn logs a warn-level message with key-value pairs
	Warn(message string, args ...interface{})

	// Error logs an error-level message with key-value pairs
	Error(message string, args ...interface{})
}
    Logger defines the interface for structured logging

type NoOpLogger struct{}
    NoOpLogger is a logger that does nothing

func (n *NoOpLogger) Debug(message string, args ...interface{})
    Debug logs nothing

func (n *NoOpLogger) Error(message string, args ...interface{})
    Error logs nothing

func (n *NoOpLogger) Info(message string, args ...interface{})
    Info logs nothing

func (n *NoOpLogger) Warn(message string, args ...interface{})
    Warn logs nothing

type ZerologLogger struct {
	// Has unexported fields.
}
    ZerologLogger is a structured logger using zerolog

func NewZerologger(opts ...ZerologOption) *ZerologLogger
    NewZerologger creates a new ZerologLogger with options

func (zl *ZerologLogger) Debug(message string, args ...interface{})
    Debug logs a debug-level message

func (zl *ZerologLogger) Error(message string, args ...interface{})
    Error logs an error-level message

func (zl *ZerologLogger) Info(message string, args ...interface{})
    Info logs an info-level message

func (zl *ZerologLogger) Warn(message string, args ...interface{})
    Warn logs a warn-level message

type ZerologOption func(*ZerologLogger)
    ZerologOption is a functional option for ZerologLogger

func WithLevel(level string) ZerologOption
    WithLevel sets the log level

func WithOutput(w io.Writer) ZerologOption
    WithOutput sets the output writer


---
## `github.com/abhipray-cpu/niyantrak/observability/metrics`

package metrics // import "github.com/abhipray-cpu/niyantrak/observability/metrics"

Package metrics provides a Prometheus-based adapter for the
github.com/abhipray-cpu/niyantrak/observability/types.Metrics interface.

It registers counters, histograms, and gauges that are scraped via the standard
/metrics HTTP endpoint.

TYPES

type Metrics interface {
	// RecordRequest records a rate limit decision
	// allowed: whether the request was allowed
	// limit: the rate limit for this key
	RecordRequest(key string, allowed bool, limit int64)

	// RecordDecisionLatency records the latency of a rate limit decision in nanoseconds
	RecordDecisionLatency(key string, latencyNs int64)

	// GetMetrics returns the current metrics state
	GetMetrics() interface{}
}
    Metrics defines the interface for rate limit metrics

type NoOpMetrics struct{}
    NoOpMetrics is a metrics implementation that does nothing

func (n *NoOpMetrics) GetMetrics() interface{}
    GetMetrics returns nil

func (n *NoOpMetrics) RecordDecisionLatency(key string, latencyNs int64)
    RecordDecisionLatency does nothing

func (n *NoOpMetrics) RecordRequest(key string, allowed bool, limit int64)
    RecordRequest does nothing

type PrometheusMetrics struct {
	// Has unexported fields.
}
    PrometheusMetrics is a Prometheus-based metrics implementation

func NewPrometheusMetrics(namespace string, registry prometheus.Registerer) *PrometheusMetrics
    NewPrometheusMetrics creates a new PrometheusMetrics with given namespace
    and registry

func (pm *PrometheusMetrics) GetMetrics() interface{}
    GetMetrics returns the current metrics

func (pm *PrometheusMetrics) RecordDecisionLatency(key string, latencyNs int64)
    RecordDecisionLatency records the latency of a rate limit decision

func (pm *PrometheusMetrics) RecordRequest(key string, allowed bool, limit int64)
    RecordRequest records a rate limit decision


---
## `github.com/abhipray-cpu/niyantrak/observability/tracing`

package tracing // import "github.com/abhipray-cpu/niyantrak/observability/tracing"

Package tracing provides an OpenTelemetry-based adapter for the
github.com/abhipray-cpu/niyantrak/observability/types.Tracer interface.

Spans are created for Allow/AllowN operations and exported via OTLP.

TYPES

type NoOpTracer struct{}
    NoOpTracer is a tracer that does nothing

func (n *NoOpTracer) AddAttribute(span *SpanContext, key string, value interface{})
    AddAttribute adds an attribute without doing anything

func (n *NoOpTracer) AddEvent(span *SpanContext, event string, args ...interface{})
    AddEvent adds an event without doing anything

func (n *NoOpTracer) EndSpan(span *SpanContext)
    EndSpan ends a span without doing anything

func (n *NoOpTracer) SetError(span *SpanContext, message string)
    SetError marks the span as error without doing anything

func (n *NoOpTracer) StartSpan(ctx context.Context, name string) *SpanContext
    StartSpan starts a span that does nothing

type OpenTelemetryTracer struct {
	// Has unexported fields.
}
    OpenTelemetryTracer is an OpenTelemetry-based tracer

func NewOpenTelemetryTracer(name string) *OpenTelemetryTracer
    NewOpenTelemetryTracer creates a new OpenTelemetry tracer

func (ot *OpenTelemetryTracer) AddAttribute(span *SpanContext, key string, value interface{})
    AddAttribute adds a single attribute to the span

func (ot *OpenTelemetryTracer) AddEvent(span *SpanContext, event string, args ...interface{})
    AddEvent adds an event to the span

func (ot *OpenTelemetryTracer) EndSpan(span *SpanContext)
    EndSpan ends the span

func (ot *OpenTelemetryTracer) SetError(span *SpanContext, message string)
    SetError marks the span as having an error

func (ot *OpenTelemetryTracer) StartSpan(ctx context.Context, name string) *SpanContext
    StartSpan starts a new span

type SpanContext struct {
	// Has unexported fields.
}
    SpanContext represents a span in the tracing system

type Tracer interface {
	// StartSpan starts a new span with the given name
	StartSpan(ctx context.Context, name string) *SpanContext

	// EndSpan ends the span and finalizes it
	EndSpan(span *SpanContext)

	// AddEvent adds an event to the span with key-value attributes
	AddEvent(span *SpanContext, event string, args ...interface{})

	// AddAttribute adds a single attribute to the span
	AddAttribute(span *SpanContext, key string, value interface{})

	// SetError marks the span as having an error
	SetError(span *SpanContext, message string)
}
    Tracer defines the interface for distributed tracing


---
## `github.com/abhipray-cpu/niyantrak/observability/types`

package obstypes // import "github.com/abhipray-cpu/niyantrak/observability/types"

Package obstypes defines the interfaces for logging, metrics, and tracing used
throughout the rate limiter. This package has ZERO external dependencies.

Use this package when your code needs to accept or store observability types
without pulling in heavy dependencies like zerolog, prometheus, or otel.

Concrete implementations live in the sibling packages:

    observability/logging   — ZerologLogger (github.com/rs/zerolog)
    observability/metrics   — PrometheusMetrics (github.com/prometheus/client_golang)
    observability/tracing   — OpenTelemetryTracer (go.opentelemetry.io/otel)

TYPES

type Logger interface {
	// Debug logs a debug-level message with key-value pairs
	Debug(message string, args ...interface{})

	// Info logs an info-level message with key-value pairs
	Info(message string, args ...interface{})

	// Warn logs a warn-level message with key-value pairs
	Warn(message string, args ...interface{})

	// Error logs an error-level message with key-value pairs
	Error(message string, args ...interface{})
}
    Logger defines the interface for structured logging.

type Metrics interface {
	// RecordRequest records a rate limit decision.
	// allowed: whether the request was allowed
	// limit: the rate limit for this key
	RecordRequest(key string, allowed bool, limit int64)

	// RecordDecisionLatency records the latency of a rate limit decision in nanoseconds.
	RecordDecisionLatency(key string, latencyNs int64)

	// GetMetrics returns the current metrics state.
	GetMetrics() interface{}
}
    Metrics defines the interface for rate limit metrics.

type NoOpLogger struct{}
    NoOpLogger is a logger that does nothing (zero overhead).

func (n *NoOpLogger) Debug(message string, args ...interface{})

func (n *NoOpLogger) Error(message string, args ...interface{})

func (n *NoOpLogger) Info(message string, args ...interface{})

func (n *NoOpLogger) Warn(message string, args ...interface{})

type NoOpMetrics struct{}
    NoOpMetrics is a metrics implementation that does nothing (zero overhead).

func (n *NoOpMetrics) GetMetrics() interface{}

func (n *NoOpMetrics) RecordDecisionLatency(key string, latencyNs int64)

func (n *NoOpMetrics) RecordRequest(key string, allowed bool, limit int64)

type NoOpTracer struct{}
    NoOpTracer is a tracer that does nothing (zero overhead).

func (n *NoOpTracer) AddAttribute(span *SpanContext, key string, value interface{})

func (n *NoOpTracer) AddEvent(span *SpanContext, event string, args ...interface{})

func (n *NoOpTracer) EndSpan(span *SpanContext)

func (n *NoOpTracer) SetError(span *SpanContext, message string)

func (n *NoOpTracer) StartSpan(ctx context.Context, name string) *SpanContext
    StartSpan returns a SpanContext that carries the original context through.

type SpanContext struct {
	// Span holds the underlying tracer span. For OpenTelemetry this is a
	// trace.Span, but this package doesn't depend on otel.
	Span interface{}

	// Ctx holds the context associated with this span.
	Ctx context.Context
}
    SpanContext represents a span in the tracing system. The Span field is an
    opaque interface so that this package remains free of external dependencies.
    Concrete tracer implementations store and retrieve their own span types via
    type assertion on this field.

func (sc *SpanContext) Context() context.Context
    Context returns the context associated with this span. Returns
    context.Background() if the span or its context is nil.

type Tracer interface {
	// StartSpan starts a new span with the given name
	StartSpan(ctx context.Context, name string) *SpanContext

	// EndSpan ends the span and finalises it
	EndSpan(span *SpanContext)

	// AddEvent adds an event to the span with key-value attributes
	AddEvent(span *SpanContext, event string, args ...interface{})

	// AddAttribute adds a single attribute to the span
	AddAttribute(span *SpanContext, key string, value interface{})

	// SetError marks the span as having an error
	SetError(span *SpanContext, message string)
}
    Tracer defines the interface for distributed tracing.


