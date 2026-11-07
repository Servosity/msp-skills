// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package liongardblackbox runs the built CLI and MCP binaries against a
// Liongard fake that echoes every credential it knows back in every
// encoding it can, whole and split across lines and adjacent values, and
// checks what a reviewer cannot read off the code (liongard handfixes
// output-chokepoint, store-never-keeps-secrets, no-response-cache,
// no-credential-in-files, response-credentials-held, api-origin-pinned,
// dry-run-sends-nothing, mcp-endpoint-write-confirm,
// mcp-mirror-write-confirm, composed-roar-auth-from-id-and-secret):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the X-ROAR-API-KEY value from LIONGARD_API_KEY, from
//     the credential store (auth set-token) or derived from the
//     LIONGARD_ACCESS_KEY_ID + LIONGARD_ACCESS_KEY_SECRET pair (and either
//     half of the pair), the X-Auth-Token session token
//     (LIONGARD_ENDPOINTS_SESSION_TOKEN_AUTH), the MCP HTTP token, a
//     credential-named flag value, a log-in password, and the credentials a
//     response carries (a new access key's secret, a log-in session token);
//   - no file the program writes, except the 0600 credential store, holds
//     one either (the local store, receipts, exports, delivered files, the
//     feedback log, the learn journal);
//   - no credential leaves for a host other than the configured one by
//     redirect;
//   - the MCP server sends a write (a POST, PUT or DELETE, or the GET that
//     starts an inspection run) only when the call sets confirm to the
//     boolean true, and --dry-run sends nothing at all.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder.
package liongardblackbox

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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	bbAPIKey       = "Qx7fb3" + "189bQ6u3sZxW4o1n7R8v52Kd"   // LIONGARD_API_KEY (the X-ROAR-API-KEY value itself)
	bbKeyID        = "Ak9e3a" + "7c1d5b2f8a46Zq"             // LIONGARD_ACCESS_KEY_ID
	bbKeySecret    = "Sv2b6f" + "4a1c8e7305abXw"             // LIONGARD_ACCESS_KEY_SECRET
	bbSession      = "Tn8c1e4a7d2b9f063Yh"                   // LIONGARD_ENDPOINTS_SESSION_TOKEN_AUTH (X-Auth-Token)
	bbStored       = "Rk1c3e9b05d2a4f68e1Pm"                 // auth set-token (stdin), the credential store
	bbArgvSec      = "-Gv1c3e7a5Jq"                          // --client-secret on the command line (starts with "-")
	bbLoginPw      = "Lp2f7b4c9eWz0"                         // authentication log-in --password
	bbMCPLoginPw   = "Mq81d3c6Vb"                            // a log-in password an MCP call previews (never sent)
	bbShortResp    = "Sh0rtPw"                               // a 7-character credential a response carries
	bbBOMResp      = "Bq6a1cXr9Tz"                           // a credential in a BOM-prefixed response
	bbGzipResp     = "Gz4f2a9c1eNk"                          // a credential in a gzip-encoded response
	bbNewKeySecret = "Nk0b7e" + "3d5a9c1fUy"                 // the secret POST /access-keys returns for a new key
	bbLoginToken   = "Lt6d0a" + "2e8c4f71Hs"                 // the session token POST /authentication/login returns
	bbHTTPToken    = "Hp7Kq2" + "Wm9Xr4Tz8Vb3Nc6Yd1Fg5Jh0Ls" // LIONGARD_MCP_HTTP_TOKEN
	bbEnvID        = "42"                                    // an environment ID
	bbLaunchpoint  = "1203"                                  // a launchpoint ID
	bbTask         = "3301"                                  // a task (alert) ID the fake redirects off-host
	bbDenied       = "40199"                                 // an environment ID the fake answers 401
	// bbStaleLaunchpoint is liongardRecord's ID: as a launchpoint it was never
	// inspected, so run-stale targets it.
	bbStaleLaunchpoint = "42"
)

// kitNotice starts every notice the shared credential kit writes in place of
// a unit it withholds.
const kitNotice = "[credkit: withheld from"

// bbPairKey is the X-ROAR-API-KEY header the CLI derives from the key pair:
// base64("<id>:<secret>"). The fake receives it on the wire, so it echoes it
// like any other credential.
var bbPairKey = base64.StdEncoding.EncodeToString([]byte(bbKeyID + ":" + bbKeySecret))

func secrets() []string {
	return []string{bbAPIKey, bbKeyID, bbKeySecret, bbPairKey, bbSession, bbStored, bbArgvSec, bbLoginPw, bbMCPLoginPw,
		bbShortResp, bbBOMResp, bbGzipResp, bbNewKeySecret, bbLoginToken, bbHTTPToken}
}

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "liongard-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "liongard-cli"+exe)
	bbMCP = filepath.Join(dir, "liongard-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/liongard-cli", bbMCP: "../../cmd/liongard-mcp"} {
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
	headers map[string]int // X-ROAR-API-KEY header values the API received
	// sessions is every request that carried X-Auth-Token, as
	// "METHOD /path value".
	sessions []string
	// redirectEnvironments sends the environments list (the syncable
	// resource every novel joins on) to the other host too, so sync meets a
	// redirect.
	redirectEnvironments bool
	// quiet answers every list read with the documented record shape and no
	// echo, so a control test can prove a write runs from a complete copy.
	quiet bool
}

