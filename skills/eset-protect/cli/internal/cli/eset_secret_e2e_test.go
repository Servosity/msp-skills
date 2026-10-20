// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/credvariants"
)

// buildE2EBinaries builds the CLI and the MCP server with the token URL
// pointed at tokenURL (the only build-time difference from a release; a
// release never sets esetTokenURLOverride).
func buildE2EBinaries(t *testing.T, tokenURL string) (cliBin, mcpBin string) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	dir := t.TempDir()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-ldflags", "-X eset-protect-pp-cli/internal/cli.esetTokenURLOverride="+tokenURL, "-o", dir+string(os.PathSeparator), "./cmd/eset-protect-cli", "./cmd/eset-protect-mcp")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	return filepath.Join(dir, "eset-protect-cli"+exe), filepath.Join(dir, "eset-protect-mcp"+exe)
}

// e2eEnv is the environment of a child: this process's, without any ESET
// variable, plus vars.
func e2eEnv(vars map[string]string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "ESET_PROTECT_") && !strings.HasPrefix(kv, "PRINTING_PRESS_") {
			env = append(env, kv)
		}
	}
	for k, v := range vars {
		env = append(env, k+"="+v)
	}
	return env
}

func runE2ECLI(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run()
	return out.String()
}

// runE2EMCP starts the MCP server on stdio, initializes it and calls each
// tool in turn, returning every response line.
func runE2EMCP(t *testing.T, bin string, env []string, calls []map[string]any) string {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Env = env
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1<<20), 16<<20)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	send := func(v map[string]any) {
		b, _ := json.Marshal(v)
		_, _ = stdin.Write(append(b, '\n'))
	}
	recv := func(id int) string {
		deadline := time.After(60 * time.Second)
		for {
			select {
			case l, ok := <-lines:
				if !ok {
					t.Fatalf("MCP server exited; stderr:\n%s", stderr.String())
				}
				if strings.Contains(l, `"id":`+strconv.Itoa(id)+`,`) || strings.Contains(l, `"id":`+strconv.Itoa(id)+`}`) {
					return l
				}
			case <-deadline:
				t.Fatalf("no MCP answer to id %d; stderr:\n%s", id, stderr.String())
			}
		}
	}
	send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "e2e", "version": "1"}}})
	all := recv(1) + "\n"
	send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	for i, c := range calls {
		id := i + 2
		send(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": c})
		all += recv(id) + "\n"
	}
	return all + stderr.String()
}

