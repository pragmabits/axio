// Package main demonstrates all the functionalities of the axio package.
//
// Run with: go run ./examples/complete/
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/pragmabits/axio"
)

const (
	logFilePath        = "axio-example.log"
	auditChainFilePath = "axio-audit-chain.json"
	configFilePath     = "examples/complete/config.yaml"
	separator          = "------------------------------------------------------------"
)

func main() {
	ctx := context.Background()

	fmt.Println("============================================================")
	fmt.Println("  Axio Logger - Complete Example")
	fmt.Println("============================================================")
	fmt.Println()

	demonstrateBasicLogger(ctx)
	demonstrateConfigLoading(ctx)
	demonstrateMultipleOutputs(ctx)
	demonstrateAnnotations(ctx)
	demonstrateNamedLoggers(ctx)
	demonstratePIIMasking(ctx)
	demonstrateHashChain()
	demonstrateTracing(ctx)
	demonstrateAgentMode(ctx)
	demonstrateProductionEnvironment(ctx)
	demonstrateDefaultConfig(ctx)
	demonstrateCombinedOptions(ctx)

	printSummary()
	cleanup()
}

// demonstrateBasicLogger shows a development logger with all log levels.
func demonstrateBasicLogger(ctx context.Context) {
	fmt.Println("[1] Basic Logger - Development Environment")
	fmt.Println(separator)

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
	defer logger.Close()

	logger.Debug(ctx, "Debug log - visible only in Development")
	logger.Info(ctx, "Application started successfully")
	logger.Warn(ctx, errors.New("slow connection"), "High latency detected")
	logger.Error(ctx, errors.New("timeout"), "Failed to connect to database")

	fmt.Println()
}

// demonstrateConfigLoading shows loading configuration from files and readers.
func demonstrateConfigLoading(ctx context.Context) {
	fmt.Println("[2] Configuration Loading from File")
	fmt.Println(separator)

	loadConfigFromFile(ctx)
	loadConfigFromYAMLReader()
	loadConfigFromJSONReader()

	fmt.Println()
}

func loadConfigFromFile(ctx context.Context) {
	if _, err := os.Stat(configFilePath); err != nil {
		fmt.Printf("  [warn] File not found: %s\n", configFilePath)
		fmt.Println("         Run from project root: go run ./examples/complete/")
		return
	}

	fileConfig, err := axio.LoadConfig(configFilePath)
	if err != nil {
		fmt.Printf("  [warn] Error loading config.yaml: %v\n", err)
		return
	}

	fmt.Printf("  [ok] Config loaded from: %s\n", configFilePath)
	fmt.Printf("       ServiceName: %s\n", fileConfig.ServiceName)
	fmt.Printf("       Environment: %s\n", fileConfig.Environment)
	fmt.Printf("       Level: %s\n", fileConfig.Level)
	fmt.Printf("       PIIEnabled: %v\n", fileConfig.PIIEnabled)
	fmt.Printf("       Audit.Enabled: %v\n", fileConfig.Audit.Enabled)

	fileLogger, err := axio.New(fileConfig)
	if err != nil {
		fmt.Printf("  [warn] Error creating logger: %v\n", err)
		return
	}
	defer fileLogger.Close()

	fileLogger.Info(ctx, "Logger created from config.yaml")
}

func loadConfigFromYAMLReader() {
	yamlContent := `
serviceName: "from-yaml-reader"
serviceVersion: "2.0.0"
environment: "staging"
level: "info"
`
	readerConfig, err := axio.LoadConfigFrom(bytes.NewReader([]byte(yamlContent)), "yaml")
	if err != nil {
		fmt.Printf("  [warn] Error loading YAML config: %v\n", err)
		return
	}

	fmt.Printf("  [ok] Config loaded from io.Reader (YAML)\n")
	fmt.Printf("       ServiceName: %s\n", readerConfig.ServiceName)
	fmt.Printf("       Environment: %s\n", readerConfig.Environment)
}

func loadConfigFromJSONReader() {
	jsonContent := `{
		"serviceName": "from-json-reader",
		"serviceVersion": "3.0.0",
		"environment": "production",
		"level": "warn"
	}`

	jsonConfig, err := axio.LoadConfigFrom(bytes.NewReader([]byte(jsonContent)), "json")
	if err != nil {
		fmt.Printf("  [warn] Error loading JSON config: %v\n", err)
		return
	}

	fmt.Printf("  [ok] Config loaded from io.Reader (JSON)\n")
	fmt.Printf("       ServiceName: %s\n", jsonConfig.ServiceName)
	fmt.Printf("       Environment: %s\n", jsonConfig.Environment)
}

