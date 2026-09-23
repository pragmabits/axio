package cli_test

import (
	"strings"
	"testing"
)

func TestNewRootCommand_ColorFlag(t *testing.T) {
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
