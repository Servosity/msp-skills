// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): each surface the
// credential kit covers, driven through the built binaries with synthetic data.

package credwire

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"servosity-msp-pp-cli/internal/cliutil"
	"servosity-msp-pp-cli/internal/credkit"
)

// The config keys the per-read hold leaves unheld are exactly the config keys
// the kit exempts, so the two never disagree about what a credential is.
func TestIdentifiersMatchNotCredentials(t *testing.T) {
	t.Parallel()
	var exempt []string
	for _, id := range Declarations.Identifiers {
		if k, ok := strings.CutPrefix(id.Source, "config:"); ok {
			exempt = append(exempt, k)
		}
	}
	want := slices.Clone(cliutil.NotCredentials)
	slices.Sort(exempt)
	slices.Sort(want)
	if !slices.Equal(exempt, want) {
		t.Fatalf("config: Identifiers %q, cliutil.NotCredentials %q: the kit and the per-read hold disagree", exempt, want)
	}
}

// The profile store hold reads cliutil's copy of the credential flags, so the
// two never disagree about which saved values are credentials.
func TestCredentialFlagsMatchCliutil(t *testing.T) {
	t.Parallel()
	if !slices.Equal(Declarations.CredentialFlags, cliutil.CredentialFlags) {
		t.Fatalf("Declarations.CredentialFlags %q, cliutil.CredentialFlags %q: the kit and the profile store hold disagree", Declarations.CredentialFlags, cliutil.CredentialFlags)
	}
}

// A mirrored tool's child inherits none of the variables the kit holds, or the
// kit would refuse to start it. The two lists cannot share a declaration:
// credwire imports cliutil.
func TestSecretEnvNamesMatchDeclarations(t *testing.T) {
	t.Parallel()
	if !slices.Equal(Declarations.SecretEnv, cliutil.SecretEnvNames) {
		t.Fatalf("Declarations.SecretEnv %q, cliutil.SecretEnvNames %q", Declarations.SecretEnv, cliutil.SecretEnvNames)
	}
	if !cliutil.IsSecretEnvEntry("servosity_msp_token=x") || cliutil.IsSecretEnvEntry("SERVOSITY_MSP_TOKENS=x") {
		t.Error("IsSecretEnvEntry must compare the whole name without case")
	}
}

// An API answer that echoes the token, whole or encoded, in a 200 body or an
// error body, reaches no output, no MCP result and no file.
func TestServerAnswerEchoingTheToken(t *testing.T) {
	t.Parallel()
	cli, mcp := binaries(t)
	b64 := base64.StdEncoding.EncodeToString([]byte(syntheticToken))
	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"200 as written": {200, `{"count":1,"next":null,"results":[{"id":1,"note":"` + syntheticToken + `"}]}`},
		"200 base64":     {200, `{"count":1,"next":null,"results":[{"id":1,"note":"` + b64 + `"}]}`},
		"400 as written": {400, `{"detail":"bad token ` + syntheticToken + `"}`},
		"400 base64":     {400, `{"detail":"bad token ` + b64 + `"}`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newSandbox(t)
			f := newFakeAPI(t, s)
			f.setAnswer(tc.status, tc.body)
			env := s.apiEnv(t, f, "", syntheticToken)
			forms := []string{syntheticToken, b64}
			res := run(t, cli, env, "resellers", "issues", "resellers", "7", "--json", "--no-cache")
			requireAbsent(t, "resellers issues", res.all(), forms...)
			frames, stderr := mcpCall(t, mcp, env, "servosity-msp_read", map[string]any{
				"endpoint_id": "resellers.issues.resellers", "params": map[string]any{"id": "7"},
			})
			requireAbsent(t, "MCP servosity-msp_read", frames+stderr, forms...)
			if hits := s.filesHolding(t, forms...); len(hits) > 0 {
				t.Errorf("the credential reached a file: %v", hits)
			}
			if !f.sawAuth("Token " + syntheticToken) {
				t.Errorf("the API never saw the token, so this test proved nothing: %s", res.all())
			}
		})
	}
}

