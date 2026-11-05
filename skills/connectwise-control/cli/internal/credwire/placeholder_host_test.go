// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated): handfixes.json no-credential-to-placeholder-host.
//
// The shipped default base URL, https://your-instance.screenconnect.com, is a
// placeholder on ConnectWise's public cloud domain: a stranger can own the
// instance it names. These tests drive the built binaries with every proxy
// variable pointing at a loopback recorder, so a request the connector does
// not refuse arrives there as a CONNECT naming the host it was meant for, and
// nothing reaches the real host. The connector must refuse before sending and
// name CONNECTWISE_CONTROL_BASE_URL.

package credwire

import (
	"bufio"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Fixture credentials: random, and sharing no 8-character piece with the
// connector's program text.
const (
	placeholderBinUser = "OX9CDjlmCkCC5eb74y"
	placeholderBinPass = "VEdUe4ciJhEknKlR7u"
)

// proxyRecorder is the loopback listener every proxy variable names. It
// records the first line of each connection and answers 403.
type proxyRecorder struct {
	ln    net.Listener
	mu    sync.Mutex
	lines []string
}

func newProxyRecorder(t *testing.T) *proxyRecorder {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &proxyRecorder{ln: ln}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				line, _ := bufio.NewReader(conn).ReadString('\n')
				p.mu.Lock()
				p.lines = append(p.lines, strings.TrimSpace(line))
				p.mu.Unlock()
				_, _ = conn.Write([]byte("HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))
			}(conn)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return p
}

func (p *proxyRecorder) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.lines...)
}

// env points every proxy variable Go reads at the recorder and empties the
// bypass lists, which an inherited value could otherwise widen.
func (p *proxyRecorder) env() []string {
	u := (&url.URL{Scheme: "http", Host: p.ln.Addr().String()}).String()
	return []string{"HTTPS_PROXY=" + u, "https_proxy=" + u, "HTTP_PROXY=" + u, "http_proxy=" + u, "NO_PROXY=", "no_proxy="}
}

func requireNoSend(t *testing.T, p *proxyRecorder) {
	t.Helper()
	if got := p.seen(); len(got) != 0 {
		t.Fatalf("the program sent %d request(s) through the proxy (it meant to reach the host each names): %q", len(got), got)
	}
}

func requireRefusalNamesTheFix(t *testing.T, what, text string) {
	t.Helper()
	if !strings.Contains(text, "CONNECTWISE_CONTROL_BASE_URL") {
		t.Errorf("%s: the output does not name CONNECTWISE_CONTROL_BASE_URL:\n%s", what, text)
	}
	requireAbsent(t, what, text, placeholderBinPass)
}

// filesNaming lists every file under root whose bytes name the placeholder host.
func filesNaming(t *testing.T, root, host string) []string {
	t.Helper()
	var hits []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if b, err := os.ReadFile(path); err == nil && strings.Contains(strings.ToLower(string(b)), host) {
			hits = append(hits, path)
		}
		return nil
	})
	return hits
}

