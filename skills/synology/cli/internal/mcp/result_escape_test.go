// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "mcp-tool-results-escape-controls" entry.

package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func TestEscapeTerminalControlsKeepsJSONValid(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{"name": "Acme\u009d52;c;VEVTVA==\u009c", "note": "x\x1by\x07"})
	got := escapeTerminalControls(string(raw))
	for _, bad := range []string{"\u009d", "\u009c", "\x1b", "\x07"} {
		if strings.Contains(got, bad) {
			t.Fatalf("raw control %q survived: %q", bad, got)
		}
	}
	var back map[string]string
	if err := json.Unmarshal([]byte(got), &back); err != nil {
		t.Fatalf("escaped text is not valid JSON: %v (%q)", err, got)
	}
	if back["note"] != "x\x1by\x07" || back["name"] != "Acme\u009d52;c;VEVTVA==\u009c" {
		t.Fatalf("round trip changed the data: %q", back)
	}
	if res := mcpResultText("a\u009bb"); strings.Contains(res.Content[0].(mcplib.TextContent).Text, "\u009b") {
		t.Fatal("mcpResultText did not escape")
	}
}

// TestEndpointResultEscapesTaskName calls the typed abb_tasks tool against a
// NAS whose task name carries an OSC 52 clipboard write in C1 form; the
// result text must carry no raw control character.
func TestEndpointResultEscapesTaskName(t *testing.T) {
	fake := newFakeDSM(t, "PC-\u009d52;c;QQ==\u009c-\x1b]52;c;SGk=\x07")
	s := sweepMCPServer(t, fake, "")
	text, isErr := callTool(t, s, "abb_tasks", map[string]any{})
	if isErr {
		t.Fatalf("abb_tasks failed: %s", text)
	}
	for _, r := range text {
		if r == 0x1b || r == 0x07 || (r >= 0x80 && r <= 0x9f) {
			t.Fatalf("raw control %U reached the MCP result: %q", r, text)
		}
	}
	if !strings.Contains(text, `\u009d52;c;QQ==\u009c`) {
		t.Fatalf("task name lost: %q", text)
	}
}

// TestToolHandlersUseEscapingConstructors is the regression guard for the
// class: a handler that calls mcplib.NewToolResultText/Error or
// NewTextContent directly bypasses the escape.
func TestToolHandlersUseEscapingConstructors(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	sub, _ := filepath.Glob("*/*.go")
	files = append(files, sub...)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "result_escape.go" || f == filepath.Join("cobratree", "result_escape.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"mcplib.NewToolResultText(", "mcplib.NewToolResultError(", "mcplib.NewTextContent("} {
			if strings.Contains(string(b), bad) {
				t.Errorf("%s calls %s directly; use mcpResultText/mcpResultError (or the escaped* constructors in cobratree)", f, bad)
			}
		}
	}
}
