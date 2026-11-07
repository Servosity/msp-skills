// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"datto-rmm-pp-cli/internal/cliutil/testenv"
)

// No registered tool can change Datto data without confirm (datto-rmm handfixes:
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
	t.Setenv("DATTO_RMM_HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	s := server.NewMCPServer("mirror-confirm", "test")
	RegisterTools(s)
	tools := s.ListTools()
	if len(tools) < 30 { // datto: Cloudflare pattern (search, read, get, execute, intents, command mirrors)
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
	// Every Datto write: the Cloudflare-pattern execute tool (every
	// state-changing endpoint of the 57: site create and update, variables,
	// quick job, move device, UDF, warranty, alert mute/resolve/unmute,
	// resetApiKeys), import (API create/upsert calls), the fleet
	// resolve-storm mirror (POST /v2/alert/<uid>/resolve under --confirm) and
	// the qbr intent.
	for _, name := range []string{"datto-rmm_execute", "import", "fleet_resolve_storm", "qbr_prep_for_one_customer"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("write tool %s not registered", name)
		}
		if ro := entry.Tool.Annotations.ReadOnlyHint; ro != nil && *ro {
			t.Errorf("%s: a Datto write must not be marked read-only", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; !gated {
			t.Errorf("%s: a Datto write must carry the confirm gate", name)
		}
	}
	// The registry and read tools, the local-store queries and the fleet
	// novels that only read (the local store, or job-results live) read only.
	for _, name := range []string{"datto-rmm_search", "datto-rmm_get", "datto-rmm_read", "search", "sql", "context",
		"fleet", "fleet_stale", "fleet_storms", "fleet_patch_gaps", "fleet_av_gaps", "fleet_sprawl", "fleet_warranty", "fleet_scorecard",
		"fleet_agent_drift", "fleet_orphans", "fleet_snapshot", "fleet_diff", "fleet_device_timeline", "fleet_sync_gaps", "fleet_job_results",
		"workflow_status", "learnings_list", "learnings_stats", "learnings_candidates", "recall", "playbook_list", "tail", "analytics"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		if ro := entry.Tool.Annotations.ReadOnlyHint; ro == nil || !*ro {
			t.Errorf("%s: a Datto read must be marked read-only", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; gated {
			t.Errorf("%s: read tool should not need confirm", name)
		}
	}
}
