// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

func TestNovelTicketsReopensCmdShape(t *testing.T) {
	flags := &rootFlags{}
	cmd := newNovelTicketsReopensCmd(flags)
	if cmd.Name() != "reopens" {
		t.Fatalf("Name = %q, want %q", cmd.Name(), "reopens")
	}
	if cmd.Annotations["mcp:read-only"] != "true" {
		t.Error("must be mcp:read-only (store-only reads)")
	}
	for _, f := range strings.Split("since,db", ",") {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("missing flag --%s", f)
		}
	}
}
