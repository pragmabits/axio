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

The `axio` command is a module of its own (`cmd/axio`), so Cobra stays out of
the library's `go.mod`. A local `go.work`, which `.gitignore` keeps out of git,
joins the two; create it once per clone. `./...` from the root does not reach
`cmd/axio`, hence the second pattern.

The `Makefile` wraps these: `make help` lists the targets and `make check` runs
every gate (format, vet, lint, tests, race detector, vulnerabilities). A target
creates the `go.work` when missing and installs the tools it runs, pinned, into
`bin/tools`; Go, git and the C compiler are only checked.

```bash
go work init . ./cmd/axio             # Once per clone: join the library and the command
go build ./... ./cmd/axio/...         # Build
go vet ./... ./cmd/axio/...           # Static analysis
golangci-lint run ./... && (cd cmd/axio && golangci-lint run ./...)  # Lint both modules (config in .golangci.yml)
go test ./... ./cmd/axio/... -count=1 # Run tests (no cache)
go test -race ./... ./cmd/axio/...    # Race detector (needs cgo and a C compiler)
go test -bench=. -benchmem -run='^$'  # Benchmarks
go run ./cmd/axio render app.log      # JSON log → the Console's text
go run ./cmd/axio verify --store chain.json app.log  # Check an audited log
go install ./cmd/axio                 # Install the axio command
go run ./examples/basic/              # Logger, levels, named, DefaultConfig
go run ./examples/config/             # LoadConfig, LoadConfigFrom
go run ./examples/outputs/            # Console+File, agent mode, production
go run ./examples/annotations/        # Field, HTTP metadata
go run ./examples/pii/                # MaskString API, masking by default, identifier shapes
go run ./examples/audit/              # Audited logger, Verify, tamper detection
go run ./examples/tracing/            # OpenTelemetry
go run ./examples/rotation/           # Size + time rotation
go run ./examples/events/             # Wide Events (Emit, error attachment)
go run ./examples/combined/           # Multiple options together
```

`cmd/axio/go.mod` requires a published version of the library, and the
workspace hides it. After pushing a library commit the command needs, point the
command at it, or `go install .../cmd/axio@latest` builds against the old one:

```bash
cd cmd/axio && GOWORK=off go get github.com/pragmabits/axio@<commit> && GOWORK=off go mod tidy
make pin-library LIBRARY_VERSION=<version>   # the same, through the Makefile
```

## Architecture

- `axio.go` — Logger interface, core types (Environment, Level, Format)
- `logger.go` — Logger implementation (wraps zap internally); builds the plain or audited core
- `annotation.go` — `Annotation`, `Annotations`, and `HTTP` metadata type
- `event.go` — Wide Event type (`Event`, `Emit`, error attachment), started from a Logger by `Logger.Event`: it owns nothing and writes through the logger's event engine, as JSON or as the Console's text
- `config.go` — Config loading (YAML, JSON, TOML)
- `output.go` — Output interface + implementations (Console, Stdout, File, RotatingFile) + RotationConfig, and the ownership of a file output by one Logger
- `rotator.go` — the one rotator per file in the process (`filesByPath`): rotation by size and by time on the write, the interval kept in the lock file's mtime, backups named as lumberjack names them, cleanup and compression in the background, and the lock against other processes
- `fileowner_linux.go`, `fileowner_other.go` — the owner of a rotated log copied to the new one: on Linux, as lumberjack does, nothing elsewhere
- `canonicalpath_linux.go`, `canonicalpath_other.go` — the resolved path of a log, the registry's key: on Linux the open descriptor's path confirms the path given, any other is resolved by name with `filepath.EvalSymlinks`
- `openlogfile_unix.go`, `openlogfile_other.go` — opening a rotating log and its lock with the descriptor left out of the runtime's poller on Unix; `os.OpenFile` elsewhere
- `options.go` — Functional options (WithOutputs, WithPII, WithPIIDisabled, WithAudit, WithAuditChain, etc.)
- `hook.go` — Hook chain processing (PII → custom hooks)
- `pii.go` — PII masking (CPF, CNPJ, credit card, email, phone) of the message, the error and every annotation, structured values walked as the JSON the log writes (`logline.ValueOptions`), binary bytes — `[]byte`, byte arrays, named byte slices — redacted inside them, a JWS/JWE in JSON serialization redacted whole, no built-in pattern for a number matching inside a whole UUID (RFC 9562 version and variant, or nil) or inside a match of a shape the caller declares (`IdentifierShapes`, refused when it could match a bare number), a number glued to a label masked and one inside a hex identifier not
- `audit.go` — Hash chain: `HashChain` (Add, Verify), `VerifyLines`, `FileStore` (locked per process), and the audited core that hashes each JSON line as it is written
- `storelock_flock.go`, `storelock_other.go` — the lock of a `FileStore` and of a rotating file: flock where it exists, nothing elsewhere
- `tracing.go` — OpenTelemetry trace extraction
- `metrics.go` — OTel metrics
- `duration.go` — duration parsing for config
- `errors.go` — Sentinel errors
- `internal/logline/` — the shape of a log line, shared by the logger and the `axio` command: keys, the reserved-key rename (`FieldKey`), audit trailer, short hash, encoder configs, the value encoding (`ValueOptions`, `encoding/json/v2`) that the encoder configs and PII masking share, and the `Renderer` that turns JSON back into the Console's text
- `cmd/axio/` — the `axio` command, a module of its own: `main`, and `internal/cli/` with the command tree (Cobra): root, `render`, `verify`, flag types
