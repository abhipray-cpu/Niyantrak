package tenant

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

// TestNewTenantBasedLimiter_ValidConfig tests constructor with valid configuration
func TestNewTenantBasedLimiter_ValidConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant-a": {Limit: 10, Window: time.Minute},
			"tenant-b": {Limit: 100, Window: time.Minute},
			"tenant-c": {Limit: 1000, Window: time.Minute},
		},
		DefaultTenant: "tenant-a",
	}

	limiter, err := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limiter == nil {
		t.Fatal("limiter should not be nil")
	}
	if limiter.Type() != "tenant" {
		t.Errorf("expected type 'tenant', got %q", limiter.Type())
	}
}

// TestNewTenantBasedLimiter_InvalidConfig tests constructor validation
func TestNewTenantBasedLimiter_InvalidConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	tests := []struct {
		name    string
		algo    algorithm.Algorithm
		be      backend.Backend
		cfg     limiters.TenantConfig
		wantErr bool
	}{
		{
			name:    "nil algorithm",
			algo:    nil,
			be:      mockBackend,
			cfg:     limiters.TenantConfig{},
			wantErr: true,
		},
		{
			name:    "nil backend",
			algo:    mockAlgo,
			be:      nil,
			cfg:     limiters.TenantConfig{},
			wantErr: true,
		},
		{
			name: "zero default limit",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.TenantConfig{
				BasicConfig: limiters.BasicConfig{
					DefaultLimit:  0,
					DefaultWindow: time.Second,
				},
				DefaultTenant: "tenant-a",
			},
			wantErr: true,
		},
		{
			name: "zero default window",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.TenantConfig{
				BasicConfig: limiters.BasicConfig{
					DefaultLimit:  100,
					DefaultWindow: 0,
				},
				DefaultTenant: "tenant-a",
			},
			wantErr: true,
		},
		{
			name: "empty default tenant",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.TenantConfig{
				BasicConfig: limiters.BasicConfig{
					DefaultLimit:  100,
					DefaultWindow: time.Second,
				},
				Tenants:       map[string]limiters.TenantLimit{"tenant-a": {Limit: 10, Window: time.Minute}},
				DefaultTenant: "",
			},
			wantErr: true,
		},
		{
			name: "empty tenants",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.TenantConfig{
				BasicConfig: limiters.BasicConfig{
					DefaultLimit:  100,
					DefaultWindow: time.Second,
				},
				Tenants:       map[string]limiters.TenantLimit{},
				DefaultTenant: "tenant-a",
			},
			wantErr: true,
		},
		{
			name: "default tenant not in tenants",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.TenantConfig{
				BasicConfig: limiters.BasicConfig{
					DefaultLimit:  100,
					DefaultWindow: time.Second,
				},
				Tenants:       map[string]limiters.TenantLimit{"tenant-a": {Limit: 10, Window: time.Minute}},
				DefaultTenant: "tenant-x",
			},
			wantErr: true,
		},
		{
			name: "negative tenant limit",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.TenantConfig{
				BasicConfig: limiters.BasicConfig{
					DefaultLimit:  100,
					DefaultWindow: time.Second,
				},
				Tenants:       map[string]limiters.TenantLimit{"tenant-a": {Limit: -10, Window: time.Minute}},
				DefaultTenant: "tenant-a",
			},
			wantErr: true,
		},
		{
			name: "zero tenant window",
			algo: mockAlgo,
			be:   mockBackend,
			cfg: limiters.TenantConfig{
				BasicConfig: limiters.BasicConfig{
					DefaultLimit:  100,
					DefaultWindow: time.Second,
				},
				Tenants:       map[string]limiters.TenantLimit{"tenant-a": {Limit: 10, Window: time.Duration(0)}},
				DefaultTenant: "tenant-a",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTenantBasedLimiter(tt.algo, tt.be, tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTenantBasedLimiter() error = %v, wantErr %v", err, tt.wantErr)
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

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant-a": {Limit: 10, Window: time.Minute},
		},
		DefaultTenant: "tenant-a",
	}

	limiter, _ := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)
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

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant-b": {Limit: 100, Window: time.Minute},
		},
		DefaultTenant: "tenant-b",
	}

	limiter, _ := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)
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

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant-a": {Limit: 10, Window: time.Minute},
		},
		DefaultTenant: "tenant-a",
	}

	limiter, _ := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)

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

// TestSetTenantLimit tests setting/updating tenant limits
func TestSetTenantLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant-a": {Limit: 10, Window: time.Minute},
		},
		DefaultTenant: "tenant-a",
	}

	limiter, _ := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)

	// Set new limit for existing tenant
	err := limiter.SetTenantLimit(context.Background(), "tenant-a", 50, 2*time.Minute)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify updated limit
	limit, window, err := limiter.GetTenantLimit(context.Background(), "tenant-a")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if limit != 50 {
		t.Errorf("expected limit 50, got %d", limit)
	}
	if window != 2*time.Minute {
		t.Errorf("expected window 2m, got %v", window)
	}

	// Add new tenant
	err = limiter.SetTenantLimit(context.Background(), "tenant-d", 500, 5*time.Minute)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	limit, _, err = limiter.GetTenantLimit(context.Background(), "tenant-d")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if limit != 500 {
		t.Errorf("expected limit 500, got %d", limit)
	}
}

