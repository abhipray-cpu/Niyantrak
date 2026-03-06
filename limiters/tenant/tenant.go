package tenant

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/abhipray-cpu/niyantrak/algorithm"
	"github.com/abhipray-cpu/niyantrak/backend"
	"github.com/abhipray-cpu/niyantrak/features"
	"github.com/abhipray-cpu/niyantrak/limiters"
	obstypes "github.com/abhipray-cpu/niyantrak/observability/types"
)

// tenantLimiter provides multi-tenancy rate limiting with aggregated statistics
type tenantLimiter struct {
	algorithm      algorithm.Algorithm
	backendBackend backend.Backend
	config         limiters.TenantConfig
	mu             sync.RWMutex
	closed         bool
	tenants        map[string]*tenantConfig // tenant ID -> limit config
	keyTenants     map[string]string        // key -> tenant mapping
	defaultTenant  string
	tenantStats    map[string]*tenantStatsData // tenant ID -> aggregated stats

	// Observability (optional, zero-cost if not configured)
	logger  obstypes.Logger
	metrics obstypes.Metrics
	tracer  obstypes.Tracer

	// Dynamic limits integration (optional, disabled by default)
	dynamics *features.DynamicLimitManager

	// Failover integration (optional, disabled by default)
	failover features.FailoverHandler
}

// tenantConfig represents a tenant's rate limit configuration
type tenantConfig struct {
	limit  int
	window time.Duration
}

// tenantStatsData represents aggregated statistics for a tenant
type tenantStatsData struct {
	tenantID      string
	totalKeys     int
	totalRequests int64
	allowedCount  int64
	deniedCount   int64
	lastUpdated   time.Time
}

// Compile-time check to ensure tenantLimiter implements limiters.Limiter
var _ limiters.Limiter = (*tenantLimiter)(nil)

// Compile-time check to ensure tenantLimiter implements limiters.TenantBasedLimiter
var _ limiters.TenantBasedLimiter = (*tenantLimiter)(nil)

// NewTenantBasedLimiter creates a new tenant-based rate limiter
func NewTenantBasedLimiter(
	algo algorithm.Algorithm,
	backendBackend backend.Backend,
	cfg limiters.TenantConfig,
) (limiters.TenantBasedLimiter, error) {
	if algo == nil {
		return nil, fmt.Errorf("algorithm cannot be nil")
	}
	if backendBackend == nil {
		return nil, fmt.Errorf("backend cannot be nil")
	}
	if cfg.DefaultLimit <= 0 {
		return nil, fmt.Errorf("default limit must be positive")
	}
	if cfg.DefaultWindow <= 0 {
		return nil, fmt.Errorf("default window must be positive")
	}
	if cfg.DefaultTenant == "" {
		return nil, fmt.Errorf("default tenant cannot be empty")
	}
	if len(cfg.Tenants) == 0 {
		return nil, fmt.Errorf("tenants cannot be empty")
	}

	// Validate algorithm configuration if provided
	if cfg.AlgorithmConfig != nil {
		if err := algo.ValidateConfig(cfg.AlgorithmConfig); err != nil {
			return nil, fmt.Errorf("invalid algorithm config: %w", err)
		}
	}

	// Initialize tenants map
	tenants := make(map[string]*tenantConfig)
	for tenantID, tenantData := range cfg.Tenants {
		if tenantID == "" {
			return nil, fmt.Errorf("tenant ID cannot be empty")
		}

		if tenantData.Limit <= 0 {
			return nil, fmt.Errorf("tenant %s limit must be positive", tenantID)
		}
		if tenantData.Window <= 0 {
			return nil, fmt.Errorf("tenant %s window must be positive", tenantID)
		}

		tenants[tenantID] = &tenantConfig{
			limit:  tenantData.Limit,
			window: tenantData.Window,
		}
	}

	// Verify default tenant exists
	if _, exists := tenants[cfg.DefaultTenant]; !exists {
		return nil, fmt.Errorf("default tenant %s not found in tenant definitions", cfg.DefaultTenant)
	}

	// Initialize observability with NoOp defaults if not provided
	var logger obstypes.Logger = &obstypes.NoOpLogger{}
	var metricsCollector obstypes.Metrics = &obstypes.NoOpMetrics{}
	var tracer obstypes.Tracer = &obstypes.NoOpTracer{}

	if cfg.Observability.Logger != nil {
		logger = cfg.Observability.Logger
	}
	if cfg.Observability.Metrics != nil {
		metricsCollector = cfg.Observability.Metrics
	}
	if cfg.Observability.Tracer != nil {
		tracer = cfg.Observability.Tracer
	}

	// Initialize dynamic limits if provided
	var dynamicsManager *features.DynamicLimitManager
	if cfg.DynamicLimits.EnableDynamicLimits && cfg.DynamicLimits.Manager != nil {
		typedDynamics, ok := cfg.DynamicLimits.Manager.(*features.DynamicLimitManager)
		if !ok {
			return nil, fmt.Errorf("dynamic limits manager must be *features.DynamicLimitManager")
		}
		dynamicsManager = typedDynamics
	}

	// Initialize failover if provided
	var failoverHandler features.FailoverHandler
	if cfg.Failover.EnableFailover && cfg.Failover.Handler != nil {
		typedFailover, ok := cfg.Failover.Handler.(features.FailoverHandler)
		if !ok {
			return nil, fmt.Errorf("failover handler must implement features.FailoverHandler interface")
		}
		failoverHandler = typedFailover
	}

	return &tenantLimiter{
		algorithm:      algo,
		backendBackend: backendBackend,
		config:         cfg,
		closed:         false,
		tenants:        tenants,
		keyTenants:     make(map[string]string),
		defaultTenant:  cfg.DefaultTenant,
		tenantStats:    make(map[string]*tenantStatsData),
		logger:         logger,
		metrics:        metricsCollector,
		tracer:         tracer,
		dynamics:       dynamicsManager,
		failover:       failoverHandler,
	}, nil
}

