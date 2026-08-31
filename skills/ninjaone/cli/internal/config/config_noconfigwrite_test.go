// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// Issue #270: a supported way to run this connector with no credential
// touching disk. NINJAONE_NO_CONFIG_WRITE makes the automatic token cache a
// no-op; it must NOT make the `auth logout` wipe inert, and with the switch
// unset or falsey the bytes written must be identical to what shipped before.
//
// The assertions look at EVERY file the process writes under a throwaway HOME
// rather than at one known path, so they hold whatever the connector's config
// format is and wherever it decides to keep credentials.

const (
	ncwToken   = "minted-token-NINJAONE"
	ncwRefresh = "minted-refresh-NINJAONE"
	ncwSecret  = "minted-secret-NINJAONE"
)

// ncwSnapshot returns relpath -> contents for every regular file under root.
func ncwSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		out[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return out
}

func ncwFilesMentioning(snap map[string]string, needle string) []string {
	var hits []string
	for name, body := range snap {
		if strings.Contains(body, needle) {
			hits = append(hits, name)
		}
	}
	sort.Strings(hits)
	return hits
}

// ncwMint runs one "an ordinary command minted a token" cycle under a
// throwaway HOME and returns everything that landed on disk.
func ncwMint(t *testing.T, value string, set bool) map[string]string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NINJAONE_CONFIG", filepath.Join(home, "config.toml"))
	if set {
		t.Setenv(NoConfigWriteEnv, value)
	} else {
		os.Unsetenv(NoConfigWriteEnv)
	}
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// A fixed expiry keeps the byte comparison deterministic.
	expiry := time.Unix(1767225600, 0).UTC()
	if err := cfg.SaveTokens("minted-cid", ncwSecret, ncwToken, ncwRefresh, expiry); err != nil {
		t.Fatalf("SaveTokens must still succeed: %v", err)
	}
	if cfg.AccessToken != ncwToken {
		t.Fatalf("the minted token was dropped in memory: %q", cfg.AccessToken)
	}
	return ncwSnapshot(t, home)
}

// Direction 1 - the switch ON: the mint succeeds and writes nothing.
func TestNoConfigWriteLeavesNothingOnDisk(t *testing.T) {
	snap := ncwMint(t, "1", true)
	for _, secret := range []string{ncwToken, ncwRefresh, ncwSecret} {
		if hits := ncwFilesMentioning(snap, secret); len(hits) > 0 {
			t.Errorf("%s reached disk despite %s=1, in: %v", secret, NoConfigWriteEnv, hits)
		}
	}
	if len(snap) != 0 {
		names := make([]string, 0, len(snap))
		for n := range snap {
			names = append(names, n)
		}
		sort.Strings(names)
		t.Errorf("%s=1 still wrote %d file(s): %v", NoConfigWriteEnv, len(snap), names)
	}
}

// Direction 2 - the switch OFF: byte-identical to the unset behaviour, for a
// genuinely unset variable and for every falsey spelling.
func TestWithoutTheSwitchTheCacheIsByteIdentical(t *testing.T) {
	baseline := ncwMint(t, "", false)
	if hits := ncwFilesMentioning(baseline, ncwToken); len(hits) == 0 {
		t.Fatalf("baseline cached nothing, so this test proves nothing; wrote: %v", baseline)
	}
	for _, off := range []string{"", "0", "false", "no", "off", "OFF", " false "} {
		got := ncwMint(t, off, true)
		if !reflect.DeepEqual(got, baseline) {
			t.Errorf("%s=%q changed what was written.\nbaseline: %v\ngot:      %v",
				NoConfigWriteEnv, off, baseline, got)
		}
	}
}

// The erase is not a credential write. `auth logout` must still clear a token
// that is already on disk, even with the switch set - guarding save()
// unconditionally would make logout silently inert.
func TestNoConfigWriteStillAllowsTheWipe(t *testing.T) {
	home := t.TempDir()
	cfgPath := filepath.Join(home, "config.toml")
	t.Setenv("HOME", home)
	t.Setenv("NINJAONE_CONFIG", cfgPath)

	// Seed through the connector's own writer so the on-disk format is
	// whatever this connector actually uses.
	os.Unsetenv(NoConfigWriteEnv)
	seed, err := Load("")
	if err != nil {
		t.Fatalf("Load (seed): %v", err)
	}
	if err := seed.SaveTokens("minted-cid", ncwSecret, ncwToken, ncwRefresh, time.Unix(1767225600, 0).UTC()); err != nil {
		t.Fatalf("SaveTokens (seed): %v", err)
	}
	if hits := ncwFilesMentioning(ncwSnapshot(t, home), ncwToken); len(hits) == 0 {
		t.Fatal("seed did not reach disk, so the wipe has nothing to prove")
	}

	t.Setenv(NoConfigWriteEnv, "1")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.ClearTokens(); err != nil {
		t.Fatalf("ClearTokens: %v", err)
	}
	for _, secret := range []string{ncwToken, ncwRefresh, ncwSecret} {
		if hits := ncwFilesMentioning(ncwSnapshot(t, home), secret); len(hits) > 0 {
			t.Errorf("logout left %s on disk, in: %v", secret, hits)
		}
	}
}

func TestNoConfigWriteTruthiness(t *testing.T) {
	cases := map[string]bool{
		"": false, "0": false, "false": false, "no": false, "off": false,
		"FALSE": false, " Off ": false,
		"1": true, "true": true, "yes": true, "on": true, "anything": true,
	}
	for value, want := range cases {
		t.Setenv(NoConfigWriteEnv, value)
		if got := NoConfigWrite(); got != want {
			t.Errorf("NoConfigWrite() with %s=%q = %v, want %v", NoConfigWriteEnv, value, got, want)
		}
	}
	os.Unsetenv(NoConfigWriteEnv)
	if NoConfigWrite() {
		t.Errorf("NoConfigWrite() = true with %s unset", NoConfigWriteEnv)
	}
	if NoConfigWriteEnv != "NINJAONE_NO_CONFIG_WRITE" {
		t.Errorf("switch is named %q; the documented name is NINJAONE_NO_CONFIG_WRITE", NoConfigWriteEnv)
	}
}
