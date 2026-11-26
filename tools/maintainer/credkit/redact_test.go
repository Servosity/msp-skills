// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func toolReturning(res *mcp.CallToolResult) server.ToolHandlerFunc {
	return MCPTools(func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) { return res, nil })
}

func callTool(t *testing.T, h server.ToolHandlerFunc) (*mcp.CallToolResult, string) {
	t.Helper()
	res, err := h(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res)
	return res, string(b)
}

// A policy result that carries held passwords reaches the client with only
// those leaves masked; every other member, and member order, is kept.
func TestRedactMCPToolResultMasksOnlyHeldLeaves(t *testing.T) {
	resetForTest(Declarations{})
	Hold("response:password", "pxPwHd61Ys3Tc8", "puPwMx72Nq4Rb9")
	policy := `{"id":"p1","name":"Servidores Linux","settings":{"update":{"proxy":{"profile":1,"settings":{"server":"192.168.100.251","port":3128,"username":"svc","password":"pxPwHd61Ys3Tc8"}}},` +
		`"general":{"settings":{"powerUser":{"enabled":true,"password":"puPwMx72Nq4Rb9"}}},` +
		`"antimalware":{"exclusions":[{"type":"folder","path":"/etc/pve/"},{"type":"folder","path":"/mnt/pve/nfs/"}]}}}`
	res, out := callTool(t, toolReturning(mcp.NewToolResultText(policy)))
	if res.IsError {
		t.Fatalf("result withheld instead of redacted: %s", out)
	}
	assertNotContains(t, out, "pxPwHd61Ys3Tc8", "puPwMx72Nq4Rb9")
	text := res.Content[0].(mcp.TextContent).Text
	want := strings.NewReplacer("pxPwHd61Ys3Tc8", Redacted, "puPwMx72Nq4Rb9", Redacted).Replace(policy)
	if text != want {
		t.Errorf("redacted text =\n%s\nwant\n%s", text, want)
	}
}

// A string leaf that carries a held value inside a longer text (an install
// link with a token) is masked whole.
func TestRedactMCPToolResultMasksLeafCarryingValue(t *testing.T) {
	resetForTest(Declarations{})
	Hold("response:token", "Tk9fQ2xWm7Rz4Lp8")
	doc := `{"items":[{"name":"linux","link":"https://cloud.example/pkg?tok=Tk9fQ2xWm7Rz4Lp8&x=1"},{"name":"win","link":"https://cloud.example/pkg?x=2"}]}`
	res, out := callTool(t, toolReturning(mcp.NewToolResultText(doc)))
	if res.IsError {
		t.Fatalf("withheld: %s", out)
	}
	assertNotContains(t, out, "Tk9fQ2xWm7Rz4Lp8")
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, `"link":"***"`) || !strings.Contains(text, `"link":"https://cloud.example/pkg?x=2"`) {
		t.Errorf("redacted text = %s", text)
	}
}

// Keys and numbers are leaves too; an indented document stays indented.
func TestRedactMCPToolResultKeysNumbersIndent(t *testing.T) {
	resetForTest(Declarations{})
	Hold("response:licenseKey", "LK7Q2M9X4P8R", "918273645501")
	doc := "{\n  \"LK7Q2M9X4P8R\": 1,\n  \"pin\": 918273645501,\n  \"ok\": [true, null, 2.5]\n}"
	res, out := callTool(t, toolReturning(mcp.NewToolResultText(doc)))
	if res.IsError {
		t.Fatalf("withheld: %s", out)
	}
	assertNotContains(t, out, "LK7Q2M9X4P8R", "918273645501")
	text := res.Content[0].(mcp.TextContent).Text
	want := "{\n  \"***\": 1,\n  \"pin\": \"***\",\n  \"ok\": [\n    true,\n    null,\n    2.5\n  ]\n}"
	if text != want {
		t.Errorf("redacted text =\n%s\nwant\n%s", text, want)
	}
}

// Text that is not one JSON document is withheld whole, as before.
func TestRedactMCPToolResultNonJSONWithheld(t *testing.T) {
	resetForTest(Declarations{})
	Hold("response:password", "nonJsonPw4Kq8z2")
	for _, text := range []string{"you sent nonJsonPw4Kq8z2", `{"a":"nonJsonPw4Kq8z2"} {"b":1}`, `{"a":"nonJsonPw4Kq8z2"`} {
		res, out := callTool(t, toolReturning(mcp.NewToolResultText(text)))
		assertNotContains(t, out, "nonJsonPw4Kq8z2")
		if !res.IsError || !strings.Contains(out, Notice(SinkMCP)) {
			t.Errorf("%q: not withheld whole: %s", text, out)
		}
	}
}

// With the held set unknown every unit is withheld; redaction releases nothing.
func TestRedactMCPToolResultUnknownHeldSet(t *testing.T) {
	resetForTest(Declarations{})
	markUnknown()
	res, out := callTool(t, toolReturning(mcp.NewToolResultText(`{"a":"x"}`)))
	if !res.IsError || strings.Contains(out, `"a"`) {
		t.Errorf("unknown held set released a result: %s", out)
	}
}

// Structured content is redacted the same way.
func TestRedactMCPToolResultStructuredContent(t *testing.T) {
	resetForTest(Declarations{})
	Hold("response:password", "structPw7Lm3Nq9")
	res := mcp.NewToolResultStructured(map[string]any{"name": "p", "password": "structPw7Lm3Nq9"}, `{"name":"p","password":"***"}`)
	got, out := callTool(t, toolReturning(res))
	if got.IsError {
		t.Fatalf("withheld: %s", out)
	}
	assertNotContains(t, out, "structPw7Lm3Nq9")
	if !strings.Contains(out, `"password":"***"`) || !strings.Contains(out, `"name":"p"`) {
		t.Errorf("structured content = %s", out)
	}
}

// A clean result passes unchanged.
func TestRedactMCPToolResultCleanUnchanged(t *testing.T) {
	resetForTest(Declarations{})
	Hold("response:password", "cleanPw8Hx2Rt5")
	in := mcp.NewToolResultText(`{"b":1,"a":"x"}`)
	got, _ := callTool(t, toolReturning(in))
	if got != in {
		t.Error("a clean result was rewritten")
	}
}

func assertNotContains(t *testing.T, out string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if strings.Contains(out, s) {
			t.Errorf("output carries %q: %s", s, out)
		}
	}
}
