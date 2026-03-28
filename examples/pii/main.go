// Package main demonstrates PII masking: the MaskString API and PIIHook integration.
//
// Run with: go run ./examples/pii/
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/pragmabits/axio"
)

func main() {
	ctx := context.Background()

	maskStringAPI()
	piiHook(ctx)
}

func maskStringAPI() {
	fmt.Println("=== PIIMasker API ===")

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
}

func piiHook(ctx context.Context) {
	fmt.Println("=== PII Hook (Console + File) ===")

	logFile := "axio-pii-example.log"

	logger, _ := axio.New(axio.Config{
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

	logger.Info(ctx, "Customer CPF 123.456.789-01 purchased with card 4111-1111-1111-1111")

	logger.With(
		axio.Annotate("cpf", "987.654.321-00"),
		axio.Annotate("email", "cliente@empresa.com.br"),
		axio.Annotate("password", "super_secret"),
	).Info(ctx, "Customer data processed")
	logger.Close()

	// Print file content to show masking in JSON output
	fmt.Println()
	fmt.Println("File content (masked):")
	if content, err := os.ReadFile(logFile); err == nil {
		fmt.Print(string(content))
	}
	os.Remove(logFile)
	fmt.Println()
}
