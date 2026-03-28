// Package main demonstrates multiple outputs, agent mode, and production environment.
//
// Run with: go run ./examples/outputs/
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/pragmabits/axio"
)

func main() {
	ctx := context.Background()

	multipleOutputs(ctx)
	agentMode(ctx)
	productionEnvironment(ctx)
}

func multipleOutputs(ctx context.Context) {
	fmt.Println("=== Multiple Outputs (Console + File) ===")

	logFile := "axio-outputs-example.log"

	logger, err := axio.New(axio.Config{
		ServiceName:    "multi-output",
		ServiceVersion: "1.0.0",
		Environment:    axio.Development,
		Level:          axio.LevelInfo,
	},
		axio.WithOutputs(
			axio.Console(axio.FormatText),
			axio.MustFile(logFile, axio.FormatJSON),
		),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	logger.With(
		axio.Annotate("request_id", "req-001"),
	).Info(ctx, "This log appears in both console AND file")
	logger.Close()

	// Print file content
	if content, err := os.ReadFile(logFile); err == nil {
		fmt.Printf("File content: %s", string(content))
	}
	os.Remove(logFile)
	fmt.Println()
}

func agentMode(ctx context.Context) {
	fmt.Println("=== Agent Mode (JSON to stdout) ===")

	logger, _ := axio.New(axio.Config{
		ServiceName: "agent-mode-demo",
		Environment: axio.Production,
		Level:       axio.LevelInfo,
	},
		axio.WithAgentMode(),
	)
	defer logger.Close()

	logger.Info(ctx, "Log optimized for agent collection")
	fmt.Println()
}

func productionEnvironment(ctx context.Context) {
	fmt.Println("=== Production Environment (JSON + Metadata) ===")

	logger, _ := axio.New(axio.Config{
		ServiceName:    "production-api",
		ServiceVersion: "2.5.1",
		Environment:    axio.Production,
		InstanceID:     "pod-abc123",
		Level:          axio.LevelInfo,
	})
	defer logger.Close()

	logger.Info(ctx, "Production log with service and deployment metadata")
	fmt.Println()
}
