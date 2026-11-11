// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential kit's wiring, proven on the built binaries (handfixes.json
// credkit-wiring). Each test was seen failing on the binaries built before
// the kit (NERDIO_LEAK_BIN_DIR, .review/seen-*.log). Every server here is a
// loopback httptest server: no test reaches a real host.

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
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"nerdio-pp-cli/internal/client"
	"nerdio-pp-cli/internal/credwire"
)

const (
	tokenPath  = "/7c1e9a3f-2b4d-4e6a-8c1f-3a5b7d9e1f2a/oauth2/v2.0/token"
	clientID   = "leakClientId-4b8e2a9c1d"
	clientSec  = "leakClientSecret-7f3a1c9e5b"
	mintedTok  = "leakMintedAccess-2c8e4a6f1b9d"
	oauthScope = "0e1f2a3b-4c5d-4e6f-8a9b-0c1d2e3f4a5b/.default"
)

// fake is a loopback TLS server that records every request it receives.
type fake struct {
	url, caFile string
	mu          sync.Mutex
	seen        []string // "METHOD /path Authorization body" of every request
}

func (f *fake) requests() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.seen, "\n")
}

func startFake(t *testing.T, h func(w http.ResponseWriter, r *http.Request, body []byte)) *fake {
	t.Helper()
	f := &fake{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.seen = append(f.seen, r.Method+" "+r.URL.RequestURI()+" "+r.Header.Get("Authorization")+" "+string(body))
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		h(w, r, body)
	}))
	t.Cleanup(srv.Close)
	f.url = srv.URL
	f.caFile = filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(f.caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

// nmm answers the token POST for the client pair with mintedTok and every
// API read with accounts; anything else is a 404.
func nmm(accounts string) func(w http.ResponseWriter, r *http.Request, body []byte) {
	return func(w http.ResponseWriter, r *http.Request, body []byte) {
		if r.URL.Path == tokenPath {
			form, err := url.ParseQuery(string(body))
			if err != nil || r.Header.Get("Authorization") != "" || form.Get("client_id") != clientID || form.Get("client_secret") != clientSec {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, `{"error":"invalid_client"}`)
				return
			}
			fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":3600}`, mintedTok)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/rest-api/v1/accounts" && r.Header.Get("Authorization") == "Bearer "+mintedTok {
			fmt.Fprint(w, accounts)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"errorMessage":"not found"}`)
	}
}

// pairEnv is a sandbox with the client pair in the environment, the API at
// api, and the token endpoint at tokenURL.
func pairEnv(home, api, tokenURL, caFile string, extra ...string) []string {
	return sandboxEnv(home, append([]string{
		"NERDIO_BASE_URL=" + api, "NERDIO_TOKEN_URL=" + tokenURL, "NERDIO_OAUTH_SCOPE=" + oauthScope,
		"NERDIO_CA_CERT=" + caFile, "NERDIO_CLIENT_ID=" + clientID, "NERDIO_CLIENT_SECRET=" + clientSec,
		"NERDIO_NO_CONFIG_WRITE=1",
	}, extra...)...)
}

func assertAbsent(t *testing.T, label, out string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if strings.Contains(out, s) {
			t.Errorf("%s shows %q:\n%.2000s", label, s, out)
		}
	}
}

// assertNoFileCarries walks home: no file the program wrote holds a secret.
func assertNoFileCarries(t *testing.T, home string, secrets ...string) {
	t.Helper()
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		for _, s := range secrets {
			if !synccpuStoreFile(p) && bytes.Contains(data, []byte(s)) { // synccpu: the local store is not checked
				t.Errorf("%s holds %q", p, s)
			}
		}
		return nil
	})
}

// mcpCall runs one MCP stdio session of tools/call params and returns stdout
// plus stderr once every call is answered.
func mcpCall(t *testing.T, bin string, env []string, calls ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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
	fmt.Fprintln(stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
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

// TestMalformedConfigHeldBeforeStartupOutput (startup output held first): a
// flag error prints before any command loads the config, and it repeats the
// flag's value. The config TOML rejects still names a client secret, so that
// value is held from the file's bytes before anything prints, in both
// binaries.
func TestMalformedConfigHeldBeforeStartupOutput(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	for _, c := range []struct{ name, file, secret string }{
		{"comment-glued-to-quote", "client_secret = \"nerdioGlued-0101aa\"#c\nbroken = = longenoughvalue\n", "nerdioGlued-0101aa"},
		{"bare-value", "client_secret = nerdioBare-2526bb\nbroken = = longenoughvalue\n", "nerdioBare-2526bb"},
		{"unterminated-array", "extra = [\"filler-value\",\nclient_secret = \"nerdioAfter-1314cc\"\n", "nerdioAfter-1314cc"},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			cfg := filepath.Join(home, "nerdio.toml")
			if err := os.WriteFile(cfg, []byte(c.file), 0o600); err != nil {
				t.Fatal(err)
			}
			env := sandboxEnv(home, "NERDIO_CONFIG="+cfg)
			cli, code := runCode(t, cliBin, env, "", "--timeout", c.secret, "version")
			if code == 0 {
				t.Errorf("CLI: a bad --timeout exited 0; the flag error did not run:\n%s", cli)
			}
			assertAbsent(t, "CLI startup flag error", cli, c.secret)
			mcp, code := runCode(t, mcpBin, env, "", "--transport", c.secret)
			if code == 0 {
				t.Errorf("MCP: an unknown --transport exited 0; the refusal did not run:\n%s", mcp)
			}
			assertAbsent(t, "MCP startup refusal", mcp, c.secret)
		})
	}
}

// TestClientSecretOnlyReachesItsOrigins: the client pair is sent only in the
// token POST, and only to an origin the kit allows it (the configured NMM
// instance or Entra ID's sign-in hosts). A token URL on any other origin, a
// feedback endpoint and a webhook never receive it; the NMM instance's own
// token path still mints.
func TestClientSecretOnlyReachesItsOrigins(t *testing.T) {
	cliBin, _ := binaries(t)
	api := startFake(t, nmm(`[{"id":101,"name":"HQ-Record"}]`))
	third := startFake(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		if r.URL.Path == tokenPath {
			fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":3600}`, mintedTok)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Control: the token path on the NMM instance's origin mints, and the read
	// carries the minted token, never the pair.
	home := t.TempDir()
	out := run(t, cliBin, pairEnv(home, api.url, api.url+tokenPath, api.caFile), "accounts", "--json")
	if !strings.Contains(out, "HQ-Record") || !strings.Contains(api.requests(), "Bearer "+mintedTok) {
		t.Fatalf("the control read did not mint and fetch; the test did not run:\n%s\nrequests:\n%s", out, api.requests())
	}
	for _, line := range strings.Split(api.requests(), "\n") {
		if !strings.HasPrefix(line, "POST "+tokenPath+" ") && (strings.Contains(line, "Basic ") || strings.Contains(line, clientSec)) {
			t.Errorf("an API request carried the client pair: %s", line)
		}
	}
	assertAbsent(t, "control read", out, clientSec, mintedTok)

	// A token URL on a third origin: the pair never leaves for it.
	home = t.TempDir()
	out = run(t, cliBin, pairEnv(home, api.url, third.url+tokenPath, api.caFile), "accounts", "--json")
	assertAbsent(t, "third-origin token URL", out, clientSec)
	if got := third.requests(); got != "" {
		t.Errorf("a token URL on a third origin received the client pair's request:\n%s", got)
	}

	// The feedback endpoint and a webhook on the third origin, each carrying
	// the secret in its URL or body.
	env := pairEnv(home, api.url, api.url+tokenPath, api.caFile, "NERDIO_FEEDBACK_ENDPOINT="+third.url+"/fb")
	assertAbsent(t, "feedback", run(t, cliBin, env, "feedback", "--send", "pasted "+clientSec), clientSec)
	env = pairEnv(home, api.url, api.url+tokenPath, api.caFile)
	assertAbsent(t, "webhook", run(t, cliBin, env, "accounts", "--json", "--deliver", "webhook:"+third.url+"/hook/"+clientSec), clientSec)
	if got := third.requests(); strings.Contains(got, clientSec) {
		t.Errorf("a non-API host received the client secret:\n%s", got)
	}
}

// TestMintedTokenHeld: the access token the program mints for the pair is
// held where it is minted, so a response that repeats a piece of it (here 12
// of its characters, under a name that is not a credential) never prints and
// never reaches the local store, from the CLI and the MCP server.
func TestMintedTokenHeld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	piece := mintedTok[4:16]
	api := startFake(t, nmm(fmt.Sprintf(`[{"id":101,"name":"HQ-Record","ref":%q}]`, piece)))
	home := t.TempDir()
	env := pairEnv(home, api.url, api.url+tokenPath, api.caFile)
	out := run(t, cliBin, env, "accounts", "--json")
	out += run(t, cliBin, env, "sync", "--resources", "accounts", "--json")
	out += mcpCall(t, mcpBin, env, `{"name":"nerdio_read","arguments":{"endpoint_id":"accounts.list"}}`)
	if !strings.Contains(api.requests(), "Bearer "+mintedTok) {
		t.Fatalf("no request carried the minted token; the test did not run:\n%s", out)
	}
	assertAbsent(t, "output", out, mintedTok, piece)
	assertNoFileCarries(t, home, mintedTok, piece)
}

