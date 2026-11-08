//go:build !unix

package axio

import "os"

// openLogFile opens the file at path with [os.OpenFile].
func openLogFile(path string, flag int, mode os.FileMode) (*os.File, error) {
	return os.OpenFile(path, flag, mode)
}
