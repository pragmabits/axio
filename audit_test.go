package axio

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

func TestHashChain_Add(t *testing.T) {
	t.Run("first_entry_has_empty_previous_hash", func(t *testing.T) {
		chain, _ := NewHashChain(nil)

		hash, prevHash, err := chain.Add([]byte("first entry"))
		assertNoError(t, err)

		if prevHash != "" {
			t.Errorf("first entry should have empty previous_hash, got %q", prevHash)
		}
		if hash == "" {
			t.Error("hash should not be empty")
		}
	})

	t.Run("subsequent_entries_link_to_previous", func(t *testing.T) {
		chain, _ := NewHashChain(nil)

		hash1, _, _ := chain.Add([]byte("first"))
		hash2, prevHash2, _ := chain.Add([]byte("second"))
		hash3, prevHash3, _ := chain.Add([]byte("third"))

		assertEqual(t, prevHash2, hash1)
		assertEqual(t, prevHash3, hash2)
		if hash1 == hash2 || hash2 == hash3 {
			t.Error("hashes should be unique for different data")
		}
	})

	t.Run("sequence_increments", func(t *testing.T) {
		chain, _ := NewHashChain(nil)

		assertEqual(t, chain.Sequence(), uint64(0))

		chain.Add([]byte("1"))
		assertEqual(t, chain.Sequence(), uint64(1))

		chain.Add([]byte("2"))
		assertEqual(t, chain.Sequence(), uint64(2))
	})

	t.Run("same_data_same_sequence_produces_different_hash_due_to_timestamp", func(t *testing.T) {
		chain1, _ := NewHashChain(nil)
		chain2, _ := NewHashChain(nil)

		hash1, _, _ := chain1.Add([]byte("same"))
		time.Sleep(time.Millisecond)
		hash2, _, _ := chain2.Add([]byte("same"))

		if hash1 == hash2 {
			t.Error("hashes should be different due to timestamp")
		}
	})
}

func TestHashChain_Verify(t *testing.T) {
	t.Run("tampered_hash_detected", func(t *testing.T) {
		chain, _ := NewHashChain(nil)

		// Create entry with a valid hash
		timestamp := time.Now().UTC()
		data := []byte("original")
		entry := ChainEntry{
			Sequence:     1,
			Timestamp:    timestamp,
			PreviousHash: "",
		}
		hash := chain.computeHash(entry, data)
		entry.Hash = hash

		// Tamper with the stored hash
		entry.Hash = hash + "tampered"

		entries := []ChainEntry{entry}
		err := chain.Verify(entries, func(i int) []byte {
			return data
		})

		if err == nil {
			t.Error("expected invalid hash error")
		}
		if !errors.Is(err, ErrHashMismatch) {
			t.Errorf("expected ErrHashMismatch, got %v", err)
		}
	})

	t.Run("broken_chain_detected", func(t *testing.T) {
		chain, _ := NewHashChain(nil)
		data := [][]byte{[]byte("first"), []byte("second")}

		// Create first entry correctly
		timestamp1 := time.Now().UTC()
		entry1 := ChainEntry{
			Sequence:     1,
			Timestamp:    timestamp1,
			PreviousHash: "",
		}
		entry1.Hash = chain.computeHash(entry1, data[0])

		// Create second entry with the wrong previous_hash
		timestamp2 := time.Now().UTC()
		entry2 := ChainEntry{
			Sequence:     2,
			Timestamp:    timestamp2,
			PreviousHash: "wrong_previous_hash", // Broken chain
		}
		entry2.Hash = chain.computeHash(entry2, data[1])

		entries := []ChainEntry{entry1, entry2}
		err := chain.Verify(entries, func(i int) []byte {
			return data[i]
		})

		if err == nil {
			t.Error("expected broken chain error")
		}
		if !errors.Is(err, ErrChainBroken) {
			t.Errorf("expected ErrChainBroken, got %v", err)
		}
	})

	t.Run("valid_chain_passes", func(t *testing.T) {
		chain, _ := NewHashChain(nil)
		data := [][]byte{[]byte("entry1"), []byte("entry2")}

		// Build valid entries with consistent timestamps
		timestamp1 := time.Now().UTC()
		entry1 := ChainEntry{
			Sequence:     1,
			Timestamp:    timestamp1,
			PreviousHash: "",
		}
		entry1.Hash = chain.computeHash(entry1, data[0])

		timestamp2 := time.Now().UTC()
		entry2 := ChainEntry{
			Sequence:     2,
			Timestamp:    timestamp2,
			PreviousHash: entry1.Hash, // Correct chaining
		}
		entry2.Hash = chain.computeHash(entry2, data[1])

		entries := []ChainEntry{entry1, entry2}
		err := chain.Verify(entries, func(i int) []byte {
			return data[i]
		})
		assertNoError(t, err)
	})
}

