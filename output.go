package axio

import (
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// OutputType defines the log output destination.
type OutputType string

const (
	// OutputConsole writes to stderr.
	// Typically used in development to avoid interfering with stdout.
	OutputConsole OutputType = "console"
	// OutputStdout writes to stdout.
	// Typically used in containers with log collection agents.
	OutputStdout OutputType = "stdout"
	// OutputFile writes to a local file.
	// Useful for environments without collection agents or for audit logs.
	OutputFile OutputType = "file"
)

// Validate checks if the output type is a valid value.
//
// Returns [ErrInvalidOutputType] if the value is not one of the defined types.
func (o OutputType) Validate() error {
	switch o {
	case OutputConsole, OutputStdout, OutputFile:
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrInvalidOutputType, o)
	}
}

// UnmarshalText implements [encoding.TextUnmarshaler] for validation during parsing.
func (o *OutputType) UnmarshalText(text []byte) error {
	value := OutputType(strings.TrimSpace(string(text)))
	if err := value.Validate(); err != nil {
		return err
	}
	*o = value
	return nil
}

// RotationConfig configures log file rotation behavior.
//
// Rotation can be triggered by file size, time interval, or both.
// When both are configured, whichever condition is met first triggers rotation.
//
// YAML example:
//
//	outputs:
//	  - type: file
//	    format: json
//	    path: /var/log/app.log
//	    rotation:
//	      maxSize: 100
//	      maxAge: 30
//	      maxBackups: 10
//	      compress: true
//	      interval: 24h
type RotationConfig struct {
	// MaxSize is the maximum size in megabytes before rotation.
	// Zero means no size-based rotation.
	MaxSize int `json:"maxSize,omitempty" yaml:"maxSize,omitempty" toml:"maxSize,omitempty" mapstructure:"maxSize,omitempty"`
	// MaxAge is the maximum number of days to retain old log files.
	// Zero means no age-based cleanup.
	MaxAge int `json:"maxAge,omitempty" yaml:"maxAge,omitempty" toml:"maxAge,omitempty" mapstructure:"maxAge,omitempty"`
	// MaxBackups is the maximum number of old log files to retain.
	// Zero means retain all old files (subject to MaxAge).
	MaxBackups int `json:"maxBackups,omitempty" yaml:"maxBackups,omitempty" toml:"maxBackups,omitempty" mapstructure:"maxBackups,omitempty"`
	// Compress determines whether rotated files are compressed with gzip.
	Compress bool `json:"compress,omitempty" yaml:"compress,omitempty" toml:"compress,omitempty" mapstructure:"compress,omitempty"`
	// LocalTime determines whether the timestamps in backup file names use local time.
	// By default, UTC is used.
	LocalTime bool `json:"localTime,omitempty" yaml:"localTime,omitempty" toml:"localTime,omitempty" mapstructure:"localTime,omitempty"`
	// Interval is the time duration between rotations (e.g., "24h", "1h", "30m").
	// Zero means no time-based rotation.
	Interval Duration `json:"interval,omitempty" yaml:"interval,omitempty" toml:"interval,omitempty" mapstructure:"interval,omitempty"`
}

// Enabled reports whether any rotation is configured.
func (rotation RotationConfig) Enabled() bool {
	return rotation.MaxSize > 0 || rotation.Interval > 0
}

// OutputConfig represents output configuration for serialization.
//
// This struct allows outputs to be configured via file (YAML, JSON, TOML)
// and then converted to concrete [Output] objects during logger creation.
//
// Example in YAML:
//
//	outputs:
//	  - type: stdout
//	    format: json
//	  - type: file
//	    format: json
//	    path: /var/log/app.log
//	    rotation:
//	      maxSize: 100
//	      interval: 24h
type OutputConfig struct {
	// Type defines the output destination (console, stdout, file).
	Type OutputType `json:"type" yaml:"type" toml:"type" mapstructure:"type"`
	// Format defines the encoding format (json, text).
	Format Format `json:"format" yaml:"format" toml:"format" mapstructure:"format"`
	// Path is the file path (required only when Type is "file").
	Path string `json:"path,omitempty" yaml:"path,omitempty" toml:"path,omitempty" mapstructure:"path,omitempty"`
	// Rotation configures log file rotation (only used when Type is "file").
	Rotation RotationConfig `json:"rotation,omitzero" yaml:"rotation,omitempty" toml:"rotation,omitempty" mapstructure:"rotation,omitempty"`
}

