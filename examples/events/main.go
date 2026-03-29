// Package main demonstrates wide events — the canonical log line pattern.
//
// Instead of scattering many log lines per request, a wide event accumulates
// all relevant context and emits a single comprehensive log entry at the end.
//
// Features demonstrated:
//   - Basic event with Add (flat fields)
//   - Context propagation (WithEvent/EventFromContext)
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
		Environment:    axio.Development,
		Level:          axio.LevelInfo,
	}

	event, err := axio.NewEvent("checkout", config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer event.Close()

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
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	}

	// Middleware: create event and store in context
	event, err := axio.NewEvent("http_request", config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer event.Close()

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
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	}

	event, err := axio.NewEvent("http_request", config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer event.Close()

	event.With(axio.Annotate("http", axio.HTTP{
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
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	}

	event, err := axio.NewEvent("checkout", config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer event.Close()

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
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	}

	event, err := axio.NewEvent("payment", config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer event.Close()

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
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	}

	event, err := axio.NewEvent("payment", config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer event.Close()

	event.Add("user_id", "usr_456")
	event.Add("amount_cents", 15999)
	event.Add("payment_provider", "stripe")

	event.SetError(errors.New("card declined by issuer"),
		axio.Annotate("error_code", "card_declined"),
		axio.Annotate("error_retriable", false),
		axio.Annotate("stripe_decline_code", "insufficient_funds"),
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
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	}

	event, err := axio.NewEvent("user_registration", config,
		axio.WithPII(
			[]axio.PIIPattern{axio.PatternCPF, axio.PatternEmail},
			axio.DefaultSensitiveFields,
		),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer event.Close()

	event.Add("user_id", "usr_new_001")
	event.Add("document", "123.456.789-01")
	event.Add("email", "john@example.com")
	event.Add("plan", "premium")

	event.Emit(ctx)
	fmt.Println()
}

// eventWithAudit demonstrates audit hash chain on events.
// Each event gets a SHA256 hash linking to the previous entry.
func eventWithAudit(ctx context.Context) {
	fmt.Println("=== Audit Hash Chain ===")

	dir, err := os.MkdirTemp("", "axio-event-audit-*")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer os.RemoveAll(dir)
	storePath := filepath.Join(dir, "chain.json")

	config := axio.Config{
		ServiceName: "compliance-service",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	}

	// First event in the chain
	event1, err := axio.NewEvent("access_grant", config,
		axio.WithAudit(storePath),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	event1.Add("user_id", "usr_456")
	event1.Add("resource", "/admin/settings")
	event1.Add("action", "read")
	event1.Emit(ctx)
	event1.Close()

	// Second event links to the first via hash chain
	event2, err := axio.NewEvent("access_grant", config,
		axio.WithAudit(storePath),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	event2.Add("user_id", "usr_789")
	event2.Add("resource", "/admin/users")
	event2.Add("action", "write")
	event2.Emit(ctx)
	event2.Close()

	fmt.Println()
}

// eventWithFileOutput demonstrates writing events to a file.
func eventWithFileOutput(ctx context.Context) {
	fmt.Println("=== File Output ===")

	dir, err := os.MkdirTemp("", "axio-event-file-*")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer os.RemoveAll(dir)
	filePath := filepath.Join(dir, "events.log")

	config := axio.Config{
		ServiceName: "checkout-service",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	}

	event, err := axio.NewEvent("checkout", config,
		axio.WithOutputs(axio.MustFile(filePath, axio.FormatJSON)),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	event.Add("user_id", "usr_456")
	event.Add("cart_total", 15999)
	event.Emit(ctx)
	event.Close()

	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}
	fmt.Printf("File content: %s", content)
	fmt.Println()
}
