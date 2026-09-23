package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pragmabits/axio/cmd/axio/internal/cli"
)

const (
	infoLine     = `{"level":"info","timestamp":"2026-09-23T14:02:55.603542381Z","logger":"orders","caller":"probe/main.go:22","message":"order created","order_id":"ord_8812"}` + "\n"
	infoRendered = "2026-09-23T14:02:55.603Z\tINFO\torders\tprobe/main.go:22\torder created\t{\"order_id\": \"ord_8812\"}\n"
	warnLine     = `{"level":"warn","timestamp":"2026-09-23T14:02:56.100000000Z","message":"payment retry"}` + "\n"
	warnRendered = "2026-09-23T14:02:56.100Z\tWARN\tpayment retry\n"
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

// execute runs the axio command with input on standard input and returns
// everything it wrote.
func execute(t *testing.T, input string, arguments ...string) (string, error) {
	t.Helper()
	root := cli.NewRootCommand()
	var output bytes.Buffer
	root.SetIn(strings.NewReader(input))
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs(arguments)
	err := root.Execute()
	return output.String(), err
}

// writeTemp writes content to a file named name in a temporary directory.
func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}
