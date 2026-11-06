// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package crowdstrikeblackbox

// Classes the exchange-online Codex review round 1 found in this shared layer,
// each proven on the built binaries:
//   - outbound-sink-bypasses-chokepoint: every request this program sends to
//     a host other than the pinned API (feedback --send) leaves through the
//     credential kit's guard;
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
	t.Parallel()
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
	// The text repeats a credential the process holds from its start in each
	// mode: the stored token, the client secret, the cached minted token.
	for _, mode := range []credMode{credStored, credPair, credPairCached} {
		mu.Lock()
		posted.Reset()
		mu.Unlock()
		sv := echoServer(t)
		e := newEnv(t, sv, mode)
		tok := mode.held()
		e.env = append(e.env, "CROWDSTRIKE_FEEDBACK_ENDPOINT="+fb.URL+"/feedback")
		out := e.run(t, "feedback", "--send", "the token "+tok+" and "+bbHTTPToken+" leaked")
		out += e.runIn(t, "pasted "+tok+"\n", "feedback", "--send", "--stdin")
		out += e.run(t, "feedback", "--send", "an ordinary note")
		mu.Lock()
		got := posted.String()
		mu.Unlock()
		// Only the clean entry is posted: a body that repeats a held value is
		// refused whole by the kit's outbound guard. (feedback --stdin: the kit
		// holds every line of a --stdin body, StdinFlags, so even a clean pasted
		// note is held; .review/followups.md.)
		if strings.Count(got, `"cli":"crowdstrike-cli"`) != 1 || !strings.Contains(got, "an ordinary note") {
			t.Fatalf("%s: want the one clean entry posted:\nposted: %.1000s\noutput: %.1000s", mode, got, out)
		}
		if !strings.Contains(out, withheldNotice) {
			t.Errorf("%s: the refused sends printed no kit notice:\n%.1500s", mode, out)
		}
		assertNoEcho(t, mode.String()+": feedback POST body", got)
		assertNoEcho(t, mode.String()+": feedback output", out)
	}
}

// Class mcp-protocol-output-bypasses-chokepoint, stdio: mcp-go answers an
// unknown tool or prompt name with a JSON-RPC error that repeats the name,
// before any tool middleware runs.
func TestMCPProtocolErrorsWithheld(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credStored, credPair} {
		sv := echoServer(t)
		e := newEnv(t, sv, mode)
		out := e.mcpCalls(t, []map[string]any{
			{"name": mode.held(), "arguments": map[string]any{}},
			{"name": "tool-" + bbHTTPToken, "arguments": map[string]any{}},
		})
		out += e.mcpRaw(t, []map[string]any{
			{"jsonrpc": "2.0", "id": 1, "method": "prompts/get", "params": map[string]any{"name": mode.held()}},
		})
		if !strings.Contains(out, `"error"`) {
			t.Fatalf("%s: no JSON-RPC error came back; the test did not run:\n%.2000s", mode, out)
		}
		assertNoEcho(t, mode.String()+": MCP protocol errors", out)
		assertJSONLines(t, out)
	}
}

// Class mcp-protocol-output-bypasses-chokepoint, streamable HTTP.
func TestMCPHTTPProtocolErrorsWithheld(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credPair)
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
	_, a := post(session, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": bbClientSecret, "arguments": map[string]any{}}})
	_, b := post(session, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "prompts/get", "params": map[string]any{"name": bbClientSecret}})
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
// JSON after the kit judged it.
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

// httpClientSite matches every way this program builds an HTTP client or sends
// through one it did not build.
var httpClientSite = regexp.MustCompile(`http\.Client\{|http\.DefaultClient|http\.DefaultTransport\.RoundTrip|http\.(Get|Post|PostForm|Head)\(`)

// unguardedClients are the sites that do not name credkit.Transport on the line,
// with the reason; the key is the file and a fragment of the line.
var unguardedClients = map[string]string{
	"internal/client/client.go|&http.Client{Timeout: timeout, Jar: jar, Transport: tr}":             "the base client.New and NewPlainHTTPClient build: ApplyClientHooks (guardClient) or guardTokenClient wraps its transport in the origin pin over cliutil.OutboundGuard, the kit's transport (crowdstrike_guard.go installPins; TestGuardWrapsTheTransport)",
	"internal/client/client.go|&http.Client{Transport: tr}":                                         "StreamingHTTPClient with a nil base; its one caller passes the guarded client",
	"internal/cliutil/oauth_token.go|c := &http.Client{Timeout: oauthTokenHTTPClientTimeout}":       "OAuthTokenHTTPClient: its transport is wrapped in OutboundGuard (the kit's transport) two lines on",
	"internal/cli/fleet_sync.go|cliutil.OAuthTokenHTTPClient(&http.Client{Timeout: flags.timeout})": "the member_cid mint client: OAuthTokenHTTPClient wraps it in OutboundGuard, and guardTokenClient pins it",
	"internal/cli/deliver.go|httpClient := &http.Client{":                                           "its Transport is credkit.Transport(nil), the next line",
	"internal/cliutil/probe.go|http.DefaultClient has no Timeout":                                   "a comment",
}

// TestEveryHTTPClientIsGuarded: a sender a reprint adds fails here until it
// leaves through the credential kit's outbound guard or is reviewed above
// (handfixes.json credkit-wiring).
func TestEveryHTTPClientIsGuarded(t *testing.T) {
	t.Parallel()
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
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range strings.Split(string(src), "\n") {
				code := strings.TrimSpace(line)
				if strings.HasPrefix(code, "//") || !httpClientSite.MatchString(code) {
					continue
				}
				sites++
				if strings.Contains(code, "credkit.Transport(") {
					continue
				}
				reviewed := false
				for key := range unguardedClients {
					file, frag, _ := strings.Cut(key, "|")
					reviewed = reviewed || file == rel && strings.Contains(code, frag)
				}
				if !reviewed {
					t.Errorf("%s:%d builds an HTTP client without the credential kit's guard: %s", rel, i+1, code)
				}
			}
			return nil
		})
	}
	if sites < 5 {
		t.Fatalf("found only %d client sites; the scan is not reading the tree", sites)
	}
}
