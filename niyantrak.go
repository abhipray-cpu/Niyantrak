// Package niyantrak provides a comprehensive rate limiting library for Go applications.
//
// Niyantrak (Sanskrit: नियंत्रक - "Controller") offers multiple rate limiting algorithms,
// pluggable storage backends, middleware support, and built-in observability.
//
// # Core Features
//
// - 5 Algorithms: Token Bucket, Leaky Bucket, Fixed Window, Sliding Window, GCRA
// - Multiple Backends: In-memory, Redis, PostgreSQL, custom
// - Middleware: HTTP and gRPC support with standard rate limit headers
// - Observability: Prometheus metrics and structured logging
// - Production-Ready: Thread-safe, low latency, comprehensive testing
//
// # Quick Start
//
// Create a basic rate limiter:
//
//	import (
//		"context"
//		"log"
//		"github.com/abhipray-cpu/niyantrak"
//		"github.com/abhipray-cpu/niyantrak/limiters/basic"
//		"github.com/abhipray-cpu/niyantrak/backend/memory"
//		"github.com/abhipray-cpu/niyantrak/algorithm"
//	)
//
//	func main() {
//		// Create memory backend
//		backend := memory.NewMemoryBackend()
//		defer backend.Close()
//
//		// Create algorithm
//		algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
//			Capacity:     100,
//			RefillRate:   10,
//			RefillPeriod: time.Second,
//		})
//
//		// Create limiter
//		limiter, err := basic.NewBasicLimiter(algo, backend, limiters.BasicConfig{
//			DefaultLimit:  100,
//			DefaultWindow: time.Second,
//		})
//		if err != nil {
//			log.Fatal(err)
//		}
//		defer limiter.Close()
//
//		// Check rate limit
//		ctx := context.Background()
//		result := limiter.Allow(ctx, "user:123")
//		if !result.Allowed {
//			log.Printf("Rate limited! Retry after %v", result.RetryAfter)
//			return
//		}
//
//		log.Printf("Request allowed. Remaining: %d/%d", result.Remaining, result.Limit)
//	}
//
// # Architecture
//
// The library uses a layered architecture:
//
//	┌─────────────────────────────────┐
//	│     Application Code            │
//	├─────────────────────────────────┤
//	│   HTTP/gRPC Middleware          │
//	├─────────────────────────────────┤
//	│   Limiter Interface             │
//	│ (Basic/Tier/Tenant/Cost/Composite) │
//	├─────────────────────────────────┤
//	│   Algorithm Layer               │
//	│ (Token Bucket, Leaky Bucket, etc) │
//	├─────────────────────────────────┤
//	│   Backend Storage               │
//	│ (Memory, Redis, PostgreSQL)     │
//	└─────────────────────────────────┘
//
// # Algorithms
//
// - TokenBucket: Allows burst traffic while maintaining average rate
// - LeakyBucket: Smooths traffic to a constant rate
// - FixedWindow: Resets counter at fixed time boundaries
// - SlidingWindow: More accurate with sliding time windows
// - GCRA: Generic Cell Rate Algorithm for precise rate limiting
//
// # Backends
//
// - Memory: In-memory storage (single-instance, no persistence)
// - Redis: Distributed storage for multi-instance deployments
// - PostgreSQL: Persistent storage with audit trail
// - Custom: Implement Backend interface for custom storage
//
// # Limiter Types
//
// - BasicLimiter: Simple per-key rate limiting
// - TierBasedLimiter: Subscription tier-based limits
// - TenantBasedLimiter: Multi-tenancy rate limiting
// - CostBasedLimiter: Operation cost-based limiting
// - CompositeLimiter: Combine multiple limiters
package niyantrak

import (
	"github.com/abhipray-cpu/niyantrak/algorithm"
	"github.com/abhipray-cpu/niyantrak/backend"
	"github.com/abhipray-cpu/niyantrak/features"
	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/middleware"
)

func init() {
	// Register algorithm state types so JSON-backed backends (Redis, PostgreSQL)
	// can reconstruct the correct concrete types after deserialization.
	backend.RegisterType((*algorithm.TokenBucketState)(nil))
	backend.RegisterType((*algorithm.LeakyBucketState)(nil))
	backend.RegisterType((*algorithm.FixedWindowState)(nil))
	backend.RegisterType((*algorithm.SlidingWindowState)(nil))
	backend.RegisterType((*algorithm.GCRAState)(nil))
}

// Re-export core types for convenience at package level
// These are the main types external code will interact with

// ============================================================================
// Algorithm Types (re-exported from algorithm package)
// ============================================================================

// Algorithm represents a rate limiting algorithm implementation
type Algorithm = algorithm.Algorithm

// TokenBucketConfig configures the Token Bucket algorithm
type TokenBucketConfig = algorithm.TokenBucketConfig

