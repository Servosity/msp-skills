// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): regression guards for handfixes.json entry
// "credkit-wiring" (class secret-echo).

package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"avanan-pp-cli/internal/cliutil"
	"avanan-pp-cli/internal/cliutil/testenv"
)

// mcpCredentialForms is every encoding a credential is looked for in output.
func mcpCredentialForms(v string) []string {
	j, _ := json.Marshal(v)
	return []string{
		v,
		base64.StdEncoding.EncodeToString([]byte(v)),
		base64.RawURLEncoding.EncodeToString([]byte(v)),
		url.QueryEscape(v),
		strings.Trim(string(j), `"`),
	}
}

func assertMCPNoCredential(t *testing.T, where, text, cred string) {
	t.Helper()
	for _, form := range mcpCredentialForms(cred) {
		if strings.Contains(text, form) {
			t.Errorf("%s carries the credential as %q:\n%s", where, form, text)
		}
	}
}

type mcpCredentialSource struct {
	name   string
	cred   string
	env    map[string]string
	config string
	pre    [][]string // companion-CLI runs before the checked call
	over   map[string]string
	tool   string // default avanan_read scopes.get (in process)
	args   map[string]any
	tokens []string
	reject bool
	echo   func(*http.Request) string
	// setup lays out files under the sandbox home before any run and returns
	// env entries for every run (a legacy config, a malformed file).
	setup func(t *testing.T, home string) []string
}

func mcpFixedEcho(v string) func(*http.Request) string {
	return func(*http.Request) string { return v }
}

func mcpWriteFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// mcpMixedCaseHex is hex whose letters alternate lower and upper case.
func mcpMixedCaseHex(s string) string {
	b := []byte(hex.EncodeToString([]byte(s)))
	upper := false
	for i, c := range b {
		if c >= 'a' && c <= 'f' {
			if upper {
				b[i] = c - 'a' + 'A'
			}
			upper = !upper
		}
	}
	return string(b)
}

func mcpHeaderEcho(name string) func(*http.Request) string {
	return func(r *http.Request) string { return r.Header.Get(name) }
}

