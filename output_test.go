package axio

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConsole(t *testing.T) {
	out := Console(FormatText)

	assertEqual(t, out.Type(), OutputConsole)
	assertEqual(t, out.Format(), FormatText)

	// Console writes to stderr
	written, err := out.Write([]byte("test"))
	assertNoError(t, err)
	if written == 0 {
		t.Error("should have written bytes")
	}

	// Close does nothing for console
	err = out.Close()
	assertNoError(t, err)
}

func TestStdout(t *testing.T) {
	out := Stdout(FormatJSON)

	assertEqual(t, out.Type(), OutputStdout)
	assertEqual(t, out.Format(), FormatJSON)

	// Close does nothing for stdout
	err := out.Close()
	assertNoError(t, err)
}

func TestFile(t *testing.T) {
	t.Run("creates_file", func(t *testing.T) {
		path := tempFile(t, "output.log")

		out, err := File(path, FormatJSON)
		assertNoError(t, err)
		defer out.Close()

		assertEqual(t, out.Type(), OutputFile)
		assertEqual(t, out.Format(), FormatJSON)

		// File should exist
		_, err = os.Stat(path)
		assertNoError(t, err)
	})

	t.Run("writes_to_file", func(t *testing.T) {
		path := tempFile(t, "write.log")

		out, err := File(path, FormatJSON)
		assertNoError(t, err)

		_, err = out.Write([]byte("test content\n"))
		assertNoError(t, err)

		out.Close()

		content := readFile(t, path)
		if content != "test content\n" {
			t.Errorf("incorrect content: %q", content)
		}
	})

	t.Run("appends_to_existing_file", func(t *testing.T) {
		path := tempFile(t, "append.log")
		writeFile(t, path, "existing\n")

		out, err := File(path, FormatJSON)
		assertNoError(t, err)

		_, err = out.Write([]byte("appended\n"))
		assertNoError(t, err)
		out.Close()

		content := readFile(t, path)
		if content != "existing\nappended\n" {
			t.Errorf("should have appended to file: %q", content)
		}
	})

	t.Run("invalid_path_returns_error", func(t *testing.T) {
		_, err := File("/nonexistent/dir/file.log", FormatJSON)
		assertError(t, err)
		if !errors.Is(err, ErrOpenFile) {
			t.Errorf("expected ErrOpenFile, got %v", err)
		}
	})

	t.Run("close_closes_file", func(t *testing.T) {
		path := tempFile(t, "close.log")

		out, _ := File(path, FormatJSON)
		err := out.Close()
		assertNoError(t, err)

		// Second write should fail (file closed)
		_, err = out.Write([]byte("after close"))
		assertError(t, err)
	})

	t.Run("second_close_returns_sentinel", func(t *testing.T) {
		out, err := File(tempFile(t, "twice.log"), FormatJSON)
		assertNoError(t, err)

		assertNoError(t, out.Close())
		if err := out.Close(); !errors.Is(err, ErrOutputClosed) {
			t.Errorf("expected ErrOutputClosed, got %v", err)
		}
	})
}

func TestRotatingFile_Close(t *testing.T) {
	t.Run("second_close_with_time_rotation_returns_sentinel", func(t *testing.T) {
		out, err := RotatingFile(tempFile(t, "twice.log"), FormatJSON, RotationConfig{Interval: Duration(time.Hour)})
		assertNoError(t, err)

		assertNoError(t, out.Close())
		if err := out.Close(); !errors.Is(err, ErrOutputClosed) {
			t.Errorf("expected ErrOutputClosed, got %v", err)
		}
	})

	t.Run("second_close_with_size_rotation_returns_sentinel", func(t *testing.T) {
		out, err := RotatingFile(tempFile(t, "twice.log"), FormatJSON, RotationConfig{MaxSize: 10})
		assertNoError(t, err)

		assertNoError(t, out.Close())
		if err := out.Close(); !errors.Is(err, ErrOutputClosed) {
			t.Errorf("expected ErrOutputClosed, got %v", err)
		}
	})
}

func TestMustFile(t *testing.T) {
	t.Run("valid_path", func(t *testing.T) {
		path := tempFile(t, "must.log")

		out := MustFile(path, FormatJSON)
		defer out.Close()

		if out == nil {
			t.Error("output should not be nil")
		}
	})

	t.Run("invalid_path_panics", func(t *testing.T) {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Error("expected panic for invalid path")
			}
		}()

		MustFile("/nonexistent/dir/file.log", FormatJSON)
	})
}

