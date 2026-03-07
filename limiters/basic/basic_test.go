package basic

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abhipray-cpu/niyantrak/algorithm"
	"github.com/abhipray-cpu/niyantrak/backend"
	backendmem "github.com/abhipray-cpu/niyantrak/backend/memory"
	"github.com/abhipray-cpu/niyantrak/features"
	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/mocks"
	obstypes "github.com/abhipray-cpu/niyantrak/observability/types"
)

// TestNewBasicLimiter_ValidConfig tests constructor with valid configurations
func TestNewBasicLimiter_ValidConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	mockBackend.EXPECT().Close().Return(nil).Times(1)

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			AlgorithmName: "token_bucket",
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			KeyTTL:        time.Minute,
		},
	)

	require.NoError(t, err, "Should create limiter without error")
	require.NotNil(t, limiter, "Limiter should not be nil")
	assert.Equal(t, "basic", limiter.Type())

	// Cleanup
	err = limiter.Close()
	assert.NoError(t, err, "Close should not error")
}

// TestNewBasicLimiter_InvalidConfig tests constructor validation
func TestNewBasicLimiter_InvalidConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	tests := []struct {
		name          string
		algo          algorithm.Algorithm
		backend       backend.Backend
		cfg           limiters.BasicConfig
		expectErr     bool
		errorContains string
	}{
		{
			name:          "nil algorithm",
			algo:          nil,
			backend:       mockBackend,
			cfg:           limiters.BasicConfig{DefaultLimit: 10, DefaultWindow: time.Second},
			expectErr:     true,
			errorContains: "algorithm cannot be nil",
		},
		{
			name:          "nil backend",
			algo:          mockAlgo,
			backend:       nil,
			cfg:           limiters.BasicConfig{DefaultLimit: 10, DefaultWindow: time.Second},
			expectErr:     true,
			errorContains: "backend cannot be nil",
		},
		{
			name:          "zero default limit",
			algo:          mockAlgo,
			backend:       mockBackend,
			cfg:           limiters.BasicConfig{DefaultLimit: 0, DefaultWindow: time.Second},
			expectErr:     true,
			errorContains: "default limit must be positive",
		},
		{
			name:          "negative default limit",
			algo:          mockAlgo,
			backend:       mockBackend,
			cfg:           limiters.BasicConfig{DefaultLimit: -5, DefaultWindow: time.Second},
			expectErr:     true,
			errorContains: "default limit must be positive",
		},
		{
			name:          "zero default window",
			algo:          mockAlgo,
			backend:       mockBackend,
			cfg:           limiters.BasicConfig{DefaultLimit: 10, DefaultWindow: 0},
			expectErr:     true,
			errorContains: "default window must be positive",
		},
		{
			name:          "negative default window",
			algo:          mockAlgo,
			backend:       mockBackend,
			cfg:           limiters.BasicConfig{DefaultLimit: 10, DefaultWindow: -time.Second},
			expectErr:     true,
			errorContains: "default window must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := NewBasicLimiter(tt.algo, tt.backend, tt.cfg)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
				assert.Nil(t, limiter)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, limiter)
				if limiter != nil {
					limiter.Close()
				}
			}
		})
	}
}

// TestAllow_WithMocks tests Allow with mocked algorithm and backend
func TestAllow_WithMocks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	ctx := context.Background()
	key := "test:user:1"

	// Setup expectations
	mockBackend.EXPECT().
		Get(ctx, key).
		Return(nil, backend.ErrKeyNotFound).
		Times(1)

	mockAlgo.EXPECT().
		Reset(ctx).
		Return(map[string]interface{}{}, nil).
		Times(1)

	mockAlgo.EXPECT().
		Allow(ctx, map[string]interface{}{}, 1).
		Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		Times(1)

	mockBackend.EXPECT().
		Set(ctx, key, gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	mockBackend.EXPECT().Close().Return(nil).Times(1)

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
		},
	)
	require.NoError(t, err)
	defer limiter.Close()

	result := limiter.Allow(ctx, key)
	assert.NotNil(t, result)
	assert.NoError(t, result.Error)
}

// TestAllow_MultipleRequests tests Allow with multiple sequential requests
func TestAllow_MultipleRequests(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	ctx := context.Background()
	key := "test:user:multiple"

	// First call - key not found, initialize
	mockBackend.EXPECT().
		Get(ctx, key).
		Return(nil, backend.ErrKeyNotFound).
		Times(1)

	mockAlgo.EXPECT().
		Reset(ctx).
		Return(map[string]interface{}{"count": 0}, nil).
		Times(1)

	mockAlgo.EXPECT().
		Allow(ctx, map[string]interface{}{"count": 0}, 1).
		Return(map[string]interface{}{"count": 1}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		Times(1)

	mockBackend.EXPECT().
		Set(ctx, key, gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	// Second call - key exists
	mockBackend.EXPECT().
		Get(ctx, key).
		Return(map[string]interface{}{"count": 1}, nil).
		Times(1)

	mockAlgo.EXPECT().
		Allow(ctx, map[string]interface{}{"count": 1}, 1).
		Return(map[string]interface{}{"count": 2}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		Times(1)

	mockBackend.EXPECT().
		Set(ctx, key, gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	mockBackend.EXPECT().Close().Return(nil).Times(1)

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
		},
	)
	require.NoError(t, err)
	defer limiter.Close()

	// First request
	result1 := limiter.Allow(ctx, key)
	assert.NotNil(t, result1)
	assert.NoError(t, result1.Error)

	// Second request
	result2 := limiter.Allow(ctx, key)
	assert.NotNil(t, result2)
	assert.NoError(t, result2.Error)
}

// TestAllow_InvalidKey tests Allow with empty key
func TestAllow_InvalidKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	mockBackend.EXPECT().Close().Return(nil).Times(1)

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
		},
	)
	require.NoError(t, err)
	defer limiter.Close()

	ctx := context.Background()

	// Empty key should fail
	result := limiter.Allow(ctx, "")
	assert.False(t, result.Allowed)
	assert.Equal(t, limiters.ErrInvalidKey, result.Error)
}

