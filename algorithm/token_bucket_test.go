package algorithm

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// floatTolerance is the default tolerance for floating-point comparisons.
const floatTolerance = 1e-3

// assertFloatEqual asserts that two float64 values are equal within floatTolerance
func assertFloatEqual(t *testing.T, expected, actual float64) {
	t.Helper()
	if math.Abs(expected-actual) > floatTolerance {
		t.Errorf("expected %f, got %f (difference: %f)", expected, actual, math.Abs(expected-actual))
	}
}

func TestTokenBucket_NewTokenBucket(t *testing.T) {
	config := TokenBucketConfig{
		Capacity:     100,
		RefillRate:   10,
		RefillPeriod: time.Second,
	}

	algo := NewTokenBucket(config)
	assert.Equal(t, "token_bucket", algo.Name())
	assert.Contains(t, algo.Description(), "Token Bucket")
}

func TestTokenBucket_ValidateConfig(t *testing.T) {
	algo := NewTokenBucket(TokenBucketConfig{})

	tests := []struct {
		name    string
		config  TokenBucketConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: TokenBucketConfig{
				Capacity:     100,
				RefillRate:   10,
				RefillPeriod: time.Second,
			},
			wantErr: false,
		},
		{
			name: "zero capacity",
			config: TokenBucketConfig{
				Capacity:     0,
				RefillRate:   10,
				RefillPeriod: time.Second,
			},
			wantErr: true,
		},
		{
			name: "negative capacity",
			config: TokenBucketConfig{
				Capacity:     -1,
				RefillRate:   10,
				RefillPeriod: time.Second,
			},
			wantErr: true,
		},
		{
			name: "zero refill rate",
			config: TokenBucketConfig{
				Capacity:     100,
				RefillRate:   0,
				RefillPeriod: time.Second,
			},
			wantErr: true,
		},
		{
			name: "zero refill period",
			config: TokenBucketConfig{
				Capacity:     100,
				RefillRate:   10,
				RefillPeriod: 0,
			},
			wantErr: true,
		},
		{
			name: "initial tokens exceed capacity",
			config: TokenBucketConfig{
				Capacity:      100,
				RefillRate:    10,
				RefillPeriod:  time.Second,
				InitialTokens: 150,
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

func TestTokenBucket_Reset(t *testing.T) {
	config := TokenBucketConfig{
		Capacity:      100,
		RefillRate:    10,
		RefillPeriod:  time.Second,
		InitialTokens: 50,
	}

	algo := NewTokenBucket(config)
	ctx := context.Background()

	state, err := algo.Reset(ctx)
	require.NoError(t, err)

	bucketState, ok := state.(*TokenBucketState)
	require.True(t, ok)
	assert.Equal(t, float64(50), bucketState.Tokens)
	assert.True(t, time.Since(bucketState.LastRefillTime) < time.Millisecond)
}

func TestTokenBucket_Allow_Basic(t *testing.T) {
	config := TokenBucketConfig{
		Capacity:     10,
		RefillRate:   1,
		RefillPeriod: time.Second,
	}

	algo := NewTokenBucket(config)
	ctx := context.Background()

	// First request - should initialize with full capacity
	newState, result, err := algo.Allow(ctx, nil, 1)
	require.NoError(t, err)

	bucketResult, ok := result.(*TokenBucketResult)
	require.True(t, ok)
	assert.True(t, bucketResult.Allowed)
	assertFloatEqual(t, 9, bucketResult.RemainingTokens)

	// Second request - should have 9 tokens left
	_, result, err = algo.Allow(ctx, newState, 1)
	require.NoError(t, err)

	bucketResult, ok = result.(*TokenBucketResult)
	require.True(t, ok)
	assert.True(t, bucketResult.Allowed)
	assertFloatEqual(t, 8, bucketResult.RemainingTokens)
}

func TestTokenBucket_Allow_Deny(t *testing.T) {
	config := TokenBucketConfig{
		Capacity:     2,
		RefillRate:   1,
		RefillPeriod: time.Second,
	}

	algo := NewTokenBucket(config)
	ctx := context.Background()

	// Use up all tokens
	newState, _, err := algo.Allow(ctx, nil, 2)
	require.NoError(t, err)

	// Next request should be denied
	_, result, err := algo.Allow(ctx, newState, 1)
	require.NoError(t, err)

	bucketResult, ok := result.(*TokenBucketResult)
	require.True(t, ok)
	assert.False(t, bucketResult.Allowed)
	assertFloatEqual(t, 0, bucketResult.RemainingTokens)
	assert.True(t, bucketResult.RetryAfter > 0)
}

func TestTokenBucket_Allow_Refill(t *testing.T) {
	config := TokenBucketConfig{
		Capacity:     10,
		RefillRate:   5,
		RefillPeriod: time.Second,
	}

	algo := NewTokenBucket(config)
	ctx := context.Background()

	// Use all tokens
	newState, _, err := algo.Allow(ctx, nil, 10)
	require.NoError(t, err)

	// Check state has 0 tokens
	bucketState := newState.(*TokenBucketState)
	assert.Equal(t, float64(0), bucketState.Tokens)

	// Simulate time passing (1 second = 5 tokens)
	// We'll test this by creating a state with old timestamp
	oldTime := time.Now().Add(-time.Second)
	testState := &TokenBucketState{
		Tokens:         0,
		LastRefillTime: oldTime,
	}

	_, result, err := algo.Allow(ctx, testState, 3)
	require.NoError(t, err)

	bucketResult := result.(*TokenBucketResult)
	assert.True(t, bucketResult.Allowed)
	assertFloatEqual(t, 2, bucketResult.RemainingTokens) // 5 tokens added - 3 used = 2 left
}

func TestTokenBucket_Allow_Cost(t *testing.T) {
	config := TokenBucketConfig{
		Capacity:     100,
		RefillRate:   10,
		RefillPeriod: time.Second,
	}

	algo := NewTokenBucket(config)
	ctx := context.Background()

	// Request with cost > 1
	_, result, err := algo.Allow(ctx, nil, 5)
	require.NoError(t, err)

	bucketResult, ok := result.(*TokenBucketResult)
	require.True(t, ok)
	assert.True(t, bucketResult.Allowed)
	assert.Equal(t, float64(95), bucketResult.RemainingTokens)
}

func TestTokenBucket_GetStats(t *testing.T) {
	config := TokenBucketConfig{
		Capacity:     100,
		RefillRate:   10,
		RefillPeriod: time.Second,
	}

	algo := NewTokenBucket(config)
	ctx := context.Background()

	// Stats with nil state
	stats := algo.GetStats(ctx, nil)
	bucketStats, ok := stats.(*TokenBucketStats)
	require.True(t, ok)
	assert.Equal(t, float64(100), bucketStats.Capacity)
	assert.Equal(t, float64(10), bucketStats.RefillRate)
	assert.Equal(t, time.Second, bucketStats.RefillPeriod)

	// Stats with actual state
	testState := &TokenBucketState{
		Tokens:         50,
		LastRefillTime: time.Now().Add(-500 * time.Millisecond),
	}

	stats = algo.GetStats(ctx, testState)
	bucketStats, ok = stats.(*TokenBucketStats)
	require.True(t, ok)
	assert.Equal(t, float64(50), bucketStats.CurrentTokens)
	assert.Equal(t, float64(100), bucketStats.Capacity)
	assert.True(t, bucketStats.NextRefillTime.After(bucketStats.LastRefillTime))
}

func TestTokenBucket_GetStats_InvalidState(t *testing.T) {
	algo := NewTokenBucket(TokenBucketConfig{})
	ctx := context.Background()

	// Invalid state type
	stats := algo.GetStats(ctx, "invalid")
	assert.Nil(t, stats)
}

func TestTokenBucket_Allow_InvalidState(t *testing.T) {
	algo := NewTokenBucket(TokenBucketConfig{})
	ctx := context.Background()

	_, _, err := algo.Allow(ctx, "invalid", 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid state type")
}

func TestTokenBucket_CalculateResetTime(t *testing.T) {
	config := TokenBucketConfig{
		Capacity:     10,
		RefillRate:   2,
		RefillPeriod: time.Second,
	}

	algo := NewTokenBucket(config)
	ctx := context.Background()

	// Test with empty bucket
	state := &TokenBucketState{
		Tokens:         0,
		LastRefillTime: time.Now(),
	}

	_, result, err := algo.Allow(ctx, state, 1)
	require.NoError(t, err)

	bucketResult := result.(*TokenBucketResult)
	assert.False(t, bucketResult.Allowed)
	assert.True(t, bucketResult.ResetTime.After(time.Now()))
}

func TestTokenBucket_InitialTokens(t *testing.T) {
	config := TokenBucketConfig{
		Capacity:      100,
		RefillRate:    10,
		RefillPeriod:  time.Second,
		InitialTokens: 25,
	}

	algo := NewTokenBucket(config)
	ctx := context.Background()

	_, result, err := algo.Allow(ctx, nil, 1)
	require.NoError(t, err)

	bucketResult := result.(*TokenBucketResult)
	assert.True(t, bucketResult.Allowed)
	assertFloatEqual(t, 24, bucketResult.RemainingTokens)
}

func TestTokenBucket_RefillOverCapacity(t *testing.T) {
	config := TokenBucketConfig{
		Capacity:     10,
		RefillRate:   5,
		RefillPeriod: time.Second,
	}

	algo := NewTokenBucket(config)
	ctx := context.Background()

	// Create state with old timestamp that would cause over-refill
	oldTime := time.Now().Add(-3 * time.Second) // 15 tokens would be added
	testState := &TokenBucketState{
		Tokens:         5,
		LastRefillTime: oldTime,
	}

	_, result, err := algo.Allow(ctx, testState, 1)
	require.NoError(t, err)

	bucketResult := result.(*TokenBucketResult)
	assert.True(t, bucketResult.Allowed)
	assert.Equal(t, float64(9), bucketResult.RemainingTokens) // Should be capped at capacity (10) - 1 = 9
}

func TestTokenBucket_ZeroElapsedTime(t *testing.T) {
	config := TokenBucketConfig{
		Capacity:     10,
		RefillRate:   5,
		RefillPeriod: time.Second,
	}

	algo := NewTokenBucket(config)
	ctx := context.Background()

	now := time.Now()
	testState := &TokenBucketState{
		Tokens:         5,
		LastRefillTime: now,
	}

	// Allow with same timestamp should not refill
	_, result, err := algo.Allow(ctx, testState, 1)
	require.NoError(t, err)

	bucketResult := result.(*TokenBucketResult)
	assert.True(t, bucketResult.Allowed)
	assertFloatEqual(t, 4, bucketResult.RemainingTokens)
}
