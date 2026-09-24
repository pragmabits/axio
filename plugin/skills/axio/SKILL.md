---
name: axio
description: "How to use axio (github.com/pragmabits/axio), the Go structured logging library. Read this before writing, reviewing or debugging Go code that logs with axio: creating a Logger with axio.New, DefaultConfig or LoadConfig, the level methods and their context and error arguments, annotations with Field, With and Named, outputs and rotation, PII masking and its defaults, the audit hash chain, wide events, hooks and custom extensions, OpenTelemetry tracing and metrics, and the axio command that renders and verifies logs. Trigger phrases include \"axio\", \"pragmabits/axio\", \"axio.New\", \"axio.Config\", \"axio.Field\", \"WithOutputs\", \"WithPII\", \"WithPIIDisabled\", \"WithAudit\", \"NewEvent\", \"wide event\", \"RotatingFile\", \"PII masking\", \"hash chain\", \"axio render\", \"axio verify\", \"structured logging in Go\"."
---

# Using axio

axio is a structured logger for Go with PII masking on by default, a tamper-evident audit chain, OpenTelemetry trace extraction and metrics, and wide events. zap is its internal engine and never part of its API: never import `go.uber.org/zap` or `zapcore` to use axio.

## Check the API against the version in use

This skill describes the latest release. The godoc of the version the project uses is the authority, and a few commands show it:

```bash
go list -m github.com/pragmabits/axio              # the version the go.mod requires
go doc github.com/pragmabits/axio                  # package overview, from that version
go doc github.com/pragmabits/axio WithPII          # one symbol
go doc -all github.com/pragmabits/axio | less      # everything
```

Before axio is a dependency, read https://pkg.go.dev/github.com/pragmabits/axio. When this skill and the godoc disagree, the godoc wins.

## Install

```bash
go get github.com/pragmabits/axio
```

axio needs Go 1.27 or newer, and `go get` raises the project's `go` directive to match. It encodes values with `encoding/json/v2`, which Go 1.27 still ships behind a GOEXPERIMENT that is on by default: building with `GOEXPERIMENT=nojsonv2` fails.

## Create a logger

```go
logger, err := axio.New(axio.Config{
    ServiceName:    "payments",
    ServiceVersion: "2.1.0",
    Environment:    axio.EnvironmentProduction,
    Level:          axio.LevelInfo,
})
if err != nil {
    return err
}
defer logger.Close()
```

`axio.DefaultConfig()` gives development, `LevelInfo` and PII masking on. `axio.LoadConfig("axio.yaml")` reads YAML, JSON or TOML by extension, and `LoadConfigFrom(reader, "yaml")` reads any reader. Loading only parses; `New` applies the defaults and validates, and a bad config fails there, wrapped in `ErrValidateConfig`.

Pass the logger down by dependency injection. A package-level logger hides who owns `Close`.

## Log

```go
logger.Debug(ctx, "cache miss", axio.Field("key", key))
logger.Info(ctx, "order created", axio.Field("order_id", id), axio.Field("total", total))
logger.Warn(ctx, err, "retrying payment", axio.Field("attempt", attempt))
logger.Error(ctx, err, "payment failed", axio.Field("order_id", id))
```

- `context.Context` always comes first. It carries the trace, and the audit chain counts records against it. Use `context.TODO()` where none exists yet.
- `Warn` and `Error` take the `error` second; `Debug` and `Info` take none. A nil error is allowed and writes no `error` key.
- The message is never formatted. A value goes in an annotation, never in a `%s` inside the message.
- `logger.With(annotations...)` returns a logger that adds them to every entry, and `logger.Named("http")` returns one whose name is joined with dots (`app.http`). Both are forks: only the root from `New` owns the outputs. `Close` on a fork returns `ErrLoggerNotRoot`, a second `Close` on the root returns `ErrLoggerClosed`, and after the root closes every log call is a silent no-op.
- The log methods return nothing and never panic. A write or hook failure is reported on stderr as one line starting with `axio:`. What can fail before the first entry (options, validation, opening files, the audit store lock) fails in `New` with a sentinel error you can test with `errors.Is`.

## Annotations

