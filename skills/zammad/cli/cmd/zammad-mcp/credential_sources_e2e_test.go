// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): Codex round 2 on the zammad 4.34.0 publish
// (2026-10-27). Pinned by this connector's handfixes.json under the
// "credkit-wiring" entry.

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
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

	"zammad-pp-cli/internal/cliutil"
)

// TestR6EveryCredentialSourceIsHeldEndToEnd drives each credential source
// through the built CLI and MCP server against a vendor stub that repeats
// the credential in every answer: a 200 answer, a 400 error, an early flag
// error that stops cobra before --config is parsed, a --dry-run preview, the
// MCP server's in-process and shell-out tools, and the server's own stderr.
// The credential must reach none of them.
func TestR6EveryCredentialSourceIsHeldEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes decide config trust here; the Windows ACL path has its own tests")
	}
	bin := t.TempDir()
	cliBin, mcpBin := filepath.Join(bin, "zammad-cli"), filepath.Join(bin, "zammad-mcp")
	// Relative to this package's directory, where go test runs.
	for out, pkg := range map[string]string{cliBin: "../zammad-cli", mcpBin: "."} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", pkg, err, b)
		}
	}

	var mu sync.Mutex
	echo := ""
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		s := echo
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/400") || strings.Contains(r.URL.Path, "/knowledge_bases") {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"bad %s"}`, s)
			return
		}
		fmt.Fprintf(w, `{"id":42,"title":"x %s y"}`, s)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "https://")
	// The kit sends no credential over plain http, so the stub is https.
	caFile := filepath.Join(bin, "stub-ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	other := "unused-" + strings.Repeat("Ot4r", 10)

	type source struct {
		name   string
		secret string
		env    []string                         // besides HOME and the stub URL
		config func(secret, base string) string // config file body; nil for none
	}
	sources := []source{
		{name: "env ZAMMAD_API_TOKEN", secret: "env-" + strings.Repeat("Ak7q", 15), env: []string{"ZAMMAD_URL=" + srv.URL}},
		{name: "env ZAMMAD_MCP_HTTP_TOKEN", secret: "mcp-" + strings.Repeat("Hb3w", 15), env: []string{"ZAMMAD_URL=" + srv.URL, "ZAMMAD_API_TOKEN=" + other}},
		{name: "env ZAMMAD_BASE_URL userinfo", secret: "uin-" + strings.Repeat("Ue5r", 15), env: []string{"ZAMMAD_API_TOKEN=" + other}},
		{name: "config api_token", secret: "cfg-" + strings.Repeat("Cf6t", 15), config: func(s, base string) string {
			return fmt.Sprintf("api_token = %q\nbase_url = %q\n", s, base)
		}},
		{name: "config [headers]", secret: "Token token=hdr-" + strings.Repeat("Hd8y", 15), config: func(s, base string) string {
			return fmt.Sprintf("api_token = %q\nbase_url = %q\n[headers]\nAuthorization = %q\n", other, base, s)
		}},
		{name: "config base_url userinfo", secret: "cbu-" + strings.Repeat("Cb9u", 15), config: func(s, _ string) string {
			return fmt.Sprintf("api_token = %q\nbase_url = %q\n", other, "https://op:"+s+"@"+host+"/api/v1")
		}},
		{name: "stored credentials", secret: "sto-" + strings.Repeat("St2i", 15), env: []string{"ZAMMAD_URL=" + srv.URL, "ZAMMAD_API_TOKEN=" + other}},
		{name: "short [headers] credential", secret: "k9Q2v", config: func(s, base string) string {
			return fmt.Sprintf("api_token = %q\nbase_url = %q\n[headers]\nX-Api-Key = %q\n", other, base, s)
		}},
		{name: "env ZAMMAD_WEBHOOK_SECRET", secret: "whe-" + strings.Repeat("Wh4e", 15), env: []string{"ZAMMAD_URL=" + srv.URL, "ZAMMAD_API_TOKEN=" + other}},
		{name: "stored webhook secret", secret: "whs-" + strings.Repeat("Ws5k", 15), env: []string{"ZAMMAD_URL=" + srv.URL, "ZAMMAD_API_TOKEN=" + other}},
	}
	sources[0].env = append(sources[0].env, "ZAMMAD_API_TOKEN="+sources[0].secret)
	sources[1].env = append(sources[1].env, "ZAMMAD_MCP_HTTP_TOKEN="+sources[1].secret)
	sources[2].env = append(sources[2].env, "ZAMMAD_BASE_URL=https://op:"+sources[2].secret+"@"+host+"/api/v1")
	sources[8].env = append(sources[8].env, "ZAMMAD_WEBHOOK_SECRET="+sources[8].secret)

	for _, src := range sources {
		t.Run(src.name, func(t *testing.T) {
			mu.Lock()
			echo = src.secret
			mu.Unlock()
			// The value the stub repeats: the bare token for a scheme form.
			leak := src.secret
			if _, bare, ok := strings.Cut(leak, "="); ok {
				leak = bare
			}
			home := t.TempDir()
			env := append(os.Environ(), "HOME="+home, "ZAMMAD_HOME="+filepath.Join(home, "zh"), "ZAMMAD_CONFIG=", "ZAMMAD_CLI_PATH="+cliBin,
				"ZAMMAD_CA_CERT="+caFile, "ZAMMAD_URL=", "ZAMMAD_BASE_URL=", "ZAMMAD_API_TOKEN=", "ZAMMAD_MCP_HTTP_TOKEN=", "ZAMMAD_WEBHOOK_SECRET=", "ZAMMAD_NO_LEARN=1")
			env = append(env, src.env...)
			var cfgArgs, mcpEnv []string
			if src.config != nil {
				path := filepath.Join(home, "private.toml")
				if err := os.WriteFile(path, []byte(src.config(src.secret, srv.URL+"/api/v1")), 0o600); err != nil {
					t.Fatal(err)
				}
				cfgArgs = []string{"--config", path}
				mcpEnv = []string{"ZAMMAD_CONFIG=" + path}
			}
			if src.name == "stored credentials" || src.name == "stored webhook secret" {
				t.Setenv("HOME", home)
				t.Setenv("ZAMMAD_HOME", filepath.Join(home, "zh"))
				path, err := cliutil.CredentialsFilePath()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				body, file := fmt.Sprintf("api_token = %q\n", src.secret), path
				if src.name == "stored webhook secret" {
					body, file = src.secret+"\n", cliutil.WebhookSecretPathNextTo(path)
				}
				if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			cli := func(args ...string) string {
				cmd := exec.Command(cliBin, args...)
				cmd.Env = env
				var out bytes.Buffer
				cmd.Stdout, cmd.Stderr = &out, &out
				_ = cmd.Run()
				return out.String()
			}
			check := func(what, out string, mustWithhold bool) {
				t.Helper()
				if strings.Contains(out, leak) {
					t.Errorf("%s printed the credential: %.300s", what, out)
				}
				if mustWithhold && !strings.Contains(out, "withheld") {
					t.Errorf("%s: the stub's echo did not reach the kit (test is vacuous): %.300s", what, out)
				}
			}
			check("CLI 200 answer", cli(append([]string{"tickets", "get", "42", "--json", "--no-cache"}, cfgArgs...)...), true)
			check("CLI 400 error", cli(append([]string{"tickets", "get", "400", "--json", "--no-cache"}, cfgArgs...)...), false)
			// --config after the bad flag: cobra stops before parsing it.
			check("CLI early flag error", cli(append([]string{"tickets", "get", "42", "--expand=" + leak}, cfgArgs...)...), true)
			check("CLI --dry-run preview", cli(append([]string{"tickets", "search", "--query=" + leak, "--dry-run"}, cfgArgs...)...), false)

			mcp := exec.Command(mcpBin)
			mcp.Env = append(append([]string(nil), env...), mcpEnv...)
			stdin, _ := mcp.StdinPipe()
			stdout, _ := mcp.StdoutPipe()
			var stderr bytes.Buffer
			mcp.Stderr = &stderr
			if err := mcp.Start(); err != nil {
				t.Fatal(err)
			}
			lines := bufio.NewReader(stdout)
			call := func(id int, method string, params any) string {
				msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
				_, _ = stdin.Write(append(msg, '\n'))
				done := make(chan string, 1)
				go func() { line, _ := lines.ReadString('\n'); done <- line }()
				select {
				case line := <-done:
					return line
				case <-time.After(60 * time.Second):
					t.Fatalf("MCP %s timed out", method)
					return ""
				}
			}
			call(1, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}})
			_, _ = io.WriteString(stdin, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n")
			tool := func(id int, name string, args map[string]any) string {
				return call(id, "tools/call", map[string]any{"name": name, "arguments": args})
			}
			check("MCP in-process 200 answer", tool(2, "zammad_read", map[string]any{"endpoint_id": "tickets.get", "params": map[string]any{"id": 42}}), true)
			check("MCP in-process 400 error", tool(3, "zammad_read", map[string]any{"endpoint_id": "tickets.get", "params": map[string]any{"id": 400}}), false)
			check("MCP shell-out error", tool(4, "kb_get", map[string]any{"answer_id": "7"}), false)
			_ = stdin.Close()
			_ = mcp.Wait()
			check("MCP server stderr", stderr.String(), false)
		})
	}
}

