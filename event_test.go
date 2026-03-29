package axio

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewEvent(t *testing.T) {
	t.Run("creates_event_with_name", func(t *testing.T) {
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputStdout, Format: FormatJSON},
		}

		event, err := NewEvent("checkout", config)
		assertNoError(t, err)

		if event == nil {
			t.Fatal("event should not be nil")
		}
	})

	t.Run("invalid_config_returns_error", func(t *testing.T) {
		config := minimalConfig()
		config.Environment = "invalid"

		_, err := NewEvent("checkout", config)
		assertError(t, err)
	})

	t.Run("accepts_options", func(t *testing.T) {
		path := tempFile(t, "event.log")
		config := minimalConfig()

		event, err := NewEvent("checkout", config,
			WithOutputs(MustFile(path, FormatJSON)),
		)
		assertNoError(t, err)

		if event == nil {
			t.Fatal("event should not be nil")
		}
	})
}

func TestEvent_ContextPropagation(t *testing.T) {
	t.Run("round_trip_through_context", func(t *testing.T) {
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputStdout, Format: FormatJSON},
		}
		event, err := NewEvent("checkout", config)
		assertNoError(t, err)

		ctx := WithEvent(context.Background(), event)
		retrieved := EventFromContext(ctx)

		if retrieved != event {
			t.Error("expected same event from context")
		}
	})

	t.Run("returns_nil_when_no_event_in_context", func(t *testing.T) {
		retrieved := EventFromContext(context.Background())
		if retrieved != nil {
			t.Error("expected nil when no event in context")
		}
	})
}

func TestEvent_Add(t *testing.T) {
	t.Run("adds_flat_fields_to_output", func(t *testing.T) {
		path := tempFile(t, "event.log")
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		}

		event, err := NewEvent("checkout", config)
		assertNoError(t, err)

		event.Add("user_id", "usr_456")
		event.Add("cart_total", 15999)
		event.Add("premium", true)
		event.Emit(context.Background())

		content := readFile(t, path)
		result := parseEventJSON(t, content)

		assertEqual(t, result["user_id"].(string), "usr_456")
		assertEqual(t, result["cart_total"].(float64), float64(15999))
		assertEqual(t, result["premium"].(bool), true)
	})
}

func TestEvent_With(t *testing.T) {
	t.Run("accepts_annotations", func(t *testing.T) {
		path := tempFile(t, "event.log")
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		}

		event, err := NewEvent("http_request", config)
		assertNoError(t, err)

		event.With(
			Annotate("method", "POST"),
			Annotate("url", "/api/checkout"),
		)
		event.Emit(context.Background())

		content := readFile(t, path)
		result := parseEventJSON(t, content)

		assertEqual(t, result["method"].(string), "POST")
		assertEqual(t, result["url"].(string), "/api/checkout")
	})

	t.Run("accepts_annotable_types", func(t *testing.T) {
		path := tempFile(t, "event.log")
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		}

		event, err := NewEvent("http_request", config)
		assertNoError(t, err)

		event.With(Annotate("http", HTTP{
			Method:     "POST",
			URL:        "/api/checkout",
			StatusCode: 201,
			LatencyMS:  45,
		}))
		event.Emit(context.Background())

		content := readFile(t, path)
		result := parseEventJSON(t, content)

		assertEqual(t, result["method"].(string), "POST")
		assertEqual(t, result["url"].(string), "/api/checkout")
		assertEqual(t, result["status_code"].(float64), float64(201))
		assertEqual(t, result["latency"].(float64), float64(45))
	})

	t.Run("supports_nested_maps", func(t *testing.T) {
		path := tempFile(t, "event.log")
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		}

		event, err := NewEvent("checkout", config)
		assertNoError(t, err)

		event.Add("user", map[string]any{
			"id":           "usr_456",
			"subscription": "premium",
		})
		event.Emit(context.Background())

		content := readFile(t, path)
		result := parseEventJSON(t, content)

		userMap, ok := result["user"].(map[string]any)
		if !ok {
			t.Fatal("expected user to be a nested object")
		}
		assertEqual(t, userMap["id"].(string), "usr_456")
		assertEqual(t, userMap["subscription"].(string), "premium")
	})
}

func TestEvent_SetError(t *testing.T) {
	t.Run("records_error_in_output", func(t *testing.T) {
		path := tempFile(t, "event.log")
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		}

		event, err := NewEvent("checkout", config)
		assertNoError(t, err)

		event.SetError(errors.New("card declined"))
		event.Emit(context.Background())

		content := readFile(t, path)
		result := parseEventJSON(t, content)

		assertEqual(t, result["error"].(string), "card declined")
	})

	t.Run("records_error_with_details", func(t *testing.T) {
		path := tempFile(t, "event.log")
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		}

		event, err := NewEvent("checkout", config)
		assertNoError(t, err)

		event.SetError(errors.New("card declined"),
			Annotate("error_code", "card_declined"),
			Annotate("error_retriable", false),
		)
		event.Emit(context.Background())

		content := readFile(t, path)
		result := parseEventJSON(t, content)

		assertEqual(t, result["error"].(string), "card declined")
		assertEqual(t, result["error_code"].(string), "card_declined")
		assertEqual(t, result["error_retriable"].(bool), false)
	})

	t.Run("no_error_field_when_not_set", func(t *testing.T) {
		path := tempFile(t, "event.log")
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		}

		event, err := NewEvent("checkout", config)
		assertNoError(t, err)

		event.Emit(context.Background())

		content := readFile(t, path)
		result := parseEventJSON(t, content)

		if _, ok := result["error"]; ok {
			t.Error("event without error should not have error field")
		}
	})
}

