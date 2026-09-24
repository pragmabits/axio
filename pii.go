package axio

import (
	"bytes"
	"context"
	"encoding"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"go.uber.org/zap/zapcore"

	"github.com/pragmabits/axio/internal/logline"
)

// DefaultPIIMaxDepth is the depth used when [PIIConfig.MaxDepth] is zero.
//
// Masking cost follows the size of a value, not its depth, so the limit only
// contains pathological nesting.
const DefaultPIIMaxDepth = 32

// redacted replaces a value whose name is sensitive, or that sits deeper than
// [PIIConfig.MaxDepth].
const redacted = "[REDACTED]"

// minimumPatternDigits is the fewest digits a built-in pattern matches: the
// eight of a phone number without its area code. The one pattern that matches
// with fewer, an e-mail address, needs an "@".
const minimumPatternDigits = 8

// PIIPattern identifies a type of personally identifiable information.
//
// Each pattern corresponds to a regular expression that detects the specific
// format of sensitive personal data and its replacement mask.
type PIIPattern string

const (
	// PatternCPF detects Brazilian CPF numbers.
	// Formats: 123.456.789-01 or 12345678901
	// Mask: ***.***.***-**
	PatternCPF PIIPattern = "cpf"
	// PatternCNPJ detects Brazilian CNPJ numbers.
	// Format: 12.345.678/0001-90
	// Mask: **.***.***/****-**
	PatternCNPJ PIIPattern = "cnpj"
	// PatternCreditCard detects credit card numbers.
	// Formats: 1234-5678-9012-3456 or 1234567890123456
	// Mask: ****-****-****-****
	PatternCreditCard PIIPattern = "credit_card"
	// PatternEmail detects email addresses.
	// Mask: ***@***.***
	PatternEmail PIIPattern = "email"
	// PatternPhone detects Brazilian phone numbers.
	// Formats with area code: (11) 99999-9999, 11 99999-9999, 11999999999
	// Formats without area code: 99999-9999, 999999999
	// Mask: (**) *****-**** (with area code) or *****-**** (without area code)
	PatternPhone PIIPattern = "phone"
	// PatternPhoneNoDDD detects Brazilian phone numbers without area code.
	// Formats: 99999-9999, 999999999, 9999-9999, 99999999
	// Mask: *****-****
	PatternPhoneNoDDD PIIPattern = "phone_no_ddd"
)

// PIIRedaction is why a value was replaced by "[REDACTED]" whole, rather than
// having its PII patterns masked.
type PIIRedaction string

const (
	// RedactionField is a value whose name matches [PIIConfig.Fields].
	RedactionField PIIRedaction = "field"
	// RedactionDepth is a container nested deeper than [PIIConfig.MaxDepth].
	RedactionDepth PIIRedaction = "depth"
	// RedactionToken is a JWT or JWE, compact or in JSON serialization.
	RedactionToken PIIRedaction = "token"
	// RedactionBinary is a []byte, a byte array or a named byte-slice type that
	// is not UTF-8 text.
	RedactionBinary PIIRedaction = "binary"
)

// CustomPII defines a custom PII pattern via regex.
//
// Allows adding domain-specific patterns that are not
// covered by the builtin patterns (CPF, CNPJ, etc).
//
// YAML example:
//
//	piiCustomPatterns:
//	  - name: matricula
//	    pattern: "MAT-\\d{6}"
//	    mask: "MAT-******"
//	  - name: protocolo
//	    pattern: "PROT-[A-Z]{2}\\d{8}"
//	    mask: "PROT-**********"
type CustomPII struct {
	// Name identifies the pattern for logs and metrics.
	Name string `json:"name" yaml:"name" toml:"name" mapstructure:"name"`
	// Pattern is the regular expression for detecting sensitive data.
	// Use Go's RE2 syntax (https://pkg.go.dev/regexp/syntax).
	Pattern string `json:"pattern" yaml:"pattern" toml:"pattern" mapstructure:"pattern"`
	// Mask is the string that replaces matches of the pattern.
	Mask string `json:"mask" yaml:"mask" toml:"mask" mapstructure:"mask"`
}

// PIIConfig configures PII masking behavior.
type PIIConfig struct {
	// Patterns specifies which builtin PII patterns to detect and mask.
	Patterns []PIIPattern
	// CustomPatterns allows defining additional patterns via regex.
	CustomPatterns []CustomPII
	// Fields specifies field names whose values should be redacted.
	// Matching is case-insensitive and uses partial matching.
	Fields []string
	// MaxDepth caps how deep masking walks into a structured annotation value —
	// a map, slice or struct — the annotation's own value being depth 1. A
	// container nested deeper is replaced by "[REDACTED]" whole, never written
	// unmasked. A value of zero falls back to [DefaultPIIMaxDepth] (32).
	MaxDepth int `json:"maxDepth,omitempty" yaml:"maxDepth,omitempty" toml:"maxDepth,omitempty" mapstructure:"maxDepth,omitempty"`
	// OmitErrorVerbose omits the verbose form of an error that formats itself —
	// the %+v zap writes beside its message, a stack trace for many errors —
	// instead of masking it. The error is written by its masked message only,
	// and its verbose form is never read. Off by default: the verbose form is
	// masked and kept, at the cost of scanning it.
	OmitErrorVerbose bool `json:"omitErrorVerbose,omitempty" yaml:"omitErrorVerbose,omitempty" toml:"omitErrorVerbose,omitempty" mapstructure:"omitErrorVerbose,omitempty"`
}

// DefaultPIIConfig returns a configuration with common patterns enabled.
//
// Enabled patterns: CPF, CNPJ, credit card.
// Sensitive fields: password, token, secret, api_key, etc.
//
// Example:
//
//	hook := axio.MustPIIHook(axio.DefaultPIIConfig())
//	logger, _ := axio.New(settings, axio.WithHooks(hook))
func DefaultPIIConfig() PIIConfig {
	return PIIConfig{
		Patterns: []PIIPattern{
			PatternCPF,
			PatternCNPJ,
			PatternCreditCard,
		},
		Fields:   DefaultSensitiveFields(),
		MaxDepth: DefaultPIIMaxDepth,
	}
}

