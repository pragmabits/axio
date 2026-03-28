package axio

import (
	"fmt"
	"strings"
	"time"
)

// Duration is a [time.Duration] that supports unmarshaling from strings
// in configuration files (e.g., "24h", "1h30m", "500ms").
//
// Go's [time.Duration] doesn't unmarshal from YAML/JSON strings natively.
// This type implements [encoding.TextUnmarshaler] and [encoding.TextMarshaler]
// to bridge that gap.
//
// Example in YAML:
//
//	rotation:
//	  interval: 24h
//
// Example in Go:
//
//	rotation := axio.RotationConfig{
//	    Interval: axio.Duration(24 * time.Hour),
//	}
type Duration time.Duration

// UnmarshalText implements [encoding.TextUnmarshaler].
func (duration *Duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(strings.TrimSpace(string(text)))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(text), err)
	}
	*duration = Duration(parsed)
	return nil
}

// MarshalText implements [encoding.TextMarshaler].
func (duration Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(duration).String()), nil
}
