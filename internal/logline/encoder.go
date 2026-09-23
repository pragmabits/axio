package logline

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
	"time"

	"go.uber.org/zap/zapcore"
)

// JSONEncoderConfig returns the encoder settings of a JSON log line:
// timestamps in UTC with nanoseconds, lowercase levels, durations in
// milliseconds, the caller as file:line and any other value encoded with
// [ValueOptions].
func JSONEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:             TimeKey,
		LevelKey:            LevelKey,
		MessageKey:          MessageKey,
		NameKey:             LoggerKey,
		CallerKey:           CallerKey,
		StacktraceKey:       StacktraceKey,
		LineEnding:          zapcore.DefaultLineEnding,
		EncodeLevel:         zapcore.LowercaseLevelEncoder,
		EncodeTime:          EncodeTime,
		EncodeDuration:      zapcore.MillisDurationEncoder,
		EncodeCaller:        zapcore.ShortCallerEncoder,
		NewReflectedEncoder: newValueEncoder,
	}
}

// ConsoleEncoderConfig returns the encoder settings of the Console's text:
// colored uppercase levels, ISO8601 timestamps in local time, readable
// durations, the caller as file:line and any other value encoded with
// [ValueOptions].
func ConsoleEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:             TimeKey,
		LevelKey:            LevelKey,
		MessageKey:          MessageKey,
		NameKey:             LoggerKey,
		CallerKey:           CallerKey,
		StacktraceKey:       StacktraceKey,
		FunctionKey:         zapcore.OmitKey,
		LineEnding:          zapcore.DefaultLineEnding,
		EncodeLevel:         zapcore.CapitalColorLevelEncoder,
		EncodeTime:          zapcore.ISO8601TimeEncoder,
		EncodeDuration:      zapcore.StringDurationEncoder,
		EncodeCaller:        zapcore.ShortCallerEncoder,
		NewReflectedEncoder: newValueEncoder,
	}
}

// EventEncoderConfig returns the encoder settings of a wide event, which is
// always JSON. It differs from [JSONEncoderConfig] in carrying the event name
// under [EventKey] and in having no level, logger name, caller or stacktrace:
// an event's severity lives in its own fields. Any other value is encoded with
// [ValueOptions].
func EventEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:             TimeKey,
		LevelKey:            zapcore.OmitKey,
		MessageKey:          EventKey,
		NameKey:             zapcore.OmitKey,
		CallerKey:           zapcore.OmitKey,
		StacktraceKey:       zapcore.OmitKey,
		LineEnding:          zapcore.DefaultLineEnding,
		EncodeLevel:         zapcore.LowercaseLevelEncoder,
		EncodeTime:          EncodeTime,
		EncodeDuration:      zapcore.MillisDurationEncoder,
		NewReflectedEncoder: newValueEncoder,
	}
}

// ValueOptions returns the options of the JSON encoding every encoder config
// writes a value with when zap has no encoding of its own for it — a struct, a
// slice, a map — joined with marshalers, which take precedence over axio's own.
// PII masking encodes a value with the same options, so what it masks is what
// the log writes.
//
// The encoding is encoding/json/v2 with nil slices and maps written as null,
// map keys sorted, invalid UTF-8 replaced by U+FFFD, duplicate names kept, HTML
// left unescaped and a time.Duration written as its nanoseconds.
//
// Example:
//
//	encoded, err := json.Marshal(value, logline.ValueOptions())
func ValueOptions(marshalers ...*json.Marshalers) json.Options {
	return json.JoinOptions(valueFormat, json.WithMarshalers(json.JoinMarshalers(json.JoinMarshalers(marshalers...), durationNanoseconds)))
}

// EncodeTime encodes a timestamp in UTC as RFC3339 with nanoseconds.
func EncodeTime(timestamp time.Time, encoder zapcore.PrimitiveArrayEncoder) {
	var buffer [len(time.RFC3339Nano) + 10]byte
	encoder.AppendString(string(timestamp.UTC().AppendFormat(buffer[:0], time.RFC3339Nano)))
}

// valueEncoder writes a value encoded with [ValueOptions].
type valueEncoder struct {
	writer io.Writer
}

// newValueEncoder returns the encoder every encoder config writes a value
// through when zap has no encoding of its own for it.
func newValueEncoder(writer io.Writer) zapcore.ReflectedEncoder {
	return valueEncoder{writer: writer}
}

func (v valueEncoder) Encode(value any) error {
	return json.MarshalWrite(v.writer, value, valueEncoding)
}

// valueFormat is every option of [ValueOptions] but the marshalers.
var valueFormat = json.JoinOptions(
	json.FormatNilSliceAsNull(true),
	json.FormatNilMapAsNull(true),
	json.Deterministic(true),
	jsontext.AllowInvalidUTF8(true),
	jsontext.AllowDuplicateNames(true),
)

// durationNanoseconds writes a time.Duration as its nanoseconds.
var durationNanoseconds = json.MarshalToFunc(func(encoder *jsontext.Encoder, duration time.Duration) error {
	return encoder.WriteToken(jsontext.Int(int64(duration)))
})

// valueEncoding is [ValueOptions] with no marshalers of a caller's, built once
// for every value the log writes.
var valueEncoding = ValueOptions()
