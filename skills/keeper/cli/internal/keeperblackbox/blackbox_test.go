// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package keeperblackbox runs the built CLI and MCP binaries against a
// Keeper Commander Service Mode fake that echoes every credential it knows back in every
// encoding it can, whole and split across lines and adjacent values, and
// checks what a reviewer cannot read off the code (keeper handfixes
// credkit-wiring, store-never-keeps-secrets, no-response-cache,
// no-credential-in-files, response-credentials-held, api-origin-pinned):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the bearer token from the environment or the
//     credential store, the MCP HTTP token, a credential-named flag value,
//     and the credentials a response carries;
//   - no file the program writes, except the 0600 credential store, holds
//     one either;
//   - no credential leaves for a host other than the configured one, by
//     redirect;
//   - the MCP server sends nothing but reads: GETs, and POSTs of Commander
//     commands on the read-only allowlist.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder. On a match the credential kit withholds
// the whole unit and writes a fixed notice: a server that echoes nothing shows the
// data still prints, and an echoing one shows the notice.
package keeperblackbox

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
	"sync/atomic"
	"testing"
	"time"
)

// bbWithheld is what the credential kit prints in place of a unit that carries a
// held value.
const bbWithheld = "[credkit: withheld from"

const (
	bbToken     = "Kq7Fb3R" + "m9Q6u3sZxW4o1n7R8v52LdTpYh" // KEEPER_API_KEY (random, as service-create makes)
	bbStored    = "Rv8kTq3Wm7Zp2Xn6Bs4Ly9HdGc"             // auth set-token (stdin), the credential store
	bbArgvSec   = "-Vq8Rk3Tm6Wp2Zx9Ny4"                    // --client-secret on the command line (starts with "-")
	bbShortResp = "Sh0rtPw"                                // a 7-character credential a response carries
	bbBOMResp   = "BomRespSecret-6a1c"                     // a credential in a BOM-prefixed response
	bbGzipResp  = "GzipRespToken-4f2a9c1e"                 // a credential in a gzip-encoded response
	bbHTTPToken = "Mh7Pq2T" + "x9Kv4Wr6Zn3Bs8Lc5Jd1Fy0GaQ" // KEEPER_MCP_HTTP_TOKEN
)

func secrets() []string {
	return []string{bbToken, bbStored, bbArgvSec, bbShortResp, bbBOMResp, bbGzipResp, bbHTTPToken}
}

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "keeper-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "keeper-cli"+exe)
	bbMCP = filepath.Join(dir, "keeper-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/keeper-cli", bbMCP: "../../cmd/keeper-mcp"} {
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
	methods []string
	mc      string                 // the managed company Commander's session is in ("" = MSP)
	honest  atomic.Bool            // a Service Mode that repeats nothing it was sent and carries no credential
	field   atomic.Pointer[string] // text a report shows in a field the CLI prints, in place of any echo
	apiKeys sync.Map               // every api-key header value seen
}

// show makes every report repeat v in a field the CLI prints.
func (sv *server) show(v string) { sv.field.Store(&v) }

