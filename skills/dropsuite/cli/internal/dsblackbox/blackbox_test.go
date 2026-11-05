// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package dsblackbox runs the built CLI and MCP binaries against a Dropsuite
// API fake that echoes every credential it knows back in every encoding it
// can, whole and split across lines and adjacent values, and checks what a
// reviewer cannot read off the code (dropsuite handfixes credkit-wiring,
// no-response-cache, api-origin-pinned, mcp-write-confirm):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the access and reseller tokens from the environment
//     or the credential store, each organization's authentication_token that
//     GET /users returns (and the requests routed with it), and the
//     credentials a response carries;
//   - no file the program writes, except the 0600 credential store, holds
//     one either;
//   - no credential leaves for a host other than the configured one by
//     redirect;
//   - an MCP call that mints a portal sign-in link previews until confirm is
//     boolean true.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder.
package dsblackbox

import (
	"bytes"
	"compress/gzip"
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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"dropsuite-pp-cli/internal/cliutil/testenv"
)

const (
	bbOrg       = "26561"
	bbAccess    = "dsACCESS7fb3189bQ6u3sZxW4o1n7R8v52"                  // DROPSUITE_ACCESS_TOKEN (reseller admin token)
	bbReseller  = "dsRSLR2c9e41aaK8p0tYq3m5w7Z1Xv"                      // DROPSUITE_RESELLER_TOKEN
	bbOrgToken  = "orgTKN" + "9d4c2b7aF3h5j7k9m1n3p5q7Wz"               // the organization's authentication_token in GET /users
	bbStored    = "storedToken-71c3e9b05d2a4f68e1"                      // auth set-token (stdin), the credential store
	bbShortResp = "Sh0rtPw"                                             // a 7-character credential a response carries
	bbBOMResp   = "BomRespSecret-6a1c"                                  // a credential in a BOM-prefixed response
	bbGzipResp  = "GzipRespToken-4f2a9c1e"                              // a credential in a gzip-encoded response
	bbHTTPToken = "mcpHttpTok-7Qz9Lw2Rk5Vx8Np3Hd6Jf4Bc"                 // DROPSUITE_MCP_HTTP_TOKEN
	bbSSOLink   = "https://portal.example.com/sso/one-time-link-for-hq" // what a confirmed sign-in link call returns
)

