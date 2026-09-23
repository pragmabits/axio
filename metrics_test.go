package axio

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"
)

func TestNoopMetrics(t *testing.T) {
	metrics := NoopMetrics{}
	ctx := context.Background()

	// All methods should be callable without panic
	metrics.LogsTotal(ctx, LevelInfo)
	metrics.PIIMasked(ctx, PatternCPF)
	metrics.AuditRecords(ctx)
	metrics.HookDuration(ctx, "test", time.Millisecond, false)
	metrics.HookDuration(ctx, "test", time.Millisecond, true)
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