// TestGetTenantLimit tests retrieving tenant configuration
func TestGetTenantLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant-a": {Limit: 10, Window: time.Minute},
			"tenant-b": {Limit: 100, Window: time.Minute},
			"tenant-c": {Limit: 1000, Window: time.Hour},
		},
		DefaultTenant: "tenant-a",
	}

	limiter, _ := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)

	tests := []struct {
		name      string
		tenantID  string
		expLimit  int
		expWindow time.Duration
		expErr    bool
	}{
		{"tenant-a", "tenant-a", 10, time.Minute, false},
		{"tenant-b", "tenant-b", 100, time.Minute, false},
		{"tenant-c", "tenant-c", 1000, time.Hour, false},
		{"nonexistent tenant", "unknown", 0, 0, true},
		{"empty tenant", "", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, window, err := limiter.GetTenantLimit(context.Background(), tt.tenantID)
			if (err != nil) != tt.expErr {
				t.Errorf("GetTenantLimit() error = %v, wantErr %v", err, tt.expErr)
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

// TestAssignKeyToTenant tests assigning keys to tenants
func TestAssignKeyToTenant(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant-a": {Limit: 10, Window: time.Minute},
			"tenant-b": {Limit: 100, Window: time.Minute},
		},
		DefaultTenant: "tenant-a",
	}

	limiter, _ := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)

	// Assign key to tenant-b
	err := limiter.AssignKeyToTenant(context.Background(), "user:123", "tenant-b")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify assignment
	tenantID, err := limiter.GetKeyTenant(context.Background(), "user:123")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if tenantID != "tenant-b" {
		t.Errorf("expected tenant 'tenant-b', got %q", tenantID)
	}

	// Test invalid inputs
	tests := []struct {
		name   string
		key    string
		tenant string
		valid  bool
	}{
		{"empty key", "", "tenant-b", false},
		{"empty tenant", "user:456", "", false},
		{"invalid tenant", "user:789", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := limiter.AssignKeyToTenant(context.Background(), tt.key, tt.tenant)
			if (err != nil) != !tt.valid {
				t.Errorf("AssignKeyToTenant() error = %v, expected error = %v", err, !tt.valid)
			}
		})
	}
}

// TestGetKeyTenant tests retrieving key tenant assignment
func TestGetKeyTenant(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant-a": {Limit: 10, Window: time.Minute},
			"tenant-b": {Limit: 100, Window: time.Minute},
		},
		DefaultTenant: "tenant-a",
	}

	limiter, _ := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)

	// Unassigned key should return default tenant
	tenantID, err := limiter.GetKeyTenant(context.Background(), "unassigned")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if tenantID != "tenant-a" {
		t.Errorf("expected default tenant 'tenant-a', got %q", tenantID)
	}

	// Assign and verify
	limiter.AssignKeyToTenant(context.Background(), "user:123", "tenant-b")
	tenantID, err = limiter.GetKeyTenant(context.Background(), "user:123")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if tenantID != "tenant-b" {
		t.Errorf("expected tenant 'tenant-b', got %q", tenantID)
	}

	// Empty key should error
	_, err = limiter.GetKeyTenant(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty key")
	}
}

// TestGetTenantStats tests retrieving aggregated tenant statistics
func TestGetTenantStats(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant-a": {Limit: 10, Window: time.Minute},
		},
		DefaultTenant: "tenant-a",
	}

	limiter, _ := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)

	// Assign keys to tenant
	limiter.AssignKeyToTenant(context.Background(), "user:1", "tenant-a")
	limiter.AssignKeyToTenant(context.Background(), "user:2", "tenant-a")
	limiter.AssignKeyToTenant(context.Background(), "user:3", "tenant-a")

	// Get stats
	stats := limiter.GetTenantStats(context.Background(), "tenant-a")
	if stats == nil {
		t.Fatal("stats should not be nil")
	}

	if stats.TenantID != "tenant-a" {
		t.Errorf("expected tenant 'tenant-a', got %q", stats.TenantID)
	}

	if stats.TotalKeys != 3 {
		t.Errorf("expected 3 total keys, got %d", stats.TotalKeys)
	}

	// Test non-existent tenant
	stats = limiter.GetTenantStats(context.Background(), "unknown")
	if stats != nil {
		t.Error("expected nil stats for non-existent tenant")
	}
}

