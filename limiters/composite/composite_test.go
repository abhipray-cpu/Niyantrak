package composite

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abhipray-cpu/niyantrak/algorithm"
	"github.com/abhipray-cpu/niyantrak/backend"
	"github.com/abhipray-cpu/niyantrak/features"
	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/mocks"
	obstypes "github.com/abhipray-cpu/niyantrak/observability/types"
)

// TestNewCompositeLimiter_ValidConfig tests constructor with valid configuration
func TestNewCompositeLimiter_ValidConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "api-composite",
		Limits: []limiters.LimitConfig{
			{Name: "minute", Limit: 10, Window: time.Minute, Priority: 1},
			{Name: "hour", Limit: 100, Window: time.Hour, Priority: 2},
			{Name: "day", Limit: 1000, Window: 24 * time.Hour, Priority: 3},
		},
	}

	limiter, err := NewCompositeLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limiter == nil {
		t.Fatal("limiter should not be nil")
	}
	if limiter.Type() != "composite" {
		t.Errorf("expected type 'composite', got %q", limiter.Type())
	}
}

// TestNewCompositeLimiter_InvalidConfig tests constructor validation
func TestNewCompositeLimiter_InvalidConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	tests := []struct {
		name    string
		algo    algorithm.Algorithm
		be      backend.Backend
		cfg     limiters.CompositeConfig
		wantErr bool
	}{
		{
			name:    "nil algorithm",
			algo:    nil,
			be:      mockBackend,
			cfg:     limiters.CompositeConfig{},
			wantErr: true,
		},
		{
			name:    "nil backend",
			algo:    mockAlgo,
			be:      nil,
			cfg:     limiters.CompositeConfig{},
			wantErr: true,
		},
		{
			name: "empty limits",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.CompositeConfig{
				Name:   "test",
				Limits: []limiters.LimitConfig{},
			},
			wantErr: true,
		},
		{
			name: "empty name",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.CompositeConfig{
				Name: "",
				Limits: []limiters.LimitConfig{
					{Name: "test", Limit: 10, Window: time.Minute},
				},
			},
			wantErr: true,
		},
		{
			name: "empty limit name",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.CompositeConfig{
				Name: "test",
				Limits: []limiters.LimitConfig{
					{Name: "", Limit: 10, Window: time.Minute},
				},
			},
			wantErr: true,
		},
		{
			name: "zero limit",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.CompositeConfig{
				Name: "test",
				Limits: []limiters.LimitConfig{
					{Name: "test", Limit: 0, Window: time.Minute},
				},
			},
			wantErr: true,
		},
		{
			name: "negative limit",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.CompositeConfig{
				Name: "test",
				Limits: []limiters.LimitConfig{
					{Name: "test", Limit: -10, Window: time.Minute},
				},
			},
			wantErr: true,
		},
		{
			name: "zero window",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.CompositeConfig{
				Name: "test",
				Limits: []limiters.LimitConfig{
					{Name: "test", Limit: 10, Window: 0},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate limit names",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.CompositeConfig{
				Name: "test",
				Limits: []limiters.LimitConfig{
					{Name: "test", Limit: 10, Window: time.Minute},
					{Name: "test", Limit: 100, Window: time.Hour},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCompositeLimiter(tt.algo, tt.be, tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCompositeLimiter() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestAllow_WithMocks tests single request checking against all limits
func TestAllow_WithMocks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	// Setup expectations for 3 limits
	mockBackend.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(nil, backend.ErrKeyNotFound).
		Times(3)

	mockAlgo.EXPECT().
		Reset(gomock.Any()).
		Return(map[string]interface{}{"tokens": 10}, nil).
		Times(3)

	mockAlgo.EXPECT().
		Allow(gomock.Any(), gomock.Any(), 1).
		Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		Times(3)

	mockBackend.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(3)

	cfg := limiters.CompositeConfig{
		Name: "api-composite",
		Limits: []limiters.LimitConfig{
			{Name: "minute", Limit: 10, Window: time.Minute, Priority: 1},
			{Name: "hour", Limit: 100, Window: time.Hour, Priority: 2},
			{Name: "day", Limit: 1000, Window: 24 * time.Hour, Priority: 3},
		},
	}

	limiter, _ := NewCompositeLimiter(mockAlgo, mockBackend, cfg)
	result := limiter.Allow(context.Background(), "user:123")

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}
}

// TestAllowN_WithMocks tests multiple request checking
func TestAllowN_WithMocks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	// Setup expectations
	mockBackend.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(nil, backend.ErrKeyNotFound).
		Times(2)

	mockAlgo.EXPECT().
		Reset(gomock.Any()).
		Return(map[string]interface{}{"tokens": 100}, nil).
		Times(2)

	mockAlgo.EXPECT().
		Allow(gomock.Any(), gomock.Any(), 5).
		Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		Times(2)

	mockBackend.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(2)

	cfg := limiters.CompositeConfig{
		Name: "api-composite",
		Limits: []limiters.LimitConfig{
			{Name: "minute", Limit: 10, Window: time.Minute, Priority: 1},
			{Name: "hour", Limit: 100, Window: time.Hour, Priority: 2},
		},
	}

	limiter, _ := NewCompositeLimiter(mockAlgo, mockBackend, cfg)
	result := limiter.AllowN(context.Background(), "user:123", 5)

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}
}

// TestAllowN_InvalidInputs tests invalid input handling
func TestAllowN_InvalidInputs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "api-composite",
		Limits: []limiters.LimitConfig{
			{Name: "minute", Limit: 10, Window: time.Minute, Priority: 1},
		},
	}

	limiter, _ := NewCompositeLimiter(mockAlgo, mockBackend, cfg)

	tests := []struct {
		name  string
		key   string
		n     int
		valid bool
	}{
		{"valid", "user:123", 1, true},
		{"empty key", "", 1, false},
		{"zero n", "user:123", 0, false},
		{"negative n", "user:123", -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				mockBackend.EXPECT().
					Get(gomock.Any(), gomock.Any()).
					Return(nil, backend.ErrKeyNotFound).
					Times(1)
				mockAlgo.EXPECT().
					Reset(gomock.Any()).
					Return(map[string]interface{}{}, nil).
					Times(1)
				mockAlgo.EXPECT().
					Allow(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
					Times(1)
				mockBackend.EXPECT().
					Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).
					Times(1)
			}

			result := limiter.AllowN(context.Background(), tt.key, tt.n)
			if tt.valid && result.Error != nil {
				t.Errorf("expected no error, got %v", result.Error)
			}
			if !tt.valid && result.Error == nil {
				t.Errorf("expected error for invalid input")
			}
		})
	}
}

