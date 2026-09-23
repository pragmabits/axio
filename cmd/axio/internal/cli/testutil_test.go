package cli_test

import (
	"strings"
	"testing"
)

// assertEqual checks that got equals want.
func assertEqual[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("got %q, want %q", any(got), any(want))
	}
}

// assertNoError checks that err is nil.
func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// assertPresent checks that text contains part.
func assertPresent(t *testing.T, text, part string) {
	t.Helper()
	if !strings.Contains(text, part) {
		t.Errorf("expected %q in:\n%s", part, text)
	}
}

// assertAbsent checks that text does not contain part.
func assertAbsent(t *testing.T, text, part string) {
	t.Helper()
	if strings.Contains(text, part) {
		t.Errorf("did not expect %q in:\n%s", part, text)
	}
}