// DefaultSensitiveFields returns the default list of field names that are
// automatically redacted.
//
// Matching is case-insensitive and uses partial matching
// (e.g., "user_password" matches "password").
//
// A fresh copy is returned on every call so callers cannot mutate the package
// default. The returned slice may be modified freely.
//
// Example:
//
//	fields := append(axio.DefaultSensitiveFields(), "cpf_titular")
//	logger, err := axio.New(config, axio.WithPII(nil, fields))
func DefaultSensitiveFields() []string {
	out := make([]string, len(defaultSensitiveFields))
	copy(out, defaultSensitiveFields)
	return out
}

// PIIMaskResult contains the result of PII masking with match counts.
type PIIMaskResult struct {
	// Masked is the string with PII masked.
	Masked string
	// Matches maps each pattern to the number of occurrences found.
	Matches map[PIIPattern]int
}

// PIIMasker masks personally identifiable information in strings and annotations.
//
// The masker detects configured patterns (CPF, CNPJ, credit card,
// etc.) and replaces them with masks that preserve the format but hide
// sensitive data.
//
// In addition to regex patterns, the masker also redacts fields whose names
// contain sensitive terms like "password", "token", "api_key", etc.
//
// Recommended usage is via [PIIHook], which applies masking
// automatically to all log entries:
//
//	hook := axio.MustPIIHook(axio.DefaultPIIConfig())
//	logger, _ := axio.New(config, axio.WithHooks(hook))
//
//	// Sensitive data is masked automatically
//	logger.Info(ctx, "Customer CPF 123.456.789-01")
//	// Output: "Customer CPF ***.***.***-**"
//
// To mask strings directly:
//
//	masker := axio.MustPIIMasker(axio.DefaultPIIConfig())
//	msg := masker.MaskString("CPF: 123.456.789-01")
//	// Result: "CPF: ***.***.***-**"
type PIIMasker struct {
	patterns         []piiPatternInfo
	fields           map[string]bool
	maxDepth         int
	omitErrorVerbose bool
	// byteKindTypes holds, per type, what mayHoldByteKinds reported for it.
	byteKindTypes sync.Map
}

// NewPIIMasker creates a new masker with the specified configuration.
//
// Returns an error if any CustomPattern has an invalid regex, and
// [ErrInvalidPIIMaxDepth] for a negative MaxDepth.
//
// Example:
//
//	masker, err := axio.NewPIIMasker(axio.PIIConfig{
//	    Patterns: []axio.PIIPattern{axio.PatternEmail},
//	    CustomPatterns: []axio.CustomPII{
//	        {Name: "matricula", Pattern: `MAT-\d{6}`, Mask: "MAT-******"},
//	    },
//	})
//	if err != nil {
//	    return err
//	}
//	masked := masker.MaskString("contato: ana@example.com, MAT-123456")
//	// masked: "contato: ***@***.***, MAT-******"
func NewPIIMasker(config PIIConfig) (*PIIMasker, error) {
	if config.MaxDepth < 0 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidPIIMaxDepth, config.MaxDepth)
	}
	maxDepth := config.MaxDepth
	if maxDepth == 0 {
		maxDepth = DefaultPIIMaxDepth
	}

	masker := &PIIMasker{
		fields:           make(map[string]bool),
		maxDepth:         maxDepth,
		omitErrorVerbose: config.OmitErrorVerbose,
	}

	for _, pattern := range config.Patterns {
		if info, ok := piiPatterns[pattern]; ok {
			masker.patterns = append(masker.patterns, piiPatternInfo{
				name:  pattern,
				regex: info.regex,
				mask:  info.mask,
			})
		}
	}

	for _, custom := range config.CustomPatterns {
		if custom.Pattern == "" {
			continue
		}
		regex, err := regexp.Compile(custom.Pattern)
		if err != nil {
			return nil, fmt.Errorf("compile PII pattern '%s': %w", custom.Name, err)
		}
		masker.patterns = append(masker.patterns, piiPatternInfo{
			name:   PIIPattern(custom.Name),
			regex:  regex,
			mask:   custom.Mask,
			custom: true,
		})
	}

	for _, field := range config.Fields {
		masker.fields[strings.ToLower(field)] = true
	}

	return masker, nil
}

// MustPIIMasker is like [NewPIIMasker] but panics on error.
//
// Useful for initialization where failure must be fatal.
//
// Example:
//
//	masker := axio.MustPIIMasker(axio.DefaultPIIConfig())
//	result := masker.MaskString("CPF: 123.456.789-01")
func MustPIIMasker(config PIIConfig) *PIIMasker {
	masker, err := NewPIIMasker(config)
	if err != nil {
		panic(err)
	}
	return masker
}

// MaskString masks the PII in input the way [PIIMasker.MaskFields] masks a
// string: the patterns, a string with the shape of base64 by the text it
// decodes to, and every JWT or JWE, which becomes "[REDACTED]" whole.
//
// Example:
//
//	masker := axio.MustPIIMasker(axio.DefaultPIIConfig())
//	result := masker.MaskString("CPF: 123.456.789-01, Card: 1234-5678-9012-3456")
//	// Result: "CPF: ***.***.***-**, Card: ****-****-****-****"
func (m *PIIMasker) MaskString(input string) string {
	masked, _ := m.maskString(input, nil)
	return masked
}

