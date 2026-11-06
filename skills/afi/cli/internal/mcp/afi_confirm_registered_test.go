// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// No registered tool can change Afi data without confirm (afi handfixes:
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
	s := server.NewMCPServer("mirror-confirm", "test")
	RegisterTools(s)
	tools := s.ListTools()
	if len(tools) < 40 {
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
	// Every Afi write: the five typed endpoint writes and the offboard novel
	// (it triggers a backup and releases a protection). import is hidden from
	// MCP (afi handfix mcp-import-hidden).
	if _, ok := tools["import"]; ok {
		t.Error("import is offered over MCP; its required --input names a host file")
	}
	for _, name := range []string{"orgs_create", "tenants_archives_delete", "tenants_jobs_tasks-trigger",
		"tenants_resources_protections-protect", "tenants_resources_protections-unprotect", "offboard"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("write tool %s not registered", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; !gated {
			t.Errorf("%s: an Afi write must carry the confirm gate", name)
		}
	}
	for _, name := range []string{"applications_list-installations", "orgs_get", "orgs_licensing_get-subscription-history",
		"orgs_licensing_list-subscriptions", "orgs_orgs_list", "orgs_tenants_get-org", "tenants_get", "tenants_archives_list",
		"tenants_audit_list-events", "tenants_policies_list", "tenants_protections_list", "tenants_quotas_tenants-get-tenant",
		"tenants_resources_get", "tenants_resources_get-stats", "tenants_resources_list", "tenants_tasks_get",
		"tenants_tasks_get-tenant-stats", "backup_stale", "coverage_gaps", "fleet_health", "reconcile_licenses", "resolve",
		"tenant_scorecard", "search", "sql"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		if ro := entry.Tool.Annotations.ReadOnlyHint; ro == nil || !*ro {
			t.Errorf("%s: an Afi read must be marked read-only", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; gated {
			t.Errorf("%s: read tool should not need confirm", name)
		}
	}
}