// kitExe is the file name go build must write for exec to run it.
func kitExe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// TestKitShellOutToolSendsTheEnvironmentKey: a tool that shells out to the CLI
// works with the key in the environment. The kit hands the child its credential;
// the child's environment carries none.
func TestKitShellOutToolSendsTheEnvironmentKey(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	bin := t.TempDir()
	mcpBin := filepath.Join(bin, kitExe("zammad-mcp"))
	for out, pkg := range map[string]string{filepath.Join(bin, kitExe("zammad-cli")): "../zammad-cli", mcpBin: "."} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", pkg, err, b)
		}
	}
	const key = "Qm7Rt2Vx" + "Abcdefgh1234"
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7}`))
	}))
	defer srv.Close()
	ca := filepath.Join(bin, "stub-ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	mcp := exec.Command(mcpBin)
	mcp.Env = []string{"HOME=" + home, "USERPROFILE=" + home, "ZAMMAD_HOME=" + home, "PATH=" + os.Getenv("PATH"), "ZAMMAD_BASE_URL=" + srv.URL + "/api/v1", "ZAMMAD_CA_CERT=" + ca, "ZAMMAD_API_TOKEN=" + key}
	// Stdin stays open until the answer: its end cancels a tool still running.
	stdin, _ := mcp.StdinPipe()
	stdout, _ := mcp.StdoutPipe()
	if err := mcp.Start(); err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"kb_get","arguments":{"answer_id":"7"}}}
`)
	lines := bufio.NewReader(stdout)
	var out strings.Builder
	for !strings.Contains(out.String(), `"id":2`) {
		line, err := lines.ReadString('\n')
		out.WriteString(line)
		if err != nil {
			break
		}
	}
	_ = stdin.Close()
	_ = mcp.Wait()
	if strings.Contains(out.String(), key) {
		t.Fatalf("the key was printed: %.300s", out.String())
	}
	mu.Lock()
	defer mu.Unlock()
	for _, auth := range seen {
		if strings.Contains(auth, key) {
			return
		}
	}
	t.Fatalf("no request carried the key (%d requests):\n%.600s", len(seen), out.String())
}