func secrets() []string {
	return []string{bbAccess, bbReseller, bbOrgToken, bbStored, bbShortResp, bbBOMResp, bbGzipResp, bbHTTPToken}
}

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "ds-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "dropsuite-cli"+exe)
	bbMCP = filepath.Join(dir, "dropsuite-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/dropsuite-cli", bbMCP: "../../cmd/dropsuite-mcp"} {
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
	return append([]string{
		s,
		"x" + s + "x",
		"Bearer " + s,
		base64.StdEncoding.EncodeToString([]byte(s)),
		base64.RawURLEncoding.EncodeToString([]byte(s)),
		hex.EncodeToString([]byte(s)),
		base32.StdEncoding.EncodeToString([]byte(s)),
		a85.String(),
		url.QueryEscape(s),
	}, textEncodedForms(s)...)
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

// sentSecrets is every known secret the request carried (its headers, a
// decoded Basic credential, the URL or the body): a real API can echo only
// what it was sent.
func sentSecrets(r *http.Request) []string {
	body, _ := io.ReadAll(r.Body)
	seen := fmt.Sprint(r.Header) + "\n" + r.URL.String() + "\n" + string(body)
	if u, p, ok := r.BasicAuth(); ok {
		seen += "\n" + u + "\n" + p
	}
	var out []string
	for _, s := range secrets() {
		if strings.Contains(seen, s) {
			out = append(out, s)
		}
	}
	return out
}

type server struct {
	url     string
	caFile  string
	mu      sync.Mutex
	offHost int
	ssoMint int // sign-in link calls that reached the API
	paths   []string
	// quiet stops the echo: the kit withholds a whole unit (an output line, a
	// file write, a SQL statement) that repeats a held value, so a test that
	// proves the program still prints and stores data sets it.
	quiet bool
}

// tlsServer starts a TLS fake (HTTP/2 enabled; the client also speaks HTTP/1.1).
func tlsServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func writeCA(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return ca
}

func echoServer(t *testing.T) *server {
	t.Helper()
	sv := &server{}
	// A second host the API redirects to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(`[]`))
	}))
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.paths = append(sv.paths, r.URL.Path)
		// Any path that names a sign-in link route, in any spelling the server
		// might route (decoded or raw, any case), counts as a mint.
		if strings.Contains(strings.ToLower(r.URL.Path+" "+r.URL.RawPath), "generate_") {
			sv.ssoMint++
		}
		sv.mu.Unlock()
		sent := sentSecrets(r)
		sv.mu.Lock()
		if sv.quiet {
			sent = nil
		}
		sv.mu.Unlock()
		echo := echoObject(sent)
		w.Header().Set("Content-Type", "application/json")
		// A large advertised budget, so the client's adaptive limiter (2 req/s
		// until headers arrive) does not pace the fan-out commands: the suite
		// fits CI's ten-minute go test default on a shared runner.
		w.Header().Set("X-Ratelimit-Remaining", "100000")
		w.Header().Set("X-Ratelimit-Reset", strconv.FormatInt(time.Now().Add(time.Second).Unix(), 10))
		org := fmt.Sprintf(`{"id":%q,"organization_id":"26561-14","email":"owner@example.com","organization_name":"HQ-Org","external_id":"ext-1","plan_id":"p1","seats_used":3,"seats_available":10,"storage_used":5,"storage_available":50,"authentication_token":%q,"echo":%s}`, bbOrg, bbOrgToken, echo)
		switch p := strings.TrimPrefix(r.URL.Path, "/api"); p {
		case "/users":
			fmt.Fprintf(w, `[%s]`, org)
		case "/users/" + bbOrg:
			fmt.Fprint(w, org)
		case "/countries":
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":"invalid token %s","message":"bad token %s","echo":%s}`, strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case "/timezones":
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case "/locales":
			fmt.Fprintf(w, "\xef\xbb\xbf"+`{"english":"en","display":%q,"password":%q}`, bbBOMResp, bbBOMResp)
		case "/status":
			fmt.Fprint(w, `"OK"`)
		case "/onedrives":
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			fmt.Fprintf(zw, `[{"id":"od1","email":"hq@example.com","display_name":"HQ-Record","current_backup_status":"Completed","last_backup":"2026-10-20T10:00:00Z","access_token":%q,"echo":%s}]`, bbGzipResp, echoObject(append(sent, bbGzipResp)))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		case "/sharepoints/domains":
			fmt.Fprintf(w, `[{"id":"d1","domain_name":"example.com","split":%s}]`, splitObject(sent))
		case "/sharepoints/domains/d1/sites":
			fmt.Fprintf(w, `[{"id":"s1","sp_site":"Finance","display_name":%q,"password":%q}]`, bbShortResp, bbShortResp)
		case "/users/generate_redirect_link", "/users/generate_support_sso_link":
			fmt.Fprintf(w, `{"redirect_url":%q}`, bbSSOLink)
		default:
			extra := ""
			if len(sent) > 0 {
				// A record whose identifier repeats the token it was sent.
				extra = fmt.Sprintf(`,{"id":%q,"email":"echo@example.com","display_name":"Echo-ID"}`, sent[0])
			}
			fmt.Fprintf(w, `[{"id":"100","email":"hq@example.com","display_name":"HQ-Record","current_backup_status":"Completed","last_backup":"2026-10-20T10:00:00Z","added_on":"2026-01-01","echo":%s}%s]`, echo, extra)
		}
	}))
	sv.url = srv.URL + "/api"
	sv.caFile = writeCA(t, srv)
	return sv
}

