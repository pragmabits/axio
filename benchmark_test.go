package axio

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// benchOutput implements Output backed by a no-op writer, isolating
// CPU/alloc measurements from I/O.
type benchOutput struct{}

func (benchOutput) Write(data []byte) (int, error) { return len(data), nil }
func (benchOutput) Sync() error                    { return nil }
func (benchOutput) Close() error                   { return nil }
func (benchOutput) Type() OutputType               { return OutputStdout }
func (benchOutput) Format() Format                 { return FormatJSON }

// benchLogger creates a logger writing JSON to a no-op output at debug level.
func benchLogger(b *testing.B) *logger {
	b.Helper()

	out := benchOutput{}
	encoder := buildEncoder(FormatJSON)
	core := zapcore.NewCore(encoder, out, zap.NewAtomicLevelAt(zap.DebugLevel))
	engine := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(minimumCallerSkip))

	return &logger{
		engine:  engine,
		trace:   NoopTracer{},
		hooks:   newHookChain(NoopMetrics{}),
		metrics: NoopMetrics{},
		outputs: []Output{out},
		closed:  new(atomic.Bool),
	}
}

// benchEntry creates an Entry suitable for benchmarking.
func benchEntry(traceID, spanID string, err error, annotations ...Annotation) *Entry {
	return &Entry{
		Timestamp:   time.Now(),
		Level:       LevelInfo,
		Message:     "benchmark message",
		Error:       err,
		Logger:      "bench",
		Caller:      "benchmark_test.go:50",
		TraceID:     traceID,
		SpanID:      spanID,
		Annotations: annotations,
	}
}

// ---------------------------------------------------------------------------
// End-to-end logging benchmarks
// ---------------------------------------------------------------------------

func BenchmarkLogger_Info(b *testing.B) {
	loggerUnderTest := benchLogger(b)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loggerUnderTest.Info(ctx, "simple log message")
	}
}

func BenchmarkLogger_Info_WithAnnotations(b *testing.B) {
	loggerUnderTest := benchLogger(b)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loggerUnderTest.With(
			Annotate("user_id", "usr_12345"),
			Annotate("tenant", "acme-corp"),
		).Info(ctx, "annotated message")
	}
}

func BenchmarkLogger_Info_WithHTTP(b *testing.B) {
	loggerUnderTest := benchLogger(b)
	ctx := context.Background()
	httpData := HTTP{
		Method:     "POST",
		URL:        "/api/v1/orders",
		StatusCode: 201,
		LatencyMS:  45,
		UserAgent:  "Mozilla/5.0",
		ClientIP:   "192.168.1.100",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loggerUnderTest.With(Annotate("http", httpData)).Info(ctx, "request processed")
	}
}

func BenchmarkLogger_Error_WithError(b *testing.B) {
	loggerUnderTest := benchLogger(b)
	ctx := context.Background()
	err := errors.New("connection refused")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loggerUnderTest.Error(ctx, err, "database connection failed")
	}
}

func BenchmarkLogger_Info_Formatted(b *testing.B) {
	loggerUnderTest := benchLogger(b)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loggerUnderTest.Info(ctx, "processed %d items in %dms", 42, 150)
	}
}

// ---------------------------------------------------------------------------
// End-to-end logging benchmarks with PII masking
// ---------------------------------------------------------------------------

// benchCustomer is a struct annotation carrying a sensitive field and a CPF.
type benchCustomer struct {
	Name     string `json:"name"`
	Document string `json:"document"`
	Password string `json:"password"`
}

// benchPIILogger is benchLogger with the PII hook [WithPII] installs.
func benchPIILogger(b *testing.B) *logger {
	b.Helper()
	loggerUnderTest := benchLogger(b)
	loggerUnderTest.hooks = newHookChain(NoopMetrics{}, MustPIIHook(DefaultPIIConfig()))
	return loggerUnderTest
}

func BenchmarkLogger_PII_Strings(b *testing.B) {
	loggerUnderTest := benchPIILogger(b)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loggerUnderTest.With(
			Annotate("user_id", "usr_12345"),
			Annotate("document", "123.456.789-01"),
		).Info(ctx, "customer registered")
	}
}

