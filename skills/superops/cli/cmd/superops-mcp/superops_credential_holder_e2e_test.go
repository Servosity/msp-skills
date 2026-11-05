// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): every credential the program holds, from every
// source, is withheld at any length from CLI and MCP output
// (credkit-wiring).

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// holderBaseToken is the API token every case sends so a request goes out;
// a case that is about another source does not echo it.
const holderBaseToken = "sopBaseHolderTok9"

const holderRawQuery = "query{ __typename }"

// holderSource configures one credential source in a fresh sandbox. values
// are what the vendor stub echoes back, as written, base64 and hex: in a
// successful tickets page, in an HTTP 200 GraphQL error for any other query,
// and in an HTTP 401 body for any other request.
type holderSource struct {
	name   string
	values []string
	// env, files (sandbox-relative path to content), CLI and MCP argv, and
	// a CLI command run (with stdin) before the checked calls.
	env     func(host string) map[string]string
	files   map[string]string
	cliArgs []string
	mcpArgs []string
	before  *holderStep
}

type holderStep struct {
	args  []string
	stdin string
}

func holderSources() []holderSource {
	const hook = "http://w~Kk9:HookPassHold3r@127.0.0.1:9/hook"
	return []holderSource{
		{name: "env-api-token", values: []string{holderBaseToken}},
		{
			name:   "config-api-token",
			values: []string{"sopCfgHolderTok4W"},
			env: func(string) map[string]string {
				return map[string]string{"SUPEROPS_API_TOKEN": "", "SUPEROPS_CONFIG": "{sandbox}/cfg/config.toml"}
			},
			files: map[string]string{"cfg/config.toml": "api_token = \"sopCfgHolderTok4W\"\n"},
		},
		{
			// The round-2 input, plus a short custom header value.
			name:   "config-headers",
			values: []string{"sop-secret-123", "Q~z7"},
			env: func(string) map[string]string {
				return map[string]string{"SUPEROPS_CONFIG": "{sandbox}/cfg/config.toml"}
			},
			files: map[string]string{"cfg/config.toml": "[headers]\nAuthorization = \"Bearer sop-secret-123\"\nX-Api-Key = \"Q~z7\"\n"},
		},
		{
			// auth set-token reads stdin and saves to the credentials file;
			// the checked calls read it back from there.
			name:   "stdin-and-credentials-file",
			values: []string{"sopStoreHolderTok8M"},
			env: func(string) map[string]string {
				return map[string]string{"SUPEROPS_API_TOKEN": ""}
			},
			before: &holderStep{args: []string{"auth", "set-token"}, stdin: "sopStoreHolderTok8M\n"},
		},
		{
			// A short username beside a password.
			name:   "base-url-userinfo",
			values: []string{"u~Q", "LongPassHolder77"},
			env: func(host string) map[string]string {
				return map[string]string{"SUPEROPS_BASE_URL": "https://u~Q:LongPassHolder77@" + host}
			},
		},
		{
			name:   "proxy-env-userinfo",
			values: []string{"p~X", "ProxyPassHold5"},
			env: func(string) map[string]string {
				p := "http://p~X:ProxyPassHold5@127.0.0.1:9"
				return map[string]string{"HTTP_PROXY": p, "HTTPS_PROXY": p, "NO_PROXY": "127.0.0.1"}
			},
		},
		{
			name:   "mcp-http-token-env",
			values: []string{"holderHttpToken0123456789abcdefXYZ"},
			env: func(string) map[string]string {
				return map[string]string{"SUPEROPS_MCP_HTTP_TOKEN": "holderHttpToken0123456789abcdefXYZ"}
			},
		},
	}
}

func holderForms(values []string) []string {
	var out []string
	for _, v := range values {
		out = append(out, v,
			base64.StdEncoding.EncodeToString([]byte(v)),
			base64.RawStdEncoding.EncodeToString([]byte(v)),
			hex.EncodeToString([]byte(v)))
	}
	return out
}

// serveHolderTLS serves h over TLS and returns the host and a trust bundle for
// SUPEROPS_CA_CERT: the kit sends no credential over plain http, on loopback too.
func serveHolderTLS(t *testing.T, h http.Handler) (string, string) {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return strings.TrimPrefix(srv.URL, "https://"), ca
}

