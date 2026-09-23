package axio

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// tempDir creates a temporary directory for testing.
// The directory is automatically cleaned up when the test completes.
func tempDir(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("", "axio-test-*")
	if err != nil {
		t.Fatalf("create temporary directory: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(directory)
	})
	return directory
}

// tempFile creates a temporary file for testing.
// Returns the file path. The file is automatically cleaned up when the test completes.
func tempFile(t *testing.T, name string) string {
	t.Helper()
	directory := tempDir(t)
	return filepath.Join(directory, name)
}

// writeFile writes content to a file for testing.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

// readFile reads file content for testing.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	return string(data)
}

// minimalConfig returns a minimal valid configuration for testing.
func minimalConfig() Config {
	return Config{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Environment:    EnvironmentDevelopment,
		Level:          LevelInfo,
	}
}

// assertError checks that err is not nil.
func assertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// assertNoError checks that err is nil.
func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// stringReader creates an io.Reader from a string for testing.
func stringReader(content string) io.Reader {
	return strings.NewReader(content)
}

// assertEqual checks that got equals want.
func assertEqual[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

// captureStdout redirects os.Stdout to a pipe. The returned function restores
// os.Stdout and returns everything written to it in between. Outputs built by
// Stdout capture os.Stdout when created, so the logger must be built after
// this call.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	return captureStream(t, &os.Stdout)
}

// captureStderr is [captureStdout] for os.Stderr, where the log path reports
// the failures it cannot return.
func captureStderr(t *testing.T) func() string {
	t.Helper()
	return captureStream(t, &os.Stderr)
}

// captureStream redirects *stream to a pipe until the returned function is
// called, and returns everything written to it in between.
func captureStream(t *testing.T, stream **os.File) func() string {
	t.Helper()
	original := *stream
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	*stream = writer
	t.Cleanup(func() { *stream = original })

	return func() string {
		t.Helper()
		*stream = original
		if err := writer.Close(); err != nil {
			t.Fatalf("close pipe writer: %v", err)
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("read pipe: %v", err)
		}
		if err := reader.Close(); err != nil {
			t.Fatalf("close pipe reader: %v", err)
		}
		return string(content)
	}
}

// readLines returns the non-empty lines of the file at path.
func readLines(t *testing.T, path string) []string {
	t.Helper()
	var lines []string
	for _, line := range strings.Split(readFile(t, path), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// bufferOutput is an Output that keeps everything written to it in memory.
type bufferOutput struct {
	format Format
	mutex  sync.Mutex
	buffer bytes.Buffer
}

// newBufferOutput returns an empty bufferOutput encoding in format.
func newBufferOutput(format Format) *bufferOutput {
	return &bufferOutput{format: format}
}

func (b *bufferOutput) Format() Format   { return b.format }
func (b *bufferOutput) Type() OutputType { return OutputStdout }
func (b *bufferOutput) Sync() error      { return nil }
func (b *bufferOutput) Close() error     { return nil }

func (b *bufferOutput) Write(data []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.buffer.Write(data)
}

// String returns everything written so far.
func (b *bufferOutput) String() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.buffer.String()
}

// failingOutput is an Output whose every write fails.
type failingOutput struct {
	format Format
}

func (f failingOutput) Format() Format   { return f.format }
func (f failingOutput) Type() OutputType { return OutputStdout }
func (f failingOutput) Sync() error      { return nil }
func (f failingOutput) Close() error     { return nil }

func (f failingOutput) Write([]byte) (int, error) {
	return 0, errors.New("disk full")
}
