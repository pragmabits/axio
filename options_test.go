package axio

import (
	"testing"

	"go.opentelemetry.io/otel/metric/noop"
)

func TestWithOutputs(t *testing.T) {
	config := minimalConfig()
	option := WithOutputs(Stdout(FormatJSON))
	err := option(&config)
	assertNoError(t, err)

	found := false
	for _, output := range config.Outputs {
		if output.Type == OutputStdout && output.Format == FormatJSON {
			found = true
		}
	}
	if !found {
		t.Error("expected stdout+json output in config")
	}
}

func TestWithAgentMode(t *testing.T) {
	config := minimalConfig()
	option := WithAgentMode()
	err := option(&config)
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
	option := WithHooks(hook)
	err := option(&config)
	assertNoError(t, err)

	assertEqual(t, len(config.hooks), 1)
}

func TestWithPII(t *testing.T) {
	config := minimalConfig()
	patterns := []PIIPattern{PatternCPF, PatternEmail}
	fields := []string{"password", "token"}
	option := WithPII(patterns, fields)
	err := option(&config)
	assertNoError(t, err)

	if !config.PIIEnabled {
		t.Error("PIIEnabled should be true")
	}
	assertEqual(t, len(config.PIIPatterns), 2)
	assertEqual(t, len(config.PIIFields), 2)
}

func TestWithPII_defaults(t *testing.T) {
	config := minimalConfig()
	option := WithPII(nil, nil)
	err := option(&config)
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
	option := WithAudit("/tmp/audit.json")
	err := option(&config)
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
		option := WithMetrics(provider)
		err := option(&config)
		assertNoError(t, err)

		if !config.Metrics.Enabled {
			t.Error("Metrics.Enabled should be true")
		}
	})

	t.Run("nil_provider_returns_error", func(t *testing.T) {
		config := minimalConfig()
		option := WithMetrics(nil)
		err := option(&config)
		assertError(t, err)
	})
}

func TestWithTracer(t *testing.T) {
	t.Run("otel_tracer", func(t *testing.T) {
		config := minimalConfig()
		option := WithTracer(Otel())
		err := option(&config)
		assertNoError(t, err)

		assertEqual(t, config.TracerType, "otel")
	})

	t.Run("noop_tracer", func(t *testing.T) {
		config := minimalConfig()
		option := WithTracer(NoopTracing())
		err := option(&config)
		assertNoError(t, err)

		assertEqual(t, config.TracerType, "noop")
	})
}