// MaskFields masks sensitive values in an annotations slice in place.
//
// Sensitivity is matched two ways:
//   - Names are checked case-insensitively against [PIIConfig.Fields]; a match
//     replaces the whole value with "[REDACTED]". Inside a structured value,
//     every key is checked the same way.
//   - Text is scanned for PII patterns via [PIIMasker.MaskString].
//
// Every value is covered as it will be written: a string; an error, by its
// message; a fmt.Stringer, by its text; a []byte, a byte array or a named
// byte-slice type, which are written as base64, by the text they hold — bytes
// that are not UTF-8 text cannot be inspected and become "[REDACTED]", inside a
// structured value too; and a structured value — map, slice, struct or pointer
// — walked as the JSON encoding the log writes for it.
//
// A string with the shape of base64 — the standard or the URL alphabet, padded
// or not — is also decoded, and masked when the text it decodes to carries PII:
// that is how bytes of text arrive in the JSON encoding of a structured value,
// and how a caller may have encoded bytes itself. A string that decodes to
// something other than text passes as it is, since nothing tells the base64 of
// binary data from any other string of that shape. A JWT or JWE, anywhere in a
// text, becomes "[REDACTED]" whole: it is a credential, and its claims may
// carry what no pattern knows. So does a JWS or JWE in JSON serialization — an
// object with a payload and its signature or signatures, or with a ciphertext
// and its iv — inside a structured value; written as text, it passes. A structured
// value that needed masking is replaced by its masked JSON tree, objects as
// map[string]any, so its keys are then written in alphabetical order; one that
// did not keeps its original type. A container nested deeper than
// [PIIConfig.MaxDepth] is replaced by "[REDACTED]" whole.
//
// A masked error is written by its masked message only, and still unwraps to
// the original, so errors.Is and errors.As keep working for later hooks.
//
// A logger expands [Annotable] values into their fields before any hook runs,
// so each field is masked on its own.
func (m *PIIMasker) MaskFields(fields Annotations) {
	m.maskAnnotations(fields, nil)
}

// MaskStringWithCounts masks input as [PIIMasker.MaskString] does and returns
// how many times each pattern matched, inside base64 included.
//
// Useful when you need to know which patterns were detected and how many times.
//
// Example:
//
//	masker := axio.MustPIIMasker(axio.DefaultPIIConfig())
//	result := masker.MaskStringWithCounts("CPF: 123.456.789-01")
//	// result.Masked: "CPF: ***.***.***-**"
//	// result.Matches: {"cpf": 1}
func (m *PIIMasker) MaskStringWithCounts(input string) PIIMaskResult {
	counter := &piiCounter{}
	masked, _ := m.maskString(input, counter)
	return PIIMaskResult{Masked: masked, Matches: counter.patterns()}
}

// MaskFieldsWithCounts masks sensitive values and returns match counts.
//
// Coverage is identical to [PIIMasker.MaskFields].
//
// Returns a map with the count of each pattern detected across all
// processed fields, including matches found inside structured values.
func (m *PIIMasker) MaskFieldsWithCounts(fields Annotations) map[PIIPattern]int {
	counter := &piiCounter{}
	m.maskAnnotations(fields, counter)
	return counter.patterns()
}

// maskAnnotations masks every annotation in place, counting the pattern
// matches and the redactions under each annotation's key when counter is not
// nil.
func (m *PIIMasker) maskAnnotations(fields Annotations, counter *piiCounter) {
	for index := range fields {
		counter.at(logline.FieldKey(fields[index].Name()))
		if masked, changed := m.maskAnnotation(fields[index], counter); changed {
			fields[index] = masked
		}
	}
}

// maskAnnotation returns the annotation with its value masked, and whether
// anything changed; when nothing did, the caller keeps the annotation it has.
func (m *PIIMasker) maskAnnotation(annotation Annotation, counter *piiCounter) (Annotation, bool) {
	name := annotation.Name()
	if m.isSensitiveField(name) {
		counter.redacted(RedactionField, 1)
		return Field(name, redacted), true
	}

	field := annotation.field
	switch field.Type {
	case zapcore.StringType:
		masked, changed := m.maskString(field.String, counter)
		return Field(name, masked), changed
	case zapcore.StringerType:
		masked, changed := m.maskString(fmt.Sprint(field.Interface), counter)
		return Field(name, masked), changed
	case zapcore.ErrorType:
		err, _ := field.Interface.(error)
		masked := m.maskError(err, counter)
		if masked == nil {
			return annotation, false
		}
		return Field(name, masked), true
	case zapcore.BinaryType:
		data, _ := field.Interface.([]byte)
		masked, changed := m.maskBytes(data, counter)
		return Field(name, masked), changed
	case zapcore.ReflectType, zapcore.ObjectMarshalerType, zapcore.ArrayMarshalerType:
		return m.maskStructured(annotation, counter)
	default:
		return annotation, false
	}
}

// maskStructured masks a structured annotation as the encoding zap writes for
// it. One whose encoding failed is always replaced, so zap never encodes the
// original again and never writes its error unmasked: a marshaler that failed
// becomes its masked partial value failing with its masked error, and a value
// that failed to encode as JSON becomes its masked error under the name
// followed by "Error", the key zap writes an encoding error under.
func (m *PIIMasker) maskStructured(annotation Annotation, counter *piiCounter) (Annotation, bool) {
	name := annotation.Name()
	value, failure, ok := encodedValue(annotation.field)
	if !ok {
		return annotation, false
	}
	masked, changed := m.maskValue(value, 1, counter)
	if failure != "" {
		message, _ := m.maskString(failure, counter)
		return Field(name, failedEncodingOf(masked, message)), true
	}
	if failed, ok := masked.(encodingFailure); ok {
		return Field(name+"Error", failed.message), true
	}
	return Field(name, masked), changed
}

// maskValue returns value with its sensitive entries masked, and whether
// anything changed. depth is how deep value sits in the annotation, the
// annotation's own value being 1.
func (m *PIIMasker) maskValue(value any, depth int, counter *piiCounter) (any, bool) {
	switch typed := value.(type) {
	case nil, bool, jsontext.Value, time.Time, time.Duration,
		int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr,
		float32, float64, complex64, complex128:
		return value, false
	case string:
		return m.maskString(typed, counter)
	case []byte:
		return m.maskBytes(typed, counter)
	case map[string]any:
		if isJOSEObject(typed) {
			counter.redacted(RedactionToken, 1)
			return redacted, true
		}
		if depth > m.maxDepth {
			counter.redacted(RedactionDepth, 1)
			return redacted, true
		}
		return m.maskObject(typed, depth, counter)
	case []any:
		if depth > m.maxDepth {
			counter.redacted(RedactionDepth, 1)
			return redacted, true
		}
		return m.maskArray(typed, depth, counter)
	default:
		return m.maskEncoded(value, depth, counter)
	}
}

