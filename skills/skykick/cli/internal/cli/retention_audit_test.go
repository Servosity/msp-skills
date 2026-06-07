// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import "testing"

func TestNovelRetentionAuditWiring(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newNovelRetentionAuditCmd(flags)
	assertFlags(t, cmd, "db", "floor-days")
	if got := cmd.Flags().Lookup("floor-days").DefValue; got != "365" {
		t.Errorf("default --floor-days=%s want 365", got)
	}
	assertWould(t, runNovelDry(t, cmd, nil))
}

func TestNovelRetentionAuditRejectsBadFloor(t *testing.T) {
	flags := &rootFlags{}
	cmd := newNovelRetentionAuditCmd(flags)
	_ = cmd.Flags().Set("floor-days", "0")
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatalf("zero --floor-days must be a usage error")
	}
}
