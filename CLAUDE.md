# Axio

Structured logging library for Go. Zap is the internal engine — never exposed in the public API.

Project rules live under `.claude/rules/` and are imported below. Read them
before writing or reviewing code:

- [`.claude/rules/constraints.md`](.claude/rules/constraints.md) — Claude behavior constraints (don't edit unless asked, etc.)
- [`.claude/rules/rules.md`](.claude/rules/rules.md) — Code rules (no zap in public API, naming, testing)
- [`.claude/rules/patterns.md`](.claude/rules/patterns.md) — Invariants and recurring code patterns

@.claude/rules/constraints.md
@.claude/rules/rules.md
@.claude/rules/patterns.md

## Commands

```bash
go build ./...                        # Build
go vet ./...                          # Static analysis
go test ./... -count=1                # Run tests (no cache)
go test -race ./...                   # Race detector
go test -bench=. -benchmem -run='^$'  # Benchmarks
go run ./examples/basic/              # Logger, levels, named, DefaultConfig
go run ./examples/config/             # LoadConfig, LoadConfigFrom
go run ./examples/outputs/            # Console+File, agent mode, production
go run ./examples/annotations/        # Annotate, HTTP metadata
go run ./examples/pii/                # MaskString API, PIIHook
go run ./examples/audit/              # Hash chain
go run ./examples/tracing/            # OpenTelemetry
go run ./examples/rotation/           # Size + time rotation
go run ./examples/events/             # Wide Events (Emit, error attachment)
go run ./examples/combined/           # Multiple options together
```

## Architecture

- `axio.go` — Logger interface, core types (Environment, Level, Format)
- `logger.go` — Logger implementation (wraps zap internally)
- `annotation.go` — `Annotation`, `Annotations`, and `HTTP` metadata type
- `event.go` — Wide Event type (`Event`, `Emit`, error attachment)
- `config.go` — Config loading (YAML, JSON, TOML)
- `output.go` — Output interface + implementations (Console, Stdout, File, RotatingFile) + RotationConfig
- `options.go` — Functional options (WithOutputs, WithPII, WithAudit, etc.)
- `hook.go` — Hook chain processing
- `pii.go` — PII masking (CPF, CNPJ, credit card, email, phone)
- `audit.go` — Hash chain for tamper-proof audit logs
- `tracing.go` — OpenTelemetry trace extraction
- `metrics.go` — OTel metrics
- `encoder.go` — internal slice/key encoders (zap adapters)
- `duration.go` — duration parsing for config
- `errors.go` — Sentinel errors
