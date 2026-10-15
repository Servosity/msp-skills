// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// TEMPORARY - proof that the windows CI job catches a Windows-only defect.
// Never merged. Removing a file that is still open succeeds on Linux and
// macOS and fails on Windows, the file-in-use class a store rotation hits.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProofRemoveWhileOpen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "data.db")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := os.Remove(p); err != nil {
		t.Fatalf("remove while open: %v", err)
	}
}
