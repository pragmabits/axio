//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package axio

import (
	"errors"
	"os"
	"syscall"
)

// lockExclusive takes an exclusive flock on file without waiting for it, and
// reports false when another open file holds it — in another process, or in
// this one through another descriptor.
func lockExclusive(file *os.File) (bool, error) {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}
