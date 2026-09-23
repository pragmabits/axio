package cli_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pragmabits/axio"
)

func TestVerify(t *testing.T) {
	t.Run("intact_log_verifies", func(t *testing.T) {
		logPath, storePath := auditedLog(t)
		output, err := execute(t, "", "verify", "--store", storePath, logPath)
		assertNoError(t, err)
		assertPresent(t, output, "verified")
	})

	t.Run("reads_standard_input", func(t *testing.T) {
		logPath, storePath := auditedLog(t)
		_, err := execute(t, readText(t, logPath), "verify", "--store", storePath)
		assertNoError(t, err)
	})

	t.Run("edited_log_fails_naming_the_line", func(t *testing.T) {
		logPath, storePath := auditedLog(t)
		edited := strings.Replace(readText(t, logPath), `"error":"card declined"`, `"error":"none"`, 1)
		_, err := execute(t, edited, "verify", "--store", storePath)
		if !errors.Is(err, axio.ErrHashMismatch) || !strings.Contains(err.Error(), "line 2") {
			t.Errorf("expected a hash mismatch on line 2, got %v", err)
		}
	})

	t.Run("truncated_log_is_incomplete", func(t *testing.T) {
		logPath, storePath := auditedLog(t)
		lines := strings.SplitAfter(readText(t, logPath), "\n")
		_, err := execute(t, strings.Join(lines[:2], ""), "verify", "--store", storePath)
		if !errors.Is(err, axio.ErrChainIncomplete) {
			t.Errorf("expected ErrChainIncomplete, got %v", err)
		}
	})

	t.Run("rotated_files_verify_in_order", func(t *testing.T) {
		older, newer, storePath := rotatedLogs(t)
		_, err := execute(t, "", "verify", "--store", storePath, older, newer)
		assertNoError(t, err)
	})

	t.Run("newer_file_alone_needs_the_hash_before_it", func(t *testing.T) {
		older, newer, storePath := rotatedLogs(t)
		_, err := execute(t, "", "verify", "--store", storePath, newer)
		if !errors.Is(err, axio.ErrChainBroken) {
			t.Errorf("expected ErrChainBroken without --previous-hash, got %v", err)
		}

		olderLines := strings.Split(strings.TrimSpace(readText(t, older)), "\n")
		lastHash := olderLines[len(olderLines)-1][strings.LastIndex(olderLines[len(olderLines)-1], `"hash":"`)+len(`"hash":"`):]
		lastHash = strings.TrimSuffix(lastHash, `"}`)
		_, err = execute(t, "", "verify", "--store", storePath, "--previous-hash", lastHash, newer)
		assertNoError(t, err)
	})

	t.Run("previous_hash_must_be_a_hash", func(t *testing.T) {
		logPath, storePath := auditedLog(t)
		_, err := execute(t, "", "verify", "--store", storePath, "--previous-hash", logPath)
		if err == nil || !strings.Contains(err.Error(), "64 hexadecimal characters") {
			t.Errorf("expected --previous-hash to be rejected, got %v", err)
		}
	})

	t.Run("store_flag_is_required", func(t *testing.T) {
		_, err := execute(t, "", "verify", "some.log")
		if err == nil || !strings.Contains(err.Error(), `"store" not set`) {
			t.Errorf("expected the missing --store to be reported, got %v", err)
		}
	})

	t.Run("store_flag_completes_json_files", func(t *testing.T) {
		output, err := execute(t, "", "__complete", "verify", "--store", "")
		assertNoError(t, err)
		assertPresent(t, output, "json")
	})

	t.Run("missing_log_file_fails_naming_it", func(t *testing.T) {
		_, storePath := auditedLog(t)
		missing := filepath.Join(t.TempDir(), "missing.log")
		_, err := execute(t, "", "verify", "--store", storePath, missing)
		if err == nil || !strings.Contains(err.Error(), missing) {
			t.Errorf("expected an error naming %s, got %v", missing, err)
		}
	})
}

// auditedLog writes three audited JSON entries and returns the log and store paths.
func auditedLog(t *testing.T) (string, string) {
	t.Helper()
	directory := t.TempDir()
	logPath, storePath := filepath.Join(directory, "audit.log"), filepath.Join(directory, "chain.json")
	writeEntries(t, logPath, storePath, "user admin login")
	return logPath, storePath
}

// rotatedLogs writes an audited log in two files, as rotation would, and
// returns the older file, the newer file and the store path.
func rotatedLogs(t *testing.T) (string, string, string) {
	t.Helper()
	directory := t.TempDir()
	older, newer, storePath := filepath.Join(directory, "audit.log.1"), filepath.Join(directory, "audit.log"), filepath.Join(directory, "chain.json")
	writeEntries(t, older, storePath, "before rotation")
	writeEntries(t, newer, storePath, "after rotation")
	return older, newer, storePath
}

// writeEntries logs an info, an error and an info to logPath, audited through storePath.
func writeEntries(t *testing.T, logPath, storePath, message string) {
	t.Helper()
	logger, err := axio.New(
		axio.Config{ServiceName: "payments", Environment: axio.Production, Level: axio.LevelInfo},
		axio.WithOutputs(axio.MustFile(logPath, axio.FormatJSON)),
		axio.WithAudit(storePath),
	)
	assertNoError(t, err)
	ctx := context.Background()
	logger.Info(ctx, message)
	logger.Error(ctx, errors.New("card declined"), "payment failed")
	logger.Info(ctx, "user admin logout")
	assertNoError(t, logger.Close())
}

// readText returns the content of the file at path.
func readText(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
