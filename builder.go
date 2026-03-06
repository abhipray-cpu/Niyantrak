package niyantrak

import (
	"time"

	"github.com/abhipray-cpu/niyantrak/algorithm"
	backendiface "github.com/abhipray-cpu/niyantrak/backend"
	"github.com/abhipray-cpu/niyantrak/backend/memory"
	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/limiters/basic"
	"github.com/abhipray-cpu/niyantrak/limiters/composite"
	"github.com/abhipray-cpu/niyantrak/limiters/cost"
	"github.com/abhipray-cpu/niyantrak/limiters/tenant"
	"github.com/abhipray-cpu/niyantrak/limiters/tier"
)

// AlgorithmType identifies the rate limiting algorithm to use.
type AlgorithmType int

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

// Option configures the rate limiter created by New.
type Option func(*options)

type options struct {
	algorithmType AlgorithmType

	// Token bucket
	tokenBucketConfig *algorithm.TokenBucketConfig

	// Leaky bucket
	leakyBucketConfig *algorithm.LeakyBucketConfig

	// Fixed window
	fixedWindowConfig *algorithm.FixedWindowConfig

	// Sliding window
	slidingWindowConfig *algorithm.SlidingWindowConfig

	// GCRA
	gcraConfig *algorithm.GCRAConfig

	// Limiter config
	defaultLimit  int
	defaultWindow time.Duration
	keyTTL        time.Duration

	// Backend
	backendInstance backendiface.Backend
}

// WithAlgorithm selects the rate limiting algorithm.
func WithAlgorithm(t AlgorithmType) Option {
	return func(o *options) {
		o.algorithmType = t
	}
}

// WithTokenBucketConfig sets the configuration for the Token Bucket algorithm.
func WithTokenBucketConfig(cfg TokenBucketConfig) Option {
	return func(o *options) {
		o.tokenBucketConfig = &cfg
	}
}

// WithLeakyBucketConfig sets the configuration for the Leaky Bucket algorithm.
func WithLeakyBucketConfig(cfg LeakyBucketConfig) Option {
	return func(o *options) {
		o.leakyBucketConfig = &cfg
	}
}

// WithFixedWindowConfig sets the configuration for the Fixed Window algorithm.
func WithFixedWindowConfig(cfg FixedWindowConfig) Option {
	return func(o *options) {
		o.fixedWindowConfig = &cfg
	}
}

// WithSlidingWindowConfig sets the configuration for the Sliding Window algorithm.
func WithSlidingWindowConfig(cfg SlidingWindowConfig) Option {
	return func(o *options) {
		o.slidingWindowConfig = &cfg
	}
}

// WithGCRAConfig sets the configuration for the GCRA algorithm.
func WithGCRAConfig(cfg GCRAConfig) Option {
	return func(o *options) {
		o.gcraConfig = &cfg
	}
}

// WithMemoryBackend uses an in-memory storage backend.
// Best for single-instance applications and development.
func WithMemoryBackend() Option {
	return func(o *options) {
		o.backendInstance = memory.NewMemoryBackend()
	}
}

// WithBackend uses a custom or pre-configured storage backend.
// Use this to pass a Redis, PostgreSQL, or custom backend.
func WithBackend(b Backend) Option {
	return func(o *options) {
		o.backendInstance = b
	}
}

// WithLimit sets the default rate limit (requests per window).
func WithLimit(limit int) Option {
	return func(o *options) {
		o.defaultLimit = limit
	}
}

// WithWindow sets the default time window for rate limiting.
func WithWindow(d time.Duration) Option {
	return func(o *options) {
		o.defaultWindow = d
	}
}

// WithKeyTTL sets how long rate limit state is kept per key.
func WithKeyTTL(d time.Duration) Option {
	return func(o *options) {
		o.keyTTL = d
	}
}

// New creates a BasicLimiter using functional options.
//
// This is the recommended entry point for simple rate limiting. For advanced
// scenarios (tier-based, tenant-based, composite), use the limiter sub-packages
// directly.
//
//	limiter, err := niyantrak.New(
//	    niyantrak.WithAlgorithm(niyantrak.TokenBucket),
//	    niyantrak.WithTokenBucketConfig(niyantrak.TokenBucketConfig{
//	        Capacity:     100,
//	        RefillRate:   100,
//	        RefillPeriod: time.Minute,
//	    }),
//	    niyantrak.WithMemoryBackend(),
//	)
func New(opts ...Option) (BasicLimiter, error) {
	o := &options{
		algorithmType: TokenBucket,
		defaultLimit:  100,
		defaultWindow: time.Minute,
		keyTTL:        time.Hour,
	}

	for _, opt := range opts {
		opt(o)
	}

	// Build algorithm
	algo, err := buildAlgorithm(o)
	if err != nil {
		return nil, err
	}

	// Default to memory backend
	if o.backendInstance == nil {
		o.backendInstance = memory.NewMemoryBackend()
	}

	cfg := limiters.BasicConfig{
		DefaultLimit:  o.defaultLimit,
		DefaultWindow: o.defaultWindow,
		KeyTTL:        o.keyTTL,
	}

	return basic.NewBasicLimiter(algo, o.backendInstance, cfg)
}

