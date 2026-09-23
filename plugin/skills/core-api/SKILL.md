---
name: core-api
description: "This skill should be used when the user asks about the axio Logger interface, creating loggers with axio.New, DefaultConfig, log levels (Debug, Info, Warn, Error), environments (EnvironmentDevelopment, EnvironmentStaging, EnvironmentProduction), formats (FormatJSON, FormatText), Named sub-loggers, or the basic logging API. Trigger phrases include \"axio.New\", \"Logger interface\", \"axio.Config\", \"DefaultConfig\", \"LevelDebug\", \"LevelInfo\", \"LevelWarn\", \"LevelError\", \"EnvironmentDevelopment\", \"EnvironmentStaging\", \"EnvironmentProduction\", \"Development\", \"Staging\", \"Production\", \"FormatJSON\", \"FormatText\", \"Named\", \"logger.Close\", \"basic axio setup\", \"create logger\"."
---

# Core API

Axio's core API for creating and using structured loggers.

## Key Types
- `Logger` interface: Named, Debug, Info, Warn, Error, With, Close
- `Config` struct: ServiceName, ServiceVersion, Environment, Level, CallerSkip, etc.
- `Environment`: EnvironmentDevelopment, EnvironmentStaging, EnvironmentProduction
- `Level`: LevelDebug, LevelInfo, LevelWarn, LevelError
- `Format`: FormatJSON, FormatText

## Important
- Debug and Info: `(ctx, message, ...args)` — NO error parameter
- Warn and Error: `(ctx, error, message, ...args)` — error is second parameter
- Always pass `context.Context` as first parameter
- Always `defer logger.Close()` after creation
- `DefaultConfig()` returns sensible defaults (EnvironmentDevelopment, LevelInfo)

## Usage
Use `/axio` command for detailed guidance on any core API topic.
