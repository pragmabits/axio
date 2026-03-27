package axio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var entryPool = sync.Pool{
	New: func() any { return &Entry{} },
}

const minimumCallerSkip = 2

func toZapLevel[T ~string | ~[]byte](level T) zapcore.Level {
	parsed, _ := zapcore.ParseLevel(string(level))
	return parsed
}

type logger struct {
	engine      *zap.Logger
	trace       Tracer
	hooks       *HookChain
	metrics     Metrics
	annotations []Annotation
	outputs     []Output
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
// or [ErrValidateConfig] if the resulting configuration is invalid.
//
// Basic example:
//
//	config := axio.Config{
//	    ServiceName:    "sales-api",
//	    ServiceVersion: "2.1.0",
//	    Environment:    axio.Production,
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

	outputs, err := BuildOutputs(config)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildOutputs, err)
	}

	metrics, err := BuildMetrics(config)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildMetrics, err)
	}

	hooks, err := BuildHooks(config)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildHooks, err)
	}

	tracer := BuildTracer(config)

	engine, err := buildEngine(config, outputs)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildEngine, err)
	}

	return &logger{
		engine:  engine,
		trace:   tracer,
		hooks:   NewHookChain(metrics, hooks...),
		metrics: metrics,
		outputs: outputs,
	}, nil
}

func buildEngine(config Config, outputs []Output) (*zap.Logger, error) {
	level := toZapLevel(config.Level)

	cores := make([]zapcore.Core, 0, len(outputs))
	for _, out := range outputs {
		encoder := buildEncoder(out.Format())
		core := zapcore.NewCore(encoder, out, zap.NewAtomicLevelAt(level))
		cores = append(cores, core)
	}

	options := []zap.Option{
		zap.AddCaller(),
		zap.AddCallerSkip(config.CallerSkip + minimumCallerSkip),
	}

	if config.Environment != Development {
		options = append(options, zap.AddStacktrace(zapcore.ErrorLevel))
	}

	core := zapcore.NewTee(cores...)
	engine := zap.New(core, options...)

	if config.Environment != Development {
		engine = engine.With(
			zap.Any("service", map[string]any{
				"name":    config.ServiceName,
				"version": config.ServiceVersion,
				"instance": map[string]any{
					"id": config.InstanceID,
				},
			}),
			zap.Any("deployment", map[string]any{
				"environment": map[string]any{
					"name": config.Environment,
				},
			}),
		)
	}

	return engine, nil
}

// buildEncoder creates the appropriate encoder for the specified format.
func buildEncoder(format Format) zapcore.Encoder {
	switch format {
	case FormatText:
		return zapcore.NewConsoleEncoder(consoleEncoderConfig)
	default:
		return zapcore.NewJSONEncoder(jsonEncoderConfig)
	}
}

func (l logger) Named(name string) Logger {
	l.engine = l.engine.Named(name)
	l.clear()
	return &l
}

func (l logger) With(annotations ...Annotation) Logger {
	l.clear()
	l.annotations = append(l.annotations, annotations...)
	return &l
}

func (l logger) Debug(ctx context.Context, message string, args ...any) {
	l.log(ctx, LevelDebug, nil, message, args...)
}

func (l logger) Info(ctx context.Context, message string, args ...any) {
	l.log(ctx, LevelInfo, nil, message, args...)
}

func (l logger) Warn(ctx context.Context, err error, message string, args ...any) {
	l.log(ctx, LevelWarn, err, message, args...)
}

func (l logger) Error(ctx context.Context, err error, message string, args ...any) {
	l.log(ctx, LevelError, err, message, args...)
}

func (l *logger) log(
	ctx context.Context,
	level Level,
	err error,
	message string,
	args ...any,
) {
	log := l.engine.Check(toZapLevel(level), l.formatMessage(message, args...))
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
	entry.Annotations = l.annotations
	entry.Hash = ""
	entry.PreviousHash = ""

	defer func() {
		*entry = Entry{}
		entryPool.Put(entry)
	}()

	if err := l.hooks.Process(ctx, entry); err != nil {
		fmt.Fprintf(os.Stderr, "axio: hook error: %v\n", err)
		return
	}

	l.metrics.LogsTotal(ctx, level)
	log.Message = entry.Message
	log.Write(l.fieldsFromEntry(entry)...)
}

func (l *logger) fieldsFromEntry(entry *Entry) []zap.Field {
	var buf [5]zap.Field
	fields := buf[:0]

	if entry.TraceID != "" {
		fields = append(fields, zap.String("trace_id", entry.TraceID))
	}
	if entry.SpanID != "" {
		fields = append(fields, zap.String("span_id", entry.SpanID))
	}
	if entry.Error != nil {
		fields = append(fields, zap.Error(entry.Error))
	}
	if annotationFields := annotationsToFields(entry.Annotations); annotationFields != nil {
		fields = append(fields, annotationFields...)
	}

	return fields
}

func annotationsToFields(annotations []Annotation) []zap.Field {
	if len(annotations) == 0 {
		return nil
	}

	expanded := expandAnnotable(annotations)
	fields := make([]zap.Field, len(expanded))
	for index := range expanded {
		fields[index] = expanded[index].field
	}
	return fields
}

// expandAnnotable replaces Annotable annotations with their expanded fields.
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

func (l *logger) clear() {
	l.annotations = nil
}

// formatMessage formats the message with the arguments.
// When no args are provided, the format string is returned directly
// without defer overhead.
func (l *logger) formatMessage(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return l.sprintfRecover(format, args)
}

// sprintfRecover calls fmt.Sprintf recovering from panics caused by
// incompatible format/args combinations.
func (l *logger) sprintfRecover(format string, args []any) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = fmt.Sprintf("[INVALID FORMAT] format=%q args=%v panic=%v", format, args, r)
			fmt.Fprintf(os.Stderr, "axio: panic in message formatting: %v\n", r)
		}
	}()
	return fmt.Sprintf(format, args...)
}

// Close releases all resources associated with the logger.
//
// This includes closing files opened by [File] outputs.
// It should be called when the logger is no longer needed,
// typically with defer in main.
//
// Example:
//
//	logger, err := axio.New(config, axio.WithOutputs(axio.MustFile("/var/log/app.log", axio.FormatJSON)))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer logger.Close()
func (l *logger) Close() error {
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