// The CLI: the first-run sequence from auth set-credentials' own example
// (save the credential, then run a command) with no base URL configured, the
// same with the credential in the environment, and the placeholder given
// explicitly in another spelling. Every data command, the generated endpoints
// and both novels, refuses before sending and names the variable; the saved
// config records no placeholder.
func TestPlaceholderHostCLISendsNothing(t *testing.T) {
	cli, _ := binaries(t)
	px := newProxyRecorder(t)

	s := newSandbox(t)
	stored := s.env(px.env()...)
	if r := run(t, cli, stored, "auth", "set-credentials", placeholderBinUser, placeholderBinPass); r.code != 0 {
		t.Fatalf("auth set-credentials: exit %d\nstdout:\n%s\nstderr:\n%s", r.code, r.stdout, r.stderr)
	}
	if hits := filesNaming(t, s.root, "your-instance.screenconnect.com"); len(hits) != 0 {
		t.Errorf("auth set-credentials wrote the shipped placeholder into %q", hits)
	}
	for _, args := range [][]string{
		{"session-groups", "--json"},
		{"sessions", "stale", "--json"},
		{"security", "access-review", "--json"},
	} {
		r := run(t, cli, stored, args...)
		if r.code == 0 {
			t.Errorf("%v with no base URL configured: exit 0\n%s", args, r.stdout)
		}
		requireRefusalNamesTheFix(t, strings.Join(args, " "), r.stdout+r.stderr)
	}

	e := newSandbox(t)
	fromEnv := e.env(append(px.env(), "CONNECTWISE_CONTROL_USERNAME="+placeholderBinUser, "CONNECTWISE_CONTROL_PASSWORD="+placeholderBinPass)...)
	r := run(t, cli, fromEnv, "audit", "get-info", "--json")
	if r.code == 0 {
		t.Errorf("audit get-info with no base URL configured: exit 0\n%s", r.stdout)
	}
	requireRefusalNamesTheFix(t, "audit get-info", r.stdout+r.stderr)
	for _, base := range []string{"https://your-instance.screenconnect.com", "HTTPS://Your-Instance.ScreenConnect.com.:443/"} {
		r := run(t, cli, append(fromEnv, "CONNECTWISE_CONTROL_BASE_URL="+base), "sessions", "list", "--session-type", "Access", "--json")
		if r.code == 0 {
			t.Errorf("sessions list with base URL %s: exit 0\n%s", base, r.stdout)
		}
		requireRefusalNamesTheFix(t, "sessions list ("+base+")", r.stdout+r.stderr)
	}
	requireNoSend(t, px)
}

// The MCP server with CONNECTWISE_CONTROL_BASE_URL at the placeholder the .mcpb
// install form used to pre-fill: a generated read tool, an endpoint tool with
// arguments, a novel's mirror and an intent each answer with the refusal, and
// nothing is sent.
func TestPlaceholderHostMCPSendsNothing(t *testing.T) {
	_, mcp := binaries(t)
	px := newProxyRecorder(t)
	s := newSandbox(t)
	env := s.env(append(px.env(),
		"CONNECTWISE_CONTROL_USERNAME="+placeholderBinUser,
		"CONNECTWISE_CONTROL_PASSWORD="+placeholderBinPass,
		"CONNECTWISE_CONTROL_BASE_URL=https://your-instance.screenconnect.com")...)
	for _, call := range []struct {
		tool string
		args map[string]any
	}{
		{"session_groups_list", map[string]any{}},
		{"sessions_list", map[string]any{"sessionType": "Access"}},
		{"sessions_stale", map[string]any{}},
		{"quarterly_user_access_review", map[string]any{}},
	} {
		frames, stderr := mcpCall(t, mcp, env, call.tool, call.args)
		requireRefusalNamesTheFix(t, "MCP "+call.tool, frames)
		requireAbsent(t, "MCP "+call.tool+" stderr", stderr, placeholderBinPass)
	}
	requireNoSend(t, px)
}

// With nothing configured the kit takes its base_url origin from
// Declarations.BaseURL.Default. That default must be no origin at all (this
// product has no shared API host), so the kit allows a credential nowhere
// until the operator names an instance; and no declared origin may be the
// placeholder host.
func TestDeclarationsNameNoPlaceholderOrigin(t *testing.T) {
	def := Declarations.BaseURL.Default
	if u, err := url.Parse(strings.TrimSpace(def)); err == nil && u.Hostname() != "" {
		t.Errorf("Declarations.BaseURL.Default %q parses to host %q: with no base URL configured, the kit allows the credential there", def, u.Hostname())
	}
	for _, o := range Declarations.Origins {
		for _, a := range o.Allowed {
			if strings.Contains(strings.ToLower(a), "your-instance") {
				t.Errorf("origin %q (source %q) names the shipped placeholder", a, o.Source)
			}
		}
	}
}
