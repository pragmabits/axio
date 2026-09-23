package cli

import (
	"errors"
	"regexp"
)

// hashValue is the value of --previous-hash: empty, or a hash as audited lines
// carry it. It implements pflag.Value, so a mistyped hash — or a file name
// taken as the flag's value — is rejected while the flags are parsed.
type hashValue string

// errInvalidHash reports a --previous-hash value that is not a hash.
var errInvalidHash = errors.New("must be empty or 64 hexadecimal characters, as in a line's hash")

// String returns the hash.
func (h *hashValue) String() string {
	return string(*h)
}

// Set accepts an empty value or a lowercase hex SHA-256.
func (h *hashValue) Set(value string) error {
	if value != "" && !hashPattern.MatchString(value) {
		return errInvalidHash
	}
	*h = hashValue(value)
	return nil
}

// Type names the value in the help: --previous-hash hash.
func (h *hashValue) Type() string {
	return "hash"
}

// hashPattern matches the hex SHA-256 an audited line carries.
var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