// maskEncoded masks value as its JSON encoding, and keeps value itself when the
// encoding had nothing to mask. A value that fails to encode becomes an
// [encodingFailure] carrying the error masked.
func (m *PIIMasker) maskEncoded(value any, depth int, counter *piiCounter) (any, bool) {
	tree, redactedBinary, err := jsonTree(value, m.mayHoldByteKinds(reflect.TypeOf(value)))
	if err != nil {
		message, _ := m.maskString(err.Error(), counter)
		return encodingFailure{message: message}, true
	}
	masked, changed := m.maskValue(tree, depth, counter)
	counter.redacted(RedactionBinary, redactedBinary)
	if !changed && redactedBinary == 0 {
		return value, false
	}
	return masked, true
}

// mayHoldByteKinds reports whether a value of valueType may hold bytes that the
// encoding writes as base64 and that are not a []byte — a byte array, a named
// byte slice, or anything behind an interface — working it out once per type.
func (m *PIIMasker) mayHoldByteKinds(valueType reflect.Type) bool {
	if valueType == nil {
		return false
	}
	if held, ok := m.byteKindTypes.Load(valueType); ok {
		// Only this method stores into byteKindTypes, and always a bool.
		return held.(bool)
	}
	held := holdsByteKinds(valueType, map[reflect.Type]bool{})
	m.byteKindTypes.Store(valueType, held)
	return held
}

// maskObject returns a copy of object with its sensitive entries masked. The
// object itself is never modified: it may be the caller's own map.
func (m *PIIMasker) maskObject(object map[string]any, depth int, counter *piiCounter) (map[string]any, bool) {
	masked := make(map[string]any, len(object))
	changed := false
	for key, value := range object {
		if m.isSensitiveField(key) {
			counter.redacted(RedactionField, 1)
			masked[key] = redacted
			changed = true
			continue
		}
		maskedValue, valueChanged := m.maskValue(value, depth+1, counter)
		masked[key] = maskedValue
		changed = changed || valueChanged
	}
	return masked, changed
}

// maskArray returns a copy of array with each element masked.
func (m *PIIMasker) maskArray(array []any, depth int, counter *piiCounter) ([]any, bool) {
	masked := make([]any, len(array))
	changed := false
	for index, value := range array {
		maskedValue, valueChanged := m.maskValue(value, depth+1, counter)
		masked[index] = maskedValue
		changed = changed || valueChanged
	}
	return masked, changed
}

// maskString masks text, first as the base64 it may be — how a struct's []byte
// field arrives, and how a caller may have encoded bytes — and then as text.
func (m *PIIMasker) maskString(text string, counter *piiCounter) (string, bool) {
	if masked, ok := m.maskBase64(text, counter); ok {
		return masked, true
	}
	masked := m.maskText(text, counter)
	return masked, masked != text
}

// maskBytes masks bytes that the log writes as base64: text is
// masked as a string would be and stays bytes; bytes that are not UTF-8 text
// cannot be inspected and become redacted.
func (m *PIIMasker) maskBytes(data []byte, counter *piiCounter) (any, bool) {
	if !utf8.Valid(data) {
		counter.redacted(RedactionBinary, 1)
		return redacted, true
	}
	text := string(data)
	masked := m.maskText(text, counter)
	if masked == text {
		return data, false
	}
	return []byte(masked), true
}

// maskBase64 returns the masked text in the encoding it came in when text has
// the shape of base64 — standard or URL alphabet, padded or not — and decodes
// to UTF-8 text carrying PII, redacted when it decodes to bytes that are not
// text but in which a PII pattern matches, and false otherwise: other binary
// data is left alone, since nothing tells the base64 of binary data from any
// other string of that shape.
func (m *PIIMasker) maskBase64(text string, counter *piiCounter) (string, bool) {
	base64Encoding := base64EncodingOf(text)
	if base64Encoding == nil {
		return "", false
	}
	// Short strings, which most values are, decode on the stack.
	var buffer [128]byte
	decoded, err := base64Encoding.AppendDecode(buffer[:0], []byte(text))
	if err != nil {
		return "", false
	}
	if !utf8.Valid(decoded) {
		if m.binaryCarriesPII(decoded, counter) {
			return redacted, true
		}
		return "", false
	}
	plain := string(decoded)
	masked := m.maskText(plain, counter)
	if masked == plain {
		return "", false
	}
	return base64Encoding.EncodeToString([]byte(masked)), true
}

// maskError returns err with the PII in its message masked, or nil when there
// is none. The verbose form of an error that formats itself — the %+v zap
// writes beside the message — is masked too, and kept; with
// [PIIConfig.OmitErrorVerbose] it is never read, and such an error is always
// replaced by its masked message alone.
func (m *PIIMasker) maskError(err error, counter *piiCounter) *maskedError {
	if err == nil {
		return nil
	}
	message, messageChanged := m.maskString(fmt.Sprint(err), counter)
	if _, formats := err.(fmt.Formatter); formats && m.omitErrorVerbose {
		return &maskedError{message: message, cause: err}
	}
	verbose, verboseChanged := m.maskVerbose(err)
	if !messageChanged && !verboseChanged {
		return nil
	}
	return &maskedError{message: message, verbose: verbose, cause: err}
}

// maskVerbose returns the %+v of an error that formats itself with its PII
// masked, and whether anything was; for any other error, it returns "".
func (m *PIIMasker) maskVerbose(err error) (string, bool) {
	if _, ok := err.(fmt.Formatter); !ok {
		return "", false
	}
	return m.maskString(fmt.Sprintf("%+v", err), nil)
}

