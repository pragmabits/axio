package axio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalPath(t *testing.T) {
	t.Run("file_moved_after_opening_keeps_the_path_it_was_opened_at", func(t *testing.T) {
		directory := tempDir(t)
		path := filepath.Join(directory, "app.log")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		defer file.Close()
		if err := os.Rename(path, filepath.Join(directory, "app-2026-11-08T12-00-00.000.log")); err != nil {
			t.Fatalf("rename %s: %v", path, err)
		}

		assertEqual(t, canonicalPath(file, path), path)
	})
}
