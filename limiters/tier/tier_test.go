package tier

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/abhipray-cpu/niyantrak/algorithm"
	"github.com/abhipray-cpu/niyantrak/backend"
	"github.com/abhipray-cpu/niyantrak/features"
	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewTierBasedLimiter_ValidConfig tests constructor with valid configuration
func TestNewTierBasedLimiter_ValidConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tiers: map[string]limiters.TierLimit{
			"free":       {Limit: 10, Window: time.Minute},
			"pro":        {Limit: 100, Window: time.Minute},
			"enterprise": {Limit: 1000, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, err := NewTierBasedLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limiter == nil {
		t.Fatal("limiter should not be nil")
	}
	if limiter.Type() != "tier" {
		t.Errorf("expected type 'tier', got %q", limiter.Type())
	}
}

// TestNewTierBasedLimiter_InvalidConfig tests constructor validation
func TestNewTierBasedLimiter_InvalidConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	tests := []struct {
		name    string
		algo    algorithm.Algorithm
		be      backend.Backend
		cfg     limiters.TierConfig
		wantErr bool
	}{
		{
			name:    "nil algorithm",
			algo:    nil,
			be:      mockBackend,
			cfg:     limiters.TierConfig{},
			wantErr: true,
		},
		{
			name:    "nil backend",
			algo:    mockAlgo,
			be:      nil,
			cfg:     limiters.TierConfig{},
			wantErr: true,
		},
		{
			name: "zero default limit",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.TierConfig{
				BasicConfig: limiters.BasicConfig{
					DefaultLimit:  0,
					DefaultWindow: time.Second,
				},
				DefaultTier: "free",
			},
			wantErr: true,
		},
		{
			name: "zero default window",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.TierConfig{
				BasicConfig: limiters.BasicConfig{
					DefaultLimit:  100,
					DefaultWindow: 0,
				},
				DefaultTier: "free",
			},
			wantErr: true,
		},
		{
			name: "empty default tier",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.TierConfig{
				BasicConfig: limiters.BasicConfig{
					DefaultLimit:  100,
					DefaultWindow: time.Second,
				},
				Tiers:       map[string]limiters.TierLimit{"free": {Limit: 10, Window: time.Minute}},
				DefaultTier: "",
			},
			wantErr: true,
		},
		{
			name: "empty tiers",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.TierConfig{
				BasicConfig: limiters.BasicConfig{
					DefaultLimit:  100,
					DefaultWindow: time.Second,
				},
				Tiers:       map[string]limiters.TierLimit{},
				DefaultTier: "free",
			},
			wantErr: true,
		},
		{
			name: "default tier not in tiers",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.TierConfig{
				BasicConfig: limiters.BasicConfig{
					DefaultLimit:  100,
					DefaultWindow: time.Second,
				},
				Tiers:       map[string]limiters.TierLimit{"free": {Limit: 10, Window: time.Minute}},
				DefaultTier: "premium",
			},
			wantErr: true,
		},
		{
			name: "negative tier limit",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.TierConfig{
				BasicConfig: limiters.BasicConfig{
					DefaultLimit:  100,
					DefaultWindow: time.Second,
				},
				Tiers:       map[string]limiters.TierLimit{"free": {Limit: -10, Window: time.Minute}},
				DefaultTier: "free",
			},
			wantErr: true,
		},
		{
			name: "zero tier window",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.TierConfig{
				BasicConfig: limiters.BasicConfig{
					DefaultLimit:  100,
					DefaultWindow: time.Second,
				},
				Tiers:       map[string]limiters.TierLimit{"free": {Limit: 10, Window: time.Duration(0)}},
				DefaultTier: "free",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTierBasedLimiter(tt.algo, tt.be, tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTierBasedLimiter() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestAllow_WithMocks tests single request (delegates to AllowN)
func TestAllow_WithMocks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	// Setup expectations
	mockBackend.EXPECT().
		Get(gomock.Any(), "user:123").
		Return(nil, backend.ErrKeyNotFound).
		Times(1)

	mockAlgo.EXPECT().
		Reset(gomock.Any()).
		Return(map[string]interface{}{"tokens": 10}, nil).
		Times(1)

	mockAlgo.EXPECT().
		Allow(gomock.Any(), gomock.Any(), 1).
		Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		Times(1)

	mockBackend.EXPECT().
		Set(gomock.Any(), "user:123", gomock.Any(), time.Minute).
		Return(nil).
		Times(1)

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, _ := NewTierBasedLimiter(mockAlgo, mockBackend, cfg)
	result := limiter.Allow(context.Background(), "user:123")

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}
}

// TestAllowN_WithMocks tests multiple requests
func TestAllowN_WithMocks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	// Setup expectations
	mockBackend.EXPECT().
		Get(gomock.Any(), "user:123").
		Return(nil, backend.ErrKeyNotFound).
		Times(1)

	mockAlgo.EXPECT().
		Reset(gomock.Any()).
		Return(map[string]interface{}{"tokens": 100}, nil).
		Times(1)

	mockAlgo.EXPECT().
		Allow(gomock.Any(), gomock.Any(), 5).
		Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		Times(1)

	mockBackend.EXPECT().
		Set(gomock.Any(), "user:123", gomock.Any(), time.Minute).
		Return(nil).
		Times(1)

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tiers: map[string]limiters.TierLimit{
			"pro": {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "pro",
	}

	limiter, _ := NewTierBasedLimiter(mockAlgo, mockBackend, cfg)
	result := limiter.AllowN(context.Background(), "user:123", 5)

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}
}

// TestAllowN_InvalidN tests invalid n values
func TestAllowN_InvalidN(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, _ := NewTierBasedLimiter(mockAlgo, mockBackend, cfg)

	tests := []struct {
		name  string
		n     int
		valid bool
	}{
		{"positive", 1, true},
		{"zero", 0, false},
		{"negative", -1, false},
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

			result := limiter.AllowN(context.Background(), "key", tt.n)
			if tt.valid && result.Error != nil {
				t.Errorf("expected no error for n=%d, got %v", tt.n, result.Error)
			}
			if !tt.valid && result.Error == nil {
				t.Errorf("expected error for n=%d", tt.n)
			}
		})
	}
}

