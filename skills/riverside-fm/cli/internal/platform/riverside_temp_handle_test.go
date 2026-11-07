// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// openFDs counts this process's open descriptors. Unix only: Windows CI
// catches the same defect as a rename sharing violation.
func openFDs(t *testing.T) int {
	t.Helper()
	d, err := os.Open("/dev/fd")
	if err != nil {
		t.Skipf("no /dev/fd: %v", err)
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		t.Skipf("no /dev/fd: %v", err)
	}
	return len(names)
}

// Hand-fix (handfixes.json temp-handle-closed-before-kit-write): the temp
// file is closed before the kit writes it and before rename, so Windows can
// rename it and no descriptor leaks.
func TestWritePrivateFileClosesTempHandle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("covered on Windows by the rename in the profile conformance tests")
	}
	path := filepath.Join(t.TempDir(), "clients", "tenant-a", "profile.toml")
	before := openFDs(t)
	if err := writePrivateFile(path, []byte("name = \"tenant-a\"\n")); err != nil {
		t.Fatalf("writePrivateFile: %v", err)
	}
	if after := openFDs(t); after != before {
		t.Fatalf("open descriptors %d -> %d: the temp file was left open", before, after)

	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "name = \"tenant-a\"\n" {
		t.Fatalf("profile = %q, %v", got, err)
	}
}
