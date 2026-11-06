// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package vsphereblackbox

// Classes an earlier review found, each proven on the built binaries:
//   - outbound-sink-bypasses-chokepoint: every request this program sends to
//     a host other than the pinned API (feedback --send) leaves through the
//     credential kit's guard;
//   - mcp-protocol-output-bypasses-chokepoint: every JSON-RPC message the MCP
//     server writes (protocol errors that echo a tool or prompt name included)
//     passes the kit (this server is stdio-only).

import (
	"bytes"
	"encoding/json"
	"io"
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
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	e.env = append(e.env, "VSPHERE_FEEDBACK_ENDPOINT="+fb.URL+"/feedback")
	out := e.run(t, "feedback", "--send", "the password "+bbPassword+" and "+bbAPISess+" leaked")
	e.runIn(t, "pasted "+bbPassword+"\n", "feedback", "--send", "--stdin")
	e.run(t, "feedback", "--send", "an ordinary note")
	e.runIn(t, "an ordinary pasted note\n", "feedback", "--send", "--stdin")
	mu.Lock()
	got := posted.String()
	mu.Unlock()
	// Only the two clean entries are posted: a body that repeats a held value is
	// refused whole. feedback --stdin takes a note, not a body of credentials, so
	// the kit must not hold its lines (internal/credwire Declarations).
	if strings.Count(got, `"cli":"vsphere-cli"`) != 2 || !strings.Contains(got, "an ordinary pasted note") {
		t.Fatalf("want the two clean entries posted:\nposted: %.1000s\noutput: %.1000s", got, out)
	}
	assertNoEcho(t, "feedback POST body", got)
	assertNoEcho(t, "feedback output", out)
}

// Class mcp-protocol-output-bypasses-chokepoint, stdio: mcp-go answers an
// unknown tool or prompt name with a JSON-RPC error that repeats the name,
// before any tool middleware runs.
func TestMCPProtocolErrorsWithheld(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	out := e.mcpCalls(t, []map[string]any{
		{"name": bbPassword, "arguments": map[string]any{}},
		{"name": "tool-" + bbPassword, "arguments": map[string]any{}},
	})
	out += e.mcpRaw(t, []map[string]any{
		{"jsonrpc": "2.0", "id": 1, "method": "prompts/get", "params": map[string]any{"name": bbPassword}},
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

// httpClientSite matches every way this program builds an HTTP client or sends
// through one it did not build.
var httpClientSite = regexp.MustCompile(`http\.Client\{|http\.DefaultClient|http\.DefaultTransport\.RoundTrip|http\.(Get|Post|PostForm|Head)\(`)

// unguardedClients are the sites that do not name credkit.Transport on the line,
// with the reason; the key is the file and a fragment of the line.
var unguardedClients = map[string]string{
	"internal/client/client.go|&http.Client{Timeout: timeout, Jar: jar, Transport: tr}": "client.New's base: ApplyClientHooks replaces its transport with the session transport over OutboundGuard, and refuses any other shape (InstallVsphereTransport)",
	"internal/client/client.go|&http.Client{Transport: tr}":                             "StreamingHTTPClient with a nil base; every caller passes the guarded client",
	"internal/cli/deliver.go|httpClient := &http.Client{":                               "its Transport is credkit.Transport(nil), the next line",
	"internal/cliutil/probe.go|http.DefaultClient has no Timeout":                       "a comment",
}

// TestEveryHTTPClientIsGuarded: a sender a reprint adds fails here until it
// leaves through the credential kit's outbound guard or is reviewed above.
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
	if sites < 3 {
		t.Fatalf("found only %d client sites; the scan is not reading the tree", sites)
	}
}
