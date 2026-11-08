//go:build linux

package axio

import (
	"os"
	"strconv"
	"syscall"
)

// canonicalPath returns absolute, the path file was opened at, with every
// symlink in it resolved. The path the kernel keeps for the descriptor of file
// confirms absolute when they are equal; when they differ — absolute goes
// through a symlink, or the file was renamed since it was opened — the path is
// resolved by name, through [filepath.EvalSymlinks], never taken from the
// descriptor, which may name a file the path no longer reaches.
func canonicalPath(file *os.File, absolute string) string {
	var target [4096]byte
	length, err := syscall.Readlink("/proc/self/fd/"+strconv.FormatUint(uint64(file.Fd()), 10), target[:])
	if err == nil && string(target[:min(max(length, 0), len(target))]) == absolute {
		return absolute
	}
	return resolvePath(absolute)
}
