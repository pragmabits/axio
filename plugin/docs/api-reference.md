# Axio API Reference

> This is a bundled reference for use when axio source code is unavailable. For the most current API, read the source files directly.

## Core Types (axio.go)

### Environment

```go
type Environment string

const (
    EnvironmentProduction  Environment = "production"
    EnvironmentStaging     Environment = "staging"
    EnvironmentDevelopment Environment = "development"
)

func (e Environment) Validate() error
func (e *Environment) UnmarshalText(text []byte) error
```

### Level

```go
type Level string

const (
    LevelDebug Level = "debug"
    LevelInfo  Level = "info"
    LevelWarn  Level = "warn"
    LevelError Level = "error"
)

func (l Level) Validate() error
func (l *Level) UnmarshalText(text []byte) error
```

### Format

```go
type Format string

const (
    FormatJSON Format = "json"
    FormatText Format = "text"
)

func (f Format) Validate() error
func (f *Format) UnmarshalText(text []byte) error
```

### Logger Interface

```go
type Logger interface {
    Named(string) Logger
    Debug(context.Context, string, ...Annotation)
    Info(context.Context, string, ...Annotation)
    Warn(context.Context, error, string, ...Annotation)   // error is 2nd param
    Error(context.Context, error, string, ...Annotation)  // error is 2nd param
    With(...Annotation) Logger
    Close() error
}
```

The message is written as given, never formatted. The annotations passed to a
level method go on that entry only, after those the logger carries from `With`.

## Logger Creation (logger.go)

```go
func New(config Config, options ...Option) (Logger, error)
```

Precedence: Config -> Options -> Defaults -> Validate

## Configuration (config.go)

### Config Struct

```go
type Config struct {
    ServiceName      string
    ServiceVersion   string
    Environment      Environment
    InstanceID       string
    Level            Level
    CallerSkip       int
    OmitCaller       bool        // write lines without the caller
    Outputs          []OutputConfig
    AgentMode        bool
    PIIEnabled       bool
    PIIPatterns      []PIIPattern
    PIICustomPatterns []CustomPII
    PIIFields        []string
    PIIMaxDepth      int         // 0 means DefaultPIIMaxDepth (32)
    PIIOmitErrorVerbose bool     // omit, instead of mask, the %+v of errors
    Audit            AuditConfig
    TracerType       string      // "otel" or "noop"
    Metrics          MetricsConfig
}
```

### MetricsConfig

```go
type MetricsConfig struct {
    Enabled      bool
    MeterName    string   // default: "axio"
    MeterVersion string   // default: "1.0.0"
}
```

### Config Functions

```go
func DefaultConfig() Config
func LoadConfig(path string) (Config, error)
func LoadConfigFrom(reader io.Reader, format string) (Config, error)
func MustLoadConfig(path string) Config
func (c *Config) Validate() error
```

Supported file formats: .json, .yaml/.yml, .toml

## Functional Options (options.go)

```go
type Option func(*Config) error

func WithOutputs(outputs ...Output) Option
func WithAgentMode() Option
func WithOmitCaller() Option
func WithHooks(hooks ...Hook) Option
func WithPII(patterns []PIIPattern, fields []string) Option
func WithPIIMaxDepth(depth int) Option
func WithPIIOmitErrorVerbose() Option
func WithAudit(storePath string) Option
func WithAuditChain(chain *HashChain) Option
func WithMetrics(provider metric.MeterProvider) Option
func WithTracer(tracer Tracer) Option
```

## Outputs (output.go)

### OutputType

```go
type OutputType string

const (
    OutputConsole OutputType = "console"   // stderr
    OutputStdout  OutputType = "stdout"    // stdout
    OutputFile    OutputType = "file"      // local file
)

func (o OutputType) Validate() error
func (o *OutputType) UnmarshalText(text []byte) error
```

### Output Interface

```go
type Output interface {
    Format() Format
    Type() OutputType
    Write([]byte) (int, error)
    Sync() error
    Close() error
}
```

### Output Constructors

```go
func Console(format Format) Output
func Stdout(format Format) Output
func File(path string, format Format) (Output, error)
func MustFile(path string, format Format) Output
func RotatingFile(path string, format Format, rotation RotationConfig) (Output, error)
func MustRotatingFile(path string, format Format, rotation RotationConfig) Output
```

### RotationConfig

```go
type RotationConfig struct {
    MaxSize    int       // MB before rotation; 0 = no size rotation
    MaxAge     int       // days to retain
    MaxBackups int       // old files to retain
    Compress   bool      // gzip rotated files
    LocalTime  bool      // use local time in backup names
    Interval   Duration  // time between rotations
}

func (r RotationConfig) Enabled() bool
```

### OutputConfig

```go
type OutputConfig struct {
    Type     OutputType
    Format   Format
    Path     string         // required for file type
    Rotation RotationConfig // only for file type
}
```

