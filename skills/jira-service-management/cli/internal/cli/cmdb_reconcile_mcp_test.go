// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build !windows

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOpenInventoryUnderMCP covers the MCP containment for cmdb reconcile
// --file: files inside the working directory open; a symlinked directory
// pointing outside, a ".." path, an absolute path elsewhere, and a
// filesystem-root working directory are refused.
func TestOpenInventoryUnderMCP(t *testing.T) {
	wd := t.TempDir()
	outside := t.TempDir()
	for _, p := range []string{filepath.Join(wd, "rmm.csv"), filepath.Join(outside, "secret.csv")} {
		if err := os.WriteFile(p, []byte("Name\nsrv-01\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(wd, "link")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(wd)

	f, err := openInventoryUnderMCP("rmm.csv")
	if err != nil {
		t.Fatalf("file inside the working directory refused: %v", err)
	}
	_ = f.Close()
	for _, bad := range []string{"link/secret.csv", "../" + filepath.Base(outside) + "/secret.csv", filepath.Join(outside, "secret.csv")} {
		if f, err := openInventoryUnderMCP(bad); err == nil {
			_ = f.Close()
			t.Fatalf("%s escaped the working directory", bad)
		}
	}

	t.Chdir("/")
	if f, err := openInventoryUnderMCP(filepath.Join(wd, "rmm.csv")); err == nil {
		_ = f.Close()
		t.Fatal("filesystem-root working directory was allowed")
	}
}
