// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class's failing input, seen
// failing on the binaries built before the kit was wired. Set
// XERO_LEAK_BIN_DIR to run them against binaries built elsewhere.

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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"xero-pp-cli/internal/credkit"
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
	if testing.Short() {
		t.Skip("builds and runs both binaries")
	}
	binOnce.Do(func() {
		if d := os.Getenv("XERO_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "xero-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{exe("xero-cli"), "../../cmd/xero-cli"},
			{exe("xero-mcp"), "../../cmd/xero-mcp"},
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
	return filepath.Join(binDir, exe("xero-cli")), filepath.Join(binDir, exe("xero-mcp"))
}

func TestMain(m *testing.M) {
	code := m.Run()
	if binDir != "" && os.Getenv("XERO_LEAK_BIN_DIR") == "" {
		_ = os.RemoveAll(binDir)
	}
	os.Exit(code)
}

// newHome is one run's throwaway home. Not t.TempDir: its path carries the
// test's name, and a path is output too.
func newHome(t *testing.T) string {
	t.Helper()
	home, err := os.MkdirTemp("", "xero-leak-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	return home
}

// vendor is a TLS stub of the Xero API (and, on the same origin, of its token
// endpoint): the kit never sends a credential over plain http, loopback
// included.
type vendor struct {
	url, caFile string
	hits        atomic.Int64
	conns       atomic.Int64
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
	ca, err := os.MkdirTemp("", "xero-leak-ca-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(ca) })
	v.caFile = filepath.Join(ca, "ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(v.caFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return v
}

const tenantID = "3f1c2a9e-0d4b-4c7e-9a51-2b8f3d6e7c10"

func (v *vendor) env(token string) []string {
	return []string{"XERO_BASE_URL=" + v.url, "XERO_CA_CERT=" + v.caFile, "XERO_ACCESS_TOKEN=" + token, "XERO_TENANT_ID=" + tenantID}
}

// closedEnv is a closed environment under one sandbox home; every press
// directory resolves under it. HOME and USERPROFILE both: os.UserHomeDir reads
// USERPROFILE on Windows.
func closedEnv(home string, extra ...string) []string {
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "XERO_HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg-config"), "XDG_DATA_HOME=" + filepath.Join(home, "xdg-data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "xdg-state"), "XDG_CACHE_HOME=" + filepath.Join(home, "xdg-cache"),
		"TMPDIR=" + os.TempDir(), "NO_COLOR=1"}
	if runtime.GOOS == "windows" {
		for _, k := range []string{"SystemRoot", "TEMP", "TMP", "APPDATA", "LOCALAPPDATA", "ComSpec", "PATHEXT"} {
			if v := os.Getenv(k); v != "" {
				env = append(env, k+"="+v)
			}
		}
	}
	return append(env, extra...)
}

func configDir(home string) string { return filepath.Join(home, "config") }
func dataDir(home string) string   { return filepath.Join(home, "data") }

func run(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	return runStdin(t, bin, env, "", args...)
}

// runStdin runs bin with stdin and returns stdout plus stderr.
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
	// A refused start prints nothing, which every absence check would pass on.
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
		if err != nil || d.IsDir() {
			return nil
		}
		for _, s := range skip {
			if s == p {
				return nil
			}
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

// refreshConfig is a config whose access token has expired, so the next API
// call first refreshes it at tokenURL with the client secret and refresh token.
func refreshConfig(tokenURL, clientSecret, refreshToken string) string {
	return fmt.Sprintf("access_token = %q\nrefresh_token = %q\ntoken_expiry = 2020-01-01T00:00:00Z\nclient_id = \"xeroClientId0123456789\"\nclient_secret = %q\ntoken_url = %q\n",
		"xeroExpired"+"Acc7Tk2m", refreshToken, clientSecret, tokenURL)
}

// TestAPIErrorEchoOfBearerIsWithheld (class token-echo): an API error body that
// echoes part of XERO_ACCESS_TOKEN. The generated masks replace the whole token
// and its encodings; a server that echoes a piece of it slipped past them. Any
// 8-character piece of a held value withholds the output.
func TestAPIErrorEchoOfBearerIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const token = "xerotok-" + "Zq81LmP0xV7c"
	piece := token[:13]
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"Type":"AuthenticationUnsuccessful","Title":"Unauthorized","Detail":"token %s... rejected","Status":401}`, piece)
	})
	home := newHome(t)
	env := closedEnv(home, v.env(token)...)
	assertAbsent(t, "accounts get", run(t, cliBin, env, "accounts", "get"), piece)
	assertAbsent(t, "accounts get --agent", run(t, cliBin, env, "accounts", "get", "--agent"), piece)
	assertAbsent(t, "MCP accounts_get", mcpCall(t, mcpBin, env, `{"name":"accounts_get","arguments":{}}`), piece)
	if v.hits.Load() == 0 {
		t.Error("no request reached the API stub, so nothing was echoed")
	}
	assertNoFileCarries(t, home, nil, piece)
}

// TestTokenEndpointEchoOfClientSecretIsWithheld (class token-endpoint echo):
// the refresh grant's error body echoes part of the client secret and of the
// refresh token. Both are held from startup (the config names them), so the
// error that repeats them is withheld on the CLI and over MCP.
func TestTokenEndpointEchoOfClientSecretIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "xeroSec-" + "Tq7Lm2Zp9Wc4"
	const refresh = "xeroRfr-" + "Bv3Kd8Qx1Ns6"
	secretPiece, refreshPiece := secret[:13], refresh[:13]
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/connect/token") {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"invalid_client","error_description":"client secret %s... and refresh token %s... rejected"}`, secretPiece, refreshPiece)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"Status":401}`)
	})
	home := newHome(t)
	cfg := filepath.Join(configDir(home), "config.toml")
	writeFile(t, cfg, refreshConfig(v.url+"/connect/token", secret, refresh))
	env := closedEnv(home, "XERO_BASE_URL="+v.url, "XERO_CA_CERT="+v.caFile, "XERO_TENANT_ID="+tenantID)
	assertAbsent(t, "accounts get", run(t, cliBin, env, "accounts", "get"), secretPiece, refreshPiece)
	assertAbsent(t, "accounts get --agent", run(t, cliBin, env, "accounts", "get", "--agent"), secretPiece, refreshPiece)
	assertAbsent(t, "MCP accounts_get", mcpCall(t, mcpBin, env, `{"name":"accounts_get","arguments":{}}`), secretPiece, refreshPiece)
	if v.hits.Load() == 0 {
		t.Error("no request reached the token stub, so nothing was echoed")
	}
	assertNoFileCarries(t, home, []string{cfg}, secretPiece, refreshPiece)
}

// TestHeldCredentialRefusedOffOriginAndOverPlainHTTP (class origin): the bearer
// goes only to the API it is configured for, and never over plain http, not
// even to loopback; the client secret and refresh token go only to the token
// endpoint's origin. A --deliver webhook whose URL carries the bearer is
// refused before it dials, and a token URL on another origin never receives
// the refresh grant.
func TestHeldCredentialRefusedOffOriginAndOverPlainHTTP(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "xerotok-" + "Orig1nQ7r3s5"
	api := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"Accounts":[{"AccountID":"a1","Code":"200","Name":"Sales"}]}`)
	})
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	var plainHits atomic.Int64
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plainHits.Add(1)
		io.WriteString(w, `{}`)
	}))
	t.Cleanup(plain.Close)

	env := closedEnv(newHome(t), api.env(token)...)
	run(t, cliBin, env, "accounts", "get", "--json", "--deliver", "webhook:"+other.url+"/hook/"+token)
	if n := other.conns.Load(); n != 0 {
		t.Errorf("webhook to another https origin: %d connection(s) opened for a URL that carries the bearer", n)
	}
	run(t, cliBin, env, "accounts", "get", "--json", "--deliver", "webhook:"+plain.URL+"/hook/"+token)
	if n := plainHits.Load(); n != 0 {
		t.Errorf("webhook over plain http: %d request(s) carried the bearer", n)
	}
	if api.hits.Load() == 0 {
		t.Error("the API stub was never read, so no webhook was attempted")
	}

	// The API base URL itself over plain http: the bearer is never sent.
	var bearerHits atomic.Int64
	plainAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization"), token) {
			bearerHits.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"Accounts":[]}`)
	}))
	t.Cleanup(plainAPI.Close)
	env = closedEnv(newHome(t), "XERO_BASE_URL="+plainAPI.URL, "XERO_ACCESS_TOKEN="+token, "XERO_TENANT_ID="+tenantID)
	assertAbsent(t, "plain http API", run(t, cliBin, env, "accounts", "get"), token)
	if n := bearerHits.Load(); n != 0 {
		t.Errorf("plain http API: %d request(s) carried the bearer", n)
	}

	// A token URL on another origin than the token endpoint and the API: the
	// refresh grant (client secret and refresh token) is never sent there.
	const secret = "xeroSec-" + "Orig2nW4k8y1"
	const refresh = "xeroRfr-" + "Orig3nH6j2u9"
	var grantHits atomic.Int64
	elsewhere := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if bytes.Contains(body, []byte(secret)) || bytes.Contains(body, []byte(refresh)) ||
			strings.Contains(r.Header.Get("Authorization"), "Basic") {
			grantHits.Add(1)
		}
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"invalid_grant"}`)
	})
	home := newHome(t)
	writeFile(t, filepath.Join(configDir(home), "config.toml"), refreshConfig(elsewhere.url+"/connect/token", secret, refresh))
	// The stubs share no CA, so trust both: the kit, not TLS, must stop the grant.
	bundle := filepath.Join(home, "bundle.pem")
	a, _ := os.ReadFile(api.caFile)
	b, _ := os.ReadFile(elsewhere.caFile)
	writeFile(t, bundle, string(a)+string(b))
	env = closedEnv(home, "XERO_BASE_URL="+api.url, "XERO_CA_CERT="+bundle, "XERO_TENANT_ID="+tenantID)
	run(t, cliBin, env, "accounts", "get")
	if n := grantHits.Load(); n != 0 {
		t.Errorf("token URL on another origin: %d request(s) carried the client secret or refresh token", n)
	}
}

