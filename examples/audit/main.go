// Package main demonstrates the audit hash chain: a logger writes audited JSON
// lines, the chain verifies them, and a single edited byte is caught.
//
// Run with: go run ./examples/audit/
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pragmabits/axio"
)

func main() {
	fmt.Println("=== Audit Hash Chain ===")

	directory, err := os.MkdirTemp("", "axio-audit-*")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer os.RemoveAll(directory)
	logPath := filepath.Join(directory, "audit.log")
	storePath := filepath.Join(directory, "chain.json")

	if err := writeAuditedLog(logPath, storePath); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Audited JSON lines:\n%s\n", content)

	fmt.Printf("Verify intact log:   %v\n", verify(logPath, storePath))

	tampered := strings.Replace(string(content), `"error":"card declined"`, `"error":"none"`, 1)
	if err := os.WriteFile(logPath, []byte(tampered), 0o600); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Verify edited log:   %v\n", verify(logPath, storePath))
}

// writeAuditedLog logs three entries as audited JSON, chained through storePath.
func writeAuditedLog(logPath, storePath string) error {
	logger, err := axio.New(
		axio.Config{ServiceName: "payments", Environment: axio.Production, Level: axio.LevelInfo},
		axio.WithOutputs(axio.MustFile(logPath, axio.FormatJSON)),
		axio.WithAudit(storePath),
	)
	if err != nil {
		return err
	}
	ctx := context.Background()
	logger.Info(ctx, "user admin login")
	logger.Error(ctx, errors.New("card declined"), "payment failed")
	logger.Info(ctx, "user admin logout")
	return logger.Close()
}

// verify checks the log at logPath against the chain stored at storePath.
func verify(logPath, storePath string) error {
	chain, err := axio.NewHashChain(axio.NewFileStore(storePath))
	if err != nil {
		return err
	}
	file, err := os.Open(logPath)
	if err != nil {
		return err
	}
	defer file.Close()
	return chain.Verify(file, "")
}
