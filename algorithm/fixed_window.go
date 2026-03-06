package algorithm

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// FixedWindowState represents the state of a fixed window
type FixedWindowState struct {
	// WindowStart is the start time of the current window
	WindowStart time.Time

	// RequestCount is the number of requests in the current window
	RequestCount int
}

// FixedWindowResult represents the result of a fixed window check
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

// FixedWindowStats represents statistics for the fixed window
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

// fixedWindow implements the Algorithm interface for Fixed Window rate limiting
type fixedWindow struct {
	config FixedWindowConfig
	clock  func() time.Time
}

// Compile-time check to ensure fixedWindow implements Algorithm interface
var _ Algorithm = (*fixedWindow)(nil)

// NewFixedWindow creates a new Fixed Window algorithm instance
func NewFixedWindow(config FixedWindowConfig) Algorithm {
	return NewFixedWindowWithClock(config, nil)
}

// NewFixedWindowWithClock creates a new Fixed Window algorithm instance with a custom clock.
// If clock is nil, time.Now is used.
func NewFixedWindowWithClock(config FixedWindowConfig, clock Clock) Algorithm {
	if clock == nil {
		clock = time.Now
	}
	return &fixedWindow{config: config, clock: clock}
}

// Allow checks if a request is allowed and updates the window state
func (fw *fixedWindow) Allow(ctx context.Context, state interface{}, cost int) (interface{}, interface{}, error) {
	now := fw.clock()

	if state == nil {
		// Initialize state with current window
		state = &FixedWindowState{
			WindowStart:  fw.alignToWindow(now),
			RequestCount: 0,
		}
	}

	windowState, ok := state.(*FixedWindowState)
	if !ok {
		return nil, nil, errors.New("invalid state type for fixed window")
	}

	// Check if we need to reset to a new window
	windowEnd := windowState.WindowStart.Add(fw.config.Window)
	if now.After(windowEnd) || now.Equal(windowEnd) {
		// Start a new window
		windowState = &FixedWindowState{
			WindowStart:  fw.alignToWindow(now),
			RequestCount: 0,
		}
	}

	// Check if adding this request would exceed the limit
	if windowState.RequestCount+cost <= fw.config.Limit {
		// Allow the request
		newState := FixedWindowState{
			WindowStart:  windowState.WindowStart,
			RequestCount: windowState.RequestCount + cost,
		}

		result := FixedWindowResult{
			Allowed:      true,
			RequestCount: newState.RequestCount,
			Limit:        fw.config.Limit,
			Remaining:    fw.config.Limit - newState.RequestCount,
			ResetTime:    windowState.WindowStart.Add(fw.config.Window),
		}

		return &newState, &result, nil
	}

	// Deny the request - limit exceeded
	result := FixedWindowResult{
		Allowed:      false,
		RequestCount: windowState.RequestCount,
		Limit:        fw.config.Limit,
		Remaining:    0,
		ResetTime:    windowState.WindowStart.Add(fw.config.Window),
		RetryAfter:   windowState.WindowStart.Add(fw.config.Window).Sub(now),
	}

	return windowState, &result, nil
}

// Reset resets the fixed window to initial state
func (fw *fixedWindow) Reset(ctx context.Context) (interface{}, error) {
	now := fw.clock()
	state := &FixedWindowState{
		WindowStart:  fw.alignToWindow(now),
		RequestCount: 0,
	}

	return state, nil
}

// GetStats returns current statistics for the fixed window
func (fw *fixedWindow) GetStats(ctx context.Context, state interface{}) interface{} {
	if state == nil {
		now := fw.clock()
		return &FixedWindowStats{
			Limit:       fw.config.Limit,
			WindowSize:  fw.config.Window,
			WindowStart: fw.alignToWindow(now),
			WindowEnd:   fw.alignToWindow(now).Add(fw.config.Window),
		}
	}

	windowState, ok := state.(*FixedWindowState)
	if !ok {
		return nil
	}

	now := fw.clock()
	windowEnd := windowState.WindowStart.Add(fw.config.Window)
	timeInWindow := now.Sub(windowState.WindowStart)
	timeUntilReset := windowEnd.Sub(now)

	if timeUntilReset < 0 {
		timeUntilReset = 0
	}

	return &FixedWindowStats{
		CurrentRequestCount: windowState.RequestCount,
		Limit:               fw.config.Limit,
		WindowSize:          fw.config.Window,
		WindowStart:         windowState.WindowStart,
		WindowEnd:           windowEnd,
		TimeInWindow:        timeInWindow,
		TimeUntilReset:      timeUntilReset,
	}
}

// ValidateConfig validates the fixed window configuration
func (fw *fixedWindow) ValidateConfig(config interface{}) error {
	fwConfig, ok := config.(FixedWindowConfig)
	if !ok {
		return errors.New("invalid config type for fixed window")
	}

	if fwConfig.Window <= 0 {
		return errors.New("window size must be positive")
	}

	if fwConfig.Limit <= 0 {
		return errors.New("limit must be positive")
	}

	return nil
}

// Name returns the algorithm name
func (fw *fixedWindow) Name() string {
	return "fixed_window"
}

// Description returns a human-readable description
func (fw *fixedWindow) Description() string {
	return fmt.Sprintf("Fixed Window: limit=%d per %v",
		fw.config.Limit, fw.config.Window)
}

// alignToWindow aligns a timestamp to the start of its window
// This ensures consistent window boundaries across distributed systems
func (fw *fixedWindow) alignToWindow(t time.Time) time.Time {
	// Get Unix timestamp in nanoseconds
	nanos := t.UnixNano()

	// Calculate window size in nanoseconds
	windowNanos := fw.config.Window.Nanoseconds()

	// Align to window boundary
	alignedNanos := (nanos / windowNanos) * windowNanos

	return time.Unix(0, alignedNanos)
}
