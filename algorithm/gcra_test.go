package algorithm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGCRA_NewGCRA(t *testing.T) {
	config := GCRAConfig{
		Period:    100 * time.Millisecond,
		BurstSize: 10,
	}

	algo := NewGCRA(config)
	assert.Equal(t, "gcra", algo.Name())
	assert.Contains(t, algo.Description(), "GCRA")
}

func TestGCRA_ValidateConfig(t *testing.T) {
	algo := NewGCRA(GCRAConfig{})

	tests := []struct {
		name    string
		config  GCRAConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: GCRAConfig{
				Period:    100 * time.Millisecond,
				BurstSize: 10,
			},
			wantErr: false,
		},
		{
			name: "zero emission interval",
			config: GCRAConfig{
				Period:    0,
				BurstSize: 10,
			},
			wantErr: true,
		},
		{
			name: "negative emission interval",
			config: GCRAConfig{
				Period:    -time.Millisecond,
				BurstSize: 10,
			},
			wantErr: true,
		},
		{
			name: "zero burst capacity",
			config: GCRAConfig{
				Period:    100 * time.Millisecond,
				BurstSize: 0,
			},
			wantErr: true,
		},
		{
			name: "negative burst capacity",
			config: GCRAConfig{
				Period:    100 * time.Millisecond,
				BurstSize: -1,
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

func TestGCRA_Reset(t *testing.T) {
	config := GCRAConfig{
		Period:    100 * time.Millisecond,
		BurstSize: 10,
	}

	algo := NewGCRA(config)
	ctx := context.Background()

	state, err := algo.Reset(ctx)
	require.NoError(t, err)

	gcraState, ok := state.(*GCRAState)
	require.True(t, ok)
	assert.True(t, time.Since(gcraState.TAT) < time.Millisecond)
	assert.True(t, time.Since(gcraState.LastUpdate) < time.Millisecond)
}

func TestGCRA_Allow_Basic(t *testing.T) {
	config := GCRAConfig{
		Period:    100 * time.Millisecond,
		BurstSize: 10,
	}

	algo := NewGCRA(config)
	ctx := context.Background()

	// First request - should be allowed
	newState, result, err := algo.Allow(ctx, nil, 1)
	require.NoError(t, err)

	gcraResult, ok := result.(*GCRAResult)
	require.True(t, ok)
	assert.True(t, gcraResult.Allowed)
	assert.Equal(t, 100*time.Millisecond, gcraResult.Limit)

	// Second request immediately - should be allowed (burst)
	newState, result, err = algo.Allow(ctx, newState, 1)
	require.NoError(t, err)

	gcraResult, ok = result.(*GCRAResult)
	require.True(t, ok)
	assert.True(t, gcraResult.Allowed)
}

func TestGCRA_Allow_Burst(t *testing.T) {
	config := GCRAConfig{
		Period:    100 * time.Millisecond,
		BurstSize: 5,
	}

	algo := NewGCRA(config)
	ctx := context.Background()

	// Send burst of requests
	var state interface{}
	var err error
	for i := 0; i < 5; i++ {
		state, _, err = algo.Allow(ctx, state, 1)
		require.NoError(t, err)
	}

	// Next request should be denied (burst exhausted)
	_, result, err := algo.Allow(ctx, state, 1)
	require.NoError(t, err)

	gcraResult := result.(*GCRAResult)
	assert.False(t, gcraResult.Allowed)
	assert.True(t, gcraResult.RetryAfter > 0)
}

func TestGCRA_Allow_Deny(t *testing.T) {
	config := GCRAConfig{
		Period:    100 * time.Millisecond,
		BurstSize: 1, // No burst
	}

	algo := NewGCRA(config)
	ctx := context.Background()

	// First request - allowed
	newState, _, err := algo.Allow(ctx, nil, 1)
	require.NoError(t, err)

	// Second request immediately - should be denied
	_, result, err := algo.Allow(ctx, newState, 1)
	require.NoError(t, err)

	gcraResult := result.(*GCRAResult)
	assert.False(t, gcraResult.Allowed)
	assert.True(t, gcraResult.RetryAfter > 0)
}

func TestGCRA_Allow_AfterWait(t *testing.T) {
	config := GCRAConfig{
		Period:    100 * time.Millisecond,
		BurstSize: 1,
	}

	algo := NewGCRA(config)
	ctx := context.Background()

	// First request
	newState, _, err := algo.Allow(ctx, nil, 1)
	require.NoError(t, err)

	// Wait for emission interval
	time.Sleep(110 * time.Millisecond)

	// Second request should be allowed now
	_, result, err := algo.Allow(ctx, newState, 1)
	require.NoError(t, err)

	gcraResult := result.(*GCRAResult)
	assert.True(t, gcraResult.Allowed)
}

func TestGCRA_Allow_Cost(t *testing.T) {
	config := GCRAConfig{
		Period:    100 * time.Millisecond,
		BurstSize: 10,
	}

	algo := NewGCRA(config)
	ctx := context.Background()

	// Request with cost > 1
	_, result, err := algo.Allow(ctx, nil, 5)
	require.NoError(t, err)

	gcraResult, ok := result.(*GCRAResult)
	require.True(t, ok)
	assert.True(t, gcraResult.Allowed)

	// TAT should advance by 5 * emission interval
	expectedTATAdvance := 5 * config.Period
	actualTATAdvance := time.Until(gcraResult.TAT)
	// Allow some tolerance for execution time
	assert.True(t, actualTATAdvance >= expectedTATAdvance-10*time.Millisecond)
	assert.True(t, actualTATAdvance <= expectedTATAdvance+10*time.Millisecond)
}

func TestGCRA_GetStats(t *testing.T) {
	config := GCRAConfig{
		Period:    100 * time.Millisecond,
		BurstSize: 10,
	}

	algo := NewGCRA(config)
	ctx := context.Background()

	// Stats with nil state
	stats := algo.GetStats(ctx, nil)
	gcraStats, ok := stats.(*GCRAStats)
	require.True(t, ok)
	assert.Equal(t, 100*time.Millisecond, gcraStats.Period)
	assert.Equal(t, 10, gcraStats.BurstSize)
	assert.False(t, gcraStats.IsThrottled)

	// Add some requests
	newState, _, err := algo.Allow(ctx, nil, 5)
	require.NoError(t, err)

	// Stats with actual state
	stats = algo.GetStats(ctx, newState)
	gcraStats, ok = stats.(*GCRAStats)
	require.True(t, ok)
	assert.Equal(t, 100*time.Millisecond, gcraStats.Period)
	assert.Equal(t, 10, gcraStats.BurstSize)
	assert.True(t, gcraStats.TAT.After(time.Now()))
}

func TestGCRA_GetStats_InvalidState(t *testing.T) {
	algo := NewGCRA(GCRAConfig{})
	ctx := context.Background()

	stats := algo.GetStats(ctx, "invalid")
	assert.Nil(t, stats)
}

func TestGCRA_Allow_InvalidState(t *testing.T) {
	algo := NewGCRA(GCRAConfig{})
	ctx := context.Background()

	_, _, err := algo.Allow(ctx, "invalid", 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid state type")
}

func TestGCRA_BurstCapacity(t *testing.T) {
	config := GCRAConfig{
		Period:    100 * time.Millisecond,
		BurstSize: 3,
	}

	algo := NewGCRA(config)
	ctx := context.Background()

	// Should allow burst of 3 requests
	var state interface{}
	var err error
	for i := 0; i < 3; i++ {
		var result interface{}
		state, result, err = algo.Allow(ctx, state, 1)
		require.NoError(t, err)
		gcraResult := result.(*GCRAResult)
		assert.True(t, gcraResult.Allowed, "request %d should be allowed", i+1)
	}

	// 4th request should be denied
	_, result, err := algo.Allow(ctx, state, 1)
	require.NoError(t, err)
	gcraResult := result.(*GCRAResult)
	assert.False(t, gcraResult.Allowed)
}

func TestGCRA_DelayTolerance(t *testing.T) {
	config := GCRAConfig{
		Period:    100 * time.Millisecond,
		BurstSize: 5,
	}

	algo := NewGCRA(config)
	ctx := context.Background()

	stats := algo.GetStats(ctx, nil)
	gcraStats := stats.(*GCRAStats)

	// Delay tolerance should be (BurstCapacity - 1) * EmissionInterval
	expectedDelayTolerance := 4 * config.Period
	assert.Equal(t, expectedDelayTolerance, gcraStats.DelayTolerance)
}

func TestGCRA_TATProgression(t *testing.T) {
	config := GCRAConfig{
		Period:    100 * time.Millisecond,
		BurstSize: 10,
	}

	algo := NewGCRA(config)
	ctx := context.Background()

	// First request
	state1, result1, err := algo.Allow(ctx, nil, 1)
	require.NoError(t, err)
	gcraResult1 := result1.(*GCRAResult)
	tat1 := gcraResult1.TAT

	// Second request
	state2, result2, err := algo.Allow(ctx, state1, 1)
	require.NoError(t, err)
	gcraResult2 := result2.(*GCRAResult)
	tat2 := gcraResult2.TAT

	// TAT should advance by emission interval
	tatDiff := tat2.Sub(tat1)
	assert.True(t, tatDiff >= config.Period-time.Millisecond)
	assert.True(t, tatDiff <= config.Period+time.Millisecond)

	// Verify state updated
	gcraState2 := state2.(*GCRAState)
	assert.Equal(t, tat2, gcraState2.TAT)
}

func TestGCRA_RetryAfterCalculation(t *testing.T) {
	config := GCRAConfig{
		Period:    100 * time.Millisecond,
		BurstSize: 1,
	}

	algo := NewGCRA(config)
	ctx := context.Background()

	// First request
	state, _, err := algo.Allow(ctx, nil, 1)
	require.NoError(t, err)

	// Immediate second request - should be denied
	_, result, err := algo.Allow(ctx, state, 1)
	require.NoError(t, err)

	gcraResult := result.(*GCRAResult)
	assert.False(t, gcraResult.Allowed)
	// RetryAfter should be close to emission interval
	assert.True(t, gcraResult.RetryAfter > 0)
	assert.True(t, gcraResult.RetryAfter <= config.Period)
}

func TestGCRA_StateUnchangedOnDeny(t *testing.T) {
	config := GCRAConfig{
		Period:    100 * time.Millisecond,
		BurstSize: 1,
	}

	algo := NewGCRA(config)
	ctx := context.Background()

	// First request
	state1, _, err := algo.Allow(ctx, nil, 1)
	require.NoError(t, err)
	gcraState1 := state1.(*GCRAState)
	tat1 := gcraState1.TAT

	// Second request immediately - denied
	state2, result, err := algo.Allow(ctx, state1, 1)
	require.NoError(t, err)
	gcraResult := result.(*GCRAResult)
	assert.False(t, gcraResult.Allowed)

	// State should be unchanged
	gcraState2 := state2.(*GCRAState)
	assert.Equal(t, tat1, gcraState2.TAT)
}

func TestGCRA_ValidateConfig_InvalidType(t *testing.T) {
	algo := NewGCRA(GCRAConfig{})

	err := algo.ValidateConfig("invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid config type")
}

func TestGCRA_Throttled(t *testing.T) {
	config := GCRAConfig{
		Period:    100 * time.Millisecond,
		BurstSize: 2,
	}

	algo := NewGCRA(config)
	ctx := context.Background()

	// Exhaust burst
	var state interface{}
	var err error
	for i := 0; i < 2; i++ {
		state, _, err = algo.Allow(ctx, state, 1)
		require.NoError(t, err)
	}

	// Check if throttled
	stats := algo.GetStats(ctx, state)
	gcraStats := stats.(*GCRAStats)
	assert.True(t, gcraStats.IsThrottled)
}

func TestGCRA_NotThrottled(t *testing.T) {
	config := GCRAConfig{
		Period:    100 * time.Millisecond,
		BurstSize: 10,
	}

	algo := NewGCRA(config)
	ctx := context.Background()

	// Single request
	state, _, err := algo.Allow(ctx, nil, 1)
	require.NoError(t, err)

	// Should not be throttled
	stats := algo.GetStats(ctx, state)
	gcraStats := stats.(*GCRAStats)
	assert.False(t, gcraStats.IsThrottled)
}

func TestGCRA_MultipleWaits(t *testing.T) {
	config := GCRAConfig{
		Period:    50 * time.Millisecond,
		BurstSize: 2,
	}

	algo := NewGCRA(config)
	ctx := context.Background()

	// Exhaust burst
	state, _, err := algo.Allow(ctx, nil, 2)
	require.NoError(t, err)

	// Try request - should be denied
	_, result1, err := algo.Allow(ctx, state, 1)
	require.NoError(t, err)
	assert.False(t, result1.(*GCRAResult).Allowed)

	// Wait and try again
	time.Sleep(60 * time.Millisecond)
	state, result2, err := algo.Allow(ctx, state, 1)
	require.NoError(t, err)
	assert.True(t, result2.(*GCRAResult).Allowed)
}

func TestGCRA_ZeroCost(t *testing.T) {
	config := GCRAConfig{
		Period:    100 * time.Millisecond,
		BurstSize: 10,
	}

	algo := NewGCRA(config)
	ctx := context.Background()

	// Request with cost 0 should still work (TAT doesn't advance)
	_, result, err := algo.Allow(ctx, nil, 0)
	require.NoError(t, err)

	gcraResult := result.(*GCRAResult)
	assert.True(t, gcraResult.Allowed)
}
