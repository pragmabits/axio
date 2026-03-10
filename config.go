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
//	    Environment:    axio.Production,
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
	// DisableSample disables sampling of high-frequency logs.
	DisableSample bool `json:"disableSample" yaml:"disableSample" toml:"disableSample" mapstructure:"disableSample"`

	// Outputs defines the log output destinations.
	// If empty and no output is specified via Options, the default is:
	// - Development: Console with FormatText
	// - Others: Stdout with FormatJSON
	Outputs []OutputConfig `json:"outputs,omitempty" yaml:"outputs,omitempty" toml:"outputs,omitempty" mapstructure:"outputs,omitempty"`

	// AgentMode indicates whether the logger is optimized for external agents.
	// When true, forces output to stdout with JSON format.
	AgentMode bool `json:"agentMode" yaml:"agentMode" toml:"agentMode" mapstructure:"agentMode"`

	// PIIEnabled enables masking of sensitive personal data.
	PIIEnabled bool `json:"piiEnabled" yaml:"piiEnabled" toml:"piiEnabled" mapstructure:"piiEnabled"`
	// PIIPatterns defines which builtin PII patterns to detect (cpf, cnpj, email, etc).
	PIIPatterns []PIIPattern `json:"piiPatterns,omitempty" yaml:"piiPatterns,omitempty" toml:"piiPatterns,omitempty" mapstructure:"piiPatterns,omitempty"`
	// PIICustomPatterns allows defining additional PII patterns via regex.
	PIICustomPatterns []CustomPII `json:"piiCustomPatterns,omitempty" yaml:"piiCustomPatterns,omitempty" toml:"piiCustomPatterns,omitempty" mapstructure:"piiCustomPatterns,omitempty"`
	// PIIFields defines fields whose values should be redacted.
	PIIFields []string `json:"piiFields,omitempty" yaml:"piiFields,omitempty" toml:"piiFields,omitempty" mapstructure:"piiFields,omitempty"`

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
}

// DefaultConfig returns a configuration with sensible default values.
//
// Defaults applied:
//   - Environment: Development
//   - Level: LevelInfo
//   - CallerSkip: 0
//   - TracerType: "noop"
//   - PIIEnabled: false
//   - Audit.Enabled: false
//   - Metrics.Enabled: false
//   - AgentMode: false
//
// Example:
//
//	config := axio.DefaultConfig()
//	config.ServiceName = "my-service"
//	config.Environment = axio.Production
//	logger, err := axio.New(config)
func DefaultConfig() Config {
	return Config{
		Environment:   Development,
		Level:         LevelInfo,
		CallerSkip:    0,
		DisableSample: false,
		TracerType:    "noop",
		PIIEnabled:    false,
		AgentMode:     false,
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

// applyDefaults applies default values only to fields that are not set.
func applyDefaults(config *Config) {
	if config.Environment == "" {
		config.Environment = Development
	}

	if config.Level == "" {
		config.Level = LevelInfo
	}

	if config.TracerType == "" {
		config.TracerType = "noop"
	}

	if len(config.Outputs) == 0 {
		if config.Environment == Development {
			config.Outputs = []OutputConfig{
				{Type: OutputConsole, Format: FormatText},
			}
		} else {
			config.Outputs = []OutputConfig{
				{Type: OutputStdout, Format: FormatJSON},
			}
		}
	}

	if config.PIIEnabled && len(config.PIIPatterns) == 0 {
		config.PIIPatterns = []PIIPattern{PatternCPF, PatternCNPJ, PatternCreditCard}
	}

	if config.PIIEnabled && len(config.PIIFields) == 0 {
		config.PIIFields = DefaultSensitiveFields
	}

	if config.Metrics.MeterName == "" {
		config.Metrics.MeterName = "axio"
	}
	if config.Metrics.MeterVersion == "" {
		config.Metrics.MeterVersion = "1.0.0"
	}
}

// Validate checks whether the configuration is valid.
//
// Validations performed:
//   - Environment is valid (production, staging, development)
//   - Level is valid (debug, info, warn, error)
//   - OutputConfig: Type and Format are valid
//   - OutputConfig: Type=file requires non-empty Path
//   - AuditConfig: Enabled=true requires non-empty StorePath
//   - AgentMode: requires stdout+json outputs
//   - TracerType is "otel", "noop", or empty
//
// Example:
//
//	if err := config.Validate(); err != nil {
//	    return fmt.Errorf("invalid configuration: %w", err)
//	}
func (config *Config) Validate() error {
	if config.Environment != "" {
		if err := config.Environment.Validate(); err != nil {
			return err
		}
	}

	if config.Level != "" {
		if err := config.Level.Validate(); err != nil {
			return err
		}
	}

	for index, output := range config.Outputs {
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

	if config.AgentMode && len(config.Outputs) > 0 {
		for index, output := range config.Outputs {
			if output.Type != OutputStdout || output.Format != FormatJSON {
				return fmt.Errorf("%w: output[%d] must be stdout+json", ErrIncompatibleOutputs, index)
			}
		}
	}

	if config.Audit.Enabled && config.Audit.StorePath == "" {
		return ErrAuditWithoutPath
	}

	if config.TracerType != "" && config.TracerType != "otel" && config.TracerType != "noop" {
		return fmt.Errorf("%w: %s (expected 'otel' or 'noop')", ErrInvalidTracer, config.TracerType)
	}

	return nil
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
