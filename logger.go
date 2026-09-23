package axio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/pragmabits/axio/internal/logline"
)

const minimumCallerSkip = 2

var entryPool = sync.Pool{
	New: func() any { return &Entry{} },
}

type logger struct {
	engine      *zap.Logger
	trace       Tracer
	hooks       *hookChain
	metrics     Metrics
	annotations []Annotation
	outputs     []Output
	closed      *atomic.Bool
	isFork      bool
	audited     bool
}

// New creates a new [Logger] with the specified configuration and options.
//
// The application order is:
//  1. Initial Config (loaded from file or created programmatically)
//  2. Options are applied (override Config values)
//  3. Defaults are applied (fill empty fields)
//  4. Final validation
//
// If no output is specified, the default behavior is:
//   - Development environment: Console with [FormatText] (colored)
//   - Other environments: Stdout with [FormatJSON] (structured)
//
// The function returns [ErrApplyOption] if any option fails to be applied
// or [ErrValidateConfig] if the resulting configuration is invalid — an
// audited Logger with no JSON output included, wrapping [ErrAuditWithoutJSON],
// since only JSON lines can be verified.
//
// Basic example:
//
//	config := axio.Config{
//	    ServiceName:    "sales-api",
//	    ServiceVersion: "2.1.0",
//	    Environment:    axio.EnvironmentProduction,
//	    Level:          axio.LevelInfo,
//	}
//	logger, err := axio.New(config)
//	if err != nil {
//	    return fmt.Errorf("failed to create logger: %w", err)
//	}
//
// Example with file and options:
//
//	config, _ := axio.LoadConfig("config.yaml")
//	logger, err := axio.New(config,
//	    axio.WithOutputs(axio.Stdout(axio.FormatJSON)),
//	    axio.WithTracer(axio.Otel()),
//	)
func New(config Config, options ...Option) (Logger, error) {
	for _, option := range options {
		if err := option(&config); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrApplyOption, err)
		}
	}

	applyDefaults(&config)

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrValidateConfig, err)
	}
	if err := config.validateAuditOutputs(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrValidateConfig, err)
	}

	outputs, err := buildOutputs(config)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildOutputs, err)
	}

	metrics, err := buildMetrics(config)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildMetrics, err)
	}

	hooks, err := buildHooks(config)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildHooks, err)
	}

	chain, err := buildAuditChain(config)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildAudit, err)
	}

	engine, err := buildEngine(config, outputs, chain, metrics)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildEngine, err)
	}

	return &logger{
		engine:  engine,
		trace:   buildTracer(config),
		hooks:   newHookChain(metrics, hooks...),
		metrics: metrics,
		outputs: outputs,
		closed:  new(atomic.Bool),
		audited: chain != nil,
	}, nil
}

func (l logger) Named(name string) Logger {
	l.engine = l.engine.Named(name)
	l.annotations = cloneAnnotations(l.annotations)
	l.isFork = true
	return &l
}

func (l logger) With(annotations ...Annotation) Logger {
	combined := make([]Annotation, 0, len(l.annotations)+len(annotations))
	combined = append(combined, l.annotations...)
	combined = append(combined, annotations...)
	l.annotations = combined
	l.isFork = true
	return &l
}

func (l logger) Debug(ctx context.Context, message string, annotations ...Annotation) {
	l.log(ctx, LevelDebug, nil, message, annotations)
}

func (l logger) Info(ctx context.Context, message string, annotations ...Annotation) {
	l.log(ctx, LevelInfo, nil, message, annotations)
}

func (l logger) Warn(ctx context.Context, err error, message string, annotations ...Annotation) {
	l.log(ctx, LevelWarn, err, message, annotations)
}

func (l logger) Error(ctx context.Context, err error, message string, annotations ...Annotation) {
	l.log(ctx, LevelError, err, message, annotations)
}

