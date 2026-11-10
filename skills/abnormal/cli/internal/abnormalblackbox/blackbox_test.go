// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package abnormalblackbox runs the built CLI and MCP binaries against an
// Abnormal Security fake that echoes every credential it was sent back in
// every encoding it can, whole and split across lines, adjacent values and
// pages, and checks what a reviewer cannot read off the code (abnormal
// handfixes credkit-wiring, no-response-cache, api-origin-pinned,
// dry-run-sends-nothing, mcp-endpoint-write-confirm,
// mcp-mirror-write-confirm):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the Authorization bearer token from
//     ABNORMAL_API_TOKEN, from the credential store (auth set-token) or from
//     a config file's api_token, a stored token the environment overrides
//     (held unused), and the MCP HTTP token. The Abnormal API carries no
//     credential in a response or a request body (spec.json), so the fake's
//     records are public data;
//   - no file the program writes, except the 0600 credential store, holds
//     one either (the local store, receipts, exports, delivered files, the
//     feedback log, the learn journal);
//   - no credential leaves for a host other than the configured one by
//     redirect;
//   - the MCP server sends a write (a POST or PATCH) only when the call sets
//     confirm to the boolean true, and --dry-run sends nothing at all.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder.
package abnormalblackbox

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
	// No credential shares an 8-character piece with text the program prints
	// itself ("abnormal-cli"): the kit withholds any unit carrying one
	// (docs/credential-leak-contract.md 2(b)).
	bbToken     = "tokQ7fb3199bQ7ok3t1fb9Q7kt1o193fQb"   // ABNORMAL_API_TOKEN (sent as Authorization: Bearer <token>)
	bbStored    = "storedToken-71c3e9b05d2a4f68e1"       // auth set-token (stdin), the credential store
	bbFileToken = "cfgTokQ5cfQ5gT5okcQ5gTf"              // api_token in a 0600 config file named by ABNORMAL_CONFIG
	bbHTTPToken = "mcpHttpTok-7cmH7pk-o7Tcm7Hpt7ok-7mcT" // ABNORMAL_MCP_HTTP_TOKEN
	bbThreat    = "184712ab-6d8b-47b3-89d7-a314efef23ff" // a threat ID
	bbCase      = "1234"                                 // a case ID
	bbRedirect  = "9f0e1d2c-3b4a-4596-8877-665544332211" // a threat ID the fake redirects off-host
	bbDenied    = "40199"                                // a threat or case ID the fake answers 401
	bbResource  = "res_42"                               // an API resource ID
	bbActionID  = "act-5c2e9a"                           // the action ID a remediation returns
	bbVendor    = "supplier.example.com"                 // a vendor domain
	bbEmail     = "hq.admin@example.com"                 // an employee
)

