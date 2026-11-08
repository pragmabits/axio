//go:build unix

package axio

import (
	"errors"
	"os"
	"syscall"
)

// openLogFile opens the file at path as [os.OpenFile] does, and leaves its
// descriptor out of the runtime's poller, which a regular file never uses.
func openLogFile(path string, flag int, mode os.FileMode) (*os.File, error) {
	for {
		descriptor, err := syscall.Open(path, flag|syscall.O_CLOEXEC, uint32(mode.Perm()))
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return nil, &os.PathError{Op: "open", Path: path, Err: err}
		}
		return os.NewFile(uintptr(descriptor), path), nil
	}
}
