package axio

import (
	"testing"

	"go.opentelemetry.io/otel/metric/noop"
)

func TestWithOutputs(t *testing.T) {
	config := minimalConfig()
	opt := WithOutputs(Stdout(FormatJSON))
	err := opt(&config)
	assertNoError(t, err)

	found := false
	for _, o := range config.Outputs {
		if o.Type == OutputStdout && o.Format == FormatJSON {
			found = true
		}
	}
	if !found {
		t.Error("expected stdout+json output in config")
	}
}

func TestWithAgentMode(t *testing.T) {
	config := minimalConfig()
	opt := WithAgentMode()
	err := opt(&config)
	assertNoError(t, err)

	if !config.AgentMode {
		t.Error("AgentMode should be true")
	}
	assertEqual(t, len(config.Outputs), 1)
	assertEqual(t, config.Outputs[0].Type, OutputStdout)
	assertEqual(t, config.Outputs[0].Format, FormatJSON)
}

func TestWithHooks(t *testing.T) {
	config := minimalConfig()
	hook := NoopHook()
	opt := WithHooks(hook)
	err := opt(&config)
	assertNoError(t, err)

	assertEqual(t, len(config.hooks), 1)
}

func TestWithPII(t *testing.T) {
	config := minimalConfig()
	patterns := []PIIPattern{PatternCPF, PatternEmail}
	fields := []string{"password", "token"}
	opt := WithPII(patterns, fields)
	err := opt(&config)
	assertNoError(t, err)

	if !config.PIIEnabled {
		t.Error("PIIEnabled should be true")
	}
	assertEqual(t, len(config.PIIPatterns), 2)
	assertEqual(t, len(config.PIIFields), 2)
}

func TestWithPII_defaults(t *testing.T) {
	config := minimalConfig()
	opt := WithPII(nil, nil)
	err := opt(&config)
	assertNoError(t, err)

	if !config.PIIEnabled {
		t.Error("PIIEnabled should be true")
	}
	// nil patterns/fields means defaults will be applied by applyDefaults
	assertEqual(t, len(config.PIIPatterns), 0)
	assertEqual(t, len(config.PIIFields), 0)
}

func TestWithAudit(t *testing.T) {
	config := minimalConfig()
	opt := WithAudit("/tmp/audit.json")
	err := opt(&config)
	assertNoError(t, err)

	if !config.Audit.Enabled {
		t.Error("Audit.Enabled should be true")
	}
	assertEqual(t, config.Audit.StorePath, "/tmp/audit.json")
}

func TestWithMetrics(t *testing.T) {
	t.Run("with_provider", func(t *testing.T) {
		config := minimalConfig()
		provider := noop.NewMeterProvider()
		opt := WithMetrics(provider)
		err := opt(&config)
		assertNoError(t, err)

		if !config.Metrics.Enabled {
			t.Error("Metrics.Enabled should be true")
		}
	})

	t.Run("nil_provider_returns_error", func(t *testing.T) {
		config := minimalConfig()
		opt := WithMetrics(nil)
		err := opt(&config)
		assertError(t, err)
	})
}

func TestWithTracer(t *testing.T) {
	t.Run("otel_tracer", func(t *testing.T) {
		config := minimalConfig()
		opt := WithTracer(Otel())
		err := opt(&config)
		assertNoError(t, err)

		assertEqual(t, config.TracerType, "otel")
	})

	t.Run("noop_tracer", func(t *testing.T) {
		config := minimalConfig()
		opt := WithTracer(NoopTracing())
		err := opt(&config)
		assertNoError(t, err)

		assertEqual(t, config.TracerType, "noop")
	})
}
