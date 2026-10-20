// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cli"
	"github.com/mark3labs/mcp-go/server"
)

// hand-fix guard (handfixes.json: client-profile-inert): no tenant-gated
// profile source is registered on this connector, so --client-profile changes
// nothing. It must say so in --help and stay out of every MCP tool schema,
// where an agent would otherwise believe it can pick a customer with it.
func TestClientProfileIsInertAndHiddenFromMCP(t *testing.T) {
	if f := cli.RootCmd().PersistentFlags().Lookup("client-profile"); f == nil || !strings.HasPrefix(f.Usage, "Unused by this connector") {
		t.Fatalf("--client-profile help must say it is unused here, got %+v", f)
	}
	s := server.NewMCPServer("eset-protect", "test")
	RegisterTools(s)
	for name, tool := range s.ListTools() {
		raw, _ := json.Marshal(tool.Tool.InputSchema)
		if strings.Contains(string(raw), `"client-profile"`) {
			t.Fatalf("tool %s exposes client-profile: %s", name, raw)
		}
	}
}
