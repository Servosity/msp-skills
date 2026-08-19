// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

//go:build windows

package cli

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// acquireSnapshotLock is the Windows half of the sidecar advisory lock. See
// auvik_snapshot_lock_unix.go for why the lock exists and why it must be held
// across the store OPEN rather than only the write transaction.
//
// Windows has no flock(2). LockFileEx is the equivalent primitive: with
// LOCKFILE_EXCLUSIVE_LOCK and no LOCKFILE_FAIL_IMMEDIATELY it blocks until the
// lock is available, which matches the Unix LOCK_EX behavior the caller expects
// (peers queue rather than erroring out). The lock is taken on a byte range;
// locking the whole possible range with a 0xFFFFFFFF/0xFFFFFFFF length is the
// conventional way to express "the entire file" on a zero-byte sidecar.
//
// UnlockFileEx is not strictly required - closing the handle drops the lock -
// but it is called explicitly so the release path is symmetric with the Unix
// one and does not depend on close ordering.
func acquireSnapshotLock(dbPath string) (release func(), err error) {
	lockPath := dbPath + ".snapshot-lock"
	// #nosec G304 -- lockPath is this CLI's own --db path plus a fixed suffix, on
	// the operator's own machine. It is a zero-byte advisory lock sidecar opened
	// 0600; nothing is read from it and no untrusted input reaches the path.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening snapshot lock %s: %w", lockPath, err)
	}
	if err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,
		lockRangeLow,
		lockRangeHigh,
		new(windows.Overlapped),
	); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquiring snapshot lock (another 'inventory diff --snapshot' may be running): %w", err)
	}
	return func() {
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, lockRangeLow, lockRangeHigh, new(windows.Overlapped))
		_ = f.Close()
	}, nil
}

// Lock the entire possible byte range of the zero-byte sidecar.
const (
	lockRangeLow  = 0xFFFFFFFF
	lockRangeHigh = 0xFFFFFFFF
)
