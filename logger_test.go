package axio

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestNew_MasksPIIByDefault(t *testing.T) {
	loaded, err := LoadConfigFrom(strings.NewReader("serviceName: payments\n"), "yaml")
	assertNoError(t, err)

	tests := []struct {
		name   string
		config Config
	}{
		{name: "default_config", config: DefaultConfig()},
		{name: "zero_config", config: Config{}},
		{name: "loaded_without_pii_keys", config: loaded},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := logCustomer(t, test.config)
			assertEqual(t, record["message"], any("customer ***.***.***-** registered"))
			assertEqual(t, record["password"], any("[REDACTED]"))
		})
	}
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

func TestLogger_Event(t *testing.T) {
	t.Run("writes_to_the_logger_outputs", func(t *testing.T) {
		output := newBufferOutput(FormatJSON)
		logger, err := New(minimalConfig(), WithOutputs(output))
		assertNoError(t, err)
		defer logger.Close()

		event := logger.Event("checkout")
		event.Add("cart_total", 15999)
		event.Emit(context.Background())

		written := output.String()
		if !strings.Contains(written, `"event":"checkout"`) || !strings.Contains(written, `"cart_total":15999`) {
			t.Errorf("expected the event on the logger's output, got:\n%s", written)
		}
	})

	t.Run("deriving_allocates_only_the_event", func(t *testing.T) {
		logger, err := New(minimalConfig(), WithOutputs(newBufferOutput(FormatJSON)))
		assertNoError(t, err)
		defer logger.Close()

		var derived *Event
		allocations := testing.AllocsPerRun(100, func() {
			derived = logger.Event("checkout")
		})
		if allocations > 1 {
			t.Errorf("expected deriving an event to allocate the event alone, got %v allocations", allocations)
		}
		_ = derived
	})

	t.Run("audited_event_extends_the_logger_chain", func(t *testing.T) {
		output := newBufferOutput(FormatJSON)
		chain, err := NewHashChain(nil)
		assertNoError(t, err)
		logger, err := New(minimalConfig(), WithOutputs(output), WithAuditChain(chain))
		assertNoError(t, err)
		defer logger.Close()
		ctx := context.Background()

		logger.Info(ctx, "before the event")
		logger.Event("checkout").Emit(ctx)
		logger.Info(ctx, "after the event")

		assertNoError(t, chain.Verify(strings.NewReader(output.String()), ""))
		assertEqual(t, chain.Sequence(), uint64(3))
	})

	t.Run("inherits_with_annotations", func(t *testing.T) {
		output := newBufferOutput(FormatJSON)
		logger, err := New(minimalConfig(), WithOutputs(output))
		assertNoError(t, err)
		defer logger.Close()

		event := logger.With(Field("tenant", "acme")).Event("checkout")
		event.Add("cart_total", 15999)
		event.Emit(context.Background())

		written := output.String()
		tenant, total := strings.Index(written, `"tenant":"acme"`), strings.Index(written, `"cart_total":15999`)
		if tenant < 0 || total < 0 || tenant > total {
			t.Errorf("expected the logger's tenant before the event's cart_total, got:\n%s", written)
		}
	})

	t.Run("inherits_the_named_name", func(t *testing.T) {
		output := newBufferOutput(FormatJSON)
		var seen string
		hook := &testHook{name: "name", process: func(_ context.Context, entry *Entry) error {
			seen = entry.Logger
			return nil
		}}
		logger, err := New(minimalConfig(), WithOutputs(output), WithHooks(hook))
		assertNoError(t, err)
		defer logger.Close()

		logger.Named("billing").Event("checkout").Emit(context.Background())

		assertEqual(t, seen, "billing")
		assertEqual(t, parseJSONLines(t, output.String())[0]["logger"], any("billing"))
	})

	t.Run("hook_changes_do_not_reach_the_logger", func(t *testing.T) {
		output := newBufferOutput(FormatJSON)
		hook := &testHook{name: "rewrite", process: func(_ context.Context, entry *Entry) error {
			if entry.Message == "checkout" {
				entry.Annotations[0] = Field("tenant", "rewritten")
			}
			return nil
		}}
		logger, err := New(minimalConfig(), WithOutputs(output), WithHooks(hook))
		assertNoError(t, err)
		defer logger.Close()
		ctx := context.Background()

		tenant := logger.With(Field("tenant", "acme"))
		tenant.Event("checkout").Emit(ctx)
		tenant.Info(ctx, "after the event")

		assertEqual(t, parseJSONLines(t, output.String())[1]["tenant"], any("acme"))
	})

	t.Run("emit_after_logger_close_writes_nothing", func(t *testing.T) {
		output := newBufferOutput(FormatJSON)
		logger, err := New(minimalConfig(), WithOutputs(output))
		assertNoError(t, err)
		direct, forked := logger.Event("checkout"), logger.Named("billing").Event("invoice")
		assertNoError(t, logger.Close())

		direct.Emit(context.Background())
		forked.Emit(context.Background())

		assertEqual(t, output.String(), "")
	})

	t.Run("masks_pii_by_default", func(t *testing.T) {
		output := newBufferOutput(FormatJSON)
		logger, err := New(DefaultConfig(), WithOutputs(output))
		assertNoError(t, err)
		defer logger.Close()

		event := logger.Event("registration")
		event.Add("document", "123.456.789-01")
		event.Add("password", "hunter2")
		event.Emit(context.Background())

		record := parseJSONLines(t, output.String())[0]
		assertEqual(t, record["document"], any("***.***.***-**"))
		assertEqual(t, record["password"], any("[REDACTED]"))
	})

	// emitBeside emits an event through a logger with a text output and a JSON
	// output, built with options, and returns what each one received.
	emitBeside := func(t *testing.T, options ...Option) (string, string) {
		t.Helper()
		text, structured := newBufferOutput(FormatText), newBufferOutput(FormatJSON)
		logger, err := New(minimalConfig(), append([]Option{WithOutputs(text, structured)}, options...)...)
		if err != nil {
			t.Fatalf("new logger: %v", err)
		}
		logger.Event("checkout").Emit(context.Background())
		assertNoError(t, logger.Close())
		return text.String(), structured.String()
	}

	t.Run("writes_text_on_text_outputs", func(t *testing.T) {
		text, structured := emitBeside(t)

		if !strings.Contains(text, "EVENT") || !strings.Contains(text, "\tcheckout\t") || strings.Contains(text, `"event":`) {
			t.Errorf("expected the event as text on the text output, got:\n%s", text)
		}
		if !strings.Contains(structured, `"event":"checkout"`) {
			t.Errorf("expected the event as JSON on the JSON output, got:\n%s", structured)
		}
	})

	t.Run("audited_event_writes_the_short_hash_on_text_outputs", func(t *testing.T) {
		chain, err := NewHashChain(nil)
		assertNoError(t, err)
		text, structured := emitBeside(t, WithAuditChain(chain))

		if !strings.Contains(text, "EVENT") || !strings.Contains(text, `"hash": "`) || strings.Contains(text, `"event":`) {
			t.Errorf("expected the event as text with the short hash on the text output, got:\n%s", text)
		}
		if !strings.Contains(structured, `"event":"checkout"`) || !strings.Contains(structured, `"hash":"`) {
			t.Errorf("expected the audited event as JSON on the JSON output, got:\n%s", structured)
		}
	})

	t.Run("logger_level_does_not_filter_events", func(t *testing.T) {
		output := newBufferOutput(FormatJSON)
		config := minimalConfig()
		config.Level = LevelError
		logger, err := New(config, WithOutputs(output))
		assertNoError(t, err)
		defer logger.Close()

		logger.Event("checkout").Emit(context.Background())

		if !strings.Contains(output.String(), `"event":"checkout"`) {
			t.Errorf("expected the event from a logger at error level, got:\n%s", output.String())
		}
	})
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

func TestNew_OutputOwnership(t *testing.T) {
	failingConfig := func() Config {
		config := minimalConfig()
		config.PIICustomPatterns = []CustomPII{{Name: "broken", Pattern: "(", Mask: "x"}}
		return config
	}

	t.Run("failed_new_releases_its_outputs", func(t *testing.T) {
		path := tempFile(t, "app.log")
		config := failingConfig()
		config.Outputs = []OutputConfig{{Type: OutputFile, Format: FormatJSON, Path: path, Rotation: RotationConfig{MaxSize: 10}}}
		_, err := New(config)
		if !errors.Is(err, ErrBuildHooks) {
			t.Fatalf("expected ErrBuildHooks, got %v", err)
		}

		other, err := RotatingFile(path, FormatJSON, RotationConfig{MaxSize: 20})
		assertNoError(t, err)
		if other != nil {
			assertNoError(t, other.Close())
		}
	})

	t.Run("failed_new_returns_given_outputs_to_the_caller", func(t *testing.T) {
		output, err := RotatingFile(tempFile(t, "app.log"), FormatJSON, RotationConfig{MaxSize: 10})
		assertNoError(t, err)
		_, err = New(failingConfig(), WithOutputs(output))
		if !errors.Is(err, ErrBuildHooks) {
			t.Fatalf("expected ErrBuildHooks, got %v", err)
		}

		logger, err := New(minimalConfig(), WithOutputs(output))
		assertNoError(t, err)
		logger.Info(context.Background(), "after the failed new")
		assertNoError(t, logger.Close())
	})

	t.Run("output_in_a_second_logger_fails", func(t *testing.T) {
		output, err := RotatingFile(tempFile(t, "shared.log"), FormatJSON, RotationConfig{MaxSize: 10})
		assertNoError(t, err)
		logger, err := New(minimalConfig(), WithOutputs(output))
		assertNoError(t, err)
		defer logger.Close()

		_, err = New(minimalConfig(), WithOutputs(output))
		if !errors.Is(err, ErrOutputInUse) {
			t.Errorf("expected ErrOutputInUse, got %v", err)
		}
	})

	t.Run("claim_refuses_a_closed_output", func(t *testing.T) {
		output, err := File(tempFile(t, "app.log"), FormatJSON)
		assertNoError(t, err)
		assertNoError(t, output.Close())

		_, err = New(minimalConfig(), WithOutputs(output))
		if !errors.Is(err, ErrOutputClosed) || !errors.Is(err, ErrBuildOutputs) {
			t.Errorf("expected ErrBuildOutputs wrapping ErrOutputClosed, got %v", err)
		}
	})
}

func TestNew_RotatingFile(t *testing.T) {
	rotatingConfig := func(path string, rotation RotationConfig) Config {
		config := minimalConfig()
		config.Outputs = []OutputConfig{{Type: OutputFile, Format: FormatJSON, Path: path, Rotation: rotation}}
		return config
	}
	logOnce := func(t *testing.T, config Config, payload string) {
		t.Helper()
		logger, err := New(config)
		if err != nil {
			t.Errorf("new logger: %v", err)
			return
		}
		logger.Info(context.Background(), "extraction", Field("payload", payload))
		assertNoError(t, logger.Close())
	}

	t.Run("two_hundred_loggers_leave_no_goroutine", func(t *testing.T) {
		config := rotatingConfig(tempFile(t, "app.log"), RotationConfig{MaxSize: 10})
		before := runtime.NumGoroutine()
		for range 200 {
			logOnce(t, config, "x")
		}
		if after := settledGoroutines(before); after > before {
			t.Errorf("200 loggers left %d goroutines behind", after-before)
		}
	})

	t.Run("concurrent_loggers_lose_no_line", func(t *testing.T) {
		directory := tempDir(t)
		config := rotatingConfig(filepath.Join(directory, "app.log"), RotationConfig{MaxSize: 1})
		payload := strings.Repeat("x", 8<<10)
		stderr := captureStderr(t)
		var group sync.WaitGroup
		for range 8 {
			group.Go(func() {
				for range 200 {
					logOnce(t, config, payload)
				}
			})
		}
		group.Wait()
		reported := stderr()

		assertEqual(t, countLogLines(t, directory), 1600)
		locks, err := filepath.Glob(filepath.Join(directory, "*.lock"))
		assertNoError(t, err)
		assertEqual(t, len(locks), 1)
		if strings.Contains(reported, "axio:") {
			t.Errorf("expected no failure on stderr, got:\n%s", reported)
		}
	})

	t.Run("sequential_loggers_rotate_by_interval", func(t *testing.T) {
		directory := tempDir(t)
		config := rotatingConfig(filepath.Join(directory, "app.log"), RotationConfig{Interval: Duration(50 * time.Millisecond)})
		deadline := time.Now().Add(400 * time.Millisecond)
		for time.Now().Before(deadline) {
			logOnce(t, config, "x")
			time.Sleep(10 * time.Millisecond)
		}

		if files := len(logFiles(t, directory)); files < 2 {
			t.Errorf("expected a backup beside the log, got %d file(s)", files)
		}
	})

	t.Run("loggers_on_one_path_close_cleanly", func(t *testing.T) {
		for _, rotation := range []RotationConfig{{MaxSize: 10}, {}} {
			path := tempFile(t, "shared.log")
			config := rotatingConfig(path, rotation)
			logger, err := New(config)
			assertNoError(t, err)
			for range 3 {
				logOnce(t, config, "x")
			}
			logger.Info(context.Background(), "after the others")
			assertNoError(t, logger.Close())

			assertEqual(t, len(readLines(t, path)), 4)
		}
	})
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

	// A call that reaches its write after Close, while its hooks ran, counts as
	// a call made after Close: it writes nothing and reports nothing. Nothing
	// is written to an output already closed, and with auditing the chain
	// never runs past the log, which would read as a log cut short.
	racingClose := func(t *testing.T, write func(logger Logger), options ...Option) string {
		t.Helper()
		logPath := tempFile(t, "app.log")
		config := minimalConfig()
		config.Outputs = []OutputConfig{{Type: OutputFile, Format: FormatJSON, Path: logPath}}
		entered, release := make(chan struct{}), make(chan struct{})
		pause := &testHook{name: "pause", process: func(_ context.Context, entry *Entry) error {
			if entry.Message == "racing" {
				close(entered)
				<-release
			}
			return nil
		}}
		logger, err := New(config, append([]Option{WithHooks(pause)}, options...)...)
		if err != nil {
			t.Fatalf("new logger: %v", err)
		}
		logger.Info(context.Background(), "before the close")
		stderr := captureStderr(t)

		written := make(chan struct{})
		go func() {
			write(logger)
			close(written)
		}()
		<-entered
		assertNoError(t, logger.Close())
		close(release)
		<-written

		if reported := stderr(); reported != "" {
			t.Errorf("expected the racing write dropped without a report, got:\n%s", reported)
		}
		return logPath
	}
	writeLine := func(logger Logger) { logger.Info(context.Background(), "racing") }
	emitEvent := func(logger Logger) { logger.Event("racing").Emit(context.Background()) }

	t.Run("line_racing_close_is_dropped", func(t *testing.T) {
		logPath := racingClose(t, writeLine)
		assertEqual(t, len(readLines(t, logPath)), 1)
	})

	t.Run("event_racing_close_is_dropped", func(t *testing.T) {
		logPath := racingClose(t, emitEvent)
		assertEqual(t, len(readLines(t, logPath)), 1)
	})

	t.Run("line_racing_close_is_not_counted", func(t *testing.T) {
		recorder := &logsRecorder{}
		racingClose(t, writeLine, func(config *Config) error {
			config.metrics = recorder
			return nil
		})
		assertEqual(t, recorder.logs.Load(), int32(1))
	})

	t.Run("audited_line_racing_close_is_dropped", func(t *testing.T) {
		storePath := tempFile(t, "chain.json")
		logPath := racingClose(t, writeLine, WithAudit(storePath))
		assertNoError(t, verifyFile(t, logPath, storePath))
	})

	t.Run("audited_event_racing_close_is_dropped", func(t *testing.T) {
		storePath := tempFile(t, "chain.json")
		logPath := racingClose(t, emitEvent, WithAudit(storePath))
		assertNoError(t, verifyFile(t, logPath, storePath))
	})

	t.Run("close_waits_for_a_write_under_way", func(t *testing.T) {
		output := newPausingOutput()
		logger, err := New(minimalConfig(), WithOutputs(output))
		if err != nil {
			t.Fatalf("new logger: %v", err)
		}
		go logger.Info(context.Background(), "under way")
		<-output.entered

		closed := make(chan error, 1)
		go func() { closed <- logger.Close() }()
		returnedEarly := false
		select {
		case err := <-closed:
			returnedEarly = true
			assertNoError(t, err)
		case <-time.After(50 * time.Millisecond):
		}
		close(output.release)
		if returnedEarly {
			t.Fatal("Close returned while a write was under way")
		}
		assertNoError(t, <-closed)

		if !strings.Contains(output.String(), "under way") {
			t.Errorf("expected the write under way to finish before Close, got %q", output.String())
		}
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

func TestLogger_BuildsCallerOnlyForHooksThatReadIt(t *testing.T) {
	if raceDetector {
		t.Skip("regexp keeps its matchers in a sync.Pool, which the race detector drains at random")
	}
	allocations := func(t *testing.T, options ...Option) float64 {
		t.Helper()
		logger, err := New(minimalConfig(), append(options, WithOutputs(benchOutput{}))...)
		assertNoError(t, err)
		defer func() { assertNoError(t, logger.Close()) }()
		return testing.AllocsPerRun(100, func() {
			logger.Info(context.Background(), "order created")
		})
	}
	callerCost := func(t *testing.T, options ...Option) float64 {
		t.Helper()
		return allocations(t, options...) - allocations(t, append(slices.Clone(options), WithOmitCaller())...)
	}
	read := callerCost(t, WithHooks(NoopHook()))

	tests := []struct {
		name    string
		options []Option
	}{
		{name: "no_hooks"},
		{name: "pii_hook", options: []Option{WithPII([]PIIPattern{PatternCPF}, nil)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if cost := callerCost(t, test.options...); cost >= read {
				t.Errorf("the caller cost %v allocations, and %v with a hook that may read it: want fewer", cost, read)
			}
		})
	}
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
				logger, err := New(config, WithOutputs(failingOutput{format: FormatJSON}))
				assertNoError(t, err)
				logger.Event("checkout").Emit(context.Background())
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
	emitter, err := New(config, WithOutputs(emitted))
	assertNoError(t, err)
	event := emitter.Event("checkout")
	event.With(annotations...)
	event.SetError(errors.New("card declined"))
	event.Emit(context.Background())
	assertNoError(t, emitter.Close())

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