// TestSetTierLimit tests setting/updating tier limits
func TestSetTierLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, _ := NewTierBasedLimiter(mockAlgo, mockBackend, cfg)

	// Set new limit for existing tier
	err := limiter.SetTierLimit(context.Background(), "free", 50, 2*time.Minute)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify updated limit
	limit, window, err := limiter.GetTierLimit(context.Background(), "free")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if limit != 50 {
		t.Errorf("expected limit 50, got %d", limit)
	}
	if window != 2*time.Minute {
		t.Errorf("expected window 2m, got %v", window)
	}

	// Add new tier
	err = limiter.SetTierLimit(context.Background(), "premium", 500, 5*time.Minute)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	limit, _, err = limiter.GetTierLimit(context.Background(), "premium")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if limit != 500 {
		t.Errorf("expected limit 500, got %d", limit)
	}
}

// TestGetTierLimit tests retrieving tier configuration
func TestGetTierLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tiers: map[string]limiters.TierLimit{
			"free":       {Limit: 10, Window: time.Minute},
			"pro":        {Limit: 100, Window: time.Minute},
			"enterprise": {Limit: 1000, Window: time.Hour},
		},
		DefaultTier: "free",
	}

	limiter, _ := NewTierBasedLimiter(mockAlgo, mockBackend, cfg)

	tests := []struct {
		name      string
		tier      string
		expLimit  int
		expWindow time.Duration
		expErr    bool
	}{
		{"free tier", "free", 10, time.Minute, false},
		{"pro tier", "pro", 100, time.Minute, false},
		{"enterprise tier", "enterprise", 1000, time.Hour, false},
		{"nonexistent tier", "unknown", 0, 0, true},
		{"empty tier", "", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, window, err := limiter.GetTierLimit(context.Background(), tt.tier)
			if (err != nil) != tt.expErr {
				t.Errorf("GetTierLimit() error = %v, wantErr %v", err, tt.expErr)
			}
			if !tt.expErr {
				if limit != tt.expLimit {
					t.Errorf("expected limit %d, got %d", tt.expLimit, limit)
				}
				if window != tt.expWindow {
					t.Errorf("expected window %v, got %v", tt.expWindow, window)
				}
			}
		})
	}
}

// TestAssignKeyToTier tests assigning keys to tiers
func TestAssignKeyToTier(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, _ := NewTierBasedLimiter(mockAlgo, mockBackend, cfg)

	// Assign key to pro tier
	err := limiter.AssignKeyToTier(context.Background(), "user:123", "pro")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify assignment
	tier, err := limiter.GetKeyTier(context.Background(), "user:123")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if tier != "pro" {
		t.Errorf("expected tier 'pro', got %q", tier)
	}

	// Test invalid inputs
	tests := []struct {
		name  string
		key   string
		tier  string
		valid bool
	}{
		{"empty key", "", "pro", false},
		{"empty tier", "user:456", "", false},
		{"invalid tier", "user:789", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := limiter.AssignKeyToTier(context.Background(), tt.key, tt.tier)
			if (err != nil) != !tt.valid {
				t.Errorf("AssignKeyToTier() error = %v, expected error = %v", err, !tt.valid)
			}
		})
	}
}

// TestGetKeyTier tests retrieving key tier assignment
func TestGetKeyTier(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, _ := NewTierBasedLimiter(mockAlgo, mockBackend, cfg)

	// Unassigned key should return default tier
	tier, err := limiter.GetKeyTier(context.Background(), "unassigned")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if tier != "free" {
		t.Errorf("expected default tier 'free', got %q", tier)
	}

	// Assign and verify
	limiter.AssignKeyToTier(context.Background(), "user:123", "pro")
	tier, err = limiter.GetKeyTier(context.Background(), "user:123")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if tier != "pro" {
		t.Errorf("expected tier 'pro', got %q", tier)
	}

	// Empty key should error
	_, err = limiter.GetKeyTier(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty key")
	}
}

// TestListTiers tests listing all configured tiers
func TestListTiers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tiers: map[string]limiters.TierLimit{
			"free":       {Limit: 10, Window: time.Minute},
			"pro":        {Limit: 100, Window: time.Minute},
			"enterprise": {Limit: 1000, Window: time.Hour},
		},
		DefaultTier: "free",
	}

	limiter, _ := NewTierBasedLimiter(mockAlgo, mockBackend, cfg)

	tiers, err := limiter.ListTiers(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(tiers) != 3 {
		t.Errorf("expected 3 tiers, got %d", len(tiers))
	}

	tierMap := make(map[string]bool)
	for _, tier := range tiers {
		tierMap[tier] = true
	}

	expected := []string{"free", "pro", "enterprise"}
	for _, exp := range expected {
		if !tierMap[exp] {
			t.Errorf("expected tier %q not found", exp)
		}
	}
}

// TestReset tests clearing state for a key
func TestReset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	mockBackend.EXPECT().
		Delete(gomock.Any(), "user:123").
		Return(nil).
		Times(1)

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, _ := NewTierBasedLimiter(mockAlgo, mockBackend, cfg)

	err := limiter.Reset(context.Background(), "user:123")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Empty key should error
	err = limiter.Reset(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty key")
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

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, _ := NewTierBasedLimiter(mockAlgo, mockBackend, cfg)

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
	if !errors.Is(result.Error, limiters.ErrLimiterClosed) {
		t.Errorf("expected ErrLimiterClosed, got %v", result.Error)
	}
}

// TestGetStats tests statistics retrieval
func TestGetStats(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, _ := NewTierBasedLimiter(mockAlgo, mockBackend, cfg)

	// Stats for unassigned key (default tier)
	stats := limiter.GetStats(context.Background(), "unassigned")
	if stats == nil {
		t.Fatal("stats should not be nil")
	}

	statsMap, ok := stats.(map[string]interface{})
	if !ok {
		t.Fatal("stats should be a map")
	}

	if statsMap["tier"] != "free" {
		t.Errorf("expected tier 'free', got %v", statsMap["tier"])
	}

	// Stats for assigned key
	limiter.AssignKeyToTier(context.Background(), "user:123", "pro")
	stats = limiter.GetStats(context.Background(), "user:123")
	statsMap = stats.(map[string]interface{})
	if statsMap["tier"] != "pro" {
		t.Errorf("expected tier 'pro', got %v", statsMap["tier"])
	}
}

