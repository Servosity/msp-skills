// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored test (handfixes.json credkit-wiring): a credential from every
// source the program reads is held before any output. The real binaries, with
// the credential kit in front of every sink, run against a stub ImmyBot over
// TLS (the kit sends no held value over plain http) that echoes the credential
// back in a successful answer; neither the CLI's stdout and stderr, nor what it
// caches and stores, nor the MCP server's tool result carries it in any form. A
// non-credential echo and the Entra tenant id (an identifier) still show, so
// the kit is not withholding everything.

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"immybot-pp-cli/internal/cliutil"
	"immybot-pp-cli/internal/cliutil/testenv"
)

// e2eSecret builds a fixture credential at run time (no literal for the
// secret scanners to flag). Two fixtures share no 8-character piece, or the kit
// reads one as a piece of the other.
func e2eSecret(name string) string {
	sum := sha256.Sum256([]byte(name))
	return hex.EncodeToString(sum[:16])
}

// e2eForms is every way the stub writes v into an answer, and so every form
// the output is searched for.
func e2eForms(v string) []string {
	return []string{v, base64.StdEncoding.EncodeToString([]byte(v)), strings.ToUpper(hex.EncodeToString([]byte(v)))}
}

// stubImmy is a fake ImmyBot instance and Entra token endpoint. Every GET
// answers 200 with one row carrying echo in each e2eForms form; the token
// endpoint mints minted, or, when tokenEcho is set, refuses with an answer
// that carries tokenEcho base64-encoded.
type stubImmy struct {
	srv       *httptest.Server
	minted    string
	echo      string
	field     string // an extra `"name":"value"` pair on the row
	tokenEcho string
	mu        sync.Mutex
	gets      int
	tokens    int
}

