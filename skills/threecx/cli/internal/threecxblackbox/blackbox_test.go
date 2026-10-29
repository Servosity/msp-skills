// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package threecxblackbox runs the built CLI and MCP binaries against a 3CX
// phone system that echoes every credential it knows back in every encoding
// it can, whole and split across lines and adjacent values, and checks what a
// reviewer cannot read off the code (threecx handfixes credkit-wiring,
// store-never-keeps-secrets, no-response-cache, response-credentials-held,
// api-origin-pinned):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the client secrets (single PBX and two registered
//     PBXes), a secret given as a flag, the access token /connect/token
//     mints, an API key a minting function returns, and passwords sent in
//     request bodies or carried in responses;
//   - no file the program writes, except the 0600 credential store, holds
//     one either;
//   - a redirect to another host is refused before anything is sent there;
//   - against a phone system that echoes nothing, the same commands still
//     print their records, with credential members replaced by name.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder.
package threecxblackbox

import (
	"bytes"
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Fixture literals are split so a scanner sees no whole synthetic secret; the values are unchanged.
const (
	bbSecret       = "cxSecret-" + "7fb3189bQ6u3sZxW4o1n7R8v52" // THREECX_CLIENT_SECRET
	bbSecretAcme   = "cxAcme-" + "A9d2c7e41f0b63X5w8Lq"         // PBX acme (THREECX_ACME_CLIENT_SECRET)
	bbSecretGlobex = "cxGlobex-" + "4d8b2e6f1a9c3e70Zq"         // PBX globex (THREECX_GLOBEX_CLIENT_SECRET)
	bbFlagSecret   = "flag" + "Secret-8e2d4f6a1c9b7e3d"         // auth login --client-secret
	bbMinted       = "mintedToken-71c3e9b05d2a4f68"             // /connect/token access_token
	bbGenKey       = "genApi" + "Key-4d8b2e6f1a9c3e71"          // ConferenceSettings/Pbx.GenerateApiKey() value
	bbTenantPass   = "SipTrunk" + "Pw-41c7e2f9a6"               // a trunk's AuthPassword the PBX returns
	bbConcealed    = "Concealed-6a1c3e5f7b"                     // a ConcealedPassword Value the PBX returns
	bbPatchPass    = "PatchPass-5c1e9a7d3b"                     // users-id update-user --auth-password
	bbStdinPass    = "StdinPass-4f2a9c1e"                       // a password in a --stdin request body
	bbMCPPass      = "McpParamPw-9e3a7c1d5b"                    // a password in threecx_execute params
	bbPIN          = "86420975"                                 // a PinNumber: redacted by name, never held (PINs equal extension numbers)
	bbShortResp    = "Sh0rtPw"                                  // a 7-character credential a response carries
	bbOAuth        = "crmAccess-3d9f7a1c5e2b"                   // CrmIntegration/Pbx.GetOAuth {Key, Value} (Pbx.CrmChoice)
	bbHelpLink     = "https://support.example/grant/9c1e5a7d3b" // SystemStatus/Pbx.GetRequestHelpLink value
	bbPKCE         = "pkceVerifier-abcdefghij0123456789"        // Pbx.OauthStateParam PKCECodeVerifier (MCP argument)
	bbAuthCode     = "authCode-5e2b9c1e7a"                      // McpServers/Pbx.Exchange code (MCP argument)
	bbVerifier     = "exchVerifier-7d3b1c9e5a"                  // McpServers/Pbx.Exchange verifier (MCP argument)
)

func secrets() []string {
	return []string{bbSecret, bbSecretAcme, bbSecretGlobex, bbFlagSecret, bbMinted, bbGenKey, bbTenantPass,
		bbConcealed, bbPatchPass, bbStdinPass, bbMCPPass, bbShortResp, bbOAuth, bbHelpLink, bbPKCE, bbAuthCode, bbVerifier}
}

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	// Short skips the builds; Windows skips the POSIX pipes and the stub's trust
	// path. The kit's Windows stdio path is covered by the kit's own CI, and the
	// Windows build of both binaries by the matrix.
	if testing.Short() || runtime.GOOS == "windows" {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "threecx-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "threecx-cli"+exe)
	bbMCP = filepath.Join(dir, "threecx-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/threecx-cli", bbMCP: "../../cmd/threecx-mcp"} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// echoForms is every encoding of s the server sends back whole.
func echoForms(s string) []string {
	var a85 bytes.Buffer
	w := ascii85.NewEncoder(&a85)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return []string{
		s,
		"x" + s + "x",
		"Bearer " + s,
		base64.StdEncoding.EncodeToString([]byte(s)),
		base64.RawURLEncoding.EncodeToString([]byte(s)),
		hex.EncodeToString([]byte(s)),
		base32.StdEncoding.EncodeToString([]byte(s)),
		a85.String(),
		url.QueryEscape(s),
	}
}

func halves(s string) (string, string) { return s[:len(s)/2], s[len(s)/2:] }

// jsonUnicode spells s as \uXXXX escapes, so the raw value appears only once
// a JSON decoder has read it.
func jsonUnicode(s string) string {
	var b strings.Builder
	for _, r := range s {
		fmt.Fprintf(&b, `\u%04x`, r)
	}
	return b.String()
}

// echoObject repeats each of sent whole, in every form.
func echoObject(sent []string) string {
	parts := []string{`"note":"echo"`}
	for i, s := range sent {
		for j, f := range echoForms(s) {
			v, _ := json.Marshal(f)
			parts = append(parts, fmt.Sprintf(`"e%d_%d":%s`, i, j, v))
		}
		parts = append(parts, fmt.Sprintf(`"u%d":"%s"`, i, jsonUnicode(s)))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// splitObject repeats each of sent split: across two adjacent values, across
// an embedded newline, and across a space.
func splitObject(sent []string) string {
	parts := []string{`"note":"split"`}
	for i, s := range sent {
		a, b := halves(s)
		parts = append(parts, fmt.Sprintf(`"s%d_a":%q,"s%d_b":%q,"s%d_nl":%q,"s%d_sp":%q`, i, a, i, b, i, a+"\n"+b, i, a+" "+b))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// sentSecrets is every known secret the request carried (its Authorization
// header, Basic credentials decoded, URL or body, form-decoded too): a real
// phone system can echo only what it was sent.
func sentSecrets(r *http.Request) []string {
	body, _ := io.ReadAll(r.Body)
	seen := r.Header.Get("Authorization") + "\n" + r.URL.String() + "\n" + string(body)
	if _, pw, ok := r.BasicAuth(); ok {
		seen += "\n" + pw
	}
	if form, err := url.QueryUnescape(string(body)); err == nil {
		seen += "\n" + form
	}
	var out []string
	for _, s := range secrets() {
		if strings.Contains(seen, s) {
			out = append(out, s)
		}
	}
	return out
}

type phoneSystem struct {
	host       string
	caFile     string
	offOrigin  atomic.Int32 // requests the redirect target received
	hits       atomic.Int32 // requests the phone system received (token endpoint included)
	redirectTo string
	honest     atomic.Bool       // true: echo nothing, the records only
	routes     map[string]string // XAPI path prefix to a fixed body, read before anything else
	rejectMint atomic.Bool       // true: the token endpoint answers 401, echoing what it was sent
}

// echoServer is a 3CX phone system at https://<host>/xapi/v1 with its token
// endpoint at /connect/token.
func echoServer(t *testing.T) *phoneSystem {
	t.Helper()
	ps := &phoneSystem{}
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ps.offOrigin.Add(1)
		fmt.Fprint(w, `{"value":[]}`)
	}))
	t.Cleanup(other.Close)
	ps.redirectTo = other.URL
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ps.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		sent := sentSecrets(r)
		echo := echoObject(sent)
		p := strings.TrimPrefix(r.URL.Path, "/xapi/v1")
		if ps.honest.Load() {
			sent, echo = nil, "{}"
		}
		for prefix, body := range ps.routes {
			if strings.HasPrefix(p, prefix) {
				fmt.Fprint(w, body)
				return
			}
		}
		switch {
		case r.URL.Path == "/connect/token" && ps.rejectMint.Load():
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":"invalid_client","echo":%s}`, echoObject(sent))
		case r.URL.Path == "/connect/token":
			if ps.honest.Load() {
				fmt.Fprintf(w, `{"access_token":%q,"expires_in":3600,"token_type":"Bearer"}`, bbMinted)
				return
			}
			fmt.Fprintf(w, `{"access_token":%q,"expires_in":3600,"token_type":"Bearer","echo":%s}`, bbMinted, echoObject(append(sent, bbMinted)))
		case strings.Contains(p, "Pbx.GetOAuth"):
			fmt.Fprintf(w, `{"@odata.context":"x","Key":"OAuth","Value":%q}`, bbOAuth)
		case strings.Contains(p, "Pbx.GetRequestHelpLink"):
			fmt.Fprintf(w, `{"@odata.context":"x","value":%q}`, bbHelpLink)
		case strings.Contains(p, "Pbx.GenerateApiKey"):
			fmt.Fprintf(w, `{"@odata.context":"x","value":%q}`, bbGenKey)
		case strings.HasPrefix(p, "/Groups"):
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":{"code":"","message":"invalid token %s","echo":%s}}`, bbMinted, echo)
		case strings.HasPrefix(p, "/Parameters"):
			http.Redirect(w, r, ps.redirectTo+"/xapi/v1/Parameters", http.StatusFound)
		case p == "/SystemStatus":
			fmt.Fprintf(w, `{"FQDN":"pbx.example.com","Version":"20.0.10.1479","LicenseActive":true,"ExpirationDate":"2027-01-01T00:00:00Z","MaxSimCalls":8,"echo":%s}`, echo)
		case strings.HasPrefix(p, "/Trunks"):
			fmt.Fprintf(w, `{"value":[{"Id":1,"Number":"10000","Gateway":{"Name":"SipCarrier","Type":"Provider"},"IsOnline":true,"AuthID":"acct","AuthPassword":%q,"echo":%s}]}`, bbTenantPass, echo)
		case strings.HasPrefix(p, "/Users("):
			if r.Method != http.MethodGet {
				fmt.Fprintf(w, `{"echo":%s}`, echo)
				return
			}
			fmt.Fprintf(w, `{"Id":2,"Number":"1001","FirstName":%q,"AuthPassword":%q}`, bbShortResp, bbShortResp)
		case strings.HasPrefix(p, "/Users"):
			// Extension 900 is also the API client's DN number (THREECX_CLIENT_ID).
			fmt.Fprintf(w, `{"value":[{"Id":2,"Number":"1001","FirstName":"Rec","VMPIN":"1001","PinNumber":%q,"AuthPassword":{"Concealed":true,"Value":%q},"split":%s,"echo":%s},{"Id":3,"Number":"900","FirstName":"Api"}]}`, bbPIN, bbConcealed, splitObject(sent), echo)
		case r.Method != http.MethodGet:
			fmt.Fprintf(w, `{"echo":%s}`, echo)
		default:
			extra := ""
			if len(sent) > 0 {
				// A record whose identifier repeats the credential it was sent.
				extra = fmt.Sprintf(`,{"Id":%q,"Name":"Echo-ID"}`, sent[0])
			}
			fmt.Fprintf(w, `{"value":[{"Id":"7","Name":"rec-row","echo":%s}%s]}`, echo, extra)
		}
	}))
	t.Cleanup(srv.Close)
	ps.host = strings.TrimPrefix(srv.URL, "https://")
	ps.caFile = filepath.Join(t.TempDir(), "ca.pem")
	var pemBuf bytes.Buffer
	_ = pem.Encode(&pemBuf, &pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	_ = pem.Encode(&pemBuf, &pem.Block{Type: "CERTIFICATE", Bytes: other.Certificate().Raw})
	if err := os.WriteFile(ps.caFile, pemBuf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return ps
}

type bbEnv struct {
	home string
	env  []string
}

func newEnv(t *testing.T, ps *phoneSystem, withEnvSecret bool) *bbEnv {
	t.Helper()
	home := t.TempDir()
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"THREECX_FQDN=" + ps.host, "THREECX_CLIENT_ID=900", "THREECX_CA_CERT=" + ps.caFile,
		"THREECX_CLI_PATH=" + bbCLI,
		"THREECX_ACME_CLIENT_SECRET=" + bbSecretAcme, "THREECX_GLOBEX_CLIENT_SECRET=" + bbSecretGlobex,
	}
	if withEnvSecret {
		env = append(env, "THREECX_CLIENT_SECRET="+bbSecret)
	}
	e := &bbEnv{home: home, env: env}
	e.run(t, "fleet", "add", "acme", "--fqdn", ps.host, "--client-id", "901", "--env-prefix", "THREECX_ACME")
	e.run(t, "fleet", "add", "globex", "--fqdn", ps.host, "--client-id", "902", "--env-prefix", "THREECX_GLOBEX")
	return e
}

func (e *bbEnv) run(t *testing.T, args ...string) string {
	t.Helper()
	return e.runIn(t, "", args...)
}

func (e *bbEnv) runIn(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bbCLI, args...)
	cmd.Env = e.env
	cmd.Dir = e.home
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("%v: timed out", args)
	}
	return out.String() + "\n" + errb.String()
}

