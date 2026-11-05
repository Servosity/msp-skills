// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json under
// the "base-url-credentials-held-and-redacted" entry (class "secret-echo").

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// baseURLSecret stands for a credential a user put inside the base URL.
const baseURLSecret = "Zq9SENTINELbaseurl4242x"

var (
	baseURLBinOnce sync.Once
	baseURLBinDir  string
	baseURLBinErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if baseURLBinDir != "" {
		_ = os.RemoveAll(baseURLBinDir)
	}
	os.Exit(code)
}

// baseURLBinaries builds the CLI and the MCP server once into one directory,
// so the MCP server finds its companion CLI beside it.
func baseURLBinaries(t *testing.T) (cli, mcp string) {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	baseURLBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "dnsfilter-baseurl-bin-")
		if err != nil {
			baseURLBinErr = err
			return
		}
		baseURLBinDir = dir
		for _, b := range []struct{ out, pkg string }{
			{"dnsfilter-cli", "."},
			{"dnsfilter-mcp", "../dnsfilter-mcp"},
		} {
			cmd := exec.Command(goBin, "build", "-o", filepath.Join(dir, b.out), b.pkg)
			cmd.Env = os.Environ()
			if out, err := cmd.CombinedOutput(); err != nil {
				baseURLBinErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if baseURLBinErr != nil {
		t.Fatal(baseURLBinErr)
	}
	return filepath.Join(baseURLBinDir, "dnsfilter-cli"), filepath.Join(baseURLBinDir, "dnsfilter-mcp")
}

// baseURLEnv is a closed environment for one run: the sandbox home and the
// case's own variables, nothing inherited that could point outside it.
func baseURLEnv(home string, extra ...string) []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"USERPROFILE=" + home,
		"DNSFILTER_HOME=" + home,
		"TMPDIR=" + os.TempDir(),
		"NO_COLOR=1",
	}
	return append(env, extra...)
}

func runBaseURLCLI(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader("")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	_ = cmd.Run() // exit codes vary by case; only the output is judged
	return out.String()
}

// mcpDefaultCalls are a typed API read and the sync mirror (which runs the
// companion CLI).
var mcpDefaultCalls = []string{
	`{"name":"dnsfilter_read","arguments":{"endpoint_id":"current-user.user-current-show"}}`,
	`{"name":"sync","arguments":{"resources":["policies"]}}`,
}

// runBaseURLMCP drives one MCP stdio session through calls (tools/call
// params) and returns stdout plus stderr.
func runBaseURLMCP(t *testing.T, bin string, env []string, calls ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
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
	if len(calls) == 0 {
		calls = mcpDefaultCalls
	}
	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
	}
	for i, c := range calls {
		requests = append(requests, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":%s}`, i+2, c))
	}
	for _, r := range requests {
		if _, err := io.WriteString(stdin, r+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	answered := map[float64]bool{}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1<<20), 16<<20)
	for scanner.Scan() {
		line := scanner.Text()
		out.WriteString(line + "\n")
		var msg struct {
			ID *float64 `json:"id"`
		}
		if json.Unmarshal([]byte(line), &msg) == nil && msg.ID != nil {
			answered[*msg.ID] = true
		}
		done := true
		for i := range calls {
			done = done && answered[float64(i+2)]
		}
		if done {
			break
		}
	}
	_ = stdin.Close()
	_, _ = io.Copy(&out, stdout)
	_ = cmd.Wait()
	for i := range calls {
		if !answered[float64(i+2)] {
			t.Errorf("MCP session ended before tool call %d answered (ctx: %v)\n%s\n%s", i+2, ctx.Err(), out.String(), stderr.String())
		}
	}
	return out.String() + stderr.String()
}

// assertNoBaseURLSecret fails when the secret, or any part of it a display
// would keep, appears in out.
func assertNoBaseURLSecret(t *testing.T, label, out string) {
	t.Helper()
	if strings.Contains(out, "SENTINEL") {
		var lines []string
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "SENTINEL") {
				lines = append(lines, l)
			}
		}
		t.Errorf("%s printed the base URL credential:\n%s", label, strings.Join(lines, "\n"))
	}
}

