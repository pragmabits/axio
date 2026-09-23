package axio

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/pragmabits/axio/internal/logline"
)

// ChainStore defines the interface for hash chain state persistence.
//
// Implement this interface to store the chain state in different
// backends (file, database, etc.).
//
// Save and Load must be safe to call from multiple goroutines.
type ChainStore interface {
	// Save persists the current chain state atomically and durably.
	//
	// The (sequence, lastHash) pair must be written as a single unit so that
	// a concurrent reader, or a crash mid-write, can never observe a partial
	// update — either both fields reflect the new entry, or both still
	// reflect the previous one. A non-nil error must guarantee that the
	// stored state was not modified.
	Save(sequence uint64, lastHash string) error
	// Load retrieves the persisted chain state.
	// Returns zero values if no state exists.
	Load() (sequence uint64, lastHash string, err error)
}

// HashChain links audited log lines into a tamper-evident chain.
//
// Each line's hash is the SHA-256 of the previous line's hash followed by the
// line's own bytes, so changing, removing or reordering a line breaks the chain
// from that line on. The chain's state — how many lines it holds and the last
// hash — lives in a [ChainStore], which is what lets [HashChain.Verify] tell a
// log that ends early from a complete one.
//
// A Logger or Event built with [WithAudit] or [WithAuditChain] writes through a
// HashChain; the chain is rarely driven by hand.
//
// Use case: audit logs for regulatory compliance (LGPD, SOX, PCI-DSS)
// where record integrity must be provable.
//
// Example:
//
//	chain, err := axio.NewHashChain(axio.NewFileStore("/var/lib/axio/chain.json"))
//	if err != nil {
//	    return err
//	}
//	file, err := os.Open("/var/log/app.log")
//	if err != nil {
//	    return err
//	}
//	defer file.Close()
//	if err := chain.Verify(file, ""); err != nil {
//	    return fmt.Errorf("audit log does not verify: %w", err)
//	}
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

// Add appends data to the chain and returns its hash and the hash before it.
//
// The hash is the hex SHA-256 of the previous hash followed by data. The new
// state is persisted before Add returns; if persisting fails, the chain is
// left as it was and the error is returned.
func (c *HashChain) Add(data []byte) (hash, previousHash string, err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	previousHash, hash, err = c.add(data)
	return hash, previousHash, err
}