// mcpCredentialSources: every source the MCP server holds a credential from
// (the config file's credential fields and [headers], the environment
// including the HTTP caller token, the credentials store, a minted and a
// re-minted session token, a short credential), plus a companion-CLI tool
// whose child process mints a token the server itself never sees.
func mcpCredentialSources() []mcpCredentialSource {
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	return []mcpCredentialSource{
		{name: "config-file", cred: "mcp-cfg-app-id-2Q", config: "app_id = \"mcp-cfg-app-id-2Q\"\nclient_secret = \"mcp-cfg-secret-2Q\"\n",
			tokens: []string{"mcp-cfg-session-2Q"}, echo: mcpHeaderEcho("x-av-app-id")},
		{name: "config-headers", cred: "mcp-hdr-value-6H", env: map[string]string{"AVANAN_APP_ID": "mcp-hdr-app-6H", "AVANAN_CLIENT_SECRET": "mcp-hdr-secret-6H"},
			config: "[headers]\nX-Reflect-Me = \"mcp-hdr-value-6H\"\n", tokens: []string{"mcp-hdr-session-6H"}, echo: mcpHeaderEcho("X-Reflect-Me")},
		{name: "env", cred: "mcp-env-app-id-3E", env: map[string]string{"AVANAN_APP_ID": "mcp-env-app-id-3E", "AVANAN_CLIENT_SECRET": "mcp-env-secret-3E"},
			tokens: []string{"mcp-env-session-3E"}, echo: mcpHeaderEcho("x-av-app-id")},
		{name: "http-caller-token", cred: "caller-bearer-tok-8T", env: map[string]string{"AVANAN_APP_ID": "mcp-http-app-8T", "AVANAN_CLIENT_SECRET": "mcp-http-secret-8T", httpTokenEnvVar: "caller-bearer-tok-8T"},
			tokens: []string{"mcp-http-session-8T"}, echo: func(*http.Request) string { return "caller-bearer-tok-8T" }},
		{name: "credentials-store", cred: "mcp-store-app-id-4S", env: map[string]string{"AVANAN_CLIENT_SECRET": "mcp-store-secret-4S"},
			pre: [][]string{{"auth", "set-token", "mcp-store-app-id-4S"}}, tokens: []string{"mcp-store-session-4S"}, echo: mcpHeaderEcho("x-av-app-id")},
		{name: "credentials-file-overridden-by-env", cred: "mcp-stored-app-id-OV2", env: map[string]string{"AVANAN_CLIENT_SECRET": "mcp-stored-secret-OV2"},
			pre: [][]string{{"auth", "set-token", "mcp-stored-app-id-OV2"}}, over: map[string]string{"AVANAN_APP_ID": "mcp-replacement-app-OV2"},
			tokens: []string{"mcp-stored-session-OV2"}, echo: func(*http.Request) string { return "mcp-stored-app-id-OV2" }},
		{name: "minted-token", cred: "mcp-minted-session-5M/0+a", env: map[string]string{"AVANAN_APP_ID": "mcp-mint-app-5M", "AVANAN_CLIENT_SECRET": "mcp-mint-secret-5M"},
			tokens: []string{"mcp-minted-session-5M/0+a"}, echo: mcpHeaderEcho("x-av-token")},
		{name: "refreshed-token", cred: "mcp-refreshed-session-7R", env: map[string]string{"AVANAN_APP_ID": "mcp-refresh-app-7R", "AVANAN_CLIENT_SECRET": "mcp-refresh-secret-7R"},
			config: "token = \"mcp-stale-session-7R\"\ntoken_expiry = " + future + "\n", tokens: []string{"mcp-refreshed-session-7R"}, reject: true, echo: mcpHeaderEcho("x-av-token")},
		{name: "short-credential", cred: "q7m", env: map[string]string{"AVANAN_APP_ID": "q7m", "AVANAN_CLIENT_SECRET": "mcp-short-secret-K3"},
			tokens: []string{"mcp-short-session-K3"}, echo: mcpHeaderEcho("x-av-app-id")},
		{name: "companion-cli-minted-token", cred: "mcp-child-session-9C", env: map[string]string{"AVANAN_APP_ID": "mcp-child-app-9C", "AVANAN_CLIENT_SECRET": "child-secret-for-tests"},
			tool: "sync", args: map[string]any{"resources": "msp-tenants", "json": true}, tokens: []string{"mcp-child-session-9C"}, echo: mcpHeaderEcho("x-av-token")},
		{name: "malformed-legacy-config", cred: "mcp-legacy-app-LG2", env: map[string]string{"AVANAN_APP_ID": "mcp-env-app-LG2", "AVANAN_CLIENT_SECRET": "child-secret-for-tests"},
			tokens: []string{"mcp-legacy-session-LG2"}, echo: mcpFixedEcho("mcp-legacy-app-LG2"),
			setup: func(t *testing.T, home string) []string {
				mcpWriteFile(t, filepath.Join(home, ".config", "avanan-cli", "config.toml"), "app_id = \"mcp-legacy-app-LG2\"\nbroken = \n")
				return []string{"AVANAN_CONFIG="}
			}},
		{name: "malformed-credentials-file", cred: "mcp-malformed-app-MC2", env: map[string]string{"AVANAN_APP_ID": "mcp-env-app-MC2", "AVANAN_CLIENT_SECRET": "child-secret-for-tests"},
			tokens: []string{"mcp-malformed-session-MC2"}, echo: mcpFixedEcho("mcp-malformed-app-MC2"),
			setup: func(t *testing.T, home string) []string {
				t.Setenv("AVANAN_HOME", home) // resolve the child's data dir
				dir, err := cliutil.DataDir()
				if err != nil {
					t.Fatal(err)
				}
				mcpWriteFile(t, filepath.Join(dir, "credentials.toml"), "app_id = \"mcp-malformed-app-MC2\"\nbroken = \n")
				return []string{"AVANAN_CONFIG="}
			}},
		{name: "header-basic-password", cred: "mcp-hdr-pass-HC3", env: map[string]string{"AVANAN_APP_ID": "mcp-hdr-app-HC3", "AVANAN_CLIENT_SECRET": "child-secret-for-tests"},
			config: "[headers]\nX-Upstream-Auth = \"Basic " + base64.StdEncoding.EncodeToString([]byte("mcp-hdr-user-HC3:mcp-hdr-pass-HC3")) + "\"\n",
			tokens: []string{"mcp-hdr-session-HC3"}, echo: mcpFixedEcho("mcp-hdr-pass-HC3")},
		{name: "header-cookie-component", cred: "mcp-cookie-sid-HC4", env: map[string]string{"AVANAN_APP_ID": "mcp-cookie-app-HC4", "AVANAN_CLIENT_SECRET": "mcp-cookie-secret-HC4"},
			config: "[headers]\nCookie = \"theme=dark; sid=mcp-cookie-sid-HC4\"\n", tokens: []string{"mcp-cookie-session-HC4"}, echo: mcpFixedEcho("mcp-cookie-sid-HC4")},
		{name: "echo-in-mixed-case-hex", cred: "mcp-hexecho-app-HX2", env: map[string]string{"AVANAN_APP_ID": "mcp-hexecho-app-HX2", "AVANAN_CLIENT_SECRET": "child-secret-for-tests"},
			tokens: []string{"mcp-hexecho-session-HX2"}, echo: func(r *http.Request) string { return mcpMixedCaseHex(r.Header.Get("x-av-app-id")) }},
	}
}

