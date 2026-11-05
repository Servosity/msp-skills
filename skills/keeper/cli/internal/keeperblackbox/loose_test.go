// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package keeperblackbox

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

// makeLoose makes p readable by other users on every OS, so the binary's
// loose-file refusal really runs: POSIX mode 0644; on Windows a BUILTIN\Users
// read allow-ACE by SID (locale-independent), the same grant the config
// package's grantBroad uses. A file written with os.WriteFile on Windows
// inherits an owner-only temp ACL, so skipping this there left the refusal
// unexercised. A failed grant fails the test; it never skips it.
func makeLoose(t *testing.T, p string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		if b, err := exec.Command("icacls", p, "/grant", "*S-1-5-32-545:R").CombinedOutput(); err != nil {
			t.Fatalf("icacls grant on the loose test file failed: %v\n%s", err, b)
		}
		return
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
}
