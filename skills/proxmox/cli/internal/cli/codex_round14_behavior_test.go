// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

// Codex round 14: a PBS storage read only through Proxmox VE is listed with
// the PBS credential saved in it, which may not see every backup group. Its
// points are real; their absence proves nothing.
func TestPVEOnlyPBSListingProvesNoAbsence(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PROXMOX_PBS_HOST", "")
	rep, rows := postureByVMID(t)
	if len(rep.Warnings) == 0 || !strings.Contains(strings.Join(rep.Warnings, " "), "read only through Proxmox VE") {
		t.Errorf("want the PVE-only PBS listing named in warnings: %+v", rep.Warnings)
	}
	fresh, never := 0, 0
	for _, r := range rows {
		switch r.Verdict {
		case "fresh":
			fresh++
		case "never":
			never++
		}
	}
	if fresh == 0 || never > 0 {
		t.Errorf("fresh points stand, absence is unproven: fresh=%d never=%d", fresh, never)
	}
	_ = f
	out, _, _ := runCLI(t, "restore-points", "100", "--json")
	for _, g := range decodeJSON[restorePointsReport](t, out).Guests {
		if g.Evidence == "full" {
			t.Errorf("restore-points claims full evidence from a PVE-only PBS listing:\n%s", out)
		}
	}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--json")
	for _, r := range decodeJSON[rpoReport](t, out).Rows {
		if r.Evidence == "full" && r.MissedRuns > 0 {
			t.Errorf("rpo proves misses from a PVE-only PBS listing: %+v", r)
		}
	}
}
