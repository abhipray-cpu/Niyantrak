package features

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/abhipray-cpu/niyantrak/backend"
	obstypes "github.com/abhipray-cpu/niyantrak/observability/types"
)

// FailoverHandler manages graceful degradation when backends fail
type FailoverHandler interface {
	// OnBackendFailure is called when primary backend operation fails
	// Should return a decision whether to allow/deny the request
	OnBackendFailure(ctx context.Context, key string, err error) interface{}

	// GetFallbackBackend returns a fallback backend if primary fails
	// Returns nil if no fallback is available
	GetFallbackBackend() interface{}

	// IsHealthy checks if the primary backend is healthy
	IsHealthy(ctx context.Context) bool

	// SwitchToFallback switches to fallback mode
	SwitchToFallback(ctx context.Context) error

	// SwitchToPrimary switches back to primary backend
	SwitchToPrimary(ctx context.Context) error

	// GetFallbackStatus returns current failover status
	GetFallbackStatus(ctx context.Context) *FailoverStatus
}

// FailoverStatus represents current failover state
type FailoverStatus struct {
	// IsFallbackActive indicates if we're using fallback
	IsFallbackActive bool

	// FailureReason is why we switched to fallback (if active)
	FailureReason string

	// SwitchedAt is when the switch happened
	SwitchedAt interface{} // time.Time

	// FailureCount is number of consecutive failures
	FailureCount int

	// LastHealthCheck is when we last checked health
	LastHealthCheck interface{} // time.Time
}

// FailoverConfig holds failover configuration
type FailoverConfig struct {
	// EnableFallback enables fallback mechanism
	EnableFallback bool

	// FallbackBackendType is the type of fallback (memory, local cache, etc.)
	FallbackBackendType string

	// HealthCheckInterval is how often to check backend health
	HealthCheckInterval interface{} // time.Duration

	// FailureThreshold is consecutive failures before switching
	FailureThreshold int

	// AutoRecovery if true, automatically switch back when primary recovers
	AutoRecovery bool

	// RecoveryCheckInterval is how often to check for recovery
	RecoveryCheckInterval interface{} // time.Duration
}

// failoverManager implements FailoverHandler with automatic failover and recovery
type failoverManager struct {
	primaryBackend      backend.Backend
	fallbackBackend     backend.Backend
	config              FailoverConfig
	mu                  sync.RWMutex
	currentBackend      backend.Backend
	isUsingFallback     bool
	failureCount        int64
	lastFailureTime     time.Time
	lastSuccessTime     time.Time
	lastHealthCheckTime time.Time
	healthCheckTicker   *time.Ticker
	recoveryCheckTicker *time.Ticker
	done                chan struct{}
	logger              obstypes.Logger
	metricsCollector    obstypes.Metrics
	tracer              obstypes.Tracer
}

// NewFailoverManager creates a new failover handler with primary and fallback backends
func NewFailoverManager(
	primaryBackend backend.Backend,
	fallbackBackend backend.Backend,
	config FailoverConfig,
	logger obstypes.Logger,
	metricsCollector obstypes.Metrics,
	tracer obstypes.Tracer,
) (FailoverHandler, error) {
	// Validate inputs
	if primaryBackend == nil {
		return nil, fmt.Errorf("primary backend cannot be nil")
	}
	if fallbackBackend == nil {
		return nil, fmt.Errorf("fallback backend cannot be nil")
	}

	// Use default config if not provided
	if config.FailureThreshold == 0 {
		config.FailureThreshold = 5
	}

	healthCheckInterval := 10 * time.Second
	if hci, ok := config.HealthCheckInterval.(time.Duration); ok {
		healthCheckInterval = hci
	}

	recoveryCheckInterval := 30 * time.Second
	if rci, ok := config.RecoveryCheckInterval.(time.Duration); ok {
		recoveryCheckInterval = rci
	}

	fm := &failoverManager{
		primaryBackend:      primaryBackend,
		fallbackBackend:     fallbackBackend,
		config:              config,
		currentBackend:      primaryBackend,
		isUsingFallback:     false,
		failureCount:        0,
		lastSuccessTime:     time.Now(),
		lastHealthCheckTime: time.Now(),
		done:                make(chan struct{}),
		logger:              logger,
		metricsCollector:    metricsCollector,
		tracer:              tracer,
	}

	// Start health check goroutine if enabled
	if config.EnableFallback {
		fm.healthCheckTicker = time.NewTicker(healthCheckInterval)
		if config.AutoRecovery {
			fm.recoveryCheckTicker = time.NewTicker(recoveryCheckInterval)
		}

		go fm.runHealthChecks()
		if config.AutoRecovery {
			go fm.runRecoveryChecks()
		}
	}

	return fm, nil
}

// OnBackendFailure handles backend operation failures
func (fm *failoverManager) OnBackendFailure(ctx context.Context, key string, err error) interface{} {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	// Increment failure counter
	newCount := atomic.AddInt64(&fm.failureCount, 1)
	fm.lastFailureTime = time.Now()

	// Log the failure
	if fm.logger != nil {
		fm.logger.Error("Backend failure", "count", newCount, "error", err)
	}

	// Record metrics - using RecordRequest for failover tracking
	if fm.metricsCollector != nil {
		fm.metricsCollector.RecordRequest(key, !fm.isUsingFallback, int64(fm.config.FailureThreshold))
	}

	// Check if we should switch to fallback
	failureThreshold := int64(fm.config.FailureThreshold)
	if failureThreshold <= 0 {
		failureThreshold = 3
	}

	if !fm.isUsingFallback && newCount >= failureThreshold {
		if fm.logger != nil {
			fm.logger.Warn("Switching to fallback backend", "count", newCount)
		}
		fm.isUsingFallback = true
		fm.currentBackend = fm.fallbackBackend
	}

	return nil
}

