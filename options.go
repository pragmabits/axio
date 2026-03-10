package axio

import (
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

// Option is a function that configures the [Logger] during creation.
//
// Options follow Go's functional configuration pattern, allowing
// flexible and extensible configuration. Use with [New].
//
// Precedence order is: Config (file) → Options → Defaults.
// Options always override Config values.
//
// Available options:
//   - [WithOutputs]: configures output destinations
//   - [WithAgentMode]: optimizes for collection by external agents
//   - [WithHooks]: configures custom processing hooks
//   - [WithPII]: configures PII masking
//   - [WithAudit]: configures auditing with hash chain
//   - [WithMetrics]: configures metrics collection
//   - [WithTracer]: configures trace extraction
type Option func(*Config) error

// WithOutputs configures the logger to write to the specified destinations.
//
// Multiple outputs can be specified for simultaneous writing to
// different destinations. Each output can have its own format.
//
// This function accepts [Output] objects and converts them internally to [OutputConfig].
// For file-based configuration, use the [Config.Outputs] field directly.
//
// Example:
//
//	logger, _ := axio.New(config,
//	    axio.WithOutputs(
//	        axio.Console(axio.FormatText),  // development
//	        axio.Stdout(axio.FormatJSON),   // collection agent
//	        axio.MustFile("/var/log/app.log", axio.FormatJSON), // file
//	    ),
//	)
func WithOutputs(outputs ...Output) Option {
	return func(config *Config) error {
		for _, output := range outputs {
			outputConfig := OutputConfig{
				Type:   output.Type(),
				Format: output.Format(),
			}

			if fileOut, ok := output.(*fileOutput); ok {
				outputConfig.Path = fileOut.path
				if err := fileOut.Close(); err != nil {
					return fmt.Errorf("close intermediate file output %s: %w", fileOut.path, err)
				}
			}

			config.Outputs = append(config.Outputs, outputConfig)
		}
		return nil
	}
}

// WithAgentMode configures the logger for use with external log collection
// agents like Promtail, Fluent Bit, or Filebeat.
//
// This option:
//   - Sets [Config.AgentMode] to true
//   - Forces output to stdout with JSON format
//   - Overwrites any previous output
//
// When this option is used, logs are written to stdout in JSON format,
// allowing external agents to collect and forward them to aggregation
// systems like Loki, Elasticsearch, or Splunk.
//
// Example:
//
//	logger, _ := axio.New(config, axio.WithAgentMode())
func WithAgentMode() Option {
	return func(config *Config) error {
		config.AgentMode = true
		config.Outputs = []OutputConfig{
			{
				Type:   OutputStdout,
				Format: FormatJSON,
			},
		}
		return nil
	}
}

// WithHooks configures the logger to process log entries through the specified hooks.
//
// Hooks are executed in the order they were registered, before the entry
// is written to outputs. If a hook returns an error, processing is
// stopped and the entry is not written.
//
// NOTE: For PIIHook and AuditHook, prefer using [WithPII] and [WithAudit] respectively.
// This function is maintained for compatibility with custom hooks.
//
// Example:
//
//	logger, _ := axio.New(config,
//	    axio.WithHooks(myCustomHook),
//	)
func WithHooks(hooks ...Hook) Option {
	return func(config *Config) error {
		config.hooks = append(config.hooks, hooks...)
		return nil
	}
}

// WithPII enables PII masking with the specified patterns and fields.
//
// If patterns is nil or empty, uses default patterns (CPF, CNPJ, CreditCard).
// If fields is nil or empty, uses [DefaultSensitiveFields].
//
// Example:
//
//	logger, _ := axio.New(config,
//	    axio.WithPII(
//	        []axio.PIIPattern{axio.PatternCPF, axio.PatternEmail},
//	        axio.DefaultSensitiveFields,
//	    ),
//	)
func WithPII(patterns []PIIPattern, fields []string) Option {
	return func(config *Config) error {
		config.PIIEnabled = true
		if len(patterns) > 0 {
			config.PIIPatterns = patterns
		}
		if len(fields) > 0 {
			config.PIIFields = fields
		}
		return nil
	}
}

// WithAudit enables auditing with hash chain persisted at the specified path.
//
// The hash chain creates a tamper-proof audit trail, where each
// log entry receives a SHA256 hash that includes the previous entry's hash.
//
// Example:
//
//	logger, _ := axio.New(config,
//	    axio.WithAudit("/var/lib/axio/audit-chain.json"),
//	)
func WithAudit(storePath string) Option {
	return func(config *Config) error {
		config.Audit.Enabled = true
		config.Audit.StorePath = storePath
		return nil
	}
}

// WithMetrics configures the logger to emit metrics using the specified MeterProvider.
//
// BREAKING CHANGE v2.0: Now receives metric.MeterProvider instead of Metrics.
//
// If provider is nil, returns [ErrNilMetricsProvider].
//
// Emitted metrics include:
//   - logs.total: Log counter by level
//   - pii.masked: Masked PII counter by pattern
//   - audit.records: Audit records counter
//   - output.errors: Write errors counter by output type
//   - hook.duration: Hook duration histogram
//
// Example:
//
//	provider := otel.GetMeterProvider()
//	logger, err := axio.New(config, axio.WithMetrics(provider))
func WithMetrics(provider metric.MeterProvider) Option {
	return func(config *Config) error {
		if provider == nil {
			return ErrNilMetricsProvider
		}
		config.Metrics.Enabled = true
		config.metricsProvider = provider
		return nil
	}
}

// WithTracer configures the logger to extract trace information from context.
//
// When configured, the logger automatically adds trace_id and span_id
// to each log entry, enabling correlation between logs and traces in
// observability systems like Jaeger, Tempo, or Zipkin.
//
// Available tracers:
//   - [Otel]: extracts from OpenTelemetry spans
//   - [NoopTracing]: never extracts information (default)
//
// Example:
//
//	logger, _ := axio.New(config, axio.WithTracer(axio.Otel()))
//
//	// In an HTTP handler with active span
//	func handleRequest(w http.ResponseWriter, r *http.Request) {
//	    ctx := r.Context() // contains span from otel middleware
//	    logger.Info(ctx, "request received")
//	    // Log will include: {"trace_id": "abc123...", "span_id": "def456..."}
//	}
func WithTracer(t Tracer) Option {
	return func(config *Config) error {
		if t != nil {
			config.tracer = t
			// Detects tracer type for serialization
			switch t.(type) {
			case *otelTraceExtractor:
				config.TracerType = "otel"
			default:
				config.TracerType = "noop"
			}
		}
		return nil
	}
}
