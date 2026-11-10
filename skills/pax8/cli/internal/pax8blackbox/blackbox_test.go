// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package pax8blackbox runs the built CLI and MCP binaries against a
// Pax8 fake that echoes every credential it knows back in every
// encoding it can, whole and split across lines and adjacent values, and
// checks what a reviewer cannot read off the code (pax8 handfixes
// credkit-wiring, no-response-cache, api-origin-pinned):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the client pair in the environment, the token in the
//     credential store, the MCP HTTP token, a credential-named flag value,
//     and the credentials a response carries under a declared field;
//   - no file the program writes, except the 0600 credential store, holds
//     one either;
//   - no credential leaves for a host other than the one it is bound to, by
//     redirect or by a configured token URL;
//   - the MCP server sends nothing but reads.
//
// The client ID and secret are bound to Pax8's token manager, so no fake can
// receive them: every run that reaches the API authenticates with a token
// stored by auth set-token. The minted token's hold is proven in process
// (internal/cli TestAuthLoginMintIsPinnedAndHeld).
//
// The kit withholds a unit whole (credential leak contract section 4, no
// partial redaction), so an answer that echoes the held token prints none of
// its data: the echoing fake proves the withholding, and the quiet fake (the
// same records, repeating nothing a request carried) proves each sink really
// writes.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder.
package pax8blackbox

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
	bbToken     = "qzSECRET7fb3189bQ6u3sZxW4o1n7R8v52" // PAX8_CLIENT_SECRET (the env client secret) gitleaks:allow
	bbClientID  = "qzCliRef-3e9a7c1d5b2f8a46"          // PAX8_CLIENT_ID gitleaks:allow
	bbMinted    = "qzMntAcc-9d2b6f4a1c8e7305ab"        // what the fake token endpoint would mint; the kit never lets the pair reach it
	bbStored    = "qzStTkn-71c3e9b05d2a4f68e1"         // auth set-token (stdin), the credential store
	bbArgvSec   = "-argvSecret-91c3e7a5"               // --client-secret on the command line (starts with "-")
	bbShortResp = "Sh0rtPw"                            // a 7-character credential a response carries (refresh_token)
	bbBOMResp   = "BomRespSecret-6a1c"                 // a credential in a BOM-prefixed response (access_token)
	bbGzipResp  = "GzipRespToken-4f2a9c1e"             // a credential in a gzip-encoded response (access_token)
	bbHTTPToken = "qzMhTkn-0123456789abcdefABCDEF0123" // PAX8_MCP_HTTP_TOKEN gitleaks:allow
)

func secrets() []string {
	return []string{bbToken, bbClientID, bbMinted, bbStored, bbArgvSec, bbShortResp, bbBOMResp, bbGzipResp, bbHTTPToken}
}

var bbCLI, bbMCP string

// kitNotice is the fixed notice the kit writes in place of a unit it withheld
// from sink (credkit.Notice).
func kitNotice(sink string) string {
	return "[credkit: withheld from " + sink + ": it carried a credential this program holds]"
}

