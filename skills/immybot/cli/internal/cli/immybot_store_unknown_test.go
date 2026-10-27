// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"immybot-pp-cli/internal/cliutil"
	"immybot-pp-cli/internal/cliutil/testenv"
)

// Whenever the instance is unknown (a config that fails to load, or a path
// asked for before any scope was set), the store path is neither data.db nor
// anything else a known instance owns.
func TestAnUnknownInstanceNeverGetsTheUnscopedStore(t *testing.T) {
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	dir := t.TempDir()
	unscoped := filepath.Join(dir, "data.db")
	if err := os.WriteFile(unscoped, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })

	bad := filepath.Join(home, "bad.toml")
	if err := os.WriteFile(bad, []byte("base_url = [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IMMYBOT_SUBDOMAIN", "beta")
	configureDefaultDBScope(bad)
	if got := defaultDBPathInDir(dir); got == unscoped {
		t.Errorf("a config that fails to load opened %s", got)
	}

	setDefaultDBScopeIdentity("", "", "")
	if got := defaultDBPathInDir(dir); got == unscoped {
		t.Errorf("a path asked for with no scope set opened %s", got)
	}

	// The other direction: a config that loads gets its own instance's store.
	good := filepath.Join(home, "good.toml")
	if err := os.WriteFile(good, []byte("base_url = \"https://{subdomain}.immy.bot\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configureDefaultDBScope(bad)
	unknown := defaultDBPathInDir(dir)
	configureDefaultDBScope(good)
	if known := defaultDBPathInDir(dir); known == unscoped || known == unknown {
		t.Errorf("a config that loads got %s (unknown scope %s)", known, unknown)
	}
}
