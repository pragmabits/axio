package axio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/pragmabits/axio/internal/logline"
)

func TestHashChain_Add(t *testing.T) {
	t.Run("first_entry_has_empty_previous_hash", func(t *testing.T) {
		chain, _ := NewHashChain(nil)

		hash, previousHash, err := chain.Add([]byte("first entry"))
		assertNoError(t, err)

		assertEqual(t, previousHash, "")
		if hash == "" {
			t.Error("hash should not be empty")
		}
	})

	t.Run("subsequent_entries_link_to_previous", func(t *testing.T) {
		chain, _ := NewHashChain(nil)

		firstHash, _, _ := chain.Add([]byte("first"))
		secondHash, secondPrevious, _ := chain.Add([]byte("second"))
		thirdHash, thirdPrevious, _ := chain.Add([]byte("third"))

		assertEqual(t, secondPrevious, firstHash)
		assertEqual(t, thirdPrevious, secondHash)
		if firstHash == secondHash || secondHash == thirdHash {
			t.Error("hashes should be unique for different data")
		}
	})

	t.Run("sequence_increments", func(t *testing.T) {
		chain, _ := NewHashChain(nil)

		assertEqual(t, chain.Sequence(), uint64(0))

		_, _, err := chain.Add([]byte("1"))
		assertNoError(t, err)
		assertEqual(t, chain.Sequence(), uint64(1))

		_, _, err = chain.Add([]byte("2"))
		assertNoError(t, err)
		assertEqual(t, chain.Sequence(), uint64(2))
	})

	t.Run("hash_is_sha256_of_previous_hash_and_data", func(t *testing.T) {
		chain, _ := NewHashChain(nil)

		firstHash, _, _ := chain.Add([]byte("first"))
		secondHash, _, _ := chain.Add([]byte("second"))

		assertEqual(t, firstHash, sha256Hex("first"))
		assertEqual(t, secondHash, sha256Hex(firstHash+"second"))
	})
}

func TestHashChain_Verify(t *testing.T) {
	bodies := []string{`{"message":"first"`, `{"message":"second"`, `{"message":"third"`, `{"message":"fourth"`}

	tests := []struct {
		name         string
		previousHash func(lines []string) string
		change       func(lines []string) []string
		want         error
	}{
		{
			name:   "intact_log_verifies",
			change: func(lines []string) []string { return lines },
		},
		{
			name:   "blank_lines_are_ignored",
			change: func(lines []string) []string { return []string{lines[0], "", lines[1], "\r", lines[2], lines[3]} },
		},
		{
			name: "changed_content",
			want: ErrHashMismatch,
			change: func(lines []string) []string {
				lines[1] = strings.Replace(lines[1], "second", "SECOND", 1)
				return lines
			},
		},
		{
			name: "changed_content_with_its_own_hash_recomputed",
			want: ErrChainBroken,
			change: func(lines []string) []string {
				body, previousHash, _, _ := logline.SplitTrailer([]byte(lines[1]))
				forged := strings.Replace(string(body), "second", "SECOND", 1)
				lines[1] = strings.TrimSuffix(string(logline.AppendTrailer([]byte(forged), previousHash, sha256Hex(previousHash+forged))), "\n")
				return lines
			},
		},
		{
			name:   "removed_middle_line",
			want:   ErrChainBroken,
			change: func(lines []string) []string { return append(lines[:1], lines[2:]...) },
		},
		{
			name:   "reordered_lines",
			want:   ErrChainBroken,
			change: func(lines []string) []string { return []string{lines[0], lines[2], lines[1], lines[3]} },
		},
		{
			name: "line_without_trailer",
			want: ErrChainBroken,
			change: func(lines []string) []string {
				return []string{lines[0], `{"message":"injected"}`, lines[1], lines[2], lines[3]}
			},
		},
		{
			name:   "removed_last_line",
			want:   ErrChainIncomplete,
			change: func(lines []string) []string { return lines[:3] },
		},
		{
			name:   "wrong_starting_hash",
			want:   ErrChainBroken,
			change: func(lines []string) []string { return lines[1:] },
		},
		{
			name:         "log_starting_mid_chain_with_its_starting_hash",
			previousHash: func(lines []string) string { return trailerHash(lines[1]) },
			change:       func(lines []string) []string { return lines[2:] },
		},
		{
			name: "whole_chain_rewritten",
			want: ErrChainIncomplete,
			change: func(lines []string) []string {
				forged := append([]string(nil), bodies...)
				forged[1] = `{"message":"SECOND"`
				_, rewritten := auditedLines(t, forged)
				return rewritten
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chain, lines := auditedLines(t, bodies)
			previousHash := ""
			if test.previousHash != nil {
				previousHash = test.previousHash(lines)
			}

			err := chain.Verify(strings.NewReader(strings.Join(test.change(lines), "\n")+"\n"), previousHash)

			if test.want == nil {
				assertNoError(t, err)
				return
			}
			if !errors.Is(err, test.want) {
				t.Errorf("expected %v, got %v", test.want, err)
			}
		})
	}
}

