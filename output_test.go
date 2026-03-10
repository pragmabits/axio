package axio

import (
	"errors"
	"os"
	"testing"
)

func TestConsole(t *testing.T) {
	out := Console(FormatText)

	assertEqual(t, out.Type(), OutputConsole)
	assertEqual(t, out.Format(), FormatText)

	// Console writes to stderr
	n, err := out.Write([]byte("test"))
	assertNoError(t, err)
	if n == 0 {
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

		out.Write([]byte("appended\n"))
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
			if r := recover(); r == nil {
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

		outputs, err := BuildOutputs(config)
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

		outputs, err := BuildOutputs(config)
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

		outputs, err := BuildOutputs(config)
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

		outputs, err := BuildOutputs(config)
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

		_, err := BuildOutputs(config)
		assertError(t, err)
	})

	t.Run("invalid_file_path_fails", func(t *testing.T) {
		config := Config{
			Outputs: []OutputConfig{
				{Type: OutputFile, Format: FormatJSON, Path: "/nonexistent/dir/file.log"},
			},
		}

		_, err := BuildOutputs(config)
		assertError(t, err)
	})

	t.Run("unknown_type_fails", func(t *testing.T) {
		config := Config{
			Outputs: []OutputConfig{
				{Type: OutputType("unknown"), Format: FormatJSON},
			},
		}

		_, err := BuildOutputs(config)
		assertError(t, err)
	})

	t.Run("empty_outputs_returns_empty_slice", func(t *testing.T) {
		config := Config{
			Outputs: []OutputConfig{},
		}

		outputs, err := BuildOutputs(config)
		assertNoError(t, err)

		if len(outputs) != 0 {
			t.Errorf("expected 0 outputs, got %d", len(outputs))
		}
	})
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

		for _, tt := range tests {
			var ot OutputType
			err := ot.UnmarshalText([]byte(tt.input))
			assertNoError(t, err)
			assertEqual(t, ot, tt.want)
		}
	})

	t.Run("invalid_type", func(t *testing.T) {
		var ot OutputType
		err := ot.UnmarshalText([]byte("invalid"))
		assertError(t, err)
	})
}

func TestFormat_UnmarshalText(t *testing.T) {
	t.Run("valid_formats", func(t *testing.T) {
		tests := []struct {
			input string
			want  Format
		}{
			{"json", FormatJSON},
			{"text", FormatText},
			{" json ", FormatJSON},
		}

		for _, tt := range tests {
			var f Format
			err := f.UnmarshalText([]byte(tt.input))
			assertNoError(t, err)
			assertEqual(t, f, tt.want)
		}
	})

	t.Run("invalid_format", func(t *testing.T) {
		var f Format
		err := f.UnmarshalText([]byte("invalid"))
		assertError(t, err)
	})
}

func TestEnvironment_UnmarshalText(t *testing.T) {
	t.Run("valid_environments", func(t *testing.T) {
		tests := []struct {
			input string
			want  Environment
		}{
			{"production", Production},
			{"staging", Staging},
			{"development", Development},
			{" production ", Production},
		}

		for _, tt := range tests {
			var e Environment
			err := e.UnmarshalText([]byte(tt.input))
			assertNoError(t, err)
			assertEqual(t, e, tt.want)
		}
	})

	t.Run("invalid_environment", func(t *testing.T) {
		var e Environment
		err := e.UnmarshalText([]byte("invalid"))
		assertError(t, err)
	})
}

func TestLevel_UnmarshalText(t *testing.T) {
	t.Run("valid_levels", func(t *testing.T) {
		tests := []struct {
			input string
			want  Level
		}{
			{"debug", LevelDebug},
			{"info", LevelInfo},
			{"warn", LevelWarn},
			{"error", LevelError},
			{" info ", LevelInfo},
		}

		for _, tt := range tests {
			var l Level
			err := l.UnmarshalText([]byte(tt.input))
			assertNoError(t, err)
			assertEqual(t, l, tt.want)
		}
	})

	t.Run("invalid_level", func(t *testing.T) {
		var l Level
		err := l.UnmarshalText([]byte("invalid"))
		assertError(t, err)
	})
}
