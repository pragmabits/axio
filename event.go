package axio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/pragmabits/axio/internal/logline"
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
	hooks       *hookChain
	trace       Tracer
	metrics     Metrics
	annotations []Annotation
	err         error
	errDetails  []Annotation
	startTime   time.Time
	outputs     []Output
	mutex       sync.Mutex
	emitted     atomic.Bool
	closed      atomic.Bool
	audited     bool
}

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

	engine, err := buildEventEngine(outputs, chain, metrics)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildEngine, err)
	}

	return &Event{
		name:      name,
		engine:    engine,
		hooks:     newHookChain(metrics, hooks...),
		trace:     buildTracer(config),
		metrics:   metrics,
		startTime: time.Now(),
		outputs:   outputs,
		audited:   chain != nil,
	}, nil
}

// Add adds a key-value annotation to the event.
//
// Uses [Field] internally, supporting the same types. Like [Field], it
// takes the value's type as a type parameter, so a value of a primitive type is
// kept without allocating; an untyped nil has no type to take and does not
// compile.
// Thread-safe — can be called from multiple goroutines.
func (e *Event) Add[T any](key string, value T) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.annotations = append(e.annotations, Field(key, value))
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
//	    axio.Field("error_code", "card_declined"),
//	    axio.Field("error_retriable", false),
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
//   - Runs hooks (PII masking, custom)
//   - With auditing on, hashes the line as it is written
//   - Writes the entry through the internal logger
//
// Emit should be called once, typically via defer in middleware.
// Subsequent calls are no-ops; only the first call produces output. After
// [Event.Close], Emit writes nothing.
//
// Hooks see the event name as [Entry.Message]; a name they change is the
// name written.
func (e *Event) Emit(ctx context.Context) {
	if !e.emitted.CompareAndSwap(false, true) {
		return
	}
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if e.closed.Load() {
		return
	}

	durationMS := time.Since(e.startTime).Milliseconds()

	entry := entryPool.Get().(*Entry)
	entry.Timestamp = e.startTime
	entry.Level = LevelInfo
	entry.Message = e.name
	entry.Error = e.err
	entry.Annotations = expandAnnotable(append(cloneAnnotations(e.annotations), e.errDetails...))

	trace, span, _ := e.trace.Extract(ctx)
	entry.TraceID = trace
	entry.SpanID = span

	defer func() {
		*entry = Entry{}
		entryPool.Put(entry)
	}()

	if err := e.hooks.process(ctx, entry); err != nil {
		fmt.Fprintf(os.Stderr, "axio: event hook error: %v\n", err)
		return
	}

	log := e.engine.Check(zap.InfoLevel, entry.Message)
	if log == nil {
		return
	}
	log.Time = e.startTime
	fields := eventFields(entry, durationMS)
	if e.audited {
		fields = append(fields, contextField(ctx))
	}
	log.Write(fields...)
}

// Close releases resources associated with the event's outputs.
//
// Call Close after [Event.Emit] to release file handles and other resources.
// For events without file outputs, Close is a no-op but should still be called
// for correctness. An Emit already writing finishes first; an Emit after Close
// writes nothing. A second call returns [ErrEventClosed].
//
// Example:
//
//	event, _ := axio.NewEvent("checkout", config)
//	defer event.Close()
//	// ... enrich event ...
//	event.Emit(ctx)
func (e *Event) Close() error {
	if !e.closed.CompareAndSwap(false, true) {
		return ErrEventClosed
	}
	e.mutex.Lock()
	defer e.mutex.Unlock()

	var errs []error
	for _, output := range e.outputs {
		if err := output.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close event: %w", errors.Join(errs...))
	}
	return nil
}

type eventContextKey struct{}

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

// buildEventEngine creates a zap logger for wide events.
//
// Uses [logline.EventEncoderConfig], which omits level, caller, logger name,
// and stacktrace fields. The message key is "event". Events are always JSON,
// whatever the output's format; with auditing on, every output receives the
// audited line.
func buildEventEngine(outputs []Output, chain *HashChain, metrics Metrics) (*zap.Logger, error) {
	level := zap.NewAtomicLevelAt(zapcore.InfoLevel)
	if chain != nil {
		return zap.New(&reportingCore{Core: &auditCore{
			LevelEnabler: level,
			chain:        chain,
			metrics:      metrics,
			canonical:    zapcore.NewJSONEncoder(logline.EventEncoderConfig()),
			jsonOutputs:  outputs,
		}}), nil
	}

	cores := make([]zapcore.Core, 0, len(outputs))
	for _, output := range outputs {
		encoder := zapcore.NewJSONEncoder(logline.EventEncoderConfig())
		cores = append(cores, zapcore.NewCore(encoder, output, level))
	}
	return zap.New(&reportingCore{Core: zapcore.NewTee(cores...)}), nil
}

// eventFields converts a processed event entry into the fields written by
// [Event.Emit]: annotations first, then duration, error and trace.
func eventFields(entry *Entry, durationMS int64) []zap.Field {
	fields := annotationsToFields(entry.Annotations)
	fields = append(fields, zap.Int64(logline.DurationKey, durationMS))

	if entry.Error != nil {
		fields = append(fields, zap.NamedError(logline.ErrorKey, entry.Error))
	}

	if entry.TraceID != "" {
		fields = append(fields, zap.String(logline.TraceIDKey, entry.TraceID))
	}
	if entry.SpanID != "" {
		fields = append(fields, zap.String(logline.SpanIDKey, entry.SpanID))
	}

	return fields
}