func newStubImmy(t *testing.T) *stubImmy {
	s := &stubImmy{minted: e2eSecret("minted-token")}
	s.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			s.mu.Lock()
			s.tokens++
			s.mu.Unlock()
			if s.tokenEcho != "" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"` + base64.StdEncoding.EncodeToString([]byte(s.tokenEcho)) + `"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"` + s.minted + `","token_type":"Bearer","expires_in":3600}`))
			return
		}
		s.mu.Lock()
		s.gets++
		s.mu.Unlock()
		f := e2eForms(s.echo)
		extra := ""
		if s.field != "" {
			extra = s.field + ","
		}
		_, _ = w.Write([]byte(`[{` + extra + `"id":42,"name":"RECEPTION","tenantId":7,"note":"` + f[1] + `","hex":"` + f[2] + `","raw":"` + f[0] + `","marker":"clean-marker-0042"}]`))
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *stubImmy) reached() int { s.mu.Lock(); defer s.mu.Unlock(); return s.gets }

func (s *stubImmy) tokenRequests() int { s.mu.Lock(); defer s.mu.Unlock(); return s.tokens }

// signIn stores a bearer token for the stub's origin, so a run reaches the
// stub without a mint: the kit sends the client secret only to Microsoft Entra.
func (s *stubImmy) signIn(t *testing.T) {
	t.Helper()
	if err := cliutil.SaveCredentials(&cliutil.Credentials{BaseURL: s.srv.URL, AccessToken: e2eSecret("signed-in-token"), TokenExpiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
}

var e2eBins struct {
	once     sync.Once
	dir      string
	cli, mcp string
	err      error
	out      []byte
}

// buildE2EBinaries builds immybot-cli and immybot-mcp once per test binary.
func buildE2EBinaries(t *testing.T) (cli, mcp string) {
	t.Helper()
	e2eBins.once.Do(func() {
		e2eBins.dir, e2eBins.err = os.MkdirTemp("", "immybot-e2e-bin-")
		if e2eBins.err != nil {
			return
		}
		root, _ := filepath.Abs(filepath.Join("..", ".."))
		for _, b := range []struct{ name, pkg string }{{"immybot-cli", "./cmd/immybot-cli"}, {"immybot-mcp", "./cmd/immybot-mcp"}} {
			out := filepath.Join(e2eBins.dir, b.name)
			cmd := exec.Command("go", "build", "-o", out, b.pkg)
			cmd.Dir = root
			if e2eBins.out, e2eBins.err = cmd.CombinedOutput(); e2eBins.err != nil {
				return
			}
		}
		e2eBins.cli, e2eBins.mcp = filepath.Join(e2eBins.dir, "immybot-cli"), filepath.Join(e2eBins.dir, "immybot-mcp")
	})
	if e2eBins.err != nil {
		t.Fatalf("build binaries: %v\n%s", e2eBins.err, e2eBins.out)
	}
	return e2eBins.cli, e2eBins.mcp
}

func TestMain(m *testing.M) {
	code := m.Run()
	if e2eBins.dir != "" {
		_ = os.RemoveAll(e2eBins.dir)
	}
	os.Exit(code)
}

// e2eHome isolates every user directory, points the program at the stub,
// and clears every credential variable.
func e2eHome(t *testing.T, s *stubImmy) string {
	t.Helper()
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	for _, k := range []string{"IMMYBOT_CLIENT_ID", "IMMYBOT_CLIENT_SECRET", "IMMYBOT_TENANT_ID", "IMMYBOT_SUBDOMAIN", "IMMYBOT_MCP_HTTP_TOKEN",
		"IMMYBOT_NO_LOCAL_DATA", "IMMYBOT_OAUTH_SCOPE", "PRINTING_PRESS_VERIFY", cliutil.MCPChildEnvVar, "IMMYBOT_NO_LEARN"} {
		t.Setenv(k, "")
	}
	t.Setenv("IMMYBOT_BASE_URL", s.srv.URL)
	t.Setenv("IMMYBOT_TOKEN_URL", s.srv.URL+"/token")
	ca := filepath.Join(t.TempDir(), "stub-ca.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.srv.Certificate().Raw})
	if err := os.WriteFile(ca, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IMMYBOT_CA_CERT", ca)
	return home
}

func e2eWriteConfig(t *testing.T, body string) {
	t.Helper()
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func e2eRun(t *testing.T, bin, stdin string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run()
	return out.String()
}

// e2eMCPRead calls immybot_read(access.list) on the MCP server binary over
// stdio and returns everything it wrote (stdout and stderr).
func e2eMCPRead(t *testing.T, mcp, cli string) string {
	t.Helper()
	return e2eMCPCall(t, mcp, cli, map[string]any{"name": "immybot_read", "arguments": map[string]any{"endpoint_id": "access.list", "params": map[string]any{}}})
}

// e2eMCPCall sends one tools/call with params to the MCP server binary over
// stdio and returns everything it wrote (stdout and stderr).
func e2eMCPCall(t *testing.T, mcp, cli string, params map[string]any) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, mcp)
	cmd.Env = append(os.Environ(), "IMMYBOT_CLI_PATH="+cli)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	send := func(v any) { b, _ := json.Marshal(v); _, _ = stdin.Write(append(b, '\n')) }
	send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "e2e", "version": "1"}}})
	send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": params})
	var all strings.Builder
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		line := sc.Text()
		all.WriteString(line + "\n")
		var msg struct {
			ID json.RawMessage `json:"id"`
		}
		if json.Unmarshal([]byte(line), &msg) == nil && string(msg.ID) == "2" {
			break
		}
	}
	_ = stdin.Close()
	_, _ = io.Copy(io.Discard, stdout)
	_ = cmd.Wait()
	return all.String() + stderr.String()
}

// e2eFilesCarrying lists every file under root, except the config and
// credentials files that store credentials by design, carrying a form of v.
func e2eFilesCarrying(t *testing.T, root, v string) []string {
	t.Helper()
	var hit []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == "config.toml" || d.Name() == "credentials.toml" {
			return nil
		}
		b, _ := os.ReadFile(p)
		for _, f := range e2eForms(v) {
			if bytes.Contains(b, []byte(f)) {
				hit = append(hit, p)
				break
			}
		}
		return nil
	})
	return hit
}

func e2eCarries(text, v string) bool {
	for _, f := range e2eForms(v) {
		if strings.Contains(text, f) {
			return true
		}
	}
	return false
}

func TestEveryCredentialSourceIsWithheldEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("drives the built binaries over POSIX file modes; the holder's unit tests run on every OS")
	}
	cli, mcp := buildE2EBinaries(t)
	envSecret, fileSecret, short := e2eSecret("env-client-secret"), e2eSecret("file-client-secret"), "Zq7#Kv9"
	storeToken, headerBearer, cookie := e2eSecret("store-access-token"), e2eSecret("header-bearer"), e2eSecret("cookie-sid")
	userinfo, httpToken := e2eSecret("base-url-password"), e2eSecret("mcp-http-token")
	clientEnv := func(secret string) func(t *testing.T, s *stubImmy) {
		return func(t *testing.T, s *stubImmy) {
			t.Setenv("IMMYBOT_CLIENT_ID", "e2e-app-client-id")
			t.Setenv("IMMYBOT_CLIENT_SECRET", secret)
			s.signIn(t)
		}
	}
	for _, tc := range []struct {
		name  string
		setUp func(t *testing.T, s *stubImmy)
		echo  func(s *stubImmy) string
	}{
		{"environment IMMYBOT_CLIENT_SECRET", clientEnv(envSecret), func(*stubImmy) string { return envSecret }},
		{"a client secret under the shared funnel's 8-byte floor", clientEnv(short), func(*stubImmy) string { return short }},
		{"config file client_secret", func(t *testing.T, s *stubImmy) {
			// The token comes with it: the kit lets this secret reach Entra only, so the stub
			// can never mint one from it.
			e2eWriteConfig(t, "client_id = \"e2e-app-client-id\"\nclient_secret = \""+fileSecret+"\"\naccess_token = \""+e2eSecret("config-access-token")+"\"\ntoken_expiry = "+time.Now().Add(time.Hour).UTC().Format(time.RFC3339)+"\n")
		}, func(*stubImmy) string { return fileSecret }},
		{"credential store access_token", func(t *testing.T, s *stubImmy) {
			if err := cliutil.SaveCredentials(&cliutil.Credentials{BaseURL: s.srv.URL, AccessToken: storeToken, TokenExpiry: time.Now().Add(time.Hour)}); err != nil {
				t.Fatal(err)
			}
		}, func(*stubImmy) string { return storeToken }},
		{"config [headers] Authorization", func(t *testing.T, s *stubImmy) {
			clientEnv(envSecret)(t, s)
			e2eWriteConfig(t, "[headers]\nAuthorization = \"Bearer "+headerBearer+"\"\n")
		}, func(*stubImmy) string { return headerBearer }},
		{"a cookie inside config [headers] Cookie", func(t *testing.T, s *stubImmy) {
			clientEnv(envSecret)(t, s)
			e2eWriteConfig(t, "[headers]\nCookie = \"theme=dark; sid="+cookie+"\"\n")
		}, func(*stubImmy) string { return cookie }},
		{"a password in IMMYBOT_BASE_URL", func(t *testing.T, s *stubImmy) {
			clientEnv(envSecret)(t, s)
			u, _ := url.Parse(s.srv.URL)
			u.User = url.UserPassword("ops", userinfo)
			t.Setenv("IMMYBOT_BASE_URL", u.String())
		}, func(*stubImmy) string { return userinfo }},
		{"IMMYBOT_MCP_HTTP_TOKEN a companion inherits", func(t *testing.T, s *stubImmy) {
			clientEnv(envSecret)(t, s)
			t.Setenv("IMMYBOT_MCP_HTTP_TOKEN", httpToken)
		}, func(*stubImmy) string { return httpToken }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStubImmy(t)
			home := e2eHome(t, s)
			tc.setUp(t, s)
			s.echo = tc.echo(s)
			dbPath := filepath.Join(home, "e2e.db")

			out := e2eRun(t, cli, "", "access", "list", "--json")
			if s.reached() == 0 {
				t.Fatalf("the CLI never reached the stub:\n%s", out)
			}
			if e2eCarries(out, s.echo) || !strings.Contains(out, "clean-marker-0042") && !strings.Contains(out, "withheld") {
				t.Fatalf("CLI output carried the credential, or showed neither the answer nor the withheld notice:\n%s", out)
			}
			_ = e2eRun(t, cli, "", "sync", "--resources", "computers", "--db", dbPath, "--rate-limit", "0")
			if hit := e2eFilesCarrying(t, home, s.echo); len(hit) > 0 {
				t.Fatalf("persisted files carry the credential: %v", hit)
			}
			before := s.reached()
			res := e2eMCPRead(t, mcp, cli)
			if s.reached() == before {
				t.Fatalf("the MCP server never reached the stub:\n%s", res)
			}
			if e2eCarries(res, s.echo) || !strings.Contains(res, `"id":2`) {
				t.Fatalf("MCP output carried the credential, or no tool result:\n%s", res)
			}
		})
	}
}

// Credentials only the CLI reads: a flag, stdin, and a malformed config file.
func TestCLIOnlyCredentialSourcesAreWithheld(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("drives the built binaries over POSIX file modes; the holder's unit tests run on every OS")
	}
	cli, _ := buildE2EBinaries(t)
	t.Run("auth login --client-secret never reaches a config token_url that is not Entra", func(t *testing.T) {
		s := newStubImmy(t)
		e2eHome(t, s)
		// Only IMMYBOT_TOKEN_URL names a sign-in endpoint off Entra's hosts; a
		// token_url in a config file must be an Entra host.
		t.Setenv("IMMYBOT_TOKEN_URL", "")
		e2eWriteConfig(t, "token_url = \""+s.srv.URL+"/token\"\n")
		flagSecret := "Fl4g#7x"
		s.tokenEcho = flagSecret
		out := e2eRun(t, cli, "", "auth", "login", "--client-id", "e2e-app-client-id", "--client-secret", flagSecret)
		if s.tokenRequests() != 0 || e2eCarries(out, flagSecret) {
			t.Fatalf("the client secret went to a host that is not Entra (%d requests), or the output carried it:\n%s", s.tokenRequests(), out)
		}
	})
	t.Run("the client secret signs in at the IMMYBOT_TOKEN_URL this run uses", func(t *testing.T) {
		s := newStubImmy(t)
		e2eHome(t, s)
		t.Setenv("IMMYBOT_CLIENT_ID", "e2e-app-client-id")
		t.Setenv("IMMYBOT_CLIENT_SECRET", e2eSecret("env-client-secret"))
		t.Setenv("IMMYBOT_TENANT_ID", "e2e-tenant-id")
		out := e2eRun(t, cli, "", "access", "list", "--json", "--data-source", "live")
		if s.tokenRequests() == 0 || s.reached() == 0 {
			t.Fatalf("the run did not sign in at IMMYBOT_TOKEN_URL (%d token requests, %d reads):\n%s", s.tokenRequests(), s.reached(), out)
		}
		if e2eCarries(out, e2eSecret("env-client-secret")) {
			t.Fatalf("the output carried the client secret:\n%s", out)
		}
	})
	t.Run("auth set-token from stdin, then echoed by an answer", func(t *testing.T) {
		s := newStubImmy(t)
		home := e2eHome(t, s)
		token := e2eSecret("stdin-token")
		s.echo = token
		if out := e2eRun(t, cli, token+"\n", "auth", "set-token"); e2eCarries(out, token) {
			t.Fatalf("set-token printed the token:\n%s", out)
		}
		out := e2eRun(t, cli, "", "access", "list", "--json")
		if s.reached() == 0 || e2eCarries(out, token) {
			t.Fatalf("the stdin token was echoed, or the stub was never reached:\n%s", out)
		}
		if hit := e2eFilesCarrying(t, home, token); len(hit) > 0 {
			t.Fatalf("persisted files carry the token: %v", hit)
		}
	})
	t.Run("a malformed legacy config file's credential line", func(t *testing.T) {
		// The CLI skips a legacy config it cannot parse with a warning and
		// still runs, so the credential its lines carry must already be held.
		s := newStubImmy(t)
		home := e2eHome(t, s)
		t.Setenv("IMMYBOT_CLIENT_ID", "e2e-app-client-id")
		t.Setenv("IMMYBOT_CLIENT_SECRET", e2eSecret("legacy-run-secret"))
		s.signIn(t)
		secret := e2eSecret("malformed-legacy")
		legacy := filepath.Join(home, ".config", "immybot-cli", "config.toml")
		if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(legacy, []byte("client_secret = \""+secret+"\"\n[[[ not toml\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		s.echo = secret
		out := e2eRun(t, cli, "", "access", "list", "--json")
		if s.reached() == 0 || e2eCarries(out, secret) {
			t.Fatalf("a malformed legacy config's credential was echoed, or the stub was never reached:\n%s", out)
		}
	})
	t.Run("a --client-secret value echoed by a later flag that fails to parse", func(t *testing.T) {
		// Cobra prints the bad value before any command runs, so a credential
		// flag is held from the raw arguments, before they are parsed (round-1
		// next-batch review).
		s := newStubImmy(t)
		e2eHome(t, s)
		const short = "Fl4g#7q"
		for _, args := range [][]string{
			{"auth", "login", "--client-secret", short, "--timeout", short},
			{"auth", "login", "--client-secret=" + short, "--timeout", short},
		} {
			out := e2eRun(t, cli, "", args...)
			if strings.Contains(out, short) || !strings.Contains(out, "withheld") {
				t.Fatalf("%v: the flag error echoed the --client-secret value, or was not withheld:\n%s", args, out)
			}
		}
	})
	t.Run("an environment credential pasted into a flag that fails to parse", func(t *testing.T) {
		// The flag error prints before any command runs, so the environment's
		// credential must be held before the arguments are parsed.
		s := newStubImmy(t)
		e2eHome(t, s)
		const short = "Zq7#Kv8"
		t.Setenv("IMMYBOT_CLIENT_SECRET", short)
		out := e2eRun(t, cli, "", "access", "list", "--max-age", short)
		if strings.Contains(out, short) || !strings.Contains(out, "withheld") {
			t.Fatalf("the flag error echoed the environment's credential, or was not withheld:\n%s", out)
		}
	})
}

// The other direction: an answer carrying no held credential, and one naming
// the operator's own Entra directory (an identifier), reach the output whole.
func TestAnAnswerWithoutAHeldCredentialIsShownWhole(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("drives the built binaries over POSIX file modes; the holder's unit tests run on every OS")
	}
	cli, mcp := buildE2EBinaries(t)
	for _, tc := range []struct{ name, echo string }{
		{"a value that is no credential", "plain-value-0042"},
		{"the Entra tenant id", "4f1c2b9e-7a1d-4c3e-9b2a-0d6e5f4a3b21"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStubImmy(t)
			e2eHome(t, s)
			t.Setenv("IMMYBOT_CLIENT_ID", "e2e-app-client-id")
			t.Setenv("IMMYBOT_CLIENT_SECRET", e2eSecret("control-secret"))
			t.Setenv("IMMYBOT_TENANT_ID", "4f1c2b9e-7a1d-4c3e-9b2a-0d6e5f4a3b21")
			s.signIn(t)
			s.echo = tc.echo
			out := e2eRun(t, cli, "", "access", "list", "--json")
			if !strings.Contains(out, tc.echo) || !strings.Contains(out, "clean-marker-0042") || strings.Contains(out, "withheld") {
				t.Fatalf("a clean answer was not shown whole:\n%s", out)
			}
			if res := e2eMCPRead(t, mcp, cli); !strings.Contains(res, tc.echo) || !strings.Contains(res, "clean-marker-0042") {
				t.Fatalf("a clean MCP result was not shown whole:\n%s", res)
			}
		})
	}
}

// A message the MCP library builds itself (a protocol error naming the tool a
// caller asked for) never passes the tool middleware, so the server's stdout
// passes the kit too (credkit.MCPOut): a message carrying a held credential is
// withheld (round-1 next-batch review).
func TestAnMCPProtocolErrorCarriesNoHeldCredential(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("drives the built binaries over POSIX file modes; the holder's unit tests run on every OS")
	}
	cli, mcp := buildE2EBinaries(t)
	s := newStubImmy(t)
	e2eHome(t, s)
	secret := e2eSecret("protocol-echo")
	t.Setenv("IMMYBOT_CLIENT_SECRET", secret)
	out := e2eMCPCall(t, mcp, cli, map[string]any{"name": base64.StdEncoding.EncodeToString([]byte(secret)), "arguments": map[string]any{}})
	if e2eCarries(out, secret) {
		t.Fatalf("the MCP server echoed a held credential:\n%s", out)
	}
	if !strings.Contains(out, `"id":1`) {
		t.Fatalf("the server stopped answering, so the protocol broke:\n%s", out)
	}
}

// The vendor returns an Entra app registration's secret in customAppRegSecret.
// The kit holds that declared response field as it passes the transport, so no
// sink carries it: not the tier command's own output, not the cache, not the store.
func TestAVendorSecretInAnAnswerReachesNoSink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("drives the built binaries over POSIX file modes; the kit's own tests run on every OS")
	}
	cli, _ := buildE2EBinaries(t)
	s := newStubImmy(t)
	home := e2eHome(t, s)
	s.signIn(t)
	secret := e2eSecret("custom-app-reg-secret")
	s.field = `"customAppRegSecret":"` + secret + `"`
	for _, args := range [][]string{
		{"access", "list", "--json"},
		{"access", "get-get-azure-tenant-auth-details-by-azure-tenant-principal-id", "11111111-1111-4111-8111-111111111111", "--json"},
	} {
		if out := e2eRun(t, cli, "", args...); e2eCarries(out, secret) {
			t.Fatalf("%v printed the vendor's secret:\n%s", args, out)
		}
	}
	if s.reached() == 0 {
		t.Fatal("the CLI never reached the stub, so this proves nothing")
	}
	if hit := e2eFilesCarrying(t, home, secret); len(hit) > 0 {
		t.Fatalf("persisted files carry the vendor's secret: %v", hit)
	}
}

// A secret passed as a tool argument is held before the tool runs, nested or not.
func TestAToolArgumentCredentialIsHeld(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("drives the built binaries over POSIX file modes; the kit's own tests run on every OS")
	}
	cli, mcp := buildE2EBinaries(t)
	s := newStubImmy(t)
	e2eHome(t, s)
	s.signIn(t)
	secret := e2eSecret("smtp-password")
	s.echo = secret
	out := e2eMCPCall(t, mcp, cli, map[string]any{"name": "immybot_read", "arguments": map[string]any{"endpoint_id": "access.list", "params": map[string]any{"password": secret}}})
	if e2eCarries(out, secret) {
		t.Fatalf("the MCP result carried a tool argument's secret:\n%s", out)
	}
}
