// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): one way for a test to make a file that other
// users can read, on every platform the program checks.

package main

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

// makeReadableByOthers makes p readable by other users, which the program
// treats as untrusted: group and other read bits on unix, and on Windows an
// allow-ACE for BUILTIN\Users by SID (locale-independent), the signal its
// DACL check reads. os.Chmod alone changes no ACL on Windows. A failure fails
// the test: a skip would hide the Windows path.
func makeReadableByOthers(t *testing.T, p string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(p, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if out, err := exec.Command("icacls", p, "/grant", "*S-1-5-32-545:R").CombinedOutput(); err != nil {
		t.Fatalf("icacls grant: %v\n%s", err, out)
	}
}