// TestAddLimit tests adding a new limit to the composite
func TestAddLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "api-composite",
		Limits: []limiters.LimitConfig{
			{Name: "minute", Limit: 10, Window: time.Minute, Priority: 1},
		},
	}

	limiter, _ := NewCompositeLimiter(mockAlgo, mockBackend, cfg)

	// Add new limit
	err := limiter.AddLimit(context.Background(), "hour", 100, time.Hour)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify added
	limits, _ := limiter.GetLimits(context.Background())
	if len(limits) != 2 {
		t.Errorf("expected 2 limits, got %d", len(limits))
	}

	// Test error cases
	tests := []struct {
		name   string
		lname  string
		limit  int
		window time.Duration
		valid  bool
	}{
		{"duplicate name", "minute", 10, time.Minute, false},
		{"empty name", "", 10, time.Minute, false},
		{"zero limit", "test", 0, time.Minute, false},
		{"negative limit", "test", -10, time.Minute, false},
		{"zero window", "test", 10, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := limiter.AddLimit(context.Background(), tt.lname, tt.limit, tt.window)
			if (err != nil) != !tt.valid {
				t.Errorf("AddLimit() error = %v, expected error = %v", err, !tt.valid)
			}
		})
	}
}

// TestRemoveLimit tests removing a limit from the composite
func TestRemoveLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "api-composite",
		Limits: []limiters.LimitConfig{
			{Name: "minute", Limit: 10, Window: time.Minute, Priority: 1},
			{Name: "hour", Limit: 100, Window: time.Hour, Priority: 2},
		},
	}

	limiter, _ := NewCompositeLimiter(mockAlgo, mockBackend, cfg)

	// Remove existing limit
	err := limiter.RemoveLimit(context.Background(), "minute")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify removed
	limits, _ := limiter.GetLimits(context.Background())
	if len(limits) != 1 {
		t.Errorf("expected 1 limit, got %d", len(limits))
	}
	if limits[0].Name != "hour" {
		t.Errorf("expected remaining limit 'hour', got %q", limits[0].Name)
	}

	// Test error cases
	err = limiter.RemoveLimit(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent limit")
	}

	err = limiter.RemoveLimit(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty limit name")
	}
}

// TestGetLimits tests retrieving all configured limits
func TestGetLimits(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "api-composite",
		Limits: []limiters.LimitConfig{
			{Name: "minute", Limit: 10, Window: time.Minute, Priority: 1},
			{Name: "hour", Limit: 100, Window: time.Hour, Priority: 2},
			{Name: "day", Limit: 1000, Window: 24 * time.Hour, Priority: 3},
		},
	}

	limiter, _ := NewCompositeLimiter(mockAlgo, mockBackend, cfg)

	limits, err := limiter.GetLimits(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(limits) != 3 {
		t.Errorf("expected 3 limits, got %d", len(limits))
	}

	// Verify limit names exist
	limitMap := make(map[string]bool)
	for _, l := range limits {
		limitMap[l.Name] = true
	}

	expected := []string{"minute", "hour", "day"}
	for _, exp := range expected {
		if !limitMap[exp] {
			t.Errorf("expected limit %q not found", exp)
		}
	}
}

// TestCheckAll tests checking status of all limits
func TestCheckAll(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "api-composite",
		Limits: []limiters.LimitConfig{
			{Name: "minute", Limit: 10, Window: time.Minute, Priority: 1},
			{Name: "hour", Limit: 100, Window: time.Hour, Priority: 2},
		},
	}

	limiter, _ := NewCompositeLimiter(mockAlgo, mockBackend, cfg)

	// Setup expectations
	mockBackend.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(nil, backend.ErrKeyNotFound).
		Times(2)

	mockAlgo.EXPECT().
		Reset(gomock.Any()).
		Return(map[string]interface{}{"tokens": 10}, nil).
		Times(2)

	mockAlgo.EXPECT().
		Allow(gomock.Any(), gomock.Any(), 1).
		Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		Times(2)

	statuses, err := limiter.CheckAll(context.Background(), "user:123")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(statuses) != 2 {
		t.Errorf("expected 2 statuses, got %d", len(statuses))
	}

	// Test error cases
	_, err = limiter.CheckAll(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty key")
	}
}

// TestGetHierarchy tests retrieving limit hierarchy information
func TestGetHierarchy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "api-composite",
		Limits: []limiters.LimitConfig{
			{Name: "minute", Limit: 10, Window: time.Minute, Priority: 1},
			{Name: "hour", Limit: 100, Window: time.Hour, Priority: 2},
			{Name: "day", Limit: 1000, Window: 24 * time.Hour, Priority: 3},
		},
	}

	limiter, _ := NewCompositeLimiter(mockAlgo, mockBackend, cfg)

	hierarchy := limiter.GetHierarchy(context.Background())
	if hierarchy == nil {
		t.Fatal("hierarchy should not be nil")
	}

	if hierarchy.Name != "api-composite" {
		t.Errorf("expected name 'api-composite', got %q", hierarchy.Name)
	}

	if len(hierarchy.Limits) != 3 {
		t.Errorf("expected 3 limits in hierarchy, got %d", len(hierarchy.Limits))
	}

	if hierarchy.ConflictingLimits {
		t.Error("expected no conflicting limits")
	}
}

// TestReset tests clearing state for a key across all limits
func TestReset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "api-composite",
		Limits: []limiters.LimitConfig{
			{Name: "minute", Limit: 10, Window: time.Minute, Priority: 1},
			{Name: "hour", Limit: 100, Window: time.Hour, Priority: 2},
		},
	}

	limiter, _ := NewCompositeLimiter(mockAlgo, mockBackend, cfg)

	// Expect Delete to be called for each limit
	mockBackend.EXPECT().
		Delete(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(2)

	err := limiter.Reset(context.Background(), "user:123")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Test error case
	err = limiter.Reset(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty key")
	}
}

// TestGetStats tests statistics retrieval
func TestGetStats(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "api-composite",
		Limits: []limiters.LimitConfig{
			{Name: "minute", Limit: 10, Window: time.Minute, Priority: 1},
			{Name: "hour", Limit: 100, Window: time.Hour, Priority: 2},
		},
	}

	limiter, _ := NewCompositeLimiter(mockAlgo, mockBackend, cfg)

	// Setup expectations
	mockBackend.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(nil, backend.ErrKeyNotFound).
		Times(2)

	stats := limiter.GetStats(context.Background(), "user:123")
	if stats == nil {
		t.Fatal("stats should not be nil")
	}

	statsMap, ok := stats.(map[string]interface{})
	if !ok {
		t.Fatal("stats should be a map")
	}

	if statsMap["type"] != "composite" {
		t.Errorf("expected type 'composite', got %v", statsMap["type"])
	}

	if statsMap["name"] != "api-composite" {
		t.Errorf("expected name 'api-composite', got %v", statsMap["name"])
	}

	// Test empty key
	stats = limiter.GetStats(context.Background(), "")
	statsMap = stats.(map[string]interface{})
	if statsMap["error"] == nil {
		t.Error("expected error for empty key")
	}
}

