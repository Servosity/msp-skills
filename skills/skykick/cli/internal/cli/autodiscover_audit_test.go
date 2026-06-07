// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import "testing"

func TestNovelAutodiscoverAuditWiring(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newNovelAutodiscoverAuditCmd(flags)
	assertFlags(t, cmd, "db", "only-off")
	if cmd.Annotations["mcp:read-only"] != "true" {
		t.Errorf("autodiscover-audit is a pure read; must be mcp:read-only")
	}
	assertWould(t, runNovelDry(t, cmd, nil))
}
