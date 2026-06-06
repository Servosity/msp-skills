// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestEmailDraftCommandShape(t *testing.T) {
	flags := &rootFlags{}
	cmd := newNovelEmailDraftCmd(flags)
	if cmd.Use != "email-draft" {
		t.Fatalf("Use = %q", cmd.Use)
	}
	if cmd.Annotations["mcp:read-only"] != "true" {
		t.Error("email-draft must be marked mcp:read-only (store-only reads, print-only output)")
	}
	for _, f := range []string{"stale", "days", "engine", "db"} {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("missing flag --%s", f)
		}
	}
}
