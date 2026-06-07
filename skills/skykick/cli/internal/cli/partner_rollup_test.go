// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import "testing"

func TestNovelPartnerRollupWiring(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newNovelPartnerRollupCmd(flags)
	assertFlags(t, cmd, "db")
	if cmd.Annotations["mcp:read-only"] != "true" {
		t.Errorf("partner-rollup is a pure read; must be mcp:read-only")
	}
	assertWould(t, runNovelDry(t, cmd, nil))
}
