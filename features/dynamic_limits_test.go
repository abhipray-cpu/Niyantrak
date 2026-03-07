package features

import (
	"context"
	"testing"
	"time"

	"github.com/abhipray-cpu/niyantrak/mocks"
	obstypes "github.com/abhipray-cpu/niyantrak/observability/types"
	"github.com/golang/mock/gomock"
)

// ============================================================================
// CORE MANAGER TESTS
// ============================================================================

// TestDynamicLimitManager_New_NoOp tests manager creation with NoOp observability
func TestDynamicLimitManager_New_NoOp(t *testing.T) {
	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
	})

	if manager == nil {
		t.Fatal("manager should not be nil")
	}
	if manager.limits == nil {
		t.Fatal("limits map should be initialized")
	}
	if manager.tierLimits == nil {
		t.Fatal("tierLimits map should be initialized")
	}
	if manager.tenantLimits == nil {
		t.Fatal("tenantLimits map should be initialized")
	}
	if manager.defaultLimit.Limit != 100 {
		t.Errorf("expected default limit 100, got %d", manager.defaultLimit.Limit)
	}
	if manager.defaultLimit.Window != time.Minute {
		t.Errorf("expected default window 1m, got %v", manager.defaultLimit.Window)
	}
}

// ============================================================================
// UPDATE LIMIT TESTS
// ============================================================================

// TestDynamicLimitManager_UpdateLimit_Success tests successful limit update
func TestDynamicLimitManager_UpdateLimit_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).Times(2)
	mockTracer.EXPECT().EndSpan(gomock.Any()).Times(2)
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info("limit updated", gomock.Any()).Times(1)
	mockMetrics.EXPECT().RecordRequest("dynamic_limit_updates", true, int64(1)).Times(1)
	mockLogger.EXPECT().Debug("retrieved dynamic limit", gomock.Any()).Times(1)

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	err := manager.UpdateLimit(ctx, "user123", 200, 2*time.Minute)
	if err != nil {
		t.Fatalf("UpdateLimit failed: %v", err)
	}

	limit, window, err := manager.GetCurrentLimit(ctx, "user123")
	if err != nil || limit != 200 || window != 2*time.Minute {
		t.Errorf("unexpected values: limit=%d, window=%v, err=%v", limit, window, err)
	}
}

// TestDynamicLimitManager_UpdateLimit_MultipleKeys tests updating multiple keys
func TestDynamicLimitManager_UpdateLimit_MultipleKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	ctx := context.Background()

	// Update multiple keys
	for i := 1; i <= 5; i++ {
		key := "user" + string(rune(48+i))
		limit := 100 + int64(i*50)
		err := manager.UpdateLimit(ctx, key, int(limit), time.Duration(i)*time.Minute)
		if err != nil {
			t.Fatalf("UpdateLimit failed for key %s: %v", key, err)
		}
	}

	// Verify all keys
	allLimits := manager.GetAllLimits()
	if len(allLimits) != 5 {
		t.Errorf("expected 5 limits, got %d", len(allLimits))
	}
}

// TestDynamicLimitManager_UpdateLimit_EmptyKey tests empty key validation
func TestDynamicLimitManager_UpdateLimit_EmptyKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).Times(1)
	mockTracer.EXPECT().EndSpan(gomock.Any()).Times(1)
	mockTracer.EXPECT().SetError(gomock.Any(), "key cannot be empty").Times(1)
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).Times(1)

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	err := manager.UpdateLimit(ctx, "", 200, 2*time.Minute)
	if err == nil {
		t.Fatal("should return error for empty key")
	}
	if err.Error() != "key cannot be empty" {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDynamicLimitManager_UpdateLimit_NegativeLimit tests negative limit validation
func TestDynamicLimitManager_UpdateLimit_NegativeLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).Times(1)
	mockTracer.EXPECT().EndSpan(gomock.Any()).Times(1)
	mockTracer.EXPECT().SetError(gomock.Any(), gomock.Any()).Times(1)
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).Times(1)

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	err := manager.UpdateLimit(ctx, "user123", -10, 2*time.Minute)
	if err == nil {
		t.Fatal("should return error for negative limit")
	}
}

// TestDynamicLimitManager_UpdateLimit_ZeroWindow tests zero window validation
func TestDynamicLimitManager_UpdateLimit_ZeroWindow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).Times(1)
	mockTracer.EXPECT().EndSpan(gomock.Any()).Times(1)
	mockTracer.EXPECT().SetError(gomock.Any(), gomock.Any()).Times(1)
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).Times(1)

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	err := manager.UpdateLimit(ctx, "user123", 200, 0)
	if err == nil {
		t.Fatal("should return error for zero window")
	}
}

