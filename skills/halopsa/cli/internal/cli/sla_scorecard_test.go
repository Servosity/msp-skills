// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

func TestNovelSlaScorecardCmdShape(t *testing.T) {
	flags := &rootFlags{}
	cmd := newNovelSlaScorecardCmd(flags)
	if cmd.Name() != "scorecard" {
		t.Fatalf("Name = %q, want %q", cmd.Name(), "scorecard")
	}
	if cmd.Annotations["mcp:read-only"] != "true" {
		t.Error("must be mcp:read-only (store-only reads)")
	}
	for _, f := range strings.Split("since,by,db", ",") {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("missing flag --%s", f)
		}
	}
}
