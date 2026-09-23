package axio

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"regexp"
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
//	hook := axio.NewPIIHook(axio.DefaultPIIConfig())
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
	patterns []piiPatternInfo
	fields   map[string]bool
	maxDepth int
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
		fields:   make(map[string]bool),
		maxDepth: maxDepth,
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
			name:  PIIPattern(custom.Name),
			regex: regex,
			mask:  custom.Mask,
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

// MaskString replaces PII patterns in the input string with masks.
//
// Example:
//
//	masker := axio.MustPIIMasker(axio.DefaultPIIConfig())
//	result := masker.MaskString("CPF: 123.456.789-01, Card: 1234-5678-9012-3456")
//	// Result: "CPF: ***.***.***-**, Card: ****-****-****-****"
func (m *PIIMasker) MaskString(input string) string {
	result := input
	for _, info := range m.patterns {
		if info.regex.MatchString(result) {
			result = info.regex.ReplaceAllString(result, info.mask)
		}
	}
	return result
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
// message; a fmt.Stringer, by its text; a []byte, which is written as base64,
// by the text it holds — bytes that are not UTF-8 text cannot be inspected and
// become "[REDACTED]", inside a structured value too; and a structured value —
// map, slice, struct or pointer — walked as the JSON encoding the log writes
// for it.
//
// A string with the shape of base64 — the standard or the URL alphabet, padded
// or not — is also decoded, and masked when the text it decodes to carries PII:
// that is how a []byte of text, a byte array or a named byte-slice type arrives
// in the JSON encoding of a structured value, and how a caller may have encoded
// bytes itself. A string that decodes to something other than text passes as
// it is, since nothing tells the base64 of binary data from any other string of
// that shape. The payload of a JWT, anywhere in a text, is decoded and masked as
// the JSON it is, sensitive claims included; the token's signature then no
// longer matches. A structured value that needed masking is replaced by its
// masked JSON tree, objects as map[string]any, so its keys are then written in
// alphabetical order; one that did not keeps its original type. A container
// nested deeper than [PIIConfig.MaxDepth] is replaced by "[REDACTED]" whole.
//
// A masked error is written by its masked message only, and still unwraps to
// the original, so errors.Is and errors.As keep working for later hooks.
//
// A logger expands [Annotable] values into their fields before any hook runs,
// so each field is masked on its own.
func (m *PIIMasker) MaskFields(fields Annotations) {
	m.maskAnnotations(fields, nil)
}

// MaskStringWithCounts replaces PII patterns and returns match counts.
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
	result := PIIMaskResult{
		Masked:  input,
		Matches: make(map[PIIPattern]int),
	}

	for _, info := range m.patterns {
		indices := info.regex.FindAllStringIndex(result.Masked, -1)
		if len(indices) > 0 {
			result.Matches[info.name] = len(indices)
			result.Masked = info.regex.ReplaceAllString(result.Masked, info.mask)
		}
	}

	return result
}

// MaskFieldsWithCounts masks sensitive values and returns match counts.
//
// Coverage is identical to [PIIMasker.MaskFields].
//
// Returns a map with the count of each pattern detected across all
// processed fields, including matches found inside structured values.
func (m *PIIMasker) MaskFieldsWithCounts(fields Annotations) map[PIIPattern]int {
	matches := make(map[PIIPattern]int)
	m.maskAnnotations(fields, matches)
	return matches
}

// maskAnnotations masks every annotation in place, adding the pattern matches
// to counts when counts is not nil.
func (m *PIIMasker) maskAnnotations(fields Annotations, counts map[PIIPattern]int) {
	for index := range fields {
		if masked, changed := m.maskAnnotation(fields[index], counts); changed {
			fields[index] = masked
		}
	}
}