// mcpCalls runs one MCP stdio session and returns everything the server
// wrote: every tools/call result and its stderr.
func (e *bbEnv) mcpCalls(t *testing.T, calls []map[string]any) string {
	t.Helper()
	var in bytes.Buffer
	enc := json.NewEncoder(&in)
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}})
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	for i, c := range calls {
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": i + 1, "method": "tools/call", "params": c})
	}
	cmd := exec.Command(bbMCP)
	cmd.Env = e.env
	cmd.Dir = e.home
	stdin, _ := cmd.StdinPipe()
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(stdin, &in)
	deadline := time.Now().Add(180 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Count(out.String(), `"id":`) >= len(calls)+1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = stdin.Close()
	waitDone := make(chan struct{})
	go func() { _ = cmd.Wait(); close(waitDone) }()
	select {
	case <-waitDone:
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
	}
	return out.String() + "\n" + errb.String()
}

// assertNoEcho fails when text holds a secret in any form the server sent,
// or both halves of one (a split the reader could rejoin).
func assertNoEcho(t *testing.T, where, text string) {
	t.Helper()
	for _, s := range secrets() {
		for _, f := range echoForms(s) {
			if strings.Contains(text, f) {
				t.Errorf("%s holds a credential as %q", where, f)
			}
		}
		if a, b := halves(s); strings.Contains(text, a) && strings.Contains(text, b) {
			t.Errorf("%s holds both halves of %q", where, s)
		}
	}
}

