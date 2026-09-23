package cli_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewRootCommand_Render(t *testing.T) {
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