func TestFileStore(t *testing.T) {
	t.Run("save_and_load", func(t *testing.T) {
		path := tempFile(t, "chain.json")
		store := NewFileStore(path)

		err := store.Save(42, "abc123hash")
		assertNoError(t, err)

		seq, hash, err := store.Load()
		assertNoError(t, err)

		assertEqual(t, seq, uint64(42))
		assertEqual(t, hash, "abc123hash")
	})

	t.Run("load_nonexistent_returns_zero", func(t *testing.T) {
		path := tempFile(t, "nonexistent.json")
		store := NewFileStore(path)

		seq, hash, err := store.Load()
		assertNoError(t, err)

		assertEqual(t, seq, uint64(0))
		assertEqual(t, hash, "")
	})

	t.Run("load_corrupted_file_returns_error", func(t *testing.T) {
		path := tempFile(t, "corrupted.json")
		writeFile(t, path, "not valid json")

		store := NewFileStore(path)
		_, _, err := store.Load()

		assertError(t, err)
		if !errors.Is(err, ErrUnmarshalChainState) {
			t.Errorf("expected ErrUnmarshalChainState, got %v", err)
		}
	})

	t.Run("save_creates_file_with_correct_permissions", func(t *testing.T) {
		path := tempFile(t, "permissions.json")
		store := NewFileStore(path)

		store.Save(1, "hash")

		info, err := os.Stat(path)
		assertNoError(t, err)

		// 0600 = owner read/write only
		if info.Mode().Perm() != 0600 {
			t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
		}
	})
}

func TestHashChain_WithStore(t *testing.T) {
	t.Run("loads_state_from_store", func(t *testing.T) {
		path := tempFile(t, "chain.json")
		store := NewFileStore(path)

		// Save initial state
		store.Save(10, "existinghash")

		// Create chain with store
		chain, err := NewHashChain(store)
		assertNoError(t, err)

		assertEqual(t, chain.Sequence(), uint64(10))
		assertEqual(t, chain.LastHash(), "existinghash")
	})

	t.Run("persists_state_on_add", func(t *testing.T) {
		path := tempFile(t, "chain.json")
		store := NewFileStore(path)

		chain, _ := NewHashChain(store)
		chain.Add([]byte("entry"))

		// Verify persisted file
		data, _ := os.ReadFile(path)
		var state struct {
			Sequence uint64 `json:"sequence"`
			LastHash string `json:"last_hash"`
		}
		json.Unmarshal(data, &state)

		assertEqual(t, state.Sequence, uint64(1))
		if state.LastHash == "" {
			t.Error("last_hash should have been persisted")
		}
	})
}

func TestAuditHook(t *testing.T) {
	t.Run("adds_hash_to_entry", func(t *testing.T) {
		hook, err := NewAuditHook(nil)
		assertNoError(t, err)

		entry := &Entry{
			Timestamp: time.Now(),
			Level:     LevelInfo,
			Message:   "test message",
		}

		err = hook.Process(context.Background(), entry)
		assertNoError(t, err)

		if entry.Hash == "" {
			t.Error("entry should have hash after processing")
		}
	})

	t.Run("chains_hashes", func(t *testing.T) {
		hook, _ := NewAuditHook(nil)

		entry1 := &Entry{Timestamp: time.Now(), Message: "first"}
		entry2 := &Entry{Timestamp: time.Now(), Message: "second"}

		hook.Process(context.Background(), entry1)
		hook.Process(context.Background(), entry2)

		if entry2.PreviousHash != entry1.Hash {
			t.Error("second entry should have previous_hash equal to the first hash")
		}
	})

	t.Run("hook_name", func(t *testing.T) {
		hook, _ := NewAuditHook(nil)
		assertEqual(t, hook.Name(), "audit")
	})

	t.Run("with_file_store", func(t *testing.T) {
		path := tempFile(t, "audit.json")
		store := NewFileStore(path)

		hook, err := NewAuditHook(store)
		assertNoError(t, err)

		entry := &Entry{Timestamp: time.Now(), Message: "test"}
		hook.Process(context.Background(), entry)

		// Verify persistence
		_, err = os.Stat(path)
		assertNoError(t, err)
	})
}

func TestMustAuditHook(t *testing.T) {
	t.Run("valid_store_returns_hook", func(t *testing.T) {
		hook := MustAuditHook(nil)
		if hook == nil {
			t.Error("expected non-nil hook")
		}
	})
}

// mockFailingStore simulates store failures for error tests
type mockFailingStore struct{}

func (m mockFailingStore) Save(uint64, string) error {
	return errors.New("simulated save error")
}

func (m mockFailingStore) Load() (uint64, string, error) {
	return 0, "", nil
}

func TestHashChain_AddWithFailingStore(t *testing.T) {
	chain, _ := NewHashChain(mockFailingStore{})

	_, _, err := chain.Add([]byte("data"))
	assertError(t, err)

	// Verify that the sequence was not incremented after failure
	assertEqual(t, chain.Sequence(), uint64(0))
}