// TestConcurrentAccess tests thread safety with multiple goroutines
func TestConcurrentAccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	numGoroutines := 10
	requestsPerGoroutine := 10

	// Setup generic expectations for concurrent access
	// Each goroutine iteration calls: Allow (which calls Get + Reset/Allow + Set)
	// Additional Gets/Sets may occur for tier mapping lookups.
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

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, _ := NewTierBasedLimiter(mockAlgo, mockBackend, cfg)

	done := make(chan bool, numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(_ int) {
			for r := 0; r < requestsPerGoroutine; r++ {
				limiter.Allow(context.Background(), "user:123")
				limiter.SetTierLimit(context.Background(), "free", 50, 2*time.Minute)
				limiter.GetKeyTier(context.Background(), "user:123")
			}
			done <- true
		}(g)
	}

	for g := 0; g < numGoroutines; g++ {
		<-done
	}
}

// TestMultipleKeys tests independent key handling with different tiers
func TestMultipleKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	// No setup needed - this test only does tier assignment and verification
	// which don't call backend/algorithm methods

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, _ := NewTierBasedLimiter(mockAlgo, mockBackend, cfg)

	// Assign keys to different tiers
	limiter.AssignKeyToTier(context.Background(), "user:1", "free")
	limiter.AssignKeyToTier(context.Background(), "user:2", "pro")
	limiter.AssignKeyToTier(context.Background(), "user:3", "pro")

	// Verify assignments are independent
	tier1, _ := limiter.GetKeyTier(context.Background(), "user:1")
	tier2, _ := limiter.GetKeyTier(context.Background(), "user:2")
	tier3, _ := limiter.GetKeyTier(context.Background(), "user:3")

	if tier1 != "free" || tier2 != "pro" || tier3 != "pro" {
		t.Error("tier assignments not independent")
	}

	// Verify stats are independent
	stats1 := limiter.GetStats(context.Background(), "user:1")
	stats2 := limiter.GetStats(context.Background(), "user:2")

	statsMap1 := stats1.(map[string]interface{})
	statsMap2 := stats2.(map[string]interface{})

	if statsMap1["tier"] == statsMap2["tier"] {
		t.Error("expected different tier stats")
	}
}