// maskText redacts every JWT and JWE in text and then masks the PII patterns,
// counting both when counter is not nil.
func (m *PIIMasker) maskText(text string, counter *piiCounter) string {
	if mayHoldToken(text) {
		text = redactTokens(text, counter)
	}
	return m.maskPatterns(text, counter)
}

// binaryCarriesPII reports whether a PII pattern matches binary data, adding
// the matches to counter when counter is not nil. A built-in pattern runs only
// on data that [mayHoldBuiltInPattern] lets through; a custom one always runs.
// The patterns read a copy, so data, a buffer on the caller's stack, never
// escapes to the heap.
func (m *PIIMasker) binaryCarriesPII(data []byte, counter *piiCounter) bool {
	builtIn := mayHoldBuiltInPattern(data)
	var copied []byte
	found := false
	for _, info := range m.patterns {
		if !info.custom && !builtIn {
			continue
		}
		if copied == nil {
			copied = bytes.Clone(data)
		}
		matches := info.regex.FindAllIndex(copied, -1)
		found = found || len(matches) > 0
		counter.matched(info.name, len(matches))
	}
	return found
}

// mayHoldBuiltInPattern reports whether data could hold a built-in PII
// pattern: it has an "@" or at least [minimumPatternDigits] digits.
func mayHoldBuiltInPattern(data []byte) bool {
	digits := 0
	for _, char := range data {
		if char == '@' {
			return true
		}
		if char >= '0' && char <= '9' {
			digits++
		}
	}
	return digits >= minimumPatternDigits
}

// maskPatterns masks every PII pattern in text, adding the matches to counter
// when counter is not nil.
func (m *PIIMasker) maskPatterns(text string, counter *piiCounter) string {
	for _, info := range m.patterns {
		matches := info.regex.FindAllStringIndex(text, -1)
		if len(matches) == 0 {
			continue
		}
		counter.matched(info.name, len(matches))
		text = info.regex.ReplaceAllString(text, info.mask)
	}
	return text
}

// isSensitiveField checks whether the field name matches any sensitive pattern.
func (m *PIIMasker) isSensitiveField(fieldName string) bool {
	for sensitiveField := range m.fields {
		if containsFold(fieldName, sensitiveField) {
			return true
		}
	}
	return false
}

// PIIHook is a hook that masks PII in log entries before they are written.
//
// The hook processes the message, the error and the structured fields,
// detecting and masking PII patterns and sensitive fields.
//
// PIIHook implements [MetricsAware] to emit metrics for masked PII.
//
// Example:
//
//	hook := axio.MustPIIHook(axio.DefaultPIIConfig())
//	logger, _ := axio.New(settings, axio.WithHooks(hook))
//
//	// The message will be masked automatically
//	logger.Info(ctx, "User with CPF 123.456.789-01 authenticated")
//	// Output: "User with CPF ***.***.***-** authenticated"
type PIIHook struct {
	masker  *PIIMasker
	metrics Metrics
	mutex   sync.RWMutex
}

// NewPIIHook creates a new [PIIHook] with the specified configuration.
//
// Returns an error if any CustomPattern has an invalid regex, and
// [ErrInvalidPIIMaxDepth] for a negative MaxDepth.
//
// Use [DefaultPIIConfig] for a default configuration with the most
// common patterns enabled.
//
// Example:
//
//	hook, err := axio.NewPIIHook(axio.DefaultPIIConfig())
//	if err != nil {
//	    return err
//	}
//	logger, _ := axio.New(config, axio.WithHooks(hook))
func NewPIIHook(config PIIConfig) (*PIIHook, error) {
	masker, err := NewPIIMasker(config)
	if err != nil {
		return nil, fmt.Errorf("create PII masker: %w", err)
	}
	return &PIIHook{masker: masker}, nil
}

// MustPIIHook is like [NewPIIHook] but panics on error.
//
// Useful for initialization where failure must be fatal.
//
// Example:
//
//	hook := axio.MustPIIHook(axio.DefaultPIIConfig())
//	logger, _ := axio.New(config, axio.WithHooks(hook))
func MustPIIHook(config PIIConfig) *PIIHook {
	hook, err := NewPIIHook(config)
	if err != nil {
		panic(err)
	}
	return hook
}

// Name returns the hook identifier.
func (p *PIIHook) Name() string {
	return "pii"
}

// SetMetrics implements [MetricsAware].
//
// When configured, the hook reports each PII pattern it masks in an entry,
// once, with the number of occurrences, and each value it redacts whole, by
// [PIIRedaction].
func (p *PIIHook) SetMetrics(metrics Metrics) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.metrics = metrics
}

// Process masks PII in the log entry.
//
// Modifies the message, the error and the fields in-place, replacing PII
// patterns with masks and sensitive fields with "[REDACTED]", as
// [PIIMasker.MaskFields] describes.
//
// If metrics is configured via [PIIHook.SetMetrics], reports each pattern
// found once per entry, with how many occurrences of it were masked, and each
// value redacted whole, by [PIIRedaction].
func (p *PIIHook) Process(ctx context.Context, entry *Entry) error {
	p.mutex.RLock()
	metrics := p.metrics
	p.mutex.RUnlock()

	// Without metrics nobody reads the counts, so nothing is counted.
	var counter *piiCounter
	if metrics != nil {
		counter = &piiCounter{}
	}
	p.masker.maskAnnotations(entry.Annotations, counter)
	counter.at(logline.ErrorKey)
	if masked := p.masker.maskError(entry.Error, counter); masked != nil {
		entry.Error = masked
	}
	counter.at(logline.MessageKey)
	entry.Message, _ = p.masker.maskString(entry.Message, counter)

	if metrics == nil {
		return nil
	}

	for count, occurrences := range counter.counts {
		origin := PIIOrigin{Annotation: count.annotation, Logger: entry.Logger}
		if count.pattern != "" {
			metrics.PIIMasked(ctx, count.pattern, origin, occurrences)
			continue
		}
		metrics.PIIRedacted(ctx, count.redaction, origin, occurrences)
	}

	return nil
}