func battery(e *bbEnv, exportFile, deliverFile string) [][]string {
	return [][]string{
		{"system-status", "get", "--json"},
		{"system-status", "get"},
		{"system-status", "get", "--csv"},
		{"trunks", "list", "--json"},
		{"trunks", "list"},
		{"trunks", "list", "--csv"},
		{"users", "list", "--json"},
		{"users", "list"},
		{"users-id", "get-user", "2", "--json", "--data-source", "live"},
		{"groups", "list", "--json"},
		{"conference-settings", "generate-api-key", "--json"},
		{"conference-settings", "generate-api-key"},
		{"crm-integration", "get-oauth", "OAuth", "--json"},
		{"crm-integration", "get-oauth", "OAuth"},
		{"system-status", "get-request-help-link", "1", "--json"},
		{"teach", "--query", "list users", "--resource-type", "users", "--resource", "2", "--no-validate",
			"--notes", "rotated " + bbSecret, "--playbook-json", `{"steps":[{"cmd":"users list --key ` + bbSecretAcme + `"}]}`, "--playbook-notes", "key " + bbSecretGlobex},
		{"trunks", "list", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		{"trunks", "list", "--json", "--deliver", "file:" + deliverFile},
		{"sync", "--resources", "trunks,users,contacts"},
		{"--pbx", "acme", "sync", "--resources", "trunks"},
		{"export", "trunks", "-o", exportFile},
		{"users-id", "update-user", "2", "--auth-password", bbPatchPass, "--dry-run", "--json"},
		{"users-id", "update-user", "2", "--auth-password", bbPatchPass, "--json", "--yes"},
		{"users-id", "update-user", "2", "--auth-password=" + bbPatchPass, "--bogus-flag"},
		{"--pbx", "acme", "trunks", "list", "--json"},
		{"--pbx", "globex", "system-status", "get", "--json"},
		{"fleet", "list", "--json"},
		{"fleet", "health", "--json"},
		{"fleet", "trunks", "--json"},
		{"fleet", "versions", "--json"},
		{"fleet", "licences", "--json"},
		{"fleet", "backups", "--json"},
		{"fleet", "exec", "/Users", "--json"},
		{"fleet", "exec", "/Trunks", "--json"},
		{"fleet", "exec", "/Users", "--csv"},
		{"audit", "--json"},
		{"fleet", "changes", "--json"},
		{"feedback", "noted the key " + bbSecret + " in a ticket"},
		{"search", "rec", "--json"},
		{"doctor"},
		{"auth", "status"},
		{"auth", "login", "--client-id", "903", "--client-secret=" + bbFlagSecret, "--bogus-flag"},
		{"auth", "login", "--client-id", "903", "--client-secret", bbFlagSecret, "--json"},
		{"trunks", "list", "--json"},
	}
}

func mcpBattery() []map[string]any {
	return []map[string]any{
		{"name": "threecx_read", "arguments": map[string]any{"endpoint_id": "trunks.list"}},
		{"name": "threecx_read", "arguments": map[string]any{"endpoint_id": "users.list"}},
		{"name": "threecx_read", "arguments": map[string]any{"endpoint_id": "system-status.get"}},
		{"name": "threecx_read", "arguments": map[string]any{"endpoint_id": "groups.list"}},
		{"name": "threecx_execute", "arguments": map[string]any{"confirm": true, "endpoint_id": "conference-settings.generate-api-key"}},
		{"name": "threecx_execute", "arguments": map[string]any{"endpoint_id": "users-id.update-user", "params": map[string]any{"Id": "2", "AuthPassword": bbMCPPass}}},
		{"name": "threecx_execute", "arguments": map[string]any{"endpoint_id": "users-id.update-user", "params": map[string]any{"Id": "2", "AuthPassword": bbMCPPass}, "confirm": true}},
		{"name": "fleet_exec", "arguments": map[string]any{"xapi-path": "/Users"}},
		{"name": "fleet_trunks", "arguments": map[string]any{}},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
		{"name": "threecx_execute", "arguments": map[string]any{"confirm": true, "endpoint_id": "crm-integration.get-oauth", "params": map[string]any{"variable": "OAuth"}}},
		{"name": "threecx_execute", "arguments": map[string]any{"confirm": true, "endpoint_id": "system-status.get-request-help-link", "params": map[string]any{"grantPeriodDays": 1}}},
		{"name": "threecx_execute", "arguments": map[string]any{"endpoint_id": "crm-integration.set-oauth-state", "params": map[string]any{
			"state": map[string]any{"Variable": "crm", "RedirectUri": "https://pbx.example/", "PKCECodeVerifier": bbPKCE}}}},
		{"name": "threecx_execute", "arguments": map[string]any{"endpoint_id": "mcp-servers.exchange", "params": map[string]any{
			"code": bbAuthCode, "verifier": bbVerifier, "tokenEndpoint": "https://idp.example/token", "scope": "mcp"}}},
	}
}

// stdinRuns feed a request body on stdin: a preview and a send.
func stdinRuns(e *bbEnv, t *testing.T) string {
	body := fmt.Sprintf(`{"AuthPassword":%q,"FirstName":"Rec"}`, bbStdinPass)
	return e.runIn(t, body, "users-id", "update-user", "2", "--stdin", "--dry-run", "--json") +
		e.runIn(t, body, "users-id", "update-user", "2", "--stdin", "--json", "--yes")
}

// Class credential-persisted-in-files.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	t.Parallel()
	ps := echoServer(t)
	e := newEnv(t, ps, true)
	exportFile := filepath.Join(e.home, "export.jsonl")
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, args := range battery(e, exportFile, deliverFile) {
		e.run(t, args...)
	}
	stdinRuns(e, t)
	e.mcpCalls(t, mcpBattery())
	var scanned, credStores int
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		scanned++
		rel, _ := filepath.Rel(e.home, path)
		if strings.HasPrefix(d.Name(), "credentials") {
			// The credential store: the one file allowed to hold a credential
			// (minted tokens, an auth login secret). A secret read from the
			// environment, a registered PBX's included, is never written
			// anywhere: the README promises the fleet key never touches disk.
			if info, err := d.Info(); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
				t.Errorf("%s mode %v, want 0600", rel, info.Mode().Perm())
			}
			for _, s := range []string{bbSecret, bbSecretAcme, bbSecretGlobex} {
				if bytes.Contains(data, []byte(s)) {
					t.Errorf("%s holds the environment secret %q", rel, s)
				}
			}
			if bytes.Contains(data, []byte(bbMinted)) {
				credStores++
			}
			return nil
		}
		assertNoEcho(t, "file "+rel, string(data))
		return nil
	})
	if scanned < 5 {
		t.Fatalf("only %d files written; the battery did not run", scanned)
	}
	// The single PBX and both fleet homes each cached the token they minted,
	// so the credential-store check above is not vacuous.
	if credStores < 3 {
		t.Errorf("want a minted token in at least 3 credential stores (single PBX, acme, globex), found %d", credStores)
	}
	// The same writes against a phone system that echoes nothing do write data.
	ps.honest.Store(true)
	e.run(t, "trunks", "list", "--json", "--deliver", "file:"+deliverFile)
	e.run(t, "sync", "--resources", "trunks,users,contacts")
	e.run(t, "export", "trunks", "-o", exportFile)
	for _, path := range []string{exportFile, deliverFile} {
		if b, err := os.ReadFile(path); err != nil || !bytes.Contains(b, []byte("SipCarrier")) {
			t.Errorf("%s: want the exported record (err %v)", filepath.Base(path), err)
		}
	}
	var sawStore bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".db") {
			if b, rerr := os.ReadFile(path); rerr == nil && bytes.Contains(b, []byte("SipCarrier")) {
				sawStore = true
			}
		}
		// A credential the program never holds (a PIN) reaches no store row
		// either: the store redacts credential-named fields by name.
		if err == nil && !d.IsDir() && strings.Contains(d.Name(), ".db") {
			if b, rerr := os.ReadFile(path); rerr == nil && bytes.Contains(b, []byte(bbPIN)) {
				t.Errorf("%s holds the PinNumber %s", d.Name(), bbPIN)
			}
		}
		return nil
	})
	if !sawStore {
		t.Errorf("no local store holds the synced record")
	}
}

