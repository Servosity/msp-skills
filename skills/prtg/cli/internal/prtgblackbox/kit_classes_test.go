// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package prtgblackbox

// The credential classes other connectors' reviews named, each from its failing
// input, driven through the built binaries wired to the shared credential kit
// (handfixes.json credkit-wiring).

import (
	"fmt"
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

// without is e with every variable of the given prefix removed, and extra added.
func (e *bbEnv) pointed(drop []string, extra ...string) *bbEnv {
	var env []string
	for _, kv := range e.env {
		keep := true
		for _, d := range drop {
			keep = keep && !strings.HasPrefix(kv, d)
		}
		if keep {
			env = append(env, kv)
		}
	}
	return &bbEnv{home: e.home, env: append(env, extra...), host: e.host}
}

// recordingCore is another client's core: it records every Authorization header
// it is sent, and answers.
func recordingCore(t *testing.T) (host string, seen func() string) {
	t.Helper()
	var mu sync.Mutex
	var log strings.Builder
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		fmt.Fprintf(&log, "%s %s auth=%s key=%s\n", r.Method, r.URL, r.Header.Get("Authorization"), r.Header.Get("X-Api-Key"))
		mu.Unlock()
		fmt.Fprint(w, `{"api":"2.0"}`)
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "https://"), func() string {
		mu.Lock()
		defer mu.Unlock()
		return log.String()
	}
}

// A credential reaches the core each way the program can be pointed at one: the
// origin the kit is shown is the one the program really uses.
func TestCredentialReachesTheConfiguredCore(t *testing.T) {
	host := honestServer(t)
	e := newEnv(t, host, true)
	cfg := filepath.Join(e.home, "pinned.toml")
	writePrivate(t, cfg, "[template_vars]\nserver = \""+host+"\"\n")
	get := []string{"sensors", "list-experimental", "--json", "--data-source", "live"}
	for name, out := range map[string]string{
		"PRTG_SERVER": e.run(t, get...),
		"--core":      e.run(t, append([]string{"--core", "acme"}, get...)...),
		"PRTG_CORE":   e.pointed(nil, "PRTG_CORE=globex").run(t, get...),
		"--config":    e.pointed([]string{"PRTG_SERVER="}).run(t, append([]string{"--config", cfg}, get...)...),
	} {
		if !strings.Contains(out, "Ping-1") {
			t.Errorf("%s: want the core read, got:\n%.800s", name, out)
		}
	}
	if !wasSent(bbKey) || !wasSent(bbKey2) {
		t.Error("a selected core's key never reached it")
	}
}

// The kit refuses a credential over plain http; a core answering only http gets
// no key, and the operator is told why.
func TestSavedKeyIsNotSentOverPlainHTTP(t *testing.T) {
	var carried sync.Map
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization"), bbKey) {
			carried.Store("yes", true)
		}
		fmt.Fprint(w, `[]`)
	}))
	t.Cleanup(plain.Close)
	e := newEnv(t, honestServer(t), false)
	e.runIn(t, bbKey+"\n", "auth", "set-token")
	out := e.pointed([]string{"PRTG_SERVER="}, "PRTG_BASE_URL="+plain.URL+"/api/v2").run(t, "sensors", "list-experimental", "--json", "--data-source", "live")
	if _, ok := carried.Load("yes"); ok {
		t.Fatalf("the saved key went over plain http:\n%.800s", out)
	}
}

