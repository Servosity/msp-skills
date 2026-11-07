// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cwcblackbox

// Checks only ConnectWise Control's shape needs (no autotask counterpart; its
// zone-discovery case is gone, ConnectWise Control has no zone): no render
// may turn the Basic security scheme into a credential flag every command
// requires; export and tail read the records where they are; the session
// nouns a credential-shaped name rule over-matches are public fields.

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The lead's 4.34.2 render finding (autotask: the spec's per-operation auth
// headers became required flags), kept guarded for this shape: no generated
// command declares the Basic credential as a flag, so every one runs on the
// configured credentials alone and an operator never types the pair on argv
// (into shell history and the process table).
func TestGeneratedCommandsRunOnTheConfiguredCredentials(t *testing.T) {
	if bbHeaderFlags {
		t.Errorf("sessions get-detail declares the Basic credential as a flag; the credentials belong in the configuration")
	}
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	for _, args := range [][]string{
		{"sessions", "list", "--session-type", "Access", "--json"},
		{"sessions", "get-detail", "--session-id", bbSessionID, "--json"},
		{"session-groups", "--json"},
		{"security", "get-configuration", "--json"},
		{"audit", "get-info", "--json"},
		{"audit", "query-log", "--json"},
	} {
		sv.resetMethods()
		out := e.run(t, args...)
		sv.mu.Lock()
		n := len(sv.methods)
		sv.mu.Unlock()
		if n == 0 || strings.Contains(out, "required flag") {
			t.Errorf("%v did not reach the API on the configured credentials:\n%.1000s", args, out)
		}
	}
	if n, all := sv.sawHeader(e.mode.sent()); n == 0 || len(all) != 1 {
		t.Errorf("want every request to carry the environment's pair, got %d of it and %v", n, all)
	}
}

// P1 class export-tail-read-field-definitions (handfix export-tail-read-records,
// autotask): the press once mapped a resource's list read to another
// endpoint, so export wrote something else as the exported records. export
// and tail of the one syncable resource (session_groups) must print a group.
func TestExportAndTailReadTheRecords(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	for _, args := range [][]string{
		{"export", "session_groups", "--format", "json"},
		{"tail", "session_groups", "--follow=false", "--json"},
	} {
		sv.resetMethods()
		out := e.run(t, args...)
		sv.mu.Lock()
		methods := append([]string(nil), sv.methods...)
		sv.mu.Unlock()
		if !strings.Contains(out, bbGroup) {
			t.Errorf("%v printed no session group (requests %v):\n%.1500s", args, methods, out)
		}
	}
}

// publicNouns are the normalized names of the ConnectWise Control session
// nouns the credential-name rule would match ("session", "password") that
// carry no credential (publicSecretLikeFields in internal/cliutil/cwc_secrets.go).
var publicNouns = map[string]bool{
	"sessionid": true, "findsessionid": true, "sessiontype": true, "sessionname": true, "sessionlimit": true,
	"sessioncount": true, "includesessioncaptures": true, "ispasswordexpired": true, "hostsessioninfo": true,
	"sessions": true, "sessiongrouppathparts": true,
}

func normName(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case 'A' <= c && c <= 'Z':
			b.WriteByte(c + ('a' - 'A'))
		case 'a' <= c && c <= 'z', '0' <= c && c <= '9':
			b.WriteByte(c)
		}
	}
	return b.String()
}

// publicNounValues reads every JSON document in out (each one that starts a
// line, and the JSON an MCP result carries as text) and records, for each
// public session noun, every value it carried as JSON text.
func publicNounValues(out string, seen map[string][]string) {
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			for k, x := range t {
				if n := normName(k); publicNouns[n] {
					b, _ := json.Marshal(x)
					seen[n] = append(seen[n], string(b))
				}
				walk(x)
			}
		case []any:
			for _, x := range t {
				walk(x)
			}
		case string:
			if s := strings.TrimSpace(t); strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
				publicNounValues(s, seen)
			}
		}
	}
	for i := 0; i < len(out); i++ {
		if (out[i] != '{' && out[i] != '[') || (i > 0 && out[i-1] != '\n') {
			continue
		}
		var v any
		if json.NewDecoder(strings.NewReader(out[i:])).Decode(&v) == nil {
			walk(v)
		}
	}
}

