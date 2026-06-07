// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import "testing"

func TestNovelDriftWiring(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newNovelDriftCmd(flags)
	assertFlags(t, cmd, "db", "stale-hours")
	if cmd.Annotations["mcp:read-only"] != "true" {
		t.Errorf("drift is a pure read; must be mcp:read-only")
	}
	assertWould(t, runNovelDry(t, cmd, nil))
}
