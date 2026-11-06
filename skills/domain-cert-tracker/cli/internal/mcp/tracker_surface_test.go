// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// The MCP surface must not offer a host-file read (clients import), a way to
// widen the port policy (--allow-port), or the --yes bypass.
func TestTrackerMCPSurfaceHasNoCLIOnlyCapabilities(t *testing.T) {
	s := server.NewMCPServer("domain-cert-tracker", "test")
	RegisterTools(s)
	tools := s.ListTools()
	if _, ok := tools["clients_import"]; ok {
		t.Fatal("clients import (a host-file read) is exposed over MCP")
	}
	for _, name := range []string{"tls_probe", "sweep", "clients_remove", "clients_set", "events_list", "report", "dns_check"} {
		tool, ok := tools[name]
		if !ok {
			t.Fatalf("tool %s missing; registered: %v", name, keys(tools))
		}
		for _, banned := range []string{"allow-port", "yes", "db", "home", "config"} {
			if _, ok := tool.Tool.InputSchema.Properties[banned]; ok {
				t.Errorf("tool %s exposes --%s", name, banned)
			}
		}
	}
	for _, name := range []string{"clients_remove", "clients_set"} {
		if _, ok := tools[name].Tool.InputSchema.Properties["confirm"]; !ok {
			t.Errorf("%s lacks the confirm argument", name)
		}
	}
	for _, name := range []string{"events_list", "report", "tls_probe", "dns_check"} {
		if h := tools[name].Tool.Annotations.ReadOnlyHint; h == nil || !*h {
			t.Errorf("%s is not marked read-only", name)
		}
	}
}

func keys(m map[string]*server.ServerTool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
