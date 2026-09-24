package axio

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	t.Run("minimal_config", func(t *testing.T) {
		config := minimalConfig()
		logger, err := New(config)
		assertNoError(t, err)
		defer logger.Close()

		if logger == nil {
			t.Error("logger should not be nil")
		}
	})

	t.Run("with_file_output", func(t *testing.T) {
		path := tempFile(t, "test.log")
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		}

		logger, err := New(config)
		assertNoError(t, err)
		defer logger.Close()

		logger.Info(context.Background(), "test message")

		content := readFile(t, path)
		if !strings.Contains(content, "test message") {
			t.Error("log should contain the message")
		}
	})

	t.Run("invalid_environment_fails", func(t *testing.T) {
		config := minimalConfig()
		config.Environment = "invalid"

		_, err := New(config)
		assertError(t, err)
		if !errors.Is(err, ErrValidateConfig) {
			t.Errorf("expected ErrValidateConfig, got %v", err)
		}
	})

	t.Run("invalid_level_fails", func(t *testing.T) {
		config := minimalConfig()
		config.Level = "invalid"

		_, err := New(config)
		assertError(t, err)
	})

	t.Run("file_output_without_path_fails", func(t *testing.T) {
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputFile, Format: FormatJSON}, // without path
		}

		_, err := New(config)
		assertError(t, err)
	})

	t.Run("production_environment_adds_service_metadata", func(t *testing.T) {
		path := tempFile(t, "prod.log")
		config := Config{
			ServiceName:    "test-service",
			ServiceVersion: "1.0.0",
			Environment:    EnvironmentProduction,
			Level:          LevelInfo,
			Outputs: []OutputConfig{
				{Type: OutputFile, Format: FormatJSON, Path: path},
			},
		}

		logger, err := New(config)
		assertNoError(t, err)
		logger.Info(context.Background(), "test")
		logger.Close()

		content := readFile(t, path)
		if !strings.Contains(content, "service") {
			t.Error("production log should contain service metadata")
		}
	})
}

func TestLogger_Levels(t *testing.T) {
	path := tempFile(t, "levels.log")
	config := Config{
		ServiceName: "test",
		Environment: EnvironmentDevelopment,
		Level:       LevelDebug, // allows all levels
		Outputs: []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		},
	}

	logger, err := New(config)
	assertNoError(t, err)
	defer logger.Close()

	ctx := context.Background()

	t.Run("debug", func(t *testing.T) {
		logger.Debug(ctx, "debug message")
	})

	t.Run("info", func(t *testing.T) {
		logger.Info(ctx, "info message")
	})

	t.Run("warn", func(t *testing.T) {
		logger.Warn(ctx, errors.New("warn error"), "warn message")
	})

	t.Run("error", func(t *testing.T) {
		logger.Error(ctx, errors.New("test error"), "error message")
	})

	content := readFile(t, path)

	levels := []string{"debug", "info", "warn", "error"}
	for _, level := range levels {
		if !strings.Contains(content, level) {
			t.Errorf("log should contain level %s", level)
		}
	}
}

func TestLogger_LevelFiltering(t *testing.T) {
	path := tempFile(t, "filtered.log")
	config := Config{
		ServiceName: "test",
		Environment: EnvironmentDevelopment,
		Level:       LevelWarn, // ignores debug and info
		Outputs: []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		},
	}

	logger, err := New(config)
	assertNoError(t, err)

	ctx := context.Background()
	logger.Debug(ctx, "should be ignored")
	logger.Info(ctx, "should be ignored too")
	logger.Warn(ctx, nil, "should appear")
	logger.Close()

	content := readFile(t, path)

	if strings.Contains(content, "should be ignored") {
		t.Error("debug and info should be filtered")
	}
	if !strings.Contains(content, "should appear") {
		t.Error("warn should appear")
	}
}

func TestLogger_Named(t *testing.T) {
	path := tempFile(t, "named.log")
	config := Config{
		ServiceName: "test",
		Environment: EnvironmentDevelopment,
		Level:       LevelInfo,
		Outputs: []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		},
	}

	logger, err := New(config)
	assertNoError(t, err)
	defer logger.Close()

	namedLogger := logger.Named("submodule")
	namedLogger.Info(context.Background(), "named log")

	content := readFile(t, path)
	if !strings.Contains(content, "submodule") {
		t.Error("log should contain logger name")
	}
}

