package features

import (
	"context"
	"fmt"
	"sync"
	"time"

	obstypes "github.com/abhipray-cpu/niyantrak/observability/types"
)

// DynamicLimitController allows runtime adjustment of rate limits
type DynamicLimitController interface {
	// UpdateLimit changes the limit for a specific key at runtime
	// Returns error if the update fails
	UpdateLimit(ctx context.Context, key string, newLimit int, window time.Duration) error

	// UpdateLimitByTier changes limits for all keys in a specific tier
	// Useful for subscription tier changes
	UpdateLimitByTier(ctx context.Context, tier string, newLimit int, window time.Duration) error

	// UpdateLimitByTenant changes limits for all keys under a tenant
	// Useful for multi-tenant applications
	UpdateLimitByTenant(ctx context.Context, tenantID string, newLimit int, window time.Duration) error

	// GetCurrentLimit retrieves the current limit for a key
	GetCurrentLimit(ctx context.Context, key string) (int, time.Duration, error)

	// ReloadConfig reloads configuration from external source (file, database, etc.)
	ReloadConfig(ctx context.Context) error
}

// DynamicLimitConfig holds configuration for dynamic limits
type DynamicLimitConfig struct {
	// ReloadInterval is how often to check for configuration changes
	ReloadInterval time.Duration

	// ConfigSource is the source of dynamic configuration (file, database, etc.)
	ConfigSource string

	// AllowOnlineUpdates indicates if limits can be updated without reload
	AllowOnlineUpdates bool

	// GracefulSwitching if true, gradually switch to new limits
	GracefulSwitching bool

	// SwitchingPeriod is the duration over which to switch limits
	SwitchingPeriod time.Duration
}

// LimitConfig represents a dynamic limit configuration
type LimitConfig struct {
	Limit  int64
	Window time.Duration
}

// DynamicLimitManager implements DynamicLimitController
type DynamicLimitManager struct {
	mu sync.RWMutex

	// limits stores per-key limit configurations
	limits map[string]*LimitConfig

	// tierLimits stores per-tier limit configurations (for TierBasedLimiter)
	tierLimits map[string]*LimitConfig

	// tenantLimits stores per-tenant limit configurations (for TenantBasedLimiter)
	tenantLimits map[string]*LimitConfig

	// defaultLimit is the fallback limit if no specific limit is configured
	defaultLimit *LimitConfig

	// updateHooks are callbacks triggered when limits are updated
	updateHooks []func(key string, config *LimitConfig)

	// observability
	logger  obstypes.Logger
	metrics obstypes.Metrics
	tracer  obstypes.Tracer
}

// DynamicLimitManagerConfig configures the DynamicLimitManager
type DynamicLimitManagerConfig struct {
	DefaultLimit  int64
	DefaultWindow time.Duration
	Logger        obstypes.Logger
	Metrics       obstypes.Metrics
	Tracer        obstypes.Tracer
}

// NewDynamicLimitManager creates a new DynamicLimitManager
func NewDynamicLimitManager(cfg DynamicLimitManagerConfig) *DynamicLimitManager {
	// Set defaults for observability
	logger := cfg.Logger
	if logger == nil {
		logger = &obstypes.NoOpLogger{}
	}

	metricsProvider := cfg.Metrics
	if metricsProvider == nil {
		metricsProvider = &obstypes.NoOpMetrics{}
	}

	tracer := cfg.Tracer
	if tracer == nil {
		tracer = &obstypes.NoOpTracer{}
	}

	return &DynamicLimitManager{
		limits:       make(map[string]*LimitConfig),
		tierLimits:   make(map[string]*LimitConfig),
		tenantLimits: make(map[string]*LimitConfig),
		defaultLimit: &LimitConfig{
			Limit:  cfg.DefaultLimit,
			Window: cfg.DefaultWindow,
		},
		updateHooks: make([]func(string, *LimitConfig), 0),
		logger:      logger,
		metrics:     metricsProvider,
		tracer:      tracer,
	}
}

