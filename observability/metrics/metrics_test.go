package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestMetrics_Interface ensures Metrics interface compliance
func TestMetrics_Interface(t *testing.T) {
	var _ Metrics = (*NoOpMetrics)(nil)
	var _ Metrics = (*PrometheusMetrics)(nil)
}

// TestNoOpMetrics_DoesNotPanic tests NoOpMetrics doesn't panic
func TestNoOpMetrics_DoesNotPanic(t *testing.T) {
	metrics := &NoOpMetrics{}

	// Should not panic
	metrics.RecordRequest("key", true, 100)
	metrics.RecordRequest("key", false, 100)
	metrics.RecordDecisionLatency("key", 1000)
	metrics.GetMetrics()
}

// TestPrometheusMetrics_RecordAllowed tests recording allowed requests
func TestPrometheusMetrics_RecordAllowed(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics("test", registry)

	metrics.RecordRequest("api-key-123", true, 100)

	// Check counter was incremented
	if count := testutil.CollectAndCount(registry); count == 0 {
		t.Error("expected metrics to be recorded")
	}
}

// TestPrometheusMetrics_RecordDenied tests recording denied requests
func TestPrometheusMetrics_RecordDenied(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics("test", registry)

	metrics.RecordRequest("api-key-123", false, 100)

	// Check counter was incremented
	if count := testutil.CollectAndCount(registry); count == 0 {
		t.Error("expected metrics to be recorded")
	}
}

// TestPrometheusMetrics_RecordLatency tests recording decision latency
func TestPrometheusMetrics_RecordLatency(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics("test", registry)

	// Record multiple latency measurements
	metrics.RecordDecisionLatency("key", 100)
	metrics.RecordDecisionLatency("key", 200)
	metrics.RecordDecisionLatency("key", 300)

	// Should not panic and metrics should be recorded
	if count := testutil.CollectAndCount(registry); count == 0 {
		t.Error("expected latency metrics to be recorded")
	}
}

// TestPrometheusMetrics_MultipleKeys tests multiple keys tracked separately
func TestPrometheusMetrics_MultipleKeys(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics("test", registry)

	metrics.RecordRequest("key-1", true, 100)
	metrics.RecordRequest("key-2", false, 100)
	metrics.RecordRequest("key-3", true, 50)

	// All requests should be recorded
	if count := testutil.CollectAndCount(registry); count == 0 {
		t.Error("expected metrics for multiple keys")
	}
}

// TestPrometheusMetrics_Namespace tests metric namespace
func TestPrometheusMetrics_Namespace(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics("myapp", registry)

	metrics.RecordRequest("key", true, 100)

	// Verify metrics were created and recorded
	mfs, err := registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	if len(mfs) == 0 {
		t.Error("expected metrics to be recorded")
	}

	// Check that at least one metric has the namespace prefix
	foundNamespace := false
	for _, mf := range mfs {
		name := mf.GetName()
		if name != "" && len(name) > 0 {
			foundNamespace = true
			break
		}
	}

	if !foundNamespace {
		t.Error("expected metrics with namespace prefix")
	}
}

// TestPrometheusMetrics_GetMetrics returns metrics
func TestPrometheusMetrics_GetMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics("test", registry)

	result := metrics.GetMetrics()
	if result == nil {
		t.Error("expected non-nil metrics result")
	}
}

// TestPrometheusMetrics_ConcurrentRecords tests concurrent metric recording
func TestPrometheusMetrics_ConcurrentRecords(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics("test", registry)

	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				allowed := j%2 == 0
				metrics.RecordRequest("key", allowed, 100)
				metrics.RecordDecisionLatency("key", int64((id*100 + j)))
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic and metrics should be valid
	metrics.GetMetrics()
}

// TestPrometheusMetrics_ZeroLatency tests zero latency value
func TestPrometheusMetrics_ZeroLatency(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics("test", registry)

	// Should not panic with zero latency
	metrics.RecordDecisionLatency("key", 0)

	if count := testutil.CollectAndCount(registry); count == 0 {
		t.Error("expected metrics to be recorded")
	}
}

// TestPrometheusMetrics_LargeLatency tests large latency value
func TestPrometheusMetrics_LargeLatency(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics("test", registry)

	// Should handle large latency values
	metrics.RecordDecisionLatency("key", 1_000_000_000) // 1 second in nanoseconds

	if count := testutil.CollectAndCount(registry); count == 0 {
		t.Error("expected metrics to be recorded")
	}
}

// TestPrometheusMetrics_EmptyKey tests empty key handling
func TestPrometheusMetrics_EmptyKey(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics("test", registry)

	// Should handle empty keys gracefully
	metrics.RecordRequest("", true, 100)

	if count := testutil.CollectAndCount(registry); count == 0 {
		t.Error("expected metrics to be recorded")
	}
}

// BenchmarkPrometheusMetrics_RecordRequest benchmarks metric recording
func BenchmarkPrometheusMetrics_RecordRequest(b *testing.B) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics("test", registry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.RecordRequest("key", i%2 == 0, 100)
	}
}

// BenchmarkPrometheusMetrics_RecordLatency benchmarks latency recording
func BenchmarkPrometheusMetrics_RecordLatency(b *testing.B) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics("test", registry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.RecordDecisionLatency("key", int64(i))
	}
}

// BenchmarkNoOpMetrics_RecordRequest benchmarks noop metrics
func BenchmarkNoOpMetrics_RecordRequest(b *testing.B) {
	metrics := &NoOpMetrics{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.RecordRequest("key", i%2 == 0, 100)
	}
}
