// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package knowbe4blackbox

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

// makeLoose makes p readable by other users: POSIX 0644, or on Windows a
// BUILTIN\Users read ACE by SID (locale-independent). An icacls failure skips.
func makeLoose(t *testing.T, p string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("icacls", p, "/grant", "*S-1-5-32-545:R").CombinedOutput(); err != nil {
			t.Skipf("icacls grant failed; skipping: %v\n%s", err, out)
		}
		return
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
}
