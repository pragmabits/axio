# Axio

Structured logging library for Go, and the `axio` command that reads its logs
(`axio render`, `axio verify`). Zap is the internal engine — never exposed in
the public API.

Project rules live under `.claude/rules/` and are imported below. Read them
before writing or reviewing code:

- [`.claude/rules/constraints.md`](.claude/rules/constraints.md) — Claude behavior constraints (don't edit unless asked, etc.)
- [`.claude/rules/rules.md`](.claude/rules/rules.md) — Code rules (no zap in public API, naming, testing)
- [`.claude/rules/patterns.md`](.claude/rules/patterns.md) — Invariants and recurring code patterns
- [`.claude/rules/code-shape.md`](.claude/rules/code-shape.md) — Function size, test-first, error handling, type escapes
- [`.claude/rules/file-layout.md`](.claude/rules/file-layout.md) — Order of declarations in a file, godoc
- [`.claude/rules/naming.md`](.claude/rules/naming.md) — Names are words, the short names that stay, subtests
- [`.claude/rules/security-default.md`](.claude/rules/security-default.md) — The secure option is the default

@.claude/rules/constraints.md
@.claude/rules/rules.md
@.claude/rules/patterns.md
@.claude/rules/code-shape.md
@.claude/rules/file-layout.md
@.claude/rules/naming.md
@.claude/rules/security-default.md

## Commands

```bash
go build ./...                        # Build
go vet ./...                          # Static analysis
golangci-lint run ./...               # Lint (config in .golangci.yml)
go test ./... -count=1                # Run tests (no cache)
go test -race ./...                   # Race detector (needs cgo and a C compiler)
go test -bench=. -benchmem -run='^$'  # Benchmarks
go run ./cmd/axio render app.log      # JSON log → the Console's text
go run ./cmd/axio verify --store chain.json app.log  # Check an audited log
go install ./cmd/axio                 # Install the axio command
go run ./examples/basic/              # Logger, levels, named, DefaultConfig
go run ./examples/config/             # LoadConfig, LoadConfigFrom
go run ./examples/outputs/            # Console+File, agent mode, production
go run ./examples/annotations/        # Annotate, HTTP metadata
go run ./examples/pii/                # MaskString API, PIIHook
go run ./examples/audit/              # Audited logger, Verify, tamper detection
go run ./examples/tracing/            # OpenTelemetry
go run ./examples/rotation/           # Size + time rotation
go run ./examples/events/             # Wide Events (Emit, error attachment)
go run ./examples/combined/           # Multiple options together
```

## Architecture

- `axio.go` — Logger interface, core types (Environment, Level, Format)
- `logger.go` — Logger implementation (wraps zap internally); builds the plain or audited core
- `annotation.go` — `Annotation`, `Annotations`, and `HTTP` metadata type
- `event.go` — Wide Event type (`Event`, `Emit`, error attachment)
- `config.go` — Config loading (YAML, JSON, TOML)
- `output.go` — Output interface + implementations (Console, Stdout, File, RotatingFile) + RotationConfig
- `options.go` — Functional options (WithOutputs, WithPII, WithAudit, WithAuditChain, etc.)
- `hook.go` — Hook chain processing (PII → custom hooks)
- `pii.go` — PII masking (CPF, CNPJ, credit card, email, phone)
- `audit.go` — Hash chain: `HashChain` (Add, Verify), `VerifyLines`, `FileStore`, and the audited core that hashes each JSON line as it is written
- `tracing.go` — OpenTelemetry trace extraction
- `metrics.go` — OTel metrics
- `duration.go` — duration parsing for config
- `errors.go` — Sentinel errors
- `internal/logline/` — the shape of a log line, shared by the logger and the `axio` command: keys, audit trailer, short hash, encoder configs, and the `Renderer` that turns JSON back into the Console's text
- `internal/cli/` — the `axio` command tree (Cobra): root, `render`, `verify`, flag types
- `cmd/axio/` — `main` for the `axio` command
