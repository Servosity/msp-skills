// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written. Not generated. Recorded hand-fix `credkit-wiring` in
// handfixes.json: every credential source, driven through the real program
// with the shared credential kit wired in.

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base32"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quickbooks-pp-cli/internal/cli"
	"quickbooks-pp-cli/internal/cliutil"
	"quickbooks-pp-cli/internal/cliutil/testenv"
	"quickbooks-pp-cli/internal/credkit"
	"quickbooks-pp-cli/internal/credwire"
	"quickbooks-pp-cli/internal/store"
)

// TestMain hands a re-executed test binary to the real program: the MCP
// server's main, or the statements of the CLI's main. Every e2e run below goes
// through process start for real.
func TestMain(m *testing.M) {
	switch os.Getenv("QBO_E2E_MAIN") {
	case "mcp":
		main()
		return
	case "cli":
		credkit.Start(credwire.Declarations)
		credwire.RestoreBaseURLEnv()
		code := 0
		if err := cli.Execute(); err != nil {
			code = cli.ExitCode(err)
		}
		credkit.Exit(code)
	}
	os.Exit(m.Run())
}

const qboE2EBaseURL = "https://quickbooks.api.intuit.com/v3/company/1001"

// qboRun runs the CLI (mode "cli") with env added and returns stdout and
// stderr together.
func qboRun(t *testing.T, env []string, stdin string, args ...string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(append(os.Environ(), "QBO_E2E_MAIN=cli"), env...)
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("%v timed out: %s", args, out.String())
	}
	return out.String()
}

// qboMCP runs the MCP server over stdio, calls one tool (tools/list when tool
// is empty), and returns every byte the server wrote (stdout and stderr).
func qboMCP(t *testing.T, env []string, tool string, args map[string]any) string {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(append(os.Environ(), "QBO_E2E_MAIN=mcp"), env...)
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
	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	var all strings.Builder
	send := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = stdin.Write(append(b, '\n'))
	}
	// await reads until a line answers id, or the kit withheld it.
	await := func(id int) {
		deadline := time.After(60 * time.Second)
		for {
			select {
			case l, ok := <-lines:
				if !ok {
					return
				}
				all.WriteString(l + "\n")
				var m struct {
					ID *int `json:"id"`
				}
				if (json.Unmarshal([]byte(l), &m) == nil && m.ID != nil && *m.ID == id) || strings.Contains(l, "withheld") {
					return
				}
			case <-deadline:
				_ = cmd.Process.Kill()
				t.Fatalf("MCP %s: no answer to %d: %s%s", tool, id, all.String(), stderr.String())
			}
		}
	}
	send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "e2e", "version": "0"}}})
	await(1)
	send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	req := map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}}
	if tool == "" {
		req["method"], req["params"] = "tools/list", map[string]any{}
	}
	send(req)
	await(2)
	_ = stdin.Close()
	for l := range lines {
		all.WriteString(l + "\n")
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	return all.String() + stderr.String()
}

// qboSeed writes rows into a fresh store as an earlier release (or a run
// that did not hold the secret yet) would have. It must run before this
// test process holds the secret the rows carry.
func qboSeed(t *testing.T, dbPath string, rows ...string) {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var raw []json.RawMessage
	for _, r := range rows {
		raw = append(raw, json.RawMessage(r))
	}
	if _, _, err := db.UpsertBatch("customers", raw); err != nil {
		t.Fatal(err)
	}
}

// qboPlanted is the form a stored row repeats a credential in: base32,
// lowercase and unpadded, a form only a case-blind, length-blind check sees.
func qboPlanted(secret string) string {
	return strings.ToLower(strings.TrimRight(base32.StdEncoding.EncodeToString([]byte(secret)), "="))
}

// qboLeaks reports whether text carries secret in a form this test planted (as
// typed, base32 in either case, padded or not), checked independently of the
// code under test.
func qboLeaks(text, secret string) bool {
	b32 := base32.StdEncoding.EncodeToString([]byte(secret))
	for _, planted := range []string{secret, b32, strings.TrimRight(b32, "="), qboPlanted(secret)} {
		if strings.Contains(text, planted) {
			return true
		}
	}
	return false
}

func qboWriteFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

type qboSource struct {
	name   string
	secret string
	// prepare gives the credential to the program and returns the env every
	// later run gets. home is the test's isolated home.
	prepare func(t *testing.T, home string, env []string) []string
}