// quietRecord carries every field the store-backed commands need, as the
// Liongard v1 schemas document them (Launchpoints, Timeline, Agents).
const quietRecord = `"Inspector":{"ID":8},"LastScheduledRun":"2026-09-01T10:00:00Z","LastAdHocRun":null,"TotalScheduledRuns":3,"TotalAdHocRuns":0,"FinishedAt":"2026-10-21T10:05:00Z","LastHeartbeat":"2026-10-21T10:00:00Z"`

// tlsServer starts an HTTP/2 TLS fake.
func tlsServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// liongardRecord is one record, shaped so the store and the novel commands
// (drift, health, launchpoints stale and run-stale, environments overview)
// have something real to read: it is an environment, and as a launchpoint it
// belongs to that environment and was never inspected (the timeline names
// another launchpoint), so run-stale has a target.
const liongardRecord = `"ID":42,"Name":"HQ-Record","Description":"HQ environment","Alias":"HQ-Launchpoint","Status":1,"Visible":true,"Environment":{"ID":42,"Name":"HQ-Record"},"Launchpoint":{"ID":7777},"System":{"ID":3454},"CreatedOn":"2026-10-20T10:00:00Z","UpdatedOn":"2026-10-21T10:00:00Z"`

// listPaths are the Liongard v1 reads that answer a bare JSON array.
var listPaths = map[string]bool{
	"/access-keys": true, "/agents": true, "/detections": true, "/environments": true, "/inspectors": true,
	"/launchpoints": true, "/logs": true, "/metrics": true, "/metrics/bulk": true, "/systems": true,
	"/tasks": true, "/timeline": true, "/users": true,
}

// isWrite reports a request that changes Liongard data: every method but GET,
// and the GET that starts an inspection (/launchpoints/{id}/run).
var runPath = regexp.MustCompile(`^/api/v1/launchpoints/[^/]+/run$`)

func isWrite(m string) bool {
	method, path, _ := strings.Cut(m, " ")
	return method != http.MethodGet || runPath.MatchString(path)
}

