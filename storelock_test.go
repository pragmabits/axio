//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package axio

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingFile_Lock(t *testing.T) {
	t.Run("second_process_fails_in_new", func(t *testing.T) {
		path := filepath.Join(tempDir(t), "app.log")
		holdRotationLock(t, path)
		config := minimalConfig()
		config.Outputs = []OutputConfig{{Type: OutputFile, Format: FormatJSON, Path: path, Rotation: RotationConfig{MaxSize: 10}}}

		_, err := New(config)
		if !errors.Is(err, ErrRotatingFileLocked) || !errors.Is(err, ErrBuildOutputs) {
			t.Errorf("expected ErrBuildOutputs wrapping ErrRotatingFileLocked, got %v", err)
		}
	})

	t.Run("lock_is_held_while_open", func(t *testing.T) {
		path := filepath.Join(tempDir(t), "app.log")
		output, err := RotatingFile(path, FormatJSON, RotationConfig{MaxSize: 10})
		assertNoError(t, err)
		defer output.Close()

		if lockTaken(t, path) {
			t.Error("expected the lock of an open rotating file to be held")
		}
	})

	t.Run("last_close_releases_the_lock", func(t *testing.T) {
		path := filepath.Join(tempDir(t), "app.log")
		output, err := RotatingFile(path, FormatJSON, RotationConfig{MaxSize: 10})
		assertNoError(t, err)
		assertNoError(t, output.Close())

		if !lockTaken(t, path) {
			t.Error("expected the lock to be free after the last close")
		}
	})
}

func TestRotator_Cleanup_Lock(t *testing.T) {
	t.Run("lock_is_kept_until_cleanup_ends", func(t *testing.T) {
		path := filepath.Join(tempDir(t), "app.log")
		writer, unblock := blockedCompressionRotator(t, path)
		writeLine(t, writer, "first\n")
		writeLine(t, writer, "second\n")
		assertNoError(t, writer.release())

		if lockTaken(t, path) {
			t.Error("expected the lock to be held while the cleanup runs")
		}
		unblock()
		writer.waitCleanup()
		if !lockTaken(t, path) {
			t.Error("expected the lock to be free once the cleanup ended")
		}
	})
}

// lockTaken tries the lock of the rotating file at path through a descriptor of
// its own, and reports whether it got it.
func lockTaken(t *testing.T, path string) bool {
	t.Helper()
	directory, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		t.Fatalf("resolve %s: %v", path, err)
	}
	file, err := os.OpenFile(filepath.Join(directory, filepath.Base(path))+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open lock of %s: %v", path, err)
	}
	defer func() { _ = file.Close() }()
	locked, err := lockExclusive(file)
	if err != nil {
		t.Fatalf("try lock of %s: %v", path, err)
	}
	return locked
}

// holdRotationLock takes the lock of the rotating file at path through a
// descriptor of its own, as another process would, until the test ends.
func holdRotationLock(t *testing.T, path string) {
	t.Helper()
	directory, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		t.Fatalf("resolve %s: %v", path, err)
	}
	file, err := os.OpenFile(filepath.Join(directory, filepath.Base(path))+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open lock of %s: %v", path, err)
	}
	t.Cleanup(func() { _ = file.Close() })
	locked, err := lockExclusive(file)
	if err != nil || !locked {
		t.Fatalf("take lock of %s: locked=%v err=%v", path, locked, err)
	}
}

func TestFileStore_Lock(t *testing.T) {
	t.Run("same_store_saves_again", func(t *testing.T) {
		store := NewFileStore(tempFile(t, "chain.json"))
		assertNoError(t, store.Save(1, "first"))
		assertNoError(t, store.Save(2, "second"))
	})

	t.Run("second_store_on_the_same_path_cannot_save", func(t *testing.T) {
		path := tempFile(t, "chain.json")
		assertNoError(t, NewFileStore(path).Save(1, "first"))

		err := NewFileStore(path).Save(2, "forked")
		if !errors.Is(err, ErrChainStoreLocked) || !errors.Is(err, ErrSaveChainState) {
			t.Errorf("expected ErrSaveChainState wrapping ErrChainStoreLocked, got %v", err)
		}
	})

	t.Run("load_does_not_take_the_lock", func(t *testing.T) {
		path := tempFile(t, "chain.json")
		assertNoError(t, NewFileStore(path).Save(7, "held"))

		sequence, lastHash, err := NewFileStore(path).Load()
		assertNoError(t, err)
		assertEqual(t, sequence, uint64(7))
		assertEqual(t, lastHash, "held")
	})

	t.Run("audit_on_a_store_in_use_fails_in_new", func(t *testing.T) {
		path := tempFile(t, "chain.json")
		assertNoError(t, NewFileStore(path).Save(1, "held"))

		_, err := New(minimalConfig(), WithOutputs(newBufferOutput(FormatJSON)), WithAudit(path))
		if !errors.Is(err, ErrChainStoreLocked) || !errors.Is(err, ErrBuildAudit) {
			t.Errorf("expected ErrBuildAudit wrapping ErrChainStoreLocked, got %v", err)
		}
	})

	t.Run("audit_chain_on_a_store_in_use_fails_in_new", func(t *testing.T) {
		path := tempFile(t, "chain.json")
		assertNoError(t, NewFileStore(path).Save(1, "held"))
		chain, err := NewHashChain(NewFileStore(path))
		assertNoError(t, err)

		_, err = New(minimalConfig(), WithOutputs(newBufferOutput(FormatJSON)), WithAuditChain(chain))
		if !errors.Is(err, ErrChainStoreLocked) || !errors.Is(err, ErrBuildAudit) {
			t.Errorf("expected ErrBuildAudit wrapping ErrChainStoreLocked, got %v", err)
		}
		_, err = NewEvent("checkout", minimalConfig(), WithOutputs(newBufferOutput(FormatJSON)), WithAuditChain(chain))
		if !errors.Is(err, ErrChainStoreLocked) {
			t.Errorf("expected ErrChainStoreLocked from NewEvent, got %v", err)
		}
	})

	t.Run("audit_chain_continues_what_the_store_holds_when_claimed", func(t *testing.T) {
		path := tempFile(t, "chain.json")
		chain, err := NewHashChain(NewFileStore(path))
		assertNoError(t, err)
		later := strings.Repeat("a", 64)
		other := NewFileStore(path)
		assertNoError(t, other.Save(7, later))
		assertNoError(t, other.lockFile.Close())

		output := newBufferOutput(FormatJSON)
		logger, err := New(minimalConfig(), WithOutputs(output), WithAuditChain(chain))
		assertNoError(t, err)
		logger.Info(context.Background(), "order created")
		assertNoError(t, logger.Close())

		assertEqual(t, parseJSONLines(t, output.String())[0]["previous_hash"], any(later))
	})
}
