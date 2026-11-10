// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package gradientblackbox runs the built CLI and MCP binaries against a
// Gradient fake that echoes every credential it knows back in every
// encoding it can, whole and split across lines and adjacent values, and
// checks what a reviewer cannot read off the code (gradient handfixes
// credkit-wiring, no-response-cache, config-file-holds-no-credential,
// api-origin-pinned, dry-run-sends-nothing, mcp-endpoint-write-confirm). A
// unit that repeats a held credential is withheld whole by the shared
// credential kit, with its notice (kitNotice); the public-data controls read a
// quiet fake that echoes nothing (quietServer):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the GRADIENT-TOKEN value from GRADIENT_TOKEN, from
//     the credential store (auth set-token) or derived from the
//     GRADIENT_VENDOR_API_KEY + GRADIENT_PARTNER_API_KEY pair (and either key
//     of the pair), the MCP HTTP token and a credential-named flag value (the
//     credential-named members a response carries are tracked, not failed:
//     outOfContract);
//   - no file the program writes, except the 0600 credential store, holds
//     one either (the local store, receipts, exports, delivered files, the
//     usage and alert ledgers, the learn journal);
//   - no credential leaves for a host other than the configured one by
//     redirect;
//   - the MCP server sends a write only when the call sets confirm to the
//     boolean true, and --dry-run sends nothing at all.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder.
package gradientblackbox

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
	bbToken      = "bbtok-kkwwzwmzqzzkkwwmwzmmqzmwwqmzkzzqq" // GRADIENT_TOKEN (the header value itself)
	bbVendorKey  = "bbvnd-kzqzmqzqmmkkqmmkzwzkmwk"           // GRADIENT_VENDOR_API_KEY
	bbPartnerKey = "bbptn-kqqmqmmwwzqwqkqzkzwmmkkmqk"        // GRADIENT_PARTNER_API_KEY
	bbStored     = "bbsto-qmkwqkmqwzwkzwqqkzzmkkwq"          // auth set-token (stdin), the credential store
	bbArgvSec    = "-bbarg-wkqqmqwqkwmqq"                    // --client-secret on the command line (starts with "-")
	bbShortResp  = "Sh0rtPw"                                 // a 7-character credential a response carries
	bbBOMResp    = "bbbom-mzqzwwmwzkqz"                      // a credential in a BOM-prefixed response
	bbGzipResp   = "bbgzp-zkkmkwmwmmqqkkzz"                  // a credential in a gzip-encoded response
	bbHTTPToken  = "bbmht-mkzkzzkzzwmzmwkwzzmkqkkmzkkww"     // GRADIENT_MCP_HTTP_TOKEN
	bbAccount    = "acct-1042"                               // an account id
	bbService    = "svc-m365-backup"                         // a service id
	bbDenied     = "svc-denied"                              // a service id the fake answers 401
	bbMessage    = "msg-6f1c2d3e"                            // an alert messageId
)

// bbPairToken is the GRADIENT-TOKEN header the CLI derives from the key pair:
// base64("<vendor>:<partner>"). The fake receives it on the wire, so it echoes
// it like any other credential.
var bbPairToken = base64.StdEncoding.EncodeToString([]byte(bbVendorKey + ":" + bbPartnerKey))

// secrets are every credential the suite gives the process that the
// credential leak contract holds (section 1: the declared secret environment,
// the credential store, the derived GRADIENT-TOKEN) and the command-line
// value. Each one in any output, file or request off its origin fails the
// test.
func secrets() []string {
	return []string{bbToken, bbVendorKey, bbPartnerKey, bbPairToken, bbStored, bbArgvSec, bbHTTPToken}
}

