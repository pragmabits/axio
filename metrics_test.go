package axio

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// measurement is one value an instrument received, with its attributes encoded.
type measurement struct {
	instrument string
	value      float64
	attributes string
}

// recordingProvider is a MeterProvider whose counters and histograms keep every
// measurement they receive.
type recordingProvider struct {
	noop.MeterProvider
	measurements *[]measurement
}

func (r recordingProvider) Meter(string, ...metric.MeterOption) metric.Meter {
	return recordingMeter{measurements: r.measurements}
}

type recordingMeter struct {
	noop.Meter
	measurements *[]measurement
}

func (r recordingMeter) Int64Counter(name string, _ ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return recordingCounter{name: name, measurements: r.measurements}, nil
}

func (r recordingMeter) Float64Histogram(name string, _ ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	return recordingHistogram{name: name, measurements: r.measurements}, nil
}

type recordingCounter struct {
	noop.Int64Counter
	name         string
	measurements *[]measurement
}

func (r recordingCounter) Add(_ context.Context, value int64, options ...metric.AddOption) {
	attributes := metric.NewAddConfig(options).Attributes()
	*r.measurements = append(*r.measurements, measurement{r.name, float64(value), attributes.Encoded(attribute.DefaultEncoder())})
}

type recordingHistogram struct {
	noop.Float64Histogram
	name         string
	measurements *[]measurement
}

func (r recordingHistogram) Record(_ context.Context, value float64, options ...metric.RecordOption) {
	attributes := metric.NewRecordConfig(options).Attributes()
	*r.measurements = append(*r.measurements, measurement{r.name, value, attributes.Encoded(attribute.DefaultEncoder())})
}

func TestOtelMetrics_Attributes(t *testing.T) {
	var measurements []measurement
	metrics, err := newOtelMetrics(recordingProvider{measurements: &measurements}, MetricsConfig{MeterName: "axio"})
	assertNoError(t, err)
	ctx := context.Background()

	metrics.LogsTotal(ctx, LevelWarn)
	metrics.LogsTotal(ctx, LevelWarn)
	metrics.PIIMasked(ctx, PatternCPF, PIIOrigin{Annotation: "document", Logger: "orders"}, 3)
	metrics.PIIMasked(ctx, PIIPattern("employee_id"), PIIOrigin{Annotation: "message"}, 1)
	metrics.PIIRedacted(ctx, RedactionField, PIIOrigin{Annotation: "password", Logger: "orders"}, 1)
	metrics.HookDuration(ctx, "pii", 2*time.Millisecond, true)
	metrics.HookDuration(ctx, "pii", time.Millisecond, false)
	metrics.AuditRecords(ctx)

	want := []measurement{
		{"logs.total", 1, "level=warn"},
		{"logs.total", 1, "level=warn"},
		{"pii.masked", 3, "annotation=document,logger=orders,pattern=cpf"},
		{"pii.masked", 1, "annotation=message,logger=,pattern=employee_id"},
		{"pii.redacted", 1, "annotation=password,logger=orders,reason=field"},
		{"hook.duration", 0.002, "error=true,hook.name=pii"},
		{"hook.duration", 0.001, "error=false,hook.name=pii"},
		{"audit.records", 1, ""},
	}
	if len(measurements) != len(want) {
		t.Fatalf("got %d measurements, want %d: %v", len(measurements), len(want), measurements)
	}
	for index := range want {
		assertEqual(t, measurements[index], want[index])
	}
}

func TestOtelMetrics_AllocatesNothing(t *testing.T) {
	metrics, err := newOtelMetrics(noop.NewMeterProvider(), MetricsConfig{MeterName: "axio"})
	assertNoError(t, err)
	ctx := context.Background()
	calls := map[string]func(){
		"logs_total": func() { metrics.LogsTotal(ctx, LevelInfo) },
		"pii_masked": func() { metrics.PIIMasked(ctx, PatternCPF, PIIOrigin{Annotation: "document", Logger: "orders"}, 2) },
		"pii_redacted": func() {
			metrics.PIIRedacted(ctx, RedactionToken, PIIOrigin{Annotation: "session", Logger: "orders"}, 1)
		},
		"hook_duration": func() { metrics.HookDuration(ctx, "pii", time.Millisecond, false) },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			call()
			if allocations := testing.AllocsPerRun(100, call); allocations != 0 {
				t.Errorf("allocated %v times per call, want 0", allocations)
			}
		})
	}
}

func TestNoopMetrics(t *testing.T) {
	metrics := NoopMetrics{}
	ctx := context.Background()

	// All methods should be callable without panic
	metrics.LogsTotal(ctx, LevelInfo)
	metrics.PIIMasked(ctx, PatternCPF, PIIOrigin{Annotation: "document"}, 1)
	metrics.PIIRedacted(ctx, RedactionField, PIIOrigin{Annotation: "password"}, 1)
	metrics.AuditRecords(ctx)
	metrics.HookDuration(ctx, "test", time.Millisecond, false)
	metrics.HookDuration(ctx, "test", time.Millisecond, true)
}

func TestBuildMetrics(t *testing.T) {
	t.Run("noop_when_disabled", func(t *testing.T) {
		config := minimalConfig()
		config.Metrics.Enabled = false

		metrics, err := buildMetrics(config)
		assertNoError(t, err)

		_, ok := metrics.(NoopMetrics)
		if !ok {
			t.Error("should return NoopMetrics when disabled")
		}
	})

	t.Run("custom_metrics", func(t *testing.T) {
		config := minimalConfig()
		config.metrics = NoopMetrics{}

		metrics, err := buildMetrics(config)
		assertNoError(t, err)

		_, ok := metrics.(NoopMetrics)
		if !ok {
			t.Error("should return custom metrics implementation")
		}
	})

	t.Run("with_provider", func(t *testing.T) {
		config := minimalConfig()
		config.Metrics.Enabled = true
		config.metricsProvider = noop.NewMeterProvider()

		metrics, err := buildMetrics(config)
		assertNoError(t, err)

		_, ok := metrics.(*otelMetrics)
		if !ok {
			t.Error("should return otelMetrics when provider is set")
		}
	})
}

func TestOtelMetrics_ConcurrentKeys(t *testing.T) {
	metrics, err := newOtelMetrics(noop.NewMeterProvider(), MetricsConfig{MeterName: "axio"})
	assertNoError(t, err)
	ctx := context.Background()
	levels := []Level{LevelDebug, LevelInfo, LevelWarn, LevelError}
	patterns := []PIIPattern{PatternCPF, PatternCNPJ, PatternEmail, PIIPattern("employee_id")}

	var group sync.WaitGroup
	for worker := range 8 {
		group.Go(func() {
			for index := range 200 {
				metrics.LogsTotal(ctx, levels[(worker+index)%len(levels)])
				metrics.PIIMasked(ctx, patterns[(worker+index)%len(patterns)], PIIOrigin{Annotation: "document"}, 1)
				metrics.HookDuration(ctx, "pii", time.Millisecond, index%2 == 0)
			}
		})
	}
	group.Wait()

	assertEqual(t, len(*metrics.levels.options.Load()), len(levels))
	assertEqual(t, len(*metrics.patterns.options.Load()), len(patterns))
	assertEqual(t, len(*metrics.hooks.options.Load()), 2)
}
