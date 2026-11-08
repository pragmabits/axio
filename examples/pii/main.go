// Package main demonstrates PII masking: the MaskString API, the masking every
// logger does by default, and the identifier shapes it leaves as they are.
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
	identifierShapes()
	loggerMasking(ctx)
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

func identifierShapes() {
	fmt.Println("=== Identifier Shapes ===")

	const name = "contracts/20261107041818-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855.csv"
	config := axio.DefaultPIIConfig()
	config.IdentifierShapes = []string{`\d{14,23}-[0-9a-f]{64}\.csv`}
	shaped := axio.MustPIIMasker(config)
	unshaped := axio.MustPIIMasker(axio.DefaultPIIConfig())

	fmt.Printf("  Without the shape: %s\n", unshaped.MaskString(name))
	fmt.Printf("  With the shape:    %s\n", shaped.MaskString(name))
	fmt.Printf("  CPF beside it:     %s\n\n", shaped.MaskString("cpf 123.456.789-01 wrote "+name))
}

func loggerMasking(ctx context.Context) {
	fmt.Println("=== Logger Masking by Default (Console + File) ===")

	logFile := "axio-pii-example.log"

	logger, _ := axio.New(axio.Config{
		ServiceName: "pii-demo",
		Environment: axio.EnvironmentDevelopment,
		Level:       axio.LevelInfo,
	},
		axio.WithOutputs(
			axio.Console(axio.FormatText),
			axio.MustFile(logFile, axio.FormatJSON),
		),
	)

	logger.Info(ctx, "Customer CPF 123.456.789-01 purchased with card 4111-1111-1111-1111")

	logger.Info(ctx, "Customer data processed",
		axio.Field("cpf", "987.654.321-00"),
		axio.Field("email", "cliente@empresa.com.br"),
		axio.Field("password", "super_secret"),
	)
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
