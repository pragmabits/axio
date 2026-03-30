---
name: tracing-metrics
description: "This skill should be used when the user asks about OpenTelemetry integration, distributed tracing, trace_id, span_id, Tracer interface, Otel(), NoopTracing, WithTracer, metrics collection, Metrics interface, WithMetrics, MeterProvider, logs.total, pii.masked, audit.records, hook.duration, NoopMetrics, MetricsAware, or observability. Trigger phrases include \"OpenTelemetry\", \"OTel\", \"tracing\", \"trace_id\", \"span_id\", \"Tracer\", \"Otel()\", \"NoopTracing\", \"WithTracer\", \"metrics\", \"WithMetrics\", \"MeterProvider\", \"Prometheus\", \"Jaeger\", \"Tempo\", \"Zipkin\", \"observability\", \"distributed tracing\", \"logs.total\", \"hook.duration\"."
---

# Tracing & Metrics

Axio integrates with OpenTelemetry for distributed tracing and metrics collection.

## Tracing
- `WithTracer(axio.Otel())` — extracts trace_id/span_id from OTel spans
- `NoopTracing()` — default, no extraction
- Tracer interface: `Extract(context.Context) (traceID, spanID string, ok bool)`

## Metrics
- `WithMetrics(provider)` — uses metric.MeterProvider
- Emitted metrics: logs.total, pii.masked, audit.records, hook.duration
- MetricsAware interface for hooks to receive metrics object
- NoopMetrics used when metrics not configured

## Custom Implementations
Implement Tracer interface for non-OTel tracing systems.
Implement Metrics interface for non-OTel metrics backends.

## Usage
Use `/axio` command for detailed tracing and metrics guidance.
