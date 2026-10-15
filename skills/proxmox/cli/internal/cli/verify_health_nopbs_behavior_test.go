// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

// With no PBS host on any cluster, verify-health checked nothing. It must not
// exit 0, or a script reading only the exit code takes it for "no failures".
func TestVerifyHealthNoPBSAnywhere(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PRINTING_PRESS_DOGFOOD", "1")
	t.Setenv("GOOD_PVE", "auditor@pve!cli=00000000-0000-0000-0000-000000000000")
	t.Setenv("GOOD_PBS", "auditor@pbs!cli:11111111-1111-1111-1111-111111111111")
	writeClustersFile(t, `
[clusters.nopbs]
pve_base_url = "`+f.pve.URL+`/api2/json"
pve_token_env = "GOOD_PVE"
pve_fingerprint = "`+f.pveFingerprint()+`"
`)
	for _, mode := range []string{"--json", "--csv", "--agent", "--human-friendly"} {
		out, stderr, err := runCLI(t, "verify-health", mode)
		if ExitCode(err) != 5 {
			t.Errorf("%s: exit %d (%v), want 5", mode, ExitCode(err), err)
		}
		if !strings.Contains(stderr, "No data: 0 of 1 cluster(s) have a PBS host configured") {
			t.Errorf("%s: stderr = %q", mode, stderr)
		}
		switch mode {
		case "--json":
			rep := decodeJSON[verifyReport](t, out)
			if len(rep.Skipped) != 1 || len(rep.Groups) != 0 || len(rep.Stores) != 0 {
				t.Errorf("JSON must still be emitted with skipped_clusters: %+v", rep)
			}
		case "--human-friendly":
			if strings.TrimSpace(out) != "" {
				t.Errorf("human mode must print no result line when nothing was checked: %q", out)
			}
		}
	}

	// Partial: one cluster with PBS is checked, the other is skipped, exit 0.
	writeClustersFile(t, `
[clusters.good]
pve_base_url = "`+f.pve.URL+`/api2/json"
pve_token_env = "GOOD_PVE"
pve_fingerprint = "`+f.pveFingerprint()+`"
pbs_host = "`+f.pbs.URL+`"
pbs_token_env = "GOOD_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"

[clusters.nopbs]
pve_base_url = "`+f.pve.URL+`/api2/json"
pve_token_env = "GOOD_PVE"
pve_fingerprint = "`+f.pveFingerprint()+`"
`)
	out, stderr, err := runCLI(t, "verify-health", "--all", "--json")
	if err != nil {
		t.Fatalf("partial PBS coverage must exit 0: %v\n%s", err, stderr)
	}
	rep := decodeJSON[verifyReport](t, out)
	if len(rep.Stores) != 1 || len(rep.Skipped) != 1 || !strings.HasPrefix(rep.Skipped[0], "nopbs:") {
		t.Errorf("stores = %+v, skipped = %v", rep.Stores, rep.Skipped)
	}
	if strings.Contains(stderr, "No data") {
		t.Errorf("partial run must not report No data: %q", stderr)
	}
}