// LeakyBucketConfig configures the Leaky Bucket algorithm
type LeakyBucketConfig = algorithm.LeakyBucketConfig

// FixedWindowConfig configures the Fixed Window algorithm
type FixedWindowConfig = algorithm.FixedWindowConfig

// SlidingWindowConfig configures the Sliding Window algorithm
type SlidingWindowConfig = algorithm.SlidingWindowConfig

// GCRAConfig configures the GCRA algorithm
type GCRAConfig = algorithm.GCRAConfig

// ============================================================================
// Backend Types (re-exported from backend package)
// ============================================================================

// Backend represents a storage backend for rate limiting state
type Backend = backend.Backend

// MemoryBackend is an in-memory storage backend
type MemoryBackend = backend.MemoryBackend

// RedisBackend is a Redis-based distributed backend
type RedisBackend = backend.RedisBackend

// PostgreSQLBackend is a PostgreSQL-based persistent backend
type PostgreSQLBackend = backend.PostgreSQLBackend

// CustomBackend allows implementing custom storage backends
type CustomBackend = backend.CustomBackend

// ============================================================================
// Limiter Types (re-exported from limiters package)
// ============================================================================

// Limiter is the base interface for all rate limiters
type Limiter = limiters.Limiter

// BasicLimiter provides simple per-key rate limiting
type BasicLimiter = limiters.BasicLimiter

// TierBasedLimiter provides subscription tier-based rate limiting
type TierBasedLimiter = limiters.TierBasedLimiter

// TenantBasedLimiter provides multi-tenancy rate limiting
type TenantBasedLimiter = limiters.TenantBasedLimiter

// CostBasedLimiter provides operation cost-based rate limiting
type CostBasedLimiter = limiters.CostBasedLimiter

// CompositeLimiter combines multiple limiters for complex scenarios
type CompositeLimiter = limiters.CompositeLimiter

// LimitResult represents the result of a rate limit check
type LimitResult = limiters.LimitResult

// TenantStats represents aggregated statistics for a tenant
type TenantStats = limiters.TenantStats

// ============================================================================
// Middleware Types (re-exported from middleware package)
// ============================================================================

// HTTPMiddleware provides HTTP integration for rate limiting
type HTTPMiddleware = middleware.HTTPMiddleware

// GRPCMiddleware provides gRPC integration for rate limiting
type GRPCMiddleware = middleware.GRPCMiddleware

// CustomMiddleware provides a base interface for custom middleware
type CustomMiddleware = middleware.CustomMiddleware

// KeyExtractor extracts a rate limit key from an HTTP request
type KeyExtractor = middleware.KeyExtractor

// GRPCKeyExtractor extracts a rate limit key from gRPC context
type GRPCKeyExtractor = middleware.GRPCKeyExtractor

// HTTPOptions contains HTTP middleware options
type HTTPOptions = middleware.HTTPOptions

// GRPCOptions contains gRPC middleware options
type GRPCOptions = middleware.GRPCOptions

// RateLimitHandler handles rate limit exceeded responses
type RateLimitHandler = middleware.RateLimitHandler

// HeaderFormatter formats rate limit response headers
type HeaderFormatter = middleware.HeaderFormatter

// HeaderNames specifies custom header names
type HeaderNames = middleware.HeaderNames

// KeyExtractorBuilder helps build custom key extractors
type KeyExtractorBuilder = middleware.KeyExtractorBuilder

// ============================================================================
// Observability
// ============================================================================
//
// Lightweight observability interfaces (Logger, Metrics, Tracer) and their
// zero-overhead NoOp implementations live in the dependency-free package:
//
//	import obstypes "github.com/abhipray-cpu/niyantrak/observability/types"
//
// Concrete implementations with external dependencies are opt-in:
//
//	import "github.com/abhipray-cpu/niyantrak/observability/logging"  // ZerologLogger  (github.com/rs/zerolog)
//	import "github.com/abhipray-cpu/niyantrak/observability/metrics"  // PrometheusMetrics (github.com/prometheus/client_golang)
//	import "github.com/abhipray-cpu/niyantrak/observability/tracing"  // OpenTelemetryTracer (go.opentelemetry.io/otel)

// ============================================================================
// Features Types (re-exported from features package)
// ============================================================================

// DynamicLimitController allows runtime adjustment of rate limits
type DynamicLimitController = features.DynamicLimitController

// FailoverHandler manages graceful degradation when backends fail
type FailoverHandler = features.FailoverHandler

// FailoverStatus represents current failover state
type FailoverStatus = features.FailoverStatus

// ============================================================================
// Configuration Types (re-exported from limiters package)
// ============================================================================

// BasicConfig holds configuration for creating a BasicLimiter
type BasicConfig = limiters.BasicConfig

// TierConfig holds configuration for creating a TierBasedLimiter
type TierConfig = limiters.TierConfig