func echoServer(t *testing.T) *server {
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
		echo := echoObject(sent)
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		if h := r.Header.Get("X-ROAR-API-KEY"); h != "" {
			sv.headers[h]++
		}
		if h := r.Header.Get("X-Auth-Token"); h != "" {
			sv.sessions = append(sv.sessions, r.Method+" "+r.URL.Path+" "+h)
		}
		redirectEnvironments := sv.redirectEnvironments
		quiet := sv.quiet
		sv.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		// A generous advertised budget: the client paces an unknown server at
		// 2 requests a second until its X-Ratelimit headers name one, which
		// stretched every multi-resource sync here by seconds. Nothing here
		// asserts pacing, and the values hold no credential.
		w.Header().Set("X-Ratelimit-Remaining", "1000000")
		w.Header().Set("X-Ratelimit-Reset", "60")
		p := strings.TrimPrefix(r.URL.Path, "/api/v1")
		get := r.Method == http.MethodGet
		if quiet && get && listPaths[p] {
			fmt.Fprintf(w, `[{%s,%s}]`, liongardRecord, quietRecord)
			return
		}
		switch {
		case p == "/groups" || p == "/tasks" || strings.Contains(p, "/"+bbDenied):
			// Rejected (the groups and tasks lists, a denied environment): the
			// error repeats what it was sent. The tasks list is the exportable,
			// syncable resource no novel reads, so sync and export meet a 401.
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"status":401,"error":"invalid key %s","message":"bad key %s","echo":%s}`, strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case p == "/tasks/"+bbTask || (redirectEnvironments && p == "/environments"):
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case get && p == "/environments/"+bbEnvID:
			// environments get-environmentid: the echo split across values,
			// lines and spaces.
			fmt.Fprintf(w, `{%s,"split":%s}`, liongardRecord, splitObject(sent))
		case get && p == "/agents":
			// agents list (and agents offline): BOM-prefixed.
			fmt.Fprintf(w, "\xef\xbb\xbf"+`[{"ID":7,"Name":"HQ-Agent","Status":0,"Environment":{"ID":42,"Name":"HQ-Record"},"LastHeartbeat":"2026-10-01T10:00:00Z","note":%q,"password":%q}]`, bbBOMResp, bbBOMResp)
		case get && p == "/launchpoints":
			// launchpoints list (and stale, run-stale, health): the documented
			// launchpoint run fields (spec Launchpoint schema; last run long ago),
			// and a short credential under its credential name. The store keeps the record
			// with it redacted by name; an echo under another name would make
			// the shared credential kit refuse the record whole, and run-stale
			// would have no target.
			fmt.Fprintf(w, `[{%s,"Inspector":{"ID":8},"LastScheduledRun":"2026-09-01T10:00:00Z","LastAdHocRun":null,"TotalScheduledRuns":3,"TotalAdHocRuns":0,"password":%q}]`, liongardRecord, bbShortResp)
		case get && p == "/timeline":
			// The timeline names another launchpoint (7777), so the
			// launchpoint above was never inspected. It echoes nothing: a
			// timeline the kit refused would leave run-stale an incomplete
			// snapshot, which it rightly refuses to plan from.
			fmt.Fprintf(w, `[{%s,"FinishedAt":"2026-10-21T10:05:00Z"}]`, liongardRecord)
		case get && p == "/systems":
			// systems list: the one answer that echoes nothing, so the data
			// path itself (stdout, a file sink, an export, the store, MCP) is
			// proven to carry records. Under the shared credential kit every
			// unit that repeats a held value is withheld whole.
			fmt.Fprintf(w, `[{%s}]`, liongardRecord)
		case get && p == "/environments":
			// environments list, sync, export: a bare array, gzip-encoded, with
			// an access token, the echo, and a record whose identifier repeats
			// the credential it was sent.
			extra := ""
			if len(sent) > 0 {
				extra = fmt.Sprintf(`,{"ID":%q,"Name":"Echo-ID","Visible":true}`, sent[0])
			}
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			fmt.Fprintf(zw, `[{%s,"accessToken":%q,"echo":%s}%s]`, liongardRecord, bbGzipResp, echoObject(append(sent, bbGzipResp)), extra)
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		case r.Method == http.MethodPost && p == "/access-keys":
			// A new access key comes back with its secret.
			fmt.Fprintf(w, `[{"AccessKeyId":"newKeyId-5c2e","AccessKeySecret":%q,"Status":1,"Name":"HQ-Record","echo":%s}]`, bbNewKeySecret, echo)
		case r.Method == http.MethodPost && p == "/authentication/login":
			// A log-in comes back with a session token.
			fmt.Fprintf(w, `{"token":%q,"account":{"user":{"ID":3,"Username":"hq.admin","FirstName":"HQ-Record"}},"echo":%s}`, bbLoginToken, echo)
		case !get:
			// Every write echoes what it was sent.
			fmt.Fprintf(w, `{%s,"echo":%s}`, liongardRecord, echo)
		case strings.HasSuffix(p, "/count"):
			fmt.Fprint(w, "1")
		case listPaths[p]:
			fmt.Fprintf(w, `[{%s,"echo":%s}]`, liongardRecord, echo)
		default:
			fmt.Fprintf(w, `{%s,"echo":%s}`, liongardRecord, echo)
		}
	}))
	sv.url = srv.URL + "/api/v1"
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.caFile = ca
	return sv
}

// sawHeader reports how many API requests carried X-ROAR-API-KEY: v, and the
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

// sawSession reports the requests that carried X-Auth-Token.
func (sv *server) sawSession() []string {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return append([]string(nil), sv.sessions...)
}

type bbEnv struct {
	home string
	env  []string
}

// credMode is where the process gets its X-ROAR-API-KEY.
type credMode int

const (
	credStored  credMode = iota // none in the environment: the battery runs on a key stored with auth set-token
	credEnvKey                  // LIONGARD_API_KEY
	credEnvBoth                 // LIONGARD_API_KEY and the key pair: LIONGARD_API_KEY wins, the pair is held unused
	credPair                    // LIONGARD_ACCESS_KEY_ID + LIONGARD_ACCESS_KEY_SECRET: the header is derived
)

// newEnv builds a clean environment against the fake with the credentials
// mode names. Every mode also carries the session token
// (LIONGARD_ENDPOINTS_SESSION_TOKEN_AUTH), which only the SessionTokenAuth
// operation (POST /authentication/verify-token) sends.
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
		"LIONGARD_BASE_URL=" + sv.url, "LIONGARD_CA_CERT=" + sv.caFile,
		"LIONGARD_CLI_PATH=" + bbCLI, "LIONGARD_MCP_HTTP_TOKEN=" + bbHTTPToken,
		"LIONGARD_ENDPOINTS_SESSION_TOKEN_AUTH=" + bbSession,
	}
	switch mode {
	case credEnvKey:
		env = append(env, "LIONGARD_API_KEY="+bbAPIKey)
	case credEnvBoth:
		env = append(env, "LIONGARD_API_KEY="+bbAPIKey, "LIONGARD_ACCESS_KEY_ID="+bbKeyID, "LIONGARD_ACCESS_KEY_SECRET="+bbKeySecret)
	case credPair:
		env = append(env, "LIONGARD_ACCESS_KEY_ID="+bbKeyID, "LIONGARD_ACCESS_KEY_SECRET="+bbKeySecret)
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
}

// importFile writes an import input whose one record repeats tok, for the
// import write path (the gradient suite's usage-push input has no Liongard
// counterpart; import is the file-driven write here).
func (e *bbEnv) importFile(t *testing.T, tok string) string {
	t.Helper()
	p := filepath.Join(e.home, "import.jsonl")
	if err := os.WriteFile(p, []byte(`{"Name":"Imported `+tok+`","Description":"HQ import"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// launchpointFlags is a launchpoint create with every flag the command
// requires.
func launchpointFlags(alias string) []string {
	return []string{"launchpoints", "launchpoint", "--alias", alias, "--environment-id", bbEnvID, "--agent-id", "7",
		"--inspector-id", "13", "--parent-id", "0", "--active-start", "2026/10/28 10:00:00", "--active-end", "2027/10/28 10:00:00",
		"--enabled", "true", "--visible", "true", "--freq-interval", "1", "--freq-type", "day", "--config", "{}", "--version", "1"}
}

// batteryParts names battery's parts. One battery is some 60 process runs,
// and the credential kit makes each one cost real CPU, so a test runs each
// part in its own home against the one fake, side by side with the others.
// The commands of a part keep their order (the clean delivery before the
// withheld one, sync before the store reads), and each part runs every
// command it ran when the battery was one list.
var batteryParts = []string{"responses", "writes", "store"}

// battery runs every output path, split into batteryParts' parts; tok is a
// credential the process holds (LIONGARD_API_KEY, the stored key, or the
// secret half of the pair). Paths are under e.home: call it with the env
// that runs the part.
func battery(t *testing.T, e *bbEnv, deliverFile, tok string) [][][]string {
	imp := e.importFile(t, tok)
	return [][][]string{{ // responses
		// Every output mode.
		{"environments", "get", "--json"},
		{"environments", "get"},
		{"environments", "get", "--csv"},
		{"environments", "get", "--plain"},
		{"environments", "get", "--json", "--receipt"},
		{"environments", "get", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		// The clean list first: a withheld delivery after it leaves the file as it was.
		{"systems", "systems", "--json", "--deliver", "file:" + deliverFile},
		{"environments", "get", "--json", "--deliver", "file:" + deliverFile},
		// Every response shape: BOM-prefixed, short credential, split echo,
		// 401 echo, refused redirect, a new key's secret, a log-in token.
		{"agents", "get", "--json"},
		{"launchpoints", "get", "--json"},
		{"environments", "get-environmentid", bbEnvID, "--json"},
		{"environments", "get-environmentid", bbDenied, "--json"},
		{"groups", "--json"},
		{"tasks", "get-alert", bbTask, "--json"},
		{"access-keys", "create", "--days-until-expiration", "30", "--json"},
		{"authentication", "log-in", "--username", "hq.admin", "--password", bbLoginPw, "--json"},
		// The one SessionTokenAuth operation: it sends X-Auth-Token.
		{"authentication", "verify-token", "--token", "123456", "--json"},
	}, { // writes
		// Writes the fake echoes, the inspection-run GET among them.
		{"environments", "update", bbEnvID, "--name", "HQ env", "--description", "d", "--parent", "1", "--status", "1", "--tier", "Core", "--json"},
		{"environments", "create", "--name", "HQ " + tok, "--description", "d", "--parent", "1", "--json"},
		{"launchpoints", "run", "run", bbLaunchpoint, "--json"},
		{"import", "environments", "--input", imp, "--json"},
		// Exports read the API, not the store.
		{"export", "environments", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
		{"export", "systems", "--format", "jsonl", "--output", filepath.Join(e.home, "export-systems.jsonl")},
		{"export", "environments", "--format", "json"},
		// A credential on the command line: a credential-named flag, an unknown
		// flag, a positional, feedback text.
		{"environments", "get", "--client-secret", bbArgvSec},
		{"environments", "get", "--json", "--" + tok},
		{"environments", "get-environmentid", tok, "--json"},
		{"feedback", "noted the key " + tok + " in a ticket"},
		// Previews: a dry run sends nothing.
		{"environments", "update", bbEnvID, "--name", "Echo " + tok, "--description", "d", "--parent", "1", "--status", "1", "--tier", "Core", "--dry-run"},
		{"environments", "create-bulk", "--body-json", `[{"Name":"Echo ` + tok + `"}]`, "--dry-run"},
		append(launchpointFlags("Echo "+tok), "--dry-run"),
		{"authentication", "log-in", "--username", "hq.admin", "--password", bbLoginPw, "--dry-run"},
		{"access-keys", "create", "--dry-run"},
		// Learnings, playbooks and learn events land in the local store's own tables.
		{"teach", "--query", "find environments " + tok, "--resource", bbEnvID, "--resource-type", "environments",
			"--notes", "rotated " + tok, "--playbook-notes", "use " + tok, "--no-validate"},
		// The same without a credential: the learning row is written.
		{"teach", "--query", "find environments weekly", "--resource", bbEnvID, "--resource-type", "environments",
			"--notes", "rotated weekly", "--no-validate"},
		{"doctor"},
		{"auth", "status"},
	}, { // store
		// The local store and the novel commands.
		{"sync", "--json"},
		{"drift", "--since", "3650d", "--json"},
		{"health", "--json"},
		{"launchpoints", "stale", "--json"},
		{"launchpoints", "run-stale", "--json"},
		{"launchpoints", "run-stale", "--confirm", "--json"},
		{"agents", "offline", "--json"},
		{"coverage", "--json"},
		{"environments", "overview", bbEnvID, "--full", "--json"},
		{"detections", "failures", "--since", "3650d", "--json"},
		{"search", "HQ", "--json"},
		{"search", "Echo", "--json", "--data-source", "local"},
	}}
}

// batteryEnvs is one env per battery part, each against sv in mode.
func batteryEnvs(t *testing.T, sv *server, mode credMode) []*bbEnv {
	t.Helper()
	envs := make([]*bbEnv, len(batteryParts))
	for i := range envs {
		envs[i] = newEnv(t, sv, mode)
	}
	return envs
}

// runBattery runs each battery part in its env, the parts side by side:
// before (when set), the part's commands, then the part's after (when set),
// all in the part's env. each sees every command's output. It returns when
// every part is done.
func runBattery(t *testing.T, envs []*bbEnv, tok string, before func(t *testing.T, e *bbEnv), each func(t *testing.T, args []string, out string), after map[string]func(t *testing.T, e *bbEnv)) {
	t.Helper()
	t.Run("battery", func(t *testing.T) {
		for i, part := range batteryParts {
			t.Run(part, func(t *testing.T) {
				t.Parallel()
				e := envs[i]
				if before != nil {
					before(t, e)
				}
				for _, args := range battery(t, e, filepath.Join(e.home, "deliver.json"), tok)[i] {
					each(t, args, e.run(t, args...))
				}
				if f := after[part]; f != nil {
					f(t, e)
				}
			})
		}
	})
}

// partEnv is the env that runs the named battery part.
func partEnv(envs []*bbEnv, part string) *bbEnv {
	return envs[slices.Index(batteryParts, part)]
}

// mcpReads are read-only tools across every response shape the fake has.
func mcpReads() []map[string]any {
	none := map[string]any{}
	read := func(id string, params map[string]any) map[string]any {
		return map[string]any{"name": "liongard_read", "arguments": map[string]any{"endpoint_id": id, "params": params}}
	}
	return []map[string]any{
		read("environments.get", none),
		read("systems.systems", none),
		read("agents.get", none),
		read("launchpoints.get", none),
		read("environments.get-environmentid", map[string]any{"EnvironmentID": bbEnvID}),
		read("groups.get", none),
		{"name": "liongard_search", "arguments": map[string]any{"query": "environments"}},
		{"name": "liongard_get", "arguments": map[string]any{"endpoint_id": "access-keys.get"}},
		{"name": "health", "arguments": none},
		{"name": "drift", "arguments": map[string]any{"since": "3650d"}},
		{"name": "launchpoints_stale", "arguments": none},
		{"name": "agents_offline", "arguments": none},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
	}
}

// mcpWrites is every write tool that takes confirm, called without it: each
// must answer with a preview and send nothing. liongard_execute previews as a
// confirmation_required object; the mirrored CLI writes (import,
// launchpoints_run_stale) and the recipe intent preview as the command's plan
// or --dry-run output followed by the gate's "Preview only: nothing was sent"
// note.
func mcpWrites() []map[string]any {
	execute := func(id string, params map[string]any) map[string]any {
		return map[string]any{"name": "liongard_execute", "arguments": map[string]any{"endpoint_id": id, "params": params}}
	}
	return []map[string]any{
		{"name": "import", "arguments": map[string]any{"resource": "environments"}},
		{"name": "launchpoints_run_stale", "arguments": map[string]any{}},
		{"name": "pivot_one_metric_across_all_systems", "arguments": map[string]any{"value": "Count"}},
		execute("environments.create", map[string]any{"Name": "Preview Co", "Description": "d"}),
		execute("environments.create-bulk", map[string]any{"body": []any{map[string]any{"Name": "Preview Co"}}}),
		execute("environments.update", map[string]any{"EnvironmentID": bbEnvID, "Name": "Preview Co"}),
		execute("environments.delete", map[string]any{"EnvironmentID": bbEnvID}),
		execute("access-keys.create", map[string]any{"daysUntilExpiration": "30"}),
		execute("access-keys.delete", map[string]any{"Access" + "KeyID": "12a3b4" + "5678cd912e3f4g"}),
		execute("agents.update", map[string]any{"AgentID": "7", "Name": "Preview agent"}),
		execute("agents.flush.job-queue", map[string]any{"AgentID": "7"}),
		execute("launchpoints.launchpoint", map[string]any{"Alias": "Preview lp"}),
		execute("launchpoints.run.run", map[string]any{"LaunchpointID": bbLaunchpoint}),
		execute("launchpoints.bulk-run", map[string]any{"LaunchPoints": []any{1203}}),
		execute("launchpoints.bulk-delete", map[string]any{}),
		execute("users.create", map[string]any{"Username": "preview"}),
		execute("metrics.create", map[string]any{"Name": "Preview metric"}),
		execute("authentication.log-in", map[string]any{"Username": "hq.admin", "Password": bbMCPLoginPw}),
		execute("authentication.verify-token", map[string]any{"Token": "123456"}),
	}
}

func mcpBattery() []map[string]any {
	return append(mcpReads(), mcpWrites()...)
}

// Class credential-persisted-in-files.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	envs := batteryEnvs(t, sv, credStored)
	// Each part's home stores the key with auth set-token first.
	setToken := func(t *testing.T, e *bbEnv) { e.runIn(t, bbStored+"\n", "auth", "set-token") }
	runBattery(t, envs, bbStored, setToken, func(*testing.T, []string, string) {}, map[string]func(*testing.T, *bbEnv){
		// The MCP server's store-backed tools read the synced store.
		"store": func(t *testing.T, e *bbEnv) { e.mcpCalls(t, mcpBattery()) },
	})
	if n, all := sv.sawHeader(bbStored); n == 0 {
		t.Fatalf("no request carried the stored key as X-ROAR-API-KEY (saw %d distinct values); the battery ran unauthenticated", len(all))
	}
	var scanned int
	for i, e := range envs {
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
					t.Errorf("%s: %s mode %v, want 0600", batteryParts[i], rel, info.Mode().Perm())
				}
				return nil
			}
			if rel == "import.jsonl" {
				return nil // the test's own input
			}
			assertNoEcho(t, batteryParts[i]+": file "+rel, string(data))
			return nil
		})
		if !sawCreds {
			t.Errorf("%s: auth set-token wrote no credential store", batteryParts[i])
		}
	}
	if scanned < 5 {
		t.Fatalf("only %d files written; the battery did not run", scanned)
	}
	// The battery really wrote data, so the scan above is not vacuous. (The
	// gradient suite also checked its usage and alert ledgers here; Liongard
	// has no ledger, so the export file and the feedback log stand in.)
	deliverFile := filepath.Join(partEnv(envs, "responses").home, "deliver.json")
	if b, err := os.ReadFile(deliverFile); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
		t.Errorf("%s: want the delivered record (err %v)", filepath.Base(deliverFile), err)
	}
	if b, err := os.ReadFile(filepath.Join(partEnv(envs, "writes").home, "export-systems.jsonl")); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
		t.Errorf("export-systems.jsonl: want the exported record (err %v):\n%.1500s", err, b)
	}
	var sawStore, sawLearning, sawFeedback bool
	for _, e := range envs {
		walkStoreEvidence(e.home, &sawStore, &sawLearning, &sawFeedback)
	}
	if !sawStore {
		t.Errorf("no local store holds a fetched record")
	}
	if !sawLearning {
		t.Errorf("no local store holds the taught learning; the teach run wrote nothing")
	}
	if !sawFeedback {
		t.Errorf("no feedback.jsonl holds the withheld feedback entry; the feedback run wrote nothing")
	}
}

