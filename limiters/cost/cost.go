package cost

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

// costLimiter is a cost-based rate limiter where different operations have different costs
type costLimiter struct {
	algorithm algorithm.Algorithm
	backend   backend.Backend
	config    limiters.CostConfig
	mu        sync.RWMutex
	closed    bool
	// costMap stores the cost for each operation type
	costMap map[string]int

	// Observability (optional, zero-cost if not configured)
	logger  obstypes.Logger
	metrics obstypes.Metrics
	tracer  obstypes.Tracer

	// Dynamic limits integration (optional, disabled by default)
	dynamics *features.DynamicLimitManager

	// Failover integration (optional, disabled by default)
	failover features.FailoverHandler
}

// Compile-time check to ensure costLimiter implements limiters.Limiter
var _ limiters.Limiter = (*costLimiter)(nil)

// Compile-time check to ensure costLimiter implements limiters.CostBasedLimiter
var _ limiters.CostBasedLimiter = (*costLimiter)(nil)

// NewCostBasedLimiter creates a new cost-based rate limiter
func NewCostBasedLimiter(
	algo algorithm.Algorithm,
	backend backend.Backend,
	cfg limiters.CostConfig,
) (limiters.CostBasedLimiter, error) {
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
	if len(cfg.Operations) == 0 {
		return nil, fmt.Errorf("operations cannot be empty")
	}

	// Validate all operation costs are positive
	for opType, cost := range cfg.Operations {
		if cost <= 0 {
			return nil, fmt.Errorf("operation %q cost must be positive, got %d", opType, cost)
		}
	}

	// Validate algorithm configuration if provided
	if cfg.AlgorithmConfig != nil {
		if err := algo.ValidateConfig(cfg.AlgorithmConfig); err != nil {
			return nil, fmt.Errorf("invalid algorithm config: %w", err)
		}
	}

	// Create a copy of operation costs to avoid external modifications
	costMap := make(map[string]int, len(cfg.Operations))
	for opType, cost := range cfg.Operations {
		costMap[opType] = cost
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

	return &costLimiter{
		algorithm: algo,
		backend:   backend,
		config:    cfg,
		closed:    false,
		costMap:   costMap,
		logger:    logger,
		metrics:   metricsCollector,
		tracer:    tracer,
		dynamics:  dynamicsManager,
		failover:  failoverHandler,
	}, nil
}

// Allow checks if a single request is allowed (delegates to AllowCost with default cost of 1)
func (cl *costLimiter) Allow(ctx context.Context, key string) *limiters.LimitResult {
	return cl.AllowN(ctx, key, 1)
}

// AllowN checks if N requests are allowed (delegates to AllowWithCost with cost = N)
func (cl *costLimiter) AllowN(ctx context.Context, key string, n int) *limiters.LimitResult {
	return cl.AllowWithCost(ctx, key, n)
}

// AllowWithCost checks if a request with specific cost is allowed
func (cl *costLimiter) AllowWithCost(ctx context.Context, key string, cost int) *limiters.LimitResult {
	// Start tracing span
	span := cl.tracer.StartSpan(ctx, "rate_limit.cost.check")
	defer cl.tracer.EndSpan(span)

	// Add input attributes
	cl.tracer.AddAttribute(span, "key", key)
	cl.tracer.AddAttribute(span, "cost", cost)

	// Track decision latency
	startTime := time.Now()
	defer func() {
		cl.metrics.RecordDecisionLatency(key, int64(time.Since(startTime)))
	}()

	cl.mu.RLock()
	if cl.closed {
		cl.mu.RUnlock()
		cl.logger.Warn("limiter is closed")
		cl.tracer.SetError(span, "limiter closed")
		return &limiters.LimitResult{
			Allowed: false,
			Error:   limiters.ErrLimiterClosed,
		}
	}
	cl.mu.RUnlock()

	if key == "" {
		cl.logger.Warn("empty key provided")
		cl.tracer.SetError(span, "empty key")
		return &limiters.LimitResult{
			Allowed: false,
			Error:   limiters.ErrInvalidKey,
		}
	}

	if cost <= 0 {
		cl.logger.Warn("invalid cost", "cost", cost)
		cl.tracer.SetError(span, "invalid cost")
		return &limiters.LimitResult{
			Allowed: false,
			Error:   fmt.Errorf("cost must be positive"),
		}
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

	// Atomic read-modify-write
	algoResult, updateErr := backend.AtomicUpdate(ctx, cl.backend, key, cl.config.KeyTTL,
		func(currentState interface{}) (interface{}, interface{}, error) {
			state := currentState

			// If key doesn't exist, initialize new state
			if state == nil {
				resetState, resetErr := cl.algorithm.Reset(ctx)
				if resetErr != nil {
					return nil, nil, fmt.Errorf("algorithm reset failed: %w", resetErr)
				}
				state = resetState
			}

			// Check if request is allowed with cost
			newState, result, err := cl.algorithm.Allow(ctx, state, cost)
			if err != nil {
				return nil, nil, fmt.Errorf("algorithm allow failed: %w", err)
			}
			return newState, result, nil
		},
	)

	if updateErr != nil {
		cl.logger.Error("atomic update failed", "key", key, "error", updateErr.Error())
		cl.tracer.SetError(span, fmt.Sprintf("atomic update failed: %v", updateErr))

		// Trigger failover if enabled
		if cl.failover != nil {
			cl.failover.OnBackendFailure(ctx, key, updateErr)

			status := cl.failover.GetFallbackStatus(ctx)
			if status.IsFallbackActive {
				cl.logger.Warn("failover activated, using fallback backend", "key", key)
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

	limitResult := cl.convertResult(algoResult)

	// Add result attributes to span
	cl.tracer.AddAttribute(span, "allowed", limitResult.Allowed)
	cl.tracer.AddAttribute(span, "remaining", limitResult.Remaining)
	cl.tracer.AddAttribute(span, "limit", limitResult.Limit)

	// Log decision and record metrics
	if limitResult.Allowed {
		cl.logger.Debug("rate_limit_allowed",
			"key", key,
			"cost", cost,
			"remaining", limitResult.Remaining,
			"limit", limitResult.Limit,
		)
		cl.metrics.RecordRequest(key, true, int64(limitResult.Limit))
	} else {
		cl.logger.Warn("rate_limit_denied",
			"key", key,
			"cost", cost,
			"remaining", limitResult.Remaining,
			"limit", limitResult.Limit,
		)
		cl.metrics.RecordRequest(key, false, int64(limitResult.Limit))
		cl.tracer.AddEvent(span, "rate_limit_exceeded", map[string]interface{}{
			"cost":      cost,
			"remaining": limitResult.Remaining,
		})
	}

	return limitResult
}

// GetOperationCost retrieves the cost for an operation
func (cl *costLimiter) GetOperationCost(ctx context.Context, operation string) (int, error) {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	cost, exists := cl.costMap[operation]
	if !exists {
		return 0, fmt.Errorf("operation %q not found", operation)
	}

	return cost, nil
}

// SetOperationCost defines the cost for a specific operation type
func (cl *costLimiter) SetOperationCost(ctx context.Context, operation string, cost int) error {
	// Start tracing span
	span := cl.tracer.StartSpan(ctx, "rate_limit.cost.set_operation_cost")
	defer cl.tracer.EndSpan(span)

	cl.tracer.AddAttribute(span, "operation", operation)
	cl.tracer.AddAttribute(span, "cost", cost)

	if operation == "" {
		cl.logger.Warn("operation cannot be empty")
		cl.tracer.SetError(span, "empty operation")
		return fmt.Errorf("operation cannot be empty")
	}

	if cost <= 0 {
		cl.logger.Warn("cost must be positive", "cost", cost)
		cl.tracer.SetError(span, "invalid cost")
		return fmt.Errorf("cost must be positive")
	}

	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.closed {
		cl.logger.Warn("limiter is closed")
		cl.tracer.SetError(span, "limiter closed")
		return limiters.ErrLimiterClosed
	}

	cl.costMap[operation] = cost

	cl.logger.Info("operation cost updated", "operation", operation, "cost", cost)
	return nil
}

// GetRemainingBudget returns remaining tokens for a key
func (cl *costLimiter) GetRemainingBudget(ctx context.Context, key string) (int, error) {
	if key == "" {
		return 0, limiters.ErrInvalidKey
	}

	cl.mu.RLock()
	if cl.closed {
		cl.mu.RUnlock()
		return 0, limiters.ErrLimiterClosed
	}
	cl.mu.RUnlock()

	_, err := cl.backend.Get(ctx, key)
	if err != nil {
		if err == backend.ErrKeyNotFound {
			// Uninitialized key has full budget
			return cl.config.DefaultLimit, nil
		}
		return 0, fmt.Errorf("backend get failed: %w", err)
	}

	// For now, return a default value
	// In production, you'd extract this from the algorithm state
	return cl.config.DefaultLimit, nil
}

// ListOperations returns all defined operations and their costs
func (cl *costLimiter) ListOperations(ctx context.Context) (map[string]int, error) {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	if cl.closed {
		return nil, limiters.ErrLimiterClosed
	}

	// Create a copy to prevent external modifications
	ops := make(map[string]int, len(cl.costMap))
	for op, cost := range cl.costMap {
		ops[op] = cost
	}

	return ops, nil
}

// Reset clears the state for a key
func (cl *costLimiter) Reset(ctx context.Context, key string) error {
	// Start tracing span
	span := cl.tracer.StartSpan(ctx, "rate_limit.cost.reset")
	defer cl.tracer.EndSpan(span)

	cl.tracer.AddAttribute(span, "key", key)

	cl.mu.RLock()
	if cl.closed {
		cl.mu.RUnlock()
		cl.logger.Warn("limiter is closed")
		cl.tracer.SetError(span, "limiter closed")
		return limiters.ErrLimiterClosed
	}
	cl.mu.RUnlock()

	if key == "" {
		cl.logger.Warn("empty key provided")
		cl.tracer.SetError(span, "empty key")
		return limiters.ErrInvalidKey
	}

	err := cl.backend.Delete(ctx, key)
	if err != nil {
		cl.logger.Error("backend delete failed", "error", err)
		cl.tracer.SetError(span, fmt.Sprintf("backend delete failed: %v", err))
		return err
	}

	cl.logger.Info("key reset", "key", key)
	return nil
}

// GetStats returns statistics for a key
func (cl *costLimiter) GetStats(ctx context.Context, key string) interface{} {
	cl.mu.RLock()
	if cl.closed {
		cl.mu.RUnlock()
		return nil
	}
	cl.mu.RUnlock()

	if key == "" {
		return nil
	}

	state, err := cl.backend.Get(ctx, key)
	if err != nil {
		if err == backend.ErrKeyNotFound {
			// Return stats for uninitialized key
			newState, err := cl.algorithm.Reset(ctx)
			if err != nil {
				return nil
			}
			return cl.algorithm.GetStats(ctx, newState)
		}
		return nil
	}

	return cl.algorithm.GetStats(ctx, state)
}

// Close cleans up resources
func (cl *costLimiter) Close() error {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.closed {
		return nil
	}

	cl.closed = true

	err := cl.backend.Close()
	if err != nil {
		cl.logger.Error("backend close failed", "error", err)
		return err
	}

	cl.logger.Info("cost limiter closed")
	return nil
}

// SetLimit updates the limit and window for the limiter (compatibility method)
func (cl *costLimiter) SetLimit(ctx context.Context, key string, limit int, window time.Duration) error {
	cl.mu.RLock()
	if cl.closed {
		cl.mu.RUnlock()
		return limiters.ErrLimiterClosed
	}
	cl.mu.RUnlock()

	if key == "" {
		return limiters.ErrInvalidKey
	}

	if limit <= 0 || window <= 0 {
		return limiters.ErrInvalidConfig
	}

	// CostBasedLimiter doesn't support per-key limits
	// This is a no-op for compatibility
	return nil
}

// Type returns the limiter type name
func (cl *costLimiter) Type() string {
	return "cost"
}

// convertResult converts algorithm result to limiter result
func (cl *costLimiter) convertResult(result interface{}) *limiters.LimitResult {
	switch r := result.(type) {
	case *algorithm.TokenBucketResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  int(r.RemainingTokens),
			Limit:      cl.config.DefaultLimit,
			ResetAt:    r.ResetTime,
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.LeakyBucketResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  r.QueueCapacity - r.QueueSize,
			Limit:      cl.config.DefaultLimit,
			ResetAt:    r.ResetTime,
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.FixedWindowResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  r.Remaining,
			Limit:      cl.config.DefaultLimit,
			ResetAt:    r.ResetTime,
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.SlidingWindowResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  r.Remaining,
			Limit:      cl.config.DefaultLimit,
			ResetAt:    r.OldestTimestamp.Add(cl.config.DefaultWindow),
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.GCRAResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  0,
			Limit:      cl.config.DefaultLimit,
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