// sawKey reports whether a request carried v as its api-key header.
func (sv *server) sawKey(v string) bool {
	_, ok := sv.apiKeys.Load(v)
	return ok
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

// commandOf is the Commander command a Service Mode request carries.
func commandOf(body []byte) string {
	var rq struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(body, &rq)
	return rq.Command
}

// report wraps rows as Service Mode wraps a --format=json command
// (keepercommander/service/util/parse_keeper_response.py).
func report(cmd string, rows ...string) string {
	return fmt.Sprintf(`{"status":"success","command":%q,"data":[%s]}`, strings.Fields(cmd)[0], strings.Join(rows, ","))
}

// echoText repeats every secret the request carried in every form, and split
// across a space: placed in fields the CLI shows.
func echoText(sent []string) string {
	var parts []string
	for _, x := range sent {
		parts = append(parts, echoForms(x)...)
		a, b := halves(x)
		parts = append(parts, a+" "+b)
	}
	return strings.Join(parts, " ")
}

func jsonStr(v string) string { b, _ := json.Marshal(v); return string(b) }

func jsonList(sent []string) string {
	var forms []string
	for _, x := range sent {
		forms = append(forms, echoForms(x)...)
	}
	b, _ := json.Marshal(forms)
	return string(b)
}

func echoServer(t *testing.T) *server {
	t.Helper()
	sv := &server{}
	// A second host the API redirects to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		sent := sentSecrets(r)
		if k := r.Header.Get("api-key"); k != "" {
			sv.apiKeys.Store(k, true)
		}
		honest := sv.honest.Load()
		if honest {
			sent = nil
		}
		echo := echoText(sent)
		if v := sv.field.Load(); v != nil {
			echo = *v
		}
		// withEcho is a record's own name, then what the server repeats.
		withEcho := func(name string) string { return strings.TrimSpace(name + " " + echo) }
		cmd := commandOf(body)
		sv.mu.Lock()
		sv.methods = append(sv.methods, strings.TrimSpace(r.Method+" "+r.URL.Path+" "+cmd))
		sv.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/health":
			if honest {
				fmt.Fprint(w, `{"status":"ok"}`)
				return
			}
			// BOM-prefixed, with a credential-named field (served in-process by the MCP server).
			fmt.Fprintf(w, "\xef\xbb\xbf"+`{"status":"ok","note":%s,"password":%s}`, jsonStr("HQ-Record "+bbBOMResp), jsonStr(bbBOMResp))
			return
		case "/api/v2/queue/status":
			// gzip-encoded, carrying two credentials in credential-named fields.
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			if honest {
				fmt.Fprintf(zw, `{"success":true,"queue_size":0,"active_requests":0,"completed_requests":1,"currently_processing":%s,"worker_running":true}`, jsonStr(withEcho("HQ-Record")))
			} else {
				fmt.Fprintf(zw, `{"success":true,"queue_size":0,"active_requests":0,"completed_requests":1,"currently_processing":%s,"worker_running":true,"access_token":%s,"password":%s}`,
					jsonStr("HQ-Record "+echoText(append(sent, bbGzipResp, bbShortResp))), jsonStr(bbGzipResp), jsonStr(bbShortResp))
			}
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
			return
		case "/api/v1/executecommand":
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":"error","error":"not found"}`))
			return
		}
		f := strings.Fields(cmd)
		if len(f) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		a, b := "", ""
		if len(sent) > 0 {
			a, b = halves(sent[0])
		}
		addons := "[]"
		if len(sent) > 0 {
			addons = jsonList(append(sent, a+"\n"+b))
		}
		switch {
		case f[0] == "switch-to-msp":
			// Commander's one global context (commands/msp.py): "Already
			// MSP" when no company is selected.
			sv.mu.Lock()
			in := sv.mc
			sv.mc = ""
			sv.mu.Unlock()
			if in == "" {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, `{"status":"error","error":%s}`, jsonStr("Unexpected error: switch-to-msp: Already MSP"))
				return
			}
			fmt.Fprint(w, `{"status":"success","command":"switch-to-msp","message":"Switched back to MSP","data":null}`)
		case f[0] == "switch-to-mc":
			sv.mu.Lock()
			in := sv.mc
			if in == "" && len(f) > 1 {
				sv.mc = f[1]
			}
			sv.mu.Unlock()
			if in != "" {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, `{"status":"error","error":%s}`, jsonStr("Unexpected error: switch-to-mc: Already switched to Managed Company id="+in))
				return
			}
			fmt.Fprintf(w, `{"status":"success","command":"switch-to-mc","message":%s,"data":null}`, jsonStr(withEcho("Switched to MC 'HQ-Record'")))
		case f[0] == "msp-info" && strings.Contains(cmd, "--pricing"):
			if honest {
				fmt.Fprint(w, report(cmd, `{"category":"Product","name":"HQ-Record","code":"business","price":"$1"}`))
				return
			}
			// BOM-prefixed, and a credential-named field: the whole answer is refused.
			fmt.Fprint(w, "\xef\xbb\xbf"+report(cmd, fmt.Sprintf(`{"category":"Product","name":%s,"code":"business","price":"$1","password":%s}`, jsonStr(bbBOMResp), jsonStr(bbBOMResp))))
		case f[0] == "msp-info" && strings.Contains(cmd, "--restriction"):
			// gzip-encoded; the credential sits in a field the CLI drops and in one it shows.
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			if honest {
				fmt.Fprint(zw, report(cmd, fmt.Sprintf(`{"Permit Name":"Allowed MSP Products","Value":%s}`, jsonStr(withEcho("HQ-Record")))))
			} else {
				fmt.Fprint(zw, report(cmd, fmt.Sprintf(`{"Permit Name":"Allowed MSP Products","Value":%s,"accessToken":%s}`, jsonStr("HQ-Record "+bbGzipResp+" "+echoText(append(sent, bbGzipResp))), jsonStr(bbGzipResp))))
			}
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		case f[0] == "msp-info":
			// The record's own name stays visible; the echo, a split across
			// adjacent values and one across a line sit in other fields.
			fmt.Fprint(w, report(cmd, fmt.Sprintf(`{"company_id":4421,"company_name":"HQ-Record","node":%s,"plan":%s,"storage":%s,"addons":%s,"allocated":5,"active":3}`,
				jsonStr(echo), jsonStr(a), jsonStr(b), addons)))
		case f[0] == "msp-billing-report":
			fmt.Fprint(w, report(cmd, fmt.Sprintf(`{"company":%s,"company_id":4421,"product":"business","licenses":3,"rate":"1.00"}`, jsonStr(withEcho("HQ-Record")))))
		case f[0] == "enterprise-info" && strings.Contains(cmd, "--teams"):
			http.Redirect(w, r, other.URL+"/api/v1/executecommand", http.StatusTemporaryRedirect)
		case f[0] == "enterprise-info" && strings.Contains(cmd, "--roles") && strings.Contains(cmd, "enforcements"):
			fmt.Fprint(w, report(cmd, fmt.Sprintf(`{"role_id":1,"name":"Admins","admin":true,"enforcements":{"require_two_factor":false,"restrict_ip_addresses":%s}}`, jsonStr(echo))))
		case f[0] == "enterprise-info" && strings.Contains(cmd, "--roles"):
			fmt.Fprint(w, report(cmd, fmt.Sprintf(`{"role_id":1,"name":%s,"admin":true,"node":%s,"user_count":1}`, jsonStr(withEcho("HQ-Record")), jsonStr(a+"\n"+b))))
		case f[0] == "enterprise-info" && strings.Contains(cmd, "--users"):
			fmt.Fprint(w, report(cmd, fmt.Sprintf(`{"user_id":1,"email":"hq@example.com","name":%s,"status":"Active","roles":["Admins"],"node":%s}`, jsonStr(withEcho("HQ-Record")), jsonStr(a)),
				`{"user_id":2,"email":"two@example.com","name":"Two","status":"Invited","2fa_enabled":true}`))
		case f[0] == "enterprise-info":
			if honest {
				fmt.Fprint(w, report(cmd, `{"node_id":1,"name":"HQ-Record","user_count":2}`))
				return
			}
			// A short credential in a credential-named field: refused whole.
			fmt.Fprint(w, report(cmd, fmt.Sprintf(`{"node_id":1,"name":"HQ-Record","user_count":2,"note":%s,"password":%s}`, jsonStr(bbShortResp), jsonStr(bbShortResp))))
		case f[0] == "user-report":
			fmt.Fprint(w, report(cmd, fmt.Sprintf(`{"email":"hq@example.com","name":%s,"status":"Active","last_login":"> 90 DAYS AGO"}`, jsonStr(withEcho("HQ-Record")))))
		case f[0] == "security-audit-report" && strings.Contains(cmd, "--breachwatch"):
			fmt.Fprint(w, report(cmd, fmt.Sprintf(`{"email":"hq@example.com","name":%s,"at_risk":1,"passed":2,"ignored":0}`, jsonStr(withEcho("HQ-Record")))))
		case f[0] == "security-audit-report":
			fmt.Fprint(w, report(cmd, fmt.Sprintf(`{"email":"hq@example.com","name":%s,"weak":2,"fair":0,"medium":1,"strong":3,"reused":1,"unique":5,"securityScore":55,"twoFactorChannel":"Off","node":%s}`, jsonStr("HQ-Record"), jsonStr(echo))))
		case f[0] == "audit-report" && strings.Contains(cmd, "--created yesterday"):
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"status":"error","error":%s}`, jsonStr(strings.TrimSpace("Please provide a valid api key "+strings.Join(sent, " ")+" "+echo)))
		case f[0] == "audit-report":
			fmt.Fprint(w, report(cmd, fmt.Sprintf(`{"created":"2026-10-20T10:00:00Z","audit_event_type":"login","username":"hq@example.com","ip_address":"192.0.2.1","message":%s}`, jsonStr(withEcho("HQ-Record")))))
		default:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"status":"error","error":"Not permitted to perform this function"}`))
		}
	}))
	sv.url = srv.URL
	sv.caFile = pemFile(t, srv)
	return sv
}

