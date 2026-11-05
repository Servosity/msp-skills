// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"appdirect-pp-cli/internal/cliutil/testenv"
)

// No registered tool can change AppDirect data without confirm (appdirect handfixes:
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
	t.Setenv("APPDIRECT_HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	s := server.NewMCPServer("mirror-confirm", "test")
	RegisterTools(s)
	tools := s.ListTools()
	if len(tools) < 30 { // appdirect: Cloudflare pattern (search, read, get, execute, intents, command mirrors)
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
	// Every AppDirect write: the Cloudflare-pattern execute tool (every
	// state-changing endpoint of the 329) and import (API create/upsert calls).
	for _, name := range []string{"appdirect_execute", "import"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("write tool %s not registered", name)
		}
		if ro := entry.Tool.Annotations.ReadOnlyHint; ro != nil && *ro {
			t.Errorf("%s: an AppDirect write must not be marked read-only", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; !gated {
			t.Errorf("%s: an AppDirect write must carry the confirm gate", name)
		}
	}
	// The registry and read tools, the local-store queries and the six novel
	// local-store views (and their parent groups) read only.
	for _, name := range []string{"appdirect_search", "appdirect_get", "appdirect_read", "search", "sql", "context",
		"reconcile", "subs_changed", "payments_unpaid", "pipeline", "pipeline_stale", "company_show",
		"subs", "payments", "company", "workflow_status", "learnings_list", "learnings_stats", "learnings_candidates", "recall", "playbook_list", "tail", "analytics"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		if ro := entry.Tool.Annotations.ReadOnlyHint; ro == nil || !*ro {
			t.Errorf("%s: an AppDirect read must be marked read-only", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; gated {
			t.Errorf("%s: read tool should not need confirm", name)
		}
	}
}
