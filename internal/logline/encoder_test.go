package logline_test

import (
	"strings"
	"testing"
	"time"

	"github.com/pragmabits/axio/internal/logline"
)

func TestEncodeTime(t *testing.T) {
	timestamp := time.Date(2025, 6, 15, 10, 30, 0, 123456789, time.FixedZone("BRT", -3*3600))

	var encoded []string
	encoder := &sliceEncoder{values: &encoded}
	logline.EncodeTime(timestamp, encoder)

	if len(encoded) != 1 {
		t.Fatalf("expected 1 encoded value, got %d", len(encoded))
	}

	result := encoded[0]

	// Must be UTC
	if !strings.HasSuffix(result, "Z") {
		t.Errorf("expected UTC (Z suffix), got %s", result)
	}

	// Must contain nanoseconds
	if !strings.Contains(result, ".123456789") {
		t.Errorf("expected nanoseconds, got %s", result)
	}

	// Must parse back as RFC3339Nano
	parsed, err := time.Parse(time.RFC3339Nano, result)
	assertNoError(t, err)
	if !parsed.Equal(timestamp) {
		t.Errorf("parsed time %v does not equal original %v", parsed, timestamp)
	}
}

func TestJSONEncoderConfig(t *testing.T) {
	config := logline.JSONEncoderConfig()
	assertEqual(t, config.TimeKey, "timestamp")
	assertEqual(t, config.LevelKey, "level")
	assertEqual(t, config.MessageKey, "message")
	assertEqual(t, config.NameKey, "logger")
	assertEqual(t, config.CallerKey, "caller")
	assertEqual(t, config.StacktraceKey, "stacktrace")
}

func TestEventEncoderConfig(t *testing.T) {
	config := logline.EventEncoderConfig()
	assertEqual(t, config.TimeKey, "timestamp")
	assertEqual(t, config.LevelKey, "") // omitted
	assertEqual(t, config.MessageKey, "event")
	assertEqual(t, config.NameKey, "")       // omitted
	assertEqual(t, config.CallerKey, "")     // omitted
	assertEqual(t, config.StacktraceKey, "") // omitted
}

func TestConsoleEncoderConfig(t *testing.T) {
	config := logline.ConsoleEncoderConfig()
	assertEqual(t, config.TimeKey, "timestamp")
	assertEqual(t, config.LevelKey, "level")
	assertEqual(t, config.MessageKey, "message")
	assertEqual(t, config.NameKey, "logger")
	assertEqual(t, config.CallerKey, "caller")
	assertEqual(t, config.StacktraceKey, "stacktrace")
}

// sliceEncoder is a minimal PrimitiveArrayEncoder for testing.
type sliceEncoder struct {
	values *[]string
}

func (e *sliceEncoder) AppendString(value string)   { *e.values = append(*e.values, value) }
func (e *sliceEncoder) AppendBool(bool)             {}
func (e *sliceEncoder) AppendByteString([]byte)     {}
func (e *sliceEncoder) AppendComplex128(complex128) {}
func (e *sliceEncoder) AppendComplex64(complex64)   {}
func (e *sliceEncoder) AppendFloat64(float64)       {}
func (e *sliceEncoder) AppendFloat32(float32)       {}
func (e *sliceEncoder) AppendInt(int)               {}
func (e *sliceEncoder) AppendInt64(int64)           {}
func (e *sliceEncoder) AppendInt32(int32)           {}
func (e *sliceEncoder) AppendInt16(int16)           {}
func (e *sliceEncoder) AppendInt8(int8)             {}
func (e *sliceEncoder) AppendUint(uint)             {}
func (e *sliceEncoder) AppendUint64(uint64)         {}
func (e *sliceEncoder) AppendUint32(uint32)         {}
func (e *sliceEncoder) AppendUint16(uint16)         {}
func (e *sliceEncoder) AppendUint8(uint8)           {}
func (e *sliceEncoder) AppendUintptr(uintptr)       {}