// Allow checks if a single request is allowed
func (tl *tenantLimiter) Allow(ctx context.Context, key string) *limiters.LimitResult {
	return tl.AllowN(ctx, key, 1)
}

// AllowN checks if N requests are allowed
func (tl *tenantLimiter) AllowN(ctx context.Context, key string, n int) *limiters.LimitResult {
	// Start tracing span
	span := tl.tracer.StartSpan(ctx, "rate_limit.tenant.check")
	defer tl.tracer.EndSpan(span)

	// Add input attributes
	tl.tracer.AddAttribute(span, "key", key)
	tl.tracer.AddAttribute(span, "requests_count", n)

	// Track decision latency
	startTime := time.Now()
	defer func() {
		tl.metrics.RecordDecisionLatency(key, int64(time.Since(startTime)))
	}()

	tl.mu.RLock()
	if tl.closed {
		tl.mu.RUnlock()
		tl.logger.Warn("limiter is closed")
		tl.tracer.SetError(span, "limiter closed")
		return &limiters.LimitResult{
			Allowed: false,
			Error:   limiters.ErrLimiterClosed,
		}
	}
	tl.mu.RUnlock()

	if key == "" {
		tl.logger.Warn("empty key provided")
		tl.tracer.SetError(span, "empty key")
		return &limiters.LimitResult{
			Allowed: false,
			Error:   limiters.ErrInvalidKey,
		}
	}

	if n <= 0 {
		tl.logger.Warn("invalid request count", "n", n)
		tl.tracer.SetError(span, "invalid n")
		return &limiters.LimitResult{
			Allowed: false,
			Error:   fmt.Errorf("n must be positive"),
		}
	}

	// Apply dynamic limits if enabled
	if tl.dynamics != nil {
		limit, window, err := tl.dynamics.GetCurrentLimit(ctx, key)
		if err == nil {
			// Successfully got dynamic limit, update it
			if err := tl.SetLimit(ctx, key, limit, window); err != nil {
				// Log the error but continue with existing limit
				tl.logger.Warn("failed to apply dynamic limit", "key", key, "error", err.Error())
			}
		}
		// If error getting dynamic limit, continue with existing limit
	}

	// Get the tenant for this key — check local cache first, then backend.
	tl.mu.RLock()
	tenantID, exists := tl.keyTenants[key]
	tl.mu.RUnlock()

	if !exists && tl.config.PersistMappings {
		// Try to recover mapping from backend (distributed consistency).
		mappingKey := "__tenant_mapping:" + key
		if raw, err := tl.backendBackend.Get(ctx, mappingKey); err == nil {
			if tenantStr, ok := raw.(string); ok && tenantStr != "" {
				tenantID = tenantStr
				exists = true
				tl.mu.Lock()
				tl.keyTenants[key] = tenantID
				tl.mu.Unlock()
			}
		}
	}

	if !exists {
		tenantID = tl.defaultTenant
	}

	// Add tenant attribute to span
	tl.tracer.AddAttribute(span, "tenant", tenantID)

	// Get tenant configuration
	tl.mu.RLock()
	tenantCfg, tenantExists := tl.tenants[tenantID]
	tl.mu.RUnlock()

	if !tenantExists {
		tl.logger.Error("tenant not found", "tenant", tenantID)
		tl.tracer.SetError(span, fmt.Sprintf("tenant %s not found", tenantID))
		return &limiters.LimitResult{
			Allowed: false,
			Error:   fmt.Errorf("tenant %s not found", tenantID),
		}
	}

	// Atomic read-modify-write
	algoResult, updateErr := backend.AtomicUpdate(ctx, tl.backendBackend, key, tenantCfg.window,
		func(currentState interface{}) (interface{}, interface{}, error) {
			state := currentState

			// Initialize if needed
			if state == nil {
				resetState, resetErr := tl.algorithm.Reset(ctx)
				if resetErr != nil {
					return nil, nil, fmt.Errorf("algorithm reset failed: %w", resetErr)
				}
				state = resetState
			}

			// Check if allowed with tenant limit
			newState, result, err := tl.algorithm.Allow(ctx, state, n)
			if err != nil {
				return nil, nil, fmt.Errorf("algorithm allow failed: %w", err)
			}
			return newState, result, nil
		},
	)

	if updateErr != nil {
		tl.logger.Error("atomic update failed", "key", key, "error", updateErr.Error())
		tl.tracer.SetError(span, fmt.Sprintf("atomic update failed: %v", updateErr))

		// Trigger failover if enabled
		if tl.failover != nil {
			tl.failover.OnBackendFailure(ctx, key, updateErr)

			status := tl.failover.GetFallbackStatus(ctx)
			if status.IsFallbackActive {
				tl.logger.Warn("failover activated, using fallback backend", "key", key)
				return &limiters.LimitResult{
					Allowed: true,
					Error:   fmt.Errorf("using fallback: %w", updateErr),
				}
			}
		}

		return &limiters.LimitResult{
			Allowed: false,
			Error:   updateErr,
		}
	}

	limitResult := tl.convertResult(algoResult, tenantCfg.limit)

	// Add result attributes to span
	tl.tracer.AddAttribute(span, "allowed", limitResult.Allowed)
	tl.tracer.AddAttribute(span, "remaining", limitResult.Remaining)
	tl.tracer.AddAttribute(span, "limit", limitResult.Limit)

	// Log decision and record metrics
	if limitResult.Allowed {
		tl.logger.Debug("rate_limit_allowed",
			"key", key,
			"tenant", tenantID,
			"remaining", limitResult.Remaining,
			"limit", limitResult.Limit,
		)
		tl.metrics.RecordRequest(key, true, int64(limitResult.Limit))
	} else {
		tl.logger.Warn("rate_limit_denied",
			"key", key,
			"tenant", tenantID,
			"remaining", limitResult.Remaining,
			"limit", limitResult.Limit,
		)
		tl.metrics.RecordRequest(key, false, int64(limitResult.Limit))
		tl.tracer.AddEvent(span, "rate_limit_exceeded", map[string]interface{}{
			"tenant":    tenantID,
			"remaining": limitResult.Remaining,
		})
	}

	// Update tenant statistics
	tl.mu.Lock()
	if _, exists := tl.tenantStats[tenantID]; !exists {
		tl.tenantStats[tenantID] = &tenantStatsData{
			tenantID:    tenantID,
			lastUpdated: time.Now(),
		}
	}
	stats := tl.tenantStats[tenantID]
	stats.totalRequests++
	if limitResult.Allowed {
		stats.allowedCount++
	} else {
		stats.deniedCount++
	}
	stats.lastUpdated = time.Now()
	tl.mu.Unlock()

	return limitResult
}