// Verify reads audited JSON lines from reader and checks the chain they form,
// from previousHash to the chain's [HashChain.LastHash].
//
// It checks every line as [VerifyLines] does — previousHash is empty for a log
// that starts the chain, or the hash before the first line otherwise — and
// then that the last line ends where the chain ends.
//
// Returns [ErrHashMismatch] for a line whose content changed, [ErrChainBroken]
// for a line that was removed, moved or inserted, or that carries no trailer,
// and [ErrChainIncomplete] when the log ends before the chain does: its end was
// removed, or the whole chain was rewritten. The error names the line.
//
// Only JSON outputs can be verified. A text output carries a shortened hash,
// for finding the same entry in the JSON output.
func (c *HashChain) Verify(reader io.Reader, previousHash string) error {
	lastHash, err := VerifyLines(reader, previousHash)
	if err != nil {
		return err
	}
	if chainHash := c.LastHash(); lastHash != chainHash {
		return fmt.Errorf("%w: log ends at %q, chain at %q", ErrChainIncomplete, logline.ShortHash(lastHash), logline.ShortHash(chainHash))
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

// add appends data to the chain; the caller holds the mutex. The state
// changes only after the store has persisted it.
func (c *HashChain) add(data []byte) (previousHash, hash string, err error) {
	previousHash = c.lastHash
	hash = hashLine(previousHash, data)
	if c.store != nil {
		if err := c.store.Save(c.sequence+1, hash); err != nil {
			return "", "", fmt.Errorf("persist chain state: %w", err)
		}
	}
	c.sequence++
	c.lastHash = hash
	return previousHash, hash, nil
}

// appendAndWrite adds data to the chain and calls write with the new hashes
// while the chain is still locked, so lines reach the outputs in chain order.
func (c *HashChain) appendAndWrite(data []byte, write func(previousHash, hash string) error) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	previousHash, hash, err := c.add(data)
	if err != nil {
		return err
	}
	return write(previousHash, hash)
}

// VerifyLines reads audited JSON lines from reader and checks each against the
// one before it, starting from previousHash, and returns the hash of the last
// line: the previousHash of whatever comes next.
//
// Each line must end with the trailer an audited Logger writes, and its hash
// must match the rest of the line. Blank lines are skipped. VerifyLines does not
// check where the chain ends, so it verifies one piece of a log — a rotated
// file — on its own; [HashChain.Verify] checks a whole log against its chain.
//
// Returns [ErrHashMismatch] for a line whose content changed and
// [ErrChainBroken] for a line that was removed, moved or inserted, or that
// carries no trailer. The error names the line.
//
// Example:
//
//	previousHash := ""
//	for _, path := range []string{"app.log.2", "app.log.1", "app.log"} {
//	    file, err := os.Open(path)
//	    if err != nil {
//	        return err
//	    }
//	    previousHash, err = axio.VerifyLines(file, previousHash)
//	    file.Close()
//	    if err != nil {
//	        return fmt.Errorf("%s: %w", path, err)
//	    }
//	}
func VerifyLines(reader io.Reader, previousHash string) (lastHash string, err error) {
	lastHash = previousHash
	lines := bufio.NewReader(reader)
	for number := 1; ; number++ {
		line, readErr := lines.ReadBytes('\n')
		if trimmed := bytes.TrimRight(line, "\r\n"); len(trimmed) > 0 {
			if lastHash, err = verifyLine(trimmed, lastHash, number); err != nil {
				return "", err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return lastHash, nil
		}
		if readErr != nil {
			return "", fmt.Errorf("read line %d: %w", number, readErr)
		}
	}
}

// FileStore persists the hash chain state in a local JSON file.
//
// Example:
//
//	chain, err := axio.NewHashChain(axio.NewFileStore("/var/lib/axio/audit-chain.json"))
type FileStore struct {
	path  string
	mutex sync.Mutex
}

// NewFileStore creates a [FileStore] that persists state at the specified path.
//
// Example:
//
//	chain, err := axio.NewHashChain(axio.NewFileStore("/var/lib/axio/audit-chain.json"))
//	if err != nil {
//	    return err
//	}
//	logger, err := axio.New(config, axio.WithAuditChain(chain))
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

// Save persists the chain state to the file atomically.
//
// The implementation writes to a sibling staging file, fsyncs it, then renames
// over the destination so that a concurrent reader (or a crash mid-write) can
// never observe a partial update — either the previous state remains intact
// or the new state is fully visible.
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

	stagingPath := s.path + ".staging"
	stagingFile, err := os.OpenFile(stagingPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrSaveChainState, s.path, err)
	}

	if _, err := stagingFile.Write(data); err != nil {
		_ = stagingFile.Close()
		_ = os.Remove(stagingPath)
		return fmt.Errorf("%w: %s: %w", ErrSaveChainState, s.path, err)
	}

	if err := stagingFile.Sync(); err != nil {
		_ = stagingFile.Close()
		_ = os.Remove(stagingPath)
		return fmt.Errorf("%w: %s: %w", ErrSaveChainState, s.path, err)
	}

	if err := stagingFile.Close(); err != nil {
		_ = os.Remove(stagingPath)
		return fmt.Errorf("%w: %s: %w", ErrSaveChainState, s.path, err)
	}

	if err := os.Rename(stagingPath, s.path); err != nil {
		_ = os.Remove(stagingPath)
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

// fileStoreState represents the persisted state format.
type fileStoreState struct {
	Sequence uint64 `json:"sequence"`
	LastHash string `json:"last_hash"`
}

// auditCore is the zapcore.Core of an audited Logger or Event.
//
// It encodes each entry once as JSON, adds those bytes to the chain and writes
// the audited line to every JSON output, all while the chain is locked: the
// hash covers exactly what is written, and lines land in chain order. Text
// outputs get the same entry with the hash shortened, for reference.
type auditCore struct {
	zapcore.LevelEnabler
	chain       *HashChain
	metrics     Metrics
	canonical   zapcore.Encoder
	jsonOutputs []Output
	textSinks   []textSink
}

func (a *auditCore) With(fields []zapcore.Field) zapcore.Core {
	clone := *a
	clone.canonical = a.canonical.Clone()
	addFields(clone.canonical, fields)
	clone.textSinks = make([]textSink, len(a.textSinks))
	for index, sink := range a.textSinks {
		encoder := sink.encoder.Clone()
		addFields(encoder, fields)
		clone.textSinks[index] = textSink{encoder: encoder, output: sink.output}
	}
	return &clone
}

func (a *auditCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if a.Enabled(entry.Level) {
		return checked.AddCore(entry, a)
	}
	return checked
}

func (a *auditCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	encoded, err := a.canonical.EncodeEntry(entry, fields)
	if err != nil {
		return fmt.Errorf("encode audited entry: %w", err)
	}
	defer encoded.Free()

	body, ok := logline.Body(encoded.Bytes())
	if !ok {
		return fmt.Errorf("encode audited entry: not a JSON object: %q", encoded.String())
	}
	err = a.chain.appendAndWrite(body, func(previousHash, hash string) error {
		return a.writeLine(entry, fields, logline.AppendTrailer(body, previousHash, hash), hash)
	})
	if err != nil {
		return err
	}
	a.metrics.AuditRecords(context.Background())
	return nil
}

func (a *auditCore) Sync() error {
	var errs []error
	for _, output := range a.jsonOutputs {
		if err := output.Sync(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, sink := range a.textSinks {
		if err := sink.output.Sync(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// writeLine writes the audited line to every JSON output, and the entry with
// its short hash to every text output.
func (a *auditCore) writeLine(entry zapcore.Entry, fields []zapcore.Field, line []byte, hash string) error {
	var errs []error
	for _, output := range a.jsonOutputs {
		if _, err := output.Write(line); err != nil {
			errs = append(errs, err)
		}
	}
	withHash := append(fields[:len(fields):len(fields)], zap.String(logline.HashKey, logline.ShortHash(hash)))
	for _, sink := range a.textSinks {
		if err := sink.write(entry, withHash); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// textSink is a text output with the encoder that writes to it.
type textSink struct {
	encoder zapcore.Encoder
	output  Output
}

// write encodes the entry as text and writes it to the output.
func (t textSink) write(entry zapcore.Entry, fields []zapcore.Field) error {
	encoded, err := t.encoder.EncodeEntry(entry, fields)
	if err != nil {
		return fmt.Errorf("encode text entry: %w", err)
	}
	defer encoded.Free()
	_, err = t.output.Write(encoded.Bytes())
	return err
}

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

// hashLine returns the hex SHA-256 of previousHash followed by body: the hash
// an audited line carries.
func hashLine(previousHash string, body []byte) string {
	hasher := sha256.New()
	// hash.Hash.Write never returns an error.
	_, _ = hasher.Write([]byte(previousHash))
	_, _ = hasher.Write(body)
	return hex.EncodeToString(hasher.Sum(nil))
}

// verifyLine checks one audited line against the hash it must continue from
// and returns the line's own hash.
func verifyLine(line []byte, expected string, number int) (string, error) {
	body, previousHash, hash, ok := logline.SplitTrailer(line)
	if !ok {
		return "", fmt.Errorf("%w: line %d carries no audit trailer", ErrChainBroken, number)
	}
	if hashLine(previousHash, body) != hash {
		return "", fmt.Errorf("%w: line %d", ErrHashMismatch, number)
	}
	if previousHash != expected {
		return "", fmt.Errorf("%w: line %d does not continue the line before it", ErrChainBroken, number)
	}
	return hash, nil
}

// addFields adds fields to encoder the way zapcore.Core.With does.
func addFields(encoder zapcore.ObjectEncoder, fields []zapcore.Field) {
	for _, field := range fields {
		field.AddTo(encoder)
	}
}

// buildAuditChain returns the chain an audited Logger or Event writes
// through, or nil when auditing is off.
func buildAuditChain(config Config) (*HashChain, error) {
	switch {
	case !config.Audit.Enabled:
		return nil, nil
	case config.auditChain != nil:
		return config.auditChain, nil
	default:
		return sharedChain(config.Audit.StorePath)
	}
}

// sharedChain returns this process's chain for storePath, loading it from the
// store the first time the path is used.
func sharedChain(storePath string) (*HashChain, error) {
	absolute, err := filepath.Abs(storePath)
	if err != nil {
		return nil, fmt.Errorf("resolve store path %s: %w", storePath, err)
	}

	chainsByPath.mutex.Lock()
	defer chainsByPath.mutex.Unlock()
	if chain, ok := chainsByPath.chains[absolute]; ok {
		return chain, nil
	}
	chain, err := NewHashChain(NewFileStore(absolute))
	if err != nil {
		return nil, err
	}
	chainsByPath.chains[absolute] = chain
	return chain, nil
}

// chainsByPath holds one chain per store path in this process, so every Logger
// and Event audited with the same [WithAudit] path extends a single chain
// instead of each loading the store and forking it.
var chainsByPath = struct {
	mutex  sync.Mutex
	chains map[string]*HashChain
}{chains: make(map[string]*HashChain)}