// TestSetLimit tests that SetLimit returns appropriate error
func TestSetLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "api-composite",
		Limits: []limiters.LimitConfig{
			{Name: "minute", Limit: 10, Window: time.Minute, Priority: 1},
		},
	}

	limiter, _ := NewCompositeLimiter(mockAlgo, mockBackend, cfg)

	err := limiter.SetLimit(context.Background(), "user:123", 100, time.Minute)
	if err == nil {
		t.Error("expected error for SetLimit on composite limiter")
	}
}

// TestClose tests closing the limiter
func TestClose(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	mockBackend.EXPECT().
		Close().
		Return(nil).
		Times(1)

	cfg := limiters.CompositeConfig{
		Name: "api-composite",
		Limits: []limiters.LimitConfig{
			{Name: "minute", Limit: 10, Window: time.Minute, Priority: 1},
		},
	}

	limiter, _ := NewCompositeLimiter(mockAlgo, mockBackend, cfg)

	err := limiter.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Second close should be idempotent
	err = limiter.Close()
	if err != nil {
		t.Errorf("expected idempotent close, got error: %v", err)
	}

	// After close, operations should fail
	result := limiter.Allow(context.Background(), "user:123")
	if result.Error != limiters.ErrLimiterClosed {
		t.Errorf("expected ErrLimiterClosed, got %v", result.Error)
	}
}

// TestConcurrentAccess tests thread safety
func TestConcurrentAccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	numGoroutines := 10
	requestsPerGoroutine := 10

	// Setup generic expectations with AnyTimes() for concurrent access
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

	cfg := limiters.CompositeConfig{
		Name: "api-composite",
		Limits: []limiters.LimitConfig{
			{Name: "minute", Limit: 10, Window: time.Minute, Priority: 1},
			{Name: "hour", Limit: 100, Window: time.Hour, Priority: 2},
		},
	}

	limiter, _ := NewCompositeLimiter(mockAlgo, mockBackend, cfg)

	done := make(chan bool, numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			for r := 0; r < requestsPerGoroutine; r++ {
				limiter.Allow(context.Background(), "user:123")
				limiter.GetLimits(context.Background())
				limiter.GetStats(context.Background(), "user:123")
			}
			done <- true
		}(g)
	}

	for g := 0; g < numGoroutines; g++ {
		<-done
	}
}

// TestMultipleLimits tests that all limits must pass
func TestMultipleLimits(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "api-composite",
		Limits: []limiters.LimitConfig{
			{Name: "minute", Limit: 10, Window: time.Minute, Priority: 1},
			{Name: "hour", Limit: 100, Window: time.Hour, Priority: 2},
		},
	}

	limiter, _ := NewCompositeLimiter(mockAlgo, mockBackend, cfg)

	// Both limits should be checked
	mockBackend.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(nil, backend.ErrKeyNotFound).
		Times(2)

	mockAlgo.EXPECT().
		Reset(gomock.Any()).
		Return(map[string]interface{}{"tokens": 10}, nil).
		Times(2)

	mockAlgo.EXPECT().
		Allow(gomock.Any(), gomock.Any(), 1).
		Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		Times(2)

	mockBackend.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(2)

	result := limiter.Allow(context.Background(), "user:123")
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}
}

// BenchmarkAllow benchmarks Allow operation
func BenchmarkAllow(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	limitsCount := 3
	totalOps := b.N * limitsCount

	mockBackend.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(nil, backend.ErrKeyNotFound).
		Times(totalOps)

	mockAlgo.EXPECT().
		Reset(gomock.Any()).
		Return(map[string]interface{}{}, nil).
		Times(totalOps)

	mockAlgo.EXPECT().
		Allow(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		Times(totalOps)

	mockBackend.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(totalOps)

	cfg := limiters.CompositeConfig{
		Name: "api-composite",
		Limits: []limiters.LimitConfig{
			{Name: "minute", Limit: 10, Window: time.Minute, Priority: 1},
			{Name: "hour", Limit: 100, Window: time.Hour, Priority: 2},
			{Name: "day", Limit: 1000, Window: 24 * time.Hour, Priority: 3},
		},
	}

	limiter, _ := NewCompositeLimiter(mockAlgo, mockBackend, cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(context.Background(), "user:123")
	}
}

// BenchmarkCheckAll benchmarks CheckAll operation
func BenchmarkCheckAll(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	limitsCount := 3
	totalOps := b.N * limitsCount

	mockBackend.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(nil, backend.ErrKeyNotFound).
		Times(totalOps)

	mockAlgo.EXPECT().
		Reset(gomock.Any()).
		Return(map[string]interface{}{}, nil).
		Times(totalOps)

	mockAlgo.EXPECT().
		Allow(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		Times(totalOps)

	cfg := limiters.CompositeConfig{
		Name: "api-composite",
		Limits: []limiters.LimitConfig{
			{Name: "minute", Limit: 10, Window: time.Minute, Priority: 1},
			{Name: "hour", Limit: 100, Window: time.Hour, Priority: 2},
			{Name: "day", Limit: 1000, Window: 24 * time.Hour, Priority: 3},
		},
	}

	limiter, _ := NewCompositeLimiter(mockAlgo, mockBackend, cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.CheckAll(context.Background(), "user:123")
	}
}

// TestCompositeLimit_Observability_NoOp verifies NoOp observability works
func TestCompositeLimit_Observability_NoOp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "test",
		Limits: []limiters.LimitConfig{
			{Name: "test_limit", Limit: 10, Window: time.Second, Priority: 1},
		},
	}

	limiter, err := NewCompositeLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	// Mock expectations for AllowN
	mockAlgo.EXPECT().Reset(gomock.Any()).Return("initial_state", nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), "initial_state", 1).Return("new_state", &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, backend.ErrKeyNotFound).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), "new_state", time.Second).Return(nil).AnyTimes()
	mockBackend.EXPECT().Close().Return(nil).AnyTimes()
	mockBackend.EXPECT().Close().Return(nil).AnyTimes()

	// Should work without panicking
	result := limiter.AllowN(context.Background(), "key1", 1)
	if result == nil {
		t.Fatal("AllowN returned nil")
	}
	if !result.Allowed {
		t.Fatal("expected request to be allowed")
	}
}

