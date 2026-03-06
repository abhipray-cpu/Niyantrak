package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics defines the interface for rate limit metrics
type Metrics interface {
	// RecordRequest records a rate limit decision
	// allowed: whether the request was allowed
	// limit: the rate limit for this key
	RecordRequest(key string, allowed bool, limit int64)

	// RecordDecisionLatency records the latency of a rate limit decision in nanoseconds
	RecordDecisionLatency(key string, latencyNs int64)

	// GetMetrics returns the current metrics state
	GetMetrics() interface{}
}

// NoOpMetrics is a metrics implementation that does nothing
type NoOpMetrics struct{}

// RecordRequest does nothing
func (n *NoOpMetrics) RecordRequest(key string, allowed bool, limit int64) {}

// RecordDecisionLatency does nothing
func (n *NoOpMetrics) RecordDecisionLatency(key string, latencyNs int64) {}

// GetMetrics returns nil
func (n *NoOpMetrics) GetMetrics() interface{} { return nil }

// PrometheusMetrics is a Prometheus-based metrics implementation
type PrometheusMetrics struct {
	requestsTotal      prometheus.Counter
	allowedTotal       prometheus.Counter
	deniedTotal        prometheus.Counter
	decisionDuration   prometheus.Histogram
	requestsByKeyTotal prometheus.CounterVec
	allowedByKeyTotal  prometheus.CounterVec
	deniedByKeyTotal   prometheus.CounterVec
}

// NewPrometheusMetrics creates a new PrometheusMetrics with given namespace and registry
func NewPrometheusMetrics(namespace string, registry prometheus.Registerer) *PrometheusMetrics {
	pm := &PrometheusMetrics{
		requestsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "rate_limit_requests_total",
			Help:      "Total number of rate limit checks",
		}),
		allowedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "rate_limit_allowed_total",
			Help:      "Total number of allowed requests",
		}),
		deniedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "rate_limit_denied_total",
			Help:      "Total number of denied requests",
		}),
		decisionDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "rate_limit_decision_duration_seconds",
			Help:      "Rate limit decision latency in seconds",
			Buckets:   prometheus.DefBuckets,
		}),
		requestsByKeyTotal: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rate_limit_requests_by_key_total",
				Help:      "Total requests by rate limit key",
			},
			[]string{"key"},
		),
		allowedByKeyTotal: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rate_limit_allowed_by_key_total",
				Help:      "Allowed requests by rate limit key",
			},
			[]string{"key"},
		),
		deniedByKeyTotal: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rate_limit_denied_by_key_total",
				Help:      "Denied requests by rate limit key",
			},
			[]string{"key"},
		),
	}

	// Register all metrics
	registry.MustRegister(pm.requestsTotal)
	registry.MustRegister(pm.allowedTotal)
	registry.MustRegister(pm.deniedTotal)
	registry.MustRegister(pm.decisionDuration)
	registry.MustRegister(&pm.requestsByKeyTotal)
	registry.MustRegister(&pm.allowedByKeyTotal)
	registry.MustRegister(&pm.deniedByKeyTotal)

	return pm
}

// RecordRequest records a rate limit decision
func (pm *PrometheusMetrics) RecordRequest(key string, allowed bool, limit int64) {
	pm.requestsTotal.Inc()
	pm.requestsByKeyTotal.WithLabelValues(key).Inc()

	if allowed {
		pm.allowedTotal.Inc()
		pm.allowedByKeyTotal.WithLabelValues(key).Inc()
	} else {
		pm.deniedTotal.Inc()
		pm.deniedByKeyTotal.WithLabelValues(key).Inc()
	}
}

// RecordDecisionLatency records the latency of a rate limit decision
func (pm *PrometheusMetrics) RecordDecisionLatency(key string, latencyNs int64) {
	// Convert nanoseconds to seconds
	latencySeconds := float64(latencyNs) / 1_000_000_000.0
	pm.decisionDuration.Observe(latencySeconds)
}

// GetMetrics returns the current metrics
func (pm *PrometheusMetrics) GetMetrics() interface{} {
	return map[string]interface{}{
		"requests_total":    pm.requestsTotal,
		"allowed_total":     pm.allowedTotal,
		"denied_total":      pm.deniedTotal,
		"decision_duration": pm.decisionDuration,
	}
}
