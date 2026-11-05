// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dropsuite-pp-cli/internal/mcp/cobratree"
)

// The tools that can change Dropsuite state (the two portal sign-in link
// endpoints, and evidence/locate with their sign-in link flags) carry the
// confirm gate in the real registration (dropsuite handfix:
// mcp-mirror-write-confirm); every other non-read-only tool is local-only, and
// the reads are not gated.
func TestRegisteredMirroredWritesNeedConfirm(t *testing.T) {
	s := server.NewMCPServer("mirror-confirm", "test")
	RegisterTools(s)
	tools := s.ListTools()
	for _, name := range []string{"portal_get-login-link", "organizations_get-support-sso-link", "evidence", "locate"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		if _, ok := entry.Tool.InputSchema.Properties["confirm"]; !ok {
			t.Errorf("%s: no confirm gate", name)
		}
		if _, ok := entry.Tool.InputSchema.Properties["yes"]; ok {
			t.Errorf("%s: --yes must never be a tool argument", name)
		}
	}
	for _, name := range []string{"health", "coverage", "billing", "hygiene", "drift", "restore_test_due",
		"accounts_list", "organizations_list", "server_get-status"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; gated {
			t.Errorf("%s: read or local-only tool should not need confirm", name)
		}
	}
	// Fail closed: every tool that is neither read-only nor gated is a local-only
	// child-CLI command.
	local := map[string]bool{"export": true, "sync": true, "health": true, "restore_test_record": true, "workflow_archive": true,
		"learnings_confirm": true, "learnings_forget": true, "learnings_reject": true, "playbook_amend": true,
		"teach": true, "teach_lookup": true, "teach_pattern": true, "teach_playbook": true}
	for name, entry := range tools {
		if hint := entry.Tool.Annotations.ReadOnlyHint; hint != nil && *hint {
			continue
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; gated {
			continue
		}
		if !local[name] {
			t.Errorf("%s: not read-only, not gated and not a reviewed local-only command", name)
		}
	}
}

// An endpoint tool that mints a sign-in link sends nothing without confirm:
// boolean true only; the string "true" previews.
func TestEndpointWriteToolPreviewsWithoutConfirm(t *testing.T) {
	t.Setenv("DROPSUITE_BASE_URL", "http://127.0.0.1:9/api")
	s := server.NewMCPServer("endpoint-confirm", "test")
	RegisterTools(s)
	entry, ok := s.ListTools()["portal_get-login-link"]
	if !ok {
		t.Fatal("portal_get-login-link not registered")
	}
	for _, confirm := range []any{nil, "true", 1, false} {
		req := mcplib.CallToolRequest{}
		args := map[string]any{"role": "admin"}
		if confirm != nil {
			args["confirm"] = confirm
		}
		req.Params.Arguments = args
		res, err := entry.Handler(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		text := res.Content[0].(mcplib.TextContent).Text
		if !strings.Contains(text, `"preview": true`) && !strings.Contains(text, `"preview":true`) {
			t.Errorf("confirm=%v: want a preview, got %s", confirm, text)
		}
	}
}

// Class host-file-read-from-mcp-argument (msp-skills check_mcp_file_flags):
// billing --invoice opens a local CSV, so the MCP billing tool must not offer
// it, and a call that supplies it anyway is refused by name before the
// companion CLI runs.
func TestBillingInvoicePathIsNotAnMCPArgument(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script stand-in")
	}
	// A recording stand-in for the companion CLI: any value it is handed shows in the result.
	fake := filepath.Join(t.TempDir(), "fake-cli")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho \"ARGS: $*\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DROPSUITE_CLI_PATH", fake)
	cobratree.ResetSiblingCLIPathCache()
	t.Cleanup(cobratree.ResetSiblingCLIPathCache)
	s := server.NewMCPServer("file-flags", "test")
	RegisterTools(s)
	entry, ok := s.ListTools()["billing"]
	if !ok {
		t.Fatal("billing not registered")
	}
	if _, offered := entry.Tool.InputSchema.Properties["invoice"]; offered {
		t.Fatal("billing offers --invoice (a host file path) as an MCP argument")
	}
	req := mcplib.CallToolRequest{}
	req.Params.Arguments = map[string]any{"invoice": "/etc/hosts"}
	res, err := entry.Handler(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	for _, c := range res.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			text += tc.Text
		}
	}
	if !res.IsError || !strings.Contains(text, "invoice") || strings.Contains(text, "/etc/hosts") {
		t.Fatalf("a call with invoice was not refused by name before the CLI ran: error=%v %s", res.IsError, text)
	}
}