// Every issues watch failure line passes the kit: a list that fails with the
// token in its body never shows it.
func TestIssuesWatchFailureEchoingTheToken(t *testing.T) {
	t.Parallel()
	cli, _ := binaries(t)
	s := newSandbox(t)
	f := newFakeAPI(t, s)
	f.setAnswer(500, `{"detail":"bad token `+syntheticToken+`"}`)
	res := run(t, cli, s.apiEnv(t, f, "", syntheticToken), "issues", "watch", "--once")
	requireAbsent(t, "issues watch", res.all(), syntheticToken)
	if res.code == 0 {
		t.Errorf("a failed list exited 0: %s", res.all())
	}
}

// A config value the TOML parser overflows on, in the diagnostic it prints, at
// the commands that read the file at startup and after it.
func TestConfigParseErrorNeverShowsTheValue(t *testing.T) {
	t.Parallel()
	cli, mcp := binaries(t)
	const secret = "123456789012345678901234567890"
	s := newSandbox(t)
	cfg := s.writePrivate(t, "broken.toml", "auth_header = "+secret+"\n")
	env := s.env("SERVOSITY_MSP_CONFIG=" + cfg)
	for _, args := range [][]string{{"doctor", "--json"}, {"resellers", "list", "--json"}, {"--config", cfg, "version"}} {
		res := run(t, cli, env, args...)
		requireAbsent(t, strings.Join(args, " "), res.all(), secret)
	}
	frames, stderr := mcpCall(t, mcp, env, "servosity-msp_read", map[string]any{
		"endpoint_id": "resellers.issues.resellers", "params": map[string]any{"id": "7"},
	})
	if strings.Contains(frames, "unknown endpoint_id") {
		t.Fatalf("the call never loaded the config, so this test proved nothing; frames:\n%s", frames)
	}
	requireAbsent(t, "MCP servosity-msp_read", frames+stderr, secret)
}

// The MCP server reloads its config on every tool call, so a file rewritten
// while it runs is held on that read, before the parser can print it.
func TestMCPConfigReloadHoldsBeforeParse(t *testing.T) {
	t.Parallel()
	_, mcp := binaries(t)
	for _, tc := range []struct {
		name, body, carried string
		silent              bool
	}{
		{name: "an overflowing number", body: "auth_header = 123456789012345678901234567890\n", carried: "123456789012345678901234567890"},
		// A one-character held value matches nearly every unit, its own
		// refusal included, so the server has nothing it may print: silence.
		{name: "a value with no letter or digit", body: "msp_token = \"!\"\n!\n", carried: "at start of key: !", silent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mcpReloadCarriesNothing(t, mcp, tc.body, tc.carried, tc.silent)
		})
	}
}

func mcpReloadCarriesNothing(t *testing.T, mcp, body, carried string, silent bool) {
	t.Helper()
	s := newSandbox(t)
	cfg := s.writePrivate(t, "reload.toml", "auth_header = \"Token Pq4oldvalue9Xz\"\n")
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, mcp)
	cmd.Env = s.env("SERVOSITY_MSP_CONFIG=" + cfg)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		_, _ = io.Copy(io.Discard, stdout)
		_ = cmd.Wait()
	}()
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	var frames strings.Builder
	await := func(id string) bool {
		t.Helper()
		for sc.Scan() {
			line := sc.Text()
			frames.WriteString(line + "\n")
			var msg struct {
				ID json.RawMessage `json:"id"`
			}
			if json.Unmarshal([]byte(line), &msg) == nil && string(msg.ID) == id {
				return true
			}
		}
		return false
	}
	send := func(m string) {
		t.Helper()
		if _, err := io.WriteString(stdin, m+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"credkit-reload-test","version":"1"}}}`)
	if !await("1") {
		t.Fatalf("the MCP server never initialized; stderr:\n%s", errb.String())
	}
	send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	send(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"servosity-msp_read","arguments":{"endpoint_id":"resellers.issues.resellers","params":{"id":"7"}}}}`)
	if !await("2") {
		if !silent {
			t.Fatalf("the MCP server never answered; frames:\n%s\nstderr:\n%s", frames.String(), errb.String())
		}
		requireAbsent(t, "MCP frames", frames.String(), carried)
		return
	}
	if !strings.Contains(frames.String(), `"isError":true`) || strings.Contains(frames.String(), "unknown endpoint_id") {
		t.Fatalf("the call did not fail on the unparseable config, so this test proved nothing; frames:\n%s", frames.String())
	}
	requireAbsent(t, "MCP frames", frames.String(), carried)
	requireAbsent(t, "MCP stderr", errb.String(), carried)
}

