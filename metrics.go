package axio

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics defines the interface for collecting logger observability metrics.
//
// Implement this interface to integrate with metrics systems like
// Prometheus, OpenTelemetry Metrics, or other backends.
//
// Collected metrics include:
//   - Log count by level
//   - Masked PII count by type
//   - Audit records count
//   - Output write errors
//   - Hook execution duration
//
// Implementations must be thread-safe, as methods can be
// called concurrently from multiple goroutines.
//
// Example implementation:
//
//	type PrometheusMetrics struct {
//	    logsCounter *prometheus.CounterVec
//	    // ...
//	}
//
//	func (m *PrometheusMetrics) LogsTotal(ctx context.Context, level axio.Level) {
//	    m.logsCounter.WithLabelValues(string(level)).Inc()
//	}
type Metrics interface {
	// LogsTotal increments the log counter at the specified level.
	LogsTotal(ctx context.Context, level Level)
	// PIIMasked increments the counter when PII of the specified type is masked.
	PIIMasked(ctx context.Context, pattern PIIPattern)
	// AuditRecords increments the counter of created audit records.
	AuditRecords(ctx context.Context)
	// OutputErrors increments the counter when an output fails to write.
	OutputErrors(ctx context.Context, output OutputType)
	// HookDuration records the execution duration of a hook.
	HookDuration(ctx context.Context, hookName string, duration time.Duration)
	// HookDurationWithError records the execution duration of a hook with error status.
	HookDurationWithError(ctx context.Context, hookName string, duration time.Duration, hasError bool)
}

// NoopMetrics is a metrics implementation that does nothing.
//
// Used as default when no metrics are configured via [WithMetrics].
type NoopMetrics struct{}

// LogsTotal does nothing.
func (NoopMetrics) LogsTotal(context.Context, Level) {}

// PIIMasked does nothing.
func (NoopMetrics) PIIMasked(context.Context, PIIPattern) {}

// AuditRecords does nothing.
func (NoopMetrics) AuditRecords(context.Context) {}

// OutputErrors does nothing.
func (NoopMetrics) OutputErrors(context.Context, OutputType) {}

// HookDuration does nothing.
func (NoopMetrics) HookDuration(context.Context, string, time.Duration) {}

// HookDurationWithError does nothing.
func (NoopMetrics) HookDurationWithError(context.Context, string, time.Duration, bool) {}

// otelMetrics implements Metrics using OpenTelemetry.
type otelMetrics struct {
	logsTotal    metric.Int64Counter
	piiMasked    metric.Int64Counter
	auditRecords metric.Int64Counter
	outputErrors metric.Int64Counter
	hookDuration metric.Float64Histogram
}

// newOtelMetrics creates a new OTel metrics instance.
func newOtelMetrics(provider metric.MeterProvider, config MetricsConfig) (*otelMetrics, error) {
	meter := provider.Meter(
		config.MeterName,
		metric.WithInstrumentationVersion(config.MeterVersion),
	)

	logsTotal, err := meter.Int64Counter(
		"logs.total",
		metric.WithDescription("Total logs emitted"),
		metric.WithUnit("{record}"),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: logs.total: %w", ErrCreateMetric, err)
	}

	piiMasked, err := meter.Int64Counter(
		"pii.masked",
		metric.WithDescription("Total PII occurrences masked"),
		metric.WithUnit("{occurrence}"),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: pii.masked: %w", ErrCreateMetric, err)
	}

	auditRecords, err := meter.Int64Counter(
		"audit.records",
		metric.WithDescription("Total audit records created"),
		metric.WithUnit("{entry}"),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: audit.records: %w", ErrCreateMetric, err)
	}

	outputErrors, err := meter.Int64Counter(
		"output.errors",
		metric.WithDescription("Total output write errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: output.errors: %w", ErrCreateMetric, err)
	}

	hookDuration, err := meter.Float64Histogram(
		"hook.duration",
		metric.WithDescription("Hook execution duration"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: hook.duration: %w", ErrCreateMetric, err)
	}

	return &otelMetrics{
		logsTotal:    logsTotal,
		piiMasked:    piiMasked,
		auditRecords: auditRecords,
		outputErrors: outputErrors,
		hookDuration: hookDuration,
	}, nil
}

// LogsTotal increments the log counter at the specified level.
func (metrics *otelMetrics) LogsTotal(ctx context.Context, level Level) {
	metrics.logsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("level", string(level)),
	))
}

// PIIMasked increments the counter when PII of the specified type is masked.
func (metrics *otelMetrics) PIIMasked(ctx context.Context, pattern PIIPattern) {
	metrics.piiMasked.Add(ctx, 1, metric.WithAttributes(
		attribute.String("pattern", string(pattern)),
	))
}

// AuditRecords increments the counter of created audit records.
func (metrics *otelMetrics) AuditRecords(ctx context.Context) {
	metrics.auditRecords.Add(ctx, 1)
}

// OutputErrors increments the counter when an output fails to write.
func (metrics *otelMetrics) OutputErrors(ctx context.Context, output OutputType) {
	metrics.outputErrors.Add(ctx, 1, metric.WithAttributes(
		attribute.String("output.type", string(output)),
	))
}

// HookDuration records the execution duration of a hook.
func (metrics *otelMetrics) HookDuration(ctx context.Context, hookName string, duration time.Duration) {
	metrics.hookDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("hook.name", hookName),
	))
}

// HookDurationWithError records the execution duration of a hook with error status.
func (metrics *otelMetrics) HookDurationWithError(ctx context.Context, hookName string, duration time.Duration, hasError bool) {
	errorValue := "false"
	if hasError {
		errorValue = "true"
	}
	metrics.hookDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("hook.name", hookName),
		attribute.String("error", errorValue),
	))
}

// BuildMetrics creates the Metrics object from configuration.
//
// Precedence order:
//  1. Custom implementation via private field config.metrics (legacy)
//  2. If Metrics.Enabled=false, returns NoopMetrics
//  3. If metricsProvider defined via WithMetrics(), uses it
//  4. If Metrics.Enabled=true without provider, uses otel.GetMeterProvider() with warning
func BuildMetrics(config Config) (Metrics, error) {
	if config.metrics != nil {
		return config.metrics, nil
	}

	if !config.Metrics.Enabled {
		return NoopMetrics{}, nil
	}

	provider := config.metricsProvider
	if provider == nil {

		provider = otel.GetMeterProvider()
		fmt.Fprintf(os.Stderr, "axio: warning: metrics enabled without WithMetrics(), using global provider\n")
	}

	return newOtelMetrics(provider, config.Metrics)
}
