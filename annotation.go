package axio

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Annotation represents a structured field attached to a log entry.
//
// Annotations allow adding typed context to logs in a structured way.
// Included implementations: [Note] for simple key-value pairs and [HTTP]
// for HTTP request metadata.
//
// Example of custom implementation:
//
//	type UserAnnotation struct {
//	    ID    string
//	    Email string
//	}
//
//	func (u UserAnnotation) Name() string  { return "user" }
//	func (u UserAnnotation) Data() any { return u }
type Annotation interface {
	// Name returns the field name in the log.
	Name() string
	// Data returns the value to be serialized.
	Data() any
	// Set updates the annotation value.
	Set(any)
}

// Annotations is a collection of annotations with helper methods.
type Annotations []Annotation

// Names returns the names of all annotations in the collection.
func (a Annotations) Names() []string {
	names := make([]string, len(a))
	for index, annotation := range a {
		names[index] = annotation.Name()
	}
	return names
}

// Data returns the values of all annotations in the collection.
func (a Annotations) Data() []any {
	contents := make([]any, len(a))
	for index, annotation := range a {
		contents[index] = annotation.Data()
	}
	return contents
}

// Add adds a new annotation to the collection and returns the modified collection.
func (a *Annotations) Add(name string, content any) Annotations {
	*a = append(*a, &Note{Key: name, Value: content})
	return *a
}

// Annotator allows fluent addition of fields during log serialization.
//
// It is used internally by the marshaling system to allow custom types
// to add multiple fields to the log.
type Annotator interface {
	// Add adds a field and returns the Annotator for chaining.
	Add(string, any) Annotator
}

// Marshaler allows custom types to control their serialization in logs.
//
// Implement this interface when you need full control over how
// a type is represented in the log.
//
// Example:
//
//	type Order struct {
//	    ID     string
//	    Items  []Item
//	    secret string // will not be logged
//	}
//
//	func (o Order) MarshalLog(a axio.Annotator) error {
//	    a.Add("order_id", o.ID)
//	    a.Add("item_count", len(o.Items))
//	    return nil
//	}
type Marshaler interface {
	// MarshalLog serializes the type using the provided Annotator.
	MarshalLog(Annotator) error
}

// Note is the default implementation of [Annotation] for simple key-value pairs.
//
// Use [Annotate] to create Notes more concisely.
type Note struct {
	// Key is the field name in the log.
	Key string
	// Value is the value to be serialized.
	Value any
}

// Annotate creates a simple annotation with key and value.
//
// Example:
//
//	logger.With(
//	    axio.Annotate("user_id", "usr_123"),
//	    axio.Annotate("action", "login"),
//	).Info(ctx, "user authenticated")
func Annotate(key string, value any) Annotation {
	return &Note{Key: key, Value: value}
}

// Name returns the annotation key.
func (d Note) Name() string { return d.Key }

// Data returns the annotation value.
func (d Note) Data() any { return d.Value }

// Set updates the note value.
func (d *Note) Set(value any) { d.Value = value }

// HTTP represents HTTP request metadata for structured logging.
//
// Use this annotation to add HTTP request context to logs,
// facilitating correlation and problem analysis.
//
// Example:
//
//	http := &axio.HTTP{
//	    Method:     "POST",
//	    URL:        "/api/v1/orders",
//	    StatusCode: 201,
//	    LatencyMS:  45,
//	    UserAgent:  r.UserAgent(),
//	    ClientIP:   r.RemoteAddr,
//	}
//	logger.With(http).Info(ctx, "request processed")
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

// Name returns "http" as the field name in the log.
func (h HTTP) Name() string { return "http" }

// Data returns the struct itself for serialization.
func (h HTTP) Data() any { return h }

// Set updates the HTTP values from the provided value.
func (h *HTTP) Set(value any) {
	switch v := value.(type) {
	case HTTP:
		*h = v
	case *HTTP:
		*h = *v
	}
}

// MarshalLog implements [Marshaler] for custom serialization.
func (h HTTP) MarshalLog(annotator Annotator) error {
	annotator.Add("method", h.Method)
	annotator.Add("url", h.URL)
	annotator.Add("status_code", h.StatusCode)
	annotator.Add("latency", h.LatencyMS)
	annotator.Add("user_agent", h.UserAgent)
	annotator.Add("client_ip", h.ClientIP)
	return nil
}

// annotator adapts zapcore.ObjectEncoder to the Annotator interface.
type annotator struct {
	encoder zapcore.ObjectEncoder
}

// Add adds a field to the encoder and returns the annotator for chaining.
func (a *annotator) Add(key string, value any) Annotator {
	toField(key, value).AddTo(a.encoder)
	return a
}

// marshaler adapts Marshaler to zapcore.ObjectMarshaler.
type marshaler struct {
	marshaler Marshaler
}

var _ zapcore.ObjectMarshaler = marshaler{}

// MarshalLogObject implements zapcore.ObjectMarshaler.
func (a marshaler) MarshalLogObject(
	encoder zapcore.ObjectEncoder,
) error {
	return a.marshaler.MarshalLog(&annotator{encoder: encoder})
}

// toField converts a Go value to an appropriate zap.Field.
//
// Supports: string, []byte, fmt.Stringer, int*, uint*, float*, bool,
// map[string]any, zapcore.ObjectMarshaler, zapcore.ArrayMarshaler, Marshaler.
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
	case Marshaler:
		return zap.Object(key, marshaler{marshaler: value})
	default:
		return zap.Any(key, value)
	}
}
