package axio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// AuditConfig represents the audit configuration with hash chain.
//
// When enabled, each log entry receives a SHA256 hash that includes
// the hash of the previous entry, forming a cryptographic chain that detects
// any tampering.
//
// YAML example:
//
//	audit:
//	  enabled: true
//	  storePath: /var/lib/axio/chain.json
type AuditConfig struct {
	// Enabled indicates whether auditing is enabled.
	Enabled bool `json:"enabled" yaml:"enabled" toml:"enabled" mapstructure:"enabled"`
	// StorePath is the file path to persist the chain state.
	// Required when Enabled is true.
	StorePath string `json:"storePath" yaml:"storePath" toml:"storePath" mapstructure:"storePath"`
}

// ChainStore defines the interface for hash chain state persistence.
//
// Implement this interface to store the chain state in different
// backends (file, database, etc.).
type ChainStore interface {
	// Save persists the current chain state.
	Save(sequence uint64, lastHash string) error
	// Load retrieves the persisted chain state.
	// Returns zero values if no state exists.
	Load() (sequence uint64, lastHash string, err error)
}

// ChainEntry represents an individual entry in the hash chain.
type ChainEntry struct {
	// Sequence is the entry sequence number.
	Sequence uint64 `json:"sequence"`
	// Timestamp is when the entry was created.
	Timestamp time.Time `json:"timestamp"`
	// Hash is the SHA256 hash of this entry.
	Hash string `json:"hash"`
	// PreviousHash is the hash of the previous entry.
	PreviousHash string `json:"previous_hash"`
}

// HashChain provides tamper-evident logging through cryptographic chaining.
//
// Each log entry receives a SHA256 hash that includes:
//   - Hash of the previous entry
//   - Sequence number
//   - Timestamp
//   - Entry data
//
// This creates an immutable chain where any modification to an entry
// invalidates all subsequent hashes, enabling tampering detection.
//
// Use case: audit logs for regulatory compliance (LGPD, SOX, PCI-DSS)
// where record integrity must be provable.
//
// Example:
//
//	store := axio.NewFileStore("/var/lib/axio/audit-chain.json")
//	chain, err := axio.NewHashChain(store)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Add entry
//	hash, prevHash, err := chain.Add([]byte("log data"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Verify integrity
//	err = chain.Verify(entries, func(i int) []byte {
//	    return getData(i)
//	})
type HashChain struct {
	sequence uint64
	lastHash string
	store    ChainStore
	mutex    sync.Mutex
}

// NewHashChain creates a new [HashChain], loading persisted state from the store if it exists.
//
// If store is nil, the chain operates in memory only and does not persist across restarts.
//
// Returns [ErrLoadChainState] if it fails to load existing state.
//
// Example:
//
//	store := axio.NewFileStore("/var/lib/axio/chain.json")
//	chain, err := axio.NewHashChain(store)
//	if err != nil {
//	    return err
//	}
func NewHashChain(store ChainStore) (*HashChain, error) {
	chain := &HashChain{store: store}

	if store != nil {
		sequence, hash, err := store.Load()
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrLoadChainState, err)
		}
		chain.sequence = sequence
		chain.lastHash = hash
	}

	return chain, nil
}

// Add appends data to the chain and returns the calculated hash and previous hash.
//
// Returns an error if state persistence fails. This ensures integrity
// of the audit chain - if the state cannot be persisted, the operation
// fails to avoid inconsistencies between memory and storage.
func (c *HashChain) Add(data []byte) (hash, previousHash string, err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.sequence++
	previousHash = c.lastHash

	entry := ChainEntry{
		Sequence:     c.sequence,
		Timestamp:    time.Now().UTC(),
		PreviousHash: previousHash,
	}

	hash = c.computeHash(entry, data)
	c.lastHash = hash

	if c.store != nil {
		if err := c.store.Save(c.sequence, c.lastHash); err != nil {
			c.sequence--
			c.lastHash = previousHash
			return "", "", fmt.Errorf("persist chain state: %w", err)
		}
	}

	return hash, previousHash, nil
}

// computeHash generates a SHA256 hash of the entry metadata and data.
func (c *HashChain) computeHash(entry ChainEntry, data []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte(entry.PreviousHash))
	_, _ = fmt.Fprintf(h, "%d", entry.Sequence)
	_, _ = h.Write([]byte(entry.Timestamp.Format(time.RFC3339Nano)))
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// Verify validates that a sequence of entries forms a valid chain.
//
// Returns an error if any hash is invalid or the chain is broken.
func (c *HashChain) Verify(entries []ChainEntry, getData func(int) []byte) error {
	for index, entry := range entries {
		data := getData(index)
		computed := c.computeHash(entry, data)

		if computed != entry.Hash {
			return fmt.Errorf("%w: sequence %d: expected %s, got %s",
				ErrHashMismatch, entry.Sequence, entry.Hash, computed)
		}

		if index > 0 && entry.PreviousHash != entries[index-1].Hash {
			return fmt.Errorf("%w: sequence %d",
				ErrChainBroken, entry.Sequence)
		}
	}
	return nil
}

