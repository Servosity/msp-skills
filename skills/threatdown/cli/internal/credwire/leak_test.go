// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class's failing input, seen
// failing on the binaries built before the kit was wired. Set
// THREATDOWN_LEAK_BIN_DIR to run them against binaries built elsewhere.
//
// Fixture credentials are 8 or more characters, split so no file carries a
// literal one, and chosen so no 8-character piece of them occurs in ordinary
// output.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	"threatdown-pp-cli/internal/credkit"
)

var (
	binOnce sync.Once
	binDir  string
	binErr  error
)

// TestMain removes the binaries this run built (none when
// THREATDOWN_LEAK_BIN_DIR names them).
func TestMain(m *testing.M) {
	code := m.Run()
	if os.Getenv("THREATDOWN_LEAK_BIN_DIR") == "" && binDir != "" {
		_ = os.RemoveAll(binDir)
	}
	os.Exit(code)
}

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func binaries(t *testing.T) (cliBin, mcpBin string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("drives Unix stdio pipes and home isolation; the Windows build is vetted and compiled by the gates")
	}
	binOnce.Do(func() {
		if d := os.Getenv("THREATDOWN_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "threatdown-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{exe("threatdown-cli"), "../../cmd/threatdown-cli"},
			{exe("threatdown-mcp"), "../../cmd/threatdown-mcp"},
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
	return filepath.Join(binDir, exe("threatdown-cli")), filepath.Join(binDir, exe("threatdown-mcp"))
}

// Fixture credentials (contract: 8 or more characters, no vendor format).
var (
	clientID     = "tdcid" + "Wq3Zr8Np"
	clientSecret = "tdsec" + "Kx4Vb9Qm2Lj"
	mintedToken  = "tdtok" + "Hy6Dn1Rc5Gw"
)

// vendor is a TLS stub of OneView and its token endpoint: the kit never sends
// a credential over plain http, loopback included.
type vendor struct {
	url, caFile string
	hits        atomic.Int64
	conns       atomic.Int64
	mints       atomic.Int64
	basicSeen   atomic.Int64
}

// startVendor serves the token endpoint with token (an HTTP status and body)
// and every other path with api.
func startVendor(t *testing.T, token, api http.HandlerFunc) *vendor {
	t.Helper()
	v := &vendor{}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth2/token" {
			v.mints.Add(1)
			if id, secret, ok := r.BasicAuth(); ok && id == clientID && secret == clientSecret {
				v.basicSeen.Add(1)
			}
			token(w, r)
			return
		}
		api(w, r)
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

func mintOK(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":3600}`, mintedToken)
}

func accountOK(w http.ResponseWriter, _ *http.Request) {
	io.WriteString(w, `{"account_id":"acct-1001","name":"Example MSP"}`)
}

// env points both binaries at v with the fixture client pair.
func (v *vendor) env() []string {
	return []string{
		"THREATDOWN_BASE_URL=" + v.url,
		"THREATDOWN_TOKEN_URL=" + v.url + "/oauth2/token",
		"THREATDOWN_CA_CERT=" + v.caFile,
		"THREATDOWN_CLIENT_ID=" + clientID,
		"THREATDOWN_CLIENT_SECRET=" + clientSecret,
	}
}

// closedEnv is a closed environment under one sandbox home; every press
// directory resolves under it.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "THREATDOWN_HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg-config"), "XDG_DATA_HOME=" + filepath.Join(home, "xdg-data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "xdg-state"), "XDG_CACHE_HOME=" + filepath.Join(home, "xdg-cache"),
		"TMPDIR=" + os.TempDir(), "NO_COLOR=1"}, extra...)
}

func configDir(home string) string { return filepath.Join(home, "config") }

func run(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	out, code := runCode(t, bin, env, "", args...)
	// A refused start prints nothing, which every absence check would pass on.
	if code == credkit.ExitRefused {
		t.Errorf("%s %s: the start was refused, so this run proves nothing:\n%s", filepath.Base(bin), strings.Join(args[:min(3, len(args))], " "), out)
	}
	return out
}

func runStdin(t *testing.T, bin string, env []string, stdin string, args ...string) string {
	t.Helper()
	out, code := runCode(t, bin, env, stdin, args...)
	if code == credkit.ExitRefused {
		t.Errorf("%s %s: the start was refused:\n%s", filepath.Base(bin), strings.Join(args, " "), out)
	}
	return out
}

