package axio

import (
	"strings"
	"testing"
	"time"
)

func TestEncodeRFC3339NanoUTC(t *testing.T) {
	ts := time.Date(2025, 6, 15, 10, 30, 0, 123456789, time.FixedZone("BRT", -3*3600))

	var encoded []string
	enc := &sliceEncoder{values: &encoded}
	encodeRFC3339NanoUTC(ts, enc)

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
	if !parsed.Equal(ts) {
		t.Errorf("parsed time %v does not equal original %v", parsed, ts)
	}
}

func TestJsonEncoderConfig(t *testing.T) {
	assertEqual(t, jsonEncoderConfig.TimeKey, "timestamp")
	assertEqual(t, jsonEncoderConfig.LevelKey, "level")
	assertEqual(t, jsonEncoderConfig.MessageKey, "message")
	assertEqual(t, jsonEncoderConfig.NameKey, "logger")
	assertEqual(t, jsonEncoderConfig.CallerKey, "caller")
	assertEqual(t, jsonEncoderConfig.StacktraceKey, "stacktrace")
}

func TestEventEncoderConfig(t *testing.T) {
	assertEqual(t, eventEncoderConfig.TimeKey, "timestamp")
	assertEqual(t, eventEncoderConfig.LevelKey, "")     // omitted
	assertEqual(t, eventEncoderConfig.MessageKey, "event")
	assertEqual(t, eventEncoderConfig.NameKey, "")       // omitted
	assertEqual(t, eventEncoderConfig.CallerKey, "")     // omitted
	assertEqual(t, eventEncoderConfig.StacktraceKey, "") // omitted
}

func TestConsoleEncoderConfig(t *testing.T) {
	assertEqual(t, consoleEncoderConfig.TimeKey, "timestamp")
	assertEqual(t, consoleEncoderConfig.LevelKey, "level")
	assertEqual(t, consoleEncoderConfig.MessageKey, "message")
	assertEqual(t, consoleEncoderConfig.NameKey, "logger")
	assertEqual(t, consoleEncoderConfig.CallerKey, "caller")
	assertEqual(t, consoleEncoderConfig.StacktraceKey, "stacktrace")
}

// sliceEncoder is a minimal PrimitiveArrayEncoder for testing.
type sliceEncoder struct {
	values *[]string
}

func (e *sliceEncoder) AppendString(s string)  { *e.values = append(*e.values, s) }
func (e *sliceEncoder) AppendBool(bool)         {}
func (e *sliceEncoder) AppendByteString([]byte) {}
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
