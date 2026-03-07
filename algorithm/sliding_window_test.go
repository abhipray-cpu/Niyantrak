package algorithm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlidingWindow_NewSlidingWindow(t *testing.T) {
	config := SlidingWindowConfig{
		Window:    time.Minute,
		Limit:     100,
		Precision: time.Second,
	}

	algo := NewSlidingWindow(config)
	assert.Equal(t, "sliding_window", algo.Name())
	assert.Contains(t, algo.Description(), "Sliding Window")
}

func TestSlidingWindow_ValidateConfig(t *testing.T) {
	algo := NewSlidingWindow(SlidingWindowConfig{})

	tests := []struct {
		name    string
		config  SlidingWindowConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: SlidingWindowConfig{
				Window:    time.Minute,
				Limit:     100,
				Precision: time.Second,
			},
			wantErr: false,
		},
		{
			name: "zero window",
			config: SlidingWindowConfig{
				Window:    0,
				Limit:     100,
				Precision: time.Second,
			},
			wantErr: true,
		},
		{
			name: "zero limit",
			config: SlidingWindowConfig{
				Window:    time.Minute,
				Limit:     0,
				Precision: time.Second,
			},
			wantErr: true,
		},
		{
			name: "zero precision",
			config: SlidingWindowConfig{
				Window:    time.Minute,
				Limit:     100,
				Precision: 0,
			},
			wantErr: true,
		},
		{
			name: "precision greater than window",
			config: SlidingWindowConfig{
				Window:    time.Second,
				Limit:     100,
				Precision: time.Minute,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := algo.ValidateConfig(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSlidingWindow_Reset(t *testing.T) {
	config := SlidingWindowConfig{
		Window:    time.Minute,
		Limit:     100,
		Precision: time.Second,
	}

	algo := NewSlidingWindow(config)
	ctx := context.Background()

	state, err := algo.Reset(ctx)
	require.NoError(t, err)

	windowState, ok := state.(*SlidingWindowState)
	require.True(t, ok)
	assert.Equal(t, 0, windowState.CurrentCount)
	assert.Equal(t, 0, windowState.PreviousCount)
	assert.False(t, windowState.CurrentWindowStart.IsZero())
}

func TestSlidingWindow_Allow_Basic(t *testing.T) {
	config := SlidingWindowConfig{
		Window:    time.Minute,
		Limit:     10,
		Precision: time.Second,
	}

	algo := NewSlidingWindow(config)
	ctx := context.Background()

	// First request
	newState, result, err := algo.Allow(ctx, nil, 1)
	require.NoError(t, err)

	windowResult, ok := result.(*SlidingWindowResult)
	require.True(t, ok)
	assert.True(t, windowResult.Allowed)
	assert.Equal(t, 1, windowResult.RequestCount)
	assert.Equal(t, 10, windowResult.Limit)
	assert.Equal(t, 9, windowResult.Remaining)

	// Second request
	_, result, err = algo.Allow(ctx, newState, 1)
	require.NoError(t, err)

	windowResult, ok = result.(*SlidingWindowResult)
	require.True(t, ok)
	assert.True(t, windowResult.Allowed)
	assert.Equal(t, 2, windowResult.RequestCount)
	assert.Equal(t, 8, windowResult.Remaining)
}

func TestSlidingWindow_Allow_Deny(t *testing.T) {
	config := SlidingWindowConfig{
		Window:    time.Minute,
		Limit:     2,
		Precision: time.Second,
	}

	algo := NewSlidingWindow(config)
	ctx := context.Background()

	// Use up the limit
	newState, _, err := algo.Allow(ctx, nil, 2)
	require.NoError(t, err)

	// Next request should be denied
	_, result, err := algo.Allow(ctx, newState, 1)
	require.NoError(t, err)

	windowResult, ok := result.(*SlidingWindowResult)
	require.True(t, ok)
	assert.False(t, windowResult.Allowed)
	assert.Equal(t, 2, windowResult.RequestCount)
	assert.Equal(t, 0, windowResult.Remaining)
	assert.True(t, windowResult.RetryAfter > 0)
}

func TestSlidingWindow_Allow_Cleanup(t *testing.T) {
	config := SlidingWindowConfig{
		Window:    time.Second,
		Limit:     5,
		Precision: 100 * time.Millisecond,
	}

	// Use deterministic clock for testing old-state migration
	now := time.Now()
	clock := func() time.Time { return now }
	algo := NewSlidingWindowWithClock(config, clock)
	ctx := context.Background()

	// Create old-format state with timestamps (backward-compatibility migration test)
	oldTime := now.Add(-2 * time.Second)
	testState := &SlidingWindowState{
		Timestamps:  []time.Time{oldTime, oldTime, now.Add(-100 * time.Millisecond)},
		LastCleanup: oldTime,
		// CurrentWindowStart is zero → triggers migration
	}

	// Make a request — should migrate old state and count 1 in-window + 1 new = 2
	_, result, err := algo.Allow(ctx, testState, 1)
	require.NoError(t, err)

	windowResult := result.(*SlidingWindowResult)
	assert.True(t, windowResult.Allowed)
	// The migration should count 1 timestamp within window, then add 1 new request
	assert.Equal(t, 2, windowResult.RequestCount)
}

func TestSlidingWindow_Allow_Cost(t *testing.T) {
	config := SlidingWindowConfig{
		Window:    time.Minute,
		Limit:     100,
		Precision: time.Second,
	}

	algo := NewSlidingWindow(config)
	ctx := context.Background()

	// Request with cost > 1
	_, result, err := algo.Allow(ctx, nil, 5)
	require.NoError(t, err)

	windowResult, ok := result.(*SlidingWindowResult)
	require.True(t, ok)
	assert.True(t, windowResult.Allowed)
	assert.Equal(t, 5, windowResult.RequestCount)
	assert.Equal(t, 95, windowResult.Remaining)
}

func TestSlidingWindow_GetStats(t *testing.T) {
	config := SlidingWindowConfig{
		Window:    time.Minute,
		Limit:     100,
		Precision: time.Second,
	}

	algo := NewSlidingWindow(config)
	ctx := context.Background()

	// Stats with nil state
	stats := algo.GetStats(ctx, nil)
	windowStats, ok := stats.(*SlidingWindowStats)
	require.True(t, ok)
	assert.Equal(t, 100, windowStats.Limit)
	assert.Equal(t, time.Minute, windowStats.Window)
	assert.Equal(t, 0.0, windowStats.WindowUtilization)

	// Add some requests
	newState, _, err := algo.Allow(ctx, nil, 50)
	require.NoError(t, err)

	// Stats with actual state
	stats = algo.GetStats(ctx, newState)
	windowStats, ok = stats.(*SlidingWindowStats)
	require.True(t, ok)
	assert.Equal(t, 50, windowStats.CurrentRequestCount)
	assert.Equal(t, 100, windowStats.Limit)
	assert.Equal(t, 50.0, windowStats.WindowUtilization)
}

func TestSlidingWindow_GetStats_InvalidState(t *testing.T) {
	algo := NewSlidingWindow(SlidingWindowConfig{})
	ctx := context.Background()

	stats := algo.GetStats(ctx, "invalid")
	assert.Nil(t, stats)
}

func TestSlidingWindow_Allow_InvalidState(t *testing.T) {
	algo := NewSlidingWindow(SlidingWindowConfig{})
	ctx := context.Background()

	_, _, err := algo.Allow(ctx, "invalid", 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid state type")
}

func TestSlidingWindow_ExactLimit(t *testing.T) {
	config := SlidingWindowConfig{
		Window:    time.Minute,
		Limit:     10,
		Precision: time.Second,
	}

	algo := NewSlidingWindow(config)
	ctx := context.Background()

	// Use exactly the limit
	newState, result, err := algo.Allow(ctx, nil, 10)
	require.NoError(t, err)

	windowResult := result.(*SlidingWindowResult)
	assert.True(t, windowResult.Allowed)
	assert.Equal(t, 10, windowResult.RequestCount)
	assert.Equal(t, 0, windowResult.Remaining)

	// Next request should be denied
	_, result, err = algo.Allow(ctx, newState, 1)
	require.NoError(t, err)

	windowResult = result.(*SlidingWindowResult)
	assert.False(t, windowResult.Allowed)
}

func TestSlidingWindow_CostExceedsLimit(t *testing.T) {
	config := SlidingWindowConfig{
		Window:    time.Minute,
		Limit:     5,
		Precision: time.Second,
	}

	algo := NewSlidingWindow(config)
	ctx := context.Background()

	// Request with cost exceeding limit
	_, result, err := algo.Allow(ctx, nil, 10)
	require.NoError(t, err)

	windowResult := result.(*SlidingWindowResult)
	assert.False(t, windowResult.Allowed)
	assert.Equal(t, 0, windowResult.RequestCount)
}

func TestSlidingWindow_CleanupOldTimestamps(t *testing.T) {
	config := SlidingWindowConfig{
		Window:    time.Second,
		Limit:     10,
		Precision: 100 * time.Millisecond,
	}

	// Use deterministic clock
	now := time.Now()
	clock := func() time.Time { return now }
	algo := NewSlidingWindowWithClock(config, clock)
	ctx := context.Background()

	// Create old-format state with mix of old and recent timestamps (migration test)
	oldTimestamps := []time.Time{
		now.Add(-2 * time.Second),         // Should not count
		now.Add(-1500 * time.Millisecond), // Should not count
		now.Add(-500 * time.Millisecond),  // Should count
		now.Add(-100 * time.Millisecond),  // Should count
	}

	testState := &SlidingWindowState{
		Timestamps:  oldTimestamps,
		LastCleanup: now.Add(-time.Second),
		// CurrentWindowStart is zero → triggers migration
	}

	// Make a request — migrates old state: 2 timestamps in window + 1 new = 3
	_, result, err := algo.Allow(ctx, testState, 1)
	require.NoError(t, err)

	windowResult := result.(*SlidingWindowResult)
	assert.True(t, windowResult.Allowed)
	assert.Equal(t, 3, windowResult.RequestCount)
}

func TestSlidingWindow_RetryAfter_Calculation(t *testing.T) {
	config := SlidingWindowConfig{
		Window:    time.Second,
		Limit:     3,
		Precision: 100 * time.Millisecond,
	}

	algo := NewSlidingWindow(config)
	ctx := context.Background()

	// Fill the window
	newState, _, err := algo.Allow(ctx, nil, 3)
	require.NoError(t, err)

	// Try to add one more
	_, result, err := algo.Allow(ctx, newState, 1)
	require.NoError(t, err)

	windowResult := result.(*SlidingWindowResult)
	assert.False(t, windowResult.Allowed)
	// Should have a retry after of approximately the window duration
	assert.True(t, windowResult.RetryAfter > 0)
	assert.True(t, windowResult.RetryAfter <= config.Window)
}

func TestSlidingWindow_MultipleRequests_Sliding(t *testing.T) {
	config := SlidingWindowConfig{
		Window:    500 * time.Millisecond,
		Limit:     3,
		Precision: 50 * time.Millisecond,
	}

	now := time.Now()
	currentTime := now
	clock := func() time.Time { return currentTime }
	algo := NewSlidingWindowWithClock(config, clock)
	ctx := context.Background()

	// Add 3 requests - fill the limit
	state1, result1, err := algo.Allow(ctx, nil, 3)
	require.NoError(t, err)
	assert.True(t, result1.(*SlidingWindowResult).Allowed)

	// Try to add more - should be denied
	state2, result2, err := algo.Allow(ctx, state1, 1)
	require.NoError(t, err)
	assert.False(t, result2.(*SlidingWindowResult).Allowed)

	// Advance time past the full window so old requests drain completely
	currentTime = now.Add(600 * time.Millisecond)

	// Should allow now (old requests in previous window, but window has advanced enough)
	_, result3, err := algo.Allow(ctx, state2, 1)
	require.NoError(t, err)
	windowResult := result3.(*SlidingWindowResult)
	assert.True(t, windowResult.Allowed)
}

func TestSlidingWindow_EmptyWindow(t *testing.T) {
	config := SlidingWindowConfig{
		Window:    time.Minute,
		Limit:     10,
		Precision: time.Second,
	}

	algo := NewSlidingWindow(config)
	ctx := context.Background()

	// Create empty state using new format
	now := time.Now()
	emptyState := &SlidingWindowState{
		CurrentCount:       0,
		PreviousCount:      0,
		CurrentWindowStart: now,
	}

	// Should allow request
	_, result, err := algo.Allow(ctx, emptyState, 1)
	require.NoError(t, err)

	windowResult := result.(*SlidingWindowResult)
	assert.True(t, windowResult.Allowed)
	assert.Equal(t, 1, windowResult.RequestCount)
}

func TestSlidingWindow_ValidateConfig_InvalidType(t *testing.T) {
	algo := NewSlidingWindow(SlidingWindowConfig{})

	err := algo.ValidateConfig("invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid config type")
}

func TestSlidingWindow_HighUtilization(t *testing.T) {
	config := SlidingWindowConfig{
		Window:    time.Minute,
		Limit:     100,
		Precision: time.Second,
	}

	algo := NewSlidingWindow(config)
	ctx := context.Background()

	// Fill to 90%
	newState, _, err := algo.Allow(ctx, nil, 90)
	require.NoError(t, err)

	// Check stats
	stats := algo.GetStats(ctx, newState)
	windowStats := stats.(*SlidingWindowStats)
	assert.Equal(t, 90.0, windowStats.WindowUtilization)
}

func TestSlidingWindow_CounterAccuracy(t *testing.T) {
	config := SlidingWindowConfig{
		Window:    time.Minute,
		Limit:     10,
		Precision: time.Second,
	}

	now := time.Now()
	currentTime := now
	clock := func() time.Time { return currentTime }
	algo := NewSlidingWindowWithClock(config, clock)
	ctx := context.Background()

	// Add multiple requests one at a time
	var state interface{}
	var err error
	for i := 0; i < 5; i++ {
		state, _, err = algo.Allow(ctx, state, 1)
		require.NoError(t, err)
	}

	// Verify counter state
	windowState := state.(*SlidingWindowState)
	assert.Equal(t, 5, windowState.CurrentCount)
	assert.Equal(t, 0, windowState.PreviousCount)
}
