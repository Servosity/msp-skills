// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestFleetHealthCommandShape(t *testing.T) {
	flags := &rootFlags{}
	cmd := newNovelFleetHealthCmd(flags)
	if cmd.Use != "fleet-health" {
		t.Fatalf("Use = %q", cmd.Use)
	}
	if cmd.Annotations["mcp:read-only"] != "true" {
		t.Error("fleet-health must be marked mcp:read-only (store-only reads)")
	}
	for _, f := range []string{"stale-days", "db"} {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("missing flag --%s", f)
		}
	}
}
