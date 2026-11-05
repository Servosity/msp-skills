// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

func TestZoomMCPHardeningAnnotations(t *testing.T) {
	root := RootCmd()
	for path, flagNames := range zoomMCPFileFlags {
		cmd := zoomFindExact(root, path)
		if cmd == nil {
			t.Fatalf("%s not found", path)
		}
		for _, name := range strings.Split(flagNames, ",") {
			if cmd.Flags().Lookup(name) == nil {
				t.Fatalf("%s has no --%s flag; update zoomMCPFileFlags", path, name)
			}
			if !strings.Contains(cmd.Annotations["mcp:write-flags"], name) {
				t.Fatalf("%s: --%s not blocked for MCP (annotations %v)", path, name, cmd.Annotations)
			}
		}
	}
	for _, path := range zoomSecretReadCommands {
		cmd := zoomFindExact(root, path)
		if cmd == nil {
			t.Fatalf("%s not found", path)
		}
		if cmd.Annotations["mcp:read-only"] == "true" {
			t.Fatalf("%s returns secrets but is still mcp:read-only", path)
		}
	}
	if cmd := zoomFindExact(root, "devices list-phone"); cmd == nil || cmd.Annotations["mcp:read-only"] != "true" {
		t.Fatal("ordinary reads must stay read-only")
	}
}
