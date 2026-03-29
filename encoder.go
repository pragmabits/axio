package axio

import (
	"time"

	"go.uber.org/zap/zapcore"
)

// jsonEncoderConfig defines the JSON encoder configuration for structured logs.
//
// Features:
//   - Timestamps in UTC using RFC3339Nano format
//   - Levels in lowercase (debug, info, warn, error)
//   - Durations in milliseconds
//   - Caller in short format (file:line)
var jsonEncoderConfig = zapcore.EncoderConfig{
	TimeKey:        "timestamp",
	LevelKey:       "level",
	MessageKey:     "message",
	NameKey:        "logger",
	CallerKey:      "caller",
	StacktraceKey:  "stacktrace",
	LineEnding:     zapcore.DefaultLineEnding,
	EncodeLevel:    zapcore.LowercaseLevelEncoder,
	EncodeTime:     encodeRFC3339NanoUTC,
	EncodeDuration: zapcore.MillisDurationEncoder,
	EncodeCaller:   zapcore.ShortCallerEncoder,
}

// consoleEncoderConfig defines the console encoder configuration for readable logs.
//
// Features:
//   - Colored levels in uppercase (DEBUG, INFO, WARN, ERROR)
//   - Timestamps in ISO8601 format
//   - Durations in readable string format
//   - Ideal for local development
var consoleEncoderConfig = zapcore.EncoderConfig{
	TimeKey:        "timestamp",
	LevelKey:       "level",
	MessageKey:     "message",
	NameKey:        "logger",
	CallerKey:      "caller",
	StacktraceKey:  "stacktrace",
	FunctionKey:    zapcore.OmitKey,
	LineEnding:     zapcore.DefaultLineEnding,
	EncodeLevel:    zapcore.CapitalColorLevelEncoder,
	EncodeTime:     zapcore.ISO8601TimeEncoder,
	EncodeDuration: zapcore.StringDurationEncoder,
	EncodeCaller:   zapcore.ShortCallerEncoder,
}

// eventEncoderConfig defines the JSON encoder configuration for wide events.
//
// Differences from jsonEncoderConfig:
//   - LevelKey omitted (wide events don't use severity levels)
//   - MessageKey is "event" (carries the event name)
//   - NameKey, CallerKey, StacktraceKey omitted (not relevant for events)
var eventEncoderConfig = zapcore.EncoderConfig{
	TimeKey:        "timestamp",
	LevelKey:       zapcore.OmitKey,
	MessageKey:     "event",
	NameKey:        zapcore.OmitKey,
	CallerKey:      zapcore.OmitKey,
	StacktraceKey:  zapcore.OmitKey,
	LineEnding:     zapcore.DefaultLineEnding,
	EncodeLevel:    zapcore.LowercaseLevelEncoder,
	EncodeTime:     encodeRFC3339NanoUTC,
	EncodeDuration: zapcore.MillisDurationEncoder,
}

// encodeRFC3339NanoUTC encodes the timestamp in UTC using RFC3339 format with nanoseconds.
func encodeRFC3339NanoUTC(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	var buf [len(time.RFC3339Nano) + 10]byte
	enc.AppendString(string(t.UTC().AppendFormat(buf[:0], time.RFC3339Nano)))
}
