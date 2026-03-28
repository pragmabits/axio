package axio

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tempDir creates a temporary directory for testing.
// The directory is automatically cleaned up when the test completes.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "axio-test-*")
	if err != nil {
		t.Fatalf("create temporary directory: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})
	return dir
}

// tempFile creates a temporary file for testing.
// Returns the file path. The file is automatically cleaned up when the test completes.
func tempFile(t *testing.T, name string) string {
	t.Helper()
	dir := tempDir(t)
	return filepath.Join(dir, name)
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
		Environment:    Development,
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
