package axio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"go.opentelemetry.io/otel/metric"
	"gopkg.in/yaml.v3"
)

// MetricsConfig configures metrics collection via OpenTelemetry.
//
// YAML example:
//
//	metrics:
//	  enabled: true
//	  meterName: axio
//	  meterVersion: 1.0.0
type MetricsConfig struct {
	// Enabled indicates whether metrics are enabled.
	Enabled bool `json:"enabled" yaml:"enabled" toml:"enabled" mapstructure:"enabled"`
	// MeterName is the OpenTelemetry meter name (default: "axio").
	MeterName string `json:"meterName" yaml:"meterName" toml:"meterName" mapstructure:"meterName"`
	// MeterVersion is the meter version (default: "1.0.0").
	MeterVersion string `json:"meterVersion" yaml:"meterVersion" toml:"meterVersion" mapstructure:"meterVersion"`
}

// Config contains the main configuration for the logger.
//
// Config can be loaded from YAML, JSON or TOML files using [LoadConfig]
// or [LoadConfigFrom], and can be customized programmatically using [Option].
//
// The precedence order is: Config (file) → Options → Defaults → Validate.
// Options always override values loaded from files.
//
// Basic example:
//
//	config := axio.Config{
//	    ServiceName:    "payments-api",
//	    ServiceVersion: "2.1.0",
//	    Environment:    axio.EnvironmentProduction,
//	    Level:          axio.LevelInfo,
//	}
//	logger, err := axio.New(config)
//
// YAML file example:
//
//	config, err := axio.LoadConfig("config.yaml")
//	if err != nil {
//	    return err
//	}
//	logger, err := axio.New(config, axio.WithTracer(axio.Otel()))
type Config struct {
	// ServiceName identifies the service in logs (e.g., "sales-api").
	ServiceName string `json:"serviceName" yaml:"serviceName" toml:"serviceName" mapstructure:"serviceName"`
	// ServiceVersion is the service version (e.g., "1.2.3").
	ServiceVersion string `json:"serviceVersion" yaml:"serviceVersion" toml:"serviceVersion" mapstructure:"serviceVersion"`
	// Environment defines the execution environment.
	Environment Environment `json:"environment" yaml:"environment" toml:"environment" mapstructure:"environment"`
	// InstanceID identifies the specific instance (e.g., pod ID, hostname).
	InstanceID string `json:"instanceId" yaml:"instanceId" toml:"instanceId" mapstructure:"instanceId"`
	// Level defines the minimum log level to be recorded.
	Level Level `json:"level" yaml:"level" toml:"level" mapstructure:"level"`
	// CallerSkip adjusts the caller depth for wrapper libraries.
	CallerSkip int `json:"callerSkip" yaml:"callerSkip" toml:"callerSkip" mapstructure:"callerSkip"`
	// OmitCaller writes lines without the caller, sparing the lookup of the
	// source location on every entry. [Entry.Caller] is then empty for hooks.
	OmitCaller bool `json:"omitCaller,omitempty" yaml:"omitCaller,omitempty" toml:"omitCaller,omitempty" mapstructure:"omitCaller,omitempty"`

	// Outputs defines the log output destinations.
	// If empty and no output is specified via Options, the default is:
	// - [EnvironmentDevelopment]: Console with FormatText
	// - Others: Stdout with FormatJSON
	Outputs []OutputConfig `json:"outputs,omitempty" yaml:"outputs,omitempty" toml:"outputs,omitempty" mapstructure:"outputs,omitempty"`

	// AgentMode indicates whether the logger is optimized for external agents.
	// When true, forces output to stdout with JSON format.
	AgentMode bool `json:"agentMode" yaml:"agentMode" toml:"agentMode" mapstructure:"agentMode"`

	// PIIDisabled turns off masking of sensitive personal data. Masking is on
	// whenever it is false, so a Config written by hand, one from
	// [DefaultConfig] and one loaded from a file without the key all mask.
	// See [WithPIIDisabled].
	PIIDisabled bool `json:"piiDisabled,omitempty" yaml:"piiDisabled,omitempty" toml:"piiDisabled,omitempty" mapstructure:"piiDisabled,omitempty"`
	// PIIPatterns defines which builtin PII patterns to detect (cpf, cnpj, email, etc).
	PIIPatterns []PIIPattern `json:"piiPatterns,omitempty" yaml:"piiPatterns,omitempty" toml:"piiPatterns,omitempty" mapstructure:"piiPatterns,omitempty"`
	// PIICustomPatterns allows defining additional PII patterns via regex.
	PIICustomPatterns []CustomPII `json:"piiCustomPatterns,omitempty" yaml:"piiCustomPatterns,omitempty" toml:"piiCustomPatterns,omitempty" mapstructure:"piiCustomPatterns,omitempty"`
	// PIIFields defines fields whose values should be redacted.
	PIIFields []string `json:"piiFields,omitempty" yaml:"piiFields,omitempty" toml:"piiFields,omitempty" mapstructure:"piiFields,omitempty"`
	// PIIMaxDepth caps how deep masking walks into a structured annotation
	// value; a container nested deeper is replaced by "[REDACTED]" whole. Zero
	// means [DefaultPIIMaxDepth]; a negative value is rejected. See
	// [PIIConfig.MaxDepth].
	PIIMaxDepth int `json:"piiMaxDepth,omitempty" yaml:"piiMaxDepth,omitempty" toml:"piiMaxDepth,omitempty" mapstructure:"piiMaxDepth,omitempty"`
	// PIIOmitErrorVerbose omits the verbose form of an error that formats
	// itself, instead of masking it. See [PIIConfig.OmitErrorVerbose].
	PIIOmitErrorVerbose bool `json:"piiOmitErrorVerbose,omitempty" yaml:"piiOmitErrorVerbose,omitempty" toml:"piiOmitErrorVerbose,omitempty" mapstructure:"piiOmitErrorVerbose,omitempty"`

	// Audit configures auditing with hash chain.
	Audit AuditConfig `json:"audit" yaml:"audit" toml:"audit" mapstructure:"audit"`

	// TracerType defines the trace extractor ("otel" or "noop").
	// When "otel", adds trace_id and span_id to logs automatically.
	// JSON tag keeps "tracer" for compatibility.
	TracerType string `json:"tracer" yaml:"tracer" toml:"tracer" mapstructure:"tracer"`

	// Metrics configures OpenTelemetry metrics collection.
	Metrics MetricsConfig `json:"metrics" yaml:"metrics" toml:"metrics" mapstructure:"metrics"`

	// Private fields for custom implementations (not serializable).
	// Set via Options like WithMetrics(), WithTracer(), WithHooks().
	metrics         Metrics
	metricsProvider metric.MeterProvider
	tracer          Tracer
	hooks           []Hook
	auditChain      *HashChain
	// resolvedOutputs holds outputs supplied directly via WithOutputs.
	// When non-empty, buildOutputs returns these and skips the OutputConfig
	// resolution path entirely (no file is opened twice).
	resolvedOutputs []Output
}

