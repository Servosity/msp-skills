// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package dsblackbox

// Codex review round 1 classes, each proven on the built binaries:
//   - preview-overridable-by-argument: a caller argument cannot turn a
//     confirm-gate preview into a send (sso-link "false", 0, false);
//   - outbound-sink-bypasses-chokepoint: every request this program sends to
//     a host other than the pinned API (feedback --send) carries no held value;
//   - mcp-protocol-output-bypasses-chokepoint: every JSON-RPC message the MCP
//     server writes (protocol errors that echo a tool or prompt name included)
//     passes the credential kit, on stdio (the server has no HTTP mode).

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// Class preview-overridable-by-argument: a caller value for a gated flag
// (sso-link "false", 0, false) or a non-boolean confirm cannot turn a preview
// into a send; no sign-in link is minted.
func TestMCPPreviewCannotBeTurnedOffByAnArgument(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	var calls []map[string]any
	for _, v := range []any{"false", false, 0.0, "0", "FALSE"} {
		calls = append(calls,
			map[string]any{"name": "locate", "arguments": map[string]any{"email": "hq@example.com", "sso-link": v, "json": true}},
			map[string]any{"name": "evidence", "arguments": map[string]any{"org": bbOrg, "with-sso-link": v, "confirm": "true", "json": true}},
			map[string]any{"name": "portal_get-login-link", "arguments": map[string]any{"dry-run": v, "confirm": v}})
	}
	out := e.mcpCalls(t, calls)
	sv.mu.Lock()
	minted := sv.ssoMint
	sv.mu.Unlock()
	if minted != 0 {
		t.Errorf("unconfirmed calls minted %d sign-in link(s)", minted)
	}
	if !strings.Contains(out, "owner@example.com") {
		t.Fatalf("the locate/evidence previews read nothing; the test did not run:\n%.2000s", out)
	}
	if n := strings.Count(out, "Preview only") + strings.Count(out, `\"preview\": true`); n < len(calls) {
		t.Errorf("want %d previews, got %d:\n%.3000s", len(calls), n, out)
	}
}

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
	e.env = append(e.env, "DROPSUITE_FEEDBACK_ENDPOINT="+fb.URL+"/feedback")
	out := e.run(t, "feedback", "--send", "the token "+bbAccess+" and "+bbHTTPToken+" leaked")
	out += e.runIn(t, "pasted "+bbAccess+"\n", "feedback", "--send", "--stdin")
	// A note with no held value is posted, so the endpoint is reachable and
	// the two above were refused by the kit, not lost.
	out += e.run(t, "feedback", "--send", "a clean note")
	mu.Lock()
	got := posted.String()
	mu.Unlock()
	if strings.Count(got, `"cli":"dropsuite-cli"`) != 1 || !strings.Contains(got, "a clean note") {
		t.Fatalf("want exactly the clean note posted; the test did not run:\nposted: %.1000s\noutput: %.1000s", got, out)
	}
	assertNoEcho(t, "feedback POST body", got)
	assertNoEcho(t, "feedback output", out)
}

// Class mcp-protocol-output-bypasses-chokepoint, stdio: mcp-go answers an
// unknown tool or prompt name with a JSON-RPC error that repeats the name,
// before any tool middleware runs.
func TestMCPProtocolErrorsWithheld(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	out := e.mcpCalls(t, []map[string]any{
		{"name": bbAccess, "arguments": map[string]any{}},
		{"name": "tool-" + bbHTTPToken, "arguments": map[string]any{}},
	})
	out += e.mcpRaw(t, []map[string]any{
		{"jsonrpc": "2.0", "id": 1, "method": "prompts/get", "params": map[string]any{"name": bbAccess}},
	})
	if !strings.Contains(out, `"error"`) {
		t.Fatalf("no JSON-RPC error came back; the test did not run:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP protocol errors", out)
	assertJSONLines(t, out)
}

// mcpRaw runs one MCP stdio session with raw JSON-RPC messages after the
// handshake and returns everything the server wrote.
func (e *bbEnv) mcpRaw(t *testing.T, msgs []map[string]any) string {
	t.Helper()
	var in bytes.Buffer
	enc := json.NewEncoder(&in)
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}})
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	for _, m := range msgs {
		_ = enc.Encode(m)
	}
	cmd := exec.Command(bbMCP)
	cmd.Env, cmd.Dir = e.env, e.home
	cmd.Stdin = &in
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
	}
	return out.String() + "\n" + errb.String()
}

var jsonRPCLine = regexp.MustCompile(`^\{.*"jsonrpc"`)

// assertJSONLines checks every JSON-RPC line the server wrote is still valid
// JSON after the chokepoint rewrote it.
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

// networkSend matches every way this program builds an outgoing HTTP request.
