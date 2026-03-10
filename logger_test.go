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
			Environment:    Production,
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
		Environment: Development,
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
		Environment: Development,
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
		Environment: Development,
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
		Environment: Development,
		Level:       LevelInfo,
		Outputs: []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		},
	}

	logger, err := New(config)
	assertNoError(t, err)
	defer logger.Close()

	logger.With(
		Annotate("user_id", "usr_123"),
		Annotate("tenant", "acme"),
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
		Environment: Development,
		Level:       LevelInfo,
		Outputs: []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		},
	}

	logger, err := New(config)
	assertNoError(t, err)
	defer logger.Close()

	httpAnnotation := &HTTP{
		Method:     "POST",
		URL:        "/api/users",
		StatusCode: 201,
		LatencyMS:  45,
	}

	logger.With(httpAnnotation).Info(context.Background(), "http request")

	content := readFile(t, path)
	if !strings.Contains(content, "POST") {
		t.Error("log should contain HTTP method")
	}
	if !strings.Contains(content, "/api/users") {
		t.Error("log should contain URL")
	}
}

func TestLogger_MessageFormatting(t *testing.T) {
	path := tempFile(t, "format.log")
	config := Config{
		ServiceName: "test",
		Environment: Development,
		Level:       LevelInfo,
		Outputs: []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		},
	}

	logger, err := New(config)
	assertNoError(t, err)
	defer logger.Close()

	logger.Info(context.Background(), "processed %d items in %s", 42, "100ms")

	content := readFile(t, path)
	if !strings.Contains(content, "processed 42 items in 100ms") {
		t.Error("log should contain formatted message")
	}
}

func TestLogger_WithNilAnnotation(t *testing.T) {
	config := minimalConfig()
	logger, err := New(config)
	assertNoError(t, err)
	defer logger.Close()

	// Should not panic
	logger.With(nil).Info(context.Background(), "test")
}

func TestLogger_Close(t *testing.T) {
	t.Run("closes_file_outputs", func(t *testing.T) {
		path := tempFile(t, "close.log")
		config := Config{
			ServiceName: "test",
			Environment: Development,
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

func TestLogger_MessageWithExtraArgs(t *testing.T) {
	path := tempFile(t, "extra.log")
	config := Config{
		ServiceName: "test",
		Environment: Development,
		Level:       LevelInfo,
		Outputs: []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		},
	}

	logger, _ := New(config)
	defer logger.Close()

	// Extra arguments are formatted normally by fmt.Sprintf
	// This test verifies there is no crash
	logger.Info(context.Background(), "value: %d extra: %s", 42, "test")

	content := readFile(t, path)
	if !strings.Contains(content, "value: 42 extra: test") {
		t.Error("message should be formatted correctly")
	}
}