type bbEnv struct {
	home string
	env  []string
}

func newEnv(t *testing.T, sv *server, withEnvToken bool) *bbEnv {
	t.Helper()
	// The child gets HOME and no DROPSUITE_HOME, so the test process is
	// sandboxed the same way (and its DROPSUITE_* and XDG_* overrides cleared).
	home := testenv.Isolate(t)
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"DROPSUITE_BASE_URL=" + sv.url, "DROPSUITE_CA_CERT=" + sv.caFile,
		"DROPSUITE_CLI_PATH=" + bbCLI, "DROPSUITE_MCP_HTTP_TOKEN=" + bbHTTPToken, "DROPSUITE_RESELLER_TOKEN=" + bbReseller,
	}
	if withEnvToken {
		env = append(env, "DROPSUITE_ACCESS_TOKEN="+bbAccess)
	}
	return &bbEnv{home: home, env: env}
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
	cmd.Env = append(append([]string{}, e.env...), "DROPSUITE_ORG="+bbOrg)
	cmd.Dir = e.home
	stdin, _ := cmd.StdinPipe()
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(stdin, &in)
	deadline := time.Now().Add(120 * time.Second)
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

// battery runs every output path; tok is the credential the process holds
// (the environment token, or the stored one when the environment has none).
func battery(e *bbEnv, deliverFile, tok string) [][]string {
	O := bbOrg
	return [][]string{
		{"organizations", "list", "--json"},
		{"organizations", "list"},
		{"organizations", "list", "--csv"},
		{"organizations", "list", "--plain"},
		{"organizations", "list", "--json", "--receipt"},
		{"organizations", "list", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		{"organizations", "list", "--json", "--deliver", "file:" + deliverFile},
		{"organizations", "list", "--dry-run", "--json"},
		{"organizations", "get", O, "--json"},
		{"organizations", "list-tenants", O, "--json"},
		{"organizations", "list-tenant-accounts", O, "t1", "--json"},
		{"accounts", "list", "--org", O, "--json"},
		{"accounts", "list", "--org", O},
		{"accounts", "list-deactivated", "--org", O, "--csv"},
		{"onedrives", "--org", O, "--json"},
		{"sharepoint-domains", "--org", O, "--json"},
		{"sharepoint-sites", "d1", "--org", O, "--json"},
		{"reference", "list-countries", "--json"},
		{"reference", "list-timezones", "--json"},
		{"reference", "list-locales", "--json"},
		{"server", "--json"},
		{"organizations", "list", "--json", "--" + tok},
		{"health", "--json"},
		{"health", "--failing", "--plain"},
		{"coverage", "--json"},
		{"billing", "--json"},
		{"evidence", "--org", O, "--json"},
		{"evidence", "--org", O, "--markdown"},
		{"hygiene", "--json"},
		{"locate", "hq@example.com", "--json"},
		{"drift", "--json"},
		{"restore-test", "record", "--org", O, "--object", "hq@example.com", "--result", "pass", "--note", "restored with " + tok},
		{"restore-test", "due", "--json"},
		{"restore-test", "list", "--json"},
		{"sync"},
		{"export", "organizations", "--format", "json", "--output", filepath.Join(e.home, "export.json")},
		{"feedback", "noted the token " + tok + " in a ticket"},
		// Learnings, playbooks and learn events land in the local store's own tables.
		{"teach", "--query", "find mailboxes " + tok, "--resource", "123", "--resource-type", "accounts",
			"--notes", "rotated " + tok, "--playbook-notes", "use " + tok, "--no-validate"},
		{"doctor"},
		{"auth", "status"},
	}
}

func mcpBattery() []map[string]any {
	return []map[string]any{
		{"name": "organizations_list", "arguments": map[string]any{}},
		{"name": "organizations_get", "arguments": map[string]any{"id": bbOrg}},
		{"name": "accounts_list", "arguments": map[string]any{}},
		{"name": "onedrives_list", "arguments": map[string]any{}},
		{"name": "reference_list-countries", "arguments": map[string]any{}},
		{"name": "reference_list-locales", "arguments": map[string]any{}},
		{"name": "health", "arguments": map[string]any{"json": true}},
		{"name": "locate", "arguments": map[string]any{"email": "hq@example.com", "json": true}},
		{"name": "evidence", "arguments": map[string]any{"org": bbOrg, "json": true}},
		{"name": "portal_get-login-link", "arguments": map[string]any{}},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
	}
}

// Class credential-persisted-in-files.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, false)
	e.runIn(t, bbStored+"\n", "auth", "set-token")
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, args := range battery(e, deliverFile, bbStored) {
		e.run(t, args...)
	}
	e.mcpCalls(t, mcpBattery())
	// The kit withholds a whole write (a file, a SQL statement) that repeats a
	// held value, so against the echoing API most of the battery stores
	// nothing. The same writes against an API that echoes nothing prove the
	// sinks still store data, and the scan below covers both runs.
	sv.mu.Lock()
	sv.quiet = true
	sv.mu.Unlock()
	for _, args := range [][]string{
		{"organizations", "list", "--json", "--deliver", "file:" + deliverFile},
		{"export", "organizations", "--format", "json", "--output", filepath.Join(e.home, "export.json")},
		{"sync"},
		{"health", "--json"},
		{"restore-test", "record", "--org", bbOrg, "--object", "hq@example.com", "--result", "pass", "--note", "restored with care"},
		{"teach", "--query", "find mailboxes", "--resource", "123", "--resource-type", "accounts",
			"--notes", "rotated keys", "--playbook-notes", "use the admin token", "--no-validate"},
	} {
		e.run(t, args...)
	}
	var scanned int
	var sawCreds bool
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
			// The credential store: the one file allowed to hold a credential.
			sawCreds = sawCreds || bytes.Contains(data, []byte(bbStored))
			if info, err := d.Info(); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
				t.Errorf("%s mode %v, want 0600", rel, info.Mode().Perm())
			}
			return nil
		}
		assertNoEcho(t, "file "+rel, string(data))
		return nil
	})
	if scanned < 5 {
		t.Fatalf("only %d files written; the battery did not run", scanned)
	}
	if !sawCreds {
		t.Errorf("auth set-token wrote no credential store")
	}
	// The battery really wrote data, so the scan above is not vacuous.
	if b, err := os.ReadFile(deliverFile); err != nil || !bytes.Contains(b, []byte("HQ-Org")) {
		t.Errorf("%s: want the delivered record (err %v)", filepath.Base(deliverFile), err)
	}
	if b, err := os.ReadFile(filepath.Join(e.home, "export.json")); err != nil || !bytes.Contains(b, []byte("HQ-Org")) {
		t.Errorf("export.json: want the exported organization (err %v)", err)
	}
	var sawStore, sawSnapshot, sawLedger, sawLearning bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".db") {
			if b, rerr := os.ReadFile(path); rerr == nil {
				sawStore = sawStore || bytes.Contains(b, []byte("HQ-Org"))
				// health saves a snapshot row per object; restore-test record a ledger row.
				sawSnapshot = sawSnapshot || bytes.Contains(b, []byte("HQ-Record"))
				sawLedger = sawLedger || bytes.Contains(b, []byte("restored with"))
				// The clean teach run's learning row was written.
				sawLearning = sawLearning || bytes.Contains(b, []byte("rotated"))
			}
		}
		return nil
	})
	if !sawStore {
		t.Errorf("no local store holds a fetched organization")
	}
	if !sawSnapshot {
		t.Errorf("no local store holds a health snapshot record")
	}
	if !sawLedger {
		t.Errorf("no local store holds the recorded restore test")
	}
	if !sawLearning {
		t.Errorf("no local store holds the taught learning; the teach run wrote nothing")
	}
}

