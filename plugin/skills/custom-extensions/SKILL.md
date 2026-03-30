---
name: custom-extensions
description: "This skill should be used when the user asks about extending axio — writing custom Output implementations, creating custom Hook implementations, extending PII masking with custom patterns, implementing the ChainStore interface, implementing the Tracer interface, implementing the Metrics interface, implementing the Annotable interface, or any axio extension task. Trigger phrases include \"custom Output\", \"custom Hook\", \"implement Output\", \"implement Hook\", \"implement ChainStore\", \"implement Tracer\", \"implement Metrics\", \"implement Annotable\", \"extend axio\", \"custom PII pattern\", \"custom extension\", \"Hook interface\", \"Output interface\", \"Annotable interface\"."
---

# Custom Extensions

Axio provides interfaces for extending every major subsystem.

## Extensible Interfaces
- `Output` — custom log destinations (Format, Type, Write, Sync, Close)
- `Hook` — custom entry processing (Name, Process)
- `ChainStore` — custom audit persistence (Save, Load)
- `Tracer` — custom trace extraction (Extract)
- `Metrics` — custom metrics backends (LogsTotal, PIIMasked, AuditRecords, HookDuration, HookDurationWithError)
- `Annotable` — complex types that produce multiple annotations (Append)

## Important Notes
- New Output implementations must be handled in WithOutputs (type assertions in options.go)
- Hook execution order is fixed: PIIHook → AuditHook → Custom hooks
- Hooks implementing MetricsAware receive the Metrics object automatically
- Annotable types are expanded during field serialization via expandAnnotable

## Usage
Use `/axio` command for detailed extension implementation guidance.
