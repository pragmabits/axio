# Axio

![English](https://img.shields.io/badge/lang-en-blue.svg)
[Português](./README.pt-BR.md) | **English**

![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/License-0BSD-blue.svg)

## What is Axio

Axio is a structured logger for Go, focused on observability, audit, and data governance. It standardizes fields, reduces the risk of sensitive data leakage, and enables correlation with distributed tracing, without coupling your application to the internal logging engine.

---

## Why a Wrapper?

### The Problem

Direct dependency on logging libraries (Zap, Logrus, zerolog) couples the entire application to a specific implementation. Changes to the logging engine require refactoring dozens of files.

### The Solution

Axio functions as an abstraction layer with a stable interface (`Logger`). Business code depends only on the Axio interface, not on the internal engine.

### Advantages of the Approach

| Advantage               | Description                                       |
| ----------------------- | ------------------------------------------------- |
| Decoupling              | Business code doesn't know about Zap              |
| Facilitated migration   | Change internal engine without refactoring apps   |
| Consistency             | Same API for all teams/services                   |
| Extensibility           | Hooks, metrics, tracing via composition           |
| Testability             | Interface facilitates mocks in tests              |
| Centralized governance  | PII, audit, formats in one place                  |

### Architecture

```
┌─────────────────────────────────────────────────┝
│           Application (business code)           │
│                       ↓                         │
│             axio.Logger (interface)            │
│                       ↓                         │
│   ┌─────────────────────────────────────────┝   │
│   │              Axio Core                 │   │
│   │  ┌─────┝ ┌─────┝ ┌───────┝ ┌─────────┝ │   │
│   │  │ PII │ │Audit│ │Tracing│ │ Metrics │ │   │
│   │  └─────┘ └─────┘ └───────┘ └─────────┘ │   │
│   │                    ↓                    │   │
│   │          Logging Engine                 │   │
│   │          (Zap - replaceable)            │   │
│   └─────────────────────────────────────────┘   │
│                       ↓                         │
│            Outputs (Console/File/Stdout)        │
└─────────────────────────────────────────────────┘
```

---

## Index

- [Installation](#installation)
- [Quick Example](#quick-example)
- [Configuration](#configuration)
  - [Main Config](#main-config)
  - [OutputConfig](#outputconfig)
  - [RotationConfig](#rotationconfig)
  - [AuditConfig](#auditconfig)
  - [MetricsConfig](#metricsconfig)
  - [File Loading](#file-loading)
- [Features](#features)
  - [Outputs](#outputs)
  - [Log Levels](#log-levels)
  - [Structured Annotations](#structured-annotations)
  - [Hooks](#hooks)
  - [PII - Sensitive Data Masking](#pii---sensitive-data-masking)
  - [Audit (Hash Chain)](#audit-hash-chain)
  - [Distributed Tracing (OpenTelemetry)](#distributed-tracing-opentelemetry)
  - [Metrics](#metrics)
  - [Wide Events](#wide-events)
- [Command Line: axio render and axio verify](#command-line-axio-render-and-axio-verify)
- [Logging Best Practices](#logging-best-practices)
- [Guide by Service Type](#guide-by-service-type)
- [Examples and Anti-patterns](#examples-and-anti-patterns)
- [Troubleshooting](#troubleshooting)

---

## Installation

Requires Go 1.27 or newer.

```bash
go get github.com/pragmabits/axio
```

```go
import "github.com/pragmabits/axio"
```

---

## Quick Example

Complete HTTP handler with context, annotations, and cleanup:

```go
package main

import (
    "context"
    "log"
    "net/http"
    "time"

    "github.com/pragmabits/axio"
)

var logger axio.Logger

func main() {
    var err error
    logger, err = axio.New(axio.Config{
        ServiceName:    "sales-api",
        ServiceVersion: "1.0.0",
        Environment:    axio.EnvironmentProduction,
        Level:          axio.LevelInfo,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer logger.Close()

    http.HandleFunc("/api/orders", handleOrder)
    http.ListenAndServe(":8080", nil)
}

func handleOrder(w http.ResponseWriter, r *http.Request) {
    start := time.Now()
    ctx := r.Context()

    // ... business logic ...

    logger.Info(ctx, "order created",
        axio.Field("http", axio.HTTP{
            Method:     r.Method,
            URL:        r.URL.Path,
            StatusCode: 201,
            LatencyMS:  time.Since(start).Milliseconds(),
            ClientIP:   r.RemoteAddr,
        }),
        axio.Field("user_id", "usr_123"),
    )

    w.WriteHeader(http.StatusCreated)
}
```

---

## Configuration

### Main Config

| Field                 | Type             | Required | Default                    | Values                                 | Validation                             |
| --------------------- | ---------------- | -------- | -------------------------- | -------------------------------------- | -------------------------------------- |
| `ServiceName`         | `string`         | No       | `""`                       | any                                    | -                                      |
| `ServiceVersion`      | `string`         | No       | `""`                       | any                                    | -                                      |
| `Environment`         | `Environment`    | No       | `development`              | `production`, `staging`, `development` | `ErrInvalidEnvironment` if invalid     |
| `InstanceID`          | `string`         | No       | `""`                       | any                                    | -                                      |
| `Level`               | `Level`          | No       | `info`                     | `debug`, `info`, `warn`, `error`       | `ErrInvalidLevel` if invalid           |
| `CallerSkip`          | `int`            | No       | `0`                        | `>= 0`                                 | -                                      |
| `AgentMode`           | `bool`           | No       | `false`                    | `true`, `false`                        | If `true`, outputs must be stdout+json |
| `Outputs`             | `[]OutputConfig` | No       | auto                       | see OutputConfig                       | Validated individually                 |
| `PIIEnabled`          | `bool`           | No       | `false`                    | `true`, `false`                        | -                                      |
| `PIIPatterns`         | `[]PIIPattern`   | No       | `[cpf, cnpj, credit_card]` | see PII table                          | -                                      |
| `PIIFields`           | `[]string`       | No       | `DefaultSensitiveFields()` | any                                    | -                                      |
| `PIIMaxDepth`         | `int`            | No       | `0` (= `32`)               | `>= 0`                                 | `ErrInvalidPIIMaxDepth` if negative    |
| `PIIOmitErrorVerbose` | `bool`           | No       | `false`                    | `true`, `false`                        | -                                      |
| `PIICustomPatterns`   | `[]CustomPII`    | No       | `[]`                       | see CustomPII                          | Regex must be valid                    |
| `TracerType`          | `string`         | No       | `noop`                     | `otel`, `noop`                         | `ErrInvalidTracer` if invalid          |
| `Audit`               | `AuditConfig`    | No       | disabled                   | see AuditConfig                        | -                                      |
| `Metrics`             | `MetricsConfig`  | No       | disabled                   | see MetricsConfig                      | -                                      |

### OutputConfig

| Field    | Type         | Required    | Default | Values                      | Validation                                    |
| -------- | ------------ | ----------- | ------- | --------------------------- | --------------------------------------------- |
| `Type`   | `OutputType` | Yes         | -       | `console`, `stdout`, `file` | `ErrInvalidOutputType` if invalid             |
| `Format` | `Format`     | Yes         | -       | `json`, `text`              | `ErrInvalidFormat` if invalid                 |
| `Path`   | `string`     | Conditional | `""`    | file path                   | `ErrFileOutputNoPath` if `Type=file` and empty|
| `Rotation` | `RotationConfig` | No     | disabled | see RotationConfig         | Only used when `Type=file`                    |

### RotationConfig

| Field        | Type       | Required | Default | Values                        | Validation |
| ------------ | ---------- | -------- | ------- | ----------------------------- | ---------- |
| `MaxSize`    | `int`      | No       | `0`     | megabytes (0 = no size limit) | -          |
| `MaxAge`     | `int`      | No       | `0`     | days (0 = no age limit)       | -          |
| `MaxBackups` | `int`      | No       | `0`     | count (0 = retain all)        | -          |
| `Compress`   | `bool`     | No       | `false` | `true`, `false`               | -          |
| `LocalTime`  | `bool`     | No       | `false` | `true`, `false`               | -          |
| `Interval`   | `Duration` | No       | `0`     | e.g., `24h`, `1h30m`, `500ms` | -          |

### AuditConfig

| Field       | Type     | Required    | Default | Values         | Validation                                       |
| ----------- | -------- | ----------- | ------- | -------------- | ------------------------------------------------ |
| `Enabled`   | `bool`   | No          | `false` | `true`, `false`| -                                                |
| `StorePath` | `string` | Conditional | `""`    | file path      | `ErrAuditWithoutPath` if `Enabled=true` and empty|

### MetricsConfig

| Field          | Type     | Required | Default | Values          | Validation |
| -------------- | -------- | -------- | ------- | --------------- | ---------- |
| `Enabled`      | `bool`   | No       | `false` | `true`, `false` | -          |
| `MeterName`    | `string` | No       | `axio`  | any             | -          |
| `MeterVersion` | `string` | No       | `1.0.0` | any             | -          |

### File Loading

Axio supports configuration via YAML, JSON, or TOML:

```go
// Load from file (detects format by extension)
config, err := axio.LoadConfig("/etc/axio/config.yaml")

// Load from io.Reader (specify format)
config, err := axio.LoadConfigFrom(reader, "yaml")

// Panic version (useful in main)
config := axio.MustLoadConfig("/etc/axio/config.yaml")
```

**Complete YAML example:**

```yaml
serviceName: sales-api
serviceVersion: 2.1.0
environment: production
instanceId: pod-abc123
level: info
callerSkip: 0
agentMode: false

outputs:
  - type: stdout
    format: json
  - type: file
    format: json
    path: /var/log/app.log
    rotation:
      maxSize: 100
      maxAge: 30
      maxBackups: 10
      compress: true
      interval: 24h

piiEnabled: true
piiPatterns:
  - cpf
  - cnpj
  - email
  - credit_card
piiFields:
  - password
  - token
  - secret
piiMaxDepth: 8
piiOmitErrorVerbose: false

piiCustomPatterns:
  - name: employee_id
    pattern: "EMP-\\d{6}"
    mask: "EMP-******"

audit:
  enabled: true
  storePath: /var/lib/axio/chain.json

tracer: otel

metrics:
  enabled: true
  meterName: axio
  meterVersion: 1.0.0
```

---

## Features

### Outputs

#### Output Types

| Type      | Destination | Typical use                           |
| --------- | ----------- | ------------------------------------- |
| `console` | stderr      | Local development                     |
| `stdout`  | stdout      | Containers with collection agents     |
| `file`    | file        | Environments without agents, auditing |

#### Formats

| Format | Description      | Use                                |
| ------ | ---------------- | ---------------------------------- |
| `json` | Structured JSON  | Production, aggregation systems    |
| `text` | Colored text     | Local development                  |

#### Behavior by Environment

| Environment   | Default Output | Format | Stack Trace |
| ------------- | -------------- | ------ | ----------- |
| `development` | Console        | Text   | No          |
| `staging`     | Stdout         | JSON   | On errors   |
| `production`  | Stdout         | JSON   | On errors   |

#### Configuration via Options

```go
// Multiple outputs
logger, _ := axio.New(config,
    axio.WithOutputs(
        axio.Console(axio.FormatText),
        axio.Stdout(axio.FormatJSON),
        axio.MustFile("/var/log/app.log", axio.FormatJSON),
    ),
)

// Agent mode (stdout + JSON, optimized for Promtail, Fluent Bit, etc.)
logger, _ := axio.New(config, axio.WithAgentMode())
```

Options override the config file: the first `WithOutputs` replaces the file's `outputs`, which are then neither opened nor validated, and later calls add to it.

#### Log Rotation

File outputs support automatic rotation by size, time interval, or both:

```go
// Size-based rotation (rotate when file exceeds 100 MB)
out, _ := axio.RotatingFile("/var/log/app.log", axio.FormatJSON, axio.RotationConfig{
    MaxSize:    100,
    MaxBackups: 5,
    Compress:   true,
})

// Time-based rotation (rotate every 24 hours)
out, _ := axio.RotatingFile("/var/log/app.log", axio.FormatJSON, axio.RotationConfig{
    Interval: axio.Duration(24 * time.Hour),
    MaxAge:   30,
})

// Combined (whichever triggers first)
out, _ := axio.RotatingFile("/var/log/app.log", axio.FormatJSON, axio.RotationConfig{
    MaxSize:    100,
    Interval:   axio.Duration(24 * time.Hour),
    MaxBackups: 10,
    MaxAge:     30,
    Compress:   true,
})

logger, _ := axio.New(config, axio.WithOutputs(out))
defer logger.Close()
```

`MustRotatingFile` is available for initialization where failure should be fatal.

---

### Log Levels

| Level | Constant     | Semantics            | When to use                      |
| ----- | ------------ | -------------------- | -------------------------------- |
| Debug | `LevelDebug` | Technical details    | Development, troubleshooting     |
| Info  | `LevelInfo`  | Normal events        | Start/end of operations, milestones |
| Warn  | `LevelWarn`  | Non-critical anomalies | Timeouts, fallbacks, degradation |
| Error | `LevelError` | Real failures        | Operation failed, requires attention |

**Methods:**

```go
logger.Debug(ctx, "debug details")
logger.Info(ctx, "items processed", axio.Field("count", count))
logger.Warn(ctx, err, "timeout querying supplier")
logger.Error(ctx, err, "failed to persist order")
```

The message is written as given, never formatted. Data goes in annotations passed after it, which the entry carries after those of the logger (`With`) and which no other call sees.

---

### Structured Annotations

#### Field

Adds key-value fields to the log:

```go
logger.Info(ctx, "order created",
    axio.Field("user_id", "usr_123"),
    axio.Field("order_id", "ord_456"),
    axio.Field("amount_cents", 15000),
)
```

An annotation named like a key axio writes itself — `timestamp`, `level`, `message`, `logger`, `caller`, `stacktrace`, `service`, `deployment`, `trace_id`, `span_id`, `error` (with `errorVerbose` and `errorCauses`), `event`, `duration_ms`, `previous_hash`, `hash` — is written behind an underscore, as `_message`, so a line never carries the same key twice.

A struct, a slice or a map is written as its JSON encoding, in `encoding/json/v2`: nil slices and maps as `null`, map keys in sorted order, a `time.Duration` as its nanoseconds and a byte array as base64. `omitempty` omits a field whose value encodes as empty — `""`, `null`, `[]`, `{}` — and `omitzero` omits `false`, `0` and every other zero value. A tag option the encoding does not accept, such as `,string` on a slice, makes the value fail: the line carries `<key>Error` in its place.

#### With

Returns a logger that attaches its annotations to every entry it writes, before those each call passes. Use it for fields that hold across several lines, such as a request's ID; the fields of one line go in the call itself:

```go
requestLogger := logger.With(axio.Field("request_id", requestID))

requestLogger.Info(ctx, "order created", axio.Field("user_id", userID))
requestLogger.Error(ctx, err, "payment failed")
```

#### HTTP

Struct for HTTP request metadata:

```go
logger.Info(ctx, "request processed", axio.Field("http", axio.HTTP{
    Method:     "POST",
    URL:        "/api/v1/orders",
    StatusCode: 201,
    LatencyMS:  45,
    UserAgent:  r.UserAgent(),
    ClientIP:   r.RemoteAddr,
}))
```

| Field        | Type     | Description                    |
| ------------ | -------- | ------------------------------ |
| `Method`     | `string` | HTTP method (GET, POST, etc.)  |
| `URL`        | `string` | Request path                   |
| `StatusCode` | `int`    | Response code                  |
| `LatencyMS`  | `int64`  | Latency in milliseconds        |
| `UserAgent`  | `string` | Client User-Agent              |
| `ClientIP`   | `string` | Client IP                      |

#### Annotable (custom types)

Implement `Annotable` for types that produce multiple fields:

```go
type Order struct {
    ID     string
    Items  []Item
    secret string // will not be logged
}

func (o Order) Append(target []axio.Annotation) []axio.Annotation {
    return append(target,
        axio.Field("order_id", o.ID),
        axio.Field("item_count", len(o.Items)),
    )
}

// Usage — fields are expanded individually in the log output
logger.Info(ctx, "order processed", axio.Field("order", order))
```

The fields are expanded before any hook runs, so PII masking and custom hooks see each one.

#### Named (sub-loggers)

Creates loggers with namespace:

```go
httpLogger := logger.Named("http")
dbLogger := logger.Named("db")
cacheLogger := logger.Named("cache")

httpLogger.Info(ctx, "request received")  // logger: "http"
dbLogger.Info(ctx, "query executed")      // logger: "db"
```

---

### Hooks

Hooks process log entries before writing. Executed in fixed order:

1. **PIIHook** - masks sensitive data
2. **Custom hooks** - in the order passed to `WithHooks`

Auditing is not a hook: the hash is computed when the entry is written, after every hook, so it covers whatever the hooks changed.

#### Hook Interface

```go
type Hook interface {
    Name() string
    Process(ctx context.Context, entry *Entry) error
}
```

#### Custom Hook

```go
type TenantHook struct {
    tenantID string
}

func (h TenantHook) Name() string { return "tenant" }

func (h TenantHook) Process(ctx context.Context, entry *axio.Entry) error {
    entry.Annotations = append(entry.Annotations,
        axio.Field("tenant_id", h.tenantID))
    return nil
}

// Usage
logger, _ := axio.New(config, axio.WithHooks(TenantHook{tenantID: "acme"}))
```

---

### PII - Sensitive Data Masking

#### What is PII?

**PII** (Personally Identifiable Information) is any data that can identify a person, directly or indirectly. Examples: CPF, CNPJ, email, phone, IP address, card numbers.

In environments with centralized logs, exposed PII represents risk of:
- Data leakage
- Non-compliance with LGPD/GDPR
- Exposure in security incidents

**References:**
- [LGPD - Law 13.709/2018](https://www.planalto.gov.br/ccivil_03/_ato2015-2018/2018/lei/l13709.htm)
- [OWASP Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)

#### Built-in Patterns

| Pattern         | Constant            | Detected formats                | Mask                  |
| --------------- | ------------------- | ------------------------------- | --------------------- |
| CPF             | `PatternCPF`        | `123.456.789-01`, `12345678901` | `***.***.***-**`      |
| CNPJ            | `PatternCNPJ`       | `12.345.678/0001-90`            | `**.***.***/****-**`  |
| Credit Card     | `PatternCreditCard` | `1234-5678-9012-3456`           | `****-****-****-****` |
| Email           | `PatternEmail`      | `user@domain.com`               | `***@***.***`         |
| Phone           | `PatternPhone`      | `(11) 99999-9999`               | `(**) *****-****`     |
| Phone (no area) | `PatternPhoneNoDDD` | `99999-9999`                    | `*****-****`          |

#### Automatic Sensitive Fields

Fields whose names contain these terms are automatically redacted to `[REDACTED]`:

`password`, `senha`, `token`, `api_key`, `apikey`, `secret`, `credential`, `authorization`, `bearer`, `private_key`, `privatekey`, `access_key`, `secret_key`, `client_secret`, `clientsecret`

#### Configuration

```go
// Via Options (recommended)
logger, _ := axio.New(config,
    axio.WithPII(
        []axio.PIIPattern{axio.PatternCPF, axio.PatternEmail},
        axio.DefaultSensitiveFields(),
    ),
)

// Via Hook directly
hook := axio.MustPIIHook(axio.DefaultPIIConfig())
logger, _ := axio.New(config, axio.WithHooks(hook))

// Via Config (YAML file)
// piiEnabled: true
// piiPatterns: [cpf, cnpj, email]
```

#### Custom Patterns

```go
config := axio.PIIConfig{
    Patterns: []axio.PIIPattern{axio.PatternCPF},
    CustomPatterns: []axio.CustomPII{
        {
            Name:    "employee_id",
            Pattern: `EMP-\d{6}`,
            Mask:    "EMP-******",
        },
    },
    Fields: axio.DefaultSensitiveFields(),
}
```

#### Coverage

PII masking covers every value a line carries:

- **The message.**
- **The error** passed to `Warn`, `Error` or `Event.SetError`, by its message and, for an error that formats itself, its verbose form (`errorVerbose`). A masked error still unwraps to the original, so `errors.Is` keeps working in later hooks.
- **Annotation names matching `PIIConfig.Fields`** — the whole value becomes `[REDACTED]`, whatever its type.
- **Strings, errors and `fmt.Stringer` values**, by their text.
- **Bytes (`[]byte`)**, which are written as base64, by the text they hold: masked text stays bytes, and bytes that are not UTF-8 text cannot be inspected and become `[REDACTED]`, whether an annotation of their own or a field or element of a structured value.
- **Base64 text.** Any string with the shape of base64, in the standard or the URL alphabet, padded or not — the message, the error, an annotation, a value inside a map or struct — is also decoded, and masked when the text it decodes to carries PII. That is how a `[]byte` of text, a byte array or a named byte-slice type inside a structured value arrives, its JSON encoding carrying it as base64. A string that decodes to binary data becomes `[REDACTED]` when a pattern matches inside it, and passes as it is otherwise: nothing tells the base64 of other binary data from any other string of that shape.
- **JWTs and JWEs.** A token anywhere in a text — a message, a URL query, an annotation — becomes `[REDACTED]` whole: it is a credential, and its claims may carry what no pattern knows. A string is taken for a token when it has a token's shape and its header decodes to a JSON object naming an algorithm (`alg`), as every JOSE header does.
- **Encoding failures.** A value whose encoding fails — a `MarshalJSON`, `MarshalLogObject` or `MarshalLogArray` that returns an error — has that error masked where it is written, under `<key>Error`, and the part it did write masked like any value.
- **Structured values** — maps, slices, structs, pointers, `http.Header` — walked as the JSON encoding the log writes for them: at every level, keys are checked against `Fields` and strings against the patterns.
- **`Annotable` values** such as `HTTP`, expanded into their fields before any hook runs.

A structured value that needed masking is written as its masked JSON tree, object keys in alphabetical order; one with nothing to mask keeps its original form. A container nested deeper than the depth limit (default `32`) is replaced by `[REDACTED]` whole, never written unmasked. Set the limit with `axio.WithPIIMaxDepth(n)`, `piiMaxDepth` in the config file, or `PIIConfig.MaxDepth` when building a `PIIMasker` or `PIIHook` yourself.

The verbose form of an error that formats itself — the `errorVerbose` beside its message, a stack trace for many errors — is masked and kept. Masking it means scanning it with every pattern: with the default patterns, about 250 µs for a stack of about 2 KB. To omit it instead, never reading it, use `axio.WithPIIOmitErrorVerbose()`, `piiOmitErrorVerbose: true` in the config file, or `PIIConfig.OmitErrorVerbose`; the error is then written by its masked message only.

```go
type User struct {
    Email    string `json:"email"`
    Password string `json:"password"`
}

logger.Info(ctx, "...", axio.Field("user", User{Email: "a@b.com", Password: "x"}))
//   -> "user": {"email": "***@***.***", "password": "[REDACTED]"}   (with PatternEmail)
```

---

### Audit (Hash Chain)

#### What is a Hash Chain?

A **hash chain** is a structure where each record contains the cryptographic hash of the previous record. Any modification to a record breaks the entire subsequent chain, allowing tampering detection.

Useful for:
- Regulatory compliance (LGPD, SOX, PCI-DSS)
- Tamper-proof audit logs
- Integrity evidence in investigations

**Important:** Hash chain detects tampering, it doesn't prevent it. Immutability depends on the storage backend.

#### How axio hashes a line

Each audited JSON line ends with two fields, always last:

| Field           | Description                                                          |
| --------------- | -------------------------------------------------------------------- |
| `previous_hash` | Hash of the previous line; empty on the first line of the chain      |
| `hash`          | SHA-256 of `previous_hash` followed by every byte before the trailer |

The hash covers exactly what was written: message, error, stacktrace, service metadata, annotations and whatever custom hooks changed. Encoding, hashing and writing happen under one lock, so the order of the chain is the order of the file, even with many goroutines logging at once.

```json
{"level":"info","timestamp":"2026-09-23T15:16:24.230105632Z","logger":"orders","caller":"app/main.go:26","message":"order created","order_id":"ord_8812","previous_hash":"","hash":"28d41c7b24128b63eed1ed71b77bc63e3f9b9b463af820a3171a0a1927b3f3a8"}
```

Only JSON outputs can be verified. A text output shows the first 6 characters of the hash, for finding the same entry in the JSON output.

#### Configuration

```go
// Chain state in a local file
logger, _ := axio.New(config,
    axio.WithOutputs(axio.MustFile("/var/log/app.log", axio.FormatJSON)),
    axio.WithAudit("/var/lib/axio/chain.json"),
)
```

Every Logger and Event audited with the same path in a process extends **one** chain, whichever was created first.

An audited Logger needs a JSON output: only JSON lines carry the hashes a log is verified against, so `New` returns `ErrAuditWithoutJSON` when every output is text. An Event writes JSON to every output and has no such requirement.

The first write takes an exclusive lock on a file beside the store (`chain.json.lock`) and holds it while the process runs: a second process using the same path fails in `New` with `ErrChainStoreLocked` instead of forking the chain. Reading the store, as verifying does, takes no lock. The lock uses `flock`, so Windows, Solaris and AIX have none.

#### Verifying a log

```go
chain, err := axio.NewHashChain(axio.NewFileStore("/var/lib/axio/chain.json"))
if err != nil {
    return err
}
file, err := os.Open("/var/log/app.log")
if err != nil {
    return err
}
defer file.Close()

// "" because the file starts the chain; for a rotated file, pass the last
// hash of the file before it.
if err := chain.Verify(file, ""); err != nil {
    return fmt.Errorf("audit log does not verify: %w", err)
}
```

| Error                | Meaning                                                          |
| -------------------- | ---------------------------------------------------------------- |
| `ErrHashMismatch`    | A line's content changed after it was written                     |
| `ErrChainBroken`     | A line was removed, moved or inserted, or carries no trailer      |
| `ErrChainIncomplete` | The log ends before the chain: its end was removed, or the whole chain was rewritten |

For rotated files, `axio.VerifyLines(reader, previousHash)` checks one file and returns the hash of its last line — the `previousHash` of the next file. `chain.Verify` is `VerifyLines` plus the check that the log ends at the chain's last hash. From the terminal, `axio verify` does both (see below).

#### Custom ChainStore

Implement `ChainStore` for custom backends (Redis, PostgreSQL, etc.) and pass the chain with `WithAuditChain`. Loggers and Events given the same chain extend one chain:

```go
type ChainStore interface {
    Save(sequence uint64, lastHash string) error
    Load() (sequence uint64, lastHash string, err error)
}

chain, err := axio.NewHashChain(redisStore)
if err != nil {
    return err
}
logger, _ := axio.New(config, axio.WithAuditChain(chain))
```

---

### Distributed Tracing (OpenTelemetry)

#### What is Distributed Tracing?

**Distributed tracing** allows tracking a request through multiple services. Each operation receives a **span** identified by:

- **trace_id**: unique identifier of the complete request
- **span_id**: unique identifier of this specific operation

With these IDs in logs, it's possible to correlate logs and traces in tools like Jaeger, Tempo, or Zipkin.

#### Why OpenTelemetry?

Axio uses OpenTelemetry (OTel) as the standard for tracing for the following reasons:

| Factor             | OpenTelemetry                                                      |
| ------------------ | ------------------------------------------------------------------ |
| **Standardization**| Official CNCF project, industry standard                           |
| **Vendor-neutral** | Works with any backend (Jaeger, Zipkin, Datadog, AWS X-Ray)        |
| **Unification**    | Traces, metrics, and logs in a single API                          |
| **Adoption**       | AWS, GCP, Azure, Datadog, Grafana, all support it                  |
| **Community**      | Active development, extensive documentation                        |
| **Future**         | Official successor to OpenTracing and OpenCensus                   |

**Considered alternatives:**
- **Jaeger client**: specific to Jaeger, discontinued in favor of OTel
- **Zipkin**: less flexible, no signal unification
- **Proprietary**: vendor lock-in

**References:**
- [OpenTelemetry](https://opentelemetry.io/)
- [OTel Go](https://opentelemetry.io/docs/languages/go/)
- [CNCF - OpenTelemetry](https://www.cncf.io/projects/opentelemetry/)

#### Configuration

```go
// Via Options (recommended)
logger, _ := axio.New(config, axio.WithTracer(axio.Otel()))

// Via Config (YAML file)
// tracer: otel

// Disable (default)
logger, _ := axio.New(config, axio.WithTracer(axio.NoopTracing()))
```

#### Usage with Active Span

```go
func handleRequest(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context() // contains span from OTel middleware

    logger.Info(ctx, "request received")
    // Log will include: {"trace_id": "abc123...", "span_id": "def456..."}
}
```

---

### Metrics

#### What are Observability Metrics?

**Metrics** are numerical values that represent the system state over time. Common types:
- **Counters**: values that only increase (e.g., total logs)
- **Histograms**: distribution of values (e.g., hook duration)

Axio emits metrics about the logging process itself, allowing monitoring of volume, errors, and performance.

**References:**
- [OTel Metrics](https://opentelemetry.io/docs/concepts/signals/metrics/)
- [Prometheus](https://prometheus.io/docs/concepts/metric_types/)

#### Emitted Metrics

| Metric          | Type      | Labels               | Description                    |
| --------------- | --------- | -------------------- | ------------------------------ |
| `logs.total`    | Counter   | `level`              | Total logs emitted             |
| `pii.masked`    | Counter   | `pattern`            | PII occurrences masked         |
| `audit.records` | Counter   | -                    | Audit records created          |
| `hook.duration` | Histogram | `hook.name`, `error` | Hook execution duration        |

#### Configuration

```go
// Via Options with MeterProvider
provider := otel.GetMeterProvider()
logger, _ := axio.New(config, axio.WithMetrics(provider))

// Via Config (uses global provider with warning)
// metrics:
//   enabled: true
//   meterName: axio
//   meterVersion: 1.0.0
```

#### Metrics Interface (custom)

```go
type Metrics interface {
    LogsTotal(ctx context.Context, level Level)
    PIIMasked(ctx context.Context, pattern PIIPattern)
    AuditRecords(ctx context.Context)
    HookDuration(ctx context.Context, hookName string, duration time.Duration, hasError bool)
}
```

---

### Wide Events

#### What are Wide Events?

**Wide events** (also called canonical log lines) replace scattered per-step log lines with a single, richly-annotated entry emitted at the end of a unit of work (e.g., an HTTP request). Instead of 5–10 lines per request, one event captures the complete context of what happened.

Wide events omit the log level field — severity is expressed through the event's own fields (`status_code`, `error`, etc.), not through traditional log levels.

#### Basic Usage

```go
event, err := axio.NewEvent("checkout", config)
if err != nil {
    return err
}
defer event.Close()

event.Add("user_id", userID)
event.Add("cart_total", 15999)
event.Add("item_count", 3)

event.Emit(ctx)
// Output: {"timestamp":"...","event":"checkout","user_id":"usr_456","cart_total":15999,"item_count":3,"duration_ms":42}
```

#### API

| Method | Description |
| ------ | ----------- |
| `NewEvent(name, config, ...Option)` | Creates a new event with the same Config/Option used by `New` |
| `Add(key, value)` | Adds a key-value field (thread-safe) |
| `With(...Annotation)` | Adds annotations, including `Annotable` types like `HTTP` |
| `SetError(err, ...Annotation)` | Records an error with optional detail annotations |
| `Emit(ctx)` | Writes the event as a single log entry (computes `duration_ms`, runs hooks; a hook may rename the event) |
| `Close()` | Releases output resources (call after `Emit`); an `Emit` after `Close` writes nothing, and a second `Close` returns `ErrEventClosed` |

#### Context Propagation (Middleware Pattern)

Store the event in the context so downstream handlers can enrich it:

```go
// Middleware: create and store
event, _ := axio.NewEvent("http_request", config)
defer event.Close()
ctx = axio.WithEvent(ctx, event)

// Handler: enrich from context
event := axio.EventFromContext(ctx)
event.Add("user_id", userID)
event.With(axio.Field("http", axio.HTTP{
    Method:     r.Method,
    URL:        r.URL.Path,
    StatusCode: 201,
    LatencyMS:  latencyMS,
}))

// Middleware: emit at end of request
event.Emit(ctx)
```

#### Error Recording

```go
// Simple error
event.SetError(err)

// Error with structured details
event.SetError(err,
    axio.Field("error_code", "card_declined"),
    axio.Field("error_retriable", false),
)
```

#### Integration with Features

Wide events support the same options as the standard logger:

```go
// With PII masking
event, _ := axio.NewEvent("user_registration", config,
    axio.WithPII(nil, nil),
)

// With audit hash chain
event, _ := axio.NewEvent("access_grant", config,
    axio.WithAudit("/var/lib/axio/chain.json"),
)

// With tracing
event, _ := axio.NewEvent("http_request", config,
    axio.WithTracer(axio.Otel()),
)
```

---

## Command Line: axio render and axio verify

```bash
go install github.com/pragmabits/axio/cmd/axio@latest
```

The command is a module of its own, `github.com/pragmabits/axio/cmd/axio`, so importing the library does not bring its Cobra dependency along.

### axio render

Logs meant for machines are JSON. `axio render` turns them back into the text a person reads on a terminal — the same text the `Console` output writes, byte for byte, colors included.

```bash
kubectl logs -f deploy/checkout | axio render
kubectl logs -f -l app=checkout --prefix | axio render
docker compose logs -f checkout | axio render
journalctl -u checkout -o cat -f | axio render
axio render /var/log/checkout.log
kubectl logs deploy/checkout | axio render | grep -E 'WARN|ERROR'
kubectl logs deploy/checkout | axio render --color=always | less -R
```

- Reads files in order, or standard input; `-` also names standard input
- Writes each line as soon as it reads it, so it follows `-f` streams
- Lines that are not axio entries pass through unchanged; a prefix before the JSON (pod name, compose service) stays in front
- Wide events show `EVENT` in the level column
- `--color=auto|always|never`: `auto` colors only a terminal and respects `NO_COLOR`
- `--utc`: timestamps in UTC instead of the local time zone
- `axio completion bash|zsh|fish|powershell` generates shell completion

### axio verify

Checks an audited JSON log against the chain stored by `WithAudit`: every line's hash matches the line, every line continues the one before it, and the log ends where the chain ends.

```bash
axio verify --store /var/lib/axio/chain.json /var/log/app.log
axio verify --store chain.json app.log.2 app.log.1 app.log          # rotated files, oldest first
axio verify --store chain.json --previous-hash "$(tail -1 app.log.1 | jq -r .hash)" app.log
kubectl logs deploy/payments | axio verify --store chain.json
```

```text
verified: the log reaches the chain's last hash 550e01                      # exit 0
axio: app.log.1: hash mismatch: line 2                                      # exit 1
axio: app.log: chain integrity compromised: line 1 does not continue the line before it
axio: log does not reach the chain's last hash: log ends at "f0503d", chain at "550e01"
```

- `--store` is required; `--previous-hash` is the hash of the line before the first one, when the oldest file you have is not the first of the chain
- Verify a log that is no longer being written: while a logger is still writing, the chain may end past the last line read

---

## Logging Best Practices

### 1. Structure before text

- Use structured fields for data; message is human summary
- Prefer stable keys: `user_id`, `order_id`, `tenant_id`
- Avoid dynamic keys: `field_123`, `user_email_john@...`

### 2. Levels with clear semantics

| Level | Use when                         |
| ----- | -------------------------------- |
| Debug | Technical details, temporary     |
| Info  | Normal events, flow milestones   |
| Warn  | Anomalies that don't interrupt   |
| Error | Real operation failure           |

**Rule:** Log error once, at the system boundary (handler, job, consumer).

### 3. Context and correlation

Always pass `context.Context` and add identifiers:

- `request_id` / `correlation_id`
- `user_id`, `tenant_id`
- `trace_id`, `span_id` (via tracing)

### 4. PII and sensitive data

- Use `PIIHook` as default defense
- Never log: password, token, secret, private key
- If you need the payload, log hash or ID, not the content

### 5. Performance and cost

- Avoid logs in hot loops; prefer aggregation
- Don't build large strings/maps unnecessarily
- In production: JSON + agent collection

### 6. Controlled cardinality

Fields with unlimited values (email, payloads) explode indexes. Maintain:

- Stable IDs (user, order, tenant)
- Status codes, methods, endpoints
- Latency in milliseconds

### 7. Audit and integrity

For critical operations, use `WithAudit` with a JSON output and combine with reliable storage.

### 8. Standard HTTP fields

```go
logger.Info(ctx, "request completed",
    axio.Field("http", axio.HTTP{
        Method:     r.Method,
        URL:        r.URL.Path,
        StatusCode: statusCode,
        LatencyMS:  latencyMS,
        ClientIP:   r.RemoteAddr,
    }),
    axio.Field("request_id", requestID),
    axio.Field("user_id", userID),
)
```

### 9. Review checklist

- [ ] Does the message summarize the event?
- [ ] Are fields consistent and stable?
- [ ] Is the level correct?
- [ ] Is there exposed PII?
- [ ] Was the error logged only once?

---

## Guide by Service Type

### HTTP/gRPC APIs

**Goal:** Measure latency, success/error, track requests.

| Event              | Level      | Suggested fields                              |
| ------------------ | ---------- | --------------------------------------------- |
| Request completed  | Info       | `http.*`, `request_id`, `user_id`, `trace_id` |
| Domain error       | Warn/Error | `+operation`, `+entity`, `+error`             |

```go
logger.Info(ctx, "request completed", axio.Field("http", axio.HTTP{...}), axio.Field("request_id", id))
```

### Workers and Jobs

**Goal:** Know when it started, finished, how much it processed.

| Event         | Level | Suggested fields                                             |
| ------------- | ----- | ------------------------------------------------------------ |
| Job started   | Info  | `job_name`, `job_id`                                         |
| Job completed | Info  | `+items_total`, `+items_ok`, `+items_failed`, `+duration_ms` |
| Item error    | Warn  | `+item_id`, `+error` (sampled)                               |

```go
logger.Info(ctx, "job completed",
    axio.Field("job_name", "reconcile_payments"),
    axio.Field("items_ok", okCount),
    axio.Field("items_failed", failedCount),
)
```

### Queue Consumers

**Goal:** Track consumption, retries, failures per message.

| Event              | Level      | Suggested fields                    |
| ------------------ | ---------- | ----------------------------------- |
| Message processed  | Info/Debug | `queue`, `message_id`, `latency_ms` |
| Message failure    | Warn/Error | `+retry_count`, `+error`            |

### External Integrations

**Goal:** Visibility of latency and failures in third parties.

| Event          | Level      | Suggested fields                                     |
| -------------- | ---------- | ---------------------------------------------------- |
| External call  | Info/Debug | `provider`, `operation`, `status_code`, `latency_ms` |
| Timeout/error  | Warn       | `+attempt`, `+timeout_ms`                            |

### CLIs and Scripts

**Goal:** Audit execution and result.

| Event | Level | Suggested fields                              |
| ----- | ----- | --------------------------------------------- |
| Start | Info  | `command`, `args_redacted`                    |
| End   | Info  | `+exit_code`, `+duration_ms`, `+output_count` |

---

## Examples and Anti-patterns

### Anti-pattern: Concatenation for structured data

**Wrong:**
```go
logger.Info(ctx, fmt.Sprintf("user=%s status=%d", userID, statusCode))
```

**Correct:**
```go
logger.Info(ctx, "request completed",
    axio.Field("user_id", userID),
    axio.Field("status_code", statusCode),
)
```

### Anti-pattern: Payload with PII

**Wrong:**
```go
logger.Info(ctx, fmt.Sprintf("payload=%+v", payload))
```

**Correct:**
```go
logger.Info(ctx, "payload received",
    axio.Field("payload_id", payload.ID),
    axio.Field("payload_size", len(payload.Data)),
)
```

### Anti-pattern: Duplicate log in layers

**Wrong:**
```go
// repository
if err != nil {
    logger.Error(ctx, err, "failed to insert")
    return err
}
```

**Correct:**
```go
// repository
if err != nil {
    return fmt.Errorf("insert order: %w", err)
}

// handler (system boundary)
if err != nil {
    logger.Error(ctx, err, "failed to create order")
}
```

### Anti-pattern: Log in hot loop

**Wrong:**
```go
for _, item := range items {
    logger.Debug(ctx, "processing item", axio.Field("item_id", item.ID))
}
```

**Correct:**
```go
logger.Info(ctx, "batch processed",
    axio.Field("items_total", len(items)),
    axio.Field("items_ok", okCount),
    axio.Field("items_failed", failedCount),
)
```

### Anti-pattern: Explosive cardinality

**Wrong:**
```go
logger.Info(ctx, "login", axio.Field("email", user.Email))
```

**Correct:**
```go
logger.Info(ctx, "login", axio.Field("user_id", user.ID))
```

### Anti-pattern: Vague message

**Wrong:**
```go
logger.Error(ctx, err, "error")
```

**Correct:**
```go
logger.Error(ctx, err, "failed to confirm payment",
    axio.Field("order_id", order.ID),
)
```

---

## Troubleshooting

### Error Table

| Error                    | Cause                                | Solution                                     |
| ------------------------ | ------------------------------------ | -------------------------------------------- |
| `ErrInvalidEnvironment`  | Invalid Environment value            | Use `production`, `staging`, or `development`|
| `ErrInvalidLevel`        | Invalid Level value                  | Use `debug`, `info`, `warn`, or `error`      |
| `ErrInvalidFormat`       | Invalid Format value                 | Use `json` or `text`                         |
| `ErrInvalidOutputType`   | Invalid OutputType value             | Use `console`, `stdout`, or `file`           |
| `ErrIncompatibleOutputs` | AgentMode with non-stdout/json output| In AgentMode, use only stdout + json         |
| `ErrFileOutputNoPath`    | File output type without path        | Specify `path` in OutputConfig               |
| `ErrAuditWithoutPath`    | Audit enabled without storePath      | Specify `storePath` in AuditConfig           |
| `ErrAuditWithoutJSON`    | Audited Logger with only text outputs| Add a JSON output; only JSON lines verify    |
| `ErrInvalidPIIMaxDepth`  | Negative PII masking depth           | Use `0` for the default, or a positive depth |
| `ErrInvalidTracer`       | Invalid TracerType value             | Use `otel` or `noop`                         |
| `ErrLoadConfig`          | Failed to read config file           | Check path and permissions                   |
| `ErrUnknownFormat`       | Unknown file extension               | Use `.yaml`, `.yml`, `.json`, or `.toml`     |
| `ErrUnmarshalConfig`     | Failed to parse config               | Check file syntax                            |
| `ErrApplyOption`         | Failed to apply Option               | Check Option parameters                      |
| `ErrValidateConfig`      | Invalid configuration after Options  | Check value combination                      |
| `ErrBuildOutputs`        | Failed to create outputs             | Check file paths                             |
| `ErrBuildHooks`          | Failed to create hooks               | Check PIICustomPatterns regex                |
| `ErrBuildMetrics`        | Failed to build metrics              | Check MeterProvider configuration            |
| `ErrBuildAudit`          | Failed to build audit chain          | Check the store path and its permissions     |
| `ErrBuildEngine`         | Failed to build logging engine       | Check output and config combination          |
| `ErrOpenFile`            | Failed to open log file              | Check path and permissions                   |
| `ErrOutputClosed`        | File output closed a second time     | Close each output once                       |
| `ErrLoadChainState`      | Failed to load chain state           | Check chain file                             |
| `ErrSaveChainState`      | Failed to save chain state           | Check write permissions                      |
| `ErrMarshalChainState`   | Failed to marshal chain state        | Internal serialization error                 |
| `ErrUnmarshalChainState` | Failed to unmarshal chain state      | Chain file corrupted or invalid format       |
| `ErrHashMismatch`        | A line's hash doesn't match it       | The line changed after it was written        |
| `ErrChainBroken`         | Chain integrity compromised          | A line was removed, moved or inserted        |
| `ErrChainIncomplete`     | Log ends before the chain            | End removed, or the whole chain rewritten    |
| `ErrNilAuditChain`       | Chain passed to WithAuditChain is nil| Pass a chain from `NewHashChain`             |
| `ErrChainStoreLocked`    | Another process holds the chain store| One process per store; give each its own path |
| `ErrNilMetricsProvider`  | Metrics provider is nil              | Pass a valid MeterProvider                   |
| `ErrCreateMetric`        | Failed to create OTel instrument     | Check provider configuration                 |
| `ErrNilTracer`           | Tracer passed to WithTracer is nil   | Pass a non-nil Tracer or omit the option     |
| `ErrLoggerClosed`        | Logger has already been closed       | Idempotent guard; check with `errors.Is`     |
| `ErrLoggerNotRoot`       | Close called on a forked Logger      | Only the root from `New` can be closed       |
| `ErrEventClosed`         | Event has already been closed        | Idempotent guard; check with `errors.Is`     |
