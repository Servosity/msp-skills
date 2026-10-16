//go:build windows

package cli

import (
	"errors"
	"os"
)

// openNoFollow opens path read-only and refuses a symlink or other reparse point.
// Windows has no O_NOFOLLOW, so Lstat first, then prove the opened handle is the
// same file so a swap between the check and the open is refused too.
func openNoFollow(path string) (*os.File, error) {
	li, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !li.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	f, err := os.Open(path) // #nosec G304 -- validated user-chosen export file
	if err != nil {
		return nil, err
	}
	if fi, err := f.Stat(); err != nil || !os.SameFile(li, fi) {
		_ = f.Close()
		return nil, errors.New("file changed while opening")
	}
	return f, nil
}
