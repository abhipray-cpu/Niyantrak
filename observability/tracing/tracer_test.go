package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
)

// TestTracer_Interface ensures Tracer interface compliance
func TestTracer_Interface(t *testing.T) {
	var _ Tracer = (*NoOpTracer)(nil)
}

// TestNoOpTracer_DoesNotPanic tests NoOpTracer doesn't panic
func TestNoOpTracer_DoesNotPanic(t *testing.T) {
	tracer := &NoOpTracer{}

	// Should not panic
	span := tracer.StartSpan(context.Background(), "test")
	tracer.EndSpan(span)
	tracer.AddEvent(span, "test_event", "key", "value")
	tracer.AddAttribute(span, "attr", "value")
	tracer.SetError(span, "error message")
}

// TestNoOpTracer_StartSpan returns context
func TestNoOpTracer_StartSpan(t *testing.T) {
	tracer := &NoOpTracer{}
	ctx := context.Background()

	span := tracer.StartSpan(ctx, "operation")
	if span == nil {
		t.Error("expected span context, got nil")
	}
}

// TestOpenTelemetryTracer_StartSpan tests span creation
func TestOpenTelemetryTracer_StartSpan(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")
	if tracer == nil {
		t.Error("expected tracer, got nil")
	}

	ctx := context.Background()
	span := tracer.StartSpan(ctx, "operation")

	if span == nil {
		t.Error("expected span, got nil")
	}
}

// TestOpenTelemetryTracer_EndSpan ends a span without panic
func TestOpenTelemetryTracer_EndSpan(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")
	ctx := context.Background()
	span := tracer.StartSpan(ctx, "operation")

	// Should not panic
	tracer.EndSpan(span)
}

// TestOpenTelemetryTracer_AddEvent adds event to span
func TestOpenTelemetryTracer_AddEvent(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")
	ctx := context.Background()
	span := tracer.StartSpan(ctx, "operation")

	// Should not panic
	tracer.AddEvent(span, "rate_limit_check", "key", "api-key-123", "limit", 100)

	tracer.EndSpan(span)
}

// TestOpenTelemetryTracer_AddAttribute adds attribute to span
func TestOpenTelemetryTracer_AddAttribute(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")
	ctx := context.Background()
	span := tracer.StartSpan(ctx, "operation")

	// Should not panic
	tracer.AddAttribute(span, "status", "allowed")
	tracer.AddAttribute(span, "latency_ms", 5)

	tracer.EndSpan(span)
}

// TestOpenTelemetryTracer_SetError sets error on span
func TestOpenTelemetryTracer_SetError(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")
	ctx := context.Background()
	span := tracer.StartSpan(ctx, "operation")

	// Should not panic
	tracer.SetError(span, "rate limiter unavailable")

	tracer.EndSpan(span)
}

// TestOpenTelemetryTracer_SpanLifecycle tests complete span lifecycle
func TestOpenTelemetryTracer_SpanLifecycle(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")
	ctx := context.Background()

	// Start span
	span := tracer.StartSpan(ctx, "rate_limit_check")
	if span == nil {
		t.Fatal("expected span")
	}

	// Add attributes
	tracer.AddAttribute(span, "key", "api-key-123")
	tracer.AddAttribute(span, "allowed", true)

	// Add event
	tracer.AddEvent(span, "limit_check_completed", "remaining", 45, "limit", 100)

	// End span
	tracer.EndSpan(span)
}

// TestOpenTelemetryTracer_NestedSpans tests nested span creation
func TestOpenTelemetryTracer_NestedSpans(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")
	ctx := context.Background()

	// Parent span
	parentSpan := tracer.StartSpan(ctx, "parent_operation")
	defer tracer.EndSpan(parentSpan)

	// Child span
	childSpan := tracer.StartSpan(ctx, "child_operation")
	defer tracer.EndSpan(childSpan)

	if parentSpan == nil || childSpan == nil {
		t.Error("expected both parent and child spans")
	}
}

// TestOpenTelemetryTracer_MultipleAttributes tests multiple attributes
func TestOpenTelemetryTracer_MultipleAttributes(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")
	ctx := context.Background()
	span := tracer.StartSpan(ctx, "operation")

	// Add various attribute types
	tracer.AddAttribute(span, "key", "api-key-123")
	tracer.AddAttribute(span, "limit", 100)
	tracer.AddAttribute(span, "allowed", true)
	tracer.AddAttribute(span, "latency_us", 5000)
	tracer.AddAttribute(span, "threshold", 0.8)

	tracer.EndSpan(span)
}

