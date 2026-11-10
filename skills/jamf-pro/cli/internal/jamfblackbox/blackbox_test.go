// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package jamfblackbox runs the built CLI and MCP binaries against a Jamf Pro
// server that echoes every credential it was sent back in every encoding it
// can, whole and split across lines and adjacent values, and checks what a
// reviewer cannot read off the code (jamf-pro handfixes credkit-wiring,
// store-never-keeps-secrets, no-response-cache, response-credentials-held,
// mcp-mirror-write-confirm, and the identity transport's host pin):
//
// A response that repeats a credential stops at the kit whole, so what still
// prints is driven by the clean sites listing.
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the client secrets (single instance and two
//     registered instances), a secret given as a flag, the access token the
//     token endpoint mints, and passwords sent in request bodies or carried
//     in responses;
//   - no file the program writes, except the 0600 credential store, holds
//     one either;
//   - a redirect to another host is refused before anything is sent there;
//   - an MCP write previews and sends nothing until confirm is boolean true.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder.
package jamfblackbox

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
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	bbSecret       = "jpSecret-ppppqqqqrrrrssssttttuuuuvv" // JAMF_PRO_CLIENT_SECRET
	bbSecretAcme   = "jpAcme-ggggzzzzggggzzzzgg"           // instance acme (its JAMF_PRO_FLEET_SECRETS entry)
	bbSecretGlobex = "jpGlobex-kkkkmmmmkkkkmmmm"           // instance globex (its JAMF_PRO_FLEET_SECRETS entry)
	bbFlagSecret   = "flagSecret-nnnnwwwwnnnnwwww"         // auth login --client-secret
	bbMinted       = "mintedToken-71c3e9b05d2a4f68"        // /api/v1/oauth/token access_token
	bbTenantPass   = "LdapBindPw-41c7e2f9a6"               // a credential-named field a response carries
	bbRecordPass   = "EaRecordPw-6a1c3e5f7b"               // the value of a name/value record labelled "Password"
	bbPatchPass    = "PrestageAdmin-5c1e9a7d3b"            // computer-prestages update --account-settings-admin-password
	bbStdinPass    = "StdinPass-4f2a9c1e"                  // a password in a --stdin request body
	bbMCPPass      = "McpParamPw-9e3a7c1d5b"               // a password in jamf-pro_execute params
	bbPIN          = "864209"                              // a device lock PIN: redacted by name, never held
	bbShortResp    = "Sh0rtPw"                             // a 7-character credential a response carries
)