// A statement carrying a held value is refused at the SQLite sink, before
// SQLite runs it, and an ordinary one still runs.
func TestMCPSQLStatementWithheldAtSQLite(t *testing.T) {
	t.Parallel()
	cli, mcp := binaries(t)
	const secret = "zq7Wsink" + "valu3Kx"
	s := newSandbox(t)
	f := newFakeAPI(t, s, issueJSON(cleanA, "11"))
	env := s.apiEnv(t, f, "", secret)
	if res := run(t, cli, env, "issues", "watch", "--once", "--json"); res.code != 0 {
		t.Fatalf("could not create the store (exit %d): %s", res.code, res.all())
	}
	frames, stderr := mcpCall(t, mcp, env, "sql", map[string]any{"query": "SELECT '" + secret + "' AS sample"})
	requireAbsent(t, "MCP sql", frames+stderr, secret)
	if !strings.Contains(frames, credkit.Notice(credkit.SinkSQLite)) {
		t.Errorf("the statement was not refused at the SQLite sink; frames:\n%s", frames)
	}
	frames, _ = mcpCall(t, mcp, env, "sql", map[string]any{"query": "SELECT 'an ordinary value' AS sample"})
	if !strings.Contains(frames, "an ordinary value") {
		t.Errorf("an ordinary statement did not run; frames:\n%s", frames)
	}
}

// A store write through the kit that is refused leaves the program working:
// the next ordinary run opens the same store and reads what the clean run kept,
// and the second open applies no migration (the schema stamp is on the file).
func TestStoreStaysUsableAcrossOpens(t *testing.T) {
	t.Parallel()
	cli, _ := binaries(t)
	s := newSandbox(t)
	f := newFakeAPI(t, s, issueJSON(cleanA, "11"))
	env := s.apiEnv(t, f, "", syntheticToken)
	if res := run(t, cli, env, "issues", "watch", "--once", "--json"); res.code != 0 {
		t.Fatalf("baseline (exit %d): %s", res.code, res.all())
	}
	f.setIssues(issueJSON(cleanA, "11"), issueJSON(late, "12"))
	if res := run(t, cli, env, "issues", "watch", "--once", "--json"); res.code != 0 {
		t.Fatalf("second poll (exit %d): %s", res.code, res.all())
	}
	res := run(t, cli, env, "events", "list", "--since", "0", "--json")
	if !strings.Contains(res.stdout, strings.Trim(late, `"`)) {
		t.Errorf("the second poll's issue has no event: %s", res.all())
	}
}

// A credential file named on the command line is held at startup, whichever
// spelling names it, so a parse error or a stored note cannot carry its value.
func TestCommandLineConfigIsHeld(t *testing.T) {
	t.Parallel()
	cli, _ := binaries(t)
	const secret = "Hv8cmdlinevalu3Rt"
	s := newSandbox(t)
	cfg := s.writePrivate(t, "named.toml", "auth_header = \"Token "+secret+"\"\n")
	for _, args := range [][]string{
		{"--config", cfg, "--bogus-flag", secret},
		{"--config=" + cfg, "--bogus-flag", secret},
		{"recall", secret, "--config", cfg},
	} {
		res := run(t, cli, s.env(), args...)
		requireAbsent(t, strings.Join(args[:1], " "), res.all(), secret)
		if hits := s.filesHolding(t, secret); len(hits) > 0 {
			t.Errorf("%v reached %v", args, hits)
		}
	}
}