// TestDynamicLimitManager_UpdateLimit_Overwrite tests overwriting existing limit
func TestDynamicLimitManager_UpdateLimit_Overwrite(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	ctx := context.Background()

	// First update
	err := manager.UpdateLimit(ctx, "key1", 100, time.Minute)
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}

	// Overwrite with different values
	err = manager.UpdateLimit(ctx, "key1", 500, 5*time.Minute)
	if err != nil {
		t.Fatalf("overwrite update failed: %v", err)
	}

	limit, window, err := manager.GetCurrentLimit(ctx, "key1")
	if err != nil {
		t.Fatalf("GetCurrentLimit failed: %v", err)
	}
	if limit != 500 || window != 5*time.Minute {
		t.Errorf("expected limit=500, window=5m; got limit=%d, window=%v", limit, window)
	}
}

// ============================================================================
// UPDATE BY TIER TESTS
// ============================================================================

// TestDynamicLimitManager_UpdateLimitByTier_Success tests successful tier limit update
func TestDynamicLimitManager_UpdateLimitByTier_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	err := manager.UpdateLimitByTier(ctx, "premium", 500, 5*time.Minute)
	if err != nil {
		t.Fatalf("UpdateLimitByTier failed: %v", err)
	}

	config, err := manager.GetTierLimit(ctx, "premium")
	if err != nil || config.Limit != 500 || config.Window != 5*time.Minute {
		t.Errorf("unexpected tier limit: limit=%d, window=%v", config.Limit, config.Window)
	}
}

// TestDynamicLimitManager_UpdateLimitByTier_MultipleTiers tests multiple tier updates
func TestDynamicLimitManager_UpdateLimitByTier_MultipleTiers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	ctx := context.Background()

	tiers := map[string]int{"free": 100, "basic": 500, "premium": 1000, "enterprise": 5000}

	for tier, limit := range tiers {
		err := manager.UpdateLimitByTier(ctx, tier, limit, time.Duration(limit/100)*time.Minute)
		if err != nil {
			t.Fatalf("UpdateLimitByTier failed for tier %s: %v", tier, err)
		}
	}

	// Verify all tiers
	for tier, expectedLimit := range tiers {
		config, err := manager.GetTierLimit(ctx, tier)
		if err != nil || config.Limit != int64(expectedLimit) {
			t.Errorf("tier %s: expected limit %d, got %d", tier, expectedLimit, config.Limit)
		}
	}
}

// TestDynamicLimitManager_UpdateLimitByTier_EmptyTier tests empty tier validation
func TestDynamicLimitManager_UpdateLimitByTier_EmptyTier(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).Times(1)
	mockTracer.EXPECT().EndSpan(gomock.Any()).Times(1)
	mockTracer.EXPECT().SetError(gomock.Any(), "tier cannot be empty").Times(1)
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).Times(1)

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	err := manager.UpdateLimitByTier(ctx, "", 500, 5*time.Minute)
	if err == nil {
		t.Fatal("should return error for empty tier")
	}
}

// ============================================================================
// UPDATE BY TENANT TESTS
// ============================================================================

// TestDynamicLimitManager_UpdateLimitByTenant_Success tests successful tenant limit update
func TestDynamicLimitManager_UpdateLimitByTenant_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	err := manager.UpdateLimitByTenant(ctx, "tenant-abc", 1000, 10*time.Minute)
	if err != nil {
		t.Fatalf("UpdateLimitByTenant failed: %v", err)
	}

	config, err := manager.GetTenantLimit(ctx, "tenant-abc")
	if err != nil || config.Limit != 1000 || config.Window != 10*time.Minute {
		t.Errorf("unexpected tenant limit: limit=%d, window=%v", config.Limit, config.Window)
	}
}

// TestDynamicLimitManager_UpdateLimitByTenant_MultipleTenants tests multiple tenant updates
func TestDynamicLimitManager_UpdateLimitByTenant_MultipleTenants(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	ctx := context.Background()

	// Update multiple tenants
	for i := 1; i <= 5; i++ {
		tenantID := "tenant-" + string(rune(96+i))
		limit := 100 * int64(i)
		err := manager.UpdateLimitByTenant(ctx, tenantID, int(limit), time.Duration(i)*time.Minute)
		if err != nil {
			t.Fatalf("UpdateLimitByTenant failed for tenant %s: %v", tenantID, err)
		}
	}
}

// TestDynamicLimitManager_UpdateLimitByTenant_EmptyTenant tests empty tenant validation
func TestDynamicLimitManager_UpdateLimitByTenant_EmptyTenant(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).Times(1)
	mockTracer.EXPECT().EndSpan(gomock.Any()).Times(1)
	mockTracer.EXPECT().SetError(gomock.Any(), "tenant cannot be empty").Times(1)
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).Times(1)

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	err := manager.UpdateLimitByTenant(ctx, "", 1000, 10*time.Minute)
	if err == nil {
		t.Fatal("should return error for empty tenant")
	}
}