// TestAllowN_WithMocks tests AllowN with mocked algorithm and backend
func TestAllowN_WithMocks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	ctx := context.Background()
	key := "test:user:allowN"

	// Setup expectations for AllowN with n=5
	mockBackend.EXPECT().
		Get(ctx, key).
		Return(nil, backend.ErrKeyNotFound).
		Times(1)

	mockAlgo.EXPECT().
		Reset(ctx).
		Return(map[string]interface{}{}, nil).
		Times(1)

	mockAlgo.EXPECT().
		Allow(ctx, map[string]interface{}{}, 5).
		Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		Times(1)

	mockBackend.EXPECT().
		Set(ctx, key, gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	mockBackend.EXPECT().Close().Return(nil).Times(1)

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			DefaultLimit:  20,
			DefaultWindow: time.Second,
		},
	)
	require.NoError(t, err)
	defer limiter.Close()

	result := limiter.AllowN(ctx, key, 5)
	assert.NotNil(t, result)
	assert.NoError(t, result.Error)
}

// TestAllowN_InvalidN tests AllowN with invalid N values
func TestAllowN_InvalidN(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	mockBackend.EXPECT().Close().Return(nil).Times(1)

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
		},
	)
	require.NoError(t, err)
	defer limiter.Close()

	ctx := context.Background()

	// Zero N
	result := limiter.AllowN(ctx, "user:1", 0)
	assert.False(t, result.Allowed)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "positive")

	// Negative N
	result = limiter.AllowN(ctx, "user:1", -5)
	assert.False(t, result.Allowed)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "positive")
}

// TestReset_WithMocks tests Reset with mocked algorithm and backend
func TestReset_WithMocks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	ctx := context.Background()
	key := "test:user:reset"

	mockBackend.EXPECT().
		Delete(ctx, key).
		Return(nil).
		Times(1)

	mockBackend.EXPECT().Close().Return(nil).Times(1)

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
		},
	)
	require.NoError(t, err)
	defer limiter.Close()

	// Reset should succeed
	err = limiter.Reset(ctx, key)
	assert.NoError(t, err, "Reset should not error")
}

// TestReset_InvalidKey tests Reset with empty key
func TestReset_InvalidKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	mockBackend.EXPECT().Close().Return(nil).Times(1)

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
		},
	)
	require.NoError(t, err)
	defer limiter.Close()

	ctx := context.Background()

	// Empty key should fail
	err = limiter.Reset(ctx, "")
	assert.Equal(t, limiters.ErrInvalidKey, err)
}

// TestGetStats_WithMocks tests GetStats with mocked algorithm and backend
func TestGetStats_WithMocks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	ctx := context.Background()
	key := "test:user:stats"

	mockBackend.EXPECT().
		Get(ctx, key).
		Return(map[string]interface{}{"count": 5}, nil).
		Times(1)

	mockAlgo.EXPECT().
		GetStats(ctx, map[string]interface{}{"count": 5}).
		Return(map[string]interface{}{"remaining": 5, "reset_at": "2025-01-23T10:00:00Z"}).
		Times(1)

	mockBackend.EXPECT().Close().Return(nil).Times(1)

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
		},
	)
	require.NoError(t, err)
	defer limiter.Close()

	// Get stats should succeed
	stats := limiter.GetStats(ctx, key)
	assert.NotNil(t, stats, "Stats should not be nil")
}

// TestClose_WithMocks tests Close with mocked algorithm and backend
func TestClose_WithMocks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	ctx := context.Background()
	key := "test:user:close"

	// First Close
	mockBackend.EXPECT().Close().Return(nil).Times(1)

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
		},
	)
	require.NoError(t, err)

	// Close should succeed
	err = limiter.Close()
	assert.NoError(t, err, "Close should not error")

	// Double close should not error (idempotent)
	err = limiter.Close()
	assert.NoError(t, err, "Double close should not error")

	// Operations after close should fail
	result := limiter.Allow(ctx, key)
	assert.False(t, result.Allowed)
	assert.Equal(t, limiters.ErrLimiterClosed, result.Error)

	err = limiter.Reset(ctx, key)
	assert.Equal(t, limiters.ErrLimiterClosed, err)

	stats := limiter.GetStats(ctx, key)
	assert.Nil(t, stats, "GetStats after close should return nil")
}

// TestSetLimit tests SetLimit method
func TestSetLimit_WithMocks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	mockBackend.EXPECT().Close().Return(nil).Times(1)

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
		},
	)
	require.NoError(t, err)
	defer limiter.Close()

	ctx := context.Background()

	// Valid SetLimit (no-op for basic limiter but should not error)
	err = limiter.SetLimit(ctx, "user:1", 20, 2*time.Second)
	assert.NoError(t, err)

	// Invalid key
	err = limiter.SetLimit(ctx, "", 20, 2*time.Second)
	assert.Equal(t, limiters.ErrInvalidKey, err)

	// Invalid limit
	err = limiter.SetLimit(ctx, "user:1", 0, 2*time.Second)
	assert.Equal(t, limiters.ErrInvalidConfig, err)

	// Invalid window
	err = limiter.SetLimit(ctx, "user:1", 20, 0)
	assert.Equal(t, limiters.ErrInvalidConfig, err)
}

// TestConcurrentAccess tests thread safety with multiple concurrent requests
func TestConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent test in short mode")
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	ctx := context.Background()
	key := "concurrent:test:key"

	// Expect multiple concurrent Get/Set calls
	mockBackend.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(nil, backend.ErrKeyNotFound).
		Times(1) // Only first goroutine gets not found

	mockAlgo.EXPECT().
		Reset(gomock.Any()).
		Return(map[string]interface{}{"count": 0}, nil).
		Times(1)

	mockAlgo.EXPECT().
		Allow(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		AnyTimes()

	mockBackend.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(map[string]interface{}{"count": 1}, nil).
		AnyTimes()

	mockBackend.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockAlgo.EXPECT().
		GetStats(gomock.Any(), gomock.Any()).
		Return(map[string]interface{}{"remaining": 5}).
		AnyTimes()

	mockBackend.EXPECT().Close().Return(nil).Times(1)

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			DefaultLimit:  1000,
			DefaultWindow: time.Second,
		},
	)
	require.NoError(t, err)
	defer limiter.Close()

	// Run 10 concurrent goroutines
	var wg sync.WaitGroup
	concurrency := 10
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(_ int) {
			defer wg.Done()

			// Each goroutine makes multiple requests
			for j := 0; j < 5; j++ {
				limiter.Allow(ctx, key)
				limiter.AllowN(ctx, key, 2)
				limiter.GetStats(ctx, key)
			}
		}(i)
	}

	wg.Wait()

	// Verify limiter still works after concurrent access
	result := limiter.Allow(ctx, key)
	assert.NotNil(t, result)
}