func secrets() []string {
	return []string{bbSecret, bbSecretAcme, bbSecretGlobex, bbFlagSecret, bbMinted, bbTenantPass, bbRecordPass,
		bbPatchPass, bbStdinPass, bbMCPPass, bbShortResp}
}

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "jamf-pro-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "jamf-pro-cli"+exe)
	bbMCP = filepath.Join(dir, "jamf-pro-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/jamf-pro-cli", bbMCP: "../../cmd/jamf-pro-mcp"} {
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
// server can echo only what it was sent.
func sentSecrets(r *http.Request) []string {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
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

type jamfServer struct {
	host       string
	caFile     string
	offOrigin  atomic.Int32 // requests the redirect target received
	writes     atomic.Int32 // non-GET API requests (token endpoint excluded)
	hits       atomic.Int32 // every request this server received
	redirectTo string
	honest     atomic.Bool       // echo nothing: answer routes, else the clean default
	rejectMint atomic.Bool       // the token endpoint answers 401, echoing what it was sent
	routes     map[string]string // path prefix -> JSON body, when honest
	pages      map[string]string // "path?page=N" -> JSON body, when honest, before any other answer
	mu         sync.Mutex
	heard      strings.Builder
}

// record keeps what a request carried, for saw.
func (js *jamfServer) record(r *http.Request) {
	js.mu.Lock()
	defer js.mu.Unlock()
	js.heard.WriteString(r.URL.String() + "\n")
	b, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(b))
	js.heard.Write(b)
	for k, v := range r.Header {
		js.heard.WriteString(k + ": " + strings.Join(v, ",") + "\n")
	}
}

// saw reports a value in any request's URL, headers or body.
func (js *jamfServer) saw(v string) bool {
	js.mu.Lock()
	defer js.mu.Unlock()
	return strings.Contains(js.heard.String(), v)
}

// cleanSites is a listing that repeats no credential: the one route the kit
// never withholds whole. It holds a named PIN and two look-alikes that print.
const cleanSites = `{"totalCount":3,"results":[{"id":"9","name":"Clean-Site","lockPin":"864209"},{"id":"10","name":"864209"},{"id":"11","name":"client-single-0001"}]}`

// echoServer is a Jamf Pro server at https://<host> with its token endpoint
// at /api/v1/oauth/token.
func echoServer(t *testing.T) *jamfServer {
	t.Helper()
	js := &jamfServer{}
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		js.offOrigin.Add(1)
		fmt.Fprint(w, `{"totalCount":0,"results":[]}`)
	}))
	t.Cleanup(other.Close)
	js.redirectTo = other.URL
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		js.hits.Add(1)
		sent := sentSecrets(r)
		js.record(r)
		echo := echoObject(sent)
		p := r.URL.Path
		if r.Method != http.MethodGet && p != "/api/v1/oauth/token" {
			js.writes.Add(1)
		}
		if p == "/api/v1/oauth/token" && js.rejectMint.Load() {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":"invalid_client","echo":%s}`, echoObject(sent))
			return
		}
		if js.honest.Load() && p != "/api/v1/oauth/token" {
			if body, ok := js.pages[p+"?page="+r.URL.Query().Get("page")]; ok {
				fmt.Fprint(w, body)
				return
			}
			if r.URL.Query().Get("page") != "" && r.URL.Query().Get("page") != "0" {
				fmt.Fprint(w, `{"totalCount":0,"results":[]}`)
				return
			}
			for prefix, body := range js.routes {
				if strings.HasPrefix(p, prefix) {
					fmt.Fprint(w, body)
					return
				}
			}
			if strings.HasPrefix(p, "/api/v1/sites") {
				fmt.Fprint(w, cleanSites)
				return
			}
			fmt.Fprint(w, `{"version":"11.12.0-t1700000000"}`)
			return
		}
		switch {
		case p == "/api/v1/oauth/token":
			fmt.Fprintf(w, `{"access_token":%q,"expires_in":1200,"token_type":"Bearer","echo":%s}`, bbMinted, echoObject(append(sent, bbMinted)))
		case strings.HasPrefix(p, "/api/v1/sites"):
			if r.URL.Query().Get("page") != "" && r.URL.Query().Get("page") != "0" {
				fmt.Fprint(w, `{"totalCount":3,"results":[]}`)
				return
			}
			fmt.Fprint(w, cleanSites)
		case strings.HasPrefix(p, "/api/v1/departments"):
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"httpStatus":401,"errors":[{"code":"INVALID_TOKEN","description":"invalid token %s","echo":%s}]}`, bbMinted, echo)
		case strings.HasPrefix(p, "/api/v1/categories"):
			http.Redirect(w, r, js.redirectTo+p, http.StatusFound)
		case p == "/api/v1/jamf-pro-version":
			fmt.Fprintf(w, `{"version":"11.12.0-t1700000000","echo":%s}`, echo)
		case strings.HasPrefix(p, "/api/v1/buildings/"):
			fmt.Fprintf(w, `{"id":"1","name":"HQ","streetAddress1":%q,"bindPassword":%q,"echo":%s}`, bbShortResp, bbShortResp, echo)
		case r.Method != http.MethodGet:
			fmt.Fprintf(w, `{"id":"1","href":"/x","echo":%s}`, echo)
		case strings.HasPrefix(p, "/api/v1/buildings") && r.URL.Query().Get("page") != "" && r.URL.Query().Get("page") != "0":
			fmt.Fprint(w, `{"totalCount":4,"results":[]}`) // Jamf Pro API pages are 0-based
		case strings.HasPrefix(p, "/api/v1/buildings"):
			extra := ""
			if len(sent) > 0 {
				// A record whose identifier repeats the credential it was sent.
				extra = fmt.Sprintf(`,{"id":%q,"name":"Echo-ID"}`, sent[0])
			}
			// The split pieces sit in their own row: a value split across
			// adjacent values withholds the whole line or record it is in.
			fmt.Fprintf(w, `{"totalCount":%d,"results":[{"id":"1","name":"HQ-Building","city":"Rec","bindPassword":%q,"lockPin":%q,"extensionAttributes":[{"name":"Password","value":%q}],"echo":%s},{"id":"2","name":"%s"},{"id":"3","name":"Split-Row","split":%s},{"id":"4","name":"client-single-0001"}%s]}`,
				map[bool]int{true: 5, false: 4}[extra != ""], bbTenantPass, bbPIN, bbRecordPass, echo, bbPIN, splitObject(sent), extra)
		case r.URL.Query().Get("page") != "" && r.URL.Query().Get("page") != "0":
			fmt.Fprint(w, `{"totalCount":1,"results":[]}`)
		default:
			fmt.Fprintf(w, `{"totalCount":1,"results":[{"id":"7","name":"rec-row","echo":%s}]}`, echo)
		}
	}))
	t.Cleanup(srv.Close)
	js.host = strings.TrimPrefix(srv.URL, "https://")
	js.caFile = filepath.Join(t.TempDir(), "ca.pem")
	var pemBuf bytes.Buffer
	_ = pem.Encode(&pemBuf, &pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	_ = pem.Encode(&pemBuf, &pem.Block{Type: "CERTIFICATE", Bytes: other.Certificate().Raw})
	if err := os.WriteFile(js.caFile, pemBuf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return js
}

type bbEnv struct {
	home string
	env  []string
}

func newEnv(t *testing.T, js *jamfServer, withEnvSecret bool) *bbEnv {
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
		"JAMF_PRO_SERVER=" + js.host, "JAMF_PRO_CLIENT_ID=client-single-0001", "JAMF_PRO_CA_CERT=" + js.caFile,
		"JAMF_PRO_CLI_PATH=" + bbCLI,
		"JAMF_PRO_FLEET_SECRETS=acme=" + bbSecretAcme + ",globex=" + bbSecretGlobex,
	}
	if withEnvSecret {
		env = append(env, "JAMF_PRO_CLIENT_SECRET="+bbSecret)
	}
	e := &bbEnv{home: home, env: env}
	e.run(t, "instance", "add", "acme", "--server", js.host, "--client-id", "client-acme-0001")
	e.run(t, "instance", "add", "globex", "--server", js.host, "--client-id", "client-globex-0001")
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

func (e *bbEnv) with(extra ...string) *bbEnv {
	return &bbEnv{home: e.home, env: append(append([]string(nil), e.env...), extra...)}
}

func battery(e *bbEnv, exportFile, deliverFile string) [][]string {
	return [][]string{
		{"sites", "list", "--json", "--data-source", "live", "--deliver", "file:" + deliverFile + ".clean"},
		{"export", "sites", "-o", exportFile + ".clean"},
		{"sync", "--resources", "sites"},
		{"jamf-pro-version", "--json"},
		{"buildings", "list", "--json"},
		{"buildings", "list"},
		{"buildings", "list", "--csv"},
		{"buildings", "get", "1", "--json", "--data-source", "live"},
		{"scripts", "list", "--json"},
		{"departments", "list", "--json"},
		{"teach", "--query", "list buildings", "--resource-type", "buildings", "--resource", "1", "--no-validate",
			"--notes", "rotated " + bbSecret, "--playbook-json", `{"steps":[{"cmd":"buildings list --key ` + bbSecretAcme + `"}]}`, "--playbook-notes", "key " + bbSecretGlobex},
		{"buildings", "list", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		{"buildings", "list", "--json", "--deliver", "file:" + deliverFile},
		{"sync", "--resources", "buildings,scripts"},
		{"--instance", "acme", "sync", "--resources", "buildings"},
		{"export", "buildings", "-o", exportFile},
		{"computer-prestages", "update", "1", "--account-settings-admin-password", bbPatchPass, "--dry-run", "--json"},
		{"computer-prestages", "update", "1", "--account-settings-admin-password", bbPatchPass, "--json", "--yes"},
		{"computer-prestages", "update", "1", "--account-settings-admin-password=" + bbPatchPass, "--bogus-flag"},
		{"--instance", "acme", "buildings", "list", "--json"},
		{"--instance", "globex", "jamf-pro-version", "--json"},
		{"instance", "list", "--json"},
		{"instance", "test", "--json"},
		{"fleet", "health", "--json"},
		{"fleet", "exec", "/api/v1/buildings", "--json"},
		{"fleet", "exec", "/api/v1/buildings", "--csv"},
		// A response credential repeated under an innocent name (streetAddress1
		// = bindPassword): the child's rule redacts the named field, so only
		// the hold taken when the response arrived withholds the repeat.
		{"fleet", "exec", "/api/v1/buildings/1", "--json"},
		{"fleet", "find", "HQ", "--json"},
		{"fleet", "policy-hygiene", "--json"},
		{"feedback", "noted the key " + bbSecret + " in a ticket"},
		{"search", "rec", "--json"},
		{"doctor"},
		{"auth", "status"},
		{"auth", "login", "--client-id", "client-flag-0001", "--client-secret=" + bbFlagSecret, "--bogus-flag"},
		{"auth", "login", "--client-id", "client-flag-0001", "--client-secret", bbFlagSecret, "--json"},
		{"buildings", "list", "--json"},
	}
}

func mcpBattery() []map[string]any {
	return []map[string]any{
		{"name": "jamf-pro_read", "arguments": map[string]any{"endpoint_id": "sites.list"}},
		{"name": "jamf-pro_read", "arguments": map[string]any{"endpoint_id": "buildings.list"}},
		{"name": "jamf-pro_read", "arguments": map[string]any{"endpoint_id": "buildings.get", "params": map[string]any{"id": "1"}}},
		{"name": "jamf-pro_read", "arguments": map[string]any{"endpoint_id": "departments.list"}},
		{"name": "jamf-pro_execute", "arguments": map[string]any{"endpoint_id": "buildings.update", "params": map[string]any{"id": "1", "name": "HQ", "bindPassword": bbMCPPass}}},
		{"name": "jamf-pro_execute", "arguments": map[string]any{"endpoint_id": "buildings.update", "params": map[string]any{"id": "1", "name": "HQ", "bindPassword": bbMCPPass}, "confirm": true}},
		{"name": "fleet_exec", "arguments": map[string]any{"api-path": "/api/v1/buildings"}},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
		{"name": "instance_list", "arguments": map[string]any{}},
	}
}

// stdinRuns feed a request body on stdin: a preview and a send.
func stdinRuns(e *bbEnv, t *testing.T) string {
	body := fmt.Sprintf(`{"accountSettings":{"adminPassword":%q},"displayName":"Rec"}`, bbStdinPass)
	return e.runIn(t, body, "computer-prestages", "update", "1", "--stdin", "--dry-run", "--json") +
		e.runIn(t, body, "computer-prestages", "update", "1", "--stdin", "--json", "--yes")
}

// Class credential-persisted-in-files.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	t.Parallel()
	js := echoServer(t)
	e := newEnv(t, js, true)
	exportFile := filepath.Join(e.home, "export.jsonl")
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, args := range battery(e, exportFile, deliverFile) {
		e.run(t, args...)
	}
	stdinRuns(e, t)
	e.mcpCalls(t, mcpBattery())
	var scanned, credStores int
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path == js.caFile {
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
			// environment, a registered instance's included, is never written.
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
		if !synccpuStoreFile(path) { // synccpu: the local store is not checked
			assertNoEcho(t, "file "+rel, string(data))
		}
		return nil
	})
	if scanned < 5 {
		t.Fatalf("only %d files written; the battery did not run", scanned)
	}
	// The minted token was cached somewhere, so the credential-store check
	// above is not vacuous.
	if credStores < 1 {
		t.Errorf("want a minted token in at least one credential store, found %d", credStores)
	}
	// The battery really wrote data, so the scan above is not vacuous.
	for _, path := range []string{exportFile + ".clean", deliverFile + ".clean"} {
		if b, err := os.ReadFile(path); err != nil || !bytes.Contains(b, []byte("Clean-Site")) {
			t.Errorf("%s: want the exported record (err %v)", filepath.Base(path), err)
		}
	}
	var sawStore bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(d.Name(), ".db") {
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				return nil
			}
			if bytes.Contains(b, []byte("Clean-Site")) {
				sawStore = true
			}
			// A PIN the program never holds reaches no store row either
			// under its credential name: the store redacts it by name.
			if bytes.Contains(b, []byte(`"lockPin":"`+bbPIN)) {
				t.Errorf("%s holds the named PIN %s", d.Name(), bbPIN)
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
	js := echoServer(t)
	e := newEnv(t, js, true)
	exportFile := filepath.Join(e.home, "export.jsonl")
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, args := range battery(e, exportFile, deliverFile) {
		assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
	}
	assertNoEcho(t, "output of computer-prestages update --stdin", stdinRuns(e, t))
	// A clean listing prints as valid JSON, nothing withheld: the record, the
	// record named like a PIN (PINs are redacted where named, never held) and the
	// client ID (a public identifier).
	cmd := exec.Command(bbCLI, "sites", "list", "--json", "--data-source", "live")
	cmd.Env, cmd.Dir = e.env, e.home
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("sites list --json: %v", err)
	}
	var doc any
	if json.Unmarshal(stdout, &doc) != nil || bytes.Contains(stdout, []byte("withheld")) {
		t.Fatalf("want valid JSON and nothing withheld, got:\n%.1500s", stdout)
	}
	for _, want := range []string{"Clean-Site", `"` + bbPIN + `"`, "client-single-0001", "[redacted]"} {
		if !bytes.Contains(stdout, []byte(want)) {
			t.Fatalf("sites list: want %s, got:\n%.1500s", want, stdout)
		}
	}
	// The listing that echoes a credential is the kit's to stop.
	if out := e.run(t, "buildings", "list", "--json", "--data-source", "live"); !strings.Contains(out, bbWithheld) {
		t.Fatalf("want the kit's notice on the echoing listing, got:\n%.1500s", out)
	}
	// The fleet reaches every registered instance (each child mints with its
	// own secret), so the fleet runs above were not error rows only.
	out := e.run(t, "fleet", "exec", "/api/v1/sites", "--json")
	if !strings.Contains(out, "acme") || !strings.Contains(out, "globex") || strings.Count(out, "Clean-Site") < 2 {
		t.Fatalf("fleet exec: want both instances read, got:\n%.2000s", out)
	}
	if out := e.run(t, "instance", "list", "--json"); !strings.Contains(out, "client-acme-0001") || !strings.Contains(out, "client-globex-0001") || strings.Contains(out, "withheld") {
		t.Fatalf("instance list: want both client IDs, nothing withheld, got:\n%.1500s", out)
	}
	mcp := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(mcp, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
		t.Fatalf("MCP session did not answer every call:\n%.2000s", mcp)
	}
	assertNoEcho(t, "MCP session", mcp)
	if !strings.Contains(mcp, "Clean-Site") {
		t.Fatalf("MCP results lost the record itself:\n%.2000s", mcp)
	}
}