func secrets() []string {
	return []string{bbToken, bbStored, bbFileToken, bbHTTPToken}
}

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "abnormal-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "abnormal-cli"+exe)
	bbMCP = filepath.Join(dir, "abnormal-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/abnormal-cli", bbMCP: "../../cmd/abnormal-mcp"} {
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
	mu      sync.Mutex
	offHost int
	methods []string       // API requests, "METHOD /path"
	headers map[string]int // Authorization header values the API received
	// redirectThreats sends the threats list (the syncable resource triage
	// ranks) to the other host too, so sync meets a redirect.
	redirectThreats bool
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

// threatDetail is GET /threats/{threat_id} for bbThreat, shaped so triage
// (which reads the synced threats_get rows) has something real to rank: one
// unremediated message received an hour before the fake started.
func threatDetail(received string) string {
	return fmt.Sprintf(`"threatId":%q,"recipientCount":3,"messages":[{"abxMessageId":4551618356913732000,"threatId":%q,`+
		`"subject":"HQ-Record invoice","fromAddress":"billing@attacker.example.net","toAddresses":[%q],"receivedTime":%q,`+
		`"attackType":"Invoice/Payment Fraud (BEC)","attackStrategy":"Name Impersonation","attackVector":"Text",`+
		`"impersonatedParty":"VIP","remediationStatus":"Not Remediated"}],"pageNumber":1`, bbThreat, bbThreat, bbEmail, received)
}

func isWrite(m string) bool {
	method, _, _ := strings.Cut(m, " ")
	return method != http.MethodGet
}

// actionPath is the action-status read remediate-watch polls.
func actionPath(p string) bool {
	return (strings.HasPrefix(p, "/threats/") || strings.HasPrefix(p, "/cases/")) && strings.Contains(p, "/actions/")
}

// managePath is POST /threats/{threat_id} or POST /cases/{case_id}.
func managePath(p string) bool {
	for _, prefix := range []string{"/threats/", "/cases/"} {
		if rest, ok := strings.CutPrefix(p, prefix); ok && rest != "" && !strings.Contains(rest, "/") {
			return true
		}
	}
	return false
}

func echoServer(t *testing.T) *server {
	t.Helper()
	sv := &server{headers: map[string]int{}}
	// A second host the API redirects to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(`{"threats":[]}`))
	}))
	received := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		sent := sentSecrets(r)
		echo := echoObject(sent)
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		if h := r.Header.Get("Authorization"); h != "" {
			sv.headers[h]++
		}
		redirectThreats := sv.redirectThreats
		sv.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		p := strings.TrimPrefix(r.URL.Path, "/v1")
		get := r.Method == http.MethodGet
		switch {
		case p == "/auditlogs" || strings.Contains(p, "/"+bbDenied):
			// Rejected (the audit log, a denied threat or case): the error
			// repeats what it was sent. The audit log is the exportable,
			// syncable resource no novel reads, so sync and export meet a 401.
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"status":401,"error":"invalid token %s","message":"bad token %s","echo":%s}`, strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case p == "/threats/"+bbRedirect || (redirectThreats && p == "/threats"):
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case get && p == "/threats/"+bbThreat:
			// threats retrieve-2 and the sync walker's threat detail: the echo
			// split across values, lines and spaces.
			fmt.Fprintf(w, `{%s,"split":%s}`, threatDetail(received), splitObject(sent))
		case get && p == "/threats":
			// threats list, sync, export: gzip-encoded, with the echo, and a
			// record whose identifier repeats the credential it was sent.
			extra := ""
			if len(sent) > 0 {
				extra = fmt.Sprintf(`,{"threatId":%q,"subject":"Echo-ID"}`, sent[0])
			}
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			fmt.Fprintf(zw, `{"threats":[{"threatId":%q,"subject":"HQ-Record","echo":%s}%s],"pageNumber":1}`,
				bbThreat, echoObject(sent), extra)
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		case get && p == "/vendors":
			// vendors list (and smoke): BOM-prefixed.
			fmt.Fprintf(w, "\xef\xbb\xbf"+`{"vendors":[{"vendorDomain":%q,"name":"HQ-Record"}],"pageNumber":1}`, bbVendor)
		case get && p == "/cases":
			// cases list (and employee-risk, smoke).
			fmt.Fprintf(w, `{"cases":[{"caseId":%q,"severity":"HIGH","severity_level":"HIGH","affectedEmployee":%q,"description":"HQ-Record","first_observed":%q}],"pageNumber":1}`,
				bbCase, bbEmail, received)
		case get && p == "/abusecampaigns":
			// The first credential it was sent, split across two pages: each
			// half is a record of its own, on its own page.
			a, b := "camp-a", "camp-b"
			if len(sent) > 0 {
				a, b = halves(sent[0])
			}
			switch r.URL.Query().Get("pageNumber") {
			case "", "1":
				fmt.Fprintf(w, `{"campaigns":[{"campaignId":%q}],"pageNumber":1,"nextPageNumber":2}`, a)
			case "2":
				fmt.Fprintf(w, `{"campaigns":[{"campaignId":%q}],"pageNumber":2,"nextPageNumber":3}`, b)
			default:
				fmt.Fprint(w, `{"campaigns":[],"pageNumber":3}`)
			}
		case get && p == "/soar/tokens":
			// A SOAR token record (TokenSwagger: no token value).
			fmt.Fprintf(w, `{"status":"success","status_code":200,"data":[{"token_id":"tok-1","name":"HQ-Record","status":"active","echo":%s}]}`, echo)
		case get && actionPath(p):
			fmt.Fprintf(w, `{"status":"done","description":"HQ-Record action finished","tenantId":1,"echo":%s}`, echo)
		case r.Method == http.MethodPost && p == "/resources/create":
			// CreateResourceResponse: the new resource's ID and status.
			fmt.Fprintf(w, `{"resourceId":%q,"status":"created","name":"HQ-Record","echo":%s}`, bbResource, echo)
		case r.Method == http.MethodPost && managePath(p):
			// A remediation or case update answers with the action to poll.
			fmt.Fprintf(w, `{"action_id":%q,"status_url":%q,"tenantName":"HQ-Record","echo":%s}`, bbActionID, sv.url+p+"/actions/"+bbActionID, echo)
		case !get:
			// Every other write echoes what it was sent.
			fmt.Fprintf(w, `{"resourceId":%q,"status":"ok","name":"HQ-Record","echo":%s}`, bbResource, echo)
		case strings.HasSuffix(p, "/logins") && strings.Contains(r.Header.Get("Accept"), "text/csv"):
			w.Header().Set("Content-Type", "text/csv")
			fmt.Fprintf(w, "timestamp,ip_address,country,status\n%s,203.0.113.5,US,success\n", received)
		case p == "/users" || p == "/roles":
			fmt.Fprintf(w, `{"status":"success","status_code":200,"data":[{"user_id":7,"id":7,"email":%q,"name":"HQ-Record","echo":%s}]}`, bbEmail, echo)
		case p == "/vendor-cases":
			fmt.Fprintf(w, `{"vendorCases":[{"vendorCaseId":123456789012,"vendorDomain":%q,"firstObservedTime":%q,"echo":%s}],"pageNumber":1}`, bbVendor, received, echo)
		case p == "/resources":
			fmt.Fprintf(w, `{"resources":[{"resourceId":%q,"name":"HQ-Record","echo":%s}],"pageNumber":1}`, bbResource, echo)
		default:
			fmt.Fprintf(w, `{"name":"HQ-Record","email":%q,"echo":%s}`, bbEmail, echo)
		}
	}))
	sv.url = srv.URL + "/v1"
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.caFile = ca
	return sv
}

// sawHeader reports how many API requests carried Authorization: v, and the
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

type bbEnv struct {
	home string
	env  []string
}

// credMode is where the process gets its bearer token.
type credMode int

const (
	credStored        credMode = iota // none in the environment: the battery runs on a token stored with auth set-token
	credEnv                           // ABNORMAL_API_TOKEN
	credEnvOverStored                 // a stored token and ABNORMAL_API_TOKEN: the environment wins, the stored token is held unused
	credConfigFile                    // api_token in a 0600 config file named by ABNORMAL_CONFIG
)

func (m credMode) String() string {
	return [...]string{"stored", "env", "env over stored", "config file"}[m]
}

// newEnv builds a clean environment against the fake with the credentials
// mode names; the stored modes run auth set-token first.
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
		"ABNORMAL_BASE_URL=" + sv.url, "ABNORMAL_CA_CERT=" + sv.caFile,
		"ABNORMAL_CLI_PATH=" + bbCLI, "ABNORMAL_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}
	e := &bbEnv{home: home, env: env}
	switch mode {
	case credStored:
		e.storeToken(t, bbStored)
	case credEnv:
		e.env = append(e.env, "ABNORMAL_API_TOKEN="+bbToken)
	case credEnvOverStored:
		e.storeToken(t, bbStored)
		e.env = append(e.env, "ABNORMAL_API_TOKEN="+bbToken)
	case credConfigFile:
		dir := filepath.Join(home, "cfg")
		_ = os.MkdirAll(dir, 0o700)
		cfg := filepath.Join(dir, "config.toml")
		if err := os.WriteFile(cfg, []byte(fmt.Sprintf("api_token = %q\n", bbFileToken)), 0o600); err != nil {
			t.Fatal(err)
		}
		e.env = append(e.env, "ABNORMAL_CONFIG="+cfg)
	}
	return e
}

// storeToken saves tok with auth set-token (stdin) and checks it landed in
// the credential store.
func (e *bbEnv) storeToken(t *testing.T, tok string) {
	t.Helper()
	out := e.runIn(t, tok+"\n", "auth", "set-token")
	var stored bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "credentials") {
			b, _ := os.ReadFile(path)
			stored = stored || bytes.Contains(b, []byte(tok))
		}
		return nil
	})
	if !stored {
		t.Fatalf("auth set-token wrote no credential store:\n%s", out)
	}
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

// foldedAlnum is s reduced to its ASCII letters and digits, lowercased: the
// form a normalizer that lowercases text and turns separators into spaces
// (the learning store's query family) leaves of a credential.
func foldedAlnum(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case 'A' <= c && c <= 'Z':
			b.WriteByte(c + ('a' - 'A'))
		case 'a' <= c && c <= 'z', '0' <= c && c <= '9':
			b.WriteByte(c)
		}
	}
	return b.String()
}

// foldedMinAlnum is the shortest folded credential the case-folded check
// matches (the chokepoint's joined-view floor); shorter ones match by chance.
const foldedMinAlnum = 16

// assertNoEcho fails when text holds a secret in any form the server sent,
// both halves of one (a split the reader could rejoin), or one case-folded
// or re-separated (a normalizer's lowercased copy).
func assertNoEcho(t *testing.T, where, text string) {
	t.Helper()
	folded := foldedAlnum(text)
	for _, s := range secrets() {
		for _, f := range echoForms(s) {
			if strings.Contains(text, f) {
				t.Errorf("%s holds a credential as %q", where, f)
			}
		}
		if a, b := halves(s); strings.Contains(text, a) && strings.Contains(text, b) {
			t.Errorf("%s holds both halves of %q", where, s)
		}
		if n := foldedAlnum(s); len(n) >= foldedMinAlnum && strings.Contains(folded, n) {
			t.Errorf("%s holds %q case-folded or re-separated", where, s)
		}
	}
}

// importFile writes an import input whose one record repeats tok, for the
// import write path (POST /resources/create per record).
func (e *bbEnv) importFile(t *testing.T, tok string) string {
	t.Helper()
	p := filepath.Join(e.home, "import.jsonl")
	if err := os.WriteFile(p, []byte(`{"name":"Imported `+tok+`","description":"HQ import"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// battery runs every output path; tok is a credential the process holds
// (ABNORMAL_API_TOKEN, the stored token or the config file's api_token).
func battery(t *testing.T, e *bbEnv, deliverFile, tok string) [][]string {
	imp := e.importFile(t, tok)
	echoed := filepath.Join(e.home, "deliver-echo.json")
	return [][]string{
		// Every output mode.
		{"threats", "retrieve", "--json"},
		{"threats", "retrieve"},
		{"threats", "retrieve", "--csv"},
		{"threats", "retrieve", "--plain"},
		{"threats", "retrieve", "--json", "--receipt"},
		{"threats", "retrieve", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		{"threats", "retrieve", "--json", "--deliver", "file:" + echoed},
		// Every response shape: BOM-prefixed, split echo, 401 echo, refused
		// redirect, split across pages.
		{"vendors", "--json"},
		{"vendors", "--json", "--deliver", "file:" + deliverFile},
		{"cases", "retrieve", "--json"},
		{"threats", "retrieve-2", bbThreat, "--json"},
		{"threats", "retrieve-2", bbDenied, "--json"},
		{"cases", "retrieve-2", bbDenied, "--json"},
		{"auditlogs", "--json"},
		{"threats", "retrieve-2", bbRedirect, "--json"},
		{"abusecampaigns", "retrieve", "--all", "--page-size", "1", "--json"},
		{"abusecampaigns", "retrieve", "--all", "--page-size", "1"},
		{"soar", "--json"},
		{"soar"},
		{"users", "--json"},
		{"api-resources", "resources-create-create", "--name", "HQ resource", "--description", "d", "--json"},
		// Writes the fake echoes, the remediation and its status poll among them.
		{"threats", "create", bbThreat, "--action", "remediate", "--json"},
		{"cases", "create", bbCase, "--action", "action_required", "--json"},
		{"api-resources", "resources-update-partial-update", bbResource, "--description", "HQ " + tok, "--json"},
		{"api-resources", "resources-actions-create", bbResource, "--action", "refresh", "--json"},
		{"remediate-watch", "threat", bbThreat, "--interval", "1s", "--timeout", "20s", "--json"},
		{"remediate-watch", "case", bbCase, "--action", "acknowledge_resolved", "--interval", "1s", "--timeout", "20s"},
		{"import", "api_resources", "--input", imp, "--json"},
		// The local store, exports and the novel commands.
		{"sync", "--json"},
		{"export", "threats", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
		{"export", "threats", "--format", "json"},
		{"export", "vendors", "--format", "jsonl", "--output", filepath.Join(e.home, "export-vendors.jsonl")},
		{"triage", "--since", "7d", "--json"},
		{"triage", "--since", "7d", "--include-remediated"},
		{"report-snapshot", "--since", "30d", "--json"},
		{"employee-risk", bbEmail, "--json"},
		{"vendor-risk", bbVendor, "--json"},
		{"smoke", "--json"},
		{"search", "HQ", "--json"},
		{"search", "Echo", "--json", "--data-source", "local"},
		// A credential on the command line: an unknown flag, a positional,
		// feedback text.
		{"threats", "retrieve", "--json", "--" + tok},
		{"threats", "retrieve-2", tok, "--json"},
		{"feedback", "noted the token " + tok + " in a ticket"},
		// Previews: a dry run sends nothing.
		{"api-resources", "resources-update-partial-update", bbResource, "--description", "Echo " + tok, "--dry-run"},
		{"api-resources", "resources-create-create", "--name", "Echo " + tok, "--description", "d", "--dry-run"},
		{"detection360", "reports-create", "--body-json", `{"description":"Echo ` + tok + `"}`, "--dry-run"},
		{"threats", "create", bbThreat, "--action", "remediate", "--dry-run"},
		{"remediate-watch", "case", bbCase, "--action", "acknowledge_resolved", "--dry-run"},
		{"import", "api_resources", "--input", imp, "--dry-run"},
		// Learnings, playbooks and learn events land in the local store's own tables.
		{"teach", "--query", "find threats " + tok, "--resource", bbThreat, "--resource-type", "threats",
			"--notes", "rotated " + tok, "--playbook-notes", "use " + tok, "--no-validate"},
		{"doctor"},
		{"auth", "status"},
	}
}

// mcpReads are read-only tools across every response shape the fake has.
func mcpReads() []map[string]any {
	none := map[string]any{}
	read := func(id string, params map[string]any) map[string]any {
		return map[string]any{"name": "abnormal_read", "arguments": map[string]any{"endpoint_id": id, "params": params}}
	}
	return []map[string]any{
		read("threats.retrieve", none),
		read("vendors.retrieve", none),
		read("cases.retrieve", none),
		read("threats.retrieve-2", map[string]any{"threat_id": bbThreat}),
		read("threats.retrieve-2", map[string]any{"threat_id": bbDenied}),
		read("soar.tokens-retrieve", none),
		read("users.retrieve", none),
		{"name": "abnormal_search", "arguments": map[string]any{"query": "threats"}},
		{"name": "abnormal_get", "arguments": map[string]any{"endpoint_id": "abusecampaigns.retrieve"}},
		{"name": "triage", "arguments": map[string]any{"since": "7d"}},
		{"name": "report_snapshot", "arguments": map[string]any{"since": "30d"}},
		{"name": "employee_risk", "arguments": map[string]any{"email": bbEmail}},
		{"name": "vendor_risk", "arguments": map[string]any{"vendor-domain": bbVendor}},
		{"name": "smoke", "arguments": none},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
	}
}

// mcpWrites is every write tool that takes confirm, called without it: each
// must answer with a preview and send nothing. abnormal_execute and the
// recipe intent preview as a confirmation_required object; the mirrored CLI
// writes (import, remediate_watch) preview as the command's --dry-run output
// followed by the gate's "Preview only: nothing was sent" note.
func mcpWrites() []map[string]any {
	execute := func(id string, params map[string]any) map[string]any {
		return map[string]any{"name": "abnormal_execute", "arguments": map[string]any{"endpoint_id": id, "params": params}}
	}
	return []map[string]any{
		{"name": "import", "arguments": map[string]any{"resource": "api_resources"}},
		{"name": "remediate_watch", "arguments": map[string]any{"threat_case": "threat", "id": bbThreat, "action": "remediate"}},
		{"name": "vendor_compromise_check", "arguments": map[string]any{"domain": bbVendor}},
		execute("threats.create", map[string]any{"threat_id": bbThreat, "action": "remediate"}),
		execute("cases.create", map[string]any{"case_id": bbCase, "action": "action_required"}),
		execute("api_resources.resources-create-create", map[string]any{"name": "Preview Co", "description": "d"}),
		execute("api_resources.resources-update-partial-update", map[string]any{"resource_id": bbResource, "description": "Preview"}),
		execute("api_resources.resources-actions-create", map[string]any{"resource_id": bbResource, "action": "refresh"}),
		execute("detection360.reports-create", map[string]any{"body": map[string]any{"description": "Preview report"}}),
		execute("email_search.search-create", map[string]any{"source": "abnormal", "filters": map[string]any{"subject": "Preview"}}),
		execute("email_search.search-remediate-create", map[string]any{"action": "delete", "remediation_reason": "false_negative", "source": "abnormal"}),
		execute("spm-v2.postures-query-create", map[string]any{"pageSize": 10}),
		execute("auditlogs.retrieve", map[string]any{}),
	}
}

func mcpBattery() []map[string]any {
	return append(mcpReads(), mcpWrites()...)
}

// Class credential-persisted-in-files.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	// ponytail: the two batteries own their server and home, so they share wall time (CI default 10m package timeout).
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, args := range battery(t, e, deliverFile, bbStored) {
		e.run(t, args...)
	}
	// Controls without a credential: the same sinks write, so the scans
	// below are not vacuous.
	e.run(t, "feedback", "control note HQ-Feedback")
	e.run(t, "teach", "--query", "find threats control", "--resource", bbThreat, "--resource-type", "threats",
		"--notes", "rotated control", "--no-validate")
	e.mcpCalls(t, mcpBattery())
	if n, all := sv.sawHeader("Bearer " + bbStored); n == 0 {
		t.Fatalf("no request carried the stored token as the bearer (saw %d distinct values); the battery ran unauthenticated", len(all))
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
		if rel == "import.jsonl" {
			return nil // the test's own input
		}
		if synccpuStoreFile(path) { // synccpu: the store is not checked for this program's own credentials
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
	if b, err := os.ReadFile(deliverFile); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
		t.Errorf("%s: want the delivered record (err %v)", filepath.Base(deliverFile), err)
	}
	if b, err := os.ReadFile(filepath.Join(e.home, "export-vendors.jsonl")); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
		t.Errorf("export-vendors.jsonl: want the exported record (err %v):\n%.1500s", err, b)
	}
	// The synced threat detail echoes the bearer split, so the kit's SQL sink
	// refuses that row; triage_sync_test.go proves the walker on a clean fake.
	var sawStore, sawLearning, sawFeedback bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".db") {
			sawStore = sawStore || bytes.Contains(b, []byte("HQ-Record"))
			// The control teach run's learning row was written.
			sawLearning = sawLearning || bytes.Contains(b, []byte("rotated control"))
		}
		// The control feedback entry was written.
		sawFeedback = sawFeedback || (d.Name() == "feedback.jsonl" && bytes.Contains(b, []byte("HQ-Feedback")))
		return nil
	})
	if !sawStore {
		t.Errorf("no local store holds a fetched record")
	}
	if !sawLearning {
		t.Errorf("no local store holds the taught learning; the teach run wrote nothing")
	}
	if !sawFeedback {
		t.Errorf("no feedback.jsonl holds the control feedback entry; the feedback run wrote nothing")
	}
}

