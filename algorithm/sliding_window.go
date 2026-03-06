package algorithm

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// SlidingWindowState represents the state of a sliding window using two-window
// counter approximation. This uses O(1) memory instead of O(n) per key.
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

// SlidingWindowResult represents the result of a sliding window check
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

// SlidingWindowStats represents statistics for the sliding window
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

// slidingWindow implements the Algorithm interface for Sliding Window rate limiting
// using the two-window counter approximation for O(1) memory per key.
//
// Algorithm: divide time into fixed windows of size W. Track count in the current
// window (C_curr) and the previous window (C_prev). The estimated request count is:
//
//	estimate = C_prev * overlapRatio + C_curr
//
// where overlapRatio = (W - elapsed) / W, and elapsed is time since the current
// window started.
type slidingWindow struct {
	config SlidingWindowConfig
	clock  func() time.Time
}

// Compile-time check to ensure slidingWindow implements Algorithm interface
var _ Algorithm = (*slidingWindow)(nil)

// NewSlidingWindow creates a new Sliding Window algorithm instance
func NewSlidingWindow(config SlidingWindowConfig) Algorithm {
	return NewSlidingWindowWithClock(config, nil)
}

// NewSlidingWindowWithClock creates a new Sliding Window algorithm instance with a custom clock.
// If clock is nil, time.Now is used.
func NewSlidingWindowWithClock(config SlidingWindowConfig, clock Clock) Algorithm {
	if clock == nil {
		clock = time.Now
	}
	return &slidingWindow{config: config, clock: clock}
}

// migrateState handles backward-compatible migration from old O(n) state to new O(1) state.
func (sw *slidingWindow) migrateState(s *SlidingWindowState, now time.Time) *SlidingWindowState {
	if !s.CurrentWindowStart.IsZero() {
		return s // already new format
	}
	// Old format: Timestamps + LastCleanup. Migrate by counting timestamps in window.
	windowStart := now.Add(-sw.config.Window)
	count := 0
	for _, ts := range s.Timestamps {
		if !ts.Before(windowStart) {
			count++
		}
	}
	return &SlidingWindowState{
		CurrentCount:       count,
		PreviousCount:      0,
		CurrentWindowStart: sw.alignToWindow(now),
	}
}

// alignToWindow aligns a timestamp to the start of its fixed window.
func (sw *slidingWindow) alignToWindow(t time.Time) time.Time {
	nanos := t.UnixNano()
	windowNanos := sw.config.Window.Nanoseconds()
	if windowNanos <= 0 {
		return t
	}
	aligned := (nanos / windowNanos) * windowNanos
	return time.Unix(0, aligned)
}

// advanceWindows advances the state if the current window has passed.
func (sw *slidingWindow) advanceWindows(s *SlidingWindowState, now time.Time) *SlidingWindowState {
	windowEnd := s.CurrentWindowStart.Add(sw.config.Window)

	if now.Before(windowEnd) {
		// Still in the current window
		return s
	}

	nextWindowEnd := windowEnd.Add(sw.config.Window)
	if now.Before(nextWindowEnd) {
		// We've moved exactly one window forward
		return &SlidingWindowState{
			CurrentCount:       0,
			PreviousCount:      s.CurrentCount,
			CurrentWindowStart: windowEnd,
		}
	}

	// Two or more windows have passed — everything is stale
	return &SlidingWindowState{
		CurrentCount:       0,
		PreviousCount:      0,
		CurrentWindowStart: sw.alignToWindow(now),
	}
}

// estimateCount returns the weighted estimate of requests in the sliding window.
func (sw *slidingWindow) estimateCount(s *SlidingWindowState, now time.Time) int {
	elapsed := now.Sub(s.CurrentWindowStart)
	windowDur := sw.config.Window

	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > windowDur {
		elapsed = windowDur
	}

	overlapRatio := float64(windowDur-elapsed) / float64(windowDur)
	estimate := float64(s.PreviousCount)*overlapRatio + float64(s.CurrentCount)

	// Truncate to int (floor) for conservative counting
	return int(estimate)
}