## Annotations (annotation.go)

```go
type Annotation struct { /* internal */ }

func Field[T any](key string, value T) Annotation
func (a Annotation) Name() string
func (a Annotation) Data() any              // int64, uint64, float64, or the value as given
func (a Annotation) Value[T any]() (T, bool) // a number converts to any type of its kind that holds it

type Annotations []Annotation

func (a Annotations) Names() []string
func (a Annotations) Data() []any
func (a *Annotations) Add[T any](key string, value T) Annotations
```

A struct, a map, or a slice zap has no encoder of its own for is written as its
`encoding/json/v2` encoding: nil slices and maps as `null`, map keys sorted, a
`time.Duration` in nanoseconds, a byte array as base64. A slice of a basic type
given as the annotation's own value (`[]string`, `[]int`, `[]bool`,
`[]time.Duration`, `[]time.Time`, `[]error`, …) is written by zap instead: nil
as `[]`, durations in milliseconds. `Data` and `Value` return it as the slice
given. `omitempty` omits a value that encodes as empty (`""`,
`null`, `[]`, `{}`); `omitzero` omits `false`, `0` and every other zero value. A
tag option the encoding rejects, such as `,string` on a slice, makes the value
fail, and the line carries `<key>Error` instead.

### Annotable Interface

```go
type Annotable interface {
    Append([]Annotation) []Annotation
}
```

A logger expands `Annotable` values into their fields before any hook runs,
so PII masking and custom hooks see each field on its own.

An annotation named like a key axio writes itself (`message`, `level`,
`error`, `hash`, `service`, …) is written behind an underscore, `_message`,
so a line never carries the same key twice.

### HTTP Annotation

```go
type HTTP struct {
    Method     string
    URL        string
    StatusCode int
    LatencyMS  int64
    UserAgent  string
    ClientIP   string
}

// Implements Annotable
func (h HTTP) Append(target []Annotation) []Annotation
```

## PII Masking (pii.go)

### PIIPattern

```go
type PIIPattern string

const (
    PatternCPF        PIIPattern = "cpf"
    PatternCNPJ       PIIPattern = "cnpj"
    PatternCreditCard PIIPattern = "credit_card"
    PatternEmail      PIIPattern = "email"
    PatternPhone      PIIPattern = "phone"
    PatternPhoneNoDDD PIIPattern = "phone_no_ddd"
)
```

### CustomPII

```go
type CustomPII struct {
    Name    string
    Pattern string  // regex
    Mask    string
}
```

### PIIConfig and PIIMasker

```go
type PIIConfig struct {
    Patterns       []PIIPattern
    CustomPatterns []CustomPII
    Fields         []string
    MaxDepth       int  // 0 means DefaultPIIMaxDepth (32)
    OmitErrorVerbose bool // omit, instead of mask, the %+v of errors
}

func DefaultPIIConfig() PIIConfig

func DefaultSensitiveFields() []string  // returns a fresh copy: password, token, api_key, secret, etc.

type PIIMasker struct { /* internal */ }

func NewPIIMasker(config PIIConfig) (*PIIMasker, error)
func MustPIIMasker(config PIIConfig) *PIIMasker
func (m *PIIMasker) MaskString(input string) string
func (m *PIIMasker) MaskStringWithCounts(input string) PIIMaskResult
func (m *PIIMasker) MaskFields(fields Annotations)
func (m *PIIMasker) MaskFieldsWithCounts(fields Annotations) map[PIIPattern]int

type PIIMaskResult struct {
    Masked  string
    Matches map[PIIPattern]int
}
```

Masking covers every value the caller hands a line: the message; the entry's error,
by its message and its verbose form (a masked error still unwraps to the
original); the error of a value that fails to encode; strings,
errors and `fmt.Stringer` annotations, by their text; a `[]byte`, a byte array
or a named byte-slice type, by the text it holds, bytes that are not UTF-8 text
becoming `[REDACTED]`, inside a structured value too; and structured values — maps, slices, structs, pointers,
`http.Header` — walked as the JSON encoding the log writes for them, keys
checked against `Fields` and strings against the patterns at every level. Any
string with the shape of base64 — standard or URL alphabet, padded or not: the
message, the error, an annotation, a nested value, and bytes of text inside a
structured value, as its JSON encoding carries them — is also decoded and masked when the text it decodes to
carries PII; one that decodes to binary data becomes `[REDACTED]` when a
pattern matches inside it, and passes otherwise. A JWT or JWE anywhere in a text
becomes `[REDACTED]` whole, and so does a JWS or JWE in JSON serialization (an
object with `payload` and `signature` or `signatures`, or with `ciphertext` and
`iv`) inside a structured value. A byte array or a named byte-slice type is
looked for only in a value whose type may hold one or an interface, at one more
allocation per field or element. `MaskString` and `MaskStringWithCounts` cover a
string the same way. A structured value that needed masking is written as its
masked JSON tree, object keys in alphabetical order; one with nothing to mask
keeps its original form. A container nested deeper than `MaxDepth` becomes
`[REDACTED]` whole.