// TestListTenants tests listing all configured tenants
func TestListTenants(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant-a": {Limit: 10, Window: time.Minute},
			"tenant-b": {Limit: 100, Window: time.Minute},
			"tenant-c": {Limit: 1000, Window: time.Hour},
		},
		DefaultTenant: "tenant-a",
	}

	limiter, _ := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)

	tenants, err := limiter.ListTenants(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(tenants) != 3 {
		t.Errorf("expected 3 tenants, got %d", len(tenants))
	}

	tenantMap := make(map[string]bool)
	for _, t := range tenants {
		tenantMap[t] = true
	}

	expected := []string{"tenant-a", "tenant-b", "tenant-c"}
	for _, exp := range expected {
		if !tenantMap[exp] {
			t.Errorf("expected tenant %q not found", exp)
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

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant-a": {Limit: 10, Window: time.Minute},
		},
		DefaultTenant: "tenant-a",
	}

	limiter, _ := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)

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

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant-a": {Limit: 10, Window: time.Minute},
		},
		DefaultTenant: "tenant-a",
	}

	limiter, _ := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)

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

// TestGetStats tests statistics retrieval
func TestGetStats(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant-a": {Limit: 10, Window: time.Minute},
			"tenant-b": {Limit: 100, Window: time.Minute},
		},
		DefaultTenant: "tenant-a",
	}

	limiter, _ := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)

	// Stats for unassigned key (default tenant)
	stats := limiter.GetStats(context.Background(), "unassigned")
	if stats == nil {
		t.Fatal("stats should not be nil")
	}

	statsMap, ok := stats.(map[string]interface{})
	if !ok {
		t.Fatal("stats should be a map")
	}

	if statsMap["tenant"] != "tenant-a" {
		t.Errorf("expected tenant 'tenant-a', got %v", statsMap["tenant"])
	}

	// Stats for assigned key
	limiter.AssignKeyToTenant(context.Background(), "user:123", "tenant-b")
	stats = limiter.GetStats(context.Background(), "user:123")
	statsMap = stats.(map[string]interface{})
	if statsMap["tenant"] != "tenant-b" {
		t.Errorf("expected tenant 'tenant-b', got %v", statsMap["tenant"])
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
	totalOperations := numGoroutines * requestsPerGoroutine

	// Setup generic expectations for concurrent access
	mockBackend.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(nil, backend.ErrKeyNotFound).
		Times(totalOperations)

	mockAlgo.EXPECT().
		Reset(gomock.Any()).
		Return(map[string]interface{}{}, nil).
		Times(totalOperations)

	mockAlgo.EXPECT().
		Allow(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		Times(totalOperations)

	mockBackend.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(totalOperations)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant-a": {Limit: 10, Window: time.Minute},
			"tenant-b": {Limit: 100, Window: time.Minute},
		},
		DefaultTenant: "tenant-a",
	}

	limiter, _ := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)

	done := make(chan bool, numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(_ int) {
			for r := 0; r < requestsPerGoroutine; r++ {
				limiter.Allow(context.Background(), "user:123")
				limiter.SetTenantLimit(context.Background(), "tenant-a", 50, 2*time.Minute)
				limiter.GetKeyTenant(context.Background(), "user:123")
			}
			done <- true
		}(g)
	}

	for g := 0; g < numGoroutines; g++ {
		<-done
	}
}

// TestMultipleKeys tests independent key handling with different tenants
func TestMultipleKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant-a": {Limit: 10, Window: time.Minute},
			"tenant-b": {Limit: 100, Window: time.Minute},
		},
		DefaultTenant: "tenant-a",
	}

	limiter, _ := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)

	// Assign keys to different tenants
	limiter.AssignKeyToTenant(context.Background(), "user:1", "tenant-a")
	limiter.AssignKeyToTenant(context.Background(), "user:2", "tenant-b")
	limiter.AssignKeyToTenant(context.Background(), "user:3", "tenant-b")

	// Verify assignments are independent
	tenant1, _ := limiter.GetKeyTenant(context.Background(), "user:1")
	tenant2, _ := limiter.GetKeyTenant(context.Background(), "user:2")
	tenant3, _ := limiter.GetKeyTenant(context.Background(), "user:3")

	if tenant1 != "tenant-a" || tenant2 != "tenant-b" || tenant3 != "tenant-b" {
		t.Error("tenant assignments not independent")
	}

	// Verify stats are independent
	stats1 := limiter.GetStats(context.Background(), "user:1")
	stats2 := limiter.GetStats(context.Background(), "user:2")

	statsMap1 := stats1.(map[string]interface{})
	statsMap2 := stats2.(map[string]interface{})

	if statsMap1["tenant"] == statsMap2["tenant"] {
		t.Error("expected different tenant stats")
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

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant-a": {Limit: 10, Window: time.Minute},
		},
		DefaultTenant: "tenant-a",
	}

	limiter, _ := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(context.Background(), "user:123")
	}
}

// BenchmarkSetTenantLimit benchmarks SetTenantLimit operation
func BenchmarkSetTenantLimit(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant-a": {Limit: 10, Window: time.Minute},
		},
		DefaultTenant: "tenant-a",
	}

	limiter, _ := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.SetTenantLimit(context.Background(), "tenant-a", 50, 2*time.Minute)
	}
}