// assertNoSecretOnDisk fails when any file the runs left under home, other
// than the files the test wrote, carries the secret: the response cache, the
// store, the journal and a --deliver file included.
func assertNoSecretOnDisk(t *testing.T, home string, written ...string) {
	t.Helper()
	skip := map[string]bool{}
	for _, w := range written {
		skip[w] = true
	}
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || skip[path] {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr == nil && bytes.Contains(data, []byte("SENTINEL")) {
			t.Errorf("%s holds the base URL credential", path)
		}
		return nil
	})
}

// TestBaseURLCredentialsNeverPrinted drives the built CLI and MCP server with
// a credential inside the base URL (userinfo password, api_key query value,
// a URL that does not parse), from the config file and from
// DNSFILTER_BASE_URL, and with a config file TOML rejects; none of the output,
// nor any file the runs leave, carries it. A base URL without credentials is
// still shown as written.
func TestBaseURLCredentialsNeverPrinted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("drives Unix config-file modes; the in-package hold and redaction tests cover Windows")
	}
	cliBin, mcpBin := baseURLBinaries(t)

	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/current_user") {
			_, _ = io.WriteString(w, `{"data":{"id":"1","type":"users","attributes":{"email":"a@example.com"}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":[],"links":{},"meta":{}}`)
	}))
	t.Cleanup(live.Close) // parallel subtests run after this function returns
	liveHost := strings.TrimPrefix(live.URL, "http://")
	dead := httptest.NewServer(http.NotFoundHandler())
	deadHost := strings.TrimPrefix(dead.URL, "http://")
	dead.Close() // every request to it fails at the network layer, so retries warn

	commands := [][]string{
		{"doctor"},
		{"doctor", "--json"},
		{"doctor", "--agent"},
		{"auth", "status"},
		{"policies", "list"},
		{"policies", "list", "--json"},
		{"sync", "--resources", "policies"},
	}
	// An unreachable server makes every request retry with a warning; two
	// commands cover that path without paying its backoff eight times.
	unreachableCommands := [][]string{{"doctor"}, {"policies", "list"}}
	cases := []struct {
		name     string
		base     string
		commands [][]string
	}{
		{"userinfo", "http://baseuser:" + baseURLSecret + "@" + liveHost, commands},
		{"query", "http://" + liveHost + "/?api_key=" + baseURLSecret, commands},
		{"query-unreachable", "http://" + deadHost + "/?api_key=" + baseURLSecret, unreachableCommands},
		{"unparseable", "ht!tp://baseuser:" + baseURLSecret + "@[::1", commands},
	}
	type run struct {
		name, mode string
		file       string // config file text; "" uses DNSFILTER_BASE_URL
		perm       os.FileMode
		base       string
		commands   [][]string
	}
	var runs []run
	for _, c := range cases {
		runs = append(runs, run{name: c.name, mode: "config", file: fmt.Sprintf("base_url = %q\n", c.base), perm: 0o600, base: c.base, commands: c.commands})
		runs = append(runs, run{name: c.name, mode: "env", base: c.base, commands: c.commands})
	}
	// A config others can change names a different server: the refusal
	// message repeats its base_url.
	runs = append(runs, run{name: "untrusted-route", mode: "config", file: fmt.Sprintf("base_url = %q\napi_key = %q\n", "http://baseuser:"+baseURLSecret+"@"+liveHost, "cfgkey"+baseURLSecret), perm: 0o644, commands: commands})
	// A config TOML rejects: its credentials are still never shown.
	runs = append(runs, run{name: "malformed-toml", mode: "config", file: fmt.Sprintf("base_url = %q\napi_key = %q\n[headers]\nX-Org = %q\nbroken = = 1\n", "http://baseuser:"+baseURLSecret+"@"+liveHost, "cfgkey"+baseURLSecret, "hdr"+baseURLSecret), perm: 0o600, commands: commands})

	for _, r := range runs {
		r := r
		t.Run(r.name+"/"+r.mode, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			extra := []string{"DNSFILTER_API_KEY=envkeyvalue123456789", "DNSFILTER_CLI_PATH=" + cliBin}
			var written []string
			if r.file != "" {
				cfgPath := filepath.Join(home, "cfg", "config.toml")
				if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(cfgPath, []byte(r.file), r.perm); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(cfgPath, r.perm); err != nil {
					t.Fatal(err)
				}
				written = append(written, cfgPath)
				extra = append(extra, "DNSFILTER_CONFIG="+cfgPath)
			} else {
				extra = append(extra, "DNSFILTER_BASE_URL="+r.base)
			}
			env := baseURLEnv(home, extra...)
			for _, args := range r.commands {
				assertNoBaseURLSecret(t, "dnsfilter-cli "+strings.Join(args, " "), runBaseURLCLI(t, cliBin, env, args...))
			}
			deliverPath := filepath.Join(home, "deliver.txt")
			assertNoBaseURLSecret(t, "dnsfilter-cli doctor --deliver", runBaseURLCLI(t, cliBin, env, "doctor", "--deliver", "file:"+deliverPath))
			assertNoBaseURLSecret(t, "dnsfilter-mcp", runBaseURLMCP(t, mcpBin, env))
			assertNoSecretOnDisk(t, home, written...)
		})
	}

	// The error writers redact any URL credential an error names, not only
	// the ones the client and the config build: a --deliver target and an MCP
	// argument are echoed by errors raised outside both.
	t.Run("error-writers", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		env := baseURLEnv(home, "DNSFILTER_API_KEY=envkeyvalue123456789", "DNSFILTER_BASE_URL="+live.URL, "DNSFILTER_CLI_PATH="+cliBin)
		target := "ftp://baseuser:" + baseURLSecret + "@127.0.0.1:1/hook"
		out := runBaseURLCLI(t, cliBin, env, "doctor", "--deliver", "webhook:"+target)
		assertNoBaseURLSecret(t, "dnsfilter-cli --deliver webhook:<url with a password>", out)
		if !strings.Contains(out, "ftp://****@127.0.0.1:1/hook") {
			t.Errorf("the CLI error lost its redacted URL:\n%s", out)
		}
		out = runBaseURLMCP(t, mcpBin, env, `{"name":"dnsfilter_read","arguments":{"endpoint_id":"`+target+`"}}`)
		assertNoBaseURLSecret(t, "dnsfilter-mcp endpoint_id <url with a password>", out)
		if !strings.Contains(out, "ftp://****@127.0.0.1:1/hook") {
			t.Errorf("the MCP error lost its redacted URL:\n%s", out)
		}
	})

	t.Run("healthy-base-url-still-shown", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		cfgPath := filepath.Join(home, "cfg", "config.toml")
		if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cfgPath, []byte(fmt.Sprintf("base_url = %q\n", live.URL)), 0o600); err != nil {
			t.Fatal(err)
		}
		env := baseURLEnv(home, "DNSFILTER_API_KEY=envkeyvalue123456789", "DNSFILTER_CONFIG="+cfgPath)
		if out := runBaseURLCLI(t, cliBin, env, "doctor"); !strings.Contains(out, "base_url: "+live.URL+"\n") {
			t.Errorf("doctor no longer shows a plain base URL as written:\n%s", out)
		}
		if out := runBaseURLCLI(t, cliBin, env, "doctor", "--json"); !strings.Contains(out, `"base_url": "`+live.URL+`"`) {
			t.Errorf("doctor --json no longer shows a plain base URL as written:\n%s", out)
		}
		envOnly := baseURLEnv(t.TempDir(), "DNSFILTER_API_KEY=envkeyvalue123456789", "DNSFILTER_BASE_URL="+live.URL)
		if out := runBaseURLCLI(t, cliBin, envOnly, "doctor"); !strings.Contains(out, "base_url: "+live.URL+"\n") {
			t.Errorf("doctor no longer shows a plain DNSFILTER_BASE_URL as written:\n%s", out)
		}
	})
}
