package axio

import (
	"context"
	"fmt"
	"maps"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// PIIOrigin is where in an entry PII was masked or a value redacted: the
// annotation — "message" or "error" for the entry's own, or an annotation's key
// as the line writes it — and the name of the logger that wrote the entry,
// empty for the root logger and for events.
type PIIOrigin struct {
	Annotation string
	Logger     string
}

// Metrics defines the interface for collecting logger observability metrics.
//
// Implement this interface to integrate with metrics systems like
// Prometheus, OpenTelemetry Metrics, or other backends.
//
// Collected metrics include:
//   - Log count by level
//   - Masked PII count by pattern and origin (annotation and logger)
//   - Values redacted whole by reason and origin
//   - Audit records count
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
	// PIIMasked adds count to the counter of masked PII of the given pattern:
	// the occurrences of it masked in one entry, at origin.
	PIIMasked(ctx context.Context, pattern PIIPattern, origin PIIOrigin, count int)
	// PIIRedacted adds count to the counter of values redacted whole for reason
	// in one entry, at origin.
	PIIRedacted(ctx context.Context, reason PIIRedaction, origin PIIOrigin, count int)
	// AuditRecords increments the counter of created audit records. ctx is the
	// context of the log call or [Event.Emit] that wrote the record.
	AuditRecords(ctx context.Context)
	// HookDuration records the execution duration of a hook along with
	// whether the hook returned an error.
	HookDuration(ctx context.Context, hookName string, duration time.Duration, hasError bool)
}

// NoopMetrics is a metrics implementation that does nothing.
//
// Used as default when no metrics are configured via [WithMetrics].
type NoopMetrics struct{}

// LogsTotal does nothing.
func (NoopMetrics) LogsTotal(context.Context, Level) {}

// PIIMasked does nothing.
func (NoopMetrics) PIIMasked(context.Context, PIIPattern, PIIOrigin, int) {}

// PIIRedacted does nothing.
func (NoopMetrics) PIIRedacted(context.Context, PIIRedaction, PIIOrigin, int) {}

// AuditRecords does nothing.
func (NoopMetrics) AuditRecords(context.Context) {}

// HookDuration does nothing.
func (NoopMetrics) HookDuration(context.Context, string, time.Duration, bool) {}

// otelMetrics implements Metrics using OpenTelemetry.
type otelMetrics struct {
	logsTotal    metric.Int64Counter
	piiMasked    metric.Int64Counter
	piiRedacted  metric.Int64Counter
	auditRecords metric.Int64Counter
	hookDuration metric.Float64Histogram
	levels       optionCache[Level, metric.AddOption]
	patterns     optionCache[patternAt, metric.AddOption]
	redactions   optionCache[redactionAt, metric.AddOption]
	hooks        optionCache[hookOutcome, metric.RecordOption]
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

	piiRedacted, err := meter.Int64Counter(
		"pii.redacted",
		metric.WithDescription("Total values redacted whole"),
		metric.WithUnit("{value}"),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: pii.redacted: %w", ErrCreateMetric, err)
	}

	auditRecords, err := meter.Int64Counter(
		"audit.records",
		metric.WithDescription("Total audit records created"),
		metric.WithUnit("{entry}"),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: audit.records: %w", ErrCreateMetric, err)
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
		piiRedacted:  piiRedacted,
		auditRecords: auditRecords,
		hookDuration: hookDuration,
		levels: optionCache[Level, metric.AddOption]{build: func(level Level) []metric.AddOption {
			return []metric.AddOption{metric.WithAttributes(attribute.String("level", string(level)))}
		}},
		patterns: optionCache[patternAt, metric.AddOption]{build: func(key patternAt) []metric.AddOption {
			return []metric.AddOption{metric.WithAttributes(append(originAttributes(key.origin), attribute.String("pattern", string(key.pattern)))...)}
		}},
		redactions: optionCache[redactionAt, metric.AddOption]{build: func(key redactionAt) []metric.AddOption {
			return []metric.AddOption{metric.WithAttributes(append(originAttributes(key.origin), attribute.String("reason", string(key.reason)))...)}
		}},
		hooks: optionCache[hookOutcome, metric.RecordOption]{build: func(outcome hookOutcome) []metric.RecordOption {
			return []metric.RecordOption{metric.WithAttributes(
				attribute.String("hook.name", outcome.name),
				attribute.String("error", strconv.FormatBool(outcome.failed)),
			)}
		}},
	}, nil
}