// maskedError is an error whose message had PII masked. zap writes its message
// and, for an original that formats itself, its masked verbose form; it unwraps
// to the original error for the hooks that run after PII.
type maskedError struct {
	message string
	verbose string
	cause   error
}

func (m maskedError) Error() string { return m.message }

// Format writes the masked verbose form for %+v, when the original has one,
// and the masked message otherwise. A Format method has no caller to return a
// write error to.
func (m maskedError) Format(state fmt.State, verb rune) {
	switch {
	case verb == 'v' && state.Flag('+') && m.verbose != "":
		_, _ = io.WriteString(state, m.verbose)
	case verb == 'q':
		_, _ = fmt.Fprintf(state, "%q", m.message)
	default:
		_, _ = io.WriteString(state, m.message)
	}
}

func (m maskedError) Unwrap() error { return m.cause }

// piiCounter tallies what masking does to one entry — the pattern matches it
// masks and the values it redacts whole — each under the annotation it happened
// in. Its map is made at the first count, so an entry with nothing to count
// allocates nothing. A nil counter counts nothing.
type piiCounter struct {
	annotation string
	counts     map[piiCount]int
}

// piiCount is what a piiCounter tallies under: a pattern or a redaction reason,
// and the annotation.
type piiCount struct {
	annotation string
	pattern    PIIPattern
	redaction  PIIRedaction
}

// at sets the annotation what is counted next happens in.
func (p *piiCounter) at(annotation string) {
	if p != nil {
		p.annotation = annotation
	}
}

// matched counts matches of pattern.
func (p *piiCounter) matched(pattern PIIPattern, matches int) {
	if p != nil && matches > 0 {
		p.add(piiCount{annotation: p.annotation, pattern: pattern}, matches)
	}
}

// redacted counts values redacted whole for reason.
func (p *piiCounter) redacted(reason PIIRedaction, values int) {
	if p != nil && values > 0 {
		p.add(piiCount{annotation: p.annotation, redaction: reason}, values)
	}
}

// add adds occurrences to count, making the map the first time.
func (p *piiCounter) add(count piiCount, occurrences int) {
	if p.counts == nil {
		p.counts = make(map[piiCount]int)
	}
	p.counts[count] += occurrences
}

// patterns returns the matches of each pattern, whatever the annotation.
func (p *piiCounter) patterns() map[PIIPattern]int {
	matches := make(map[PIIPattern]int)
	for count, occurrences := range p.counts {
		if count.pattern != "" {
			matches[count.pattern] += occurrences
		}
	}
	return matches
}

// failedObject is the masked partial object of an ObjectMarshaler that failed:
// it writes the fields and fails with the masked message.
type failedObject struct {
	fields  map[string]any
	message string
}

func (f failedObject) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	for _, key := range slices.Sorted(maps.Keys(f.fields)) {
		if err := encoder.AddReflected(key, f.fields[key]); err != nil {
			return err
		}
	}
	return maskedError{message: f.message}
}

// failedArray is the masked partial array of an ArrayMarshaler that failed: it
// writes the elements and fails with the masked message.
type failedArray struct {
	elements []any
	message  string
}

func (f failedArray) MarshalLogArray(encoder zapcore.ArrayEncoder) error {
	for _, element := range f.elements {
		if err := encoder.AppendReflected(element); err != nil {
			return err
		}
	}
	return maskedError{message: f.message}
}

// encodingFailure stands in for a value that failed to encode as JSON. It fails
// to encode in turn, with the original's error masked, so the error zap writes
// for whatever holds it carries no PII.
type encodingFailure struct {
	message string
}

func (e encodingFailure) MarshalJSON() ([]byte, error) {
	return nil, maskedError{message: e.message}
}

// containsFold reports whether value contains target, case-insensitively, without allocating.
// It iterates by byte offset, which is correct for ASCII substrings but may miss
// matches involving multi-byte Unicode case folding where byte lengths differ.
func containsFold(value, target string) bool {
	if len(target) == 0 {
		return true
	}
	if len(target) > len(value) {
		return false
	}
	for index := 0; index <= len(value)-len(target); index++ {
		if strings.EqualFold(value[index:index+len(target)], target) {
			return true
		}
	}
	return false
}

// encodedValue returns the value zap encodes for field: an ObjectMarshaler or
// ArrayMarshaler as the map or slice it marshals to — only what it chooses to
// write — and anything else as the value itself. failure is the message of the
// error a marshaler failed with, which zap writes under the field's name
// followed by "Error".
func encodedValue(field zapcore.Field) (value any, failure string, ok bool) {
	if field.Type == zapcore.ReflectType {
		return field.Interface, "", true
	}
	encoder := zapcore.NewMapObjectEncoder()
	field.AddTo(encoder)
	value, ok = encoder.Fields[field.Key]
	failure, _ = encoder.Fields[field.Key+"Error"].(string)
	return value, failure, ok
}

// failedEncodingOf returns the masked partial value of a marshaler that failed,
// as a marshaler failing in turn with message, so zap writes both the way it
// wrote the original's.
func failedEncodingOf(value any, message string) any {
	if elements, ok := value.([]any); ok {
		return failedArray{elements: elements, message: message}
	}
	fields, _ := value.(map[string]any)
	return failedObject{fields: fields, message: message}
}

// base64EncodingOf returns the base64 encoding text has the shape of — the
// standard or the URL alphabet, padded or not — or nil when it has none. It
// spares a decode attempt to most strings. Text of letters and digits alone
// fits both alphabets, and both decode it the same way.
func base64EncodingOf(text string) *base64.Encoding {
	body := strings.TrimRight(text, "=")
	padded := len(body) < len(text)
	if !base64Length(len(text), len(body), padded) {
		return nil
	}
	url, ok := base64Alphabet(body)
	switch {
	case !ok:
		return nil
	case url && padded:
		return base64.URLEncoding
	case url:
		return base64.RawURLEncoding
	case padded:
		return base64.StdEncoding
	default:
		return base64.RawStdEncoding
	}
}