// ============================================================================
// GET CURRENT LIMIT TESTS
// ============================================================================

// TestDynamicLimitManager_GetCurrentLimit_Default tests default limit fallback
func TestDynamicLimitManager_GetCurrentLimit_Default(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).Times(1)
	mockTracer.EXPECT().EndSpan(gomock.Any()).Times(1)
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug("using default limit", gomock.Any()).Times(1)

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	limit, window, err := manager.GetCurrentLimit(ctx, "unknown-key")
	if err != nil || limit != 100 || window != time.Minute {
		t.Errorf("unexpected default limit: limit=%d, window=%v, err=%v", limit, window, err)
	}
}

// TestDynamicLimitManager_GetCurrentLimit_Configured tests retrieving configured limit
func TestDynamicLimitManager_GetCurrentLimit_Configured(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug("retrieved dynamic limit", gomock.Any()).Times(1)
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	manager.UpdateLimit(ctx, "key1", 500, 5*time.Minute)

	limit, window, err := manager.GetCurrentLimit(ctx, "key1")
	if err != nil || limit != 500 || window != 5*time.Minute {
		t.Errorf("unexpected limit: limit=%d, window=%v, err=%v", limit, window, err)
	}
}

// ============================================================================
// UPDATE DEFAULT LIMIT TESTS
// ============================================================================

// TestDynamicLimitManager_UpdateDefaultLimit_Success tests default limit update
func TestDynamicLimitManager_UpdateDefaultLimit_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).Times(1)
	mockTracer.EXPECT().EndSpan(gomock.Any()).Times(1)
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info("default limit updated", gomock.Any()).Times(1)
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	err := manager.UpdateDefaultLimit(ctx, 250, 3*time.Minute)
	if err != nil {
		t.Fatalf("UpdateDefaultLimit failed: %v", err)
	}

	defaultConfig := manager.GetDefaultLimit()
	if defaultConfig.Limit != 250 || defaultConfig.Window != 3*time.Minute {
		t.Errorf("unexpected default limit: limit=%d, window=%v", defaultConfig.Limit, defaultConfig.Window)
	}
}

// TestDynamicLimitManager_UpdateDefaultLimit_NegativeLimit tests validation
func TestDynamicLimitManager_UpdateDefaultLimit_NegativeLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).Times(1)
	mockTracer.EXPECT().EndSpan(gomock.Any()).Times(1)
	mockTracer.EXPECT().SetError(gomock.Any(), gomock.Any()).Times(1)
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).Times(1)

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	err := manager.UpdateDefaultLimit(ctx, -50, 2*time.Minute)
	if err == nil {
		t.Fatal("should return error for negative default limit")
	}
}

// ============================================================================
// ADD UPDATE HOOK TESTS
// ============================================================================

// TestDynamicLimitManager_AddUpdateHook_SingleHook tests single hook
func TestDynamicLimitManager_AddUpdateHook_SingleHook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	hookCalled := false
	hook := func(key string, config *LimitConfig) {
		hookCalled = true
	}

	manager.AddUpdateHook(hook)

	ctx := context.Background()
	manager.UpdateLimit(ctx, "test-key", 300, 5*time.Minute)

	if !hookCalled {
		t.Error("update hook should have been called")
	}
}

// TestDynamicLimitManager_AddUpdateHook_MultipleHooks tests multiple hooks
func TestDynamicLimitManager_AddUpdateHook_MultipleHooks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	hook1Called := false
	hook2Called := false
	hook3Called := false

	manager.AddUpdateHook(func(key string, config *LimitConfig) {
		hook1Called = true
	})
	manager.AddUpdateHook(func(key string, config *LimitConfig) {
		hook2Called = true
	})
	manager.AddUpdateHook(func(key string, config *LimitConfig) {
		hook3Called = true
	})

	ctx := context.Background()
	manager.UpdateLimit(ctx, "test-key", 300, 5*time.Minute)

	if !hook1Called || !hook2Called || !hook3Called {
		t.Error("all hooks should have been called")
	}
}

// ============================================================================
// GET ALL LIMITS TESTS
// ============================================================================

// TestDynamicLimitManager_GetAllLimits_Empty tests empty limits
func TestDynamicLimitManager_GetAllLimits_Empty(t *testing.T) {
	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
	})

	allLimits := manager.GetAllLimits()
	if len(allLimits) != 0 {
		t.Errorf("expected 0 limits, got %d", len(allLimits))
	}
}