// TestTenantLimiter_Observability_NoOp tests noop observability
func TestTenantLimiter_Observability_NoOp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBackend := mocks.NewMockBackend(ctrl)
	mockAlgo := mocks.NewMockAlgorithm(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
		},
		DefaultTenant: "default",
		Tenants: map[string]limiters.TenantLimit{
			"default": {Limit: 10, Window: time.Second},
		},
	}

	limiter, err := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	mockBackend.EXPECT().
		Get(gomock.Any(), "test_key").
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
		Set(gomock.Any(), "test_key", gomock.Any(), time.Second).
		Return(nil).
		Times(1)

	result := limiter.Allow(context.Background(), "test_key")
	if result == nil || !result.Allowed {
		t.Error("expected allowed result")
	}
}

// TestTenantLimiter_Observability_WithLogger tests logger observability
func TestTenantLimiter_Observability_WithLogger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBackend := mocks.NewMockBackend(ctrl)
	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			Observability: limiters.ObservabilityConfig{
				Logger: mockLogger,
			},
		},
		DefaultTenant: "default",
		Tenants: map[string]limiters.TenantLimit{
			"default": {Limit: 10, Window: time.Second},
		},
	}

	limiter, err := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	mockBackend.EXPECT().
		Get(gomock.Any(), "test_key").
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
		Set(gomock.Any(), "test_key", gomock.Any(), time.Second).
		Return(nil).
		Times(1)

	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()

	result := limiter.Allow(context.Background(), "test_key")
	if result == nil || !result.Allowed {
		t.Error("expected allowed result")
	}
}

// TestTenantLimiter_Observability_WithMetrics tests metrics observability
func TestTenantLimiter_Observability_WithMetrics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBackend := mocks.NewMockBackend(ctrl)
	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			Observability: limiters.ObservabilityConfig{
				Metrics: mockMetrics,
			},
		},
		DefaultTenant: "default",
		Tenants: map[string]limiters.TenantLimit{
			"default": {Limit: 10, Window: time.Second},
		},
	}

	limiter, err := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	mockBackend.EXPECT().
		Get(gomock.Any(), "test_key").
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
		Set(gomock.Any(), "test_key", gomock.Any(), time.Second).
		Return(nil).
		Times(1)

	mockMetrics.EXPECT().RecordDecisionLatency(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	result := limiter.Allow(context.Background(), "test_key")
	if result == nil || !result.Allowed {
		t.Error("expected allowed result")
	}
}

// TestTenantLimiter_Observability_WithTracer tests tracer observability
func TestTenantLimiter_Observability_WithTracer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBackend := mocks.NewMockBackend(ctrl)
	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			Observability: limiters.ObservabilityConfig{
				Tracer: mockTracer,
			},
		},
		DefaultTenant: "default",
		Tenants: map[string]limiters.TenantLimit{
			"default": {Limit: 10, Window: time.Second},
		},
	}

	limiter, err := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	gomock.InOrder(
		mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.tenant.check").Return(&obstypes.SpanContext{}),
		mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes(),
		mockTracer.EXPECT().EndSpan(gomock.Any()).Times(1),
	)

	mockBackend.EXPECT().
		Get(gomock.Any(), "test_key").
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
		Set(gomock.Any(), "test_key", gomock.Any(), time.Second).
		Return(nil).
		Times(1)

	result := limiter.Allow(context.Background(), "test_key")
	if result == nil || !result.Allowed {
		t.Error("expected allowed result")
	}
}

// TestTenantLimiter_Observability_AllThree tests all three observability types
func TestTenantLimiter_Observability_AllThree(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBackend := mocks.NewMockBackend(ctrl)
	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			Observability: limiters.ObservabilityConfig{
				Logger:  mockLogger,
				Metrics: mockMetrics,
				Tracer:  mockTracer,
			},
		},
		DefaultTenant: "default",
		Tenants: map[string]limiters.TenantLimit{
			"default": {Limit: 10, Window: time.Second},
		},
	}

	limiter, err := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	gomock.InOrder(
		mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.tenant.check").Return(&obstypes.SpanContext{}),
		mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes(),
		mockTracer.EXPECT().EndSpan(gomock.Any()).Times(1),
	)

	mockBackend.EXPECT().
		Get(gomock.Any(), "test_key").
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
		Set(gomock.Any(), "test_key", gomock.Any(), time.Second).
		Return(nil).
		Times(1)

	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordDecisionLatency(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	result := limiter.Allow(context.Background(), "test_key")
	if result == nil || !result.Allowed {
		t.Error("expected allowed result")
	}
}

