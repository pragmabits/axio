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
		Environment: axio.EnvironmentDevelopment,
		Level:       axio.LevelInfo,
	})
	defer logger.Close()

	fmt.Println("=== Key-Value Annotations ===")

	logger.Info(ctx, "User initiated checkout",
		axio.Field("user_id", "usr_12345"),
		axio.Field("tenant", "acme-corp"),
		axio.Field("action", "checkout"),
	)

	fmt.Println()
	fmt.Println("=== HTTP Annotation ===")

	logger.Info(ctx, "Request processed successfully",
		axio.Field("http", axio.HTTP{
			Method:     "POST",
			URL:        "/api/v1/orders",
			StatusCode: 201,
			LatencyMS:  45,
			UserAgent:  "Mozilla/5.0",
			ClientIP:   "192.168.1.100",
		}),
	)
	fmt.Println()
}