// Class secret-in-output, once per credential source the bearer token can
// come from: ABNORMAL_API_TOKEN (with a stored token the environment
// overrides, held unused) and a config file's api_token. (The stored token
// as the sent bearer runs in TestNoCredentialInAnyWrittenFile.)
func TestNoCredentialInAnyOutput(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		mode credMode
		tok  string   // the credential the process sends, typed on the command line
		held []string // credentials the process holds but never sends, typed where the program repeats them
	}{
		{credEnvOverStored, bbToken, []string{bbStored, bbHTTPToken}},
		{credConfigFile, bbFileToken, []string{bbHTTPToken}},
	} {
		t.Run(c.mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, c.mode)
			deliverFile := filepath.Join(e.home, "deliver.json")
			for _, args := range battery(t, e, deliverFile, c.tok) {
				assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
			}
			// A held value the API never receives, typed where the program
			// repeats it: a path the fake echoes and a dry-run body.
			for _, held := range c.held {
				for _, args := range [][]string{
					{"threats", "retrieve-2", held, "--json"},
					{"api-resources", "resources-create-create", "--name", "HQ " + held, "--description", "d", "--dry-run"},
					{"import", "api_resources", "--input", e.importFile(t, held), "--dry-run"},
				} {
					assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
				}
			}
			// A record that carries no credential still prints, as valid JSON:
			// the kit withholds only the units that carry one.
			cmd := exec.Command(bbCLI, "vendors", "--json")
			cmd.Env, cmd.Dir = e.env, e.home
			stdout, err := cmd.Output()
			if err != nil {
				t.Fatalf("vendors --json: %v", err)
			}
			var doc any
			if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("HQ-Record")) {
				t.Fatalf("want valid JSON with the record, got:\n%.1500s", stdout)
			}
			out := e.mcpCalls(t, mcpBattery())
			if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
				t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
			}
			assertNoEcho(t, "MCP session", out)
			if !strings.Contains(out, "HQ-Record") {
				t.Fatalf("MCP results lost the record itself:\n%.2000s", out)
			}
			// The fake echoed the bearer it really received, so the scans above
			// covered it; no other credential source was sent.
			n, all := sv.sawHeader("Bearer " + c.tok)
			if n == 0 || len(all) != 1 {
				t.Fatalf("want every request to carry the %s bearer, got %d of it and %d distinct values", c.mode, n, len(all))
			}
		})
	}
}

