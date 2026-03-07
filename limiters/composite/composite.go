// Package composite provides a composite rate limiter combining multiple independent rate limits
package composite

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/abhipray-cpu/niyantrak/algorithm"
	"github.com/abhipray-cpu/niyantrak/backend"
	"github.com/abhipray-cpu/niyantrak/features"
	"github.com/abhipray-cpu/niyantrak/limiters"
	obstypes "github.com/abhipray-cpu/niyantrak/observability/types"
)

// compositeLimiter implements CompositeLimiter using multiple independent rate limiters
type compositeLimiter struct {
	limiters       map[string]*limitData
	config         limiters.CompositeConfig
	mu             sync.RWMutex
	closed         bool
	algorithm      algorithm.Algorithm
	backendBackend backend.Backend

	// Observability (optional, zero-cost if not configured)
	logger  obstypes.Logger
	metrics obstypes.Metrics
	tracer  obstypes.Tracer

	// Dynamic limits integration (optional, disabled by default)
	dynamics *features.DynamicLimitManager

	// Failover integration (optional, disabled by default)
	failover features.FailoverHandler
}

// limitData holds state for a single limit
type limitData struct {
	name           string
	limit          int
	window         time.Duration
	algorithm      algorithm.Algorithm
	backendBackend backend.Backend
	priority       int
	createdAt      time.Time
}

