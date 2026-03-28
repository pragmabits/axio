// Package main demonstrates the hash chain for tamper-proof audit logging.
//
// Run with: go run ./examples/audit/
package main

import (
	"fmt"
	"os"

	"github.com/pragmabits/axio"
)

func main() {
	fmt.Println("=== Hash Chain (Audit with Integrity) ===")

	chainFile := "axio-audit-example.json"
	store := axio.NewFileStore(chainFile)

	chain, err := axio.NewHashChain(store)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	entries := []string{
		"User admin login",
		"Permission change for operators group",
		"Customer data export",
		"User admin logout",
	}

	fmt.Println("Adding entries to hash chain:")
	for index, entry := range entries {
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

	fmt.Printf("\nChain persisted to: %s\n", chainFile)
	fmt.Printf("Sequence: %d\n", chain.Sequence())
	fmt.Printf("Last hash: %s...\n", chain.LastHash()[:16])

	// Print chain file content
	if content, err := os.ReadFile(chainFile); err == nil {
		fmt.Printf("File: %s\n", string(content))
	}
	os.Remove(chainFile)
}
