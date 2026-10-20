// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

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
	// The escape is lossless: the wire text carries no raw control byte, and
	// a JSON decode restores exactly the original values (encoding/json had
	// already escaped the C0 bytes; the C1 runes are now \u00XX escapes too).
	if back["note"] != "x\x1by\x07" || back["name"] != "Acme\u009d52;c;VEVTVA==\u009c" {
		t.Fatalf("round trip changed the data: %q", back)
	}
	if res := mcpResultText("a\u009bb"); strings.Contains(res.Content[0].(mcplib.TextContent).Text, "\u009b") {
		t.Fatal("mcpResultText did not escape")
	}
}

// TestToolHandlersUseEscapingConstructors is the regression guard for the
// class: a handler that calls mcplib.NewToolResultText or NewToolResultError
// directly bypasses the escape, so no file in this package may do so.
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
				t.Fatalf("%s calls %s directly; use mcpResultText/mcpResultError (or the escaped* constructors in cobratree)", f, bad)
			}
		}
	}
}