// NewCompositeLimiter creates a new composite rate limiter
func NewCompositeLimiter(algo algorithm.Algorithm, be backend.Backend, cfg limiters.CompositeConfig) (limiters.CompositeLimiter, error) {
	// Validate inputs
	if algo == nil {
		return nil, fmt.Errorf("algorithm cannot be nil")
	}
	if be == nil {
		return nil, fmt.Errorf("backend cannot be nil")
	}
	if len(cfg.Limits) == 0 {
		return nil, fmt.Errorf("composite limiter must have at least one limit")
	}
	if cfg.Name == "" {
		return nil, fmt.Errorf("composite limiter name cannot be empty")
	}

	// Validate each limit
	limitMap := make(map[string]*limitData)
	seenPriorities := make(map[int]bool)

	for i, lc := range cfg.Limits {
		if lc.Name == "" {
			return nil, fmt.Errorf("limit at index %d has empty name", i)
		}
		if lc.Limit <= 0 {
			return nil, fmt.Errorf("limit %q must be positive, got %d", lc.Name, lc.Limit)
		}
		if lc.Window <= 0 {
			return nil, fmt.Errorf("limit %q window must be positive, got %v", lc.Name, lc.Window)
		}
		if _, exists := limitMap[lc.Name]; exists {
			return nil, fmt.Errorf("duplicate limit name: %q", lc.Name)
		}

		limitMap[lc.Name] = &limitData{
			name:           lc.Name,
			limit:          lc.Limit,
			window:         lc.Window,
			algorithm:      algo,
			backendBackend: be,
			priority:       lc.Priority,
			createdAt:      time.Now(),
		}

		seenPriorities[lc.Priority] = true
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

	return &compositeLimiter{
		limiters:       limitMap,
		config:         cfg,
		algorithm:      algo,
		backendBackend: be,
		closed:         false,
		logger:         logger,
		metrics:        metricsCollector,
		tracer:         tracer,
		dynamics:       dynamicsManager,
		failover:       failoverHandler,
	}, nil
}

// Allow checks if a single request is allowed against all limits
func (cl *compositeLimiter) Allow(ctx context.Context, key string) *limiters.LimitResult {
	return cl.AllowN(ctx, key, 1)
}

// AllowN checks if N requests are allowed against all limits
func (cl *compositeLimiter) AllowN(ctx context.Context, key string, n int) *limiters.LimitResult {
	// Start tracing span
	span := cl.tracer.StartSpan(ctx, "rate_limit.composite.check")
	defer cl.tracer.EndSpan(span)

	// Add input attributes
	cl.tracer.AddAttribute(span, "key", key)
	cl.tracer.AddAttribute(span, "requests_count", n)

	// Track decision latency
	startTime := time.Now()
	defer func() {
		cl.metrics.RecordDecisionLatency(key, int64(time.Since(startTime)))
	}()

	// Verify limiter not closed
	cl.mu.RLock()
	if cl.closed {
		cl.mu.RUnlock()
		cl.logger.Warn("limiter is closed")
		cl.tracer.SetError(span, "limiter closed")
		return &limiters.LimitResult{Allowed: false, Error: limiters.ErrLimiterClosed}
	}
	cl.mu.RUnlock()

	// Validate inputs
	if key == "" {
		cl.logger.Warn("empty key provided")
		cl.tracer.SetError(span, "empty key")
		return &limiters.LimitResult{Allowed: false, Error: limiters.ErrInvalidKey}
	}
	if n <= 0 {
		cl.logger.Warn("invalid request count", "n", n)
		cl.tracer.SetError(span, "invalid n")
		return &limiters.LimitResult{Allowed: false, Error: fmt.Errorf("n must be positive")}
	}

	// Apply dynamic limits if enabled
	if cl.dynamics != nil {
		limit, window, err := cl.dynamics.GetCurrentLimit(ctx, key)
		if err == nil {
			// Successfully got dynamic limit, update it
			if err := cl.SetLimit(ctx, key, limit, window); err != nil {
				// Log the error but continue with existing limit
				cl.logger.Warn("failed to apply dynamic limit", "key", key, "error", err.Error())
			}
		}
		// If error getting dynamic limit, continue with existing limit
	}

	// Check all limits
	var minRemaining int = -1
	var resetAt time.Time

	cl.mu.RLock()
	limits := make([]*limitData, 0, len(cl.limiters))
	for _, ld := range cl.limiters {
		limits = append(limits, ld)
	}
	cl.mu.RUnlock()

	for _, ld := range limits {
		// Atomic read-modify-write for each sub-limit
		compositeKey := fmt.Sprintf("%s:%s", key, ld.name)

		algoResult, updateErr := backend.AtomicUpdate(ctx, ld.backendBackend, compositeKey, ld.window,
			func(currentState interface{}) (interface{}, interface{}, error) {
				state := currentState
				if state == nil {
					s, _ := ld.algorithm.Reset(ctx)
					state = s
				}
				newState, result, err := ld.algorithm.Allow(ctx, state, n)
				if err != nil {
					return nil, nil, err
				}
				return newState, result, nil
			},
		)

		if updateErr != nil {
			cl.logger.Error("atomic update failed", "limit", ld.name, "key", compositeKey, "error", updateErr)
			cl.tracer.SetError(span, fmt.Sprintf("atomic update failed: %v", updateErr))

			// Trigger failover if enabled
			if cl.failover != nil {
				cl.failover.OnBackendFailure(ctx, compositeKey, updateErr)

				status := cl.failover.GetFallbackStatus(ctx)
				if status.IsFallbackActive {
					cl.logger.Warn("failover activated, using fallback backend", "key", compositeKey)
					return &limiters.LimitResult{
						Allowed: true,
						Error:   fmt.Errorf("using fallback: %w", updateErr),
					}
				}
			}

			return &limiters.LimitResult{Allowed: false, Error: updateErr}
		}

		// Convert algorithm result
		limitResult := cl.convertResult(algoResult, ld.limit, ld.window)

		// Record metrics for this limit check
		if limitResult.Allowed {
			cl.metrics.RecordRequest(ld.name, true, int64(n))
			cl.logger.Debug("limit check passed", "limit", ld.name, "key", key, "requests", n)
		} else {
			cl.metrics.RecordRequest(ld.name, false, int64(n))
			cl.logger.Debug("limit check failed", "limit", ld.name, "key", key, "requests", n)
		}

		// Track minimum remaining (all must pass)
		if minRemaining == -1 || limitResult.Remaining < minRemaining {
			minRemaining = limitResult.Remaining
		}

		// If any limit denies, deny overall
		if !limitResult.Allowed {
			cl.logger.Info("composite request denied by limit", "limit", ld.name, "key", key, "requests", n)
			cl.tracer.AddAttribute(span, "denied_by", ld.name)
			if resetAt.IsZero() {
				resetAt = time.Now().Add(ld.window)
			}
			return &limiters.LimitResult{
				Allowed:    false,
				Remaining:  0,
				Limit:      ld.limit,
				ResetAt:    resetAt,
				RetryAfter: ld.window,
				Error:      limiters.ErrLimitExceeded,
			}
		}

		// Track earliest reset
		if resetAt.IsZero() || time.Now().Add(ld.window).Before(resetAt) {
			resetAt = time.Now().Add(ld.window)
		}
	}

	if minRemaining < 0 {
		minRemaining = 0
	}

	cl.logger.Debug("composite request allowed", "key", key, "requests", n, "remaining", minRemaining)
	cl.tracer.AddAttribute(span, "allowed", true)
	cl.tracer.AddAttribute(span, "remaining", minRemaining)

	return &limiters.LimitResult{
		Allowed:   true,
		Remaining: minRemaining,
		Limit:     cl.getLargestLimit(),
		ResetAt:   resetAt,
		Error:     nil,
	}
}

// AddLimit adds a new limit to the composite
func (cl *compositeLimiter) AddLimit(ctx context.Context, name string, limit int, window time.Duration) error {
	// Validate inputs
	if name == "" {
		return fmt.Errorf("limit name cannot be empty")
	}
	if limit <= 0 {
		return fmt.Errorf("limit must be positive, got %d", limit)
	}
	if window <= 0 {
		return fmt.Errorf("window must be positive, got %v", window)
	}

	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.closed {
		return limiters.ErrLimiterClosed
	}

	if _, exists := cl.limiters[name]; exists {
		return fmt.Errorf("limit %q already exists", name)
	}

	cl.limiters[name] = &limitData{
		name:           name,
		limit:          limit,
		window:         window,
		algorithm:      cl.algorithm,
		backendBackend: cl.backendBackend,
		priority:       len(cl.limiters), // Add to end by default
		createdAt:      time.Now(),
	}

	return nil
}

// RemoveLimit removes a limit from the composite
func (cl *compositeLimiter) RemoveLimit(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("limit name cannot be empty")
	}

	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.closed {
		return limiters.ErrLimiterClosed
	}

	if _, exists := cl.limiters[name]; !exists {
		return fmt.Errorf("limit %q not found", name)
	}

	delete(cl.limiters, name)
	return nil
}

