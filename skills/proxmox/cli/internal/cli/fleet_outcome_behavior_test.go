// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// No reachable cluster must never look like a healthy, empty result.
func TestFleetAllUnreachable(t *testing.T) {
	newFixture(t)
	t.Setenv("PRINTING_PRESS_DOGFOOD", "1") // no client retries: keep the test fast
	t.Setenv("PROXMOX_BASE_URL", "https://127.0.0.1:1/api2/json")
	t.Setenv("PROXMOX_PBS_HOST", "127.0.0.1:2")
	for _, args := range [][]string{
		{"posture", "--json", "--fail-on-red"}, {"backup", "coverage", "--json"}, {"failures", "--json"},
		{"capacity", "--json"}, {"rpo", "--json"}, {"verify-health", "--json"},
	} {
		out, stderr, err := runCLI(t, args...)
		if ExitCode(err) != 5 {
			t.Errorf("%v: exit %d (%v), want 5", args, ExitCode(err), err)
		}
		var env struct {
			FetchFailures []fetchFailure `json:"fetch_failures"`
		}
		if json.Unmarshal([]byte(out), &env) != nil || len(env.FetchFailures) == 0 {
			t.Errorf("%v: JSON must still be emitted with fetch_failures, got %q", args, out)
		}
		if !strings.Contains(stderr, "No data: 1 of 1 cluster(s) unreachable") {
			t.Errorf("%v: stderr = %q", args, stderr)
		}
		if strings.Contains(stderr, " : ") {
			t.Errorf("%v: stray separator in %q", args, stderr)
		}
	}
	if _, _, err := runCLI(t, "restore-points", "104", "--json"); ExitCode(err) != 5 {
		t.Errorf("restore-points with no reachable cluster: exit %d, want 5", ExitCode(err))
	}
	// Human mode must not print an all-clear line.
	out, _, _ := runCLI(t, "backup", "coverage")
	if strings.Contains(out, "Every one of") || strings.Contains(out, "No uncovered guest") {
		t.Errorf("human output claims full coverage with no data: %q", out)
	}
}

func TestFleetPartiallyUnreachable(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PRINTING_PRESS_DOGFOOD", "1")
	t.Setenv("GOOD_PVE", "auditor@pve!cli=00000000-0000-0000-0000-000000000000")
	t.Setenv("GOOD_PBS", "auditor@pbs!cli:11111111-1111-1111-1111-111111111111")
	writeClustersFile(t, `
[clusters.good]
pve_base_url = "`+f.pve.URL+`/api2/json"
pve_token_env = "GOOD_PVE"
pve_fingerprint = "`+f.pveFingerprint()+`"
pbs_host = "`+f.pbs.URL+`"
pbs_token_env = "GOOD_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"

[clusters.down]
pve_base_url = "https://127.0.0.1:1/api2/json"
pve_token_env = "GOOD_PVE"
pbs_namespace = "down"
`)
	if _, _, err := runCLI(t, "posture", "--json", "--fail-on-red", "--only", "green"); ExitCode(err) != 1 {
		t.Errorf("--fail-on-red must trip when a cluster could not be read, got %v", err)
	}
	out, stderr, err := runCLI(t, "posture", "--json")
	if err != nil {
		t.Fatalf("partial failure must still exit 0: %v", err)
	}
	if !strings.Contains(stderr, "PARTIAL: 1 of 2 cluster(s)") {
		t.Errorf("stderr = %q", stderr)
	}
	rep := decodeJSON[postureReport](t, out)
	for _, r := range rep.Rows {
		if r.Cluster == "down" {
			t.Errorf("unreachable cluster produced a row: %+v", r)
		}
	}
	if len(rep.FetchFailures) != 1 || rep.FetchFailures[0].Cluster != "down" {
		t.Errorf("fetch_failures = %+v", rep.FetchFailures)
	}
	// A profile without pbs_host must not reach PBS even under the harness fallback.
	writeClustersFile(t, `
[clusters.nopbs]
pve_base_url = "`+f.pve.URL+`/api2/json"
pve_token_env = "GOOD_PVE"
pve_fingerprint = "`+f.pveFingerprint()+`"
`)
	out, stderr, err = runCLI(t, "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if rep := decodeJSON[postureReport](t, out); len(rep.FetchFailures) != 0 {
		t.Errorf("profile without PBS attempted PBS calls: %+v (stderr %s)", rep.FetchFailures, stderr)
	}
}