func runCode(t *testing.T, bin string, env []string, stdin string, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
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

func storePath(home string) string { return filepath.Join(home, "data", "credentials.toml") }

// assertNoFileCarries walks home; skip names the files the test itself wrote.
// The 0600 credential store is a declared source, written outside the kit, so
// it is never judged.
func assertNoFileCarries(t *testing.T, home string, skip []string, secrets ...string) {
	t.Helper()
	skip = append(skip, storePath(home))
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

// TestTokenEndpointEchoOfClientSecretIsWithheld (class token-endpoint-echo,
// .review/kit-findings.md): a token endpoint that rejects the client and echoes
// the client secret in its error body. auth login printed the body with
// cliutil.SanitizeErrorBody, which leaves the value intact; the automatic mint
// masked the whole value but not a piece of it. The secret is held from
// startup, so every output that carries it, or any 8-character piece of it, is
// withheld.
func TestTokenEndpointEchoOfClientSecretIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	piece := clientSecret[:10]
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":"invalid_client","client_secret":%q,"hint":"secret %s... is not valid"}`, clientSecret, piece)
	}, accountOK)
	home := t.TempDir()
	env := closedEnv(home, v.env()...)
	assertAbsent(t, "auth login", run(t, cliBin, env, "auth", "login"), clientSecret, piece)
	assertAbsent(t, "auth login --client-secret", run(t, cliBin, closedEnv(home, "THREATDOWN_BASE_URL="+v.url,
		"THREATDOWN_TOKEN_URL="+v.url+"/oauth2/token", "THREATDOWN_CA_CERT="+v.caFile),
		"auth", "login", "--client-id", clientID, "--client-secret", clientSecret), clientSecret, piece)
	assertAbsent(t, "account-info (automatic mint)", run(t, cliBin, env, "account-info"), clientSecret, piece)
	assertAbsent(t, "MCP read (automatic mint)", mcpCall(t, mcpBin, env, `{"name":"threatdown_read","arguments":{"endpoint_id":"account_info.get"}}`), clientSecret, piece)
	if v.mints.Load() == 0 {
		t.Error("no request reached the token endpoint, so nothing was echoed")
	}
	assertNoFileCarries(t, home, nil, clientSecret, piece)
}

// TestAPIErrorEchoOfMintedTokenIsWithheld (class token-echo): an API error body
// that echoes part of the bearer the token endpoint minted. The generated masks
// replace the whole token and its encodings; a server that echoes a piece of it
// slipped past them. The kit holds access_token from the token response
// (ResponseFields) before the program reads it.
func TestAPIErrorEchoOfMintedTokenIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	piece := mintedToken[:12]
	v := startVendor(t, mintOK, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":"unauthorized","message":"token %s... is not valid for this account"}`, piece)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env()...)
	assertAbsent(t, "account-info", run(t, cliBin, env, "account-info"), piece)
	assertAbsent(t, "account-info --agent", run(t, cliBin, env, "account-info", "--agent"), piece)
	assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, `{"name":"threatdown_read","arguments":{"endpoint_id":"account_info.get"}}`), piece)
	if v.mints.Load() == 0 || v.hits.Load() <= v.mints.Load() {
		t.Error("the stub never minted a token and answered an API call, so nothing was echoed")
	}
	assertNoFileCarries(t, home, nil, mintedToken, piece)
}

// TestEncodedEchoOfMintedTokenIsWithheld (class encoded-echo, the 0.1.3
// promise): an API error that repeats the bearer HTML-entity-encoded,
// JSON-escaped, double URL-encoded, base64 or hex is withheld like a plain
// echo, in the CLI, --agent and MCP. The generated mask caught each form of the
// whole token; the same forms of a 12-character piece of it slipped past.
func TestEncodedEchoOfMintedTokenIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	for _, echoed := range []struct{ name, value string }{{"whole", mintedToken}, {"piece", mintedToken[:12]}} {
		for name, form := range encodings(echoed.value) {
			t.Run(echoed.name+"-"+name, func(t *testing.T) {
				v := startVendor(t, mintOK, func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
					// The form is written raw: a JSON string carries \u escapes as is.
					io.WriteString(w, `{"error":"unauthorized","message":"token `+form+` is not valid"}`)
				})
				env := closedEnv(t.TempDir(), v.env()...)
				assertAbsent(t, "account-info", run(t, cliBin, env, "account-info"), form, echoed.value)
				assertAbsent(t, "account-info --agent", run(t, cliBin, env, "account-info", "--agent"), form, echoed.value)
				assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, `{"name":"threatdown_read","arguments":{"endpoint_id":"account_info.get"}}`), form, echoed.value)
				if v.mints.Load() == 0 || v.hits.Load() <= v.mints.Load() {
					t.Error("the stub never minted a token and answered an API call, so nothing was echoed")
				}
			})
		}
	}
}