// TestStartupOutputNeverShowsMalformedFileCredential (class startup ordering):
// a credential inside a malformed config or credentials file is held before
// either binary writes its first byte, so an error that echoes an argument (a
// flag error, in both binaries) never shows it, whether the config comes from
// --config, XERO_CONFIG or the default path.
func TestStartupOutputNeverShowsMalformedFileCredential(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "xeroCfgSec" + "7Kq9xZw3"
	const refresh = "xeroCredRfr" + "4Mn8Pv2q"
	const config = "client_id = \"xeroClientId0123456789\"\nclient_secret = \"" + secret + "\"\nbroken = [\n"
	const creds = "refresh_token = \"" + refresh + "\"\nbroken = {\n"

	// Every word of a file that does not parse is held, and a held word that
	// occurs in public text refuses the start, so the fixtures carry no short
	// common word ("bad" does occur).
	cases := []struct {
		name   string
		setup  func(t *testing.T, home string) (env []string, args []string)
		secret string
	}{
		{"default-config", func(t *testing.T, home string) ([]string, []string) {
			writeFile(t, filepath.Join(configDir(home), "config.toml"), config)
			return nil, nil
		}, secret},
		{"env-config", func(t *testing.T, home string) ([]string, []string) {
			p := filepath.Join(home, "elsewhere", "malformed.toml")
			writeFile(t, p, config)
			return []string{"XERO_CONFIG=" + p}, nil
		}, secret},
		{"flag-config", func(t *testing.T, home string) ([]string, []string) {
			p := filepath.Join(home, "elsewhere", "malformed.toml")
			writeFile(t, p, config)
			return nil, []string{"--config", p}
		}, secret},
		{"credentials-file", func(t *testing.T, home string) ([]string, []string) {
			writeFile(t, filepath.Join(dataDir(home), "credentials.toml"), creds)
			return nil, nil
		}, refresh},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			extraEnv, extraArgs := c.setup(t, home)
			env := closedEnv(home, extraEnv...)
			cliArgs := append(append([]string{}, extraArgs...), "--timeout", c.secret, "version")
			assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, cliArgs...), c.secret)
			if extraArgs == nil { // the MCP server has no --config
				assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", c.secret), c.secret)
			}
		})
	}
}

