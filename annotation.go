package axio

import (
	"math"
	"reflect"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Annotation represents a structured field attached to a log entry.
//
// Use [Field] to create annotations for simple key-value pairs.
// For complex types that produce multiple fields, implement [Annotable].
//
// The internal storage wraps a zapcore.Field directly to avoid a second
// type-switch on every log call. The field is unexported and unreachable
// through any method or signature on the public API; downstream consumers
// interact only via [Annotation.Name], [Annotation.Data] and [Annotation.Value]. This is a
// deliberate exception to the project rule that axio's public types use
// only axio-native types — the alternative would pay a per-call translation
// cost with no user-visible reward (godoc already hides unexported fields).
//
// Example:
//
//	logger.Info(ctx, "user authenticated",
//	    axio.Field("user_id", "usr_123"),
//	    axio.Field("action", "login"),
//	)
type Annotation struct {
	field zapcore.Field
}

// Field creates an annotation with the given key and value.
//
// For primitive types (string, integers, floats, bool), the implementation
// is zero-allocation when inlined by the compiler. Complex types (structs,
// maps, slices) fall back to interface boxing.
//
// Example:
//
//	axio.Field("user_id", "usr_123")
//	axio.Field("count", 42)
//	axio.Field("order", myOrder)
func Field[T any](key string, value T) Annotation {
	switch concrete := any(value).(type) {
	case string:
		return Annotation{field: zap.String(key, concrete)}
	case int:
		return Annotation{field: zap.Int(key, concrete)}
	case int64:
		return Annotation{field: zap.Int64(key, concrete)}
	case int32:
		return Annotation{field: zap.Int32(key, concrete)}
	case uint:
		return Annotation{field: zap.Uint(key, concrete)}
	case uint64:
		return Annotation{field: zap.Uint64(key, concrete)}
	case float64:
		return Annotation{field: zap.Float64(key, concrete)}
	case bool:
		return Annotation{field: zap.Bool(key, concrete)}
	default:
		return uncommonField(key, value)
	}
}

// Name returns the annotation key.
func (a Annotation) Name() string { return a.field.Key }

// Data returns the annotation value: int64 for a signed integer, uint64 for an
// unsigned one, float64 for a floating-point number, and any other value as it
// was given.
func (a Annotation) Data() any {
	if value, ok := a.integerData(); ok {
		return value
	}
	switch a.field.Type {
	case zapcore.StringType:
		return a.field.String
	case zapcore.Float64Type:
		return math.Float64frombits(uint64(a.field.Integer))
	case zapcore.Float32Type:
		return float64(math.Float32frombits(uint32(a.field.Integer)))
	case zapcore.BoolType:
		return a.field.Integer == 1
	case zapcore.DurationType:
		return time.Duration(a.field.Integer)
	case zapcore.TimeType:
		return a.moment()
	case zapcore.ArrayMarshalerType:
		return givenSlice(a.field.Interface)
	default:
		return a.field.Interface
	}
}

// Value returns the annotation's value as a T, and whether it holds one. A
// number converts to any number type of its kind — signed, unsigned or
// floating point — that holds it exactly, so an annotation made from an int
// comes back as an int, an int64, or an int8 when it fits. Any other value must
// be a T.
//
// Example:
//
//	if count, ok := annotation.Value[int](); ok {
//	    total += count
//	}
func (a Annotation) Value[T any]() (T, bool) {
	data := a.Data()
	if value, ok := data.(T); ok {
		return value, true
	}
	var value T
	ok := convertNumber(data, reflect.ValueOf(&value).Elem())
	return value, ok
}

// integerData returns the value of a field zap stores as an integer: int64 for
// a signed one, uint64 for an unsigned one or a uintptr.
func (a Annotation) integerData() (any, bool) {
	switch a.field.Type {
	case zapcore.Int64Type, zapcore.Int32Type, zapcore.Int16Type, zapcore.Int8Type:
		return a.field.Integer, true
	case zapcore.Uint64Type, zapcore.Uint32Type, zapcore.Uint16Type, zapcore.Uint8Type, zapcore.UintptrType:
		return uint64(a.field.Integer), true
	default:
		return nil, false
	}
}

// moment returns the time a TimeType field holds: nanoseconds since the epoch,
// in the location zap keeps beside them.
func (a Annotation) moment() time.Time {
	moment := time.Unix(0, a.field.Integer)
	if location, ok := a.field.Interface.(*time.Location); ok {
		return moment.In(location)
	}
	return moment
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

// Add adds a new annotation to the collection and returns the modified
// collection. Like [Field], it takes the value's type as a type parameter, so a
// value of a primitive type is kept without allocating.
func (a *Annotations) Add[T any](key string, value T) Annotations {
	*a = append(*a, Field(key, value))
	return *a
}

// Annotable allows complex types to produce annotations for log entries.
//
// Types that implement this interface can be passed to a level method or to
// [Logger.With] via [Field]. The logger detects the [Annotable]
// implementation and expands the annotations before the hooks run, so PII
// masking and custom hooks see each field on its own.
//
// Append appends the type's annotations to the provided slice and
// returns the extended slice.
//
// Example:
//
//	func (h HTTP) Append(target []axio.Annotation) []axio.Annotation {
//	    return append(target,
//	        axio.Field("method", h.Method),
//	        axio.Field("url", h.URL),
//	        axio.Field("status_code", h.StatusCode),
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
//	logger.Info(ctx, "request processed", axio.Field("http", http))
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
		Field("method", h.Method),
		Field("url", h.URL),
		Field("status_code", h.StatusCode),
		Field("latency", h.LatencyMS),
		Field("user_agent", h.UserAgent),
		Field("client_ip", h.ClientIP),
	)
}