// encodings is s in each text encoding the 0.1.3 notes name.
func encodings(s string) map[string]string {
	var entity, jsonEsc, pct strings.Builder
	for _, b := range []byte(s) {
		fmt.Fprintf(&entity, "&#%d;", b)
		fmt.Fprintf(&jsonEsc, `\u%04x`, b)
		fmt.Fprintf(&pct, "%%%02X", b)
	}
	return map[string]string{
		"html-entity":  entity.String(),
		"json-escaped": jsonEsc.String(),
		"double-url":   url.QueryEscape(pct.String()),
		"base64":       base64.StdEncoding.EncodeToString([]byte(s)),
		"hex":          hex.EncodeToString([]byte(s)),
	}
}

// TestStartupCredentialsHeldBeforeAnyOutput (class startup-sources): every
// credential known at startup is held before the first byte of output, so a
// flag-parse error that echoes one is withheld in both binaries: each secret
// environment variable, the credential store, config.toml (one that does not
// parse included), a run profile saved under a credential flag's name, and a
// credential inside the configured base URL.
func TestStartupCredentialsHeldBeforeAnyOutput(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	mcpToken := "tdmcp" + strings.Repeat("Zx7Qp", 7)
	spacefast := "sfk" + "Vt5Mw8Ry2Ub"
	storeSecret := "tdstore" + "Fq2Wn7Kd"
	cfgSecret := "tdcfg" + "Lm4Ts8Hb3"
	malformed := "tdmal" + "Gx9Ue1Pz"
	profileSecret := "tdprof" + "Jr5Ya2Ce7"
	urlPassword := "tdurl" + "Oi3Bq6Vn"
	cases := []struct {
		name    string
		env     []string
		setup   func(t *testing.T, home string)
		secrets []string
	}{
		{"env-client-pair", []string{"THREATDOWN_CLIENT_ID=" + clientID, "THREATDOWN_CLIENT_SECRET=" + clientSecret}, nil, []string{clientID, clientSecret}},
		{"env-mcp-http-token", []string{"THREATDOWN_MCP_HTTP_TOKEN=" + mcpToken}, nil, []string{mcpToken}},
		{"env-spacefast-token", []string{"SPACEFAST_TOKEN=" + spacefast}, nil, []string{spacefast}},
		{"base-url-password", []string{"THREATDOWN_BASE_URL=https://ops:" + urlPassword + "@api.threatdown.com/oneview"}, nil, []string{urlPassword}},
		{"credential-store", nil, func(t *testing.T, home string) {
			writeFile(t, storePath(home), "client_id = \""+clientID+"\"\nclient_secret = \""+storeSecret+"\"\n")
		}, []string{storeSecret}},
		{"config", nil, func(t *testing.T, home string) {
			writeFile(t, filepath.Join(configDir(home), "config.toml"), "base_url = \"https://api.threatdown.com/oneview\"\nclient_secret = \""+cfgSecret+"\"\n")
		}, []string{cfgSecret}},
		{"malformed-config", nil, func(t *testing.T, home string) {
			writeFile(t, filepath.Join(configDir(home), "config.toml"), "client_secret = \""+malformed+"\" broken\n")
		}, []string{malformed}},
		{"profile-value", nil, func(t *testing.T, home string) {
			writeFile(t, filepath.Join(configDir(home), "profiles.json"),
				`{"profiles":{"ops":{"name":"ops","values":{"client-secret":"`+profileSecret+`","site":"main"}}}}`)
		}, []string{profileSecret}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			if c.setup != nil {
				c.setup(t, home)
			}
			env := closedEnv(home, c.env...)
			for _, s := range c.secrets {
				assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", s, "version"), s)
				assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", s), s)
			}
		})
	}
}

