package axio

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pragmabits/axio/internal/logline"
)

func TestLoadConfig(t *testing.T) {
	t.Run("yaml_file", func(t *testing.T) {
		path := tempFile(t, "config.yaml")
		writeFile(t, path, `
serviceName: my-service
serviceVersion: "1.0.0"
environment: production
level: info
`)

		config, err := LoadConfig(path)
		assertNoError(t, err)

		assertEqual(t, config.ServiceName, "my-service")
		assertEqual(t, config.ServiceVersion, "1.0.0")
		assertEqual(t, config.Environment, EnvironmentProduction)
		assertEqual(t, config.Level, LevelInfo)
	})

	t.Run("json_file", func(t *testing.T) {
		path := tempFile(t, "config.json")
		writeFile(t, path, `{
  "serviceName": "json-service",
  "serviceVersion": "2.0.0",
  "environment": "staging",
  "level": "debug"
}`)

		config, err := LoadConfig(path)
		assertNoError(t, err)

		assertEqual(t, config.ServiceName, "json-service")
		assertEqual(t, config.Environment, EnvironmentStaging)
		assertEqual(t, config.Level, LevelDebug)
	})

	t.Run("toml_file", func(t *testing.T) {
		path := tempFile(t, "config.toml")
		writeFile(t, path, `
serviceName = "toml-service"
serviceVersion = "3.0.0"
environment = "development"
level = "warn"
`)

		config, err := LoadConfig(path)
		assertNoError(t, err)

		assertEqual(t, config.ServiceName, "toml-service")
		assertEqual(t, config.Environment, EnvironmentDevelopment)
		assertEqual(t, config.Level, LevelWarn)
	})

	t.Run("file_not_found", func(t *testing.T) {
		_, err := LoadConfig("/nonexistent/path/config.yaml")
		assertError(t, err)
		if !errors.Is(err, ErrLoadConfig) {
			t.Errorf("expected ErrLoadConfig, got %v", err)
		}
	})

	t.Run("unknown_extension", func(t *testing.T) {
		path := tempFile(t, "config.txt")
		writeFile(t, path, "content")

		_, err := LoadConfig(path)
		assertError(t, err)
		if !errors.Is(err, ErrUnknownFormat) {
			t.Errorf("expected ErrUnknownFormat, got %v", err)
		}
	})

	t.Run("invalid_yaml", func(t *testing.T) {
		path := tempFile(t, "invalid.yaml")
		writeFile(t, path, `
serviceName: [invalid yaml
`)

		_, err := LoadConfig(path)
		assertError(t, err)
		if !errors.Is(err, ErrUnmarshalConfig) {
			t.Errorf("expected ErrUnmarshalConfig, got %v", err)
		}
	})

	t.Run("with_outputs", func(t *testing.T) {
		path := tempFile(t, "outputs.yaml")
		writeFile(t, path, `
serviceName: with-outputs
environment: production
level: info
outputs:
  - type: stdout
    format: json
  - type: file
    format: json
    path: /var/log/app.log
`)

		config, err := LoadConfig(path)
		assertNoError(t, err)

		if len(config.Outputs) != 2 {
			t.Errorf("expected 2 outputs, got %d", len(config.Outputs))
		}
		assertEqual(t, config.Outputs[0].Type, OutputStdout)
		assertEqual(t, config.Outputs[1].Type, OutputFile)
		assertEqual(t, config.Outputs[1].Path, "/var/log/app.log")
	})

	t.Run("with_pii_config", func(t *testing.T) {
		path := tempFile(t, "pii.yaml")
		writeFile(t, path, `
serviceName: pii-service
environment: production
level: info
piiEnabled: true
piiPatterns:
  - cpf
  - email
`)

		config, err := LoadConfig(path)
		assertNoError(t, err)

		if !config.PIIEnabled {
			t.Error("PIIEnabled should be true")
		}
		if len(config.PIIPatterns) != 2 {
			t.Errorf("expected 2 patterns, got %d", len(config.PIIPatterns))
		}
	})

	t.Run("with_audit_config", func(t *testing.T) {
		path := tempFile(t, "audit.yaml")
		writeFile(t, path, `
serviceName: audit-service
environment: production
level: info
audit:
  enabled: true
  storePath: /var/lib/axio/chain.json
`)

		config, err := LoadConfig(path)
		assertNoError(t, err)

		if !config.Audit.Enabled {
			t.Error("Audit.Enabled should be true")
		}
		assertEqual(t, config.Audit.StorePath, "/var/lib/axio/chain.json")
	})
}

