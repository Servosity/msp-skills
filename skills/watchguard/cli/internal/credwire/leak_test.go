// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class's failing input, seen
// failing on the binaries built before the kit was wired. Set
// WATCHGUARD_LEAK_BIN_DIR to run them against binaries built elsewhere.

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

	"watchguard-pp-cli/internal/credkit"
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
	binOnce.Do(func() {
		if d := os.Getenv("WATCHGUARD_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "watchguard-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{exe("watchguard-cli"), "../../cmd/watchguard-cli"},
			{exe("watchguard-mcp"), "../../cmd/watchguard-mcp"},
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
	return filepath.Join(binDir, exe("watchguard-cli")), filepath.Join(binDir, exe("watchguard-mcp"))
}

// vendor is a TLS stub of WatchGuard Cloud, its token endpoint included (the
// token URL is the region host plus /oauth/token): the kit never sends a
// credential over plain http, loopback included.
type vendor struct {
	url, caFile string
	hits        atomic.Int64
	conns       atomic.Int64
	mu          sync.Mutex
	seen        []string // every request's API key header and Authorization
}

const (
	accessID     = "q7Xc" + "Rv92kLmW4t"
	accessSecret = "Zp3n" + "Hq81vTc6Ke"
	apiKey       = "Jw5r" + "Bx47mDy2Gs"
)

func startVendor(t *testing.T, h http.HandlerFunc) *vendor {
	t.Helper()
	v := &vendor{}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.hits.Add(1)
		v.mu.Lock()
		v.seen = append(v.seen, r.Header.Get("WatchGuard-API-Key")+" "+r.Header.Get("Authorization"))
		v.mu.Unlock()
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
	v.caFile = filepath.Join(t.TempDir(), "ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(v.caFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return v
}

func (v *vendor) sawAPIKey() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return slices.ContainsFunc(v.seen, func(s string) bool { return strings.Contains(s, apiKey) })
}

// tokenHandler answers the token endpoint: minted for a plain grant, scoped for
// one that names an audience. Anything else goes to api.
func tokenHandler(minted, scoped string, api http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth/token") {
			_ = r.ParseForm()
			tok := minted
			if r.PostForm.Get("audience") != "" {
				tok = scoped
			}
			fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":3600}`, tok)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/audiences") {
			io.WriteString(w, `{"audience":"aud-acc1"}`)
			return
		}
		api(w, r)
	}
}

func (v *vendor) env() []string {
	return []string{"WATCHGUARD_BASE_URL=" + v.url, "WATCHGUARD_TOKEN_URL=" + v.url + "/oauth/token", "WATCHGUARD_CA_CERT=" + v.caFile,
		"WATCHGUARD_CLIENT_ID=" + accessID, "WATCHGUARD_CLIENT_SECRET=" + accessSecret, "WATCHGUARD_API_KEY=" + apiKey,
		"WATCHGUARD_ACCOUNT_ID=ACC-1"}
}

// closedEnv is a closed environment under one sandbox home; every press
// directory resolves under it.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "WATCHGUARD_HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg-config"), "XDG_DATA_HOME=" + filepath.Join(home, "xdg-data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "xdg-state"), "XDG_CACHE_HOME=" + filepath.Join(home, "xdg-cache"),
		"TMPDIR=" + os.TempDir(), "NO_COLOR=1"}, extra...)
}

func configDir(home string) string { return filepath.Join(home, "config") }

func run(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	return runStdin(t, bin, env, "", args...)
}

