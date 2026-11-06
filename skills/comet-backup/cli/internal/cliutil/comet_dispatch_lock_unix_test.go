//go:build !windows

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// comet-backup handfix (rerun-dispatch-lock): a lock directory that is not
// provably this account's own refuses the dispatch; a private one works.

package cliutil

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

// checkFixedDispatchLockIdentity: the lock is at a fixed absolute path named
// by the effective uid, never under a home, XDG or temp lookup.
func checkFixedDispatchLockIdentity(t *testing.T, got string) {
	t.Helper()
	if want := "/tmp/comet-backup-locks-" + strconv.Itoa(os.Geteuid()) + "/" + testDispatchLock + ".lock"; got != want {
		t.Fatalf("lock = %q, want the fixed per-account path %q", got, want)
	}
}

func TestDispatchLockRefusesHostileDirectory(t *testing.T) {
	type hostile struct {
		name  string
		setup func(t *testing.T, dir string)
	}
	cases := []hostile{
		{"symlink to a private directory", func(t *testing.T, dir string) {
			target := t.TempDir()
			mustChmod(t, target, 0o700)
			mustSymlink(t, target, dir)
		}},
		{"symlink to a world-writable directory", func(t *testing.T, dir string) {
			target := t.TempDir()
			mustChmod(t, target, 0o777)
			mustSymlink(t, target, dir)
		}},
		{"dangling symlink", func(t *testing.T, dir string) {
			mustSymlink(t, filepath.Join(t.TempDir(), "absent"), dir)
		}},
		{"world-writable directory", func(t *testing.T, dir string) { mustMkdir(t, dir, 0o777) }},
		{"group-writable directory", func(t *testing.T, dir string) { mustMkdir(t, dir, 0o770) }},
		{"world-writable sticky directory", func(t *testing.T, dir string) {
			mustMkdir(t, dir, 0o700)
			mustChmod(t, dir, 0o777|os.ModeSticky)
		}},
		{"regular file", func(t *testing.T, dir string) {
			if err := os.WriteFile(dir, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"lock file is a symlink", func(t *testing.T, dir string) {
			mustMkdir(t, dir, 0o700)
			target := filepath.Join(t.TempDir(), "elsewhere.lock")
			if err := os.WriteFile(target, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			mustSymlink(t, target, filepath.Join(dir, testDispatchLock+".lock"))
		}},
		{"lock file is a directory", func(t *testing.T, dir string) {
			mustMkdir(t, dir, 0o700)
			mustMkdir(t, filepath.Join(dir, testDispatchLock+".lock"), 0o700)
		}},
	}
	if os.Geteuid() == 0 {
		cases = append(cases, hostile{"directory owned by another account", func(t *testing.T, dir string) {
			mustMkdir(t, dir, 0o700)
			if err := os.Chown(dir, 65534, 65534); err != nil {
				t.Fatal(err)
			}
		}})
	} else {
		t.Log("not root: the foreign-owner case runs only as root (chown)")
	}
	if runtime.GOOS == "darwin" {
		cases = append(cases, hostile{"darwin ACL grants everyone write", func(t *testing.T, dir string) {
			mustMkdir(t, dir, 0o700)
			if out, err := exec.Command("/bin/chmod", "+a", "everyone allow add_file,delete_child", dir).CombinedOutput(); err != nil {
				t.Skipf("chmod +a unavailable: %v %s", err, out)
			}
		}})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "comet-backup-locks-test")
			tc.setup(t, dir)
			ran, err := tryDispatchLockIn(dir, testDispatchLock, func() error {
				t.Error("the dispatch ran under a lock that is not private to this account")
				return nil
			})
			if ran || !errors.Is(err, ErrDispatchLockUnsafe) {
				t.Fatalf("= (%v, %v), want a refusal (ErrDispatchLockUnsafe)", ran, err)
			}
		})
	}
}

// The checks stay silent on the directories a healthy account has.
func TestDispatchLockAcceptsPrivateDirectory(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, dir string)
	}{
		{"missing (created 0700)", func(*testing.T, string) {}},
		{"existing 0700", func(t *testing.T, dir string) { mustMkdir(t, dir, 0o700) }},
		{"existing 0755", func(t *testing.T, dir string) { mustMkdir(t, dir, 0o755) }},
		{"existing lock file", func(t *testing.T, dir string) {
			mustMkdir(t, dir, 0o700)
			if err := os.WriteFile(filepath.Join(dir, testDispatchLock+".lock"), []byte{0}, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			mustChmod(t, parent, 0o700)
			dir := filepath.Join(parent, "comet-backup-locks-test")
			tc.setup(t, dir)
			ran, err := tryDispatchLockIn(dir, testDispatchLock, func() error { return nil })
			if !ran || err != nil {
				t.Fatalf("= (%v, %v), want the lock taken", ran, err)
			}
			fi, err := os.Lstat(dir)
			if err != nil {
				t.Fatal(err)
			}
			if !fi.IsDir() || fi.Mode().Perm()&0o022 != 0 {
				t.Fatalf("lock directory mode %v, want a directory no other account can write", fi.Mode())
			}
		})
	}
}

func mustMkdir(t *testing.T, dir string, mode os.FileMode) {
	t.Helper()
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustChmod(t, dir, mode)
}

func mustChmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// The lock keeps its mtime fresh for /tmp cleaners without writing to the file.
func TestDispatchLockFreshensMtimeWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "job.lock")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	ran, err := tryDispatchLockIn(dir, "job", func() error { return nil })
	if err != nil || !ran {
		t.Fatalf("ran=%v err=%v", ran, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Errorf("the lock file was written: %d bytes", info.Size())
	}
	if time.Since(info.ModTime()) > time.Hour {
		t.Errorf("mtime not freshened: %v", info.ModTime())
	}
}