// Class secret-in-output.
func TestNoCredentialInAnyOutput(t *testing.T) {
	t.Parallel()
	ps := echoServer(t)
	e := newEnv(t, ps, true)
	exportFile := filepath.Join(e.home, "export.jsonl")
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, args := range battery(e, exportFile, deliverFile) {
		assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
	}
	assertNoEcho(t, "output of users-id update-user --stdin", stdinRuns(e, t))
	// A response that repeats a held value is withheld whole, with the kit's notice.
	if out := e.run(t, "system-status", "get", "--json"); !strings.Contains(out, "credkit: withheld") {
		t.Errorf("want the kit's notice for an echoing response, got:\n%.1500s", out)
	}
	// A phone system that echoes nothing: the records print, the credential
	// members are replaced by name, and JSON output stays valid JSON.
	ps.honest.Store(true)
	cmd := exec.Command(bbCLI, "trunks", "list", "--json")
	cmd.Env, cmd.Dir = e.env, e.home
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("trunks list --json: %v", err)
	}
	var doc any
	if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("SipCarrier")) || !bytes.Contains(stdout, []byte("[redacted]")) {
		t.Fatalf("want valid JSON with the record and the password redacted, got:\n%.1500s", stdout)
	}
	assertNoEcho(t, "honest trunks list", string(stdout))
	// The fleet reaches every registered PBX (each child mints with its own
	// secret), so the fleet runs above were not error rows only.
	out := e.run(t, "fleet", "versions", "--json")
	if strings.Count(out, "20.0.10.1479") < 2 {
		t.Fatalf("fleet versions: want both PBXes read, got:\n%.1500s", out)
	}
	// Identifiers and tenant data that are not credentials still print: the
	// client IDs, an extension equal to a client ID, and an extension number
	// equal to its voicemail PIN (PINs are redacted where named, never held).
	if out := e.run(t, "fleet", "list", "--json"); !strings.Contains(out, "901") || !strings.Contains(out, "902") || strings.Contains(out, "withheld") {
		t.Fatalf("fleet list: want both client IDs, nothing withheld, got:\n%.1500s", out)
	}
	if out := e.run(t, "users", "list", "--json"); !strings.Contains(out, `"1001"`) || !strings.Contains(out, `"900"`) {
		t.Fatalf("users list: want the extension numbers, the client ID's included, got:\n%.1500s", out)
	}
	if out := e.mcpCalls(t, []map[string]any{{"name": "threecx_read", "arguments": map[string]any{"endpoint_id": "trunks.list"}}, {"name": "fleet_trunks", "arguments": map[string]any{}}}); strings.Count(out, "SipCarrier") < 2 {
		t.Fatalf("MCP results lost the record itself (the read and the mirrored fleet child):\n%.2000s", out)
	}
	ps.honest.Store(false)
	mcp := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(mcp, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
		t.Fatalf("MCP session did not answer every call:\n%.2000s", mcp)
	}
	assertNoEcho(t, "MCP session", mcp)
}

