// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/store"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// hostileName holds every rune JSON leaves raw that a terminal acts on: C1
// CSI and NEL, and bidi override and isolate controls. Built from code
// points so the source holds no raw control.
var hostileName = "HARMLESS" + string(rune(0x9b)) + "2J" + string(rune(0x85)) + "X" + string(rune(0x202e)) + "XX" + string(rune(0x2066)) + "Y" + string(rune(0x90)) + "Z"

func hasTerminalRune(s string) bool { return strings.ContainsFunc(s, terminalRune) }

// TestInProcessToolsEscapeTerminalControls (adversarial round 6, P2):
// eset-protect_execute, sql and search answer in-process through
// encoding/json only, which leaves U+0080-U+009F and the bidi controls raw,
// so a hostile device name reached the MCP host with CSI and RLO intact
// (the CLI escapes them). Every tool's text is now escaped like the CLI's
// writer, and stays valid JSON that decodes to the same name
// (handfixes.json: mcp-results-escape-terminal-controls).
func TestInProcessToolsEscapeTerminalControls(t *testing.T) {
	t.Setenv("ESET_PROTECT_MCP_READ_ONLY", "")
	esetFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"devices": []map[string]string{{"uuid": "00000000-0000-0000-0000-000000000001", "displayName": hostileName}}, "nextPageToken": ""})
	})
	path, err := mcpDBPath()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	row, _ := json.Marshal(map[string]string{"uuid": "00000000-0000-0000-0000-000000000001", "displayName": hostileName})
	if _, _, err := db.UpsertBatch("devices", []json.RawMessage{row}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	s := server.NewMCPServer("eset-protect", "test")
	RegisterTools(s)
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{"eset-protect_execute", map[string]any{"endpoint_id": "devices.list", "params": map[string]any{}}},
		{"sql", map[string]any{"query": "SELECT data FROM resources WHERE resource_type='devices'"}},
		{"search", map[string]any{"query": "HARMLESS"}},
	} {
		raw := callTool(t, s, call.name, call.args)
		var resp struct {
			Result mcplib.CallToolResult `json:"result"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("%s: %v\n%s", call.name, err, raw)
		}
		text := ""
		for _, c := range resp.Result.Content {
			if tc, ok := c.(mcplib.TextContent); ok {
				text += tc.Text
			}
		}
		if hasTerminalRune(text) {
			t.Fatalf("%s: a raw C1 or bidi control reached the MCP host: %q", call.name, text)
		}
		if want := escapedHostileName(); !strings.Contains(text, want) {
			t.Fatalf("%s: the name was not escaped in place (want %q): %q", call.name, want, text)
		}
		// Lossless: the JSON text decodes back to the hostile name.
		var v any
		if err := json.Unmarshal([]byte(text), &v); err != nil {
			t.Fatalf("%s: the escaped text is no longer JSON: %v\n%q", call.name, err, text)
		}
		if b, _ := json.Marshal(v); !strings.Contains(decodeAll(string(b)), hostileName) {
			t.Fatalf("%s: the decoded JSON lost the name: %s", call.name, b)
		}
	}
}

// decodeAll unquotes every JSON string in s and joins them, so a nested
// JSON-in-JSON value is searchable.
func decodeAll(s string) string {
	var out strings.Builder
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case string:
			out.WriteString(t)
			out.WriteString("\n")
			var inner any
			if json.Unmarshal([]byte(t), &inner) == nil {
				walk(inner)
			}
		case map[string]any:
			for _, y := range t {
				walk(y)
			}
		case []any:
			for _, y := range t {
				walk(y)
			}
		}
	}
	var v any
	if json.Unmarshal([]byte(s), &v) == nil {
		walk(v)
	}
	return out.String()
}

// escapedHostileName is hostileName as the CLI writer prints it, built from
// code points: backslash (92), "u" and four hex digits per control.
func escapedHostileName() string {
	esc := func(hex string) string { return string(rune(92)) + "u" + hex }
	return "HARMLESS" + esc("009b") + "2J" + esc("0085") + "X" + esc("202e") + "XX" + esc("2066") + "Y" + esc("0090") + "Z"
}
