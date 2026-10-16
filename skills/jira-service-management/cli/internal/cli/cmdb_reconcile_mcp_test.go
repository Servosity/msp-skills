// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build !windows

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestInventoryPathAllowedUnderMCP covers the MCP containment for
// cmdb reconcile --file: a filesystem-root working directory, a symlinked
// directory pointing outside, and an opened file that is not the file the
// path resolves to (the directory-swap race).
func TestInventoryPathAllowedUnderMCP(t *testing.T) {
	wd := t.TempDir()
	outside := t.TempDir()
	inside := filepath.Join(wd, "rmm.csv")
	secret := filepath.Join(outside, "secret.csv")
	for _, p := range []string{inside, secret} {
		if err := os.WriteFile(p, []byte("Name\nsrv-01\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(wd, "link")); err != nil {
		t.Fatal(err)
	}
	open := func(p string) *os.File {
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Close() })
		return f
	}
	t.Chdir(wd)

	if err := inventoryPathAllowedUnderMCP("rmm.csv", open(inside)); err != nil {
		t.Fatalf("file inside the working directory refused: %v", err)
	}
	if err := inventoryPathAllowedUnderMCP("link/secret.csv", open(secret)); err == nil {
		t.Fatal("file reached through a symlinked directory outside the working directory was allowed")
	}
	// The path resolves inside, but the descriptor holds a different file:
	// what a directory swapped for a symlink between open and check yields.
	if err := inventoryPathAllowedUnderMCP("rmm.csv", open(secret)); err == nil {
		t.Fatal("opened file differing from the checked path was allowed")
	}

	t.Chdir("/")
	if err := inventoryPathAllowedUnderMCP(inside, open(inside)); err == nil {
		t.Fatal("filesystem-root working directory was allowed")
	}
}
