# Axio API Reference

> This is a bundled reference for use when axio source code is unavailable. For the most current API, read the source files directly.

## Core Types (axio.go)

### Environment

```go
type Environment string

const (
    Production  Environment = "production"
    Staging     Environment = "staging"
    Development Environment = "development"
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
    Debug(context.Context, string, ...any)
    Info(context.Context, string, ...any)
    Warn(context.Context, error, string, ...any)   // error is 2nd param
    Error(context.Context, error, string, ...any)   // error is 2nd param
    With(...Annotation) Logger
    Close() error
}
```

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
    DisableSample    bool
    Outputs          []OutputConfig
    AgentMode        bool
    PIIEnabled       bool
    PIIPatterns      []PIIPattern
    PIICustomPatterns []CustomPII
    PIIFields        []string
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
func WithHooks(hooks ...Hook) Option
func WithPII(patterns []PIIPattern, fields []string) Option
func WithAudit(storePath string) Option
func WithMetrics(provider metric.MeterProvider) Option
func WithTracer(t Tracer) Option
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
    MaxSize    int       // MB before rotation
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

### BuildOutputs

```go
func BuildOutputs(config Config) ([]Output, error)
```

## Annotations (annotation.go)

```go
type Annotation struct { /* internal */ }

func Annotate[T any](key string, value T) Annotation
func (a Annotation) Name() string
func (a Annotation) Data() any
func (a *Annotation) Set(value any)

type Annotations []Annotation

func (a Annotations) Names() []string
func (a Annotations) Data() []any
func (a *Annotations) Add(key string, value any) Annotations
```

### Annotable Interface

```go
type Annotable interface {
    Append([]Annotation) []Annotation
}
```

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
}

func DefaultPIIConfig() PIIConfig

var DefaultSensitiveFields []string  // password, token, api_key, secret, etc.

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
    Hash         string  // set by AuditHook
    PreviousHash string  // set by AuditHook
}
```

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

### HookChain

```go
type HookChain struct { /* internal */ }

func NewHookChain(metrics Metrics, hooks ...Hook) *HookChain
func (c *HookChain) Add(hook Hook)
func (c *HookChain) Process(ctx context.Context, entry *Entry) error
func (c *HookChain) Len() int

func NoopHook() Hook
```

### BuildHooks

```go
func BuildHooks(config Config) ([]Hook, error)
```

Execution order (fixed): PIIHook -> AuditHook -> Custom hooks

## Audit (audit.go)

### AuditConfig

```go
type AuditConfig struct {
    Enabled   bool
    StorePath string  // required when Enabled
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

### HashChain

```go
type HashChain struct { /* internal */ }

func NewHashChain(store ChainStore) (*HashChain, error)
func (c *HashChain) Add(data []byte) (hash, previousHash string, err error)
func (c *HashChain) Verify(entries []ChainEntry, getData func(int) []byte) error
func (c *HashChain) Sequence() uint64
func (c *HashChain) LastHash() string

type ChainEntry struct {
    Sequence     uint64
    Timestamp    time.Time
    Hash         string
    PreviousHash string
}
```

### AuditHook

```go
type AuditHook struct { /* internal */ }

func NewAuditHook(store ChainStore) (*AuditHook, error)
func MustAuditHook(store ChainStore) *AuditHook
func (h *AuditHook) Name() string
func (h *AuditHook) Process(ctx context.Context, entry *Entry) error
func (h *AuditHook) SetMetrics(metrics Metrics)  // implements MetricsAware
```

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

### BuildTracer

```go
func BuildTracer(config Config) Tracer
```

## Metrics (metrics.go)

```go
type Metrics interface {
    LogsTotal(ctx context.Context, level Level)
    PIIMasked(ctx context.Context, pattern PIIPattern)
    AuditRecords(ctx context.Context)
    HookDuration(ctx context.Context, hookName string, duration time.Duration)
    HookDurationWithError(ctx context.Context, hookName string, duration time.Duration, hasError bool)
}

type NoopMetrics struct{}
```

### BuildMetrics

```go
func BuildMetrics(config Config) (Metrics, error)
```

## Wide Events (event.go)

```go
type Event struct { /* internal */ }

func NewEvent(name string, config Config, options ...Option) (*Event, error)
func WithEvent(ctx context.Context, event *Event) context.Context
func EventFromContext(ctx context.Context) *Event

func (e *Event) Add(key string, value any)
func (e *Event) With(annotations ...Annotation)
func (e *Event) SetError(err error, details ...Annotation)
func (e *Event) Emit(ctx context.Context)
func (e *Event) Close() error
```

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
)

var (
    ErrApplyOption    = errors.New("failed to apply option")
    ErrValidateConfig = errors.New("configuration validation failed")
    ErrBuildOutputs   = errors.New("failed to build outputs")
    ErrBuildHooks     = errors.New("failed to build hooks")
    ErrBuildMetrics   = errors.New("failed to build metrics")
    ErrBuildEngine    = errors.New("failed to build engine")
)

var (
    ErrOpenFile = errors.New("failed to open file")
)

var (
    ErrLoadChainState      = errors.New("failed to load chain state")
    ErrSaveChainState      = errors.New("failed to save chain state")
    ErrMarshalChainState   = errors.New("failed to marshal chain state")
    ErrUnmarshalChainState = errors.New("failed to unmarshal chain state")
    ErrHashMismatch        = errors.New("hash mismatch")
    ErrChainBroken         = errors.New("chain integrity compromised")
    ErrSerializeEntry      = errors.New("failed to serialize audit entry")
    ErrCreateAuditHook     = errors.New("failed to create audit hook")
)

var (
    ErrNilMetricsProvider = errors.New("metrics provider cannot be nil")
    ErrCreateMetric       = errors.New("failed to create metric instrument")
)
```
