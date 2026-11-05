// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (issue #492). Not generated. Recorded hand-fix
// `qbo-token-refresh` in handfixes.json.

package cliutil

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// qboSyncFile, qboRenameFile and qboSyncDirectory are the steps whose order
// makes a write durable; tests replace them to observe that order and their
// failures.
var (
	qboSyncFile      = func(f *os.File) error { return f.Sync() }
	qboRenameFile    = renamePrivateFileWithRetry
	qboSyncDirectory = qboSyncDir
)

// DurableAtomicWritePrivateFile is AtomicWritePrivateFile for a file whose
// loss costs a credential: the OAuth tokens a refresh returns, after which
// QuickBooks may no longer honor the earlier refresh token. The temporary
// file is created in the same directory with O_CREATE|O_EXCL (os.CreateTemp),
// so an existing file or symlink at its name is never opened or followed; it
// gets fileMode, the data, and an fsync before it is renamed over path. The
// directory is then synced so the rename survives a crash too; a failure
// there is returned (the new file is already in place, but its durability is
// not confirmed). A symlink at path itself is replaced, not written through.
func DurableAtomicWritePrivateFile(path string, data []byte, fileMode, dirMode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("creating private file dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary private file: %w", err)
	}
	tmpPath := tmp.Name()
	fail := func(step string, err error) error {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%s temporary private file: %w", step, err)
	}
	if err := tmp.Chmod(fileMode); err != nil {
		return fail("securing", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fail("writing", err)
	}
	if err := qboSyncFile(tmp); err != nil {
		return fail("flushing", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing temporary private file: %w", err)
	}
	if err := qboRenameFile(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("publishing private file: %w", err)
	}
	if err := qboSyncDirectory(dir); err != nil {
		return fmt.Errorf("the file was written, but flushing its directory failed: %w", err)
	}
	return nil
}

// qboSyncDir flushes a directory entry change. Windows cannot open a
// directory for sync and commits a rename with the file system's own
// journal, so it is skipped there, as is a file system that does not
// support a directory fsync.
func qboSyncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(filepath.Clean(dir))
	if err != nil {
		return err
	}
	syncErr := d.Sync()
	closeErr := d.Close()
	if syncErr != nil && !qboDirSyncUnsupported(syncErr) {
		return syncErr
	}
	return closeErr
}