// Close releases all resources associated with the logger.
//
// This includes closing files opened by [File] outputs.
// It should be called when the logger is no longer needed,
// typically with defer in main.
//
// Only the root logger (the one returned by [New]) owns its outputs and
// engine. Loggers produced by [Logger.Named] and [Logger.With] share those
// resources with the root, so calling Close on them returns [ErrLoggerNotRoot]
// and leaves all resources untouched.
//
// After Close is called on the root, subsequent log calls become no-ops on
// the root and all of its forks, and a second call to Close returns
// [ErrLoggerClosed].
//
// Example:
//
//	logger, err := axio.New(config, axio.WithOutputs(axio.MustFile("/var/log/app.log", axio.FormatJSON)))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer logger.Close()
func (l *logger) Close() error {
	if l.isFork {
		return ErrLoggerNotRoot
	}
	if !l.closed.CompareAndSwap(false, true) {
		return ErrLoggerClosed
	}

	var errs []error

	if err := l.engine.Sync(); err != nil {
		errs = append(errs, fmt.Errorf("sync engine: %w", err))
	}

	for _, out := range l.outputs {
		if err := out.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close logger: %w", errors.Join(errs...))
	}
	return nil
}

func (l *logger) log(
	ctx context.Context,
	level Level,
	err error,
	message string,
	annotations []Annotation,
) {
	if l.closed.Load() {
		return
	}

	log := l.engine.Check(toZapLevel(level), message)
	if log == nil {
		return
	}

	trace, span, _ := l.trace.Extract(ctx)

	entry := entryPool.Get().(*Entry)
	entry.Timestamp = log.Time
	entry.Logger = log.LoggerName
	entry.Caller = log.Caller.String()
	entry.Level = level
	entry.Message = log.Message
	entry.Error = err
	entry.TraceID = trace
	entry.SpanID = span
	entry.Annotations = expandAnnotable(cloneAnnotations(l.annotations, annotations))

	defer func() {
		*entry = Entry{}
		entryPool.Put(entry)
	}()

	if err := l.hooks.process(ctx, entry); err != nil {
		fmt.Fprintf(os.Stderr, "axio: hook error: %v\n", err)
		return
	}

	l.metrics.LogsTotal(ctx, level)
	log.Message = entry.Message
	fields := l.fieldsFromEntry(entry)
	if l.audited {
		fields = append(fields, contextField(ctx))
	}
	log.Write(fields...)
}

func (l *logger) fieldsFromEntry(entry *Entry) []zap.Field {
	var backing [5]zap.Field
	fields := backing[:0]

	if entry.TraceID != "" {
		fields = append(fields, zap.String(logline.TraceIDKey, entry.TraceID))
	}
	if entry.SpanID != "" {
		fields = append(fields, zap.String(logline.SpanIDKey, entry.SpanID))
	}
	if entry.Error != nil {
		fields = append(fields, zap.NamedError(logline.ErrorKey, entry.Error))
	}
	if annotationFields := annotationsToFields(entry.Annotations); annotationFields != nil {
		fields = append(fields, annotationFields...)
	}

	return fields
}

// buildEngine builds the zap logger behind a Logger: an audited core when
// chain is set, one plain core per output otherwise.
func buildEngine(config Config, outputs []Output, chain *HashChain, metrics Metrics) (*zap.Logger, error) {
	level := zap.NewAtomicLevelAt(toZapLevel(config.Level))
	metadata := serviceMetadata(config)

	var core zapcore.Core
	if chain != nil {
		core = newAuditedCore(level, outputs, metadata, chain, metrics)
	} else {
		core = newPlainCore(level, outputs, metadata)
	}
	core = &reportingCore{Core: core}

	options := []zap.Option{
		zap.AddCaller(),
		zap.AddCallerSkip(config.CallerSkip + minimumCallerSkip),
	}
	if config.Environment != EnvironmentDevelopment {
		options = append(options, zap.AddStacktrace(zapcore.ErrorLevel))
	}

	return zap.New(core, options...), nil
}

// newPlainCore writes each output in its own format. Only JSON outputs carry
// the service metadata: in text it would repeat the same fields on every line.
func newPlainCore(level zapcore.LevelEnabler, outputs []Output, metadata []zapcore.Field) zapcore.Core {
	cores := make([]zapcore.Core, 0, len(outputs))
	for _, output := range outputs {
		core := zapcore.NewCore(buildEncoder(output.Format()), output, level)
		if output.Format() == FormatJSON {
			core = core.With(metadata)
		}
		cores = append(cores, core)
	}
	return zapcore.NewTee(cores...)
}