// TestType tests Type method
func TestType_WithMocks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	mockBackend.EXPECT().Close().Return(nil).Times(1)

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
		},
	)
	require.NoError(t, err)
	defer limiter.Close()

	assert.Equal(t, "basic", limiter.Type())
}

// TestMultipleKeys tests limiter with multiple different keys
func TestMultipleKeys_WithMocks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	ctx := context.Background()

	// Setup expectations for 5 different keys
	mockBackend.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(nil, backend.ErrKeyNotFound).
		AnyTimes()

	mockAlgo.EXPECT().
		Reset(gomock.Any()).
		Return(map[string]interface{}{}, nil).
		AnyTimes()

	mockAlgo.EXPECT().
		Allow(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		AnyTimes()

	mockBackend.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockAlgo.EXPECT().
		GetStats(gomock.Any(), gomock.Any()).
		Return(map[string]interface{}{"remaining": 10}).
		AnyTimes()

	mockBackend.EXPECT().Close().Return(nil).Times(1)

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
		},
	)
	require.NoError(t, err)
	defer limiter.Close()

	// Different keys should have independent limits
	keys := []string{"user:1", "user:2", "user:3", "api:key1", "api:key2"}

	for _, key := range keys {
		result := limiter.Allow(ctx, key)
		assert.True(t, result.Allowed, "Request for key %s should be allowed", key)
	}

	// Each key should have its own stats
	for _, key := range keys {
		stats := limiter.GetStats(ctx, key)
		assert.NotNil(t, stats, "Stats for key %s should not be nil", key)
	}
}

// BenchmarkAllow benchmarks Allow operation with real backend
func BenchmarkAllow(b *testing.B) {
	algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity:     10000,
		RefillRate:   10000,
		RefillPeriod: time.Second,
	})
	be := backendmem.NewMemoryBackend()
	defer be.Close()

	limiter, _ := NewBasicLimiter(
		algo,
		be,
		limiters.BasicConfig{
			DefaultLimit:  10000,
			DefaultWindow: time.Second,
		},
	)
	defer limiter.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(ctx, "bench:key")
	}
}

// BenchmarkAllowN benchmarks AllowN operation with real backend
func BenchmarkAllowN(b *testing.B) {
	algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
		Capacity:     10000,
		RefillRate:   10000,
		RefillPeriod: time.Second,
	})
	be := backendmem.NewMemoryBackend()
	defer be.Close()

	limiter, _ := NewBasicLimiter(
		algo,
		be,
		limiters.BasicConfig{
			DefaultLimit:  10000,
			DefaultWindow: time.Second,
		},
	)
	defer limiter.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.AllowN(ctx, "bench:key", 5)
	}
}

// BenchmarkAllow_WithMocks benchmarks Allow operation with mocked algorithm and backend
func BenchmarkAllow_WithMocks(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	ctx := context.Background()
	key := "bench:mock:key"

	// Setup expectations for high volume
	mockBackend.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(nil, backend.ErrKeyNotFound).
		Times(1)

	mockAlgo.EXPECT().
		Reset(gomock.Any()).
		Return(map[string]interface{}{}, nil).
		Times(1)

	mockBackend.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(map[string]interface{}{}, nil).
		AnyTimes()

	mockAlgo.EXPECT().
		Allow(gomock.Any(), gomock.Any(), 1).
		Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		AnyTimes()

	mockBackend.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockBackend.EXPECT().Close().Return(nil).Times(1)

	limiter, _ := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			DefaultLimit:  10000,
			DefaultWindow: time.Second,
		},
	)
	defer limiter.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(ctx, key)
	}
}

// BenchmarkAllowN_WithMocks benchmarks AllowN operation with mocked algorithm and backend
func BenchmarkAllowN_WithMocks(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	ctx := context.Background()
	key := "bench:mock:allowN"

	// Setup expectations for high volume
	mockBackend.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(nil, backend.ErrKeyNotFound).
		Times(1)

	mockAlgo.EXPECT().
		Reset(gomock.Any()).
		Return(map[string]interface{}{}, nil).
		Times(1)

	mockBackend.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(map[string]interface{}{}, nil).
		AnyTimes()

	mockAlgo.EXPECT().
		Allow(gomock.Any(), gomock.Any(), 5).
		Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		AnyTimes()

	mockBackend.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockBackend.EXPECT().Close().Return(nil).Times(1)

	limiter, _ := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			DefaultLimit:  10000,
			DefaultWindow: time.Second,
		},
	)
	defer limiter.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.AllowN(ctx, key, 5)
	}
}

// TestBasicLimiter_Observability_NoOp tests that NoOp observability works (zero overhead)
func TestBasicLimiter_Observability_NoOp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	mockAlgo.EXPECT().ValidateConfig(gomock.Any()).Return(nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), "test-key").Return(nil, nil).Times(1)
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(map[string]interface{}{"tokens": 10}, nil).Times(1)
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), 1).Return(
		map[string]interface{}{"tokens": 9},
		&algorithm.TokenBucketResult{Allowed: true, RemainingTokens: 9},
		nil,
	).Times(1)
	mockBackend.EXPECT().Set(gomock.Any(), "test-key", gomock.Any(), gomock.Any()).Return(nil).Times(1)

	config := limiters.BasicConfig{
		DefaultLimit:  10,
		DefaultWindow: time.Minute,
	}

	limiter, err := NewBasicLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	result := limiter.Allow(ctx, "test-key")

	if !result.Allowed {
		t.Fatal("first request should be allowed")
	}
}

