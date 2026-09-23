package cli

import (
	"errors"
	"io"
	"os"
)

// colorMode is the value of --color. It implements pflag.Value, so an invalid
// value is rejected while the flags are parsed, before any command runs.
type colorMode string

const (
	colorAuto   colorMode = "auto"
	colorAlways colorMode = "always"
	colorNever  colorMode = "never"
)

// errInvalidColor reports a --color value other than the three modes.
var errInvalidColor = errors.New("must be auto, always or never")

// enabled reports whether output written to writer is colored. auto colors
// only a terminal, and not even that when NO_COLOR is set; always and never
// are the user's explicit choice, so they win over NO_COLOR.
func (c colorMode) enabled(writer io.Writer) bool {
	switch c {
	case colorAlways:
		return true
	case colorNever:
		return false
	default:
		return os.Getenv("NO_COLOR") == "" && isTerminal(writer)
	}
}

// String returns the mode as the user wrote it.
func (c *colorMode) String() string {
	return string(*c)
}

// Set accepts one of the three modes.
func (c *colorMode) Set(value string) error {
	switch mode := colorMode(value); mode {
	case colorAuto, colorAlways, colorNever:
		*c = mode
		return nil
	default:
		return errInvalidColor
	}
}

// Type names the value in the help: --color when.
func (c *colorMode) Type() string {
	return "when"
}

// isTerminal reports whether writer is a terminal.
func isTerminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