`axio.Field(key, value)` takes the value's type as a type parameter, so primitives stay typed and allocation-free. An untyped `nil` does not compile. Structs, maps and slices are written as JSON through `encoding/json/v2`. A type implementing `Annotable` (`Append([]Annotation) []Annotation`) is expanded into its own fields before any hook runs; `axio.HTTP{Method, URL, StatusCode, LatencyMS, UserAgent, ClientIP}` is one. An annotation named like a key axio writes itself (`message`, `level`, `error`, `hash`...) is written with a leading underscore (`_message`), so a line never repeats a key.

## Environments, levels and defaults

| Environment | Default output | Stack trace |
|---|---|---|
| `EnvironmentDevelopment` | colored text on stderr (`Console`) | no |
| `EnvironmentStaging` | JSON on stdout | on errors |
| `EnvironmentProduction` | JSON on stdout | on errors |

The default output applies only when neither the config nor `WithOutputs` names one. `Level` is the minimum written: `LevelDebug`, `LevelInfo`, `LevelWarn` or `LevelError`. Service metadata (`ServiceName`, `ServiceVersion`, `InstanceID`, the environment) goes only into JSON lines.

## Configuration

Precedence: the file or struct, then options, then defaults, then validation. Options always override the file.

| Option | Effect |
|---|---|
| `WithOutputs(outputs...)` | destinations; the first call replaces the outputs from a file |
| `WithAgentMode()` | stdout JSON only, for Promtail, Fluent Bit or Filebeat |
| `WithOmitCaller()` | skips the caller lookup, about half the cost of a line |
| `WithHooks(hooks...)` | custom hooks, after PII masking, in the order given |
| `WithPII(patterns, fields)` | chooses the patterns and sensitive fields; the patterns listed replace the defaults |
| `WithPIIDisabled()` | turns masking off |
| `WithPIIMaxDepth(n)` | how deep masking walks into a structured value (default 32) |
| `WithPIIOmitErrorVerbose()` | drops an error's verbose form instead of masking it |
| `WithAudit(path)` | audit chain with its state in a file |
| `WithAuditChain(chain)` | audit chain over any `ChainStore` |
| `WithMetrics(provider)` | OpenTelemetry metrics from a `metric.MeterProvider` |
| `WithTracer(axio.Otel())` | adds `trace_id` and `span_id` from the context's span |

A config file uses the same fields in camelCase:

```yaml
serviceName: payments
environment: production
level: info
outputs:
  - type: file
    format: json
    path: /var/log/payments.log
    rotation: { maxSize: 100, maxAge: 30, maxBackups: 10, compress: true, interval: 24h }
piiPatterns: [cpf, cnpj, credit_card, email]
audit: { enabled: true, storePath: /var/lib/payments/chain.json }
tracer: otel
```

The rest: `serviceVersion`, `instanceId`, `callerSkip`, `omitCaller`, `agentMode`, `piiDisabled`, `piiFields`, `piiCustomPatterns`, `piiMaxDepth`, `piiOmitErrorVerbose` and `metrics`.

## Outputs

`Console(format)` writes to stderr, `Stdout(format)` to stdout, `File(path, format)` and `MustFile` to a file, and `RotatingFile(path, format, rotation)` and `MustRotatingFile` to a rotating file. Each output has its own format, `FormatJSON` or `FormatText`. `RotationConfig` has `MaxSize` in MB (0 means no size rotation), `MaxAge` in days, `MaxBackups`, `Compress`, `LocalTime` and `Interval` (a `Duration`, such as `24h`). A custom `Output` implements `Format`, `Type`, `Write`, `Sync` and `Close`, and its `Type()` must be `OutputConsole` or `OutputStdout`.

## PII masking

Masking is on in every Logger and Event, whether the `Config` comes from `DefaultConfig`, a literal or a file. `WithPIIDisabled()` or `piiDisabled: true` turns it off. It costs roughly 1.3× for a plain message and up to about 2.8× for a struct or an error.