// BuildOutputs creates concrete outputs from configuration.
//
// Processing order:
//  1. Iterates over [Config.Outputs]
//  2. Creates concrete output for each configuration
//  3. For [OutputFile], opens the specified file
//
// Returns error if any output cannot be created.
func BuildOutputs(config Config) ([]Output, error) {
	outputs := make([]Output, 0, len(config.Outputs))

	for index, outputConfig := range config.Outputs {
		var output Output
		var err error

		switch outputConfig.Type {
		case OutputConsole:
			output = Console(outputConfig.Format)

		case OutputStdout:
			output = Stdout(outputConfig.Format)

		case OutputFile:
			if outputConfig.Path == "" {
				return nil, fmt.Errorf("output[%d]: type 'file' requires 'path'", index)
			}
			if outputConfig.Rotation.Enabled() {
				output, err = RotatingFile(outputConfig.Path, outputConfig.Format, outputConfig.Rotation)
			} else {
				output, err = File(outputConfig.Path, outputConfig.Format)
			}
			if err != nil {
				return nil, fmt.Errorf("output[%d]: %w", index, err)
			}

		default:
			return nil, fmt.Errorf("output[%d]: unknown type: %s", index, outputConfig.Type)
		}

		outputs = append(outputs, output)
	}

	return outputs, nil
}

// Output defines a log output destination with its format and type.
//
// The interface allows configuring multiple destinations simultaneously,
// each with its own encoding format.
//
// Available implementations:
//   - [Console]: writes to stderr (development)
//   - [Stdout]: writes to stdout (containers)
//   - [File]: writes to local file
//
// Example:
//
//	logger, _ := axio.New(config,
//	    axio.WithOutputs(
//	        axio.Console(axio.FormatText),
//	        axio.Stdout(axio.FormatJSON),
//	    ),
//	)
type Output interface {
	// Format returns the encoding format of this output.
	Format() Format
	// Type returns the output destination type.
	Type() OutputType
	// Write writes data to the output.
	Write([]byte) (int, error)
	// Sync flushes buffered data to the output.
	Sync() error
	// Close releases resources associated with the output.
	// Returns nil for outputs that don't require cleanup (console, stdout).
	Close() error
}

// output is the internal implementation of Output.
type output struct {
	zapcore.WriteSyncer
	format     Format
	outputType OutputType
}

func (o *output) Format() Format {
	return o.format
}

func (o *output) Type() OutputType {
	return o.outputType
}

// Close returns nil for outputs that don't require cleanup.
func (o *output) Close() error {
	return nil
}

// Console creates an output that writes to stderr.
//
// Typically used in development with [FormatText] for colorized output.
// Using stderr avoids mixing logs with normal program output to stdout.
//
// Example:
//
//	logger, _ := axio.New(config,
//	    axio.WithOutputs(axio.Console(axio.FormatText)),
//	)
func Console(format Format) Output {
	return &output{
		WriteSyncer: zapcore.Lock(os.Stderr),
		format:      format,
		outputType:  OutputConsole,
	}
}

// Stdout creates an output that writes to stdout.
//
// Typically used in container environments where log agents collect from stdout.
// [FormatJSON] is recommended for log aggregation systems.
//
// This is the default output when [WithAgentMode] is used.
//
// Example:
//
//	logger, _ := axio.New(config,
//	    axio.WithOutputs(axio.Stdout(axio.FormatJSON)),
//	)
func Stdout(format Format) Output {
	return &output{
		WriteSyncer: zapcore.Lock(os.Stdout),
		format:      format,
		outputType:  OutputStdout,
	}
}

// fileOutput encapsulates a file with its path for identification.
// When rotation is configured, lumberjack handles the underlying file;
// otherwise, a plain os.File is used.
type fileOutput struct {
	*output
	path       string
	file       *os.File           // used for plain files (nil when rotating)
	lumberjack *lumberjack.Logger // used for rotating files (nil otherwise)
	interval   Duration           // time-based rotation interval (zero if disabled)
	ticker     *time.Ticker       // time-based rotation (nil otherwise)
	done       chan struct{}       // stops time rotation goroutine (nil otherwise)
}