// demonstrateMultipleOutputs shows logging to both console and file simultaneously.
func demonstrateMultipleOutputs(ctx context.Context) {
	fmt.Println("[3] Multiple Outputs (Console + File)")
	fmt.Println(separator)

	multiOutputConfig := axio.Config{
		ServiceName:    "multi-output",
		ServiceVersion: "1.0.0",
		Environment:    axio.Development,
		Level:          axio.LevelInfo,
	}

	multiOutputLogger, err := axio.New(multiOutputConfig,
		axio.WithOutputs(
			axio.Console(axio.FormatText),                    // colorized terminal (stderr)
			axio.MustFile(logFilePath, axio.FormatJSON), // structured JSON file
		),
	)
	if err != nil {
		fmt.Printf("Error creating multi-output logger: %v\n", err)
		os.Exit(1)
	}
	defer multiOutputLogger.Close()

	multiOutputLogger.With(
		axio.Annotate("request_id", "req-001"),
	).Info(ctx, "This log appears in both console AND file")

	fmt.Printf("  Log file created: %s\n", logFilePath)
	fmt.Println()
}

// demonstrateAnnotations shows structured annotations and HTTP metadata.
func demonstrateAnnotations(ctx context.Context) {
	fmt.Println("[4] Structured Annotations (Annotate + HTTP)")
	fmt.Println(separator)

	annotatedLogger, _ := axio.New(axio.Config{
		ServiceName: "sales-api",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	})
	defer annotatedLogger.Close()

	// Simple key-value annotations
	annotatedLogger.With(
		axio.Annotate("user_id", "usr_12345"),
		axio.Annotate("tenant", "acme-corp"),
		axio.Annotate("action", "checkout"),
	).Info(ctx, "User initiated checkout")

	// HTTP annotation via Annotable interface
	httpAnnotation := axio.HTTP{
		Method:     "POST",
		URL:        "/api/v1/orders",
		StatusCode: 201,
		LatencyMS:  45,
		UserAgent:  "Mozilla/5.0",
		ClientIP:   "192.168.1.100",
	}

	annotatedLogger.With(
		axio.Annotate("http", httpAnnotation),
	).Info(ctx, "Request processed successfully")

	fmt.Println()
}