func TestBuildOutputs(t *testing.T) {
	t.Run("builds_console", func(t *testing.T) {
		config := Config{
			Outputs: []OutputConfig{
				{Type: OutputConsole, Format: FormatText},
			},
		}

		outputs, err := buildOutputs(config)
		assertNoError(t, err)

		if len(outputs) != 1 {
			t.Fatalf("expected 1 output, got %d", len(outputs))
		}
		assertEqual(t, outputs[0].Type(), OutputConsole)
	})

	t.Run("builds_stdout", func(t *testing.T) {
		config := Config{
			Outputs: []OutputConfig{
				{Type: OutputStdout, Format: FormatJSON},
			},
		}

		outputs, err := buildOutputs(config)
		assertNoError(t, err)

		if len(outputs) != 1 {
			t.Fatalf("expected 1 output, got %d", len(outputs))
		}
		assertEqual(t, outputs[0].Type(), OutputStdout)
	})

	t.Run("builds_file", func(t *testing.T) {
		path := tempFile(t, "build.log")
		config := Config{
			Outputs: []OutputConfig{
				{Type: OutputFile, Format: FormatJSON, Path: path},
			},
		}

		outputs, err := buildOutputs(config)
		assertNoError(t, err)
		defer outputs[0].Close()

		if len(outputs) != 1 {
			t.Fatalf("expected 1 output, got %d", len(outputs))
		}
		assertEqual(t, outputs[0].Type(), OutputFile)
	})

	t.Run("builds_multiple_outputs", func(t *testing.T) {
		path := tempFile(t, "multi.log")
		config := Config{
			Outputs: []OutputConfig{
				{Type: OutputConsole, Format: FormatText},
				{Type: OutputStdout, Format: FormatJSON},
				{Type: OutputFile, Format: FormatJSON, Path: path},
			},
		}

		outputs, err := buildOutputs(config)
		assertNoError(t, err)
		defer outputs[2].Close()

		if len(outputs) != 3 {
			t.Fatalf("expected 3 outputs, got %d", len(outputs))
		}
	})

	t.Run("file_without_path_fails", func(t *testing.T) {
		config := Config{
			Outputs: []OutputConfig{
				{Type: OutputFile, Format: FormatJSON}, // without path
			},
		}

		_, err := buildOutputs(config)
		assertError(t, err)
	})

	t.Run("invalid_file_path_fails", func(t *testing.T) {
		config := Config{
			Outputs: []OutputConfig{
				{Type: OutputFile, Format: FormatJSON, Path: "/nonexistent/dir/file.log"},
			},
		}

		_, err := buildOutputs(config)
		assertError(t, err)
	})

	t.Run("unknown_type_fails", func(t *testing.T) {
		config := Config{
			Outputs: []OutputConfig{
				{Type: OutputType("unknown"), Format: FormatJSON},
			},
		}

		_, err := buildOutputs(config)
		assertError(t, err)
	})

	t.Run("empty_outputs_returns_empty_slice", func(t *testing.T) {
		config := Config{
			Outputs: []OutputConfig{},
		}

		outputs, err := buildOutputs(config)
		assertNoError(t, err)

		if len(outputs) != 0 {
			t.Errorf("expected 0 outputs, got %d", len(outputs))
		}
	})
}

func TestOutputType_Validate(t *testing.T) {
	valid := []OutputType{OutputConsole, OutputStdout, OutputFile}
	for _, ot := range valid {
		if err := ot.Validate(); err != nil {
			t.Errorf("output type %s should be valid", ot)
		}
	}

	invalid := OutputType("invalid")
	if err := invalid.Validate(); err == nil {
		t.Error("invalid output type should return error")
	}
}

func TestOutputType_UnmarshalText(t *testing.T) {
	t.Run("valid_types", func(t *testing.T) {
		tests := []struct {
			input string
			want  OutputType
		}{
			{"console", OutputConsole},
			{"stdout", OutputStdout},
			{"file", OutputFile},
			{" console ", OutputConsole}, // with spaces
		}

		for _, test := range tests {
			var ot OutputType
			err := ot.UnmarshalText([]byte(test.input))
			assertNoError(t, err)
			assertEqual(t, ot, test.want)
		}
	})

	t.Run("invalid_type", func(t *testing.T) {
		var ot OutputType
		err := ot.UnmarshalText([]byte("invalid"))
		assertError(t, err)
	})
}

