// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package salesbuildrblackbox runs the built CLI and MCP binaries against a
// Salesbuildr fake that echoes every credential it knows back in every
// encoding it can, whole and split across lines and adjacent values, and
// checks what a reviewer cannot read off the code (salesbuildr handfixes
// credkit-wiring, no-response-cache, config-file-holds-no-credential,
// api-origin-pinned, dry-run-sends-nothing, mcp-endpoint-write-confirm). A
// unit that repeats a held credential is withheld whole by the shared
// credential kit, with its notice (kitNotice); the public-data controls read a
// quiet fake that echoes nothing (quietServer):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors,
//     on success and on error) holds a credential: the api-key value from
//     SALESBUILDR_API_KEY or from the credential store (auth set-token), a
//     vendor-issued integration-key header value (--integration-key), the
//     MCP HTTP token, a credential-named flag value, and the password members
//     a response carries (a declared response field; the undeclared
//     credential-named members are tracked, not failed: outOfContract);
//   - no file the program writes, except the 0600 credential store, holds
//     one either (the local store, receipts, exports, delivered files,
//     feedback, profiles, the learn journal);
//   - no request (and so no credential) leaves for a host other than the
//     configured one by redirect, from a CLI read or write, sync, or MCP;
//   - the MCP server sends a vendor write only when the call sets confirm to
//     the boolean true, and --dry-run sends nothing at all.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder.
package salesbuildrblackbox

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
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// The credential fixtures are spelled from letters no program text uses
// together (k, m, q, w, z), so a piece of one never matches an ordinary
// word, flag name or URL the program prints: the kit withholds any unit that
// repeats a piece of a held value.
const (
	bbToken     = "bbtok-kkwwzwmzqzzkkwwmwzmmqzmwwqmzkzzqq" // SALESBUILDR_API_KEY (the api-key header value itself)
	bbStored    = "bbsto-qmkwqkmqwzwkzwqqkzzmkkwq"          // auth set-token (stdin), the credential store
	bbIntegKey  = "bbint-zmqkwwqkzmzqmwkkqzwm"              // --integration-key: a vendor-issued request header value
	bbArgvSec   = "-bbarg-wkqqmqwqkwmqq"                    // --client-secret on the command line (starts with "-")
	bbShortResp = "Sh0rtPw"                                 // a 7-character credential a response carries
	bbBOMResp   = "bbbom-mzqzwwmwzkqz"                      // a credential in a BOM-prefixed response
	bbGzipResp  = "bbgzp-zkkmkwmwmmqqkkzz"                  // an accessToken member of a gzip-encoded response (not a declared field)
	bbBearerRsp = "bbbrr-wqmzkqwzmkqwzmkq"                  // a value a response carries only as "Bearer <it>" under a plain name (not a declared field)
	bbHTTPToken = "bbmht-mkzkzzkzzwmzmwkwzzmkqkkmzkkww"     // SALESBUILDR_MCP_HTTP_TOKEN
	bbCompany   = "cmp-1042"                                // a company id
	bbProduct   = "prd-7"                                   // a product id
	bbOpp       = "opp-55"                                  // an opportunity id
	bbExt       = "PSA-1042"                                // an external identifier
	bbDenied    = "denied-401"                              // an id the fake answers 401
	bbMoved     = "moved-307"                               // an id the fake redirects to another host
	bbPrefix    = "/public-api"                             // the API base path
)

// secrets are every credential the suite gives the process that the
// credential leak contract holds (section 1: the declared secret environment,
// the credential store, the declared --integration-key flag, the declared
// password response field) and the command-line value. Each one in any
// output, file or request off its origin fails the test.
func secrets() []string {
	return []string{bbToken, bbStored, bbIntegKey, bbArgvSec, bbShortResp, bbBOMResp, bbHTTPToken}
}

// outOfContract are the credential-named members the fake's answers carry that
// the connector does not declare (an accessToken, a "Bearer <it>" under a
// plain name). The contract holds a response field only when the connector
// declares it (section 1), and Salesbuildr's documented responses (spec.json)
// carry no credential member; the declared names are password, otp, pin,
// passcode and the integration key. The fake still answers them, and an
// output that carries one is counted (outOfContractSeen, reported by
// TestMain), never failed: the old connector-side layer withheld them by
// name, the kit does not (handfixes.json credkit-wiring).
func outOfContract() []string {
	return []string{bbGzipResp, bbBearerRsp}
}

// kitNotice is the fixed notice the kit writes in place of a unit it
// withheld from sink (credkit.Notice).
func kitNotice(sink string) string {
	return "[credkit: withheld from " + sink + ": it carried a credential this program holds]"
}

// outOfContractSeen counts the checked outputs that carried an outOfContract value.
var outOfContractSeen struct {
	sync.Mutex
	n int
}

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "salesbuildr-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "salesbuildr-cli"+exe)
	bbMCP = filepath.Join(dir, "salesbuildr-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/salesbuildr-cli", bbMCP: "../../cmd/salesbuildr-mcp"} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	fmt.Fprintf(os.Stderr, "salesbuildrblackbox: %d checked output(s) carried an out-of-contract value (outOfContract; tracked, not a leak)\n", outOfContractSeen.n)
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
	mu      sync.Mutex
	offHost int
	methods []string       // API requests, "METHOD /path"
	headers map[string]int // api-key header values the API received
	integ   map[string]int // integration-key header values the API received
	// redirectList sends the company list (a syncable resource) to the other
	// host too, so sync and the MCP list read meet a redirect.
	redirectList bool
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

