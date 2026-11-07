// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class's failing input, seen
// failing on the binaries built before the kit was wired. Set
// CONNECTWISE_AUTOMATE_LEAK_BIN_DIR to run them against binaries built
// elsewhere.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectwise-automate-pp-cli/internal/credkit"
)

var (
	binOnce sync.Once
	binDir  string
	binErr  error
)

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func binaries(t *testing.T) (cliBin, mcpBin string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("drives Unix config-file modes; the Windows build is vetted and compiled by the gates")
	}
	if testing.Short() {
		t.Skip("builds and runs both binaries")
	}
	binOnce.Do(func() {
		if d := os.Getenv("CONNECTWISE_AUTOMATE_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "cwa-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{exe("connectwise-automate-cli"), "../../cmd/connectwise-automate-cli"},
			{exe("connectwise-automate-mcp"), "../../cmd/connectwise-automate-mcp"},
		} {
			if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, b.out), b.pkg).CombinedOutput(); err != nil {
				binErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, exe("connectwise-automate-cli")), filepath.Join(binDir, exe("connectwise-automate-mcp"))
}

// vendor is a TLS stub of an Automate server: the kit never sends a
// credential over plain http, loopback included.
type vendor struct {
	url, host, caFile string
	hits              atomic.Int64
	conns             atomic.Int64
}

func startVendor(t *testing.T, h http.HandlerFunc) *vendor {
	t.Helper()
	v := &vendor{}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}))
	// A connection is counted before TLS: a refused request never dials.
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			v.conns.Add(1)
		}
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	v.url = srv.URL
	v.host = strings.TrimPrefix(srv.URL, "https://")
	v.caFile = filepath.Join(t.TempDir(), "ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(v.caFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return v
}

func (v *vendor) env(token string) []string {
	return []string{"CONNECTWISE_AUTOMATE_BASE_URL=" + v.url, "CONNECTWISE_AUTOMATE_CA_CERT=" + v.caFile, "CONNECTWISE_AUTOMATE_TOKEN=" + token}
}

// closedEnv is a closed environment under one sandbox home; every press
// directory resolves under it.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "CONNECTWISE_AUTOMATE_HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg-config"), "XDG_DATA_HOME=" + filepath.Join(home, "xdg-data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "xdg-state"), "XDG_CACHE_HOME=" + filepath.Join(home, "xdg-cache"),
		"TMPDIR=" + os.TempDir(), "NO_COLOR=1"}, extra...)
}

func configDir(home string) string { return filepath.Join(home, "config") }

func run(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	out, code := runCode(t, bin, env, args...)
	// A refused start prints nothing, which every absence check would pass on.
	if code == credkit.ExitRefused {
		t.Errorf("%s %s: the start was refused, so this run proves nothing:\n%s", filepath.Base(bin), strings.Join(args[:min(3, len(args))], " "), out)
	}
	return out
}

func runCode(t *testing.T, bin string, env []string, args ...string) (string, int) {
	t.Helper()
	return runStdinCode(t, bin, env, "", args...)
}

// runStdin is run with stdin.
func runStdin(t *testing.T, bin string, env []string, stdin string, args ...string) string {
	t.Helper()
	out, code := runStdinCode(t, bin, env, stdin, args...)
	if code == credkit.ExitRefused {
		t.Errorf("%s %s: the start was refused:\n%s", filepath.Base(bin), strings.Join(args, " "), out)
	}
	return out
}

func runStdinCode(t *testing.T, bin string, env []string, stdin string, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run() // exit codes vary by case; the output is judged
	return out.String(), cmd.ProcessState.ExitCode()
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertAbsent(t *testing.T, label, out string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if strings.Contains(out, s) {
			t.Errorf("%s shows %q:\n%s", label, s, out)
		}
	}
}

// assertNoFileCarries walks home; skip names the files allowed to carry a
// secret (the ones the test wrote, the credential store).
func assertNoFileCarries(t *testing.T, home string, skip []string, secrets ...string) {
	t.Helper()
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || slices.Contains(skip, p) {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		for _, s := range secrets {
			if bytes.Contains(data, []byte(s)) {
				t.Errorf("%s holds %q", p, s)
			}
		}
		return nil
	})
}

