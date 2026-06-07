// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import "testing"

func TestNovelCoverageGapsWiring(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newNovelCoverageGapsCmd(flags)
	assertFlags(t, cmd, "db", "type")
	assertWould(t, runNovelDry(t, cmd, nil))
}

func TestNovelCoverageGapsRejectsBadType(t *testing.T) {
	flags := &rootFlags{}
	cmd := newNovelCoverageGapsCmd(flags)
	_ = cmd.Flags().Set("type", "exotic")
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatalf("invalid --type must be a usage error")
	}
}