// salesbuildrRecord is one record every resource answers with, shaped so the
// store and the novel commands (quote stale/thin/funnel, pricing drift,
// opportunity velocity/winrate/mrr-forecast, product velocity, company
// whitespace, reconcile-psa) have something real to read: a sent quote with
// a thin line item, an open opportunity, a product with no external
// identifier.
const salesbuildrRecord = `"id":"cmp-1042","name":"HQ-Record","number":"Q-1042","title":"HQ quote","status":"sent","statusId":"open",` +
	`"company":{"id":"cmp-1042","name":"HQ-Record"},"externalIdentifier":"","price":100,"cost":95,"markup":5,"msrp":120,` +
	`"probability":0.5,"monthlyRevenue":200,"monthlyProfit":40,"pipelineStage":{"id":"st-1","name":"Proposal"},` +
	`"items":[{"id":"li-1","name":"HQ-Line","product":{"id":"prd-7","name":"HQ-Line"},"price":100,"cost":95,"markup":5,"quantity":2}],` +
	`"products":[{"id":"prd-7","price":90,"cost":95}],` +
	`"createdAt":"2026-01-10T10:00:00Z","sentAt":"2026-01-12T10:00:00Z","updatedAt":"2026-01-12T10:00:00Z"`

// resultsLists answer {"results":[...],"total":N}; bareLists answer a bare
// array (the Salesbuildr public API's two list shapes).
var (
	resultsLists = map[string]bool{"/company": true, "/opportunity": true, "/product": true, "/quote": true, "/quote/approved": true}
	bareLists    = map[string]bool{"/contact": true, "/pricing-book": true, "/quote-template": true, "/quote-widget-template": true,
		"/quote-discount-group": true, "/category/tree": true}
)

func echoServer(t *testing.T) *server {
	t.Helper()
	return fakeServer(t, false)
}

// quietServer is the fake answering as Salesbuildr does: the same records,
// repeating nothing a request carried. The kit withholds a unit whole
// (credential leak contract section 4, no partial redaction), so an answer
// that echoes the held key never prints or persists any of its public data;
// the quiet fake is what proves a sink really writes.
func quietServer(t *testing.T) *server {
	t.Helper()
	return fakeServer(t, true)
}