// TestBasicLimiter_Observability_WithMockLogger tests logging integration
func TestBasicLimiter_Observability_WithMockLogger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)

	mockAlgo.EXPECT().ValidateConfig(gomock.Any()).Return(nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), "test-key").Return(nil, nil).Times(1)
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(map[string]interface{}{"tokens": 10}, nil).Times(1)
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), 1).Return(
		map[string]interface{}{"tokens": 9},
		&algorithm.TokenBucketResult{Allowed: true, RemainingTokens: 9},
		nil,
	).Times(1)
	mockBackend.EXPECT().Set(gomock.Any(), "test-key", gomock.Any(), gomock.Any()).Return(nil).Times(1)

	mockLogger.EXPECT().Debug("rate_limit_allowed", gomock.Any()).Times(1)

	config := limiters.BasicConfig{
		DefaultLimit:  10,
		DefaultWindow: time.Minute,
		Observability: limiters.ObservabilityConfig{
			Logger: mockLogger,
		},
	}

	limiter, err := NewBasicLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	result := limiter.Allow(ctx, "test-key")

	if !result.Allowed {
		t.Fatal("first request should be allowed")
	}
}

// TestBasicLimiter_Observability_WithMockMetrics tests metrics integration
func TestBasicLimiter_Observability_WithMockMetrics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)

	mockAlgo.EXPECT().ValidateConfig(gomock.Any()).Return(nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), "test-key").Return(nil, nil).Times(1)
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(map[string]interface{}{"tokens": 10}, nil).Times(1)
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), 1).Return(
		map[string]interface{}{"tokens": 9},
		&algorithm.TokenBucketResult{Allowed: true, RemainingTokens: 9},
		nil,
	).Times(1)
	mockBackend.EXPECT().Set(gomock.Any(), "test-key", gomock.Any(), gomock.Any()).Return(nil).Times(1)

	mockMetrics.EXPECT().RecordDecisionLatency("test-key", gomock.Any()).Times(1)
	mockMetrics.EXPECT().RecordRequest("test-key", true, gomock.Any()).Times(1)

	config := limiters.BasicConfig{
		DefaultLimit:  10,
		DefaultWindow: time.Minute,
		Observability: limiters.ObservabilityConfig{
			Metrics: mockMetrics,
		},
	}

	limiter, err := NewBasicLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	result := limiter.Allow(ctx, "test-key")

	if !result.Allowed {
		t.Fatal("first request should be allowed")
	}
}

// TestBasicLimiter_Observability_WithMockTracer tests tracing integration
func TestBasicLimiter_Observability_WithMockTracer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockAlgo.EXPECT().ValidateConfig(gomock.Any()).Return(nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), "test-key").Return(nil, nil).Times(1)
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(map[string]interface{}{"tokens": 10}, nil).Times(1)
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), 1).Return(
		map[string]interface{}{"tokens": 9},
		&algorithm.TokenBucketResult{Allowed: true, RemainingTokens: 9},
		nil,
	).Times(1)
	mockBackend.EXPECT().Set(gomock.Any(), "test-key", gomock.Any(), gomock.Any()).Return(nil).Times(1)

	mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.check").Return(nil).Times(1)
	mockTracer.EXPECT().AddAttribute(nil, "key", "test-key").Times(1)
	mockTracer.EXPECT().AddAttribute(nil, "requests_count", 1).Times(1)
	mockTracer.EXPECT().AddAttribute(nil, "allowed", true).Times(1)
	mockTracer.EXPECT().AddAttribute(nil, "remaining", gomock.Any()).Times(1)
	mockTracer.EXPECT().AddAttribute(nil, "limit", gomock.Any()).Times(1)
	mockTracer.EXPECT().EndSpan(nil).Times(1)

	config := limiters.BasicConfig{
		DefaultLimit:  10,
		DefaultWindow: time.Minute,
		Observability: limiters.ObservabilityConfig{
			Tracer:        mockTracer,
			EnableTracing: true,
		},
	}

	limiter, err := NewBasicLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	result := limiter.Allow(ctx, "test-key")

	if !result.Allowed {
		t.Fatal("first request should be allowed")
	}
}

// TestBasicLimiter_Observability_AllThree tests all three layers together
func TestBasicLimiter_Observability_AllThree(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockAlgo.EXPECT().ValidateConfig(gomock.Any()).Return(nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), "test-key").Return(nil, nil).Times(1)
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(map[string]interface{}{"tokens": 10}, nil).Times(1)
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), 1).Return(
		map[string]interface{}{"tokens": 9},
		&algorithm.TokenBucketResult{Allowed: true, RemainingTokens: 9},
		nil,
	).Times(1)
	mockBackend.EXPECT().Set(gomock.Any(), "test-key", gomock.Any(), gomock.Any()).Return(nil).Times(1)

	mockLogger.EXPECT().Debug("rate_limit_allowed", gomock.Any()).Times(1)
	mockMetrics.EXPECT().RecordDecisionLatency("test-key", gomock.Any()).Times(1)
	mockMetrics.EXPECT().RecordRequest("test-key", true, gomock.Any()).Times(1)
	mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.check").Return(nil).Times(1)
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().EndSpan(nil).Times(1)

	config := limiters.BasicConfig{
		DefaultLimit:  10,
		DefaultWindow: time.Minute,
		Observability: limiters.ObservabilityConfig{
			Logger:        mockLogger,
			Metrics:       mockMetrics,
			Tracer:        mockTracer,
			EnableTracing: true,
			LogLevel:      "debug",
		},
	}

	limiter, err := NewBasicLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	result := limiter.Allow(ctx, "test-key")

	if !result.Allowed {
		t.Fatal("first request should be allowed")
	}
}

// TestBasicLimiter_WithDynamicLimits_Enabled tests limiter with dynamic limits enabled
func TestBasicLimiter_WithDynamicLimits_Enabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	// Create real DynamicLimitManager
	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  10,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(nil, nil).AnyTimes()

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			AlgorithmName: "token_bucket",
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			KeyTTL:        time.Minute,
			DynamicLimits: limiters.DynamicLimitConfig{
				EnableDynamicLimits: true,
				Manager:             dynamicMgr,
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Test that the limiter works with default limits
	result := limiter.Allow(ctx, "user123")
	assert.True(t, result.Allowed)
}

