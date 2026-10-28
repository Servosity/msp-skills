// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(mcp-writes-need-confirm).

package mcp

import (
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"synology-pp-cli/internal/cli"
)

// TestWritesPreviewUntilConfirmed: abb_backup, abb_cancel and session_logout
// (start or cancel a backup, end the DSM session) are exactly the gated tools.
// Without "confirm": true (a JSON boolean) they return a preview and send
// nothing; an undeclared or missing argument is refused; confirmed, the call
// reaches the NAS without the confirm argument. Read tools are not gated.
func TestWritesPreviewUntilConfirmed(t *testing.T) {
	fake := newFakeDSM(t, "Office PCs")
	s := sweepMCPServer(t, fake, "")
	var gated []string
	for name, st := range s.ListTools() {
		if _, ok := st.Tool.InputSchema.Properties[confirmArg]; ok {
			gated = append(gated, name)
		}
	}
	sort.Strings(gated)
	if got := strings.Join(gated, ","); got != "abb_backup,abb_cancel,session_logout" {
		t.Fatalf("gated tools = %s", got)
	}
	for _, args := range []map[string]any{
		{"task_ids": "[5]"},
		{"task_ids": "[5]", "confirm": "true"},
		{"task_ids": "[5]", "confirm": false},
		{"task_ids": "[5]", "confirm": 1},
	} {
		text, isErr := callTool(t, s, "abb_backup", args)
		if isErr || !strings.Contains(text, `"dry_run":true`) || !strings.Contains(text, "nothing was sent") {
			t.Fatalf("abb_backup %v: want a preview, got isErr=%v %s", args, isErr, text)
		}
	}
	for _, args := range []map[string]any{{"confirm": true}, {"task_ids": "[5]", "dry-run": true}} {
		if text, isErr := callTool(t, s, "abb_cancel", args); !isErr || !strings.Contains(text, "nothing was sent") {
			t.Fatalf("abb_cancel %v: want a refusal, got %s", args, text)
		}
	}
	if text, _ := callTool(t, s, "session_logout", map[string]any{}); !strings.Contains(text, `"dry_run":true`) {
		t.Fatalf("session_logout without confirm: %s", text)
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Fatalf("an unconfirmed or refused write reached the NAS: %+v", calls)
	}
	if text, isErr := callTool(t, s, "abb_backup", map[string]any{"task_ids": "[5]", "confirm": true}); isErr {
		t.Fatalf("confirmed abb_backup failed: %s", text)
	}
	calls := fake.Calls()
	if len(calls) == 0 || calls[len(calls)-1].Method != "backup" {
		t.Fatalf("the confirmed backup did not reach the NAS: %+v", calls)
	}
	if _, sent := calls[len(calls)-1].Query["confirm"]; sent {
		t.Fatalf("confirm was forwarded to the NAS: %+v", calls[len(calls)-1])
	}
	if _, isErr := callTool(t, s, "abb_tasks", map[string]any{}); isErr {
		t.Fatal("a read tool was refused")
	}
}

// TestMirrorCarriesNoDSMWrite: every endpoint command that is not read-only
// (a DSM write) is a typed tool behind the confirm gate, never a Cobra-tree
// mirror tool that would shell out unconfirmed.
func TestMirrorCarriesNoDSMWrite(t *testing.T) {
	s := sweepMCPServer(t, nil, "")
	var walk func(c *cobra.Command)
	n := 0
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			walk(sub)
		}
		if c.Annotations["pp:endpoint"] == "" || c.Annotations["mcp:read-only"] == "true" {
			return
		}
		n++
		parts := strings.Fields(c.CommandPath())[1:]
		name := strings.ReplaceAll(strings.Join(parts, "_"), "-", "_")
		st := s.GetTool(name)
		if st == nil {
			t.Fatalf("DSM write %q has no typed tool %q", c.CommandPath(), name)
		}
		if _, ok := st.Tool.InputSchema.Properties[confirmArg]; !ok {
			t.Fatalf("DSM write tool %q is not behind the confirm gate", name)
		}
	}
	walk(cli.RootCmd())
	if n < 3 {
		t.Fatalf("found %d DSM write commands; abb backup, abb cancel and session logout at least", n)
	}
}