// Sequence returns the current sequence number.
func (c *HashChain) Sequence() uint64 {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.sequence
}

// LastHash returns the hash of the last entry.
func (c *HashChain) LastHash() string {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.lastHash
}

// FileStore persists the hash chain state in a local JSON file.
//
// Example:
//
//	store := axio.NewFileStore("/var/lib/axio/audit-chain.json")
//	hook, err := axio.NewAuditHook(store)
type FileStore struct {
	path  string
	mutex sync.Mutex
}

// fileStoreState represents the persisted state format.
type fileStoreState struct {
	Sequence uint64 `json:"sequence"`
	LastHash string `json:"last_hash"`
}

// NewFileStore creates a [FileStore] that persists state at the specified path.
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

// Save persists the chain state to the file.
func (s *FileStore) Save(sequence uint64, lastHash string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	state := fileStoreState{
		Sequence: sequence,
		LastHash: lastHash,
	}

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMarshalChainState, err)
	}

	if err := os.WriteFile(s.path, data, 0600); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrSaveChainState, s.path, err)
	}

	return nil
}

// Load retrieves the chain state from the file.
//
// Returns zero values if the file does not exist.
func (s *FileStore) Load() (uint64, string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, "", nil
		}
		return 0, "", fmt.Errorf("%w: %s: %w", ErrLoadChainState, s.path, err)
	}

	var state fileStoreState
	if err := json.Unmarshal(data, &state); err != nil {
		return 0, "", fmt.Errorf("%w: %w", ErrUnmarshalChainState, err)
	}

	return state.Sequence, state.LastHash, nil
}

// AuditHook adds hash chain information to log entries for tampering detection.
//
// The hook populates the Hash and PrevHash fields of [Entry], creating a
// cryptographic chain that allows log integrity verification.
//
// AuditHook implements [MetricsAware] to emit metrics for audit records.
//
// Example:
//
//	store := axio.NewFileStore("/var/lib/axio/audit.json")
//	hook, err := axio.NewAuditHook(store)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	logger, _ := axio.New(config, axio.WithHooks(hook))
type AuditHook struct {
	chain   *HashChain
	metrics Metrics
	mutex   sync.RWMutex
}

// NewAuditHook creates an [AuditHook] with the specified chain store.
//
// Pass nil for an in-memory chain that does not persist across restarts.
//
// Returns [ErrCreateAuditHook] if it fails to create the chain.
//
// Example:
//
//	store := axio.NewFileStore("/var/lib/axio/audit.json")
//	hook, err := axio.NewAuditHook(store)
//	if err != nil {
//	    return err
//	}
//	logger, _ := axio.New(config, axio.WithHooks(hook))
func NewAuditHook(store ChainStore) (*AuditHook, error) {
	chain, err := NewHashChain(store)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateAuditHook, err)
	}
	return &AuditHook{chain: chain}, nil
}

// MustAuditHook is like [NewAuditHook] but panics on error.
//
// Useful for initialization where failure must be fatal.
//
// Example:
//
//	store := axio.NewFileStore("/var/lib/axio/audit.json")
//	hook := axio.MustAuditHook(store)
//	logger, _ := axio.New(config, axio.WithHooks(hook))
func MustAuditHook(store ChainStore) *AuditHook {
	hook, err := NewAuditHook(store)
	if err != nil {
		panic(err)
	}
	return hook
}

// Name returns the hook identifier.
func (hook *AuditHook) Name() string {
	return "audit"
}

// SetMetrics implements [MetricsAware].
//
// When configured, the hook emits metrics for each created audit record.
func (hook *AuditHook) SetMetrics(metrics Metrics) {
	hook.mutex.Lock()
	defer hook.mutex.Unlock()
	hook.metrics = metrics
}

// Process adds hash chain information to the log entry.
//
// If metrics is configured via [SetMetrics], emits metrics for
// each successfully created audit record.
func (hook *AuditHook) Process(ctx context.Context, entry *Entry) error {
	fields := make(map[string]any, len(entry.Annotations))
	for _, annotation := range entry.Annotations {
		fields[annotation.Name()] = annotation.Data()
	}

	data, err := json.Marshal(map[string]any{
		"timestamp": entry.Timestamp,
		"level":     entry.Level,
		"message":   entry.Message,
		"logger":    entry.Logger,
		"caller":    entry.Caller,
		"trace_id":  entry.TraceID,
		"span_id":   entry.SpanID,
		"fields":    fields,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSerializeEntry, err)
	}

	hash, previousHash, err := hook.chain.Add(data)
	if err != nil {
		return fmt.Errorf("add entry to chain: %w", err)
	}
	entry.Hash = hash
	entry.PreviousHash = previousHash

	hook.mutex.RLock()
	metrics := hook.metrics
	hook.mutex.RUnlock()

	if metrics != nil {
		metrics.AuditRecords(ctx)
	}

	return nil
}