// TestTenantLimiter_Observability_AllowN tests observability with AllowN
func TestTenantLimiter_Observability_AllowN(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBackend := mocks.NewMockBackend(ctrl)
	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  100,
			DefaultWindow: time.Second,
			Observability: limiters.ObservabilityConfig{
				Logger:  mockLogger,
				Metrics: mockMetrics,
				Tracer:  mockTracer,
			},
		},
		DefaultTenant: "default",
		Tenants: map[string]limiters.TenantLimit{
			"default": {Limit: 100, Window: time.Second},
		},
	}

	limiter, err := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	gomock.InOrder(
		mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.tenant.check").Return(&obstypes.SpanContext{}),
		mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes(),
		mockTracer.EXPECT().EndSpan(gomock.Any()).Times(1),
	)

	mockBackend.EXPECT().
		Get(gomock.Any(), "test_key").
		Return(nil, backend.ErrKeyNotFound).
		Times(1)

	mockAlgo.EXPECT().
		Reset(gomock.Any()).
		Return(map[string]interface{}{"tokens": 100}, nil).
		Times(1)

	mockAlgo.EXPECT().
		Allow(gomock.Any(), gomock.Any(), 10).
		Return(map[string]interface{}{}, &algorithm.TokenBucketResult{Allowed: true}, nil).
		Times(1)

	mockBackend.EXPECT().
		Set(gomock.Any(), "test_key", gomock.Any(), time.Second).
		Return(nil).
		Times(1)

	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordDecisionLatency(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	result := limiter.AllowN(context.Background(), "test_key", 10)
	if result == nil || !result.Allowed {
		t.Error("expected allowed result")
	}
}

// TestTenantLimiter_Observability_SetTenantLimit tests observability for SetTenantLimit
func TestTenantLimiter_Observability_SetTenantLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBackend := mocks.NewMockBackend(ctrl)
	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			Observability: limiters.ObservabilityConfig{
				Logger: mockLogger,
			},
		},
		DefaultTenant: "default",
		Tenants: map[string]limiters.TenantLimit{
			"default": {Limit: 10, Window: time.Second},
			"premium": {Limit: 50, Window: time.Second},
		},
	}

	limiter, err := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()

	err = limiter.SetTenantLimit(context.Background(), "premium", 100, time.Second)
	if err != nil {
		t.Fatalf("failed to set tenant limit: %v", err)
	}
}

// TestTenantLimiter_Observability_AssignKey tests observability for AssignKeyToTenant
func TestTenantLimiter_Observability_AssignKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBackend := mocks.NewMockBackend(ctrl)
	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			Observability: limiters.ObservabilityConfig{
				Logger: mockLogger,
			},
		},
		DefaultTenant: "default",
		Tenants: map[string]limiters.TenantLimit{
			"default": {Limit: 10, Window: time.Second},
			"premium": {Limit: 100, Window: time.Second},
		},
	}

	limiter, err := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()

	err = limiter.AssignKeyToTenant(context.Background(), "premium_key", "premium")
	if err != nil {
		t.Fatalf("failed to assign key: %v", err)
	}
}

// TestTenantLimiter_Observability_GetStats tests observability for GetTenantStats
func TestTenantLimiter_Observability_GetStats(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBackend := mocks.NewMockBackend(ctrl)
	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			Observability: limiters.ObservabilityConfig{
				Logger: mockLogger,
			},
		},
		DefaultTenant: "default",
		Tenants: map[string]limiters.TenantLimit{
			"default": {Limit: 10, Window: time.Second},
		},
	}

	limiter, err := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()

	// First assign a key to create stats
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	limiter.AssignKeyToTenant(context.Background(), "test_key", "default")

	stats := limiter.GetTenantStats(context.Background(), "default")
	if stats == nil {
		t.Error("expected non-nil stats")
	}
}

// TestTenantLimiter_Observability_MultiTenant tests multi-tenant observability scenario
func TestTenantLimiter_Observability_MultiTenant(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBackend := mocks.NewMockBackend(ctrl)
	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			DefaultLimit:  10,
			DefaultWindow: time.Second,
			Observability: limiters.ObservabilityConfig{
				Logger:  mockLogger,
				Metrics: mockMetrics,
				Tracer:  mockTracer,
			},
		},
		DefaultTenant: "default",
		Tenants: map[string]limiters.TenantLimit{
			"default": {Limit: 10, Window: time.Second},
			"vip":     {Limit: 1000, Window: time.Second},
		},
	}

	limiter, err := NewTenantBasedLimiter(mockAlgo, mockBackend, cfg)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	gomock.InOrder(
		mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.tenant.check").Return(&obstypes.SpanContext{}),
		mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes(),
		mockTracer.EXPECT().EndSpan(gomock.Any()).Times(1),
	)

	mockBackend.EXPECT().
		Get(gomock.Any(), "default_key").
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
		Set(gomock.Any(), "default_key", gomock.Any(), time.Second).
		Return(nil).
		Times(1)

	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordDecisionLatency(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	result := limiter.Allow(context.Background(), "default_key")
	if result == nil || !result.Allowed {
		t.Error("expected allowed result for default tenant")
	}
}