// Class credential-sent-to-another-host: a redirect off the configured API
// host is refused before it is followed, from the CLI, sync and MCP alike.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	redirect := e.run(t, "threats", "retrieve-2", bbRedirect, "--json")
	sv.mu.Lock()
	sv.redirectThreats = true
	sv.mu.Unlock()
	syncOut := e.run(t, "sync", "--resources", "threats", "--json")
	e.mcpCalls(t, []map[string]any{
		{"name": "abnormal_read", "arguments": map[string]any{"endpoint_id": "threats.retrieve-2", "params": map[string]any{"threat_id": bbRedirect}}},
		{"name": "abnormal_read", "arguments": map[string]any{"endpoint_id": "threats.retrieve"}},
	})
	sv.mu.Lock()
	n := sv.offHost
	var redirected int
	for _, m := range sv.methods {
		if strings.HasSuffix(m, "/v1/threats/"+bbRedirect) || strings.HasSuffix(m, "/v1/threats") {
			redirected++
		}
	}
	sv.mu.Unlock()
	if n != 0 {
		t.Fatalf("the other host was contacted %d time(s)", n)
	}
	if redirected < 4 {
		t.Fatalf("only %d request(s) met a redirect; the CLI, sync and MCP calls did not all run", redirected)
	}
	for where, out := range map[string]string{"threats retrieve-2": redirect, "sync": syncOut} {
		if !strings.Contains(out, "leave the configured Abnormal host") {
			t.Fatalf("%s: want the pin refusal, got:\n%.1500s", where, out)
		}
	}
}