// TestBuiltBinariesNeverShowASecret runs the two Codex round 6 scenarios end
// to end on built binaries (the CLI and the MCP server over stdio) against a
// local token service and API (handfixes.json:
// secret-registry-masks-every-boundary).
//
// 1. The token service rejects the password grant and echoes the submitted
// password in error_description (raw, JSON-escaped and percent-encoded):
// devices list, auth login, doctor and doctor --json, and MCP execute.
//
// 2. A cached-token refresh: the first bearer of each process gets HTTP 401,
// the 401 re-mint's bearer gets HTTP 403 with {"message":"Rejected credential
// <new bearer>"}; the same body also echoes the MCP HTTP token, which a stdio
// server never reads after start-up. devices list and MCP execute.
//
// No output may let the oracle recover any of the secrets, and each run must
// show it reached the failing call (the HTTP status and "[redacted]").
func TestBuiltBinariesNeverShowASecret(t *testing.T) {
	r := credvariants.NewRand(97)
	password := "pw-" + credvariants.RandomSecret(r)
	mcpToken := "mcp-http-" + randomAlnum(r, 24)
	suffix := randomAlnum(r, 20)

	var mu sync.Mutex
	mode, issued := "echo", 0
	var tokens []string
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_ = req.ParseForm()
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if mode == "echo" {
			sent := req.Form.Get("password")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant", "error_description": "Rejected password " + sent + " (" + url.QueryEscape(sent) + ")"})
			return
		}
		issued++
		tok := fmt.Sprintf("tok%02d-%s", issued, suffix)
		tokens = append(tokens, tok)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": tok, "refresh_token": "rt" + tok, "token_type": "Bearer", "expires_in": 3600})
	}))
	t.Cleanup(tokenSrv.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		bearer := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
		n, _ := strconv.Atoi(strings.TrimPrefix(strings.SplitN(bearer, "-", 2)[0], "tok"))
		w.Header().Set("Content-Type", "application/json")
		if n%2 == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"message":"expired"}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Rejected credential " + bearer + " for caller " + mcpToken})
	}))
	t.Cleanup(api.Close)
	cliBin, mcpBin := buildE2EBinaries(t, tokenSrv.URL)

	env := func(home string) []string {
		return e2eEnv(map[string]string{
			"ESET_PROTECT_HOME":           home,
			"ESET_PROTECT_BASE_URL":       api.URL,
			"ESET_PROTECT_REGION":         "eu",
			"ESET_PROTECT_USERNAME":       "api@msp.example",
			"ESET_PROTECT_PASSWORD":       password,
			"ESET_PROTECT_MCP_HTTP_TOKEN": mcpToken,
			"ESET_PROTECT_NO_LEARN":       "1",
			"ESET_PROTECT_CLI_PATH":       cliBin,
		})
	}
	leaks := func(out string, values ...string) string {
		for _, v := range values {
			if credvariants.SecretRecoverable([]byte(out), v) {
				return v
			}
		}
		return ""
	}
	execute := []map[string]any{{"name": "eset-protect_execute", "arguments": map[string]any{"endpoint_id": "devices.list", "params": map[string]any{}}}}

	// Scenario 1: the token service echoes the submitted password.
	home := t.TempDir()
	for _, args := range [][]string{{"devices", "list", "--data-source", "live", "--json"}, {"auth", "login"}, {"doctor"}, {"doctor", "--json"}} {
		out := runE2ECLI(t, cliBin, env(home), args...)
		if v := leaks(out, password); v != "" {
			t.Fatalf("%v shows the password:\n%s", args, out)
		}
		if !strings.Contains(out, "invalid_grant") || !strings.Contains(out, "[redacted]") {
			t.Fatalf("%v did not reach the token rejection:\n%s", args, out)
		}
	}
	out := runE2EMCP(t, mcpBin, env(t.TempDir()), execute)
	if leaks(out, password) != "" || !strings.Contains(out, "invalid_grant") {
		t.Fatalf("MCP execute shows the password or missed the rejection:\n%s", out)
	}

	// Scenario 2: the 401 re-mint's bearer, echoed in a 403.
	mu.Lock()
	mode = "mint"
	mu.Unlock()
	out = runE2ECLI(t, cliBin, env(t.TempDir()), "devices", "list", "--data-source", "live", "--json")
	mu.Lock()
	minted := append([]string(nil), tokens...)
	mu.Unlock()
	if len(minted) != 2 {
		t.Fatalf("the CLI should mint, get 401 and re-mint once; minted %v:\n%s", minted, out)
	}
	if v := leaks(out, append(minted, mcpToken, password)...); v != "" {
		t.Fatalf("devices list shows %q:\n%s", v, out)
	}
	if !strings.Contains(out, "403") || !strings.Contains(out, "[redacted]") {
		t.Fatalf("devices list did not reach the 403 after the re-mint:\n%s", out)
	}
	out = runE2EMCP(t, mcpBin, env(t.TempDir()), execute)
	mu.Lock()
	minted = append([]string(nil), tokens...)
	mu.Unlock()
	if len(minted) != 4 {
		t.Fatalf("MCP execute should mint, get 401 and re-mint once; minted %v:\n%s", minted, out)
	}
	if v := leaks(out, append(minted, mcpToken, password)...); v != "" {
		t.Fatalf("MCP execute shows %q:\n%s", v, out)
	}
	if !strings.Contains(out, "403") || !strings.Contains(out, "[redacted]") {
		t.Fatalf("MCP execute did not reach the 403 after the re-mint:\n%s", out)
	}
}

func randomAlnum(r *credvariants.Rand, n int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[r.Intn(len(alphabet))]
	}
	return string(b)
}