func fakeServer(t *testing.T, quiet bool) *server {
	t.Helper()
	sv := &server{headers: map[string]int{}, integ: map[string]int{}}
	// A second host the API redirects to: no request may ever reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte("[]"))
	}))
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		sent := sentSecrets(r)
		if quiet {
			sent = nil
		}
		echo := echoObject(sent)
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		if h := r.Header.Get("api-key"); h != "" {
			sv.headers[h]++
		}
		if h := r.Header.Get("integration-key"); h != "" {
			sv.integ[h]++
		}
		redirectList := sv.redirectList
		sv.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		p := strings.TrimPrefix(r.URL.Path, bbPrefix)
		get := r.Method == http.MethodGet
		switch {
		case p == "/contract/items" || strings.Contains(p, "/"+bbDenied):
			// Rejected: the error repeats what it was sent.
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"statusCode":401,"error":"invalid api-key %s","message":"bad api-key %s","echo":%s}`, strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case strings.Contains(p, "/"+bbMoved) || (redirectList && get && p == "/company"):
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case get && strings.HasPrefix(p, "/product/") && !strings.HasPrefix(p, "/product/ext/"):
			// product get by id: the echo split across values, lines and spaces.
			fmt.Fprintf(w, `{%s,"split":%s}`, salesbuildrRecord, splitObject(sent))
		case get && p == "/category":
			// categories: BOM-prefixed, a bare array, a credential under a credential name.
			fmt.Fprint(w, "\xef\xbb\xbf"+fmt.Sprintf(`[{"id":"cat-1","name":"HQ-Category","parentId":null,"note":%q,"password":%q}]`, bbBOMResp, bbBOMResp))
		case get && strings.HasPrefix(p, "/opportunity/"):
			// opportunity get: a short credential, and one recognisable only by
			// its shape (an auth scheme value under a plain name).
			fmt.Fprintf(w, `{%s,"note":%q,"password":%q,"comment":%q}`, salesbuildrRecord, bbShortResp, bbShortResp, "Bearer "+bbBearerRsp)
		case get && resultsLists[p]:
			// company/opportunity/product/quote lists (list, sync, export):
			// gzip-encoded, with an access token, the echo, and a record whose
			// identifier repeats the credential it was sent.
			extra := ""
			if len(sent) > 0 {
				extra = fmt.Sprintf(`,{"id":%q,"name":"Echo-ID","externalIdentifier":""}`, sent[0])
			}
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			fmt.Fprintf(zw, `{"results":[{%s,"accessToken":%q,"echo":%s}%s],"total":%d}`, salesbuildrRecord, bbGzipResp, echoObject(append(sent, bbGzipResp)), extra, 1+strings.Count(extra, "Echo-ID"))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		case get && bareLists[p]:
			fmt.Fprintf(w, `[{%s,"echo":%s}]`, salesbuildrRecord, echo)
		default:
			// Every write and every other read echoes what it was sent.
			fmt.Fprintf(w, `{%s,"echo":%s}`, salesbuildrRecord, echo)
		}
	}))
	sv.url = srv.URL + bbPrefix
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.caFile = ca
	return sv
}

// sawHeader reports how many API requests carried api-key: v, and the
// distinct header values the fake received.
func (sv *server) sawHeader(v string) (int, []string) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var all []string
	for h := range sv.headers {
		all = append(all, h)
	}
	return sv.headers[v], all
}

// sawIntegrationKey reports how many API requests carried integration-key: v.
func (sv *server) sawIntegrationKey(v string) int {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return sv.integ[v]
}

// writes is every request the fake received that was not a GET.
func (sv *server) writes() []string {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var out []string
	for _, m := range sv.methods {
		if !strings.HasPrefix(m, "GET ") {
			out = append(out, m)
		}
	}
	return out
}

type bbEnv struct {
	home string
	env  []string
}

// credMode is where the process gets its api-key.
type credMode int

const (
	credStored   credMode = iota // none in the environment: the program runs on a token stored with auth set-token
	credEnvToken                 // SALESBUILDR_API_KEY
)

func (m credMode) String() string {
	if m == credEnvToken {
		return "env token"
	}
	return "stored token"
}

// newEnv builds a clean environment against the fake with the credentials
// mode names. credStored stores bbStored with auth set-token first.
func newEnv(t *testing.T, sv *server, mode credMode) *bbEnv {
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
		"SALESBUILDR_BASE_URL=" + sv.url, "SALESBUILDR_CA_CERT=" + sv.caFile,
		"SALESBUILDR_CLI_PATH=" + bbCLI, "SALESBUILDR_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}
	e := &bbEnv{home: home, env: append(env, windowsEnv()...)}
	switch mode {
	case credEnvToken:
		e.env = append(e.env, "SALESBUILDR_API_KEY="+bbToken)
	case credStored:
		assertNoEcho(t, "auth set-token output", e.runIn(t, bbStored+"\n", "auth", "set-token"))
	}
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
	return e.mcpPhases(t, calls)
}

// mcpPhases runs one MCP stdio session, sending each phase of tools/call
// requests only after every earlier call was answered (mcp-go serves the calls
// of one phase concurrently), and returns everything the server wrote. The
// calls are numbered from 1 in the order given.
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
	deadline := time.Now().Add(180 * time.Second)
	for _, calls := range phases {
		for _, c := range calls {
			id++
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": c})
		}
		for time.Now().Before(deadline) && len(mcpReplies(out.String())) < id+1 {
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

// mcpReplies maps each JSON-RPC reply an MCP session wrote to its request id.
func mcpReplies(out string) map[int]string {
	replies := map[int]string{}
	for _, line := range strings.Split(out, "\n") {
		var msg struct {
			ID *int `json:"id"`
		}
		if json.Unmarshal([]byte(line), &msg) == nil && msg.ID != nil {
			replies[*msg.ID] = line
		}
	}
	return replies
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
	for _, s := range outOfContract() {
		if strings.Contains(text, s) {
			outOfContractSeen.Lock()
			outOfContractSeen.n++
			outOfContractSeen.Unlock()
			break
		}
	}
}

// writeCmd is one CLI write command: its words, the positional arguments it
// takes, and whether it reads a JSON body from --stdin.
type writeCmd struct {
	words []string
	args  []string
	stdin bool
}

// cliWrites is every Salesbuildr write command the CLI has (every POST, PUT
// and DELETE endpoint; TestWriteListCoversEveryEndpoint keeps it complete).
func cliWrites() []writeCmd {
	id, ext := []string{bbCompany}, []string{bbExt}
	return []writeCmd{
		{[]string{"company", "public-create"}, nil, true},
		{[]string{"company", "public-update"}, id, true},
		{[]string{"company", "public-upsert"}, ext, true},
		{[]string{"company", "public-delete"}, id, false},
		{[]string{"company", "public-delete-by-external-identifier"}, ext, false},
		{[]string{"contact", "company-public-create"}, nil, true},
		{[]string{"contact", "company-public-update"}, id, true},
		{[]string{"contact", "company-public-upsert"}, ext, true},
		{[]string{"contact", "company-public-upsert-by-email"}, nil, true},
		{[]string{"contact", "company-public-undelete-by-ext-id"}, ext, true},
		{[]string{"contact", "company-public-delete"}, ext, false},
		{[]string{"contact", "company-public-delete-by-id"}, id, false},
		{[]string{"contact", "undelete", "company-contact-public-contact-by-id"}, id, true},
		{[]string{"field", "public-update-values"}, []string{"industry"}, true},
		{[]string{"opportunity", "public-upsert"}, ext, true},
		{[]string{"opportunity", "public-win"}, ext, true},
		{[]string{"opportunity", "public-lose"}, ext, true},
		{[]string{"pricing-book", "public-create"}, nil, true},
		{[]string{"pricing-book", "public-update"}, id, true},
		{[]string{"product", "public-create"}, nil, true},
		{[]string{"product", "public-create-batch"}, nil, true},
		{[]string{"product", "public-update"}, []string{bbProduct}, true},
		{[]string{"product", "public-upsert-by-external-identifier"}, ext, true},
		{[]string{"product", "public-delete"}, []string{bbProduct}, false},
		{[]string{"product", "public-delete-by-external-identifier"}, ext, false},
		{[]string{"product", "inventory", "product-public-put-inventories"}, []string{bbProduct}, true},
		{[]string{"quote", "public-create-draft"}, nil, true},
		{[]string{"quote", "public-create-approved"}, nil, true},
		{[]string{"quote-discount-group", "discount-group-public-create"}, nil, true},
		{[]string{"quote-discount-group", "discount-group-public-update"}, id, true},
		{[]string{"quote-discount-group", "companies", "discount-group-public-update-company-discount-group"}, id, true},
		{[]string{"quote-template", "public-create"}, nil, true},
		{[]string{"quote-template", "public-update"}, id, true},
		{[]string{"quote-template", "public-validate-template"}, nil, true},
		{[]string{"quote-widget-template", "public-create"}, nil, true},
		{[]string{"quote-widget-template", "public-update"}, id, true},
		{[]string{"quote-widget-template", "public-validate-widget"}, nil, true},
		{[]string{"sync-state"}, id, false},
	}
}

// argv is the command line for w with extra flags; its stdin body (when w
// reads one) is body.
func (w writeCmd) argv(extra ...string) []string {
	out := append(append(append([]string{}, w.words...), w.args...), extra...)
	if w.stdin {
		out = append(out, "--stdin")
	} else if w.words[0] == "sync-state" {
		out = append(out, "--state", "synced", "--error", "none", "--external-identifier", bbExt)
	}
	return out
}

// bbRun is one CLI run: its stdin, then its command line.
type bbRun struct {
	stdin string
	args  []string
}

// battery runs every output path; tok is a credential the process holds
// (SALESBUILDR_API_KEY or the stored token). Each entry is the stdin, then
// the command line.
func battery(t *testing.T, e *bbEnv, deliverFile, tok string) []bbRun {
	t.Helper()
	// Test inputs live outside HOME, so the file scan reads only what the
	// program wrote.
	importFile := filepath.Join(t.TempDir(), "import.jsonl")
	if err := os.WriteFile(importFile, []byte(`{"name":"Echo `+tok+`","number":"1","type":"customer"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := func(args ...string) bbRun { return bbRun{"", args} }
	return []bbRun{
		// Every output mode.
		a("company", "list", "--json"),
		a("company", "list"),
		a("company", "list", "--csv"),
		a("company", "list", "--plain"),
		a("company", "list", "--json", "--receipt"),
		a("company", "list", "--json", "--audit-dir", filepath.Join(e.home, "audit")),
		a("company", "list", "--json", "--deliver", "file:"+deliverFile),
		a("quote", "list", "--json", "--select", "id,name,echo"),
		// Every response shape: BOM-prefixed, short credential, split echo,
		// 401 echo, refused redirect, an echoed integration-key header.
		a("category", "list", "--json"),
		a("opportunity", "get", bbOpp, "--json"),
		a("product", "public-get-id", bbProduct, "--json"),
		a("contract", "--json"),
		a("contract"),
		a("company", "get", bbMoved, "--json"),
		a("company", "get", bbCompany, "--integration-key", bbIntegKey, "--json"),
		a("product", "list", "--integration-key", bbIntegKey),
		// Writes the fake echoes, one it rejects, one carrying the integration key.
		a("company", "update", bbCompany, "--name", "HQ", "--number", "1042", "--type", "customer", "--json"),
		a("opportunity", "public-win", bbExt, "--json"),
		a("product", "delete", bbProduct, "--json"),
		a("company", "delete", bbDenied, "--json"),
		a("company", "delete", bbDenied),
		{`{"firstName":"Pat","lastName":"Lee","email":"pat@example.com"}`, []string{"contact", "create", "--stdin", "--integration-key", bbIntegKey, "--json"}},
		{`{"name":"HQ","number":"1042","type":"customer"}`, []string{"company", "public-upsert", bbExt, "--stdin", "--json"}},
		{"", []string{"import", "company", "--input", importFile, "--json"}},
		// The local store, exports and the novel commands.
		a("sync", "--json"),
		a("export", "product", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")),
		a("export", "quote", "--format", "json"),
		a("export", "opportunity", bbOpp, "--format", "json"),
		a("quote", "stale", "--days", "1", "--json"),
		a("quote", "thin", "--json"),
		a("quote", "funnel", "--json"),
		a("pricing", "drift", "--json"),
		a("opportunity", "velocity", "--json"),
		a("opportunity", "winrate", "--json"),
		a("opportunity", "mrr-forecast", "--json"),
		a("product", "velocity", "--json"),
		a("company", "whitespace", bbCompany, "--json"),
		a("reconcile-psa", "--json"),
		a("sql", "SELECT data FROM resources"),
		a("sql", "SELECT * FROM company", "--json"),
		a("search", "HQ", "--json"),
		a("search", "Echo", "--json", "--data-source", "local"),
		a("company", "list", "--json", "--data-source", "local"),
		a("workflow", "status", "--json"),
		// A credential on the command line: a credential-named flag, an unknown
		// flag, a positional, feedback text, a profile description.
		a("company", "list", "--client-secret", bbArgvSec),
		a("company", "list", "--json", "--"+tok),
		a("company", "get", tok, "--json"),
		a("feedback", "noted the token "+tok+" in a ticket"),
		{"pasted " + tok + "\n", []string{"feedback", "--stdin"}},
		a("profile", "save", "hq", "--json", "--description", "uses "+tok),
		a("profile", "show", "hq"),
		// Previews: a dry run sends nothing.
		a("company", "update", bbCompany, "--name", "Echo "+tok, "--number", "1", "--type", "customer", "--dry-run"),
		{`{"name":"Echo ` + tok + `","categoryId":"cat-1"}`, []string{"product", "create", "--stdin", "--integration-key", bbIntegKey, "--dry-run"}},
		a("quote", "public-create-draft", "--title", "Echo "+tok, "--opportunity-id", bbOpp, "--dry-run"),
		a("sync-state", bbCompany, "--state", "error", "--error", "Echo "+tok, "--external-identifier", bbExt, "--integration-key", bbIntegKey, "--dry-run"),
		{"", []string{"import", "company", "--input", importFile, "--dry-run"}},
		// Learnings, playbooks and learn events land in the local store's own tables.
		a("teach", "--query", "find company "+tok, "--resource", bbCompany, "--resource-type", "company",
			"--notes", "rotated "+tok, "--playbook-notes", "use "+tok, "--no-validate"),
		a("recall", "find company "+tok, "--json"),
		a("doctor"),
		a("auth", "status"),
	}
}

// mcpReads are the read tools across every response shape the fake has, and
// the novel mirrors over the store.
func mcpReads() []map[string]any {
	none := map[string]any{}
	read := func(id string, params map[string]any) map[string]any {
		args := map[string]any{"endpoint_id": id}
		if params != nil {
			args["params"] = params
		}
		return map[string]any{"name": "salesbuildr_read", "arguments": args}
	}
	return []map[string]any{
		read("company.public-get-list", nil),
		read("category.public-get-all-categories", nil),
		read("opportunity.public-get-by-id", map[string]any{"id": bbOpp}),
		read("product.public-get-id", map[string]any{"id": bbProduct}),
		read("contract.public-get-items", nil),
		read("company.public-get", map[string]any{"id": bbCompany, "integration-key": bbIntegKey}),
		read("quote.public-get", nil),
		{"name": "salesbuildr_search", "arguments": map[string]any{"query": "update company"}},
		{"name": "salesbuildr_get", "arguments": map[string]any{"endpoint_id": "company.public-get"}},
		{"name": "sync", "arguments": map[string]any{"resources": "company,product,quote,opportunity"}},
		{"name": "quote_stale", "arguments": map[string]any{"days": 1}},
		{"name": "quote_thin", "arguments": none},
		{"name": "quote_funnel", "arguments": none},
		{"name": "pricing_drift", "arguments": none},
		{"name": "opportunity_velocity", "arguments": none},
		{"name": "opportunity_winrate", "arguments": none},
		{"name": "opportunity_mrr_forecast", "arguments": none},
		{"name": "product_velocity", "arguments": none},
		{"name": "company_whitespace", "arguments": map[string]any{"company": bbCompany}},
		{"name": "reconcile_psa", "arguments": none},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
		{"name": "search", "arguments": map[string]any{"query": "HQ"}},
		{"name": "export", "arguments": map[string]any{"resource": "product", "format": "json"}},
		{"name": "workflow_status", "arguments": none},
		{"name": "context", "arguments": none},
	}
}

// executeParams fills every path placeholder a Salesbuildr endpoint has, a
// body field, and the integration-key header (a preview must not print it).
func executeParams() map[string]any {
	return map[string]any{"id": bbCompany, "externalIdentifier": bbExt, "field": "industry", "entityType": "quote", "name": "Preview Co",
		"integration-key": bbIntegKey}
}

// mcpEndpointWrites is every state-changing endpoint salesbuildr_execute
// reaches (TestWriteListCoversEveryEndpoint keeps it complete).
func mcpEndpointWrites() []string {
	return []string{
		"company.public-create", "company.public-delete", "company.public-delete-by-external-identifier",
		"company.public-update", "company.public-upsert",
		"contact.company-public-create", "contact.company-public-delete", "contact.company-public-delete-by-id",
		"contact.company-public-undelete-by-ext-id", "contact.company-public-update", "contact.company-public-upsert",
		"contact.company-public-upsert-by-email", "contact.undelete.company-contact-public-contact-by-id",
		"field.public-update-values",
		"opportunity.public-lose", "opportunity.public-upsert", "opportunity.public-win",
		"pricing-book.public-create", "pricing-book.public-update",
		"product.public-create", "product.public-create-batch", "product.public-delete",
		"product.public-delete-by-external-identifier", "product.public-update",
		"product.public-upsert-by-external-identifier", "product.inventory.product-public-put-inventories",
		"quote.public-create-approved", "quote.public-create-draft",
		"quote-discount-group.discount-group-public-create", "quote-discount-group.discount-group-public-update",
		"quote-discount-group.companies.discount-group-public-update-company-discount-group",
		"quote-template.public-create", "quote-template.public-update", "quote-template.public-validate-template",
		"quote-widget-template.public-create", "quote-widget-template.public-update", "quote-widget-template.public-validate-widget",
		"sync-state.sync-public-put",
	}
}

// mcpWrites is every write tool, called without a boolean-true confirm: the
// vendor writes through salesbuildr_execute (confirm absent, false, the
// string "true"), the mirrored import, and the two recipe intents. Each must
// answer with a preview and send nothing. The endpoint and recipe tools
// preview as a confirmation_required object; the mirrored import previews as
// the command's --dry-run plan followed by the gate's note.
func mcpWrites() []map[string]any {
	var out []map[string]any
	for i, id := range mcpEndpointWrites() {
		args := map[string]any{"endpoint_id": id, "params": executeParams()}
		switch i % 4 {
		case 1:
			args["confirm"] = false
		case 2:
			args["confirm"] = "true"
		case 3:
			args["confirm"] = 1
		}
		out = append(out, map[string]any{"name": "salesbuildr_execute", "arguments": args})
	}
	return append(out,
		map[string]any{"name": "import", "arguments": map[string]any{"resource": "company"}},
		map[string]any{"name": "import", "arguments": map[string]any{"resource": "company", "confirm": "true"}},
		map[string]any{"name": "account_review_cross_sell", "arguments": map[string]any{"value": bbCompany}},
		map[string]any{"name": "catalog_margin_audit_in_sql", "arguments": map[string]any{"value": "SELECT 1", "confirm": "true"}},
	)
}

func mcpBattery() []map[string]any {
	return append(mcpReads(), mcpWrites()...)
}

// Class credential-persisted-in-files: every file under HOME (and the -o and
// --deliver targets, which sit there) is scanned after the whole CLI and MCP
// battery ran on a stored token.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, r := range battery(t, e, deliverFile, bbStored) {
		e.runIn(t, r.stdin, r.args...)
	}
	e.mcpCalls(t, mcpBattery())
	// Controls without a credential: the same sinks write, so the scans below
	// are not vacuous. Every battery answer echoes the held key, so the kit
	// withheld each of its store, delivery, export and profile writes whole
	// (credential leak contract section 4, no partial redaction); the data
	// controls read the quiet fake instead, into the same home.
	quiet := quietServer(t)
	echoEnv := e.env
	e.env = append(withoutEnv(withoutEnv(e.env, "SALESBUILDR_BASE_URL"), "SALESBUILDR_CA_CERT"),
		"SALESBUILDR_BASE_URL="+quiet.url, "SALESBUILDR_CA_CERT="+quiet.caFile)
	e.run(t, "sync", "--json")
	e.run(t, "company", "list", "--json", "--deliver", "file:"+deliverFile)
	e.run(t, "export", "product", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl"))
	e.env = echoEnv
	e.run(t, "profile", "save", "hq", "--json", "--description", "control profile")
	e.run(t, "feedback", "control feedback note")
	e.run(t, "teach", "--query", "find company control", "--resource", bbCompany, "--resource-type", "company",
		"--notes", "rotated control", "--no-validate")
	if n, all := sv.sawHeader(bbStored); n == 0 {
		t.Fatalf("no request carried the stored token as api-key (saw %d distinct values); the battery ran unauthenticated", len(all))
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
		if os.Getenv("BB_DEBUG") != "" {
			t.Logf("FILE %s (%d bytes)", rel, len(data))
		}
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
	if scanned < 8 {
		t.Fatalf("only %d files written; the battery did not run", scanned)
	}
	if !sawCreds {
		t.Errorf("auth set-token wrote no credential store")
	}
	// The battery really wrote data, so the scan above is not vacuous.
	for _, f := range []string{deliverFile, filepath.Join(e.home, "export.jsonl")} {
		if b, err := os.ReadFile(f); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
			t.Errorf("%s: want a fetched record (err %v):\n%.800s", filepath.Base(f), err, b)
		}
	}
	var sawStore, sawLearning, sawFeedback, sawProfile, sawJournal bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		rel := filepath.ToSlash(path)
		switch {
		case strings.HasSuffix(d.Name(), ".db"):
			sawStore = sawStore || bytes.Contains(b, []byte("HQ-Record"))
			// The teach run's learning row was written, its secret withheld in place.
			sawLearning = sawLearning || bytes.Contains(b, []byte("rotated"))
		case strings.Contains(rel, "feedback"):
			sawFeedback = sawFeedback || len(b) > 0
		case strings.Contains(rel, "profile"):
			sawProfile = sawProfile || bytes.Contains(b, []byte("hq"))
		case strings.Contains(rel, "/learn/journal-"):
			sawJournal = sawJournal || len(b) > 0
		}
		return nil
	})
	for what, saw := range map[string]bool{
		"a local store holding a fetched record": sawStore,
		"the taught learning in the local store": sawLearning,
		"a feedback entry":                       sawFeedback,
		"the saved profile":                      sawProfile,
		"a learn journal entry":                  sawJournal,
	} {
		if !saw {
			t.Errorf("no file holds %s; the battery wrote less than it should, so the scan proves less", what)
		}
	}
}