func TestEvent_Emit(t *testing.T) {
	t.Run("emits_event_with_name_and_duration", func(t *testing.T) {
		path := tempFile(t, "event.log")
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		}

		event, err := NewEvent("checkout", config)
		assertNoError(t, err)

		event.Emit(context.Background())

		content := readFile(t, path)
		result := parseEventJSON(t, content)

		// Must have event name
		assertEqual(t, result["event"].(string), "checkout")

		// Must have duration_ms
		if _, ok := result["duration_ms"]; !ok {
			t.Error("expected duration_ms field in output")
		}

		// Must have timestamp
		if _, ok := result["timestamp"]; !ok {
			t.Error("expected timestamp field in output")
		}

		// Must NOT have level
		if _, ok := result["level"]; ok {
			t.Error("event output should not contain level field")
		}

		// Must NOT have caller
		if _, ok := result["caller"]; ok {
			t.Error("event output should not contain caller field")
		}

		// Must NOT have logger name
		if _, ok := result["logger"]; ok {
			t.Error("event output should not contain logger field")
		}

		// Must NOT have stacktrace
		if _, ok := result["stacktrace"]; ok {
			t.Error("event output should not contain stacktrace field")
		}
	})

	t.Run("empty_event_has_name_and_duration_only", func(t *testing.T) {
		path := tempFile(t, "event.log")
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		}

		event, err := NewEvent("ping", config)
		assertNoError(t, err)

		event.Emit(context.Background())

		content := readFile(t, path)
		result := parseEventJSON(t, content)

		// Only timestamp, event, duration_ms should be present
		assertEqual(t, result["event"].(string), "ping")
		if _, ok := result["duration_ms"]; !ok {
			t.Error("expected duration_ms")
		}
		if _, ok := result["timestamp"]; !ok {
			t.Error("expected timestamp")
		}
	})

	t.Run("concurrent_enrichment_is_safe", func(t *testing.T) {
		path := tempFile(t, "event.log")
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		}

		event, err := NewEvent("concurrent", config)
		assertNoError(t, err)

		var waitGroup sync.WaitGroup
		for index := 0; index < 100; index++ {
			waitGroup.Add(1)
			go func(number int) {
				defer waitGroup.Done()
				event.Add("field", number)
			}(index)
		}
		waitGroup.Wait()

		event.Emit(context.Background())

		content := readFile(t, path)
		if len(content) == 0 {
			t.Error("expected event output after concurrent enrichment")
		}
	})

	t.Run("pii_masking_works_on_event_fields", func(t *testing.T) {
		path := tempFile(t, "event.log")
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		}

		event, err := NewEvent("checkout", config,
			WithPII([]PIIPattern{PatternCPF}, DefaultSensitiveFields),
		)
		assertNoError(t, err)

		event.Add("document", "123.456.789-01")
		event.Emit(context.Background())

		content := readFile(t, path)
		if strings.Contains(content, "123.456.789-01") {
			t.Error("CPF should have been masked by PII hook")
		}
		if !strings.Contains(content, "***.***.***-**") {
			t.Error("expected masked CPF pattern in output")
		}
	})

	t.Run("pii_masking_works_on_error_details", func(t *testing.T) {
		path := tempFile(t, "event.log")
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		}

		event, err := NewEvent("checkout", config,
			WithPII([]PIIPattern{PatternCPF}, DefaultSensitiveFields),
		)
		assertNoError(t, err)

		event.SetError(errors.New("validation failed"),
			Annotate("customer_document", "123.456.789-01"),
		)
		event.Emit(context.Background())

		content := readFile(t, path)
		if strings.Contains(content, "123.456.789-01") {
			t.Error("CPF in error details should have been masked by PII hook")
		}
		if !strings.Contains(content, "***.***.***-**") {
			t.Error("expected masked CPF pattern in error details")
		}
	})

	t.Run("timestamp_is_event_start_time", func(t *testing.T) {
		path := tempFile(t, "event.log")
		config := minimalConfig()
		config.Outputs = []OutputConfig{
			{Type: OutputFile, Format: FormatJSON, Path: path},
		}

		event, err := NewEvent("checkout", config)
		assertNoError(t, err)

		startTime := event.startTime

		// Sleep to create a measurable gap between start and emit
		time.Sleep(50 * time.Millisecond)

		event.Emit(context.Background())

		content := readFile(t, path)
		result := parseEventJSON(t, content)

		timestampStr := result["timestamp"].(string)
		parsed, parseErr := time.Parse(time.RFC3339Nano, timestampStr)
		assertNoError(t, parseErr)

		// Timestamp must equal the start time, not the emit time.
		// With 50ms sleep, if timestamp were time.Now() at emit,
		// the diff would be >= 50ms.
		diff := parsed.Sub(startTime.UTC())
		if diff < 0 {
			diff = -diff
		}
		if diff > 5*time.Millisecond {
			t.Errorf("timestamp should equal start time, but diff was %v (timestamp=%v, startTime=%v)",
				diff, parsed, startTime.UTC())
		}
	})
}

// parseEventJSON parses a single JSON line from event output.
func parseEventJSON(t *testing.T, content string) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &result); err != nil {
		t.Fatalf("failed to parse event JSON: %v\ncontent: %s", err, content)
	}
	return result
}
