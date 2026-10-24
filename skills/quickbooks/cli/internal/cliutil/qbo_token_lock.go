// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (issue #492). Not generated. Recorded hand-fix
// `qbo-token-refresh` in handfixes.json.

package cliutil

import (
	"fmt"
	"io"
	"path/filepath"
)

// QBOWithTokenLock serializes fn against every other caller on path, in this
// process and in every other one, with the same primitives as WithFileLock
// (the per-path mutex, then an exclusive OS lock on path+".lock"). It differs
// in how it opens the lock file (qboOpenLockFile): a symlink planted at that
// path is refused (O_NOFOLLOW; FILE_FLAG_OPEN_REPARSE_POINT on Windows)
// instead of followed, so the lock never creates, writes to or locks a file
// somewhere else, and anything but a regular file is refused too. The token refresh runs its re-read, check,
// refresh and save inside it.
func QBOWithTokenLock(path string, fn func() error) error {
	if path == "" {
		return fmt.Errorf("token lock: no path")
	}
	mu := fileLockFor(path)
	mu.Lock()
	defer mu.Unlock()
	lockPath := filepath.Clean(path + ".lock")
	f, err := qboOpenLockFile(lockPath)
	if err != nil {
		return fmt.Errorf("lock %s: %w", lockPath, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return fmt.Errorf("lock %s: %w", lockPath, err)
	}
	if st.Size() == 0 {
		_, _ = f.Write([]byte{0})
		_, _ = f.Seek(0, io.SeekStart)
	}
	if err := lockFile(f); err != nil {
		return fmt.Errorf("lock %s: %w", lockPath, err)
	}
	defer func() { _ = unlockFile(f) }()
	return fn()
}
