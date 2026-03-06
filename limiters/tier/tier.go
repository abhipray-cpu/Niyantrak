package tier

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

// tierLimiter provides subscription tier-based rate limiting
type tierLimiter struct {
	algorithm   algorithm.Algorithm
	backend     backend.Backend
	config      limiters.TierConfig
	mu          sync.RWMutex
	closed      bool
	tiers       map[string]*tierConfig // tier name -> limit config
	keyTiers    map[string]string      // key -> tier mapping
	defaultTier string

	// Observability (optional, zero-cost if not configured)
	logger  obstypes.Logger
	metrics obstypes.Metrics
	tracer  obstypes.Tracer

	// Dynamic limits integration (optional, disabled by default)
	dynamics *features.DynamicLimitManager

	// Failover integration (optional, disabled by default)
	failover features.FailoverHandler
}

// tierConfig represents a tier's rate limit configuration
type tierConfig struct {
	limit  int
	window time.Duration
}

// Compile-time check to ensure tierLimiter implements limiters.Limiter
var _ limiters.Limiter = (*tierLimiter)(nil)

// Compile-time check to ensure tierLimiter implements limiters.TierBasedLimiter
var _ limiters.TierBasedLimiter = (*tierLimiter)(nil)

// NewTierBasedLimiter creates a new tier-based rate limiter
func NewTierBasedLimiter(
	algo algorithm.Algorithm,
	backend backend.Backend,
	cfg limiters.TierConfig,
) (limiters.TierBasedLimiter, error) {
	if algo == nil {
		return nil, fmt.Errorf("algorithm cannot be nil")
	}
	if backend == nil {
		return nil, fmt.Errorf("backend cannot be nil")
	}
	if cfg.DefaultLimit <= 0 {
		return nil, fmt.Errorf("default limit must be positive")
	}
	if cfg.DefaultWindow <= 0 {
		return nil, fmt.Errorf("default window must be positive")
	}
	if cfg.DefaultTier == "" {
		return nil, fmt.Errorf("default tier cannot be empty")
	}
	if len(cfg.Tiers) == 0 {
		return nil, fmt.Errorf("tiers cannot be empty")
	}

	// Validate algorithm configuration if provided
	if cfg.AlgorithmConfig != nil {
		if err := algo.ValidateConfig(cfg.AlgorithmConfig); err != nil {
			return nil, fmt.Errorf("invalid algorithm config: %w", err)
		}
	}

	// Initialize tiers map
	tiers := make(map[string]*tierConfig)
	for tierName, tierData := range cfg.Tiers {
		if tierName == "" {
			return nil, fmt.Errorf("tier name cannot be empty")
		}

		if tierData.Limit <= 0 {
			return nil, fmt.Errorf("tier %s limit must be positive", tierName)
		}
		if tierData.Window <= 0 {
			return nil, fmt.Errorf("tier %s window must be positive", tierName)
		}

		tiers[tierName] = &tierConfig{
			limit:  tierData.Limit,
			window: tierData.Window,
		}
	}

	// Verify default tier exists
	if _, exists := tiers[cfg.DefaultTier]; !exists {
		return nil, fmt.Errorf("default tier %s not found in tier definitions", cfg.DefaultTier)
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

	return &tierLimiter{
		algorithm:   algo,
		backend:     backend,
		config:      cfg,
		closed:      false,
		tiers:       tiers,
		keyTiers:    make(map[string]string),
		defaultTier: cfg.DefaultTier,
		logger:      logger,
		metrics:     metricsCollector,
		tracer:      tracer,
		dynamics:    dynamicsManager,
		failover:    failoverHandler,
	}, nil
}

// Allow checks if a single request is allowed
func (tl *tierLimiter) Allow(ctx context.Context, key string) *limiters.LimitResult {
	return tl.AllowN(ctx, key, 1)
}

// AllowN checks if N requests are allowed
func (tl *tierLimiter) AllowN(ctx context.Context, key string, n int) *limiters.LimitResult {
	// Start tracing span
	span := tl.tracer.StartSpan(ctx, "rate_limit.tier.check")
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

	// Get the tier for this key — check local cache first, then backend.
	tl.mu.RLock()
	tier, exists := tl.keyTiers[key]
	tl.mu.RUnlock()

	if !exists && tl.config.PersistMappings {
		// Try to recover mapping from backend (distributed consistency).
		mappingKey := "__tier_mapping:" + key
		if raw, err := tl.backend.Get(ctx, mappingKey); err == nil {
			if tierStr, ok := raw.(string); ok && tierStr != "" {
				tier = tierStr
				exists = true
				// Cache locally for subsequent fast lookups.
				tl.mu.Lock()
				tl.keyTiers[key] = tier
				tl.mu.Unlock()
			}
		}
	}

	if !exists {
		tier = tl.defaultTier
	}

	// Add tier attribute to span
	tl.tracer.AddAttribute(span, "tier", tier)

	// Get tier configuration
	tl.mu.RLock()
	tierCfg, tierExists := tl.tiers[tier]
	tl.mu.RUnlock()

	if !tierExists {
		tl.logger.Error("tier not found", "tier", tier)
		tl.tracer.SetError(span, fmt.Sprintf("tier %s not found", tier))
		return &limiters.LimitResult{
			Allowed: false,
			Error:   fmt.Errorf("tier %s not found", tier),
		}
	}

	// Atomic read-modify-write
	algoResult, updateErr := backend.AtomicUpdate(ctx, tl.backend, key, tierCfg.window,
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

			// Check if allowed with tier limit
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

	limitResult := tl.convertResult(algoResult, tierCfg.limit)

	// Add result attributes to span
	tl.tracer.AddAttribute(span, "allowed", limitResult.Allowed)
	tl.tracer.AddAttribute(span, "remaining", limitResult.Remaining)
	tl.tracer.AddAttribute(span, "limit", limitResult.Limit)

	// Log decision and record metrics
	if limitResult.Allowed {
		tl.logger.Debug("rate_limit_allowed",
			"key", key,
			"tier", tier,
			"remaining", limitResult.Remaining,
			"limit", limitResult.Limit,
		)
		tl.metrics.RecordRequest(key, true, int64(limitResult.Limit))
	} else {
		tl.logger.Warn("rate_limit_denied",
			"key", key,
			"tier", tier,
			"remaining", limitResult.Remaining,
			"limit", limitResult.Limit,
		)
		tl.metrics.RecordRequest(key, false, int64(limitResult.Limit))
		tl.tracer.AddEvent(span, "rate_limit_exceeded", map[string]interface{}{
			"tier":      tier,
			"remaining": limitResult.Remaining,
		})
	}

	return limitResult
}

// SetTierLimit configures limits for a specific tier
func (tl *tierLimiter) SetTierLimit(ctx context.Context, tier string, limit int, window time.Duration) error {
	// Start tracing span
	span := tl.tracer.StartSpan(ctx, "rate_limit.tier.set_tier_limit")
	defer tl.tracer.EndSpan(span)

	tl.tracer.AddAttribute(span, "tier", tier)
	tl.tracer.AddAttribute(span, "limit", limit)
	tl.tracer.AddAttribute(span, "window_ms", window.Milliseconds())

	if tier == "" {
		tl.logger.Warn("tier name cannot be empty")
		tl.tracer.SetError(span, "empty tier name")
		return fmt.Errorf("tier name cannot be empty")
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
	tl.tiers[tier] = &tierConfig{
		limit:  limit,
		window: window,
	}
	tl.mu.Unlock()

	tl.logger.Info("tier limit updated", "tier", tier, "limit", limit, "window", window)
	return nil
}

// GetTierLimit retrieves the limit configuration for a tier
func (tl *tierLimiter) GetTierLimit(ctx context.Context, tier string) (int, time.Duration, error) {
	if tier == "" {
		return 0, 0, fmt.Errorf("tier name cannot be empty")
	}

	tl.mu.RLock()
	defer tl.mu.RUnlock()

	tierCfg, exists := tl.tiers[tier]
	if !exists {
		return 0, 0, limiters.ErrInvalidTier
	}

	return tierCfg.limit, tierCfg.window, nil
}

// AssignKeyToTier assigns a key to a specific tier
func (tl *tierLimiter) AssignKeyToTier(ctx context.Context, key string, tier string) error {
	// Start tracing span
	span := tl.tracer.StartSpan(ctx, "rate_limit.tier.assign_key")
	defer tl.tracer.EndSpan(span)

	tl.tracer.AddAttribute(span, "key", key)
	tl.tracer.AddAttribute(span, "tier", tier)

	if key == "" {
		tl.logger.Warn("empty key provided")
		tl.tracer.SetError(span, "empty key")
		return limiters.ErrInvalidKey
	}
	if tier == "" {
		tl.logger.Warn("empty tier name")
		tl.tracer.SetError(span, "empty tier name")
		return fmt.Errorf("tier name cannot be empty")
	}

	tl.mu.RLock()
	if tl.closed {
		tl.mu.RUnlock()
		tl.logger.Warn("limiter is closed")
		tl.tracer.SetError(span, "limiter closed")
		return limiters.ErrLimiterClosed
	}
	// Verify tier exists
	if _, exists := tl.tiers[tier]; !exists {
		tl.mu.RUnlock()
		tl.logger.Warn("tier not found", "tier", tier)
		tl.tracer.SetError(span, "tier not found")
		return limiters.ErrInvalidTier
	}
	tl.mu.RUnlock()

	tl.mu.Lock()
	tl.keyTiers[key] = tier
	tl.mu.Unlock()

	// Persist mapping to backend for distributed consistency.
	if tl.config.PersistMappings {
		mappingKey := "__tier_mapping:" + key
		if err := tl.backend.Set(ctx, mappingKey, tier, 0); err != nil {
			tl.logger.Warn("failed to persist tier mapping", "key", key, "error", err.Error())
			// Non-fatal — local mapping is still set.
		}
	}

	tl.logger.Info("key assigned to tier", "key", key, "tier", tier)
	return nil
}

// GetKeyTier retrieves which tier a key belongs to
func (tl *tierLimiter) GetKeyTier(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", limiters.ErrInvalidKey
	}

	tl.mu.RLock()
	tier, exists := tl.keyTiers[key]
	tl.mu.RUnlock()

	if exists {
		return tier, nil
	}

	// Try backend (distributed persistence) when enabled.
	if tl.config.PersistMappings {
		mappingKey := "__tier_mapping:" + key
		if raw, err := tl.backend.Get(ctx, mappingKey); err == nil {
			if tierStr, ok := raw.(string); ok && tierStr != "" {
				tl.mu.Lock()
				tl.keyTiers[key] = tierStr
				tl.mu.Unlock()
				return tierStr, nil
			}
		}
	}

	return tl.defaultTier, nil
}

// ListTiers returns all configured tiers
func (tl *tierLimiter) ListTiers(ctx context.Context) ([]string, error) {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	tiers := make([]string, 0, len(tl.tiers))
	for tierName := range tl.tiers {
		tiers = append(tiers, tierName)
	}

	return tiers, nil
}

// SetLimit updates the limit for a specific key
func (tl *tierLimiter) SetLimit(ctx context.Context, key string, limit int, window time.Duration) error {
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

	// Get or assign tier for this key
	tl.mu.RLock()
	tier, exists := tl.keyTiers[key]
	tl.mu.RUnlock()

	if !exists {
		tier = tl.defaultTier
	}

	// Update tier limit (affects all keys using this tier)
	return tl.SetTierLimit(ctx, tier, limit, window)
}

// Reset clears the state for a key
func (tl *tierLimiter) Reset(ctx context.Context, key string) error {
	// Start tracing span
	span := tl.tracer.StartSpan(ctx, "rate_limit.tier.reset")
	defer tl.tracer.EndSpan(span)

	tl.tracer.AddAttribute(span, "key", key)

	if key == "" {
		tl.logger.Warn("empty key provided")
		tl.tracer.SetError(span, "empty key")
		return limiters.ErrInvalidKey
	}

	tl.mu.RLock()
	if tl.closed {
		tl.mu.RUnlock()
		tl.logger.Warn("limiter is closed")
		tl.tracer.SetError(span, "limiter closed")
		return limiters.ErrLimiterClosed
	}
	tl.mu.RUnlock()

	err := tl.backend.Delete(ctx, key)
	if err != nil {
		tl.logger.Error("rate limit reset failed", "key", key, "error", err)
		tl.tracer.SetError(span, fmt.Sprintf("reset failed: %v", err))
	} else {
		tl.logger.Info("rate limit reset", "key", key)
	}

	return err
}

// GetStats returns statistics for a key
func (tl *tierLimiter) GetStats(ctx context.Context, key string) interface{} {
	if key == "" {
		return nil
	}

	tl.mu.RLock()
	if tl.closed {
		tl.mu.RUnlock()
		return nil
	}

	tier, exists := tl.keyTiers[key]
	tl.mu.RUnlock()

	if !exists {
		tier = tl.defaultTier
	}

	return map[string]interface{}{
		"key":  key,
		"tier": tier,
	}
}

// Close cleans up resources
func (tl *tierLimiter) Close() error {
	tl.mu.Lock()
	if tl.closed {
		tl.mu.Unlock()
		return nil
	}
	tl.closed = true
	tl.mu.Unlock()

	tl.logger.Info("closing tier limiter")
	return tl.backend.Close()
}

// Type returns the limiter type name
func (tl *tierLimiter) Type() string {
	return "tier"
}

// convertResult converts algorithm result to LimitResult
func (tl *tierLimiter) convertResult(result interface{}, tierLimit int) *limiters.LimitResult {
	switch r := result.(type) {
	case *algorithm.TokenBucketResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  int(r.RemainingTokens),
			Limit:      tierLimit,
			ResetAt:    r.ResetTime,
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.LeakyBucketResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  r.QueueCapacity - r.QueueSize,
			Limit:      tierLimit,
			ResetAt:    r.ResetTime,
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.FixedWindowResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  r.Remaining,
			Limit:      tierLimit,
			ResetAt:    r.ResetTime,
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.SlidingWindowResult:
		remaining := r.Limit - r.RequestCount
		if remaining < 0 {
			remaining = 0
		}
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  remaining,
			Limit:      tierLimit,
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.GCRAResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  0,
			Limit:      tierLimit,
			ResetAt:    r.TAT,
			RetryAfter: r.RetryAfter,
		}
	default:
		tl.logger.Error("unknown algorithm result type", "type", fmt.Sprintf("%T", result))
		return &limiters.LimitResult{
			Allowed: false,
			Error:   fmt.Errorf("unknown algorithm result type: %T", result),
		}
	}
}