// TestDynamicLimitManager_GetAllLimits_Multiple tests multiple limits
func TestDynamicLimitManager_GetAllLimits_Multiple(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	ctx := context.Background()

	// Add multiple limits
	for i := 1; i <= 10; i++ {
		key := "key" + string(rune(48+i%10))
		manager.UpdateLimit(ctx, key, 100*i, time.Duration(i)*time.Minute)
	}

	allLimits := manager.GetAllLimits()
	if len(allLimits) < 1 {
		t.Errorf("expected at least 1 limit, got %d", len(allLimits))
	}
}

// ============================================================================
// RELOAD CONFIG TESTS
// ============================================================================

// TestDynamicLimitManager_ReloadConfig tests config reload
func TestDynamicLimitManager_ReloadConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockTracer.EXPECT().StartSpan(gomock.Any(), "rate_limit.dynamic.reload_config").Return(&obstypes.SpanContext{}).Times(1)
	mockTracer.EXPECT().EndSpan(gomock.Any()).Times(1)
	mockLogger.EXPECT().Info("config reload requested - no-op for now", gomock.Any()).Times(1)
	mockMetrics.EXPECT().RecordRequest("dynamic_config_reloads", true, int64(1)).Times(1)

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	err := manager.ReloadConfig(ctx)
	if err != nil {
		t.Fatalf("ReloadConfig failed: %v", err)
	}
}

// ============================================================================
// CONCURRENCY TESTS
// ============================================================================

// TestDynamicLimitManager_ConcurrentUpdates tests concurrent updates
func TestDynamicLimitManager_ConcurrentUpdates(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	done := make(chan bool, 10)

	// Launch 10 goroutines to update limits concurrently
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				key := "key" + string(rune(48+id))
				limit := 100 + j
				manager.UpdateLimit(ctx, key, limit, time.Duration(j+1)*time.Minute)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify results
	allLimits := manager.GetAllLimits()
	if len(allLimits) == 0 {
		t.Error("should have at least some limits after concurrent updates")
	}
}

// ============================================================================
// EDGE CASE TESTS
// ============================================================================

// TestDynamicLimitManager_LargeLimitValue tests large limit values
func TestDynamicLimitManager_LargeLimitValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	largeLimit := int(1e9)
	err := manager.UpdateLimit(ctx, "large-key", largeLimit, 1*time.Hour)
	if err != nil {
		t.Fatalf("UpdateLimit failed for large value: %v", err)
	}

	limit, _, err := manager.GetCurrentLimit(ctx, "large-key")
	if err != nil || limit != largeLimit {
		t.Errorf("unexpected large limit: expected %d, got %d", largeLimit, limit)
	}
}

// TestDynamicLimitManager_MinimumLimitValue tests minimum valid limit (1)
func TestDynamicLimitManager_MinimumLimitValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	err := manager.UpdateLimit(ctx, "min-key", 1, 1*time.Second)
	if err != nil {
		t.Fatalf("UpdateLimit failed for minimum value: %v", err)
	}

	limit, _, err := manager.GetCurrentLimit(ctx, "min-key")
	if err != nil || limit != 1 {
		t.Errorf("unexpected minimum limit: expected 1, got %d", limit)
	}
}

// TestDynamicLimitManager_SmallWindow tests very small time windows
func TestDynamicLimitManager_SmallWindow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	err := manager.UpdateLimit(ctx, "small-window", 100, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("UpdateLimit failed for small window: %v", err)
	}

	_, window, err := manager.GetCurrentLimit(ctx, "small-window")
	if err != nil || window != 1*time.Millisecond {
		t.Errorf("unexpected window: expected 1ms, got %v", window)
	}
}

// TestDynamicLimitManager_LargeWindow tests very large time windows
func TestDynamicLimitManager_LargeWindow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockMetrics := mocks.NewMockMetrics(ctrl)
	mockTracer := mocks.NewMockTracer(ctrl)

	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().RecordRequest(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).Return(&obstypes.SpanContext{}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any()).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	manager := NewDynamicLimitManager(DynamicLimitManagerConfig{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Logger:        mockLogger,
		Metrics:       mockMetrics,
		Tracer:        mockTracer,
	})

	ctx := context.Background()
	largeWindow := 365 * 24 * time.Hour
	err := manager.UpdateLimit(ctx, "large-window", 100, largeWindow)
	if err != nil {
		t.Fatalf("UpdateLimit failed for large window: %v", err)
	}

	_, window, err := manager.GetCurrentLimit(ctx, "large-window")
	if err != nil || window != largeWindow {
		t.Errorf("unexpected window: expected %v, got %v", largeWindow, window)
	}
}