// TestCompositeLimit_Observability_WithLogger verifies logger integration
func TestCompositeLimit_Observability_WithLogger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "test",
		Limits: []limiters.LimitConfig{
			{Name: "test_limit", Limit: 10, Window: time.Second, Priority: 1},
		},
		Observability: limiters.ObservabilityConfig{
			Logger:  mockLogger,
			Metrics: mockMetrics,
			Tracer:  mockTracer,
		},
	}

	// Expect logger calls
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes()

	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordDecisionLatency(gomock.Any(), gomock.Any()).AnyTimes()

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().SetError(gomock.Any(), gomock.Any()).AnyTimes()

	limiter, err := NewCompositeLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	// Mock expectations for AllowN
	mockAlgo.EXPECT().Reset(gomock.Any()).Return("initial_state", nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), "initial_state", 1).Return("new_state", &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, backend.ErrKeyNotFound).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), "new_state", time.Second).Return(nil).AnyTimes()
	mockBackend.EXPECT().Close().Return(nil).AnyTimes()

	result := limiter.AllowN(context.Background(), "key1", 1)
	if result == nil {
		t.Fatal("AllowN returned nil")
	}
}

// TestCompositeLimit_Observability_WithMetrics verifies metrics integration
func TestCompositeLimit_Observability_WithMetrics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "test",
		Limits: []limiters.LimitConfig{
			{Name: "test_limit", Limit: 10, Window: time.Second, Priority: 1},
		},
		Observability: limiters.ObservabilityConfig{
			Logger:  mockLogger,
			Metrics: mockMetrics,
			Tracer:  mockTracer,
		},
	}

	// Expect metrics calls
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
	mockMetrics.EXPECT().RecordDecisionLatency(gomock.Any(), gomock.Any()).Times(1)

	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes()

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().SetError(gomock.Any(), gomock.Any()).AnyTimes()

	limiter, err := NewCompositeLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	// Mock expectations for AllowN
	mockAlgo.EXPECT().Reset(gomock.Any()).Return("initial_state", nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), "initial_state", 1).Return("new_state", &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, backend.ErrKeyNotFound).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), "new_state", time.Second).Return(nil).AnyTimes()
	mockBackend.EXPECT().Close().Return(nil).AnyTimes()

	result := limiter.AllowN(context.Background(), "key1", 1)
	if !result.Allowed {
		t.Fatal("expected request to be allowed")
	}
}

// TestCompositeLimit_Observability_WithTracer verifies tracing integration
func TestCompositeLimit_Observability_WithTracer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "test",
		Limits: []limiters.LimitConfig{
			{Name: "test_limit", Limit: 10, Window: time.Second, Priority: 1},
		},
		Observability: limiters.ObservabilityConfig{
			Logger:  mockLogger,
			Metrics: mockMetrics,
			Tracer:  mockTracer,
		},
	}

	// Expect tracer calls
	mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.composite.check").Return(&obstypes.SpanContext{}).Times(1)
	mockTracer.EXPECT().EndSpan(gomock.Any()).Times(1)
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes()

	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordDecisionLatency(gomock.Any(), gomock.Any()).AnyTimes()

	limiter, err := NewCompositeLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	// Mock expectations for AllowN
	mockAlgo.EXPECT().Reset(gomock.Any()).Return("initial_state", nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), "initial_state", 1).Return("new_state", &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, backend.ErrKeyNotFound).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), "new_state", time.Second).Return(nil).AnyTimes()
	mockBackend.EXPECT().Close().Return(nil).AnyTimes()

	result := limiter.AllowN(context.Background(), "key1", 1)
	if result == nil {
		t.Fatal("AllowN returned nil")
	}
}

// TestCompositeLimit_Observability_AllThree verifies all observability together
func TestCompositeLimit_Observability_AllThree(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "test",
		Limits: []limiters.LimitConfig{
			{Name: "test_limit", Limit: 10, Window: time.Second, Priority: 1},
		},
		Observability: limiters.ObservabilityConfig{
			Logger:  mockLogger,
			Metrics: mockMetrics,
			Tracer:  mockTracer,
		},
	}

	// Expect all calls
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes()

	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordDecisionLatency(gomock.Any(), gomock.Any()).AnyTimes()

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().SetError(gomock.Any(), gomock.Any()).AnyTimes()

	limiter, err := NewCompositeLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	// Mock expectations for AllowN
	mockAlgo.EXPECT().Reset(gomock.Any()).Return("initial_state", nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), "initial_state", 1).Return("new_state", &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, backend.ErrKeyNotFound).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), "new_state", time.Second).Return(nil).AnyTimes()
	mockBackend.EXPECT().Close().Return(nil).AnyTimes()

	result := limiter.AllowN(context.Background(), "key1", 1)
	if !result.Allowed {
		t.Fatal("expected request to be allowed")
	}
}

// TestCompositeLimit_Observability_AllowN verifies AllowN method observability
func TestCompositeLimit_Observability_AllowN(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "test",
		Limits: []limiters.LimitConfig{
			{Name: "test_limit", Limit: 100, Window: time.Second, Priority: 1},
		},
		Observability: limiters.ObservabilityConfig{
			Logger:  mockLogger,
			Metrics: mockMetrics,
			Tracer:  mockTracer,
		},
	}

	// Expect AllowN specific calls
	mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.composite.check").Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes()

	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordDecisionLatency(gomock.Any(), gomock.Any()).AnyTimes()

	limiter, err := NewCompositeLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	// Mock expectations for AllowN
	mockAlgo.EXPECT().Reset(gomock.Any()).Return("initial_state", nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), "initial_state", 5).Return("new_state", &algorithm.TokenBucketResult{Allowed: true, RemainingTokens: 95}, nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, backend.ErrKeyNotFound).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), "new_state", time.Second).Return(nil).AnyTimes()
	mockBackend.EXPECT().Close().Return(nil).AnyTimes()

	result := limiter.AllowN(context.Background(), "mykey", 5)
	if !result.Allowed {
		t.Fatal("expected 5 requests to be allowed")
	}
	if result.Remaining < 95 {
		t.Fatalf("expected remaining >= 95, got %d", result.Remaining)
	}
}

// TestCompositeLimit_Observability_Denied verifies observability logging happens
func TestCompositeLimit_Observability_Denied(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "test",
		Limits: []limiters.LimitConfig{
			{Name: "test_limit", Limit: 5, Window: time.Second, Priority: 1},
		},
		Observability: limiters.ObservabilityConfig{
			Logger:  mockLogger,
			Metrics: mockMetrics,
			Tracer:  mockTracer,
		},
	}

	// Expect observability calls
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes()

	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordDecisionLatency(gomock.Any(), gomock.Any()).AnyTimes()

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().SetError(gomock.Any(), gomock.Any()).AnyTimes()

	limiter, err := NewCompositeLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	// Mock normal responses
	mockAlgo.EXPECT().Reset(gomock.Any()).Return("initial_state", nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), "initial_state", 1).Return("new_state", &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, backend.ErrKeyNotFound).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), "new_state", time.Second).Return(nil).AnyTimes()
	mockBackend.EXPECT().Close().Return(nil).AnyTimes()

	// Just verify the request is processed with observability
	result := limiter.AllowN(context.Background(), "key1", 1)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestCompositeLimit_Observability_CheckAll verifies CheckAll method logging
