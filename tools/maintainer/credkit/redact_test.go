// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"context"
	"encoding/base64"
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
	const proxyPw, powerPw = "pxPwHd61Ys3Tc8", "puPwMx72Nq4Rb9" // synthetic fixtures
	Hold("response:password", proxyPw, powerPw)
	policy := `{"id":"p1","name":"Servidores Linux","settings":{"update":{"proxy":{"profile":1,"settings":{"server":"192.168.100.251","port":3128,"username":"svc","password":"` + proxyPw + `"}}},` +
		`"general":{"settings":{"powerUser":{"enabled":true,"password":"` + powerPw + `"}}},` +
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
	if !strings.Contains(text, `"link":"[redacted]"`) || !strings.Contains(text, `"link":"https://cloud.example/pkg?x=2"`) {
		t.Errorf("redacted text = %s", text)
	}
}

// Numbers are leaves too, keys are kept; an indented document stays indented.
func TestRedactMCPToolResultKeysNumbersIndent(t *testing.T) {
	resetForTest(Declarations{})
	Hold("response:licenseKey", "918273645501")
	doc := "{\n  \"LK7Q2M9X4P8R\": 1,\n  \"pin\": 918273645501,\n  \"ok\": [true, null, 2.5]\n}"
	res, out := callTool(t, toolReturning(mcp.NewToolResultText(doc)))
	if res.IsError {
		t.Fatalf("withheld: %s", out)
	}
	assertNotContains(t, out, "918273645501")
	text := res.Content[0].(mcp.TextContent).Text
	want := "{\n  \"LK7Q2M9X4P8R\": 1,\n  \"pin\": \"[redacted]\",\n  \"ok\": [\n    true,\n    null,\n    2.5\n  ]\n}"
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
	const structPw = "structPw7Lm3Nq9" // a synthetic fixture
	Hold("response:password", structPw)
	res := mcp.NewToolResultStructured(map[string]any{"name": "p", "password": structPw}, `{"name":"p","password":"[redacted]"}`)
	got, out := callTool(t, toolReturning(res))
	if got.IsError {
		t.Fatalf("withheld: %s", out)
	}
	assertNotContains(t, out, "structPw7Lm3Nq9")
	if !strings.Contains(out, `"password":"[redacted]"`) || !strings.Contains(out, `"name":"p"`) {
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

// encodeString agrees with encoding/json on every rune class, without HTML
// escaping.
func TestRedactEncodeString(t *testing.T) {
	for _, s := range []string{"", "plain", `q"b\s`, "nl\ncr\rtab\t\x01\x1f", "<a>&amp;", "\u2028\u2029", "ünïcødé 😀", "bad\xffutf8"} {
		got, _ := encodeString(s)
		var back string
		if err := json.Unmarshal(got, &back); err != nil {
			t.Fatalf("%q: invalid JSON %s: %v", s, got, err)
		}
		if want := strings.ToValidUTF8(s, "\ufffd"); back != want {
			t.Errorf("%q: round trip = %q", s, back)
		}
		if strings.Contains(s, "<") && !strings.Contains(string(got), "<") {
			t.Errorf("%q: HTML escaped: %s", s, got)
		}
	}
}

// A content piece that is clean on its own can still be withheld inside the
// result's envelope (here a run of members across pieces): no leaf carries
// it, so the result is withheld whole.
func TestRedactMCPToolResultResidualUsesWholeResult(t *testing.T) {
	resetForTest(Declarations{})
	Hold("response:password", strings.Repeat("\x00", 8))
	clean := mcp.NewToolResultText(`{"ok":true}`)
	spanning := mcp.TextContent{Type: "text", Text: `{"z":[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0]}`}
	clean.Content = append(clean.Content, spanning)
	b, _ := json.Marshal(clean)
	if !withheld(b) {
		t.Fatal("the matcher does not assemble this value: the test proves nothing")
	}
	res, out := callTool(t, toolReturning(clean))
	if !res.IsError || strings.Contains(out, Redacted) {
		t.Errorf("result = %s", out)
	}
}

// A result the check could not finish (exhausted, here past maxUnit) is not a
// match: it is withheld whole, never redacted. Redacting it masked ordinary
// leaves until the result fit, and bisected for minutes on a large one.
func TestRedactMCPToolResultExhaustedWithheldWhole(t *testing.T) {
	resetForTest(Declarations{})
	Hold("env:X", "notInThisResult9Kq")
	defer func(u int) { maxUnit = u }(maxUnit)
	maxUnit = 400
	doc := `{"a":"` + strings.Repeat("a", 200) + `","b":"` + strings.Repeat("b", 200) + `","c":"` + strings.Repeat("c", 200) + `"}`
	res, out := callTool(t, toolReturning(mcp.NewToolResultText(doc)))
	if !res.IsError || strings.Contains(out, Redacted) {
		t.Errorf("an exhausted result was redacted: %s", out)
	}
}

// A held key is not masked: two masked sibling keys would collide and a JSON
// reader keeps only the last member. The result is withheld whole.
func TestRedactMCPToolResultHeldKeyWithheldWhole(t *testing.T) {
	resetForTest(Declarations{})
	Hold("response:licenseKey", "LK7Q2M9X4P8R", "M7R2Q8L9V4T6")
	doc := `{"LK7Q2M9X4P8R":{"host":"srv-a","backupOk":false},"M7R2Q8L9V4T6":{"host":"srv-b","backupOk":true}}`
	res, out := callTool(t, toolReturning(mcp.NewToolResultText(doc)))
	assertNotContains(t, out, "LK7Q2M9X4P8R", "M7R2Q8L9V4T6")
	if !res.IsError || strings.Contains(out, Redacted) {
		t.Errorf("a result with held keys was redacted: %s", out)
	}
}

// JSON nested past maxRedactDepth is withheld whole, not tokenized.
func TestRedactMCPToolResultDeepNestingWithheldWhole(t *testing.T) {
	resetForTest(Declarations{})
	Hold("response:password", "deepNestPw4Kq8z2")
	doc := strings.Repeat("[", maxRedactDepth+1) + `"deepNestPw4Kq8z2"` + strings.Repeat("]", maxRedactDepth+1)
	res, out := callTool(t, toolReturning(mcp.NewToolResultText(doc)))
	assertNotContains(t, out, "deepNestPw4Kq8z2")
	if !res.IsError || strings.Contains(out, Redacted) {
		t.Errorf("a too-deep result was redacted: %s", out)
	}
}

// A leaf whose own check cannot finish (exhausted) is not a found match:
// masking it would mark ordinary data as a credential, so the result is
// withheld whole.
func TestRedactMCPToolResultExhaustedLeafWithheldWhole(t *testing.T) {
	resetForTest(Declarations{})
	Hold("env:X", "exhaustLeafPw4Kq8z2")
	defer func(d int) { maxDepth = d }(maxDepth)
	nested := "ordinary text"
	for range 8 {
		nested = base64.StdEncoding.EncodeToString([]byte(nested))
	}
	maxDepth = 6 // the leaf alone exhausts; the envelope with it masked does not
	if verdictOf([]byte(nested)) != exhausted {
		t.Fatal("the fixture leaf does not exhaust: the test proves nothing")
	}
	doc := `{"p":"exhaustLeafPw4Kq8z2","pad":"` + nested + `"}`
	if b, _ := json.Marshal(mcp.NewToolResultText(doc)); verdictOf(b) != leak {
		t.Fatal("the fixture result is not a leak: the test proves nothing")
	}
	res, out := callTool(t, toolReturning(mcp.NewToolResultText(doc)))
	assertNotContains(t, out, "exhaustLeafPw4Kq8z2")
	if !res.IsError || strings.Contains(out, Redacted) {
		t.Errorf("a result with an exhausted leaf was redacted: %s", out)
	}
}

// A held value the decoders assemble across leaves (here character codes) is
// in no leaf on its own. Masking only the leaf that completes the match left
// the other seven characters readable, so such a result is withheld whole.
func TestRedactMCPToolResultCrossLeafMatchWithheldWhole(t *testing.T) {
	resetForTest(Declarations{})
	Hold("response:password", "p@ssw0rd")
	doc := `{"id":"n1","note":[112,64,115,115,119,48,114,100]}`
	if b, _ := json.Marshal(mcp.NewToolResultText(doc)); verdictOf(b) != leak {
		t.Fatal("the matcher does not assemble the codes: the test proves nothing")
	}
	res, out := callTool(t, toolReturning(mcp.NewToolResultText(doc)))
	if !res.IsError || strings.Contains(out, Redacted) || strings.Contains(out, "115,119,48") {
		t.Errorf("a cross-leaf match was partly redacted: %s", out)
	}
}
