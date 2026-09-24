// Package axio provides a high-performance structured logger for Go applications.
//
// Axio offers enterprise-grade features such as PII masking, hash chain auditing,
// and integration with OpenTelemetry for distributed tracing, exposed through a
// small, stable public API that intentionally hides the underlying engine.
//
// # Basic Usage
//
// To create a simple logger:
//
//	config := axio.Config{
//	    ServiceName:    "my-service",
//	    ServiceVersion: "1.0.0",
//	    Environment:    axio.EnvironmentDevelopment,
//	    Level:          axio.LevelInfo,
//	}
//	logger, err := axio.New(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	logger.Info(ctx, "application started")
//
// # Configuration with Options
//
// The logger can be configured using the functional options pattern:
//
//	logger, err := axio.New(config,
//	    axio.WithOutputs(axio.Stdout(axio.FormatJSON)),
//	    axio.WithHooks(axio.MustPIIHook(axio.DefaultPIIConfig())),
//	    axio.WithTracer(axio.Otel()),
//	)
//
// # Multiple Outputs
//
// Axio supports multiple simultaneous outputs with different formats:
//
//	logger, err := axio.New(config,
//	    axio.WithOutputs(
//	        axio.Console(axio.FormatText),
//	        axio.Stdout(axio.FormatJSON),
//	        axio.MustFile("/var/log/app.log", axio.FormatJSON),
//	    ),
//	)
//
// # PII Masking
//
// Axio detects and masks sensitive personal data. CPF, CNPJ and credit cards are
// masked by default; e-mails ([PatternEmail]) and phones ([PatternPhone]) are
// masked when their patterns are listed:
//
//   - CPF: 123.456.789-01 → ***.***.***-**
//   - CNPJ: 12.345.678/0001-90 → **.***.***/****-**
//   - Credit cards: 1234-5678-9012-3456 → ****-****-****-****
//   - E-mails: user@example.com → ***@***.***
//   - Brazilian phones: (11) 99999-9999 → (**) *****-****
//
// Example:
//
//	piiHook := axio.MustPIIHook(axio.DefaultPIIConfig())
//	logger, _ := axio.New(config, axio.WithHooks(piiHook))
//	logger.Info(ctx, "Customer CPF: 123.456.789-01")
//	// Output: "Customer CPF: ***.***.***-**"
//
// Masking covers every value the caller hands a line — the message, the error
// and each annotation, structs and maps included; see [PIIMasker.MaskFields].
//
// # Hash Chain Auditing
//
// For logs that require integrity proof (LGPD, SOX, PCI-DSS compliance):
//
//	logger, _ := axio.New(config,
//	    axio.WithOutputs(axio.MustFile("/var/log/app.log", axio.FormatJSON)),
//	    axio.WithAudit("/var/lib/axio/chain.json"),
//	)
//
// Each JSON line ends with previous_hash and hash, the SHA-256 of the previous
// hash followed by the line's own bytes: a chain that detects any change,
// removal or reordering. [HashChain.Verify] checks a log against the chain.
// An audited Logger needs a JSON output, and a [FileStore] is held by one
// process at a time.
//
// # OpenTelemetry Integration
//
// Axio automatically adds trace_id and span_id to logs when
// configured with a tracer:
//
//	logger, _ := axio.New(config, axio.WithTracer(axio.Otel()))
//
// # Structured Annotations
//
// Contextualize your logs with typed annotations:
//
//	logger.Info(ctx, "order created successfully",
//	    axio.Field("user_id", userID),
//	    axio.Field("tenant", tenantName),
//	    axio.Field("http", axio.HTTP{
//	        Method:     "POST",
//	        URL:        "/api/v1/orders",
//	        StatusCode: 201,
//	        LatencyMS:  45,
//	    }),
//	)
//
// # Agent Mode
//
// For environments with log collection agents (Promtail, Fluent Bit, Filebeat):
//
//	logger, _ := axio.New(config, axio.WithAgentMode())
//
// This forces JSON output to stdout, optimized for collection by external agents.
//
// # Log Levels
//
// Axio supports four severity levels:
//
//   - [LevelDebug]: Detailed information for debug
//   - [LevelInfo]: Informational events about normal operations
//   - [LevelWarn]: Anomalous conditions that deserve attention
//   - [LevelError]: Errors that affect operation
//
// # Environments
//
// The logger behavior varies according to the execution environment:
//
//   - [EnvironmentDevelopment]: Colored console, no stack traces
//   - [EnvironmentStaging]: JSON, with stack traces on errors
//   - [EnvironmentProduction]: JSON, with stack traces on errors
package axio

import (
	"context"
	"fmt"
	"strings"
)