// Class secret-in-output, once with SALESBUILDR_API_KEY and once with the
// token stored by auth set-token (each sent as the api-key header, which the
// fake then echoes).
func TestNoCredentialInAnyOutput(t *testing.T) {
	for _, c := range []struct {
		mode credMode
		tok  string // the credential the process holds, typed on the command line too
	}{
		{credEnvToken, bbToken},
		{credStored, bbStored},
	} {
		t.Run(c.mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, c.mode)
			deliverFile := filepath.Join(e.home, "deliver.json")
			for _, r := range battery(t, e, deliverFile, c.tok) {
				out := e.runIn(t, r.stdin, r.args...)
				// BB_DEBUG=1 logs every run's output (diagnosing a battery entry
				// that stops at a flag error and so proves nothing).
				if os.Getenv("BB_DEBUG") != "" {
					t.Logf("RUN %v\n%.1200s", r.args, out)
				}
				assertNoEcho(t, fmt.Sprintf("output of %v", r.args), out)
			}
			// Every answer here echoes the held key, so the kit withholds what
			// carries it whole (credential leak contract section 4: no partial
			// redaction) and says so with its notice: the scans above were not
			// vacuous.
			if out := e.run(t, "company", "list", "--json"); !strings.Contains(out, kitNotice("stdout")) {
				t.Fatalf("company list --json: want the kit's stdout notice for the echoed key, got:\n%.1500s", out)
			}
			// The same read prints its data, as valid JSON, from the quiet fake.
			quiet := quietServer(t)
			qe := &bbEnv{home: e.home, env: append(withoutEnv(withoutEnv(e.env, "SALESBUILDR_BASE_URL"), "SALESBUILDR_CA_CERT"),
				"SALESBUILDR_BASE_URL="+quiet.url, "SALESBUILDR_CA_CERT="+quiet.caFile)}
			cmd := exec.Command(bbCLI, "company", "list", "--json")
			cmd.Env, cmd.Dir = qe.env, qe.home
			stdout, err := cmd.Output()
			if err != nil {
				t.Fatalf("company list --json (quiet fake): %v", err)
			}
			var doc any
			if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("HQ-Record")) {
				t.Fatalf("quiet fake: want valid JSON with the record, got:\n%.1500s", stdout)
			}
			out := e.mcpCalls(t, mcpBattery())
			if os.Getenv("BB_DEBUG") != "" {
				t.Logf("MCP\n%s", out)
			}
			if got := len(mcpReplies(out)); got < len(mcpBattery())+1 {
				t.Fatalf("MCP session answered %d of %d calls:\n%.2000s", got-1, len(mcpBattery()), out)
			}
			assertNoEcho(t, "MCP session", out)
			if !strings.Contains(out, kitNotice("mcp")) {
				t.Fatalf("the MCP session withheld nothing; the echoes did not reach it:\n%.2000s", out)
			}
			if qout := qe.mcpCalls(t, mcpReads()); !strings.Contains(qout, "HQ-Record") {
				t.Fatalf("quiet fake: MCP results lost the record itself:\n%.2000s", qout)
			}
			// The fake echoed the header it really received, so the scans above
			// covered it; no other credential was sent as the api-key.
			n, all := sv.sawHeader(c.tok)
			if n == 0 || len(all) != 1 {
				t.Fatalf("want every request to carry the %s as api-key, got %d of it and %d distinct values", c.mode, n, len(all))
			}
			// The integration key reached the API (CLI and MCP), so its echo
			// was in what the scans above read.
			if n := sv.sawIntegrationKey(bbIntegKey); n < 3 {
				t.Fatalf("the integration-key header reached the API %d time(s), want the CLI read, list, write and the MCP read", n)
			}
		})
	}
}