### PIIHook

```go
type PIIHook struct { /* internal */ }

func NewPIIHook(config PIIConfig) (*PIIHook, error)
func MustPIIHook(config PIIConfig) *PIIHook
func (h *PIIHook) Name() string
func (h *PIIHook) Process(ctx context.Context, entry *Entry) error
func (h *PIIHook) SetMetrics(metrics Metrics)  // implements MetricsAware
```

## Hooks (hook.go)

### Entry

```go
type Entry struct {
    Timestamp    time.Time
    Level        Level
    Message      string
    Error        error
    Logger       string
    Caller       string
    TraceID      string
    SpanID       string
    Annotations  Annotations
}
```

A hook changes what is written through `Message`, `Error`, `TraceID`, `SpanID` and `Annotations`. `Timestamp`, `Level`, `Logger` and `Caller` are read only. `Caller` is in the form the line writes (`checkout/handler.go:42`), and empty with `WithOmitCaller` and for an `Event`.

### Hook Interface

```go
type Hook interface {
    Name() string
    Process(ctx context.Context, entry *Entry) error
}
```

### MetricsAware Interface

```go
type MetricsAware interface {
    SetMetrics(metrics Metrics)
}
```

### NoopHook

```go
func NoopHook() Hook
```

The chain runs in a fixed order: the `PIIHook` that `WithPII` or `piiEnabled`
turns on, then the hooks passed to `WithHooks`, in the order passed, so custom
hooks observe the already-masked entry. A `PIIHook` passed to `WithHooks` is one
of those hooks and runs where it was passed.
Auditing is not a hook: the hash is computed when the entry is written,
after every hook, so it covers what the hooks changed. The chain itself is
an unexported implementation detail.

## Audit (audit.go)

### AuditConfig

```go
type AuditConfig struct {
    Enabled   bool
    StorePath string  // required when Enabled, unless WithAuditChain is used
}
```

### ChainStore Interface

```go
type ChainStore interface {
    Save(sequence uint64, lastHash string) error
    Load() (sequence uint64, lastHash string, err error)
}
```

### FileStore

```go
type FileStore struct { /* internal */ }

func NewFileStore(path string) *FileStore
func (s *FileStore) Save(sequence uint64, lastHash string) error
func (s *FileStore) Load() (uint64, string, error)
```

`New` and `NewEvent` take an exclusive `flock` on `path + ".lock"` — for the
`WithAudit` path, and for a `FileStore` passed through `WithAuditChain`, whose
chain is then loaded from the store again — and hold it while the process runs;
a second process on the same store fails in `New` with `ErrChainStoreLocked`
(wrapped in `ErrBuildAudit`). A `FileStore` used on its own takes the lock at its
first `Save`.
`Load` takes no lock. Windows, Solaris and AIX have no lock.

### HashChain

```go
type HashChain struct { /* internal */ }

func NewHashChain(store ChainStore) (*HashChain, error)
func (c *HashChain) Add(data []byte) (hash, previousHash string, err error)
func (c *HashChain) Verify(reader io.Reader, previousHash string) error
func (c *HashChain) Sequence() uint64
func (c *HashChain) LastHash() string

func VerifyLines(reader io.Reader, previousHash string) (lastHash string, err error)
```

`Add` hashes `sha256(previousHash ‖ data)`. An audited Logger or Event writes
every JSON line with `previous_hash` and `hash` as its last two keys, hashing
exactly the bytes before them; hashing, saving the state and writing happen
under the chain's lock, so the file's order is the chain's order. A Logger's
text outputs show the first 6 characters of the hash, for reference only; an
audited Event writes its JSON line to every output.

`Verify` reads audited JSON lines and returns `ErrHashMismatch` (a line
changed), `ErrChainBroken` (a line removed, moved, inserted or without
trailer) or `ErrChainIncomplete` (the log ends before `LastHash`). Pass `""`
as `previousHash` for a log that starts the chain. `VerifyLines` checks lines
without the end-of-chain check and returns the last line's hash, for
verifying rotated files one at a time. From the terminal: `axio verify
--store chain.json [file...]`.

`WithAudit(path)` shares one chain per path across every Logger and Event in
the process. `WithAuditChain(chain)` takes a chain over any `ChainStore`. An
audited Logger needs a JSON output (`ErrAuditWithoutJSON`); an Event writes
JSON to every output.

## Tracing (tracing.go)

