package logline_test

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

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

// duplicateNames is a value whose own JSON repeats a name.
type duplicateNames struct{}

func (duplicateNames) MarshalJSON() ([]byte, error) { return []byte(`{"a":1,"a":2}`), nil }

func TestValueOptions(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name:  "duration_in_nanoseconds",
			value: struct{ Timeout time.Duration }{time.Second},
			want:  `{"Timeout":1000000000}`,
		},
		{
			name: "nil_slice_and_map_as_null",
			value: struct {
				List []int
				Set  map[string]int
			}{},
			want: `{"List":null,"Set":null}`,
		},
		{
			name:  "map_keys_sorted",
			value: map[string]int{"e": 5, "c": 3, "a": 1, "d": 4, "b": 2, "f": 6},
			want:  `{"a":1,"b":2,"c":3,"d":4,"e":5,"f":6}`,
		},
		{
			name:  "invalid_utf8_replaced",
			value: "a\xffb",
			want:  "\"a�b\"",
		},
		{
			name:  "duplicate_names_kept",
			value: duplicateNames{},
			want:  `{"a":1,"a":2}`,
		},
		{
			name:  "html_not_escaped",
			value: "<a&b>",
			want:  `"<a&b>"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.value, logline.ValueOptions())
			assertNoError(t, err)
			assertEqual(t, string(encoded), test.want)
		})
	}

	t.Run("marshalers_joined_with_axio_own", func(t *testing.T) {
		upper := json.MarshalToFunc(func(encoder *jsontext.Encoder, text string) error {
			return encoder.WriteToken(jsontext.String(strings.ToUpper(text)))
		})
		value := struct {
			Name    string
			Timeout time.Duration
		}{"ana", time.Second}

		encoded, err := json.Marshal(value, logline.ValueOptions(upper))
		assertNoError(t, err)
		assertEqual(t, string(encoded), `{"Name":"ANA","Timeout":1000000000}`)
	})
}

func TestValueOptions_WrittenByEveryEncoderConfig(t *testing.T) {
	value := struct {
		Active bool `json:",omitempty"`
		ID     [2]byte
	}{ID: [2]byte{1, 2}}
	want, err := json.Marshal(value, logline.ValueOptions())
	assertNoError(t, err)

	encoders := []struct {
		name    string
		encoder zapcore.Encoder
	}{
		{"json", zapcore.NewJSONEncoder(logline.JSONEncoderConfig())},
		{"console", zapcore.NewConsoleEncoder(logline.ConsoleEncoderConfig())},
		{"event", zapcore.NewJSONEncoder(logline.EventEncoderConfig())},
	}
	for _, test := range encoders {
		t.Run(test.name, func(t *testing.T) {
			line, err := test.encoder.EncodeEntry(zapcore.Entry{Message: "entry"}, []zapcore.Field{zap.Reflect("value", value)})
			assertNoError(t, err)
			if !strings.Contains(line.String(), string(want)) {
				t.Errorf("line %q does not carry the value as %s", line.String(), want)
			}
		})
	}
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