// maskAnnotation returns the annotation with its value masked, and whether
// anything changed; when nothing did, the caller keeps the annotation it has.
func (m *PIIMasker) maskAnnotation(annotation Annotation, counts map[PIIPattern]int) (Annotation, bool) {
	name := annotation.Name()
	if m.isSensitiveField(name) {
		return Annotate(name, redacted), true
	}

	field := annotation.field
	switch field.Type {
	case zapcore.StringType:
		masked, changed := m.maskString(field.String, counts)
		return Annotate(name, masked), changed
	case zapcore.StringerType:
		masked, changed := m.maskString(fmt.Sprint(field.Interface), counts)
		return Annotate(name, masked), changed
	case zapcore.ErrorType:
		err, _ := field.Interface.(error)
		masked := m.maskError(err, counts)
		if masked == nil {
			return annotation, false
		}
		return Annotate(name, masked), true
	case zapcore.BinaryType:
		data, _ := field.Interface.([]byte)
		masked, changed := m.maskBytes(data, counts)
		return Annotate(name, masked), changed
	case zapcore.ReflectType, zapcore.ObjectMarshalerType, zapcore.ArrayMarshalerType:
		value, ok := encodedValue(field)
		if !ok {
			return annotation, false
		}
		masked, changed := m.maskValue(value, 1, counts)
		return Annotate(name, masked), changed
	default:
		return annotation, false
	}
}

// maskValue returns value with its sensitive entries masked, and whether
// anything changed. depth is how deep value sits in the annotation, the
// annotation's own value being 1.
func (m *PIIMasker) maskValue(value any, depth int, counts map[PIIPattern]int) (any, bool) {
	switch typed := value.(type) {
	case nil, bool, jsontext.Value, time.Time, time.Duration,
		int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr,
		float32, float64, complex64, complex128:
		return value, false
	case string:
		return m.maskString(typed, counts)
	case []byte:
		return m.maskBytes(typed, counts)
	case map[string]any:
		if depth > m.maxDepth {
			return redacted, true
		}
		return m.maskObject(typed, depth, counts)
	case []any:
		if depth > m.maxDepth {
			return redacted, true
		}
		return m.maskArray(typed, depth, counts)
	default:
		return m.maskEncoded(value, depth, counts)
	}
}

// maskEncoded masks value as its JSON encoding, and keeps value itself when the
// encoding had nothing to mask.
func (m *PIIMasker) maskEncoded(value any, depth int, counts map[PIIPattern]int) (any, bool) {
	tree, redactedBinary, ok := jsonTree(value)
	if !ok {
		return value, false
	}
	masked, changed := m.maskValue(tree, depth, counts)
	if !changed && !redactedBinary {
		return value, false
	}
	return masked, true
}

// maskObject returns a copy of object with its sensitive entries masked. The
// object itself is never modified: it may be the caller's own map.
func (m *PIIMasker) maskObject(object map[string]any, depth int, counts map[PIIPattern]int) (map[string]any, bool) {
	masked := make(map[string]any, len(object))
	changed := false
	for key, value := range object {
		if m.isSensitiveField(key) {
			masked[key] = redacted
			changed = true
			continue
		}
		maskedValue, valueChanged := m.maskValue(value, depth+1, counts)
		masked[key] = maskedValue
		changed = changed || valueChanged
	}
	return masked, changed
}

// maskArray returns a copy of array with each element masked.
func (m *PIIMasker) maskArray(array []any, depth int, counts map[PIIPattern]int) ([]any, bool) {
	masked := make([]any, len(array))
	changed := false
	for index, value := range array {
		maskedValue, valueChanged := m.maskValue(value, depth+1, counts)
		masked[index] = maskedValue
		changed = changed || valueChanged
	}
	return masked, changed
}

// maskString masks text, first as the base64 it may be — how a struct's []byte
// field arrives, and how a caller may have encoded bytes — and then as text.
func (m *PIIMasker) maskString(text string, counts map[PIIPattern]int) (string, bool) {
	if masked, ok := m.maskBase64(text, counts); ok {
		return masked, true
	}
	masked := m.maskText(text, counts)
	return masked, masked != text
}

