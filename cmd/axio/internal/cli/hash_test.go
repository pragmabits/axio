package cli_test

import (
	"strings"
	"testing"
)

func TestNewRootCommand_PreviousHashFlag(t *testing.T) {
	t.Run("invalid_value_is_rejected", func(t *testing.T) {
		logPath, storePath := auditedLog(t)
		_, err := execute(t, "", "verify", "--store", storePath, "--previous-hash", logPath)
		if err == nil || !strings.Contains(err.Error(), "64 hexadecimal characters") {
			t.Errorf("expected --previous-hash to be rejected, got %v", err)
		}
	})
}