// TestHeldCredentialRefusedOffOriginAndOverPlainHTTP (class origin): a
// credential goes only to the API it is configured for (and the client pair to
// the token URL), and never over plain http, not even to loopback. A --deliver
// webhook whose URL carries the client secret is refused before it dials: to
// another https origin and to a plain http loopback one. A token URL on an
// origin no credential is bound to never receives the client pair.
func TestHeldCredentialRefusedOffOriginAndOverPlainHTTP(t *testing.T) {
	cliBin, _ := binaries(t)
	api := startVendor(t, mintOK, accountOK)
	other := startVendor(t, mintOK, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	var plainHits atomic.Int64
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plainHits.Add(1)
		io.WriteString(w, `{}`)
	}))
	t.Cleanup(plain.Close)

	home := t.TempDir()
	env := closedEnv(home, api.env()...)
	run(t, cliBin, env, "account-info", "--json", "--deliver", "webhook:"+other.url+"/hook/"+clientSecret)
	if n := other.conns.Load(); n != 0 {
		t.Errorf("webhook to another https origin: %d connection(s) opened for a URL that carries the client secret", n)
	}
	run(t, cliBin, env, "account-info", "--json", "--deliver", "webhook:"+plain.URL+"/hook/"+clientSecret)
	if n := plainHits.Load(); n != 0 {
		t.Errorf("webhook over plain http: %d request(s) carried the client secret", n)
	}
	if api.hits.Load() == 0 {
		t.Error("the API stub was never read, so no webhook was attempted")
	}

	// A token URL on another origin than the API's: the pair is never sent.
	env = closedEnv(t.TempDir(), "THREATDOWN_BASE_URL="+api.url, "THREATDOWN_TOKEN_URL="+other.url+"/oauth2/token",
		"THREATDOWN_CA_CERT="+api.caFile, "THREATDOWN_CLIENT_ID="+clientID, "THREATDOWN_CLIENT_SECRET="+clientSecret)
	assertAbsent(t, "off-origin token URL", run(t, cliBin, env, "account-info"), clientSecret)
	if n := other.basicSeen.Load(); n != 0 {
		t.Errorf("off-origin token URL: %d request(s) carried the client pair", n)
	}

	// The API and token URL over plain http: the pair is never sent.
	var pairHits atomic.Int64
	plainAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, secret, ok := r.BasicAuth(); ok && (id == clientID || secret == clientSecret) {
			pairHits.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth2/token" {
			mintOK(w, r)
			return
		}
		accountOK(w, r)
	}))
	t.Cleanup(plainAPI.Close)
	env = closedEnv(t.TempDir(), "THREATDOWN_BASE_URL="+plainAPI.URL, "THREATDOWN_TOKEN_URL="+plainAPI.URL+"/oauth2/token",
		"THREATDOWN_CLIENT_ID="+clientID, "THREATDOWN_CLIENT_SECRET="+clientSecret)
	assertAbsent(t, "plain http API", run(t, cliBin, env, "account-info"), clientSecret)
	if n := pairHits.Load(); n != 0 {
		t.Errorf("plain http API: %d request(s) carried the client pair", n)
	}
}

// TestStdinBodyHeldBeforePreview (class stdin-body): a --stdin request body is
// held before anything prints, so a --dry-run preview of a body that carries a
// credential is withheld.
func TestStdinBodyHeldBeforePreview(t *testing.T) {
	cliBin, _ := binaries(t)
	const field = "tdbody" + "Sw8Nk3Ea"
	home := t.TempDir()
	env := closedEnv(home, "THREATDOWN_BASE_URL=https://api.threatdown.com/oneview")
	body := "{\n  \"policy_name\": \"Servers\",\n  \"as_uninstall_password\": \"" + field + "\"\n}\n"
	out := runStdin(t, cliBin, env, body, "policies", "create", "--stdin", "--dry-run")
	assertAbsent(t, "policies create --stdin --dry-run", out, field)
	assertNoFileCarries(t, home, nil, field)
}