// Class false-withheld-public-session-noun: "session" and "password" read as
// credential names, so the rule must leave ConnectWise Control's session
// nouns alone. Every one appears in a response, a request preview or both,
// on the CLI and over MCP; none may be redacted by name or withheld, the
// session list (the GetLiveData envelope included) still lists its sessions,
// and the values that are no credential print as the instance sent them.
func TestSessionNounsArePublic(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	var out strings.Builder
	for _, args := range [][]string{
		{"sessions", "list", "--session-type", "Access", "--json"},  // HostSessionInfo, Sessions, SessionGroupPathParts, SessionID, SessionType
		{"sessions", "list", "--session-type", "Support", "--json"}, // the bare array: sessionID, sessionType
		{"sessions", "get-detail", "--session-id", bbSessionID, "--json"},
		{"session-groups", "--json"},                // sessionCount, sessionType
		{"security", "get-configuration", "--json"}, // isPasswordExpired
		{"audit", "query-log", "--json"},            // sessionName
		{"sessions", "stale", "--json"},             // session_id, session_type
		{"sessions", "list", "--session-type", "Access", "--find-session-id", bbSessionID, "--session-limit", "5", "--dry-run", "--json"},
		{"audit", "query-log", "--session-name", "HQ-Record kiosk", "--include-session-captures", "--dry-run", "--json"},
	} {
		out.WriteString(e.run(t, args...) + "\n")
	}
	out.WriteString(e.mcpCalls(t, []map[string]any{
		{"name": "sessions_list", "arguments": map[string]any{"sessionType": "Access"}},
		{"name": "sessions_update-name", "arguments": map[string]any{"sessionID": bbSessionID, "newName": "HQ"}}, // a preview: sessionID
		{"name": "audit_query-log", "arguments": map[string]any{"sessionName": "HQ-Record kiosk"}},
	}))
	got := out.String()
	seen := map[string][]string{}
	publicNounValues(got, seen)
	for n := range publicNouns {
		vals := seen[n]
		if len(vals) == 0 {
			t.Errorf("no output carried %s; the case did not run", n)
		}
		for _, v := range vals {
			switch {
			case n == "sessions" || n == "hostsessioninfo" || n == "sessiongrouppathparts":
				// A container: still the object or array the instance sent (a
				// credential inside it is withheld in place, the container is not).
				if !strings.HasPrefix(v, "{") && !strings.HasPrefix(v, "[") {
					t.Errorf("%s is no longer a container: %.300s", n, v)
				}
			case strings.Contains(v, "[redacted]"):
				t.Errorf("%s was redacted by name: %.300s", n, v)
			case strings.Contains(v, "text withheld") && n != "sessionid":
				// Only the Echo-ID session's id repeats a credential the
				// request carried (withheld by value, which is right).
				t.Errorf("%s was withheld: %.300s", n, v)
			}
		}
	}
	for _, want := range []string{bbSessionID, `"HQ-Record kiosk"`, `"All Machines"`, `"HQ-Record spare"`} {
		if !strings.Contains(got, want) {
			t.Errorf("the output lost %s:\n%.2000s", want, got)
		}
	}
}

