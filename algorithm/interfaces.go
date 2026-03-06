// Package algorithm provides rate limiting algorithm interfaces
package algorithm

import (
	"context"
	"time"
)

// Algorithm represents a rate limiting algorithm implementation
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

// TokenBucketConfig configures the Token Bucket algorithm
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

// LeakyBucketConfig configures the Leaky Bucket algorithm
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

// FixedWindowConfig configures the Fixed Window algorithm
type FixedWindowConfig struct {
	// Limit is the maximum number of requests per window
	Limit int

	// Window is the duration of each time window
	Window time.Duration

	// WindowAlignment aligns windows to clock boundaries (minute, hour, etc.)
	WindowAlignment string
}

// SlidingWindowConfig configures the Sliding Window algorithm
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

// GCRAConfig configures the GCRA (Generic Cell Rate Algorithm)
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

// Clock is a function that returns the current time.
// Algorithms accept an optional Clock for deterministic testing.
// When nil, time.Now is used.
type Clock func() time.Time

// AlgorithmFactory creates algorithm instances
type AlgorithmFactory interface {
	// Create creates a new algorithm instance with the given config
	Create(config interface{}) (Algorithm, error)

	// Type returns the algorithm type this factory creates
	Type() string
}

// AlgorithmRegistry manages algorithm factories
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