```go
type Tracer interface {
    Extract(context.Context) (traceID string, spanID string, ok bool)
}

type NoopTracer struct{}

func (n NoopTracer) Extract(ctx context.Context) (string, string, bool)
func NoopTracing() Tracer
func Otel() Tracer
```

## Metrics (metrics.go)

```go
type Metrics interface {
    LogsTotal(ctx context.Context, level Level)
    PIIMasked(ctx context.Context, pattern PIIPattern, origin PIIOrigin, count int)
    PIIRedacted(ctx context.Context, reason PIIRedaction, origin PIIOrigin, count int)
    AuditRecords(ctx context.Context)
    HookDuration(ctx context.Context, hookName string, duration time.Duration, hasError bool)
}

type PIIOrigin struct {
    Annotation string // "message", "error", or an annotation's key as written
    Logger     string // the Named logger; empty for the root logger and events
}

type PIIRedaction string // declared in pii.go: why a value became [REDACTED] whole

const (
    RedactionField  PIIRedaction = "field"  // its name matches PIIConfig.Fields
    RedactionDepth  PIIRedaction = "depth"  // nested deeper than MaxDepth
    RedactionToken  PIIRedaction = "token"  // a JWT or JWE, compact or JSON
    RedactionBinary PIIRedaction = "binary" // bytes that are not text
)

type NoopMetrics struct{}
```

## Wide Events (event.go)

```go
type Event struct { /* internal */ }

func NewEvent(name string, config Config, options ...Option) (*Event, error)
func WithEvent(ctx context.Context, event *Event) context.Context
func EventFromContext(ctx context.Context) *Event

func (e *Event) Add[T any](key string, value T)
func (e *Event) With(annotations ...Annotation)
func (e *Event) SetError(err error, details ...Annotation)
func (e *Event) Emit(ctx context.Context)
func (e *Event) Close() error
```

A hook may rename the event through `Entry.Message`. An `Emit` after `Close`
writes nothing; a second `Close` returns `ErrEventClosed`.

## Duration (duration.go)

```go
type Duration time.Duration  // unmarshals from strings like "24h", "1h30m"

func (d *Duration) UnmarshalText(text []byte) error
func (d Duration) MarshalText() ([]byte, error)
```

## Sentinel Errors (errors.go)

```go
var (
    ErrInvalidEnvironment  = errors.New("invalid environment")
    ErrInvalidLevel        = errors.New("invalid level")
    ErrInvalidFormat       = errors.New("invalid format")
    ErrInvalidOutputType   = errors.New("invalid output type")
    ErrIncompatibleOutputs = errors.New("outputs incompatible with agent mode")
    ErrLoadConfig          = errors.New("failed to load configuration")
    ErrUnknownFormat       = errors.New("unknown file format")
    ErrUnmarshalConfig     = errors.New("failed to unmarshal configuration")
    ErrInvalidTracer       = errors.New("invalid tracer")
    ErrAuditWithoutPath    = errors.New("audit enabled requires storePath")
    ErrFileOutputNoPath    = errors.New("output type 'file' requires 'path'")
    ErrAuditWithoutJSON    = errors.New("audit requires a JSON output")
    ErrInvalidPIIMaxDepth  = errors.New("PII max depth cannot be negative")
)

var (
    ErrApplyOption    = errors.New("failed to apply option")
    ErrValidateConfig = errors.New("configuration validation failed")
    ErrBuildOutputs   = errors.New("failed to build outputs")
    ErrBuildHooks     = errors.New("failed to build hooks")
    ErrBuildMetrics   = errors.New("failed to build metrics")
    ErrBuildAudit     = errors.New("failed to build audit chain")
    ErrBuildEngine    = errors.New("failed to build engine")
)

var (
    ErrOpenFile     = errors.New("failed to open file")
    ErrOutputClosed = errors.New("output already closed")
)

var (
    ErrLoadChainState      = errors.New("failed to load chain state")
    ErrSaveChainState      = errors.New("failed to save chain state")
    ErrMarshalChainState   = errors.New("failed to marshal chain state")
    ErrUnmarshalChainState = errors.New("failed to unmarshal chain state")
    ErrHashMismatch        = errors.New("hash mismatch")
    ErrChainBroken         = errors.New("chain integrity compromised")
    ErrChainIncomplete     = errors.New("log does not reach the chain's last hash")
    ErrNilAuditChain       = errors.New("audit chain cannot be nil")
    ErrChainStoreLocked    = errors.New("chain store is in use by another process")
)

var (
    ErrNilMetricsProvider = errors.New("metrics provider cannot be nil")
    ErrCreateMetric       = errors.New("failed to create metric instrument")
    ErrNilTracer          = errors.New("tracer cannot be nil")
)

var (
    ErrLoggerClosed  = errors.New("logger already closed")
    ErrLoggerNotRoot = errors.New("close called on forked logger")
    ErrEventClosed   = errors.New("event already closed")
)
```