// shortTemp is an empty directory. Not t.TempDir: its path carries the test's name,
// which can share a piece with the values these tests hold.
func shortTemp(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "keeper-kit-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// pemFile writes srv's certificate for KEEPER_CA_CERT.
func pemFile(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	ca := filepath.Join(shortTemp(t), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return ca
}

type bbEnv struct {
	home string
	env  []string
}

func newEnv(t *testing.T, sv *server, withEnvToken bool) *bbEnv {
	t.Helper()
	home := shortTemp(t)
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"KEEPER_BASE_URL=" + sv.url, "KEEPER_CA_CERT=" + sv.caFile,
		"KEEPER_CLI_PATH=" + bbCLI, "KEEPER_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}
	if withEnvToken {
		env = append(env, "KEEPER_API_KEY="+bbToken)
	}
	return &bbEnv{home: home, env: env}
}

// with is a copy of e with these variables added.
func (e *bbEnv) with(extra ...string) *bbEnv {
	return &bbEnv{home: e.home, env: append(append([]string{}, e.env...), extra...)}
}

// without is a copy of e with these variables removed.
func (e *bbEnv) without(names ...string) *bbEnv {
	out := &bbEnv{home: e.home}
	for _, kv := range e.env {
		name, _, _ := strings.Cut(kv, "=")
		keep := true
		for _, n := range names {
			keep = keep && n != name
		}
		if keep {
			out.env = append(out.env, kv)
		}
	}
	return out
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
	waitDone := make(chan struct{})
	go func() { _ = cmd.Wait(); close(waitDone) }()
	id := 0
	deadline := time.Now().Add(120 * time.Second)
wait:
	for _, calls := range phases {
		for _, c := range calls {
			id++
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": c})
		}
		for time.Now().Before(deadline) && strings.Count(out.String(), `"id":`) < id+1 {
			select {
			case <-waitDone: // it refused to start, or stopped
				break wait
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	_ = stdin.Close()
	select {
	case <-waitDone:
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
	}
	return out.String() + "\n" + errb.String()
}

// mcpRaw runs one MCP stdio session with raw JSON-RPC messages after the
// handshake and returns everything the server wrote.
func (e *bbEnv) mcpRaw(t *testing.T, msgs []map[string]any) string {
	t.Helper()
	var in bytes.Buffer
	enc := json.NewEncoder(&in)
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}})
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	for _, m := range msgs {
		_ = enc.Encode(m)
	}
	cmd := exec.Command(bbMCP)
	cmd.Env, cmd.Dir = e.env, e.home
	cmd.Stdin = &in
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
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
		// Pieces under 8 characters are out of contract (section 6, item 6).
		if a, b := halves(s); len(a) >= 8 && strings.Contains(text, a) && strings.Contains(text, b) {
			t.Errorf("%s holds both halves of %q", where, s)
		}
	}
}

// noPiece fails on the value or any 8-character piece of it.
func noPiece(t *testing.T, where, text, value string) {
	t.Helper()
	for i := 0; i+8 <= len(value); i++ {
		if strings.Contains(text, value[i:i+8]) {
			t.Errorf("%s shows a piece of the credential (%q):\n%.1200s", where, value[i:i+8], text)
			return
		}
	}
}

// battery runs every output path; tok is the credential the process holds
// (the environment key, or the stored one when the environment has none).
func battery(e *bbEnv, deliverFile, tok string) [][]string {
	return [][]string{
		{"companies", "list", "--json"},
		{"companies", "list"},
		{"companies", "list", "--csv"},
		{"companies", "list", "--plain"},
		{"companies", "list", "--json", "--receipt"},
		{"companies", "list", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		{"companies", "list", "--json", "--deliver", "file:" + deliverFile},
		{"companies", "get", "4421", "--json"},
		{"products", "list", "--json"},
		{"permits", "list", "--json"},
		{"billing", "report", "--by-company", "--json"},
		{"users", "list", "--json"},
		{"users", "list", "--mc", "4421", "--json"},
		{"users", "last-login", "--days", "90", "--json"},
		{"nodes", "list", "--json"},
		{"teams", "list", "--json"},
		{"roles", "list", "--json"},
		{"security-audit", "report", "--mc", "4421", "--json"},
		{"breachwatch", "summary", "--json"},
		{"audit-trail", "events", "--json"},
		{"audit-trail", "events", "--created", "yesterday", "--json"},
		{"audit-trail", "summary", "--json"},
		{"posture", "report", "--json"},
		{"posture", "report", "--days", "90", "--json"},
		{"posture", "runs", "--json"},
		{"posture", "diff", "--json"},
		{"events", "list", "--json"},
		{"users", "find", "hq", "--json"},
		{"users", "hygiene", "--json"},
		{"users", "risk", "--json"},
		{"allowlist", "--json"},
		{"queue", "--json"},
		{"health", "--json"},
		// Refused before any request.
		{"companies", "get", tok, "--json"},
		{"audit-trail", "events", "--username", tok, "--json"},
		{"users", "list", "--mc", tok},
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
		{"name": "companies_list", "arguments": map[string]any{}},
		{"name": "users_list", "arguments": map[string]any{"mc": "4421"}},
		{"name": "security_audit_report", "arguments": map[string]any{}},
		{"name": "breachwatch_summary", "arguments": map[string]any{}},
		{"name": "audit_trail_events", "arguments": map[string]any{}},
		{"name": "permits_list", "arguments": map[string]any{}},
		{"name": "posture_report", "arguments": map[string]any{"confirm": true}}, // runs only with confirm
		{"name": "users_risk", "arguments": map[string]any{}},
		{"name": "events_list", "arguments": map[string]any{}},
		{"name": "queue_get-status", "arguments": map[string]any{}},
		{"name": "health_get", "arguments": map[string]any{}},
	}
}

// Class credential-persisted-in-files.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	t.Parallel()
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
	scan := func() {
		t.Helper()
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
	}
	scan()
	if scanned < 5 {
		t.Fatalf("only %d files written; the battery did not run", scanned)
	}
	if !sawCreds {
		t.Errorf("auth set-token wrote no credential store")
	}
	// Against a server that echoes nothing the same writes do land, so the scan is not
	// vacuous; and then it runs again, over what landed.
	sv.honest.Store(true)
	e.run(t, "companies", "list", "--json", "--deliver", "file:"+deliverFile)
	e.run(t, "posture", "report", "--json")
	e.run(t, "teach", "--query", "find users", "--resource", "123", "--resource-type", "users", "--notes", "rotated on schedule", "--no-validate")
	scan()
	if b, err := os.ReadFile(deliverFile); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
		t.Errorf("%s: want the delivered record (err %v)", filepath.Base(deliverFile), err)
	}
	var sawStore, sawLearning bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(d.Name(), ".db") {
			if b, rerr := os.ReadFile(path); rerr == nil {
				sawStore = sawStore || bytes.Contains(b, []byte("HQ-Record"))
				sawLearning = sawLearning || bytes.Contains(b, []byte("rotated on schedule"))
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
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, args := range battery(e, deliverFile, bbToken) {
		assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
	}
	// A report that repeats a held value is withheld whole, with the kit's notice.
	if out := e.run(t, "companies", "list", "--json"); !strings.Contains(out, bbWithheld) {
		t.Errorf("want the kit's notice for an echoing report, got:\n%.1500s", out)
	}
	out := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
		t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP session", out)
	if !strings.Contains(out, bbWithheld) {
		t.Errorf("want the kit's notice in the MCP session, got:\n%.2000s", out)
	}
	// A server that echoes nothing: the data still prints, and JSON output stays
	// valid JSON.
	sv.honest.Store(true)
	cmd := exec.Command(bbCLI, "companies", "list", "--json")
	cmd.Env, cmd.Dir = e.env, e.home
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("companies list --json: %v", err)
	}
	var doc any
	if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("HQ-Record")) || bytes.Contains(stdout, []byte(bbWithheld)) {
		t.Fatalf("want valid JSON with the record and no notice, got:\n%.1500s", stdout)
	}
	honest := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(honest, "HQ-Record") {
		t.Fatalf("MCP results lost the record itself:\n%.2000s", honest)
	}
	if strings.Contains(honest, bbWithheld) {
		t.Errorf("a server that echoes nothing still had something withheld:\n%.3000s", honest)
	}
	assertNoEcho(t, "honest MCP session", honest)
}

