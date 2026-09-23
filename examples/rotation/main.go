// Package main demonstrates log file rotation: size-based and time-based.
// Both modes are verified by checking that backup files are actually created.
//
// Run with: go run ./examples/rotation/
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pragmabits/axio"
)

func main() {
	ctx := context.Background()

	sizeRotation(ctx)
	timeRotation(ctx)
}

func sizeRotation(ctx context.Context) {
	fmt.Println("=== Size-based Rotation (MaxSize=1MB) ===")

	tempDir, err := os.MkdirTemp("", "axio-size-rotation-*")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "size-test.log")

	logger, err := axio.New(axio.Config{
		ServiceName: "size-rotation",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	},
		axio.WithOutputs(
			axio.MustRotatingFile(testFile, axio.FormatJSON, axio.RotationConfig{
				MaxSize:    1,
				MaxBackups: 3,
				Compress:   false,
			}),
		),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Write enough data to exceed 1MB
	payload := strings.Repeat("x", 200)
	for index := range 4000 {
		logger.Info(ctx, "Size rotation test line %d: %s", index, payload)
	}
	logger.Close()

	reportBackups(tempDir, "size-test.log", "Size-based")
}

func timeRotation(ctx context.Context) {
	fmt.Println("=== Time-based Rotation (Interval=200ms) ===")

	tempDir, err := os.MkdirTemp("", "axio-time-rotation-*")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "time-test.log")

	logger, err := axio.New(axio.Config{
		ServiceName: "time-rotation",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	},
		axio.WithOutputs(
			axio.MustRotatingFile(testFile, axio.FormatJSON, axio.RotationConfig{
				MaxSize:    100, // large enough to never trigger size rotation
				MaxBackups: 3,
				Compress:   false,
				Interval:   axio.Duration(200 * time.Millisecond),
			}),
		),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Write lines, wait for ticker, write again
	for index := range 10 {
		logger.Info(ctx, "Before time rotation, line %d", index)
	}
	time.Sleep(300 * time.Millisecond)
	logger.Info(ctx, "After time rotation")
	logger.Close()

	reportBackups(tempDir, "time-test.log", "Time-based")
}

// reportBackups lists the files in tempDir and reports whether rotation left
// at least one backup beside the active log file.
func reportBackups(tempDir, activeName, label string) {
	entries, _ := os.ReadDir(tempDir)
	var backups int
	for _, entry := range entries {
		info, _ := entry.Info()
		if info != nil {
			fmt.Printf("  - %s (%d bytes)\n", entry.Name(), info.Size())
		}
		if entry.Name() != activeName {
			backups++
		}
	}

	if backups > 0 {
		fmt.Printf("[ok] %s rotation verified\n", label)
	} else {
		fmt.Printf("[FAIL] %s rotation did not trigger\n", label)
	}
	fmt.Println()
}