// base64Length reports whether a text of length, bodyLength of it before any
// padding, can be base64: padded to a multiple of four with at most two '=',
// or unpadded with a length some input encodes to.
func base64Length(length, bodyLength int, padded bool) bool {
	if bodyLength == 0 {
		return false
	}
	if padded {
		return length%4 == 0 && length-bodyLength <= 2
	}
	return bodyLength%4 != 1
}

// base64Alphabet reports, in one pass, whether body is written in a base64
// alphabet and whether that alphabet is the URL one. Body may not mix the two.
func base64Alphabet(body string) (url, ok bool) {
	standard := false
	for index := 0; index < len(body); index++ {
		switch char := body[index]; {
		case char == '+' || char == '/':
			standard = true
		case char == '-' || char == '_':
			url = true
		case !isAlphanumeric(char):
			return false, false
		}
	}
	return url, !standard || !url
}

// isAlphanumeric reports whether char is an ASCII letter or digit.
func isAlphanumeric(char byte) bool {
	return char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9'
}

// jsonTree returns value as the JSON tree it encodes to — map[string]any,
// []any, string, jsontext.Value for a number, bool or nil — in the encoding the
// log writes it with, the bytes that are not UTF-8 text redacted on the way,
// and how many were: once encoded, those bytes cannot be told from a string. A
// []byte is always redacted; a byte array or a named byte slice only when
// byteKinds is set, which costs an allocation for every value encoded.
func jsonTree(value any, byteKinds bool) (tree any, redactedBinary int, err error) {
	redaction := binaryRedactions.Get().(*binaryRedaction)
	defer binaryRedactions.Put(redaction)
	redaction.redacted = 0
	options := redaction.options
	if byteKinds {
		options = redaction.byteKindOptions
	}
	encoded, err := json.Marshal(value, options)
	if err != nil {
		return nil, 0, err
	}
	tree, err = decodeJSON(encoded)
	return tree, redaction.redacted, err
}

// decodeJSON returns the JSON value data holds as a tree, each number kept as
// the jsontext.Value it was written as so none loses precision on the way back
// out.
func decodeJSON(data []byte) (any, error) {
	var tree any
	if err := json.Unmarshal(data, &tree, treeDecoding); err != nil {
		return nil, err
	}
	return tree, nil
}

// binaryRedaction redacts, in one encoding at a time, the bytes that are not
// UTF-8 text, and records how many it did.
type binaryRedaction struct {
	// options redact a []byte.
	options json.Options
	// byteKindOptions redact any bytes the encoding writes as base64. The
	// encoding hands their marshaler every value, each converted to an
	// interface, which allocates.
	byteKindOptions json.Options
	redacted        int
}

// newBinaryRedaction returns a binaryRedaction with its options built, once
// for every encoding it serves.
func newBinaryRedaction() *binaryRedaction {
	redaction := &binaryRedaction{}
	redaction.options = logline.ValueOptions(json.MarshalToFunc(redaction.marshalBytes))
	redaction.byteKindOptions = logline.ValueOptions(json.MarshalToFunc(redaction.marshalByteKinds))
	return redaction
}

// marshalBytes writes data as redacted when it is not UTF-8 text, and leaves
// text to the default encoding.
func (b *binaryRedaction) marshalBytes(encoder *jsontext.Encoder, data []byte) error {
	if utf8.Valid(data) {
		return errors.ErrUnsupported
	}
	b.redacted++
	return encoder.WriteToken(jsontext.String(redacted))
}

// marshalByteKinds writes the value pointer points to as [binaryRedaction.marshalBytes]
// does when it is bytes the encoding writes as base64, and leaves anything else
// to the default encoding.
func (b *binaryRedaction) marshalByteKinds(encoder *jsontext.Encoder, pointer any) error {
	if data, ok := pointer.(*[]byte); ok {
		return b.marshalBytes(encoder, *data)
	}
	value := reflect.ValueOf(pointer).Elem()
	if !isBase64Bytes(value.Type()) {
		return errors.ErrUnsupported
	}
	return b.marshalBytes(encoder, value.Bytes())
}

// holdsByteKinds reports whether a value of valueType may hold bytes the
// encoding writes as base64 other than a []byte, which the cheaper options
// already redact. seen holds the types already on the way down, so a recursive
// type ends.
func holdsByteKinds(valueType reflect.Type, seen map[reflect.Type]bool) bool {
	if seen[valueType] || hasMarshalingMethod(valueType) {
		return false
	}
	seen[valueType] = true
	switch valueType.Kind() {
	case reflect.Interface:
		return true
	case reflect.Pointer, reflect.Map:
		return holdsByteKinds(valueType.Elem(), seen)
	case reflect.Slice, reflect.Array:
		if isBase64Bytes(valueType) {
			return valueType != reflect.TypeFor[[]byte]()
		}
		return holdsByteKinds(valueType.Elem(), seen)
	case reflect.Struct:
		return fieldsHoldByteKinds(valueType, seen)
	default:
		return false
	}
}

// fieldsHoldByteKinds reports whether a field of structType that the encoding
// writes may hold bytes, as [holdsByteKinds] does. An unexported field is never
// written, but an embedded one's fields are.
func fieldsHoldByteKinds(structType reflect.Type, seen map[reflect.Type]bool) bool {
	for index := range structType.NumField() {
		field := structType.Field(index)
		if !field.IsExported() && !field.Anonymous {
			continue
		}
		if holdsByteKinds(field.Type, seen) {
			return true
		}
	}
	return false
}

// isBase64Bytes reports whether the encoding writes a value of valueType as
// base64: a slice or an array of byte, named or not, with no marshaling method
// of its own. A slice or an array of a named byte type is written as numbers.
func isBase64Bytes(valueType reflect.Type) bool {
	kind := valueType.Kind()
	if kind != reflect.Slice && kind != reflect.Array {
		return false
	}
	element := valueType.Elem()
	if element.Kind() != reflect.Uint8 || element.PkgPath() != "" {
		return false
	}
	return !hasMarshalingMethod(valueType)
}

