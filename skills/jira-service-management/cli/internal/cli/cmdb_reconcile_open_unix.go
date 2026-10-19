//go:build !windows

package cli

import (
	"os"
	"syscall"
)

// inventoryOpenFlags opens an export read-only. O_NONBLOCK: a FIFO would
// otherwise block before the regular-file check.
const inventoryOpenFlags = os.O_RDONLY | syscall.O_NONBLOCK

// openNoFollow opens path read-only and refuses a symlink at the final component.
func openNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, inventoryOpenFlags|syscall.O_NOFOLLOW, 0) // #nosec G304 -- validated user-chosen export file
}