// TestKitHTTPRefusalsPassTheKit: a refusal written beside the kit returned
// its fixed text whatever was held (Codex kit-r1). The class: every answer of
// the HTTP server, the refusals before the handler included, passes the kit.
func TestKitHTTPRefusalsPassTheKit(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	bin := filepath.Join(t.TempDir(), kitExe("zammad-mcp"))
	if b, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, b)
	}
	serve := func(t *testing.T, apiToken, httpToken string) string {
		t.Helper()
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := l.Addr().String()
		l.Close()
		home := t.TempDir()
		cmd := exec.Command(bin, "--transport", "http", "--addr", addr)
		cmd.Env = []string{"HOME=" + home, "USERPROFILE=" + home, "ZAMMAD_HOME=" + home, "PATH=" + os.Getenv("PATH"),
			"ZAMMAD_API_TOKEN=" + apiToken, "ZAMMAD_MCP_HTTP_TOKEN=" + httpToken}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		for i := 0; i < 100; i++ {
			if c, err := net.Dial("tcp", addr); err == nil {
				c.Close()
				return addr
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("the server never listened")
		return ""
	}
	raw := func(t *testing.T, addr, request string) string {
		t.Helper()
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(10 * time.Second))
		if _, err := io.WriteString(c, request); err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(c)
		return string(b)
	}
	httpToken := "mcp-" + strings.Repeat("Ht7k", 12)
	big := "POST /mcp HTTP/1.1\r\nHost: x\r\nAuthorization: Bearer " + httpToken + "\r\nContent-Length: 4194305\r\n\r\n"
	addr := serve(t, "request body too large", httpToken)
	if got := raw(t, addr, big); strings.Contains(got, "request body too large") {
		t.Errorf("the size refusal repeated the API token:\n%s", got)
	} else if !strings.HasPrefix(got, "HTTP/1.1 413") {
		t.Errorf("the size refusal lost its status:\n%s", got)
	}
	addr = serve(t, "wrongpass-"+strings.Repeat("Az3d", 8), httpToken)
	if got := raw(t, addr, big); !strings.Contains(got, "request body too large") {
		t.Errorf("with nothing to withhold the refusal lost its text:\n%s", got)
	}
	noAuth := "POST /mcp HTTP/1.1\r\nHost: x\r\nConnection: close\r\nContent-Length: 0\r\n\r\n"
	if got := raw(t, addr, noAuth); !strings.HasPrefix(got, "HTTP/1.1 401") || !strings.Contains(got, `realm="mcp"`) {
		t.Errorf("the auth refusal changed:\n%s", got)
	}
	addr = serve(t, `realm="mcp"`, httpToken)
	if got := raw(t, addr, noAuth); strings.Contains(got, `realm="mcp"`) {
		t.Errorf("the auth refusal repeated the API token in a header:\n%s", got)
	}
}
