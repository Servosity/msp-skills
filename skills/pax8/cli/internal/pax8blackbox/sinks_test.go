// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package pax8blackbox

// Classes the exchange-online Codex review round 1 found, each proven on the
// built binaries:
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
	"os"
	"os/exec"
	"path/filepath"
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
	e := newEnv(t, sv, true).store(t)
	e.env = append(e.env, "PAX8_FEEDBACK_ENDPOINT="+fb.URL+"/feedback")
	out := e.run(t, "feedback", "--send", "the token "+bbToken+" and "+bbHTTPToken+" leaked")
	out += e.runIn(t, "pasted "+bbToken+"\n", "feedback", "--send", "--stdin")
	out += e.run(t, "feedback", "--send", "the stored token "+bbStored)
	// A POST that carries a held credential to the feedback endpoint (not an
	// allowed origin) is withheld by the kit's transport.
	if strings.Count(out, kitNotice("outbound request")) != 3 {
		t.Errorf("want every credential-bearing POST withheld:\n%.1500s", out)
	}
	// A control entry without a credential is posted, so the sink works.
	out += e.run(t, "feedback", "--send", "control HQ-Feedback")
	mu.Lock()
	got := posted.String()
	mu.Unlock()
	if strings.Count(got, `"cli":"pax8-cli"`) != 1 || !strings.Contains(got, "HQ-Feedback") {
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
	e := newEnv(t, sv, true)
	// Each message in its own session: the kit closes a session's output once
	// it withholds a protocol message (kitClosedMCP), so a later message of the
	// same session would never be judged. Each session must show the message
	// reached the server: a JSON-RPC error, or the kit's close.
	var all strings.Builder
	sessions := []string{}
	for _, c := range []map[string]any{
		{"name": bbToken, "arguments": map[string]any{}},
		{"name": "tool-" + bbHTTPToken, "arguments": map[string]any{}},
	} {
		sessions = append(sessions, e.mcpCalls(t, []map[string]any{c}))
	}
	sessions = append(sessions, e.mcpRaw(t, []map[string]any{
		{"jsonrpc": "2.0", "id": 1, "method": "prompts/get", "params": map[string]any{"name": bbToken}},
	}))
	for i, out := range sessions {
		if !strings.Contains(out, `"error"`) && !strings.Contains(out, kitClosedMCP) {
			t.Errorf("session %d: neither a JSON-RPC error nor the kit's close came back; the message did not run:\n%.2000s", i, out)
		}
		all.WriteString(out)
	}
	assertNoEcho(t, "MCP protocol errors", all.String())
	assertJSONLines(t, all.String())
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

// assertJSONLines checks every JSON-RPC line the server wrote is valid JSON.
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
var networkSend = regexp.MustCompile(`http\.NewRequest\(|http\.NewRequestWithContext\(|http\.Post\(|http\.PostForm\(|http\.Get\(`)

// unfilteredSends are the request sites whose transport is set outside the
// sending function, with the reason; the key is the file and a fragment of the
// line. Every other site must build its client with credkit.Transport in the
// sending function itself (kitTransport below).
var unfilteredSends = map[string]string{
	"internal/client/client.go|http.NewRequestWithContext(ctx, method, targetURL, bodyReader)":                                 "the API client: its transport is client.OutboundGuard (credkit.Transport) under the origin pin (pax8_guard.go)",
	"internal/client/client.go|http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))":   "the automatic client-credentials mint: the same client transport (OAuthTokenHTTPClient keeps it)",
	"internal/client/client.go|http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(params.Encode()))": "the refresh grant: the same client transport",
	"internal/cli/auth.go|http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))":                        "auth login: rootFlags.oauthHTTPClient builds its client with NewPlainHTTPClient (client.OutboundGuard) under the token pin",
	"internal/cliutil/probe.go|http.NewRequestWithContext(ctx, \"GET\", url, nil)":                                             "doctor reachability probe: its nil-client fallback is credkit.Transport, and every caller's client is the API client",
}

// kitTransport is how a sending function shows its request passes the shared
// credential kit (handfixes.json credkit-wiring): URL, headers and body are
// checked, and a held value goes only to its declared origin.
var kitTransport = regexp.MustCompile(`credkit\.Transport\(`)

// TestEveryNetworkSendPassesTheKit: every request this program builds is sent
// through credkit.Transport (handfixes.json credkit-wiring, network sinks), so
// a sender a reprint adds fails here until it is routed through the kit or
// reviewed above.
func TestEveryNetworkSendPassesTheKit(t *testing.T) {
	root := filepath.Join("..", "..")
	var sites int
	for _, dir := range []string{"internal", "cmd"} {
		_ = filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(rel, "internal/credkit/") || strings.HasPrefix(rel, "internal/config/pax8_persist.go") {
				return nil // the vendored kit, and the persist filter's probe, which sends nothing
			}
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(src), "\n")
			for i, line := range lines {
				code := strings.TrimSpace(line)
				if strings.HasPrefix(code, "//") || !networkSend.MatchString(code) {
					continue
				}
				sites++
				listed := false
				for key := range unfilteredSends {
					file, frag, _ := strings.Cut(key, "|")
					listed = listed || (file == rel && strings.Contains(code, frag))
				}
				if listed {
					continue
				}
				// The kit's transport must sit in the sending function itself: a
				// mark in a neighbouring function does not count.
				start, end := i, i
				for start > 0 && !strings.HasPrefix(lines[start], "func ") {
					start--
				}
				for end < len(lines)-1 && lines[end] != "}" {
					end++
				}
				if !kitTransport.MatchString(strings.Join(lines[start:end+1], "\n")) {
					t.Errorf("%s:%d sends a request without credkit.Transport in the sending function: %s", rel, i+1, code)
				}
			}
			return nil
		})
	}
	if sites < 3 {
		t.Fatalf("found only %d request sites; the scan is not reading the tree", sites)
	}
}
