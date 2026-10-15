// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestVerifyHealthSpread(t *testing.T) {
	newFixture(t)
	out, stderr, err := runCLI(t, "verify-health", "--all", "--json")
	if err != nil {
		t.Fatalf("verify-health: %v\n%s", err, stderr)
	}
	rep := decodeJSON[verifyReport](t, out)
	if len(rep.Stores) != 1 || rep.Stores[0].Store != "main" {
		t.Fatalf("stores = %+v", rep.Stores)
	}
	s := rep.Stores[0]
	if s.FailedSnapshots != 2 || s.FailingGroups != 2 || s.GCStillBadChunks != 3 || s.NeverVerified != 2 {
		t.Errorf("store rollup = %+v", s)
	}
	if len(s.UnreachedNS) != 0 {
		t.Errorf("root namespace is reached by job v1, got unreached %v", s.UnreachedNS)
	}
	g := map[string]verifyGroupRow{}
	for _, r := range rep.Groups {
		g[r.Group] = r
	}
	if r := g["vm/104"]; r.Status != "failing" || !r.NewestIsBad || r.NewestVerifiedOKAt == "" {
		t.Errorf("vm/104 = %+v", r)
	}
	if r := g["vm/100"]; r.Status != "failing" || r.NewestIsBad || r.Failed != 1 {
		t.Errorf("vm/100 (old failure, newer good points) = %+v", r)
	}
	if r := g["vm/101"]; r.Status != "outdated" || r.Outdated != 1 {
		t.Errorf("vm/101 verified 40 days ago must be outdated (job outdated-after 30): %+v", r)
	}
	if r := g["ct/107"]; r.Status != "unverified" || r.NeverVerified != 1 {
		t.Errorf("ct/107 = %+v", r)
	}
	if s.OutdatedSnapshots != 1 || s.VerifyJobsUnknown || s.GCUnknown {
		t.Errorf("store outdated/unknown flags = %+v", s)
	}
	// Default view hides healthy groups.
	out, _, err = runCLI(t, "verify-health", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeJSON[verifyReport](t, out).Groups {
		if r.Status == "ok" {
			t.Errorf("healthy group %s listed without --all", r.Group)
		}
	}
}

func TestVerifyJobReach(t *testing.T) {
	depth1 := 1
	j := verifyJobDef{NS: "clients", MaxDepth: &depth1}
	for ns, want := range map[string]bool{"clients": true, "clients/acme": true, "clients/acme/old": false, "other": false, "": false} {
		if got := j.reaches(ns); got != want {
			t.Errorf("reaches(%q) = %v, want %v", ns, got, want)
		}
	}
	root := verifyJobDef{}
	if !root.reaches("") || !root.reaches("a/b/c") {
		t.Error("root job with default depth must reach nested namespaces")
	}
	if got := upidStartTime("UPID:pve1:00001234:00ABCDEF:6500AB12:vzdump::root@pam:"); got != 0x6500AB12 {
		t.Errorf("PVE upid start = %d", got)
	}
	if got := upidStartTime("UPID:pbs:000003E8:00000064:00000001:6500AB12:verificationjob:main\\x3av1:root@pam:"); got != 0x6500AB12 {
		t.Errorf("PBS upid start = %d", got)
	}
	_ = time.Now
}
