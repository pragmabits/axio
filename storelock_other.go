//go:build !(darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd)

package axio

import "os"

// lockExclusive does nothing and reports the lock as taken: this system has no
// flock, so a [FileStore] or a rotating file shared by two processes is not
// detected here.
func lockExclusive(*os.File) (bool, error) {
	return true, nil
}
