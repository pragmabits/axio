package axio

import (
	"context"
	"errors"
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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
	http        *HTTP
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
	for _, annotation := range annotations {
		if annotation == nil {
			continue
		}
		switch a := annotation.(type) {
		case *HTTP:
			l.http = a
		default:
			l.annotations = append(l.annotations, annotation)
		}
	}
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
	entry := &Entry{
		Timestamp:   log.Time,
		Logger:      log.LoggerName,
		Caller:      log.Caller.String(),
		Level:       level,
		Message:     log.Message,
		Error:       err,
		TraceID:     trace,
		SpanID:      span,
		Annotations: l.annotations,
	}

	if err := l.hooks.Process(ctx, entry); err != nil {
		fmt.Fprintf(os.Stderr, "axio: hook error: %v\n", err)
		return
	}

	l.metrics.LogsTotal(ctx, level)
	log.Message = entry.Message
	log.Write(l.fieldsFromEntry(entry)...)
}

func (l *logger) fieldsFromEntry(entry *Entry) []zap.Field {
	fields := []zap.Field{
		zap.String("trace_id", entry.TraceID),
		zap.String("span_id", entry.SpanID),
		zap.Error(entry.Error),
	}

	if l.http != nil {
		fields = append(fields, zap.Object("http", marshaler{l.http}))
	}

	annotationFields := annotationsToFields(entry.Annotations)
	fields = append(fields, zap.Any("annotations", annotationFields))

	return fields
}

func annotationsToFields(annotations []Annotation) []zap.Field {
	fields := make([]zap.Field, len(annotations))
	for index, annotation := range annotations {
		fields[index] = toField(annotation.Name(), annotation.Data())
	}
	return fields
}

func (l *logger) clear() {
	l.annotations = nil
	l.http = nil
}

// formatMessage formats the message with the arguments, recovering from panic
// if the arguments are incompatible with the format.
func (l *logger) formatMessage(format string, args ...any) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = fmt.Sprintf("[INVALID FORMAT] format=%q args=%v panic=%v", format, args, r)
			fmt.Fprintf(os.Stderr, "axio: panic in message formatting: %v\n", r)
		}
	}()

	if len(args) == 0 {
		return format
	}
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
