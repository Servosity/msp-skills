// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// kaseya-bms handfix (loose-file-every-os): a test that needs a file other
// users can read makes it so on every OS. Skipping the chmod on Windows left a
// t.TempDir() file with its private inherited DACL, so the permission refusal
// the test relies on never happened there.

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

// loosen makes path readable by other users: mode 0644 on POSIX, and on
// Windows an allow-ACE for BUILTIN\Users (by SID, so any locale works). A
// failure fails the test: a skipped loosening is a case that never ran.
func loosen(t testing.TB, path string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatalf("loosen %s: %v", path, err)
		}
		return
	}
	if out, err := exec.Command("icacls", path, "/grant", "*S-1-5-32-545:R").CombinedOutput(); err != nil {
		t.Fatalf("loosen %s: icacls: %v\n%s", path, err, out)
	}
}
