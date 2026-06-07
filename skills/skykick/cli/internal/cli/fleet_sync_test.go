// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import "testing"

func TestNovelFleetSyncWiring(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newNovelFleetSyncCmd(flags)
	if cmd.Use != "fleet-sync" {
		t.Fatalf("Use=%q", cmd.Use)
	}
	assertFlags(t, cmd, "db", "limit", "workers", "top-alerts", "skip")
	if cmd.Annotations["mcp:read-only"] != "true" {
		t.Errorf("fleet-sync only writes the local cache; must be mcp:read-only")
	}
	assertWould(t, runNovelDry(t, cmd, nil))
}