// mcpCall runs one MCP stdio session of tools/call params and returns stdout
// plus stderr, with every call answered.
func mcpCall(t *testing.T, bin string, env []string, calls ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = env
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	fmt.Fprintln(stdin, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	for i, c := range calls {
		fmt.Fprintf(stdin, `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":%s}`+"\n", i+2, c)
	}
	var out strings.Builder
	answered := 0
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for answered < len(calls) && sc.Scan() {
		out.WriteString(sc.Text() + "\n")
		var msg struct {
			ID *int `json:"id"`
		}
		if json.Unmarshal(sc.Bytes(), &msg) == nil && msg.ID != nil && *msg.ID >= 2 {
			answered++
		}
	}
	_ = stdin.Close()
	_, _ = io.Copy(&out, stdout)
	_ = cmd.Wait()
	if answered != len(calls) {
		t.Errorf("MCP answered %d of %d calls:\n%s\n%s", answered, len(calls), out.String(), stderr.String())
	}
	return out.String() + stderr.String()
}

// TestAPIErrorEchoOfTokenIsWithheld (class token-echo): an API error body that
// echoes part of CONNECTWISE_AUTOMATE_TOKEN. The generated masks replace the
// whole token and its encodings; a server that echoes a piece of it slipped
// past them. Any 8-character piece of a held value withholds the output.
func TestAPIErrorEchoOfTokenIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const token = "cwatok-" + "Zq81LmP0xV7c"
	piece := token[:12]
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"Message":"token %s... rejected"}`, piece)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env(token)...)
	assertAbsent(t, "computers list", run(t, cliBin, env, "computers", "list"), piece)
	assertAbsent(t, "computers list --agent", run(t, cliBin, env, "computers", "list", "--agent"), piece)
	assertAbsent(t, "MCP computers_list", mcpCall(t, mcpBin, env, `{"name":"computers_list","arguments":{}}`), piece)
	if v.hits.Load() == 0 {
		t.Error("no request reached the API stub, so nothing was echoed")
	}
	assertNoFileCarries(t, home, nil, piece)
}

// TestServerTemplateIsTheAllowedOrigin (class base-url): the default base URL
// is the template https://{server}/cwa/api/v1. With CONNECTWISE_AUTOMATE_SERVER
// set, the token and the clientId header reach that server and nothing else;
// without a server the template names no host and the token is sent nowhere.
func TestServerTemplateIsTheAllowedOrigin(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const token = "cwatok-" + "S3rv3rT9mpl8"
	const clientID = "cwaclient-" + "0a1b2c3d4e5f"
	var bearer, client atomic.Int64
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer "+token && strings.HasPrefix(r.URL.Path, "/cwa/api/v1/") {
			bearer.Add(1)
		}
		if r.Header.Get("clientId") == clientID {
			client.Add(1)
		}
		io.WriteString(w, `[{"Id":1,"ComputerName":"ws-01"}]`)
	})
	home := t.TempDir()
	env := closedEnv(home, "CONNECTWISE_AUTOMATE_SERVER="+v.host, "CONNECTWISE_AUTOMATE_CA_CERT="+v.caFile,
		"CONNECTWISE_AUTOMATE_TOKEN="+token, "CONNECTWISE_AUTOMATE_CLIENT_ID="+clientID)
	out := run(t, cliBin, env, "computers", "list", "--json")
	if !strings.Contains(out, "ws-01") {
		t.Errorf("computers list against the configured server lost its answer:\n%s", out)
	}
	mcpCall(t, mcpBin, env, `{"name":"computers_list","arguments":{}}`)
	if n := bearer.Load(); n < 2 {
		t.Errorf("the configured server saw the token on %d request(s), want the CLI's and the MCP server's", n)
	}
	if n := client.Load(); n < 2 {
		t.Errorf("the configured server saw the clientId header on %d request(s), want the CLI's and the MCP server's", n)
	}

	// No server: the shipped placeholder host is never an allowed origin.
	env = closedEnv(t.TempDir(), "CONNECTWISE_AUTOMATE_TOKEN="+token)
	out, _ = runCode(t, cliBin, env, "computers", "list", "--json")
	assertAbsent(t, "computers list without a server", out, token)
	if !strings.Contains(out, "credkit") {
		t.Errorf("computers list without a server was not refused by the kit:\n%s", out)
	}
}

// TestProfileCredentialValueHeldAtStartup (class profile-value): a value saved
// in profiles.json under a credential flag's name (--password) is held at
// startup.
func TestProfileCredentialValueHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "cwaProf" + "Pass-5d4c3b2a19"
	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "profiles.json"),
		`{"profiles":{"minter":{"name":"minter","values":{"password":"`+secret+`","username":"tech.user"}}}}`)
	env := closedEnv(home)
	assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", secret, "version"), secret)
	assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", secret), secret)
	assertAbsent(t, "profile show", run(t, cliBin, env, "profile", "show", "minter"), secret)
}

// TestStartupOutputNeverShowsMalformedConfigCredential (class startup): a
// credential inside a config file that does not parse is held before either
// binary writes its first byte, so an error that echoes an argument never
// shows it. The kit holds every word of a file that does not parse, and one
// of them occurs in the program's public text, so it refuses the start
// (credkit.ExitRefused) and prints its one-line notice: never the credential.
// The default path, --config and CONNECTWISE_AUTOMATE_CONFIG all count.
func TestStartupOutputNeverShowsMalformedConfigCredential(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "cwaStart" + "upTok7Kq9xZ"
	const body = "automate_token = \"" + secret + "\"\nbad = [\n"
	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "config.toml"), body)
	other := filepath.Join(t.TempDir(), "other.toml")
	writeFile(t, other, body)
	for _, c := range []struct {
		label string
		bin   string
		env   []string
		args  []string
	}{
		{"CLI unknown command, default config", cliBin, closedEnv(home), []string{secret}},
		{"MCP unknown transport, default config", mcpBin, closedEnv(home), []string{"--transport", secret}},
		{"CLI unknown command, --config", cliBin, closedEnv(t.TempDir()), []string{"--config", other, secret}},
		{"MCP unknown transport, CONNECTWISE_AUTOMATE_CONFIG", mcpBin, closedEnv(t.TempDir(), "CONNECTWISE_AUTOMATE_CONFIG="+other), []string{"--transport", secret}},
	} {
		out, code := runCode(t, c.bin, c.env, c.args...)
		assertAbsent(t, c.label, out, secret)
		if code == 0 {
			t.Errorf("%s: exit 0:\n%s", c.label, out)
		}
	}
}

// TestHeldCredentialRefusedOffOriginAndOverPlainHTTP (class origin): the token
// goes only to the server it is configured for, and never over plain http,
// not even to loopback. A --deliver webhook whose URL carries the token is
// refused before it dials.
func TestHeldCredentialRefusedOffOriginAndOverPlainHTTP(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "cwatok-" + "Orig1nQ7r3s5"
	api := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `[{"Id":1,"ComputerName":"ws-01"}]`)
	})
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	var plainHits atomic.Int64
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plainHits.Add(1)
		io.WriteString(w, `{}`)
	}))
	t.Cleanup(plain.Close)

	home := t.TempDir()
	env := closedEnv(home, api.env(token)...)
	run(t, cliBin, env, "computers", "list", "--json", "--deliver", "webhook:"+other.url+"/hook/"+token)
	if n := other.conns.Load(); n != 0 {
		t.Errorf("webhook to another https origin: %d connection(s) opened for a URL that carries the token", n)
	}
	run(t, cliBin, env, "computers", "list", "--json", "--deliver", "webhook:"+plain.URL+"/hook/"+token)
	if n := plainHits.Load(); n != 0 {
		t.Errorf("webhook over plain http: %d request(s) carried the token", n)
	}
	if api.hits.Load() == 0 {
		t.Error("the API stub was never read, so no webhook was attempted")
	}

	// The API base URL itself over plain http: the token is never sent.
	var bearerHits atomic.Int64
	plainAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization"), token) {
			bearerHits.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `[]`)
	}))
	t.Cleanup(plainAPI.Close)
	env = closedEnv(t.TempDir(), "CONNECTWISE_AUTOMATE_BASE_URL="+plainAPI.URL, "CONNECTWISE_AUTOMATE_TOKEN="+token)
	assertAbsent(t, "plain http API", run(t, cliBin, env, "computers", "list"), token)
	if n := bearerHits.Load(); n != 0 {
		t.Errorf("plain http API: %d request(s) carried the token", n)
	}
}

// TestSpacefastTokenHeld (class spacefast-token): SPACEFAST_TOKEN, the
// credential of the --deliver spacefast: sink, is held from startup in both
// binaries.
func TestSpacefastTokenHeld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "spacefastTok" + "4b7c9d1e2f3a"
	env := closedEnv(t.TempDir(), "SPACEFAST_TOKEN="+secret)
	assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", secret, "version"), secret)
	assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", secret), secret)
	assertAbsent(t, "CLI deliver target error", run(t, cliBin, env, "version", "--deliver", "spacefast:"+secret+"/x"), secret)
}

// TestJournalNeverRecordsAHeldCredential (class journal): the learn journal
// records a mistyped flag's name. A credential pasted where a flag name goes
// (--<token>) was written to state/learn/journal-*.jsonl; the journal appends
// through the kit (credkit.AppendFile), which withholds an entry carrying a
// held value.
func TestJournalNeverRecordsAHeldCredential(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "cwatok" + "JournL8x2q9"
	home := t.TempDir()
	env := closedEnv(home, "CONNECTWISE_AUTOMATE_TOKEN="+token)
	run(t, cliBin, env, "computers", "list", "--"+token)
	run(t, cliBin, env, "computers", "list", "--condtion", token)
	journals, _ := filepath.Glob(filepath.Join(home, "state", "learn", "journal-*.jsonl"))
	if len(journals) == 0 {
		t.Fatal("no journal segment was written, so nothing was checked")
	}
	assertNoFileCarries(t, home, nil, token)
}

// TestFeedbackStdinIsRecorded (class missing data): `feedback --stdin` is a
// StdinFlags name, so the kit would hold the whole body and withhold the local
// record. Feedback text is no credential, so its body is taken off stdin
// before the kit reads it; a held credential pasted into it is still withheld.
func TestFeedbackStdinIsRecorded(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "cwatokFeedbk3Lp7q"
	const text = "the computers list output was confusing"
	home := t.TempDir()
	env := closedEnv(home, "CONNECTWISE_AUTOMATE_TOKEN="+token)
	runStdin(t, cliBin, env, text+"\n", "feedback", "--stdin")
	runStdin(t, cliBin, env, "pasted "+token+" by mistake\n", "feedback", "--stdin")
	record, err := os.ReadFile(filepath.Join(home, "data", "feedback.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(record), text) {
		t.Errorf("feedback.jsonl lost the feedback:\n%s", record)
	}
	assertNoFileCarries(t, home, nil, token)
}

// TestMCPWritePreviewNeverEchoesACredential (kit note a): apitoken_mint and
// apitoken_refresh preview until confirm is true, and the preview repeated
// every argument, the password, the two-factor passcode and the current token
// included. Both tools now refuse over MCP before any preview (minting is
// human-in-the-loop only), so neither answer carries a credential argument.
func TestMCPWritePreviewNeverEchoesACredential(t *testing.T) {
	_, mcpBin := binaries(t)
	const password = "cwaPrevw" + "Pass-8e2d4f6a"
	const passcode = "cwa2fa" + "-731904"
	const token = "cwatok-" + "Pr3v13wR9fr4"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	env := closedEnv(t.TempDir(), "CONNECTWISE_AUTOMATE_BASE_URL="+v.url, "CONNECTWISE_AUTOMATE_CA_CERT="+v.caFile)
	mint, _ := json.Marshal(map[string]any{"name": "apitoken_mint", "arguments": map[string]any{
		"username": "tech.user", "password": password, "two-factor": passcode}})
	refresh, _ := json.Marshal(map[string]any{"name": "apitoken_refresh", "arguments": map[string]any{"token": token}})
	out := mcpCall(t, mcpBin, env, string(mint), string(refresh))
	assertAbsent(t, "MCP unconfirmed previews", out, password, passcode, token)
	if n := strings.Count(out, "human-in-the-loop only"); n != 2 {
		t.Errorf("want two refusals naming the CLI command, got %d:\n%s", n, out)
	}
	if n := v.hits.Load(); n != 0 {
		t.Errorf("an unconfirmed preview sent %d request(s)", n)
	}
}

// TestMintedTokenNeverPrintedOrMirrored (kit note b): POST /apitoken answers
// with the bearer token in AccessToken. The CLI printed it and wrote it to the
// local SQLite mirror (the apitoken table and the resources row); the MCP tool
// returned it. The CLI now saves it to the 0600 credential store, as auth
// set-token does, and prints the answer without it; a later command
// authenticates with it. The MCP tool's answer carrying it is withheld.
func TestMintedTokenNeverPrintedOrMirrored(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const password = "cwaMint" + "Pass-3c9a7e1b"
	const minted = "cwaMinted" + "Tok-6f2b8d0c4e1a"
	var mints, bearer atomic.Int64
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/apitoken") {
			mints.Add(1)
			io.WriteString(w, `{"AccessToken":"`+minted+`","TokenType":"Bearer","ExpirationDate":"2030-01-02T03:04:05Z","UserId":7}`)
			return
		}
		if r.Header.Get("Authorization") == "Bearer "+minted {
			bearer.Add(1)
		}
		io.WriteString(w, `[{"Id":1,"ComputerName":"ws-01"}]`)
	})
	home := t.TempDir()
	env := closedEnv(home, "CONNECTWISE_AUTOMATE_BASE_URL="+v.url, "CONNECTWISE_AUTOMATE_CA_CERT="+v.caFile)

	out, code := runCode(t, cliBin, env, "apitoken", "mint", "--username", "tech.user", "--password", password, "--json")
	if mints.Load() != 1 {
		t.Fatalf("apitoken mint sent %d mint request(s), want 1 (exit %d):\n%s", mints.Load(), code, out)
	}
	assertAbsent(t, "apitoken mint --json", out, minted, password)
	if code != 0 {
		t.Errorf("apitoken mint exit %d, want 0:\n%s", code, out)
	}
	store := filepath.Join(home, "data", "credentials.toml")
	saved, err := os.ReadFile(store)
	if err != nil || !bytes.Contains(saved, []byte(minted)) {
		t.Errorf("the minted token is not in the credential store %s (err %v)", store, err)
	}
	assertNoFileCarries(t, home, []string{store}, minted, password)

	// The saved token authenticates the next command.
	run(t, cliBin, env, "computers", "list", "--json")
	if bearer.Load() == 0 {
		t.Error("computers list after apitoken mint did not send the minted token")
	}

	mint, _ := json.Marshal(map[string]any{"name": "apitoken_mint", "arguments": map[string]any{
		"username": "tech.user", "password": password, "confirm": true}})
	frames := mcpCall(t, mcpBin, closedEnv(t.TempDir(), "CONNECTWISE_AUTOMATE_BASE_URL="+v.url, "CONNECTWISE_AUTOMATE_CA_CERT="+v.caFile), string(mint))
	if mints.Load() != 1 {
		t.Errorf("the MCP apitoken_mint sent a mint request; a token minted there is withheld and lost:\n%s", frames)
	}
	if !strings.Contains(frames, "apitoken mint") {
		t.Errorf("the MCP apitoken_mint refusal does not name the CLI command:\n%s", frames)
	}
	assertAbsent(t, "MCP apitoken_mint", frames, minted, password)
}
