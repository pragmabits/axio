// Package main demonstrates all the functionalities of the axio package.
//
// Run with: go run examples/complete/main.go
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pragmabits/axio"
)

func main() {
	ctx := context.Background()

	fmt.Println("=" + strings.Repeat("=", 59))
	fmt.Println("  Axio Logger - Complete Example")
	fmt.Println("=" + strings.Repeat("=", 59))
	fmt.Println()

	// =========================================================================
	// 1. BASIC LOGGER (Development)
	// =========================================================================
	fmt.Println("[1] Basic Logger - Development Environment")
	fmt.Println(strings.Repeat("-", 60))

	config := axio.Config{
		ServiceName:    "axio-example",
		ServiceVersion: "1.0.0",
		Environment:    axio.Development,
		Level:          axio.LevelDebug,
	}

	logger, err := axio.New(config)
	if err != nil {
		fmt.Printf("Error creating logger: %v\n", err)
		os.Exit(1)
	}

	logger.Debug(ctx, "Debug log - visible only in Development")
	logger.Info(ctx, "Application started successfully")
	logger.Warn(ctx, errors.New("slow connection"), "High latency detected")
	logger.Error(ctx, errors.New("timeout"), "Failed to connect to database")

	fmt.Println()

	// =========================================================================
	// 2. CONFIGURATION LOADING FROM FILE
	// =========================================================================
	fmt.Println("[2] Configuration Loading from File")
	fmt.Println(strings.Repeat("-", 60))

	// 2.1 LoadConfig - loads from YAML file
	configPath := "examples/complete/config.yaml"

	if _, err := os.Stat(configPath); err == nil {
		fileConfig, err := axio.LoadConfig(configPath)
		if err != nil {
			fmt.Printf("⚠︝  Error loading config.yaml: %v\n", err)
		} else {
			fmt.Printf("✅ Config loaded from: %s\n", configPath)
			fmt.Printf("   ServiceName: %s\n", fileConfig.ServiceName)
			fmt.Printf("   Environment: %s\n", fileConfig.Environment)
			fmt.Printf("   Level: %s\n", fileConfig.Level)
			fmt.Printf("   PIIEnabled: %v\n", fileConfig.PIIEnabled)
			fmt.Printf("   Audit.Enabled: %v\n", fileConfig.Audit.Enabled)

			fileLogger, err := axio.New(fileConfig)
			if err != nil {
				fmt.Printf("⚠︝  Error creating logger: %v\n", err)
			} else {
				fileLogger.Info(ctx, "Logger created from config.yaml")
			}
		}
	} else {
		fmt.Printf("⚠︝  File not found: %s\n", configPath)
		fmt.Println("   Run from project root: go run examples/complete/main.go")
	}

	// 2.2 LoadConfigFrom - loads from io.Reader (YAML)
	yamlContent := `
serviceName: "from-yaml-reader"
serviceVersion: "2.0.0"
environment: "staging"
level: "info"
`
	readerConfig, err := axio.LoadConfigFrom(bytes.NewReader([]byte(yamlContent)), "yaml")
	if err != nil {
		fmt.Printf("⚠︝  Error loading YAML config: %v\n", err)
	} else {
		fmt.Printf("✅ Config loaded from io.Reader (YAML)\n")
		fmt.Printf("   ServiceName: %s\n", readerConfig.ServiceName)
		fmt.Printf("   Environment: %s\n", readerConfig.Environment)
	}

	// 2.3 LoadConfigFrom - loads from io.Reader (JSON)
	jsonContent := `{
		"serviceName": "from-json-reader",
		"serviceVersion": "3.0.0",
		"environment": "production",
		"level": "warn"
	}`
	jsonConfig, err := axio.LoadConfigFrom(bytes.NewReader([]byte(jsonContent)), "json")
	if err != nil {
		fmt.Printf("⚠︝  Error loading JSON config: %v\n", err)
	} else {
		fmt.Printf("✅ Config loaded from io.Reader (JSON)\n")
		fmt.Printf("   ServiceName: %s\n", jsonConfig.ServiceName)
		fmt.Printf("   Environment: %s\n", jsonConfig.Environment)
	}

	fmt.Println()

	// =========================================================================
	// 3. MULTIPLE OUTPUTS
	// =========================================================================
	fmt.Println("[3] Multiple Outputs (Console + File)")
	fmt.Println(strings.Repeat("-", 60))

	logFile := "axio-example.log"

	multiConfig := axio.Config{
		ServiceName:    "multi-output",
		ServiceVersion: "1.0.0",
		Environment:    axio.Development,
		Level:          axio.LevelInfo,
	}

	multiLogger, err := axio.New(multiConfig,
		axio.WithOutputs(
			axio.Console(axio.FormatText),           // colorized stderr
			axio.MustFile(logFile, axio.FormatJSON), // JSON file
		),
	)
	if err != nil {
		fmt.Printf("Error creating multi-output logger: %v\n", err)
		os.Exit(1)
	}

	multiLogger.With(axio.Annotate("X", 10)).Info(ctx, "This log appears in both console AND file")

	fmt.Printf("📝 Log file created: %s\n", logFile)
	fmt.Println()

	// =========================================================================
	// 4. STRUCTURED ANNOTATIONS
	// =========================================================================
	fmt.Println("[4] Structured Annotations (Annotate + HTTP)")
	fmt.Println(strings.Repeat("-", 60))

	annotatedLogger, _ := axio.New(axio.Config{
		ServiceName: "sales-api",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	})

	// Simple annotations
	annotatedLogger.With(
		axio.Annotate("user_id", "usr_12345"),
		axio.Annotate("tenant", "acme-corp"),
		axio.Annotate("action", "checkout"),
	).Info(ctx, "User initiated checkout")

	// HTTP annotation
	httpAnnotation := &axio.HTTP{
		Method:     "POST",
		URL:        "/api/v1/orders",
		StatusCode: 201,
		LatencyMS:  45,
		UserAgent:  "Mozilla/5.0",
		ClientIP:   "192.168.1.100",
	}

	annotatedLogger.With(httpAnnotation).Info(ctx, "Request processed successfully")

	fmt.Println()

	// =========================================================================
	// 5. NAMED LOGGERS (Sub-loggers)
	// =========================================================================
	fmt.Println("[5] Named Loggers (Sub-loggers)")
	fmt.Println(strings.Repeat("-", 60))

	baseLogger, _ := axio.New(axio.Config{
		ServiceName: "microservice",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	})

	httpHandler := baseLogger.Named("http")
	dbRepository := baseLogger.Named("db")
	cacheLayer := baseLogger.Named("cache")

	httpHandler.Info(ctx, "Request received at /api/users")
	dbRepository.Info(ctx, "Query executed in 12ms")
	cacheLayer.Info(ctx, "Cache hit for key user:123")

	fmt.Println()

	// =========================================================================
	// 6. PII MASKING (Sensitive Data Masking)
	// =========================================================================
	fmt.Println("[6] PII Masking (PIIMasker API)")
	fmt.Println(strings.Repeat("-", 60))

	masker := axio.MustPIIMasker(axio.DefaultPIIConfig())

	// Mask strings with PII patterns
	testCases := []string{
		"Customer CPF: 123.456.789-01",
		"Company CNPJ: 12.345.678/0001-90",
		"Email: contato@empresa.com.br",
		"Phone: (11) 99999-8888",
		"Card: 4111-1111-1111-1111",
	}

	for _, original := range testCases {
		masked := masker.MaskString(original)
		fmt.Printf("  Original: %s\n", original)
		fmt.Printf("  Masked:   %s\n\n", masked)
	}

	// Demonstrate PII via Hook (idiomatic way)
	fmt.Println("[6.1] PII Masking via Hook (Annotations)")
	fmt.Println(strings.Repeat("-", 60))

	piiLogger, _ := axio.New(axio.Config{
		ServiceName: "pii-demo",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	},
		axio.WithOutputs(
			axio.Console(axio.FormatText),
			axio.MustFile(logFile, axio.FormatJSON),
		),
		axio.WithHooks(axio.MustPIIHook(axio.DefaultPIIConfig())),
	)

	// Message with sensitive data - automatically masked
	piiLogger.Info(ctx, "Customer CPF 123.456.789-01 purchased with card 4111-1111-1111-1111")

	// Annotations with sensitive data - also masked
	piiLogger.With(
		axio.Annotate("cpf", "987.654.321-00"),
		axio.Annotate("email", "cliente@empresa.com.br"),
		axio.Annotate("password", "super_secret"),
	).Info(ctx, "Customer data processed")

	fmt.Println("  ✅ CPF, card, email and password automatically masked")
	fmt.Println()

	// =========================================================================
	// 7. HASH CHAIN (Direct Audit)
	// =========================================================================
	fmt.Println("[7] Hash Chain (Audit with Integrity)")
	fmt.Println(strings.Repeat("-", 60))

	chainStorePath := "axio-audit-chain.json"

	// Create FileStore for persistence
	store := axio.NewFileStore(chainStorePath)

	// Create HashChain
	chain, err := axio.NewHashChain(store)
	if err != nil {
		fmt.Printf("⚠︝  Error creating HashChain: %v\n", err)
	} else {
		// Add entries to the chain
		entries := []string{
			"User admin login",
			"Permission change for operators group",
			"Customer data export",
			"User admin logout",
		}

		fmt.Println("Adding entries to hash chain:")
		for i, entry := range entries {
			hash, prevHash, err := chain.Add([]byte(entry))
			if err != nil {
				fmt.Printf("  [%d] ERROR: %v\n", i+1, err)
				continue
			}
			fmt.Printf("  [%d] %s\n", i+1, entry)
			fmt.Printf("      Hash: %s...\n", hash[:16])
			if prevHash != "" {
				fmt.Printf("      PrevHash: %s...\n", prevHash[:16])
			}
		}

		fmt.Printf("\n✅ Hash chain persisted to: %s\n", chainStorePath)
		fmt.Printf("   Current sequence: %d\n", chain.Sequence())
		fmt.Printf("   Last hash: %s...\n", chain.LastHash()[:16])

		// Verify file content
		if data, err := os.ReadFile(chainStorePath); err == nil {
			fmt.Printf("   Content: %s\n", string(data))
		}
	}

	fmt.Println()

	// =========================================================================
	// 8. TRACING (OpenTelemetry)
	// =========================================================================
	fmt.Println("[8] Tracing (OpenTelemetry Integration)")
	fmt.Println(strings.Repeat("-", 60))

	tracingLogger, _ := axio.New(axio.Config{
		ServiceName: "tracing-demo",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	},
		axio.WithTracer(axio.Otel()),
	)

	tracingLogger.Info(ctx, "Log with tracing support (trace_id will be added if there is an active span)")

	fmt.Println()

	// =========================================================================
	// 9. AGENT MODE (Promtail, Fluent Bit, etc.)
	// =========================================================================
	fmt.Println("[9] Agent Mode (JSON to stdout)")
	fmt.Println(strings.Repeat("-", 60))

	agentLogger, _ := axio.New(axio.Config{
		ServiceName: "agent-mode-demo",
		Environment: axio.Production,
		Level:       axio.LevelInfo,
	},
		axio.WithAgentMode(),
	)

	agentLogger.Info(ctx, "Log optimized for agent collection")

	fmt.Println()

	// =========================================================================
	// 10. PRODUCTION ENVIRONMENT (Structured JSON)
	// =========================================================================
	fmt.Println("[10] Production Environment (JSON + Metadata)")
	fmt.Println(strings.Repeat("-", 60))

	prodLogger, _ := axio.New(axio.Config{
		ServiceName:    "production-api",
		ServiceVersion: "2.5.1",
		Environment:    axio.Production,
		InstanceID:     "pod-abc123",
		Level:          axio.LevelInfo,
	})

	prodLogger.Info(ctx, "Production log with service and deployment metadata")

	fmt.Println()

	// =========================================================================
	// 11. DefaultConfig + Customization
	// =========================================================================
	fmt.Println("[11] DefaultConfig + Customization")
	fmt.Println(strings.Repeat("-", 60))

	defaultCfg := axio.DefaultConfig()
	defaultCfg.ServiceName = "default-demo"
	defaultCfg.Level = axio.LevelDebug

	defaultLogger, _ := axio.New(defaultCfg)
	defaultLogger.Debug(ctx, "Logger created with DefaultConfig() and customized")

	fmt.Println()

	// =========================================================================
	// 12. OPTION COMBINATION
	// =========================================================================
	fmt.Println("[12] Combining Multiple Options")
	fmt.Println(strings.Repeat("-", 60))

	combinedLogger, _ := axio.New(axio.Config{
		ServiceName: "combined-demo",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	},
		axio.WithOutputs(axio.Console(axio.FormatText)),
		axio.WithPII(nil, nil), // uses defaults
		axio.WithTracer(axio.Otel()),
	)

	combinedLogger.With(
		axio.Annotate("transaction_id", "txn_987654"),
	).Info(ctx, "Log with multiple combined options")

	fmt.Println()

	// =========================================================================
	// SUMMARY
	// =========================================================================
	fmt.Println("=" + strings.Repeat("=", 59))
	fmt.Println("  Complete Example Finished!")
	fmt.Println("=" + strings.Repeat("=", 59))
	fmt.Println()
	fmt.Println("Files created:")
	fmt.Printf("  📝 %s\n", logFile)
	fmt.Printf("  🔗 %s\n", chainStorePath)
	fmt.Println()
	fmt.Println("Demonstrated features:")
	fmt.Println("  ✅ Basic logger (Development/Production)")
	fmt.Println("  ✅ Config loading (LoadConfig, LoadConfigFrom)")
	fmt.Println("  ✅ Multiple outputs (Console, File)")
	fmt.Println("  ✅ Log levels (Debug, Info, Warn, Error)")
	fmt.Println("  ✅ Structured annotations (Annotate, HTTP)")
	fmt.Println("  ✅ Named loggers (sub-loggers)")
	fmt.Println("  ✅ PII Masking (MaskString, MaskFields)")
	fmt.Println("  ✅ Hash Chain (audit with integrity)")
	fmt.Println("  ✅ Tracing (OpenTelemetry)")
	fmt.Println("  ✅ Agent Mode (Promtail, Fluent Bit)")
	fmt.Println("  ✅ DefaultConfig + customization")
	fmt.Println("  ✅ Combining multiple options")
	fmt.Println()

	// Cleanup
	time.Sleep(100 * time.Millisecond) // wait for log flush
}