// UpdateLimit updates the limit for a specific key
func (m *DynamicLimitManager) UpdateLimit(ctx context.Context, key string, newLimit int, window time.Duration) error {
	span := m.tracer.StartSpan(ctx, "rate_limit.dynamic.update_limit")
	defer m.tracer.EndSpan(span)

	// Validate inputs
	if key == "" {
		err := fmt.Errorf("key cannot be empty")
		m.tracer.SetError(span, err.Error())
		m.logger.Error("failed to update limit", "error", err)
		return err
	}

	if newLimit <= 0 {
		err := fmt.Errorf("limit must be positive, got %d", newLimit)
		m.tracer.SetError(span, err.Error())
		m.logger.Error("failed to update limit", "error", err, "key", key, "limit", newLimit)
		return err
	}

	if window <= 0 {
		err := fmt.Errorf("window must be positive, got %v", window)
		m.tracer.SetError(span, err.Error())
		m.logger.Error("failed to update limit", "error", err, "key", key, "window", window)
		return err
	}

	// Update the limit
	m.mu.Lock()
	config := &LimitConfig{
		Limit:  int64(newLimit),
		Window: window,
	}
	m.limits[key] = config
	hooks := make([]func(string, *LimitConfig), len(m.updateHooks))
	copy(hooks, m.updateHooks)
	m.mu.Unlock()

	// Add span attributes
	m.tracer.AddAttribute(span, "key", key)
	m.tracer.AddAttribute(span, "limit", int64(newLimit))
	m.tracer.AddAttribute(span, "window_seconds", window.Seconds())

	// Log the update
	m.logger.Info("limit updated", "key", key, "limit", newLimit, "window", window)

	// Record metrics
	m.metrics.RecordRequest("dynamic_limit_updates", true, 1)

	// Trigger update hooks
	for _, hook := range hooks {
		hook(key, config)
	}

	return nil
}

// UpdateLimitByTier updates the limit for a specific tier
func (m *DynamicLimitManager) UpdateLimitByTier(ctx context.Context, tier string, newLimit int, window time.Duration) error {
	span := m.tracer.StartSpan(ctx, "rate_limit.dynamic.update_tier_limit")
	defer m.tracer.EndSpan(span)

	// Validate inputs
	if tier == "" {
		err := fmt.Errorf("tier cannot be empty")
		m.tracer.SetError(span, err.Error())
		m.logger.Error("failed to update tier limit", "error", err)
		return err
	}

	if newLimit <= 0 {
		err := fmt.Errorf("limit must be positive, got %d", newLimit)
		m.tracer.SetError(span, err.Error())
		m.logger.Error("failed to update tier limit", "error", err, "tier", tier, "limit", newLimit)
		return err
	}

	if window <= 0 {
		err := fmt.Errorf("window must be positive, got %v", window)
		m.tracer.SetError(span, err.Error())
		m.logger.Error("failed to update tier limit", "error", err, "tier", tier, "window", window)
		return err
	}

	// Update the tier limit
	m.mu.Lock()
	config := &LimitConfig{
		Limit:  int64(newLimit),
		Window: window,
	}
	m.tierLimits[tier] = config
	hooks := make([]func(string, *LimitConfig), len(m.updateHooks))
	copy(hooks, m.updateHooks)
	m.mu.Unlock()

	// Add span attributes
	m.tracer.AddAttribute(span, "tier", tier)
	m.tracer.AddAttribute(span, "limit", int64(newLimit))
	m.tracer.AddAttribute(span, "window_seconds", window.Seconds())

	// Log the update
	m.logger.Info("tier limit updated", "tier", tier, "limit", newLimit, "window", window)

	// Record metrics
	m.metrics.RecordRequest("dynamic_tier_limit_updates", true, 1)

	// Trigger update hooks
	for _, hook := range hooks {
		hook("tier:"+tier, config)
	}

	return nil
}