// DefaultConfig returns a configuration with sensible default values.
//
// Defaults applied:
//   - Environment: EnvironmentDevelopment
//   - Level: LevelInfo
//   - CallerSkip: 0
//   - TracerType: "noop"
//   - PIIDisabled: false, so PII masking is on
//   - Audit.Enabled: false
//   - Metrics.Enabled: false
//   - AgentMode: false
//
// Example:
//
//	config := axio.DefaultConfig()
//	config.ServiceName = "my-service"
//	config.Environment = axio.EnvironmentProduction
//	logger, err := axio.New(config)
func DefaultConfig() Config {
	return Config{
		Environment: EnvironmentDevelopment,
		Level:       LevelInfo,
		CallerSkip:  0,
		TracerType:  "noop",
		PIIDisabled: false,
		AgentMode:   false,
		Audit: AuditConfig{
			Enabled: false,
		},
		Metrics: MetricsConfig{
			Enabled:      false,
			MeterName:    "axio",
			MeterVersion: "1.0.0",
		},
	}
}

// LoadConfig loads configuration from a file.
//
// The format is detected automatically from the extension:
//   - .json: JSON
//   - .yaml or .yml: YAML
//   - .toml: TOML
//
// LoadConfig only parses the file. Full validation
// (including applying defaults) happens when [New] is called.
//
// Example:
//
//	config, err := axio.LoadConfig("/etc/axio/config.yaml")
//	if err != nil {
//	    return fmt.Errorf("failed to load configuration: %w", err)
//	}
//	logger, err := axio.New(config)
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrLoadConfig, err)
	}

	var format string
	switch {
	case strings.HasSuffix(path, ".json"):
		format = "json"
	case strings.HasSuffix(path, ".yaml"), strings.HasSuffix(path, ".yml"):
		format = "yaml"
	case strings.HasSuffix(path, ".toml"):
		format = "toml"
	default:
		return Config{}, fmt.Errorf("%w: %s (expected .json, .yaml, .yml or .toml)", ErrUnknownFormat, path)
	}

	return LoadConfigFrom(bytes.NewReader(data), format)
}

