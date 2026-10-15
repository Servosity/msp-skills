// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"slices"
	"testing"
)

func TestCapacityRunway(t *testing.T) {
	newFixture(t)
	out, stderr, err := runCLI(t, "capacity", "--json")
	if err != nil {
		t.Fatalf("capacity: %v\n%s", err, stderr)
	}
	rep := decodeJSON[capacityReport](t, out)
	if len(rep.Rows) != 2 {
		t.Fatalf("want PBS datastore + local dir storage (PBS-backed pbs1 folded into the datastore row), got %+v", rep.Rows)
	}
	pbs := rep.Rows[0]
	if pbs.Kind != "pbs-datastore" || pbs.Name != "main" || pbs.DaysToFull == nil || *pbs.DaysToFull < 2.9 || *pbs.DaysToFull > 3.1 {
		t.Fatalf("first row must be the datastore that fills in ~3 days: %+v", pbs)
	}
	// 2% free, GC failed.
	for _, flag := range []string{"nearly-full", "gc-deadlock-risk"} {
		if !slices.Contains(pbs.Flags, flag) {
			t.Errorf("missing flag %s: %v", flag, pbs.Flags)
		}
	}
	if pbs.UsedPct != 98 {
		t.Errorf("used pct = %v", pbs.UsedPct)
	}
	local := rep.Rows[1]
	if local.Kind != "pve-storage" || local.Name != "local" || local.DaysToFull != nil || !slices.Equal(local.Flags, []string{"no-forecast"}) {
		t.Errorf("local storage row = %+v", local)
	}
	// --within keeps filling rows, and PVE storage it cannot forecast.
	out, _, err = runCLI(t, "capacity", "--within", "30d", "--json")
	if err != nil {
		t.Fatal(err)
	}
	rep = decodeJSON[capacityReport](t, out)
	if len(rep.Rows) != 2 || !slices.Contains(rep.Rows[0].Flags, "filling") || !slices.Contains(rep.Rows[1].Flags, "no-forecast") {
		t.Errorf("--within 30d = %+v", rep.Rows)
	}
	// A generous margin does not change GC semantics; a zero margin drops the deadlock flag.
	out, _, err = runCLI(t, "capacity", "--gc-margin", "0", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if rows := decodeJSON[capacityReport](t, out).Rows; slices.Contains(rows[0].Flags, "gc-deadlock-risk") {
		t.Errorf("gc-margin 0 must not flag deadlock: %v", rows[0].Flags)
	}
	if _, _, err := runCLI(t, "capacity", "--gc-margin", "150%", "--json"); ExitCode(err) != 2 {
		t.Errorf("bad --gc-margin must exit 2, got %v", err)
	}
}
