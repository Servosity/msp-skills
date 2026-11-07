// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package msgraphblackbox runs the built CLI and MCP binaries against a
// Microsoft Graph fake that echoes every credential it knows back in every
// encoding it can, whole and split across lines and adjacent values, and
// checks what a reviewer cannot read off the code (microsoft-graph handfixes
// output-chokepoint, store-never-keeps-secrets, no-response-cache,
// no-credential-in-files, response-credentials-held, api-origin-pinned):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the bearer token from the environment or the
//     credential store, the MCP HTTP token, a credential-named flag value,
//     and the credentials a response carries;
//   - no file the program writes, except the 0600 credential store, holds
//     one either;
//   - no credential leaves for a host other than the configured one, by
//     redirect or by @odata.nextLink;
//   - the MCP server sends nothing but reads.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder.
package msgraphblackbox

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
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	bbToken     = "graphTOKEN7fb3189bQ6u3sZxW4o1n7R8v52" // MICROSOFT_GRAPH_TOKEN
	bbStored    = "storedToken-71c3e9b05d2a4f68e1"       // auth set-token (stdin), the credential store
	bbArgvSec   = "-argvSecret-91c3e7a5"                 // --client-secret on the command line (starts with "-")
	bbShortResp = "Sh0rtPw"                              // a 7-character credential a response carries
	bbBOMResp   = "BomRespSecret-6a1c"                   // a credential in a BOM-prefixed response
	bbGzipResp  = "GzipRespToken-4f2a9c1e"               // a credential in a gzip-encoded response
	bbHTTPToken = "mcpHttpToken-0123456789abcdefABCDEF"  // MICROSOFT_GRAPH_MCP_HTTP_TOKEN
)

// withheldNotice is the start of what the credential kit prints in place of a
// unit that carries a credential the program holds.
const withheldNotice = "[credkit: withheld"