// outOfContract are the credential-named members the fake's answers carry (a
// password, an accessToken). The contract holds a response field only when
// the connector declares it (section 1), and Gradient declares none: its
// documented responses (spec.json) carry no credential member. The fake still
// answers them, and an output that carries one is counted (outOfContractSeen,
// reported by TestMain), never failed: the old connector-side layer withheld
// them by name, the kit does not (handfixes.json credkit-wiring).
func outOfContract() []string {
	return []string{bbShortResp, bbBOMResp, bbGzipResp}
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
	dir, err := os.MkdirTemp("", "gradient-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "gradient-cli"+exe)
	bbMCP = filepath.Join(dir, "gradient-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/gradient-cli", bbMCP: "../../cmd/gradient-mcp"} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	fmt.Fprintf(os.Stderr, "gradientblackbox: %d checked output(s) carried an out-of-contract value (outOfContract; tracked, not a leak)\n", outOfContractSeen.n)
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
	headers map[string]int // GRADIENT-TOKEN header values the API received
	// redirectAccounts sends the accounts list (the one syncable resource)
	// to the other host too, so sync meets a redirect.
	redirectAccounts bool
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

// gradientRecord is one account record, shaped so the store, the novel
// commands (hygiene unmapped, status ready) and alert send have something
// real to read: it is unmapped, and it carries a messageId.
const gradientRecord = `"id":"acct-1042","name":"HQ-Record","description":"HQ account","mappingId":"","isMapped":false,"accountId":"acct-1042","serviceId":"svc-m365-backup","messageId":"msg-6f1c2d3e","alertId":"cli-hq","status":"active","createdAt":"2026-10-20T10:00:00Z","updatedAt":"2026-10-21T10:00:00Z"`

// envelope wraps one object the way the Synthesize vendor API answers a
// single-object read.
func envelope(obj string) string {
	return `{"data":` + obj + `,"message":"ok","status":200}`
}

func echoServer(t *testing.T) *server {
	t.Helper()
	return fakeServer(t, false)
}

// quietServer is the fake answering as Gradient does: the same records,
// repeating nothing a request carried. The kit withholds a unit whole
// (credential leak contract section 4, no partial redaction), so an answer
// that echoes the held token never prints or persists any of its public data;
// the quiet fake is what proves a sink really writes.
func quietServer(t *testing.T) *server {
	t.Helper()
	return fakeServer(t, true)
}

func fakeServer(t *testing.T, quiet bool) *server {
	t.Helper()
	sv := &server{headers: map[string]int{}}
	// A second host the API redirects to: a credential must never reach it.
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
		if h := r.Header.Get("GRADIENT-TOKEN"); h != "" {
			sv.headers[h]++
		}
		redirectAccounts := sv.redirectAccounts
		sv.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		p := strings.TrimPrefix(r.URL.Path, "/api")
		get := r.Method == http.MethodGet
		switch {
		case p == "/vendor-api/organization/clients" || strings.Contains(p, "/"+bbDenied):
			// Rejected: the error repeats what it was sent.
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"status":401,"error":"invalid token %s","message":"bad token %s","echo":%s}`, strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case strings.HasPrefix(p, "/vendor-api/alerting/debug/") || (redirectAccounts && p == "/vendor-api/organization/accounts"):
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case get && strings.HasPrefix(p, "/vendor-api/service/"):
			// services get: the echo split across values, lines and spaces.
			fmt.Fprint(w, envelope(fmt.Sprintf(`{%s,"split":%s}`, gradientRecord, splitObject(sent))))
		case get && p == "/vendor-api":
			// vendor get (and hygiene unmapped): BOM-prefixed, with a SKU missing metadata.
			fmt.Fprint(w, "\xef\xbb\xbf"+envelope(fmt.Sprintf(`{"id":"vend-1","name":"HQ-Vendor","description":"HQ vendor","skus":[{"id":"sku-1","name":"HQ-SKU","description":"","category":"Backup","subcategory":""}],"note":%q,"password":%q}`, bbBOMResp, bbBOMResp)))
		case get && p == "/vendor-api/organization":
			// integration get (and status ready): a short credential.
			fmt.Fprintf(w, `{"organizationId":"org-1","name":"HQ-Org","vendorId":"vend-1","vendorName":"HQ-Vendor","integrationId":"int-1","status":"active","lastSyncedAt":"2026-10-20T10:00:00Z","note":%q,"password":%q}`, bbShortResp, bbShortResp)
		case get && p == "/vendor-api/organization/accounts":
			// accounts list, sync, export: a bare array, gzip-encoded, with an
			// access token, the echo, and a record whose identifier repeats the
			// token it was sent.
			extra := ""
			if len(sent) > 0 {
				extra = fmt.Sprintf(`,{"id":%q,"name":"Echo-ID","isMapped":false}`, sent[0])
			}
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			fmt.Fprintf(zw, `[{%s,"accessToken":%q,"echo":%s}%s]`, gradientRecord, bbGzipResp, echoObject(append(sent, bbGzipResp)), extra)
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		case !get:
			// Every write echoes what it was sent.
			fmt.Fprintf(w, `{%s,"echo":%s}`, gradientRecord, echo)
		default:
			fmt.Fprint(w, envelope(fmt.Sprintf(`{%s,"echo":%s}`, gradientRecord, echo)))
		}
	}))
	sv.url = srv.URL + "/api"
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.caFile = ca
	return sv
}

// sawHeader reports how many API requests carried GRADIENT-TOKEN: v, and the
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

// credMode is where the process gets its GRADIENT-TOKEN.
type credMode int

const (
	credStored   credMode = iota // none in the environment: the battery runs on a token stored with auth set-token
	credEnvToken                 // GRADIENT_TOKEN
	credEnvBoth                  // GRADIENT_TOKEN and the key pair: GRADIENT_TOKEN wins, the pair is held unused
	credPair                     // GRADIENT_VENDOR_API_KEY + GRADIENT_PARTNER_API_KEY: the header is derived
)

// newEnv builds a clean environment against the fake with the credentials
// mode names.
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
		"GRADIENT_BASE_URL=" + sv.url, "GRADIENT_CA_CERT=" + sv.caFile,
		"GRADIENT_CLI_PATH=" + bbCLI, "GRADIENT_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}
	switch mode {
	case credEnvToken:
		env = append(env, "GRADIENT_TOKEN="+bbToken)
	case credEnvBoth:
		env = append(env, "GRADIENT_TOKEN="+bbToken, "GRADIENT_VENDOR_API_KEY="+bbVendorKey, "GRADIENT_PARTNER_API_KEY="+bbPartnerKey)
	case credPair:
		env = append(env, "GRADIENT_VENDOR_API_KEY="+bbVendorKey, "GRADIENT_PARTNER_API_KEY="+bbPartnerKey)
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

// countsFiles writes two usage-push inputs: the first has a row the fake
// rejects (its 401 echo lands in the push ledger's error field), the second
// changes a count, so usage drift has two runs to compare.
func (e *bbEnv) countsFiles(t *testing.T) (string, string) {
	t.Helper()
	first := filepath.Join(e.home, "counts-1.csv")
	second := filepath.Join(e.home, "counts-2.csv")
	for p, rows := range map[string]string{
		first:  bbAccount + "," + bbService + ",3\n" + bbAccount + "," + bbDenied + ",1\n",
		second: bbAccount + "," + bbService + ",5\n",
	} {
		if err := os.WriteFile(p, []byte("accountId,serviceId,unitCount\n"+rows), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return first, second
}

// battery runs every output path; tok is a credential the process holds
// (GRADIENT_TOKEN, the stored token, or a key of the pair).
func battery(t *testing.T, e *bbEnv, deliverFile, tok string) [][]string {
	counts1, counts2 := e.countsFiles(t)
	return [][]string{
		// Every output mode.
		{"accounts", "list", "--json"},
		{"accounts", "list"},
		{"accounts", "list", "--csv"},
		{"accounts", "list", "--plain"},
		{"accounts", "list", "--json", "--receipt"},
		{"accounts", "list", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		{"accounts", "list", "--json", "--deliver", "file:" + deliverFile},
		// Every response shape: BOM-prefixed, short credential, split echo,
		// 401 echo, refused redirect.
		{"vendor", "get", "--json"},
		{"integration", "get", "--json"},
		{"services", "get", bbService, "--json"},
		{"clients", "--json"},
		{"ticket-events", bbMessage, "--json"},
		// Writes the fake echoes; alert send and usage push also write the ledgers.
		{"mappings", "update", bbService, "--name", "HQ map", "--json"},
		{"accounts", "update-one", bbAccount, "--name", "HQ", "--json"},
		{"billing", bbDenied, "--account-id", bbAccount, "--unit-count", "2", "--json"},
		{"alert", "send", "--account", bbAccount, "--title", "rotated " + tok, "--description", "nightly job failed", "--json"},
		{"usage", "push", "--file", counts1, "--json"},
		{"usage", "push", "--file", counts2, "--json"},
		// The local store, exports and the novel commands.
		{"sync", "--json"},
		{"export", "accounts", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
		{"export", "accounts", "--format", "json"},
		{"hygiene", "unmapped", "--json"},
		{"status", "ready", "--json"},
		{"usage", "drift", "--json"},
		{"alert", "trace", "--json"},
		{"search", "HQ", "--json"},
		{"search", "Echo", "--json", "--data-source", "local"},
		// A credential on the command line: a credential-named flag, an unknown
		// flag, a positional, feedback text.
		{"accounts", "list", "--client-secret", bbArgvSec},
		{"accounts", "list", "--json", "--" + tok},
		{"services", "get", tok, "--json"},
		{"feedback", "noted the token " + tok + " in a ticket"},
		// Previews: a dry run sends nothing.
		{"mappings", "update", bbService, "--name", "Echo " + tok, "--dry-run"},
		{"accounts", "create", "--body-json", `[{"id":"acct-9","name":"Echo ` + tok + `"}]`, "--dry-run"},
		{"alerting", bbAccount, "--alert-id", "a-1", "--title", "Echo " + tok, "--description", "d", "--dry-run"},
		{"integration", "update-status", "--status", "active", "--dry-run"},
		// Learnings, playbooks and learn events land in the local store's own tables.
		{"teach", "--query", "find accounts " + tok, "--resource", bbAccount, "--resource-type", "accounts",
			"--notes", "rotated " + tok, "--playbook-notes", "use " + tok, "--no-validate"},
		{"doctor"},
		{"auth", "status"},
	}
}

// mcpReads are read-only tools across every response shape the fake has.
func mcpReads() []map[string]any {
	none := map[string]any{}
	return []map[string]any{
		{"name": "accounts_list", "arguments": none},
		{"name": "vendor_get", "arguments": none},
		{"name": "integration_get", "arguments": none},
		{"name": "services_get", "arguments": map[string]any{"serviceId": bbService}},
		{"name": "clients_list", "arguments": none},
		{"name": "hygiene_unmapped", "arguments": none},
		{"name": "status_ready", "arguments": none},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
	}
}

// mcpWrites is every write tool that takes confirm, called without it: each
// must answer with a preview and send nothing. The endpoint tools preview as
// a confirmation_required object; the mirrored CLI writes (alert_send,
// usage_push, import) preview as the command's --dry-run plan followed by the
// gate's "Preview only: nothing was sent" note.
func mcpWrites() []map[string]any {
	return []map[string]any{
		{"name": "usage_push", "arguments": map[string]any{}},
		{"name": "import", "arguments": map[string]any{"resource": "accounts"}},
		{"name": "accounts_create", "arguments": map[string]any{"body_json": `[{"id":"acct-9","name":"Preview Co"}]`}},
		{"name": "accounts_update", "arguments": map[string]any{"body_json": `[{"id":"acct-9","name":"Preview Co"}]`}},
		{"name": "accounts_update-one", "arguments": map[string]any{"accountId": bbAccount, "name": "Preview Co"}},
		{"name": "mappings_create", "arguments": map[string]any{"body_json": `[{"id":"map-9","name":"Preview map"}]`}},
		{"name": "mappings_update", "arguments": map[string]any{"serviceId": bbService, "name": "Preview map"}},
		{"name": "mappings_update-bulk", "arguments": map[string]any{"body_json": `[{"id":"map-9","name":"Preview map"}]`}},
		{"name": "services_create", "arguments": map[string]any{"name": "Preview svc", "description": "d", "category": "Backup", "subcategory": "M365"}},
		{"name": "billing_create", "arguments": map[string]any{"serviceId": bbService, "unitCount": 3, "accountId": bbAccount}},
		{"name": "alerting_dispatch", "arguments": map[string]any{"accountId": bbAccount, "alertId": "a-1", "title": "Preview", "description": "d", "priority": 1, "status": 1}},
		{"name": "integration_update-status", "arguments": map[string]any{"status": "active"}},
		{"name": "vendor_update", "arguments": map[string]any{"name": "Preview Vendor"}},
		{"name": "alert_send", "arguments": map[string]any{"account": bbAccount, "title": "Preview", "description": "d"}},
	}
}

func mcpBattery() []map[string]any {
	return append(mcpReads(), mcpWrites()...)
}

// Class credential-persisted-in-files.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	e.runIn(t, bbStored+"\n", "auth", "set-token")
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, args := range battery(t, e, deliverFile, bbStored) {
		e.run(t, args...)
	}
	e.mcpCalls(t, mcpBattery())
	// Controls without a credential: the same sinks write, so the scans below
	// are not vacuous. Every battery answer echoes the held token, so the kit
	// withheld each of its store, delivery and ledger writes whole (credential
	// leak contract section 4, no partial redaction); the data controls read
	// the quiet fake instead, into the same home (rocketcyber precedent).
	quiet := quietServer(t)
	echoEnv := e.env
	e.env = append(withoutEnv(withoutEnv(e.env, "GRADIENT_BASE_URL"), "GRADIENT_CA_CERT"),
		"GRADIENT_BASE_URL="+quiet.url, "GRADIENT_CA_CERT="+quiet.caFile)
	counts1, _ := e.countsFiles(t)
	e.run(t, "sync", "--json")
	e.run(t, "accounts", "list", "--json", "--deliver", "file:"+deliverFile)
	e.run(t, "usage", "push", "--file", counts1, "--json")
	e.run(t, "alert", "send", "--account", bbAccount, "--title", "control alert", "--description", "nightly job failed", "--json")
	e.env = echoEnv
	e.run(t, "teach", "--query", "find accounts control", "--resource", bbAccount, "--resource-type", "accounts",
		"--notes", "rotated control", "--no-validate")
	if n, all := sv.sawHeader(bbStored); n == 0 {
		t.Fatalf("no request carried the stored token as GRADIENT-TOKEN (saw %d distinct values); the battery ran unauthenticated", len(all))
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
		if !synccpuStoreFile(path) { // synccpu: the local store is not checked
			assertNoEcho(t, "file "+rel, string(data))
		}
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
	ledger := filepath.Join(e.home, ".config", "gradient-cli", "ledger")
	if b, err := os.ReadFile(filepath.Join(ledger, "pushes.jsonl")); err != nil || !bytes.Contains(b, []byte(bbService)) || !bytes.Contains(b, []byte(`"failed"`)) {
		t.Errorf("pushes.jsonl: want the pushed and the rejected row (err %v):\n%.1500s", err, b)
	}
	if b, err := os.ReadFile(filepath.Join(ledger, "alerts.jsonl")); err != nil || !bytes.Contains(b, []byte(bbMessage)) {
		t.Errorf("alerts.jsonl: want the dispatched alert (err %v):\n%.1500s", err, b)
	}
	// The battery's alert, its title typed with the held token, reached the
	// sink as the kit's notice: the scan above covered a real write.
	if b, _ := os.ReadFile(filepath.Join(ledger, "alerts.jsonl")); !bytes.Contains(b, []byte(kitNotice("file"))) {
		t.Errorf("alerts.jsonl holds no withheld entry; the credential-bearing alert never reached the sink:\n%.1500s", b)
	}
	var sawStore, sawLearning bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".db") {
			if b, rerr := os.ReadFile(path); rerr == nil {
				sawStore = sawStore || bytes.Contains(b, []byte("HQ-Record"))
				// The teach run's learning row was written, its secret withheld in place.
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

// Class secret-in-output, once with GRADIENT_TOKEN (the key pair set too, and
// unused: GRADIENT_TOKEN wins) and once with the key pair alone (the CLI
// derives the header, which the fake then echoes).
func TestNoCredentialInAnyOutput(t *testing.T) {
	for _, c := range []struct {
		name   string
		mode   credMode
		tok    string // a credential the process holds, typed on the command line
		header string // the GRADIENT-TOKEN value the API must receive
	}{
		{"env token", credEnvBoth, bbToken, bbToken},
		{"key pair", credPair, bbVendorKey, bbPairToken},
	} {
		t.Run(c.name, func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, c.mode)
			deliverFile := filepath.Join(e.home, "deliver.json")
			for _, args := range battery(t, e, deliverFile, c.tok) {
				assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
			}
			// Every answer here echoes the held token, so the kit withholds
			// what carries it whole (credential leak contract section 4: no
			// partial redaction) and says so with its notice: the scans above
			// were not vacuous.
			if out := e.run(t, "accounts", "list", "--json"); !strings.Contains(out, kitNotice("stdout")) {
				t.Fatalf("accounts list --json: want the kit's stdout notice for the echoed token, got:\n%.1500s", out)
			}
			out := e.mcpCalls(t, mcpBattery())
			if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
				t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
			}
			assertNoEcho(t, "MCP session", out)
			if !strings.Contains(out, kitNotice("mcp")) {
				t.Fatalf("the MCP session withheld nothing; the echoes did not reach it:\n%.2000s", out)
			}
			// The same reads print their data, as valid JSON, from the quiet fake.
			quiet := quietServer(t)
			qe := &bbEnv{home: e.home, env: append(withoutEnv(withoutEnv(e.env, "GRADIENT_BASE_URL"), "GRADIENT_CA_CERT"),
				"GRADIENT_BASE_URL="+quiet.url, "GRADIENT_CA_CERT="+quiet.caFile)}
			cmd := exec.Command(bbCLI, "accounts", "list", "--json")
			cmd.Env, cmd.Dir = qe.env, qe.home
			stdout, err := cmd.Output()
			if err != nil {
				t.Fatalf("accounts list --json (quiet fake): %v", err)
			}
			var doc any
			if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("HQ-Record")) {
				t.Fatalf("quiet fake: want valid JSON with the record, got:\n%.1500s", stdout)
			}
			if qout := qe.mcpCalls(t, mcpReads()); !strings.Contains(qout, "HQ-Record") {
				t.Fatalf("quiet fake: MCP results lost the record itself:\n%.2000s", qout)
			}
			// The fake echoed the header it really received, so the scans above
			// covered it; no other credential source was sent.
			n, all := sv.sawHeader(c.header)
			if n == 0 || len(all) != 1 {
				t.Fatalf("want every request to carry the %s header, got %d of it and %d distinct values", c.name, n, len(all))
			}
		})
	}
}

// Class credential-sent-to-another-host: a redirect off the configured API
// host is refused before it is followed, from the CLI, sync and MCP alike.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvToken)
	redirect := e.run(t, "ticket-events", bbMessage, "--json")
	sv.mu.Lock()
	sv.redirectAccounts = true
	sv.mu.Unlock()
	syncOut := e.run(t, "sync", "--resources", "accounts", "--json")
	e.mcpCalls(t, []map[string]any{
		{"name": "ticket_events_check", "arguments": map[string]any{"messageId": bbMessage}},
		{"name": "accounts_list", "arguments": map[string]any{}},
	})
	sv.mu.Lock()
	n := sv.offHost
	var redirected int
	for _, m := range sv.methods {
		if strings.Contains(m, "/vendor-api/alerting/debug/") || strings.HasSuffix(m, "/vendor-api/organization/accounts") {
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
	for where, out := range map[string]string{"ticket-events": redirect, "sync": syncOut} {
		if !strings.Contains(out, "leave the configured Gradient host") {
			t.Fatalf("%s: want the pin refusal, got:\n%.1500s", where, out)
		}
	}
}

// mcp-endpoint-write-confirm and mcp-mirror-write-confirm: every request the
// MCP server and its mirrored CLI children send to the API is a GET while no
// call sets confirm, though the battery calls every write tool; each write
// answers with a preview.
func TestMCPSendsOnlyReadsWithoutConfirm(t *testing.T) {
	// The quiet fake: the reads must answer their data, which an answer that
	// echoes the held token never does (withheld whole by contract).
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
			t.Errorf("%s without confirm did not return a preview:\n%.1500s", call["name"], r)
		}
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
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvToken)
	update := func(confirm any) []map[string]any {
		return []map[string]any{{"name": "mappings_update", "arguments": map[string]any{"serviceId": bbService, "name": "HQ map", "confirm": confirm}}}
	}
	e.mcpPhases(t, update("true"))
	sv.mu.Lock()
	for _, m := range sv.methods {
		if !strings.HasPrefix(m, "GET ") {
			t.Errorf("confirm \"true\" (a string) sent %s", m)
		}
	}
	sv.mu.Unlock()
	e.mcpPhases(t, update(true))
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var sent bool
	for _, m := range sv.methods {
		sent = sent || m == "PATCH /api/vendor-api/organization/services/"+bbService
	}
	if !sent {
		t.Fatalf("confirm true did not send the PATCH; requests: %v", sv.methods)
	}
}

// dry-run-sends-nothing: a --dry-run write never reaches the API, from a cold
// start with GRADIENT_TOKEN or the key pair in the environment.
func TestDryRunSendsNothing(t *testing.T) {
	for _, mode := range []credMode{credEnvToken, credPair} {
		sv := echoServer(t)
		e := newEnv(t, sv, mode)
		counts, _ := e.countsFiles(t)
		for _, args := range [][]string{
			{"mappings", "update", bbService, "--name", "Dry Co", "--dry-run"},
			{"mappings", "create", "--body-json", `[{"id":"map-9"}]`, "--dry-run"},
			{"mappings", "update-bulk", "--body-json", `[{"id":"map-9"}]`, "--dry-run"},
			{"accounts", "create", "--body-json", `[{"id":"acct-9","name":"Dry Co"}]`, "--dry-run"},
			{"accounts", "update", "--body-json", `[{"id":"acct-9","name":"Dry Co"}]`, "--dry-run"},
			{"accounts", "update-one", bbAccount, "--name", "Dry Co", "--dry-run"},
			{"services", "create", "--name", "Dry svc", "--description", "d", "--category", "Backup", "--subcategory", "M365", "--dry-run"},
			{"billing", bbService, "--account-id", bbAccount, "--unit-count", "3", "--dry-run"},
			{"alerting", bbAccount, "--alert-id", "a-1", "--title", "Dry", "--description", "d", "--dry-run"},
			{"alert", "send", "--account", bbAccount, "--title", "Dry", "--description", "d", "--dry-run"},
			{"usage", "push", "--file", counts, "--dry-run"},
			{"integration", "update-status", "--status", "active", "--dry-run"},
			{"vendor", "update", "--name", "Dry Vendor", "--dry-run"},
		} {
			// Each run reaches its preview, so the count below is not vacuous
			// (a flag error would send nothing either).
			if out := e.run(t, args...); !strings.Contains(out, "dry_run") && !strings.Contains(out, "would ") {
				t.Errorf("%v (mode %d) printed no preview:\n%.1500s", args, mode, out)
			}
		}
		sv.mu.Lock()
		if len(sv.methods) != 0 {
			t.Errorf("--dry-run (mode %d) sent %v", mode, sv.methods)
		}
		sv.mu.Unlock()
		// Control: the same write without --dry-run reaches the fake.
		e.run(t, "mappings", "update", bbService, "--name", "Dry Co")
		sv.mu.Lock()
		if len(sv.methods) != 1 || sv.methods[0] != "PATCH /api/vendor-api/organization/services/"+bbService {
			t.Errorf("control (mode %d): want the one PATCH, got %v", mode, sv.methods)
		}
		sv.mu.Unlock()
	}
}