// hasMarshalingMethod reports whether valueType encodes itself, which the
// encoding prefers to its default for the kind.
func hasMarshalingMethod(valueType reflect.Type) bool {
	methods := reflect.PointerTo(valueType)
	for _, method := range marshalingMethods {
		if methods.Implements(method) {
			return true
		}
	}
	return false
}

// mayHoldToken reports whether text holds what the header of a JWT or JWE
// starts with: "{" followed by a quote, a space or a line break, in base64url.
// It spares the token pattern to most text.
func mayHoldToken(text string) bool {
	for rest := text; ; {
		index := strings.IndexByte(rest, 'e')
		if index < 0 || index+2 >= len(rest) {
			return false
		}
		switch rest[index+1 : index+3] {
		case "yJ", "yI", "yA", "wo", "wk", "w0":
			return true
		}
		rest = rest[index+1:]
	}
}

// redactTokens replaces every JWT and JWE in text with redacted, counting each.
func redactTokens(text string, counter *piiCounter) string {
	matches := tokenPattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var builder strings.Builder
	last := 0
	for _, match := range matches {
		candidate := text[match[0]:match[1]]
		builder.WriteString(text[last:match[0]])
		if isToken(candidate) {
			builder.WriteString(redacted)
			counter.redacted(RedactionToken, 1)
		} else {
			builder.WriteString(candidate)
		}
		last = match[1]
	}
	builder.WriteString(text[last:])
	return builder.String()
}

// isToken reports whether candidate, which has the shape of a JWT or JWE, is
// one: a token's header is a JSON object naming its algorithm, as every JOSE
// header does.
func isToken(candidate string) bool {
	header, _, _ := strings.Cut(candidate, ".")
	decoded, err := base64.RawURLEncoding.DecodeString(header)
	if err != nil {
		return false
	}
	tree, err := decodeJSON(decoded)
	if err != nil {
		return false
	}
	fields, _ := tree.(map[string]any)
	_, ok := fields["alg"]
	return ok
}

// isJOSEObject reports whether object is a JWS — a payload beside its signature
// or signatures — or a JWE — a ciphertext beside its initialization vector — in
// JSON serialization, the form of a token that is an object rather than a
// string of dots.
func isJOSEObject(object map[string]any) bool {
	if _, ok := object["payload"]; ok {
		_, flattened := object["signature"]
		_, general := object["signatures"]
		return flattened || general
	}
	_, ciphertext := object["ciphertext"]
	_, vector := object["iv"]
	return ciphertext && vector
}

// piiPatternInfo holds the regex and mask for a PII pattern.
type piiPatternInfo struct {
	name  PIIPattern
	regex *regexp.Regexp
	mask  string
	// custom marks a pattern from [PIIConfig.CustomPatterns], which may match
	// what no built-in one needs.
	custom bool
}

// piiPatterns maps pattern types to their regexes and masks.
var piiPatterns = map[PIIPattern]piiPatternInfo{
	PatternCPF: {
		regex: regexp.MustCompile(`\b\d{3}\.?\d{3}\.?\d{3}-?\d{2}\b`),
		mask:  "***.***.***-**",
	},
	PatternCNPJ: {
		regex: regexp.MustCompile(`\b\d{2}\.?\d{3}\.?\d{3}/?\d{4}-?\d{2}\b`),
		mask:  "**.***.***/****-**",
	},
	PatternCreditCard: {
		regex: regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`),
		mask:  "****-****-****-****",
	},
	PatternEmail: {
		regex: regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`),
		mask:  "***@***.***",
	},
	PatternPhone: {
		regex: regexp.MustCompile(`\(?\d{2}\)?\s?\d{4,5}[\s-]?\d{4}`),
		mask:  "(**) *****-****",
	},
	PatternPhoneNoDDD: {
		regex: regexp.MustCompile(`\b9?\d{4}[\s-]?\d{4}\b`),
		mask:  "*****-****",
	},
}

// tokenPattern matches the shape of a JWT — header, payload and signature, the
// last empty when unsigned — or of a JWE, five segments long. Every segment is
// base64url, and the header, a JSON object, starts as [mayHoldToken] expects,
// wherever it starts: a token glued to a word, as in "session_eyJ…", is
// matched too.
var tokenPattern = regexp.MustCompile(`(?:eyJ|eyI|eyA|ewo|ewk|ew0)[A-Za-z0-9_-]*(?:\.[A-Za-z0-9_-]*){2}(?:(?:\.[A-Za-z0-9_-]*){2})?`)

// binaryRedactions holds the binaryRedaction of every encoding not in progress.
var binaryRedactions = sync.Pool{New: func() any { return newBinaryRedaction() }}

// marshalingMethods are the interfaces through which a type encodes itself.
var marshalingMethods = []reflect.Type{
	reflect.TypeFor[json.MarshalerTo](),
	reflect.TypeFor[json.Marshaler](),
	reflect.TypeFor[encoding.TextAppender](),
	reflect.TypeFor[encoding.TextMarshaler](),
}

// treeDecoding decodes JSON under the options the log writes it with, each
// number into an any kept as the jsontext.Value it was written as.
var treeDecoding = json.JoinOptions(logline.ValueOptions(), json.WithUnmarshalers(json.UnmarshalFromFunc(func(decoder *jsontext.Decoder, value *any) error {
	if decoder.PeekKind() != '0' {
		return errors.ErrUnsupported
	}
	number, err := decoder.ReadValue()
	if err != nil {
		return err
	}
	*value = number.Clone()
	return nil
})))

var defaultSensitiveFields = []string{
	"password", "senha",
	"token", "api_key", "apikey",
	"secret", "credential",
	"authorization", "bearer",
	"private_key", "privatekey",
	"access_key", "secret_key",
	"client_secret", "clientsecret",
}