// walkStoreEvidence reports what the battery's runs wrote under home: a
// local store holding a fetched record, the clean teach run's learning row,
// and the feedback entry withheld whole (its text repeats a held value).
func walkStoreEvidence(home string, sawStore, sawLearning, sawFeedback *bool) {
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".db") {
			*sawStore = *sawStore || bytes.Contains(b, []byte("HQ-Record"))
			// The clean teach run's learning row was written.
			*sawLearning = *sawLearning || bytes.Contains(b, []byte("rotated weekly"))
		}
		// The feedback entry was written, its text withheld whole (it repeats
		// a held value).
		*sawFeedback = *sawFeedback || (d.Name() == "feedback.jsonl" && bytes.Contains(b, []byte("withheld")))
		return nil
	})
}

// Class secret-in-output, once with LIONGARD_API_KEY (the key pair set too,
// and unused: LIONGARD_API_KEY wins) and once with the key pair alone (the
// CLI derives the header, which the fake then echoes). The session token is
// set in both and must reach the API only as X-Auth-Token on its own
// operation.
func TestNoCredentialInAnyOutput(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		mode   credMode
		tok    string // a credential the process holds, typed on the command line
		header string // the X-ROAR-API-KEY value the API must receive
	}{
		{"env key", credEnvBoth, bbAPIKey, bbAPIKey},
		{"key pair", credPair, bbKeySecret, bbPairKey},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			runBattery(t, batteryEnvs(t, sv, c.mode), c.tok, nil, func(t *testing.T, args []string, out string) {
				assertNoEcho(t, fmt.Sprintf("output of %v", args), out)
			}, map[string]func(*testing.T, *bbEnv){
				"responses": func(t *testing.T, e *bbEnv) {
					// The other half of the pair (unused when LIONGARD_API_KEY wins, and
					// held all the same) and the session token, typed where the program
					// repeats them: a path the fake echoes and a dry-run body.
					for _, held := range []string{bbKeyID, bbSession} {
						for _, args := range [][]string{
							{"environments", "get-environmentid", held, "--json"},
							{"environments", "create", "--name", "HQ " + held, "--description", "d", "--parent", "1", "--dry-run"},
						} {
							assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
						}
					}
					// Data that repeats no credential still prints as valid JSON; an
					// answer that echoes one is withheld whole, with the kit's notice.
					cmd := exec.Command(bbCLI, "systems", "systems", "--json")
					cmd.Env, cmd.Dir = e.env, e.home
					stdout, err := cmd.Output()
					if err != nil {
						t.Fatalf("systems systems --json: %v", err)
					}
					var doc any
					if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("HQ-Record")) {
						t.Fatalf("want valid JSON with the record, got:\n%.1500s", stdout)
					}
					cmd = exec.Command(bbCLI, "environments", "get", "--json")
					cmd.Env, cmd.Dir = e.env, e.home
					stdout, _ = cmd.Output()
					if !bytes.Contains(stdout, []byte(kitNotice)) {
						t.Fatalf("want the kit's withheld notice for the echoed answer, got:\n%.1500s", stdout)
					}
				},
				// The MCP server's store-backed tools read the synced store.
				"store": func(t *testing.T, e *bbEnv) {
					out := e.mcpCalls(t, mcpBattery())
					if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
						t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
					}
					assertNoEcho(t, "MCP session", out)
					if !strings.Contains(out, "HQ-Record") {
						t.Fatalf("MCP results lost the record itself:\n%.2000s", out)
					}
				},
			})
			// The fake echoed the header it really received, so the scans above
			// covered it; no other credential source was sent.
			n, all := sv.sawHeader(c.header)
			if n == 0 || len(all) != 1 {
				t.Fatalf("want every request to carry the %s header, got %d of it and %d distinct values", c.name, n, len(all))
			}
			// The session token went out (so the scans covered its echo), and
			// only on the SessionTokenAuth operation.
			sessions := sv.sawSession()
			if len(sessions) == 0 {
				t.Fatalf("no request carried X-Auth-Token; authentication verify-token did not run")
			}
			for _, s := range sessions {
				if s != "POST /api/v1/authentication/verify-token "+bbSession {
					t.Errorf("X-Auth-Token went out on another operation or with another value: %s", s)
				}
			}
		})
	}
}