// demonstrateNamedLoggers shows sub-logger creation with hierarchical names.
func demonstrateNamedLoggers(ctx context.Context) {
	fmt.Println("[5] Named Loggers (Sub-loggers)")
	fmt.Println(separator)

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

// demonstratePIIMasking shows PII detection and masking capabilities.
func demonstratePIIMasking(ctx context.Context) {
	fmt.Println("[6] PII Masking (PIIMasker API)")
	fmt.Println(separator)

	masker := axio.MustPIIMasker(axio.DefaultPIIConfig())

	testInputs := []string{
		"Customer CPF: 123.456.789-01",
		"Company CNPJ: 12.345.678/0001-90",
		"Email: contato@empresa.com.br",
		"Phone: (11) 99999-8888",
		"Card: 4111-1111-1111-1111",
	}

	for _, original := range testInputs {
		masked := masker.MaskString(original)
		fmt.Printf("  Original: %s\n", original)
		fmt.Printf("  Masked:   %s\n\n", masked)
	}

	demonstratePIIHook(ctx)
}

// demonstratePIIHook shows PII masking integrated via hook with dual output.
func demonstratePIIHook(ctx context.Context) {
	fmt.Println("[6.1] PII Masking via Hook (Console + File)")
	fmt.Println(separator)

	piiLogger, _ := axio.New(axio.Config{
		ServiceName: "pii-demo",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	},
		axio.WithOutputs(
			axio.Console(axio.FormatText),                    // terminal output
			axio.MustFile(logFilePath, axio.FormatJSON), // file output
		),
		axio.WithHooks(axio.MustPIIHook(axio.DefaultPIIConfig())),
	)
	defer piiLogger.Close()

	// Message with sensitive data - automatically masked
	piiLogger.Info(ctx, "Customer CPF 123.456.789-01 purchased with card 4111-1111-1111-1111")

	// Annotations with sensitive data - also masked
	piiLogger.With(
		axio.Annotate("cpf", "987.654.321-00"),
		axio.Annotate("email", "cliente@empresa.com.br"),
		axio.Annotate("password", "super_secret"),
	).Info(ctx, "Customer data processed")

	fmt.Println("  [ok] CPF, card, email and password automatically masked")
	fmt.Println()
}

// demonstrateHashChain shows the audit hash chain for tamper-proof logging.
func demonstrateHashChain() {
	fmt.Println("[7] Hash Chain (Audit with Integrity)")
	fmt.Println(separator)

	store := axio.NewFileStore(auditChainFilePath)

	chain, err := axio.NewHashChain(store)
	if err != nil {
		fmt.Printf("  [warn] Error creating HashChain: %v\n", err)
		return
	}

	auditEntries := []string{
		"User admin login",
		"Permission change for operators group",
		"Customer data export",
		"User admin logout",
	}

	fmt.Println("  Adding entries to hash chain:")
	for index, entry := range auditEntries {
		hash, previousHash, err := chain.Add([]byte(entry))
		if err != nil {
			fmt.Printf("  [%d] ERROR: %v\n", index+1, err)
			continue
		}
		fmt.Printf("  [%d] %s\n", index+1, entry)
		fmt.Printf("      Hash: %s...\n", hash[:16])
		if previousHash != "" {
			fmt.Printf("      PrevHash: %s...\n", previousHash[:16])
		}
	}

	fmt.Printf("\n  [ok] Hash chain persisted to: %s\n", auditChainFilePath)
	fmt.Printf("       Current sequence: %d\n", chain.Sequence())
	fmt.Printf("       Last hash: %s...\n", chain.LastHash()[:16])

	fmt.Println()
}

// demonstrateTracing shows OpenTelemetry trace integration.
func demonstrateTracing(ctx context.Context) {
	fmt.Println("[8] Tracing (OpenTelemetry Integration)")
	fmt.Println(separator)

	tracingLogger, _ := axio.New(axio.Config{
		ServiceName: "tracing-demo",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	},
		axio.WithTracer(axio.Otel()),
	)
	defer tracingLogger.Close()

	tracingLogger.Info(ctx, "Log with tracing support (trace_id will be added if there is an active span)")

	fmt.Println()
}

// demonstrateAgentMode shows JSON-to-stdout mode for log collection agents.
func demonstrateAgentMode(ctx context.Context) {
	fmt.Println("[9] Agent Mode (JSON to stdout)")
	fmt.Println(separator)

	agentLogger, _ := axio.New(axio.Config{
		ServiceName: "agent-mode-demo",
		Environment: axio.Production,
		Level:       axio.LevelInfo,
	},
		axio.WithAgentMode(),
	)
	defer agentLogger.Close()

	agentLogger.Info(ctx, "Log optimized for agent collection")

	fmt.Println()
}

// demonstrateProductionEnvironment shows structured JSON logging with service metadata.
func demonstrateProductionEnvironment(ctx context.Context) {
	fmt.Println("[10] Production Environment (JSON + Metadata)")
	fmt.Println(separator)

	productionLogger, _ := axio.New(axio.Config{
		ServiceName:    "production-api",
		ServiceVersion: "2.5.1",
		Environment:    axio.Production,
		InstanceID:     "pod-abc123",
		Level:          axio.LevelInfo,
	})
	defer productionLogger.Close()

	productionLogger.Info(ctx, "Production log with service and deployment metadata")

	fmt.Println()
}

// demonstrateDefaultConfig shows using DefaultConfig with customizations.
func demonstrateDefaultConfig(ctx context.Context) {
	fmt.Println("[11] DefaultConfig + Customization")
	fmt.Println(separator)

	defaultConfig := axio.DefaultConfig()
	defaultConfig.ServiceName = "default-demo"
	defaultConfig.Level = axio.LevelDebug

	defaultLogger, _ := axio.New(defaultConfig)
	defer defaultLogger.Close()

	defaultLogger.Debug(ctx, "Logger created with DefaultConfig() and customized")

	fmt.Println()
}

// demonstrateCombinedOptions shows combining multiple options in a single logger.
func demonstrateCombinedOptions(ctx context.Context) {
	fmt.Println("[12] Combining Multiple Options")
	fmt.Println(separator)

	combinedLogger, _ := axio.New(axio.Config{
		ServiceName: "combined-demo",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	},
		axio.WithOutputs(axio.Console(axio.FormatText)),
		axio.WithPII(nil, nil), // uses defaults
		axio.WithTracer(axio.Otel()),
	)
	defer combinedLogger.Close()

	combinedLogger.With(
		axio.Annotate("transaction_id", "txn_987654"),
	).Info(ctx, "Log with multiple combined options")

	fmt.Println()
}

func printSummary() {
	fmt.Println("============================================================")
	fmt.Println("  Complete Example Finished!")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("Files created:")
	fmt.Printf("  - %s (JSON log output)\n", logFilePath)
	fmt.Printf("  - %s (audit hash chain)\n", auditChainFilePath)
	fmt.Println()
	fmt.Println("Demonstrated features:")

	features := []string{
		"Basic logger (Development/Production)",
		"Config loading (LoadConfig, LoadConfigFrom)",
		"Multiple outputs (Console, File)",
		"Log levels (Debug, Info, Warn, Error)",
		"Structured annotations (Annotate, HTTP via Annotable)",
		"Named loggers (sub-loggers)",
		"PII Masking (MaskString, Hook integration)",
		"Hash Chain (audit with integrity)",
		"Tracing (OpenTelemetry)",
		"Agent Mode (Promtail, Fluent Bit)",
		"DefaultConfig + customization",
		"Combining multiple options",
	}

	for _, feature := range features {
		fmt.Printf("  - %s\n", feature)
	}

	fmt.Println()
}

func cleanup() {
	fmt.Println("Log file content (" + logFilePath + "):")
	fmt.Println(separator)
	if content, err := os.ReadFile(logFilePath); err == nil {
		fmt.Println(string(content))
	}

	filesToRemove := []string{logFilePath, auditChainFilePath}
	for _, path := range filesToRemove {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: failed to remove %s: %v\n", path, err)
		}
	}
}

