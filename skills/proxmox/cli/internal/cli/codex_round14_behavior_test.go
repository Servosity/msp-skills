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
	// Fresh points count, but none reads green: a newer snapshot that failed
	// verification may be hidden from the saved credential (Codex round 50).
	incomplete, green, never := 0, 0, 0
	for _, r := range rows {
		switch {
		case r.Verdict == "archives-incomplete":
			incomplete++
		case r.Severity == "green" && r.CoveredByJob:
			green++
		case r.Verdict == "never":
			never++
		}
	}
	if incomplete == 0 || green > 0 || never > 0 {
		t.Errorf("fresh points count but are not green, absence is unproven: archives-incomplete=%d green=%d never=%d", incomplete, green, never)
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