// runStdin runs bin with stdin and returns stdout plus stderr. A refused start
// prints nothing, which every absence check would pass on, so it fails.
func runStdin(t *testing.T, bin string, env []string, stdin string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run() // exit codes vary by case; the output is judged
	if cmd.ProcessState.ExitCode() == credkit.ExitRefused {
		t.Errorf("%s %s: the start was refused, so this run proves nothing:\n%s", filepath.Base(bin), strings.Join(args[:min(3, len(args))], " "), out.String())
	}
	return out.String()
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

// assertNoFileCarries walks home; skip names the files the test itself wrote.
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

// TestMintedBearerEchoIsWithheld (class minted-token): the bearer the program
// mints (client credentials, and the audience-scoped one WATCHGUARD_AUDIENCE
// asks for) is held the moment it arrives, so an API error that echoes a piece
// of it is withheld. The generated masks replace the whole token; a server
// that echoes a piece of it slipped past them.
func TestMintedBearerEchoIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const minted = "wgMint" + "Tok-Ka93Lx7Qp2"
	const scoped = "wgScop" + "Tok-Vb51Nz8Hr4"
	for _, c := range []struct {
		name, piece string
		extra       []string
	}{
		{"minted", minted[:13], nil},
		{"scoped", scoped[:13], []string{"WATCHGUARD_AUDIENCE=aud-acc1"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			v := startVendor(t, tokenHandler(minted, scoped, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintf(w, `{"error":"invalid_token","error_description":"token %s... rejected"}`, c.piece)
			}))
			home := t.TempDir()
			env := closedEnv(home, append(v.env(), c.extra...)...)
			assertAbsent(t, "accounts list-managed", run(t, cliBin, env, "accounts", "list-managed"), c.piece)
			assertAbsent(t, "accounts list-managed --agent", run(t, cliBin, env, "accounts", "list-managed", "--agent"), c.piece)
			assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, `{"name":"watchguard_read","arguments":{"endpoint_id":"accounts.list-managed"}}`), c.piece)
			if v.hits.Load() < 2 {
				t.Error("the API stub was not reached past the token endpoint, so nothing was echoed")
			}
			// The credential store is where a minted token belongs (a declared exemption).
			assertNoFileCarries(t, home, []string{filepath.Join(home, "data", "credentials.toml")}, c.piece)
		})
	}
}

// TestAuthLoginDryRunNeverPrintsTheAccessID (class access-id-on-output,
// handfixes.json auth-login-dry-run-no-access-id): the Access ID is a declared
// credential. `auth login --dry-run` printed it, and once the kit held it the
// whole preview was withheld; the preview now says what would happen without
// it.
func TestAuthLoginDryRunNeverPrintsTheAccessID(t *testing.T) {
	cliBin, _ := binaries(t)
	v := startVendor(t, func(http.ResponseWriter, *http.Request) {})
	home := t.TempDir()
	for _, env := range [][]string{
		closedEnv(home, v.env()...),
		closedEnv(home, "WATCHGUARD_BASE_URL="+v.url, "WATCHGUARD_TOKEN_URL="+v.url+"/oauth/token", "WATCHGUARD_CA_CERT="+v.caFile),
	} {
		out := run(t, cliBin, env, "auth", "login", "--dry-run", "--client-id", accessID, "--client-secret", accessSecret)
		if !strings.Contains(out, "grant_type=client_credentials") {
			t.Errorf("auth login --dry-run printed no preview:\n%s", out)
		}
		assertAbsent(t, "auth login --dry-run", out, accessID, accessSecret)
	}
	if n := v.conns.Load(); n != 0 {
		t.Errorf("a dry run opened %d connection(s)", n)
	}
}

// TestStdinBodyCredentialHeld (class stdin, Declarations.StdinFlags): a
// --stdin request body is held, so an API error that echoes a piece of the
// password it carried is withheld. The generated redaction knows the password
// field's name, not a piece of its value in a response.
func TestStdinBodyCredentialHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const password = "wgUser" + "Pw-Tq72Mn4Zs8"
	piece := password[:13]
	var gotBody atomic.Bool
	v := startVendor(t, tokenHandler("wgMint"+"Tok-Stdn81Kq", "unused", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if bytes.Contains(body, []byte(password)) {
			gotBody.Store(true)
		}
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":"bad_request","error_description":"password %s... does not meet policy"}`, piece)
	}))
	home := t.TempDir()
	env := closedEnv(home, v.env()...)
	out := runStdin(t, cliBin, env, `{"login":"user@acme.example","password":"`+password+`"}`, "authpoint", "password", "R1", "--stdin", "--yes")
	assertAbsent(t, "authpoint password --stdin", out, piece)
	if !gotBody.Load() {
		t.Errorf("the request body never reached the API stub, so nothing was echoed:\n%s", out)
	}
	assertNoFileCarries(t, home, nil, piece)
}