// SetTenantLimit configures limits for a specific tenant
func (tl *tenantLimiter) SetTenantLimit(ctx context.Context, tenantID string, limit int, window time.Duration) error {
	// Start tracing span
	span := tl.tracer.StartSpan(ctx, "rate_limit.tenant.set_tenant_limit")
	defer tl.tracer.EndSpan(span)

	tl.tracer.AddAttribute(span, "tenant", tenantID)
	tl.tracer.AddAttribute(span, "limit", limit)
	tl.tracer.AddAttribute(span, "window_ms", window.Milliseconds())

	if tenantID == "" {
		tl.logger.Warn("tenant ID cannot be empty")
		tl.tracer.SetError(span, "empty tenant ID")
		return fmt.Errorf("tenant ID cannot be empty")
	}
	if limit <= 0 {
		tl.logger.Warn("limit must be positive", "limit", limit)
		tl.tracer.SetError(span, "invalid limit")
		return fmt.Errorf("limit must be positive")
	}
	if window <= 0 {
		tl.logger.Warn("window must be positive", "window", window)
		tl.tracer.SetError(span, "invalid window")
		return fmt.Errorf("window must be positive")
	}

	tl.mu.RLock()
	if tl.closed {
		tl.mu.RUnlock()
		tl.logger.Warn("limiter is closed")
		tl.tracer.SetError(span, "limiter closed")
		return limiters.ErrLimiterClosed
	}
	tl.mu.RUnlock()

	tl.mu.Lock()
	tl.tenants[tenantID] = &tenantConfig{
		limit:  limit,
		window: window,
	}
	tl.mu.Unlock()

	tl.logger.Info("tenant limit updated", "tenant", tenantID, "limit", limit, "window", window)
	return nil
}