// Class api-origin-pinned: a redirect to another host is refused before the
// request reaches it, and the credential never travels there.
func TestRedirectToAnotherHostIsRefused(t *testing.T) {
	t.Parallel()
	ps := echoServer(t)
	e := newEnv(t, ps, true)
	out := e.run(t, "parameters", "list", "--json", "--data-source", "live")
	if ps.offOrigin.Load() != 0 {
		t.Fatalf("the redirect target received %d request(s)", ps.offOrigin.Load())
	}
	if !strings.Contains(out, "leave the configured 3CX phone system") {
		t.Fatalf("want the origin refusal, got:\n%.1500s", out)
	}
}

// Class write-without-confirm: an MCP write previews and sends nothing
// without confirm: true.
func TestMCPWriteNeedsConfirm(t *testing.T) {
	t.Parallel()
	var patches atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/connect/token":
			fmt.Fprint(w, `{"access_token":"tok-`+"0123456789abcdef"+`","expires_in":3600}`)
		case r.Method != http.MethodGet:
			patches.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			fmt.Fprint(w, `{"value":[]}`)
		}
	}))
	t.Cleanup(srv.Close)
	ps := &phoneSystem{host: strings.TrimPrefix(srv.URL, "https://"), caFile: filepath.Join(t.TempDir(), "ca.pem")}
	var pemBuf bytes.Buffer
	_ = pem.Encode(&pemBuf, &pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	_ = os.WriteFile(ps.caFile, pemBuf.Bytes(), 0o600)
	e := newEnv(t, ps, true)
	params := map[string]any{"Id": "2", "FirstName": "Rec"}
	out := e.mcpCalls(t, []map[string]any{
		{"name": "threecx_execute", "arguments": map[string]any{"endpoint_id": "users-id.update-user", "params": params}},
		{"name": "threecx_execute", "arguments": map[string]any{"endpoint_id": "users-id.update-user", "params": params, "confirm": "true"}},
	})
	if patches.Load() != 0 || !strings.Contains(out, "confirmation_required") {
		t.Fatalf("previews sent %d write(s); output:\n%.1500s", patches.Load(), out)
	}
	e.mcpCalls(t, []map[string]any{
		{"name": "threecx_execute", "arguments": map[string]any{"endpoint_id": "users-id.update-user", "params": params, "confirm": true}},
	})
	if patches.Load() != 1 {
		t.Fatalf("confirm: true sent %d write(s), want 1", patches.Load())
	}
}

// MCP recipe argv: a tool argument never becomes a flag of the mirrored CLI
// child (this connector has no recipe intents; the mirror refuses a
// flag-like positional and joins structured values to their own flag).
func TestMCPArgumentNeverBecomesAFlag(t *testing.T) {
	t.Parallel()
	ps := echoServer(t)
	e := newEnv(t, ps, true)
	target := filepath.Join(e.home, "mcp-deliver.json")
	out := e.mcpCalls(t, []map[string]any{
		{"name": "fleet_exec", "arguments": map[string]any{"xapi-path": "--deliver=file:" + target}},
		{"name": "fleet_exec", "arguments": map[string]any{"xapi-path": "/Users", "param": []any{"--pbx=globex"}, "only": "acme"}},
	})
	if !strings.Contains(out, "flag-like positional argument") {
		t.Fatalf("want the flag-like positional refused, got:\n%.1500s", out)
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("a tool argument wrote a file through --deliver")
	}
	if strings.Contains(out, `"pbx": "globex"`) || strings.Contains(out, `"pbx":"globex"`) {
		t.Fatalf("a structured value retargeted the run:\n%.1500s", out)
	}
}
