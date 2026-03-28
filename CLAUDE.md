# Axio

Structured logging library for Go. Zap is the internal engine — never exposed in the public API.

## RULES — READ BEFORE DOING ANYTHING

1. **DO NOT edit, create, delete, or revert any file unless the user explicitly asks you to.** "Test this" means run tests and report. "Check this" means read and report. "Fix this" means NOW you can edit. If the user didn't say edit, you don't edit. Period.

2. **DO NOT add scope beyond what was asked.** If the user says "test", you test. You don't fix, refactor, improve, or "while I'm here" anything. Stay inside the request boundary.

3. **DO NOT assume intent.** If you're unsure whether the user wants you to change something, ask. Do not guess. Do not "help" by doing extra work.

4. **DO NOT revert changes without being asked.** If you made an unauthorized edit and the user is angry, STOP. Do not compound the mistake by reverting without permission. Wait for instructions.

5. **Report findings, then wait.** When you find a bug, a failing test, or a problem: describe it clearly and stop. The user decides what happens next.

6. **No zap/zapcore in public API.** All public types, interfaces, and function signatures use axio's own types. Zap is an internal implementation detail.

7. **Naming conventions are non-negotiable:**
   - Receivers: single-letter, Go-conventional (`l` for `*logger`, `m` for `*PIIMasker`, `h` for `HTTP`)
   - Everything else (params, variables, struct fields, loop vars): descriptive names, no abbreviations (`value` not `v`, `index` not `i`, `fieldName` not `fn`)

8. **Do not rename or remove methods that already work.** When refactoring a type, reimplement existing methods on the new type with the same signatures.

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
go run ./examples/combined/           # Multiple options together
```

## Architecture

- `axio.go` — Logger interface, core types (Environment, Level, Format)
- `logger.go` — Logger implementation (wraps zap internally)
- `config.go` — Config loading (YAML, JSON, TOML)
- `output.go` — Output interface + implementations (Console, Stdout, File, RotatingFile) + RotationConfig
- `options.go` — Functional options (WithOutputs, WithPII, WithAudit, etc.)
- `hook.go` — Hook chain processing
- `pii.go` — PII masking (CPF, CNPJ, credit card, email, phone)
- `audit.go` — Hash chain for tamper-proof audit logs
- `tracing.go` — OpenTelemetry trace extraction
- `metrics.go` — OTel metrics
- `errors.go` — Sentinel errors

## Testing

- Pure `testing` package — no testify, no external frameworks
- Subtests with `t.Run()` for organization
- Test helpers in `testutil_test.go` (tempDir, tempFile, assertEqual, etc.)
- Table-driven tests where appropriate

## Gotchas

- `WithOutputs` converts `Output` objects to `OutputConfig` via type assertions — new Output implementations must be handled there