func TestLogger_With(t *testing.T) {
	path := tempFile(t, "with.log")
	config := Config{
		ServiceName: "test",
		Environment: EnvironmentDevelopment,
		Level:       LevelInfo,
		Outputs: []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		},
	}

	logger, err := New(config)
	assertNoError(t, err)
	defer logger.Close()

	logger.With(
		Field("user_id", "usr_123"),
		Field("tenant", "acme"),
	).Info(context.Background(), "with annotations")

	content := readFile(t, path)
	if !strings.Contains(content, "user_id") {
		t.Error("log should contain user_id annotation")
	}
	if !strings.Contains(content, "usr_123") {
		t.Error("log should contain annotation value")
	}
}

func TestLogger_WithHTTP(t *testing.T) {
	path := tempFile(t, "http.log")
	config := Config{
		ServiceName: "test",
		Environment: EnvironmentDevelopment,
		Level:       LevelInfo,
		Outputs: []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		},
	}

	logger, err := New(config)
	assertNoError(t, err)
	defer logger.Close()

	httpAnnotation := HTTP{
		Method:     "POST",
		URL:        "/api/users",
		StatusCode: 201,
		LatencyMS:  45,
	}

	logger.With(Field("http", httpAnnotation)).Info(context.Background(), "http request")

	content := readFile(t, path)
	if !strings.Contains(content, "POST") {
		t.Error("log should contain HTTP method")
	}
	if !strings.Contains(content, "/api/users") {
		t.Error("log should contain URL")
	}
}

func TestLogger_LevelAnnotations(t *testing.T) {
	ctx := context.Background()
	newLogger := func(t *testing.T, options ...Option) (Logger, *bufferOutput) {
		t.Helper()
		output := newBufferOutput(FormatJSON)
		config := minimalConfig()
		config.Level = LevelDebug
		loggerUnderTest, err := New(config, append([]Option{WithOutputs(output)}, options...)...)
		assertNoError(t, err)
		t.Cleanup(func() { _ = loggerUnderTest.Close() })
		return loggerUnderTest, output
	}

	t.Run("written_as_fields_at_every_level", func(t *testing.T) {
		loggerUnderTest, output := newLogger(t)
		cause := errors.New("declined")
		loggerUnderTest.Debug(ctx, "order created", Field("user_id", "usr_1"))
		loggerUnderTest.Info(ctx, "order created", Field("user_id", "usr_1"))
		loggerUnderTest.Warn(ctx, cause, "order created", Field("user_id", "usr_1"))
		loggerUnderTest.Error(ctx, cause, "order created", Field("user_id", "usr_1"))

		records := parseJSONLines(t, output.String())
		assertEqual(t, len(records), 4)
		for _, record := range records {
			assertEqual(t, record["message"], any("order created"))
			assertEqual(t, record["user_id"], any("usr_1"))
		}
	})

	t.Run("message_written_as_given", func(t *testing.T) {
		loggerUnderTest, output := newLogger(t)
		loggerUnderTest.Info(ctx, "rate at 100%d", Field("rate", 5))

		records := parseJSONLines(t, output.String())
		assertEqual(t, records[0]["message"], any("rate at 100%d"))
	})

	t.Run("written_after_the_logger_own", func(t *testing.T) {
		loggerUnderTest, output := newLogger(t)
		loggerUnderTest.With(Field("request_id", "req_1")).Info(ctx, "order created", Field("user_id", "usr_1"))

		line := output.String()
		requestAt, userAt := strings.Index(line, `"request_id"`), strings.Index(line, `"user_id"`)
		if requestAt < 0 || userAt < 0 || requestAt > userAt {
			t.Errorf("want request_id before user_id, got %s", line)
		}
	})

	t.Run("not_kept_by_the_logger", func(t *testing.T) {
		loggerUnderTest, output := newLogger(t)
		child := loggerUnderTest.With(Field("request_id", "req_1"))
		child.Info(ctx, "first", Field("user_id", "usr_1"))
		child.Info(ctx, "second")

		records := parseJSONLines(t, output.String())
		assertEqual(t, len(records), 2)
		assertEqual(t, records[0]["user_id"], any("usr_1"))
		if _, ok := records[1]["user_id"]; ok {
			t.Error("an annotation passed to one call reached the next")
		}
		assertEqual(t, records[1]["request_id"], any("req_1"))
	})

	t.Run("masked_by_pii", func(t *testing.T) {
		loggerUnderTest, output := newLogger(t, WithPII(nil, nil))
		loggerUnderTest.Info(ctx, "order created", Field("document", "123.456.789-01"))

		records := parseJSONLines(t, output.String())
		assertEqual(t, records[0]["document"], any("***.***.***-**"))
	})
	t.Run("caller_slice_not_changed_by_hooks", func(t *testing.T) {
		loggerUnderTest, _ := newLogger(t, WithPII(nil, nil))
		annotations := []Annotation{Field("document", "123.456.789-01")}
		loggerUnderTest.Info(ctx, "order created", annotations...)

		assertEqual(t, annotations[0].Data(), any("123.456.789-01"))
	})
}