func TestLoadConfigFrom(t *testing.T) {
	t.Run("yaml_format", func(t *testing.T) {
		reader := strings.NewReader(`
serviceName: reader-service
environment: development
level: info
`)

		config, err := LoadConfigFrom(reader, "yaml")
		assertNoError(t, err)
		assertEqual(t, config.ServiceName, "reader-service")
	})

	t.Run("json_format", func(t *testing.T) {
		reader := strings.NewReader(`{"serviceName": "json-reader"}`)

		config, err := LoadConfigFrom(reader, "json")
		assertNoError(t, err)
		assertEqual(t, config.ServiceName, "json-reader")
	})

	t.Run("removed_disable_sample_key_is_ignored", func(t *testing.T) {
		for format, content := range map[string]string{
			"yaml": "serviceName: legacy\ndisableSample: true\n",
			"json": `{"serviceName": "legacy", "disableSample": true}`,
			"toml": "serviceName = \"legacy\"\ndisableSample = true\n",
		} {
			config, err := LoadConfigFrom(strings.NewReader(content), format)
			assertNoError(t, err)
			assertEqual(t, config.ServiceName, "legacy")
		}
	})

	t.Run("pii_max_depth", func(t *testing.T) {
		reader := strings.NewReader("piiEnabled: true\npiiMaxDepth: 4\n")

		config, err := LoadConfigFrom(reader, "yaml")
		assertNoError(t, err)
		assertEqual(t, config.PIIMaxDepth, 4)
	})

	t.Run("omit_caller", func(t *testing.T) {
		config, err := LoadConfigFrom(strings.NewReader("omitCaller: true\n"), "yaml")
		assertNoError(t, err)
		assertEqual(t, config.OmitCaller, true)
	})

	t.Run("pii_omit_error_verbose", func(t *testing.T) {
		reader := strings.NewReader("piiEnabled: true\npiiOmitErrorVerbose: true\n")

		config, err := LoadConfigFrom(reader, "yaml")
		assertNoError(t, err)
		assertEqual(t, config.PIIOmitErrorVerbose, true)
	})

	t.Run("unknown_format", func(t *testing.T) {
		reader := strings.NewReader("content")

		_, err := LoadConfigFrom(reader, "xml")
		assertError(t, err)
		if !errors.Is(err, ErrUnknownFormat) {
			t.Errorf("expected ErrUnknownFormat, got %v", err)
		}
	})
}

func TestMustLoadConfig(t *testing.T) {
	t.Run("valid_file", func(t *testing.T) {
		path := tempFile(t, "valid.yaml")
		writeFile(t, path, `serviceName: must-service`)

		config := MustLoadConfig(path)
		assertEqual(t, config.ServiceName, "must-service")
	})

	t.Run("invalid_file_panics", func(t *testing.T) {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Error("expected panic for invalid file")
			}
		}()

		MustLoadConfig("/nonexistent/file.yaml")
	})
}

func TestConfig_Validate(t *testing.T) {
	t.Run("valid_config", func(t *testing.T) {
		config := minimalConfig()
		err := config.Validate()
		assertNoError(t, err)
	})

	t.Run("invalid_environment", func(t *testing.T) {
		config := minimalConfig()
		config.Environment = "invalid"

		err := config.Validate()
		assertError(t, err)
		if !errors.Is(err, ErrInvalidEnvironment) {
			t.Errorf("expected ErrInvalidEnvironment, got %v", err)
		}
	})

	t.Run("invalid_level", func(t *testing.T) {
		config := minimalConfig()
		config.Level = "invalid"

		err := config.Validate()
		assertError(t, err)
		if !errors.Is(err, ErrInvalidLevel) {
			t.Errorf("expected ErrInvalidLevel, got %v", err)
		}
	})

	t.Run("file_output_without_path", func(t *testing.T) {
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputFile, Format: FormatJSON}, // without path
		}

		err := config.Validate()
		assertError(t, err)
		if !errors.Is(err, ErrFileOutputNoPath) {
			t.Errorf("expected ErrFileOutputNoPath, got %v", err)
		}
	})

	t.Run("negative_pii_max_depth", func(t *testing.T) {
		config := minimalConfig()
		config.PIIMaxDepth = -1

		if err := config.Validate(); !errors.Is(err, ErrInvalidPIIMaxDepth) {
			t.Errorf("expected ErrInvalidPIIMaxDepth, got %v", err)
		}
	})

	t.Run("audit_enabled_without_path", func(t *testing.T) {
		config := minimalConfig()
		config.Audit.Enabled = true
		// without StorePath

		err := config.Validate()
		assertError(t, err)
		if !errors.Is(err, ErrAuditWithoutPath) {
			t.Errorf("expected ErrAuditWithoutPath, got %v", err)
		}
	})

	t.Run("invalid_tracer_type", func(t *testing.T) {
		config := minimalConfig()
		config.TracerType = "invalid"

		err := config.Validate()
		assertError(t, err)
		if !errors.Is(err, ErrInvalidTracer) {
			t.Errorf("expected ErrInvalidTracer, got %v", err)
		}
	})

	t.Run("agent_mode_requires_stdout_json", func(t *testing.T) {
		config := minimalConfig()
		config.AgentMode = true
		config.Outputs = []OutputConfig{
			{Type: OutputConsole, Format: FormatText}, // incompatible
		}

		err := config.Validate()
		assertError(t, err)
		if !errors.Is(err, ErrIncompatibleOutputs) {
			t.Errorf("expected ErrIncompatibleOutputs, got %v", err)
		}
	})

	t.Run("agent_mode_with_stdout_json_passes", func(t *testing.T) {
		config := minimalConfig()
		config.AgentMode = true
		config.Outputs = []OutputConfig{
			{Type: OutputStdout, Format: FormatJSON},
		}

		err := config.Validate()
		assertNoError(t, err)
	})
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assertEqual(t, config.Environment, EnvironmentDevelopment)
	assertEqual(t, config.Level, LevelInfo)
	assertEqual(t, config.TracerType, "noop")
	if config.PIIEnabled {
		t.Error("PIIEnabled should be false by default")
	}
	if config.Audit.Enabled {
		t.Error("Audit.Enabled should be false by default")
	}
	if config.AgentMode {
		t.Error("AgentMode should be false by default")
	}
}

