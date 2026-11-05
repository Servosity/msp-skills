// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"kaseya-vsa-pp-cli/internal/cliutil"
	"kaseya-vsa-pp-cli/internal/cliutil/testenv"
)

// TestLooseProfileStoreIsRefused: a profile sets root flags such as --insecure,
// --ca-cert and --tls-server-name, so a profiles.json other users can change
// must not be applied (it would turn TLS verification off for the token).
func TestLooseProfileStoreIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits do not grant access on Windows; ReadOpenedConfig's ACL check is tested in cliutil")
	}
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	p, err := profileStorePath()
	if err != nil {
		t.Fatal(err)
	}
	body := `{"profiles":{"nightly":{"name":"nightly","values":{"insecure":"true"}}}}`
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if prof, err := GetProfile("nightly"); err != nil || prof == nil {
		t.Fatalf("owner-only store: profile=%v err=%v", prof, err)
	}
	if err := os.Chmod(p, 0o666); err != nil {
		t.Fatal(err)
	}
	if prof, err := GetProfile("nightly"); err == nil || prof != nil {
		t.Fatalf("loose store %s applied: profile=%v err=%v", filepath.Base(p), prof, err)
	}
}