// Class credential-sent-to-another-host: every disk source of a credential is
// judged against where this run really sends. A's saved key, profile store and
// [headers] never reach B, however the run names B; A still gets them.
func TestDiskCredentialsStayWithTheirCore(t *testing.T) {
	hostA := honestServer(t)
	hostB, seenB := recordingCore(t)
	const fileKey, headerA = "keyOfTenantA-5f1c97ab", "headerOfTenantA-8e2b7c"
	e := newEnv(t, hostA, false)
	saved := e.pointed([]string{"PRTG_API_KEY=", "PRTG_FLEET_KEYS="})
	get := []string{"sensors", "list-experimental", "--json", "--data-source", "live"}
	saved.runIn(t, bbKey+"\n", "auth", "set-token")
	if out := saved.run(t, get...); !wasSent(bbKey) {
		t.Fatalf("the saved key stopped working on its own core:\n%.800s", out)
	}
	baseA := "https://" + hostA + "/api/v2"
	storeDir := filepath.Join(e.home, "store-profile")
	storeCfg := filepath.Join(storeDir, "config.toml")
	writePrivate(t, storeCfg, "base_url = \""+baseA+"\"\n")
	writePrivate(t, filepath.Join(storeDir, "data", "credentials.toml"), "base_url = \""+baseA+"\"\napi_key = \""+fileKey+"\"\n")
	fileCfg := filepath.Join(e.home, "file-key.toml")
	writePrivate(t, fileCfg, "api_key = \""+fileKey+"\"\n[template_vars]\nserver = \""+hostA+"\"\n")
	headerCfg := filepath.Join(e.home, "header-profile.toml")
	writePrivate(t, headerCfg, "base_url = \""+baseA+"\"\n[headers]\nX-Api-Key = \""+headerA+"\"\n")
	for _, target := range [][]string{{"PRTG_SERVER=" + hostB}, {"PRTG_SERVER=" + hostA, "PRTG_BASE_URL=https://" + hostB + "/api/v2"}} {
		over := saved.pointed([]string{"PRTG_SERVER="}, target...)
		out := over.run(t, get...)
		for _, cfg := range []string{storeCfg, fileCfg, headerCfg} {
			out += over.run(t, append([]string{"--config", cfg}, get...)...)
		}
		if got := seenB(); strings.Contains(got, bbKey) || strings.Contains(got, fileKey) || strings.Contains(got, headerA) {
			t.Fatalf("%v: another client's core received a credential:\n%s\noutput:\n%.800s", target, got, out)
		}
	}
	// A config file's key, on the core it names.
	if out := saved.run(t, append([]string{"--config", fileCfg}, get...)...); !strings.Contains(out, "Ping-1") {
		t.Fatalf("a config file's key stopped working on its own core:\n%.800s", out)
	}
}

