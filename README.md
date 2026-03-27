# Axio

![English](https://img.shields.io/badge/lang-en-blue.svg)
[Português](./README.pt-BR.md) | **English**

![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/License-MIT-blue.svg)

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
- [Logging Best Practices](#logging-best-practices)
- [Guide by Service Type](#guide-by-service-type)
- [Examples and Anti-patterns](#examples-and-anti-patterns)
- [Troubleshooting](#troubleshooting)

---

## Installation

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
        Environment:    axio.Production,
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

    logger.With(
        &axio.HTTP{
            Method:     r.Method,
            URL:        r.URL.Path,
            StatusCode: 201,
            LatencyMS:  time.Since(start).Milliseconds(),
            ClientIP:   r.RemoteAddr,
        },
        axio.Annotate("user_id", "usr_123"),
    ).Info(ctx, "order created")

    w.WriteHeader(http.StatusCreated)
}
```

---

## Configuration

### Main Config

| Field               | Type             | Required    | Default                    | Values                                 | Validation                                |
| ------------------- | ---------------- | ----------- | -------------------------- | -------------------------------------- | ----------------------------------------- |
| `ServiceName`       | `string`         | No          | `""`                       | any                                    | -                                         |
| `ServiceVersion`    | `string`         | No          | `""`                       | any                                    | -                                         |
| `Environment`       | `Environment`    | No          | `development`              | `production`, `staging`, `development` | `ErrInvalidEnvironment` if invalid        |
| `InstanceID`        | `string`         | No          | `""`                       | any                                    | -                                         |
| `Level`             | `Level`          | No          | `info`                     | `debug`, `info`, `warn`, `error`       | `ErrInvalidLevel` if invalid              |
| `CallerSkip`        | `int`            | No          | `0`                        | `>= 0`                                 | -                                         |
| `DisableSample`     | `bool`           | No          | `false`                    | `true`, `false`                        | -                                         |
| `AgentMode`         | `bool`           | No          | `false`                    | `true`, `false`                        | If `true`, outputs must be stdout+json    |
| `Outputs`           | `[]OutputConfig` | No          | auto                       | see OutputConfig                       | Validated individually                    |
| `PIIEnabled`        | `bool`           | No          | `false`                    | `true`, `false`                        | -                                         |
| `PIIPatterns`       | `[]PIIPattern`   | No          | `[cpf, cnpj, credit_card]` | see PII table                          | -                                         |
| `PIIFields`         | `[]string`       | No          | `DefaultSensitiveFields`   | any                                    | -                                         |
| `PIICustomPatterns` | `[]CustomPII`    | No          | `[]`                       | see CustomPII                          | Regex must be valid                       |
| `TracerType`        | `string`         | No          | `noop`                     | `otel`, `noop`                         | `ErrInvalidTracer` if invalid             |
| `Audit`             | `AuditConfig`    | No          | disabled                   | see AuditConfig                        | -                                         |
| `Metrics`           | `MetricsConfig`  | No          | disabled                   | see MetricsConfig                      | -                                         |

### OutputConfig

| Field    | Type         | Required    | Default | Values                      | Validation                                    |
| -------- | ------------ | ----------- | ------- | --------------------------- | --------------------------------------------- |
| `Type`   | `OutputType` | Yes         | -       | `console`, `stdout`, `file` | `ErrInvalidOutputType` if invalid             |
| `Format` | `Format`     | Yes         | -       | `json`, `text`              | `ErrInvalidFormat` if invalid                 |
| `Path`   | `string`     | Conditional | `""`    | file path                   | `ErrFileOutputNoPath` if `Type=file` and empty|

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
logger.Info(ctx, "processed %d items", count)
logger.Warn(ctx, err, "timeout querying supplier")
logger.Error(ctx, err, "failed to persist order")
```

---

### Structured Annotations

#### Annotate

Adds key-value fields to the log:

```go
logger.With(
    axio.Annotate("user_id", "usr_123"),
    axio.Annotate("order_id", "ord_456"),
    axio.Annotate("amount_cents", 15000),
).Info(ctx, "order created")
```

#### HTTP

Struct for HTTP request metadata:

```go
logger.With(&axio.HTTP{
    Method:     "POST",
    URL:        "/api/v1/orders",
    StatusCode: 201,
    LatencyMS:  45,
    UserAgent:  r.UserAgent(),
    ClientIP:   r.RemoteAddr,
}).Info(ctx, "request processed")
```

| Field        | Type     | Description                    |
| ------------ | -------- | ------------------------------ |
| `Method`     | `string` | HTTP method (GET, POST, etc.)  |
| `URL`        | `string` | Request path                   |
| `StatusCode` | `int`    | Response code                  |
| `LatencyMS`  | `int64`  | Latency in milliseconds        |
| `UserAgent`  | `string` | Client User-Agent              |
| `ClientIP`   | `string` | Client IP                      |

#### Marshaler (custom)

Implement `Marshaler` for complex types:

```go
type Order struct {
    ID     string
    Items  []Item
    secret string // will not be logged
}

func (o Order) MarshalLog(a axio.Annotator) error {
    a.Add("order_id", o.ID)
    a.Add("item_count", len(o.Items))
    return nil
}

// Usage
logger.With(axio.Annotate("order", order)).Info(ctx, "order processed")
```

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
2. **AuditHook** - calculates hash chain
3. **Custom hooks** - in the order passed to `WithHooks`

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
        axio.Annotate("tenant_id", h.tenantID))
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
| CNPJ            | `PatternCNPJ`       | `12.345.678/0001-90`            | `**.***.***/**01-**`  |
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
        axio.DefaultSensitiveFields,
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
    Fields: axio.DefaultSensitiveFields,
}
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

#### Added Fields

| Field       | Description               |
| ----------- | ------------------------- |
| `hash`      | SHA256 hash of this entry |
| `prev_hash` | Hash of previous entry    |

#### Configuration

```go
// Via Options
logger, _ := axio.New(config,
    axio.WithAudit("/var/lib/axio/chain.json"),
)

// Via Hook directly
store := axio.NewFileStore("/var/lib/axio/chain.json")
hook, _ := axio.NewAuditHook(store)
logger, _ := axio.New(config, axio.WithHooks(hook))
```

#### Custom ChainStore

Implement `ChainStore` for custom backends (Redis, PostgreSQL, etc.):

```go
type ChainStore interface {
    Save(sequence uint64, lastHash string) error
    Load() (sequence uint64, lastHash string, err error)
}
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
    HookDuration(ctx context.Context, hookName string, duration time.Duration)
    HookDurationWithError(ctx context.Context, hookName string, duration time.Duration, hasError bool)
}
```

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

For critical operations, use `AuditHook` and combine with reliable storage.

### 8. Standard HTTP fields

```go
logger.With(
    &axio.HTTP{
        Method:     r.Method,
        URL:        r.URL.Path,
        StatusCode: statusCode,
        LatencyMS:  latencyMS,
        ClientIP:   r.RemoteAddr,
    },
    axio.Annotate("request_id", requestID),
    axio.Annotate("user_id", userID),
).Info(ctx, "request completed")
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
logger.With(&axio.HTTP{...}, axio.Annotate("request_id", id)).Info(ctx, "request completed")
```

### Workers and Jobs

**Goal:** Know when it started, finished, how much it processed.

| Event         | Level | Suggested fields                                             |
| ------------- | ----- | ------------------------------------------------------------ |
| Job started   | Info  | `job_name`, `job_id`                                         |
| Job completed | Info  | `+items_total`, `+items_ok`, `+items_failed`, `+duration_ms` |
| Item error    | Warn  | `+item_id`, `+error` (sampled)                               |

```go
logger.With(
    axio.Annotate("job_name", "reconcile_payments"),
    axio.Annotate("items_ok", okCount),
    axio.Annotate("items_failed", failedCount),
).Info(ctx, "job completed")
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
logger.Info(ctx, "user=%s status=%d", userID, statusCode)
```

**Correct:**
```go
logger.With(
    axio.Annotate("user_id", userID),
    axio.Annotate("status_code", statusCode),
).Info(ctx, "request completed")
```

### Anti-pattern: Payload with PII

**Wrong:**
```go
logger.Info(ctx, "payload=%+v", payload)
```

**Correct:**
```go
logger.With(
    axio.Annotate("payload_id", payload.ID),
    axio.Annotate("payload_size", len(payload.Data)),
).Info(ctx, "payload received")
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
    logger.Debug(ctx, "processing item %s", item.ID)
}
```

**Correct:**
```go
logger.With(
    axio.Annotate("items_total", len(items)),
    axio.Annotate("items_ok", okCount),
    axio.Annotate("items_failed", failedCount),
).Info(ctx, "batch processed")
```

### Anti-pattern: Explosive cardinality

**Wrong:**
```go
logger.With(axio.Annotate("email", user.Email)).Info(ctx, "login")
```

**Correct:**
```go
logger.With(axio.Annotate("user_id", user.ID)).Info(ctx, "login")
```

### Anti-pattern: Vague message

**Wrong:**
```go
logger.Error(ctx, err, "error")
```

**Correct:**
```go
logger.With(
    axio.Annotate("order_id", order.ID),
).Error(ctx, err, "failed to confirm payment")
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
| `ErrInvalidTracer`       | Invalid TracerType value             | Use `otel` or `noop`                         |
| `ErrLoadConfig`          | Failed to read config file           | Check path and permissions                   |
| `ErrUnknownFormat`       | Unknown file extension               | Use `.yaml`, `.yml`, `.json`, or `.toml`     |
| `ErrUnmarshalConfig`     | Failed to parse config               | Check file syntax                            |
| `ErrApplyOption`         | Failed to apply Option               | Check Option parameters                      |
| `ErrValidateConfig`      | Invalid configuration after Options  | Check value combination                      |
| `ErrBuildOutputs`        | Failed to create outputs             | Check file paths                             |
| `ErrBuildHooks`          | Failed to create hooks               | Check PIICustomPatterns regex                |
| `ErrBuildMetrics`        | Failed to build metrics              | Check MeterProvider configuration            |
| `ErrBuildEngine`         | Failed to build logging engine       | Check output and config combination          |
| `ErrOpenFile`            | Failed to open log file              | Check path and permissions                   |
| `ErrLoadChainState`      | Failed to load chain state           | Check chain file                             |
| `ErrSaveChainState`      | Failed to save chain state           | Check write permissions                      |
| `ErrMarshalChainState`   | Failed to marshal chain state        | Internal serialization error                 |
| `ErrUnmarshalChainState` | Failed to unmarshal chain state      | Chain file corrupted or invalid format       |
| `ErrHashMismatch`        | Calculated hash doesn't match        | Audit chain corrupted                        |
| `ErrChainBroken`         | Chain integrity compromised          | Records have been tampered with              |
| `ErrSerializeEntry`      | Failed to serialize audit entry      | Entry contains non-serializable data         |
| `ErrCreateAuditHook`     | Failed to create audit hook          | Check chain store configuration              |
| `ErrNilMetricsProvider`  | Metrics provider is nil              | Pass a valid MeterProvider                   |
| `ErrCreateMetric`        | Failed to create OTel instrument     | Check provider configuration                 |
