// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

// loosen makes p readable by other users: 0644 on POSIX, and on Windows an
// allow-ACE for BUILTIN\Users by SID (a chmod changes nothing there, so the
// config would still read as private and the refusal under test never runs).
func loosen(t *testing.T, p string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("icacls", p, "/grant", "*S-1-5-32-545:R").CombinedOutput(); err != nil {
			t.Skipf("icacls grant failed (environment); skipping: %v\n%s", err, out)
		}
		return
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
}
