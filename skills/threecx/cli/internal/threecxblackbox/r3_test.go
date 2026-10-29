// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package threecxblackbox

// The classes the second publish pass's budget stopped on, and kit adoption item
// 8, each from its failing input, through the built binaries.

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// recordingHost is a phone system that remembers every request's headers. It
// shares httptest's one certificate, so a's CA file trusts it.
func recordingHost(t *testing.T) (host string, seen func() string) {
	t.Helper()
	var mu sync.Mutex
	var heads strings.Builder
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		fmt.Fprintf(&heads, "%v\n", r.Header)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"value":[]}`)
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "https://"), func() string {
		mu.Lock()
		defer mu.Unlock()
		return heads.String()
	}
}

func (e *bbEnv) without(prefix string, extra ...string) *bbEnv {
	var env []string
	for _, kv := range e.env {
		if !strings.HasPrefix(kv, prefix) {
			env = append(env, kv)
		}
	}
	return &bbEnv{home: e.home, env: append(env, extra...)}
}

func writePrivate(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Class credential-sent-to-another-host: every disk source of a credential is
// judged by one check, against where this run really sends. A's profile store
// and A's config [headers] never reach B, however the run names B.
func TestDiskCredentialsStayWithTheirPhoneSystem(t *testing.T) {
	t.Parallel()
	const tokenA, headerA = "tokenOfTenantA-5f1c9", "headerOfTenantA-8e2b7"
	a := echoServer(t)
	a.honest.Store(true)
	bHost, bSeen := recordingHost(t)
	e := newEnv(t, a, false).without("THREECX_FQDN=")
	get := []string{"system-status", "get", "--json", "--data-source", "live"}
	baseA := "https://" + a.host + "/xapi/v1"

	storeDir := filepath.Join(e.home, "store-profile")
	storeCfg := filepath.Join(storeDir, "config.toml")
	writePrivate(t, storeCfg, "base_url = \""+baseA+"\"\n")
	writePrivate(t, filepath.Join(storeDir, "data", "credentials.toml"), "base_url = \""+baseA+"\"\naccess_token = \""+tokenA+"\"\n")
	unbound := filepath.Join(e.home, "unbound-profile", "config.toml")
	writePrivate(t, unbound, "base_url = \""+baseA+"\"\n")
	writePrivate(t, filepath.Join(filepath.Dir(unbound), "data", "credentials.toml"), "access_token = \""+tokenA+"\"\n")
	headerCfg := filepath.Join(e.home, "header-profile.toml")
	writePrivate(t, headerCfg, "base_url = \""+baseA+"\"\n[headers]\nX-Api-Key = \""+headerA+"\"\n")

	for _, cfg := range []string{storeCfg, unbound, headerCfg} {
		for _, target := range []string{"THREECX_BASE_URL=https://" + bHost + "/xapi/v1", "THREECX_FQDN=" + bHost} {
			out := e.with(target).run(t, append([]string{"--config", cfg}, get...)...)
			if seen := bSeen(); strings.Contains(seen, tokenA) || strings.Contains(seen, headerA) {
				t.Fatalf("%s with %s: another phone system received it:\n%s\noutput:\n%.800s", cfg, target, seen, out)
			}
		}
	}
	// Its own phone system still gets it.
	before := a.hits.Load()
	if out := e.with("THREECX_BASE_URL="+baseA).run(t, append([]string{"--config", storeCfg}, get...)...); a.hits.Load() == before {
		t.Fatalf("the saved credential stopped reaching its own phone system:\n%.800s", out)
	}
}

// THREECX_BASE_URL may name the template; the credential saved for that phone
// system is still its own.
func TestTemplatedBaseURLKeepsItsSavedCredential(t *testing.T) {
	t.Parallel()
	a := echoServer(t)
	a.honest.Store(true)
	e := newEnv(t, a, false).with("THREECX_BASE_URL=https://{fqdn}/xapi/v1")
	e.run(t, "auth", "login", "--client-id", "900", "--client-secret", bbFlagSecret)
	if out := e.run(t, "system-status", "get", "--json", "--data-source", "live"); !strings.Contains(out, "20.0.10.1479") {
		t.Fatalf("a templated base URL refused its own saved credential:\n%.800s", out)
	}
}

// Kit item 8(a): a --stdin body is held whole before anything prints, so a value
// under a name no declaration lists prints nowhere, previews included.
func TestStdinBodyIsHeldBeforeAPreview(t *testing.T) {
	t.Parallel()
	const value = "NoDeclaredName-Zq41"
	_, e := honestEnv(t, nil)
	if out := e.runIn(t, `{"Mystery":"`+value+`"}`, "trunks", "create", "--stdin", "--dry-run"); strings.Contains(out, value) {
		t.Fatalf("a preview printed the --stdin body:\n%.800s", out)
	}
	if out := e.runIn(t, `{"Mystery":"`+value+`",`, "trunks", "create", "--stdin", "--dry-run"); strings.Contains(out, value) {
		t.Fatalf("a malformed body's error printed the --stdin body:\n%.800s", out)
	}
}

// Kit item 8(c): a saved profile's value under a credential flag's name or under
// base-url is a source, every duplicate included.
func TestSavedProfileCredentialsAreHeld(t *testing.T) {
	t.Parallel()
	const first, last, base = "ProfileFirst-7c2d", "ProfileLast-91ab", "https://profile-base.example/xapi/v1"
	body := `{"profiles":{"p":{"name":"p","values":{"auth-password":"` + first + `","auth_password":"` + last + `","base-url":"` + base + `","select":"Number","vmpin":"7391","--client-secret":"` + first + `x"}}}}`
	body = strings.Replace(body, `"select"`, `"auth-password":"`+last+`","select"`, 1)
	echo := `{"Echo":"` + first + `","Other":"` + last + `","Base":"` + base + `","Pin":"7391","Flagged":"` + first + `x","Kept":"Number"}`
	_, e := honestEnv(t, map[string]string{"/SystemStatus": echo})
	for name, path := range map[string]string{
		"config dir": filepath.Join(e.home, ".config", "threecx-cli", "profiles.json"),
		"legacy":     filepath.Join(e.home, ".threecx-cli", "profiles.json"),
		"--home":     filepath.Join(e.home, "elsewhere", "config", "profiles.json"),
	} {
		writePrivate(t, path, body)
		args := []string{"system-status", "get", "--json", "--data-source", "live"}
		if name == "--home" {
			args = append([]string{"--home", filepath.Join(e.home, "elsewhere")}, args...)
		}
		out := e.run(t, args...)
		for _, v := range []string{first, last, base, `"Pin":"7391"`, first + "x"} {
			if strings.Contains(out, v) {
				t.Errorf("%s: a saved profile's %s printed:\n%.800s", name, v, out)
			}
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	// A value under no credential name is not a credential.
	_, plain := honestEnv(t, map[string]string{"/SystemStatus": `{"Version":"20.0.10.1479","Kept":"KeptColumn-Number"}`})
	writePrivate(t, filepath.Join(plain.home, ".config", "threecx-cli", "profiles.json"), `{"profiles":{"p":{"name":"p","values":{"select":"KeptColumn-Number"}}}}`)
	if out := plain.run(t, "system-status", "get", "--json", "--data-source", "live"); !strings.Contains(out, "KeptColumn-Number") {
		t.Errorf("a saved profile's plain value was held:\n%.800s", out)
	}
}

// Class host-file-written-from-mcp-argument: a tool argument never names the
// phone system, so it never reaches config.toml, and the CLI flag is refused alike.
func TestPathContextCannotNameThePhoneSystem(t *testing.T) {
	t.Parallel()
	const attacker = "attacker.example"
	a := echoServer(t)
	a.honest.Store(true)
	e := newEnv(t, a, true).with("THREECX_BASE_URL=https://"+a.host+"/xapi/v1", "THREECX_TOKEN_URL=https://"+a.host+"/connect/token")
	out := e.mcpCalls(t, []map[string]any{{"name": "sync", "arguments": map[string]any{"resources": "users", "path-context": "fqdn=" + attacker}}})
	if !strings.Contains(out, "THREECX_FQDN") {
		t.Errorf("the MCP tool did not refuse the argument:\n%.800s", out)
	}
	cli := e.run(t, "sync", "--resources", "users", "--path-context", "fqdn="+attacker)
	if !strings.Contains(cli, "THREECX_FQDN") {
		t.Errorf("the CLI did not refuse the flag:\n%.800s", cli)
	}
	_ = filepath.WalkDir(e.home, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".toml") {
			if data, _ := os.ReadFile(path); strings.Contains(string(data), attacker) {
				t.Errorf("%s holds the tool argument:\n%s", path, data)
			}
		}
		return nil
	})
}

// Class mcp-read-only-off-under-pbx-selection: a selection scrubs what could
// redirect a phone system's credential; it never turns off the server's own
// controls.
func TestPBXSelectionKeepsTheServersControls(t *testing.T) {
	t.Parallel()
	ps, e := honestEnv(t, nil, "THREECX_PBX=acme")
	before := ps.hits.Load()
	out := e.with("THREECX_MCP_READ_ONLY=1").mcpCalls(t, []map[string]any{{"name": "threecx_execute", "arguments": map[string]any{
		"endpoint_id": "users-id.update-user", "confirm": true, "params": map[string]any{"Id": "2", "FirstName": "Changed"}}}})
	if n := ps.hits.Load(); n != before || !strings.Contains(out, "threecx_execute' not found") {
		t.Errorf("read-only mode was lost under a selection (%d request(s) sent):\n%.1500s", n-before, out)
	}
	out = e.with("THREECX_NO_LOCAL_DATA=1").mcpCalls(t, []map[string]any{{"name": "sync", "arguments": map[string]any{"resources": "users"}}})
	if !strings.Contains(out, "THREECX_NO_LOCAL_DATA") {
		t.Errorf("the local-data switch was lost under a selection:\n%.1500s", out)
	}

	// A configured HTTP bearer survives the selection, so the server starts.
	token := strings.Repeat("httpBearer-", 4)
	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, bbMCP, "--transport", "http", "--addr", addr)
	cmd.Env = append(e.env, "THREECX_MCP_HTTP_TOKEN="+token)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	deadline := time.Now().Add(20 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the HTTP server did not start under a selection: %s", stderr.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Budget 3 round 1: a profile saved under a registered PBX's own home is held too,
// because a selection moves the home after Start.
func TestPBXHomeProfileCredentialsAreHeld(t *testing.T) {
	t.Parallel()
	const secret = "PbxProfilePass-84zQ"
	_, e := honestEnv(t, nil)
	e.run(t, "--pbx", "acme", "system-status", "get", "--json", "--data-source", "live")
	var homes []string
	_ = filepath.WalkDir(e.home, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() && filepath.Base(filepath.Dir(filepath.Dir(path))) == "pbx" && filepath.Base(filepath.Dir(path)) == "acme" {
			homes = append(homes, path)
		}
		return nil
	})
	if len(homes) != 1 {
		t.Fatalf("want the PBX's one home, got %v", homes)
	}
	writePrivate(t, filepath.Join(homes[0], "config", "profiles.json"), `{"profiles":{"p":{"name":"p","values":{"auth-password":"`+secret+`"}}}}`)
	if out := e.run(t, "--pbx", "acme", "profile", "show", "p", "--json"); strings.Contains(out, secret) {
		t.Fatalf("a profile under the PBX's home printed its credential:\n%.800s", out)
	}
}

// Fresh review, budget 3: export's id fills a resource's detail path, and on 3CX a
// detail path can be a function that grants or allocates. It reads records; every
// function with an argument stays with threecx_execute and its confirm.
func TestExportNeverCallsAFunction(t *testing.T) {
	t.Parallel()
	ps, e := honestEnv(t, nil)
	for _, args := range []map[string]any{
		{"resource": "system-status", "id": "365"},
		{"resource": "trunks", "id": "Provider"},
	} {
		before := ps.hits.Load()
		out := e.mcpCalls(t, []map[string]any{{"name": "export", "arguments": args}})
		if ps.hits.Load() != before {
			t.Errorf("MCP export %v reached the phone system with no confirm:\n%.800s", args, out)
		}
		if !strings.Contains(out, "threecx_execute") {
			t.Errorf("MCP export %v did not say why:\n%.800s", args, out)
		}
	}
	before := ps.hits.Load()
	out := e.run(t, "export", "system-status", "365")
	if ps.hits.Load() != before {
		t.Errorf("CLI export reached the phone system:\n%.800s", out)
	}
	// A listing still works.
	before = ps.hits.Load()
	if out := e.run(t, "export", "users"); ps.hits.Load() == before {
		t.Errorf("export users stopped reaching the phone system:\n%.800s", out)
	}
}