func mcpReflectingServer(t *testing.T, src mcpCredentialSource) (*httptest.Server, func() int) {
	t.Helper()
	var (
		mu       sync.Mutex
		minted   int
		rejected bool
		hits     int
	)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/v1.0/auth" {
			_, _ = w.Write([]byte(src.tokens[min(minted, len(src.tokens)-1)]))
			minted++
			return
		}
		if src.reject && !rejected {
			rejected = true
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"responseEnvelope":{"responseCode":401,"responseText":"expired"}}`))
			return
		}
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"responseEnvelope":{"requestId":"r1","responseCode":200,"responseText":"OK","recordsNumber":1},"responseData":[{"id":"t1","name":%q}]}`, "us:"+src.echo(r))
	}))
	t.Cleanup(srv.Close)
	return srv, func() int { mu.Lock(); defer mu.Unlock(); return hits }
}

// mcpServerCAFile writes srv's certificate as a PEM file for AVANAN_CA_CERT: the
// kit sends no credential over plain http, loopback included.
func mcpServerCAFile(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub-ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// buildModuleCommand builds the command at pkg (a package path relative to
// this test's directory) into dir as name.
func buildModuleCommand(t *testing.T, dir, name, pkg string) string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		goBin = filepath.Join(runtime.GOROOT(), "bin", "go")
	}
	out := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	if b, err := exec.Command(goBin, "build", "-o", out, pkg).CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, b)
	}
	return out
}

// callToolOverStdio starts the built MCP server, initializes it, calls one
// tool, and returns everything the process wrote (stdout and stderr).
func callToolOverStdio(t *testing.T, bin string, env []string, tool string, args map[string]any) string {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	call, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args}})
	for _, line := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		string(call),
	} {
		_, _ = stdin.Write([]byte(line + "\n"))
	}
	done := make(chan string, 1)
	go func() {
		var all strings.Builder
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			all.WriteString(sc.Text() + "\n")
			if strings.Contains(sc.Text(), `"id":2`) {
				break
			}
		}
		done <- all.String()
	}()
	var out string
	select {
	case out = <-done:
	case <-time.After(90 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("no tools/call reply from %s within 90s; stderr:\n%s", tool, stderr.String())
	}
	_ = stdin.Close()
	waited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(waited) }()
	select {
	case <-waited:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		<-waited
	}
	return out + stderr.String()
}

