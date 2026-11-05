// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (mimecast; handfixes.json credkit-wiring): one test per
// credential class the factory review and the kit adoption brief name, each
// driven from its failing input against the built binaries.

package mimecastblackbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writePrivate(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertNoneOf(t *testing.T, where, text string, values ...string) {
	t.Helper()
	for _, v := range values {
		for _, f := range forms(v) {
			if strings.Contains(text, f) {
				t.Errorf("%s printed %q as %q:\n%.1500s", where, v, f, text)
			}
		}
	}
}

// Class outgoing-holder-strength-filter and kit item 8(a): a --stdin body is
// held whole before a preview prints it, however weak or short its password,
// a member of a duplicate key and a body that does not parse included.
// Factory r1 failing input: journaling update jx1 --stdin --dry-run with
// {"smtpJournalingConnection":{"password":"lowercasepassword"}}.
func TestKitStdinBodyIsHeldBeforeAPreview(t *testing.T) {
	f := newFake(t)
	home := t.TempDir()
	const first, last = "DupFirst-4Qm8Zt", "DupLast-7Wr2Xk"
	for _, c := range []struct {
		body   string
		values []string
	}{
		{`{"smtpJournalingConnection":{"password":"` + plainPassword + `"}}`, []string{plainPassword}},
		{`{"smtpJournalingConnection":{"password":"` + newPassword + `"}}`, []string{newPassword}},
		{`{"pop3JournalingConnection":{"password":"` + shortPassword + `"}}`, []string{shortPassword}},
		{`{"smtpJournalingConnection":{"password":"` + first + `","password":"` + last + `"}}`, []string{first, last}},
		{`{"smtpJournalingConnection":{"password":"` + newPassword + `",`, []string{newPassword}},
	} {
		for _, args := range [][]string{
			{"--dry-run", "journaling", "update", "jx1", "--stdin", "--json"},
			{"--dry-run", "journaling", "create", "--stdin", "--json"},
		} {
			out, _ := runCLIStdin(t, home, f, c.body, args...)
			assertNoneOf(t, fmt.Sprintf("%v with %s", args, c.body), out, c.values...)
		}
	}
	if n := f.journalWrites.Load(); n != 0 {
		t.Fatalf("a --dry-run sent %d journaling write(s)", n)
	}
}

// Class mcp-preview-credential-leak: a confirmation preview shows the request
// without sending it, and a password only that request carries is held
// before the preview prints. Factory r1 failing input: mimecast_execute
// journaling.update with smtpJournalingConnection.password SmtpNew-123456 and
// no confirm.
func TestKitMCPPreviewHoldsFreshPasswords(t *testing.T) {
	f := newFake(t)
	s := startMCP(t, env(t.TempDir(), f))
	for _, pw := range []string{newPassword, plainPassword, shortPassword} {
		res := s.call("mimecast_execute", map[string]any{"endpoint_id": "journaling.update",
			"params": map[string]any{"journaling_id": "jx1", "smtpJournalingConnection": map[string]any{"password": pw}}})
		assertNoneOf(t, "mimecast_execute preview", res, pw)
		if !strings.Contains(res, "preview") && !strings.Contains(res, kitNotice) {
			t.Errorf("mimecast_execute without confirm neither previewed nor was withheld: %s", res)
		}
	}
	// The mirrored command tool, whose flag is the credential.
	res := s.call("journaling_update", map[string]any{"journaling_id": "jx1", "smtp-journaling-connection-password": newPassword})
	assertNoneOf(t, "journaling_update preview", res, newPassword)
	_ = s.in.Close()
	assertNoneOf(t, "the MCP run", s.transcript(), newPassword, plainPassword, shortPassword)
	if n := f.journalWrites.Load(); n != 0 {
		t.Fatalf("a preview sent %d journaling write(s)", n)
	}
}

// unknownToolProbes are class mcp-protocol-funnel-bypass's two shapes: an
// unknown tool named after a credential held at startup (factory r1), and
// one named after a password only its own arguments carry (factory r2).
func unknownToolProbes() []map[string]any {
	return []map[string]any{
		{"name": clientSecret, "arguments": map[string]any{}},
		{"name": freshPassword, "arguments": map[string]any{"smtpJournalingConnection": map[string]any{"password": freshPassword}}},
	}
}

// Class mcp-protocol-funnel-bypass over stdio: the protocol layer echoes an
// unknown tool's name before any tool middleware runs.
func TestKitMCPUnknownToolNeverEchoesStdio(t *testing.T) {
	f := newFake(t)
	s := startMCP(t, env(t.TempDir(), f))
	for _, p := range unknownToolProbes() {
		m := s.send("tools/call", p)
		b, _ := json.Marshal(m)
		if !strings.Contains(string(b), `"error"`) && !strings.Contains(string(b), kitNotice) {
			t.Errorf("an unknown tool must answer with an error or be withheld: %s", b)
		}
	}
	// The server keeps answering after a withheld frame.
	if res := s.call("mimecast_read", map[string]any{"endpoint_id": "journaling.get", "params": map[string]any{"journaling_id": "jx-ok"}}); !strings.Contains(res, "Plain journal") {
		t.Errorf("the server stopped answering after the probes: %s", res)
	}
	_ = s.in.Close()
	assertNoneOf(t, "MCP stdio", s.transcript(), clientSecret, freshPassword)
}

// Class mcp-protocol-funnel-bypass over HTTP: the same probes, as a client of
// the streamable HTTP server sends them.
func TestKitMCPUnknownToolNeverEchoesHTTP(t *testing.T) {
	f := newFake(t)
	const token = "HttpBearer-5Lp9Qx2Vm7-Rt3Wn8Ky4Bd6Zh1"
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, exe("mimecast-mcp"), "--transport", "http", "--addr", addr)
	cmd.Env = append(env(t.TempDir(), f), "MIMECAST_MCP_HTTP_TOKEN="+token)
	stderr := &bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = stderr, stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	// The process this test started, by PID (CommandContext kills it).
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if c, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
			_ = c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the MCP HTTP server never listened on %s:\n%s", addr, stderr)
		}
	}
	hc := &http.Client{Timeout: 60 * time.Second}
	var transcript strings.Builder
	post := func(session, body string) *http.Response {
		req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/mcp", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if session != "" {
			req.Header.Set("Mcp-Session-Id", session)
		}
		resp, err := hc.Do(req)
		if err != nil {
			t.Fatalf("POST %.80s: %v", body, err)
		}
		payload, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var head strings.Builder
		_ = resp.Header.Write(&head)
		fmt.Fprintf(&transcript, "%d\n%s\n%s\n", resp.StatusCode, head.String(), payload)
		return resp
	}
	init := post("", `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	session := init.Header.Get("Mcp-Session-Id")
	if session == "" {
		t.Fatalf("initialize returned no session id:\n%s", transcript.String())
	}
	post(session, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	for i, p := range unknownToolProbes() {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": i + 1, "method": "tools/call", "params": p})
		post(session, string(b))
	}
	text := transcript.String()
	assertNoneOf(t, "MCP HTTP responses", text, clientSecret, freshPassword)
	assertNoneOf(t, "MCP HTTP stderr", stderr.String(), clientSecret, freshPassword)
	if strings.Count(text, `"error"`)+strings.Count(text, kitNotice) < len(unknownToolProbes()) {
		t.Errorf("each unknown tool must answer with an error or be withheld:\n%.3000s", text)
	}
}

// Class mcp-local-write-confirm-bypass: a local command that rewrites a file
// is a write too. Factory r1 failing input: jobs_prune {"older-than":"0s"}
// against an existing jobs ledger, without confirm.
func TestKitMCPLocalWriteNeedsConfirm(t *testing.T) {
	f := newFake(t)
	home := t.TempDir()
	ledger := filepath.Join(home, "mh", "state", "jobs.jsonl")
	row := `{"job_id":"j-old","resource":"held","endpoint":"release","status":"completed","submitted_at":"2020-01-01T00:00:00Z","updated_at":"2020-01-01T00:00:00Z"}` + "\n"
	writePrivate(t, ledger, row)
	s := startMCP(t, env(home, f))
	for _, args := range []map[string]any{
		{"older-than": "0s"},
		{"older-than": "0s", "confirm": "true"},
		{"older-than": "0s", "confirm": false},
	} {
		res := s.call("jobs_prune", args)
		if got, _ := os.ReadFile(ledger); string(got) != row {
			t.Fatalf("jobs_prune %v rewrote the ledger without confirm: true: %s\nledger now: %q", args, res, got)
		}
	}
	res := s.call("jobs_prune", map[string]any{"older-than": "0s", "confirm": true})
	if got, _ := os.ReadFile(ledger); string(got) == row {
		t.Fatalf("confirm: true must run the prune (the gate is not just closed): %s", res)
	}
}

// Kit item 8(c): a saved profile's value under a credential flag's name or
// under base-url is a credential source, held at startup, every member of a
// duplicate key included, wherever the program reads profiles.json from.
func TestKitSavedProfileCredentialsAreHeld(t *testing.T) {
	const first, last = "ProfileFirst-7c2dQ", "ProfileLast-91abW"
	const secret = "ProfileSecret-3Hv8Kp"
	const base = "https://profile-base.example"
	body := `{"profiles":{"p":{"name":"p","values":{"smtp-journaling-connection-password":"` + first +
		`","smtp-journaling-connection-password":"` + last + `","client-secret":"` + secret +
		`","base-url":"` + base + `","select":"KeptColumn-Number"}}}}`
	f := newFake(t)
	home := t.TempDir()
	other := filepath.Join(home, "elsewhere")
	for name, tc := range map[string]struct {
		path string
		args []string
	}{
		"config dir": {filepath.Join(home, "mh", "config", "profiles.json"), nil},
		"legacy":     {filepath.Join(home, ".mimecast-cli", "profiles.json"), nil},
		"--home":     {filepath.Join(other, "config", "profiles.json"), []string{"--home", other}},
	} {
		writePrivate(t, tc.path, body)
		out, _ := runCLI(t, home, f, append(tc.args, "profile", "show", "p", "--json")...)
		assertNoneOf(t, name+": profile show", out, first, last, secret, base)
		if err := os.Remove(tc.path); err != nil {
			t.Fatal(err)
		}
	}
	// A value under no credential name is not a credential.
	writePrivate(t, filepath.Join(home, "mh", "config", "profiles.json"), `{"profiles":{"p":{"name":"p","values":{"select":"KeptColumn-Number"}}}}`)
	if out, _ := runCLI(t, home, f, "profile", "show", "p", "--json"); !strings.Contains(out, "KeptColumn-Number") {
		t.Errorf("a saved profile's plain value was held:\n%.800s", out)
	}
}

// Kit item 8(b): credentials go to the base URL of the config the program
// really reads. An explicit --config or MIMECAST_CONFIG wins over the
// discovered config.toml, whose base_url names another host here: the
// request reaches the selected base with its credentials, and the other host
// receives none.
func TestKitCredentialsReachTheSelectedConfigBase(t *testing.T) {
	f := newFake(t)
	home := t.TempDir()
	writePrivate(t, filepath.Join(home, "mh", "config", "config.toml"), "base_url = \""+f.evil.URL+"\"\n")
	explicit := filepath.Join(home, "explicit.toml")
	writePrivate(t, explicit, "base_url = \""+f.srv.URL+"\"\n")
	creds := append(baseEnv(home), "MIMECAST_CLIENT_ID="+clientID, "MIMECAST_CLIENT_SECRET="+clientSecret,
		"MIMECAST_TOKEN_URL="+f.srv.URL+"/oauth/token")
	for name, run := range map[string]func() (string, int){
		"--config": func() (string, int) {
			return runWith(t, creds, "", "--config", explicit, "journaling", "get", "jx-ok", "--json")
		},
		"MIMECAST_CONFIG": func() (string, int) {
			return runWith(t, append(creds, "MIMECAST_CONFIG="+explicit), "", "journaling", "get", "jx-ok", "--json")
		},
	} {
		if out, code := run(); code != 0 || !strings.Contains(out, "Plain journal") {
			t.Errorf("%s: the selected base did not answer (exit %d):\n%s", name, code, out)
		}
	}
	if n := f.evilHits.Load(); n != 0 {
		t.Errorf("the discovered config's host was reached %d time(s)", n)
	}
	f.assertEvilSawNothing(t)
}

// Adversarial round 1 failing input: export journaling --output out.jsonl
// wrote the journaling SMTP and POP3 passwords from the documented Get
// Journaling Services answer into a plain file. The export file is a sink the
// kit checks (contract 5), so no held value may reach it.
func TestKitExportFileHoldsNoCredential(t *testing.T) {
	f := newFake(t)
	home := t.TempDir()
	for _, format := range []string{"jsonl", "json"} {
		out := filepath.Join(home, "export-"+format+".out")
		text, _ := runCLI(t, home, f, "export", "journaling", "--format", format, "--output", out)
		assertNoneOf(t, "export journaling --format "+format, text, smtpPassword, pop3Password)
		if data, err := os.ReadFile(out); err == nil {
			assertNoneOf(t, "export file "+out, string(data), smtpPassword, pop3Password)
		}
	}
	scanFiles(t, home)
}