// A saved run profile is a credential source: a value under any declared
// credential flag's name (any key case, each duplicate key) is held on every
// read of the store, so no profile command can print it. A value under any
// other name is data and still prints.
func TestProfileCredentialFlagValuesAreHeld(t *testing.T) {
	t.Parallel()
	cli, _ := binaries(t)
	for _, flag := range Declarations.CredentialFlags {
		name := strings.TrimPrefix(flag, "--")
		for _, key := range []string{name, strings.ToUpper(name)} {
			first := "Q7x9" + "pV2kL4m8R6s3" + key
			last := "Z3k8" + "wQ5nH2j7T4u9" + key
			s := newSandbox(t)
			store := `{"profiles":{"p":{"name":"p","values":{"` + key + `":"` + first + `","` + key + `":"` + last + `","limit":"plainlimit25"}}}}`
			if err := os.MkdirAll(filepath.Join(s.home, "config"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(s.home, "config", "profiles.json"), []byte(store), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{{"profile", "show", "p", "--json"}, {"profile", "use", "p"}, {"profile", "list", "--json"}} {
				res := run(t, cli, s.env(), args...)
				requireAbsent(t, key+": "+strings.Join(args, " "), res.all(), first, last)
			}
		}
	}
	s := newSandbox(t)
	if err := os.MkdirAll(filepath.Join(s.home, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.home, "config", "profiles.json"), []byte(`{"profiles":{"p":{"name":"p","values":{"limit":"plainlimit25"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if res := run(t, cli, s.env(), "profile", "show", "p", "--json"); !strings.Contains(res.stdout, "plainlimit25") {
		t.Errorf("a non-credential profile value no longer prints:\n%s", res.all())
	}
}

// A plain sync with the token held, on the built binary, where the kit scans
// what sync stores: a resource that is really a binary must not reach the store
// (one 4 MiB body there ran past a minute and gigabytes), a redirect off the
// origin is not followed, and both are named in the summary.
func TestPlainSyncSkipsBinaryResources(t *testing.T) {
	t.Parallel()
	cli, _ := binaries(t)
	s := newSandbox(t)
	f := newFakeAPI(t, s)
	var offOrigin atomic.Int32
	elsewhere := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { offOrigin.Add(1) }))
	t.Cleanup(elsewhere.Close)
	binary := bytes.Repeat([]byte("MZ\x90\x00"), 2<<20) // 8 MiB
	ordinary := f.Config.Handler
	f.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stats/":
			http.Redirect(w, r, "/installer.bin", http.StatusFound)
		case "/installer.bin":
			w.Header().Set("Content-Type", "binary/octet-stream")
			_, _ = w.Write(binary)
		case "/reports/usage/":
			http.Redirect(w, r, elsewhere.URL+"/installer.bin", http.StatusFound)
		default:
			ordinary.ServeHTTP(w, r)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cli, "sync", "--json")
	cmd.Env = s.apiEnv(t, f, "", syntheticToken)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("sync did not finish in 90s\n%s", stderr.String())
	}
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, stderr.String())
	}
	if n := offOrigin.Load(); n != 0 {
		t.Errorf("%d request(s) followed the redirect off the origin", n)
	}
	var summary string
	for _, line := range strings.Split(stderr.String(), "\n") {
		if strings.Contains(line, `"event":"sync_summary"`) {
			summary = line
		}
	}
	for _, name := range []string{`"stats"`, `"reports-usage"`} {
		if !strings.Contains(summary, name) {
			t.Errorf("summary does not name %s: %s", name, summary)
		}
	}
	if !f.sawAuth("Token " + syntheticToken) {
		t.Errorf("the API never saw the token, so this test proved nothing:\n%s", stderr.String())
	}
}