func BenchmarkLogger_PII_Words(b *testing.B) {
	loggerUnderTest := benchPIILogger(b)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loggerUnderTest.With(
			Annotate("status", "done"),
			Annotate("tenant", "acme"),
			Annotate("plan", "premium1"),
			Annotate("region", "saopaulo"),
		).Info(ctx, "order settled")
	}
}

func BenchmarkLogger_PII_Map(b *testing.B) {
	loggerUnderTest := benchPIILogger(b)
	ctx := context.Background()
	customer := map[string]any{"name": "alice", "document": "123.456.789-01", "password": "hunter2"}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loggerUnderTest.With(Annotate("customer", customer)).Info(ctx, "customer registered")
	}
}

func BenchmarkLogger_PII_Struct(b *testing.B) {
	loggerUnderTest := benchPIILogger(b)
	ctx := context.Background()
	customer := benchCustomer{Name: "alice", Document: "123.456.789-01", Password: "hunter2"}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loggerUnderTest.With(Annotate("customer", customer)).Info(ctx, "customer registered")
	}
}

func BenchmarkLogger_PII_Bytes(b *testing.B) {
	loggerUnderTest := benchPIILogger(b)
	ctx := context.Background()
	body := []byte(`{"name":"alice","document":"123.456.789-01"}`)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loggerUnderTest.With(Annotate("body", body)).Info(ctx, "request received")
	}
}

func BenchmarkLogger_PII_StructWithBytes(b *testing.B) {
	loggerUnderTest := benchPIILogger(b)
	ctx := context.Background()
	request := struct {
		Route string `json:"route"`
		Body  []byte `json:"body"`
	}{Route: "/api/v1/customers", Body: []byte(`{"name":"alice","document":"123.456.789-01"}`)}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loggerUnderTest.With(Annotate("request", request)).Info(ctx, "request received")
	}
}

func BenchmarkLogger_PII_HTTP(b *testing.B) {
	loggerUnderTest := benchPIILogger(b)
	ctx := context.Background()
	httpData := HTTP{
		Method:     "GET",
		URL:        "/api/v1/customers?cpf=123.456.789-01",
		StatusCode: 200,
		LatencyMS:  45,
		UserAgent:  "Mozilla/5.0",
		ClientIP:   "192.168.1.100",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loggerUnderTest.With(Annotate("http", httpData)).Info(ctx, "request processed")
	}
}

func BenchmarkLogger_PII_Error(b *testing.B) {
	loggerUnderTest := benchPIILogger(b)
	ctx := context.Background()
	err := errors.New("customer 123.456.789-01 rejected")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loggerUnderTest.Error(ctx, err, "registration failed")
	}
}

// ---------------------------------------------------------------------------
// Component benchmarks: fieldsFromEntry
// ---------------------------------------------------------------------------

func BenchmarkFieldsFromEntry_Minimal(b *testing.B) {
	loggerUnderTest := benchLogger(b)
	entry := benchEntry("", "", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loggerUnderTest.fieldsFromEntry(entry)
	}
}

func BenchmarkFieldsFromEntry_Full(b *testing.B) {
	loggerUnderTest := benchLogger(b)
	entry := benchEntry(
		"abc123def456789012345678901234aa",
		"span1234567890ab",
		errors.New("timeout"),
		Annotate("request_id", "req-001"),
		Annotate("user_id", "usr-999"),
		Annotate("region", "us-east-1"),
	)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loggerUnderTest.fieldsFromEntry(entry)
	}
}

// ---------------------------------------------------------------------------
// Component benchmarks: annotationsToFields
// ---------------------------------------------------------------------------

func BenchmarkAnnotationsToFields(b *testing.B) {
	annotations := Annotations{
		Annotate("user_id", "usr_12345"),
		Annotate("tenant", "acme-corp"),
		Annotate("action", "create_order"),
		Annotate("region", "us-east-1"),
		Annotate("version", "2.1.0"),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		annotationsToFields(annotations)
	}
}

// ---------------------------------------------------------------------------
// Component benchmarks: hookChain
// ---------------------------------------------------------------------------

