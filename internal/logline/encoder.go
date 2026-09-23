package logline

import (
	"time"

	"go.uber.org/zap/zapcore"
)

// JSONEncoderConfig returns the encoder settings of a JSON log line:
// timestamps in UTC with nanoseconds, lowercase levels, durations in
// milliseconds and the caller as file:line.
func JSONEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:        TimeKey,
		LevelKey:       LevelKey,
		MessageKey:     MessageKey,
		NameKey:        LoggerKey,
		CallerKey:      CallerKey,
		StacktraceKey:  StacktraceKey,
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     EncodeTime,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
}

// ConsoleEncoderConfig returns the encoder settings of the Console's text:
// colored uppercase levels, ISO8601 timestamps in local time, readable
// durations and the caller as file:line.
func ConsoleEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:        TimeKey,
		LevelKey:       LevelKey,
		MessageKey:     MessageKey,
		NameKey:        LoggerKey,
		CallerKey:      CallerKey,
		StacktraceKey:  StacktraceKey,
		FunctionKey:    zapcore.OmitKey,
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
}

// EventEncoderConfig returns the encoder settings of a wide event, which is
// always JSON. It differs from [JSONEncoderConfig] in carrying the event name
// under [EventKey] and in having no level, logger name, caller or stacktrace:
// an event's severity lives in its own fields.
func EventEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:        TimeKey,
		LevelKey:       zapcore.OmitKey,
		MessageKey:     EventKey,
		NameKey:        zapcore.OmitKey,
		CallerKey:      zapcore.OmitKey,
		StacktraceKey:  zapcore.OmitKey,
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     EncodeTime,
		EncodeDuration: zapcore.MillisDurationEncoder,
	}
}

// EncodeTime encodes a timestamp in UTC as RFC3339 with nanoseconds.
func EncodeTime(timestamp time.Time, encoder zapcore.PrimitiveArrayEncoder) {
	var buffer [len(time.RFC3339Nano) + 10]byte
	encoder.AppendString(string(timestamp.UTC().AppendFormat(buffer[:0], time.RFC3339Nano)))
}