func TestApplyDefaults(t *testing.T) {
	t.Run("sets_missing_environment", func(t *testing.T) {
		config := Config{}
		applyDefaults(&config)
		assertEqual(t, config.Environment, EnvironmentDevelopment)
	})

	t.Run("sets_missing_level", func(t *testing.T) {
		config := Config{}
		applyDefaults(&config)
		assertEqual(t, config.Level, LevelInfo)
	})

	t.Run("sets_missing_tracer_type", func(t *testing.T) {
		config := Config{}
		applyDefaults(&config)
		assertEqual(t, config.TracerType, "noop")
	})

	t.Run("development_gets_console_output", func(t *testing.T) {
		config := Config{Environment: EnvironmentDevelopment}
		applyDefaults(&config)

		if len(config.Outputs) != 1 {
			t.Fatalf("expected 1 output, got %d", len(config.Outputs))
		}
		assertEqual(t, config.Outputs[0].Type, OutputConsole)
		assertEqual(t, config.Outputs[0].Format, FormatText)
	})

	t.Run("production_gets_stdout_json", func(t *testing.T) {
		config := Config{Environment: EnvironmentProduction}
		applyDefaults(&config)

		if len(config.Outputs) != 1 {
			t.Fatalf("expected 1 output, got %d", len(config.Outputs))
		}
		assertEqual(t, config.Outputs[0].Type, OutputStdout)
		assertEqual(t, config.Outputs[0].Format, FormatJSON)
	})

	t.Run("pii_enabled_sets_default_patterns", func(t *testing.T) {
		config := Config{PIIEnabled: true}
		applyDefaults(&config)

		if len(config.PIIPatterns) == 0 {
			t.Error("PIIPatterns should have default patterns")
		}
		if len(config.PIIFields) == 0 {
			t.Error("PIIFields should have default fields")
		}
	})

	t.Run("does_not_override_existing_values", func(t *testing.T) {
		config := Config{
			Environment: EnvironmentStaging,
			Level:       LevelError,
			TracerType:  "otel",
		}
		applyDefaults(&config)

		assertEqual(t, config.Environment, EnvironmentStaging)
		assertEqual(t, config.Level, LevelError)
		assertEqual(t, config.TracerType, "otel")
	})
}

func TestConfig_ValidateAuditOutputs(t *testing.T) {
	config := Config{ServiceName: "checkout", Environment: EnvironmentProduction, Level: LevelInfo}

	t.Run("logger_with_text_outputs_only_fails", func(t *testing.T) {
		_, err := New(config, WithOutputs(newBufferOutput(FormatText)), WithAudit(tempFile(t, "chain.json")))
		if !errors.Is(err, ErrAuditWithoutJSON) || !errors.Is(err, ErrValidateConfig) {
			t.Errorf("expected ErrValidateConfig wrapping ErrAuditWithoutJSON, got %v", err)
		}
	})

	t.Run("logger_with_audit_chain_and_text_outputs_only_fails", func(t *testing.T) {
		chain, err := NewHashChain(nil)
		assertNoError(t, err)
		_, err = New(config, WithOutputs(newBufferOutput(FormatText)), WithAuditChain(chain))
		if !errors.Is(err, ErrAuditWithoutJSON) {
			t.Errorf("expected ErrAuditWithoutJSON, got %v", err)
		}
	})

	t.Run("event_with_text_outputs_only_is_accepted", func(t *testing.T) {
		output := newBufferOutput(FormatText)
		event, err := NewEvent("checkout", config, WithOutputs(output), WithAudit(tempFile(t, "chain.json")))
		assertNoError(t, err)
		if err != nil {
			return
		}
		event.Emit(context.Background())
		assertNoError(t, event.Close())

		if _, _, _, ok := logline.SplitTrailer([]byte(output.String())); !ok {
			t.Errorf("an event writes the audited JSON line to every output, got %q", output.String())
		}
	})
}