func secrets() []string {
	return []string{bbToken, bbStored, bbArgvSec, bbShortResp, bbBOMResp, bbGzipResp, bbHTTPToken}
}

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "msgraph-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "microsoft-graph-cli"+exe)
	bbMCP = filepath.Join(dir, "microsoft-graph-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/microsoft-graph-cli", bbMCP: "../../cmd/microsoft-graph-mcp"} {
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

// sentSecrets is every known secret the request carried (its headers, the
// URL or the body): a real API can echo only what it was sent.
func sentSecrets(r *http.Request) []string {
	body, _ := io.ReadAll(r.Body)
	seen := fmt.Sprint(r.Header) + "\n" + r.URL.String() + "\n" + string(body)
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
	quiet   bool // a real Graph answer: no echo, no credential in any response
	mu      sync.Mutex
	offHost int
	methods []string
}

// tlsServer starts an HTTP/2 TLS fake.
func tlsServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// graphRecord is one directory-shaped record every collection returns, so the
// analytics and the store have something real to read.
const graphRecord = `"id":"5c802b19-3600-83f1-1767-7b9edc7f38ab","displayName":"HQ-Record","userPrincipalName":"hq@example.com","mail":"hq@example.com","accountEnabled":true,"userType":"Member","createdDateTime":"2026-10-20T10:00:00Z","assignedLicenses":[],"severity":"high","status":"new","skuPartNumber":"ENTERPRISEPACK","skuId":"6fd2c87f-b296-42f0-b197-1e91e994b900","consumedUnits":1,"prepaidUnits":{"enabled":5},"complianceState":"compliant","isEncrypted":true,"lastSyncDateTime":"2026-10-20T10:00:00Z","telephoneNumber":"+12052582895","isEnterpriseVoiceEnabled":true`

// quietServer is echoServer without the echoes and the response credentials:
// a real Graph answer, whose records must reach every sink the echoing
// server's are withheld from.
func quietServer(t *testing.T) *server {
	t.Helper()
	return newServer(t, true)
}

func echoServer(t *testing.T) *server {
	t.Helper()
	return newServer(t, false)
}

func newServer(t *testing.T, quiet bool) *server {
	t.Helper()
	sv := &server{quiet: quiet}
	// A second host the API redirects and links to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		sent := sentSecrets(r)
		echo := echoObject(sent)
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		sv.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		p := strings.TrimPrefix(r.URL.Path, "/v1.0")
		if sv.quiet {
			if strings.Count(strings.Trim(p, "/"), "/") >= 1 && !strings.HasSuffix(p, "/members") && !strings.HasSuffix(p, "/owners") && p != "/users/me" &&
				!strings.HasPrefix(p, "/admin/") && !strings.HasPrefix(p, "/deviceManagement/") && !strings.HasPrefix(p, "/security/") {
				fmt.Fprintf(w, `{%s}`, graphRecord)
				return
			}
			fmt.Fprintf(w, `{"value":[{%s}]}`, graphRecord)
			return
		}
		switch {
		case strings.HasPrefix(p, "/security/alerts_v2"):
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":{"code":"InvalidAuthenticationToken","message":"invalid token %s","innerError":{"message":"bad token %s"}},"echo":%s}`, strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case strings.HasPrefix(p, "/directoryRoles"):
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case p == "/devices" && r.URL.Query().Get("$skiptoken") == "":
			fmt.Fprintf(w, `{"value":[{%s}],"@odata.nextLink":%q}`, graphRecord, other.URL+r.URL.Path+"?$skiptoken=2")
		case strings.HasPrefix(p, "/deviceManagement/managedDevices"):
			fmt.Fprintf(w, `{"value":[{%s,"split":%s}]}`, graphRecord, splitObject(sent))
		case strings.HasPrefix(p, "/subscribedSkus"):
			fmt.Fprintf(w, "\xef\xbb\xbf"+`{"value":[{%s,"capabilityStatus":%q,"password":%q}]}`, graphRecord, bbBOMResp, bbBOMResp)
		case strings.HasPrefix(p, "/admin/teams/userConfigurations"):
			fmt.Fprintf(w, `{"value":[{%s,"note":%q,"password":%q}]}`, graphRecord, bbShortResp, bbShortResp)
		case strings.HasPrefix(p, "/admin/teams/telephoneNumberManagement"):
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			fmt.Fprintf(zw, `{"value":[{%s,"accessToken":%q,"echo":%s}]}`, graphRecord, bbGzipResp, echoObject(append(sent, bbGzipResp)))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		default:
			extra := ""
			if len(sent) > 0 {
				// A record whose identifier repeats the token it was sent.
				extra = fmt.Sprintf(`,{"id":%q,"displayName":"Echo-ID"}`, sent[0])
			}
			if strings.Count(strings.Trim(p, "/"), "/") >= 1 && !strings.HasSuffix(p, "/members") && !strings.HasSuffix(p, "/owners") && p != "/users/me" {
				// A single object (users/{id}, ...).
				fmt.Fprintf(w, `{%s,"echo":%s}`, graphRecord, echo)
				return
			}
			fmt.Fprintf(w, `{"value":[{%s,"echo":%s}%s]}`, graphRecord, echo, extra)
		}
	}))
	sv.url = srv.URL + "/v1.0"
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.caFile = ca
	return sv
}

type bbEnv struct {
	home string
	env  []string
}

func newEnv(t *testing.T, sv *server, withEnvToken bool) *bbEnv {
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
		"MICROSOFT_GRAPH_BASE_URL=" + sv.url, "MICROSOFT_GRAPH_CA_CERT=" + sv.caFile,
		"MICROSOFT_GRAPH_CLI_PATH=" + bbCLI, "MICROSOFT_GRAPH_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}
	if withEnvToken {
		env = append(env, "MICROSOFT_GRAPH_TOKEN="+bbToken)
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
	return e.mcpPhases(t, calls)
}

// mcpPhases runs one MCP stdio session, sending each phase of tools/call
// requests only after every earlier call was answered (mcp-go serves the calls
// of one phase concurrently), and returns everything the server wrote.
func (e *bbEnv) mcpPhases(t *testing.T, phases ...[]map[string]any) string {
	t.Helper()
	cmd := exec.Command(bbMCP)
	cmd.Env = e.env
	cmd.Dir = e.home
	stdin, _ := cmd.StdinPipe()
	var out, errb lockedBuffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(stdin)
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}})
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	id := 0
	deadline := time.Now().Add(120 * time.Second)
	for _, calls := range phases {
		for _, c := range calls {
			id++
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": c})
		}
		for time.Now().Before(deadline) && strings.Count(out.String(), `"id":`) < id+1 {
			time.Sleep(100 * time.Millisecond)
		}
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

// lockedBuffer is a bytes.Buffer safe to poll while a process writes to it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
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
		// The kit holds a value under 8 characters only as a whole token
		// (internal/credkit/match.go): its halves are ordinary text.
		if a, b := halves(s); len(s) >= 8 && strings.Contains(text, a) && strings.Contains(text, b) {
			t.Errorf("%s holds both halves of %q", where, s)
		}
	}
}

