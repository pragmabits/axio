//go:build !linux

package axio

import "os"

// canonicalPath returns absolute, the path file was opened at, with every
// symlink in it resolved by [filepath.EvalSymlinks].
func canonicalPath(_ *os.File, absolute string) string {
	return resolvePath(absolute)
}