// TestJournalNeverRecordsAHeldCredential (class journal): the learn journal
// records a mistyped flag's name. A credential pasted where a flag name goes
// (--<secret>) must not reach state/learn/journal-*.jsonl; the journal appends
// through the kit (credkit.AppendFile), which withholds an entry carrying a held
// value.
func TestJournalNeverRecordsAHeldCredential(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	env := closedEnv(home, "THREATDOWN_CLIENT_ID="+clientID, "THREATDOWN_CLIENT_SECRET="+clientSecret)
	run(t, cliBin, env, "account-info", "--"+clientSecret)
	run(t, cliBin, env, "account-info", "--acount", clientSecret)
	journals, _ := filepath.Glob(filepath.Join(home, "state", "learn", "journal-*.jsonl"))
	if len(journals) == 0 {
		t.Fatal("no journal segment was written, so nothing was checked")
	}
	assertNoFileCarries(t, home, nil, clientSecret)
}

// TestFeedbackStdinIsRecorded (class missing data): `feedback --stdin` reads
// feedback text. --stdin is a StdinFlags name, so the kit would hold the whole
// body and withhold the local record while the command still said it was
// recorded. Feedback text is no credential (`feedback <text>` is never held),
// so its body is taken off stdin before the kit reads it; a held credential
// pasted into it is still withheld.
func TestFeedbackStdinIsRecorded(t *testing.T) {
	cliBin, _ := binaries(t)
	const text = "the fleet stale output was confusing"
	home := t.TempDir()
	env := closedEnv(home, "THREATDOWN_CLIENT_SECRET="+clientSecret)
	runStdin(t, cliBin, env, text+"\n", "feedback", "--stdin")
	runStdin(t, cliBin, env, "pasted "+clientSecret+" by mistake\n", "feedback", "--stdin")
	records, _ := filepath.Glob(filepath.Join(home, "data", "feedback.jsonl"))
	if len(records) == 0 {
		t.Fatal("no feedback ledger was written")
	}
	record, err := os.ReadFile(records[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(record), text) {
		t.Errorf("feedback.jsonl lost the feedback:\n%s", record)
	}
	assertNoFileCarries(t, home, nil, clientSecret)
}

// TestOrdinaryOutputStillPrints (class missing data): with the kit wired, a
// minted token and the client pair held, ordinary results still print and the
// response cache still writes them.
func TestOrdinaryOutputStillPrints(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	v := startVendor(t, mintOK, accountOK)
	home := t.TempDir()
	env := closedEnv(home, v.env()...)
	for _, out := range []string{
		run(t, cliBin, env, "account-info", "--json"),
		mcpCall(t, mcpBin, env, `{"name":"threatdown_read","arguments":{"endpoint_id":"account_info.get"}}`),
	} {
		if !strings.Contains(out, "acct-1001") {
			t.Errorf("the account id is missing from ordinary output:\n%s", out)
		}
		assertAbsent(t, "ordinary output", out, "credkit: withheld")
	}
	assertNoFileCarries(t, home, nil, clientSecret, mintedToken)
}

// TestMCPRequestCredentialArgumentsAreHeld (class mcp-request-credential, Codex
// r1): an MCP call whose request carries a credential field (the policy
// tamper-protection secret, an uninstall password, a webhook signing token)
// must not see it come back in the preview or in any file. The argument names
// are declared in MCPArguments, so the kit holds each value before any handler
// runs, nested under params included.
func TestMCPRequestCredentialArgumentsAreHeld(t *testing.T) {
	_, mcpBin := binaries(t)
	home := t.TempDir()
	env := closedEnv(home, "THREATDOWN_BASE_URL=https://api.threatdown.com/oneview")
	cases := map[string]string{
		"secret":                "tdsecret" + "Tq7Q9v2Lm",
		"as_uninstall_password": "tdunins" + "Pw4Kx8Rz",
		"secret_token":          "tdsign" + "Tk3Hv6Yw",
	}
	for name, value := range cases {
		call := fmt.Sprintf(`{"name":"threatdown_execute","arguments":{"endpoint_id":"policies.secret-set","params":{"id":"pol-1",%q:%q}}}`, name, value)
		assertAbsent(t, "threatdown_execute preview with "+name, mcpCall(t, mcpBin, env, call), value)
		assertNoFileCarries(t, home, nil, value)
	}
}
