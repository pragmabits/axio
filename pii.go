package axio

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

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
	// Mask: **.***.***/**01-**
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
		mask:  "**.***.***/**01-**",
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

// DefaultSensitiveFields contains field names that are automatically redacted.
//
// Matching is case-insensitive and uses partial matching
// (e.g., "user_password" matches "password").
var DefaultSensitiveFields = []string{
	"password", "senha",
	"token", "api_key", "apikey",
	"secret", "credential",
	"authorization", "bearer",
	"private_key", "privatekey",
	"access_key", "secret_key",
	"client_secret", "clientsecret",
}

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
		Fields: DefaultSensitiveFields,
	}
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

// NewPIIMasker creates a new masker with the specified configuration.
//
// Returns an error if any CustomPattern has an invalid regex.
func NewPIIMasker(config PIIConfig) (*PIIMasker, error) {
	masker := &PIIMasker{
		fields: make(map[string]bool),
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

// MaskFields masks sensitive values in a map.
//
// Field names are matched case-insensitively using partial matching.
// Nested maps are processed recursively.
//
// Fields whose names contain sensitive terms (password, token, etc.)
// are replaced with "[REDACTED]". String values are processed
// by [MaskString] to detect PII patterns.
func (m *PIIMasker) MaskFields(fields Annotations) {
	if fields == nil {
		return
	}

	for index := range fields {
		if m.isSensitiveField(fields[index].Name()) {
			fields[index].Set("[REDACTED]")
			continue
		}

		switch value := fields[index].Data().(type) {
		case string:
			fields[index].Set(m.MaskString(value))
		default:
			continue
		}
	}
}

// PIIMaskResult contains the result of PII masking with match counts.
type PIIMaskResult struct {
	// Masked is the string with PII masked.
	Masked string
	// Matches maps each pattern to the number of occurrences found.
	Matches map[PIIPattern]int
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
// Similar to [MaskFields], but returns a map with the count of each pattern
// detected across all processed fields.
func (m *PIIMasker) MaskFieldsWithCounts(fields Annotations) map[PIIPattern]int {
	matches := make(map[PIIPattern]int)

	if fields == nil {
		return matches
	}

	for index := range fields {
		if m.isSensitiveField(fields[index].Name()) {
			fields[index].Set("[REDACTED]")
			continue
		}

		switch value := fields[index].Data().(type) {
		case string:
			result := m.MaskStringWithCounts(value)
			fields[index].Set(result.Masked)
			for pattern, count := range result.Matches {
				matches[pattern] += count
			}
		}
	}

	return matches
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

// PIIHook is a hook that masks PII in log entries before they are written.
//
// The hook processes both the message and the structured fields,
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
func (hook *PIIHook) Name() string {
	return "pii"
}

// SetMetrics implements [MetricsAware].
//
// When configured, the hook emits metrics for each detected PII pattern.
func (hook *PIIHook) SetMetrics(metrics Metrics) {
	hook.mutex.Lock()
	defer hook.mutex.Unlock()
	hook.metrics = metrics
}

// Process masks PII in the log entry.
//
// Modifies the message and fields in-place, replacing PII patterns
// with masks and sensitive fields with "[REDACTED]".
//
// If metrics is configured via [SetMetrics], emits metrics for
// each detected PII occurrence.
func (hook *PIIHook) Process(ctx context.Context, entry *Entry) error {
	fieldMatches := hook.masker.MaskFieldsWithCounts(entry.Annotations)
	messageResult := hook.masker.MaskStringWithCounts(entry.Message)
	entry.Message = messageResult.Masked

	hook.mutex.RLock()
	metrics := hook.metrics
	hook.mutex.RUnlock()

	if metrics == nil {
		return nil
	}

	for pattern, count := range messageResult.Matches {
		for range count {
			metrics.PIIMasked(ctx, pattern)
		}
	}

	for pattern, count := range fieldMatches {
		for range count {
			metrics.PIIMasked(ctx, pattern)
		}
	}

	return nil
}
