//go:build !windows

package cli

import (
	"os"
	"syscall"
)

// openNoFollow opens path read-only and refuses a symlink at the final component.
// O_NONBLOCK: a FIFO would otherwise block here before the regular-file check.
func openNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0) // #nosec G304 -- validated user-chosen export file
}
