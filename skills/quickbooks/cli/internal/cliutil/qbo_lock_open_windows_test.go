//go:build windows

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (issue #492). Not generated. Recorded hand-fix
// `qbo-token-refresh` in handfixes.json.

package cliutil

import (
	"os"
	"path/filepath"
	"testing"
)

// Codex round 1 (P2): on Windows a symlink at the lock path is refused too
// (FILE_FLAG_OPEN_REPARSE_POINT), not followed to its target.
func TestTokenLockRefusesASymlinkOnWindows(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credentials.toml.refresh")
	if err := os.Symlink(victim, path+".lock"); err != nil {
		t.Skipf("this account cannot create symlinks: %v", err)
	}
	ran := false
	if err := QBOWithTokenLock(path, func() error { ran = true; return nil }); err == nil || ran {
		t.Fatalf("the lock followed a symlink (err %v, fn ran %v)", err, ran)
	}
	if st, _ := os.Stat(victim); st.Size() != 0 {
		t.Fatal("the lock wrote to the symlink's target")
	}
}
