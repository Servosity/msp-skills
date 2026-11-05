// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package betterstackblackbox

// Classes the exchange-online Codex review round 1 found in this shared layer,
// each proven on the built binaries:
//   - outbound-sink-bypasses-chokepoint: every request this program sends to
//     a host other than the pinned API (feedback --send) passes the chokepoint;
//   - mcp-protocol-output-bypasses-chokepoint: every JSON-RPC message the MCP
//     server writes (protocol errors that echo a tool or prompt name included)
//     passes the chokepoint, on stdio and on streamable HTTP.

import (
	"bytes"
	"encoding/base64"
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
	e := newEnv(t, sv, credEnvOverStored)
	e.env = append(e.env, "BETTERSTACK_FEEDBACK_ENDPOINT="+fb.URL+"/feedback")
	out := e.run(t, "feedback", "--send", "the key "+bbToken+", its base64 "+base64.StdEncoding.EncodeToString([]byte(bbToken))+" and "+bbHTTPToken+" leaked")
	out += e.runIn(t, "pasted "+bbToken+"\n", "feedback", "--send", "--stdin")
	out += e.run(t, "feedback", "--send", "the stored token "+bbStored)
	// Feedback text that repeats a held credential is withheld by the
	// credential kit from feedback.jsonl and from the POST; --stdin is refused
	// (stdin is held whole). Only the control entry below may arrive.
	// A control entry without a credential is posted, so the sink works.
	out += e.run(t, "feedback", "--send", "control HQ-Feedback")
	mu.Lock()
	got := posted.String()
	mu.Unlock()
	if strings.Count(got, `"cli":"betterstack-cli"`) != 1 || !strings.Contains(got, "HQ-Feedback") {
		t.Fatalf("the feedback endpoint did not receive exactly the control entry:\nposted: %.1000s\noutput: %.1000s", got, out)
	}
	assertNoEcho(t, "feedback POST body", got)
	assertNoEcho(t, "feedback output", out)
}

// Class mcp-protocol-output-bypasses-chokepoint, stdio: mcp-go answers an
// unknown tool or prompt name with a JSON-RPC error that repeats the name,
// before any tool middleware runs.
func TestMCPProtocolErrorsWithheld(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvOverStored)
	out := e.mcpCalls(t, []map[string]any{
		{"name": bbToken, "arguments": map[string]any{}},
		{"name": "tool-" + bbHTTPToken, "arguments": map[string]any{}},
		{"name": bbStored, "arguments": map[string]any{}},
		{"name": base64.StdEncoding.EncodeToString([]byte(bbToken)), "arguments": map[string]any{}},
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
	e := newEnv(t, sv, credEnvOverStored)
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
	_, c := post(session, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": bbStored, "arguments": map[string]any{}}})
	out := a + "\n" + b + "\n" + c
	if !strings.Contains(out, `"error"`) {
		t.Fatalf("no JSON-RPC error came back; the test did not run:\n%.2000s\n%s", out, errb.String())
	}
	assertNoEcho(t, "MCP HTTP protocol errors", out+"\n"+errb.String())
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