// Class credential-sent-to-another-host: a redirect off the configured API
// host is refused before it is followed, from a CLI read and write, sync and
// MCP (read, list, execute with confirm, the sync tool) alike.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvToken)
	outs := map[string]string{
		"CLI read":  e.run(t, "company", "get", bbMoved, "--json"),
		"CLI write": e.run(t, "company", "delete", bbMoved, "--json"),
	}
	sv.mu.Lock()
	sv.redirectList = true
	sv.mu.Unlock()
	outs["sync"] = e.run(t, "sync", "--resources", "company", "--json")
	mcp := e.mcpCalls(t, []map[string]any{
		{"name": "salesbuildr_read", "arguments": map[string]any{"endpoint_id": "company.public-get", "params": map[string]any{"id": bbMoved}}},
		{"name": "salesbuildr_read", "arguments": map[string]any{"endpoint_id": "company.public-get-list"}},
		{"name": "salesbuildr_execute", "arguments": map[string]any{"endpoint_id": "company.public-delete", "params": map[string]any{"id": bbMoved}, "confirm": true}},
		{"name": "sync", "arguments": map[string]any{"resources": "company"}},
	})
	sv.mu.Lock()
	n := sv.offHost
	var redirected []string
	for _, m := range sv.methods {
		if strings.Contains(m, "/"+bbMoved) || m == "GET "+bbPrefix+"/company" {
			redirected = append(redirected, m)
		}
	}
	sv.mu.Unlock()
	if n != 0 {
		t.Fatalf("the other host was contacted %d time(s)", n)
	}
	// CLI read, CLI write, sync, MCP read, MCP list, MCP execute, MCP sync.
	if len(redirected) < 7 {
		t.Fatalf("only %d request(s) met a redirect (%v); the CLI, sync and MCP calls did not all run", len(redirected), redirected)
	}
	var deletes int
	for _, m := range redirected {
		if strings.HasPrefix(m, "DELETE ") {
			deletes++
		}
	}
	if deletes < 2 {
		t.Fatalf("want the CLI and the MCP delete to meet the redirect, got %v", redirected)
	}
	outs["MCP"] = mcp
	for where, out := range outs {
		if !strings.Contains(out, "leave the configured Salesbuildr host") {
			t.Errorf("%s: want the pin refusal, got:\n%.1500s", where, out)
		}
	}
}