// TenantConfig holds configuration for creating a TenantBasedLimiter
type TenantConfig = limiters.TenantConfig

// CostConfig holds configuration for creating a CostBasedLimiter
type CostConfig = limiters.CostConfig

// CompositeConfig holds configuration for creating a CompositeLimiter
type CompositeConfig = limiters.CompositeConfig

// ObservabilityConfig holds observability settings
type ObservabilityConfig = limiters.ObservabilityConfig

// DynamicLimitConfig holds dynamic limit configuration
type DynamicLimitConfig = limiters.DynamicLimitConfig

// FailoverConfig holds failover configuration
type FailoverConfig = limiters.FailoverConfig

// LimitConfig represents a single rate limit rule
type LimitConfig = limiters.LimitConfig

// ============================================================================
// Error Types (re-exported for convenience)
// ============================================================================

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

// ============================================================================
// Package-level Documentation
// ============================================================================

// Limiter Types Overview
//
// # BasicLimiter
// Simple per-key rate limiting. Use for:
// - API rate limiting per user/API key
// - Simple traffic shaping
// - Individual resource limits
//
// Example:
//	limiter, _ := basic.NewBasicLimiter(algo, backend, config)
//	result := limiter.Allow(ctx, "user:123")
//
// # TierBasedLimiter
// Subscription tier-based limits. Use for:
// - SaaS applications with tiers (free, pro, enterprise)
// - Different rate limits per subscription level
// - Tier-specific overrides
//
// Example:
//	tierLimiter.SetTierLimit(ctx, "premium", 10000, time.Hour)
//	tierLimiter.AssignKeyToTier(ctx, "user:123", "premium")
//
// # TenantBasedLimiter
// Multi-tenancy rate limiting. Use for:
// - Multi-tenant platforms
// - Separate limits per organization/customer
// - Cross-key aggregation for tenants
//
// Example:
//	tenantLimiter.SetTenantLimit(ctx, "org:456", 5000, time.Hour)
//	tenantLimiter.AssignKeyToTenant(ctx, "user:123", "org:456")
//
// # CostBasedLimiter
// Operation cost-based limiting. Use for:
// - Operations with different resource costs
// - Consumption-based billing
// - AllowN with variable token consumption
//
// Example:
//	result := costLimiter.AllowN(ctx, "user:123", 10) // 10 tokens for expensive op
//
// # CompositeLimiter
// Combine multiple limiters. Use for:
// - Complex scenarios with multiple constraints
// - Per-user + per-hour + per-minute limits
// - Layered rate limiting policies
//
// Example:
//	composite := &CompositeLimiter{
//		Limiters: []Limiter{perSecondLimiter, perMinuteLimiter},
//	}
//
// # Middleware Integration
//
// Middlewares provide seamless integration with web frameworks:
//
// ## HTTP Middleware
// Integrates with standard http.Handler and popular frameworks (Chi, Gin, Echo, Fiber).
//
// Example with standard library:
//	import "github.com/abhipray-cpu/niyantrak/middleware/http"
//
//	httpMW := http.New()
//	opts := &HTTPOptions{
//		Extractor: func(r *http.Request) (string, error) {
//			return r.Header.Get("X-API-Key"), nil
//		},
//		SkipPaths: []string{"/health", "/metrics"},
//	}
//
//	mux := http.NewServeMux()
//	mux.HandleFunc("/api/users", httpMW.Wrap(handleUsers, limiter, opts).ServeHTTP)
//
// ## gRPC Middleware
// Integrates as unary and stream interceptors.
//
// Example with gRPC:
//	import "github.com/abhipray-cpu/niyantrak/middleware/grpc"
//
//	grpcMW := grpc.New()
//	opts := &GRPCOptions{
//		Extractor: func(ctx context.Context) (string, error) {
//			md, ok := metadata.FromIncomingContext(ctx)
//			if !ok {
//				return "", errors.New("no metadata")
//			}
//			keys := md.Get("x-api-key")
//			if len(keys) > 0 {
//				return keys[0], nil
//			}
//			return "", errors.New("no api key")
//		},
//		EnableMetadata: true,
//	}
//
//	server := grpc.NewServer(
//		grpc.UnaryInterceptor(grpcMW.UnaryInterceptor(limiter, opts).(grpc.UnaryServerInterceptor)),
//		grpc.StreamInterceptor(grpcMW.StreamInterceptor(limiter, opts).(grpc.StreamServerInterceptor)),
//	)
//
// ## Key Extraction Strategies
//
// Common strategies for extracting rate limit keys:
//
// - From Header: X-API-Key, Authorization, custom headers
// - From Query: api_key parameter in URL
// - From Path: user ID in URL path
// - From Context: User info stored in request context
// - Composite: Combine multiple strategies with fallbacks
//
// Access middleware via:
//	- github.com/abhipray-cpu/niyantrak/middleware/http - HTTP middleware
//	- github.com/abhipray-cpu/niyantrak/middleware/grpc - gRPC middleware
//	- github.com/abhipray-cpu/niyantrak/middleware/adapters - Framework adapters (Chi, Gin, Echo, Fiber)
//
// # Observability
//
// Built-in observability supports monitoring and debugging.
// Interfaces and NoOp implementations (zero overhead) are in the lightweight package:
//
//	import obstypes "github.com/abhipray-cpu/niyantrak/observability/types"
//
// Concrete implementations are opt-in — import only what you need:
//
// ## Logging
// Structured logging for debugging and operational insights.
//
//	import "github.com/abhipray-cpu/niyantrak/observability/logging"  // pulls in zerolog
//
//	logger := logging.NewZerologger(
//		logging.WithLevel("info"),
//		logging.WithOutput(os.Stderr),
//	)
//	config.Observability.Logger = logger
//
// ## Metrics
// Prometheus metrics for monitoring rate limit decisions.
//
//	import "github.com/abhipray-cpu/niyantrak/observability/metrics"  // pulls in prometheus
//
//	metricsCollector := metrics.NewPrometheusMetrics("niyantrak", registry)
//	config.Observability.Metrics = metricsCollector
//
// ## Tracing
// Distributed tracing with OpenTelemetry for request tracking.
//
//	import "github.com/abhipray-cpu/niyantrak/observability/tracing"  // pulls in otel
//
//	tracer := tracing.NewOpenTelemetryTracer("niyantrak")
//	config.Observability.Tracer = tracer
//
// # Advanced Features
//
// ## Dynamic Limits
// Adjust rate limits at runtime without restarting.
//
// Access via:
//	import "github.com/abhipray-cpu/niyantrak/features"
//
// Capabilities:
//	- UpdateLimit() - Change limit for specific key
//	- UpdateLimitByTier() - Change limits for subscription tier
//	- UpdateLimitByTenant() - Change limits for tenant
//	- GetCurrentLimit() - Query current limit
//	- ReloadConfig() - Reload from external source
//
// Example:
//	dynamicLimits := &DynamicLimitConfig{
//		AllowOnlineUpdates: true,
//		GracefulSwitching: true,
//		SwitchingPeriod: 5 * time.Minute,
//	}
//	config.DynamicLimits = dynamicLimits
//
// Use cases:
//	- Upgrade/downgrade users during their session
//	- Adjust limits based on system load
//	- Provide temporary limit increases
//	- Time-based limit changes (peak hours)
//
// ## Failover Strategy
// Graceful degradation when backend storage fails.
//
// Strategies:
//	- FailOpen - Allow requests when backend fails (default, better UX)
//	- FailClosed - Deny requests when backend fails (more secure)
//	- LocalFallback - Use in-memory fallback backend
//
// Example:
//	failover := &FailoverConfig{
//		EnableFallback: true,
//		FallbackBackendType: "memory",
//		FailureThreshold: 5,
//		AutoRecovery: true,
//	}
//	config.Failover = failover
//
// Features:
//	- Automatic fallback on backend failure
//	- Health checking with configurable intervals
//	- Automatic recovery when backend recovers
//	- Graceful switch back to primary
//	- Failure reason tracking
//	- Detailed failover status reporting
//
// # Integration Checklist
//
// When using Niyantrak:
//	✓ Choose algorithm (TokenBucket recommended for general use)
//	✓ Choose backend (Memory for single-instance, Redis for distributed)
//	✓ Choose limiter type (BasicLimiter for most cases)
//	✓ Integrate middleware (HTTP or gRPC)
//	✓ Configure observability (logging, metrics, tracing)
//	✓ Enable features (dynamic limits, failover) as needed
//	✓ Set appropriate limits and windows for your use case
//	✓ Test with expected traffic patterns
//
// # Package Organization
//
// For direct imports beyond the public API, use:
//	- github.com/abhipray-cpu/niyantrak/algorithm - Algorithm implementations
//	- github.com/abhipray-cpu/niyantrak/backend - Backend implementations
//	- github.com/abhipray-cpu/niyantrak/limiters - Limiter implementations
//	- github.com/abhipray-cpu/niyantrak/middleware - Middleware implementations
//	- github.com/abhipray-cpu/niyantrak/observability/types - Observability interfaces (zero deps)
//	- github.com/abhipray-cpu/niyantrak/observability/logging - Zerolog logger (opt-in)
//	- github.com/abhipray-cpu/niyantrak/observability/metrics - Prometheus metrics (opt-in)
//	- github.com/abhipray-cpu/niyantrak/observability/tracing - OpenTelemetry tracer (opt-in)
//	- github.com/abhipray-cpu/niyantrak/features - Advanced features