// MustLoadConfig is like [LoadConfig] but panics on error.
//
// Useful for initialization where failure must be fatal.
//
// Example:
//
//	config := axio.MustLoadConfig("/etc/axio/config.yaml")
//	logger, _ := axio.New(config)
func MustLoadConfig(path string) Config {
	config, err := LoadConfig(path)
	if err != nil {
		panic(err)
	}
	return config
}

// LoadConfigFrom loads configuration from an [io.Reader].
//
// Supported formats: "json", "yaml", "toml"
//
// LoadConfigFrom only parses the data. Full validation
// (including applying defaults) happens when [New] is called.
//
// Example:
//
//	config, err := axio.LoadConfigFrom(reader, "yaml")
//	if err != nil {
//	    return err
//	}
//	logger, err := axio.New(config)
func LoadConfigFrom(reader io.Reader, format string) (Config, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return Config{}, fmt.Errorf("%w: failed to read data: %w", ErrLoadConfig, err)
	}

	var config Config

	switch format {
	case "json":
		if err := json.Unmarshal(data, &config); err != nil {
			return Config{}, fmt.Errorf("%w: %w", ErrUnmarshalConfig, err)
		}
	case "yaml", "yml":
		if err := yaml.Unmarshal(data, &config); err != nil {
			return Config{}, fmt.Errorf("%w: %w", ErrUnmarshalConfig, err)
		}
	case "toml":
		if err := toml.Unmarshal(data, &config); err != nil {
			return Config{}, fmt.Errorf("%w: %w", ErrUnmarshalConfig, err)
		}
	default:
		return Config{}, fmt.Errorf("%w: %s (expected json, yaml or toml)", ErrUnknownFormat, format)
	}

	return config, nil
}

// Validate checks whether the configuration is valid.
//
// Validations performed:
//   - Environment is valid (production, staging, development)
//   - Level is valid (debug, info, warn, error)
//   - OutputConfig: Type and Format are valid
//   - OutputConfig: Type=file requires non-empty Path
//   - AuditConfig: Enabled=true requires non-empty StorePath
//   - PIIMaxDepth is not negative
//   - AgentMode: requires stdout+json outputs
//   - TracerType is "otel", "noop", or empty
//
// Example:
//
//	if err := config.Validate(); err != nil {
//	    return fmt.Errorf("invalid configuration: %w", err)
//	}
func (c *Config) Validate() error {
	if err := c.validateEnums(); err != nil {
		return err
	}

	if err := c.validateOutputs(); err != nil {
		return err
	}

	if err := c.validateAgentMode(); err != nil {
		return err
	}

	if err := c.validateAudit(); err != nil {
		return err
	}

	if c.PIIMaxDepth < 0 {
		return fmt.Errorf("%w: %d", ErrInvalidPIIMaxDepth, c.PIIMaxDepth)
	}

	if c.TracerType != "" && c.TracerType != "otel" && c.TracerType != "noop" {
		return fmt.Errorf("%w: %s (expected 'otel' or 'noop')", ErrInvalidTracer, c.TracerType)
	}

	return nil
}