// One run's path context never reaches the config file, and never names the core.
func TestPathContextCannotNameTheCore(t *testing.T) {
	const attacker = "attacker.example"
	e := newEnv(t, honestServer(t), true)
	out := e.mcpCalls(t, []map[string]any{{"name": "sync", "arguments": map[string]any{"resources": "sensors", "path-context": "server=" + attacker}}})
	out += e.run(t, "sync", "--resources", "sensors", "--path-context", "server="+attacker)
	if strings.Contains(out, attacker+"/api") {
		t.Errorf("a tool argument named the core:\n%.800s", out)
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

// Kit item 8(a): a --stdin body is held whole before anything prints, a preview and
// a malformed body's error included, and every member of a duplicate key with it.
func TestStdinBodyIsHeldBeforeAPreview(t *testing.T) {
	const value, first, last = "NoDeclaredName-Zq41", "Fa71cQ93xZ", "Lb20dW84yU"
	e := newEnv(t, honestServer(t), true)
	for _, body := range []string{`{"username":"u","mystery":"` + value + `"}`, `{"username":"u","mystery":"` + value + `",`} {
		if out := e.runIn(t, body, "session", "login", "--stdin", "--dry-run"); strings.Contains(out, value) {
			t.Fatalf("a preview or an error printed the --stdin body:\n%.800s", out)
		}
	}
	if out := e.runIn(t, `{"username":"u","password":12345678901}`, "session", "login", "--stdin", "--dry-run"); strings.Contains(out, "12345678901") {
		t.Fatalf("a numeric password printed:\n%.800s", out)
	}
	body := `{"username":"u","password":"` + first + `","password":"` + last + `"}`
	if out := e.runIn(t, body, "session", "login", "--stdin", "--dry-run", "--json"); strings.Contains(out, first) || strings.Contains(out, last) {
		t.Fatalf("a member of a duplicate key printed:\n%.800s", out)
	}
}

// Kit item 8(c): a saved profile's value under a credential flag's name or under
// base-url is a source, every duplicate included, in the config directory, the
// legacy path, under --home and under a registered core's home.
func TestSavedProfileCredentialsAreHeld(t *testing.T) {
	const first, last = "ProfileFirst-7c2d", "ProfileLast-91ab"
	const base = "https://profile-base.example/api/v2"
	body := `{"profiles":{"p":{"name":"p","values":{"password":"` + first + `","password":"` + last + `","base-url":"` + base + `","select":"KeptColumn-Number"}}}}`
	e := newEnv(t, honestServer(t), true)
	coreRun := e.run(t, "--core", "acme", "profile", "list", "--json")
	_ = coreRun
	var coreHome string
	_ = filepath.WalkDir(e.home, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() && filepath.Base(filepath.Dir(path)) == "acme" {
			coreHome = path
		}
		return nil
	})
	if coreHome == "" {
		t.Fatal("no home for the registered core")
	}
	for name, tc := range map[string]struct {
		path string
		args []string
	}{
		"config dir": {filepath.Join(e.home, ".config", "prtg-cli", "profiles.json"), nil},
		"legacy":     {filepath.Join(e.home, ".prtg-cli", "profiles.json"), nil},
		"--home":     {filepath.Join(e.home, "elsewhere", "config", "profiles.json"), []string{"--home", filepath.Join(e.home, "elsewhere")}},
		"core home":  {filepath.Join(coreHome, "config", "profiles.json"), []string{"--core", "acme"}},
	} {
		writePrivate(t, tc.path, body)
		out := e.run(t, append(tc.args, "profile", "show", "p", "--json")...)
		for _, v := range []string{first, last, base} {
			if strings.Contains(out, v) {
				t.Errorf("%s: a saved profile's %s printed:\n%.800s", name, v, out)
			}
		}
		if err := os.Remove(tc.path); err != nil {
			t.Fatal(err)
		}
	}
	// A value under no credential name is not a credential.
	plain := newEnv(t, honestServer(t), true)
	writePrivate(t, filepath.Join(plain.home, ".config", "prtg-cli", "profiles.json"), `{"profiles":{"p":{"name":"p","values":{"select":"KeptColumn-Number"}}}}`)
	if out := plain.run(t, "profile", "show", "p", "--json"); !strings.Contains(out, "KeptColumn-Number") {
		t.Errorf("a saved profile's plain value was held:\n%.800s", out)
	}
}

// A selection scrubs what could redirect a core's key; it never turns off the MCP
// server's own controls, and a selection naming no core stops the server.
func TestCoreSelectionKeepsTheServersControls(t *testing.T) {
	e := newEnv(t, honestServer(t), false).pointed(nil, "PRTG_CORE=acme")
	before := wasSent(bbKey)
	out := e.pointed(nil, "PRTG_MCP_READ_ONLY=1").mcpCalls(t, []map[string]any{{"name": "prtg_execute", "arguments": map[string]any{
		"endpoint_id": "session.login", "confirm": true, "params": map[string]any{"username": "operator-name", "password": "OperatorPa" + "ss-5d17e2"}}}})
	if !strings.Contains(out, "prtg_execute' not found") || wasSent(bbKey) != before {
		t.Errorf("read-only mode was lost under a selection:\n%.1500s", out)
	}
	out = e.pointed(nil, "PRTG_NO_LOCAL_DATA=1").mcpCalls(t, []map[string]any{{"name": "sync", "arguments": map[string]any{"resources": "sensors"}}})
	if !strings.Contains(out, "PRTG_NO_LOCAL_DATA") {
		t.Errorf("the local-data switch was lost under a selection:\n%.1500s", out)
	}
	out = e.mcpCalls(t, []map[string]any{{"name": "prtg_read", "arguments": map[string]any{"endpoint_id": "sensors.list-experimental"}}})
	if !strings.Contains(out, "Ping-1") {
		t.Errorf("the native tool did not reach the selected core:\n%.1500s", out)
	}
	// A configured HTTP bearer survives the selection, so the server starts.
	token := strings.Repeat("httpBearer-", 4)
	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	srv := exec.Command(bbMCP, "--transport", "http", "--addr", addr)
	srv.Env, srv.Dir = append(e.env, "PRTG_MCP_HTTP_TOKEN="+token), e.home
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Process.Kill(); _ = srv.Wait() }()
	code := 0
	for i := 0; i < 100 && code == 0; i++ {
		if resp, err := http.Post("http://"+addr+"/mcp", "application/json", strings.NewReader("{}")); err == nil {
			code = resp.StatusCode
			_ = resp.Body.Close()
		} else {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if code != http.StatusUnauthorized {
		t.Errorf("want the bearer enforced on a selected core, got status %d", code)
	}
	cmd := exec.Command(bbMCP)
	cmd.Env = e.pointed([]string{"PRTG_CORE="}, "PRTG_CORE=nosuch").env
	cmd.Dir = e.home
	if b, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(b), "nosuch") {
		t.Errorf("want a non-zero exit naming the core, got %v:\n%.800s", err, b)
	}
}