// UpdateLimitByTenant updates the limit for a specific tenant
func (m *DynamicLimitManager) UpdateLimitByTenant(ctx context.Context, tenantID string, newLimit int, window time.Duration) error {
	span := m.tracer.StartSpan(ctx, "rate_limit.dynamic.update_tenant_limit")
	defer m.tracer.EndSpan(span)

	// Validate inputs
	if tenantID == "" {
		err := fmt.Errorf("tenant cannot be empty")
		m.tracer.SetError(span, err.Error())
		m.logger.Error("failed to update tenant limit", "error", err)
		return err
	}

	if newLimit <= 0 {
		err := fmt.Errorf("limit must be positive, got %d", newLimit)
		m.tracer.SetError(span, err.Error())
		m.logger.Error("failed to update tenant limit", "error", err, "tenant", tenantID, "limit", newLimit)
		return err
	}

	if window <= 0 {
		err := fmt.Errorf("window must be positive, got %v", window)
		m.tracer.SetError(span, err.Error())
		m.logger.Error("failed to update tenant limit", "error", err, "tenant", tenantID, "window", window)
		return err
	}

	// Update the tenant limit
	m.mu.Lock()
	config := &LimitConfig{
		Limit:  int64(newLimit),
		Window: window,
	}
	m.tenantLimits[tenantID] = config
	hooks := make([]func(string, *LimitConfig), len(m.updateHooks))
	copy(hooks, m.updateHooks)
	m.mu.Unlock()

	// Add span attributes
	m.tracer.AddAttribute(span, "tenant", tenantID)
	m.tracer.AddAttribute(span, "limit", int64(newLimit))
	m.tracer.AddAttribute(span, "window_seconds", window.Seconds())

	// Log the update
	m.logger.Info("tenant limit updated", "tenant", tenantID, "limit", newLimit, "window", window)

	// Record metrics
	m.metrics.RecordRequest("dynamic_tenant_limit_updates", true, 1)

	// Trigger update hooks
	for _, hook := range hooks {
		hook("tenant:"+tenantID, config)
	}

	return nil
}

// GetCurrentLimit retrieves the current limit for a key
func (m *DynamicLimitManager) GetCurrentLimit(ctx context.Context, key string) (int, time.Duration, error) {
	span := m.tracer.StartSpan(ctx, "rate_limit.dynamic.get_limit")
	defer m.tracer.EndSpan(span)

	m.mu.RLock()
	config, exists := m.limits[key]
	defaultConfig := m.defaultLimit
	m.mu.RUnlock()

	m.tracer.AddAttribute(span, "key", key)
	m.tracer.AddAttribute(span, "limit_exists", exists)

	if exists {
		m.logger.Debug("retrieved dynamic limit", "key", key, "limit", config.Limit, "window", config.Window)
		m.tracer.AddAttribute(span, "limit", config.Limit)
		m.tracer.AddAttribute(span, "window_seconds", config.Window.Seconds())
		return int(config.Limit), config.Window, nil
	}

	// Return default limit if no specific limit is configured
	m.logger.Debug("using default limit", "key", key, "limit", defaultConfig.Limit, "window", defaultConfig.Window)
	m.tracer.AddAttribute(span, "limit", defaultConfig.Limit)
	m.tracer.AddAttribute(span, "window_seconds", defaultConfig.Window.Seconds())
	m.tracer.AddAttribute(span, "using_default", true)
	return int(defaultConfig.Limit), defaultConfig.Window, nil
}

// ReloadConfig reloads the entire configuration
func (m *DynamicLimitManager) ReloadConfig(ctx context.Context) error {
	span := m.tracer.StartSpan(ctx, "rate_limit.dynamic.reload_config")
	defer m.tracer.EndSpan(span)

	// For now, this is a no-op. In a real implementation, this would reload from external source
	m.logger.Info("config reload requested - no-op for now")
	m.metrics.RecordRequest("dynamic_config_reloads", true, 1)

	return nil
}

// Helper methods

// GetTierLimit retrieves the current limit for a tier
func (m *DynamicLimitManager) GetTierLimit(ctx context.Context, tier string) (*LimitConfig, error) {
	span := m.tracer.StartSpan(ctx, "rate_limit.dynamic.get_tier_limit")
	defer m.tracer.EndSpan(span)

	m.mu.RLock()
	config, exists := m.tierLimits[tier]
	defaultConfig := m.defaultLimit
	m.mu.RUnlock()

	m.tracer.AddAttribute(span, "tier", tier)
	m.tracer.AddAttribute(span, "limit_exists", exists)

	if exists {
		m.logger.Debug("retrieved dynamic tier limit", "tier", tier, "limit", config.Limit, "window", config.Window)
		m.tracer.AddAttribute(span, "limit", config.Limit)
		m.tracer.AddAttribute(span, "window_seconds", config.Window.Seconds())
		return config, nil
	}

	// Return default limit if no specific limit is configured
	m.logger.Debug("using default tier limit", "tier", tier, "limit", defaultConfig.Limit, "window", defaultConfig.Window)
	m.tracer.AddAttribute(span, "limit", defaultConfig.Limit)
	m.tracer.AddAttribute(span, "window_seconds", defaultConfig.Window.Seconds())
	m.tracer.AddAttribute(span, "using_default", true)
	return defaultConfig, nil
}

