//go:build windows

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// profiles.json and receipts written under a directory that grants Everyone
// read do not inherit that ACE (handfixes.json private-file-owner-only-dacl).
func TestWritePrivateFile_PublicDirectoryStaysPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shared")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("icacls", dir, "/grant", "*S-1-1-0:(OI)(CI)R").CombinedOutput(); err != nil {
		t.Fatalf("icacls: %v %s", err, out)
	}
	path := filepath.Join(dir, "profiles.json")
	if err := writePrivateFile(path, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("icacls", path).CombinedOutput()
	if err != nil {
		t.Fatalf("icacls: %v %s", err, out)
	}
	if s := strings.ToLower(string(out)); strings.Contains(s, "everyone") || strings.Contains(s, "s-1-1-0") {
		t.Fatalf("profiles.json inherited the shared directory's Everyone ACE:\n%s", out)
	}
}