// Close releases resources associated with this file output.
// For rotating files, stops the time-based rotation goroutine and closes lumberjack.
// For plain files, closes the os.File.
func (f *fileOutput) Close() error {
	if f.ticker != nil {
		f.ticker.Stop()
		close(f.done)
	}
	if f.lumberjack != nil {
		if err := f.lumberjack.Close(); err != nil {
			return fmt.Errorf("close rotating file %s: %w", f.path, err)
		}
		return nil
	}
	if f.file != nil {
		if err := f.file.Close(); err != nil {
			return fmt.Errorf("close file %s: %w", f.path, err)
		}
	}
	return nil
}

func (f *fileOutput) startTimeRotation(interval time.Duration) {
	f.ticker = time.NewTicker(interval)
	f.done = make(chan struct{})

	go func() {
		for {
			select {
			case <-f.ticker.C:
				_ = f.lumberjack.Rotate()
			case <-f.done:
				return
			}
		}
	}()
}

// File creates an output that writes to a file at the specified path.
//
// The file is created if it doesn't exist, or logs are appended if it already exists.
// Returns [ErrOpenFile] if the file cannot be opened.
//
// The file must be closed by calling [Logger.Close] when the logger is no longer needed.
//
// Example:
//
//	out, err := axio.File("/var/log/app.log", axio.FormatJSON)
//	if err != nil {
//	    return fmt.Errorf("failed to create file output: %w", err)
//	}
//	logger, _ := axio.New(settings, axio.WithOutputs(out))
//	defer logger.Close()
func File(path string, format Format) (Output, error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrOpenFile, path, err)
	}

	return &fileOutput{
		output: &output{
			WriteSyncer: zapcore.AddSync(file),
			format:      format,
			outputType:  OutputFile,
		},
		path: path,
		file: file,
	}, nil
}

// MustFile is like [File] but panics on error.
//
// Useful for initialization where failure should be fatal.
//
// Example:
//
//	logger, _ := axio.New(settings,
//	    axio.WithOutputs(axio.MustFile("/var/log/app.log", axio.FormatJSON)),
//	)
func MustFile(path string, format Format) Output {
	out, err := File(path, format)
	if err != nil {
		panic(err)
	}
	return out
}

// RotatingFile creates a file output with automatic rotation.
//
// Rotation is triggered by file size (MaxSize), time interval (Interval), or both.
// When both are configured, whichever condition is met first triggers rotation.
//
// Example:
//
//	out, err := axio.RotatingFile("/var/log/app.log", axio.FormatJSON, axio.RotationConfig{
//	    MaxSize:    100,
//	    MaxBackups: 5,
//	    MaxAge:     30,
//	    Compress:   true,
//	    Interval:   axio.Duration(24 * time.Hour),
//	})
func RotatingFile(path string, format Format, rotation RotationConfig) (Output, error) {
	lumber := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    rotation.MaxSize,
		MaxAge:     rotation.MaxAge,
		MaxBackups: rotation.MaxBackups,
		Compress:   rotation.Compress,
		LocalTime:  rotation.LocalTime,
	}

	fileOut := &fileOutput{
		output: &output{
			WriteSyncer: zapcore.AddSync(lumber),
			format:      format,
			outputType:  OutputFile,
		},
		path:       path,
		lumberjack: lumber,
		interval:   rotation.Interval,
	}

	if rotation.Interval > 0 {
		fileOut.startTimeRotation(time.Duration(rotation.Interval))
	}

	return fileOut, nil
}

// MustRotatingFile is like [RotatingFile] but panics on error.
//
// Useful for initialization where failure should be fatal.
//
// Example:
//
//	logger, _ := axio.New(config,
//	    axio.WithOutputs(
//	        axio.MustRotatingFile("/var/log/app.log", axio.FormatJSON, axio.RotationConfig{
//	            MaxSize:  100,
//	            Compress: true,
//	        }),
//	    ),
//	)
func MustRotatingFile(path string, format Format, rotation RotationConfig) Output {
	out, err := RotatingFile(path, format, rotation)
	if err != nil {
		panic(err)
	}
	return out
}