// newAuditedCore writes every output through the audit chain. The hash covers
// the JSON encoding, service metadata included; text outputs get the entry
// without metadata and with the hash shortened.
func newAuditedCore(level zapcore.LevelEnabler, outputs []Output, metadata []zapcore.Field, chain *HashChain, metrics Metrics) zapcore.Core {
	core := &auditCore{
		LevelEnabler: level,
		chain:        chain,
		metrics:      metrics,
		canonical:    zapcore.NewJSONEncoder(logline.JSONEncoderConfig()),
	}
	addFields(core.canonical, metadata)
	for _, output := range outputs {
		if output.Format() == FormatJSON {
			core.jsonOutputs = append(core.jsonOutputs, output)
			continue
		}
		core.textSinks = append(core.textSinks, textSink{encoder: buildEncoder(output.Format()), output: output})
	}
	return core
}

// reportingCore reports a failed write on stderr as one line prefixed axio:,
// as the rest of the log path reports what it cannot return, and keeps the
// error from zap, which would print it again in its own format.
type reportingCore struct {
	zapcore.Core
}

func (r *reportingCore) With(fields []zapcore.Field) zapcore.Core {
	return &reportingCore{Core: r.Core.With(fields)}
}

func (r *reportingCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if r.Enabled(entry.Level) {
		return checked.AddCore(entry, r)
	}
	return checked
}

func (r *reportingCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	if err := r.Core.Write(entry, fields); err != nil {
		fmt.Fprintf(os.Stderr, "axio: write error: %v\n", err)
	}
	return nil
}

// serviceMetadata returns the service and deployment fields a JSON line
// carries outside [EnvironmentDevelopment].
func serviceMetadata(config Config) []zapcore.Field {
	if config.Environment == EnvironmentDevelopment {
		return nil
	}
	return []zapcore.Field{
		zap.Any(logline.ServiceKey, map[string]any{
			"name":    config.ServiceName,
			"version": config.ServiceVersion,
			"instance": map[string]any{
				"id": config.InstanceID,
			},
		}),
		zap.Any(logline.DeploymentKey, map[string]any{
			"environment": map[string]any{
				"name": config.Environment,
			},
		}),
	}
}

// buildEncoder creates the appropriate encoder for the specified format.
func buildEncoder(format Format) zapcore.Encoder {
	switch format {
	case FormatText:
		return zapcore.NewConsoleEncoder(logline.ConsoleEncoderConfig())
	default:
		return zapcore.NewJSONEncoder(logline.JSONEncoderConfig())
	}
}

func toZapLevel[T ~string | ~[]byte](level T) zapcore.Level {
	parsed, _ := zapcore.ParseLevel(string(level))
	return parsed
}

// cloneAnnotations returns a fresh slice holding the annotations of every
// source, in order. Used when forking a logger and when handing annotations to
// hooks, to break aliasing with the backing arrays of the parent and of the
// caller, so mutations (e.g. by hooks) reach neither.
func cloneAnnotations(sources ...[]Annotation) []Annotation {
	length := 0
	for _, source := range sources {
		length += len(source)
	}
	if length == 0 {
		return nil
	}
	clone := make([]Annotation, 0, length)
	for _, source := range sources {
		clone = append(clone, source...)
	}
	return clone
}

// annotationsToFields returns the fields the annotations are written as, each
// under [logline.FieldKey] of its name.
func annotationsToFields(annotations []Annotation) []zap.Field {
	if len(annotations) == 0 {
		return nil
	}

	expanded := expandAnnotable(annotations)
	fields := make([]zap.Field, len(expanded))
	for index := range expanded {
		fields[index] = expanded[index].field
		fields[index].Key = logline.FieldKey(fields[index].Key)
	}
	return fields
}

// expandAnnotable replaces Annotable annotations with their expanded fields. A
// logger expands them before the hooks run, so PII masking sees each field;
// serialization expands what a hook added after that.
func expandAnnotable(annotations []Annotation) []Annotation {
	hasAnnotable := false
	for _, annotation := range annotations {
		if _, ok := annotation.field.Interface.(Annotable); ok {
			hasAnnotable = true
			break
		}
	}
	if !hasAnnotable {
		return annotations
	}

	expanded := make([]Annotation, 0, len(annotations))
	for _, annotation := range annotations {
		if provider, ok := annotation.field.Interface.(Annotable); ok {
			expanded = provider.Append(expanded)
		} else {
			expanded = append(expanded, annotation)
		}
	}
	return expanded
}
