// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package threecxblackbox

// Two more recipients and sources (handfixes.json credkit-wiring): a proxy
// named by the environment, and a config rewritten while the MCP server runs.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeProxy records what each connection sends first, and answers nothing.
func fakeProxy(t *testing.T) (addr string, heads func() string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var mu sync.Mutex
	var seen strings.Builder
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(5 * time.Second))
				r := bufio.NewReader(c)
				for {
					line, err := r.ReadString('\n')
					mu.Lock()
					seen.WriteString(line)
					mu.Unlock()
					if err != nil || line == "\r\n" {
						break
					}
				}
				_, _ = io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
			}(c)
		}
	}()
	return ln.Addr().String(), func() string {
		mu.Lock()
		defer mu.Unlock()
		return seen.String()
	}
}

// A proxy is a second recipient. One whose own URL carries the client secret
// is never handed it; one with no credential still gets the tunnel request, so
// the first half is not passing because nothing was sent at all.
func TestProxyNeverReceivesAHeldCredential(t *testing.T) {
	t.Parallel()
	const host = "pbx.example.test"
	run := func(proxyAuth string, args ...string) string {
		t.Helper()
		addr, heads := fakeProxy(t)
		_, e := honestEnv(t, nil)
		var env []string
		for _, kv := range e.env {
			if !strings.HasPrefix(kv, "THREECX_FQDN=") {
				env = append(env, kv)
			}
		}
		over := &bbEnv{home: e.home, env: append(env, "THREECX_FQDN="+host, "NO_PROXY=", "no_proxy=",
			"HTTPS_PROXY=http://"+proxyAuth+addr)}
		out := over.run(t, args...)
		assertNoEcho(t, "proxy run output", out)
		return heads()
	}
	for _, args := range [][]string{
		{"system-status", "get", "--json", "--data-source", "live"},
		{"auth", "login"}, // the token client is built apart from the API client
	} {
		if got := run("u:"+bbSecret+"@", args...); strings.Contains(strings.ToLower(got), "proxy-authorization") {
			t.Errorf("%v: the proxy received a held credential:\n%s", args, got)
		}
		if got := run("", args...); !strings.HasPrefix(got, "CONNECT "+host+":443 ") {
			t.Errorf("%v: a proxy with no credential did not get the tunnel request:\n%q", args, got)
		}
	}
}

// A config the operator rewrites while the MCP server runs is read again by the
// next call, so its values are held before the parser can quote them.
func TestMCPConfigRewrittenWhileRunningIsHeldBeforeParse(t *testing.T) {
	t.Parallel()
	const secret = "123456789012345678901234567890"
	_, e := honestEnv(t, nil)
	cfg := filepath.Join(e.home, "profile.toml")
	if err := os.WriteFile(cfg, []byte("client_id = \"900\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bbMCP)
	cmd.Env = append(append([]string{}, e.env...), "THREECX_CONFIG="+cfg)
	cmd.Dir = e.home
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		_, _ = io.Copy(io.Discard, stdout)
		_ = cmd.Wait()
	}()
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	var frames strings.Builder
	await := func(id string) {
		t.Helper()
		for sc.Scan() {
			frames.WriteString(sc.Text() + "\n")
			var msg struct {
				ID json.RawMessage `json:"id"`
			}
			if json.Unmarshal(sc.Bytes(), &msg) == nil && string(msg.ID) == id {
				return
			}
		}
		t.Fatalf("the MCP server never answered id %s:\n%s\n%s", id, frames.String(), errb.String())
	}
	send := func(m string) {
		t.Helper()
		if _, err := io.WriteString(stdin, m+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	await("1")
	send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if err := os.WriteFile(cfg, []byte("client_secret = "+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	send(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"threecx_read","arguments":{"endpoint_id":"system-status.get"}}}`)
	await("2")
	if !strings.Contains(frames.String(), `"isError":true`) {
		t.Fatalf("threecx_read did not fail on the unparseable config, so this test proved nothing:\n%s", frames.String())
	}
	if strings.Contains(frames.String()+errb.String(), secret) {
		t.Errorf("a rewritten config value printed:\n%s\n%s", frames.String(), errb.String())
	}
}
