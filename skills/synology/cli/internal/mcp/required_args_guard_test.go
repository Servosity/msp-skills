// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(mcp-required-args-enforced).

package mcp

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// callViaServer sends a tools/call through the server's own message path
// (the one stdio uses), so every middleware runs.
func callViaServer(t *testing.T, s *server.MCPServer, name string, args map[string]any) (string, bool) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args}})
	b, _ := json.Marshal(s.HandleMessage(context.Background(), raw))
	var resp struct {
		Result *struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(b, &resp); err != nil || resp.Result == nil {
		t.Fatalf("%s: protocol answer %s", name, b)
	}
	text := ""
	for _, c := range resp.Result.Content {
		text += c.Text
	}
	return text, resp.Result.IsError
}

// TestMCPRequiredArgsAreEnforced: abb_backup and abb_cancel declare task_ids
// as required, yet a call without it sent the write to the NAS (the CLI
// refuses the same call). Every tool now refuses a call that omits, nulls or
// blanks an argument its schema marks required, before anything is sent;
// reads (abb_result_detail) and writes alike; the same call with the
// argument goes through.
func TestMCPRequiredArgsAreEnforced(t *testing.T) {
	fake := newFakeDSM(t, "Office PCs")
	s := sweepMCPServer(t, fake, "")
	for _, c := range []struct {
		tool string
		args map[string]any
	}{
		{"abb_backup", map[string]any{}},
		{"abb_backup", map[string]any{"task_ids": ""}},
		{"abb_backup", map[string]any{"task_ids": "   "}},
		{"abb_backup", map[string]any{"task_ids": nil}},
		{"abb_cancel", map[string]any{}},
		{"abb_result_detail", map[string]any{"limit": 5}},
		{"abb_transfers", map[string]any{"time_start": 1}},
	} {
		text, isErr := callViaServer(t, s, c.tool, c.args)
		if !isErr || !strings.Contains(text, "requires") || !strings.Contains(text, "nothing was sent") {
			t.Fatalf("%s %v: isErr=%v text=%s, want a refusal naming the missing argument", c.tool, c.args, isErr, text)
		}
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Fatalf("a call missing a required argument reached the NAS: %+v", calls)
	}
	if text, isErr := callViaServer(t, s, "abb_backup", map[string]any{"task_ids": "[8]"}); isErr {
		t.Fatalf("abb_backup with task_ids was refused: %s", text)
	}
	backups := 0
	for _, c := range fake.Calls() {
		if c.Method == "backup" && c.Query["task_ids"] == "[8]" {
			backups++
		}
	}
	if backups != 1 {
		t.Fatalf("abb_backup with task_ids sent %d backup request(s): %+v", backups, fake.Calls())
	}

	// The class: every registered tool with a required argument refuses a
	// call without it, and nothing reaches the NAS.
	var names []string
	for name, st := range s.ListTools() {
		if len(st.Tool.InputSchema.Required) > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) < 3 {
		t.Fatalf("fixture: only %d tools declare a required argument: %v", len(names), names)
	}
	before := len(fake.Calls())
	for _, name := range names {
		if text, isErr := callViaServer(t, s, name, map[string]any{}); !isErr || !strings.Contains(text, "requires") {
			t.Errorf("%s {} : isErr=%v text=%.200s, want the required-argument refusal", name, isErr, text)
		}
	}
	if after := len(fake.Calls()); after != before {
		t.Fatalf("calls without their required arguments reached the NAS: %+v", fake.Calls()[before:])
	}
}