// mcp-endpoint-write-confirm and mcp-mirror-write-confirm: every request the
// MCP server and its mirrored CLI children send to the API is a read (a GET)
// while no call sets confirm, though the battery calls every write tool; each
// write answers with a preview.
func TestMCPSendsOnlyReadsWithoutConfirm(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	// The store holds the synced threats first, so triage has something to
	// rank while the writes preview.
	e.run(t, "sync", "--json")
	sv.mu.Lock()
	sv.methods = nil
	sv.mu.Unlock()
	out := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(out, "HQ-Record") {
		t.Fatalf("the MCP battery fetched nothing:\n%.2000s", out)
	}
	replies := mcpReplies(out)
	for i, call := range mcpWrites() {
		id := len(mcpReads()) + i + 1
		if r := replies[id]; !strings.Contains(r, "confirmation_required") && !strings.Contains(r, "Preview only: nothing was sent") {
			t.Errorf("%s %v without confirm did not return a preview:\n%.1500s", call["name"], call["arguments"], r)
		}
	}
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if len(sv.methods) == 0 {
		t.Fatal("no request reached the fake")
	}
	for _, m := range sv.methods {
		if isWrite(m) {
			t.Errorf("the MCP server sent %s without confirm", m)
		}
	}
}

// Control for the test above: with confirm set to boolean true the same write
// reaches the API, so the gate previews rather than drops; the string "true"
// still previews. Both gates: the endpoint tool (abnormal_execute) and the
// mirrored write (remediate_watch, which posts the remediation and polls it).
func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	write := func(confirm any) []map[string]any {
		return []map[string]any{
			{"name": "abnormal_execute", "arguments": map[string]any{"endpoint_id": "cases.create",
				"params": map[string]any{"case_id": bbCase, "action": "action_required"}, "confirm": confirm}},
			{"name": "remediate_watch", "arguments": map[string]any{"threat_case": "threat", "id": bbThreat, "action": "remediate",
				"interval": "1s", "timeout": "20s", "confirm": confirm}},
		}
	}
	e.mcpPhases(t, write("true"))
	sv.mu.Lock()
	for _, m := range sv.methods {
		if isWrite(m) {
			t.Errorf("confirm \"true\" (a string) sent %s", m)
		}
	}
	sv.mu.Unlock()
	e.mcpPhases(t, write(true))
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var caseUpdate, remediate, poll bool
	for _, m := range sv.methods {
		caseUpdate = caseUpdate || m == "POST /v1/cases/"+bbCase
		remediate = remediate || m == "POST /v1/threats/"+bbThreat
		poll = poll || m == "GET /v1/threats/"+bbThreat+"/actions/"+bbActionID
	}
	if !caseUpdate {
		t.Errorf("confirm true did not send the case update; requests: %v", sv.methods)
	}
	if !remediate || !poll {
		t.Errorf("confirm true did not post the remediation and poll its action; requests: %v", sv.methods)
	}
}

