---
name: configuration
description: "This skill should be used when the user asks about axio configuration — loading config from files, YAML/JSON/TOML format, LoadConfig, LoadConfigFrom, MustLoadConfig, Config struct fields, functional options (Option type), config validation, config precedence (Config → Options → Defaults → Validate), or programmatic configuration. Trigger phrases include \"LoadConfig\", \"LoadConfigFrom\", \"MustLoadConfig\", \"config.yaml\", \"axio.yaml\", \"YAML config\", \"JSON config\", \"TOML config\", \"Config struct\", \"Option\", \"functional options\", \"config precedence\", \"config validation\", \"Validate\"."
---

# Configuration

Axio supports configuration via files (YAML, JSON, TOML) and programmatic functional options.

## Config Loading
- `LoadConfig(path)` — auto-detects format from extension
- `LoadConfigFrom(reader, format)` — reads from any io.Reader
- `MustLoadConfig(path)` — panics on error

## Precedence
Config (file) → Options (override) → Defaults (fill empty) → Validate

## Key Config Fields
ServiceName, ServiceVersion, Environment, InstanceID, Level, CallerSkip, DisableSample, Outputs, AgentMode, PIIEnabled, PIIPatterns, PIICustomPatterns, PIIFields, Audit, TracerType, Metrics

## Usage
Use `/axio` command for detailed configuration guidance.
