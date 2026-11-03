// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build unix

package credkit

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// within fails the test when f has not returned after d: a credential read
// that blocks would hang the program before its first statement runs.
func within(t *testing.T, d time.Duration, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatal("still running: the read blocked")
	}
}

// Contract 1, fail closed: a credential path that is a pipe is never opened
// (the open or the read would block, or take the program's own input); it
// makes every value unknown, so every unit is withheld.
func TestKitGatesFifoConfigNeverBlocks(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Skip("mkfifo:", err)
	}
	resetForTest(Declarations{})
	within(t, 3*time.Second, func() { atStartup(func() { gatherFile(p) }) })
	if !leaks("clean") {
		t.Error("a credential path that is a pipe did not make every unit withheld")
	}
	// After "--" the program never reads it, so neither does the kit.
	resetForTest(Declarations{})
	within(t, 3*time.Second, func() { atStartup(func() { gather(Declarations{}, []string{"version", "--", "--config", p}) }) })
	if leaks("clean") {
		t.Error(`a --config after "--" was read`)
	}
}

// Contract 1: filepath.Clean is lexical. With b a symbolic link, "dir/b/../c"
// is the OS's real/c, not dir/c; reading dir/c instead would hold the wrong
// file's values and leave the credentials the program reads unheld. The kit
// reads and writes the file the OS names, never the cleaned path's file.
func TestKitGatesSymlinkDotDotNeverReadsAnotherFile(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.MkdirAll(filepath.Join(real, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(real, "sub"), filepath.Join(dir, "b")); err != nil {
		t.Skip("symlink:", err)
	}
	if err := os.WriteFile(filepath.Join(real, "config.toml"), []byte("api_token = 'real-file-token-6262'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("api_token = 'decoy-token-5151'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "b") + "/../config.toml"
	if b, err := os.ReadFile(p); err != nil || string(b) != "api_token = 'real-file-token-6262'\n" {
		t.Fatalf("the OS reads %q (%v) at %s; the test's premise does not hold", b, err, p)
	}
	resetForTest(Declarations{ConfigFields: []string{"api_token"}})
	atStartup(func() { gatherFile(p) })
	if !leaks("real-file-token-6262") {
		t.Error("the credential in the file the program reads is not withheld")
	}
	resetForTest(Declarations{})
	w := filepath.Join(dir, "b") + "/../new.txt"
	if err := WriteFile(w, []byte("x\n"), 0o600); err != nil {
		t.Errorf("WriteFile(%s) = %v", w, err)
	}
	if b, err := os.ReadFile(filepath.Join(real, "new.txt")); err != nil || string(b) != "x\n" {
		t.Errorf("WriteFile did not write the file the OS names: %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); !os.IsNotExist(err) {
		t.Error("WriteFile created the cleaned path's file")
	}
}

// The opens go through os.Root, which alone would refuse a final symbolic
// link that leaves the directory (a dotfile manager's link). The kit follows
// it as the OS does, and an O_NOFOLLOW open still refuses it.
func TestKitGatesFinalSymlinkFollowedAsTheOSDoes(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	target := filepath.Join(b, "config.toml")
	if err := os.WriteFile(target, []byte("api_token = 'linked-token-7373'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(a, "config.toml")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlink:", err)
	}
	if err := os.Symlink("config.toml", filepath.Join(a, "rel")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{link, filepath.Join(a, "rel")} {
		if got, err := readRegular(p); err != nil || string(got) != "api_token = 'linked-token-7373'\n" {
			t.Errorf("readRegular(%s) = %q, %v", p, got, err)
		}
	}
	if f, err := openPath(link, os.O_RDONLY|openNofollow, 0); err == nil {
		_ = f.Close()
		t.Error("an O_NOFOLLOW open followed a final symbolic link")
	}
	loop := filepath.Join(a, "loop")
	if err := os.Symlink("loop", loop); err != nil {
		t.Fatal(err)
	}
	if _, err := readRegular(loop); err == nil {
		t.Error("a symbolic link loop was read")
	}
}

// os.OpenFile with O_CREATE|O_EXCL refuses an existing name, a symbolic link
// included, even one whose target does not exist. The kit's open does too:
// it never follows the link and creates the target.
func TestKitGatesExclusiveCreateRefusesASymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlink:", err)
	}
	resetForTest(Declarations{})
	if w, err := OpenFile(link, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err == nil {
		_ = w.Close()
		t.Error("an exclusive create through a symbolic link succeeded")
	} else if !os.IsExist(err) {
		t.Errorf("err = %v, want the name to exist", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Error("an exclusive create through a symbolic link created its target")
	}
}