// Class api-origin-pinned: a redirect to another host is refused before the
// request reaches it, and the credential never travels there.
func TestRedirectToAnotherHostIsRefused(t *testing.T) {
	t.Parallel()
	js := echoServer(t)
	e := newEnv(t, js, true)
	out := e.run(t, "categories", "list", "--json", "--data-source", "live")
	if js.offOrigin.Load() != 0 {
		t.Fatalf("the redirect target received %d request(s)", js.offOrigin.Load())
	}
	if !strings.Contains(out, "redirects are refused") {
		t.Fatalf("want the redirect refusal, got:\n%.1500s", out)
	}
}

// Class write-without-confirm: an MCP write previews and sends nothing
// without confirm: true, on the execute tool and on a mirrored write.
func TestMCPWriteNeedsConfirm(t *testing.T) {
	t.Parallel()
	js := echoServer(t)
	e := newEnv(t, js, true)
	params := map[string]any{"id": "1", "name": "HQ"}
	importFile := filepath.Join(e.home, "import.jsonl")
	if err := os.WriteFile(importFile, []byte(`{"name":"Imported"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := e.mcpCalls(t, []map[string]any{
		{"name": "jamf-pro_execute", "arguments": map[string]any{"endpoint_id": "buildings.update", "params": params}},
		{"name": "jamf-pro_execute", "arguments": map[string]any{"endpoint_id": "buildings.update", "params": params, "confirm": "true"}},
		{"name": "import", "arguments": map[string]any{"resource": "buildings", "input": importFile}},
		{"name": "import", "arguments": map[string]any{"resource": "buildings", "input": importFile, "confirm": "true"}},
	})
	if js.writes.Load() != 0 || !strings.Contains(out, "confirmation_required") || !strings.Contains(out, "Preview only") {
		t.Fatalf("previews sent %d write(s); output:\n%.2500s", js.writes.Load(), out)
	}
	e.mcpCalls(t, []map[string]any{
		{"name": "jamf-pro_execute", "arguments": map[string]any{"endpoint_id": "buildings.update", "params": params, "confirm": true}},
	})
	if js.writes.Load() != 1 {
		t.Fatalf("confirm: true sent %d write(s), want 1", js.writes.Load())
	}
}

// MCP recipe argv: a tool argument never becomes a flag or a command of the
// mirrored CLI child.
func TestMCPArgumentNeverBecomesAFlag(t *testing.T) {
	t.Parallel()
	js := echoServer(t)
	e := newEnv(t, js, true)
	target := filepath.Join(e.home, "mcp-deliver.json")
	out := e.mcpCalls(t, []map[string]any{
		{"name": "fleet_exec", "arguments": map[string]any{"api-path": "--deliver=file:" + target}},
		{"name": "any_read_across_all_clients", "arguments": map[string]any{"path": "--deliver=file:" + target}},
		{"name": "broken_policies_across_clients", "arguments": map[string]any{"slug": "exec"}},
	})
	if _, err := os.Stat(target); err == nil {
		t.Fatalf("a tool argument wrote a file through --deliver:\n%.1500s", out)
	}
	if !strings.Contains(out, `unknown MCP parameter \"slug\"`) && !strings.Contains(out, `unknown MCP parameter "slug"`) {
		t.Fatalf("want the slug refused, got:\n%.1500s", out)
	}
}