// battery runs every output path; tok is the credential the process holds
// (the environment token, or the stored one when the environment has none).
func battery(e *bbEnv, deliverFile, tok string) [][]string {
	const id = "5c802b19-3600-83f1-1767-7b9edc7f38ab"
	return [][]string{
		{"users", "list", "--json"},
		{"users", "list"},
		{"users", "list", "--csv"},
		{"users", "list", "--plain"},
		{"users", "list", "--json", "--receipt"},
		{"users", "list", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		{"users", "list", "--json", "--deliver", "file:" + deliverFile},
		{"users", "get", id, "--json"},
		{"groups", "list", "--json"},
		{"directory-roles", "list", "--json"},
		{"devices", "list", "--json"},
		{"managed-devices", "list", "--json"},
		{"licenses", "skus", "--json"},
		{"teams-users", "list", "--json"},
		{"phone-numbers", "list", "--json"},
		{"security", "alerts", "--json"},
		{"security", "incidents", "--json"},
		{"pull", "--json"},
		{"sync", "--json"},
		{"export", "users", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
		{"licenses", "waste", "--json"},
		{"licenses", "orphans", "--json"},
		{"licenses", "map", "ENTERPRISEPACK", "--json"},
		{"admins", "audit", "--json"},
		{"security", "triage", "--json"},
		{"managed-devices", "drift", "--json"},
		{"tenant", "snapshot", "--json"},
		{"groups", "risk", "--json"},
		{"apps", "consent", "--json"},
		// Refused before any request: the token's tenant cannot be confirmed.
		{"transcripts", "list", "--meeting", tok},
		{"transcripts", "download", "--meeting", "m1", "--transcript", tok},
		{"users", "list", "--client-secret", bbArgvSec},
		{"users", "list", "--json", "--" + tok},
		{"feedback", "noted the token " + tok + " in a ticket"},
		// Learnings, playbooks and learn events land in the local store's own tables.
		{"teach", "--query", "find users " + tok, "--resource", "123", "--resource-type", "users",
			"--notes", "rotated " + tok, "--playbook-notes", "use " + tok, "--no-validate"},
		{"doctor"},
		{"auth", "status"},
	}
}

func mcpBattery() []map[string]any {
	return []map[string]any{
		{"name": "users_list", "arguments": map[string]any{}},
		{"name": "users_get", "arguments": map[string]any{"id": "5c802b19-3600-83f1-1767-7b9edc7f38ab"}},
		{"name": "teams-users_list", "arguments": map[string]any{}},
		{"name": "phone-numbers_list", "arguments": map[string]any{}},
		{"name": "managed-devices_list", "arguments": map[string]any{}},
		{"name": "security_alerts", "arguments": map[string]any{}},
		{"name": "apps_consent", "arguments": map[string]any{}},
		{"name": "licenses_waste", "arguments": map[string]any{}},
		{"name": "transcripts_list", "arguments": map[string]any{"meeting": "m1"}},
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
	// The battery really reached the API with the stored token, so the scan
	// above is not vacuous (TestOrdinaryRecordsReachEveryFile proves the sinks
	// write what carries no credential).
	sv.mu.Lock()
	n := len(sv.methods)
	sv.mu.Unlock()
	if n < 10 {
		t.Fatalf("only %d requests reached the API; the battery did not run", n)
	}
}

// The control for TestNoCredentialInAnyWrittenFile: a Graph that repeats no
// credential has its records reach every file sink, so a write withheld there
// is the credential and not a broken sink.
func TestOrdinaryRecordsReachEveryFile(t *testing.T) {
	e := newEnv(t, quietServer(t), true)
	deliverFile := filepath.Join(e.home, "deliver.json")
	exportFile := filepath.Join(e.home, "export.jsonl")
	for _, args := range [][]string{
		{"users", "list", "--json", "--deliver", "file:" + deliverFile},
		{"export", "users", "--format", "jsonl", "--output", exportFile},
		{"sync", "--json"},
		{"teach", "--query", "find users", "--resource", "123", "--resource-type", "users", "--notes", "rotated", "--no-validate"},
		{"feedback", "an ordinary note"},
	} {
		if out := e.run(t, args...); strings.Contains(out, withheldNotice) {
			t.Errorf("%v withheld an ordinary record:\n%.1500s", args, out)
		}
	}
	for _, f := range []string{deliverFile, exportFile} {
		if b, err := os.ReadFile(f); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
			t.Errorf("%s: want the record (err %v)", filepath.Base(f), err)
		}
	}
	var sawStore, sawLearning bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".db") {
			if b, rerr := os.ReadFile(path); rerr == nil {
				sawStore = sawStore || bytes.Contains(b, []byte("HQ-Record"))
				sawLearning = sawLearning || bytes.Contains(b, []byte("rotated"))
			}
		}
		return nil
	})
	if !sawStore {
		t.Errorf("no local store holds a fetched record")
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
	for _, args := range battery(e, deliverFile, bbToken) {
		assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
	}
	// The unit that repeats a credential is withheld whole, with the kit's notice.
	if users := e.run(t, "users", "list", "--json"); !strings.Contains(users, withheldNotice) {
		t.Fatalf("users list --json repeats a credential and printed no notice:\n%.1500s", users)
	}
	out := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
		t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP session", out)
}