func TestLogger_WithNilAnnotation(t *testing.T) {
	config := minimalConfig()
	logger, err := New(config)
	assertNoError(t, err)
	defer logger.Close()

	// Should not panic with empty annotations
	logger.With().Info(context.Background(), "test")
}

func TestLogger_Close(t *testing.T) {
	t.Run("closes_file_outputs", func(t *testing.T) {
		path := tempFile(t, "close.log")
		config := Config{
			ServiceName: "test",
			Environment: EnvironmentDevelopment,
			Level:       LevelInfo,
			Outputs: []OutputConfig{
				{Type: OutputFile, Format: FormatJSON, Path: path},
			},
		}

		logger, _ := New(config)
		err := logger.Close()
		assertNoError(t, err)
	})

	t.Run("close_is_idempotent", func(t *testing.T) {
		config := minimalConfig()
		logger, _ := New(config)

		logger.Close()
		// Second call should not cause a serious error
		logger.Close()
	})
}

func TestLogger_OmitsEmptyTraceAndError(t *testing.T) {
	path := tempFile(t, "omit.log")
	config := Config{
		ServiceName: "test",
		Environment: EnvironmentDevelopment,
		Level:       LevelDebug,
		Outputs: []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		},
	}

	logger, err := New(config)
	assertNoError(t, err)
	defer logger.Close()

	ctx := context.Background()

	t.Run("no_trace_no_error_omits_fields", func(t *testing.T) {
		logger.Info(ctx, "clean message")

		content := readFile(t, path)
		if strings.Contains(content, `"trace_id"`) {
			t.Error("trace_id should be omitted when empty")
		}
		if strings.Contains(content, `"span_id"`) {
			t.Error("span_id should be omitted when empty")
		}
		// "error" key from zap.Error should not appear when nil
		// Check that there's no "error":null in the output
		if strings.Contains(content, `"error"`) {
			t.Error("error should be omitted when nil")
		}
	})

	t.Run("error_present_when_set", func(t *testing.T) {
		logger.Error(ctx, errors.New("something broke"), "error message")

		content := readFile(t, path)
		if !strings.Contains(content, `"error"`) {
			t.Error("error field should be present when error is set")
		}
		if !strings.Contains(content, "something broke") {
			t.Error("error value should appear in log")
		}
	})
}

func TestLogger_ServiceMetadataOnlyInJSON(t *testing.T) {
	text, structured := newBufferOutput(FormatText), newBufferOutput(FormatJSON)
	logger, err := New(
		Config{ServiceName: "checkout", ServiceVersion: "1.4.2", Environment: EnvironmentProduction, Level: LevelInfo},
		WithOutputs(text, structured),
	)
	assertNoError(t, err)
	logger.Info(context.Background(), "order created")
	assertNoError(t, logger.Close())

	for _, key := range []string{`"service"`, `"deployment"`} {
		if strings.Contains(text.String(), key) {
			t.Errorf("text line should not carry %s: %s", key, text.String())
		}
		if !strings.Contains(structured.String(), key) {
			t.Errorf("JSON line should carry %s: %s", key, structured.String())
		}
	}
}

