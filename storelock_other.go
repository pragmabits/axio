//go:build !(darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd)

package axio

import "os"

// lockExclusive does nothing: this system has no flock, so a [FileStore] shared
// by two processes is not detected here.
func lockExclusive(*os.File) error {
	return nil
}
