package axio

import (
	"errors"
	"fmt"
	"os"
	"sync/atomic"

	"go.uber.org/zap/zapcore"
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
	return unmarshalEnum(text, o)
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
//   - [RotatingFile]: writes to a local file, rotated by size, by time or both
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

// RotationErrorReporter is implemented by outputs that record the outcome of
// their rotation, by size or by time. Detect via type assertion on an [Output]:
//
//	if reporter, ok := out.(axio.RotationErrorReporter); ok {
//	    if err := reporter.LastRotationError(); err != nil {
//	        // ...
//	    }
//	}
type RotationErrorReporter interface {
	LastRotationError() error
}

// output is the internal implementation of Output.
type output struct {
	zapcore.WriteSyncer
	format     Format
	outputType OutputType
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

// fileOutput encapsulates a file with its path for identification. A rotating
// one writes through the rotator its file shares with every other output on
// it; a plain one writes to its own os.File. After Close, a write returns
// [ErrOutputClosed] wrapping [os.ErrClosed] and opens nothing again.
type fileOutput struct {
	*output
	path    string
	key     string      // the plain file's key in filesByPath (empty when rotating)
	file    *os.File    // used for plain files (nil when rotating)
	rotator *rotator    // used for rotating files (nil otherwise)
	owned   atomic.Bool // set by the Logger or Event the output is given to
	closed  atomic.Bool // set by the first Close
}

// File creates an output that writes to a file at the specified path.
//
// The file is created if it doesn't exist, or logs are appended if it already exists.
// Returns [ErrOpenFile] if the file cannot be opened, and [ErrRotationConflict]
// if this process has it open as a [RotatingFile].
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
	key, err := registerPlainFile(path, file)
	if err != nil {
		// Nothing was written through the file, so its close error says nothing.
		_ = file.Close()
		return nil, err
	}

	return &fileOutput{
		output: &output{
			WriteSyncer: zapcore.AddSync(file),
			format:      format,
			outputType:  OutputFile,
		},
		path: path,
		key:  key,
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
// It happens on a write: the one that would take the file past MaxSize, or the
// first once Interval has passed since the last rotation, whose time is kept as
// the modification time of the lock file beside the log (path.lock). A file
// nothing was written to is not rotated.
//
// Every output on one file shares this process's rotator for it, whatever path
// reaches the file, so each Logger and Event can open its own. The first takes
// the lock, which is held until the last one closes. Returns [ErrOpenFile] when
// the file cannot be opened, [ErrRotationConflict] when this process has it open
// with another rotation or as a [File], and [ErrRotatingFileLocked] when another
// process holds it.
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
	writer, err := openRotator(path, rotation)
	if err != nil {
		return nil, err
	}
	return &fileOutput{
		output: &output{
			WriteSyncer: writer,
			format:      format,
			outputType:  OutputFile,
		},
		path:    path,
		rotator: writer,
	}, nil
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

func (f *fileOutput) Write(data []byte) (int, error) {
	if f.closed.Load() {
		return 0, fmt.Errorf("%w: %s: %w", ErrOutputClosed, f.path, os.ErrClosed)
	}
	return f.output.Write(data)
}

// Close releases resources associated with this file output. A rotating
// file gives back its reference to the rotator, the last one closing the file
// and its lock; a plain file closes its os.File. A second call returns
// [ErrOutputClosed].
func (f *fileOutput) Close() error {
	if !f.closed.CompareAndSwap(false, true) {
		return fmt.Errorf("%w: %s", ErrOutputClosed, f.path)
	}
	if f.rotator != nil {
		if err := f.rotator.release(); err != nil {
			return fmt.Errorf("close rotating file %s: %w", f.path, err)
		}
		return nil
	}
	releasePlainFile(f.key)
	if f.file != nil {
		if err := f.file.Close(); err != nil {
			return fmt.Errorf("close file %s: %w", f.path, err)
		}
	}
	return nil
}

// claim makes the output belong to the Logger or Event being built. Returns
// [ErrOutputClosed] for a closed output and [ErrOutputInUse] for one that
// already belongs to another.
func (f *fileOutput) claim() error {
	if f.closed.Load() {
		return fmt.Errorf("%w: %s", ErrOutputClosed, f.path)
	}
	if !f.owned.CompareAndSwap(false, true) {
		return fmt.Errorf("%w: %s", ErrOutputInUse, f.path)
	}
	return nil
}

// unclaim gives the output back to its caller, after the Logger or Event it
// was given to failed to be built.
func (f *fileOutput) unclaim() {
	f.owned.Store(false)
}

// LastRotationError returns the error from the most recent rotation attempt,
// by size or by time, or nil if it succeeded or no rotation has been attempted.
// A plain file never rotates, and always returns nil.
func (f *fileOutput) LastRotationError() error {
	if f.rotator == nil {
		return nil
	}
	return f.rotator.lastRotationError()
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
	// Interval is the time after the last rotation, by size or by time, from
	// which the next write rotates the file (e.g., "24h", "1h", "30m").
	// Zero means no time-based rotation.
	Interval Duration `json:"interval,omitempty" yaml:"interval,omitempty" toml:"interval,omitempty" mapstructure:"interval,omitempty"`
}

// Enabled reports whether any rotation is configured.
func (r RotationConfig) Enabled() bool {
	return r.MaxSize > 0 || r.Interval > 0
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

// buildOutputs creates concrete outputs from configuration.
//
// When outputs were supplied directly via [WithOutputs], they are returned
// as-is (no file is opened twice). Otherwise the function resolves
// [Config.Outputs] into concrete [Output] instances:
//  1. Iterates over [Config.Outputs]
//  2. Creates concrete output for each configuration
//  3. For [OutputFile], opens the specified file
//
// Returns error if any output cannot be created, after closing the ones
// created before it.
func buildOutputs(config Config) ([]Output, error) {
	if len(config.resolvedOutputs) > 0 {
		return config.resolvedOutputs, nil
	}

	outputs := make([]Output, 0, len(config.Outputs))
	for index, outputConfig := range config.Outputs {
		output, err := buildOutput(index, outputConfig)
		if err != nil {
			return nil, errors.Join(err, closeOutputs(outputs))
		}
		outputs = append(outputs, output)
	}
	return outputs, nil
}

// buildOutput creates the output configured at index of [Config.Outputs].
func buildOutput(index int, outputConfig OutputConfig) (Output, error) {
	switch outputConfig.Type {
	case OutputConsole:
		return Console(outputConfig.Format), nil
	case OutputStdout:
		return Stdout(outputConfig.Format), nil
	case OutputFile:
		if outputConfig.Path == "" {
			return nil, fmt.Errorf("output[%d]: type 'file' requires 'path'", index)
		}
		var output Output
		var err error
		if outputConfig.Rotation.Enabled() {
			output, err = RotatingFile(outputConfig.Path, outputConfig.Format, outputConfig.Rotation)
		} else {
			output, err = File(outputConfig.Path, outputConfig.Format)
		}
		if err != nil {
			return nil, fmt.Errorf("output[%d]: %w", index, err)
		}
		return output, nil
	default:
		return nil, fmt.Errorf("output[%d]: unknown type: %s", index, outputConfig.Type)
	}
}

// openOutputs builds the outputs of config and claims those given through
// [WithOutputs] for the Logger or Event being built, all or none. Returns
// [ErrBuildOutputs], wrapping [ErrOutputInUse] or [ErrOutputClosed] for an
// output that cannot be claimed.
func openOutputs(config Config) ([]Output, error) {
	outputs, err := buildOutputs(config)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildOutputs, err)
	}
	if err := claimOutputs(outputs); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildOutputs, err)
	}
	return outputs, nil
}

// claimOutputs claims every file output of outputs, and none when one of them
// cannot be claimed.
func claimOutputs(outputs []Output) error {
	for index, output := range outputs {
		file, ok := output.(*fileOutput)
		if !ok {
			continue
		}
		if err := file.claim(); err != nil {
			unclaimOutputs(outputs[:index])
			return err
		}
	}
	return nil
}

// unclaimOutputs gives every file output of outputs back to its caller.
func unclaimOutputs(outputs []Output) {
	for _, output := range outputs {
		if file, ok := output.(*fileOutput); ok {
			file.unclaim()
		}
	}
}

// abandonOutputs undoes [openOutputs] for a Logger or Event that failed to be
// built after it, and returns cause: it closes the outputs built from
// [Config.Outputs] and gives those passed to [WithOutputs] back to the caller,
// who still holds them.
func abandonOutputs(config Config, outputs []Output, cause error) error {
	if len(config.resolvedOutputs) > 0 {
		unclaimOutputs(outputs)
		return cause
	}
	return errors.Join(cause, closeOutputs(outputs))
}

// closeOutputs closes every output, and returns the failures together.
func closeOutputs(outputs []Output) error {
	var errs []error
	for _, output := range outputs {
		if err := output.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
