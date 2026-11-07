// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cometblackbox

// Hand-written: each test is a review's or an adopter's failing input, driven
// through the built binaries.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func filesHolding(t *testing.T, dir, secret string, skip ...string) []string {
	t.Helper()
	var hits []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		for _, s := range skip {
			if p == s {
				return nil
			}
		}
		if b, err := os.ReadFile(p); err == nil && bytes.Contains(b, []byte(secret)) {
			hits = append(hits, p)
		}
		return nil
	})
	return hits
}

// Class stdin-body: a --stdin body whose credential the map parse never sees (a
// malformed body, or an earlier duplicate member) is held before anything prints.
func TestStdinBodyHeldBeforeItsParseError(t *testing.T) {
	t.Parallel()
	sv := cleanServer(t)
	e := newEnv(t, sv, false)
	const dup, bad = "dupStdinMember-7f3c1a", "badStdinBody-4e9d2c"
	// The map parse keeps only the last member, so only a hold on the raw body knows
	// the earlier one when a later answer repeats it.
	sv.mu.Lock()
	sv.extra = dup
	sv.mu.Unlock()
	body := `{"Password":"` + dup + `","Password":"later-value"}`
	out := e.runIn(t, body, "constellation", "last-report", "--stdin", "--json")
	assertNotPrinted(t, "duplicate member", out, dup)
	if !strings.Contains(out, "credkit: withheld") {
		t.Errorf("no notice, so the answer never repeated the member:\n%s", out)
	}
	out = e.runIn(t, `{"Password":"`+bad+`", broken`, "constellation", "last-report", "--stdin", "--json")
	assertNotPrinted(t, "malformed body", out, bad)
}

// Class redacted-marker: a profile read, redacted, then written back would store
// "[redacted]" as every Storage Vault's key. The write is refused before sending.
func TestRedactedMarkerIsNeverWrittenBack(t *testing.T) {
	t.Parallel()
	sv := cleanServer(t)
	e := newEnv(t, sv, false)
	profile := `{"Destinations":{"d":{"S3SecretKey":"[redacted]"}}}`
	out := e.run(t, "users", "set-profile", "--target-user", "acme", "--profile-data", profile, "--yes")
	if !strings.Contains(out, "[redacted]") && !strings.Contains(out, "credkit: withheld") {
		t.Fatalf("the write was not refused:\n%s", out)
	}
	sv.mu.Lock()
	hits := sv.hits["set-user-profile"]
	sv.mu.Unlock()
	if hits != 0 {
		t.Fatalf("the marker reached Comet %d times", hits)
	}
}

// Class profile-store: a run profile carries only flags some command declares and
// none that takes a credential; a URL's password in any field is held.
func TestRunProfileCarriesOnlyTheAllowList(t *testing.T) {
	t.Parallel()
	sv := cleanServer(t)
	e := newEnv(t, sv, false)
	const secret, free = "profileTargetPw-1f7c3a", "profileFreeText-8a2d4e"
	dir := filepath.Join(e.home, "profiles-config")
	e.env = append(e.env, "COMET_BACKUP_CONFIG_DIR="+dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	store := `{"profiles":{"p":{"name":"p","description":"nightly","values":{"limit":"7","target-password":"` + secret + `","my-notes":"` + free + `"}}}}`
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(store), 0o600); err != nil {
		t.Fatal(err)
	}
	out := e.run(t, "profile", "show", "p", "--json")
	assertNotPrinted(t, "profile show", out, secret, free)
	if !strings.Contains(out, `"limit"`) {
		t.Errorf("an allowed value was dropped:\n%s", out)
	}
}