// Class credential-sent-to-another-host: a redirect off the configured API
// host is refused before it is followed, from the CLI, sync and MCP alike.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvKey)
	redirect := e.run(t, "tasks", "get-alert", bbTask, "--json")
	sv.mu.Lock()
	sv.redirectEnvironments = true
	sv.mu.Unlock()
	syncOut := e.run(t, "sync", "--resources", "environments", "--json")
	e.mcpCalls(t, []map[string]any{
		{"name": "liongard_read", "arguments": map[string]any{"endpoint_id": "tasks.get-alert", "params": map[string]any{"TaskID": bbTask}}},
		{"name": "liongard_read", "arguments": map[string]any{"endpoint_id": "environments.get"}},
	})
	sv.mu.Lock()
	n := sv.offHost
	var redirected int
	for _, m := range sv.methods {
		if strings.HasSuffix(m, "/api/v1/tasks/"+bbTask) || strings.HasSuffix(m, "/api/v1/environments") {
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
	for where, out := range map[string]string{"tasks get-alert": redirect, "sync": syncOut} {
		if !strings.Contains(out, "leave the configured Liongard host") {
			t.Fatalf("%s: want the pin refusal, got:\n%.1500s", where, out)
		}
	}
}

// mcp-endpoint-write-confirm and mcp-mirror-write-confirm: every request the
// MCP server and its mirrored CLI children send to the API is a read (a GET
// other than the inspection-run GET) while no call sets confirm, though the
// battery calls every write tool; each write answers with a preview.
func TestMCPSendsOnlyReadsWithoutConfirm(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvKey)
	// The store holds a never-inspected launchpoint first, so run-stale's plan
	// has a target its preview must not run.
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
		// A preview that repeats a held argument (log-in's Password) is
		// withheld whole by the kit; what was sent is checked below either way.
		if r := replies[id]; !strings.Contains(r, "confirmation_required") && !strings.Contains(r, "Preview only: nothing was sent") && !strings.Contains(r, kitNotice) {
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
// still previews. Both gates: the endpoint tool (liongard_execute) and the
// mirrored write whose own --confirm flag the gate supplies
// (launchpoints run-stale).
func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	sv.quiet = true
	e := newEnv(t, sv, credEnvKey)
	e.run(t, "sync", "--json")
	update := func(confirm any) []map[string]any {
		return []map[string]any{
			{"name": "liongard_execute", "arguments": map[string]any{"endpoint_id": "environments.update",
				"params": map[string]any{"EnvironmentID": bbEnvID, "Name": "HQ env"}, "confirm": confirm}},
			{"name": "launchpoints_run_stale", "arguments": map[string]any{"confirm": confirm}},
		}
	}
	e.mcpPhases(t, update("true"))
	sv.mu.Lock()
	for _, m := range sv.methods {
		if isWrite(m) {
			t.Errorf("confirm \"true\" (a string) sent %s", m)
		}
	}
	sv.mu.Unlock()
	e.mcpPhases(t, update(true))
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var put, run bool
	for _, m := range sv.methods {
		put = put || m == "PUT /api/v1/environments/"+bbEnvID
		run = run || m == "GET /api/v1/launchpoints/"+bbStaleLaunchpoint+"/run"
	}
	if !put {
		t.Errorf("confirm true did not send the PUT; requests: %v", sv.methods)
	}
	if !run {
		t.Errorf("confirm true did not start the stale launchpoint's inspection run; requests: %v", sv.methods)
	}
}

// dry-run-sends-nothing: a --dry-run write never reaches the API, from a cold
// start with LIONGARD_API_KEY or the key pair in the environment.
func TestDryRunSendsNothing(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credEnvKey, credPair} {
		t.Run(fmt.Sprintf("mode %d", mode), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			imp := e.importFile(t, "dry")
			for _, args := range [][]string{
				{"environments", "update", bbEnvID, "--name", "Dry Co", "--description", "d", "--parent", "1", "--status", "1", "--tier", "Core", "--dry-run"},
				{"environments", "create", "--name", "Dry Co", "--description", "d", "--parent", "1", "--dry-run"},
				{"environments", "create-bulk", "--body-json", `[{"Name":"Dry Co"}]`, "--dry-run"},
				{"environments", "delete", bbEnvID, "--dry-run"},
				{"access-keys", "create", "--days-until-expiration", "30", "--dry-run"},
				{"access-keys", "delete", "12a3b45678cd912e3f4g", "--dry-run"},
				{"agents", "update", "7", "--name", "Dry agent", "--dry-run"},
				{"agents", "delete", "7", "--dry-run"},
				{"agents", "flush", "job-queue", "7", "--dry-run"},
				append(launchpointFlags("Dry lp"), "--dry-run"),
				{"launchpoints", "update", bbLaunchpoint, "--alias", "Dry lp", "--dry-run"},
				{"launchpoints", "run", "run", bbLaunchpoint, "--dry-run"},
				{"launchpoints", "bulk-run", "--launch-points", "[1203]", "--dry-run"},
				{"launchpoints", "run-stale", "--confirm", "--dry-run"},
				{"users", "create", "--username", "dry", "--email", "dry@example.com", "--dry-run"},
				{"metrics", "create", "--name", "Dry metric", "--inspector-id", "13", "--queries", "[]", "--dry-run"},
				{"authentication", "log-in", "--username", "hq.admin", "--password", "Dq4vX9pL2", "--dry-run"},
				{"authentication", "verify-token", "--token", "123456", "--dry-run"},
				{"import", "environments", "--input", imp, "--dry-run"},
			} {
				// Each run reaches its preview, so the count below is not vacuous
				// (a flag error would send nothing either).
				// A preview that repeats a held flag value (--password, --token)
				// is withheld whole by the kit: its notice is the preview.
				if out := e.run(t, args...); !strings.Contains(out, "dry_run") && !strings.Contains(out, "dry run") && !strings.Contains(out, "would ") && !strings.Contains(out, kitNotice) {
					t.Errorf("%v (mode %d) printed no preview:\n%.1500s", args, mode, out)
				}
			}
			sv.mu.Lock()
			if len(sv.methods) != 0 {
				t.Errorf("--dry-run (mode %d) sent %v", mode, sv.methods)
			}
			sv.mu.Unlock()
			// Control: the same write without --dry-run reaches the fake.
			e.run(t, "environments", "update", bbEnvID, "--name", "Dry Co", "--description", "d", "--parent", "1", "--status", "1", "--tier", "Core")
			sv.mu.Lock()
			if len(sv.methods) != 1 || sv.methods[0] != "PUT /api/v1/environments/"+bbEnvID {
				t.Errorf("control (mode %d): want the one PUT, got %v", mode, sv.methods)
			}
			sv.mu.Unlock()
		})
	}
}
