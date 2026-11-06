// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json review-r1-kit-child-and-config-writes).

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A rollback restores a credential store's bytes, never its group or world
// access: a credential is never written to a file other users can read.
func TestRestoreCredentialsFileIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits")
	}
	p := filepath.Join(t.TempDir(), "credentials.toml")
	snap := credentialsSnapshot{path: p, data: []byte("api_key = \"Hc5Pw9Ld3Ks7Nq2X\"\n"), perm: 0o644}
	if err := restoreCredentialsFile(snap); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("restored credential store has mode %o", info.Mode().Perm())
	}
}