// dry-run-sends-nothing: a --dry-run write never reaches the API, from a cold
// start with the token in the environment, the credential store or a config
// file.
func TestDryRunSendsNothing(t *testing.T) {
	for _, mode := range []credMode{credEnv, credStored, credConfigFile} {
		sv := echoServer(t)
		e := newEnv(t, sv, mode)
		imp := e.importFile(t, "dry")
		for _, args := range [][]string{
			{"threats", "create", bbThreat, "--action", "remediate", "--dry-run"},
			{"cases", "create", bbCase, "--action", "action_required", "--dry-run"},
			{"api-resources", "resources-create-create", "--name", "Dry Co", "--description", "d", "--dry-run"},
			{"api-resources", "resources-update-partial-update", bbResource, "--description", "Dry Co", "--dry-run"},
			{"api-resources", "resources-actions-create", bbResource, "--action", "refresh", "--dry-run"},
			{"detection360", "reports-create", "--body-json", `{"description":"Dry report"}`, "--dry-run"},
			{"email-search", "search-create", "--filters-start-time", "2026-10-01T00:00:00Z", "--filters-end-time", "2026-10-28T00:00:00Z",
				"--source", "abnormal", "--dry-run"},
			{"email-search", "search-remediate-create", "--action", "delete", "--remediation-reason", "false_negative",
				"--source", "abnormal", "--dry-run"},
			{"spm-v2", "postures-query-create", "--dry-run"},
			{"remediate-watch", "threat", bbThreat, "--dry-run"},
			{"remediate-watch", "case", bbCase, "--action", "acknowledge_resolved", "--dry-run"},
			{"import", "api_resources", "--input", imp, "--dry-run"},
			{"threats", "retrieve", "--dry-run"},
		} {
			// Each run reaches its preview, so the count below is not vacuous
			// (a flag error would send nothing either).
			if out := e.run(t, args...); !strings.Contains(out, "dry_run") && !strings.Contains(out, "dry run") && !strings.Contains(out, "would ") {
				t.Errorf("%v (mode %s) printed no preview:\n%.1500s", args, mode, out)
			}
		}
		sv.mu.Lock()
		if len(sv.methods) != 0 {
			t.Errorf("--dry-run (mode %s) sent %v", mode, sv.methods)
		}
		sv.mu.Unlock()
		// Control: the same write without --dry-run reaches the fake.
		out := e.run(t, "api-resources", "resources-update-partial-update", bbResource, "--description", "Dry Co")
		sv.mu.Lock()
		if len(sv.methods) != 1 || sv.methods[0] != "PATCH /v1/resources/"+bbResource+"/update" {
			t.Errorf("control (mode %s): want the one PATCH, got %v:\n%.1500s", mode, sv.methods, out)
		}
		sv.mu.Unlock()
	}
}

// synccpuStoreFile reports whether path is the local SQLite store or one of its
// side files. synccpu (owner GO 2026-11-09) dropped the store's credential check
// except for held vendor response values, so a credential this program holds from
// its own configuration may sit in the store; every other file is still checked.
func synccpuStoreFile(path string) bool {
	for _, s := range []string{".db", ".db-wal", ".db-shm", ".db-journal"} {
		if strings.HasSuffix(path, s) {
			return true
		}
	}
	return false
}
