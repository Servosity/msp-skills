// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

// Classes the exchange-online Codex review round 1 found in the connector's
// former credential layer, each proven on the built binaries against the
// shared credential kit:
//   - outbound-sink-bypasses-chokepoint: every request this program sends to
//     a host other than the API (feedback --send) passes the kit's outbound
//     guard;
//   - mcp-protocol-output-bypasses-chokepoint: every JSON-RPC message the MCP
//     server writes (protocol errors that echo a tool or prompt name included)
//     passes the chokepoint, on stdio and on streamable HTTP.

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
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
	budget(t)
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
	e.env = append(e.env, "REWST_FEEDBACK_ENDPOINT="+fb.URL+"/feedback")
	out := e.run(t, "feedback", "--send", "the key "+bbToken+", its base64 "+base64.StdEncoding.EncodeToString([]byte(bbToken))+" and "+bbHTTPToken+" leaked")
	out += e.runIn(t, "pasted "+bbToken+"\n", "feedback", "--send", "--stdin")
	out += e.run(t, "feedback", "--send", "the stored token "+bbStored)
	out += e.run(t, "feedback", "--send", "the HTTP token "+bbHTTPToken+" and its hex "+hex.EncodeToString([]byte(bbHTTPToken)))
	// The kit's outbound guard refuses every credential-bearing entry (and
	// its file sink records the kit's notice in place of the entry).
	if strings.Count(out, "[credkit: withheld from outbound request: ") != 4 {
		t.Errorf("want every credential-bearing entry refused:\n%.1500s", out)
	}
	// A control entry without a credential is posted, so the sink works.
	out += e.run(t, "feedback", "--send", "control HQ-Feedback")
	mu.Lock()
	got := posted.String()
	mu.Unlock()
	if strings.Count(got, `"cli":"rewst-cli"`) != 1 || !strings.Contains(got, "HQ-Feedback") {
		t.Fatalf("the feedback endpoint did not receive exactly the control entry:\nposted: %.1000s\noutput: %.1000s", got, out)
	}
	assertNoEcho(t, "feedback POST body", got)
	assertNoEcho(t, "feedback output", out)
}

// Class mcp-protocol-output-bypasses-chokepoint, stdio: mcp-go answers an
// unknown tool or prompt name with a JSON-RPC error that repeats the name,
// before any tool middleware runs.
func TestMCPProtocolErrorsWithheld(t *testing.T) {
	budget(t)
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvOverStored)
	out := e.mcpCalls(t, []map[string]any{
		{"name": bbToken, "arguments": map[string]any{}},
		{"name": "tool-" + bbHTTPToken, "arguments": map[string]any{}},
		{"name": bbStored, "arguments": map[string]any{}},
		{"name": base64.StdEncoding.EncodeToString([]byte(bbToken)), "arguments": map[string]any{}},
		{"name": hex.EncodeToString([]byte(bbToken)), "arguments": map[string]any{}},
		{"name": "tool-" + base64.StdEncoding.EncodeToString([]byte(bbStored)), "arguments": map[string]any{}},
	})
	out += e.mcpRaw(t, []map[string]any{
		{"jsonrpc": "2.0", "id": 1, "method": "prompts/get", "params": map[string]any{"name": bbToken}},
		{"jsonrpc": "2.0", "id": 2, "method": "prompts/get", "params": map[string]any{"name": bbStored}},
	})
	if !strings.Contains(out, `"error"`) {
		t.Fatalf("no JSON-RPC error came back; the test did not run:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP protocol errors", out)
	assertJSONLines(t, out)
}