// maskBytes masks bytes that the log writes as base64: text is
// masked as a string would be and stays bytes; bytes that are not UTF-8 text
// cannot be inspected and become redacted.
func (m *PIIMasker) maskBytes(data []byte, counts map[PIIPattern]int) (any, bool) {
	if !utf8.Valid(data) {
		return redacted, true
	}
	text := string(data)
	masked := m.maskText(text, counts)
	if masked == text {
		return data, false
	}
	return []byte(masked), true
}

// maskBase64 returns the masked text in the encoding it came in when text has
// the shape of base64 — standard or URL alphabet, padded or not — and decodes
// to UTF-8 text carrying PII, and false otherwise: decoded bytes that are not
// text are left alone, since nothing tells the base64 of binary data from any
// other string of that shape.
func (m *PIIMasker) maskBase64(text string, counts map[PIIPattern]int) (string, bool) {
	encoding := base64EncodingOf(text)
	if encoding == nil {
		return "", false
	}
	// Short strings, which most values are, decode on the stack.
	var buffer [128]byte
	decoded, err := encoding.AppendDecode(buffer[:0], []byte(text))
	if err != nil || !utf8.Valid(decoded) {
		return "", false
	}
	plain := string(decoded)
	masked := m.maskText(plain, counts)
	if masked == plain {
		return "", false
	}
	return encoding.EncodeToString([]byte(masked)), true
}

// maskToken returns a JWT with the PII in its payload masked. The payload is
// JSON, walked like any structured value, so a sensitive claim is redacted and
// every string in it masked; the signature no longer matches, which a copy in a
// log does not need. A token whose payload is not JSON comes back unchanged.
func (m *PIIMasker) maskToken(token string, counts map[PIIPattern]int) string {
	parts := strings.SplitN(token, ".", 3)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return token
	}
	claims, ok := firstValue(payload)
	if !ok {
		return token
	}
	tree, ok := decodeJSON(claims)
	if !ok {
		return token
	}
	masked, changed := m.maskValue(tree, 1, counts)
	if !changed {
		return token
	}
	encoded, err := json.Marshal(masked, logline.ValueOptions())
	if err != nil {
		return redacted
	}
	parts[1] = base64.RawURLEncoding.EncodeToString(encoded)
	return strings.Join(parts, ".")
}

// maskError returns err with the PII in its message masked, or nil when there
// is none. An error whose verbose form — the %+v zap writes beside the message
// of an error that formats itself — carries PII is masked too, and loses that
// verbose form.
func (m *PIIMasker) maskError(err error, counts map[PIIPattern]int) *maskedError {
	if err == nil {
		return nil
	}
	masked, changed := m.maskString(fmt.Sprint(err), counts)
	if !changed && !m.verboseCarriesPII(err) {
		return nil
	}
	return &maskedError{message: masked, cause: err}
}

// verboseCarriesPII reports whether the %+v of an error that formats itself
// has PII to mask.
func (m *PIIMasker) verboseCarriesPII(err error) bool {
	if _, ok := err.(fmt.Formatter); !ok {
		return false
	}
	verbose := fmt.Sprintf("%+v", err)
	return m.MaskString(verbose) != verbose
}

