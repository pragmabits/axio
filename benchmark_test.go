package axio

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// benchOutput implements Output backed by a no-op writer, isolating
// CPU/alloc measurements from I/O.
type benchOutput struct{}

func (benchOutput) Write(p []byte) (int, error) { return len(p), nil }
func (benchOutput) Sync() error                 { return nil }
func (benchOutput) Close() error                { return nil }
func (benchOutput) Type() OutputType             { return OutputStdout }
func (benchOutput) Format() Format               { return FormatJSON }

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
		hooks:   NewHookChain(NoopMetrics{}),
		metrics: NoopMetrics{},
		outputs: []Output{out},
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
	l := benchLogger(b)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		l.Info(ctx, "simple log message")
	}
}

func BenchmarkLogger_Info_WithAnnotations(b *testing.B) {
	l := benchLogger(b)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		l.With(
			Annotate("user_id", "usr_12345"),
			Annotate("tenant", "acme-corp"),
		).Info(ctx, "annotated message")
	}
}

func BenchmarkLogger_Info_WithHTTP(b *testing.B) {
	l := benchLogger(b)
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
		l.With(Annotate("http", httpData)).Info(ctx, "request processed")
	}
}

func BenchmarkLogger_Error_WithError(b *testing.B) {
	l := benchLogger(b)
	ctx := context.Background()
	err := errors.New("connection refused")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		l.Error(ctx, err, "database connection failed")
	}
}

func BenchmarkLogger_Info_Formatted(b *testing.B) {
	l := benchLogger(b)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		l.Info(ctx, "processed %d items in %dms", 42, 150)
	}
}

// ---------------------------------------------------------------------------
// Component benchmarks: fieldsFromEntry
// ---------------------------------------------------------------------------

func BenchmarkFieldsFromEntry_Minimal(b *testing.B) {
	l := benchLogger(b)
	entry := benchEntry("", "", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		l.fieldsFromEntry(entry)
	}
}

func BenchmarkFieldsFromEntry_Full(b *testing.B) {
	l := benchLogger(b)
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
		l.fieldsFromEntry(entry)
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
// Component benchmarks: toField
// ---------------------------------------------------------------------------

func BenchmarkToField_String(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		toField("key", "some string value")
	}
}

func BenchmarkToField_Int(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		toField("key", 42)
	}
}

func BenchmarkToField_Map(b *testing.B) {
	m := map[string]any{
		"user_id": "usr_123",
		"count":   42,
		"active":  true,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		toField("key", m)
	}
}

// ---------------------------------------------------------------------------
// Component benchmarks: HookChain
// ---------------------------------------------------------------------------

func BenchmarkHookChain_Process_NoHooks(b *testing.B) {
	chain := NewHookChain(NoopMetrics{})
	ctx := context.Background()
	entry := benchEntry("", "", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = chain.Process(ctx, entry)
	}
}

func BenchmarkHookChain_Process_ThreeHooks(b *testing.B) {
	chain := NewHookChain(NoopMetrics{}, NoopHook(), NoopHook(), NoopHook())
	ctx := context.Background()
	entry := benchEntry("", "", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = chain.Process(ctx, entry)
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
// Component benchmarks: encodeRFC3339NanoUTC
// ---------------------------------------------------------------------------

// arrayEncoder is a minimal PrimitiveArrayEncoder for benchmarking.
type arrayEncoder struct {
	buf string
}

func (a *arrayEncoder) AppendBool(bool)             {}
func (a *arrayEncoder) AppendByteString([]byte)     {}
func (a *arrayEncoder) AppendComplex128(complex128) {}
func (a *arrayEncoder) AppendComplex64(complex64)   {}
func (a *arrayEncoder) AppendFloat64(float64)       {}
func (a *arrayEncoder) AppendFloat32(float32)       {}
func (a *arrayEncoder) AppendInt(int)               {}
func (a *arrayEncoder) AppendInt64(int64)           {}
func (a *arrayEncoder) AppendInt32(int32)           {}
func (a *arrayEncoder) AppendInt16(int16)           {}
func (a *arrayEncoder) AppendInt8(int8)             {}
func (a *arrayEncoder) AppendString(s string)       { a.buf = s }
func (a *arrayEncoder) AppendUint(uint)             {}
func (a *arrayEncoder) AppendUint64(uint64)         {}
func (a *arrayEncoder) AppendUint32(uint32)         {}
func (a *arrayEncoder) AppendUint16(uint16)         {}
func (a *arrayEncoder) AppendUint8(uint8)           {}
func (a *arrayEncoder) AppendUintptr(uintptr)       {}

func BenchmarkEncodeRFC3339NanoUTC(b *testing.B) {
	t := time.Date(2025, 3, 26, 12, 30, 45, 123456789, time.UTC)
	enc := &arrayEncoder{}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		encodeRFC3339NanoUTC(t, enc)
	}
}

// ---------------------------------------------------------------------------
// Component benchmarks: formatMessage
// ---------------------------------------------------------------------------

func BenchmarkFormatMessage_NoArgs(b *testing.B) {
	l := benchLogger(b)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		l.formatMessage("simple message without formatting")
	}
}

func BenchmarkFormatMessage_WithArgs(b *testing.B) {
	l := benchLogger(b)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		l.formatMessage("processed %d items in %s for user %s", 42, "150ms", "usr_123")
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