// uncommonField covers the primitive types [Field] does not switch on
// directly, falling back to interface boxing for everything else. The split
// keeps the common types on a single type switch.
func uncommonField[T any](key string, value T) Annotation {
	switch concrete := any(value).(type) {
	case int8:
		return Annotation{field: zap.Int8(key, concrete)}
	case int16:
		return Annotation{field: zap.Int16(key, concrete)}
	case uint8:
		return Annotation{field: zap.Uint8(key, concrete)}
	case uint16:
		return Annotation{field: zap.Uint16(key, concrete)}
	case uint32:
		return Annotation{field: zap.Uint32(key, concrete)}
	case float32:
		return Annotation{field: zap.Float32(key, concrete)}
	default:
		return Annotation{field: zap.Any(key, value)}
	}
}

// convertNumber sets target to number when target is a number type of the same
// kind that holds it exactly, and reports whether it did. The reflect.Value
// names the one boundary where the type of the target is known only to the
// caller of [Annotation.Value].
func convertNumber(number any, target reflect.Value) bool {
	switch typed := number.(type) {
	case int64:
		return setNumber(target.CanInt, target.OverflowInt, target.SetInt, typed)
	case uint64:
		return setNumber(target.CanUint, target.OverflowUint, target.SetUint, typed)
	case float64:
		return setNumber(target.CanFloat, target.OverflowFloat, target.SetFloat, typed)
	default:
		return false
	}
}

// setNumber sets number through set when the target is of its kind, as can
// reports, and holds it exactly, as overflows reports.
func setNumber[N int64 | uint64 | float64](can func() bool, overflows func(N) bool, set func(N), number N) bool {
	if !can() || overflows(number) {
		return false
	}
	set(number)
	return true
}

// givenSlice returns a slice that zap keeps in one of its own array types — a
// []int as its ints, a []string as its stringArray — as the slice it was
// given, and any other value as it is. The reflect.Value names the boundary
// where zap's unexported types meet the caller's.
func givenSlice(value any) any {
	array := reflect.ValueOf(value)
	if array.Kind() != reflect.Slice || array.Type().PkgPath() != zapArrayPackage {
		return value
	}
	return array.Convert(reflect.SliceOf(array.Type().Elem())).Interface()
}

// zapArrayPackage is the package of the array types zap.Any keeps a slice of a
// basic type in.
var zapArrayPackage = reflect.TypeOf(zap.Bools("", nil).Interface).PkgPath()
