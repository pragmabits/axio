package axio

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Event represents a wide event that accumulates annotations throughout a
// unit of work (e.g., an HTTP request) and emits a single comprehensive
// log entry at the end.
//
// Wide events follow the canonical log line pattern: instead of scattering
// many log lines per request, one richly-annotated event captures the complete
// context of what happened.
//
// The event output omits the log level field — severity is expressed through
// the event's own fields (status_code, error, etc.), not through traditional
// log levels.
//
// Events are thread-safe and can be enriched concurrently from multiple goroutines.
//
// Example:
//
//	event, err := axio.NewEvent("checkout", config)
//	if err != nil {
//	    return err
//	}
//	ctx = axio.WithEvent(ctx, event)
//	defer event.Emit(ctx)
//
//	// Later, in handlers:
//	event := axio.EventFromContext(ctx)
//	event.Add("user_id", userID)
//	event.Add("cart_total", cartTotal)
type Event struct {
	name        string
	engine      *zap.Logger
	hooks       *HookChain
	trace       Tracer
	metrics     Metrics
	annotations []Annotation
	err         error
	errDetails  []Annotation
	startTime   time.Time
	outputs     []Output
	mutex       sync.Mutex
}

type eventContextKey struct{}

// NewEvent creates a new wide event with the specified name and configuration.
//
// The event builds its own internal logger using the provided configuration,
// with the level field omitted from output. The same [Config] and [Option]
// functions used with [New] work here.
//
// Example:
//
//	event, err := axio.NewEvent("http_request", config,
//	    axio.WithOutputs(axio.Stdout(axio.FormatJSON)),
//	    axio.WithPII(nil, nil),
//	)
func NewEvent(name string, config Config, options ...Option) (*Event, error) {
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

	engine, err := buildEventEngine(outputs)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildEngine, err)
	}

	return &Event{
		name:      name,
		engine:    engine,
		hooks:     NewHookChain(metrics, hooks...),
		trace:     tracer,
		metrics:   metrics,
		startTime: time.Now(),
		outputs:   outputs,
	}, nil
}

// buildEventEngine creates a zap logger for wide events.
//
// Uses eventEncoderConfig which omits level, caller, logger name,
// and stacktrace fields. The message key is "event".
func buildEventEngine(outputs []Output) (*zap.Logger, error) {
	cores := make([]zapcore.Core, 0, len(outputs))
	for _, output := range outputs {
		encoder := zapcore.NewJSONEncoder(eventEncoderConfig)
		core := zapcore.NewCore(encoder, output, zap.NewAtomicLevelAt(zapcore.InfoLevel))
		cores = append(cores, core)
	}

	core := zapcore.NewTee(cores...)
	return zap.New(core), nil
}

// WithEvent stores the event in the context for retrieval by downstream handlers.
//
// Typically called in middleware to make the event available to the entire
// request chain. Retrieve the event later with [EventFromContext].
//
// Example:
//
//	event, _ := axio.NewEvent("http_request", config)
//	ctx = axio.WithEvent(ctx, event)
func WithEvent(ctx context.Context, event *Event) context.Context {
	return context.WithValue(ctx, eventContextKey{}, event)
}

// EventFromContext retrieves the event from the context.
// Returns nil if no event is stored in the context.
//
// Example:
//
//	event := axio.EventFromContext(ctx)
//	if event != nil {
//	    event.Add("user_id", userID)
//	}
func EventFromContext(ctx context.Context) *Event {
	event, _ := ctx.Value(eventContextKey{}).(*Event)
	return event
}

// Add adds a key-value annotation to the event.
//
// Uses [Annotate] internally, supporting the same types.
// Thread-safe — can be called from multiple goroutines.
func (e *Event) Add(key string, value any) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.annotations = append(e.annotations, Annotate(key, value))
}

// With adds annotations to the event.
//
// Accepts [Annotation] values, including [Annotable] types that expand
// into multiple fields.
// Thread-safe — can be called from multiple goroutines.
func (e *Event) With(annotations ...Annotation) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.annotations = append(e.annotations, annotations...)
}

// SetError records an error on the event with optional detail annotations.
//
// The error appears as an "error" field in the output.
// Detail annotations are added alongside the error for richer context.
//
// Example:
//
//	event.SetError(err)
//
//	event.SetError(err,
//	    axio.Annotate("error_code", "card_declined"),
//	    axio.Annotate("error_retriable", false),
//	)
func (e *Event) SetError(err error, details ...Annotation) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.err = err
	e.errDetails = details
}

// Emit writes the wide event as a single log entry.
//
// This method:
//   - Computes duration_ms from the event creation time
//   - Adds all accumulated annotations, error, and duration
//   - Runs hooks (PII masking, audit chain, custom)
//   - Writes the entry through the internal logger
//
// Emit should be called once, typically via defer in middleware.
func (e *Event) Emit(ctx context.Context) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	durationMS := time.Since(e.startTime).Milliseconds()

	entry := entryPool.Get().(*Entry)
	entry.Timestamp = e.startTime
	entry.Level = LevelInfo
	entry.Message = e.name
	entry.Error = e.err
	entry.Annotations = append(e.annotations, e.errDetails...)
	entry.Hash = ""
	entry.PreviousHash = ""

	trace, span, _ := e.trace.Extract(ctx)
	entry.TraceID = trace
	entry.SpanID = span

	defer func() {
		*entry = Entry{}
		entryPool.Put(entry)
	}()

	if err := e.hooks.Process(ctx, entry); err != nil {
		fmt.Fprintf(os.Stderr, "axio: event hook error: %v\n", err)
		return
	}

	fields := annotationsToFields(entry.Annotations)
	fields = append(fields, zap.Int64("duration_ms", durationMS))

	if entry.Error != nil {
		fields = append(fields, zap.Error(entry.Error))
	}

	if entry.TraceID != "" {
		fields = append(fields, zap.String("trace_id", entry.TraceID))
	}
	if entry.SpanID != "" {
		fields = append(fields, zap.String("span_id", entry.SpanID))
	}

	if entry.Hash != "" {
		fields = append(fields, zap.String("hash", entry.Hash))
	}
	if entry.PreviousHash != "" {
		fields = append(fields, zap.String("previous_hash", entry.PreviousHash))
	}

	log := e.engine.Check(zap.InfoLevel, e.name)
	if log == nil {
		return
	}
	log.Time = e.startTime
	log.Write(fields...)
}

// Close releases resources associated with the event's outputs.
//
// Call Close after [Event.Emit] to release file handles and other resources.
// For events without file outputs, Close is a no-op but should still be called
// for correctness.
//
// Example:
//
//	event, _ := axio.NewEvent("checkout", config)
//	defer event.Close()
//	// ... enrich event ...
//	event.Emit(ctx)
func (e *Event) Close() error {
	for _, output := range e.outputs {
		if err := output.Close(); err != nil {
			return err
		}
	}
	return nil
}
