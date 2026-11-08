//go:build !linux

package axio

import "os"

// copyOwner does nothing on this system: a rotated log's owner is copied on
// Linux only.
func copyOwner(string, os.FileInfo) error {
	return nil
}