// Class credential-sent-to-another-host: a redirect off the configured
// Service Mode host never reaches it.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	redirect := e.run(t, "teams", "list", "--json")
	e.mcpCalls(t, []map[string]any{{"name": "teams_list", "arguments": map[string]any{}}})
	sv.mu.Lock()
	n := sv.offHost
	sv.mu.Unlock()
	if n != 0 {
		t.Fatalf("the other host was contacted %d time(s)", n)
	}
	if !strings.Contains(redirect, "leave the configured Keeper Service Mode host") {
		t.Fatalf("redirect: want the pin refusal, got:\n%.1500s", redirect)
	}
}

// allowedFirstTokens mirrors internal/keeper.AllowedCommands.
var allowedFirstTokens = map[string]bool{"msp-info": true, "msp-billing-report": true, "enterprise-info": true, "user-report": true,
	"security-audit-report": true, "audit-report": true, "switch-to-mc": true, "switch-to-msp": true}

// The connector is read-only: every request the CLI and the MCP server (and
// its mirrored CLI children) send is a GET or an allowlisted Commander report,
// and no request ever carries a write or host-file flag.
func TestOnlyReadsAreSent(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	sv.honest.Store(true)
	if out := e.mcpCalls(t, mcpBattery()); !strings.Contains(out, "HQ-Record") {
		t.Fatalf("the MCP battery fetched nothing:\n%.2000s", out)
	}
	sv.honest.Store(false)
	e.mcpCalls(t, mcpBattery())
	for _, args := range battery(e, filepath.Join(e.home, "deliver.json"), bbToken) {
		e.run(t, args...)
	}
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if len(sv.methods) == 0 {
		t.Fatal("no request reached the fake")
	}
	sawCommand := false
	for _, m := range sv.methods {
		if strings.HasPrefix(m, "GET ") {
			continue
		}
		f := strings.Fields(m)
		if len(f) < 3 || f[0] != "POST" || f[1] != "/api/v1/executecommand" || !allowedFirstTokens[f[2]] {
			t.Errorf("sent %q", m)
			continue
		}
		sawCommand = true
		for _, bad := range []string{"--output", "--save", "--attempt-fix", "--record-details", "--apply-action", "--force"} {
			if strings.Contains(m, bad) {
				t.Errorf("sent a write or host-file flag: %q", m)
			}
		}
	}
	if !sawCommand {
		t.Fatal("no Commander command was sent")
	}
}

