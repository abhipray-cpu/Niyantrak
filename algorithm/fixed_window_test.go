package algorithm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixedWindow_NewFixedWindow(t *testing.T) {
	config := FixedWindowConfig{
		Window: time.Minute,
		Limit:  100,
	}

	algo := NewFixedWindow(config)
	assert.Equal(t, "fixed_window", algo.Name())
	assert.Contains(t, algo.Description(), "Fixed Window")
}

func TestFixedWindow_ValidateConfig(t *testing.T) {
	algo := NewFixedWindow(FixedWindowConfig{})

	tests := []struct {
		name    string
		config  FixedWindowConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: FixedWindowConfig{
				Window: time.Minute,
				Limit:  100,
			},
			wantErr: false,
		},
		{
			name: "zero window size",
			config: FixedWindowConfig{
				Window: 0,
				Limit:  100,
			},
			wantErr: true,
		},
		{
			name: "negative window size",
			config: FixedWindowConfig{
				Window: -time.Second,
				Limit:  100,
			},
			wantErr: true,
		},
		{
			name: "zero limit",
			config: FixedWindowConfig{
				Window: time.Minute,
				Limit:  0,
			},
			wantErr: true,
		},
		{
			name: "negative limit",
			config: FixedWindowConfig{
				Window: time.Minute,
				Limit:  -1,
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

func TestFixedWindow_Reset(t *testing.T) {
	config := FixedWindowConfig{
		Window: time.Minute,
		Limit:  100,
	}

	algo := NewFixedWindow(config)
	ctx := context.Background()

	state, err := algo.Reset(ctx)
	require.NoError(t, err)

	windowState, ok := state.(*FixedWindowState)
	require.True(t, ok)
	assert.Equal(t, 0, windowState.RequestCount)
	// Window should be aligned, so it should be at or before now
	assert.True(t, windowState.WindowStart.Before(time.Now()) || windowState.WindowStart.Equal(time.Now()))
}

func TestFixedWindow_Allow_Basic(t *testing.T) {
	config := FixedWindowConfig{
		Window: time.Minute,
		Limit:  10,
	}

	algo := NewFixedWindow(config)
	ctx := context.Background()

	// First request - should initialize with empty window
	newState, result, err := algo.Allow(ctx, nil, 1)
	require.NoError(t, err)

	windowResult, ok := result.(*FixedWindowResult)
	require.True(t, ok)
	assert.True(t, windowResult.Allowed)
	assert.Equal(t, 1, windowResult.RequestCount)
	assert.Equal(t, 10, windowResult.Limit)
	assert.Equal(t, 9, windowResult.Remaining)

	// Second request - should have 2 requests
	_, result, err = algo.Allow(ctx, newState, 1)
	require.NoError(t, err)

	windowResult, ok = result.(*FixedWindowResult)
	require.True(t, ok)
	assert.True(t, windowResult.Allowed)
	assert.Equal(t, 2, windowResult.RequestCount)
	assert.Equal(t, 8, windowResult.Remaining)
}

func TestFixedWindow_Allow_Deny(t *testing.T) {
	config := FixedWindowConfig{
		Window: time.Minute,
		Limit:  2,
	}

	algo := NewFixedWindow(config)
	ctx := context.Background()

	// Use up the limit
	newState, _, err := algo.Allow(ctx, nil, 2)
	require.NoError(t, err)

	// Next request should be denied
	_, result, err := algo.Allow(ctx, newState, 1)
	require.NoError(t, err)

	windowResult, ok := result.(*FixedWindowResult)
	require.True(t, ok)
	assert.False(t, windowResult.Allowed)
	assert.Equal(t, 2, windowResult.RequestCount)
	assert.Equal(t, 0, windowResult.Remaining)
	assert.True(t, windowResult.RetryAfter > 0)
}

func TestFixedWindow_Allow_WindowReset(t *testing.T) {
	config := FixedWindowConfig{
		Window: time.Second,
		Limit:  5,
	}

	algo := NewFixedWindow(config)
	ctx := context.Background()

	// Use up the limit in current window
	newState, _, err := algo.Allow(ctx, nil, 5)
	require.NoError(t, err)

	// Check we're at limit
	windowState := newState.(*FixedWindowState)
	assert.Equal(t, 5, windowState.RequestCount)

	// Create state with expired window (simulate time passing)
	oldTime := time.Now().Add(-2 * time.Second)
	expiredState := &FixedWindowState{
		WindowStart:  oldTime,
		RequestCount: 5,
	}

	// Should start new window and allow request
	_, result, err := algo.Allow(ctx, expiredState, 1)
	require.NoError(t, err)

	windowResult := result.(*FixedWindowResult)
	assert.True(t, windowResult.Allowed)
	assert.Equal(t, 1, windowResult.RequestCount) // New window, count reset
}

func TestFixedWindow_Allow_Cost(t *testing.T) {
	config := FixedWindowConfig{
		Window: time.Minute,
		Limit:  100,
	}

	algo := NewFixedWindow(config)
	ctx := context.Background()

	// Request with cost > 1
	_, result, err := algo.Allow(ctx, nil, 5)
	require.NoError(t, err)

	windowResult, ok := result.(*FixedWindowResult)
	require.True(t, ok)
	assert.True(t, windowResult.Allowed)
	assert.Equal(t, 5, windowResult.RequestCount)
	assert.Equal(t, 95, windowResult.Remaining)
}

func TestFixedWindow_GetStats(t *testing.T) {
	config := FixedWindowConfig{
		Window: time.Minute,
		Limit:  100,
	}

	algo := NewFixedWindow(config)
	ctx := context.Background()

	// Stats with nil state
	stats := algo.GetStats(ctx, nil)
	windowStats, ok := stats.(*FixedWindowStats)
	require.True(t, ok)
	assert.Equal(t, 100, windowStats.Limit)
	assert.Equal(t, time.Minute, windowStats.WindowSize)
	assert.True(t, windowStats.WindowEnd.After(windowStats.WindowStart))

	// Stats with actual state
	testState := &FixedWindowState{
		WindowStart:  time.Now().Add(-30 * time.Second),
		RequestCount: 50,
	}

	stats = algo.GetStats(ctx, testState)
	windowStats, ok = stats.(*FixedWindowStats)
	require.True(t, ok)
	assert.Equal(t, 50, windowStats.CurrentRequestCount)
	assert.Equal(t, 100, windowStats.Limit)
	assert.True(t, windowStats.TimeInWindow > 0)
	assert.True(t, windowStats.TimeUntilReset > 0)
}

func TestFixedWindow_GetStats_InvalidState(t *testing.T) {
	algo := NewFixedWindow(FixedWindowConfig{})
	ctx := context.Background()

	// Invalid state type
	stats := algo.GetStats(ctx, "invalid")
	assert.Nil(t, stats)
}

func TestFixedWindow_Allow_InvalidState(t *testing.T) {
	algo := NewFixedWindow(FixedWindowConfig{})
	ctx := context.Background()

	_, _, err := algo.Allow(ctx, "invalid", 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid state type")
}

func TestFixedWindow_ResetTime(t *testing.T) {
	config := FixedWindowConfig{
		Window: time.Minute,
		Limit:  10,
	}

	algo := NewFixedWindow(config)
	ctx := context.Background()

	// Make a request
	_, result, err := algo.Allow(ctx, nil, 5)
	require.NoError(t, err)

	windowResult := result.(*FixedWindowResult)
	assert.True(t, windowResult.Allowed)

	// Reset time should be in the future
	assert.True(t, windowResult.ResetTime.After(time.Now()))

	// Reset time should be at most WindowSize away (could be less due to alignment)
	timeDiff := time.Until(windowResult.ResetTime)
	assert.True(t, timeDiff > 0 && timeDiff <= config.Window)
}

func TestFixedWindow_RetryAfter(t *testing.T) {
	config := FixedWindowConfig{
		Window: time.Minute,
		Limit:  5,
	}

	algo := NewFixedWindow(config)
	ctx := context.Background()

	// Fill the window
	newState, _, err := algo.Allow(ctx, nil, 5)
	require.NoError(t, err)

	// Try to exceed limit
	_, result, err := algo.Allow(ctx, newState, 1)
	require.NoError(t, err)

	windowResult := result.(*FixedWindowResult)
	assert.False(t, windowResult.Allowed)

	// RetryAfter should be positive and less than window size
	assert.True(t, windowResult.RetryAfter > 0)
	assert.True(t, windowResult.RetryAfter <= config.Window)
}

func TestFixedWindow_ExactLimit(t *testing.T) {
	config := FixedWindowConfig{
		Window: time.Minute,
		Limit:  10,
	}

	algo := NewFixedWindow(config)
	ctx := context.Background()

	// Use exactly the limit
	newState, result, err := algo.Allow(ctx, nil, 10)
	require.NoError(t, err)

	windowResult := result.(*FixedWindowResult)
	assert.True(t, windowResult.Allowed)
	assert.Equal(t, 10, windowResult.RequestCount)
	assert.Equal(t, 0, windowResult.Remaining)

	// Next request should be denied
	_, result, err = algo.Allow(ctx, newState, 1)
	require.NoError(t, err)

	windowResult = result.(*FixedWindowResult)
	assert.False(t, windowResult.Allowed)
}

func TestFixedWindow_CostExceedsLimit(t *testing.T) {
	config := FixedWindowConfig{
		Window: time.Minute,
		Limit:  5,
	}

	algo := NewFixedWindow(config)
	ctx := context.Background()

	// Request with cost exceeding limit
	_, result, err := algo.Allow(ctx, nil, 10)
	require.NoError(t, err)

	windowResult := result.(*FixedWindowResult)
	assert.False(t, windowResult.Allowed)
	assert.Equal(t, 0, windowResult.RequestCount) // No requests added
}

func TestFixedWindow_AlignToWindow(t *testing.T) {
	config := FixedWindowConfig{
		Window: time.Minute,
		Limit:  100,
	}

	algo := NewFixedWindow(config)
	ctx := context.Background()

	// Make two requests close in time
	state1, result1, err := algo.Allow(ctx, nil, 1)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	state2, result2, err := algo.Allow(ctx, state1, 1)
	require.NoError(t, err)

	// Both should be in same window
	windowState1 := state1.(*FixedWindowState)
	windowState2 := state2.(*FixedWindowState)
	assert.Equal(t, windowState1.WindowStart, windowState2.WindowStart)

	// Request counts should accumulate
	windowResult1 := result1.(*FixedWindowResult)
	windowResult2 := result2.(*FixedWindowResult)
	assert.Equal(t, 1, windowResult1.RequestCount)
	assert.Equal(t, 2, windowResult2.RequestCount)
}

func TestFixedWindow_WindowBoundary(t *testing.T) {
	config := FixedWindowConfig{
		Window: time.Second,
		Limit:  5,
	}

	algo := NewFixedWindow(config)
	ctx := context.Background()

	// Fill window
	newState, _, err := algo.Allow(ctx, nil, 5)
	require.NoError(t, err)

	windowState := newState.(*FixedWindowState)

	// Create state at exact window boundary
	boundaryState := &FixedWindowState{
		WindowStart:  windowState.WindowStart,
		RequestCount: 5,
	}

	// Simulate exactly at window end
	time.Sleep(time.Second)

	// Should start new window
	_, result, err := algo.Allow(ctx, boundaryState, 1)
	require.NoError(t, err)

	windowResult := result.(*FixedWindowResult)
	assert.True(t, windowResult.Allowed)
	assert.Equal(t, 1, windowResult.RequestCount) // New window
}

func TestFixedWindow_MultipleWindows(t *testing.T) {
	config := FixedWindowConfig{
		Window: 500 * time.Millisecond,
		Limit:  3,
	}

	algo := NewFixedWindow(config)
	ctx := context.Background()

	// First window - fill it
	state1, result1, err := algo.Allow(ctx, nil, 3)
	require.NoError(t, err)
	assert.True(t, result1.(*FixedWindowResult).Allowed)

	// Try to exceed in same window
	state2, result2, err := algo.Allow(ctx, state1, 1)
	require.NoError(t, err)
	assert.False(t, result2.(*FixedWindowResult).Allowed)

	// Wait for window to expire
	time.Sleep(600 * time.Millisecond)

	// Should start new window and allow request
	_, result3, err := algo.Allow(ctx, state2, 1)
	require.NoError(t, err)
	windowResult := result3.(*FixedWindowResult)
	assert.True(t, windowResult.Allowed)
	assert.Equal(t, 1, windowResult.RequestCount) // New window
}

func TestFixedWindow_ValidateConfig_InvalidType(t *testing.T) {
	algo := NewFixedWindow(FixedWindowConfig{})

	err := algo.ValidateConfig("invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid config type")
}

func TestFixedWindow_ExpiredWindow_Stats(t *testing.T) {
	config := FixedWindowConfig{
		Window: time.Second,
		Limit:  10,
	}

	algo := NewFixedWindow(config)
	ctx := context.Background()

	// Create state with expired window
	oldState := &FixedWindowState{
		WindowStart:  time.Now().Add(-2 * time.Second),
		RequestCount: 5,
	}

	stats := algo.GetStats(ctx, oldState)
	windowStats := stats.(*FixedWindowStats)

	// TimeUntilReset should be 0 or very small (window expired)
	assert.True(t, windowStats.TimeUntilReset <= 100*time.Millisecond)
}
