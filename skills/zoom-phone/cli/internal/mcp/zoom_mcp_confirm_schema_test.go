// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"zoom-phone-pp-cli/internal/cliutil"
)

// zoom-phone hand-fix: zoom-phone-mcp-write-confirm. The server process (no
// ZOOM_PHONE_MCP_CHILD) builds the mirrored offboard and fleet tools with a
// boolean confirm property; the child's strict --confirm only works because
// the server turns the JSON boolean true into a bare --confirm.
func TestZoomMCPConfirmPropertyIsBoolean(t *testing.T) {
	t.Setenv(cliutil.MCPChildEnvVar, "")
	s := server.NewMCPServer("zoom-confirm-schema", "test")
	RegisterTools(s)
	found := 0
	for name, entry := range s.ListTools() {
		if entry == nil {
			continue
		}
		prop, ok := entry.Tool.InputSchema.Properties["confirm"]
		if !ok {
			continue
		}
		raw, _ := json.Marshal(prop)
		var typed struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(raw, &typed)
		if typed.Type != "boolean" {
			t.Errorf("tool %s: confirm schema %s; want boolean", name, raw)
		}
		found++
	}
	if found < 2 {
		t.Fatalf("confirm property on %d tools; want at least offboard and fleet", found)
	}
}
