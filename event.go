package axio

import (
	"context"
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
// An event comes from a [Logger] through [Logger.Event] and owns nothing: it
// writes to that logger's outputs, through its hooks and its audit chain, and
// stops writing once the root logger is closed. A JSON output gets the event as
// JSON and a text output as the Console's text, with EVENT in the level column.
//
// The event output omits the log level field — severity is expressed through
// the event's own fields (status_code, error, etc.), not through traditional
// log levels.
//
// Events are thread-safe and can be enriched concurrently from multiple goroutines.
//
// Example:
//
//	event := logger.Event("checkout")
//	ctx = axio.WithEvent(ctx, event)
//	defer event.Emit(ctx)
//
//	// Later, in handlers:
//	event := axio.EventFromContext(ctx)
//	event.Add("user_id", userID)
//	event.Add("cart_total", cartTotal)
type Event struct {
	name        string
	loggerName  string
	engine      *zap.Logger
	hooks       *hookChain
	trace       Tracer
	annotations []Annotation
	err         error
	errDetails  []Annotation
	startTime   time.Time
	mutex       sync.Mutex
	emitted     atomic.Bool
	lifecycle   *lifecycle
	audited     bool
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
// Subsequent calls are no-ops; only the first call produces output. After the
// root logger the event came from is closed, Emit writes nothing.
//
// Hooks see the event name as [Entry.Message]; a name they change is the
// name written.
func (e *Event) Emit(ctx context.Context) {
	if !e.emitted.CompareAndSwap(false, true) {
		return
	}
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if e.lifecycle.closed.Load() {
		return
	}

	durationMS := time.Since(e.startTime).Milliseconds()

	entry := entryPool.Get().(*Entry)
	entry.Timestamp = e.startTime
	entry.Level = LevelInfo
	entry.Message = e.name
	entry.Logger = e.loggerName
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
	log.LoggerName = e.loggerName
	fields := eventFields(entry, durationMS)
	if e.audited {
		fields = append(fields, contextField(ctx))
	}
	e.lifecycle.write(log, fields)
}

type eventContextKey struct{}

// WithEvent stores the event in the context for retrieval by downstream handlers.
//
// Typically called in middleware to make the event available to the entire
// request chain. Retrieve the event later with [EventFromContext].
//
// Example:
//
//	event := logger.Event("http_request")
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

// buildEventEngine creates the zap logger a [Logger]'s events write through,
// over the logger's outputs and audit chain: JSON outputs get
// [logline.EventEncoderConfig], with the name under "event" and no level,
// caller or stacktrace, and text outputs [logline.EventConsoleEncoderConfig].
// Its level is fixed at info, so every event is written whatever the logger's
// level. With auditing on, the JSON line is hashed and the text carries the
// short hash, as for an entry.
func buildEventEngine(outputs []Output, parts engineParts) *zap.Logger {
	level := zap.NewAtomicLevelAt(zapcore.InfoLevel)
	if parts.chain != nil {
		return zap.New(&reportingCore{Core: newAuditedCore(level, outputs, nil, parts, eventEncoding())})
	}
	return zap.New(&reportingCore{Core: newPlainCore(level, outputs, nil, eventEncoding())})
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
