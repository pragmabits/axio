package axio

import (
	"errors"
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
//   - [WithOmitCaller]: writes lines without the caller
//   - [WithHooks]: configures custom processing hooks
//   - [WithPII]: configures PII masking
//   - [WithPIIMaxDepth]: sets how deep PII masking walks into structured values
//   - [WithPIIOmitErrorVerbose]: omits the verbose form of errors instead of masking it
//   - [WithAudit]: configures auditing with a hash chain stored in a file
//   - [WithAuditChain]: configures auditing with a hash chain of your own
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
// Options override the Config: the first WithOutputs replaces the outputs in
// [Config.Outputs], which are then neither opened nor validated, and later
// calls add to it.
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
		if len(outputs) > 0 && len(config.resolvedOutputs) == 0 {
			config.Outputs = nil
		}
		for _, output := range outputs {
			config.resolvedOutputs = append(config.resolvedOutputs, output)
			// Mirror metadata so Validate() and AgentMode rules — which
			// iterate config.Outputs — still apply. The file handle stays
			// owned by the resolved output; no Close/Reopen dance.
			outputConfig := OutputConfig{
				Type:   output.Type(),
				Format: output.Format(),
			}
			if fileOut, ok := output.(*fileOutput); ok {
				outputConfig.Path = fileOut.path
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
//   - Overwrites any previous output, closing the ones passed to an earlier
//     [WithOutputs]; an error closing them is returned
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

		// WithOutputs handed these to the logger, and the logger will no longer
		// hold them, so nothing else is left to close them.
		discarded := config.resolvedOutputs
		config.resolvedOutputs = nil
		var errs []error
		for _, output := range discarded {
			if err := output.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
}

// WithHooks configures the logger to process log entries through the specified hooks.
//
// Hooks run in the order they were registered, after PII masking and before
// the entry is written; with auditing on, the hash covers what they changed.
// If a hook returns an error, processing stops and the entry is not written.
//
// For PII masking, prefer [WithPII], which runs before every custom hook.
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
// Masking covers the message, the error and every annotation, as
// [PIIMasker.MaskFields] describes, before any custom hook runs.
//
// Example:
//
//	logger, _ := axio.New(config,
//	    axio.WithPII(
//	        []axio.PIIPattern{axio.PatternCPF, axio.PatternEmail},
//	        axio.DefaultSensitiveFields(),
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

// WithPIIMaxDepth sets how deep PII masking walks into a structured
// annotation value, as [Config.PIIMaxDepth] does from a file. A container
// nested deeper is replaced by "[REDACTED]" whole; zero means
// [DefaultPIIMaxDepth]. It takes effect with masking on, through [WithPII] or
// [Config.PIIEnabled].
//
// Returns [ErrInvalidPIIMaxDepth] for a negative depth.
//
// Example:
//
//	logger, err := axio.New(config,
//	    axio.WithPII(nil, nil),
//	    axio.WithPIIMaxDepth(8),
//	)
func WithPIIMaxDepth(depth int) Option {
	return func(config *Config) error {
		if depth < 0 {
			return fmt.Errorf("%w: %d", ErrInvalidPIIMaxDepth, depth)
		}
		config.PIIMaxDepth = depth
		return nil
	}
}

// WithPIIOmitErrorVerbose makes PII masking omit the verbose form of an error
// that formats itself — the %+v, a stack trace for many errors — instead of
// masking it, as [Config.PIIOmitErrorVerbose] does from a file. The error is
// then written by its masked message only, and its verbose form is never read,
// which spares scanning it: a stack of a couple of kilobytes costs about 250 µs
// to mask. It takes effect with masking on, through [WithPII] or
// [Config.PIIEnabled].
//
// Example:
//
//	logger, err := axio.New(config,
//	    axio.WithPII(nil, nil),
//	    axio.WithPIIOmitErrorVerbose(),
//	)
func WithPIIOmitErrorVerbose() Option {
	return func(config *Config) error {
		config.PIIOmitErrorVerbose = true
		return nil
	}
}

// WithOmitCaller writes lines without the caller, as [Config.OmitCaller] does
// from a file. Finding the caller walks the stack on every entry, and it can
// cost about half of writing a line; without it, [Entry.Caller] is empty for
// hooks and [Config.CallerSkip] has no effect.
//
// Example:
//
//	logger, err := axio.New(config, axio.WithOmitCaller())
func WithOmitCaller() Option {
	return func(config *Config) error {
		config.OmitCaller = true
		return nil
	}
}

// WithAudit enables auditing with a hash chain whose state is persisted at
// storePath.
//
// Every JSON line then ends with previous_hash and hash: the SHA-256 of the
// previous line's hash followed by the line's own bytes, so the hash covers
// exactly what was written. Verify the log with [HashChain.Verify]. Text
// outputs show a shortened hash for reference only.
//
// Every Logger and Event audited with the same storePath in this process
// extends one chain, whichever was created first. Another process holding the
// same store makes [New] fail with [ErrBuildAudit] wrapping
// [ErrChainStoreLocked]; see [FileStore]. An audited Logger needs a JSON
// output, or [New] returns [ErrAuditWithoutJSON].
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

// WithAuditChain enables auditing through chain, for chain state kept in a
// [ChainStore] other than a local file. Loggers and Events given the same
// chain extend one chain.
//
// A chain over a [FileStore] is locked the way [WithAudit] locks its store:
// [New] or [NewEvent] takes the store's lock and loads the chain from it again,
// so a store another process holds fails there with [ErrChainStoreLocked], and
// state another process saved after [NewHashChain] loaded it is continued.
//
// Returns [ErrNilAuditChain] if chain is nil.
//
// Example:
//
//	chain, err := axio.NewHashChain(redisStore)
//	if err != nil {
//	    return err
//	}
//	logger, err := axio.New(config, axio.WithAuditChain(chain))
func WithAuditChain(chain *HashChain) Option {
	return func(config *Config) error {
		if chain == nil {
			return ErrNilAuditChain
		}
		config.Audit.Enabled = true
		config.auditChain = chain
		return nil
	}
}

// WithMetrics configures the logger to emit metrics using the specified
// OpenTelemetry MeterProvider.
//
// If provider is nil, returns [ErrNilMetricsProvider].
//
// Emitted metrics include:
//   - logs.total: Log counter by level
//   - pii.masked: Masked PII counter by pattern and origin
//   - pii.redacted: Counter of values redacted whole, by reason and origin
//   - audit.records: Audit records counter
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
func WithTracer(tracer Tracer) Option {
	return func(config *Config) error {
		if tracer == nil {
			return ErrNilTracer
		}
		config.tracer = tracer
		// Detects tracer type for serialization
		switch tracer.(type) {
		case *otelTraceExtractor:
			config.TracerType = "otel"
		default:
			config.TracerType = "noop"
		}
		return nil
	}
}