// Class secret-in-output.
func TestNoCredentialInAnyOutput(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, args := range battery(e, deliverFile, bbAccess) {
		assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
	}
	// Against an API that echoes nothing, the data still prints and JSON
	// output stays valid JSON (an echoing answer is withheld whole).
	sv.mu.Lock()
	sv.quiet = true
	sv.mu.Unlock()
	cmd := exec.Command(bbCLI, "accounts", "list", "--org", bbOrg, "--json")
	cmd.Env, cmd.Dir = e.env, e.home
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("accounts list --json: %v", err)
	}
	var doc any
	if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("HQ-Record")) {
		t.Fatalf("want valid JSON with the record, got:\n%.1500s", stdout)
	}
	// The organization request really went out with the organization token,
	// so the scan above covered a routed request.
	sv.mu.Lock()
	paths := strings.Join(sv.paths, " ")
	sv.mu.Unlock()
	if !strings.Contains(paths, "/api/accounts") || !strings.Contains(paths, "/api/users") {
		t.Fatalf("the battery never reached the organization routes: %s", paths)
	}
	quietOut := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(quietOut, "HQ-Record") {
		t.Fatalf("MCP results lost the record itself:\n%.2000s", quietOut)
	}
	sv.mu.Lock()
	sv.quiet = false
	sv.mu.Unlock()
	out := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
		t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP session", out)
	assertNoEcho(t, "quiet MCP session", quietOut)
}