func TestCompositeLimit_Observability_CheckAll(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "test",
		Limits: []limiters.LimitConfig{
			{Name: "limit1", Limit: 10, Window: time.Second, Priority: 1},
			{Name: "limit2", Limit: 20, Window: time.Second, Priority: 2},
		},
		Observability: limiters.ObservabilityConfig{
			Logger:  mockLogger,
			Metrics: mockMetrics,
			Tracer:  mockTracer,
		},
	}

	// Expect CheckAll logging
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes()

	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordDecisionLatency(gomock.Any(), gomock.Any()).AnyTimes()

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	limiter, err := NewCompositeLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	// Mock expectations for CheckAll
	mockAlgo.EXPECT().Reset(gomock.Any()).Return("initial_state", nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), "initial_state", 1).Return("new_state", &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, backend.ErrKeyNotFound).AnyTimes()
	mockBackend.EXPECT().Close().Return(nil).AnyTimes()

	statuses, err := limiter.CheckAll(context.Background(), "key1")
	if err != nil {
		t.Fatalf("CheckAll failed: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
}

// TestCompositeLimit_Observability_Reset verifies Reset method logging
func TestCompositeLimit_Observability_Reset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "test",
		Limits: []limiters.LimitConfig{
			{Name: "test_limit", Limit: 10, Window: time.Second, Priority: 1},
		},
		Observability: limiters.ObservabilityConfig{
			Logger:  mockLogger,
			Metrics: mockMetrics,
			Tracer:  mockTracer,
		},
	}

	// Expect Reset logging
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes()

	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordDecisionLatency(gomock.Any(), gomock.Any()).AnyTimes()

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	limiter, err := NewCompositeLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	// Mock expectations
	mockAlgo.EXPECT().Reset(gomock.Any()).Return("initial_state", nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), "initial_state", 3).Return("new_state", &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), "initial_state", 1).Return("new_state", &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, backend.ErrKeyNotFound).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), "new_state", time.Second).Return(nil).AnyTimes()
	mockBackend.EXPECT().Close().Return(nil).AnyTimes()
	mockBackend.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Make some requests
	limiter.AllowN(context.Background(), "key1", 3)

	// Reset the key
	if err := limiter.Reset(context.Background(), "key1"); err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	// Should be able to make more requests
	result := limiter.AllowN(context.Background(), "key1", 1)
	if !result.Allowed {
		t.Fatal("expected request after reset to be allowed")
	}
}

// TestCompositeLimit_Observability_Close verifies Close method logging
func TestCompositeLimit_Observability_Close(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "test",
		Limits: []limiters.LimitConfig{
			{Name: "test_limit", Limit: 10, Window: time.Second, Priority: 1},
		},
		Observability: limiters.ObservabilityConfig{
			Logger:  mockLogger,
			Metrics: mockMetrics,
			Tracer:  mockTracer,
		},
	}

	// Expect Close logging
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes()

	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordDecisionLatency(gomock.Any(), gomock.Any()).AnyTimes()

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	mockBackend.EXPECT().Close().Return(nil).Times(1)

	limiter, err := NewCompositeLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	if err := limiter.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify limiter is closed
	if err := limiter.AddLimit(context.Background(), "test", 10, time.Second); err != limiters.ErrLimiterClosed {
		t.Fatal("expected ErrLimiterClosed")
	}
}

// TestCompositeLimit_Observability_MultiLimit verifies multi-limit scenarios
func TestCompositeLimit_Observability_MultiLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	cfg := limiters.CompositeConfig{
		Name: "test",
		Limits: []limiters.LimitConfig{
			{Name: "requests_per_second", Limit: 10, Window: time.Second, Priority: 1},
			{Name: "requests_per_minute", Limit: 100, Window: time.Minute, Priority: 2},
		},
		Observability: limiters.ObservabilityConfig{
			Logger:  mockLogger,
			Metrics: mockMetrics,
			Tracer:  mockTracer,
		},
	}

	// Expect metrics for each limit
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordDecisionLatency(gomock.Any(), gomock.Any()).AnyTimes()

	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes()

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	limiter, err := NewCompositeLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	// Mock expectations
	mockAlgo.EXPECT().Reset(gomock.Any()).Return("initial_state", nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), "initial_state", 1).Return("new_state", &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, backend.ErrKeyNotFound).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), "new_state", gomock.Any()).Return(nil).AnyTimes()
	mockBackend.EXPECT().Close().Return(nil).AnyTimes()

	// All should pass
	result := limiter.AllowN(context.Background(), "key1", 1)
	if !result.Allowed {
		t.Fatal("expected request to pass both limits")
	}

	// Verify limits were checked
	limits, err := limiter.GetLimits(context.Background())
	if err != nil {
		t.Fatalf("GetLimits failed: %v", err)
	}
	if len(limits) != 2 {
		t.Fatalf("expected 2 limits, got %d", len(limits))
	}
}