func TestVerifyLines(t *testing.T) {
	bodies := []string{`{"message":"first"`, `{"message":"second"`, `{"message":"third"`}

	t.Run("returns_the_hash_of_the_last_line", func(t *testing.T) {
		_, lines := auditedLines(t, bodies)
		lastHash, err := VerifyLines(strings.NewReader(strings.Join(lines, "\n")+"\n"), "")
		assertNoError(t, err)
		assertEqual(t, lastHash, trailerHash(lines[2]))
	})

	t.Run("does_not_check_where_the_chain_ends", func(t *testing.T) {
		_, lines := auditedLines(t, bodies)
		lastHash, err := VerifyLines(strings.NewReader(strings.Join(lines[:2], "\n")), "")
		assertNoError(t, err)
		assertEqual(t, lastHash, trailerHash(lines[1]))
	})

	t.Run("continues_from_previous_hash", func(t *testing.T) {
		_, lines := auditedLines(t, bodies)
		lastHash, err := VerifyLines(strings.NewReader(lines[2]), trailerHash(lines[1]))
		assertNoError(t, err)
		assertEqual(t, lastHash, trailerHash(lines[2]))
	})

	t.Run("empty_log_ends_where_it_started", func(t *testing.T) {
		lastHash, err := VerifyLines(strings.NewReader("\n\n"), "abc")
		assertNoError(t, err)
		assertEqual(t, lastHash, "abc")
	})

	t.Run("changed_line_is_reported", func(t *testing.T) {
		_, lines := auditedLines(t, bodies)
		lines[1] = strings.Replace(lines[1], "second", "SECOND", 1)
		_, err := VerifyLines(strings.NewReader(strings.Join(lines, "\n")), "")
		if !errors.Is(err, ErrHashMismatch) || !strings.Contains(err.Error(), "line 2") {
			t.Errorf("expected a hash mismatch on line 2, got %v", err)
		}
	})

	t.Run("removed_line_is_reported", func(t *testing.T) {
		_, lines := auditedLines(t, bodies)
		_, err := VerifyLines(strings.NewReader(lines[0]+"\n"+lines[2]), "")
		if !errors.Is(err, ErrChainBroken) {
			t.Errorf("expected ErrChainBroken, got %v", err)
		}
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

		assertNoError(t, store.Save(1, "hash"))

		info, err := os.Stat(path)
		assertNoError(t, err)

		// 0600 = owner read/write only
		if info.Mode().Perm() != 0600 {
			t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
		}
	})

	t.Run("no_staging_leftover_on_success", func(t *testing.T) {
		path := tempFile(t, "staging-leftover.json")
		store := NewFileStore(path)

		err := store.Save(7, "successhash")
		assertNoError(t, err)

		if _, err := os.Stat(path + ".staging"); !os.IsNotExist(err) {
			t.Errorf("expected staging file to be removed, got err=%v", err)
		}
	})
}

func TestHashChain_WithStore(t *testing.T) {
	t.Run("loads_state_from_store", func(t *testing.T) {
		path := tempFile(t, "chain.json")
		store := NewFileStore(path)

		// Save initial state
		assertNoError(t, store.Save(10, "existinghash"))

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
		_, _, err := chain.Add([]byte("entry"))
		assertNoError(t, err)

		// Verify persisted file
		data, _ := os.ReadFile(path)
		var state struct {
			Sequence uint64 `json:"sequence"`
			LastHash string `json:"last_hash"`
		}
		assertNoError(t, json.Unmarshal(data, &state))

		assertEqual(t, state.Sequence, uint64(1))
		if state.LastHash == "" {
			t.Error("last_hash should have been persisted")
		}
	})
}

