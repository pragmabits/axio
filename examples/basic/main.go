// Package main demonstrates basic logger usage, log levels, level filtering,
// named loggers, and DefaultConfig.
//
// Run with: go run ./examples/basic/
package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/pragmabits/axio"
)

func main() {
	ctx := context.Background()

	basicLogger(ctx)
	levelFiltering(ctx)
	namedLoggers(ctx)
	defaultConfig(ctx)
}

func basicLogger(ctx context.Context) {
	fmt.Println("=== Basic Logger (Development) ===")

	logger, err := axio.New(axio.Config{
		ServiceName:    "basic-example",
		ServiceVersion: "1.0.0",
		Environment:    axio.Development,
		Level:          axio.LevelDebug,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer logger.Close()

	logger.Debug(ctx, "Debug log - visible only in Development")
	logger.Info(ctx, "Application started successfully")
	logger.Warn(ctx, errors.New("slow connection"), "High latency detected")
	logger.Error(ctx, errors.New("timeout"), "Failed to connect to database")
	fmt.Println()
}

func levelFiltering(ctx context.Context) {
	fmt.Println("=== Level Filtering (Warn) ===")

	logger, _ := axio.New(axio.Config{
		ServiceName: "level-filter",
		Environment: axio.Development,
		Level:       axio.LevelWarn,
	})
	defer logger.Close()

	fmt.Println("Logger level set to Warn — Debug and Info are discarded:")
	logger.Debug(ctx, "This debug log should NOT appear")
	logger.Info(ctx, "This info log should NOT appear")
	logger.Warn(ctx, errors.New("disk 90%% full"), "This warn log SHOULD appear")
	logger.Error(ctx, errors.New("disk full"), "This error log SHOULD appear")
	fmt.Println()
}

func namedLoggers(ctx context.Context) {
	fmt.Println("=== Named Loggers (Sub-loggers) ===")

	baseLogger, _ := axio.New(axio.Config{
		ServiceName: "microservice",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	})
	defer baseLogger.Close()

	httpHandler := baseLogger.Named("http")
	databaseRepository := baseLogger.Named("db")
	cacheLayer := baseLogger.Named("cache")

	httpHandler.Info(ctx, "Request received at /api/users")
	databaseRepository.Info(ctx, "Query executed in 12ms")
	cacheLayer.Info(ctx, "Cache hit for key user:123")
	fmt.Println()
}

func defaultConfig(ctx context.Context) {
	fmt.Println("=== DefaultConfig + Customization ===")

	config := axio.DefaultConfig()
	config.ServiceName = "default-demo"
	config.Level = axio.LevelDebug

	logger, _ := axio.New(config)
	defer logger.Close()

	logger.Debug(ctx, "Logger created with DefaultConfig() and customized")
	fmt.Println()
}