// TestBasicLimiter_WithDynamicLimits_UpdateLimit tests updating limits at runtime
func TestBasicLimiter_WithDynamicLimits_UpdateLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  10,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(nil, nil).AnyTimes()

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			AlgorithmName: "token_bucket",
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			KeyTTL:        time.Minute,
			DynamicLimits: limiters.DynamicLimitConfig{
				EnableDynamicLimits: true,
				Manager:             dynamicMgr,
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Update limit for specific key
	err = dynamicMgr.UpdateLimit(ctx, "premium_user", 100, time.Second)
	require.NoError(t, err)

	// Verify limit was updated
	limit, window, err := dynamicMgr.GetCurrentLimit(ctx, "premium_user")
	require.NoError(t, err)
	assert.Equal(t, 100, limit)
	assert.Equal(t, time.Second, window)

	// Test with updated limit
	result := limiter.Allow(ctx, "premium_user")
	assert.True(t, result.Allowed)
}

// TestBasicLimiter_DynamicLimits_MultipleKeys tests dynamic limits with multiple keys
func TestBasicLimiter_DynamicLimits_MultipleKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  10,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(nil, nil).AnyTimes()

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			AlgorithmName: "token_bucket",
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			KeyTTL:        time.Minute,
			DynamicLimits: limiters.DynamicLimitConfig{
				EnableDynamicLimits: true,
				Manager:             dynamicMgr,
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Set different limits for different users
	err = dynamicMgr.UpdateLimit(ctx, "free_user", 5, time.Second)
	require.NoError(t, err)

	err = dynamicMgr.UpdateLimit(ctx, "premium_user", 100, time.Second)
	require.NoError(t, err)

	err = dynamicMgr.UpdateLimit(ctx, "enterprise_user", 1000, time.Second)
	require.NoError(t, err)

	// Verify each key has correct limits
	limit, window, err := dynamicMgr.GetCurrentLimit(ctx, "free_user")
	require.NoError(t, err)
	assert.Equal(t, 5, limit)
	assert.Equal(t, time.Second, window)

	limit, window, err = dynamicMgr.GetCurrentLimit(ctx, "premium_user")
	require.NoError(t, err)
	assert.Equal(t, 100, limit)
	assert.Equal(t, time.Second, window)

	limit, window, err = dynamicMgr.GetCurrentLimit(ctx, "enterprise_user")
	require.NoError(t, err)
	assert.Equal(t, 1000, limit)
	assert.Equal(t, time.Second, window)

	// Test limiter with each key
	result := limiter.Allow(ctx, "free_user")
	assert.True(t, result.Allowed)

	result = limiter.Allow(ctx, "premium_user")
	assert.True(t, result.Allowed)

	result = limiter.Allow(ctx, "enterprise_user")
	assert.True(t, result.Allowed)
}

// TestBasicLimiter_DynamicLimits_ConcurrentUpdates tests concurrent limit updates
func TestBasicLimiter_DynamicLimits_ConcurrentUpdates(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  10,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(nil, nil).AnyTimes()

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			AlgorithmName: "token_bucket",
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			KeyTTL:        time.Minute,
			DynamicLimits: limiters.DynamicLimitConfig{
				EnableDynamicLimits: true,
				Manager:             dynamicMgr,
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Concurrent updates and reads
	var wg sync.WaitGroup
	numGoroutines := 10
	wg.Add(numGoroutines * 2)

	// Concurrent writers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			key := "user" + string(rune(id))
			err := dynamicMgr.UpdateLimit(ctx, key, 10+id, time.Second)
			assert.NoError(t, err)
		}(i)
	}

	// Concurrent readers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			key := "user" + string(rune(id))
			result := limiter.Allow(ctx, key)
			assert.NotNil(t, result)
		}(i)
	}

	wg.Wait()
}

// TestBasicLimiter_DynamicLimits_Disabled tests limiter with dynamic limits disabled
func TestBasicLimiter_DynamicLimits_Disabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(nil, nil).AnyTimes()

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			AlgorithmName: "token_bucket",
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			KeyTTL:        time.Minute,
			DynamicLimits: limiters.DynamicLimitConfig{
				EnableDynamicLimits: false,
				Manager:             nil,
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Should work without dynamic limits
	result := limiter.Allow(ctx, "user123")
	assert.True(t, result.Allowed)
}