// TestTenantBasedLimiter_WithDynamicLimits_Enabled tests limiter with dynamic limits enabled
func TestTenantBasedLimiter_WithDynamicLimits_Enabled(t *testing.T) {
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

	limiter, err := NewTenantBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TenantConfig{
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
			DefaultTenant: "default",
			Tenants: map[string]limiters.TenantLimit{
				"default":  {Limit: 100, Window: time.Second},
				"tenant_a": {Limit: 100, Window: time.Second},
				"tenant_b": {Limit: 500, Window: time.Second},
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

// TestTenantBasedLimiter_WithDynamicLimits_UpdateLimitByTenant tests updating tenant limits
func TestTenantBasedLimiter_WithDynamicLimits_UpdateLimitByTenant(t *testing.T) {
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

	limiter, err := NewTenantBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TenantConfig{
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
			DefaultTenant: "default",
			Tenants: map[string]limiters.TenantLimit{
				"default":  {Limit: 100, Window: time.Second},
				"tenant_a": {Limit: 100, Window: time.Second},
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Update tenant limit
	err = dynamicMgr.UpdateLimitByTenant(ctx, "tenant_a", 1000, time.Second)
	require.NoError(t, err)

	// Verify the update worked
	result := limiter.Allow(ctx, "tenant_a_user")
	assert.True(t, result.Allowed)
}

// TestTenantBasedLimiter_DynamicLimits_MultipleTenants tests multiple tenants
func TestTenantBasedLimiter_DynamicLimits_MultipleTenants(t *testing.T) {
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

	limiter, err := NewTenantBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TenantConfig{
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
			DefaultTenant: "default",
			Tenants: map[string]limiters.TenantLimit{
				"default":    {Limit: 100, Window: time.Second},
				"acme_corp":  {Limit: 200, Window: time.Second},
				"techstart":  {Limit: 500, Window: time.Second},
				"enterprise": {Limit: 2000, Window: time.Second},
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Update different tenants
	_ = dynamicMgr.UpdateLimitByTenant(ctx, "acme_corp", 300, time.Second)
	_ = dynamicMgr.UpdateLimitByTenant(ctx, "techstart", 800, time.Second)
	_ = dynamicMgr.UpdateLimitByTenant(ctx, "enterprise", 5000, time.Second)

	// Test limiter with different tenants
	result := limiter.Allow(ctx, "acme_user")
	assert.True(t, result.Allowed)

	result = limiter.Allow(ctx, "tech_user")
	assert.True(t, result.Allowed)

	result = limiter.Allow(ctx, "enterprise_user")
	assert.True(t, result.Allowed)
}

// TestTenantBasedLimiter_DynamicLimits_ConcurrentUpdates tests concurrent operations
func TestTenantBasedLimiter_DynamicLimits_ConcurrentUpdates(t *testing.T) {
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

	limiter, err := NewTenantBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TenantConfig{
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
			DefaultTenant: "default",
			Tenants: map[string]limiters.TenantLimit{
				"default": {Limit: 100, Window: time.Second},
				"tenant1": {Limit: 100, Window: time.Second},
				"tenant2": {Limit: 200, Window: time.Second},
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

	// Concurrent tenant updates
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			tenant := "tenant1"
			if id%2 == 0 {
				tenant = "tenant2"
			}
			_ = dynamicMgr.UpdateLimitByTenant(ctx, tenant, 100+id*10, time.Second)
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

// TestTenantBasedLimiter_DynamicLimits_Disabled tests with dynamic limits disabled
func TestTenantBasedLimiter_DynamicLimits_Disabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	mockBackend.EXPECT().Close().Return(nil)
	mockBackend.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockBackend.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAlgo.EXPECT().Allow(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &algorithm.TokenBucketResult{Allowed: true}, nil).AnyTimes()
	mockAlgo.EXPECT().Reset(gomock.Any()).Return(nil, nil).AnyTimes()

	limiter, err := NewTenantBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TenantConfig{
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
			DefaultTenant: "default",
			Tenants: map[string]limiters.TenantLimit{
				"default": {Limit: 100, Window: time.Second},
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

// TestTenantBasedLimiter_DynamicLimits_ValidationErrors tests validation
func TestTenantBasedLimiter_DynamicLimits_ValidationErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)

	limiter, err := NewTenantBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TenantConfig{
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
			DefaultTenant: "default",
			Tenants: map[string]limiters.TenantLimit{
				"default": {Limit: 100, Window: time.Second},
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Test invalid tenant updates
	err = dynamicMgr.UpdateLimitByTenant(ctx, "", 100, time.Second)
	assert.Error(t, err, "empty tenant should error")

	err = dynamicMgr.UpdateLimitByTenant(ctx, "tenant", -1, time.Second)
	assert.Error(t, err, "negative limit should error")

	err = dynamicMgr.UpdateLimitByTenant(ctx, "tenant", 100, 0)
	assert.Error(t, err, "zero window should error")
}

// TestTenantBasedLimiter_DynamicLimits_UpdateHooks tests update hooks
func TestTenantBasedLimiter_DynamicLimits_UpdateHooks(t *testing.T) {
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

	limiter, err := NewTenantBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TenantConfig{
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
			DefaultTenant: "default",
			Tenants: map[string]limiters.TenantLimit{
				"default":  {Limit: 100, Window: time.Second},
				"tenant_a": {Limit: 100, Window: time.Second},
				"tenant_b": {Limit: 200, Window: time.Second},
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Update tenants - should trigger hooks
	_ = dynamicMgr.UpdateLimitByTenant(ctx, "tenant_a", 150, time.Second)
	_ = dynamicMgr.UpdateLimitByTenant(ctx, "tenant_b", 300, time.Second)

	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, hookCalls, 2)
}

// TestTenantBasedLimiter_DynamicLimits_FallbackToDefault tests default fallback
func TestTenantBasedLimiter_DynamicLimits_FallbackToDefault(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  250,
		DefaultWindow: 2 * time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)

	limiter, err := NewTenantBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TenantConfig{
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
			DefaultTenant: "default",
			Tenants: map[string]limiters.TenantLimit{
				"default": {Limit: 100, Window: time.Second},
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Unconfigured tenant should use default
	limit, window, err := dynamicMgr.GetCurrentLimit(ctx, "unconfigured_tenant")
	require.NoError(t, err)
	assert.Equal(t, 250, limit)
	assert.Equal(t, 2*time.Second, window)
}

// TestTenantBasedLimiter_DynamicLimits_GetAllLimits tests retrieving all limits
func TestTenantBasedLimiter_DynamicLimits_GetAllLimits(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAlgo := mocks.NewMockAlgorithm(ctrl)
	mockBackend := mocks.NewMockBackend(ctrl)

	dynamicMgr := features.NewDynamicLimitManager(features.DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Second,
	})

	mockBackend.EXPECT().Close().Return(nil)

	limiter, err := NewTenantBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TenantConfig{
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
			DefaultTenant: "default",
			Tenants: map[string]limiters.TenantLimit{
				"default": {Limit: 100, Window: time.Second},
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

	// Add per-key limits (not tenant limits)
	_ = dynamicMgr.UpdateLimit(ctx, "user:acme:123", 200, time.Second)
	_ = dynamicMgr.UpdateLimit(ctx, "user:techstart:456", 500, time.Second)
	_ = dynamicMgr.UpdateLimit(ctx, "user:enterprise:789", 2000, time.Second)

	// Should return all
	allLimits = dynamicMgr.GetAllLimits()
	assert.Len(t, allLimits, 3)
}

// TestTenantBasedLimiter_DynamicLimits_AllowN tests AllowN with dynamic limits
func TestTenantBasedLimiter_DynamicLimits_AllowN(t *testing.T) {
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

	limiter, err := NewTenantBasedLimiter(
		mockAlgo,
		mockBackend,
		limiters.TenantConfig{
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
			DefaultTenant: "default",
			Tenants: map[string]limiters.TenantLimit{
				"default": {Limit: 100, Window: time.Second},
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Update tenant limit
	_ = dynamicMgr.UpdateLimitByTenant(ctx, "batch_tenant", 500, time.Second)

	// Test AllowN
	result := limiter.AllowN(ctx, "batch_user", 10)
	assert.True(t, result.Allowed)
}

// mockBackendWithFailure simulates backend failures for tenant limiter
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

// mockTenantAlgorithm for tenant limiter tests
type mockTenantAlgorithm struct{}

func (m *mockTenantAlgorithm) Name() string        { return "mock" }
func (m *mockTenantAlgorithm) Description() string { return "mock algorithm" }
func (m *mockTenantAlgorithm) Reset(ctx context.Context) (interface{}, error) {
	return map[string]interface{}{"count": 0, "reset_at": time.Now()}, nil
}
func (m *mockTenantAlgorithm) Allow(ctx context.Context, state interface{}, cost int) (interface{}, interface{}, error) {
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
func (m *mockTenantAlgorithm) ValidateConfig(config interface{}) error { return nil }
func (m *mockTenantAlgorithm) GetStats(ctx context.Context, state interface{}) interface{} {
	return state
}

// TestTenantLimiterWithoutFailover validates baseline without failover
func TestTenantLimiterWithoutFailover(t *testing.T) {
	primaryBE := newMockBackendWithFailure()

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			AlgorithmName: "fixed_window",
			DefaultLimit:  5,
			DefaultWindow: time.Second,
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant1": {Limit: 10, Window: time.Second},
			"tenant2": {Limit: 20, Window: time.Second},
		},
		DefaultTenant: "tenant1",
	}

	limiter, err := NewTenantBasedLimiter(&mockTenantAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	// Assign key to tenant
	err = limiter.AssignKeyToTenant(ctx, "test-key", "tenant1")
	if err != nil {
		t.Fatalf("Failed to assign key to tenant: %v", err)
	}

	result := limiter.Allow(ctx, "test-key")
	if !result.Allowed {
		t.Error("Request should be allowed without failover")
	}

	t.Log("Tenant limiter without failover test passed")
}

// TestTenantLimiterWithFailoverNoFailure validates failover enabled but no failures
func TestTenantLimiterWithFailoverNoFailure(t *testing.T) {
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

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			AlgorithmName: "fixed_window",
			DefaultLimit:  5,
			DefaultWindow: time.Second,
			Failover: limiters.FailoverConfig{
				EnableFailover: true,
				Handler:        failoverHandler,
			},
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant1": {Limit: 10, Window: time.Second},
		},
		DefaultTenant: "tenant1",
	}

	limiter, err := NewTenantBasedLimiter(&mockTenantAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	err = limiter.AssignKeyToTenant(ctx, "test-key", "tenant1")
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

	t.Log("Tenant limiter with failover (no failures) test passed")
}

// TestTenantLimiterFailoverOnGetFailure validates failover triggers on Get failures
func TestTenantLimiterFailoverOnGetFailure(t *testing.T) {
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

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			AlgorithmName: "fixed_window",
			DefaultLimit:  5,
			DefaultWindow: time.Second,
			Failover: limiters.FailoverConfig{
				EnableFailover: true,
				Handler:        failoverHandler,
			},
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant1": {Limit: 10, Window: time.Second},
		},
		DefaultTenant: "tenant1",
	}

	limiter, err := NewTenantBasedLimiter(&mockTenantAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	err = limiter.AssignKeyToTenant(ctx, "test-key", "tenant1")
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

	t.Log("Tenant limiter failover on Get failure test passed")
}

// TestTenantLimiterFailoverOnSetFailure validates failover triggers on Set failures
func TestTenantLimiterFailoverOnSetFailure(t *testing.T) {
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

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			AlgorithmName: "fixed_window",
			DefaultLimit:  5,
			DefaultWindow: time.Second,
			Failover: limiters.FailoverConfig{
				EnableFailover: true,
				Handler:        failoverHandler,
			},
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant1": {Limit: 10, Window: time.Second},
		},
		DefaultTenant: "tenant1",
	}

	limiter, err := NewTenantBasedLimiter(&mockTenantAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	err = limiter.AssignKeyToTenant(ctx, "test-key", "tenant1")
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

	t.Log("Tenant limiter failover on Set failure test passed")
}

// TestTenantLimiterMultipleFailuresBeforeFailover validates threshold-based failover
func TestTenantLimiterMultipleFailuresBeforeFailover(t *testing.T) {
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

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			AlgorithmName: "fixed_window",
			DefaultLimit:  5,
			DefaultWindow: time.Second,
			Failover: limiters.FailoverConfig{
				EnableFailover: true,
				Handler:        failoverHandler,
			},
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant1": {Limit: 10, Window: time.Second},
		},
		DefaultTenant: "tenant1",
	}

	limiter, err := NewTenantBasedLimiter(&mockTenantAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	err = limiter.AssignKeyToTenant(ctx, "test-key", "tenant1")
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

	t.Log("Tenant limiter multiple failures before failover test passed")
}

// TestTenantLimiterFailoverMultipleTenants validates failover with multiple tenants
func TestTenantLimiterFailoverMultipleTenants(t *testing.T) {
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

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			AlgorithmName: "fixed_window",
			DefaultLimit:  5,
			DefaultWindow: time.Second,
			Failover: limiters.FailoverConfig{
				EnableFailover: true,
				Handler:        failoverHandler,
			},
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant1": {Limit: 10, Window: time.Second},
			"tenant2": {Limit: 20, Window: time.Second},
			"tenant3": {Limit: 30, Window: time.Second},
		},
		DefaultTenant: "tenant1",
	}

	limiter, err := NewTenantBasedLimiter(&mockTenantAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	// Test multiple tenants
	tenants := []string{"tenant1", "tenant2", "tenant3"}
	for i, tenant := range tenants {
		key := "key-" + tenant
		err = limiter.AssignKeyToTenant(ctx, key, tenant)
		if err != nil {
			t.Fatalf("Failed to assign key to %s: %v", tenant, err)
		}

		result := limiter.Allow(ctx, key)
		t.Logf("Tenant %s, request %d: allowed=%v", tenant, i+1, result.Allowed)
	}

	status := failoverHandler.GetFallbackStatus(ctx)
	t.Logf("Failover status: active=%v, count=%d", status.IsFallbackActive, status.FailureCount)

	t.Log("Tenant limiter failover with multiple tenants test passed")
}

// TestTenantLimiterConcurrentFailover validates failover under concurrent load
func TestTenantLimiterConcurrentFailover(t *testing.T) {
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

	cfg := limiters.TenantConfig{
		BasicConfig: limiters.BasicConfig{
			AlgorithmName: "fixed_window",
			DefaultLimit:  100,
			DefaultWindow: time.Second,
			Failover: limiters.FailoverConfig{
				EnableFailover: true,
				Handler:        failoverHandler,
			},
		},
		Tenants: map[string]limiters.TenantLimit{
			"tenant1": {Limit: 100, Window: time.Second},
		},
		DefaultTenant: "tenant1",
	}

	limiter, err := NewTenantBasedLimiter(&mockTenantAlgorithm{}, primaryBE, cfg)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ctx := context.Background()

	err = limiter.AssignKeyToTenant(ctx, "test-key", "tenant1")
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

	t.Log("Tenant limiter concurrent failover test passed")
}
