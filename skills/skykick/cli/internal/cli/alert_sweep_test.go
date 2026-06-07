// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import "testing"

func TestNovelAlertSweepWiring(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newNovelAlertSweepCmd(flags)
	assertFlags(t, cmd, "db", "limit", "complete", "apply")
	if cmd.Annotations["mcp:read-only"] == "true" {
		t.Errorf("alert-sweep can mutate upstream with --complete --apply; must NOT be read-only")
	}
	out := runNovelDry(t, cmd, nil)
	assertWould(t, out)
}
