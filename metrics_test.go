package axio

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"
)

func TestNoopMetrics(t *testing.T) {
	m := NoopMetrics{}
	ctx := context.Background()

	// All methods should be callable without panic
	m.LogsTotal(ctx, LevelInfo)
	m.PIIMasked(ctx, PatternCPF)
	m.AuditRecords(ctx)
	m.HookDuration(ctx, "test", time.Millisecond, false)
	m.HookDuration(ctx, "test", time.Millisecond, true)
}

func Test_buildMetrics_noop_when_disabled(t *testing.T) {
	config := minimalConfig()
	config.Metrics.Enabled = false

	metrics, err := buildMetrics(config)
	assertNoError(t, err)

	_, ok := metrics.(NoopMetrics)
	if !ok {
		t.Error("should return NoopMetrics when disabled")
	}
}

func Test_buildMetrics_custom_metrics(t *testing.T) {
	config := minimalConfig()
	config.metrics = NoopMetrics{}

	metrics, err := buildMetrics(config)
	assertNoError(t, err)

	_, ok := metrics.(NoopMetrics)
	if !ok {
		t.Error("should return custom metrics implementation")
	}
}

func Test_buildMetrics_with_provider(t *testing.T) {
	config := minimalConfig()
	config.Metrics.Enabled = true
	config.metricsProvider = noop.NewMeterProvider()

	metrics, err := buildMetrics(config)
	assertNoError(t, err)

	_, ok := metrics.(*otelMetrics)
	if !ok {
		t.Error("should return otelMetrics when provider is set")
	}
}
