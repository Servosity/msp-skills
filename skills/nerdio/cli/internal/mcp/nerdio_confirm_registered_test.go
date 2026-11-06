// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"nerdio-pp-cli/internal/cliutil/testenv"
)

// No registered tool can change Nerdio data without confirm (nerdio handfixes:
// mcp-endpoint-write-confirm, mcp-mirror-write-confirm). A tool must either be
// read-only, write only the CLI's own files (local store, learnings,
// playbooks: listed here as local), or carry the confirm gate, or this test
// fails; so an endpoint or command a later print adds is covered too.
func TestRegisteredMirroredWritesNeedConfirm(t *testing.T) {
	local := map[string]bool{
		"export": true, "sync": true, "workflow_archive": true, "learnings_forget": true, "learnings_reject": true,
		"learnings_confirm": true, "playbook_amend": true, "teach": true, "teach_lookup": true, "teach_pattern": true,
		"teach_playbook": true,
	}
	home := testenv.Isolate(t)
	t.Setenv("NERDIO_HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	s := server.NewMCPServer("mirror-confirm", "test")
	RegisterTools(s)
	tools := s.ListTools()
	if len(tools) < 30 { // nerdio: Cloudflare pattern (search, read, get, execute, command mirrors)
		t.Fatalf("only %d tools registered", len(tools))
	}
	for name, entry := range tools {
		if _, ok := entry.Tool.InputSchema.Properties["yes"]; ok {
			t.Errorf("%s: --yes must never be a tool argument", name)
		}
		if ro := entry.Tool.Annotations.ReadOnlyHint; ro != nil && *ro {
			continue
		}
		if local[name] {
			continue
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; !gated {
			t.Errorf("%s may change state and has no confirm gate", name)
		}
	}
	// Every Nerdio write: the Cloudflare-pattern execute tool (every
	// state-changing endpoint), import (API create/upsert calls) and the
	// mirrored scripted-actions fan-run (executes a script in many accounts).
	for _, name := range []string{"nerdio_execute", "import", "scripted_actions_fan_run"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("write tool %s not registered", name)
		}
		if ro := entry.Tool.Annotations.ReadOnlyHint; ro != nil && *ro {
			t.Errorf("%s: a Nerdio write must not be marked read-only", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; !gated {
			t.Errorf("%s: a Nerdio write must carry the confirm gate", name)
		}
	}
	// The registry and read tools, the local-store queries and the read-only
	// novels (the fleet fan-outs, usages drift, job wait) and their parent
	// group read only.
	for _, name := range []string{"nerdio_search", "nerdio_get", "nerdio_read", "search", "sql", "context",
		"fleet", "fleet_autoscale_audit", "fleet_host_estate", "fleet_billing_rollup", "usages_drift", "job_wait",
		"workflow_status", "learnings_list", "learnings_stats", "learnings_candidates", "recall", "playbook_list", "tail", "analytics"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		if ro := entry.Tool.Annotations.ReadOnlyHint; ro == nil || !*ro {
			t.Errorf("%s: a Nerdio read must be marked read-only", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; gated {
			t.Errorf("%s: read tool should not need confirm", name)
		}
	}
	// fleet autoscale-audit --baseline reads a local JSON file (mcp:read-flags):
	// the tool offers no baseline argument over MCP.
	if _, ok := tools["fleet_autoscale_audit"].Tool.InputSchema.Properties["baseline"]; ok {
		t.Errorf("fleet_autoscale_audit offers a baseline argument (a local file read) over MCP")
	}
	if _, ok := tools["fleet_autoscale_audit"].Tool.InputSchema.Properties["accounts"]; !ok {
		t.Errorf("fleet_autoscale_audit lists no accounts argument; the schema check above reads the wrong tool")
	}
}