func TestLogger_HookSeesCallerAsWritten(t *testing.T) {
	output := newBufferOutput(FormatJSON)
	var seen string
	hook := &testHook{name: "caller", process: func(_ context.Context, entry *Entry) error {
		seen = entry.Caller
		return nil
	}}
	logger, err := New(minimalConfig(), WithOutputs(output), WithHooks(hook))
	assertNoError(t, err)
	logger.Info(context.Background(), "order created")
	assertNoError(t, logger.Close())

	assertEqual(t, any(seen), parseJSONLines(t, output.String())[0]["caller"])
}

func TestReportingCore_Write(t *testing.T) {
	config := Config{ServiceName: "checkout", Environment: EnvironmentProduction, Level: LevelInfo}
	failingChain, err := NewHashChain(nil)
	assertNoError(t, err)
	failingChain.store = &mockFailingStore{}

	tests := []struct {
		name  string
		write func(t *testing.T)
		cause string
	}{
		{
			name: "logger",
			write: func(t *testing.T) {
				logger, err := New(config, WithOutputs(failingOutput{format: FormatJSON}))
				assertNoError(t, err)
				logger.Info(context.Background(), "order created")
			},
			cause: "disk full",
		},
		{
			name: "audited_logger",
			write: func(t *testing.T) {
				logger, err := New(config, WithOutputs(newBufferOutput(FormatJSON)), WithAuditChain(failingChain))
				assertNoError(t, err)
				logger.Info(context.Background(), "order created")
			},
			cause: "persist chain state",
		},
		{
			name: "event",
			write: func(t *testing.T) {
				event, err := NewEvent("checkout", config, WithOutputs(failingOutput{format: FormatJSON}))
				assertNoError(t, err)
				event.Emit(context.Background())
			},
			cause: "disk full",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stderr := captureStderr(t)
			test.write(t)
			written := stderr()

			if !strings.HasPrefix(written, "axio: write error: ") || strings.Count(written, "\n") != 1 {
				t.Errorf("expected one line starting with %q, got %q", "axio: write error: ", written)
			}
			if !strings.Contains(written, test.cause) {
				t.Errorf("expected the cause %q in %q", test.cause, written)
			}
		})
	}
}

func TestAnnotationsToFields_ReservedKeyIsRenamed(t *testing.T) {
	reserved := []string{
		"timestamp", "level", "message", "logger", "caller", "stacktrace",
		"service", "deployment", "trace_id", "span_id",
		"error", "errorVerbose", "errorCauses",
		"event", "duration_ms", "previous_hash", "hash",
	}
	annotations := make([]Annotation, len(reserved))
	for index, key := range reserved {
		annotations[index] = Field(key, "user")
	}
	config := Config{ServiceName: "checkout", Environment: EnvironmentProduction, Level: LevelInfo}

	logged := newBufferOutput(FormatJSON)
	chain, err := NewHashChain(nil)
	assertNoError(t, err)
	logger, err := New(config, WithOutputs(logged), WithAuditChain(chain))
	assertNoError(t, err)
	logger.Named("orders").With(annotations...).Error(context.Background(), errors.New("card declined"), "payment failed")
	assertNoError(t, logger.Close())

	emitted := newBufferOutput(FormatJSON)
	event, err := NewEvent("checkout", config, WithOutputs(emitted))
	assertNoError(t, err)
	event.With(annotations...)
	event.SetError(errors.New("card declined"))
	event.Emit(context.Background())
	assertNoError(t, event.Close())

	for name, line := range map[string]string{"logger": logged.String(), "event": emitted.String()} {
		for _, key := range reserved {
			if !strings.Contains(line, `"_`+key+`":"user"`) || strings.Contains(line, `"`+key+`":"user"`) {
				t.Errorf("%s: the annotation %q should be written as %q only:\n%s", name, key, "_"+key, line)
			}
		}
	}
}