// Logger defines the main interface for structured logging.
//
// The interface provides methods for different severity levels
// (Debug, Info, Warn, Error) and supports contextualization through
// structured annotations.
//
// Logging methods accept a [context.Context] as the first parameter,
// allowing automatic integration with distributed tracing when a
// [Tracer] is configured.
//
// Example:
//
//	logger, _ := axio.New(settings)
//
//	// Simple log
//	logger.Info(ctx, "user authenticated")
//
//	// Log with annotations of its own
//	logger.Info(ctx, "order created",
//	    axio.Field("user_id", userID),
//	    axio.Field("http", axio.HTTP{Method: "POST", URL: "/api/orders"}),
//	)
//
//	// A logger that attaches annotations to every entry it writes
//	requestLogger := logger.With(axio.Field("request_id", requestID))
//
//	// Error log
//	logger.Error(ctx, err, "failed to process payment")
type Logger interface {
	// Named creates a sub-logger with an additional name.
	// Names are concatenated with dots (e.g., "app.http.handler").
	Named(string) Logger
	// Debug logs a debug message with the given annotations. The message is
	// written as given; it is never formatted.
	Debug(context.Context, string, ...Annotation)
	// Info logs an informational message with the given annotations.
	Info(context.Context, string, ...Annotation)
	// Warn logs a warning with the associated error and the given annotations.
	Warn(context.Context, error, string, ...Annotation)
	// Error logs an error with the associated error and the given annotations.
	Error(context.Context, error, string, ...Annotation)
	// With returns a logger that attaches the annotations to every entry it
	// writes, before the annotations given to each call.
	With(...Annotation) Logger
	// Close releases resources owned by the root logger (open files, rotation
	// goroutines, etc.). It should be called when the logger is no longer
	// needed, typically via defer in main.
	//
	// Only the root Logger (returned by [New]) owns resources. Calling Close
	// on a logger produced by [Logger.Named] or [Logger.With] returns
	// [ErrLoggerNotRoot] and leaves all resources untouched. A second Close
	// on the root returns [ErrLoggerClosed]. After the root is closed, log
	// calls on the root and on every fork become silent no-ops.
	Close() error
}

// Environment represents the execution environment of the application.
//
// The environment affects the default behavior of the logger:
//   - Output format (JSON vs colored text)
//   - Inclusion of stack traces
//   - Service metadata fields
type Environment string

const (
	// EnvironmentProduction indicates production environment.
	// JSON logs with stack traces on errors and service metadata.
	EnvironmentProduction Environment = "production"
	// EnvironmentStaging indicates staging environment.
	// Behavior similar to production for realistic testing.
	EnvironmentStaging Environment = "staging"
	// EnvironmentDevelopment indicates development environment.
	// Colored text logs for better readability during development.
	EnvironmentDevelopment Environment = "development"
)

// Validate checks whether the environment is a valid value.
//
// Returns [ErrInvalidEnvironment] if the value is not one of the defined environments.
func (e Environment) Validate() error {
	switch e {
	case EnvironmentProduction, EnvironmentStaging, EnvironmentDevelopment:
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrInvalidEnvironment, e)
	}
}

// UnmarshalText implements [encoding.TextUnmarshaler] for validation during parsing.
func (e *Environment) UnmarshalText(text []byte) error {
	return unmarshalEnum(text, e)
}

// Level represents the severity of a log entry.
//
// Levels follow the standard logging convention, from least to most severe:
// Debug < Info < Warn < Error.
//
// Logs with a level below the configured [Config.Level] are discarded.
type Level string

const (
	// LevelDebug indicates debug logs with detailed information.
	// Typically disabled in production due to volume.
	LevelDebug Level = "debug"
	// LevelInfo indicates informational logs about normal operations.
	// Default level for most applications in production.
	LevelInfo Level = "info"
	// LevelWarn indicates anomalous conditions that deserve attention.
	// The application keeps running, but something unexpected occurred.
	LevelWarn Level = "warn"
	// LevelError indicates errors that affect operation.
	// Requires immediate attention from the operations team.
	LevelError Level = "error"
)

// Validate checks whether the level is a valid value.
//
// Returns [ErrInvalidLevel] if the value is not one of the defined levels.
func (l Level) Validate() error {
	switch l {
	case LevelDebug, LevelInfo, LevelWarn, LevelError:
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrInvalidLevel, l)
	}
}

// UnmarshalText implements [encoding.TextUnmarshaler] for validation during parsing.
func (l *Level) UnmarshalText(text []byte) error {
	return unmarshalEnum(text, l)
}

// Format defines the encoding format of log output.
type Format string

const (
	// FormatJSON produces structured JSON logs.
	// Ideal for log aggregation systems such as Loki, Elasticsearch, or Splunk.
	FormatJSON Format = "json"
	// FormatText produces readable colored text logs.
	// Ideal for development and local debugging.
	FormatText Format = "text"
)

// Validate checks whether the format is a valid value.
//
// Returns [ErrInvalidFormat] if the value is not one of the defined formats.
func (f Format) Validate() error {
	switch f {
	case FormatJSON, FormatText:
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrInvalidFormat, f)
	}
}

// UnmarshalText implements [encoding.TextUnmarshaler] for validation during parsing.
func (f *Format) UnmarshalText(text []byte) error {
	return unmarshalEnum(text, f)
}

// unmarshalEnum sets target to text, trimmed, once the enum's Validate accepts
// it, and leaves target alone otherwise.
func unmarshalEnum[T interface {
	~string
	Validate() error
}](text []byte, target *T) error {
	value := T(strings.TrimSpace(string(text)))
	if err := value.Validate(); err != nil {
		return err
	}
	*target = value
	return nil
}
