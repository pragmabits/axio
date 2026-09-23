package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pragmabits/axio/internal/cli"
)

const (
	infoLine     = `{"level":"info","timestamp":"2026-09-23T14:02:55.603542381Z","logger":"orders","caller":"probe/main.go:22","message":"order created","order_id":"ord_8812"}` + "\n"
	infoRendered = "2026-09-23T14:02:55.603Z\tINFO\torders\tprobe/main.go:22\torder created\t{\"order_id\": \"ord_8812\"}\n"
	warnLine     = `{"level":"warn","timestamp":"2026-09-23T14:02:56.100000000Z","message":"payment retry"}` + "\n"
	warnRendered = "2026-09-23T14:02:56.100Z\tWARN\tpayment retry\n"
)

func TestRender(t *testing.T) {
	t.Run("reads_standard_input", func(t *testing.T) {
		output, err := execute(t, infoLine, "render", "--utc")
		assertNoError(t, err)
		assertEqual(t, output, infoRendered)
	})

	t.Run("reads_files_in_order", func(t *testing.T) {
		first, second := writeTemp(t, "first.log", infoLine), writeTemp(t, "second.log", warnLine)
		output, err := execute(t, "", "render", "--utc", first, second)
		assertNoError(t, err)
		assertEqual(t, output, infoRendered+warnRendered)
	})

	t.Run("dash_reads_standard_input", func(t *testing.T) {
		first := writeTemp(t, "first.log", infoLine)
		output, err := execute(t, warnLine, "render", "--utc", first, "-")
		assertNoError(t, err)
		assertEqual(t, output, infoRendered+warnRendered)
	})

	t.Run("missing_file_fails_naming_it", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing.log")
		_, err := execute(t, "", "render", missing)
		if err == nil || !strings.Contains(err.Error(), missing) {
			t.Errorf("expected an error naming %s, got %v", missing, err)
		}
	})

	t.Run("local_time_by_default", func(t *testing.T) {
		output, err := execute(t, infoLine, "render")
		assertNoError(t, err)
		parsed, _ := time.Parse(time.RFC3339Nano, "2026-09-23T14:02:55.603542381Z")
		want := parsed.In(time.Local).Format("2006-01-02T15:04:05.000Z0700") + "\t"
		if !strings.HasPrefix(output, want) {
			t.Errorf("expected local time %q, got %q", want, output)
		}
	})
}

func TestColorFlag(t *testing.T) {
	const coloredInfo = "\x1b[34mINFO\x1b[0m"

	t.Run("auto_does_not_color_a_pipe", func(t *testing.T) {
		output, err := execute(t, infoLine, "render")
		assertNoError(t, err)
		assertAbsent(t, output, "\x1b[")
	})

	t.Run("always_colors", func(t *testing.T) {
		output, err := execute(t, infoLine, "render", "--color=always")
		assertNoError(t, err)
		assertPresent(t, output, coloredInfo)
	})

	t.Run("never_does_not_color", func(t *testing.T) {
		output, err := execute(t, infoLine, "render", "--color", "never")
		assertNoError(t, err)
		assertAbsent(t, output, "\x1b[")
	})

	t.Run("always_wins_over_NO_COLOR", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		output, err := execute(t, infoLine, "render", "--color=always")
		assertNoError(t, err)
		assertPresent(t, output, coloredInfo)
	})

	t.Run("invalid_value_is_rejected", func(t *testing.T) {
		_, err := execute(t, infoLine, "render", "--color=sometimes")
		if err == nil || !strings.Contains(err.Error(), "auto, always or never") {
			t.Errorf("expected the allowed values in the error, got %v", err)
		}
	})

	t.Run("completion_offers_the_three_modes", func(t *testing.T) {
		output, err := execute(t, "", "__complete", "render", "--color", "")
		assertNoError(t, err)
		for _, mode := range []string{"auto", "always", "never"} {
			assertPresent(t, output, mode)
		}
	})
}

func TestRootCommand(t *testing.T) {
	t.Run("version_flag", func(t *testing.T) {
		output, err := execute(t, "", "--version")
		assertNoError(t, err)
		assertPresent(t, output, "axio version ")
	})

	t.Run("help_groups_render_under_reading_logs", func(t *testing.T) {
		output, err := execute(t, "", "--help")
		assertNoError(t, err)
		assertPresent(t, output, "Reading logs:\n  render")
	})

	t.Run("completion_scripts", func(t *testing.T) {
		output, err := execute(t, "", "completion", "bash")
		assertNoError(t, err)
		assertPresent(t, output, "bash completion")
	})

	t.Run("unknown_flag_points_to_help", func(t *testing.T) {
		_, err := execute(t, "", "render", "--bogus")
		if err == nil || !strings.Contains(err.Error(), "axio render --help") {
			t.Errorf("expected a pointer to the help, got %v", err)
		}
	})
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