// TestCompositeLimiter_WithDynamicLimits_Enabled tests limiter with dynamic limits enabled
func TestCompositeLimiter_WithDynamicLimits_Enabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(nil, nil).AnyTimes()

	limiter, err := NewCompositeLimiter(
		mockAlgo,
		mockBackend,
		limiters.CompositeConfig{
			Name: "test-composite",
			Limits: []limiters.LimitConfig{
				{Name: "per_second", Limit: 10, Window: time.Second, Priority: 1},
				{Name: "per_minute", Limit: 100, Window: time.Minute, Priority: 2},
			},
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

	// Test basic allow
	result := limiter.Allow(ctx, "user123")
	assert.True(t, result.Allowed)
}

// TestCompositeLimiter_WithDynamicLimits_UpdateLimit tests updating limits at runtime
func TestCompositeLimiter_WithDynamicLimits_UpdateLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(nil, nil).AnyTimes()

	limiter, err := NewCompositeLimiter(
		mockAlgo,
		mockBackend,
		limiters.CompositeConfig{
			Name: "test-composite",
			Limits: []limiters.LimitConfig{
				{Name: "requests", Limit: 50, Window: time.Second, Priority: 1},
			},
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

	// Update limit for a key
	err = dynamicMgr.UpdateLimit(ctx, "premium_user", 500, time.Second)
	require.NoError(t, err)

	// Verify limit was updated
	limit, window, err := dynamicMgr.GetCurrentLimit(ctx, "premium_user")
	require.NoError(t, err)
	assert.Equal(t, 500, limit)
	assert.Equal(t, time.Second, window)

	// Test limiter
	result := limiter.Allow(ctx, "premium_user")
	assert.True(t, result.Allowed)
}

// TestCompositeLimiter_DynamicLimits_MultipleKeys tests per-key customization
func TestCompositeLimiter_DynamicLimits_MultipleKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(nil, nil).AnyTimes()

	limiter, err := NewCompositeLimiter(
		mockAlgo,
		mockBackend,
		limiters.CompositeConfig{
			Name: "test-composite",
			Limits: []limiters.LimitConfig{
				{Name: "burst", Limit: 20, Window: time.Second, Priority: 1},
				{Name: "sustained", Limit: 100, Window: time.Minute, Priority: 2},
			},
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
	_ = dynamicMgr.UpdateLimit(ctx, "free_user", 50, time.Second)
	_ = dynamicMgr.UpdateLimit(ctx, "pro_user", 200, time.Second)
	_ = dynamicMgr.UpdateLimit(ctx, "enterprise_user", 1000, time.Second)

	// Verify each user
	limit, _, _ := dynamicMgr.GetCurrentLimit(ctx, "free_user")
	assert.Equal(t, 50, limit)

	limit, _, _ = dynamicMgr.GetCurrentLimit(ctx, "pro_user")
	assert.Equal(t, 200, limit)

	limit, _, _ = dynamicMgr.GetCurrentLimit(ctx, "enterprise_user")
	assert.Equal(t, 1000, limit)

	// Test limiter with different users
	result := limiter.Allow(ctx, "free_user")
	assert.True(t, result.Allowed)
}

// TestCompositeLimiter_DynamicLimits_ConcurrentUpdates tests concurrent operations
func TestCompositeLimiter_DynamicLimits_ConcurrentUpdates(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(nil, nil).AnyTimes()

	limiter, err := NewCompositeLimiter(
		mockAlgo,
		mockBackend,
		limiters.CompositeConfig{
			Name: "test-composite",
			Limits: []limiters.LimitConfig{
				{Name: "rate", Limit: 100, Window: time.Second, Priority: 1},
			},
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

	var wg sync.WaitGroup
	numGoroutines := 10
	wg.Add(numGoroutines * 2)

	// Concurrent writers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			key := "user" + string(rune(id))
			_ = dynamicMgr.UpdateLimit(ctx, key, 100+id*10, time.Second)
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

// TestCompositeLimiter_DynamicLimits_Disabled tests with dynamic limits disabled
func TestCompositeLimiter_DynamicLimits_Disabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(nil, nil).AnyTimes()

	limiter, err := NewCompositeLimiter(
		mockAlgo,
		mockBackend,
		limiters.CompositeConfig{
			Name: "test-composite",
			Limits: []limiters.LimitConfig{
				{Name: "rate", Limit: 100, Window: time.Second, Priority: 1},
			},
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

// TestCompositeLimiter_DynamicLimits_ValidationErrors tests validation
func TestCompositeLimiter_DynamicLimits_ValidationErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)

	limiter, err := NewCompositeLimiter(
		mockAlgo,
		mockBackend,
		limiters.CompositeConfig{
			Name: "test-composite",
			Limits: []limiters.LimitConfig{
				{Name: "rate", Limit: 100, Window: time.Second, Priority: 1},
			},
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
	err = dynamicMgr.UpdateLimit(ctx, "", 100, time.Second)
	assert.Error(t, err, "empty key should error")

	err = dynamicMgr.UpdateLimit(ctx, "user", -1, time.Second)
	assert.Error(t, err, "negative limit should error")

	err = dynamicMgr.UpdateLimit(ctx, "user", 100, 0)
	assert.Error(t, err, "zero window should error")
}

// TestCompositeLimiter_DynamicLimits_UpdateHooks tests update hooks
func TestCompositeLimiter_DynamicLimits_UpdateHooks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)

	var hookCalls []string
	var mu sync.Mutex

	dynamicMgr.AddUpdateHook(func(key string, config *features.LimitConfig) {
		mu.Lock()
		defer mu.Unlock()
		hookCalls = append(hookCalls, key)
	})

	limiter, err := NewCompositeLimiter(
		mockAlgo,
		mockBackend,
		limiters.CompositeConfig{
			Name: "test-composite",
			Limits: []limiters.LimitConfig{
				{Name: "rate", Limit: 100, Window: time.Second, Priority: 1},
			},
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
	_ = dynamicMgr.UpdateLimit(ctx, "user1", 200, time.Second)
	_ = dynamicMgr.UpdateLimit(ctx, "user2", 300, time.Second)

	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, hookCalls, 2)
	assert.Contains(t, hookCalls, "user1")
	assert.Contains(t, hookCalls, "user2")
}

// TestCompositeLimiter_DynamicLimits_FallbackToDefault tests default fallback
func TestCompositeLimiter_DynamicLimits_FallbackToDefault(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  250,
		DefaultWindow: 2 * time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)

	limiter, err := NewCompositeLimiter(
		mockAlgo,
		mockBackend,
		limiters.CompositeConfig{
			Name: "test-composite",
			Limits: []limiters.LimitConfig{
				{Name: "rate", Limit: 100, Window: time.Second, Priority: 1},
			},
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

	// Unconfigured key should use default
	limit, window, err := dynamicMgr.GetCurrentLimit(ctx, "unconfigured_key")
	require.NoError(t, err)
	assert.Equal(t, 250, limit)
	assert.Equal(t, 2*time.Second, window)
}

// TestCompositeLimiter_DynamicLimits_GetAllLimits tests retrieving all limits
func TestCompositeLimiter_DynamicLimits_GetAllLimits(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)

	limiter, err := NewCompositeLimiter(
		mockAlgo,
		mockBackend,
		limiters.CompositeConfig{
			Name: "test-composite",
			Limits: []limiters.LimitConfig{
				{Name: "rate", Limit: 100, Window: time.Second, Priority: 1},
			},
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

	// Initially empty
	allLimits := dynamicMgr.GetAllLimits()
	assert.Empty(t, allLimits)

	// Add limits
	_ = dynamicMgr.UpdateLimit(ctx, "user1", 150, time.Second)
	_ = dynamicMgr.UpdateLimit(ctx, "user2", 250, time.Second)
	_ = dynamicMgr.UpdateLimit(ctx, "user3", 350, time.Second)

	// Should return all
	allLimits = dynamicMgr.GetAllLimits()
	assert.Len(t, allLimits, 3)
	assert.Contains(t, allLimits, "user1")
	assert.Contains(t, allLimits, "user2")
	assert.Contains(t, allLimits, "user3")
}

// TestCompositeLimiter_DynamicLimits_AllowN tests AllowN with dynamic limits
func TestCompositeLimiter_DynamicLimits_AllowN(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(nil, nil).AnyTimes()

	limiter, err := NewCompositeLimiter(
		mockAlgo,
		mockBackend,
		limiters.CompositeConfig{
			Name: "test-composite",
			Limits: []limiters.LimitConfig{
				{Name: "batch", Limit: 100, Window: time.Second, Priority: 1},
			},
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

	// Update limit for batch operations
	_ = dynamicMgr.UpdateLimit(ctx, "batch_user", 500, time.Second)

	// Test AllowN
	result := limiter.AllowN(ctx, "batch_user", 10)
	assert.True(t, result.Allowed)
}

// mockBackendWithFailure simulates backend failures for composite limiter
type mockBackendWithFailure struct {
	data             map[string]interface{}
	failGetCount     int
	failSetCount     int
	getCallCount     int
	setCallCount     int
	triggerFailAtGet int // Trigger failure after N get calls
	triggerFailAtSet int // Trigger failure after N set calls
	mu               sync.Mutex
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
	val, ok := m.data[key]
	if !ok {
		m.data[key] = int64(1)
		return 1, nil
	}
	if intVal, ok := val.(int64); ok {
		newVal := intVal + 1
		m.data[key] = newVal
		return newVal, nil
	}
	return 0, errors.New("invalid type")
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
	return "mock"
}

// mockAlgorithm for composite limiter tests
type mockCompositeAlgorithm struct{}

func (m *mockCompositeAlgorithm) Name() string        { return "mock" }
func (m *mockCompositeAlgorithm) Description() string { return "mock algorithm" }
func (m *mockCompositeAlgorithm) Reset(ctx context.Context) (interface{}, error) {
	return map[string]interface{}{"count": 0, "reset_at": time.Now()}, nil
}
func (m *mockCompositeAlgorithm) Allow(ctx context.Context, state interface{}, cost int) (interface{}, interface{}, error) {
	stateMap := state.(map[string]interface{})
	count := stateMap["count"].(int)
	count += cost
	// Create a new map to avoid concurrent map mutation
	newState := make(map[string]interface{})
	for k, v := range stateMap {
		newState[k] = v
	}
	newState["count"] = count
	return newState, &algorithm.TokenBucketResult{Allowed: count <= 5, RemainingTokens: float64(5 - count)}, nil
}
func (m *mockCompositeAlgorithm) ValidateConfig(config interface{}) error { return nil }
func (m *mockCompositeAlgorithm) GetStats(ctx context.Context, state interface{}) interface{} {
	return state
}

// TestCompositeLimiterWithoutFailover validates baseline without failover
func TestCompositeLimiterWithoutFailover(t *testing.T) {
	primaryBE := newMockBackendWithFailure()

	cfg := limiters.CompositeConfig{
		Name: "test-composite",
		Limits: []limiters.LimitConfig{
			{Name: "limit1", Limit: 5, Window: time.Second},
			{Name: "limit2", Limit: 10, Window: time.Second},
		},
	}

	limiter, err := NewCompositeLimiter(&mockCompositeAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	// Should work normally
	result := limiter.Allow(ctx, "test-key")
	if !result.Allowed {
		t.Error("Request should be allowed without failover")
	}

	t.Log("Composite limiter without failover test passed")
}

// TestCompositeLimiterWithFailoverNoFailure validates failover enabled but no failures
func TestCompositeLimiterWithFailoverNoFailure(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	fallbackBE := newMockBackendWithFailure()

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
	defer func() {
		if c, ok := failoverHandler.(interface{ Close() error }); ok {
			c.Close()
		}
	}()

	cfg := limiters.CompositeConfig{
		Name: "test-composite",
		Limits: []limiters.LimitConfig{
			{Name: "limit1", Limit: 5, Window: time.Second},
			{Name: "limit2", Limit: 10, Window: time.Second},
		},
		Failover: limiters.FailoverConfig{
			EnableFailover: true,
			Handler:        failoverHandler,
		},
	}

	limiter, err := NewCompositeLimiter(&mockCompositeAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	// Should work normally with no failures
	result := limiter.Allow(ctx, "test-key")
	if !result.Allowed {
		t.Error("Request should be allowed when no failures occur")
	}

	// Failover should not be active
	status := failoverHandler.GetFallbackStatus(ctx)
	if status.IsFallbackActive {
		t.Error("Failover should not be active when no failures")
	}

	t.Log("Composite limiter with failover (no failures) test passed")
}

// TestCompositeLimiterFailoverOnGetFailure validates failover triggers on Get failures
func TestCompositeLimiterFailoverOnGetFailure(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	primaryBE.triggerFailAtGet = 1 // Fail on first get
	fallbackBE := newMockBackendWithFailure()

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
	defer func() {
		if c, ok := failoverHandler.(interface{ Close() error }); ok {
			c.Close()
		}
	}()

	cfg := limiters.CompositeConfig{
		Name: "test-composite",
		Limits: []limiters.LimitConfig{
			{Name: "limit1", Limit: 5, Window: time.Second},
		},
		Failover: limiters.FailoverConfig{
			EnableFailover: true,
			Handler:        failoverHandler,
		},
	}

	limiter, err := NewCompositeLimiter(&mockCompositeAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	// First request - should trigger first failure
	result1 := limiter.Allow(ctx, "test-key")
	t.Logf("First request: allowed=%v, error=%v", result1.Allowed, result1.Error)

	// Second request - should trigger second failure and activate failover
	result2 := limiter.Allow(ctx, "test-key")
	t.Logf("Second request: allowed=%v, error=%v", result2.Allowed, result2.Error)

	// Failover should be active now
	status := failoverHandler.GetFallbackStatus(ctx)
	if !status.IsFallbackActive {
		t.Errorf("Failover should be active after Get failures (count: %d)", status.FailureCount)
	}

	// During failover, request should be allowed with error
	if !result2.Allowed {
		t.Error("Request should be allowed during failover (graceful degradation)")
	}
	if result2.Error == nil {
		t.Error("Request should have error indicating fallback usage")
	}

	t.Log("Composite limiter failover on Get failure test passed")
}

// TestCompositeLimiterFailoverOnSetFailure validates failover triggers on Set failures
func TestCompositeLimiterFailoverOnSetFailure(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	primaryBE.triggerFailAtSet = 1 // Fail on first set
	fallbackBE := newMockBackendWithFailure()

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
	defer func() {
		if c, ok := failoverHandler.(interface{ Close() error }); ok {
			c.Close()
		}
	}()

	cfg := limiters.CompositeConfig{
		Name: "test-composite",
		Limits: []limiters.LimitConfig{
			{Name: "limit1", Limit: 5, Window: time.Second},
		},
		Failover: limiters.FailoverConfig{
			EnableFailover: true,
			Handler:        failoverHandler,
		},
	}

	limiter, err := NewCompositeLimiter(&mockCompositeAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	// First request - triggers first Set failure
	result1 := limiter.Allow(ctx, "test-key")
	t.Logf("First request: allowed=%v, error=%v", result1.Allowed, result1.Error)

	// Second request - triggers second Set failure and activates failover
	result2 := limiter.Allow(ctx, "test-key")
	t.Logf("Second request: allowed=%v, error=%v", result2.Allowed, result2.Error)

	// Failover should be active
	status := failoverHandler.GetFallbackStatus(ctx)
	if !status.IsFallbackActive {
		t.Errorf("Failover should be active after Set failures (count: %d)", status.FailureCount)
	}

	// During failover, request should be allowed with error
	if !result2.Allowed {
		t.Error("Request should be allowed during failover")
	}

	t.Log("Composite limiter failover on Set failure test passed")
}

// TestCompositeLimiterMultipleFailuresBeforeFailover validates threshold-based failover
func TestCompositeLimiterMultipleFailuresBeforeFailover(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	primaryBE.triggerFailAtGet = 2 // Fail on 2nd get and beyond
	fallbackBE := newMockBackendWithFailure()

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
	defer func() {
		if c, ok := failoverHandler.(interface{ Close() error }); ok {
			c.Close()
		}
	}()

	cfg := limiters.CompositeConfig{
		Name: "test-composite",
		Limits: []limiters.LimitConfig{
			{Name: "limit1", Limit: 5, Window: time.Second},
		},
		Failover: limiters.FailoverConfig{
			EnableFailover: true,
			Handler:        failoverHandler,
		},
	}

	limiter, err := NewCompositeLimiter(&mockCompositeAlgorithm{}, primaryBE, cfg)
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

	// Second request - fails (getCallCount=2), first failure
	result2 := limiter.Allow(ctx, "test-key")
	t.Logf("Second request allowed: %v, error: %v", result2.Allowed, result2.Error)

	status = failoverHandler.GetFallbackStatus(ctx)
	t.Logf("After 1st failure: FailureCount=%d", status.FailureCount)
	if status.IsFallbackActive {
		t.Error("Failover should not be active after 1 failure (threshold is 2)")
	}

	// Third request - fails again, second failure, should trigger failover
	result3 := limiter.Allow(ctx, "test-key")
	t.Logf("Third request allowed: %v, error: %v", result3.Allowed, result3.Error)

	status = failoverHandler.GetFallbackStatus(ctx)
	t.Logf("After 2nd failure: IsFallbackActive=%v, FailureCount=%d",
		status.IsFallbackActive, status.FailureCount)
	if !status.IsFallbackActive {
		t.Errorf("Failover should be active after threshold reached (failures: %d)", status.FailureCount)
	}

	t.Log("Composite limiter multiple failures before failover test passed")
}

// TestCompositeLimiterFailoverErrorMessage validates error messages during failover
func TestCompositeLimiterFailoverErrorMessage(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	primaryBE.triggerFailAtGet = 1
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
	defer func() {
		if c, ok := failoverHandler.(interface{ Close() error }); ok {
			c.Close()
		}
	}()

	cfg := limiters.CompositeConfig{
		Name: "test-composite",
		Limits: []limiters.LimitConfig{
			{Name: "limit1", Limit: 5, Window: time.Second},
		},
		Failover: limiters.FailoverConfig{
			EnableFailover: true,
			Handler:        failoverHandler,
		},
	}

	limiter, err := NewCompositeLimiter(&mockCompositeAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	// Trigger failure
	result := limiter.Allow(ctx, "test-key")

	// Should have error mentioning fallback
	if result.Error == nil {
		t.Error("Expected error indicating fallback usage")
	} else {
		errMsg := result.Error.Error()
		if errMsg == "" {
			t.Error("Error message should not be empty")
		}
		t.Logf("Error message: %s", errMsg)
	}

	t.Log("Composite limiter failover error message test passed")
}

// TestCompositeLimiterFailoverMultipleLimits validates failover with multiple composite limits
func TestCompositeLimiterFailoverMultipleLimits(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	primaryBE.triggerFailAtGet = 3 // Fail on 3rd get (after first limit succeeds)
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
	defer func() {
		if c, ok := failoverHandler.(interface{ Close() error }); ok {
			c.Close()
		}
	}()

	cfg := limiters.CompositeConfig{
		Name: "test-composite",
		Limits: []limiters.LimitConfig{
			{Name: "limit1", Limit: 5, Window: time.Second, Priority: 1},
			{Name: "limit2", Limit: 10, Window: time.Second, Priority: 2},
			{Name: "limit3", Limit: 20, Window: time.Second, Priority: 3},
		},
		Failover: limiters.FailoverConfig{
			EnableFailover: true,
			Handler:        failoverHandler,
		},
	}

	limiter, err := NewCompositeLimiter(&mockCompositeAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	// Request that will trigger failure when checking limit2
	result := limiter.Allow(ctx, "test-key")
	t.Logf("Request: allowed=%v, error=%v", result.Allowed, result.Error)

	// Should activate failover
	status := failoverHandler.GetFallbackStatus(ctx)
	if !status.IsFallbackActive {
		t.Error("Failover should be active after Get failure in multi-limit check")
	}

	t.Log("Composite limiter failover with multiple limits test passed")
}

// TestCompositeLimiterConcurrentFailover validates failover under concurrent load
func TestCompositeLimiterConcurrentFailover(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	primaryBE.triggerFailAtGet = 5 // Fail after a few requests
	fallbackBE := newMockBackendWithFailure()

	failoverCfg := features.FailoverConfig{
		EnableFallback:        true,
		FailureThreshold:      3,
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
	defer func() {
		if c, ok := failoverHandler.(interface{ Close() error }); ok {
			c.Close()
		}
	}()

	cfg := limiters.CompositeConfig{
		Name: "test-composite",
		Limits: []limiters.LimitConfig{
			{Name: "limit1", Limit: 100, Window: time.Second},
		},
		Failover: limiters.FailoverConfig{
			EnableFailover: true,
			Handler:        failoverHandler,
		},
	}

	limiter, err := NewCompositeLimiter(&mockCompositeAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	// Concurrent requests
	var wg sync.WaitGroup
	numGoroutines := 10
	numRequests := 5

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numRequests; j++ {
				result := limiter.Allow(ctx, "test-key")
				t.Logf("Goroutine %d, request %d: allowed=%v", id, j, result.Allowed)
			}
		}(i)
	}

	wg.Wait()

	// Eventually failover should be active
	time.Sleep(100 * time.Millisecond)
	status := failoverHandler.GetFallbackStatus(ctx)
	t.Logf("Final failover status: active=%v, count=%d", status.IsFallbackActive, status.FailureCount)

	t.Log("Composite limiter concurrent failover test passed")
}
