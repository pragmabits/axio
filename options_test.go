package axio

import (
	"context"
	"errors"
	"os"
	"strings"
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

func TestWithPII_Defaults(t *testing.T) {
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

func TestWithPIIMaxDepth(t *testing.T) {
	t.Run("sets_the_depth", func(t *testing.T) {
		config := minimalConfig()
		assertNoError(t, WithPIIMaxDepth(3)(&config))
		assertEqual(t, config.PIIMaxDepth, 3)
	})

	t.Run("negative_depth_returns_error", func(t *testing.T) {
		config := minimalConfig()
		if err := WithPIIMaxDepth(-1)(&config); !errors.Is(err, ErrInvalidPIIMaxDepth) {
			t.Errorf("expected ErrInvalidPIIMaxDepth, got %v", err)
		}
	})

	t.Run("limits_the_depth_the_logger_masks", func(t *testing.T) {
		output := newBufferOutput(FormatJSON)
		logger, err := New(minimalConfig(), WithOutputs(output), WithPII(nil, nil), WithPIIMaxDepth(1))
		assertNoError(t, err)
		logger.With(Annotate("order", map[string]any{"customer": map[string]any{"name": "alice"}})).Info(context.Background(), "order created")
		assertNoError(t, logger.Close())

		if !strings.Contains(output.String(), `"order":{"customer":"[REDACTED]"}`) {
			t.Errorf("expected the map at depth 2 redacted with depth 1, got %s", output.String())
		}
	})
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

// TestWithTracer_NilReturnsErr verifies that WithTracer rejects a nil tracer.
func TestWithTracer_NilReturnsErr(t *testing.T) {
	config := minimalConfig()
	_, err := New(config, WithTracer(nil))
	if !errors.Is(err, ErrNilTracer) {
		t.Errorf("expected ErrNilTracer, got %v", err)
	}
}

// TestWithOutputs_NoDoubleOpen verifies that supplying a MustFile to
// WithOutputs leaves a single file handle open and writes go to it directly.
func TestWithOutputs_NoDoubleOpen(t *testing.T) {
	path := tempFile(t, "single-open.log")
	config := Config{
		ServiceName: "t", Environment: EnvironmentDevelopment, Level: LevelInfo,
	}
	out := MustFile(path, FormatJSON)
	logger, err := New(config, WithOutputs(out))
	assertNoError(t, err)

	logger.Info(context.Background(), "hello-resolved-output")

	// Same file handle should still be writeable here — the resolved output
	// was not closed by WithOutputs.
	if _, err := out.Write([]byte(`{"manual":"write"}` + "\n")); err != nil {
		t.Errorf("resolved output should still be open: %v", err)
	}

	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	content := readFile(t, path)
	if !strings.Contains(content, "hello-resolved-output") {
		t.Errorf("missing logger write: %s", content)
	}
	if !strings.Contains(content, `"manual":"write"`) {
		t.Errorf("missing direct write to resolved output: %s", content)
	}
}

func TestWithAgentMode_OverridesEarlierOutputs(t *testing.T) {
	path := tempFile(t, "discarded.log")
	discarded, err := File(path, FormatText)
	assertNoError(t, err)

	stdout := captureStdout(t)
	logger, err := New(
		Config{ServiceName: "test", Environment: EnvironmentProduction, Level: LevelInfo},
		WithOutputs(discarded),
		WithAgentMode(),
	)
	assertNoError(t, err)
	defer logger.Close()

	logger.Info(context.Background(), "agent line")
	written := stdout()

	if !strings.Contains(written, `"message":"agent line"`) {
		t.Errorf("expected the entry as JSON on stdout, got %q", written)
	}
	assertEqual(t, readFile(t, path), "")
	if _, err := discarded.Write([]byte("late\n")); !errors.Is(err, os.ErrClosed) {
		t.Errorf("expected the discarded output to be closed, got %v", err)
	}
}

func TestWithOutputs_OverridesConfigOutputs(t *testing.T) {
	t.Run("config_outputs_are_neither_opened_nor_validated", func(t *testing.T) {
		configured := tempFile(t, "configured.log")
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: configured},
			{Type: OutputFile, Format: FormatJSON},
		}
		output := newBufferOutput(FormatJSON)

		logger, err := New(config, WithOutputs(output))
		assertNoError(t, err)
		if err != nil {
			return
		}
		logger.Info(context.Background(), "order created")
		assertNoError(t, logger.Close())

		if !strings.Contains(output.String(), `"message":"order created"`) {
			t.Errorf("expected the entry on the option's output, got %q", output.String())
		}
		if _, err := os.Stat(configured); !os.IsNotExist(err) {
			t.Errorf("the config's file output should not be opened, stat returned %v", err)
		}
	})

	t.Run("later_calls_add_outputs", func(t *testing.T) {
		first, second := newBufferOutput(FormatJSON), newBufferOutput(FormatJSON)

		logger, err := New(minimalConfig(), WithOutputs(first), WithOutputs(second))
		assertNoError(t, err)
		logger.Info(context.Background(), "order created")
		assertNoError(t, logger.Close())

		for _, output := range []*bufferOutput{first, second} {
			if !strings.Contains(output.String(), `"message":"order created"`) {
				t.Errorf("expected the entry on every output, got %q", output.String())
			}
		}
	})
}