// TestProfileCredentialValueHeldAtStartup (class profile-value): a value saved
// in profiles.json under a credential flag's name (--client-secret) is held at
// startup, in the profile store and in the legacy one.
func TestProfileCredentialValueHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "xeroProf" + "Sec-5d4c3b2a19"
	const legacySecret = "xeroLegacy" + "Sec-8e7f6a5b"
	home := newHome(t)
	writeFile(t, filepath.Join(configDir(home), "profiles.json"),
		`{"profiles":{"watcher":{"name":"watcher","values":{"client-secret":"`+secret+`","where":"Status==\"ACTIVE\""}}}}`)
	writeFile(t, filepath.Join(home, ".xero-cli", "profiles.json"),
		`{"profiles":{"old":{"name":"old","values":{"client-secret":"`+legacySecret+`"}}}}`)
	env := closedEnv(home)
	for _, s := range []string{secret, legacySecret} {
		assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", s, "version"), s)
		assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", s), s)
	}
	assertAbsent(t, "profile show", run(t, cliBin, env, "profile", "show", "watcher"), secret)
}

// TestJournalNeverRecordsAHeldCredential (class journal): the learn journal
// records a mistyped flag's name. A credential pasted where a flag name goes
// (--<token>) was written to state/learn/journal-*.jsonl; the journal appends
// through the kit (credkit.AppendFile), which withholds an entry carrying a
// held value.
func TestJournalNeverRecordsAHeldCredential(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "xerotok" + "JournL8x2q9"
	home := newHome(t)
	env := closedEnv(home, "XERO_ACCESS_TOKEN="+token, "XERO_TENANT_ID="+tenantID)
	run(t, cliBin, env, "accounts", "get", "--"+token)
	run(t, cliBin, env, "accounts", "get", "--wher", token)
	journals, _ := filepath.Glob(filepath.Join(home, "state", "learn", "journal-*.jsonl"))
	if len(journals) == 0 {
		t.Fatal("no journal segment was written, so nothing was checked")
	}
	assertNoFileCarries(t, home, nil, token)
}