// mcp-endpoint-write-confirm and mcp-mirror-write-confirm: every request the
// MCP server and its mirrored CLI children send to the API is a GET while no
// call sets confirm to boolean true, though the battery calls every write
// tool; each write answers with a preview.
func TestMCPSendsOnlyReadsWithoutConfirm(t *testing.T) {
	// The quiet fake: the reads must answer their data, which an answer that
	// echoes the held key never does (withheld whole by contract).
	sv := quietServer(t)
	e := newEnv(t, sv, credEnvToken)
	out := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(out, "HQ-Record") {
		t.Fatalf("the MCP battery fetched nothing:\n%.2000s", out)
	}
	replies := mcpReplies(out)
	for i, call := range mcpWrites() {
		id := len(mcpReads()) + i + 1
		if r := replies[id]; !strings.Contains(r, "confirmation_required") && !strings.Contains(r, "Preview only: nothing was sent") {
			t.Errorf("%s %v without confirm true did not return a preview:\n%.1500s", call["name"], call["arguments"], r)
		}
	}
	sv.mu.Lock()
	reached := len(sv.methods)
	sv.mu.Unlock()
	if reached == 0 {
		t.Fatal("no request reached the fake")
	}
	for _, m := range sv.writes() {
		t.Errorf("the MCP server sent %s without confirm true", m)
	}
}

