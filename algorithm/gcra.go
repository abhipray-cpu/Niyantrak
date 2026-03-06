package algorithm

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// GCRAState represents the state of a GCRA limiter
type GCRAState struct {
	// TAT (Theoretical Arrival Time) is the virtual scheduled time
	TAT time.Time

	// LastUpdate is when the state was last updated
	LastUpdate time.Time
}

// GCRAResult represents the result of a GCRA check
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

// GCRAStats represents statistics for GCRA
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

// gcra implements the Algorithm interface for GCRA rate limiting
type gcra struct {
	config GCRAConfig
	clock  func() time.Time
}

// Compile-time check to ensure gcra implements Algorithm interface
var _ Algorithm = (*gcra)(nil)

// NewGCRA creates a new GCRA algorithm instance
func NewGCRA(config GCRAConfig) Algorithm {
	return NewGCRAWithClock(config, nil)
}

// NewGCRAWithClock creates a new GCRA algorithm instance with a custom clock.
// If clock is nil, time.Now is used.
func NewGCRAWithClock(config GCRAConfig, clock Clock) Algorithm {
	if clock == nil {
		clock = time.Now
	}
	return &gcra{config: config, clock: clock}
}

// Allow checks if a request is allowed and updates the GCRA state
func (g *gcra) Allow(ctx context.Context, state interface{}, cost int) (interface{}, interface{}, error) {
	now := g.clock()

	if state == nil {
		// Initialize state with TAT at current time
		state = &GCRAState{
			TAT:        now,
			LastUpdate: now,
		}
	}

	gcraState, ok := state.(*GCRAState)
	if !ok {
		return nil, nil, errors.New("invalid state type for GCRA")
	}

	// Calculate emission interval (time between requests)
	emissionInterval := g.config.Period

	// Calculate delay tolerance (burst capacity)
	// DelayTolerance = Period * (BurstSize - 1)
	delayTolerance := emissionInterval * time.Duration(g.config.BurstSize-1)

	// Calculate TAT for multiple costs
	tatIncrement := emissionInterval * time.Duration(cost)

	// GCRA algorithm:
	// 1. Check current TAT - if it's in the past, we can use now
	currentTAT := gcraState.TAT
	if now.After(currentTAT) {
		currentTAT = now
	}

	// 2. Check if request is conforming
	// Request is allowed if: currentTAT <= now + DelayTolerance
	allowanceLimit := now.Add(delayTolerance)

	if currentTAT.Before(allowanceLimit) || currentTAT.Equal(allowanceLimit) {
		// Request is conforming - allow it
		// Calculate new TAT
		newTAT := currentTAT.Add(tatIncrement)

		newState := GCRAState{
			TAT:        newTAT,
			LastUpdate: now,
		}

		result := GCRAResult{
			Allowed:    true,
			TAT:        newTAT,
			TimeToAct:  now,
			Limit:      emissionInterval,
			RetryAfter: 0,
		}

		return &newState, &result, nil
	}

	// Request is non-conforming - deny it
	// Calculate how long to wait
	retryAfter := currentTAT.Sub(allowanceLimit)
	if retryAfter < 0 {
		retryAfter = 0
	}

	result := GCRAResult{
		Allowed:    false,
		TAT:        gcraState.TAT,
		TimeToAct:  now.Add(retryAfter),
		Limit:      emissionInterval,
		RetryAfter: retryAfter,
	}

	// Return existing state (unchanged for non-conforming requests)
	return gcraState, &result, nil
}

// Reset resets the GCRA to initial state
func (g *gcra) Reset(ctx context.Context) (interface{}, error) {
	now := g.clock()
	state := &GCRAState{
		TAT:        now,
		LastUpdate: now,
	}

	return state, nil
}

// GetStats returns current statistics for GCRA
func (g *gcra) GetStats(ctx context.Context, state interface{}) interface{} {
	now := g.clock()
	emissionInterval := g.config.Period
	delayTolerance := emissionInterval * time.Duration(g.config.BurstSize-1)

	if state == nil {
		return &GCRAStats{
			TAT:             now,
			Period:          emissionInterval,
			BurstSize:       g.config.BurstSize,
			DelayTolerance:  delayTolerance,
			NextAllowedTime: now,
			IsThrottled:     false,
		}
	}

	gcraState, ok := state.(*GCRAState)
	if !ok {
		return nil
	}

	// Calculate next allowed time
	nextAllowed := gcraState.TAT
	if now.After(nextAllowed) {
		nextAllowed = now
	}

	// Check if throttled
	isThrottled := gcraState.TAT.After(now.Add(delayTolerance))

	return &GCRAStats{
		TAT:             gcraState.TAT,
		Period:          emissionInterval,
		BurstSize:       g.config.BurstSize,
		DelayTolerance:  delayTolerance,
		NextAllowedTime: nextAllowed,
		IsThrottled:     isThrottled,
	}
}

// ValidateConfig validates the GCRA configuration
func (g *gcra) ValidateConfig(config interface{}) error {
	gcraConfig, ok := config.(GCRAConfig)
	if !ok {
		return errors.New("invalid config type for GCRA")
	}

	if gcraConfig.Period <= 0 {
		return errors.New("emission interval must be positive")
	}

	if gcraConfig.BurstSize <= 0 {
		return errors.New("burst capacity must be positive")
	}

	return nil
}

// Name returns the algorithm name
func (g *gcra) Name() string {
	return "gcra"
}

// Description returns a human-readable description
func (g *gcra) Description() string {
	return fmt.Sprintf("GCRA: period=%v, burst_size=%d",
		g.config.Period, g.config.BurstSize)
}
