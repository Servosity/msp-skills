// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-tool-results-escape-controls" and
// "mcp-read-only-withholds-credentials" entries.

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/mcp/redact"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// callToolWire drives one tools/call through the server's real JSON-RPC
// dispatch (middleware included) and returns the encoded response frame,
// which is exactly what reaches the MCP client.
func callToolWire(t *testing.T, s *server.MCPServer, name string, args map[string]any) string {
	t.Helper()
	req, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp := s.HandleMessage(context.Background(), req)
	out, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// resultTexts decodes a tools/call response frame into its text blocks.
func resultTexts(t *testing.T, frame string) []string {
	t.Helper()
	var decoded struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(frame), &decoded); err != nil {
		t.Fatalf("decode frame: %v (%s)", err, frame)
	}
	if decoded.Error != nil {
		return []string{decoded.Error.Message}
	}
	texts := make([]string, 0, len(decoded.Result.Content))
	for _, c := range decoded.Result.Content {
		texts = append(texts, c.Text)
	}
	return texts
}

func newFunnelServer(t *testing.T, probes map[string]server.ToolHandlerFunc) *server.MCPServer {
	t.Helper()
	s := server.NewMCPServer("result-funnel", "test")
	RegisterTools(s)
	for name, h := range probes {
		// Probe tools are added after RegisterTools, the way a later
		// registrar would add one; the funnel must still cover them.
		s.AddTool(mcplib.NewTool(name), h)
	}
	return s
}

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
	if escapeTerminalControls("a\r\nb") != "a\r\nb" || escapeTerminalControls("a\rb") != `a\u000db` {
		t.Fatal("CR handling wrong: CRLF must pass, a bare CR must be escaped")
	}
}

// TestResultFunnelEscapesEveryTool is the class guard: every tool result and
// handler error, whatever registered it, reaches the wire with C0/C1/ESC
// controls escaped.
func TestResultFunnelEscapesEveryTool(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	payload := "Acme\u009d52;c;VEVTVA==\u009c \x1b[8mhidden\x1b[0m\rover"
	s := newFunnelServer(t, map[string]server.ToolHandlerFunc{
		"probe_text": func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			res := mcplib.NewToolResultText(payload)
			res.Content = append(res.Content, mcplib.NewTextContent("hint: "+payload))
			return res, nil
		},
		"probe_error": func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			return mcplib.NewToolResultError(payload), nil
		},
		"probe_go_error": func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			return nil, errors.New(payload)
		},
	})
	for _, name := range []string{"probe_text", "probe_error", "probe_go_error"} {
		frame := callToolWire(t, s, name, map[string]any{})
		texts := resultTexts(t, frame)
		if len(texts) == 0 {
			t.Fatalf("%s: no text in %s", name, frame)
		}
		for _, text := range texts {
			for _, bad := range []string{"\u009d", "\u009c", "\x1b", "\r"} {
				if strings.Contains(text, bad) {
					t.Fatalf("%s: raw control %q reached the client: %q", name, bad, text)
				}
			}
			if !strings.Contains(text, `\u009d`) {
				t.Fatalf("%s: expected the escape in %q", name, text)
			}
		}
	}
}

const credentialSentinel = "SENTINEL-NET-SECRET-7f3a"