func qboSources() []qboSource {
	config := func(body string) func(*testing.T, string, []string) []string {
		return func(t *testing.T, home string, env []string) []string {
			p := filepath.Join(home, "qbo.toml")
			qboWriteFile(t, p, body)
			return append(env, "QUICKBOOKS_CONFIG="+p)
		}
	}
	return []qboSource{
		{"environment client secret", "Qz7v1x", func(_ *testing.T, _ string, env []string) []string {
			return append(env, "QUICKBOOKS_CLIENT_SECRET=Qz7v1x")
		}},
		{"environment MCP HTTP token (three characters)", "Q7z", func(_ *testing.T, _ string, env []string) []string {
			return append(env, "QUICKBOOKS_MCP_HTTP_TOKEN=Q7z")
		}},
		{"config access_token (short)", "Wv4tz9", config("access_token = \"Wv4tz9\"\n")},
		{"config auth_header after a scheme word", "k7Qp2xZr9", config("auth_header = \"Bearer k7Qp2xZr9\"\n")},
		{"config [headers] value", "Jd8sq2", config("[headers]\nX-Tenant-Key = \"Jd8sq2\"\n")},
		{"config auth_header parameter", "Tp5wq8z", config("auth_header = 'Token token=\"Tp5wq8z\", realm=\"api\"'\n")},
		{"password in the environment's base URL", "PwE2e9x", func(_ *testing.T, _ string, env []string) []string {
			return append(env, "QUICKBOOKS_BASE_URL=https://audituser:PwE2e9x@quickbooks.api.intuit.com/v3/company/1001")
		}},
		{"password in the config's base_url", "Pb6kw3z", config("base_url = \"https://cfguser:Pb6kw3z@quickbooks.api.intuit.com/v3/company/1001\"\n")},
		{"password in a base URL url.Parse refuses", "UrlOnlySecret9q", func(_ *testing.T, _ string, env []string) []string {
			return append(env, "QUICKBOOKS_BASE_URL=https://u:UrlOnlySecret9q@quickbooks.api.intuit.com:bad/v3/company/1")
		}},
		{"credential store, set from stdin", "Rt6yb3n", func(t *testing.T, _ string, env []string) []string {
			out := qboRun(t, env, "Rt6yb3n\n", "auth", "set-token")
			if qboLeaks(out, "Rt6yb3n") {
				t.Fatalf("auth set-token printed the token: %s", out)
			}
			return env
		}},
		{"refreshed token", "Fm2nc8v", func(t *testing.T, home string, env []string) []string {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"Fm2nc8v","refresh_token":"rfrsh-AAA111-rotated","expires_in":3600}`))
			}))
			t.Cleanup(srv.Close)
			p := filepath.Join(home, "qbo.toml")
			qboWriteFile(t, p, "client_id = \"cid-e2e\"\nclient_secret = \"cs-BBB222-e2e\"\nrefresh_token = \"rfrsh-AAA111\"\n")
			env = append(env, "QUICKBOOKS_CONFIG="+p)
			// The token endpoint is on the base URL's origin, the only test
			// origin the kit lets a client secret and a refresh token reach.
			out := qboRun(t, append(env, "QUICKBOOKS_TOKEN_URL="+srv.URL, "QUICKBOOKS_BASE_URL="+srv.URL, lkCAEnv(t, srv)), "", "auth", "refresh")
			if !strings.Contains(out, "refreshed") || qboLeaks(out, "Fm2nc8v") {
				t.Fatalf("auth refresh: %s", out)
			}
			return env
		}},
	}
}

// TestQBOEveryCredentialSourceReachesNoOutput gives the program one
// credential per source (environment, config field, auth header, [headers],
// the credential store from stdin, a refreshed token), each short enough
// that a length exemption would skip it, then drives the CLI and the MCP
// server for real: a stored row repeating it in lowercase unpadded base32, the dry-run request
// dump repeating it as typed, a delivered file, an MCP sql result, and every
// file the runs wrote. None may carry it in any encoding the check knows; a
// clean row still prints.
func TestQBOEveryCredentialSourceReachesNoOutput(t *testing.T) {
	for _, src := range qboSources() {
		t.Run(src.name, func(t *testing.T) {
			home := testenv.Isolate(t)
			seed := filepath.Join(home, "seed.db")
			clean := filepath.Join(home, "clean.db")
			stored, _ := json.Marshal(qboPlanted(src.secret))
			qboSeed(t, seed, `{"Id":"1","DisplayName":`+string(stored)+`,"Balance":10}`)
			qboSeed(t, clean, `{"Id":"2","DisplayName":"Clean Marker Co","Balance":5}`)
			env := src.prepare(t, home, []string{"QUICKBOOKS_BASE_URL=" + qboE2EBaseURL})
			seedEnv := append(env, "QUICKBOOKS_DB_PATH="+seed)
			delivered := filepath.Join(home, "delivered.json")

			outputs := map[string]string{
				"cli json":   qboRun(t, seedEnv, "", "customers", "top", "--by", "balance", "--json"),
				"cli table":  qboRun(t, seedEnv, "", "customers", "top", "--by", "balance"),
				"cli select": qboRun(t, seedEnv, "", "customers", "top", "--by", "balance", "--json", "--select", "nosuchfield"),
				"cli dry-run": qboRun(t, seedEnv, "", "customers", "create", "--display-name", "Acme",
					"--notes", src.secret, "--dry-run"),
				"cli deliver": qboRun(t, seedEnv, "", "customers", "top", "--by", "balance", "--json", "--deliver", "file:"+delivered),
				"mcp sql":     qboMCP(t, seedEnv, "sql", map[string]any{"query": "SELECT data FROM resources WHERE resource_type='customers'"}),
			}
			if b, err := os.ReadFile(delivered); err == nil {
				outputs["delivered file"] = string(b)
			}
			for name, out := range outputs {
				if qboLeaks(out, src.secret) {
					t.Errorf("%s carried the credential: %s", name, out)
				}
			}
			for _, name := range []string{"cli json", "cli dry-run", "mcp sql"} {
				if !strings.Contains(outputs[name], "withheld") {
					t.Errorf("%s was not withheld, so the case did not reach the credential: %s", name, outputs[name])
				}
			}

			// Every file the runs wrote, except the credential store itself and
			// the seeded store, which hold the credential by design.
			_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				base := filepath.Base(p)
				if strings.HasPrefix(base, "seed.db") || base == "qbo.toml" || base == "credentials.toml" || base == "config.toml" {
					return nil
				}
				if b, err := os.ReadFile(p); err == nil && qboLeaks(string(b), src.secret) {
					t.Errorf("file %s carries the credential", p)
				}
				return nil
			})

			// Output that repeats no credential is not withheld.
			cleanEnv := append(env, "QUICKBOOKS_DB_PATH="+clean)
			if out := qboRun(t, cleanEnv, "", "customers", "top", "--by", "balance", "--json"); !strings.Contains(out, "Clean Marker Co") {
				t.Errorf("a clean row did not print: %s", out)
			}
			if out := qboMCP(t, cleanEnv, "sql", map[string]any{"query": "SELECT data FROM resources"}); !strings.Contains(out, "Clean Marker Co") {
				t.Errorf("a clean MCP result did not arrive: %s", out)
			}
		})
	}
}

// TestQBOAFileThatFailsToDecodeLeaksNothing gives the program a config file
// and a credentials file that hold a credential and then fail to decode (a
// key defined twice, the credential as the key). A decode error names only
// its position, and what was decoded before it failed is held, so neither
// the CLI nor the MCP server repeats the credential.
func TestQBOAFileThatFailsToDecodeLeaksNothing(t *testing.T) {
	const secret = "Nb7tq2w"
	body := "access_token = \"" + secret + "\"\n[headers]\n" + secret + " = \"x\"\n" + secret + " = \"y\"\n"
	for _, name := range []string{"config file", "credentials file"} {
		t.Run(name, func(t *testing.T) {
			home := testenv.Isolate(t)
			env := []string{"QUICKBOOKS_BASE_URL=" + qboE2EBaseURL}
			if name == "config file" {
				p := filepath.Join(home, "qbo.toml")
				qboWriteFile(t, p, body)
				env = append(env, "QUICKBOOKS_CONFIG="+p)
			} else {
				dir, err := cliutil.DataDir()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				qboWriteFile(t, filepath.Join(dir, "credentials.toml"), body)
			}
			outputs := map[string]string{
				"auth status": qboRun(t, env, "", "auth", "status"),
				"customers":   qboRun(t, env, "", "customers", "top", "--by", "balance", "--json"),
				"mcp sql":     qboMCP(t, env, "sql", map[string]any{"query": "SELECT 1"}),
			}
			for out, text := range outputs {
				if qboLeaks(text, secret) {
					t.Errorf("%s carried the credential: %s", out, text)
				}
			}
		})
	}
}

// A URL that carries a secret (a webhook's path, a feedback endpoint's) is its
// own key: delivery has to send it, so it is not held, and no error or success
// line names it. A password in a URL is held.
func TestQBOAURLThatCarriesASecretLeaksNothing(t *testing.T) {
	home := testenv.Isolate(t)
	clean := filepath.Join(home, "clean.db")
	qboSeed(t, clean, `{"Id":"2","DisplayName":"Clean Marker Co","Balance":5}`)
	env := []string{"QUICKBOOKS_BASE_URL=" + qboE2EBaseURL, "QUICKBOOKS_DB_PATH=" + clean}
	for secret, out := range map[string]string{
		"WhSecret9q": qboRun(t, env, "", "customers", "top", "--by", "balance", "--json",
			"--deliver", "webhook:https://hooks.example.invalid/services/T0/B0/WhSecret9q"),
		"FbSecret7w": qboRun(t, append(env, "QUICKBOOKS_FEEDBACK_ENDPOINT=https://fb.example.invalid/hook/FbSecret7w"), "",
			"feedback", "the sync was slow", "--send"),
		"UrlOnlySecret8q": qboRun(t, append(env, "QUICKBOOKS_BASE_URL=https://u:UrlOnlySecret8q@quickbooks.api.intuit.com:bad/v3/company/1"), "",
			"accounts", "list", "--dry-run", "--data-source", "live"),
	} {
		if strings.Contains(out, secret) || qboLeaks(out, secret) {
			t.Errorf("a URL's secret %s printed: %s", secret, out)
		}
	}
}

// Codex next-batch review 3, round 1: profile save records --deliver without
// parsing the sink, so a webhook URL saved in a profile printed on save and on
// every later show. The class: a run profile never stores a webhook URL and
// never prints one an earlier release stored.
func TestQBOAProfileThatCarriesAWebhookLeaksNothing(t *testing.T) {
	testenv.Isolate(t)
	env := []string{"QUICKBOOKS_BASE_URL=" + qboE2EBaseURL}
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(dir, "profiles.json")
	out := qboRun(t, env, "", "profile", "save", "hook", "--json", "--deliver", "webhook:https://hooks.example.invalid/services/WebhookSecret9q")
	if b, err := os.ReadFile(store); err == nil && strings.Contains(string(b), "WebhookSecret9q") {
		t.Errorf("profile save stored the webhook URL: %s", b)
	}
	outputs := map[string]string{"profile save": out}
	// A profile an earlier release saved.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	qboWriteFile(t, store, `{"profiles":{"old":{"name":"old","values":{"deliver":"webhook:https://hooks.example.invalid/services/WebhookSecret8r","json":"true"}}}}`)
	outputs["profile show"] = qboRun(t, env, "", "profile", "show", "old")
	outputs["profile list"] = qboRun(t, env, "", "profile", "list", "--json")
	outputs["agent-context"] = qboRun(t, env, "", "agent-context")
	for name, out := range outputs {
		for _, secret := range []string{"WebhookSecret9q", "WebhookSecret8r"} {
			if strings.Contains(out, secret) || qboLeaks(out, secret) {
				t.Errorf("%s printed %s: %s", name, secret, out)
			}
		}
	}
}

// The real-shaped credential set a QuickBooks user holds: a long access
// token, a client secret and a refresh token.
var qboRealShapedEnv = []string{
	"QUICKBOOKS_BASE_URL=" + qboE2EBaseURL,
	"QUICKBOOKS_ACCESS_TOKEN=" + strings.Repeat("eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0-_x", 20)[:1000],
	"QUICKBOOKS_CLIENT_SECRET=clsSENTz7Rk4client.0a1+2b3/4c5=6d7e8f",
	"QUICKBOOKS_REFRESH_TOKEN=AB11" + strings.Repeat("x7Qp", 12),
}

// verify-msp-skills on the ADOPT head: a whole-text search with every
// decoding at every depth ran past its work cap on the server's own
// tools/list (110 KB) and withheld it, so every MCP session failed. The
// class: ordinary output, the program's own and the API's, is never
// withheld for the cost of checking it.
func TestQBOOrdinaryOutputIsNotWithheld(t *testing.T) {
	testenv.Isolate(t)
	if out := qboMCP(t, qboRealShapedEnv, "", nil); strings.Contains(out, "withheld") || !strings.Contains(out, `"quickbooks_read"`) {
		t.Errorf("tools/list was withheld or incomplete: %.300s", out)
	}
	if out := qboRun(t, qboRealShapedEnv, "", "--help"); strings.Contains(out, "withheld") || !strings.Contains(out, "Usage") {
		t.Errorf("--help was withheld: %.300s", out)
	}
}
