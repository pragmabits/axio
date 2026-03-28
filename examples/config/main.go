// Package main demonstrates loading configuration from files and readers,
// including rotation config inspection.
//
// Run with: go run ./examples/config/
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/pragmabits/axio"
)

const configFilePath = "examples/config/config.yaml"

func main() {
	ctx := context.Background()

	loadFromFile(ctx)
	loadFromYAMLReader()
	loadFromJSONReader()
}

func loadFromFile(ctx context.Context) {
	fmt.Println("=== Config from File ===")

	if _, err := os.Stat(configFilePath); err != nil {
		fmt.Printf("  [warn] File not found: %s\n", configFilePath)
		fmt.Println("         Run from project root: go run ./examples/config/")
		return
	}

	config, err := axio.LoadConfig(configFilePath)
	if err != nil {
		fmt.Printf("  [warn] Error loading config: %v\n", err)
		return
	}

	fmt.Printf("  ServiceName:   %s\n", config.ServiceName)
	fmt.Printf("  Environment:   %s\n", config.Environment)
	fmt.Printf("  Level:         %s\n", config.Level)
	fmt.Printf("  PIIEnabled:    %v\n", config.PIIEnabled)
	fmt.Printf("  Audit.Enabled: %v\n", config.Audit.Enabled)

	for index, output := range config.Outputs {
		if output.Rotation.Enabled() {
			fmt.Printf("  Output[%d] rotation:\n", index)
			fmt.Printf("    MaxSize:    %d MB\n", output.Rotation.MaxSize)
			fmt.Printf("    MaxAge:     %d days\n", output.Rotation.MaxAge)
			fmt.Printf("    MaxBackups: %d\n", output.Rotation.MaxBackups)
			fmt.Printf("    Compress:   %v\n", output.Rotation.Compress)
			fmt.Printf("    Interval:   %s\n", time.Duration(output.Rotation.Interval))
		}
	}

	logger, err := axio.New(config)
	if err != nil {
		fmt.Printf("  [warn] Error creating logger: %v\n", err)
		return
	}
	defer logger.Close()

	logger.Info(ctx, "Logger created from config.yaml")
	fmt.Println()
}

func loadFromYAMLReader() {
	fmt.Println("=== Config from YAML Reader ===")

	yamlContent := `
serviceName: "from-yaml-reader"
serviceVersion: "2.0.0"
environment: "staging"
level: "info"
`
	config, err := axio.LoadConfigFrom(bytes.NewReader([]byte(yamlContent)), "yaml")
	if err != nil {
		fmt.Printf("  [warn] Error: %v\n", err)
		return
	}

	fmt.Printf("  ServiceName: %s\n", config.ServiceName)
	fmt.Printf("  Environment: %s\n", config.Environment)
	fmt.Println()
}

func loadFromJSONReader() {
	fmt.Println("=== Config from JSON Reader ===")

	jsonContent := `{
		"serviceName": "from-json-reader",
		"serviceVersion": "3.0.0",
		"environment": "production",
		"level": "warn"
	}`

	config, err := axio.LoadConfigFrom(bytes.NewReader([]byte(jsonContent)), "json")
	if err != nil {
		fmt.Printf("  [warn] Error: %v\n", err)
		return
	}

	fmt.Printf("  ServiceName: %s\n", config.ServiceName)
	fmt.Printf("  Environment: %s\n", config.Environment)
	fmt.Println()
}