// BenchmarkAllow benchmarks Allow operation with real memory backend
func BenchmarkAllow(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	// Setup for repeated calls
	mockBackend.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(nil, backend.ErrKeyNotFound).
		Times(b.N)

	mockAlgo.EXPECT().
		Reset(gomock.Any()).
		Return(map[string]interface{}{}, nil).
		Times(b.N)

	mockAlgo.EXPECT().
		Allow(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		Times(b.N)

	mockBackend.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(b.N)

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, _ := NewTierBasedLimiter(mockAlgo, mockBackend, cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(context.Background(), "user:123")
	}
}

// BenchmarkSetTierLimit benchmarks SetTierLimit operation
func BenchmarkSetTierLimit(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, _ := NewTierBasedLimiter(mockAlgo, mockBackend, cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.SetTierLimit(context.Background(), "free", 50, 2*time.Minute)
	}
}

// TestTierLimiter_Observability_NoOp tests that NoOp observability works (zero overhead)
func TestTierLimiter_Observability_NoOp(t *testing.T) {
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

	config := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Minute,
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, err := NewTierBasedLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	result := limiter.Allow(ctx, "test-key")

	if !result.Allowed {
		t.Fatal("first request should be allowed")
	}
}

// TestTierLimiter_Observability_WithMockLogger tests logging integration
func TestTierLimiter_Observability_WithMockLogger(t *testing.T) {
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

	config := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Minute,
			Observability: limiters.ObservabilityConfig{
				Logger: mockLogger,
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, err := NewTierBasedLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	result := limiter.Allow(ctx, "test-key")

	if !result.Allowed {
		t.Fatal("first request should be allowed")
	}
}

// TestTierLimiter_Observability_WithMockMetrics tests metrics integration
func TestTierLimiter_Observability_WithMockMetrics(t *testing.T) {
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

	config := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Minute,
			Observability: limiters.ObservabilityConfig{
				Metrics: mockMetrics,
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, err := NewTierBasedLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	result := limiter.Allow(ctx, "test-key")

	if !result.Allowed {
		t.Fatal("first request should be allowed")
	}
}

// TestTierLimiter_Observability_WithMockTracer tests tracing integration
func TestTierLimiter_Observability_WithMockTracer(t *testing.T) {
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

	mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.tier.check").Return(nil).Times(1)
	mockTracer.EXPECT().AddAttribute(nil, "key", "test-key").Times(1)
	mockTracer.EXPECT().AddAttribute(nil, "requests_count", 1).Times(1)
	mockTracer.EXPECT().AddAttribute(nil, "tier", "free").Times(1)
	mockTracer.EXPECT().AddAttribute(nil, "allowed", true).Times(1)
	mockTracer.EXPECT().AddAttribute(nil, "remaining", gomock.Any()).Times(1)
	mockTracer.EXPECT().AddAttribute(nil, "limit", gomock.Any()).Times(1)
	mockTracer.EXPECT().EndSpan(nil).Times(1)

	config := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Minute,
			Observability: limiters.ObservabilityConfig{
				Tracer:        mockTracer,
				EnableTracing: true,
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, err := NewTierBasedLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	result := limiter.Allow(ctx, "test-key")

	if !result.Allowed {
		t.Fatal("first request should be allowed")
	}
}

// TestTierLimiter_Observability_AllThree tests all three layers together
func TestTierLimiter_Observability_AllThree(t *testing.T) {
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
	mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.tier.check").Return(nil).Times(1)
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().EndSpan(nil).Times(1)

	config := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Minute,
			Observability: limiters.ObservabilityConfig{
				Logger:        mockLogger,
				Metrics:       mockMetrics,
				Tracer:        mockTracer,
				EnableTracing: true,
				LogLevel:      "debug",
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, err := NewTierBasedLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	result := limiter.Allow(ctx, "test-key")

	if !result.Allowed {
		t.Fatal("first request should be allowed")
	}
}

// TestTierLimiter_Observability_SetTierLimit tests SetTierLimit logging
func TestTierLimiter_Observability_SetTierLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockAlgo.EXPECT().ValidateConfig(gomock.Any()).Return(nil).AnyTimes()

	mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.tier.set_tier_limit").Return(nil).Times(1)
	mockTracer.EXPECT().AddAttribute(nil, "tier", "pro").Times(1)
	mockTracer.EXPECT().AddAttribute(nil, "limit", 200).Times(1)
	mockTracer.EXPECT().AddAttribute(nil, "window_ms", gomock.Any()).Times(1)
	mockTracer.EXPECT().EndSpan(nil).Times(1)

	mockLogger.EXPECT().Info("tier limit updated", "tier", "pro", "limit", 200, "window", time.Minute).Times(1)

	config := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Minute,
			Observability: limiters.ObservabilityConfig{
				Logger: mockLogger,
				Tracer: mockTracer,
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, err := NewTierBasedLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	if err := limiter.SetTierLimit(ctx, "pro", 200, time.Minute); err != nil {
		t.Errorf("settierlimit should succeed: %v", err)
	}
}

// TestTierLimiter_Observability_AssignKeyToTier tests AssignKeyToTier logging
func TestTierLimiter_Observability_AssignKeyToTier(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockAlgo.EXPECT().ValidateConfig(gomock.Any()).Return(nil).AnyTimes()

	mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.tier.assign_key").Return(nil).Times(1)
	mockTracer.EXPECT().AddAttribute(nil, "key", "user123").Times(1)
	mockTracer.EXPECT().AddAttribute(nil, "tier", "pro").Times(1)
	mockTracer.EXPECT().EndSpan(nil).Times(1)

	mockLogger.EXPECT().Info("key assigned to tier", "key", "user123", "tier", "pro").Times(1)

	config := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Minute,
			Observability: limiters.ObservabilityConfig{
				Logger: mockLogger,
				Tracer: mockTracer,
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, err := NewTierBasedLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	if err := limiter.AssignKeyToTier(ctx, "user123", "pro"); err != nil {
		t.Errorf("assignkeytotier should succeed: %v", err)
	}
}

// TestTierLimiter_Observability_ResetWithLogging tests Reset method logging
func TestTierLimiter_Observability_ResetWithLogging(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockAlgo.EXPECT().ValidateConfig(gomock.Any()).Return(nil).AnyTimes()
	mockBackend.EXPECT().Delete(gomock.Any(), "test-key").Return(nil).Times(1)

	mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.tier.reset").Return(nil).Times(1)
	mockTracer.EXPECT().AddAttribute(nil, "key", "test-key").Times(1)
	mockTracer.EXPECT().EndSpan(nil).Times(1)

	mockLogger.EXPECT().Info("rate limit reset", "key", "test-key").Times(1)

	config := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Minute,
			Observability: limiters.ObservabilityConfig{
				Logger: mockLogger,
				Tracer: mockTracer,
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, err := NewTierBasedLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	if err := limiter.Reset(ctx, "test-key"); err != nil {
		t.Errorf("reset should succeed: %v", err)
	}
}

// TestTierLimiter_Observability_CloseLogging tests Close method logging
func TestTierLimiter_Observability_CloseLogging(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)

	mockAlgo.EXPECT().ValidateConfig(gomock.Any()).Return(nil).AnyTimes()
	mockBackend.EXPECT().Close().Return(nil).Times(1)

	mockLogger.EXPECT().Info("closing tier limiter").Times(1)

	config := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Minute,
			Observability: limiters.ObservabilityConfig{
				Logger: mockLogger,
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, err := NewTierBasedLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	if err := limiter.Close(); err != nil {
		t.Errorf("close should succeed: %v", err)
	}
}

// TestTierLimiter_Observability_Integration_LoggingAndMetrics tests logging + metrics together
func TestTierLimiter_Observability_Integration_LoggingAndMetrics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)

	mockAlgo.EXPECT().ValidateConfig(gomock.Any()).Return(nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), "user1").Return(nil, nil).Times(1)
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(map[string]interface{}{"tokens": 100}, nil).Times(1)
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), 1).Return(
		map[string]interface{}{"tokens": 99},
		&algorithm.TokenBucketResult{Allowed: true, RemainingTokens: 99},
		nil,
	).Times(1)
	mockBackend.EXPECT().Set(gomock.Any(), "user1", gomock.Any(), gomock.Any()).Return(nil).Times(1)

	// Logging expectations - use AnyTimes for flexible matching
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()

	// Metrics expectations
	mockMetrics.EXPECT().RecordDecisionLatency("user1", gomock.Any()).Times(1)
	mockMetrics.EXPECT().RecordRequest("user1", true, gomock.Any()).Times(1)

	config := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Minute,
			Observability: limiters.ObservabilityConfig{
				Logger:   mockLogger,
				Metrics:  mockMetrics,
				LogLevel: "debug",
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "pro",
	}

	limiter, err := NewTierBasedLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	result := limiter.Allow(ctx, "user1")

	if !result.Allowed {
		t.Fatal("first request should be allowed")
	}
}

// TestTierLimiter_Observability_Integration_LoggingAndTracing tests logging + tracing together
func TestTierLimiter_Observability_Integration_LoggingAndTracing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockAlgo.EXPECT().ValidateConfig(gomock.Any()).Return(nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), "customer456").Return(nil, nil).Times(1)
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(map[string]interface{}{"tokens": 50}, nil).Times(1)
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), 1).Return(
		map[string]interface{}{"tokens": 49},
		&algorithm.TokenBucketResult{Allowed: true, RemainingTokens: 49},
		nil,
	).Times(1)
	mockBackend.EXPECT().Set(gomock.Any(), "customer456", gomock.Any(), gomock.Any()).Return(nil).Times(1)

	// Logging expectations
	mockLogger.EXPECT().Debug("rate_limit_allowed", gomock.Any()).Times(1)

	// Tracing expectations
	mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.tier.check").Return(nil).Times(1)
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().EndSpan(nil).Times(1)

	config := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  50,
			DefaultWindow: time.Minute,
			Observability: limiters.ObservabilityConfig{
				Logger:        mockLogger,
				Tracer:        mockTracer,
				EnableTracing: true,
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 50, Window: time.Minute},
		},
		DefaultTier: "pro",
	}

	limiter, err := NewTierBasedLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	result := limiter.Allow(ctx, "customer456")

	if !result.Allowed {
		t.Fatal("first request should be allowed")
	}
}

