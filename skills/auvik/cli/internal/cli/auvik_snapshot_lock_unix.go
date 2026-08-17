// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

//go:build !windows

package cli

import (
	"fmt"
	"os"
	"syscall"
)

// withSnapshotLock serializes `inventory diff --snapshot` across processes.
//
// WHY: the snapshot write is the only place a hand-written command opens the
// local store read-write. Two of them running at once (an agent fanning out, a
// cron overlapping an interactive run, or a verification harness sampling
// several commands in parallel) both grow the SQLite file while the driver holds
// it mmap'd. When one process extends the file the other's mapping is no longer
// backed by valid pages, and the read faults with SIGBUS rather than returning a
// normal "database is busy" error -- SQLite's own busy handling never gets a
// chance to run, because the fault happens below it.
//
// Reproduced deterministically: 60 sequential runs pass; 12 concurrent runs
// crash only the writers, never the readers.
//
// An advisory exclusive lock on a sidecar file makes the write path
// single-writer. Readers are unaffected -- they never take this lock and now
// never open the store read-write at all.
// The lock MUST be held across the store OPEN, not just the write transaction.
// store.OpenWithContext runs migrations, which themselves write and can grow the
// file. Locking only the later INSERT still left the migration write racing a
// peer's mapping -- measured: 5 of 36 processes still crashed. Holding the lock
// from before the open through after the close takes it to 0.
func acquireSnapshotLock(dbPath string) (release func(), err error) {
	lockPath := dbPath + ".snapshot-lock"
	// #nosec G304 -- lockPath is this CLI's own --db path plus a fixed suffix, on
	// the operator's own machine. It is a zero-byte advisory lock sidecar opened
	// 0600; nothing is read from it and no untrusted input reaches the path.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening snapshot lock %s: %w", lockPath, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquiring snapshot lock (another 'inventory diff --snapshot' may be running): %w", err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