// LogsTotal increments the log counter at the specified level.
func (o *otelMetrics) LogsTotal(ctx context.Context, level Level) {
	o.logsTotal.Add(ctx, 1, o.levels.get(level)...)
}

// PIIMasked adds count to the counter of masked PII of the given pattern.
func (o *otelMetrics) PIIMasked(ctx context.Context, pattern PIIPattern, origin PIIOrigin, count int) {
	o.piiMasked.Add(ctx, int64(count), o.patterns.get(patternAt{pattern: pattern, origin: origin})...)
}

// PIIRedacted adds count to the counter of values redacted whole for reason.
func (o *otelMetrics) PIIRedacted(ctx context.Context, reason PIIRedaction, origin PIIOrigin, count int) {
	o.piiRedacted.Add(ctx, int64(count), o.redactions.get(redactionAt{reason: reason, origin: origin})...)
}

// AuditRecords increments the counter of created audit records.
func (o *otelMetrics) AuditRecords(ctx context.Context) {
	o.auditRecords.Add(ctx, 1)
}

// HookDuration records the execution duration of a hook along with whether
// the hook returned an error.
func (o *otelMetrics) HookDuration(ctx context.Context, hookName string, duration time.Duration, hasError bool) {
	o.hookDuration.Record(ctx, duration.Seconds(), o.hooks.get(hookOutcome{name: hookName, failed: hasError})...)
}

// buildMetrics creates the Metrics object from configuration.
//
// Precedence order:
//  1. Custom implementation via private field config.metrics (legacy)
//  2. If Metrics.Enabled=false, returns NoopMetrics
//  3. If metricsProvider defined via WithMetrics(), uses it
//  4. If Metrics.Enabled=true without provider, uses otel.GetMeterProvider() with warning
func buildMetrics(config Config) (Metrics, error) {
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

// patternAt is what a pii.masked measurement is recorded under.
type patternAt struct {
	pattern PIIPattern
	origin  PIIOrigin
}

// redactionAt is what a pii.redacted measurement is recorded under.
type redactionAt struct {
	reason PIIRedaction
	origin PIIOrigin
}

// originAttributes returns the attributes of where in an entry PII was found.
func originAttributes(origin PIIOrigin) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("annotation", origin.Annotation),
		attribute.String("logger", origin.Logger),
	}
}

// hookOutcome is what a hook.duration measurement is recorded under: the hook
// and whether it failed.
type hookOutcome struct {
	name   string
	failed bool
}

// optionCache holds the measurement options of each key's attributes, built the
// first time the key is seen: building an attribute set costs more than the
// measurement it is passed to. A promoted key is read from a typed map without
// a lock. A new key waits in recent, read under the mutex, until the misses
// outnumber the promoted keys, and then recent is promoted into a new map. A
// promotion copies the map after at least as many misses as it has keys, so
// the copies cost a constant per miss however many keys arrive.
type optionCache[K comparable, O any] struct {
	promoted atomic.Pointer[map[K][]O]
	mutex    sync.Mutex
	recent   map[K][]O
	misses   int
	build    func(K) []O
}

// get returns the options for key, building them the first time.
func (o *optionCache[K, O]) get(key K) []O {
	if promoted := o.promoted.Load(); promoted != nil {
		if found, ok := (*promoted)[key]; ok {
			return found
		}
	}
	return o.miss(key)
}

// miss returns the options for a key not yet promoted, building them into
// recent the first time, and promotes recent once the misses outnumber the
// promoted keys.
func (o *optionCache[K, O]) miss(key K) []O {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	var promoted map[K][]O
	if current := o.promoted.Load(); current != nil {
		promoted = *current
	}
	if found, ok := promoted[key]; ok {
		return found
	}
	found, ok := o.recent[key]
	if !ok {
		found = o.build(key)
		if o.recent == nil {
			o.recent = make(map[K][]O)
		}
		o.recent[key] = found
	}
	o.misses++
	if o.misses > len(promoted) {
		o.promote(promoted)
	}
	return found
}

// promote replaces the promoted map with one holding its keys and recent's.
func (o *optionCache[K, O]) promote(promoted map[K][]O) {
	next := make(map[K][]O, len(promoted)+len(o.recent))
	maps.Copy(next, promoted)
	maps.Copy(next, o.recent)
	o.promoted.Store(&next)
	o.recent, o.misses = nil, 0
}
