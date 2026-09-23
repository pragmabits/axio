//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package axio

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// lockExclusive takes an exclusive flock on file without waiting for it.
// Returns [ErrChainStoreLocked] when another open file holds it.
func lockExclusive(file *os.File) error {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return fmt.Errorf("%w: %w", ErrChainStoreLocked, err)
	}
	return err
}