// validateEnums checks Environment and Level when they are set.
func (c *Config) validateEnums() error {
	if c.Environment != "" {
		if err := c.Environment.Validate(); err != nil {
			return err
		}
	}

	if c.Level != "" {
		if err := c.Level.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// validateOutputs checks the Type and Format of every output and that file
// outputs carry a Path.
func (c *Config) validateOutputs() error {
	for index, output := range c.Outputs {
		if err := output.Type.Validate(); err != nil {
			return fmt.Errorf("output[%d]: %w", index, err)
		}
		if err := output.Format.Validate(); err != nil {
			return fmt.Errorf("output[%d]: %w", index, err)
		}
		if output.Type == OutputFile && output.Path == "" {
			return fmt.Errorf("%w: output[%d]", ErrFileOutputNoPath, index)
		}
	}
	return nil
}

// validateAgentMode checks that, in agent mode, every output is stdout+json.
func (c *Config) validateAgentMode() error {
	if !c.AgentMode {
		return nil
	}

	for index, output := range c.Outputs {
		if output.Type != OutputStdout || output.Format != FormatJSON {
			return fmt.Errorf("%w: output[%d] must be stdout+json", ErrIncompatibleOutputs, index)
		}
	}
	return nil
}

// validateAudit checks that auditing has somewhere to keep its chain: a
// StorePath, or a chain of its own from [WithAuditChain].
func (c *Config) validateAudit() error {
	if c.Audit.Enabled && c.Audit.StorePath == "" && c.auditChain == nil {
		return ErrAuditWithoutPath
	}
	return nil
}

// validateAuditOutputs checks that an audited Logger has a JSON output. It is
// not part of [Config.Validate]: an Event writes JSON to every output, so the
// same Config is valid for an audited Event.
func (c *Config) validateAuditOutputs() error {
	if !c.Audit.Enabled {
		return nil
	}
	for _, output := range c.Outputs {
		if output.Format == FormatJSON {
			return nil
		}
	}
	return ErrAuditWithoutJSON
}

// applyDefaults applies default values only to fields that are not set.
func applyDefaults(config *Config) {
	if config.Environment == "" {
		config.Environment = EnvironmentDevelopment
	}

	if config.Level == "" {
		config.Level = LevelInfo
	}

	if config.TracerType == "" {
		config.TracerType = "noop"
	}

	applyOutputDefaults(config)
	applyPIIDefaults(config)

	if config.Metrics.MeterName == "" {
		config.Metrics.MeterName = "axio"
	}
	if config.Metrics.MeterVersion == "" {
		config.Metrics.MeterVersion = "1.0.0"
	}
}

// applyOutputDefaults picks the default output for the environment when none
// was configured: colored console text in development, stdout JSON elsewhere.
func applyOutputDefaults(config *Config) {
	if len(config.Outputs) > 0 {
		return
	}

	if config.Environment == EnvironmentDevelopment {
		config.Outputs = []OutputConfig{
			{Type: OutputConsole, Format: FormatText},
		}
		return
	}

	config.Outputs = []OutputConfig{
		{Type: OutputStdout, Format: FormatJSON},
	}
}

// applyPIIDefaults fills the builtin patterns and sensitive fields PII masking
// uses when none were given.
func applyPIIDefaults(config *Config) {
	if len(config.PIIPatterns) == 0 {
		config.PIIPatterns = []PIIPattern{PatternCPF, PatternCNPJ, PatternCreditCard}
	}

	if len(config.PIIFields) == 0 {
		config.PIIFields = DefaultSensitiveFields()
	}
}