// TestLogger_With_AccumulatesAcrossChain verifies that chained With() calls
// preserve previously-attached annotations.
func TestLogger_With_AccumulatesAcrossChain(t *testing.T) {
	path := tempFile(t, "chain.log")
	config := Config{
		ServiceName: "t", Environment: EnvironmentDevelopment, Level: LevelInfo,
		Outputs: []OutputConfig{{Type: OutputFile, Format: FormatJSON, Path: path}},
	}
	logger, err := New(config)
	assertNoError(t, err)
	defer logger.Close()

	logger.
		With(Field("first", "A")).
		With(Field("second", "B")).
		Info(context.Background(), "chained")

	content := readFile(t, path)
	if !strings.Contains(content, `"first":"A"`) {
		t.Errorf("expected first=A in chained log, got %s", content)
	}
	if !strings.Contains(content, `"second":"B"`) {
		t.Errorf("expected second=B in chained log, got %s", content)
	}
}

// TestLogger_With_DoesNotMutateParent verifies that deriving children from a
// shared parent doesn't bleed annotations between siblings or back into parent.
func TestLogger_With_DoesNotMutateParent(t *testing.T) {
	path := tempFile(t, "siblings.log")
	config := Config{
		ServiceName: "t", Environment: EnvironmentDevelopment, Level: LevelInfo,
		Outputs: []OutputConfig{{Type: OutputFile, Format: FormatJSON, Path: path}},
	}
	logger, err := New(config)
	assertNoError(t, err)
	defer logger.Close()

	parent := logger.With(Field("a", 1))
	child1 := parent.With(Field("b", 2))
	child2 := parent.With(Field("c", 3))

	ctx := context.Background()
	parent.Info(ctx, "parent_first")
	child1.Info(ctx, "child1")
	child2.Info(ctx, "child2")
	parent.Info(ctx, "parent_second")

	lines := strings.Split(strings.TrimSpace(readFile(t, path)), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d: %s", len(lines), lines)
	}
	if strings.Contains(lines[0], `"b"`) || strings.Contains(lines[0], `"c"`) {
		t.Error("parent_first leaked sibling annotations")
	}
	if !strings.Contains(lines[1], `"a":1`) || !strings.Contains(lines[1], `"b":2`) {
		t.Error("child1 missing a or b")
	}
	if strings.Contains(lines[2], `"b"`) {
		t.Error("child2 leaked b from sibling")
	}
	if strings.Contains(lines[3], `"b"`) || strings.Contains(lines[3], `"c"`) {
		t.Error("parent_second was mutated by children")
	}
}

// TestLogger_Named_PreservesAnnotations verifies that Named keeps previously
// attached annotations rather than clearing them.
func TestLogger_Named_PreservesAnnotations(t *testing.T) {
	path := tempFile(t, "named.log")
	config := Config{
		ServiceName: "t", Environment: EnvironmentDevelopment, Level: LevelInfo,
		Outputs: []OutputConfig{{Type: OutputFile, Format: FormatJSON, Path: path}},
	}
	logger, err := New(config)
	assertNoError(t, err)
	defer logger.Close()

	logger.
		With(Field("user_id", "u-1")).
		Named("sub").
		Info(context.Background(), "after named")

	content := readFile(t, path)
	if !strings.Contains(content, `"user_id":"u-1"`) {
		t.Errorf("Named should preserve With annotations, got: %s", content)
	}
	if !strings.Contains(content, `"logger":"sub"`) {
		t.Errorf("Named should attach logger name, got: %s", content)
	}
}

// TestLogger_Close_ReturnsSentinel verifies the second Close returns
// ErrLoggerClosed and that errors.Is recognises it.
func TestLogger_Close_ReturnsSentinel(t *testing.T) {
	path := tempFile(t, "sentinel.log")
	config := Config{
		ServiceName: "t", Environment: EnvironmentDevelopment, Level: LevelInfo,
		Outputs: []OutputConfig{{Type: OutputFile, Format: FormatJSON, Path: path}},
	}
	logger, err := New(config)
	assertNoError(t, err)

	if err := logger.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	err = logger.Close()
	if !errors.Is(err, ErrLoggerClosed) {
		t.Errorf("second Close should return ErrLoggerClosed, got %v", err)
	}
}

