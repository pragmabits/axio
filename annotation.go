package axio

import (
	"fmt"
	"math"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Annotation represents a structured field attached to a log entry.
//
// Use [Annotate] to create annotations for simple key-value pairs.
// For complex types that produce multiple fields, implement [Annotable].
//
// Example:
//
//	logger.With(
//	    axio.Annotate("user_id", "usr_123"),
//	    axio.Annotate("action", "login"),
//	).Info(ctx, "user authenticated")
type Annotation struct {
	field zapcore.Field
}

// Annotate creates an annotation with the given key and value.
//
// For primitive types (string, integers, floats, bool), this function is
// zero-allocation when inlined by the compiler. Complex types (structs,
// maps, slices) fall back to interface boxing via [zap.Any].
//
// Example:
//
//	axio.Annotate("user_id", "usr_123")
//	axio.Annotate("count", 42)
//	axio.Annotate("order", myOrder)
func Annotate[T any](key string, value T) Annotation {
	switch concrete := any(value).(type) {
	case string:
		return Annotation{field: zap.String(key, concrete)}
	case int:
		return Annotation{field: zap.Int(key, concrete)}
	case int8:
		return Annotation{field: zap.Int8(key, concrete)}
	case int16:
		return Annotation{field: zap.Int16(key, concrete)}
	case int32:
		return Annotation{field: zap.Int32(key, concrete)}
	case int64:
		return Annotation{field: zap.Int64(key, concrete)}
	case uint:
		return Annotation{field: zap.Uint(key, concrete)}
	case uint8:
		return Annotation{field: zap.Uint8(key, concrete)}
	case uint16:
		return Annotation{field: zap.Uint16(key, concrete)}
	case uint32:
		return Annotation{field: zap.Uint32(key, concrete)}
	case uint64:
		return Annotation{field: zap.Uint64(key, concrete)}
	case float32:
		return Annotation{field: zap.Float32(key, concrete)}
	case float64:
		return Annotation{field: zap.Float64(key, concrete)}
	case bool:
		return Annotation{field: zap.Bool(key, concrete)}
	default:
		return Annotation{field: zap.Any(key, value)}
	}
}

// Name returns the annotation key.
func (a Annotation) Name() string { return a.field.Key }

// Data returns the annotation value.
func (a Annotation) Data() any {
	switch a.field.Type {
	case zapcore.StringType:
		return a.field.String
	case zapcore.Int64Type, zapcore.Int32Type, zapcore.Int16Type, zapcore.Int8Type:
		return a.field.Integer
	case zapcore.Uint64Type, zapcore.Uint32Type, zapcore.Uint16Type, zapcore.Uint8Type:
		return uint64(a.field.Integer)
	case zapcore.Float64Type:
		return math.Float64frombits(uint64(a.field.Integer))
	case zapcore.Float32Type:
		return float64(math.Float32frombits(uint32(a.field.Integer)))
	case zapcore.BoolType:
		return a.field.Integer == 1
	default:
		return a.field.Interface
	}
}

// Set updates the annotation value, preserving the key.
func (a *Annotation) Set(value any) {
	a.field = toField(a.field.Key, value)
}

// Annotations is a collection of annotations with helper methods.
type Annotations []Annotation

// Names returns the names of all annotations in the collection.
func (a Annotations) Names() []string {
	names := make([]string, len(a))
	for index := range a {
		names[index] = a[index].field.Key
	}
	return names
}

// Data returns the values of all annotations in the collection.
func (a Annotations) Data() []any {
	contents := make([]any, len(a))
	for index := range a {
		contents[index] = a[index].Data()
	}
	return contents
}

// Add adds a new annotation to the collection and returns the modified collection.
func (a *Annotations) Add(key string, value any) Annotations {
	*a = append(*a, Annotate(key, value))
	return *a
}

// Annotable allows complex types to produce annotations for log entries.
//
// Types that implement this interface can be passed to [Logger.With]
// via [Annotate]. The logger detects the [Annotable] implementation
// and expands the annotations during field serialization.
//
// Append appends the type's annotations to the provided slice and
// returns the extended slice.
//
// Example:
//
//	func (h HTTP) Append(target []axio.Annotation) []axio.Annotation {
//	    return append(target,
//	        axio.Annotate("method", h.Method),
//	        axio.Annotate("url", h.URL),
//	        axio.Annotate("status_code", h.StatusCode),
//	    )
//	}
type Annotable interface {
	Append([]Annotation) []Annotation
}

// HTTP represents HTTP request metadata for structured logging.
//
// Use this annotation to add HTTP request context to logs,
// facilitating correlation and problem analysis.
//
// HTTP implements [Annotable] to produce individual fields for each
// request attribute.
//
// Example:
//
//	http := axio.HTTP{
//	    Method:     "POST",
//	    URL:        "/api/v1/orders",
//	    StatusCode: 201,
//	    LatencyMS:  45,
//	    UserAgent:  r.UserAgent(),
//	    ClientIP:   r.RemoteAddr,
//	}
//	logger.With(axio.Annotate("http", http)).Info(ctx, "request processed")
type HTTP struct {
	// Method is the HTTP method (GET, POST, PUT, DELETE, etc).
	Method string `json:"method"`
	// URL is the request path.
	URL string `json:"url"`
	// StatusCode is the HTTP response code.
	StatusCode int `json:"status_code"`
	// LatencyMS is the request latency in milliseconds.
	LatencyMS int64 `json:"latency"`
	// UserAgent is the client's User-Agent header.
	UserAgent string `json:"user_agent"`
	// ClientIP is the client's IP address.
	ClientIP string `json:"client_ip"`
}

// Append implements [Annotable] for HTTP request metadata.
func (h HTTP) Append(target []Annotation) []Annotation {
	return append(target,
		Annotate("method", h.Method),
		Annotate("url", h.URL),
		Annotate("status_code", h.StatusCode),
		Annotate("latency", h.LatencyMS),
		Annotate("user_agent", h.UserAgent),
		Annotate("client_ip", h.ClientIP),
	)
}

// toField converts a Go value to an appropriate zap.Field.
//
// Supports: string, []byte, fmt.Stringer, int*, uint*, float*, bool,
// map[string]any, zapcore.ObjectMarshaler, zapcore.ArrayMarshaler.
// Any other type falls through to zap.Any for best-effort serialization.
func toField(key string, raw any) zap.Field {
	switch value := raw.(type) {
	case string:
		return zap.String(key, value)
	case []byte:
		return zap.ByteString(key, value)
	case fmt.Stringer:
		return zap.String(key, value.String())
	case int:
		return zap.Int(key, value)
	case int8:
		return zap.Int8(key, value)
	case int16:
		return zap.Int16(key, value)
	case int32:
		return zap.Int32(key, value)
	case int64:
		return zap.Int64(key, value)
	case uint:
		return zap.Uint(key, value)
	case uint8:
		return zap.Uint8(key, value)
	case uint16:
		return zap.Uint16(key, value)
	case uint32:
		return zap.Uint32(key, value)
	case uint64:
		return zap.Uint64(key, value)
	case float32:
		return zap.Float32(key, value)
	case float64:
		return zap.Float64(key, value)
	case bool:
		return zap.Bool(key, value)
	case map[string]any:
		return zap.Any(key, value)
	case zapcore.ObjectMarshaler:
		return zap.Object(key, value)
	case zapcore.ArrayMarshaler:
		return zap.Array(key, value)
	default:
		return zap.Any(key, value)
	}
}