// P1 class host-file-from-mcp-argument: the MCP server runs as the operator's
// account, so no tool argument may make it read or write a file the caller
// names, by a path-valued flag, a root flag the tool does not declare, a
// positional that starts with "-" (parsed as a flag by the CLI child), the raw
// args field, or SQL (readfile, ATTACH). A marker file stands for a host file
// the caller wants read; a directory stands for where it wants a write.
func TestMCPArgumentsNeverTouchAHostFile(t *testing.T) {
	t.Parallel()
	const marker = "HOSTFILE-MARKER-7c1e9a"
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	e.run(t, "sync", "--json")
	r := filepath.Join(e.home, "host-read.txt")
	if err := os.WriteFile(r, []byte(`{"title":"`+marker+`","steps":[]}`+"\n"+marker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(e.home, "host-write")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	w := filepath.Join(dir, "pwn.out")
	calls := []map[string]any{
		// reads
		{"name": "import", "arguments": map[string]any{"resource": "sessions", "input": r, "confirm": true}},
		{"name": "teach", "arguments": map[string]any{"query": "q", "resource": []any{bbGroup}, "resource-type": "session_groups", "playbook-file": r, "no-validate": true}},
		{"name": "teach_playbook", "arguments": map[string]any{"query": "q", "playbook-file": r}},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT readfile('" + r + "')"}},
		{"name": "tail", "arguments": map[string]any{"positional-resource": "--config=" + r, "follow": false}},
		{"name": "export", "arguments": map[string]any{"resource": "session_groups", "args": "--config " + r}},
		{"name": "sessions_stale", "arguments": map[string]any{"days": 14, "config": r}},
		// writes
		{"name": "export", "arguments": map[string]any{"resource": "session_groups", "output": w}},
		{"name": "export", "arguments": map[string]any{"resource": "session_groups", "id": "--output=" + w}},
		{"name": "export", "arguments": map[string]any{"resource": "session_groups", "args": "--output " + w}},
		{"name": "tail", "arguments": map[string]any{"resource": "session_groups", "follow": false, "deliver": "file:" + w}},
		{"name": "analytics", "arguments": map[string]any{"type": "session_groups", "audit-dir": dir}},
		{"name": "sync", "arguments": map[string]any{"db": w}},
		{"name": "sql", "arguments": map[string]any{"query": "ATTACH DATABASE '" + w + "' AS p"}},
		{"name": "sql", "arguments": map[string]any{"query": "WITH x AS (SELECT 1) SELECT * FROM x; ATTACH DATABASE '" + w + "' AS p"}},
		{"name": "teach", "arguments": map[string]any{"query": "q", "resource": []any{"--output=" + w}, "resource-type": "session_groups", "no-validate": true}},
		{"name": "learnings_reject", "arguments": map[string]any{"id": "--deliver=file:" + w}},
		{"name": "tail", "arguments": map[string]any{"positional-resource": "--deliver=file:" + w, "follow": false}},
		{"name": "sessions_stale", "arguments": map[string]any{"group": "--deliver=file:" + w}},
	}
	out := e.mcpCalls(t, calls)
	replies := mcpReplies(out)
	for i, c := range calls {
		if replies[i+1] == "" {
			t.Errorf("call %d %v was never answered", i+1, c)
		}
	}
	if strings.Contains(out, marker) {
		t.Errorf("an MCP result repeats the host file the caller named:\n%.3000s", out)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		var names []string
		for _, en := range ents {
			names = append(names, en.Name())
		}
		t.Errorf("an MCP call wrote %v into the directory the caller named:\n%.3000s", names, out)
	}
}

// P1 class mcp-http-open-door: streamable HTTP never serves without the token
// (it refuses to start without one, and off loopback without TLS too), and a
// loopback server refuses a caller that omits it or sends another.
func TestMCPHTTPNeedsTheToken(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	for _, c := range []struct {
		env  []string
		args []string
		want string
	}{
		{withoutEnv(e.env, "CONNECTWISE_CONTROL_MCP_HTTP_TOKEN"), []string{"--transport", "http", "--addr", "0.0.0.0:0"}, "CONNECTWISE_CONTROL_MCP_HTTP_TOKEN"},
		{withoutEnv(e.env, "CONNECTWISE_CONTROL_MCP_HTTP_TOKEN"), []string{"--transport", "http", "--addr", "127.0.0.1:0"}, "CONNECTWISE_CONTROL_MCP_HTTP_TOKEN"},
		{e.env, []string{"--transport", "http", "--addr", "0.0.0.0:0"}, "tls"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		cmd := exec.CommandContext(ctx, bbMCP, c.args...)
		cmd.Env, cmd.Dir = c.env, e.home
		out, err := cmd.CombinedOutput()
		timedOut := ctx.Err() != nil
		cancel()
		if timedOut || err == nil || !strings.Contains(strings.ToLower(string(out)), strings.ToLower(c.want)) {
			t.Errorf("%v: want a refusal to serve naming %q (err %v):\n%.800s", c.args, c.want, err, out)
		}
		assertNoEcho(t, fmt.Sprint(c.args), string(out))
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	cmd := exec.Command(bbMCP, "--transport", "http", "--addr", addr)
	cmd.Env, cmd.Dir = e.env, e.home
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	init := `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
	status := func(bearer string) int {
		req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/mcp", strings.NewReader(init))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	var ok int
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if ok = status(bbHTTPToken); ok != 0 {
			break
		}
	}
	if ok != http.StatusOK {
		t.Fatalf("the token never opened the loopback server (status %d); the test did not run", ok)
	}
	for _, bearer := range []string{"", "wrong-" + bbHTTPToken[6:], strings.ToLower(bbHTTPToken)} {
		if got := status(bearer); got != http.StatusUnauthorized && got != http.StatusForbidden {
			t.Errorf("bearer %q: want 401 or 403, got %d", bearer, got)
		}
	}
}

func withoutEnv(env []string, key string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return out
}
