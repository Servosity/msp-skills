// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package keeperblackbox

// Classes the exchange-online Codex review round 1 found, each proven on the built
// binaries:
//   - outbound-sink-bypasses-chokepoint: every request this program sends to
//     a host other than the pinned API (feedback --send) passes the kit;
//   - mcp-protocol-output-bypasses-chokepoint: every JSON-RPC message the MCP
//     server writes (protocol errors that echo a tool or prompt name included)
//     passes the kit, on stdio and on streamable HTTP.

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// Class outbound-sink-bypasses-chokepoint: feedback --send posts to an
// operator-configured endpoint, which is not the pinned API host.
func TestFeedbackSendWithholdsCredentials(t *testing.T) {
	var mu sync.Mutex
	var posted bytes.Buffer
	fb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		posted.Write(b)
		posted.WriteString("\n")
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(fb.Close)
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	e.env = append(e.env, "KEEPER_FEEDBACK_ENDPOINT="+fb.URL+"/feedback")
	out := e.run(t, "feedback", "--send", "the token "+bbToken+" and "+bbHTTPToken+" leaked")
	out += e.runIn(t, "pasted "+bbToken+"\n", "feedback", "--send", "--stdin")
	mu.Lock()
	got := posted.String()
	mu.Unlock()
	// A body that repeats a held value is never posted: the kit refuses the whole
	// request, and the refusal says so.
	if got != "" {
		t.Errorf("the feedback endpoint received a body that repeats a held value:\n%.1000s", got)
	}
	if !strings.Contains(out, bbWithheld) {
		t.Errorf("want the kit's notice for the refused sends, got:\n%.1000s", out)
	}
	assertNoEcho(t, "feedback output", out)
	// A clean note arrives whole, so the refusals above are the kit, not a broken sender.
	e.run(t, "feedback", "--send", "a clean note")
	mu.Lock()
	got = posted.String()
	mu.Unlock()
	if strings.Count(got, `"cli":"keeper-cli"`) != 1 || !strings.Contains(got, "a clean note") {
		t.Fatalf("the feedback endpoint did not receive the clean note:\n%.1000s", got)
	}
}

// Class mcp-protocol-output-bypasses-chokepoint, stdio: mcp-go answers an
// unknown tool or prompt name with a JSON-RPC error that repeats the name,
// before any tool middleware runs.
func TestMCPProtocolErrorsWithheld(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	out := e.mcpCalls(t, []map[string]any{
		{"name": bbToken, "arguments": map[string]any{}},
		{"name": "tool-" + bbHTTPToken, "arguments": map[string]any{}},
	})
	out += e.mcpRaw(t, []map[string]any{
		{"jsonrpc": "2.0", "id": 1, "method": "prompts/get", "params": map[string]any{"name": bbToken}},
	})
	if !strings.Contains(out, `"error"`) {
		t.Fatalf("no JSON-RPC error came back; the test did not run:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP protocol errors", out)
	assertJSONLines(t, out)
}

// Class mcp-protocol-output-bypasses-chokepoint, streamable HTTP.
func TestMCPHTTPProtocolErrorsWithheld(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	cmd := exec.Command(bbMCP, "--transport", "http", "--addr", addr)
	cmd.Env, cmd.Dir = e.env, e.home
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	base := "http://" + addr + "/mcp"
	post := func(session string, msg map[string]any) (string, string) {
		b, _ := json.Marshal(msg)
		req, _ := http.NewRequest(http.MethodPost, base, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+bbHTTPToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if session != "" {
			req.Header.Set("Mcp-Session-Id", session)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", ""
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.Header.Get("Mcp-Session-Id"), string(body)
	}
	init := map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}}
	var session, initBody string
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if session, initBody = post("", init); initBody != "" {
			break
		}
	}
	if initBody == "" {
		t.Fatalf("MCP HTTP server never answered:\n%s", errb.String())
	}
	_, a := post(session, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": bbToken, "arguments": map[string]any{}}})
	_, b := post(session, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "prompts/get", "params": map[string]any{"name": bbToken}})
	out := a + "\n" + b
	if !strings.Contains(out, `"error"`) {
		t.Fatalf("no JSON-RPC error came back; the test did not run:\n%.2000s\n%s", out, errb.String())
	}
	assertNoEcho(t, "MCP HTTP protocol errors", out+"\n"+errb.String())
}

var jsonRPCLine = regexp.MustCompile(`^\{.*"jsonrpc"`)

// assertJSONLines checks every JSON-RPC line the server wrote is still valid
// JSON after the kit read it.
func assertJSONLines(t *testing.T, out string) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if !jsonRPCLine.MatchString(line) {
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Errorf("a JSON-RPC line is no longer JSON (%v): %.300s", err, line)
		}
	}
}