// kitClosedMCP is what the MCP server logs when the kit withholds a JSON-RPC
// message the tool layer did not answer: the kit fails the session's output
// closed, so that request and every later one get no reply.
const kitClosedMCP = "credkit: the MCP response failed closed"

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "pax8-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "pax8-cli"+exe)
	bbMCP = filepath.Join(dir, "pax8-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/pax8-cli", bbMCP: "../../cmd/pax8-mcp"} {
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
	url      string
	tokenURL string
	caFile   string
	mu       sync.Mutex
	offHost  int
	methods  []string // API requests
	mints    int      // token requests
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

// pax8Record is one record every collection returns, shaped so the store and
// the analytics (reconcile, mrr, overage, since, company show, spend) have
// something real to read.
const pax8Record = `"id":"5c802b19-3600-83f1-1767-7b9edc7f38ab","name":"HQ-Record","companyId":"5c802b19-3600-83f1-1767-7b9edc7f38ab","productId":"6fd2c87f-b296-42f0-b197-1e91e994b900","productName":"HQ-Product","subscriptionId":"5c802b19-3600-83f1-1767-7b9edc7f38ab","status":"Active","quantity":3,"price":12.5,"partnerCost":9.5,"billingTerm":"Monthly","total":37.5,"invoiceDate":"2026-10-01","createdDate":"2026-10-20T10:00:00Z","updatedDate":"2026-10-21T10:00:00Z","email":"hq@example.com"`

// pageOf wraps rows the way the Partner API pages a list.
func pageOf(rows string) string {
	return `{"content":[` + rows + `],"page":{"size":200,"totalElements":1,"totalPages":1,"number":0}}`
}

// echoServer is the fake echoing every credential a request carried.
func echoServer(t *testing.T) *server {
	t.Helper()
	return fakeServer(t, false)
}

// quietServer is the fake answering as Pax8 does: the same records, repeating
// nothing a request carried and no credential.
func quietServer(t *testing.T) *server {
	t.Helper()
	return fakeServer(t, true)
}

func fakeServer(t *testing.T, quiet bool) *server {
	t.Helper()
	sv := &server{}
	// A second host the API and the token endpoint redirect to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(pageOf("")))
	}))
	// The token manager: mints bbMinted for the configured client, and echoes
	// what it was sent in the token response's extra fields.
	tok := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		sent := sentSecrets(r)
		sv.mu.Lock()
		sv.mints++
		sv.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost || r.URL.Path != "/oauth/token" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"invalid_request","echo":%s}`, echoObject(sent))
			return
		}
		form, _ := url.ParseQuery(string(body))
		if form.Get("client_id") != bbClientID || form.Get("client_secret") != bbToken {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":"access_denied","error_description":"bad client %s","echo":%s}`, strings.Join(sent, " "), echoObject(sent))
			return
		}
		fmt.Fprintf(w, `{"access_token":%q,"expires_in":86400,"token_type":"Bearer","echo":%s}`, bbMinted, echoObject(sent))
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
		p := strings.TrimPrefix(r.URL.Path, "/v1")
		if quiet {
			switch {
			case strings.HasPrefix(p, "/invoices/draftItems"):
				http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
			case r.Method != http.MethodGet, len(strings.Split(strings.Trim(p, "/"), "/")) == 2:
				fmt.Fprintf(w, `{%s}`, pax8Record)
			default:
				fmt.Fprint(w, pageOf(`{`+pax8Record+`}`))
			}
			return
		}
		switch {
		case strings.HasPrefix(p, "/orders"):
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"status":401,"error":"invalid token %s","message":"bad token %s","echo":%s}`, strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case strings.HasPrefix(p, "/invoices/draftItems"):
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case p == "/products":
			fmt.Fprint(w, pageOf(fmt.Sprintf(`{%s,"split":%s}`, pax8Record, splitObject(sent))))
		case p == "/subscriptions":
			fmt.Fprint(w, "\xef\xbb\xbf"+pageOf(fmt.Sprintf(`{%s,"note":%q,"access_token":%q}`, pax8Record, bbBOMResp, bbBOMResp)))
		case strings.HasSuffix(p, "/contacts"):
			fmt.Fprint(w, pageOf(fmt.Sprintf(`{%s,"note":%q,"refresh_token":%q}`, pax8Record, bbShortResp, bbShortResp)))
		case p == "/invoices":
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			fmt.Fprint(zw, pageOf(fmt.Sprintf(`{%s,"access_token":%q,"echo":%s}`, pax8Record, bbGzipResp, echoObject(append(sent, bbGzipResp)))))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		default:
			if r.Method != http.MethodGet {
				fmt.Fprintf(w, `{%s,"echo":%s}`, pax8Record, echo)
				return
			}
			extra := ""
			if len(sent) > 0 {
				// A record whose identifier repeats the token it was sent.
				extra = fmt.Sprintf(`,{"id":%q,"name":"Echo-ID"}`, sent[0])
			}
			segs := strings.Split(strings.Trim(p, "/"), "/")
			if len(segs) == 2 {
				// A single object (companies/{id}, ...).
				fmt.Fprintf(w, `{%s,"echo":%s}`, pax8Record, echo)
				return
			}
			fmt.Fprint(w, pageOf(fmt.Sprintf(`{%s,"echo":%s}%s`, pax8Record, echo, extra)))
		}
	}))
	sv.url = srv.URL + "/v1"
	sv.tokenURL = tok.URL + "/oauth/token"
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

// newEnv builds a clean environment against the fake; withEnvCreds sets the
// client credentials in the environment (else the battery runs on a token
// stored with auth set-token).
func newEnv(t *testing.T, sv *server, withEnvCreds bool) *bbEnv {
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
		"PAX8_BASE_URL=" + sv.url, "PAX8_TOKEN_URL=" + sv.tokenURL, "PAX8_CA_CERT=" + sv.caFile,
		"PAX8_CLI_PATH=" + bbCLI, "PAX8_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}
	if withEnvCreds {
		env = append(env, "PAX8_CLIENT_ID="+bbClientID, "PAX8_CLIENT_SECRET="+bbToken)
	}
	return &bbEnv{home: home, env: env}
}

// store saves the bearer token the run authenticates with, with a far expiry:
// a token with no recorded expiry is minted over once per process when a client
// pair is present, and no fake can mint (the kit binds the pair to Pax8).
func (e *bbEnv) store(t *testing.T) *bbEnv {
	t.Helper()
	e.runIn(t, bbStored+"\n", "auth", "set-token")
	var stored bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "credentials.toml" {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			t.Fatal(rerr)
		}
		stored = bytes.Contains(data, []byte(bbStored))
		if werr := os.WriteFile(path, append([]byte("token_expiry = 2099-01-01T00:00:00Z\n"), data...), 0o600); werr != nil {
			t.Fatal(werr)
		}
		return nil
	})
	if !stored {
		t.Fatal("auth set-token wrote no credential store")
	}
	return e
}

// useServer points the environment at another fake, the store kept.
func (e *bbEnv) useServer(sv *server) {
	e.env = append(withoutEnv(withoutEnv(e.env, "PAX8_BASE_URL"), "PAX8_CA_CERT"), "PAX8_BASE_URL="+sv.url, "PAX8_CA_CERT="+sv.caFile)
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
		if a, b := halves(s); strings.Contains(text, a) && strings.Contains(text, b) {
			t.Errorf("%s holds both halves of %q", where, s)
		}
	}
}

// battery runs every output path; tok is a credential the process holds
// (the environment client secret, or the stored token when the environment
// has none).
func battery(e *bbEnv, deliverFile, tok string) [][]string {
	const id = "5c802b19-3600-83f1-1767-7b9edc7f38ab"
	return [][]string{
		{"companies", "find", "--json"},
		{"companies", "find"},
		{"companies", "find", "--csv"},
		{"companies", "find", "--plain"},
		{"companies", "find", "--json", "--receipt"},
		{"companies", "find", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		{"companies", "find", "--json", "--deliver", "file:" + deliverFile},
		{"companies", "get", id, "--json"},
		{"companies", "contacts", "get", id, "--json"},
		{"products", "find-all", "--json"},
		{"products", "pricing", "find-by-product-id", id, "--json"},
		{"subscriptions", "find", "--json"},
		{"subscriptions", "history", "find-subscription-by-subscription-id", id, "--json"},
		{"invoices", "find-partner", "--json"},
		{"invoices", "find-partner-draft-items", "--json"},
		{"invoices", "items", "find-partner-invoice", id, "--json"},
		{"orders", "find", "--json"},
		{"usage-summaries", "usage-lines", "find", id, "--usage-date", "2026-01-01", "--json"},
		{"sync", "--json"},
		{"export", "companies", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
		{"reconcile", "--json"},
		{"mrr", "--json"},
		{"overage", "--json"},
		{"since", "30d", "--json"},
		{"company", "show", id, "--json"},
		{"spend", "--json"},
		{"companies", "find", "--client-secret", bbArgvSec},
		{"companies", "find", "--json", "--" + tok},
		{"companies", "get", tok, "--json"},
		{"feedback", "noted the token " + tok + " in a ticket"},
		// Previews: a dry run sends nothing and mints nothing.
		{"subscriptions", "delete", id, "--dry-run"},
		{"companies", "create-company", "--name", "Echo " + tok, "--dry-run"},
		// Learnings, playbooks and learn events land in the local store's own tables.
		{"teach", "--query", "find companies " + tok, "--resource", "123", "--resource-type", "companies",
			"--notes", "rotated " + tok, "--playbook-notes", "use " + tok, "--no-validate"},
		{"doctor"},
		{"auth", "status"},
	}
}

func mcpBattery() []map[string]any {
	const id = "5c802b19-3600-83f1-1767-7b9edc7f38ab"
	return []map[string]any{
		{"name": "companies_find", "arguments": map[string]any{}},
		{"name": "companies_get", "arguments": map[string]any{"companyId": id}},
		{"name": "companies_contacts_get", "arguments": map[string]any{"companyId": id}},
		{"name": "products_find-all", "arguments": map[string]any{}},
		{"name": "subscriptions_find", "arguments": map[string]any{}},
		{"name": "invoices_find-partner", "arguments": map[string]any{}},
		{"name": "orders_find", "arguments": map[string]any{}},
		{"name": "reconcile", "arguments": map[string]any{}},
		{"name": "company_show", "arguments": map[string]any{"args": id}},
		// Writes without confirm: previews, nothing sent.
		{"name": "companies_create-company", "arguments": map[string]any{"name": "Preview Co"}},
		{"name": "subscriptions_delete", "arguments": map[string]any{"subscriptionId": id}},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
	}
}

// Class credential-persisted-in-files.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, false).store(t)
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, args := range battery(e, deliverFile, bbStored) {
		e.run(t, args...)
	}
	e.mcpCalls(t, mcpBattery())
	// Controls without a credential: every answer above echoes the held token,
	// so the kit withheld each of its store, export and delivery writes whole.
	// The same sinks write what the quiet fake answers, into the same home, so
	// the scan below is not vacuous.
	echoEnv := e.env
	e.useServer(quietServer(t))
	e.run(t, "sync", "--json")
	e.run(t, "export", "companies", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl"))
	e.run(t, "companies", "find", "--json", "--deliver", "file:"+deliverFile)
	e.env = echoEnv
	e.run(t, "feedback", "control note HQ-Feedback")
	e.run(t, "teach", "--query", "find companies control", "--resource", "123", "--resource-type", "companies", "--notes", "rotated control", "--no-validate")
	e.run(t, "profile", "save", "control", "--description", "HQ-Profile control", "--select", "id")
	var scanned int
	var sawCreds, sawStore, sawLearning, sawFeedback, sawFeedbackWithheld, sawProfile bool
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
		if !synccpuStoreFile(path) { // synccpu: the local store is not checked
			assertNoEcho(t, "file "+rel, string(data))
		}
		if strings.Contains(d.Name(), ".db") {
			sawStore = sawStore || bytes.Contains(data, []byte("HQ-Record"))
			sawLearning = sawLearning || bytes.Contains(data, []byte("rotated control"))
		}
		if d.Name() == "feedback.jsonl" {
			sawFeedback = sawFeedback || bytes.Contains(data, []byte("HQ-Feedback"))
			sawFeedbackWithheld = sawFeedbackWithheld || bytes.Contains(data, []byte(kitNotice("file")))
		}
		sawProfile = sawProfile || bytes.Contains(data, []byte("HQ-Profile control"))
		return nil
	})
	if scanned < 5 {
		t.Fatalf("only %d files written; the battery did not run", scanned)
	}
	if !sawCreds {
		t.Errorf("auth set-token wrote no credential store")
	}
	if b, err := os.ReadFile(deliverFile); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
		t.Errorf("%s: want the delivered record (err %v)", filepath.Base(deliverFile), err)
	}
	if b, err := os.ReadFile(filepath.Join(e.home, "export.jsonl")); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
		t.Errorf("export.jsonl: want the exported record (err %v)", err)
	}
	for what, ok := range map[string]bool{
		"no local store holds a fetched record":                                    sawStore,
		"no local store holds the taught learning":                                 sawLearning,
		"no feedback.jsonl holds the control entry":                                sawFeedback,
		"no feedback.jsonl holds a withheld entry; the credential never got there": sawFeedbackWithheld,
		"no profile file holds the control profile":                                sawProfile,
	} {
		if !ok {
			t.Error(what)
		}
	}
}

// Class secret-in-output.
func TestNoCredentialInAnyOutput(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true).store(t)
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, args := range battery(e, deliverFile, bbToken) {
		assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
	}
	// Every answer here echoes the held token, so the kit withholds it whole
	// and says so: the scans above were not vacuous.
	if out := e.run(t, "companies", "find", "--json"); !strings.Contains(out, kitNotice("stdout")) {
		t.Fatalf("want the kit's stdout notice for the echoed token, got:\n%.1500s", out)
	}
	out := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
		t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP session", out)
	if !strings.Contains(out, kitNotice("mcp")) {
		t.Fatalf("the MCP session withheld nothing; the echoes did not reach it:\n%.2000s", out)
	}
	// The same reads print their data from the quiet fake, as valid JSON.
	e.useServer(quietServer(t))
	cmd := exec.Command(bbCLI, "companies", "find", "--json")
	cmd.Env, cmd.Dir = e.env, e.home
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("companies find --json: %v", err)
	}
	var doc any
	if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("HQ-Record")) {
		t.Fatalf("want valid JSON with the record, got:\n%.1500s", stdout)
	}
	if out := e.mcpCalls(t, mcpBattery()); !strings.Contains(out, "HQ-Record") {
		t.Fatalf("MCP results lost the record itself:\n%.2000s", out)
	}
}

// The client pair is bound to Pax8's token manager (Declarations.Origins), so a
// token URL naming anything else is refused before anything is sent: an
// environment or a file cannot move the secret to another host.
func TestClientPairNeverLeavesForAnotherTokenHost(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	out := e.run(t, "companies", "find", "--json")
	assertNoEcho(t, "output", out)
	if !strings.Contains(out, kitNotice("outbound request")) {
		t.Fatalf("want the kit's outbound notice, got:\n%.1500s", out)
	}
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if sv.mints != 0 {
		t.Fatalf("the fake token host received %d request(s) carrying the client pair", sv.mints)
	}
}

// Class credential-sent-to-another-host: a redirect off the configured API
// host is refused before it is followed, from the CLI, sync and MCP alike.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true).store(t)
	redirect := e.run(t, "invoices", "find-partner-draft-items", "--json")
	e.run(t, "sync", "--resources", "invoices-draft-items", "--json")
	e.mcpCalls(t, []map[string]any{
		{"name": "invoices_find-partner-draft-items", "arguments": map[string]any{}},
	})
	sv.mu.Lock()
	n := sv.offHost
	sv.mu.Unlock()
	if n != 0 {
		t.Fatalf("the other host was contacted %d time(s)", n)
	}
	if !strings.Contains(redirect, "leave the configured Pax8 host") {
		t.Fatalf("redirect: want the pin refusal, got:\n%.1500s", redirect)
	}
}

// mcp-endpoint-write-confirm and mcp-mirror-write-confirm: every request the
// MCP server and its mirrored CLI children send to the API is a GET while no
// call sets confirm, though the battery asks for a company to be created and
// a subscription to be cancelled.
func TestMCPSendsOnlyReadsWithoutConfirm(t *testing.T) {
	sv := quietServer(t)
	e := newEnv(t, sv, true).store(t)
	out := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(out, "HQ-Record") {
		t.Fatalf("the MCP battery fetched nothing:\n%.2000s", out)
	}
	if strings.Count(out, "confirmation_required") < 2 {
		t.Fatalf("the two write calls did not return previews:\n%.3000s", out)
	}
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if len(sv.methods) == 0 {
		t.Fatal("no request reached the fake")
	}
	for _, m := range sv.methods {
		if !strings.HasPrefix(m, "GET ") {
			t.Errorf("the MCP server sent %s without confirm", m)
		}
	}
}

// Control for the test above: with confirm set to boolean true the same write
// reaches the API, so the gate previews rather than drops; the string "true"
// still previews.
func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	sv := quietServer(t)
	e := newEnv(t, sv, true).store(t)
	const id = "5c802b19-3600-83f1-1767-7b9edc7f38ab"
	e.mcpPhases(t, []map[string]any{{"name": "subscriptions_delete", "arguments": map[string]any{"subscriptionId": id, "confirm": "true"}}})
	sv.mu.Lock()
	for _, m := range sv.methods {
		if !strings.HasPrefix(m, "GET ") {
			t.Errorf("confirm \"true\" (a string) sent %s", m)
		}
	}
	sv.mu.Unlock()
	e.mcpPhases(t, []map[string]any{{"name": "subscriptions_delete", "arguments": map[string]any{"subscriptionId": id, "confirm": true}}})
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var sent bool
	for _, m := range sv.methods {
		sent = sent || m == "DELETE /v1/subscriptions/"+id
	}
	if !sent {
		t.Fatalf("confirm true did not send the DELETE; requests: %v", sv.methods)
	}
}

// dry-run-sends-nothing: a --dry-run write neither mints a token nor reaches
// the API, from a cold start with client credentials in the environment.
func TestDryRunSendsNothing(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	const id = "5c802b19-3600-83f1-1767-7b9edc7f38ab"
	e.run(t, "subscriptions", "delete", id, "--dry-run")
	e.run(t, "companies", "create-company", "--name", "Dry Co", "--dry-run")
	e.run(t, "companies", "update-company", id, "--name", "Dry Co", "--dry-run")
	e.run(t, "auth", "login", "--dry-run")
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if sv.mints != 0 || len(sv.methods) != 0 {
		t.Fatalf("--dry-run sent %d token request(s) and %v", sv.mints, sv.methods)
	}
}
