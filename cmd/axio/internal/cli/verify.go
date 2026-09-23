package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/pragmabits/axio"
	"github.com/pragmabits/axio/internal/logline"
)

// newVerifyCommand returns axio verify.
func newVerifyCommand() *cobra.Command {
	var storePath string
	var previousHash hashValue

	verifyCommand := &cobra.Command{
		Use:   "verify --store chain.json [file...]",
		Short: "Check an audited JSON log against its hash chain",
		Long: `verify checks that an audited JSON log is exactly what was written: every
line's hash matches the line, every line continues the one before it, and the
log ends where the chain stored at --store ends.

It reads the files in order, oldest first, as one log, or standard input when
there is none; "-" also names standard input. Pass rotated files in rotation
order. When the oldest file you have is not the first of the chain, give the
hash of the line before it with --previous-hash.

Verify a log that is no longer being written: while a logger is still
writing, the chain may end past the last line read.

It exits 0 when the log verifies and 1 otherwise, naming the file and line
that broke: a changed line, a removed, moved or inserted line, or a log that
ends before the chain does.`,
		Example: `  axio verify --store /var/lib/axio/chain.json /var/log/app.log
  axio verify --store chain.json app.log.2 app.log.1 app.log
  axio verify --store chain.json --previous-hash "$(tail -1 app.log.1 | jq -r .hash)" app.log
  kubectl logs deploy/payments | axio verify --store chain.json`,
		GroupID: logsGroup,
		Args:    cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, files []string) error {
			return verifyFiles(command, storePath, string(previousHash), files)
		},
	}

	flags := verifyCommand.Flags()
	flags.StringVar(&storePath, "store", "", "chain state file written by the logger (the WithAudit path)")
	flags.Var(&previousHash, "previous-hash", "hash of the line before the first one, when the log starts mid-chain")
	// Both calls fail only for a flag that does not exist, and both flags are
	// defined just above.
	_ = verifyCommand.MarkFlagRequired("store")
	_ = verifyCommand.MarkFlagFilename("store", "json")

	return verifyCommand
}

// verifyFiles checks files, oldest first, as one log against the chain stored
// at storePath, and reports where the chain ends when they verify.
func verifyFiles(command *cobra.Command, storePath, previousHash string, files []string) error {
	chain, err := axio.NewHashChain(axio.NewFileStore(storePath))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		files = []string{standardInput}
	}

	lastHash := previousHash
	for _, name := range files {
		if lastHash, err = verifyFile(command, name, lastHash); err != nil {
			return err
		}
	}
	if chainHash := chain.LastHash(); lastHash != chainHash {
		return fmt.Errorf("%w: log ends at %q, chain at %q", axio.ErrChainIncomplete, logline.ShortHash(lastHash), logline.ShortHash(chainHash))
	}

	_, err = fmt.Fprintf(command.OutOrStdout(), "verified: the log reaches the chain's last hash %s\n", logline.ShortHash(lastHash))
	return err
}

// verifyFile checks one file, or standard input when name is "-", continuing
// from previousHash, and returns the hash of its last line.
func verifyFile(command *cobra.Command, name, previousHash string) (string, error) {
	if name == standardInput {
		return verifyReader(command.InOrStdin(), "standard input", previousHash)
	}
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return verifyReader(file, name, previousHash)
}

// verifyReader checks the lines of reader, naming it in the error.
func verifyReader(reader io.Reader, name, previousHash string) (string, error) {
	lastHash, err := axio.VerifyLines(reader, previousHash)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return lastHash, nil
}
