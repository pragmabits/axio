// Package main demonstrates structured annotations and HTTP metadata.
//
// Run with: go run ./examples/annotations/
package main

import (
	"context"
	"fmt"

	"github.com/pragmabits/axio"
)

func main() {
	ctx := context.Background()

	logger, _ := axio.New(axio.Config{
		ServiceName: "sales-api",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	})
	defer logger.Close()

	fmt.Println("=== Key-Value Annotations ===")

	logger.With(
		axio.Annotate("user_id", "usr_12345"),
		axio.Annotate("tenant", "acme-corp"),
		axio.Annotate("action", "checkout"),
	).Info(ctx, "User initiated checkout")

	fmt.Println()
	fmt.Println("=== HTTP Annotation ===")

	logger.With(
		axio.Annotate("http", axio.HTTP{
			Method:     "POST",
			URL:        "/api/v1/orders",
			StatusCode: 201,
			LatencyMS:  45,
			UserAgent:  "Mozilla/5.0",
			ClientIP:   "192.168.1.100",
		}),
	).Info(ctx, "Request processed successfully")
	fmt.Println()
}