// MCP writes preview until confirm is boolean true: a mirrored command that writes
// local data does not run until then, and with it, it runs once. Nothing is sent to
// Service Mode but reads either way.
func TestMCPWritesNeedConfirm(t *testing.T) {
	sv := echoServer(t)
	sv.honest.Store(true)
	e := newEnv(t, sv, true)
	requests := func() int {
		sv.mu.Lock()
		defer sv.mu.Unlock()
		return len(sv.methods)
	}
	posture := func(confirm any) map[string]any {
		args := map[string]any{}
		if confirm != nil {
			args["confirm"] = confirm
		}
		return map[string]any{"name": "posture_report", "arguments": args}
	}
	before := requests()
	for i, c := range []any{nil, false, "true", 1} {
		out := e.mcpCalls(t, []map[string]any{posture(c)})
		if !strings.Contains(out, "Preview only") {
			t.Errorf("posture_report with confirm #%d (%v) did not preview:\n%.1500s", i, c, out)
		}
	}
	if n := requests(); n != before {
		t.Errorf("an unconfirmed posture_report sent %d request(s)", n-before)
	}
	if out := e.mcpCalls(t, []map[string]any{posture(true)}); requests() == before {
		t.Errorf("posture_report with confirm: true sent nothing:\n%.1500s", out)
	}
	// The local learning writers are no MCP tools at all: the server writes no learning.
	names := map[string]bool{}
	for _, tool := range mcpTools(t, e) {
		names[tool.Name] = true
	}
	for _, name := range []string{"teach", "teach_pattern", "teach_lookup", "teach_playbook", "playbook_amend", "learnings_confirm", "learnings_reject", "learnings_forget"} {
		if names[name] {
			t.Errorf("%s is an MCP tool; the MCP server must write no learning", name)
		}
	}
	// A group that only prints help is no tool either.
	for _, name := range []string{"users", "companies", "posture", "audit_trail"} {
		if names[name] {
			t.Errorf("%s is an MCP tool that only prints help", name)
		}
	}
	sv.mu.Lock()
	defer sv.mu.Unlock()
	for _, m := range sv.methods {
		f := strings.Fields(m)
		if f[0] != "GET" && (len(f) < 3 || !allowedFirstTokens[f[2]]) {
			t.Errorf("sent %q", m)
		}
	}
}
