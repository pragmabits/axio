---
name: outputs
description: "This skill should be used when the user asks about axio output destinations — Console, Stdout, File, RotatingFile, MustFile, MustRotatingFile, OutputConfig, OutputType, RotationConfig, WithOutputs, WithAgentMode, file rotation, log rotation, multiple outputs, MaxSize, MaxAge, MaxBackups, Compress, Interval, or Duration type. Trigger phrases include \"Console\", \"Stdout\", \"File output\", \"RotatingFile\", \"MustFile\", \"WithOutputs\", \"WithAgentMode\", \"rotation\", \"MaxSize\", \"MaxAge\", \"MaxBackups\", \"Compress\", \"log rotation\", \"file rotation\", \"Interval\", \"multiple outputs\", \"agent mode\", \"Promtail\", \"Fluent Bit\", \"Filebeat\"."
---

# Outputs

Axio supports multiple simultaneous output destinations with independent formats.

## Output Types
- `Console(format)` — stderr (development)
- `Stdout(format)` — stdout (containers/agents)
- `File(path, format)` — plain file (returns Output, error)
- `RotatingFile(path, format, rotation)` — file with rotation

## Rotation
RotationConfig: MaxSize (MB), MaxAge (days), MaxBackups, Compress, LocalTime, Interval (Duration)
- Size-based, time-based, or both
- Uses lumberjack internally

## Agent Mode
`WithAgentMode()` forces stdout+JSON for log collection agents.

## Usage
Use `/axio` command for detailed output configuration guidance.