// TestProfileCredentialValueHeldAtStartup (class profile-value): a value saved
// in profiles.json under a credential flag's name (--client-secret), and the
// password in a saved base URL's userinfo, are held at startup.
func TestProfileCredentialValueHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "wgProf" + "Sec-5d4c3b2a19"
	const userPw = "wgProf" + "UiPw-8e7f6a5b"
	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "profiles.json"),
		`{"profiles":{"ops":{"name":"ops","values":{"client-secret":"`+secret+`","base-url":"https://ops:`+userPw+`@api.usa.cloud.watchguard.com","limit":"25"}}}}`)
	env := closedEnv(home)
	for _, s := range []string{secret, userPw} {
		assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", s, "version"), s)
		assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", s), s)
	}
	assertAbsent(t, "profile show", run(t, cliBin, env, "profile", "show", "ops"), secret, userPw)
}

// TestExplicitConfigDecidesTheOrigin (class origin, explicit --config): the
// base URL the run really uses decides where a credential may go. The default
// config names one host and an explicit --config another; the API key goes to
// the --config host and a --deliver webhook to the default config's host,
// with the key in its URL, is refused before it dials.
func TestExplicitConfigDecidesTheOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	api := startVendor(t, tokenHandler("wgMint"+"Tok-Cfg47Wn2", "unused", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"data":[{"id":"ACC-2","name":"acme"}],"totalItems":1}`)
	}))
	stale := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "config.toml"), fmt.Sprintf("base_url = %q\n", stale.url))
	explicit := filepath.Join(t.TempDir(), "explicit.toml")
	writeFile(t, explicit, fmt.Sprintf("base_url = %q\ntoken_url = %q\n", api.url, api.url+"/oauth/token"))
	env := closedEnv(home, "WATCHGUARD_CA_CERT="+api.caFile, "WATCHGUARD_CLIENT_ID="+accessID,
		"WATCHGUARD_CLIENT_SECRET="+accessSecret, "WATCHGUARD_API_KEY="+apiKey, "WATCHGUARD_ACCOUNT_ID=ACC-1")
	out := run(t, cliBin, env, "--config", explicit, "accounts", "list-managed", "--json", "--deliver", "webhook:"+stale.url+"/hook/"+apiKey)
	if !api.sawAPIKey() {
		t.Errorf("the --config host never received the API key; the run's own origin was refused:\n%s", out)
	}
	if n := stale.conns.Load(); n != 0 {
		t.Errorf("webhook to the default config's host: %d connection(s) opened for a URL that carries the API key", n)
	}
}

// TestHeldCredentialRefusedOffOriginAndOverPlainHTTP (class origin): the API
// key goes only to the API it is configured for, and never over plain http,
// not even to loopback. A --deliver webhook whose URL carries it is refused
// before it dials: to another https origin, and to a plain http loopback one.
func TestHeldCredentialRefusedOffOriginAndOverPlainHTTP(t *testing.T) {
	cliBin, _ := binaries(t)
	api := startVendor(t, tokenHandler("wgMint"+"Tok-Orig9Qr3", "unused", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"data":[{"id":"ACC-2","name":"acme"}],"totalItems":1}`)
	}))
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	var plainHits atomic.Int64
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plainHits.Add(1)
		io.WriteString(w, `{}`)
	}))
	t.Cleanup(plain.Close)

	env := closedEnv(t.TempDir(), api.env()...)
	run(t, cliBin, env, "accounts", "list-managed", "--json", "--deliver", "webhook:"+other.url+"/hook/"+apiKey)
	if n := other.conns.Load(); n != 0 {
		t.Errorf("webhook to another https origin: %d connection(s) opened for a URL that carries the API key", n)
	}
	run(t, cliBin, env, "accounts", "list-managed", "--json", "--deliver", "webhook:"+plain.URL+"/hook/"+apiKey)
	if n := plainHits.Load(); n != 0 {
		t.Errorf("webhook over plain http: %d request(s) carried the API key", n)
	}
	if !api.sawAPIKey() {
		t.Error("the API stub never received the API key, so no webhook was attempted")
	}

	// The API base URL itself over plain http: no credential is sent.
	var credHits atomic.Int64
	plainAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		all := r.Header.Get("WatchGuard-API-Key") + r.Header.Get("Authorization") + string(body)
		if strings.Contains(all, apiKey) || strings.Contains(all, accessSecret) || r.Header.Get("Authorization") != "" {
			credHits.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"access_token":"x","data":[]}`)
	}))
	t.Cleanup(plainAPI.Close)
	env = closedEnv(t.TempDir(), "WATCHGUARD_BASE_URL="+plainAPI.URL, "WATCHGUARD_TOKEN_URL="+plainAPI.URL+"/oauth/token",
		"WATCHGUARD_CLIENT_ID="+accessID, "WATCHGUARD_CLIENT_SECRET="+accessSecret, "WATCHGUARD_API_KEY="+apiKey, "WATCHGUARD_ACCOUNT_ID=ACC-1")
	assertAbsent(t, "plain http API", run(t, cliBin, env, "accounts", "list-managed"), apiKey, accessSecret)
	if n := credHits.Load(); n != 0 {
		t.Errorf("plain http API: %d request(s) carried a credential", n)
	}
}

// TestSpacefastTokenHeld (class spacefast-token): SPACEFAST_TOKEN, the
// credential of the --deliver spacefast: sink, is held from startup in both
// binaries.
func TestSpacefastTokenHeld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "sft_" + "live_8c2d4e6f1a3b"
	env := closedEnv(t.TempDir(), "SPACEFAST_TOKEN="+secret)
	assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", secret, "version"), secret)
	assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", secret), secret)
	assertAbsent(t, "CLI deliver target error", run(t, cliBin, env, "version", "--deliver", "spacefast:"+secret+"/x"), secret)
}

// TestJournalNeverRecordsAHeldCredential (class journal): the learn journal
// records a mistyped flag's name and value. A credential pasted where a flag
// name goes (--<key>), or after a mistyped flag, was written to
// state/learn/journal-*.jsonl; the journal appends through the kit
// (credkit.AppendFile), which withholds an entry carrying a held value.
func TestJournalNeverRecordsAHeldCredential(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	env := closedEnv(home, "WATCHGUARD_API_KEY="+apiKey, "WATCHGUARD_ACCOUNT_ID=ACC-1")
	run(t, cliBin, env, "accounts", "list-managed", "--"+apiKey)
	run(t, cliBin, env, "accounts", "list-managed", "--nmae", apiKey)
	journals, _ := filepath.Glob(filepath.Join(home, "state", "learn", "journal-*.jsonl"))
	if len(journals) == 0 {
		t.Fatal("no journal segment was written, so nothing was checked")
	}
	assertNoFileCarries(t, home, nil, apiKey)
}

// TestFeedbackStdinIsRecorded (class missing data): `feedback --stdin` is a
// StdinFlags name, so the kit would hold the whole body and withhold the local
// record while the command still said it was recorded. Feedback text is no
// credential, so its body is taken off stdin before the kit reads it; a held
// credential pasted into it is still withheld.
func TestFeedbackStdinIsRecorded(t *testing.T) {
	cliBin, _ := binaries(t)
	const text = "the accounts list output was confusing"
	home := t.TempDir()
	env := closedEnv(home, "WATCHGUARD_API_KEY="+apiKey)
	runStdin(t, cliBin, env, text+"\n", "feedback", "--stdin")
	runStdin(t, cliBin, env, "pasted "+apiKey+" by mistake\n", "feedback", "--stdin")
	record, err := os.ReadFile(filepath.Join(home, "data", "feedback.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(record), text) {
		t.Errorf("feedback.jsonl lost the feedback:\n%s", record)
	}
	assertNoFileCarries(t, home, nil, apiKey)
}
