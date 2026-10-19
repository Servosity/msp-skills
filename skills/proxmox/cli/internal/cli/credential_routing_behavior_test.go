// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

// clusters add chooses where a token is sent and which pin vouches for that
// host, so it must never be an MCP tool (the walker skips mcp:hidden).
func TestClustersAddIsNotAnMCPTool(t *testing.T) {
	cmd, _, err := RootCmd().Find([]string{"clusters", "add"})
	if err != nil || cmd.Name() != "add" {
		t.Fatalf("clusters add not found: %v", err)
	}
	if cmd.Annotations["mcp:hidden"] != "true" {
		t.Fatalf("clusters add annotations = %v, want mcp:hidden", cmd.Annotations)
	}
}

// --path-context runs after the client binds tokens and pins to the configured
// hosts, so it must not re-point host or pbs_host.
func TestPathContextCannotRepointTheHost(t *testing.T) {
	newFixture(t)
	for _, kv := range []string{"host=attacker.example.com", "pbs_host=attacker.example.com:8007"} {
		_, _, err := runCLI(t, "sync", "--resources", "cluster-resources", "--path-context", kv, "--dry-run")
		if err == nil || !strings.Contains(err.Error(), "cannot change the Proxmox host") {
			t.Errorf("--path-context %s: err = %v, want a refusal", kv, err)
		}
	}
}