// Class mcp-protocol-output-bypasses-chokepoint, streamable HTTP.
func TestMCPHTTPProtocolErrorsWithheld(t *testing.T) {
	budget(t)
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
	_, d := post(session, map[string]any{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": map[string]any{"name": base64.RawURLEncoding.EncodeToString([]byte(bbToken)), "arguments": map[string]any{}}})
	out := a + "\n" + b + "\n" + c + "\n" + d
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

// unguardedSends are the request sites whose client is built elsewhere, with
// the reason; the key is the file and a fragment of the line. Every other
// site must send through a client whose transport is the credential kit's
// (credkit.Transport in the sending function).
var unguardedSends = map[string]string{
	"internal/client/client.go|http.NewRequestWithContext(ctx, method, targetURL, bodyReader)": "the API client: New wraps its transport in OutboundGuard (credkit.Transport) and every hook keeps it beneath (rewst_guard.go)",
	"internal/cliutil/probe.go|http.NewRequestWithContext(ctx, \"GET\", url, nil)":             "press template ProbeReachable: a bodiless GET carrying only a Range header, no credential; no non-test caller in this tree (doctor does not use it)",
}

// kitTransport is a client transport the credential kit guards.
var kitTransport = regexp.MustCompile(`credkit\.Transport\(`)

// TestEveryNetworkSendPassesTheChokepoint: every request this program builds
// is sent by a client whose transport is the credential kit's outbound guard,
// so a sender a reprint adds fails here until it is routed through
// credkit.Transport or reviewed above.
func TestEveryNetworkSendPassesTheChokepoint(t *testing.T) {
	budget(t)
	root := filepath.Join("..", "..")
	var sites int
	for _, dir := range []string{"internal", "cmd"} {
		_ = filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(rel, "internal/credkit/") {
				return nil // the kit itself (vendored, guard 11)
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
				for key := range unguardedSends {
					file, frag, _ := strings.Cut(key, "|")
					listed = listed || (file == rel && strings.Contains(code, frag))
				}
				if listed {
					continue
				}
				// The guarded client must be built in the sending function itself.
				start, end := i, i
				for start > 0 && !strings.HasPrefix(lines[start], "func ") {
					start--
				}
				for end < len(lines)-1 && lines[end] != "}" {
					end++
				}
				if !kitTransport.MatchString(strings.Join(lines[start:end+1], "\n")) {
					t.Errorf("%s:%d sends a request without the credential kit's transport: %s", rel, i+1, code)
				}
			}
			return nil
		})
	}
	if sites < 3 {
		t.Fatalf("found only %d request sites; the scan is not reading the tree", sites)
	}
}

// A withheld request is withheld again on replay (the kit judges the same bytes
// the same way), so the client stops at the first refusal: no retry line and no
// backoff. The MCP HTTP token may go nowhere, so a request carrying it is withheld.
func TestWithheldRequestIsNotRetried(t *testing.T) {
	budget(t)
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	out := e.run(t, "sync", "--resources", "pages", "--param", "note="+bbHTTPToken)
	if !kitWithheld(out) {
		t.Fatalf("the request was not withheld; the test did not run:\n%.1500s", out)
	}
	// The retry line repeats the URL, so the kit withholds it too: one stderr
	// notice per retry.
	if strings.Contains(out, "retrying in") || strings.Count(out, "[credkit: withheld from stderr: ") > 1 {
		t.Errorf("a withheld request was retried:\n%.1500s", out)
	}
	if m := sv.allMethods(); len(m) != 0 {
		t.Errorf("a withheld request reached the fake: %v", m)
	}
}

// Class credential-on-stdin (leak contract section 1, adopter item 8a): a
// --stdin body is held whole before anything prints, so a value only the body
// carries never prints, in a preview or in an answer. The only test that fails
// when StdinFlags is not declared: everywhere else the body repeats a value
// another source already holds.
func TestStdinBodyIsHeld(t *testing.T) {
	budget(t)
	const onlyInBody = "stdinOnlySecret-5d9e2b7c41"
	e := newEnv(t, echoServer(t), credEnv).lean()
	for _, args := range [][]string{
		{"org-variables", "create", "--stdin", "--dry-run"},
		{"org-variables", "create", "--stdin", "--yes", "--json"},
	} {
		out := e.runIn(t, orgVarBody(onlyInBody), args...)
		if strings.Contains(out, onlyInBody) {
			t.Errorf("%v repeats the --stdin body's value:\n%.1500s", args, out)
		}
		if !kitWithheld(out) {
			t.Errorf("%v: want the kit's notice in place of what repeats the body:\n%.1500s", args, out)
		}
	}
}