func buildAlgorithm(o *options) (Algorithm, error) {
	switch o.algorithmType {
	case TokenBucket:
		cfg := algorithm.TokenBucketConfig{
			Capacity:     100,
			RefillRate:   100,
			RefillPeriod: time.Minute,
		}
		if o.tokenBucketConfig != nil {
			cfg = *o.tokenBucketConfig
		}
		if o.defaultLimit > 0 && o.tokenBucketConfig == nil {
			cfg.Capacity = o.defaultLimit
			cfg.RefillRate = o.defaultLimit
			cfg.RefillPeriod = o.defaultWindow
		}
		return algorithm.NewTokenBucket(cfg), nil

	case LeakyBucket:
		cfg := algorithm.LeakyBucketConfig{
			Capacity:   100,
			LeakRate:   100,
			LeakPeriod: time.Minute,
		}
		if o.leakyBucketConfig != nil {
			cfg = *o.leakyBucketConfig
		}
		if o.defaultLimit > 0 && o.leakyBucketConfig == nil {
			cfg.Capacity = o.defaultLimit
			cfg.LeakRate = o.defaultLimit
			cfg.LeakPeriod = o.defaultWindow
		}
		return algorithm.NewLeakyBucket(cfg), nil

	case FixedWindow:
		cfg := algorithm.FixedWindowConfig{
			Limit:  100,
			Window: time.Minute,
		}
		if o.fixedWindowConfig != nil {
			cfg = *o.fixedWindowConfig
		}
		if o.defaultLimit > 0 && o.fixedWindowConfig == nil {
			cfg.Limit = o.defaultLimit
			cfg.Window = o.defaultWindow
		}
		return algorithm.NewFixedWindow(cfg), nil

	case SlidingWindow:
		cfg := algorithm.SlidingWindowConfig{
			Limit:  100,
			Window: time.Minute,
		}
		if o.slidingWindowConfig != nil {
			cfg = *o.slidingWindowConfig
		}
		if o.defaultLimit > 0 && o.slidingWindowConfig == nil {
			cfg.Limit = o.defaultLimit
			cfg.Window = o.defaultWindow
		}
		return algorithm.NewSlidingWindow(cfg), nil

	case GCRA:
		cfg := algorithm.GCRAConfig{
			Period:    time.Minute,
			Limit:     100,
			BurstSize: 10,
		}
		if o.gcraConfig != nil {
			cfg = *o.gcraConfig
		}
		if o.defaultLimit > 0 && o.gcraConfig == nil {
			cfg.Limit = o.defaultLimit
			cfg.Period = o.defaultWindow
		}
		return algorithm.NewGCRA(cfg), nil

	default:
		return nil, ErrInvalidConfig
	}
}

// ---------------------------------------------------------------------------
// Convenience constructors for advanced limiter types
// ---------------------------------------------------------------------------

// NewTierBased creates a TierBasedLimiter using the builder's algorithm
// options and a TierConfig.
//
//	limiter, err := niyantrak.NewTierBased(
//	    niyantrak.TierConfig{ ... },
//	    niyantrak.WithAlgorithm(niyantrak.TokenBucket),
//	    niyantrak.WithMemoryBackend(),
//	)
func NewTierBased(cfg limiters.TierConfig, opts ...Option) (TierBasedLimiter, error) {
	o := &options{
		algorithmType: TokenBucket,
		defaultLimit:  100,
		defaultWindow: time.Minute,
		keyTTL:        time.Hour,
	}
	for _, opt := range opts {
		opt(o)
	}

	algo, err := buildAlgorithm(o)
	if err != nil {
		return nil, err
	}

	if o.backendInstance == nil {
		o.backendInstance = memory.NewMemoryBackend()
	}

	return tier.NewTierBasedLimiter(algo, o.backendInstance, cfg)
}

// NewTenantBased creates a TenantBasedLimiter using the builder's algorithm
// options and a TenantConfig.
func NewTenantBased(cfg limiters.TenantConfig, opts ...Option) (TenantBasedLimiter, error) {
	o := &options{
		algorithmType: TokenBucket,
		defaultLimit:  100,
		defaultWindow: time.Minute,
		keyTTL:        time.Hour,
	}
	for _, opt := range opts {
		opt(o)
	}

	algo, err := buildAlgorithm(o)
	if err != nil {
		return nil, err
	}

	if o.backendInstance == nil {
		o.backendInstance = memory.NewMemoryBackend()
	}

	return tenant.NewTenantBasedLimiter(algo, o.backendInstance, cfg)
}

// NewCostBased creates a CostBasedLimiter using the builder's algorithm
// options and a CostConfig.
func NewCostBased(cfg limiters.CostConfig, opts ...Option) (CostBasedLimiter, error) {
	o := &options{
		algorithmType: TokenBucket,
		defaultLimit:  100,
		defaultWindow: time.Minute,
		keyTTL:        time.Hour,
	}
	for _, opt := range opts {
		opt(o)
	}

	algo, err := buildAlgorithm(o)
	if err != nil {
		return nil, err
	}

	if o.backendInstance == nil {
		o.backendInstance = memory.NewMemoryBackend()
	}

	return cost.NewCostBasedLimiter(algo, o.backendInstance, cfg)
}

// NewComposite creates a CompositeLimiter using the builder's algorithm
// options and a CompositeConfig.
func NewComposite(cfg limiters.CompositeConfig, opts ...Option) (CompositeLimiter, error) {
	o := &options{
		algorithmType: TokenBucket,
		defaultLimit:  100,
		defaultWindow: time.Minute,
		keyTTL:        time.Hour,
	}
	for _, opt := range opts {
		opt(o)
	}

	algo, err := buildAlgorithm(o)
	if err != nil {
		return nil, err
	}

	if o.backendInstance == nil {
		o.backendInstance = memory.NewMemoryBackend()
	}

	return composite.NewCompositeLimiter(algo, o.backendInstance, cfg)
}