// Control for the test above: with confirm set to boolean true the same write
// reaches the API, so the gate previews rather than drops; the string "true",
// false, 1 and an absent confirm still preview.
func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvToken)
	update := func(confirm any) []map[string]any {
		args := map[string]any{"endpoint_id": "company.public-update",
			"params": map[string]any{"id": bbCompany, "name": "HQ", "number": "1042", "type": "customer", "integration-key": bbIntegKey}}
		if confirm != nil {
			args["confirm"] = confirm
		}
		return []map[string]any{{"name": "salesbuildr_execute", "arguments": args}}
	}
	for _, c := range []any{nil, false, "true", 1, "yes"} {
		out := e.mcpPhases(t, update(c))
		if !strings.Contains(out, "confirmation_required") {
			t.Errorf("confirm %#v returned no preview:\n%.1500s", c, out)
		}
		assertNoEcho(t, fmt.Sprintf("preview for confirm %#v", c), out)
		if w := sv.writes(); len(w) != 0 {
			t.Fatalf("confirm %#v sent %v", c, w)
		}
	}
	out := e.mcpPhases(t, update(true))
	want := "PUT " + bbPrefix + "/company/" + bbCompany
	if w := sv.writes(); len(w) != 1 || w[0] != want {
		t.Fatalf("confirm true: want exactly %s, got %v:\n%.1500s", want, w, out)
	}
	if sv.sawIntegrationKey(bbIntegKey) != 1 {
		t.Fatalf("the confirmed write did not carry the integration-key header, so its echo was not tested")
	}
	assertNoEcho(t, "confirmed MCP write", out)
}