func BenchmarkHookChain_Process_NoHooks(b *testing.B) {
	chain := newHookChain(NoopMetrics{})
	ctx := context.Background()
	entry := benchEntry("", "", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = chain.process(ctx, entry)
	}
}

func BenchmarkHookChain_Process_ThreeHooks(b *testing.B) {
	chain := newHookChain(NoopMetrics{}, NoopHook(), NoopHook(), NoopHook())
	ctx := context.Background()
	entry := benchEntry("", "", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = chain.process(ctx, entry)
	}
}

// ---------------------------------------------------------------------------
// Component benchmarks: PIIMasker
// ---------------------------------------------------------------------------

func BenchmarkPIIMasker_MaskString(b *testing.B) {
	masker := MustPIIMasker(PIIConfig{
		Patterns: []PIIPattern{PatternCPF, PatternCNPJ, PatternCreditCard},
	})
	input := "Customer CPF 123.456.789-01 registered"

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		masker.MaskString(input)
	}
}

func BenchmarkPIIMasker_NoMatch(b *testing.B) {
	masker := MustPIIMasker(PIIConfig{
		Patterns: []PIIPattern{PatternCPF, PatternCNPJ, PatternCreditCard},
	})
	input := "This is a clean log message with no PII data whatsoever"

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		masker.MaskString(input)
	}
}

func BenchmarkPIIMasker_MaskFields_NoMap(b *testing.B) {
	masker := MustPIIMasker(DefaultPIIConfig())

	annotations := Annotations{
		Annotate("user_id", "usr_12345"),
		Annotate("tenant", "acme-corp"),
		Annotate("route", "/api/v1/orders"),
		Annotate("method", "POST"),
		Annotate("status", "ok"),
		Annotate("region", "us-east-1"),
		Annotate("service", "checkout"),
		Annotate("version", "2.1.0"),
		Annotate("environment", "production"),
		Annotate("correlation_id", "corr_98765"),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		masker.MaskFields(annotations)
	}
}

func BenchmarkPIIMasker_MaskFields_ShallowMap(b *testing.B) {
	masker := MustPIIMasker(DefaultPIIConfig())

	annotations := Annotations{
		Annotate("context", map[string]any{
			"user_id":     "usr_12345",
			"tenant":      "acme-corp",
			"route":       "/api/v1/orders",
			"method":      "POST",
			"status":      "ok",
			"region":      "us-east-1",
			"service":     "checkout",
			"correlation": "corr_98765",
			"password":    "hunter2",
			"api_key":     "ak_secret",
		}),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		masker.MaskFields(annotations)
	}
}

func BenchmarkPIIMasker_MaskFields_DeepMap_AtCap(b *testing.B) {
	masker := MustPIIMasker(DefaultPIIConfig())

	annotations := Annotations{
		Annotate("payload", map[string]any{
			"password": "outer-secret",
			"profile": map[string]any{
				"name":     "alice",
				"password": "inner-secret",
			},
		}),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		masker.MaskFields(annotations)
	}
}

// ---------------------------------------------------------------------------
// Component benchmarks: NoopTracer
// ---------------------------------------------------------------------------

func BenchmarkNoopTracer_Extract(b *testing.B) {
	tracer := NoopTracer{}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		tracer.Extract(ctx)
	}
}

// ---------------------------------------------------------------------------
// Component benchmarks: formatMessage
// ---------------------------------------------------------------------------

func BenchmarkFormatMessage_NoArgs(b *testing.B) {
	loggerUnderTest := benchLogger(b)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loggerUnderTest.formatMessage("simple message without formatting")
	}
}

func BenchmarkFormatMessage_WithArgs(b *testing.B) {
	loggerUnderTest := benchLogger(b)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loggerUnderTest.formatMessage("processed %d items in %s for user %s", 42, "150ms", "usr_123")
	}
}

// Ensure benchOutput satisfies Output at compile time.
var _ Output = benchOutput{}

// Prevent compiler from optimizing away results.
var benchSink any

func init() {
	_ = benchSink
	_ = fmt.Sprint // keep fmt import for formatMessage benchmarks
}
