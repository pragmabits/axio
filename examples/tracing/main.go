// Package main demonstrates OpenTelemetry trace integration.
//
// Run with: go run ./examples/tracing/
package main

import (
	"context"
	"fmt"

	"github.com/pragmabits/axio"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== Tracing (OpenTelemetry) ===")

	logger, _ := axio.New(axio.Config{
		ServiceName: "tracing-demo",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	},
		axio.WithTracer(axio.Otel()),
	)
	defer logger.Close()

	logger.Info(ctx, "Log with tracing support (trace_id added when active span exists)")
	fmt.Println()
}