// TestLogger_LogIsNoopAfterClose verifies that logging after Close doesn't
// panic and silently drops the call.
func TestLogger_LogIsNoopAfterClose(t *testing.T) {
	path := tempFile(t, "after-close.log")
	config := Config{
		ServiceName: "t", Environment: EnvironmentDevelopment, Level: LevelInfo,
		Outputs: []OutputConfig{{Type: OutputFile, Format: FormatJSON, Path: path}},
	}
	logger, err := New(config)
	assertNoError(t, err)
	logger.Info(context.Background(), "before-close")
	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Must NOT panic.
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Errorf("log after Close panicked: %v", recovered)
		}
	}()
	logger.Info(context.Background(), "after-close-should-be-dropped")

	content := readFile(t, path)
	if strings.Contains(content, "after-close-should-be-dropped") {
		t.Errorf("log after Close should be dropped, got: %s", content)
	}
}

// TestLogger_CloseOnFork_DoesNotAffectRoot verifies that calling Close on a
// logger produced by With or Named returns ErrLoggerNotRoot and leaves the
// root logger fully functional.
func TestLogger_CloseOnFork_DoesNotAffectRoot(t *testing.T) {
	path := tempFile(t, "fork-close.log")
	config := Config{
		ServiceName: "t", Environment: EnvironmentDevelopment, Level: LevelInfo,
		Outputs: []OutputConfig{{Type: OutputFile, Format: FormatJSON, Path: path}},
	}
	root, err := New(config)
	assertNoError(t, err)
	defer root.Close()

	ctx := context.Background()

	forkWith := root.With(Field("scope", "with"))
	if err := forkWith.Close(); !errors.Is(err, ErrLoggerNotRoot) {
		t.Errorf("With-fork Close should return ErrLoggerNotRoot, got %v", err)
	}

	forkNamed := root.Named("sub")
	if err := forkNamed.Close(); !errors.Is(err, ErrLoggerNotRoot) {
		t.Errorf("Named-fork Close should return ErrLoggerNotRoot, got %v", err)
	}

	root.Info(ctx, "root-still-writes")
	forkWith.Info(ctx, "with-fork-still-writes")
	forkNamed.Info(ctx, "named-fork-still-writes")

	content := readFile(t, path)
	for _, marker := range []string{"root-still-writes", "with-fork-still-writes", "named-fork-still-writes"} {
		if !strings.Contains(content, marker) {
			t.Errorf("expected %q in output after fork Close, got: %s", marker, content)
		}
	}
}

// TestLogger_CloseOnFork_DoesNotCloseSharedOutputs verifies that a fork's
// Close does not tear down the file outputs the root still uses.
func TestLogger_CloseOnFork_DoesNotCloseSharedOutputs(t *testing.T) {
	path := tempFile(t, "fork-shared-output.log")
	config := Config{
		ServiceName: "t", Environment: EnvironmentDevelopment, Level: LevelInfo,
	}
	out := MustFile(path, FormatJSON)
	root, err := New(config, WithOutputs(out))
	assertNoError(t, err)

	fork := root.With(Field("scope", "fork"))
	if err := fork.Close(); !errors.Is(err, ErrLoggerNotRoot) {
		t.Fatalf("fork Close should return ErrLoggerNotRoot, got %v", err)
	}

	root.Info(context.Background(), "after-fork-close")

	if _, err := out.Write([]byte(`{"manual":"post-fork-close"}` + "\n")); err != nil {
		t.Errorf("shared output should still be open after fork Close: %v", err)
	}

	if err := root.Close(); err != nil {
		t.Fatalf("root Close: %v", err)
	}

	content := readFile(t, path)
	if !strings.Contains(content, "after-fork-close") {
		t.Errorf("missing root write after fork Close: %s", content)
	}
	if !strings.Contains(content, `"manual":"post-fork-close"`) {
		t.Errorf("missing direct write to shared output after fork Close: %s", content)
	}
}
