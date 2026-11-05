// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"netsapiens-pp-cli/internal/cliutil/testenv"
)

// credential-migration-fail-open: relocating a legacy config whose plaintext
// credential cannot be removed from the old file refuses and explains; it never
// reports success with the plaintext copy left behind.
func TestClassLegacyCredentialMigrationFailsClosed(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a directory the test user cannot write")
	}
	testenv.Isolate(t)
	dir := t.TempDir()
	legacyDir := filepath.Join(dir, "legacy")
	if err := os.Mkdir(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(legacyDir, "config.toml")
	if err := os.WriteFile(legacy, []byte("access_token = \"LegacyToken55443322\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(legacyDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(legacyDir, 0o700) })
	c := &Config{Path: filepath.Join(dir, "new", "config.toml"), legacySourcePath: legacy, AccessToken: "LegacyToken55443322"}
	if err := c.save(); err == nil {
		t.Fatal("the legacy plaintext credential could not be removed, yet the migration reported success")
	}
}
