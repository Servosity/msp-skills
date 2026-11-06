// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package autotaskblackbox

// Checks only Autotask's shape needs (no atera counterpart): zone discovery
// carries the UserName (an identifier) to a bootstrap host that is not the
// configured API;
// the 4.34.2 render once required the three auth headers on every generated
// command's argv; and export and tail read the field definitions where the
// records are.

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// zone-discovery-pinned: zone looks the tenant's zone up on
// webservices.autotask.net with the UserName (from --user, AUTOTASK_USERNAME
// or AUTOTASK_PSA_USER_NAME) in the query. Behind the recording proxy the
// tunnel is refused, so the lookup must fail readably, print no credential,
// reach no host but the bootstrap one, and save no zone; the MCP zone tool
// (confirm true runs it as given) the same.
func TestZoneReachesOnlyTheBootstrapHost(t *testing.T) {
	sv := echoServer(t)
	for _, c := range []struct {
		mode credMode
		args []string
	}{
		{credEnv, []string{"zone", "--user", bbZoneUser, "--no-save", "--json"}},
		{credEnv, []string{"zone", "--user", bbZoneUser, "--json"}}, // --save is the default
		{credEnvZoneUser, []string{"zone", "--no-save", "--json"}},  // AUTOTASK_USERNAME
		{credEnv, []string{"zone", "--json"}},                       // AUTOTASK_PSA_USER_NAME
		{credStored, []string{"zone", "--json", "--agent"}},
	} {
		e := newEnv(t, sv, c.mode)
		proxySeen.Lock()
		before := len(proxySeen.reqs)
		proxySeen.Unlock()
		out := e.run(t, c.args...)
		// zone-error-withheld-whole: the CLI keeps the cause readable (the request
		// error minus the user-bearing URL), so the operator sees why it failed.
		if strings.Contains(out, "zone_url") || !zoneFailed(out) {
			t.Errorf("%v (%s): want the readable discovery failure, got:\n%.1500s", c.args, c.mode, out)
		}
		assertNoEcho(t, strings.Join(c.args, " "), out)
		out = e.mcpCalls(t, []map[string]any{{"name": "zone", "arguments": map[string]any{"user": bbZoneUser, "confirm": true}}})
		if !zoneFailed(out) {
			t.Errorf("MCP zone confirm true (%s): want the discovery failure, got:\n%.1500s", c.mode, out)
		}
		assertNoEcho(t, "MCP zone", out)
		proxySeen.Lock()
		seen := append([]string(nil), proxySeen.reqs[before:]...)
		proxySeen.Unlock()
		if len(seen) == 0 {
			t.Errorf("%v (%s): the lookup never reached the bootstrap host (proxy saw %v)", c.args, c.mode, seen)
		}
		for _, r := range seen {
			if !strings.HasPrefix(r, "CONNECT webservices.autotask.net:443 ") {
				t.Errorf("%v (%s): a request left for another host: %s", c.args, c.mode, r)
			}
		}
		_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if b, rerr := os.ReadFile(path); rerr == nil && strings.Contains(string(b), "autotask.net") {
				rel, _ := filepath.Rel(e.home, path)
				t.Errorf("%v (%s): %s saved a zone the lookup never returned", c.args, c.mode, rel)
			}
			return nil
		})
	}
}

// zoneFailed reports a zone run that resolved nothing and said why: the
// discovery failure, never a zone and never the kit's notice in its place.
func zoneFailed(out string) bool {
	return !strings.Contains(out, "zone_url") && strings.Contains(out, "zone discovery request failed") &&
		!strings.Contains(out, "credkit: withheld")
}

