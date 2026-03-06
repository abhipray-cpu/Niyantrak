package basic

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

// basicLimiter is a simple per-key rate limiter
type basicLimiter struct {
	algorithm algorithm.Algorithm
	backend   backend.Backend
	config    limiters.BasicConfig
	mu        sync.RWMutex
	closed    bool

	// Observability integrations (all optional, default to NoOp for zero overhead)
	logger  obstypes.Logger
	metrics obstypes.Metrics
	tracer  obstypes.Tracer

	// Dynamic limits integration (optional, disabled by default)
	dynamics *features.DynamicLimitManager

	// Failover integration (optional, disabled by default)
	failover features.FailoverHandler
}

// Compile-time check to ensure basicLimiter implements limiters.Limiter
var _ limiters.Limiter = (*basicLimiter)(nil)

// Compile-time check to ensure basicLimiter implements limiters.BasicLimiter
var _ limiters.BasicLimiter = (*basicLimiter)(nil)

// NewBasicLimiter creates a new basic rate limiter
func NewBasicLimiter(
	algo algorithm.Algorithm,
	backend backend.Backend,
	cfg limiters.BasicConfig,
) (limiters.BasicLimiter, error) {
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

	// Validate algorithm configuration if provided
	if cfg.AlgorithmConfig != nil {
		if err := algo.ValidateConfig(cfg.AlgorithmConfig); err != nil {
			return nil, fmt.Errorf("invalid algorithm config: %w", err)
		}
	}

	// Initialize observability with NoOp defaults if not provided (zero overhead)
	var typedLogger obstypes.Logger = &obstypes.NoOpLogger{}
	if cfg.Observability.Logger != nil {
		typedLogger = cfg.Observability.Logger
	}

	var typedMetrics obstypes.Metrics = &obstypes.NoOpMetrics{}
	if cfg.Observability.Metrics != nil {
		typedMetrics = cfg.Observability.Metrics
	}

	var typedTracer obstypes.Tracer = &obstypes.NoOpTracer{}
	if cfg.Observability.Tracer != nil {
		typedTracer = cfg.Observability.Tracer
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

	return &basicLimiter{
		algorithm: algo,
		backend:   backend,
		config:    cfg,
		closed:    false,
		logger:    typedLogger,
		metrics:   typedMetrics,
		tracer:    typedTracer,
		dynamics:  dynamicsManager,
		failover:  failoverHandler,
	}, nil
}

// Allow checks if a single request is allowed
func (bl *basicLimiter) Allow(ctx context.Context, key string) *limiters.LimitResult {
	return bl.AllowN(ctx, key, 1)
}

// AllowN checks if N requests are allowed
func (bl *basicLimiter) AllowN(ctx context.Context, key string, n int) *limiters.LimitResult {
	// Start tracing span (even if NoOp, this is ~1.5 ns/op overhead)
	span := bl.tracer.StartSpan(ctx, "rate_limit.check")
	defer bl.tracer.EndSpan(span)

	// Add input attributes to span
	bl.tracer.AddAttribute(span, "key", key)
	bl.tracer.AddAttribute(span, "requests_count", n)

	// Check if limiter is closed
	bl.mu.RLock()
	if bl.closed {
		bl.mu.RUnlock()
		bl.logger.Error("limiter is closed", "key", key)
		bl.tracer.SetError(span, "limiter is closed")
		return &limiters.LimitResult{
			Allowed: false,
			Error:   limiters.ErrLimiterClosed,
		}
	}
	bl.mu.RUnlock()

	// Validate key
	if key == "" {
		bl.logger.Warn("empty key provided")
		bl.tracer.SetError(span, "empty key")
		return &limiters.LimitResult{
			Allowed: false,
			Error:   limiters.ErrInvalidKey,
		}
	}

	// Validate n
	if n <= 0 {
		bl.logger.Warn("invalid request count", "key", key, "n", n)
		bl.tracer.SetError(span, "invalid request count")
		return &limiters.LimitResult{
			Allowed: false,
			Error:   fmt.Errorf("n must be positive"),
		}
	}

	// Apply dynamic limits if enabled
	if bl.dynamics != nil {
		limit, window, err := bl.dynamics.GetCurrentLimit(ctx, key)
		if err == nil {
			// Successfully got dynamic limit, update it
			if err := bl.SetLimit(ctx, key, limit, window); err != nil {
				// Log the error but continue with existing limit
				bl.logger.Warn("failed to apply dynamic limit", "key", key, "error", err.Error())
			}
		}
		// If error getting dynamic limit, continue with existing limit
	}

	// Record start time for latency metrics
	startTime := time.Now()

	// Atomic read-modify-write: the update function is called with the
	// current state (nil when the key doesn't exist) and returns the
	// new state plus the algorithm result. On backends that implement
	// AtomicBackend this runs under a single lock/transaction; otherwise
	// it falls back to the previous Get→compute→Set sequence.
	algoResult, updateErr := backend.AtomicUpdate(ctx, bl.backend, key, bl.config.KeyTTL,
		func(currentState interface{}) (interface{}, interface{}, error) {
			state := currentState

			// If key doesn't exist, initialize new state
			if state == nil {
				resetState, resetErr := bl.algorithm.Reset(ctx)
				if resetErr != nil {
					return nil, nil, fmt.Errorf("algorithm reset failed: %w", resetErr)
				}
				state = resetState
			}

			// Check if request is allowed with cost = n
			newState, result, err := bl.algorithm.Allow(ctx, state, n)
			if err != nil {
				return nil, nil, fmt.Errorf("algorithm allow failed: %w", err)
			}
			return newState, result, nil
		},
	)

	if updateErr != nil {
		bl.logger.Error("atomic update failed", "key", key, "error", updateErr.Error())
		bl.tracer.SetError(span, fmt.Sprintf("atomic update failed: %v", updateErr))

		// Trigger failover if enabled
		if bl.failover != nil {
			bl.failover.OnBackendFailure(ctx, key, updateErr)

			status := bl.failover.GetFallbackStatus(ctx)
			if status.IsFallbackActive {
				bl.logger.Warn("failover activated, using fallback backend", "key", key)
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

	// Convert algorithm result to limiter result
	limitResult := bl.convertResult(algoResult)

	// Record decision latency
	latencyNs := time.Since(startTime).Nanoseconds()
	bl.metrics.RecordDecisionLatency(key, latencyNs)

	// Add decision attributes to span
	bl.tracer.AddAttribute(span, "allowed", limitResult.Allowed)
	bl.tracer.AddAttribute(span, "remaining", limitResult.Remaining)
	bl.tracer.AddAttribute(span, "limit", limitResult.Limit)

	// Log and record metrics based on decision
	if limitResult.Allowed {
		bl.logger.Debug("rate_limit_allowed",
			"key", key,
			"remaining", limitResult.Remaining,
			"limit", limitResult.Limit,
			"window_ms", bl.config.DefaultWindow.Milliseconds())
		bl.metrics.RecordRequest(key, true, int64(limitResult.Limit))
	} else {
		bl.logger.Warn("rate_limit_denied",
			"key", key,
			"retry_after_ms", limitResult.RetryAfter.Milliseconds(),
			"limit", limitResult.Limit)
		bl.metrics.RecordRequest(key, false, int64(limitResult.Limit))
		bl.tracer.AddEvent(span, "rate_limit_exceeded",
			"retry_after_ms", limitResult.RetryAfter.Milliseconds())
	}

	// Log errors if any
	if limitResult.Error != nil {
		bl.logger.Error("rate limit evaluation error",
			"key", key,
			"error", limitResult.Error.Error())
		bl.tracer.SetError(span, limitResult.Error.Error())
	}

	return limitResult
}

// Reset clears the state for a key
func (bl *basicLimiter) Reset(ctx context.Context, key string) error {
	// Start span
	span := bl.tracer.StartSpan(ctx, "rate_limit.reset")
	defer bl.tracer.EndSpan(span)
	bl.tracer.AddAttribute(span, "key", key)

	bl.mu.RLock()
	if bl.closed {
		bl.mu.RUnlock()
		bl.logger.Error("limiter is closed", "key", key, "operation", "reset")
		bl.tracer.SetError(span, "limiter is closed")
		return limiters.ErrLimiterClosed
	}
	bl.mu.RUnlock()

	if key == "" {
		bl.logger.Warn("empty key provided for reset")
		bl.tracer.SetError(span, "empty key")
		return limiters.ErrInvalidKey
	}

	err := bl.backend.Delete(ctx, key)
	if err != nil {
		bl.logger.Error("reset failed", "key", key, "error", err.Error())
		bl.tracer.SetError(span, err.Error())
		return err
	}

	bl.logger.Info("rate limit reset", "key", key)
	return nil
}

// GetStats returns statistics for a key
func (bl *basicLimiter) GetStats(ctx context.Context, key string) interface{} {
	bl.mu.RLock()
	if bl.closed {
		bl.mu.RUnlock()
		return nil
	}
	bl.mu.RUnlock()

	if key == "" {
		return nil
	}

	state, err := bl.backend.Get(ctx, key)
	if err != nil {
		if err == backend.ErrKeyNotFound {
			// Return stats for uninitialized key
			newState, err := bl.algorithm.Reset(ctx)
			if err != nil {
				return nil
			}
			return bl.algorithm.GetStats(ctx, newState)
		}
		return nil
	}

	return bl.algorithm.GetStats(ctx, state)
}

// Close cleans up resources
func (bl *basicLimiter) Close() error {
	bl.logger.Info("closing basic limiter")

	bl.mu.Lock()
	defer bl.mu.Unlock()

	if bl.closed {
		return nil
	}

	bl.closed = true
	return bl.backend.Close()
}

// SetLimit updates the limit for a specific key
func (bl *basicLimiter) SetLimit(ctx context.Context, key string, limit int, window time.Duration) error {
	// Start span
	span := bl.tracer.StartSpan(ctx, "rate_limit.set_limit")
	defer bl.tracer.EndSpan(span)

	bl.tracer.AddAttribute(span, "key", key)
	bl.tracer.AddAttribute(span, "limit", limit)
	bl.tracer.AddAttribute(span, "window_ms", window.Milliseconds())

	bl.mu.RLock()
	if bl.closed {
		bl.mu.RUnlock()
		bl.logger.Error("limiter is closed", "key", key, "operation", "set_limit")
		bl.tracer.SetError(span, "limiter is closed")
		return limiters.ErrLimiterClosed
	}
	bl.mu.RUnlock()

	if key == "" {
		bl.logger.Warn("empty key provided for set_limit")
		bl.tracer.SetError(span, "empty key")
		return limiters.ErrInvalidKey
	}

	if limit <= 0 || window <= 0 {
		bl.logger.Warn("invalid limit configuration", "key", key, "limit", limit, "window_ms", window.Milliseconds())
		bl.tracer.SetError(span, "invalid limit configuration")
		return limiters.ErrInvalidConfig
	}

	// BasicLimiter doesn't support per-key limits
	// This is a no-op for compatibility
	bl.logger.Info("set_limit called (no-op for basic limiter)", "key", key, "limit", limit)
	return nil
}

// Type returns the limiter type name
func (bl *basicLimiter) Type() string {
	return "basic"
}

// convertResult converts algorithm-specific result to limiter result
func (bl *basicLimiter) convertResult(result interface{}) *limiters.LimitResult {
	switch r := result.(type) {
	case *algorithm.TokenBucketResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  int(r.RemainingTokens),
			Limit:      bl.config.DefaultLimit,
			ResetAt:    r.ResetTime,
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.LeakyBucketResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  r.QueueCapacity - r.QueueSize,
			Limit:      r.QueueCapacity,
			ResetAt:    r.ResetTime,
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.FixedWindowResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  r.Remaining,
			Limit:      r.Limit,
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
			Limit:      r.Limit,
			RetryAfter: r.RetryAfter,
		}
	case *algorithm.GCRAResult:
		return &limiters.LimitResult{
			Allowed:    r.Allowed,
			Remaining:  0, // GCRA doesn't expose remaining count
			Limit:      bl.config.DefaultLimit,
			ResetAt:    r.TAT,
			RetryAfter: r.RetryAfter,
		}
	default:
		// Unknown result type — deny to be safe
		bl.logger.Error("unknown algorithm result type", "type", fmt.Sprintf("%T", result))
		return &limiters.LimitResult{
			Allowed: false,
			Error:   fmt.Errorf("unknown algorithm result type: %T", result),
		}
	}
}