// Class config-reload: the MCP server reloads the config on every call. A file
// rewritten while it runs is held before the parse that would print its value.
func TestMCPConfigReloadHoldsBeforeParse(t *testing.T) {
	t.Parallel()
	sv := cleanServer(t)
	e := newEnv(t, sv, false)
	const secret = "123456789012345678901234567890"
	dir := filepath.Join(e.home, "reload-config")
	cfg := filepath.Join(dir, "config.toml")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, cfg, "auth_header = \"oldcredentialvalue999\"\n")
	cmd := exec.Command(bbMCP)
	cmd.Env, cmd.Dir = append(append([]string{}, e.env...), "COMET_BACKUP_CONFIG_DIR="+dir), e.home
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(stdout)
	exchange := func(id int, msg string) string {
		t.Helper()
		if _, err := io.WriteString(stdin, msg+"\n"); err != nil {
			t.Fatal(err)
		}
		for {
			var frame map[string]any
			if err := dec.Decode(&frame); err != nil {
				t.Fatalf("no answer to %d: %v; stderr:\n%s", id, err, errb.String())
			}
			if frame["id"] == float64(id) {
				b, _ := json.Marshal(frame)
				return string(b)
			}
		}
	}
	exchange(1, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	_, _ = io.WriteString(stdin, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n")
	writeConfig(t, cfg, "auth_header = "+secret+"\n")
	reply := exchange(2, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"comet-backup_read","arguments":{"endpoint_id":"users.list","confirm":true}}}`)
	_ = stdin.Close()
	_ = cmd.Wait()
	if !strings.Contains(reply, `"isError":true`) {
		t.Fatalf("the read did not fail on the unparseable config, so this test proved nothing:\n%s", reply)
	}
	assertNotPrinted(t, "MCP reply", reply, secret)
	assertNotPrinted(t, "MCP stderr", errb.String(), secret)
}

// Class sql-sink: an MCP sql statement that carries a credential is refused at
// the SQLite sink, not only in the rows it returns; an ordinary one still runs.
func TestMCPSQLStatementWithheldAtSQLite(t *testing.T) {
	t.Parallel()
	sv := cleanServer(t)
	e := newEnv(t, sv, false)
	e.run(t, "users", "list", "--json")
	e.run(t, "teach", "--query", "find tenants", "--resource", "acme", "--resource-type", "user")
	out := e.mcpCalls(t, []map[string]any{
		{"name": "sql", "arguments": map[string]any{"query": "SELECT '" + bbPassword + "' AS sample"}},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT 'an ordinary value' AS sample"}},
	})
	assertNoEcho(t, "MCP sql", out)
	if !strings.Contains(out, "an ordinary value") {
		t.Fatalf("an ordinary statement did not run:\n%s", out)
	}
}

// Class learning-logs: a credential on the command line of recall or teach is
// never persisted, and the journal is really written.
func TestLearningLogsNeverPersistCredential(t *testing.T) {
	t.Parallel()
	sv := cleanServer(t)
	e := newEnv(t, sv, false)
	e.run(t, "recall", bbPassword, "--json")
	e.run(t, "teach", "--query", "find items", "--resource-type", "user", "--resource", "acme", "--notes", "uses "+bbPassword)
	if hits := filesHolding(t, e.home, bbPassword, sv.ca); len(hits) > 0 {
		t.Errorf("persisted to %v", hits)
	}
	e.run(t, "teach", "--query", "find items", "--resource-type", "user", "--resource", "acme", "--notes", "an ordinary note")
	if hits := filesHolding(t, e.home, "an ordinary note"); len(hits) == 0 {
		t.Error("teach persists nothing even without a credential, so this test proved nothing")
	}
}

// The journal's own session variable: a credential pasted there stays out of it.
func TestJournalSessionVariableIsNotPersistedWhenItCarriesACredential(t *testing.T) {
	t.Parallel()
	sv := cleanServer(t)
	e := newEnv(t, sv, false)
	e.env = append(e.env, "COMET_BACKUP_LEARN_SESSION="+bbPassword)
	e.run(t, "version")
	if hits := filesHolding(t, e.home, bbPassword, sv.ca); len(hits) > 0 {
		t.Errorf("persisted to %v", hits)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// Class mcp-http: a bearer that carries the refusal's own text must not be echoed
// by the refusal, and a refused request leaves the server answering.
func TestMCPHTTPRefusalsPassTheKit(t *testing.T) {
	t.Parallel()
	const token = "Qz7unauthorizedWm4kP9xL2vB8nR5tY3"
	sv := cleanServer(t)
	e := newEnv(t, sv, false)
	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	cmd := exec.Command(bbMCP, "--transport", "http", "--addr", addr)
	cmd.Dir = e.home
	for _, kv := range e.env {
		if !strings.HasPrefix(kv, "COMET_BACKUP_MCP_HTTP_TOKEN=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "COMET_BACKUP_MCP_HTTP_TOKEN="+token)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	exchange := func(req string) string {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for {
			c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
			if err != nil {
				if time.Now().After(deadline) {
					t.Fatalf("never listened; stderr:\n%s", errb.String())
				}
				time.Sleep(50 * time.Millisecond)
				continue
			}
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(20 * time.Second))
			_, _ = io.WriteString(c, req)
			b, err := io.ReadAll(c)
			if err != nil && !errors.Is(err, io.EOF) {
				t.Logf("read: %v", err)
			}
			return string(b)
		}
	}
	resp := exchange("GET /mcp HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\nAccept: text/event-stream\r\n\r\n")
	if strings.Contains(strings.ToLower(resp), "unauthorized") {
		t.Errorf("the refusal carries a piece of the held bearer:\n%q", resp)
	}
	if !strings.HasPrefix(resp, "HTTP/1.1 4") {
		t.Errorf("not refused:\n%q", resp)
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
	resp = exchange("POST /mcp HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\nAuthorization: Bearer " + token + "\r\nContent-Type: application/json\r\nAccept: application/json, text/event-stream\r\n" + fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body)) + body)
	if !strings.HasPrefix(resp, "HTTP/1.1 200") || !strings.Contains(resp, "protocolVersion") {
		t.Errorf("an ordinary initialize after the refusal was not answered:\n%q\nstderr:\n%s", resp, errb.String())
	}
	if strings.Contains(errb.String(), token) {
		t.Errorf("stderr carries the bearer:\n%s", errb.String())
	}
}