// GetTenantLimit retrieves the limit configuration for a tenant
func (tl *tenantLimiter) GetTenantLimit(ctx context.Context, tenantID string) (int, time.Duration, error) {
	if tenantID == "" {
		return 0, 0, fmt.Errorf("tenant ID cannot be empty")
	}

	tl.mu.RLock()
	defer tl.mu.RUnlock()

	tenantCfg, exists := tl.tenants[tenantID]
	if !exists {
		return 0, 0, limiters.ErrInvalidTenant
	}

	return tenantCfg.limit, tenantCfg.window, nil
}

// AssignKeyToTenant assigns a key to a specific tenant
func (tl *tenantLimiter) AssignKeyToTenant(ctx context.Context, key string, tenantID string) error {
	// Start tracing span
	span := tl.tracer.StartSpan(ctx, "rate_limit.tenant.assign_key")
	defer tl.tracer.EndSpan(span)

	tl.tracer.AddAttribute(span, "key", key)
	tl.tracer.AddAttribute(span, "tenant", tenantID)

	if key == "" {
		tl.logger.Warn("empty key provided")
		tl.tracer.SetError(span, "empty key")
		return limiters.ErrInvalidKey
	}
	if tenantID == "" {
		tl.logger.Warn("empty tenant ID")
		tl.tracer.SetError(span, "empty tenant ID")
		return fmt.Errorf("tenant ID cannot be empty")
	}

	tl.mu.RLock()
	if tl.closed {
		tl.mu.RUnlock()
		tl.logger.Warn("limiter is closed")
		tl.tracer.SetError(span, "limiter closed")
		return limiters.ErrLimiterClosed
	}
	// Verify tenant exists
	if _, exists := tl.tenants[tenantID]; !exists {
		tl.mu.RUnlock()
		tl.logger.Warn("tenant not found", "tenant", tenantID)
		tl.tracer.SetError(span, fmt.Sprintf("tenant %s not found", tenantID))
		return limiters.ErrInvalidTenant
	}
	tl.mu.RUnlock()

	tl.mu.Lock()
	tl.keyTenants[key] = tenantID
	// Initialize tenant stats if needed
	if _, exists := tl.tenantStats[tenantID]; !exists {
		tl.tenantStats[tenantID] = &tenantStatsData{
			tenantID:    tenantID,
			lastUpdated: time.Now(),
		}
	}
	tl.tenantStats[tenantID].totalKeys++
	tl.mu.Unlock()

	// Persist mapping to backend for distributed consistency.
	if tl.config.PersistMappings {
		mappingKey := "__tenant_mapping:" + key
		if err := tl.backendBackend.Set(ctx, mappingKey, tenantID, 0); err != nil {
			tl.logger.Warn("failed to persist tenant mapping", "key", key, "error", err.Error())
			// Non-fatal — local mapping is still set.
		}
	}

	tl.logger.Info("key assigned to tenant", "key", key, "tenant", tenantID)
	return nil
}

// GetKeyTenant retrieves which tenant a key belongs to
func (tl *tenantLimiter) GetKeyTenant(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", limiters.ErrInvalidKey
	}

	tl.mu.RLock()
	tenantID, exists := tl.keyTenants[key]
	tl.mu.RUnlock()

	if exists {
		return tenantID, nil
	}

	// Try backend (distributed persistence) when enabled.
	if tl.config.PersistMappings {
		mappingKey := "__tenant_mapping:" + key
		if raw, err := tl.backendBackend.Get(ctx, mappingKey); err == nil {
			if tenantStr, ok := raw.(string); ok && tenantStr != "" {
				tl.mu.Lock()
				tl.keyTenants[key] = tenantStr
				tl.mu.Unlock()
				return tenantStr, nil
			}
		}
	}

	return tl.defaultTenant, nil
}