// maskText masks the payload of every JWT in text and then the PII patterns,
// adding the matches to counts when counts is not nil.
func (m *PIIMasker) maskText(text string, counts map[PIIPattern]int) string {
	if strings.Contains(text, ".ey") {
		text = tokenPattern.ReplaceAllStringFunc(text, func(token string) string {
			return m.maskToken(token, counts)
		})
	}
	if counts == nil {
		return m.MaskString(text)
	}
	for _, info := range m.patterns {
		if matches := info.regex.FindAllStringIndex(text, -1); len(matches) > 0 {
			counts[info.name] += len(matches)
			text = info.regex.ReplaceAllString(text, info.mask)
		}
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
// Returns an error if any CustomPattern has an invalid regex.
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
// When configured, the hook emits metrics for each detected PII pattern.
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
// If metrics is configured via [SetMetrics], emits metrics for
// each detected PII occurrence.
func (p *PIIHook) Process(ctx context.Context, entry *Entry) error {
	matches := p.masker.MaskFieldsWithCounts(entry.Annotations)
	if masked := p.masker.maskError(entry.Error, matches); masked != nil {
		entry.Error = masked
	}
	entry.Message, _ = p.masker.maskString(entry.Message, matches)

	p.mutex.RLock()
	metrics := p.metrics
	p.mutex.RUnlock()

	if metrics == nil {
		return nil
	}

	for pattern, count := range matches {
		for range count {
			metrics.PIIMasked(ctx, pattern)
		}
	}

	return nil
}

// maskedError is an error whose message had PII masked. zap writes only its
// message; it unwraps to the original error for the hooks that run after PII.
type maskedError struct {
	message string
	cause   error
}

func (m maskedError) Error() string { return m.message }

func (m maskedError) Unwrap() error { return m.cause }

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
// write — and anything else as the value itself.
func encodedValue(field zapcore.Field) (any, bool) {
	if field.Type == zapcore.ReflectType {
		return field.Interface, true
	}
	encoder := zapcore.NewMapObjectEncoder()
	field.AddTo(encoder)
	value, ok := encoder.Fields[field.Key]
	return value, ok
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
// log writes it with, every []byte that is not UTF-8 text redacted on the way,
// and whether any was: once encoded, those bytes cannot be told from a string.
func jsonTree(value any) (tree any, redactedBinary, ok bool) {
	redaction := binaryRedactions.Get().(*binaryRedaction)
	defer binaryRedactions.Put(redaction)
	redaction.redacted = false
	encoded, err := json.Marshal(value, redaction.options)
	if err != nil {
		return nil, false, false
	}
	tree, ok = decodeJSON(encoded)
	return tree, redaction.redacted, ok
}

// decodeJSON returns the JSON value data holds as a tree, each number kept as
// the jsontext.Value it was written as so none loses precision on the way back
// out.
func decodeJSON(data []byte) (any, bool) {
	var tree any
	if err := json.Unmarshal(data, &tree, treeDecoding); err != nil {
		return nil, false
	}
	return tree, true
}

// binaryRedaction redacts, in one encoding at a time, every []byte that is not
// UTF-8 text, and records whether it did.
type binaryRedaction struct {
	options  json.Options
	redacted bool
}

// newBinaryRedaction returns a binaryRedaction with its options built, once
// for every encoding it serves.
func newBinaryRedaction() *binaryRedaction {
	redaction := &binaryRedaction{}
	redaction.options = logline.ValueOptions(json.MarshalToFunc(redaction.marshalBytes))
	return redaction
}

// marshalBytes writes data as redacted when it is not UTF-8 text, and leaves
// text to the default encoding.
func (b *binaryRedaction) marshalBytes(encoder *jsontext.Encoder, data []byte) error {
	if utf8.Valid(data) {
		return errors.ErrUnsupported
	}
	b.redacted = true
	return encoder.WriteToken(jsontext.String(redacted))
}

// firstValue returns the first JSON value data starts with, and false when it
// starts with none; whatever follows that value is left out.
func firstValue(data []byte) (jsontext.Value, bool) {
	value, err := jsontext.NewDecoder(bytes.NewReader(data), treeDecoding).ReadValue()
	return value, err == nil
}

// piiPatternInfo holds the regex and mask for a PII pattern.
type piiPatternInfo struct {
	name  PIIPattern
	regex *regexp.Regexp
	mask  string
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

// tokenPattern matches a JWT: a header and a payload, both base64url JSON
// objects and so both starting with "ey", and a signature, empty when unsigned.
var tokenPattern = regexp.MustCompile(`ey[A-Za-z0-9_-]+\.ey[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*`)

// binaryRedactions holds the binaryRedaction of every encoding not in progress.
var binaryRedactions = sync.Pool{New: func() any { return newBinaryRedaction() }}

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