func TestRotationConfig_Enabled(t *testing.T) {
	t.Run("empty_config_disabled", func(t *testing.T) {
		rotation := RotationConfig{}
		assertEqual(t, rotation.Enabled(), false)
	})

	t.Run("size_only_enabled", func(t *testing.T) {
		rotation := RotationConfig{MaxSize: 100}
		assertEqual(t, rotation.Enabled(), true)
	})

	t.Run("interval_only_enabled", func(t *testing.T) {
		rotation := RotationConfig{Interval: Duration(24 * time.Hour)}
		assertEqual(t, rotation.Enabled(), true)
	})

	t.Run("both_enabled", func(t *testing.T) {
		rotation := RotationConfig{
			MaxSize:  100,
			Interval: Duration(24 * time.Hour),
		}
		assertEqual(t, rotation.Enabled(), true)
	})

	t.Run("only_maxage_not_enabled", func(t *testing.T) {
		rotation := RotationConfig{MaxAge: 30}
		assertEqual(t, rotation.Enabled(), false)
	})
}

func TestRotatingFile(t *testing.T) {
	maxSize := func(t *testing.T, rotation RotationConfig) int {
		t.Helper()
		output, err := RotatingFile(tempFile(t, "sized.log"), FormatJSON, rotation)
		assertNoError(t, err)
		defer func() { assertNoError(t, output.Close()) }()
		file, ok := output.(*fileOutput)
		if !ok {
			t.Fatalf("expected a *fileOutput, got %T", output)
		}
		return file.lumberjack.MaxSize
	}

	t.Run("zero_max_size_never_rotates_by_size", func(t *testing.T) {
		if size := maxSize(t, RotationConfig{Interval: Duration(time.Hour)}); size < 1<<30 {
			t.Errorf("lumberjack got MaxSize %d MB, want a size no file reaches; 0 is its 100 MB default", size)
		}
	})

	t.Run("max_size_is_kept", func(t *testing.T) {
		assertEqual(t, maxSize(t, RotationConfig{MaxSize: 50}), 50)
	})

	t.Run("creates_with_size_rotation", func(t *testing.T) {
		path := tempFile(t, "rotating.log")

		output, err := RotatingFile(path, FormatJSON, RotationConfig{
			MaxSize:    10,
			MaxBackups: 3,
		})
		assertNoError(t, err)
		defer output.Close()

		assertEqual(t, output.Type(), OutputFile)
		assertEqual(t, output.Format(), FormatJSON)
	})

	t.Run("creates_with_time_rotation", func(t *testing.T) {
		path := tempFile(t, "time-rotating.log")

		output, err := RotatingFile(path, FormatJSON, RotationConfig{
			Interval: Duration(time.Hour),
		})
		assertNoError(t, err)
		defer output.Close()

		assertEqual(t, output.Type(), OutputFile)
	})

	t.Run("creates_with_combined_rotation", func(t *testing.T) {
		path := tempFile(t, "combined-rotating.log")

		output, err := RotatingFile(path, FormatJSON, RotationConfig{
			MaxSize:  50,
			Interval: Duration(12 * time.Hour),
			Compress: true,
		})
		assertNoError(t, err)
		defer output.Close()

		assertEqual(t, output.Type(), OutputFile)
	})

	t.Run("writes_to_file", func(t *testing.T) {
		path := tempFile(t, "write-rotating.log")

		output, err := RotatingFile(path, FormatJSON, RotationConfig{
			MaxSize: 10,
		})
		assertNoError(t, err)

		_, err = output.Write([]byte("test log entry\n"))
		assertNoError(t, err)

		output.Close()

		content := readFile(t, path)
		if content != "test log entry\n" {
			t.Errorf("unexpected content: %q", content)
		}
	})

	t.Run("close_stops_ticker", func(t *testing.T) {
		path := tempFile(t, "close-ticker.log")

		output, err := RotatingFile(path, FormatJSON, RotationConfig{
			Interval: Duration(time.Hour),
		})
		assertNoError(t, err)

		// Close should not panic or error
		err = output.Close()
		assertNoError(t, err)
	})
}