// GetTenantStats returns aggregated stats for all keys in a tenant
func (tl *tenantLimiter) GetTenantStats(ctx context.Context, tenantID string) *limiters.TenantStats {
	if tenantID == "" {
		return nil
	}

	tl.mu.RLock()
	defer tl.mu.RUnlock()

	statsData, exists := tl.tenantStats[tenantID]
	if !exists {
		return nil
	}

	// Calculate current rate (requests per second)
	timeDiff := time.Since(statsData.lastUpdated).Seconds()
	var currentRate float64
	if timeDiff > 0 {
		currentRate = float64(statsData.totalRequests) / timeDiff
	}

	return &limiters.TenantStats{
		TenantID:      tenantID,
		TotalKeys:     statsData.totalKeys,
		TotalRequests: statsData.totalRequests,
		AllowedCount:  statsData.allowedCount,
		DeniedCount:   statsData.deniedCount,
		CurrentRate:   currentRate,
		LastUpdated:   statsData.lastUpdated,
	}
}

// ListTenants returns all configured tenants
func (tl *tenantLimiter) ListTenants(ctx context.Context) ([]string, error) {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	tenants := make([]string, 0, len(tl.tenants))
	for tenantID := range tl.tenants {
		tenants = append(tenants, tenantID)
	}

	return tenants, nil
}

// SetLimit updates the limit for a specific key (affects its tenant)
func (tl *tenantLimiter) SetLimit(ctx context.Context, key string, limit int, window time.Duration) error {
	if key == "" {
		return limiters.ErrInvalidKey
	}
	if limit <= 0 {
		return fmt.Errorf("limit must be positive")
	}
	if window <= 0 {
		return fmt.Errorf("window must be positive")
	}

	tl.mu.RLock()
	if tl.closed {
		tl.mu.RUnlock()
		return limiters.ErrLimiterClosed
	}
	tl.mu.RUnlock()

	// Get or assign tenant for this key
	tl.mu.RLock()
	tenantID, exists := tl.keyTenants[key]
	tl.mu.RUnlock()

	if !exists {
		tenantID = tl.defaultTenant
	}

	// Update tenant limit (affects all keys using this tenant)
	return tl.SetTenantLimit(ctx, tenantID, limit, window)
}

// Reset clears the state for a key
func (tl *tenantLimiter) Reset(ctx context.Context, key string) error {
	if key == "" {
		return limiters.ErrInvalidKey
	}

	tl.mu.RLock()
	if tl.closed {
		tl.mu.RUnlock()
		return limiters.ErrLimiterClosed
	}
	tl.mu.RUnlock()

	return tl.backendBackend.Delete(ctx, key)
}

// GetStats returns statistics for a key
func (tl *tenantLimiter) GetStats(ctx context.Context, key string) interface{} {
	if key == "" {
		return nil
	}

	tl.mu.RLock()
	if tl.closed {
		tl.mu.RUnlock()
		return nil
	}

	tenantID, exists := tl.keyTenants[key]
	tl.mu.RUnlock()

	if !exists {
		tenantID = tl.defaultTenant
	}

	return map[string]interface{}{
		"key":    key,
		"tenant": tenantID,
	}
}

// Close cleans up resources
func (tl *tenantLimiter) Close() error {
	tl.mu.Lock()
	if tl.closed {
		tl.mu.Unlock()
		return nil
	}
	tl.closed = true
	tl.mu.Unlock()

	return tl.backendBackend.Close()
}

// Type returns the limiter type name
func (tl *tenantLimiter) Type() string {
	return "tenant"
}

// convertResult converts algorithm result to LimitResult
func (tl *tenantLimiter) convertResult(result interface{}, tenantLimit int) *limiters.LimitResult {
	switch r := result.(type) {
	case *algorithm.TokenBucketResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  int(r.RemainingTokens),
			Limit:      tenantLimit,
			ResetAt:    r.ResetTime,
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.LeakyBucketResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  r.QueueCapacity - r.QueueSize,
			Limit:      tenantLimit,
			ResetAt:    r.ResetTime,
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.FixedWindowResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  r.Remaining,
			Limit:      tenantLimit,
			ResetAt:    r.ResetTime,
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.SlidingWindowResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  r.Remaining,
			Limit:      tenantLimit,
			ResetAt:    r.OldestTimestamp.Add(tl.config.DefaultWindow),
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.GCRAResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  0,
			Limit:      tenantLimit,
			ResetAt:    r.TimeToAct,
			RetryAfter: r.RetryAfter,
		}
	default:
		return &limiters.LimitResult{
			Allowed: false,
			Error:   fmt.Errorf("unknown algorithm result type: %T", result),
		}
	}
}