// TestStdinBodyCredentialHeldBeforeAnythingPrints (class stdin body): a
// --stdin request body is held before anything prints, so a credential field
// inside it never shows in the --dry-run preview, and never in an API error
// that echoes the body back.
func TestStdinBodyCredentialHeldBeforeAnythingPrints(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "xeroBodySec" + "9Hd2Lq7v"
	body := "{\n  \"Name\": \"Sales\",\n  \"Code\": \"201\",\n  \"client_secret\": \"" + secret + "\"\n}\n"
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		got, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"Type":"ValidationException","Detail":"rejected %s"}`, bytes.ReplaceAll(got, []byte(`"`), []byte(`'`)))
	})
	env := closedEnv(newHome(t), v.env("xerotok-"+"Stdin4Rk8m2w")...)
	assertAbsent(t, "accounts create --stdin --dry-run", runStdin(t, cliBin, env, body, "accounts", "create", "--stdin", "--dry-run"), secret)
	assertAbsent(t, "accounts create --stdin --dry-run --agent", runStdin(t, cliBin, env, body, "accounts", "create", "--stdin", "--dry-run", "--agent"), secret)
	assertAbsent(t, "accounts create --stdin", runStdin(t, cliBin, env, body, "accounts", "create", "--stdin", "--yes"), secret)
}

// TestFeedbackStdinIsRecorded (class missing data): SKILL.md documents
// `feedback --stdin`. --stdin is a StdinFlags name, so the kit would hold the
// whole body and withhold the local record, while the command still said
// "feedback recorded locally". Feedback text is no credential, so its body is
// taken off stdin before the kit reads it; a held credential pasted into it is
// still withheld.
func TestFeedbackStdinIsRecorded(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "xerotokFeedbk3Lp7q"
	const text = "the aging buckets output was confusing"
	home := newHome(t)
	env := closedEnv(home, "XERO_ACCESS_TOKEN="+token)
	runStdin(t, cliBin, env, text+"\n", "feedback", "--stdin")
	runStdin(t, cliBin, env, "pasted "+token+" by mistake\n", "feedback", "--stdin")
	record, err := os.ReadFile(filepath.Join(dataDir(home), "feedback.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(record), text) {
		t.Errorf("feedback.jsonl lost the feedback:\n%s", record)
	}
	assertNoFileCarries(t, home, nil, token)
}

// TestSpacefastTokenHeld (class spacefast-token): SPACEFAST_TOKEN, the
// credential of the --deliver spacefast: sink, is held from startup in both
// binaries.
func TestSpacefastTokenHeld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "sft_" + "live_4b7c9d1e2f3a"
	env := closedEnv(newHome(t), "SPACEFAST_TOKEN="+secret)
	assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", secret, "version"), secret)
	assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", secret), secret)
	assertAbsent(t, "CLI deliver target error", run(t, cliBin, env, "version", "--deliver", "spacefast:"+secret+"/x"), secret)
}
