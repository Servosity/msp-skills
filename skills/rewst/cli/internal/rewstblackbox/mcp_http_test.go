// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Threat-model class http-mode-reachable-off-loopback-without-a-token: the
// MCP server's HTTP transport never serves without REWST_MCP_HTTP_TOKEN (at
// least 32 characters), never serves plaintext off loopback (any wildcard or
// routable --addr needs --tls-cert and --tls-key), and on loopback answers
// only a caller that sends the token as Authorization: Bearer. The refusals
// happen before the server binds (nothing listens on a wildcard address in
// this test), and the loopback control proves the right token is served.
func TestMCPHTTPModeNeedsTokenAndLoopback(t *testing.T) {
	budget(t)
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	start := func(env []string, addr string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, bbMCP, "--transport", "http", "--addr", addr)
		cmd.Env, cmd.Dir = env, e.home
		out, err := cmd.CombinedOutput()
		if ctx.Err() != nil {
			return string(out), ctx.Err()
		}
		return string(out), err
	}
	noToken := withoutEnv(e.env, "REWST_MCP_HTTP_TOKEN")
	for _, c := range []struct {
		name string
		env  []string
		addr string
	}{
		{"wildcard, no token", noToken, "0.0.0.0:0"},
		{"wildcard, a token, no TLS", e.env, "0.0.0.0:0"},
		{"IPv6 wildcard, a token, no TLS", e.env, "[::]:0"},
		{"loopback, no token", noToken, "127.0.0.1:0"},
		{"loopback, a short token", append(withoutEnv(e.env, "REWST_MCP_HTTP_TOKEN"), "REWST_MCP_HTTP_TOKEN=short-token-0123"), "127.0.0.1:0"},
	} {
		out, err := start(c.env, c.addr)
		if err == nil || err == context.DeadlineExceeded {
			t.Errorf("%s: the HTTP transport started (err %v):\n%.1000s", c.name, err, out)
		}
		assertNoEcho(t, c.name, out)
	}
	// Loopback with the token: only the right Bearer token is served.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	cmd := exec.Command(bbMCP, "--transport", "http", "--addr", addr)
	cmd.Env, cmd.Dir = e.env, e.home
	var errb lockedBuffer
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	init, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}})
	status := func(auth string) int {
		req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/mcp", bytes.NewReader(init))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	var right int
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if right = status("Bearer " + bbHTTPToken); right != 0 {
			break
		}
	}
	if right != http.StatusOK {
		t.Fatalf("the right token was not served (status %d):\n%s", right, errb.String())
	}
	for _, auth := range []string{"", "Bearer wrong-token-0123456789abcdefABCDEF", "Bearer " + bbToken, bbHTTPToken, "Basic " + bbHTTPToken} {
		if got := status(auth); got != http.StatusUnauthorized && got != http.StatusForbidden {
			t.Errorf("Authorization %q: status %d, want 401", strings.ReplaceAll(auth, bbHTTPToken, "<token>"), got)
		}
	}
	assertNoEcho(t, "MCP HTTP stderr", errb.String())
}