- **Patterns.** CPF, CNPJ and credit card by default. Email (`PatternEmail`), phone (`PatternPhone`) and phone without area code (`PatternPhoneNoDDD`) only when listed through `WithPII`. Custom patterns: `CustomPII{Name, Pattern, Mask}`.
- **Sensitive fields.** A key containing `password`, `senha`, `token`, `api_key`, `secret`, `credential`, `authorization`, `bearer`, `private_key` and the other `DefaultSensitiveFields()` has its value replaced by `[REDACTED]`, case-insensitively.
- **Coverage.** The message, the error and its verbose form, and every annotation: strings, Stringers, errors, bytes of text, structs, maps, slices and `http.Header`, walked as the JSON the line writes. Base64 text is decoded and checked. JWT, JWE and JWS in JSON serialization become `[REDACTED]`. Bytes that are not text become `[REDACTED]`. A container nested deeper than the depth limit becomes `[REDACTED]` whole.
- **Not covered.** The logger name, service metadata, the caller and the stack trace are written as they are.
- **Outside a logger.** `axio.MustPIIMasker(axio.DefaultPIIConfig()).MaskString(text)`.
- **Order.** The built-in `PIIHook` runs before every custom hook. A `PIIHook` passed through `WithHooks` runs where it was passed, so with `WithPIIDisabled()` it can mask what an earlier hook added.

## Wide events

One line per unit of work, built up as it runs:

```go
event, err := axio.NewEvent("http_request", config)
if err != nil {
    return err
}
defer event.Close()
ctx = axio.WithEvent(ctx, event)
defer event.Emit(ctx)

// anywhere down the call chain; nil when the context carries no event
if event := axio.EventFromContext(ctx); event != nil {
    event.Add("user_id", userID)
    event.SetError(err, axio.Field("error_code", "card_declined"))
}
```

`Add`, `With` and `SetError` are safe from any goroutine. `Emit` writes one JSON line with the accumulated fields and `duration_ms`, runs PII masking, custom hooks and auditing, and only the first call writes. The line has no level field. An event takes the same `Config` and options as `New`, and `Close` releases its outputs.

## Audit chain

`WithAudit("/var/lib/app/chain.json")` makes each JSON line end with `previous_hash` and `hash`: SHA-256 over the previous hash and the line's own bytes, so any change, removal or reordering breaks the chain. An audited Logger needs a JSON output (`ErrAuditWithoutJSON`). Every Logger and Event on the same path in a process extends one chain, and `New` locks the store against a second process (`ErrChainStoreLocked`; no lock on Windows, Solaris or AIX). `WithAuditChain(chain)` takes a chain over your own `ChainStore` (`Save(sequence, lastHash)` and `Load()`). To verify: `chain.Verify(file, "")`, or `VerifyLines(reader, previousHash)` for one rotated file at a time. The errors are `ErrHashMismatch`, `ErrChainBroken` and `ErrChainIncomplete`. PII is masked before hashing.

## Hooks and extensions

A `Hook` has `Name() string` and `Process(ctx, *Entry) error`. It can change `Message`, `Error`, `TraceID`, `SpanID` and `Annotations`; `Timestamp`, `Level`, `Logger` and `Caller` are read-only. A hook error stops the chain, and that entry is not written. A hook implementing `MetricsAware` receives the `Metrics`. The other extension points are `Output`, `ChainStore`, `Tracer` (`Extract(ctx) (traceID, spanID string, ok bool)`) and `Annotable`.

## Tracing and metrics

`WithTracer(axio.Otel())` adds `trace_id` and `span_id` from the OpenTelemetry span in the context. `WithMetrics(provider)` records `logs.total`, `pii.masked`, `pii.redacted`, `audit.records` and `hook.duration`. No option takes a `Metrics` implementation: another backend plugs in through its OpenTelemetry exporter.

## The axio command

```bash
go install github.com/pragmabits/axio/cmd/axio@latest
kubectl logs -f deploy/payments | axio render                  # JSON lines → the Console's text
axio verify --store /var/lib/app/chain.json app.log.1 app.log   # oldest first; exit 0 when intact
```

`render` reads files or stdin, passes through lines that are not axio entries, keeps any prefix before the JSON, and takes `--utc` and `--color auto|always|never`. `verify --previous-hash` starts mid-chain. `axio --version` prints the release without its `cmd/axio/` tag prefix.
