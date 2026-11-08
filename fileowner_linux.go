//go:build linux

package axio

import (
	"os"
	"syscall"
)

// copyOwner gives the file at name the owner and group of previous, and does
// nothing when they are already its own.
func copyOwner(name string, previous os.FileInfo) error {
	owner, ok := previous.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	current, err := os.Stat(name)
	if err != nil {
		return err
	}
	if own, ok := current.Sys().(*syscall.Stat_t); ok && own.Uid == owner.Uid && own.Gid == owner.Gid {
		return nil
	}
	return os.Chown(name, int(owner.Uid), int(owner.Gid))
}