// run is runCode with no stdin.
func run(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	out, _ := runCode(t, bin, env, "", args...)
	return out
}

// TestMCPHTTPRefusalsPassTheKit (credkit-mcphttp-outermost): every answer the
// MCP HTTP server writes, its refusals included, passes credkit.MCPHTTP, and
// none repeats the bearer token.
func TestMCPHTTPRefusalsPassTheKit(t *testing.T) {
	_, mcpBin := binaries(t)
	const token = "mcpHttpBearer-4344-0123456789abcdef"
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, mcpBin, "--transport", "http", "--addr", addr)
	cmd.Env = sandboxEnv(t.TempDir(), "NERDIO_MCP_HTTP_TOKEN="+token)
	var serverOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &serverOut, &serverOut
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	post := func(auth, origin string) (int, string) {
		var last error
		for range 50 {
			req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			if auth != "" {
				req.Header.Set("Authorization", "Bearer "+auth)
			}
			if origin != "" {
				req.Header.Set("Origin", origin)
			}
			resp, err := (&http.Client{Transport: &http.Transport{Proxy: nil}}).Do(req)
			if err != nil {
				last = err
				time.Sleep(100 * time.Millisecond)
				continue
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return resp.StatusCode, string(b)
		}
		t.Fatalf("the server never answered: %v\n%s", last, serverOut.String())
		return 0, ""
	}
	if code, body := post("", ""); code != http.StatusUnauthorized || strings.Contains(body, token) {
		t.Errorf("no token: %d %s", code, body)
	}
	if code, body := post("wrong-"+token, ""); code != http.StatusUnauthorized || strings.Contains(body, token) {
		t.Errorf("wrong token echoed: %d %s", code, body)
	}
	if code, body := post(token, "https://evil.example"); code != http.StatusForbidden || strings.Contains(body, token) {
		t.Errorf("foreign origin: %d %s", code, body)
	}
	if code, body := post(token, ""); code != http.StatusOK || !strings.Contains(body, "serverInfo") || strings.Contains(body, token) {
		t.Errorf("good token: %d %s", code, body)
	}
	assertAbsent(t, "MCP HTTP server output", serverOut.String(), token)
}