func serveHolderStub(t *testing.T, values []string, requests *atomic.Int64) (string, string) {
	t.Helper()
	var echo []string
	for _, v := range values {
		echo = append(echo, v, base64.StdEncoding.EncodeToString([]byte(v)), hex.EncodeToString([]byte(v)))
	}
	msg := "credential " + strings.Join(echo, " / ") + " invalid"
	return serveHolderTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && bytes.Contains(body, []byte("getTicketList")) {
			ticket := map[string]any{"ticketId": "7000000000000001", "displayId": "T-1", "subject": msg, "status": "Open", "updatedTime": "2026-10-01T00:00:00"}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"getTicketList": map[string]any{
				"nodes": []any{ticket}, "listInfo": map[string]any{"hasMore": false, "page": 1, "pageSize": 50, "totalCount": 1},
			}}})
			return
		}
		if r.Method != http.MethodPost || !bytes.Contains(body, []byte(`"query"`)) {
			w.WriteHeader(http.StatusUnauthorized)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"errors": []any{map[string]any{"message": msg}}})
	}))
}

// holderBins keeps one build of each binary for the whole run: a link per test
// took this package past go test's ten-minute default under emulated linux.
// ponytail: one lock, so builds run in turn; two binaries, so it costs nothing.
var holderBins = struct {
	sync.Mutex
	dir   string
	built map[string]string
}{built: map[string]string{}}

func TestMain(m *testing.M) {
	code := m.Run()
	if holderBins.dir != "" {
		_ = os.RemoveAll(holderBins.dir)
	}
	os.Exit(code)
}

func buildHolderBinary(t *testing.T, name, pkg string) string {
	t.Helper()
	holderBins.Lock()
	defer holderBins.Unlock()
	if bin, ok := holderBins.built[pkg]; ok {
		return bin
	}
	if holderBins.dir == "" {
		dir, err := os.MkdirTemp("", "superops-holder-bins")
		if err != nil {
			t.Fatal(err)
		}
		holderBins.dir = dir
	}
	bin := filepath.Join(holderBins.dir, name)
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, pkg).CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, out)
	}
	holderBins.built[pkg] = bin
	return bin
}

// holderEnv is a clean environment in sandbox: nothing from the caller's
// but PATH and the temp dir, so no other credential is held.
func holderEnv(sandbox string, vars map[string]string) []string {
	env := map[string]string{
		"PATH": os.Getenv("PATH"), "TMPDIR": os.Getenv("TMPDIR"), "TEMP": os.Getenv("TEMP"), "TMP": os.Getenv("TMP"),
		"HOME": sandbox, "USERPROFILE": sandbox,
		"APPDATA": filepath.Join(sandbox, "AppData", "Roaming"), "LOCALAPPDATA": filepath.Join(sandbox, "AppData", "Local"),
		"PRINTING_PRESS_DOGFOOD": "1",
	}
	for k, v := range vars {
		env[k] = strings.ReplaceAll(v, "{sandbox}", sandbox)
	}
	var out []string
	for k, v := range env {
		if v != "" {
			out = append(out, k+"="+v)
		}
	}
	return out
}

func runHolderCLI(t *testing.T, bin string, env []string, stdin string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	out, _ := cmd.CombinedOutput()
	return string(out)
}

// runHolderMCP starts the MCP server over stdio, makes each tools/call and
// returns every response line and the server's stderr.
func runHolderMCP(t *testing.T, bin string, env []string, args []string, calls ...map[string]any) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	send := func(msg map[string]any) {
		b, _ := json.Marshal(msg)
		_, _ = in.Write(append(b, '\n'))
	}
	var mu sync.Mutex
	var lines []string
	got := map[float64]bool{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(outPipe)
		sc.Buffer(make([]byte, 1<<20), 8<<20)
		for sc.Scan() {
			var m struct {
				ID float64 `json:"id"`
			}
			_ = json.Unmarshal(sc.Bytes(), &m)
			mu.Lock()
			lines = append(lines, sc.Text())
			got[m.ID] = true
			mu.Unlock()
		}
	}()
	send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "holder-test", "version": "0"},
	}})
	send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	for i, call := range calls {
		send(map[string]any{"jsonrpc": "2.0", "id": i + 2, "method": "tools/call", "params": call})
	}
	deadline := time.Now().Add(80 * time.Second)
	for {
		mu.Lock()
		all := true
		for i := range calls {
			all = all && got[float64(i+2)]
		}
		mu.Unlock()
		if all || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = in.Close()
	<-done
	_ = cmd.Wait()
	mu.Lock()
	defer mu.Unlock()
	for i := range calls {
		if !got[float64(i+2)] {
			t.Errorf("MCP call %d got no response; stderr:\n%s", i+2, stderr.String())
		}
	}
	return strings.Join(lines, "\n") + "\n" + stderr.String()
}

