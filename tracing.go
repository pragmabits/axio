package axio

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// Tracer extracts distributed tracing information from the context.
//
// When configured via [WithTracer], the logger automatically adds
// trace_id and span_id to each log entry, enabling correlation between
// logs and traces in observability systems like Jaeger, Tempo or Zipkin.
//
// Available implementations:
//   - [Otel]: extracts from OpenTelemetry spans
//   - [NoopTracing]: never extracts information (default)
//
// Example:
//
//	logger, _ := axio.New(config, axio.WithTracer(axio.Otel()))
//
//	// In an HTTP handler with an active span
//	func handleRequest(w http.ResponseWriter, r *http.Request) {
//	    ctx := r.Context() // contains span from otel middleware
//	    logger.Info(ctx, "request received")
//	    // Log will include: {"trace_id": "abc123...", "span_id": "def456..."}
//	}
type Tracer interface {
	// Extract extracts trace_id and span_id from the context.
	// Returns (traceID, spanID, ok). If ok is false, no trace is active.
	Extract(context.Context) (string, string, bool)
}

// NoopTracer is a tracer that never extracts information.
//
// It is the default tracer when none is configured.
type NoopTracer struct{}

var _ Tracer = NoopTracer{}

// Extract always returns empty values and false.
func (n NoopTracer) Extract(ctx context.Context) (string, string, bool) {
	return "", "", false
}

// NoopTracing returns a no-op tracer that never extracts trace information.
func NoopTracing() Tracer {
	return NoopTracer{}
}

// otelTraceExtractor extracts trace information from OpenTelemetry.
type otelTraceExtractor struct{}

var _ Tracer = (*otelTraceExtractor)(nil)

// Otel returns a tracer that extracts information from OpenTelemetry spans.
//
// Use this function to integrate with applications that use OpenTelemetry
// for distributed tracing.
//
// Example:
//
//	logger, _ := axio.New(config, axio.WithTracer(axio.Otel()))
func Otel() Tracer {
	return &otelTraceExtractor{}
}

// Extract extracts trace_id and span_id from the OpenTelemetry span in the context.
func (*otelTraceExtractor) Extract(ctx context.Context) (string, string, bool) {
	span := trace.SpanContextFromContext(ctx)
	if !span.IsValid() {
		return "", "", false
	}
	return span.TraceID().String(), span.SpanID().String(), true
}

// BuildTracer creates the tracer from configuration.
//
// Precedence order:
//  1. Custom implementation via [WithTracer]
//  2. "otel": returns [Otel]
//  3. "noop" or empty: returns [NoopTracing]
//  4. Invalid value: returns [NoopTracing] as fallback
func BuildTracer(config Config) Tracer {
	if config.tracer != nil {
		return config.tracer
	}

	switch config.TracerType {
	case "otel":
		return Otel()
	case "noop", "":
		return NoopTracing()
	default:
		return NoopTracing()
	}
}