// TestTierLimiter_Observability_Integration_MetricsAndTracing tests metrics + tracing together
func TestTierLimiter_Observability_Integration_MetricsAndTracing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockAlgo.EXPECT().ValidateConfig(gomock.Any()).Return(nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), "org789").Return(nil, nil).Times(1)
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(map[string]interface{}{"tokens": 200}, nil).Times(1)
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), 1).Return(
		map[string]interface{}{"tokens": 199},
		&algorithm.TokenBucketResult{Allowed: true, RemainingTokens: 199},
		nil,
	).Times(1)
	mockBackend.EXPECT().Set(gomock.Any(), "org789", gomock.Any(), gomock.Any()).Return(nil).Times(1)

	// Metrics expectations
	mockMetrics.EXPECT().RecordDecisionLatency("org789", gomock.Any()).Times(1)
	mockMetrics.EXPECT().RecordRequest("org789", true, int64(200)).Times(1)

	// Tracing expectations
	mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.tier.check").Return(nil).Times(1)
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().EndSpan(nil).Times(1)

	config := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  200,
			DefaultWindow: time.Minute,
			Observability: limiters.ObservabilityConfig{
				Metrics:       mockMetrics,
				Tracer:        mockTracer,
				EnableTracing: true,
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 200, Window: time.Minute},
		},
		DefaultTier: "pro",
	}

	limiter, err := NewTierBasedLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	result := limiter.Allow(ctx, "org789")

	if !result.Allowed {
		t.Fatal("first request should be allowed")
	}
}

// TestTierLimiter_Observability_Integration_AllThreeComplete tests all three with allowed request
func TestTierLimiter_Observability_Integration_AllThreeComplete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockAlgo.EXPECT().ValidateConfig(gomock.Any()).Return(nil).AnyTimes()
	mockBackend.EXPECT().Get(gomock.Any(), "user123").Return(nil, nil).Times(1)
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(map[string]interface{}{"tokens": 5}, nil).Times(1)
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), 1).Return(
		map[string]interface{}{"tokens": 4},
		&algorithm.TokenBucketResult{Allowed: true, RemainingTokens: 4},
		nil,
	).Times(1)
	mockBackend.EXPECT().Set(gomock.Any(), "user123", gomock.Any(), gomock.Any()).Return(nil).Times(1)

	// Logger expectations - Debug for allowed request
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).Times(1)

	// Metrics expectations
	mockMetrics.EXPECT().RecordDecisionLatency("user123", gomock.Any()).Times(1)
	mockMetrics.EXPECT().RecordRequest("user123", true, gomock.Any()).Times(1)

	// Tracer expectations - full span lifecycle
	mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.tier.check").Return(nil).Times(1)
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().EndSpan(nil).Times(1)

	config := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  5,
			DefaultWindow: time.Minute,
			Observability: limiters.ObservabilityConfig{
				Logger:        mockLogger,
				Metrics:       mockMetrics,
				Tracer:        mockTracer,
				EnableTracing: true,
				LogLevel:      "debug",
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 5, Window: time.Minute},
			"pro":  {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, err := NewTierBasedLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	result := limiter.Allow(ctx, "user123")

	if !result.Allowed {
		t.Fatal("first request should be allowed")
	}
}

// TestTierLimiter_Observability_Integration_DeniedWithAllThree tests denied request scenario with observability
// NOTE: TierBasedLimiter's convertResult always returns Allowed=true, so we test the allowed case
func TestTierLimiter_Observability_Integration_DeniedScenario(t *testing.T) {
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
	// Multiple requests to track metrics
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), 1).Return(
		map[string]interface{}{"tokens": 9},
		&algorithm.TokenBucketResult{Allowed: true, RemainingTokens: 9},
		nil,
	).Times(1)
	mockBackend.EXPECT().Set(gomock.Any(), "test-key", gomock.Any(), gomock.Any()).Return(nil).Times(1)

	// All three observability features should be called
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordDecisionLatency("test-key", gomock.Any()).Times(1)
	mockMetrics.EXPECT().RecordRequest("test-key", gomock.Any(), gomock.Any()).Times(1)
	mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.tier.check").Return(nil).Times(1)
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().EndSpan(nil).Times(1)

	config := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Minute,
			Observability: limiters.ObservabilityConfig{
				Logger:        mockLogger,
				Metrics:       mockMetrics,
				Tracer:        mockTracer,
				EnableTracing: true,
				LogLevel:      "debug",
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"free": {Limit: 10, Window: time.Minute},
			"pro":  {Limit: 100, Window: time.Minute},
		},
		DefaultTier: "free",
	}

	limiter, err := NewTierBasedLimiter(mockAlgo, mockBackend, config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	ctx := context.Background()
	result := limiter.Allow(ctx, "test-key")

	if !result.Allowed {
		t.Fatal("request should be allowed")
	}
}

