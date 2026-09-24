//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package axio

import (
	"context"
	"errors"
	"strings"
	"testing"
)

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
