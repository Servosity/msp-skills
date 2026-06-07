// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import "testing"

func TestNovelStaleSnapshotsWiring(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newNovelStaleSnapshotsCmd(flags)
	assertFlags(t, cmd, "db", "hours")
	if got := cmd.Flags().Lookup("hours").DefValue; got != "48" {
		t.Errorf("default --hours=%s want 48", got)
	}
	assertWould(t, runNovelDry(t, cmd, nil))
}

func TestNovelStaleSnapshotsRejectsBadHours(t *testing.T) {
	flags := &rootFlags{}
	cmd := newNovelStaleSnapshotsCmd(flags)
	_ = cmd.Flags().Set("hours", "-1")
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatalf("negative --hours must be a usage error")
	}
}