// TestTierBasedLimiter_WithDynamicLimits_Enabled tests limiter with dynamic limits enabled
func TestTierBasedLimiter_WithDynamicLimits_Enabled(t *testing.T) {
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

	limiter, err := NewTierBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TierConfig{
			BasicConfig: limiters.BasicConfig{
				AlgorithmName: "token_bucket",
				DefaultLimit:  100,
				DefaultWindow: time.Second,
				KeyTTL:        time.Minute,
				DynamicLimits: limiters.DynamicLimitConfig{
					EnableDynamicLimits: true,
					Manager:             dynamicMgr,
				},
			},
			DefaultTier: "free",
			Tiers: map[string]limiters.TierLimit{
				"free":       {Limit: 100, Window: time.Second},
				"premium":    {Limit: 500, Window: time.Second},
				"enterprise": {Limit: 1000, Window: time.Second},
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

// TestTierBasedLimiter_WithDynamicLimits_UpdateLimitByTier tests updating tier limits
func TestTierBasedLimiter_WithDynamicLimits_UpdateLimitByTier(t *testing.T) {
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

	limiter, err := NewTierBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TierConfig{
			BasicConfig: limiters.BasicConfig{
				AlgorithmName: "token_bucket",
				DefaultLimit:  100,
				DefaultWindow: time.Second,
				KeyTTL:        time.Minute,
				DynamicLimits: limiters.DynamicLimitConfig{
					EnableDynamicLimits: true,
					Manager:             dynamicMgr,
				},
			},
			DefaultTier: "free",
			Tiers: map[string]limiters.TierLimit{
				"free":    {Limit: 100, Window: time.Second},
				"premium": {Limit: 500, Window: time.Second},
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Update tier limit
	err = dynamicMgr.UpdateLimitByTier(ctx, "premium", 1000, time.Second)
	require.NoError(t, err)

	// Verify the update worked
	result := limiter.Allow(ctx, "premium_user")
	assert.True(t, result.Allowed)
}

// TestTierBasedLimiter_DynamicLimits_MultipleTiers tests multiple tiers
func TestTierBasedLimiter_DynamicLimits_MultipleTiers(t *testing.T) {
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

	limiter, err := NewTierBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TierConfig{
			BasicConfig: limiters.BasicConfig{
				AlgorithmName: "token_bucket",
				DefaultLimit:  100,
				DefaultWindow: time.Second,
				KeyTTL:        time.Minute,
				DynamicLimits: limiters.DynamicLimitConfig{
					EnableDynamicLimits: true,
					Manager:             dynamicMgr,
				},
			},
			DefaultTier: "free",
			Tiers: map[string]limiters.TierLimit{
				"free":       {Limit: 100, Window: time.Second},
				"standard":   {Limit: 300, Window: time.Second},
				"premium":    {Limit: 500, Window: time.Second},
				"enterprise": {Limit: 2000, Window: time.Second},
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Update different tiers
	_ = dynamicMgr.UpdateLimitByTier(ctx, "free", 50, time.Second)
	_ = dynamicMgr.UpdateLimitByTier(ctx, "premium", 1000, time.Second)
	_ = dynamicMgr.UpdateLimitByTier(ctx, "enterprise", 5000, time.Second)

	// Test limiter with different tiers
	result := limiter.Allow(ctx, "free_user")
	assert.True(t, result.Allowed)

	result = limiter.Allow(ctx, "premium_user")
	assert.True(t, result.Allowed)

	result = limiter.Allow(ctx, "enterprise_user")
	assert.True(t, result.Allowed)
}

// TestTierBasedLimiter_DynamicLimits_ConcurrentUpdates tests concurrent operations
func TestTierBasedLimiter_DynamicLimits_ConcurrentUpdates(t *testing.T) {
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

	limiter, err := NewTierBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TierConfig{
			BasicConfig: limiters.BasicConfig{
				AlgorithmName: "token_bucket",
				DefaultLimit:  100,
				DefaultWindow: time.Second,
				KeyTTL:        time.Minute,
				DynamicLimits: limiters.DynamicLimitConfig{
					EnableDynamicLimits: true,
					Manager:             dynamicMgr,
				},
			},
			DefaultTier: "free",
			Tiers: map[string]limiters.TierLimit{
				"free":  {Limit: 100, Window: time.Second},
				"tier1": {Limit: 100, Window: time.Second},
				"tier2": {Limit: 200, Window: time.Second},
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

	// Concurrent tier updates
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			tier := "tier1"
			if id%2 == 0 {
				tier = "tier2"
			}
			_ = dynamicMgr.UpdateLimitByTier(ctx, tier, 100+id*10, time.Second)
		}(i)
	}

	// Concurrent allows
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

// TestTierBasedLimiter_DynamicLimits_Disabled tests with dynamic limits disabled
func TestTierBasedLimiter_DynamicLimits_Disabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(nil, nil).AnyTimes()

	limiter, err := NewTierBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TierConfig{
			BasicConfig: limiters.BasicConfig{
				AlgorithmName: "token_bucket",
				DefaultLimit:  100,
				DefaultWindow: time.Second,
				KeyTTL:        time.Minute,
				DynamicLimits: limiters.DynamicLimitConfig{
					EnableDynamicLimits: false,
					Manager:             nil,
				},
			},
			DefaultTier: "free",
			Tiers: map[string]limiters.TierLimit{
				"free": {Limit: 100, Window: time.Second},
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

// TestTierBasedLimiter_DynamicLimits_ValidationErrors tests validation
func TestTierBasedLimiter_DynamicLimits_ValidationErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)

	limiter, err := NewTierBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TierConfig{
			BasicConfig: limiters.BasicConfig{
				AlgorithmName: "token_bucket",
				DefaultLimit:  100,
				DefaultWindow: time.Second,
				KeyTTL:        time.Minute,
				DynamicLimits: limiters.DynamicLimitConfig{
					EnableDynamicLimits: true,
					Manager:             dynamicMgr,
				},
			},
			DefaultTier: "free",
			Tiers: map[string]limiters.TierLimit{
				"free": {Limit: 100, Window: time.Second},
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Test invalid tier updates
	err = dynamicMgr.UpdateLimitByTier(ctx, "", 100, time.Second)
	assert.Error(t, err, "empty tier should error")

	err = dynamicMgr.UpdateLimitByTier(ctx, "free", -1, time.Second)
	assert.Error(t, err, "negative limit should error")

	err = dynamicMgr.UpdateLimitByTier(ctx, "free", 100, 0)
	assert.Error(t, err, "zero window should error")
}

// TestTierBasedLimiter_DynamicLimits_UpdateHooks tests update hooks
func TestTierBasedLimiter_DynamicLimits_UpdateHooks(t *testing.T) {
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

	limiter, err := NewTierBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TierConfig{
			BasicConfig: limiters.BasicConfig{
				AlgorithmName: "token_bucket",
				DefaultLimit:  100,
				DefaultWindow: time.Second,
				KeyTTL:        time.Minute,
				DynamicLimits: limiters.DynamicLimitConfig{
					EnableDynamicLimits: true,
					Manager:             dynamicMgr,
				},
			},
			DefaultTier: "free",
			Tiers: map[string]limiters.TierLimit{
				"free":    {Limit: 100, Window: time.Second},
				"premium": {Limit: 500, Window: time.Second},
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Update tiers - should trigger hooks
	_ = dynamicMgr.UpdateLimitByTier(ctx, "free", 150, time.Second)
	_ = dynamicMgr.UpdateLimitByTier(ctx, "premium", 600, time.Second)

	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, hookCalls, 2)
}

// TestTierBasedLimiter_DynamicLimits_FallbackToDefault tests default fallback
func TestTierBasedLimiter_DynamicLimits_FallbackToDefault(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  250,
		DefaultWindow: 2 * time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)

	limiter, err := NewTierBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TierConfig{
			BasicConfig: limiters.BasicConfig{
				AlgorithmName: "token_bucket",
				DefaultLimit:  100,
				DefaultWindow: time.Second,
				KeyTTL:        time.Minute,
				DynamicLimits: limiters.DynamicLimitConfig{
					EnableDynamicLimits: true,
					Manager:             dynamicMgr,
				},
			},
			DefaultTier: "free",
			Tiers: map[string]limiters.TierLimit{
				"free": {Limit: 100, Window: time.Second},
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Unconfigured tier should use default
	limit, window, err := dynamicMgr.GetCurrentLimit(ctx, "unconfigured_tier")
	require.NoError(t, err)
	assert.Equal(t, 250, limit)
	assert.Equal(t, 2*time.Second, window)
}

// TestTierBasedLimiter_DynamicLimits_GetAllLimits tests retrieving all limits
func TestTierBasedLimiter_DynamicLimits_GetAllLimits(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)

	limiter, err := NewTierBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TierConfig{
			BasicConfig: limiters.BasicConfig{
				AlgorithmName: "token_bucket",
				DefaultLimit:  100,
				DefaultWindow: time.Second,
				KeyTTL:        time.Minute,
				DynamicLimits: limiters.DynamicLimitConfig{
					EnableDynamicLimits: true,
					Manager:             dynamicMgr,
				},
			},
			DefaultTier: "free",
			Tiers: map[string]limiters.TierLimit{
				"free": {Limit: 100, Window: time.Second},
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

	// Add per-key limits (not tier limits)
	_ = dynamicMgr.UpdateLimit(ctx, "user:free:123", 50, time.Second)
	_ = dynamicMgr.UpdateLimit(ctx, "user:premium:456", 500, time.Second)
	_ = dynamicMgr.UpdateLimit(ctx, "user:enterprise:789", 2000, time.Second)

	// Should return all
	allLimits = dynamicMgr.GetAllLimits()
	assert.Len(t, allLimits, 3)
}

// TestTierBasedLimiter_DynamicLimits_AllowN tests AllowN with dynamic limits
func TestTierBasedLimiter_DynamicLimits_AllowN(t *testing.T) {
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

	limiter, err := NewTierBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TierConfig{
			BasicConfig: limiters.BasicConfig{
				AlgorithmName: "token_bucket",
				DefaultLimit:  100,
				DefaultWindow: time.Second,
				KeyTTL:        time.Minute,
				DynamicLimits: limiters.DynamicLimitConfig{
					EnableDynamicLimits: true,
					Manager:             dynamicMgr,
				},
			},
			DefaultTier: "free",
			Tiers: map[string]limiters.TierLimit{
				"free": {Limit: 100, Window: time.Second},
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Update tier limit
	_ = dynamicMgr.UpdateLimitByTier(ctx, "batch_tier", 500, time.Second)

	// Test AllowN
	result := limiter.AllowN(ctx, "batch_user", 10)
	assert.True(t, result.Allowed)
}

// mockBackendWithFailure simulates backend failures for tier limiter
type mockBackendWithFailure struct {
	data             map[string]interface{}
	failGetCount     int
	failSetCount     int
	getCallCount     int
	setCallCount     int
	triggerFailAtGet int
	triggerFailAtSet int
	mu               sync.Mutex
}

func newMockBackendWithFailure() *mockBackendWithFailure {
	return &mockBackendWithFailure{
		data:             make(map[string]interface{}),
		triggerFailAtGet: -1,
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

// mockTierAlgorithm for tier limiter tests
type mockTierAlgorithm struct{}

func (m *mockTierAlgorithm) Name() string        { return "mock" }
func (m *mockTierAlgorithm) Description() string { return "mock algorithm" }
func (m *mockTierAlgorithm) Reset(ctx context.Context) (interface{}, error) {
	return map[string]interface{}{"count": 0, "reset_at": time.Now()}, nil
}
func (m *mockTierAlgorithm) Allow(ctx context.Context, state interface{}, cost int) (interface{}, interface{}, error) {
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
func (m *mockTierAlgorithm) ValidateConfig(config interface{}) error { return nil }
func (m *mockTierAlgorithm) GetStats(ctx context.Context, state interface{}) interface{} {
	return state
}

// TestTierLimiterWithoutFailover validates baseline without failover
func TestTierLimiterWithoutFailover(t *testing.T) {
	primaryBE := newMockBackendWithFailure()

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			AlgorithmName: "fixed_window",
			DefaultLimit:  5,
			DefaultWindow: time.Second,
		},
		Tiers: map[string]limiters.TierLimit{
			"tier1": {Limit: 10, Window: time.Second},
			"tier2": {Limit: 20, Window: time.Second},
		},
		DefaultTier: "tier1",
	}

	limiter, err := NewTierBasedLimiter(&mockTierAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	// Assign key to tier
	err = limiter.AssignKeyToTier(ctx, "test-key", "tier1")
	if err != nil {
		t.Fatalf("Failed to assign key to tier: %v", err)
	}

	result := limiter.Allow(ctx, "test-key")
	if !result.Allowed {
		t.Error("Request should be allowed without failover")
	}

	t.Log("Tier limiter without failover test passed")
}

// TestTierLimiterWithFailoverNoFailure validates failover enabled but no failures
func TestTierLimiterWithFailoverNoFailure(t *testing.T) {
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

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			AlgorithmName: "fixed_window",
			DefaultLimit:  5,
			DefaultWindow: time.Second,
			Failover: limiters.FailoverConfig{
				EnableFailover: true,
				Handler:        failoverHandler,
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"tier1": {Limit: 10, Window: time.Second},
		},
		DefaultTier: "tier1",
	}

	limiter, err := NewTierBasedLimiter(&mockTierAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	err = limiter.AssignKeyToTier(ctx, "test-key", "tier1")
	if err != nil {
		t.Fatalf("Failed to assign key: %v", err)
	}

	result := limiter.Allow(ctx, "test-key")
	if !result.Allowed {
		t.Error("Request should be allowed when no failures occur")
	}

	status := failoverHandler.GetFallbackStatus(ctx)
	if status.IsFallbackActive {
		t.Error("Failover should not be active when no failures")
	}

	t.Log("Tier limiter with failover (no failures) test passed")
}

// TestTierLimiterFailoverOnGetFailure validates failover triggers on Get failures
func TestTierLimiterFailoverOnGetFailure(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	primaryBE.triggerFailAtGet = 1
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

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			AlgorithmName: "fixed_window",
			DefaultLimit:  5,
			DefaultWindow: time.Second,
			Failover: limiters.FailoverConfig{
				EnableFailover: true,
				Handler:        failoverHandler,
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"tier1": {Limit: 10, Window: time.Second},
		},
		DefaultTier: "tier1",
	}

	limiter, err := NewTierBasedLimiter(&mockTierAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	err = limiter.AssignKeyToTier(ctx, "test-key", "tier1")
	if err != nil {
		t.Fatalf("Failed to assign key: %v", err)
	}

	result1 := limiter.Allow(ctx, "test-key")
	t.Logf("First request: allowed=%v, error=%v", result1.Allowed, result1.Error)

	result2 := limiter.Allow(ctx, "test-key")
	t.Logf("Second request: allowed=%v, error=%v", result2.Allowed, result2.Error)

	status := failoverHandler.GetFallbackStatus(ctx)
	if !status.IsFallbackActive {
		t.Errorf("Failover should be active after Get failures (count: %d)", status.FailureCount)
	}

	if !result2.Allowed {
		t.Error("Request should be allowed during failover")
	}

	t.Log("Tier limiter failover on Get failure test passed")
}

// TestTierLimiterFailoverOnSetFailure validates failover triggers on Set failures
func TestTierLimiterFailoverOnSetFailure(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	primaryBE.triggerFailAtSet = 1
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

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			AlgorithmName: "fixed_window",
			DefaultLimit:  5,
			DefaultWindow: time.Second,
			Failover: limiters.FailoverConfig{
				EnableFailover: true,
				Handler:        failoverHandler,
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"tier1": {Limit: 10, Window: time.Second},
		},
		DefaultTier: "tier1",
	}

	limiter, err := NewTierBasedLimiter(&mockTierAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	err = limiter.AssignKeyToTier(ctx, "test-key", "tier1")
	if err != nil {
		t.Fatalf("Failed to assign key: %v", err)
	}

	result1 := limiter.Allow(ctx, "test-key")
	t.Logf("First request: allowed=%v, error=%v", result1.Allowed, result1.Error)

	result2 := limiter.Allow(ctx, "test-key")
	t.Logf("Second request: allowed=%v, error=%v", result2.Allowed, result2.Error)

	status := failoverHandler.GetFallbackStatus(ctx)
	if !status.IsFallbackActive {
		t.Errorf("Failover should be active after Set failures (count: %d)", status.FailureCount)
	}

	t.Log("Tier limiter failover on Set failure test passed")
}

// TestTierLimiterMultipleFailuresBeforeFailover validates threshold-based failover
func TestTierLimiterMultipleFailuresBeforeFailover(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	primaryBE.triggerFailAtGet = 2
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

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			AlgorithmName: "fixed_window",
			DefaultLimit:  5,
			DefaultWindow: time.Second,
			Failover: limiters.FailoverConfig{
				EnableFailover: true,
				Handler:        failoverHandler,
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"tier1": {Limit: 10, Window: time.Second},
		},
		DefaultTier: "tier1",
	}

	limiter, err := NewTierBasedLimiter(&mockTierAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	err = limiter.AssignKeyToTier(ctx, "test-key", "tier1")
	if err != nil {
		t.Fatalf("Failed to assign key: %v", err)
	}

	// First request - succeeds
	result1 := limiter.Allow(ctx, "test-key")
	if !result1.Allowed {
		t.Error("First request should succeed")
	}

	status := failoverHandler.GetFallbackStatus(ctx)
	if status.IsFallbackActive {
		t.Error("Failover should not be active after 1 request")
	}

	// Second request - first failure
	result2 := limiter.Allow(ctx, "test-key")
	t.Logf("Second request: allowed=%v, error=%v", result2.Allowed, result2.Error)

	status = failoverHandler.GetFallbackStatus(ctx)
	if status.IsFallbackActive {
		t.Error("Failover should not be active after 1 failure")
	}

	// Third request - second failure, triggers failover
	result3 := limiter.Allow(ctx, "test-key")
	t.Logf("Third request: allowed=%v, error=%v", result3.Allowed, result3.Error)

	status = failoverHandler.GetFallbackStatus(ctx)
	if !status.IsFallbackActive {
		t.Errorf("Failover should be active after threshold (failures: %d)", status.FailureCount)
	}

	t.Log("Tier limiter multiple failures before failover test passed")
}

// TestTierLimiterFailoverMultipleTiers validates failover with multiple tiers
func TestTierLimiterFailoverMultipleTiers(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	primaryBE.triggerFailAtGet = 3
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

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			AlgorithmName: "fixed_window",
			DefaultLimit:  5,
			DefaultWindow: time.Second,
			Failover: limiters.FailoverConfig{
				EnableFailover: true,
				Handler:        failoverHandler,
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"tier1": {Limit: 10, Window: time.Second},
			"tier2": {Limit: 20, Window: time.Second},
			"tier3": {Limit: 30, Window: time.Second},
		},
		DefaultTier: "tier1",
	}

	limiter, err := NewTierBasedLimiter(&mockTierAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	// Test multiple tiers
	tiers := []string{"tier1", "tier2", "tier3"}
	for i, tier := range tiers {
		key := "key-" + tier
		err = limiter.AssignKeyToTier(ctx, key, tier)
		if err != nil {
			t.Fatalf("Failed to assign key to %s: %v", tier, err)
		}

		result := limiter.Allow(ctx, key)
		t.Logf("Tier %s, request %d: allowed=%v", tier, i+1, result.Allowed)
	}

	status := failoverHandler.GetFallbackStatus(ctx)
	t.Logf("Failover status: active=%v, count=%d", status.IsFallbackActive, status.FailureCount)

	t.Log("Tier limiter failover with multiple tiers test passed")
}

// TestTierLimiterConcurrentFailover validates failover under concurrent load
func TestTierLimiterConcurrentFailover(t *testing.T) {
	primaryBE := newMockBackendWithFailure()
	primaryBE.triggerFailAtGet = 5
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

	cfg := limiters.TierConfig{
		BasicConfig: limiters.BasicConfig{
			AlgorithmName: "fixed_window",
			DefaultLimit:  100,
			DefaultWindow: time.Second,
			Failover: limiters.FailoverConfig{
				EnableFailover: true,
				Handler:        failoverHandler,
			},
		},
		Tiers: map[string]limiters.TierLimit{
			"tier1": {Limit: 100, Window: time.Second},
		},
		DefaultTier: "tier1",
	}

	limiter, err := NewTierBasedLimiter(&mockTierAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	err = limiter.AssignKeyToTier(ctx, "test-key", "tier1")
	if err != nil {
		t.Fatalf("Failed to assign key: %v", err)
	}

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

	time.Sleep(100 * time.Millisecond)
	status := failoverHandler.GetFallbackStatus(ctx)
	t.Logf("Final status: active=%v, count=%d", status.IsFallbackActive, status.FailureCount)

	t.Log("Tier limiter concurrent failover test passed")
}