// TestEveryCredentialSourceIsWithheldFromMCPOutput drives each credential
// source end to end through the built MCP server over stdio: the server
// reflects the credential in a 200 answer, and no form of it may reach the
// tool result, the process's stderr, the cache or the store, while the
// withheld notice proves the answer was seen.
func TestEveryCredentialSourceIsWithheldFromMCPOutput(t *testing.T) {
	bins := t.TempDir()
	mcpBin := buildModuleCommand(t, bins, "avanan-mcp", ".")
	cliBin := buildModuleCommand(t, bins, "avanan-cli", "../avanan-cli")
	for _, src := range mcpCredentialSources() {
		t.Run(src.name, func(t *testing.T) {
			for _, entry := range os.Environ() {
				name, _, _ := strings.Cut(entry, "=")
				if strings.HasPrefix(name, "AVANAN_") || strings.HasPrefix(name, "PRINTING_PRESS_") {
					t.Setenv(name, "")
				}
			}
			home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
			srv, hits := mcpReflectingServer(t, src)
			if src.config != "" {
				if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(src.config), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			env := append(os.Environ(), "AVANAN_HOME="+home, "AVANAN_CONFIG="+filepath.Join(home, "config.toml"),
				"AVANAN_BASE_URL="+srv.URL, "AVANAN_CA_CERT="+mcpServerCAFile(t, srv), "AVANAN_CLI_PATH="+cliBin)
			for k, v := range src.env {
				env = append(env, k+"="+v)
			}
			if src.setup != nil {
				env = append(env, src.setup(t, home)...)
			}
			for _, pre := range src.pre {
				cmd := exec.Command(cliBin, pre...)
				cmd.Env = env
				out, _ := cmd.CombinedOutput()
				assertMCPNoCredential(t, strings.Join(pre, " "), string(out), src.cred)
			}
			for k, v := range src.over {
				env = append(env, k+"="+v)
			}
			tool, args := src.tool, src.args
			if tool == "" {
				tool, args = "avanan_read", map[string]any{"endpoint_id": "scopes.get", "params": map[string]any{}}
			}
			out := callToolOverStdio(t, mcpBin, env, tool, args)
			if hits() == 0 {
				t.Fatalf("the server answered no data request; the source was not exercised:\n%s", out)
			}
			assertMCPNoCredential(t, "MCP output", out, src.cred)
			if strings.Contains(strings.ToLower(out), hex.EncodeToString([]byte(src.cred))) {
				t.Errorf("MCP output carries the credential in hex:\n%s", out)
			}
			// a sync reports a withheld row by name and skips it (#612); every other path prints the kit's notice
			if !strings.Contains(out, "[credkit: withheld") && !strings.Contains(out, "withheld by the credential check") {
				t.Errorf("the reflected answer was not withheld:\n%s", out)
			}
			_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() || d.Name() == "config.toml" || d.Name() == "credentials.toml" {
					return nil
				}
				if b, rerr := os.ReadFile(path); rerr == nil {
					assertMCPNoCredential(t, "file "+path, string(b), src.cred)
				}
				return nil
			})
		})
	}
}

// TestMCPStderrIsChecked: the server's own stderr passes the holder, so a
// startup error that repeats a held value prints the notice instead, and the
// exit status is kept.
func TestMCPStderrIsChecked(t *testing.T) {
	bin := buildModuleCommand(t, t.TempDir(), "avanan-mcp", ".")
	const v = "mcp-stderr-held-SE1"
	cmd := exec.Command(bin, "--transport", v)
	cmd.Env = append(os.Environ(), "AVANAN_APP_ID="+v)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 {
		t.Fatalf("an unknown transport exited %v, want status 2:\n%s", err, out)
	}
	assertMCPNoCredential(t, "stderr", string(out), v)
	if len(out) == 0 {
		t.Error("the transport error printed nothing")
	}
}

// TestMCPStartupDiagnosticsAreChecked: a warning the server prints while it
// resolves its paths at start (an ignored path override) passes the stderr
// check, after the HTTP token and the env credentials are held.
func TestMCPStartupDiagnosticsAreChecked(t *testing.T) {
	bin := buildModuleCommand(t, t.TempDir(), "avanan-mcp", ".")
	const v = "tok-dir-OR2"
	for _, cred := range []string{httpTokenEnvVar, "AVANAN_CLIENT_SECRET"} {
		cmd := exec.Command(bin, "--transport", "bogus")
		cmd.Env = append(os.Environ(), "AVANAN_CONFIG=", "AVANAN_HOME="+t.TempDir(), cred+"="+v, "AVANAN_CONFIG_DIR="+v)
		out, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 2 {
			t.Fatalf("an unknown transport exited %v, want status 2:\n%s", err, out)
		}
		assertMCPNoCredential(t, cred+" startup stderr", string(out), v)
	}
}

// TestMCPFlagErrorsReachStderr: the flag parser's own exits (an unknown flag,
// a missing value) drain the checked stderr, so their message is not lost.
func TestMCPFlagErrorsReachStderr(t *testing.T) {
	bin := buildModuleCommand(t, t.TempDir(), "avanan-mcp", ".")
	for _, args := range [][]string{{"--no-such-flag"}, {"--addr"}, {"--no-such-flag"}, {"--no-such-flag"}} {
		out, err := exec.Command(bin, args...).CombinedOutput()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 2 {
			t.Fatalf("%v exited %v, want status 2:\n%s", args, err, out)
		}
		if !strings.Contains(string(out), "flag needs an argument") && !strings.Contains(string(out), "flag provided but not defined") {
			t.Errorf("%v lost the flag error; stderr:\n%s", args, out)
		}
	}
}