func TestLogger_Audit(t *testing.T) {
	t.Run("json_lines_verify_against_the_chain", func(t *testing.T) {
		logPath, storePath := tempFile(t, "audited.log"), tempFile(t, "audited-chain.json")
		logAudited(t, logPath, WithAudit(storePath))

		lines := readLines(t, logPath)
		assertEqual(t, len(lines), 3)
		if !strings.Contains(lines[0], `"previous_hash":"","hash":"`) {
			t.Errorf("first line should open the chain with an empty previous_hash: %s", lines[0])
		}
		assertNoError(t, verifyFile(t, logPath, storePath))
	})

	t.Run("every_written_field_is_covered", func(t *testing.T) {
		replacements := []struct {
			name string
			old  string
			new  string
		}{
			{name: "message", old: `"message":"order created"`, new: `"message":"order deleted"`},
			{name: "error", old: `"error":"card declined"`, new: `"error":"none"`},
			{name: "service_metadata", old: `"name":"checkout"`, new: `"name":"billing"`},
			{name: "stacktrace", old: `"stacktrace":"`, new: `"stacktrace":"forged\n`},
			{name: "annotation", old: `"order_id":"ord_8812"`, new: `"order_id":"ord_0001"`},
		}
		for _, replacement := range replacements {
			t.Run(replacement.name, func(t *testing.T) {
				logPath, storePath := tempFile(t, "audited.log"), tempFile(t, "audited-chain.json")
				logAudited(t, logPath, WithAudit(storePath))

				content := readFile(t, logPath)
				if !strings.Contains(content, replacement.old) {
					t.Fatalf("log does not contain %s:\n%s", replacement.old, content)
				}
				writeFile(t, logPath, strings.Replace(content, replacement.old, replacement.new, 1))

				if err := verifyFile(t, logPath, storePath); !errors.Is(err, ErrHashMismatch) {
					t.Errorf("expected ErrHashMismatch, got %v", err)
				}
			})
		}
	})

	t.Run("changes_made_by_custom_hooks_are_covered", func(t *testing.T) {
		logPath, storePath := tempFile(t, "audited.log"), tempFile(t, "audited-chain.json")
		tenant := &testHook{name: "tenant", process: func(ctx context.Context, entry *Entry) error {
			entry.Annotations = append(entry.Annotations, Annotate("tenant", "acme"))
			return nil
		}}
		logAudited(t, logPath, WithAudit(storePath), WithHooks(tenant))

		content := readFile(t, logPath)
		if !strings.Contains(content, `"tenant":"acme"`) {
			t.Fatalf("hook annotation missing from log:\n%s", content)
		}
		assertNoError(t, verifyFile(t, logPath, storePath))

		writeFile(t, logPath, strings.Replace(content, `"tenant":"acme"`, `"tenant":"evil"`, 1))
		if err := verifyFile(t, logPath, storePath); !errors.Is(err, ErrHashMismatch) {
			t.Errorf("expected ErrHashMismatch, got %v", err)
		}
	})

	t.Run("concurrent_writes_keep_chain_order", func(t *testing.T) {
		logPath := tempFile(t, "concurrent.log")
		chain, err := NewHashChain(nil)
		assertNoError(t, err)
		logger, err := New(
			Config{ServiceName: "checkout", Environment: Production, Level: LevelInfo},
			WithOutputs(MustFile(logPath, FormatJSON)),
			WithAuditChain(chain),
		)
		assertNoError(t, err)

		var group sync.WaitGroup
		for worker := range 8 {
			group.Add(1)
			go func() {
				defer group.Done()
				for index := range 200 {
					logger.Info(context.Background(), "worker %d line %d", worker, index)
				}
			}()
		}
		group.Wait()
		assertNoError(t, logger.Close())

		assertEqual(t, len(readLines(t, logPath)), 1600)
		file, err := os.Open(logPath)
		assertNoError(t, err)
		defer file.Close()
		assertNoError(t, chain.Verify(file, ""))
	})

	t.Run("text_output_shows_short_hash_only", func(t *testing.T) {
		logPath, storePath := tempFile(t, "audited.log"), tempFile(t, "audited-chain.json")
		text := newBufferOutput(FormatText)
		logger, err := New(
			Config{ServiceName: "checkout", Environment: Production, Level: LevelInfo},
			WithOutputs(text, MustFile(logPath, FormatJSON)),
			WithAudit(storePath),
		)
		assertNoError(t, err)
		logger.Info(context.Background(), "order created")
		assertNoError(t, logger.Close())

		hash := trailerHash(readLines(t, logPath)[0])
		if len(hash) < logline.ShortHashLength {
			t.Fatalf("JSON line carries no audit trailer: %s", readLines(t, logPath)[0])
		}
		line := strings.TrimSuffix(text.String(), "\n")
		if !strings.HasSuffix(line, `"hash": "`+hash[:logline.ShortHashLength]+`"}`) {
			t.Errorf("text line should end with the short hash %s: %s", hash[:logline.ShortHashLength], line)
		}
		for _, absent := range []string{hash, "previous_hash"} {
			if strings.Contains(line, absent) {
				t.Errorf("text line should not contain %q: %s", absent, line)
			}
		}
	})
}