// TestReadOnlyResultFunnelWithholdsCredentials: under the read-only switch no
// tool result carries a credential in any shape a tool can produce (a JSON
// list, NDJSON, a JSON body quoted in an error, JSON nested in a string, a
// CSV or table header, key=value prose, a later echo of a withheld value),
// while site names stay readable. Off, results are untouched.
func TestReadOnlyResultFunnelWithholdsCredentials(t *testing.T) {
	network := `{"data":[{"id":7,"name":"Main Office","secret_key":"` + credentialSentinel + `","organization":{"options":{"client_secret":"` + credentialSentinel + `-2"}},"has_secret_key":true}]}`
	probes := map[string]string{
		"probe_json":   network,
		"probe_ndjson": `{"id":1,"name":"HQ","secret_key":"` + credentialSentinel + `"}` + "\n" + `{"id":2,"name":"Lab","token":"` + credentialSentinel + `-tok"}` + "\n",
		"probe_quoted": `cli dnsfilter-cli: exit status 1 (output: ` + network + `)`,
		"probe_nested": `{"payload":"{\"secret_key\":\"` + credentialSentinel + `\",\"name\":\"Inner\"}"}`,
		"probe_csv":    "id,name,secret_key\n7,Main Office," + credentialSentinel + "\n",
		"probe_table":  "ID  NAME         SECRET_KEY\n7   Main Office  " + credentialSentinel + "\n",
		"probe_kv":     "name: Main Office\nsecret_key: " + credentialSentinel + "\n",
		"probe_pin":    `{"data":{"attributes":{"uninstall_pin":"493817"}}}`,
	}
	handlers := map[string]server.ToolHandlerFunc{}
	for name, text := range probes {
		text := text
		handlers[name] = func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			return mcplib.NewToolResultText(text), nil
		}
	}
	handlers["probe_echo"] = func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return mcplib.NewToolResultText("site Main Office was deployed with " + credentialSentinel), nil
	}

	t.Run("read-only on", func(t *testing.T) {
		t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
		s := newFunnelServer(t, handlers)
		for _, name := range []string{"probe_json", "probe_ndjson", "probe_quoted", "probe_nested", "probe_csv", "probe_table", "probe_kv", "probe_pin", "probe_echo"} {
			frame := callToolWire(t, s, name, map[string]any{})
			if strings.Contains(frame, "SENTINEL") || strings.Contains(frame, "493817") {
				t.Fatalf("%s: credential reached the client under read-only: %s", name, frame)
			}
		}
		frame := callToolWire(t, s, "probe_json", map[string]any{})
		texts := resultTexts(t, frame)
		var decoded map[string]any
		if err := json.Unmarshal([]byte(texts[0]), &decoded); err != nil {
			t.Fatalf("redacted JSON no longer decodes: %v (%s)", err, texts[0])
		}
		if !strings.Contains(texts[0], `"name":"Main Office"`) || !strings.Contains(texts[0], `"has_secret_key":true`) {
			t.Fatalf("redaction removed non-credential data: %s", texts[0])
		}
		if !strings.Contains(texts[0], redact.WithheldFieldPlaceholder) {
			t.Fatalf("withheld marker missing: %s", texts[0])
		}
	})

	t.Run("read-only off", func(t *testing.T) {
		t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
		s := newFunnelServer(t, handlers)
		frame := callToolWire(t, s, "probe_json", map[string]any{})
		if !strings.Contains(frame, credentialSentinel) {
			t.Fatalf("default mode must not rewrite results: %s", frame)
		}
	})
}

// TestReadOnlyFilterKeepsAuthHintReadable: the auth hint names this CLI's
// env var with a placeholder, which is not a credential field.
func TestReadOnlyFilterKeepsAuthHintReadable(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	hint := `HTTP 401: Set your API key with: export DNSFILTER_API_KEY="your-token-here" (PowerShell: $env:DNSFILTER_API_KEY = "your-token-here")`
	if got := redact.Final(hint); got != hint {
		t.Fatalf("auth hint withheld: %q", got)
	}
	refusal := "refusing GET api-keys.index while DNSFILTER_MCP_READ_ONLY=1: it returns or manages a credential"
	if got := redact.Final(refusal); got != refusal {
		t.Fatalf("refusal withheld: %q", got)
	}
}

// TestEscapeBidiControls: the bidi embedding/override/isolate controls a
// tenant name can carry (Trojan Source) reach the client escaped, losslessly
// inside JSON.
func TestEscapeBidiControls(t *testing.T) {
	name := "policy-\u202egnp.exe\u202c and \u2066isolate\u2069 \u202a\u202b\u202d\u2067\u2068"
	raw, _ := json.Marshal(map[string]string{"name": name})
	got := escapeTerminalControls(string(raw))
	for _, r := range []rune{0x202a, 0x202b, 0x202c, 0x202d, 0x202e, 0x2066, 0x2067, 0x2068, 0x2069} {
		if strings.ContainsRune(got, r) {
			t.Fatalf("raw bidi control U+%04X reached the client: %q", r, got)
		}
	}
	if !strings.Contains(got, `\u202e`) || !strings.Contains(got, `\u2066`) {
		t.Fatalf("expected \\u202e / \\u2066 escapes: %q", got)
	}
	var back map[string]string
	if err := json.Unmarshal([]byte(got), &back); err != nil || back["name"] != name {
		t.Fatalf("escape is not lossless inside JSON: %v %q", err, back)
	}
	if plain := "Main Office \u05e9\u05dc\u05d5\u05dd"; escapeTerminalControls(plain) != plain {
		t.Fatal("ordinary RTL text must pass unchanged")
	}
}
