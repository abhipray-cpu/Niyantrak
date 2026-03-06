package algorithm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

// TokenBucketState represents the state of a token bucket
type TokenBucketState struct {
	// Tokens is the current number of tokens in the bucket
	Tokens float64

	// LastRefillTime is when tokens were last refilled
	LastRefillTime time.Time
}

// TokenBucketResult represents the result of a token bucket check
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

// TokenBucketStats represents statistics for the token bucket
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

// tokenBucket implements the Algorithm interface for Token Bucket rate limiting
type tokenBucket struct {
	config TokenBucketConfig
	clock  func() time.Time
}

// Compile-time check to ensure tokenBucket implements Algorithm interface
var _ Algorithm = (*tokenBucket)(nil)

// NewTokenBucket creates a new Token Bucket algorithm instance
func NewTokenBucket(config TokenBucketConfig) Algorithm {
	return NewTokenBucketWithClock(config, nil)
}

// NewTokenBucketWithClock creates a new Token Bucket algorithm instance with a custom clock.
// If clock is nil, time.Now is used.
func NewTokenBucketWithClock(config TokenBucketConfig, clock Clock) Algorithm {
	if clock == nil {
		clock = time.Now
	}
	return &tokenBucket{config: config, clock: clock}
}

// Allow checks if a request is allowed and updates the bucket state
func (tb *tokenBucket) Allow(ctx context.Context, state interface{}, cost int) (interface{}, interface{}, error) {
	if state == nil {
		// Initialize state
		now := tb.clock()
		initialTokens := float64(tb.config.Capacity)
		if tb.config.InitialTokens > 0 {
			initialTokens = float64(tb.config.InitialTokens)
		}
		state = &TokenBucketState{
			Tokens:         initialTokens,
			LastRefillTime: now,
		}
	}

	bucketState, ok := state.(*TokenBucketState)
	if !ok {
		return nil, nil, errors.New("invalid state type for token bucket")
	}

	now := tb.clock()

	// Refill tokens based on elapsed time
	refilledState := tb.refillTokens(*bucketState, now)

	// Check if we have enough tokens
	costFloat := float64(cost)
	if refilledState.Tokens >= costFloat {
		// Allow the request
		newState := TokenBucketState{
			Tokens:         refilledState.Tokens - costFloat,
			LastRefillTime: refilledState.LastRefillTime,
		}

		result := TokenBucketResult{
			Allowed:         true,
			RemainingTokens: newState.Tokens,
			ResetTime:       tb.calculateResetTime(newState, now),
		}

		return &newState, &result, nil
	}

	// Deny the request
	result := TokenBucketResult{
		Allowed:         false,
		RemainingTokens: refilledState.Tokens,
		ResetTime:       tb.calculateResetTime(refilledState, now),
		RetryAfter:      tb.calculateRetryAfter(refilledState, costFloat, now),
	}

	return &refilledState, &result, nil
}

// Reset resets the token bucket to initial state
func (tb *tokenBucket) Reset(ctx context.Context) (interface{}, error) {
	now := tb.clock()
	initialTokens := float64(tb.config.Capacity)
	if tb.config.InitialTokens > 0 {
		initialTokens = float64(tb.config.InitialTokens)
	}

	state := &TokenBucketState{
		Tokens:         initialTokens,
		LastRefillTime: now,
	}

	return state, nil
}

// GetStats returns current statistics for the token bucket
func (tb *tokenBucket) GetStats(ctx context.Context, state interface{}) interface{} {
	if state == nil {
		return &TokenBucketStats{
			Capacity:     float64(tb.config.Capacity),
			RefillRate:   float64(tb.config.RefillRate),
			RefillPeriod: tb.config.RefillPeriod,
		}
	}

	bucketState, ok := state.(*TokenBucketState)
	if !ok {
		return nil
	}

	nextRefill := bucketState.LastRefillTime.Add(tb.config.RefillPeriod)

	return &TokenBucketStats{
		CurrentTokens:  bucketState.Tokens,
		Capacity:       float64(tb.config.Capacity),
		RefillRate:     float64(tb.config.RefillRate),
		RefillPeriod:   tb.config.RefillPeriod,
		LastRefillTime: bucketState.LastRefillTime,
		NextRefillTime: nextRefill,
	}
}

// ValidateConfig validates the token bucket configuration
func (tb *tokenBucket) ValidateConfig(config interface{}) error {
	tbConfig, ok := config.(TokenBucketConfig)
	if !ok {
		return errors.New("invalid config type for token bucket")
	}

	if tbConfig.Capacity <= 0 {
		return errors.New("capacity must be positive")
	}

	if tbConfig.RefillRate <= 0 {
		return errors.New("refill rate must be positive")
	}

	if tbConfig.RefillPeriod <= 0 {
		return errors.New("refill period must be positive")
	}

	if tbConfig.InitialTokens < 0 {
		return errors.New("initial tokens cannot be negative")
	}

	if tbConfig.InitialTokens > tbConfig.Capacity {
		return errors.New("initial tokens cannot exceed capacity")
	}

	return nil
}

// Name returns the algorithm name
func (tb *tokenBucket) Name() string {
	return "token_bucket"
}

// Description returns a human-readable description
func (tb *tokenBucket) Description() string {
	return fmt.Sprintf("Token Bucket: capacity=%d, refill_rate=%d/%v",
		tb.config.Capacity, tb.config.RefillRate, tb.config.RefillPeriod)
}

// refillTokens calculates how many tokens should be added based on elapsed time
func (tb *tokenBucket) refillTokens(state TokenBucketState, now time.Time) TokenBucketState {
	elapsed := now.Sub(state.LastRefillTime)
	if elapsed <= 0 {
		return state
	}

	// Calculate how many refill periods have passed
	refillPeriods := float64(elapsed) / float64(tb.config.RefillPeriod)
	tokensToAdd := refillPeriods * float64(tb.config.RefillRate)

	newTokens := math.Min(float64(tb.config.Capacity), state.Tokens+tokensToAdd)

	return TokenBucketState{
		Tokens:         newTokens,
		LastRefillTime: now,
	}
}

// calculateResetTime calculates when the bucket will have tokens again
func (tb *tokenBucket) calculateResetTime(state TokenBucketState, now time.Time) time.Time {
	if state.Tokens >= float64(tb.config.Capacity) {
		// Bucket is full, next reset is next refill period
		return state.LastRefillTime.Add(tb.config.RefillPeriod)
	}

	// Calculate when bucket will be full
	tokensNeeded := float64(tb.config.Capacity) - state.Tokens
	timeNeeded := time.Duration((tokensNeeded / float64(tb.config.RefillRate)) * float64(tb.config.RefillPeriod))

	return now.Add(timeNeeded)
}

// calculateRetryAfter calculates how long to wait before retrying
func (tb *tokenBucket) calculateRetryAfter(state TokenBucketState, cost float64, _ time.Time) time.Duration {
	if state.Tokens >= cost {
		return 0
	}

	// Calculate tokens needed
	tokensNeeded := cost - state.Tokens

	// Calculate time needed to get enough tokens
	timeNeeded := time.Duration((tokensNeeded / float64(tb.config.RefillRate)) * float64(tb.config.RefillPeriod))

	return timeNeeded
}