// The control for TestNoCredentialInAnyOutput: a Graph that repeats no
// credential has its records printed, on the CLI and through the MCP server,
// and JSON output stays JSON.
func TestOrdinaryRecordsStillPrint(t *testing.T) {
	e := newEnv(t, quietServer(t), true)
	cmd := exec.Command(bbCLI, "users", "list", "--json")
	cmd.Env, cmd.Dir = e.env, e.home
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("users list --json: %v", err)
	}
	var doc any
	if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("HQ-Record")) {
		t.Fatalf("want valid JSON with the record, got:\n%.1500s", stdout)
	}
	out := e.mcpCalls(t, []map[string]any{
		{"name": "users_list", "arguments": map[string]any{}},
		{"name": "users_get", "arguments": map[string]any{"id": "5c802b19-3600-83f1-1767-7b9edc7f38ab"}},
	})
	if strings.Contains(out, withheldNotice) || strings.Count(out, "HQ-Record") < 2 {
		t.Fatalf("MCP results lost the record:\n%.3000s", out)
	}
}

// Class credential-sent-to-another-host: neither a redirect nor an
// @odata.nextLink off the configured API host reaches it (pull follows a
// nextLink's path and query on the configured host only).
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	redirect := e.run(t, "directory-roles", "list", "--json")
	e.run(t, "pull", "--only", "devices,directory-roles", "--json")
	e.run(t, "sync", "--resources", "devices", "--json")
	e.mcpCalls(t, []map[string]any{
		{"name": "directory-roles_list", "arguments": map[string]any{}},
		{"name": "devices_list", "arguments": map[string]any{}},
	})
	sv.mu.Lock()
	n := sv.offHost
	sv.mu.Unlock()
	if n != 0 {
		t.Fatalf("the other host was contacted %d time(s)", n)
	}
	if !strings.Contains(redirect, "leave the configured Microsoft Graph API host") {
		t.Fatalf("redirect: want the pin refusal, got:\n%.1500s", redirect)
	}
}

// The connector is read-only: every request the MCP server and its mirrored
// CLI children send is a GET.
func TestMCPSendsOnlyReads(t *testing.T) {
	sv := quietServer(t)
	e := newEnv(t, sv, true)
	out := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(out, "HQ-Record") {
		t.Fatalf("the MCP battery fetched nothing:\n%.2000s", out)
	}
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if len(sv.methods) == 0 {
		t.Fatal("no request reached the fake")
	}
	for _, m := range sv.methods {
		if !strings.HasPrefix(m, "GET ") {
			t.Errorf("the MCP server sent %s", m)
		}
	}
}