func TestMustRotatingFile(t *testing.T) {
	t.Run("valid_path", func(t *testing.T) {
		path := tempFile(t, "must-rotating.log")

		output := MustRotatingFile(path, FormatJSON, RotationConfig{
			MaxSize: 10,
		})
		defer output.Close()

		if output == nil {
			t.Error("output should not be nil")
		}
	})
}

func TestBuildOutputs_WithRotation(t *testing.T) {
	t.Run("builds_rotating_file", func(t *testing.T) {
		path := tempFile(t, "build-rotating.log")
		config := Config{
			Outputs: []OutputConfig{
				{
					Type:   OutputFile,
					Format: FormatJSON,
					Path:   path,
					Rotation: RotationConfig{
						MaxSize:    100,
						MaxBackups: 5,
						Compress:   true,
					},
				},
			},
		}

		outputs, err := buildOutputs(config)
		assertNoError(t, err)
		defer outputs[0].Close()

		if len(outputs) != 1 {
			t.Fatalf("expected 1 output, got %d", len(outputs))
		}
		assertEqual(t, outputs[0].Type(), OutputFile)
	})

	t.Run("builds_plain_file_without_rotation", func(t *testing.T) {
		path := tempFile(t, "plain.log")
		config := Config{
			Outputs: []OutputConfig{
				{
					Type:   OutputFile,
					Format: FormatJSON,
					Path:   path,
				},
			},
		}

		outputs, err := buildOutputs(config)
		assertNoError(t, err)
		defer outputs[0].Close()

		if len(outputs) != 1 {
			t.Fatalf("expected 1 output, got %d", len(outputs))
		}
	})
}

func TestRotatingFile_TimeRotation(t *testing.T) {
	t.Run("rotates_on_interval", func(t *testing.T) {
		directory := tempDir(t)
		path := filepath.Join(directory, "timed.log")

		output, err := RotatingFile(path, FormatJSON, RotationConfig{
			Interval: Duration(100 * time.Millisecond),
		})
		assertNoError(t, err)

		// Write initial content
		_, err = output.Write([]byte("before rotation\n"))
		assertNoError(t, err)

		// Wait for at least one rotation tick
		time.Sleep(250 * time.Millisecond)

		// Write after rotation should have happened
		_, err = output.Write([]byte("after rotation\n"))
		assertNoError(t, err)

		output.Close()

		// The current file should contain the post-rotation content
		content := readFile(t, path)
		if content == "after rotation\n" {
			return
		}

		// Rotation may have created a backup; just verify current file is writable
		// and the backup exists
		entries, _ := os.ReadDir(directory)
		if len(entries) < 2 {
			t.Errorf("expected rotated backup file, got %d files", len(entries))
		}
	})
}

func TestRotationConfig_YAMLUnmarshal(t *testing.T) {
	yamlContent := `
serviceName: "rotation-test"
serviceVersion: "1.0.0"
environment: "production"
level: "info"
outputs:
  - type: file
    format: json
    path: /var/log/app.log
    rotation:
      maxSize: 100
      maxAge: 30
      maxBackups: 10
      compress: true
      interval: 24h
`
	config, err := LoadConfigFrom(
		stringReader(yamlContent),
		"yaml",
	)
	assertNoError(t, err)

	if len(config.Outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(config.Outputs))
	}

	rotation := config.Outputs[0].Rotation
	assertEqual(t, rotation.MaxSize, 100)
	assertEqual(t, rotation.MaxAge, 30)
	assertEqual(t, rotation.MaxBackups, 10)
	assertEqual(t, rotation.Compress, true)
	assertEqual(t, rotation.Interval, Duration(24*time.Hour))
	assertEqual(t, rotation.Enabled(), true)
}

func TestFileOutput_LastRotationError(t *testing.T) {
	t.Run("nil_when_unused", func(t *testing.T) {
		output := &fileOutput{}
		if got := output.LastRotationError(); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("stores_and_clears", func(t *testing.T) {
		output := &fileOutput{}
		sentinel := errors.New("rotate failed")
		output.lastRotationError.Store(&sentinel)
		if got := output.LastRotationError(); got != sentinel {
			t.Fatalf("expected %v, got %v", sentinel, got)
		}
		output.lastRotationError.Store(nil)
		if got := output.LastRotationError(); got != nil {
			t.Fatalf("expected nil after clear, got %v", got)
		}
	})
}