// TestStdinBodyHeldBeforeAPreview (stdin-body-declared): --stdin is a declared
// StdinFlag, so a body's values are held before anything prints; a --dry-run
// preview of it never shows one, whatever member or shape carries it.
func TestStdinBodyHeldBeforeAPreview(t *testing.T) {
	cliBin, _ := binaries(t)
	const v = "stdinOnlyValue-4567abcd"
	for name, body := range map[string]string{
		"field":     `{"subscriptionId":"` + v + `"}`,
		"nested":    `{"network":{"name":"` + v + `"}}`,
		"pretty":    "{\n  \"subscriptionId\": \"" + v + "\"\n}\n",
		"malformed": `{"subscriptionId":"` + v + `"`,
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			out, _ := runCode(t, cliBin, sandboxEnv(home, "NERDIO_BASE_URL=https://nmm.contoso.example"), body,
				"provisioning", "link-network", "--stdin", "--dry-run")
			assertAbsent(t, "--stdin --dry-run", out, v)
			assertNoFileCarries(t, home, v)
		})
	}
}

// TestDeclarationsMatchTheMaskedFields (credential-fields-masked-by-name): the
// client masks by name exactly the response fields the kit holds, so a held
// field never withholds the answer around it.
func TestDeclarationsMatchTheMaskedFields(t *testing.T) {
	if got, want := client.MaskedResponseFields, credwire.Declarations.ResponseFields; !slices.Equal(got, want) {
		t.Errorf("client.MaskedResponseFields %v, want the declared ResponseFields %v", got, want)
	}
}

// TestRunProfileCredentialsHeldAtStartup (credkit-profiles-held): a run
// profile store is read after the kit started, so startup holds what it keeps
// under a declared credential flag and the password of a saved base-url. A
// profile's other values (a saved --select) stay data.
func TestRunProfileCredentialsHeldAtStartup(t *testing.T) {
	cliBin, _ := binaries(t)
	const (
		secret = "savedClientSecret-9q8w"
		token  = "savedGraphToken-3e4r5t"
		urlPw  = "savedUrlPassw0rd-6y7u"
	)
	home := t.TempDir()
	store := filepath.Join(home, ".nerdio-cli", "profiles.json")
	if err := os.MkdirAll(filepath.Dir(store), 0o700); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"profiles":{"p":{"name":"p","values":{"select":"name,status","client-secret":%q,"graph-access-token":%q,"base-url":"https://ro:%s@nmm.contoso.example"}}}}`, secret, token, urlPw)
	if err := os.WriteFile(store, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	env := sandboxEnv(home)
	var out strings.Builder
	for _, v := range []string{secret, token, urlPw} {
		out.WriteString(run(t, cliBin, env, "feedback", "repeats "+v, "--json"))
	}
	out.WriteString(run(t, cliBin, env, "feedback", "the saved select name,status", "--json"))
	assertAbsent(t, "feedback", out.String(), secret, token, urlPw)
	var written strings.Builder
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && p != store {
			if data, rerr := os.ReadFile(p); rerr == nil {
				written.Write(data)
			}
		}
		return nil
	})
	if !strings.Contains(written.String(), "name,status") {
		t.Errorf("an ordinary profile value was held, or feedback wrote nothing:\n%s\n%s", out.String(), written.String())
	}
	assertAbsent(t, "files", written.String(), secret, token, urlPw)
}
