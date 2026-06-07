// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import "testing"

func TestNovelFleetHealthWiring(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newNovelFleetHealthCmd(flags)
	if cmd.Use != "fleet-health" {
		t.Fatalf("Use=%q", cmd.Use)
	}
	assertFlags(t, cmd, "db", "flag-gaps")
	if cmd.Annotations["mcp:read-only"] != "true" {
		t.Errorf("fleet-health is a pure read; must be mcp:read-only")
	}
	assertWould(t, runNovelDry(t, cmd, nil))
}

func TestNovelFleetHealthNoStoreGuidance(t *testing.T) {
	flags := &rootFlags{asJSON: true}
	cmd := newNovelFleetHealthCmd(flags)
	_ = cmd.Flags().Set("db", t.TempDir()+"/empty.db")
	var err error
	func() {
		defer func() { _ = recover() }()
		err = cmd.RunE(cmd, nil)
	}()
	if err == nil || !contains(err.Error(), "fleet-sync") {
		t.Errorf("empty store should point the user at fleet-sync, got: %v", err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
