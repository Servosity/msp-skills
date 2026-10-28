// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "client-profile-inert" entry. External test package so it can
// import internal/cli.

package cobratree_test

import (
	"testing"

	"dnsfilter-pp-cli/internal/cli"
	"dnsfilter-pp-cli/internal/mcp/cobratree"
	"github.com/mark3labs/mcp-go/server"
)

// TestMirroredToolsOmitInertClientProfile: no tenant-gated platform source is
// registered for DNSFilter, so --client-profile changes nothing; offering it
// in a tool schema would let an agent believe it had scoped itself.
func TestMirroredToolsOmitInertClientProfile(t *testing.T) {
	s := server.NewMCPServer("client-profile", "test")
	cobratree.RegisterAll(s, cli.RootCmd(), cobratree.SiblingCLIPath)
	if len(s.ListTools()) == 0 {
		t.Fatal("no mirrored tools registered")
	}
	for name, entry := range s.ListTools() {
		if _, ok := entry.Tool.InputSchema.Properties["client-profile"]; ok {
			t.Errorf("tool %s offers the inert client-profile flag", name)
		}
	}
}
