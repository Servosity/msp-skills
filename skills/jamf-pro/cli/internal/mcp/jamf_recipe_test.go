// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// TestRecipeToolArgumentsNeverChooseTheCommand (jamf-pro hand-fix
// mcp-recipe-argv): each recipe intent runs its own command whatever the
// tool arguments say; an argument that would name a subcommand or set a flag
// is refused or stays data.
func TestRecipeToolArgumentsNeverChooseTheCommand(t *testing.T) {
	oldPath, oldErr := recipeCLIPath, recipeCLIPathErr
	t.Cleanup(func() { recipeCLIPath, recipeCLIPathErr = oldPath, oldErr })
	recipeCLIPath = writeRecipeIntentRecorder(t)
	recipeCLIPathErr = nil
	type handler func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error)
	call := func(h handler, args map[string]any) (string, bool) {
		t.Helper()
		res, err := h(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: args}})
		if err != nil {
			t.Fatalf("transport error: %v", err)
		}
		var b strings.Builder
		for _, c := range res.Content {
			if text, ok := c.(mcplib.TextContent); ok {
				b.WriteString(text.Text)
			}
		}
		return strings.TrimSpace(b.String()), res.IsError
	}

	if got, isErr := call(handleBrokenPoliciesAcrossClients, map[string]any{}); isErr || got != "fleet policy-hygiene --agent" {
		t.Fatalf("broken_policies_across_clients must run fleet policy-hygiene, got error=%v %q", isErr, got)
	}
	for _, slug := range []string{"exec", "instance", "policy-hygiene"} {
		if got, isErr := call(handleBrokenPoliciesAcrossClients, map[string]any{"slug": slug}); !isErr || !strings.Contains(got, `unknown MCP parameter "slug"`) {
			t.Fatalf("slug %q must be refused, got error=%v %q", slug, isErr, got)
		}
	}
	got, isErr := call(handleAnyReadAcrossAllClients, map[string]any{"path": "--instance=globex"})
	if isErr || got != "fleet exec --agent -- --instance=globex" {
		t.Fatalf("a flag-like path must stay data, got error=%v %q", isErr, got)
	}
	if got, isErr := call(handleAnyReadAcrossAllClients, map[string]any{"path": "/api/v1/buildings"}); isErr || got != "fleet exec /api/v1/buildings --agent" {
		t.Fatalf("path must be the exec argument, got error=%v %q", isErr, got)
	}

	// The guard itself: an argv that resolves to another command is refused.
	if _, err := runRecipeCommand(context.Background(), []string{"fleet", "policy-hygiene", "--agent"}, []string{"fleet", "exec", "--agent"}); err == nil {
		t.Fatal("an argv that resolves to another command must be refused")
	}
}
