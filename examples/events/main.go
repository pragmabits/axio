// Package main demonstrates wide events — the canonical log line pattern.
//
// Instead of scattering many log lines per request, a wide event accumulates
// all relevant context and emits a single comprehensive log entry at the end.
// An event comes from a Logger and writes through it: to its outputs, through
// its PII masking and its audit chain, as JSON or as the Console's text.
//
// Features demonstrated:
//   - Basic event with Add (flat fields)
//   - Context propagation (WithEvent/EventFromContext), and an event that
//     carries the name and annotations of the logger it came from
//   - Annotable types (HTTP metadata)
//   - Nested data (maps)
//   - Simple error (SetError)
//   - Detailed error (SetError with annotations)
//   - PII masking on event fields
//   - Audit hash chain on events
//   - File output
//
// Run with: go run ./examples/events/
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pragmabits/axio"
)

func main() {
	ctx := context.Background()

	basicEvent(ctx)
	contextPropagation(ctx)
	eventWithAnnotable(ctx)
	eventWithNestedData(ctx)
	eventWithSimpleError(ctx)
	eventWithDetailedError(ctx)
	eventWithPII(ctx)
	eventWithAudit(ctx)
	eventWithFileOutput(ctx)
}

// basicEvent demonstrates a simple wide event with flat key-value fields.
func basicEvent(ctx context.Context) {
	fmt.Println("=== Basic Wide Event ===")

	config := axio.Config{
		ServiceName:    "checkout-service",
		ServiceVersion: "2.4.1",
		Environment:    axio.EnvironmentDevelopment,
		Level:          axio.LevelInfo,
	}

	logger, err := axio.New(config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer logger.Close()

	event := logger.Event("checkout")
	event.Add("user_id", "usr_456")
	event.Add("cart_total", 15999)
	event.Add("coupon_applied", "SAVE20")
	event.Add("item_count", 3)

	event.Emit(ctx)
	fmt.Println()
}

// contextPropagation demonstrates the middleware pattern: create an event,
// store it in context, enrich it from downstream handlers, emit at the end.
func contextPropagation(ctx context.Context) {
	fmt.Println("=== Context Propagation (Middleware Pattern) ===")

	config := axio.Config{
		ServiceName: "api-gateway",
		Environment: axio.EnvironmentDevelopment,
		Level:       axio.LevelInfo,
	}

	logger, err := axio.New(config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer logger.Close()

	// Middleware: create the event from the request's logger, which gives it
	// its name and its request_id, and store it in context
	requestLogger := logger.Named("http").With(axio.Field("request_id", "req_7f3a"))
	event := requestLogger.Event("http_request")
	ctx = axio.WithEvent(ctx, event)

	// Simulate handler chain enriching the event via context
	authMiddleware(ctx)
	checkoutHandler(ctx)

	// Middleware: emit at the end of the request lifecycle
	event.Emit(ctx)
	fmt.Println()
}

func authMiddleware(ctx context.Context) {
	event := axio.EventFromContext(ctx)
	event.Add("auth_method", "bearer_token")
	event.Add("user_id", "usr_789")
	event.Add("user_role", "admin")
}

func checkoutHandler(ctx context.Context) {
	event := axio.EventFromContext(ctx)
	event.Add("cart_total", 24999)
	event.Add("item_count", 5)
	event.Add("payment_method", "credit_card")
}

// eventWithAnnotable demonstrates using the existing HTTP Annotable type
// to add structured metadata that expands into individual fields.
func eventWithAnnotable(ctx context.Context) {
	fmt.Println("=== Annotable Types (HTTP Metadata) ===")

	config := axio.Config{
		ServiceName: "api-gateway",
		Environment: axio.EnvironmentDevelopment,
		Level:       axio.LevelInfo,
	}

	logger, err := axio.New(config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer logger.Close()

	event := logger.Event("http_request")
	event.With(axio.Field("http", axio.HTTP{
		Method:     "POST",
		URL:        "/api/v1/orders",
		StatusCode: 201,
		LatencyMS:  45,
		UserAgent:  "Mozilla/5.0",
		ClientIP:   "192.168.1.100",
	}))

	event.Add("user_id", "usr_789")
	event.Add("order_id", "ord_abc123")

	event.Emit(ctx)
	fmt.Println()
}

// eventWithNestedData demonstrates nested JSON objects using maps.
func eventWithNestedData(ctx context.Context) {
	fmt.Println("=== Nested Data (Maps) ===")

	config := axio.Config{
		ServiceName: "checkout-service",
		Environment: axio.EnvironmentDevelopment,
		Level:       axio.LevelInfo,
	}

	logger, err := axio.New(config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer logger.Close()

	event := logger.Event("checkout")
	event.Add("user", map[string]any{
		"id":           "usr_456",
		"subscription": "premium",
		"account_age":  847,
	})

	event.Add("cart", map[string]any{
		"id":         "cart_xyz",
		"item_count": 3,
		"total":      15999,
	})

	event.Add("feature_flags", map[string]any{
		"new_checkout_flow": true,
		"express_payment":   false,
	})

	event.Emit(ctx)
	fmt.Println()
}

// eventWithSimpleError demonstrates recording an error without extra details.
func eventWithSimpleError(ctx context.Context) {
	fmt.Println("=== Simple Error ===")

	config := axio.Config{
		ServiceName: "payment-service",
		Environment: axio.EnvironmentDevelopment,
		Level:       axio.LevelInfo,
	}

	logger, err := axio.New(config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer logger.Close()

	event := logger.Event("payment")
	event.Add("user_id", "usr_456")
	event.Add("amount_cents", 15999)

	event.SetError(errors.New("connection timeout"))

	event.Emit(ctx)
	fmt.Println()
}

// eventWithDetailedError demonstrates recording an error with structured details.
func eventWithDetailedError(ctx context.Context) {
	fmt.Println("=== Detailed Error ===")

	config := axio.Config{
		ServiceName: "payment-service",
		Environment: axio.EnvironmentDevelopment,
		Level:       axio.LevelInfo,
	}

	logger, err := axio.New(config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer logger.Close()

	event := logger.Event("payment")
	event.Add("user_id", "usr_456")
	event.Add("amount_cents", 15999)
	event.Add("payment_provider", "stripe")

	event.SetError(errors.New("card declined by issuer"),
		axio.Field("error_code", "card_declined"),
		axio.Field("error_retriable", false),
		axio.Field("stripe_decline_code", "insufficient_funds"),
	)

	event.Emit(ctx)
	fmt.Println()
}

// eventWithPII demonstrates PII masking on event fields.
// Sensitive data (CPF, email) is automatically masked before output.
func eventWithPII(ctx context.Context) {
	fmt.Println("=== PII Masking ===")

	config := axio.Config{
		ServiceName: "user-service",
		Environment: axio.EnvironmentDevelopment,
		Level:       axio.LevelInfo,
	}

	logger, err := axio.New(config,
		axio.WithPII(
			[]axio.PIIPattern{axio.PatternCPF, axio.PatternEmail},
			axio.DefaultSensitiveFields(),
		),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer logger.Close()

	event := logger.Event("user_registration")
	event.Add("user_id", "usr_new_001")
	event.Add("document", "123.456.789-01")
	event.Add("email", "john@example.com")
	event.Add("plan", "premium")

	event.Emit(ctx)
	fmt.Println()
}

// eventWithAudit demonstrates audit hash chain on events.
// Each event gets a SHA256 hash linking to the previous entry: the full hash on
// the JSON output, which is the one verified, and its first characters on the
// text output. An audited logger needs a JSON output.
func eventWithAudit(ctx context.Context) {
	fmt.Println("=== Audit Hash Chain ===")

	directory, err := os.MkdirTemp("", "axio-event-audit-*")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer os.RemoveAll(directory)
	storePath := filepath.Join(directory, "chain.json")

	config := axio.Config{
		ServiceName: "compliance-service",
		Environment: axio.EnvironmentDevelopment,
		Level:       axio.LevelInfo,
	}

	logger, err := axio.New(config,
		axio.WithOutputs(axio.Console(axio.FormatText), axio.Stdout(axio.FormatJSON)),
		axio.WithAudit(storePath),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer logger.Close()

	// First event in the chain
	first := logger.Event("access_grant")
	first.Add("user_id", "usr_456")
	first.Add("resource", "/admin/settings")
	first.Add("action", "read")
	first.Emit(ctx)

	// Second event links to the first via hash chain
	second := logger.Event("access_grant")
	second.Add("user_id", "usr_789")
	second.Add("resource", "/admin/users")
	second.Add("action", "write")
	second.Emit(ctx)

	fmt.Println()
}

// eventWithFileOutput demonstrates writing events to a file.
func eventWithFileOutput(ctx context.Context) {
	fmt.Println("=== File Output ===")

	directory, err := os.MkdirTemp("", "axio-event-file-*")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer os.RemoveAll(directory)
	filePath := filepath.Join(directory, "events.log")

	config := axio.Config{
		ServiceName: "checkout-service",
		Environment: axio.EnvironmentDevelopment,
		Level:       axio.LevelInfo,
	}

	logger, err := axio.New(config,
		axio.WithOutputs(axio.MustFile(filePath, axio.FormatJSON)),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	event := logger.Event("checkout")
	event.Add("user_id", "usr_456")
	event.Add("cart_total", 15999)
	event.Emit(ctx)
	if err := logger.Close(); err != nil {
		fmt.Printf("Error closing logger: %v\n", err)
		return
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}
	fmt.Printf("File content: %s", content)
	fmt.Println()
}
