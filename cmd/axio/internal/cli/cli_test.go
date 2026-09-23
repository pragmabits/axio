package cli_test

import (
	"strings"
	"testing"
)

func TestNewRootCommand(t *testing.T) {
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
