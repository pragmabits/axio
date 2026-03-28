// Package main demonstrates combining multiple options in a single logger.
//
// Run with: go run ./examples/combined/
package main

import (
	"context"
	"fmt"

	"github.com/pragmabits/axio"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== Combined Options (Console + PII + Tracing) ===")

	logger, _ := axio.New(axio.Config{
		ServiceName: "combined-demo",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	},
		axio.WithOutputs(axio.Console(axio.FormatText)),
		axio.WithPII(nil, nil),
		axio.WithTracer(axio.Otel()),
	)
	defer logger.Close()

	logger.With(
		axio.Annotate("transaction_id", "txn_987654"),
	).Info(ctx, "Log with multiple combined options")

	logger.Info(ctx, "Customer CPF 123.456.789-01 should be masked by PII hook")
	fmt.Println()
}
