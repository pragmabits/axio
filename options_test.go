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

	assertEqual(t, len(config.PIIPatterns), 2)
	assertEqual(t, len(config.PIIFields), 2)
}

func TestWithPII_Defaults(t *testing.T) {
	config := minimalConfig()
	option := WithPII(nil, nil)
	err := option(&config)
	assertNoError(t, err)

	// nil patterns/fields means defaults will be applied by applyDefaults
	assertEqual(t, len(config.PIIPatterns), 0)
	assertEqual(t, len(config.PIIFields), 0)
}

func TestWithPII_OverridesDisabledConfig(t *testing.T) {
	config := minimalConfig()
	config.PIIDisabled = true

	record := logCustomer(t, config, WithPII(nil, nil))
	assertEqual(t, record["message"], any("customer ***.***.***-** registered"))
	assertEqual(t, record["password"], any("[REDACTED]"))
}

func TestWithPIIDisabled(t *testing.T) {
	t.Run("sets_the_flag", func(t *testing.T) {
		config := minimalConfig()
		assertNoError(t, WithPIIDisabled()(&config))
		assertEqual(t, config.PIIDisabled, true)
	})

	t.Run("writes_personal_data_as_given", func(t *testing.T) {
		record := logCustomer(t, minimalConfig(), WithPIIDisabled())
		assertEqual(t, record["message"], any("customer 123.456.789-01 registered"))
		assertEqual(t, record["password"], any("hunter2"))
	})
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
		logger.With(Field("order", map[string]any{"customer": map[string]any{"name": "alice"}})).Info(context.Background(), "order created")
		assertNoError(t, logger.Close())

		if !strings.Contains(output.String(), `"order":{"customer":"[REDACTED]"}`) {
			t.Errorf("expected the map at depth 2 redacted with depth 1, got %s", output.String())
		}
	})
}

func TestWithPIIOmitErrorVerbose(t *testing.T) {
	t.Run("sets_the_flag", func(t *testing.T) {
		config := minimalConfig()
		assertNoError(t, WithPIIOmitErrorVerbose()(&config))
		assertEqual(t, config.PIIOmitErrorVerbose, true)
	})

	t.Run("omits_the_verbose_form_the_logger_writes", func(t *testing.T) {
		output := newBufferOutput(FormatJSON)
		logger, err := New(minimalConfig(), WithOutputs(output), WithPII(nil, nil), WithPIIOmitErrorVerbose())
		assertNoError(t, err)
		logger.Error(context.Background(), piiVerboseError{message: "rejected 123.456.789-01", stack: "at lookup"}, "registration failed")
		logger.Error(context.Background(), piiVerboseError{message: "rejected", stack: "at lookup"}, "registration failed")
		assertNoError(t, logger.Close())

		records := parseJSONLines(t, output.String())
		assertEqual(t, len(records), 2)
		assertEqual(t, records[0]["error"], any("rejected ***.***.***-**"))
		assertEqual(t, records[1]["error"], any("rejected"))
		for _, record := range records {
			if verbose, ok := record["errorVerbose"]; ok {
				t.Errorf("errorVerbose written: %v", verbose)
			}
		}
	})
}

func TestWithOmitCaller(t *testing.T) {
	writeCapturingCaller := func(t *testing.T, options ...Option) (map[string]any, string) {
		t.Helper()
		output := newBufferOutput(FormatJSON)
		var seen string
		hook := &testHook{name: "caller", process: func(_ context.Context, entry *Entry) error {
			seen = entry.Caller
			return nil
		}}
		logger, err := New(minimalConfig(), append([]Option{WithOutputs(output), WithHooks(hook)}, options...)...)
		assertNoError(t, err)
		logger.Info(context.Background(), "order created")
		assertNoError(t, logger.Close())
		return parseJSONLines(t, output.String())[0], seen
	}

	t.Run("sets_the_flag", func(t *testing.T) {
		config := minimalConfig()
		assertNoError(t, WithOmitCaller()(&config))
		assertEqual(t, config.OmitCaller, true)
	})

	t.Run("lines_and_hooks_carry_no_caller", func(t *testing.T) {
		record, seen := writeCapturingCaller(t, WithOmitCaller())
		if caller, ok := record["caller"]; ok {
			t.Errorf("caller written: %v", caller)
		}
		assertEqual(t, seen, "")
	})

	t.Run("caller_written_by_default", func(t *testing.T) {
		record, seen := writeCapturingCaller(t)
		if caller, _ := record["caller"].(string); !strings.Contains(caller, "/options_test.go:") {
			t.Errorf("caller = %q, want this test file", caller)
		}
		if !strings.Contains(seen, "options_test.go:") {
			t.Errorf("hook saw caller %q, want this test file", seen)
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

func TestWithAuditChain(t *testing.T) {
	t.Run("nil_chain_is_rejected", func(t *testing.T) {
		config := minimalConfig()
		err := WithAuditChain(nil)(&config)
		if !errors.Is(err, ErrNilAuditChain) {
			t.Errorf("expected ErrNilAuditChain, got %v", err)
		}
	})

	t.Run("enables_audit_without_a_store_path", func(t *testing.T) {
		chain, _ := NewHashChain(nil)
		config := minimalConfig()
		assertNoError(t, WithAuditChain(chain)(&config))
		assertEqual(t, config.Audit.Enabled, true)
		assertNoError(t, config.Validate())
	})
}
