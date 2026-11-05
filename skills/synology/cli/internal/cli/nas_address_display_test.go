// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(dsm-display-address).

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"synology-pp-cli/internal/cliutil"
)

// TestNASAddressNeverPrintedRaw: SYNOLOGY_BASE_URL=https://admin:Password7731@nas:5001
// made doctor --json print the password in base_url; a query or fragment in
// the address reached doctor and the dry-run preview lines; a fleet entry
// hand-edited with a password printed it in fleet list and fleet add. Every
// printed NAS address goes through dsm.DisplayAddress.
func TestNASAddressNeverPrintedRaw(t *testing.T) {
	secrets := []string{"Password7731", "QSECRET44", "FRAGSECRET55"}
	check := func(label, text string) {
		t.Helper()
		for _, s := range secrets {
			if strings.Contains(text, s) {
				t.Errorf("%s printed %q: %s", label, s, text)
			}
		}
	}
	run := func(args ...string) string {
		out, errOut, _ := sweepExecute(t, args...)
		return out + errOut
	}

	sweepIsolate(t)
	t.Setenv("SYNOLOGY_ACCOUNT", "admin")
	t.Setenv("SYNOLOGY_PASSWORD", "pw-other-1")
	t.Setenv("SYNOLOGY_BASE_URL", "https://admin:Password7731@127.0.0.1:9/?tok=QSECRET44#FRAGSECRET55")
	out := run("doctor", "--json")
	if !strings.Contains(out, `"base_url": "https://127.0.0.1:9/"`) {
		t.Errorf("doctor --json base_url is not the display address: %s", out)
	}
	check("doctor --json (user info)", out)
	check("doctor (user info)", run("doctor"))

	t.Setenv("SYNOLOGY_BASE_URL", "https://127.0.0.1:9/?tok=QSECRET44#FRAGSECRET55")
	for _, args := range [][]string{
		{"doctor", "--json"}, {"doctor"}, {"user", "--dry-run"}, {"system", "dsm", "--dry-run"},
		{"abb", "backup", "--task-ids", "[5]", "--dry-run"}, {"abb", "backup", "--task-ids", "[5]", "--dry-run", "--json"},
		{"session", "logout", "--dry-run"}, {"auth", "login"},
	} {
		text := run(args...)
		check(strings.Join(args, " ")+" (query)", text)
		if strings.Contains(strings.Join(args, " "), "dry-run") && !strings.Contains(text, "127.0.0.1:9") {
			t.Errorf("%v no longer names the NAS: %s", args, text)
		}
	}

	// A fleet entry hand-edited to carry a password and a query.
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	reg, _ := json.Marshal(map[string]any{"nas": []map[string]string{{"name": "acme", "url": "https://admin:Password7731@127.0.0.1:9?tok=QSECRET44", "env_prefix": "SYNOLOGY_ACME"}}})
	if err := os.WriteFile(filepath.Join(dir, "fleet.json"), reg, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "a")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "pw-acme-1")
	for _, args := range [][]string{
		{"fleet", "list", "--json"}, {"fleet", "list"}, {"fleet", "sync", "--dry-run"}, {"fleet", "sync", "--dry-run", "--json"},
		{"fleet", "add", "acme", "--tls-fingerprint", strings.Repeat("ab", 32), "--json"}, {"fleet", "add", "acme", "--tls-fingerprint", strings.Repeat("cd", 32)},
	} {
		text := run(args...)
		check(strings.Join(args, " ")+" (registry)", text)
		if !strings.Contains(text, "127.0.0.1:9") {
			t.Errorf("%v no longer names the NAS: %s", args, text)
		}
	}
}