func TestAudit_LoggerAndEventShareChainOnSamePath(t *testing.T) {
	logPath, storePath := tempFile(t, "shared.log"), tempFile(t, "shared-chain.json")
	config := Config{ServiceName: "checkout", Environment: Production, Level: LevelInfo}

	logger, err := New(config, WithOutputs(MustFile(logPath, FormatJSON)), WithAudit(storePath))
	assertNoError(t, err)
	event, err := NewEvent("http_request", config, WithOutputs(MustFile(logPath, FormatJSON)), WithAudit(storePath))
	assertNoError(t, err)

	logger.Info(context.Background(), "before the event")
	event.Add("route", "/api/v1/orders")
	event.Emit(context.Background())
	logger.Info(context.Background(), "after the event")
	assertNoError(t, event.Close())
	assertNoError(t, logger.Close())

	assertEqual(t, len(readLines(t, logPath)), 3)
	assertNoError(t, verifyFile(t, logPath, storePath))
}

func TestWithAuditChain(t *testing.T) {
	t.Run("nil_chain_is_rejected", func(t *testing.T) {
		config := minimalConfig()
		err := WithAuditChain(nil)(&config)
		if !errors.Is(err, ErrNilAuditChain) {
			t.Errorf("expected ErrNilAuditChain, got %v", err)
		}
	})

	t.Run("enables_audit_without_a_store_path", func(t *testing.T) {
		chain, _ := NewHashChain(nil)
		config := minimalConfig()
		assertNoError(t, WithAuditChain(chain)(&config))
		assertEqual(t, config.Audit.Enabled, true)
		assertNoError(t, config.Validate())
	})
}

// auditedLines adds each body to a fresh in-memory chain and returns the chain
// with the audited lines it produced, without line endings.
func auditedLines(t *testing.T, bodies []string) (*HashChain, []string) {
	t.Helper()
	chain, err := NewHashChain(nil)
	assertNoError(t, err)
	lines := make([]string, 0, len(bodies))
	for _, body := range bodies {
		hash, previousHash, err := chain.Add([]byte(body))
		assertNoError(t, err)
		lines = append(lines, strings.TrimSuffix(string(logline.AppendTrailer([]byte(body), previousHash, hash)), "\n"))
	}
	return chain, lines
}

// logAudited writes three entries — an info, an error with a stacktrace and a
// forked info — as JSON to path, with the given audit options.
func logAudited(t *testing.T, path string, options ...Option) {
	t.Helper()
	all := append([]Option{WithOutputs(MustFile(path, FormatJSON))}, options...)
	logger, err := New(Config{ServiceName: "checkout", ServiceVersion: "1.4.2", Environment: Production, Level: LevelInfo}, all...)
	assertNoError(t, err)
	orders := logger.Named("orders").With(Annotate("order_id", "ord_8812"))
	orders.Info(context.Background(), "order created")
	orders.Error(context.Background(), errors.New("card declined"), "payment failed")
	orders.Info(context.Background(), "order cancelled")
	assertNoError(t, logger.Close())
}

// verifyFile checks the audited log at logPath against the chain stored at storePath.
func verifyFile(t *testing.T, logPath, storePath string) error {
	t.Helper()
	chain, err := NewHashChain(NewFileStore(storePath))
	assertNoError(t, err)
	file, err := os.Open(logPath)
	assertNoError(t, err)
	defer file.Close()
	return chain.Verify(file, "")
}

// trailerHash returns the hash in the audit trailer of line.
func trailerHash(line string) string {
	_, _, hash, _ := logline.SplitTrailer([]byte(line))
	return hash
}

// sha256Hex returns the hex SHA-256 of text.
func sha256Hex(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
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