// GetTenantLimit retrieves the current limit for a tenant
func (m *DynamicLimitManager) GetTenantLimit(ctx context.Context, tenant string) (*LimitConfig, error) {
	span := m.tracer.StartSpan(ctx, "rate_limit.dynamic.get_tenant_limit")
	defer m.tracer.EndSpan(span)

	m.mu.RLock()
	config, exists := m.tenantLimits[tenant]
	defaultConfig := m.defaultLimit
	m.mu.RUnlock()

	m.tracer.AddAttribute(span, "tenant", tenant)
	m.tracer.AddAttribute(span, "limit_exists", exists)

	if exists {
		m.logger.Debug("retrieved dynamic tenant limit", "tenant", tenant, "limit", config.Limit, "window", config.Window)
		m.tracer.AddAttribute(span, "limit", config.Limit)
		m.tracer.AddAttribute(span, "window_seconds", config.Window.Seconds())
		return config, nil
	}

	// Return default limit if no specific limit is configured
	m.logger.Debug("using default tenant limit", "tenant", tenant, "limit", defaultConfig.Limit, "window", defaultConfig.Window)
	m.tracer.AddAttribute(span, "limit", defaultConfig.Limit)
	m.tracer.AddAttribute(span, "window_seconds", defaultConfig.Window.Seconds())
	m.tracer.AddAttribute(span, "using_default", true)
	return defaultConfig, nil
}

// AddUpdateHook registers a callback to be triggered when limits are updated
func (m *DynamicLimitManager) AddUpdateHook(hook func(key string, config *LimitConfig)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateHooks = append(m.updateHooks, hook)
	m.logger.Debug("update hook added", "total_hooks", len(m.updateHooks))
}

// GetAllLimits returns all configured limits
func (m *DynamicLimitManager) GetAllLimits() map[string]*LimitConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Create a copy to avoid external mutation
	result := make(map[string]*LimitConfig, len(m.limits))
	for key, config := range m.limits {
		result[key] = &LimitConfig{
			Limit:  config.Limit,
			Window: config.Window,
		}
	}
	return result
}

// GetDefaultLimit returns the default limit configuration
func (m *DynamicLimitManager) GetDefaultLimit() *LimitConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return &LimitConfig{
		Limit:  m.defaultLimit.Limit,
		Window: m.defaultLimit.Window,
	}
}

// UpdateDefaultLimit updates the default limit
func (m *DynamicLimitManager) UpdateDefaultLimit(ctx context.Context, limit int64, window time.Duration) error {
	span := m.tracer.StartSpan(ctx, "rate_limit.dynamic.update_default_limit")
	defer m.tracer.EndSpan(span)

	if limit <= 0 {
		err := fmt.Errorf("limit must be positive, got %d", limit)
		m.tracer.SetError(span, err.Error())
		m.logger.Error("failed to update default limit", "error", err, "limit", limit)
		return err
	}

	if window <= 0 {
		err := fmt.Errorf("window must be positive, got %v", window)
		m.tracer.SetError(span, err.Error())
		m.logger.Error("failed to update default limit", "error", err, "window", window)
		return err
	}

	m.mu.Lock()
	m.defaultLimit = &LimitConfig{
		Limit:  limit,
		Window: window,
	}
	m.mu.Unlock()

	m.tracer.AddAttribute(span, "limit", limit)
	m.tracer.AddAttribute(span, "window_seconds", window.Seconds())
	m.logger.Info("default limit updated", "limit", limit, "window", window)
	m.metrics.RecordRequest("dynamic_default_limit_updates", true, 1)

	return nil
}
