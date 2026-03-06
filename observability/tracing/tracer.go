package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// SpanContext represents a span in the tracing system
type SpanContext struct {
	span oteltrace.Span
	ctx  context.Context
}

// Tracer defines the interface for distributed tracing
type Tracer interface {
	// StartSpan starts a new span with the given name
	StartSpan(ctx context.Context, name string) *SpanContext

	// EndSpan ends the span and finalizes it
	EndSpan(span *SpanContext)

	// AddEvent adds an event to the span with key-value attributes
	AddEvent(span *SpanContext, event string, args ...interface{})

	// AddAttribute adds a single attribute to the span
	AddAttribute(span *SpanContext, key string, value interface{})

	// SetError marks the span as having an error
	SetError(span *SpanContext, message string)
}

// NoOpTracer is a tracer that does nothing
type NoOpTracer struct{}

// StartSpan starts a span that does nothing
func (n *NoOpTracer) StartSpan(ctx context.Context, name string) *SpanContext {
	return &SpanContext{
		span: oteltrace.SpanFromContext(ctx),
		ctx:  ctx,
	}
}

// EndSpan ends a span without doing anything
func (n *NoOpTracer) EndSpan(span *SpanContext) {}

// AddEvent adds an event without doing anything
func (n *NoOpTracer) AddEvent(span *SpanContext, event string, args ...interface{}) {}

// AddAttribute adds an attribute without doing anything
func (n *NoOpTracer) AddAttribute(span *SpanContext, key string, value interface{}) {}

// SetError marks the span as error without doing anything
func (n *NoOpTracer) SetError(span *SpanContext, message string) {}

// OpenTelemetryTracer is an OpenTelemetry-based tracer
type OpenTelemetryTracer struct {
	tracer oteltrace.Tracer
}

// NewOpenTelemetryTracer creates a new OpenTelemetry tracer
func NewOpenTelemetryTracer(name string) *OpenTelemetryTracer {
	return &OpenTelemetryTracer{
		tracer: otel.Tracer(name),
	}
}

// StartSpan starts a new span
func (ot *OpenTelemetryTracer) StartSpan(ctx context.Context, name string) *SpanContext {
	newCtx, span := ot.tracer.Start(ctx, name)
	return &SpanContext{
		span: span,
		ctx:  newCtx,
	}
}

// EndSpan ends the span
func (ot *OpenTelemetryTracer) EndSpan(span *SpanContext) {
	if span != nil && span.span != nil {
		span.span.End()
	}
}

// AddEvent adds an event to the span
func (ot *OpenTelemetryTracer) AddEvent(span *SpanContext, event string, args ...interface{}) {
	if span == nil || span.span == nil {
		return
	}

	attrs := parseTraceArgs(args)
	span.span.AddEvent(event, oteltrace.WithAttributes(attrs...))
}

// AddAttribute adds a single attribute to the span
func (ot *OpenTelemetryTracer) AddAttribute(span *SpanContext, key string, value interface{}) {
	if span == nil || span.span == nil {
		return
	}

	attr := parseTraceValue(key, value)
	span.span.SetAttributes(attr)
}

// SetError marks the span as having an error
func (ot *OpenTelemetryTracer) SetError(span *SpanContext, message string) {
	if span == nil || span.span == nil {
		return
	}

	span.span.SetStatus(codes.Error, message)
}

// parseTraceArgs converts key-value pairs to OpenTelemetry attributes
func parseTraceArgs(args []interface{}) []attribute.KeyValue {
	var attrs []attribute.KeyValue

	for i := 0; i < len(args)-1; i += 2 {
		if key, ok := args[i].(string); ok {
			attr := parseTraceValue(key, args[i+1])
			attrs = append(attrs, attr)
		}
	}

	return attrs
}

// parseTraceValue converts a key-value pair to an OpenTelemetry attribute
func parseTraceValue(key string, value interface{}) attribute.KeyValue {
	switch v := value.(type) {
	case string:
		return attribute.String(key, v)
	case int:
		return attribute.Int(key, v)
	case int64:
		return attribute.Int64(key, v)
	case float64:
		return attribute.Float64(key, v)
	case bool:
		return attribute.Bool(key, v)
	default:
		return attribute.String(key, toString(v))
	}
}

// toString converts a value to string for logging
func toString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case nil:
		return "<nil>"
	default:
		// Use fmt-like string representation
		return fmt.Sprintf("%v", v)
	}
}
