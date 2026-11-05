// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (issue #492). Not generated. Recorded hand-fix
// `qbo-token-refresh` in handfixes.json.

package cliutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The refreshed tokens reach the disk before the rename publishes them: the
// temporary file is flushed (fsync) first, then renamed over the store.
func TestDurableWriteFlushesBeforeTheRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.toml")
	var order []string
	prevSync, prevRename := qboSyncFile, qboRenameFile
	t.Cleanup(func() { qboSyncFile, qboRenameFile = prevSync, prevRename })
	qboSyncFile = func(f *os.File) error {
		if st, err := f.Stat(); err != nil || st.Size() != int64(len("token = 1\n")) {
			t.Errorf("synced before the data was written")
		}
		order = append(order, "sync")
		return f.Sync()
	}
	qboRenameFile = func(from, to string) error {
		order = append(order, "rename")
		return os.Rename(from, to)
	}
	if err := DurableAtomicWritePrivateFile(path, []byte("token = 1\n"), 0o600, 0o700); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "sync" || order[1] != "rename" {
		t.Fatalf("steps = %v, want sync then rename", order)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v, want 0600", st.Mode().Perm())
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".credentials.toml.*.tmp")); len(left) != 0 {
		t.Fatalf("temporary files left behind: %v", left)
	}
}

// A symlink at the store's path is replaced by the new file; the write
// never goes through it to the file it names.
func TestDurableWriteReplacesASymlinkInsteadOfFollowingIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need a privilege on Windows")
	}
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credentials.toml")
	if err := os.Symlink(victim, path); err != nil {
		t.Fatal(err)
	}
	if err := DurableAtomicWritePrivateFile(path, []byte("token = 2\n"), 0o600, 0o700); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "untouched" {
		t.Fatalf("the write went through the symlink: %q", b)
	}
	if st, err := os.Lstat(path); err != nil || st.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("the store is still a symlink")
	}
}

// A symlink planted at the refresh lock's path is refused: the lock never
// creates, writes or locks the file it names, and fn does not run.
func TestTokenLockRefusesASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need a privilege on Windows")
	}
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credentials.toml.refresh")
	if err := os.Symlink(victim, path+".lock"); err != nil {
		t.Fatal(err)
	}
	ran := false
	err := QBOWithTokenLock(path, func() error { ran = true; return nil })
	if err == nil || ran {
		t.Fatalf("the lock followed a symlink (err %v, fn ran %v)", err, ran)
	}
	if st, _ := os.Stat(victim); st.Size() != 0 {
		t.Fatal("the lock wrote to the symlink's target")
	}
	// A dangling symlink is refused too, so nothing is created at its target.
	dangling := filepath.Join(dir, "elsewhere")
	path2 := filepath.Join(dir, "other.refresh")
	if err := os.Symlink(dangling, path2+".lock"); err != nil {
		t.Fatal(err)
	}
	if err := QBOWithTokenLock(path2, func() error { return nil }); err == nil {
		t.Fatal("the lock followed a dangling symlink")
	}
	if _, err := os.Stat(dangling); err == nil {
		t.Fatal("the lock created the dangling symlink's target")
	}
	// The ordinary case still locks and runs fn.
	if err := QBOWithTokenLock(filepath.Join(dir, "fine.refresh"), func() error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("a plain lock failed: %v", err)
	}
}

// Codex round 1 (P2): a directory flush that fails is reported, not treated
// as durable success.
func TestDurableWriteReportsAFailedDirectoryFlush(t *testing.T) {
	prev := qboSyncDirectory
	t.Cleanup(func() { qboSyncDirectory = prev })
	qboSyncDirectory = func(string) error { return os.ErrPermission }
	path := filepath.Join(t.TempDir(), "credentials.toml")
	err := DurableAtomicWritePrivateFile(path, []byte("token = 3\n"), 0o600, 0o700)
	if err == nil || !strings.Contains(err.Error(), "flushing its directory failed") {
		t.Fatalf("a failed directory flush returned %v", err)
	}
}