// GetLimits returns all configured limits
func (cl *compositeLimiter) GetLimits(ctx context.Context) ([]limiters.LimitConfig, error) {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	if cl.closed {
		return nil, limiters.ErrLimiterClosed
	}

	limits := make([]limiters.LimitConfig, 0, len(cl.limiters))
	for _, ld := range cl.limiters {
		limits = append(limits, limiters.LimitConfig{
			Name:     ld.name,
			Limit:    ld.limit,
			Window:   ld.window,
			Priority: ld.priority,
		})
	}

	return limits, nil
}

// CheckAll checks all limits and returns which ones are exceeded
func (cl *compositeLimiter) CheckAll(ctx context.Context, key string) ([]limiters.LimitStatus, error) {
	if key == "" {
		cl.logger.Warn("empty key provided to CheckAll")
		return nil, limiters.ErrInvalidKey
	}

	cl.mu.RLock()
	if cl.closed {
		cl.mu.RUnlock()
		cl.logger.Warn("limiter is closed")
		return nil, limiters.ErrLimiterClosed
	}
	limits := make([]*limitData, 0, len(cl.limiters))
	for _, ld := range cl.limiters {
		limits = append(limits, ld)
	}
	cl.mu.RUnlock()

	statuses := make([]limiters.LimitStatus, 0, len(limits))

	for _, ld := range limits {
		compositeKey := fmt.Sprintf("%s:%s", key, ld.name)
		state, err := ld.backendBackend.Get(ctx, compositeKey)
		if err != nil && !errors.Is(err, backend.ErrKeyNotFound) {
			cl.logger.Error("failed to get state in CheckAll", "limit", ld.name, "error", err)
			return nil, err
		}
		if state == nil {
			state, _ = ld.algorithm.Reset(ctx)
		}

		// Peek at what would happen with one more request
		_, result, _ := ld.algorithm.Allow(ctx, state, 1)
		limitResult := cl.convertResult(result, ld.limit, ld.window)

		remaining := limitResult.Remaining
		progress := 0
		if remaining < ld.limit {
			progress = int((float64(ld.limit-remaining) / float64(ld.limit)) * 100)
		}

		cl.logger.Debug("limit status checked", "limit", ld.name, "key", key, "allowed", limitResult.Allowed, "remaining", remaining, "progress", progress)

		statuses = append(statuses, limiters.LimitStatus{
			Name:      ld.name,
			Allowed:   limitResult.Allowed,
			Remaining: remaining,
			Limit:     ld.limit,
			ResetAt:   time.Now().Add(ld.window),
			Progress:  progress,
		})
	}

	return statuses, nil
}

// GetHierarchy returns the hierarchy/relationship between limits
func (cl *compositeLimiter) GetHierarchy(ctx context.Context) *limiters.LimitHierarchy {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	limits := make([]limiters.LimitConfig, 0, len(cl.limiters))
	for _, ld := range cl.limiters {
		limits = append(limits, limiters.LimitConfig{
			Name:     ld.name,
			Limit:    ld.limit,
			Window:   ld.window,
			Priority: ld.priority,
		})
	}

	relationships := []string{}
	for i := 0; i < len(limits)-1; i++ {
		if limits[i].Limit < limits[i+1].Limit {
			relationships = append(relationships, fmt.Sprintf("%s < %s", limits[i].Name, limits[i+1].Name))
		}
	}

	return &limiters.LimitHierarchy{
		Name:              cl.config.Name,
		Limits:            limits,
		Relationships:     relationships,
		ConflictingLimits: false,
	}
}

