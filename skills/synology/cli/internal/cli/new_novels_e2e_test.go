// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Cobra-level checks for the novels the 4.34.2 reprint added.

package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"synology-pp-cli/internal/cliutil/testenv"
)

// TestNewNovelsDryRunEmptyFleetAndSyncedMirror: abb unprotected, fleet capacity
// and abb change-rate preview under --dry-run, refuse an empty fleet (exit 10,
// never an empty all-clear), refuse a bad flag (exit 2), and answer from a
// synced mirror with JSON rows that keep null for what was not measured.
func TestNewNovelsDryRunEmptyFleetAndSyncedMirror(t *testing.T) {
	testenv.Isolate(t)
	cmds := [][]string{{"abb", "unprotected"}, {"fleet", "capacity"}, {"abb", "change-rate"}}
	for _, c := range cmds {
		if out, code := runCLI(t, append(c, "--dry-run", "--json")...); code != 0 || !strings.Contains(out, `"dry_run":true`) {
			t.Fatalf("%v --dry-run: code=%d out=%s", c, code, out)
		}
		if out, code := runCLI(t, append(c, "--json")...); code != 10 || strings.TrimSpace(out) != "" {
			t.Fatalf("%v on an empty fleet: code=%d out=%q", c, code, out)
		}
	}
	for _, bad := range [][]string{{"abb", "unprotected", "--since", "0d"}, {"fleet", "capacity", "--horizon", "soon"},
		{"abb", "change-rate", "--spike", "-1"}, {"abb", "change-rate", "--flat-runs", "-2"}} {
		if _, code := runCLI(t, append(bad, "--json")...); code != 2 {
			t.Fatalf("%v: exit %d, want 2", bad, code)
		}
	}

	srv := fakeABBNAS(t)
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "s3cret")
	if out, code := runCLI(t, "fleet", "add", "acme", "--url", srv.URL, "--json"); code != 0 {
		t.Fatalf("fleet add: code=%d out=%s", code, out)
	}
	if out, code := runCLI(t, "fleet", "sync", "--json"); code != 0 {
		t.Fatalf("fleet sync: code=%d out=%s", code, out)
	}
	for _, c := range cmds {
		out, code := runCLI(t, append(c, "--json", "--no-fail")...)
		var rows []map[string]any
		if code != 0 || json.Unmarshal([]byte(out), &rows) != nil {
			t.Fatalf("%v on a synced mirror: code=%d out=%s", c, code, out)
		}
		for _, r := range rows {
			if r["kind"] == "nas_not_synced" {
				t.Fatalf("%v: a freshly synced NAS reads as not synced: %s", c, out)
			}
		}
	}
	// One sync is one storage snapshot: no growth yet, so no runway, and the
	// unknown is null, never 0.
	out, _ := runCLI(t, "fleet", "capacity", "--json", "--no-fail")
	if !strings.Contains(out, `"kind": "volume"`) || !strings.Contains(out, `"days_to_full": null`) || strings.Contains(out, `"days_to_full": 0`) {
		t.Fatalf("fleet capacity after one sync: %s", out)
	}
}
