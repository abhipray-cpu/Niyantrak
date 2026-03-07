package algorithm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLeakyBucket_NewLeakyBucket(t *testing.T) {
	config := LeakyBucketConfig{
		Capacity:   100,
		LeakRate:   10,
		LeakPeriod: time.Second,
	}

	algo := NewLeakyBucket(config)
	assert.Equal(t, "leaky_bucket", algo.Name())
	assert.Contains(t, algo.Description(), "Leaky Bucket")
}

func TestLeakyBucket_ValidateConfig(t *testing.T) {
	algo := NewLeakyBucket(LeakyBucketConfig{})

	tests := []struct {
		name    string
		config  LeakyBucketConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: LeakyBucketConfig{
				Capacity:   100,
				LeakRate:   10,
				LeakPeriod: time.Second,
			},
			wantErr: false,
		},
		{
			name: "zero capacity",
			config: LeakyBucketConfig{
				Capacity:   0,
				LeakRate:   10,
				LeakPeriod: time.Second,
			},
			wantErr: true,
		},
		{
			name: "negative capacity",
			config: LeakyBucketConfig{
				Capacity:   -1,
				LeakRate:   10,
				LeakPeriod: time.Second,
			},
			wantErr: true,
		},
		{
			name: "zero leak rate",
			config: LeakyBucketConfig{
				Capacity:   100,
				LeakRate:   0,
				LeakPeriod: time.Second,
			},
			wantErr: true,
		},
		{
			name: "zero leak period",
			config: LeakyBucketConfig{
				Capacity:   100,
				LeakRate:   10,
				LeakPeriod: 0,
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

func TestLeakyBucket_Reset(t *testing.T) {
	config := LeakyBucketConfig{
		Capacity:   100,
		LeakRate:   10,
		LeakPeriod: time.Second,
	}

	algo := NewLeakyBucket(config)
	ctx := context.Background()

	state, err := algo.Reset(ctx)
	require.NoError(t, err)

	bucketState, ok := state.(*LeakyBucketState)
	require.True(t, ok)
	assert.Equal(t, 0, bucketState.QueueSize)
	assert.True(t, time.Since(bucketState.LastLeakTime) < time.Millisecond)
}

func TestLeakyBucket_Allow_Basic(t *testing.T) {
	config := LeakyBucketConfig{
		Capacity:   10,
		LeakRate:   1,
		LeakPeriod: time.Second,
	}

	algo := NewLeakyBucket(config)
	ctx := context.Background()

	// First request - should initialize with empty queue
	newState, result, err := algo.Allow(ctx, nil, 1)
	require.NoError(t, err)

	bucketResult, ok := result.(*LeakyBucketResult)
	require.True(t, ok)
	assert.True(t, bucketResult.Allowed)
	assert.Equal(t, 1, bucketResult.QueueSize)
	assert.Equal(t, 10, bucketResult.QueueCapacity)

	// Second request - should have 2 in queue
	_, result, err = algo.Allow(ctx, newState, 1)
	require.NoError(t, err)

	bucketResult, ok = result.(*LeakyBucketResult)
	require.True(t, ok)
	assert.True(t, bucketResult.Allowed)
	assert.Equal(t, 2, bucketResult.QueueSize)
}

func TestLeakyBucket_Allow_Deny(t *testing.T) {
	config := LeakyBucketConfig{
		Capacity:   2,
		LeakRate:   1,
		LeakPeriod: time.Second,
	}

	algo := NewLeakyBucket(config)
	ctx := context.Background()

	// Fill the queue
	newState, _, err := algo.Allow(ctx, nil, 2)
	require.NoError(t, err)

	// Next request should be denied
	_, result, err := algo.Allow(ctx, newState, 1)
	require.NoError(t, err)

	bucketResult, ok := result.(*LeakyBucketResult)
	require.True(t, ok)
	assert.False(t, bucketResult.Allowed)
	assert.Equal(t, 2, bucketResult.QueueSize)
	assert.True(t, bucketResult.RetryAfter > 0)
}

func TestLeakyBucket_Allow_Leak(t *testing.T) {
	config := LeakyBucketConfig{
		Capacity:   10,
		LeakRate:   5,
		LeakPeriod: time.Second,
	}

	algo := NewLeakyBucket(config)
	ctx := context.Background()

	// Fill the queue
	newState, _, err := algo.Allow(ctx, nil, 10)
	require.NoError(t, err)

	// Check state has 10 in queue
	bucketState := newState.(*LeakyBucketState)
	assert.Equal(t, 10, bucketState.QueueSize)

	// Simulate time passing (1 second = 5 requests leaked)
	oldTime := time.Now().Add(-time.Second)
	testState := &LeakyBucketState{
		QueueSize:    10,
		LastLeakTime: oldTime,
	}

	_, result, err := algo.Allow(ctx, testState, 3)
	require.NoError(t, err)

	bucketResult := result.(*LeakyBucketResult)
	assert.True(t, bucketResult.Allowed)
	assert.Equal(t, 8, bucketResult.QueueSize) // 10 - 5 leaked + 3 added = 8
}

func TestLeakyBucket_Allow_Cost(t *testing.T) {
	config := LeakyBucketConfig{
		Capacity:   100,
		LeakRate:   10,
		LeakPeriod: time.Second,
	}

	algo := NewLeakyBucket(config)
	ctx := context.Background()

	// Request with cost > 1
	_, result, err := algo.Allow(ctx, nil, 5)
	require.NoError(t, err)

	bucketResult, ok := result.(*LeakyBucketResult)
	require.True(t, ok)
	assert.True(t, bucketResult.Allowed)
	assert.Equal(t, 5, bucketResult.QueueSize)
}

func TestLeakyBucket_GetStats(t *testing.T) {
	config := LeakyBucketConfig{
		Capacity:   100,
		LeakRate:   10,
		LeakPeriod: time.Second,
	}

	algo := NewLeakyBucket(config)
	ctx := context.Background()

	// Stats with nil state
	stats := algo.GetStats(ctx, nil)
	bucketStats, ok := stats.(*LeakyBucketStats)
	require.True(t, ok)
	assert.Equal(t, 100, bucketStats.Capacity)
	assert.Equal(t, 10, bucketStats.LeakRate)
	assert.Equal(t, time.Second, bucketStats.LeakPeriod)

	// Stats with actual state
	testState := &LeakyBucketState{
		QueueSize:    50,
		LastLeakTime: time.Now().Add(-500 * time.Millisecond),
	}

	stats = algo.GetStats(ctx, testState)
	bucketStats, ok = stats.(*LeakyBucketStats)
	require.True(t, ok)
	assert.Equal(t, 50, bucketStats.CurrentQueueSize)
	assert.Equal(t, 100, bucketStats.Capacity)
	assert.True(t, bucketStats.NextLeakTime.After(bucketStats.LastLeakTime))
	assert.True(t, bucketStats.EstimatedWaitTime > 0)
}

func TestLeakyBucket_GetStats_InvalidState(t *testing.T) {
	algo := NewLeakyBucket(LeakyBucketConfig{})
	ctx := context.Background()

	// Invalid state type
	stats := algo.GetStats(ctx, "invalid")
	assert.Nil(t, stats)
}

func TestLeakyBucket_Allow_InvalidState(t *testing.T) {
	algo := NewLeakyBucket(LeakyBucketConfig{})
	ctx := context.Background()

	_, _, err := algo.Allow(ctx, "invalid", 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid state type")
}

func TestLeakyBucket_CalculateResetTime(t *testing.T) {
	config := LeakyBucketConfig{
		Capacity:   10,
		LeakRate:   2,
		LeakPeriod: time.Second,
	}

	algo := NewLeakyBucket(config)
	ctx := context.Background()

	// Test with full queue
	state := &LeakyBucketState{
		QueueSize:    10,
		LastLeakTime: time.Now(),
	}

	_, result, err := algo.Allow(ctx, state, 1)
	require.NoError(t, err)

	bucketResult := result.(*LeakyBucketResult)
	assert.False(t, bucketResult.Allowed)
	assert.True(t, bucketResult.ResetTime.After(time.Now()))
}

func TestLeakyBucket_LeakEmptyQueue(t *testing.T) {
	config := LeakyBucketConfig{
		Capacity:   10,
		LeakRate:   5,
		LeakPeriod: time.Second,
	}

	algo := NewLeakyBucket(config)
	ctx := context.Background()

	// Create empty queue with old timestamp
	oldTime := time.Now().Add(-time.Second)
	testState := &LeakyBucketState{
		QueueSize:    0,
		LastLeakTime: oldTime,
	}

	_, result, err := algo.Allow(ctx, testState, 1)
	require.NoError(t, err)

	bucketResult := result.(*LeakyBucketResult)
	assert.True(t, bucketResult.Allowed)
	assert.Equal(t, 1, bucketResult.QueueSize)
}

func TestLeakyBucket_LeakMoreThanQueue(t *testing.T) {
	config := LeakyBucketConfig{
		Capacity:   10,
		LeakRate:   5,
		LeakPeriod: time.Second,
	}

	algo := NewLeakyBucket(config)
	ctx := context.Background()

	// Create state with old timestamp that would leak more than queue size
	oldTime := time.Now().Add(-3 * time.Second) // 15 requests would be leaked
	testState := &LeakyBucketState{
		QueueSize:    10,
		LastLeakTime: oldTime,
	}

	_, result, err := algo.Allow(ctx, testState, 1)
	require.NoError(t, err)

	bucketResult := result.(*LeakyBucketResult)
	assert.True(t, bucketResult.Allowed)
	assert.Equal(t, 1, bucketResult.QueueSize) // Queue emptied (10-15=0) then 1 added
}

func TestLeakyBucket_PartialLeakPeriod(t *testing.T) {
	config := LeakyBucketConfig{
		Capacity:   10,
		LeakRate:   5,
		LeakPeriod: time.Second,
	}

	algo := NewLeakyBucket(config)
	ctx := context.Background()

	// Create state with partial leak period elapsed
	oldTime := time.Now().Add(-500 * time.Millisecond) // Half a second
	testState := &LeakyBucketState{
		QueueSize:    5,
		LastLeakTime: oldTime,
	}

	_, result, err := algo.Allow(ctx, testState, 1)
	require.NoError(t, err)

	bucketResult := result.(*LeakyBucketResult)
	assert.True(t, bucketResult.Allowed)
	assert.Equal(t, 6, bucketResult.QueueSize) // No leak yet (partial period), 5 + 1 = 6
}

func TestLeakyBucket_ZeroElapsedTime(t *testing.T) {
	config := LeakyBucketConfig{
		Capacity:   10,
		LeakRate:   5,
		LeakPeriod: time.Second,
	}

	algo := NewLeakyBucket(config)
	ctx := context.Background()

	now := time.Now()
	testState := &LeakyBucketState{
		QueueSize:    5,
		LastLeakTime: now,
	}

	// Allow with same timestamp should not leak
	_, result, err := algo.Allow(ctx, testState, 1)
	require.NoError(t, err)

	bucketResult := result.(*LeakyBucketResult)
	assert.True(t, bucketResult.Allowed)
	assert.Equal(t, 6, bucketResult.QueueSize)
}

func TestLeakyBucket_MultipleLeakPeriods(t *testing.T) {
	config := LeakyBucketConfig{
		Capacity:   20,
		LeakRate:   3,
		LeakPeriod: time.Second,
	}

	algo := NewLeakyBucket(config)
	ctx := context.Background()

	// Simulate 3 leak periods passing (3 seconds = 9 requests leaked)
	oldTime := time.Now().Add(-3 * time.Second)
	testState := &LeakyBucketState{
		QueueSize:    15,
		LastLeakTime: oldTime,
	}

	_, result, err := algo.Allow(ctx, testState, 2)
	require.NoError(t, err)

	bucketResult := result.(*LeakyBucketResult)
	assert.True(t, bucketResult.Allowed)
	assert.Equal(t, 8, bucketResult.QueueSize) // 15 - 9 leaked + 2 added = 8
}

func TestLeakyBucket_RetryAfter_Calculation(t *testing.T) {
	config := LeakyBucketConfig{
		Capacity:   5,
		LeakRate:   2,
		LeakPeriod: time.Second,
	}

	algo := NewLeakyBucket(config)
	ctx := context.Background()

	// Fill the queue completely with a leak time in the past
	// so we don't count the partial current period
	oldTime := time.Now().Add(-100 * time.Millisecond)
	state := &LeakyBucketState{
		QueueSize:    5,
		LastLeakTime: oldTime,
	}

	_, result, err := algo.Allow(ctx, state, 1)
	require.NoError(t, err)

	bucketResult := result.(*LeakyBucketResult)
	assert.False(t, bucketResult.Allowed)
	// Should need approximately one leak period to make space
	// (allowing some tolerance for execution time)
	assert.True(t, bucketResult.RetryAfter > 800*time.Millisecond)
}

func TestLeakyBucket_EmptyQueueResetTime(t *testing.T) {
	config := LeakyBucketConfig{
		Capacity:   10,
		LeakRate:   5,
		LeakPeriod: time.Second,
	}

	algo := NewLeakyBucket(config)
	ctx := context.Background()

	state := &LeakyBucketState{
		QueueSize:    0,
		LastLeakTime: time.Now(),
	}

	_, result, err := algo.Allow(ctx, state, 1)
	require.NoError(t, err)

	bucketResult := result.(*LeakyBucketResult)
	assert.True(t, bucketResult.Allowed)
	// Reset time should be in the future (when this 1 request will be processed)
	assert.True(t, bucketResult.ResetTime.After(time.Now()))
}

func TestLeakyBucket_ValidateConfig_InvalidType(t *testing.T) {
	algo := NewLeakyBucket(LeakyBucketConfig{})

	err := algo.ValidateConfig("invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid config type")
}
