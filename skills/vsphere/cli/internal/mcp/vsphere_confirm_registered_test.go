// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// provenReads is every registered tool that may run without confirm: a GET
// of vCenter, a read of the local store, or a read of the CLI's own learnings
// (vsphere handfix: mcp-mirror-write-confirm). It is an allowlist: a tool a
// later print adds is unclassified until it is reviewed and named here, and an
// unclassified tool must carry the confirm gate or this test fails.
var provenReads = map[string]bool{
	"appliance_version": true, "cluster_get": true, "cluster_list": true, "context": true,
	"datacenter_get": true, "datacenter_list": true, "datastore_get": true, "datastore_list": true,
	"folder_list": true, "host_list": true, "learnings_candidates": true, "learnings_list": true,
	"learnings_stats": true, "licence": true, "licence_exposure": true, "migration": true,
	"migration_blockers": true, "migration_capacity": true, "migration_proxmox_map": true,
	"network_list": true, "playbook_list": true, "recall": true, "resource_pool_get": true,
	"resource_pool_list": true, "rvtools": true, "rvtools_export": true, "search": true,
	"session_get": true, "snapshots": true, "snapshots_audit": true, "sql": true,
	"tagging_association_list": true, "tagging_category_get": true, "tagging_category_list": true,
	"tagging_tag_get": true, "tagging_tag_list": true, "vi_datastore_summary": true,
	"vi_host_summary": true, "vi_service_instance_content": true, "vi_vm_layout-ex": true,
	"vi_vm_snapshot": true, "vi_vm_storage": true, "vi_vm_summary": true, "vm_get": true,
	"vm_guest_identity": true, "vm_guest_local-filesystem": true, "vm_guest_network-interfaces": true,
	"vm_hardware_boot": true, "vm_hardware_cdroms": true, "vm_hardware_cpu": true,
	"vm_hardware_disk": true, "vm_hardware_disks": true, "vm_hardware_get": true,
	"vm_hardware_memory": true, "vm_hardware_nic": true, "vm_hardware_nics": true,
	"vm_hardware_nvme-adapters": true, "vm_hardware_sata-adapters": true,
	"vm_hardware_scsi-adapters": true, "vm_list": true, "vm_power_get": true, "vm_tools_get": true,
	"workflow_status": true,
}

// No registered tool can change vCenter state, and every tool that writes
// anything (vCenter, the local store, learnings, playbooks, an export) needs
// confirm boolean true (vsphere handfix: mcp-mirror-write-confirm; Codex r1
// class mcp-write-without-confirm). Fail closed: a tool is ungated only when
// it is marked read-only AND named in provenReads; every other tool,
// including one a later print adds, must carry the confirm gate.
func TestRegisteredMirroredWritesNeedConfirm(t *testing.T) {
	s := server.NewMCPServer("mirror-confirm", "test")
	RegisterTools(s)
	tools := s.ListTools()
	if len(tools) < 40 {
		t.Fatalf("only %d tools registered", len(tools))
	}
	var gatedCount int
	for name, entry := range tools {
		if _, ok := entry.Tool.InputSchema.Properties["yes"]; ok {
			t.Errorf("%s: --yes must never be a tool argument", name)
		}
		ro := entry.Tool.Annotations.ReadOnlyHint != nil && *entry.Tool.Annotations.ReadOnlyHint
		_, gated := entry.Tool.InputSchema.Properties["confirm"]
		switch {
		case provenReads[name]:
			if !ro {
				t.Errorf("%s: a proven read must be marked read-only", name)
			}
			if gated {
				t.Errorf("%s: a read should not need confirm", name)
			}
		case ro:
			t.Errorf("%s is marked read-only but is not a reviewed read: name it in provenReads after review, or gate it", name)
		case !gated:
			t.Errorf("%s may write and has no confirm gate", name)
		default:
			gatedCount++
		}
	}
	for name := range provenReads {
		if _, ok := tools[name]; !ok {
			t.Errorf("%s is listed as a proven read but is not registered", name)
		}
	}
	for _, name := range []string{"learnings_forget", "learnings_confirm", "learnings_reject", "teach", "teach_lookup", "teach_pattern", "teach_playbook", "playbook_amend", "export", "sync", "workflow_archive"} {
		entry, ok := tools[name]
		if !ok {
			t.Errorf("%s not registered", name)
			continue
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; !gated {
			t.Errorf("%s writes local files and has no confirm gate", name)
		}
	}
	if gatedCount == 0 {
		t.Fatal("no tool carries the confirm gate; the gate is not installed")
	}
}

// Codex r1 input: {"name":"learnings_forget","arguments":{"query":"find
// application","all":true}} must not delete anything without confirm. The
// unconfirmed call returns a preview and never starts the CLI.
func TestLearningsForgetWithoutConfirmOnlyPreviews(t *testing.T) {
	s := server.NewMCPServer("mirror-confirm", "test")
	RegisterTools(s)
	entry := s.GetTool("learnings_forget")
	if entry == nil {
		t.Fatal("learnings_forget not registered")
	}
	req := mcplib.CallToolRequest{}
	req.Params.Name = "learnings_forget"
	req.Params.Arguments = map[string]any{"query": "find application", "all": true}
	res, err := entry.Handler(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	if out := b.String(); !strings.Contains(out, "Preview only: nothing was sent") {
		t.Fatalf("learnings_forget without confirm must only preview, got %s", out)
	}
}
