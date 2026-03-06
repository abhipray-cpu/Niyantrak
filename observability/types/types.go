// Package obstypes defines the interfaces for logging, metrics, and tracing
// used throughout the rate limiter. This package has ZERO external dependencies.
//
// Use this package when your code needs to accept or store observability types
// without pulling in heavy dependencies like zerolog, prometheus, or otel.
//
// Concrete implementations live in the sibling packages:
//
//	observability/logging   — ZerologLogger (github.com/rs/zerolog)
//	observability/metrics   — PrometheusMetrics (github.com/prometheus/client_golang)
//	observability/tracing   — OpenTelemetryTracer (go.opentelemetry.io/otel)
package obstypes

import (
	"context"
)

// ============================================================================
// Logging
// ============================================================================

// Logger defines the interface for structured logging.
type Logger interface {
	// Debug logs a debug-level message with key-value pairs
	Debug(message string, args ...interface{})

	// Info logs an info-level message with key-value pairs
	Info(message string, args ...interface{})

	// Warn logs a warn-level message with key-value pairs
	Warn(message string, args ...interface{})

	// Error logs an error-level message with key-value pairs
	Error(message string, args ...interface{})
}

// NoOpLogger is a logger that does nothing (zero overhead).
type NoOpLogger struct{}

func (n *NoOpLogger) Debug(message string, args ...interface{}) {}
func (n *NoOpLogger) Info(message string, args ...interface{})  {}
func (n *NoOpLogger) Warn(message string, args ...interface{})  {}
func (n *NoOpLogger) Error(message string, args ...interface{}) {}

// ============================================================================
// Metrics
// ============================================================================

// Metrics defines the interface for rate limit metrics.
type Metrics interface {
	// RecordRequest records a rate limit decision.
	// allowed: whether the request was allowed
	// limit: the rate limit for this key
	RecordRequest(key string, allowed bool, limit int64)

	// RecordDecisionLatency records the latency of a rate limit decision in nanoseconds.
	RecordDecisionLatency(key string, latencyNs int64)

	// GetMetrics returns the current metrics state.
	GetMetrics() interface{}
}

// NoOpMetrics is a metrics implementation that does nothing (zero overhead).
type NoOpMetrics struct{}

func (n *NoOpMetrics) RecordRequest(key string, allowed bool, limit int64) {}
func (n *NoOpMetrics) RecordDecisionLatency(key string, latencyNs int64)   {}
func (n *NoOpMetrics) GetMetrics() interface{}                             { return nil }

// ============================================================================
// Tracing
// ============================================================================

// SpanContext represents a span in the tracing system.
// The Span field is an opaque interface so that this package remains free of
// external dependencies. Concrete tracer implementations store and retrieve
// their own span types via type assertion on this field.
type SpanContext struct {
	// Span holds the underlying tracer span. For OpenTelemetry this is a
	// trace.Span, but this package doesn't depend on otel.
	Span interface{}

	// Ctx holds the context associated with this span.
	Ctx context.Context
}

// Context returns the context associated with this span.
// Returns context.Background() if the span or its context is nil.
func (sc *SpanContext) Context() context.Context {
	if sc == nil || sc.Ctx == nil {
		return context.Background()
	}
	return sc.Ctx
}

// Tracer defines the interface for distributed tracing.
type Tracer interface {
	// StartSpan starts a new span with the given name
	StartSpan(ctx context.Context, name string) *SpanContext

	// EndSpan ends the span and finalises it
	EndSpan(span *SpanContext)

	// AddEvent adds an event to the span with key-value attributes
	AddEvent(span *SpanContext, event string, args ...interface{})

	// AddAttribute adds a single attribute to the span
	AddAttribute(span *SpanContext, key string, value interface{})

	// SetError marks the span as having an error
	SetError(span *SpanContext, message string)
}

// NoOpTracer is a tracer that does nothing (zero overhead).
type NoOpTracer struct{}

// StartSpan returns a SpanContext that carries the original context through.
func (n *NoOpTracer) StartSpan(ctx context.Context, name string) *SpanContext {
	return &SpanContext{Ctx: ctx}
}

func (n *NoOpTracer) EndSpan(span *SpanContext)                                     {}
func (n *NoOpTracer) AddEvent(span *SpanContext, event string, args ...interface{}) {}
func (n *NoOpTracer) AddAttribute(span *SpanContext, key string, value interface{}) {}
func (n *NoOpTracer) SetError(span *SpanContext, message string)                    {}