// Allow checks if a request is allowed and updates the sliding window state
func (sw *slidingWindow) Allow(ctx context.Context, state interface{}, cost int) (interface{}, interface{}, error) {
	now := sw.clock()

	if state == nil {
		state = &SlidingWindowState{
			CurrentCount:       0,
			PreviousCount:      0,
			CurrentWindowStart: sw.alignToWindow(now),
		}
	}

	windowState, ok := state.(*SlidingWindowState)
	if !ok {
		return nil, nil, errors.New("invalid state type for sliding window")
	}

	// Migrate old O(n) state if needed
	windowState = sw.migrateState(windowState, now)

	// Advance windows if time has moved past current window
	windowState = sw.advanceWindows(windowState, now)

	// Estimate current count
	currentEstimate := sw.estimateCount(windowState, now)

	if currentEstimate+cost <= sw.config.Limit {
		// Allow the request
		newState := &SlidingWindowState{
			CurrentCount:       windowState.CurrentCount + cost,
			PreviousCount:      windowState.PreviousCount,
			CurrentWindowStart: windowState.CurrentWindowStart,
		}

		newEstimate := sw.estimateCount(newState, now)
		remaining := sw.config.Limit - newEstimate
		if remaining < 0 {
			remaining = 0
		}

		result := SlidingWindowResult{
			Allowed:         true,
			RequestCount:    newEstimate,
			Limit:           sw.config.Limit,
			Remaining:       remaining,
			OldestTimestamp: now.Add(-sw.config.Window),
		}

		return newState, &result, nil
	}

	// Deny the request
	remaining := sw.config.Limit - currentEstimate
	if remaining < 0 {
		remaining = 0
	}

	retryAfter := sw.calculateRetryAfter(windowState, cost, now)

	result := SlidingWindowResult{
		Allowed:         false,
		RequestCount:    currentEstimate,
		Limit:           sw.config.Limit,
		Remaining:       0,
		OldestTimestamp: now.Add(-sw.config.Window),
		RetryAfter:      retryAfter,
	}

	return windowState, &result, nil
}

// Reset resets the sliding window to initial state
func (sw *slidingWindow) Reset(ctx context.Context) (interface{}, error) {
	now := sw.clock()
	state := &SlidingWindowState{
		CurrentCount:       0,
		PreviousCount:      0,
		CurrentWindowStart: sw.alignToWindow(now),
	}

	return state, nil
}

// GetStats returns current statistics for the sliding window
func (sw *slidingWindow) GetStats(ctx context.Context, state interface{}) interface{} {
	if state == nil {
		return &SlidingWindowStats{
			Limit:             sw.config.Limit,
			Window:            sw.config.Window,
			WindowUtilization: 0,
		}
	}

	windowState, ok := state.(*SlidingWindowState)
	if !ok {
		return nil
	}

	now := sw.clock()
	windowState = sw.migrateState(windowState, now)
	windowState = sw.advanceWindows(windowState, now)
	estimate := sw.estimateCount(windowState, now)

	utilization := 0.0
	if sw.config.Limit > 0 {
		utilization = float64(estimate) / float64(sw.config.Limit) * 100.0
	}

	return &SlidingWindowStats{
		CurrentRequestCount: estimate,
		Limit:               sw.config.Limit,
		Window:              sw.config.Window,
		OldestTimestamp:     now.Add(-sw.config.Window),
		NewestTimestamp:     now,
		WindowUtilization:   utilization,
	}
}

// ValidateConfig validates the sliding window configuration
func (sw *slidingWindow) ValidateConfig(config interface{}) error {
	swConfig, ok := config.(SlidingWindowConfig)
	if !ok {
		return errors.New("invalid config type for sliding window")
	}

	if swConfig.Window <= 0 {
		return errors.New("window must be positive")
	}

	if swConfig.Limit <= 0 {
		return errors.New("limit must be positive")
	}

	if swConfig.Precision <= 0 {
		return errors.New("precision must be positive")
	}

	if swConfig.Precision > swConfig.Window {
		return errors.New("precision cannot be greater than window")
	}

	return nil
}

// Name returns the algorithm name
func (sw *slidingWindow) Name() string {
	return "sliding_window"
}

// Description returns a human-readable description
func (sw *slidingWindow) Description() string {
	return fmt.Sprintf("Sliding Window: limit=%d per %v (precision=%v)",
		sw.config.Limit, sw.config.Window, sw.config.Precision)
}

// calculateRetryAfter estimates how long to wait before retrying.
// With the two-window counter, we estimate when enough "previous" weight will
// drain to free up capacity.
func (sw *slidingWindow) calculateRetryAfter(s *SlidingWindowState, cost int, now time.Time) time.Duration {
	estimate := sw.estimateCount(s, now)
	excess := (estimate + cost) - sw.config.Limit
	if excess <= 0 {
		return 0
	}

	// The previous-window contribution decreases linearly as elapsed increases.
	// At elapsed=Window, overlapRatio=0 and only CurrentCount matters.
	// We need: PreviousCount * ((Window - (elapsed+dt)) / Window) + CurrentCount + cost <= Limit
	// Solving for dt when PreviousCount > 0:
	if s.PreviousCount > 0 {
		// How much the previous count needs to decay
		needed := float64(excess)
		// Previous weight decays at rate PreviousCount/Window per unit time
		decayRate := float64(s.PreviousCount) / float64(sw.config.Window)
		if decayRate > 0 {
			dt := time.Duration(needed / decayRate)
			if dt < time.Millisecond {
				dt = time.Millisecond
			}
			return dt
		}
	}

	// If no previous count, we must wait for the window to roll over
	elapsed := now.Sub(s.CurrentWindowStart)
	remaining := sw.config.Window - elapsed
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}