// The lead's 4.34.2 render finding, kept guarded: the spec's per-operation
// auth header parameters became required --api-integration-code, --user-name
// and --secret flags on every generated command, so none ran on the
// configured credentials and an operator had to type all three on argv (into
// shell history and the process table). Every generated command must run on
// the configured credentials alone and declare no such flag.
func TestGeneratedCommandsRunOnTheConfiguredCredentials(t *testing.T) {
	t.Parallel()
	if bbHeaderFlags {
		t.Errorf("tickets query-item declares the auth headers as flags; the credentials belong in the configuration")
	}
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	for _, args := range [][]string{
		{"tickets", "query-item", bbTicketID, "--json"},
		{"companies", "query", "--filter", allFilter, "--json"},
		{"tickets", "query-count", "--filter", allFilter, "--json"},
		{"tickets", "query-field-definitions", "--json"},
		{"tickets", "url-parameter-query", "--search", `{"filter":` + allFilter + `}`, "--json"},
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
		t.Errorf("want every request to carry the environment's triple, got %d of it and %v", n, all)
	}
}

// P1 class export-tail-read-field-definitions (handfix export-tail-read-records):
// the press mapped every resource's list read to /<Entity>/entityInformation/fields,
// so `export tickets` wrote the ticket field definitions as the exported records
// and `tail tickets` polled them. Both must print a ticket.
func TestExportAndTailReadTheRecords(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	sv.setClean(true) // the records print (the kit withholds an echoing one whole)
	e := newEnv(t, sv, credEnv)
	for _, args := range [][]string{
		{"export", "tickets", "--format", "json"},
		{"tail", "tickets", "--follow=false", "--json"},
	} {
		sv.resetMethods()
		out := e.run(t, args...)
		sv.mu.Lock()
		methods := append([]string(nil), sv.methods...)
		sv.mu.Unlock()
		if !strings.Contains(out, "T20261028.0001") {
			t.Errorf("%v printed no ticket (requests %v):\n%.1500s", args, methods, out)
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
		{"name": "import", "arguments": map[string]any{"resource": "tickets", "input": r, "confirm": true}},
		{"name": "teach", "arguments": map[string]any{"query": "q", "resource": []any{bbTicketID}, "resource-type": "tickets", "playbook-file": r, "no-validate": true}},
		{"name": "teach_playbook", "arguments": map[string]any{"query": "q", "playbook-file": r}},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT readfile('" + r + "')"}},
		{"name": "tail", "arguments": map[string]any{"positional-resource": "--config=" + r, "follow": false}},
		{"name": "export", "arguments": map[string]any{"resource": "tickets", "args": "--config " + r}},
		{"name": "stale", "arguments": map[string]any{"days": "14", "config": r}},
		// writes
		{"name": "export", "arguments": map[string]any{"resource": "tickets", "output": w}},
		{"name": "export", "arguments": map[string]any{"resource": "tickets", "id": "--output=" + w}},
		{"name": "export", "arguments": map[string]any{"resource": "tickets", "args": "--output " + w}},
		{"name": "tail", "arguments": map[string]any{"resource": "tickets", "follow": false, "deliver": "file:" + w}},
		{"name": "analytics", "arguments": map[string]any{"type": "tickets", "audit-dir": dir}},
		{"name": "sync", "arguments": map[string]any{"db": w}},
		{"name": "sql", "arguments": map[string]any{"query": "ATTACH DATABASE '" + w + "' AS p"}},
		{"name": "sql", "arguments": map[string]any{"query": "WITH x AS (SELECT 1) SELECT * FROM x; ATTACH DATABASE '" + w + "' AS p"}},
		{"name": "teach", "arguments": map[string]any{"query": "q", "resource": []any{"--output=" + w}, "resource-type": "tickets", "no-validate": true}},
		{"name": "picklist", "arguments": map[string]any{"entity": "--deliver=file:" + w, "field": "status"}},
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
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	for _, c := range []struct {
		env  []string
		args []string
		want string
	}{
		{withoutEnv(e.env, "AUTOTASK_MCP_HTTP_TOKEN"), []string{"--transport", "http", "--addr", "0.0.0.0:0"}, "AUTOTASK_MCP_HTTP_TOKEN"},
		{withoutEnv(e.env, "AUTOTASK_MCP_HTTP_TOKEN"), []string{"--transport", "http", "--addr", "127.0.0.1:0"}, "AUTOTASK_MCP_HTTP_TOKEN"},
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