// Class credential-sent-to-another-host: a redirect off the configured API
// host never reaches it, from the CLI or the MCP server.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	redirect := e.run(t, "reference", "list-timezones", "--json")
	e.mcpCalls(t, []map[string]any{
		{"name": "reference_list-timezones", "arguments": map[string]any{}},
	})
	sv.mu.Lock()
	n := sv.offHost
	sv.mu.Unlock()
	if n != 0 {
		t.Fatalf("the other host was contacted %d time(s)", n)
	}
	if !strings.Contains(redirect, "leave the configured Dropsuite API host") {
		t.Fatalf("redirect: want the pin refusal, got:\n%.1500s", redirect)
	}
}

// MCP writes preview until confirm is boolean true: the unconfirmed sign-in
// link calls (endpoint tool, and locate/evidence with their link flags) mint
// nothing; the confirmed endpoint call mints exactly one link and returns it.
func TestMCPWritesNeedConfirm(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	unconfirmed := []map[string]any{
		{"name": "portal_get-login-link", "arguments": map[string]any{}},
		{"name": "portal_get-login-link", "arguments": map[string]any{"confirm": "true"}},
		{"name": "organizations_get-support-sso-link", "arguments": map[string]any{"email": "owner@example.com", "confirm": 1}},
		{"name": "locate", "arguments": map[string]any{"email": "hq@example.com", "sso-link": true, "json": true}},
		{"name": "evidence", "arguments": map[string]any{"org": bbOrg, "with-sso-link": true, "confirm": false, "json": true}},
	}
	out := e.mcpCalls(t, unconfirmed)
	sv.mu.Lock()
	minted := sv.ssoMint
	sv.mu.Unlock()
	if minted != 0 {
		t.Fatalf("unconfirmed MCP calls minted %d sign-in link(s):\n%.2000s", minted, out)
	}
	if strings.Count(out, "confirm") < len(unconfirmed) || strings.Contains(out, bbSSOLink) {
		t.Fatalf("want a preview for every unconfirmed call and no link, got:\n%.3000s", out)
	}
	out = e.mcpCalls(t, []map[string]any{
		{"name": "portal_get-login-link", "arguments": map[string]any{"confirm": true}},
	})
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if sv.ssoMint != 1 || !strings.Contains(out, bbSSOLink) {
		t.Fatalf("confirm: true minted %d link(s) (want 1, returned to the caller):\n%.2000s", sv.ssoMint, out)
	}
}