// dry-run-sends-nothing: every CLI write command (and import) under --dry-run
// prints a preview and never reaches the API, from a cold start with
// SALESBUILDR_API_KEY or a stored token.
func TestDryRunSendsNothing(t *testing.T) {
	for _, mode := range []credMode{credEnvToken, credStored} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			importFile := filepath.Join(t.TempDir(), "import.jsonl")
			if err := os.WriteFile(importFile, []byte(`{"name":"Dry Co","number":"1","type":"customer"}`+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			body := `{"name":"Dry Co","number":"1","type":"customer"}`
			runs := 0
			for _, w := range cliWrites() {
				args := w.argv("--integration-key", bbIntegKey, "--dry-run")
				// Each run reaches its preview, so the count below is not
				// vacuous (a flag error would send nothing either).
				out := e.runIn(t, body, args...)
				if !strings.Contains(out, "dry run") && !strings.Contains(out, "dry_run") {
					t.Errorf("%v printed no preview:\n%.1500s", args, out)
				}
				assertNoEcho(t, fmt.Sprintf("dry run of %v", args), out)
				runs++
			}
			if out := e.run(t, "import", "company", "--input", importFile, "--dry-run"); !strings.Contains(out, "dry run") && !strings.Contains(out, "dry_run") && !strings.Contains(out, "would ") {
				t.Errorf("import --dry-run printed no preview:\n%.1500s", out)
			}
			if runs != 38 {
				t.Fatalf("ran %d write commands, want 38", runs)
			}
			sv.mu.Lock()
			if len(sv.methods) != 0 {
				t.Errorf("--dry-run sent %v", sv.methods)
			}
			sv.mu.Unlock()
			// Control: the same write without --dry-run reaches the fake.
			e.runIn(t, body, "company", "public-update", bbCompany, "--stdin")
			want := "PUT " + bbPrefix + "/company/" + bbCompany
			if w := sv.writes(); len(w) != 1 || w[0] != want {
				t.Errorf("control: want the one %s, got %v", want, w)
			}
		})
	}
}

// TestWriteListCoversEveryEndpoint keeps cliWrites and mcpEndpointWrites
// complete: every POST, PUT, PATCH and DELETE command the CLI declares is in
// cliWrites, and every endpoint salesbuildr_execute gates is in
// mcpEndpointWrites, so an endpoint a reprint adds is dry-run and confirm
// tested too.
func TestWriteListCoversEveryEndpoint(t *testing.T) {
	cli := map[string]bool{}
	for _, w := range cliWrites() {
		cli[strings.Join(w.words, " ")] = true
	}
	src := filepath.Join("..", "cli")
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	writeMethod := regexp.MustCompile(`"pp:method": "(POST|PUT|PATCH|DELETE)"`)
	var found int
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".go") || strings.HasSuffix(ent.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(src, ent.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for range writeMethod.FindAllIndex(b, -1) {
			found++
			// The command path is the file name's words (resource_sub_command).
			words := strings.Split(strings.TrimSuffix(ent.Name(), ".go"), "_")
			if words[0] == "promoted" {
				words = words[1:]
			}
			if !cli[strings.Join(words, " ")] {
				t.Errorf("%s is a write command missing from cliWrites", strings.Join(words, " "))
			}
		}
	}
	if found != len(cliWrites()) || found < 38 {
		t.Errorf("found %d write commands in internal/cli, cliWrites lists %d", found, len(cliWrites()))
	}
	// The execute registry: every endpoint that needs confirm.
	orch, err := os.ReadFile(filepath.Join("..", "mcp", "code_orch.go"))
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, id := range mcpEndpointWrites() {
		listed[id] = true
	}
	gated := regexp.MustCompile(`(?s)ID:\s+"([^"]+)",.*?NeedsConfirm:\s+true`)
	entry := regexp.MustCompile(`(?s)\{\s*ID:\s+"[^"]+",.*?keywords:`)
	var registry int
	for _, block := range entry.FindAll(orch, -1) {
		if m := gated.FindSubmatch(block); m != nil {
			registry++
			if !listed[string(m[1])] {
				t.Errorf("salesbuildr_execute gates %s, missing from mcpEndpointWrites", m[1])
			}
		}
	}
	if registry != len(mcpEndpointWrites()) {
		t.Errorf("salesbuildr_execute gates %d endpoints, mcpEndpointWrites lists %d", registry, len(mcpEndpointWrites()))
	}
}