// TestOpenTelemetryTracer_ConcurrentSpans tests concurrent span creation
func TestOpenTelemetryTracer_ConcurrentSpans(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")

	done := make(chan bool)

	// Create spans concurrently
	for i := 0; i < 10; i++ {
		go func(id int) {
			ctx := context.Background()
			span := tracer.StartSpan(ctx, "concurrent_op")
			defer tracer.EndSpan(span)

			tracer.AddAttribute(span, "goroutine_id", id)
			tracer.AddEvent(span, "processing", "id", id)

			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestOpenTelemetryTracer_EmptyNames handles empty span names
func TestOpenTelemetryTracer_EmptyNames(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")
	ctx := context.Background()

	// Should handle empty name gracefully
	span := tracer.StartSpan(ctx, "")
	if span == nil {
		t.Error("expected span even with empty name")
	}
	tracer.EndSpan(span)
}

// BenchmarkNoOpTracer_StartSpan benchmarks noop span creation
func BenchmarkNoOpTracer_StartSpan(b *testing.B) {
	tracer := &NoOpTracer{}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracer.StartSpan(ctx, "operation")
	}
}

// BenchmarkOpenTelemetryTracer_StartSpan benchmarks OTel span creation
func BenchmarkOpenTelemetryTracer_StartSpan(b *testing.B) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		span := tracer.StartSpan(ctx, "operation")
		tracer.EndSpan(span)
	}
}

// BenchmarkOpenTelemetryTracer_AddAttribute benchmarks attribute addition
func BenchmarkOpenTelemetryTracer_AddAttribute(b *testing.B) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")
	ctx := context.Background()
	span := tracer.StartSpan(ctx, "operation")
	defer tracer.EndSpan(span)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracer.AddAttribute(span, "key", "value")
	}
}

// TestOpenTelemetryTracer_NilSpan handles nil span gracefully
func TestOpenTelemetryTracer_NilSpan(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")

	// Should not panic with nil span
	tracer.EndSpan(nil)
	tracer.AddEvent(nil, "event")
	tracer.AddAttribute(nil, "key", "value")
	tracer.SetError(nil, "error")
}

// TestOpenTelemetryTracer_AttributeTypes tests various attribute types
func TestOpenTelemetryTracer_AttributeTypes(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")
	ctx := context.Background()
	span := tracer.StartSpan(ctx, "operation")

	// Test different types
	tracer.AddAttribute(span, "string_val", "test")
	tracer.AddAttribute(span, "int_val", 42)
	tracer.AddAttribute(span, "int64_val", int64(9999))
	tracer.AddAttribute(span, "float64_val", 3.14)
	tracer.AddAttribute(span, "bool_val", true)
	tracer.AddAttribute(span, "nil_val", nil)

	tracer.EndSpan(span)
}

// TestOpenTelemetryTracer_EventWithVariousArgs tests events with various arguments
func TestOpenTelemetryTracer_EventWithVariousArgs(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")
	ctx := context.Background()
	span := tracer.StartSpan(ctx, "operation")

	// Test event with various argument types
	tracer.AddEvent(span, "test_event",
		"string", "value",
		"int", 42,
		"float", 3.14,
		"bool", true,
	)

	tracer.EndSpan(span)
}

// TestOpenTelemetryTracer_OddArgumentCount tests graceful handling of odd args
func TestOpenTelemetryTracer_OddArgumentCount(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")
	ctx := context.Background()
	span := tracer.StartSpan(ctx, "operation")

	// Should handle odd number of args gracefully
	tracer.AddEvent(span, "event", "key1", "value1", "key2")

	tracer.EndSpan(span)
}

// TestOpenTelemetryTracer_NonStringKeys tests non-string keys in arguments
func TestOpenTelemetryTracer_NonStringKeys(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")
	ctx := context.Background()
	span := tracer.StartSpan(ctx, "operation")

	// Should skip non-string keys
	tracer.AddEvent(span, "event", 123, "value", "key", "value")

	tracer.EndSpan(span)
}

// TestOpenTelemetryTracer_RaceCondition tests concurrent span operations
func TestOpenTelemetryTracer_RaceCondition(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	tracer := NewOpenTelemetryTracer("test")

	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(id int) {
			ctx := context.Background()
			span := tracer.StartSpan(ctx, "concurrent")

			for j := 0; j < 10; j++ {
				tracer.AddAttribute(span, "id", id)
				tracer.AddEvent(span, "event", "index", j)
				tracer.AddAttribute(span, "value", j*id)
			}

			tracer.EndSpan(span)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
