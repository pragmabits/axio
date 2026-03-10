// package axio provides a high-performance structured logger for Go applications.
//
// Axio is built on top of [go.uber.org/zap] and offers enterprise-grade features
// such as PII masking, hash chain auditing, and integration with
// OpenTelemetry for distributed tracing.
//
// # Basic Usage
//
// To create a simple logger:
//
//	config := axio.Config{
//	    ServiceName:    "my-service",
//	    ServiceVersion: "1.0.0",
//	    Environment:    axio.Development,
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
// Axio automatically detects and masks sensitive personal data:
//
//   - CPF: 123.456.789-01 → ***.***.***-**
//   - CNPJ: 12.345.678/0001-90 → **.***.***/**01-**
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
// # Hash Chain Auditing
//
// For logs that require integrity proof (LGPD, SOX, PCI-DSS compliance):
//
//	store := axio.NewFileStore("/var/lib/axio/chain.json")
//	hook, _ := axio.NewAuditHook(store)
//	logger, _ := axio.New(config, axio.WithHooks(hook))
//
// Each log entry receives a SHA256 hash that includes the hash of the previous entry,
// forming a cryptographic hash chain that detects any tampering.
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
//	logger.With(
//	    axio.Annotate("user_id", userID),
//	    axio.Annotate("tenant", tenantName),
//	    &axio.HTTP{
//	        Method:     "POST",
//	        URL:        "/api/v1/orders",
//	        StatusCode: 201,
//	        LatencyMS:  45,
//	    },
//	).Info(ctx, "order created successfully")
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
//   - [Development]: Colored console, no stack traces
//   - [Staging]: JSON, with stack traces on errors
//   - [Production]: JSON, with stack traces and sampling
package axio

import (
	"context"
	"fmt"
	"strings"
)

// Environment represents the execution environment of the application.
//
// The environment affects the default behavior of the logger:
//   - Output format (JSON vs colored text)
//   - Inclusion of stack traces
//   - Service metadata fields
type Environment string

const (
	// Production indicates production environment.
	// JSON logs with stack traces on errors and service metadata.
	Production Environment = "production"
	// Staging indicates staging environment.
	// Behavior similar to production for realistic testing.
	Staging Environment = "staging"
	// Development indicates development environment.
	// Colored text logs for better readability during development.
	Development Environment = "development"
)

// Validate checks whether the environment is a valid value.
//
// Returns [ErrInvalidEnvironment] if the value is not one of the defined environments.
func (e Environment) Validate() error {
	switch e {
	case Production, Staging, Development:
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrInvalidEnvironment, e)
	}
}

// UnmarshalText implements [encoding.TextUnmarshaler] for validation during parsing.
func (e *Environment) UnmarshalText(text []byte) error {
	value := Environment(strings.TrimSpace(string(text)))
	if err := value.Validate(); err != nil {
		return err
	}
	*e = value
	return nil
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
	value := Level(strings.TrimSpace(string(text)))
	if err := value.Validate(); err != nil {
		return err
	}
	*l = value
	return nil
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
	value := Format(strings.TrimSpace(string(text)))
	if err := value.Validate(); err != nil {
		return err
	}
	*f = value
	return nil
}

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
//	// Formatted log
//	logger.Info(ctx, "processed %d items in %v", count, duration)
//
//	// Log with structured annotations
//	logger.With(
//	    axio.Annotate("user_id", userID),
//	    &axio.HTTP{Method: "POST", URL: "/api/orders"},
//	).Info(ctx, "order created")
//
//	// Error log
//	logger.Error(ctx, err, "failed to process payment")
type Logger interface {
	// Named creates a sub-logger with an additional name.
	// Names are concatenated with dots (e.g., "app.http.handler").
	Named(string) Logger
	// Debug logs a debug message.
	Debug(context.Context, string, ...any)
	// Info logs an informational message.
	Info(context.Context, string, ...any)
	// Warn logs a warning with the associated error.
	Warn(context.Context, error, string, ...any)
	// Error logs an error with the associated error.
	Error(context.Context, error, string, ...any)
	// With returns a logger with additional annotations attached.
	With(...Annotation) Logger
	// Close releases resources associated with the logger (open files, etc).
	// It should be called when the logger is no longer needed.
	Close() error
}