// Reset clears the state for a key across all limits
func (cl *compositeLimiter) Reset(ctx context.Context, key string) error {
	if key == "" {
		cl.logger.Warn("empty key provided to Reset")
		return limiters.ErrInvalidKey
	}

	cl.mu.RLock()
	if cl.closed {
		cl.mu.RUnlock()
		cl.logger.Warn("limiter is closed")
		return limiters.ErrLimiterClosed
	}
	limits := make([]*limitData, 0, len(cl.limiters))
	for _, ld := range cl.limiters {
		limits = append(limits, ld)
	}
	cl.mu.RUnlock()

	for _, ld := range limits {
		compositeKey := fmt.Sprintf("%s:%s", key, ld.name)
		if err := ld.backendBackend.Delete(ctx, compositeKey); err != nil {
			cl.logger.Error("failed to delete state in Reset", "limit", ld.name, "key", compositeKey, "error", err)
			return err
		}
	}

	cl.logger.Info("composite limiter state reset", "key", key)
	return nil
}

// GetStats returns statistics for a key
func (cl *compositeLimiter) GetStats(ctx context.Context, key string) interface{} {
	if key == "" {
		return map[string]interface{}{"error": "invalid key"}
	}

	cl.mu.RLock()
	if cl.closed {
		cl.mu.RUnlock()
		return map[string]interface{}{"error": "limiter closed"}
	}
	limits := make([]*limitData, 0, len(cl.limiters))
	for _, ld := range cl.limiters {
		limits = append(limits, ld)
	}
	cl.mu.RUnlock()

	limitStats := make(map[string]interface{})
	for _, ld := range limits {
		compositeKey := fmt.Sprintf("%s:%s", key, ld.name)
		state, err := ld.backendBackend.Get(ctx, compositeKey)
		if err != nil && !errors.Is(err, backend.ErrKeyNotFound) {
			limitStats[ld.name] = map[string]interface{}{"error": err.Error()}
			continue
		}

		if state == nil {
			limitStats[ld.name] = map[string]interface{}{
				"limit":     ld.limit,
				"window":    ld.window.String(),
				"remaining": ld.limit,
			}
		} else {
			limitStats[ld.name] = state
		}
	}

	return map[string]interface{}{
		"key":    key,
		"type":   "composite",
		"name":   cl.config.Name,
		"limits": limitStats,
	}
}

// SetLimit updates a limit (delegated to specific limit)
func (cl *compositeLimiter) SetLimit(ctx context.Context, key string, limit int, window time.Duration) error {
	// For composite limiters, SetLimit is not directly applicable
	// Use AddLimit or RemoveLimit for limit management
	return fmt.Errorf("use AddLimit or RemoveLimit for composite limiter management")
}

// Close cleans up resources
func (cl *compositeLimiter) Close() error {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.closed {
		return nil // Idempotent
	}

	cl.closed = true
	cl.logger.Info("composite limiter closed", "name", cl.config.Name)

	if err := cl.backendBackend.Close(); err != nil {
		cl.logger.Error("failed to close backend", "error", err)
		return err
	}

	cl.limiters = nil
	return nil
}

// Type returns the limiter type name
func (cl *compositeLimiter) Type() string {
	return "composite"
}

// getLargestLimit returns the largest limit among all limits
func (cl *compositeLimiter) getLargestLimit() int {
	maxLimit := 0
	for _, ld := range cl.limiters {
		if ld.limit > maxLimit {
			maxLimit = ld.limit
		}
	}
	return maxLimit
}

// convertResult converts algorithm result to limiter result
func (cl *compositeLimiter) convertResult(result interface{}, limit int, window time.Duration) *limiters.LimitResult {
	switch r := result.(type) {
	case *algorithm.TokenBucketResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  int(r.RemainingTokens),
			Limit:      limit,
			ResetAt:    r.ResetTime,
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.LeakyBucketResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  r.QueueCapacity - r.QueueSize,
			Limit:      limit,
			ResetAt:    r.ResetTime,
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.FixedWindowResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  r.Remaining,
			Limit:      limit,
			ResetAt:    r.ResetTime,
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.SlidingWindowResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  r.Remaining,
			Limit:      limit,
			ResetAt:    r.OldestTimestamp.Add(window),
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.GCRAResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  0,
			Limit:      limit,
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

// Verify interface compliance
var _ limiters.CompositeLimiter = (*compositeLimiter)(nil)
var _ limiters.Limiter = (*compositeLimiter)(nil)
