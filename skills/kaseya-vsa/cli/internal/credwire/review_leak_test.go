// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fixtureSecret is fake, split so a secret scanner does not read it as real.
const fixtureSecret = "plainsecret" + "7Qz4kW"

// fakeProxy records the head of every request it is sent and refuses them all.
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
			br := bufio.NewReader(c)
			for {
				line, err := br.ReadString('\n')
				mu.Lock()
				seen.WriteString(line)
				mu.Unlock()
				if err != nil || line == "\r\n" {
					break
				}
			}
			_, _ = c.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n"))
			_ = c.Close()
		}
	}()
	return ln.Addr().String(), func() string {
		mu.Lock()
		defer mu.Unlock()
		return seen.String()
	}
}

// The CONNECT the Chrome transport writes itself passes the kit: a proxy
// credential that is also a held credential never reaches the proxy, and a
// proxy with no credential still gets the tunnel request.
func TestKitProxyConnectPassesTheKit(t *testing.T) {
	cli, _ := binaries(t)
	const secret = "ProxySecret4821xyz"
	vendor := []string{"KASEYA_VSA_BASE_URL=https://h.vsax.net/api/v3", "KASEYA_VSA_TOKEN_ID=abcd1234", "NO_PROXY=", "no_proxy="}

	addr, heads := fakeProxy(t)
	s := newSandbox(t)
	run(t, cli, s.env(append(vendor, "KASEYA_VSA_TOKEN_SECRET="+secret, "HTTPS_PROXY=http://u:"+secret+"@"+addr)...),
		"organizations", "list", "--json")
	if got := heads(); strings.Contains(strings.ToLower(got), "proxy-authorization") {
		t.Errorf("the proxy received a held credential:\n%s", got)
	}

	addr, heads = fakeProxy(t)
	s = newSandbox(t)
	run(t, cli, s.env(append(vendor, "KASEYA_VSA_TOKEN_SECRET="+secret, "HTTPS_PROXY=http://"+addr)...),
		"organizations", "list", "--json")
	if got := heads(); !strings.HasPrefix(got, "CONNECT h.vsax.net:443 ") {
		t.Errorf("a proxy with no credential did not get the tunnel request:\n%q", got)
	}
}

// The transport's fixed browser headers are judged with the rest of the
// request: a held token holding the word Chromium never leaves in sec-ch-ua.
func TestKitChromeHeadersAreJudgedWithTheRequest(t *testing.T) {
	cli, _ := binaries(t)
	s := newSandbox(t)
	srv := newRDAPServer(t, s, 200, "[]")
	run(t, cli, s.env("KASEYA_VSA_BASE_URL="+srv.URL, "KASEYA_VSA_CA_CERT="+srv.caFile,
		"KASEYA_VSA_TOKEN_ID=abcd1234", "KASEYA_VSA_TOKEN_SECRET="+fixtureSecret,
		"KASEYA_VSA_MCP_HTTP_TOKEN=McpOpaqueChromiumToken593682471095"), "organizations", "list", "--json")
	if n := srv.requests(); n != 0 {
		t.Errorf("%d request(s) carried a piece of a held token in their browser headers", n)
	}

	s = newSandbox(t)
	srv = newRDAPServer(t, s, 200, "[]")
	res := run(t, cli, s.env("KASEYA_VSA_BASE_URL="+srv.URL, "KASEYA_VSA_CA_CERT="+srv.caFile,
		"KASEYA_VSA_TOKEN_ID=abcd1234", "KASEYA_VSA_TOKEN_SECRET="+fixtureSecret), "organizations", "list", "--json")
	if srv.requests() == 0 {
		t.Errorf("an ordinary request did not leave: stdout=%q stderr=%q", res.stdout, res.stderr)
	}
}

// set-credentials holds the values Cobra actually gives it, a leading dash
// after -- included, so the success message cannot print one inside a path.
func TestKitSetCredentialsHoldsDashedPositionals(t *testing.T) {
	cli, _ := binaries(t)
	s := newSandbox(t)
	const secret = "-BetaSecret4821"
	home := filepath.Join(s.root, secret)
	res := run(t, cli, s.env(), "--home", home, "auth", "set-credentials", "--", "-AlphaSecret4821", secret)
	requireAbsent(t, "stdout", res.stdout, secret[1:])
	requireAbsent(t, "stderr", res.stderr, secret[1:])
}

// A config file naming only itself still falls back to the global credential
// store, so startup holds that store too.
func TestKitExplicitConfigHoldsTheGlobalStore(t *testing.T) {
	cli, _ := binaries(t)
	s := newSandbox(t)
	const secret = "kaseya-vsa-cli"
	store := filepath.Join(s.home, "data", "credentials.toml")
	if err := os.MkdirAll(filepath.Dir(store), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store, []byte("vsa_token_secret = \""+secret+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := s.writePrivate(t, "profile/config.toml", "")
	res := run(t, cli, s.env(), "--config", cfg, "--help")
	if strings.Contains(res.stdout+res.stderr, "Usage") && strings.Contains(res.stdout, secret) {
		t.Errorf("help printed a secret from the global store:\n%s", res.stdout)
	}
}

// The legacy store's owner marker goes through the kit: a scope hash that is
// also a piece of a held token is not written.
func TestKitOwnerMarkerPassesTheKit(t *testing.T) {
	cli, _ := binaries(t)
	s := newSandbox(t)
	sum := sha256.Sum256([]byte("hello"))
	piece := hex.EncodeToString(sum[:])[:12]
	cfg := filepath.Join(s.home, "config", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("auth_header = \"hello\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(s.home, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.home, "data", "data.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, cli, s.env("KASEYA_VSA_MCP_HTTP_TOKEN=McpOpaque"+piece+"Token593682471095"), "health", "--check", "offline")
	if hits := filesHolding(t, s.home, piece); len(hits) > 0 {
		t.Errorf("a piece of a held token reached %v", hits)
	}
}

// Every mirrored tool that writes anywhere, the CLI's own local files
// included, previews until confirm is true.
func TestKitLocalWriteToolsNeedConfirm(t *testing.T) {
	_, mcp := binaries(t)
	s := newSandbox(t)
	frames, stderr := mcpCall(t, mcp, s.env(), "teach", map[string]any{
		"query": "find devices in group", "resource": []string{"42"}, "resource-type": "devices"})
	if !strings.Contains(frames, "preview") {
		t.Errorf("teach ran without confirm: frames=%s stderr=%s", frames, stderr)
	}
	if hits := filesHolding(t, s.home, "find devices in group"); len(hits) > 0 {
		t.Errorf("teach wrote %v without confirm", hits)
	}
}