// GetFallbackBackend returns the fallback backend
func (fm *failoverManager) GetFallbackBackend() interface{} {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	return fm.fallbackBackend
}

// IsHealthy checks if the primary backend is healthy
func (fm *failoverManager) IsHealthy(ctx context.Context) bool {
	if fm.primaryBackend == nil {
		return false
	}

	// Simple health check: try to ping the backend
	// This is a basic implementation; more sophisticated checks can be added
	testKey := "__health_check__"
	testValue := []byte{1}

	// Try to set a test value
	if err := fm.primaryBackend.Set(ctx, testKey, testValue, 1*time.Second); err != nil {
		return false
	}

	// Try to get the test value
	if _, err := fm.primaryBackend.Get(ctx, testKey); err != nil {
		return false
	}

	return true
}

// SwitchToFallback manually switches to fallback backend
func (fm *failoverManager) SwitchToFallback(ctx context.Context) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if fm.isUsingFallback {
		return fmt.Errorf("already using fallback backend")
	}

	if fm.logger != nil {
		fm.logger.Warn("Manually switching to fallback backend")
	}
	fm.isUsingFallback = true
	fm.currentBackend = fm.fallbackBackend

	return nil
}

// SwitchToPrimary manually switches back to primary backend
func (fm *failoverManager) SwitchToPrimary(ctx context.Context) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if !fm.isUsingFallback {
		return fmt.Errorf("already using primary backend")
	}

	if fm.logger != nil {
		fm.logger.Info("Switching back to primary backend")
	}
	fm.isUsingFallback = false
	fm.currentBackend = fm.primaryBackend
	atomic.StoreInt64(&fm.failureCount, 0)
	fm.lastSuccessTime = time.Now()

	return nil
}

// GetFallbackStatus returns the current failover status
func (fm *failoverManager) GetFallbackStatus(ctx context.Context) *FailoverStatus {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	return &FailoverStatus{
		IsFallbackActive: fm.isUsingFallback,
		FailureReason: func() string {
			if fm.isUsingFallback {
				return fmt.Sprintf("Primary backend failed %d consecutive times", atomic.LoadInt64(&fm.failureCount))
			}
			return ""
		}(),
		SwitchedAt:      fm.lastFailureTime,
		FailureCount:    int(atomic.LoadInt64(&fm.failureCount)),
		LastHealthCheck: fm.lastHealthCheckTime,
	}
}

// GetCurrentBackend returns the current active backend
func (fm *failoverManager) GetCurrentBackend() backend.Backend {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	return fm.currentBackend
}

// runHealthChecks periodically checks the health of the primary backend
func (fm *failoverManager) runHealthChecks() {
	defer fm.healthCheckTicker.Stop()

	for {
		select {
		case <-fm.done:
			return
		case <-fm.healthCheckTicker.C:
			fm.performHealthCheck()
		}
	}
}

// performHealthCheck performs a single health check
func (fm *failoverManager) performHealthCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fm.mu.Lock()
	fm.lastHealthCheckTime = time.Now()
	fm.mu.Unlock()

	if !fm.IsHealthy(ctx) {
		if fm.logger != nil {
			fm.logger.Warn("Health check failed for primary backend")
		}
		fm.OnBackendFailure(ctx, "", fmt.Errorf("health check failed"))
		return
	}

	// Health check passed
	fm.mu.Lock()
	if atomic.LoadInt64(&fm.failureCount) > 0 {
		if fm.logger != nil {
			fm.logger.Info("Health check passed, resetting failure count")
		}
		atomic.StoreInt64(&fm.failureCount, 0)
	}
	fm.mu.Unlock()
}

// runRecoveryChecks periodically checks if the primary backend has recovered
func (fm *failoverManager) runRecoveryChecks() {
	if fm.recoveryCheckTicker == nil {
		return
	}
	defer fm.recoveryCheckTicker.Stop()

	for {
		select {
		case <-fm.done:
			return
		case <-fm.recoveryCheckTicker.C:
			fm.checkAndRecovery()
		}
	}
}

// checkAndRecovery checks if we should recover to primary backend
func (fm *failoverManager) checkAndRecovery() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fm.mu.RLock()
	isUsingFallback := fm.isUsingFallback
	fm.mu.RUnlock()

	if !isUsingFallback {
		return // Already using primary
	}

	if fm.IsHealthy(ctx) {
		if fm.logger != nil {
			fm.logger.Info("Primary backend recovered, switching back")
		}
		if err := fm.SwitchToPrimary(ctx); err != nil {
			if fm.logger != nil {
				fm.logger.Error("Failed to switch to primary", "error", err)
			}
		}
	}
}

// Close closes the failover manager and stops background goroutines
func (fm *failoverManager) Close() error {
	close(fm.done)

	if fm.healthCheckTicker != nil {
		fm.healthCheckTicker.Stop()
	}
	if fm.recoveryCheckTicker != nil {
		fm.recoveryCheckTicker.Stop()
	}

	// Close both backends
	var primaryErr, fallbackErr error

	if fm.primaryBackend != nil {
		if closeable, ok := fm.primaryBackend.(interface{ Close() error }); ok {
			primaryErr = closeable.Close()
		}
	}

	if fm.fallbackBackend != nil {
		if closeable, ok := fm.fallbackBackend.(interface{ Close() error }); ok {
			fallbackErr = closeable.Close()
		}
	}

	if primaryErr != nil {
		return primaryErr
	}

	return fallbackErr
}
