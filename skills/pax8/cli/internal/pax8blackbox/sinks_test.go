// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package pax8blackbox

// Classes the exchange-online Codex review round 1 found in this shared layer,
// each proven on the built binaries:
//   - outbound-sink-bypasses-chokepoint: every request this program sends to
//     a host other than the pinned API (feedback --send) passes the chokepoint;
//   - mcp-protocol-output-bypasses-chokepoint: every JSON-RPC message the MCP
//     server writes (protocol errors that echo a tool or prompt name included)
//     passes the chokepoint, on stdio and on streamable HTTP.

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
	e := newEnv(t, sv, true)
	e.env = append(e.env, "PAX8_FEEDBACK_ENDPOINT="+fb.URL+"/feedback")
	out := e.run(t, "feedback", "--send", "the token "+bbToken+" and "+bbHTTPToken+" leaked")
	e.runIn(t, "pasted "+bbToken+"\n", "feedback", "--send", "--stdin")
	mu.Lock()
	got := posted.String()
	mu.Unlock()
	// Both entries were posted (their text is withheld whole: the chokepoint
	// replaces a JSON string that repeats a held value).
	if strings.Count(got, `"cli":"pax8-cli"`) != 2 {
		t.Fatalf("the feedback endpoint received nothing; the test did not run:\nposted: %.1000s\noutput: %.1000s", got, out)
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
var networkSend = regexp.MustCompile(`http\.NewRequest\(|http\.NewRequestWithContext\(|http\.Post\(|http\.PostForm\(|http\.Get\(`)

// unfilteredSends are the request sites that may skip the chokepoint, with the
// reason; the key is the file and a fragment of the line. Only requests to the
// pinned API origin belong here: every other site must send through
// cliutil.SendOutbound (outboundSend below).
var unfilteredSends = map[string]string{
	"internal/client/client.go|http.NewRequestWithContext(ctx, method, targetURL, bodyReader)":                                 "the API client: pinned to the configured origin (pax8_guard.go); its params and body are held (HoldRequestCredentials) before a preview or a send",
	"internal/cliutil/probe.go|http.NewRequestWithContext(ctx, \"GET\", url, nil)":                                             "doctor reachability probe: a bodiless GET of the configured API base URL",
	"internal/cli/auth.go|http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))":                        "auth login: the client credentials go to the configured token URL only (guardTokenClient: that exact path, POST, no Authorization), redirects stay on its origin, and the minted token is held before anything prints",
	"internal/client/client.go|http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))":   "the automatic client-credentials mint: pinned to the configured token URL (pax8_guard.go), same-origin redirects only (OAuthTokenHTTPClient), the minted token held by the response hold",
	"internal/client/client.go|http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(params.Encode()))": "the refresh grant: pinned like the mint; the refresh token and client secret it sends are held from config load",
	"internal/cliutil/pax8_outbound.go|http.NewRequest(method, u.String(), bytes.NewReader(body))":                             "SendOutbound itself: the URL and headers were just checked, and every caller withholds the body first (its call sites are checked by this scan)",
}

// bodyWithheldByCaller are non-API sites whose body the only caller withholds;
// they still must send through cliutil.SendOutbound.
var bodyWithheldByCaller = map[string]string{
	"internal/cli/deliver.go|http.NewRequest(http.MethodPost, url, bytes.NewReader(body))": "deliverWebhook is reached only through Deliver, which withholds the body first (cliutil.WithholdBytes)",
}

// outboundSend is the one sender for non-API hosts (Codex r2: the URL and
// headers of a non-API request must pass the held-value check too).
var outboundSend = regexp.MustCompile(`cliutil\.SendOutbound\(`)

// TestEveryNetworkSendPassesTheChokepoint: every request body sent anywhere
// but the pinned API passes the chokepoint (pax8 handfix:
// output-chokepoint, network sinks), so a sender a reprint adds fails here
// until it is routed through cliutil.WithholdBytes or reviewed above.
func TestEveryNetworkSendPassesTheChokepoint(t *testing.T) {
	root := filepath.Join("..", "..")
	var sites int
	for _, dir := range []string{"internal", "cmd"} {
		_ = filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
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
				listed := func(m map[string]string) bool {
					for key := range m {
						file, frag, _ := strings.Cut(key, "|")
						if file == rel && strings.Contains(code, frag) {
							return true
						}
					}
					return false
				}
				if listed(unfilteredSends) {
					continue
				}
				// The chokepoint and the sender must sit in the sending function itself:
				// a mark in a neighbouring function does not count.
				start, end := i, i
				for start > 0 && !strings.HasPrefix(lines[start], "func ") {
					start--
				}
				for end < len(lines)-1 && lines[end] != "}" {
					end++
				}
				if !listed(bodyWithheldByCaller) && !chokepointMark.MatchString(strings.Join(lines[start:i+1], "\n")) {
					t.Errorf("%s:%d sends a request body without the chokepoint: %s", rel, i+1, code)
				}
				if !outboundSend.MatchString(strings.Join(lines[i:end+1], "\n")) {
					t.Errorf("%s:%d sends to a non-API host without cliutil.SendOutbound (URL and header check, no redirects): %s", rel, i+1, code)
				}
			}
			return nil
		})
	}
	if sites < 3 {
		t.Fatalf("found only %d request sites; the scan is not reading the tree", sites)
	}
}