// TestBasicLimiter_DynamicLimits_NilManager tests with nil manager
func TestBasicLimiter_DynamicLimits_NilManager(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(nil, nil).AnyTimes()

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			AlgorithmName: "token_bucket",
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			KeyTTL:        time.Minute,
			DynamicLimits: limiters.DynamicLimitConfig{
				EnableDynamicLimits: true,
				Manager:             nil, // nil manager
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Should still work even with nil manager (graceful degradation)
	result := limiter.Allow(ctx, "user123")
	assert.True(t, result.Allowed)
}

// TestBasicLimiter_DynamicLimits_AllowN tests AllowN with dynamic limits
func TestBasicLimiter_DynamicLimits_AllowN(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  10,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(nil, nil).AnyTimes()

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			AlgorithmName: "token_bucket",
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			KeyTTL:        time.Minute,
			DynamicLimits: limiters.DynamicLimitConfig{
				EnableDynamicLimits: true,
				Manager:             dynamicMgr,
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Update limit
	err = dynamicMgr.UpdateLimit(ctx, "batch_user", 100, time.Second)
	require.NoError(t, err)

	// Test AllowN with dynamic limits
	result := limiter.AllowN(ctx, "batch_user", 5)
	assert.True(t, result.Allowed)
}

// TestBasicLimiter_DynamicLimits_ValidationErrors tests validation errors
func TestBasicLimiter_DynamicLimits_ValidationErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  10,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			AlgorithmName: "token_bucket",
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			KeyTTL:        time.Minute,
			DynamicLimits: limiters.DynamicLimitConfig{
				EnableDynamicLimits: true,
				Manager:             dynamicMgr,
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Test invalid limit values
	err = dynamicMgr.UpdateLimit(ctx, "", 10, time.Second)
	assert.Error(t, err, "empty key should error")

	err = dynamicMgr.UpdateLimit(ctx, "user", -1, time.Second)
	assert.Error(t, err, "negative limit should error")

	err = dynamicMgr.UpdateLimit(ctx, "user", 10, 0)
	assert.Error(t, err, "zero window should error")

	err = dynamicMgr.UpdateLimit(ctx, "user", 10, -time.Second)
	assert.Error(t, err, "negative window should error")
}

// TestBasicLimiter_DynamicLimits_FallbackToDefault tests fallback to default limits
func TestBasicLimiter_DynamicLimits_FallbackToDefault(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  25,
		DefaultWindow: 2 * time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(nil, nil).AnyTimes()

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			AlgorithmName: "token_bucket",
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			KeyTTL:        time.Minute,
			DynamicLimits: limiters.DynamicLimitConfig{
				EnableDynamicLimits: true,
				Manager:             dynamicMgr,
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Key with no specific limit should use default from DynamicLimitManager
	limit, window, err := dynamicMgr.GetCurrentLimit(ctx, "unconfigured_key")
	require.NoError(t, err)
	assert.Equal(t, 25, limit) // Default from DynamicLimitManager
	assert.Equal(t, 2*time.Second, window)
}

// TestBasicLimiter_DynamicLimits_UpdateHooks tests update hooks
func TestBasicLimiter_DynamicLimits_UpdateHooks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  10,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(nil, nil).AnyTimes()

	// Track hook calls
	var hookCalls []string
	var mu sync.Mutex

	dynamicMgr.AddUpdateHook(func(key string, config *features.LimitConfig) {
		mu.Lock()
		defer mu.Unlock()
		hookCalls = append(hookCalls, key)
	})

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			AlgorithmName: "token_bucket",
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			KeyTTL:        time.Minute,
			DynamicLimits: limiters.DynamicLimitConfig{
				EnableDynamicLimits: true,
				Manager:             dynamicMgr,
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Update limits - should trigger hooks
	err = dynamicMgr.UpdateLimit(ctx, "user1", 50, time.Second)
	require.NoError(t, err)

	err = dynamicMgr.UpdateLimit(ctx, "user2", 100, time.Second)
	require.NoError(t, err)

	// Give hooks time to execute
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, hookCalls, 2, "hooks should be called for each update")
	assert.Contains(t, hookCalls, "user1")
	assert.Contains(t, hookCalls, "user2")
}

// TestBasicLimiter_DynamicLimits_GetAllLimits tests retrieving all configured limits
func TestBasicLimiter_DynamicLimits_GetAllLimits(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  10,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	limiter, err := NewBasicLimiter(
		mockAlgo,
		mockBackend,
		limiters.BasicConfig{
			AlgorithmName: "token_bucket",
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			KeyTTL:        time.Minute,
			DynamicLimits: limiters.DynamicLimitConfig{
				EnableDynamicLimits: true,
				Manager:             dynamicMgr,
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Initially should be empty
	allLimits := dynamicMgr.GetAllLimits()
	assert.Empty(t, allLimits)

	// Add multiple limits
	err = dynamicMgr.UpdateLimit(ctx, "user1", 50, time.Second)
	require.NoError(t, err)

	err = dynamicMgr.UpdateLimit(ctx, "user2", 100, time.Second)
	require.NoError(t, err)

	err = dynamicMgr.UpdateLimit(ctx, "user3", 200, time.Second)
	require.NoError(t, err)

	// Should return all configured limits
	allLimits = dynamicMgr.GetAllLimits()
	assert.Len(t, allLimits, 3)
	assert.Contains(t, allLimits, "user1")
	assert.Contains(t, allLimits, "user2")
	assert.Contains(t, allLimits, "user3")
}

// Test Failover Integration with Basic Limiter
// These tests validate that the failover mechanism properly integrates with the basic rate limiter

// mockBackendWithFailure simulates backend failures
type mockBackendWithFailure struct {
	mu               sync.Mutex
	data             map[string]interface{}
	failGetCount     int
	failSetCount     int
	getCallCount     int
	setCallCount     int
	triggerFailAtGet int // Trigger failure after N get calls
	triggerFailAtSet int // Trigger failure after N set calls
}

func newMockBackendWithFailure() *mockBackendWithFailure {
	return &mockBackendWithFailure{
		data:             make(map[string]interface{}),
		triggerFailAtGet: -1, // No failure by default
		triggerFailAtSet: -1,
	}
}

func (m *mockBackendWithFailure) Get(ctx context.Context, key string) (interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.getCallCount++
	if m.triggerFailAtGet > 0 && m.getCallCount >= m.triggerFailAtGet {
		m.failGetCount++
		return nil, errors.New("get backend failure")
	}
	if val, ok := m.data[key]; ok {
		return val, nil
	}
	return nil, backend.ErrKeyNotFound
}

func (m *mockBackendWithFailure) Set(ctx context.Context, key string, state interface{}, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.setCallCount++
	if m.triggerFailAtSet > 0 && m.setCallCount >= m.triggerFailAtSet {
		m.failSetCount++
		return errors.New("set backend failure")
	}
	m.data[key] = state
	return nil
}

func (m *mockBackendWithFailure) IncrementAndGet(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.triggerFailAtSet > 0 && m.setCallCount >= m.triggerFailAtSet {
		return 0, errors.New("increment backend failure")
	}
	var current int64 = 0
	if val, ok := m.data[key]; ok {
		if v, ok := val.(int64); ok {
			current = v
		}
	}
	current++
	m.data[key] = current
	return current, nil
}

func (m *mockBackendWithFailure) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, key)
	return nil
}

func (m *mockBackendWithFailure) Close() error {
	return nil
}

func (m *mockBackendWithFailure) Ping(ctx context.Context) error {
	return nil
}

func (m *mockBackendWithFailure) Type() string {
	return "mock-failure"
}

// TestBasicLimiterWithoutFailover validates basic limiter works without failover
func TestBasicLimiterWithoutFailover(t *testing.T) {
	primaryBE := newMockBackendWithFailure()

	cfg := limiters.BasicConfig{
		AlgorithmName: "fixed_window",
		DefaultLimit:  5,
		DefaultWindow: time.Second,
		Failover: limiters.FailoverConfig{
			EnableFailover: false, // Disabled
		},
	}

	limiter, err := NewBasicLimiter(
		&mockAlgorithm{},
		primaryBE,
		cfg,
	)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	// First request should succeed
	result := limiter.Allow(ctx, "test-key")
	if !result.Allowed {
		t.Error("First request should be allowed")
	}

	t.Log("Basic limiter without failover test passed")
}

// TestBasicLimiterWithFailoverNoFailure validates limiter with failover enabled but no failures
func TestBasicLimiterWithFailoverNoFailure(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	fallbackBE := newMockBackendWithFailure()

	// Create failover handler
	failoverCfg := features.FailoverConfig{
		EnableFallback:        true,
		FailureThreshold:      2,
		HealthCheckInterval:   100 * time.Millisecond,
		RecoveryCheckInterval: 100 * time.Millisecond,
		AutoRecovery:          true,
	}

	failoverHandler, err := features.NewFailoverManager(
		primaryBE,
		fallbackBE,
		failoverCfg,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Failed to create failover handler: %v", err)
	}

	cfg := limiters.BasicConfig{
		AlgorithmName: "fixed_window",
		DefaultLimit:  5,
		DefaultWindow: time.Second,
		Failover: limiters.FailoverConfig{
			EnableFailover: true,
			Handler:        failoverHandler,
		},
	}

	limiter, err := NewBasicLimiter(
		&mockAlgorithm{},
		primaryBE,
		cfg,
	)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	// Request should succeed with no failures
	result := limiter.Allow(ctx, "test-key")
	if !result.Allowed {
		t.Error("Request should be allowed when no failures occur")
	}

	// Verify failover is not active
	status := failoverHandler.GetFallbackStatus(ctx)
	if status.IsFallbackActive {
		t.Error("Failover should not be active when no failures occur")
	}

	t.Log("Basic limiter with failover (no failure) test passed")
}

// TestBasicLimiterFailoverOnGetFailure validates failover is triggered on get failures
func TestBasicLimiterFailoverOnGetFailure(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	primaryBE.triggerFailAtGet = 1 // Fail on first get
	fallbackBE := newMockBackendWithFailure()

	// Create failover handler
	failoverCfg := features.FailoverConfig{
		EnableFallback:        true,
		FailureThreshold:      1,
		HealthCheckInterval:   100 * time.Millisecond,
		RecoveryCheckInterval: 100 * time.Millisecond,
	}

	failoverHandler, err := features.NewFailoverManager(
		primaryBE,
		fallbackBE,
		failoverCfg,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Failed to create failover handler: %v", err)
	}

	cfg := limiters.BasicConfig{
		AlgorithmName: "fixed_window",
		DefaultLimit:  5,
		DefaultWindow: time.Second,
		Failover: limiters.FailoverConfig{
			EnableFailover: true,
			Handler:        failoverHandler,
		},
	}

	limiter, err := NewBasicLimiter(
		&mockAlgorithm{},
		primaryBE,
		cfg,
	)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	// Request will trigger get failure
	result := limiter.Allow(ctx, "test-key")

	// During failover, request should be allowed (degraded mode)
	if !result.Allowed {
		t.Errorf("Request should be allowed during failover, got error: %v", result.Error)
	}

	// Verify failover is now active
	status := failoverHandler.GetFallbackStatus(ctx)
	if !status.IsFallbackActive {
		t.Error("Failover should be active after failure")
	}

	t.Log("Basic limiter failover on get failure test passed")
}

// TestBasicLimiterFailoverOnSetFailure validates failover is triggered on set failures
func TestBasicLimiterFailoverOnSetFailure(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	primaryBE.triggerFailAtSet = 1 // Fail on first set
	fallbackBE := newMockBackendWithFailure()

	// Create failover handler
	failoverCfg := features.FailoverConfig{
		EnableFallback:        true,
		FailureThreshold:      1,
		HealthCheckInterval:   100 * time.Millisecond,
		RecoveryCheckInterval: 100 * time.Millisecond,
	}

	failoverHandler, err := features.NewFailoverManager(
		primaryBE,
		fallbackBE,
		failoverCfg,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Failed to create failover handler: %v", err)
	}

	cfg := limiters.BasicConfig{
		AlgorithmName: "fixed_window",
		DefaultLimit:  5,
		DefaultWindow: time.Second,
		Failover: limiters.FailoverConfig{
			EnableFailover: true,
			Handler:        failoverHandler,
		},
	}

	limiter, err := NewBasicLimiter(
		&mockAlgorithm{},
		primaryBE,
		cfg,
	)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	// Request will trigger set failure
	result := limiter.Allow(ctx, "test-key")

	// During failover, request should be allowed (degraded mode)
	if !result.Allowed {
		t.Errorf("Request should be allowed during failover, got error: %v", result.Error)
	}

	// Verify failover is now active
	status := failoverHandler.GetFallbackStatus(ctx)
	if !status.IsFallbackActive {
		t.Error("Failover should be active after failure")
	}

	t.Log("Basic limiter failover on set failure test passed")
}

// TestBasicLimiterMultipleFailuresBeforeFailover validates threshold-based failover
func TestBasicLimiterMultipleFailuresBeforeFailover(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	primaryBE.triggerFailAtGet = 2 // Fail on 2nd get and beyond
	fallbackBE := newMockBackendWithFailure()

	// Create failover handler with threshold of 2
	failoverCfg := features.FailoverConfig{
		EnableFallback:        true,
		FailureThreshold:      2,
		HealthCheckInterval:   100 * time.Millisecond,
		RecoveryCheckInterval: 100 * time.Millisecond,
	}

	failoverHandler, err := features.NewFailoverManager(
		primaryBE,
		fallbackBE,
		failoverCfg,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Failed to create failover handler: %v", err)
	}

	cfg := limiters.BasicConfig{
		AlgorithmName: "fixed_window",
		DefaultLimit:  5,
		DefaultWindow: time.Second,
		Failover: limiters.FailoverConfig{
			EnableFailover: true,
			Handler:        failoverHandler,
		},
	}

	limiter, err := NewBasicLimiter(
		&mockAlgorithm{},
		primaryBE,
		cfg,
	)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	// First request - succeeds (getCallCount=1)
	result1 := limiter.Allow(ctx, "test-key")
	if !result1.Allowed {
		t.Error("First request should succeed")
	}

	status := failoverHandler.GetFallbackStatus(ctx)
	if status.IsFallbackActive {
		t.Error("Failover should not be active after 1 request")
	}

	// Second request - fails (getCallCount=2, >= triggerFailAtGet=2), first failure
	result2 := limiter.Allow(ctx, "test-key")
	t.Logf("Second request allowed: %v, error: %v", result2.Allowed, result2.Error)

	status = failoverHandler.GetFallbackStatus(ctx)
	t.Logf("After 1st failure: FailureCount=%d", status.FailureCount)
	if status.IsFallbackActive {
		t.Error("Failover should not be active after 1 failure (threshold is 2)")
	}

	// Third request - fails again (getCallCount=3), second failure, should trigger failover
	result3 := limiter.Allow(ctx, "test-key")
	t.Logf("Third request allowed: %v, error: %v", result3.Allowed, result3.Error)

	// After threshold exceeded, failover should be active
	status = failoverHandler.GetFallbackStatus(ctx)
	t.Logf("Failover status after 2nd failure: IsFallbackActive=%v, FailureCount=%d",
		status.IsFallbackActive, status.FailureCount)
	if !status.IsFallbackActive {
		t.Errorf("Failover should be active after threshold reached (failures: %d)", status.FailureCount)
	}

	t.Log("Basic limiter multiple failures before failover test passed")
}

// TestBasicLimiterFailoverErrorMessage validates error messages during failover
func TestBasicLimiterFailoverErrorMessage(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	primaryBE.triggerFailAtGet = 1 // Fail immediately
	fallbackBE := newMockBackendWithFailure()

	failoverCfg := features.FailoverConfig{
		EnableFallback:        true,
		FailureThreshold:      1,
		HealthCheckInterval:   100 * time.Millisecond,
		RecoveryCheckInterval: 100 * time.Millisecond,
	}

	failoverHandler, err := features.NewFailoverManager(
		primaryBE,
		fallbackBE,
		failoverCfg,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Failed to create failover handler: %v", err)
	}

	cfg := limiters.BasicConfig{
		AlgorithmName: "fixed_window",
		DefaultLimit:  5,
		DefaultWindow: time.Second,
		Failover: limiters.FailoverConfig{
			EnableFailover: true,
			Handler:        failoverHandler,
		},
	}

	limiter, err := NewBasicLimiter(
		&mockAlgorithm{},
		primaryBE,
		cfg,
	)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	result := limiter.Allow(ctx, "test-key")

	// Error should mention failover/fallback
	if result.Error == nil {
		t.Error("Expected error during failover")
	} else if !contains(result.Error.Error(), "fallback") && !contains(result.Error.Error(), "failure") {
		t.Errorf("Error should mention fallback or failure, got: %v", result.Error)
	}

	t.Log("Basic limiter failover error message test passed")
}

// TestBasicLimiterFailoverWithLogging validates logging during failover
func TestBasicLimiterFailoverWithLogging(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	primaryBE.triggerFailAtGet = 1 // Fail immediately
	fallbackBE := newMockBackendWithFailure()

	logger := &testLogger{messages: []string{}}

	failoverCfg := features.FailoverConfig{
		EnableFallback:        true,
		FailureThreshold:      1,
		HealthCheckInterval:   100 * time.Millisecond,
		RecoveryCheckInterval: 100 * time.Millisecond,
	}

	failoverHandler, err := features.NewFailoverManager(
		primaryBE,
		fallbackBE,
		failoverCfg,
		logger,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Failed to create failover handler: %v", err)
	}
	defer func() {
		if closer, ok := failoverHandler.(interface{ Close() error }); ok {
			closer.Close()
		}
	}()

	cfg := limiters.BasicConfig{
		AlgorithmName: "fixed_window",
		DefaultLimit:  5,
		DefaultWindow: time.Second,
		Observability: limiters.ObservabilityConfig{
			Logger: logger,
		},
		Failover: limiters.FailoverConfig{
			EnableFailover: true,
			Handler:        failoverHandler,
		},
	}

	limiter, err := NewBasicLimiter(
		&mockAlgorithm{},
		primaryBE,
		cfg,
	)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	_ = limiter.Allow(ctx, "test-key")

	// Verify logging occurred — lock the logger for safe reads
	logger.mu.Lock()
	msgCount := len(logger.messages)
	messagesCopy := make([]string, msgCount)
	copy(messagesCopy, logger.messages)
	logger.mu.Unlock()

	if msgCount == 0 {
		t.Error("Expected log messages during failover")
	}

	// Check for error log about backend failure
	foundError := false
	for _, msg := range messagesCopy {
		if contains(msg, "backend get failed") || contains(msg, "ERROR") || contains(msg, "atomic update failed") {
			foundError = true
			break
		}
	}

	if !foundError {
		t.Error("Expected error log message about backend failure")
	}

	t.Log("Basic limiter failover with logging test passed")
}

// Helper functions

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// mockAlgorithm for testing
type mockAlgorithm struct{}

func (m *mockAlgorithm) Allow(ctx context.Context, state interface{}, cost int) (interface{}, interface{}, error) {
	return state, &algorithm.TokenBucketResult{
		Allowed:         true,
		RemainingTokens: 5,
	}, nil
}

func (m *mockAlgorithm) Reset(ctx context.Context) (interface{}, error) {
	return map[string]interface{}{
		"counter": 0,
	}, nil
}

func (m *mockAlgorithm) GetStats(ctx context.Context, state interface{}) interface{} {
	return map[string]interface{}{
		"total": 10,
		"used":  5,
	}
}

func (m *mockAlgorithm) ValidateConfig(config interface{}) error {
	return nil
}

func (m *mockAlgorithm) Name() string {
	return "mock"
}

func (m *mockAlgorithm) Description() string {
	return "mock algorithm for testing"
}

// testLogger for testing
type testLogger struct {
	mu       sync.Mutex
	messages []string
}

func (t *testLogger) Debug(message string, args ...interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messages = append(t.messages, fmt.Sprintf("DEBUG: %s %v", message, args))
}

func (t *testLogger) Info(message string, args ...interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messages = append(t.messages, fmt.Sprintf("INFO: %s %v", message, args))
}

func (t *testLogger) Warn(message string, args ...interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messages = append(t.messages, fmt.Sprintf("WARN: %s %v", message, args))
}

func (t *testLogger) Error(message string, args ...interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messages = append(t.messages, fmt.Sprintf("ERROR: %s %v", message, args))
}

var _ obstypes.Logger = (*testLogger)(nil)