func assertHolderWithheld(t *testing.T, where, out string, values []string) {
	t.Helper()
	low := strings.ToLower(out)
	for _, form := range holderForms(values) {
		if strings.Contains(low, strings.ToLower(form)) {
			t.Errorf("%s shows %q:\n%s", where, form, out)
			return
		}
	}
	// The echo reached output and was removed: masked (the client's masks
	// cover its own credentials as written and encoded), withheld whole, or
	// (a store write) refused.
	if !strings.Contains(out, "withheld") && !strings.Contains(out, "credential ****") && !strings.Contains(out, "records not stored") {
		t.Errorf("%s: want the vendor's echo masked or withheld:\n%s", where, out)
	}
}

// One credential per source, each echoed back by the vendor as written, base64
// and hex, through fresh CLI processes (raw query's error, tickets list's
// successful output, sync into the store) and a fresh MCP server (the
// mirrored raw_query and the in-process tickets_list). No form is shown, at
// any length, and the whole run is grepped: every line either binary wrote
// and every file the run left in the sandbox but the operator's own
// credential files.
func TestEveryCredentialSourceIsWithheldFromCLIAndMCPOutput(t *testing.T) {
	t.Parallel()
	cliBin := buildHolderBinary(t, "superops-cli", "../superops-cli")
	mcpBin := buildHolderBinary(t, "superops-mcp", ".")
	for _, src := range holderSources() {
		t.Run(src.name, func(t *testing.T) {
			var requests atomic.Int64
			host, ca := serveHolderStub(t, src.values, &requests)
			sandbox := t.TempDir()
			vars := map[string]string{
				"SUPEROPS_BASE_URL":         "https://" + host,
				"SUPEROPS_CA_CERT":          ca,
				"SUPEROPS_API_TOKEN":        holderBaseToken,
				"SUPEROPS_SUBDOMAIN":        "acme-holder",
				"SUPEROPS_CLI_PATH":         cliBin,
				"SUPEROPS_LEARN_NO_CAPTURE": "1",
			}
			if src.env != nil {
				for k, v := range src.env(host) {
					vars[k] = v
				}
			}
			for rel, content := range src.files {
				p := filepath.Join(sandbox, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			env := holderEnv(sandbox, vars)
			if src.before != nil {
				out := runHolderCLI(t, cliBin, env, src.before.stdin, src.before.args...)
				assertHolderWithheldOrQuiet(t, "CLI "+strings.Join(src.before.args, " "), out, src.values)
			}

			before := requests.Load()
			out := runHolderCLI(t, cliBin, env, "", append(append([]string{}, src.cliArgs...), "raw", "query", holderRawQuery, "--json")...)
			assertHolderWithheld(t, "CLI raw query", out, src.values)
			if requests.Load() == before {
				t.Errorf("CLI raw query sent nothing:\n%s", out)
			}
			run := out
			// A delivered copy of stdout is one more sink; the sandbox grep reads it.
			list, delivered := []string{"tickets", "list", "--json"}, filepath.Join(sandbox, "delivered.json")
			if src.cliArgs == nil {
				list = append(list, "--deliver", "file:"+delivered)
			}
			for _, args := range [][]string{list, {"sync", "--resources", "tickets"}} {
				before = requests.Load()
				out = runHolderCLI(t, cliBin, env, "", append(append([]string{}, src.cliArgs...), args...)...)
				assertHolderWithheld(t, "CLI "+strings.Join(args, " "), out, src.values)
				if requests.Load() == before {
					t.Errorf("CLI %s sent nothing:\n%s", strings.Join(args, " "), out)
				}
				run += out
			}

			before = requests.Load()
			out = runHolderMCP(t, mcpBin, env, src.mcpArgs,
				map[string]any{"name": "raw_query", "arguments": map[string]any{"args": holderRawQuery}},
				map[string]any{"name": "tickets_list", "arguments": map[string]any{}},
			)
			assertHolderWithheld(t, "MCP raw_query and tickets_list", out, src.values)
			if requests.Load()-before < 2 {
				t.Errorf("MCP sent %d requests, want 2:\n%s", requests.Load()-before, out)
			}
			// The echo withholds the whole delivered copy: the path was reached and said so.
			if _, err := os.Stat(delivered); src.cliArgs == nil && err != nil && !strings.Contains(run, "withheld from file") {
				t.Errorf("tickets list neither delivered nor refused the file: %v\n%s", err, run)
			}
			assertRunWithheld(t, sandbox, run+out, src.values)
		})
	}
}

// The kit starts no child with a held value in its argv or environment, and
// hands a mirrored tool's companion CLI the API token and a password-carrying
// base URL itself. The request still goes out carrying them, and no process
// listing or child failure shows them.
func TestMCPChildGetsItsCredentialsFromTheKit(t *testing.T) {
	t.Parallel()
	cliBin := buildHolderBinary(t, "superops-cli", "../superops-cli")
	mcpBin := buildHolderBinary(t, "superops-mcp", ".")
	const heldValue = "childHandoffTok8842"
	var mu sync.Mutex
	var auth []string
	host, ca := serveHolderTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auth = append(auth, r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"__typename":"reachedTheStub"}}`)
	}))
	for name, base := range map[string]string{
		"plain base URL":           "https://" + host,
		"base URL with a password": "https://child-user:ChildPass4340x@" + host,
		"base URL with percent":    "https://child-user:Child%2FPass4340x@" + host,
	} {
		t.Run(name, func(t *testing.T) {
			mu.Lock()
			auth = nil
			mu.Unlock()
			sandbox := t.TempDir()
			env := holderEnv(sandbox, map[string]string{
				"SUPEROPS_BASE_URL": base, "SUPEROPS_CA_CERT": ca, "SUPEROPS_API_TOKEN": heldValue,
				"SUPEROPS_SUBDOMAIN": "acme-holder", "SUPEROPS_CLI_PATH": cliBin, "SUPEROPS_LEARN_NO_CAPTURE": "1",
			})
			out := runHolderMCP(t, mcpBin, env, nil, map[string]any{"name": "raw_query", "arguments": map[string]any{"args": holderRawQuery}})
			if strings.Contains(out, "child process") || !strings.Contains(out, "reachedTheStub") {
				t.Fatalf("the companion CLI did not answer:\n%s", out)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(auth) == 0 || !strings.Contains(auth[0], heldValue) {
				t.Errorf("the child's request did not carry the token: %q", auth)
			}
			assertRunWithheld(t, sandbox, out, []string{heldValue, "ChildPass4340x", "Child%2FPass4340x", "Child/Pass4340x"})
		})
	}
}

// assertHolderWithheldOrQuiet: a setup command may succeed (no echo to
// withhold) but must never show a form of the values.
func assertHolderWithheldOrQuiet(t *testing.T, where, out string, values []string) {
	t.Helper()
	low := strings.ToLower(out)
	for _, form := range holderForms(values) {
		if strings.Contains(low, strings.ToLower(form)) {
			t.Errorf("%s shows %q:\n%s", where, form, out)
			return
		}
	}
}

// assertRunWithheld greps a whole run: everything the binaries wrote, and
// every file under the sandbox but the operator's own credential files
// (config.toml and credentials.toml hold the credential by design). The
// store, its WAL, the response cache, receipts, the jobs ledger and the learn
// journal must hold no form of any value.
func assertRunWithheld(t *testing.T, sandbox, output string, values []string) {
	t.Helper()
	forms := holderForms(values)
	check := func(where, text string) {
		low := strings.ToLower(text)
		for _, form := range forms {
			if strings.Contains(low, strings.ToLower(form)) {
				t.Errorf("%s holds %q", where, form)
				return
			}
		}
	}
	check("the binaries' output", output)
	_ = filepath.WalkDir(sandbox, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == "config.toml" || d.Name() == "credentials.toml" {
			return nil
		}
		b, _ := os.ReadFile(p)
		rel, _ := filepath.Rel(sandbox, p)
		check(rel, string(b))
		return nil
	})
}
