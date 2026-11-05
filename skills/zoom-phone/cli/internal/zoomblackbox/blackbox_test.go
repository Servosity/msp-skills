// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package zoomblackbox runs the built CLI and MCP binaries against a Zoom fake
// and checks what a reviewer cannot read off the code (handfixes.json
// credkit-wiring): each test starts from the failing input a review named. The
// oracle is the exact strings the fake sent, so a pass does not depend on the
// program's own decoder.
package zoomblackbox

import (
	"bytes"
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
	bbWithheld = "[credkit: withheld from"

	bbAccount = "acct-7Hq2xLm"
	bbClient  = "clientId-4f1e9a7c3b2d"
	bbSecret  = "clientSecret-91c3e7a5d2f8b604"
	bbMinted  = "mintedTOKEN-7fb3189bQ6u3sZxW4o1n7R8v52"
	bbStored  = "storedToken-71c3e9b05d2a4f68e1"
)

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "zoom-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "zoom-phone-cli"+exe)
	bbMCP = filepath.Join(dir, "zoom-phone-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/zoom-phone-cli", bbMCP: "../../cmd/zoom-phone-mcp"} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// forms is what a reader could recover a credential from.
func forms(s string) []string {
	return []string{
		s,
		base64.StdEncoding.EncodeToString([]byte(s)),
		base64.RawURLEncoding.EncodeToString([]byte(s)),
		hex.EncodeToString([]byte(s)),
		url.QueryEscape(s),
	}
}

func assertNoForm(t *testing.T, where, text string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		for _, f := range forms(s) {
			if strings.Contains(text, f) {
				t.Errorf("%s holds a credential as %q:\n%.1500s", where, f, text)
			}
		}
	}
}

func echo(values ...string) string {
	var parts []string
	for i, v := range values {
		for j, f := range forms(v) {
			b, _ := json.Marshal(f)
			parts = append(parts, fmt.Sprintf(`"e%d_%d":%s`, i, j, b))
		}
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// tlsServer starts an HTTP/2 TLS fake (the generated client speaks h2 only).
func tlsServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// api is a Zoom fake: it mints at /oauth/token and answers from handler.
type api struct {
	srv     *httptest.Server
	caFile  string
	mints   atomic.Int32
	mu      sync.Mutex
	bodies  []string
	authSet map[string]bool
}

func newAPI(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, body string)) *api {
	t.Helper()
	a := &api{authSet: map[string]bool{}}
	a.srv = tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		a.mu.Lock()
		a.bodies = append(a.bodies, string(raw))
		a.authSet[r.Header.Get("Authorization")] = true
		a.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/token" {
			a.mints.Add(1)
			fmt.Fprintf(w, `{"access_token":%q,"token_type":"bearer","expires_in":3600,"scope":"phone:read"}`, bbMinted)
			return
		}
		handler(w, r, string(raw))
	}))
	ca := filepath.Join(shortTemp(t), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: a.srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	a.caFile = ca
	return a
}

func (a *api) sawBody(want string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, b := range a.bodies {
		if strings.Contains(b, want) {
			return true
		}
	}
	return false
}

func (a *api) sawAuth(want string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.authSet[want]
}

type bbEnv struct {
	home string
	env  []string
}

// shortTemp is an empty directory. Not t.TempDir: its path carries the test's
// name, which can share a piece with the values these tests hold.
func shortTemp(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "zp-kit-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func newEnv(t *testing.T, a *api) *bbEnv {
	t.Helper()
	home := shortTemp(t)
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	return &bbEnv{home: home, env: []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"ZOOM_PHONE_BASE_URL=" + a.srv.URL + "/v2", "ZOOM_PHONE_CA_CERT=" + a.caFile,
		"ZOOM_PHONE_CLI_PATH=" + bbCLI,
		"HTTPS_PROXY=http://127.0.0.1:9", "HTTP_PROXY=http://127.0.0.1:9", // nothing here reaches a real host; loopback is never proxied
	}}
}

// minting adds the Server-to-Server app, with its token URL on the fake.
func (e *bbEnv) minting(a *api) *bbEnv {
	return e.with("ZOOM_PHONE_ACCOUNT_ID="+bbAccount, "ZOOM_PHONE_CLIENT_ID="+bbClient,
		"ZOOM_PHONE_CLIENT_SECRET="+bbSecret, "ZOOM_PHONE_TOKEN_URL="+a.srv.URL+"/oauth/token")
}

// stored writes a config holding a saved token and selects it.
func (e *bbEnv) stored(t *testing.T) *bbEnv {
	t.Helper()
	p := filepath.Join(e.home, "stored.toml")
	if err := os.WriteFile(p, []byte("access_token = \""+bbStored+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return e.with("ZOOM_PHONE_CONFIG=" + p)
}

func (e *bbEnv) with(extra ...string) *bbEnv {
	return &bbEnv{home: e.home, env: append(append([]string{}, e.env...), extra...)}
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
	if err := cmd.Start(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("%v: timed out", args)
	}
	return out.String() + "\n" + errb.String()
}

// mcpCalls runs one MCP stdio session and returns everything the server wrote.
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
	var out, errb lockedBuffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitDone := make(chan struct{})
	go func() { _ = cmd.Wait(); close(waitDone) }()
	_, _ = io.Copy(stdin, &in)
	deadline := time.Now().Add(120 * time.Second)
wait:
	for time.Now().Before(deadline) {
		select {
		case <-waitDone:
			break wait
		case <-time.After(100 * time.Millisecond):
		}
		if strings.Count(out.String(), `"id":`) >= len(calls)+1 {
			break
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

func read(endpoint string, params map[string]any) map[string]any {
	return map[string]any{"name": "zoom-phone_read", "arguments": map[string]any{"endpoint_id": endpoint, "params": params}}
}

// scanFiles fails when a file under home, other than the 0600 credential files
// the test passes as allowed, holds one of secrets.
func scanFiles(t *testing.T, home string, allowed func(rel string) bool, secrets ...string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		rel, _ := filepath.Rel(home, path)
		if allowed != nil && allowed(rel) {
			return nil
		}
		n++
		assertNoForm(t, "file "+rel, string(data), secrets...)
		return nil
	})
	return n
}

// Round-2 input 1 (short-values), verbatim: a 3-character PIN, echoed by HTTP 400
// inside base64 of "PIN:123".
func TestShortPinInsideEncodedMessage(t *testing.T) {
	const encoded = "UElOOjEyMw=="
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"message":%q}`, encoded)
	})
	e := newEnv(t, a).stored(t)
	out := e.run(t, "common-areas", "update-pin-code", "CA", "--pin-code", "123", "--yes", "--json")
	if strings.Contains(out, "UElOOjEyMw") || strings.Contains(out, "PIN:123") {
		t.Errorf("CLI shows the PIN in its encoded echo:\n%s", out)
	}
	if a.sawBody(`"pin_code":"123"`) {
		// 123 is in the kit's public text (contract section 2): held at startup it refuses to start.
		t.Errorf("a PIN that collides with public text was sent:\n%s", out)
	}
	mcp := e.mcpCalls(t, []map[string]any{{"name": "zoom-phone_execute", "arguments": map[string]any{
		"endpoint_id": "common_areas.update-pin-code", "confirm": true,
		"params": map[string]any{"commonAreaId": "CA", "pin_code": "123"}}}})
	if strings.Contains(mcp, "UElOOjEyMw") {
		t.Errorf("MCP shows the PIN in its encoded echo:\n%s", mcp)
	}
}

// The same input with a 4-digit PIN, which starts: the request carries it and the
// encoded echo never shows, on a success result either.
func TestShortPinSentAndHeld(t *testing.T) {
	const pin, encoded = "7319", "UElOOjczMTk="
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) {
		if r.URL.Query().Get("ok") == "" && strings.Contains(r.URL.Path, "/CA/") {
			w.WriteHeader(http.StatusBadRequest)
		}
		fmt.Fprintf(w, `{"message":%q}`, encoded)
	})
	e := newEnv(t, a).stored(t)
	out := e.run(t, "common-areas", "update-pin-code", "CA", "--pin-code", pin, "--yes", "--json")
	if strings.Contains(out, "UElOOjczMTk") {
		t.Errorf("CLI shows the PIN in its encoded echo:\n%s", out)
	}
	if !a.sawBody(`"pin_code":"` + pin + `"`) {
		t.Errorf("the request lost the PIN: %q\n%s", a.bodies, out)
	}
	mcp := e.mcpCalls(t, []map[string]any{
		{"name": "zoom-phone_execute", "arguments": map[string]any{
			"endpoint_id": "common_areas.update-pin-code", "confirm": true,
			"params": map[string]any{"commonAreaId": "CB", "pin_code": pin}}},
		read("users.phone-settings", map[string]any{"userId": "u1"}),
	})
	if strings.Contains(mcp, "UElOOjczMTk") {
		t.Errorf("MCP shows the PIN in its encoded echo:\n%s", mcp)
	}
}

// Round-2 input 2 (retention): after 4,097 PINs a response carried, an error echoes
// the first in base64. Nothing ages out of what the process holds.
func TestRetentionAfter4097ObservedPins(t *testing.T) {
	var pins strings.Builder
	for n := 100000; n <= 104096; n++ {
		if n > 100000 {
			pins.WriteByte(',')
		}
		fmt.Fprintf(&pins, `{"id":"u%d","desk_phone":{"pin_code":"%d"}}`, n, n)
	}
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) {
		if strings.HasSuffix(r.URL.Path, "/echo/settings") {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"message":"MTAwMDAw","n":"313030303031"}`)
			return
		}
		fmt.Fprintf(w, `{"users":[%s]}`, pins.String())
	})
	e := newEnv(t, a).stored(t)
	out := e.mcpCalls(t, []map[string]any{
		read("users.phone-settings", map[string]any{"userId": "u1"}),
		read("users.phone-settings", map[string]any{"userId": "echo"}),
	})
	for _, bad := range []string{"MTAwMDAw", "313030303031", "100000", "104096"} {
		if strings.Contains(out, bad) {
			t.Errorf("the MCP session shows %q:\n%.1500s", bad, out)
		}
	}
	if !strings.Contains(out, `"id":2`) {
		t.Errorf("the second call never answered:\n%.1500s", out)
	}
}

// A credential in a text/plain body, on a read: "password=" is a declared field in
// a body the JSON redaction cannot parse.
func TestPasswordFormInTextPlainMCPRead(t *testing.T) {
	const pw = "Sp3cialValue9"
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "status=ok&password=%s", pw)
	})
	e := newEnv(t, a).stored(t)
	out := e.mcpCalls(t, []map[string]any{read("users.phone-settings", map[string]any{"userId": "u1"})})
	assertNoForm(t, "MCP read", out, pw)
	if !strings.Contains(out, `"id":1`) {
		t.Errorf("the call never answered:\n%.1500s", out)
	}
	cli := e.run(t, "users", "phone-settings", "u1", "--json")
	assertNoForm(t, "CLI read", cli, pw)
}

// A config that does not parse still holds what it carries: no startup output, on
// either binary, repeats a credential from it.
func TestMalformedConfigAtStartup(t *testing.T) {
	const secret = "Malf0rmedSecret" + "-7c1e4b"      // fake; split for the secret scanner
	const overflow = "123456789012345678901234567890" // the parser's overflow diagnostic repeats it
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) { fmt.Fprint(w, `{"users":[]}`) })
	e := newEnv(t, a)
	p := filepath.Join(e.home, "broken.toml")
	if err := os.WriteFile(p, []byte("client_secret = \""+secret+"\nphone_client_secret = "+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e = e.with("ZOOM_PHONE_CONFIG=" + p)
	for _, args := range [][]string{{"users", "list-phone", "--json"}, {"doctor"}, {"auth", "status"}} {
		assertNoForm(t, strings.Join(args, " "), e.run(t, args...), secret)
	}
	assertNoForm(t, "MCP", e.mcpCalls(t, []map[string]any{read("users.phone-settings", map[string]any{"userId": "u1"})}), secret)
	if err := os.WriteFile(p, []byte("auth_header = "+overflow+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"users", "list-phone", "--json"}, {"doctor"}, {"auth", "status"}} {
		assertNoForm(t, "overflow "+strings.Join(args, " "), e.run(t, args...), overflow)
	}
	assertNoForm(t, "overflow MCP", e.mcpCalls(t, []map[string]any{read("users.phone-settings", map[string]any{"userId": "u1"})}), overflow)
}

// A minted token reaches the API it was minted for and never reaches any output.
func TestMintedTokenHeldAndSent(t *testing.T) {
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) {
		fmt.Fprintf(w, `{"users":[{"id":"u1","note":%s}]}`, echo(bbMinted, bbSecret, bbClient))
	})
	e := newEnv(t, a).minting(a)
	out := e.run(t, "users", "list-phone", "--json") + e.mcpCalls(t, []map[string]any{read("users.list-phone", map[string]any{})})
	assertNoForm(t, "output", out, bbMinted, bbSecret, bbClient)
	if a.mints.Load() == 0 || !a.sawAuth("Bearer "+bbMinted) {
		t.Fatalf("the mint or the API call did not happen: mints=%d auth=%v\n%.1500s", a.mints.Load(), a.authSet, out)
	}
}

// auth login against a token endpoint on the configured origin works under the kit
// and shows neither the secret nor the token.
func TestAuthLoginSameOrigin(t *testing.T) {
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) { fmt.Fprint(w, `{"users":[]}`) })
	e := newEnv(t, a).minting(a)
	out := e.run(t, "auth", "login")
	if a.mints.Load() != 1 {
		t.Fatalf("mints = %d:\n%s", a.mints.Load(), out)
	}
	assertNoForm(t, "auth login", out, bbMinted, bbSecret)
	if strings.Contains(out, bbWithheld) {
		t.Errorf("auth login output was withheld:\n%s", out)
	}
}

// A response field names a credential: the value is held for the rest of the run,
// so its echo in a later page or field goes nowhere.
func TestResponseFieldHeld(t *testing.T) {
	const pw = "Sip-Passw0rd-e41b"
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) {
		fmt.Fprintf(w, `{"id":"u1","provision":{"sip_accounts":[{"password":%q}]},"note":%q}`, pw, base64.StdEncoding.EncodeToString([]byte(pw)))
	})
	e := newEnv(t, a).stored(t)
	deliver := filepath.Join(e.home, "deliver.json")
	out := e.run(t, "users", "phone-settings", "u1", "--json", "--deliver", "file:"+deliver) +
		e.mcpCalls(t, []map[string]any{read("users.phone-settings", map[string]any{"userId": "u1"})})
	assertNoForm(t, "output", out, pw)
	scanFiles(t, e.home, func(rel string) bool { return rel == "stored.toml" }, pw, bbStored)
}

// A --stdin body's credential field is sent once and held: the preview and the echo
// show neither. The body still reaches the API whole, pretty-printed or not.
func TestStdinBodyHeld(t *testing.T) {
	const pin = "86429"
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) {
		fmt.Fprintf(w, `{"id":"CA","note":"set %s","e":%q}`, pin, base64.StdEncoding.EncodeToString([]byte(pin)))
	})
	e := newEnv(t, a).stored(t)
	args := []string{"common-areas", "update-pin-code", "CA", "--stdin", "--yes", "--json"}
	compact := `{"pin_code":"` + pin + `"}`
	assertNoForm(t, "dry run", e.runIn(t, compact, append(args, "--dry-run")...), pin)
	out := e.runIn(t, compact, args...)
	assertNoForm(t, "run", out, pin)
	if !a.sawBody(pin) {
		t.Errorf("the body never reached the API:\n%s", out)
	}
	// The same body pretty-printed is the same document.
	a.mu.Lock()
	a.bodies = nil
	a.mu.Unlock()
	out = e.runIn(t, "{\n  \"pin_code\": \""+pin+"\"\n}\n", args...)
	assertNoForm(t, "pretty", out, pin)
	if !a.sawBody(pin) {
		t.Errorf("the pretty-printed body never reached the API:\n%s", out)
	}
	// A body that does not parse is refused, and the refusal does not repeat it.
	assertNoForm(t, "malformed", e.runIn(t, `{"pin_code":"`+pin, args...), pin)
}

// A response repeating a body's ordinary fields is the command's result: its ID
// is what a script keeps, and a retry without it creates a duplicate.
func TestStdinCreateKeepsItsResult(t *testing.T) {
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) {
		fmt.Fprint(w, `{"id":"cqNEW123","name":"Support Queue","extension_number":2001,"site_id":"s1"}`)
	})
	e := newEnv(t, a).stored(t)
	args := []string{"call-queues", "create", "--stdin", "--yes", "--json"}
	for name, body := range map[string]string{
		"compact": `{"name":"Support Queue","extension_number":2001,"site_id":"s1"}` + "\n",
		"pretty":  "{\n  \"name\": \"Support Queue\",\n  \"extension_number\": 2001,\n  \"site_id\": \"s1\"\n}\n",
	} {
		out := e.runIn(t, body, args...)
		if strings.Contains(out, bbWithheld) || !strings.Contains(out, "cqNEW123") {
			t.Errorf("%s: the create lost its result:\n%s", name, out)
		}
	}
}

// export's documented form for a large dataset: JSONL to a file, past the size at
// which a buffered writer's flushes left a record open across kit units.
func TestExportJSONLToFile(t *testing.T) {
	const n = 300
	rows := make([]string, n)
	for i := range rows {
		rows[i] = fmt.Sprintf(`{"id":"u%d","email":"first%d@example.com","name":"First%d Last%d","extension_number":%d,"status":"activate","site":{"id":"s1","name":"HQ"}}`, i, i, i, i, 1000+i)
	}
	page := fmt.Sprintf(`{"page_size":%d,"next_page_token":"","total_records":%d,"users":[%s]}`, n, n, strings.Join(rows, ","))
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) { fmt.Fprint(w, page) })
	e := newEnv(t, a).minting(a)
	path := filepath.Join(e.home, "users.jsonl")
	out := e.run(t, "export", "users", "--format", "jsonl", "--output", path)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) != n || strings.Contains(string(b), bbWithheld) {
		t.Fatalf("export wrote %d of %d lines:\n%s", len(lines), n, out)
	}
	for _, l := range lines {
		if !json.Valid([]byte(l)) {
			t.Fatalf("export wrote a line that is no record: %q", l)
		}
	}
}

// A note is free text: passed as arguments it is saved. One that carries a held
// credential reaches no file; the kit writes its notice in its place.
func TestFeedbackNote(t *testing.T) {
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) { fmt.Fprint(w, `{}`) })
	e := newEnv(t, a).stored(t)
	out := e.run(t, "feedback", "the call queue listing is great")
	if strings.Contains(out, bbWithheld) || !strings.Contains(out, "recorded") {
		t.Fatalf("feedback was not recorded:\n%s", out)
	}
	saved := false
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, _ := os.ReadFile(path); bytes.Contains(b, []byte("call queue listing")) {
				saved = true
			}
		}
		return nil
	})
	if !saved {
		t.Errorf("the feedback was not saved:\n%s", out)
	}
	out = e.run(t, "feedback", "noted the token "+bbStored+" in a ticket")
	assertNoForm(t, "feedback", out, bbStored)
	scanFiles(t, e.home, func(rel string) bool { return rel == "stored.toml" }, bbStored)
}

// A run profile store whose value sits under a credential flag's name: held on every
// later run, so the profile commands and an echo show it nowhere.
func TestProfileStoreValueHeld(t *testing.T) {
	const pin = "Pr0filePin-3e7a"
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) {
		fmt.Fprintf(w, `{"id":"CA","note":%q}`, base64.StdEncoding.EncodeToString([]byte(pin)))
	})
	e := newEnv(t, a).stored(t)
	if out := e.run(t, "profile", "save", "desk", "--json", "--compact"); strings.Contains(out, bbWithheld) {
		t.Fatalf("profile save: %s", out)
	}
	var store string
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.Name() == "profiles.json" {
			store = path
		}
		return nil
	})
	raw, err := os.ReadFile(store)
	if err != nil {
		t.Fatalf("no profile store: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"compact"`)) {
		t.Fatalf("unexpected store shape: %s", raw)
	}
	raw = bytes.Replace(raw, []byte(`"compact"`), []byte(`"pin-code": "`+pin+`", "compact"`), 1)
	if err := os.WriteFile(store, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	out := e.run(t, "profile", "show", "desk", "--json") + e.run(t, "profile", "list", "--json") +
		e.run(t, "users", "phone-settings", "u1", "--json")
	assertNoForm(t, "profile output", out, pin)
}

// A mistyped flag can be a pasted token; a session label can be anything. Neither
// reaches the journal or any other file.
func TestJournalHoldsNoCredential(t *testing.T) {
	const pasted = "pastedToken-5d2a4f68e1c3"
	const label = "sessionLabel-9b7e1d4a"
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) { fmt.Fprint(w, `{"users":[]}`) })
	e := newEnv(t, a).stored(t).with("ZOOM_PHONE_LEARN_SESSION=" + label)
	e.run(t, "users", "list-phone", "--json", "--"+pasted)
	e.run(t, "users", "list-phone", "--json")
	if scanFiles(t, e.home, func(rel string) bool { return rel == "stored.toml" }, pasted, label, bbStored) == 0 {
		t.Error("no file was scanned")
	}
}

// Wrong origins: a redirect off the API host gets no credential; a token URL off the
// Zoom hosts and the configured origin gets neither the secret nor the client ID.
func TestCredentialNeverLeavesForAnotherOrigin(t *testing.T) {
	var offAuth atomic.Int32
	var offHits atomic.Int32
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offHits.Add(1)
		if r.Header.Get("Authorization") != "" {
			offAuth.Add(1)
		}
		fmt.Fprint(w, `{"access_token":"x","expires_in":3600}`)
	}))
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) {
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
	})
	e := newEnv(t, a).stored(t)
	e.run(t, "users", "list-phone", "--json")
	e.mcpCalls(t, []map[string]any{read("users.list-phone", map[string]any{})})
	if offAuth.Load() != 0 {
		t.Errorf("a redirect carried a credential to another host %d time(s)", offAuth.Load())
	}

	offHits.Store(0)
	m := newEnv(t, a).with("ZOOM_PHONE_ACCOUNT_ID="+bbAccount, "ZOOM_PHONE_CLIENT_ID="+bbClient,
		"ZOOM_PHONE_CLIENT_SECRET="+bbSecret, "ZOOM_PHONE_TOKEN_URL="+other.URL+"/oauth/token")
	out := m.run(t, "auth", "login") + m.run(t, "users", "list-phone", "--json")
	if offHits.Load() != 0 {
		t.Errorf("the client credentials went to another origin %d time(s):\n%s", offHits.Load(), out)
	}
	assertNoForm(t, "refused token URL", out, bbSecret)
}

// A saved account's credentials: fleet children run, each mints its own token, and
// no output or file outside its own 0600 store shows a secret. accounts list stays
// usable: name and account ID visible, nothing withheld.
func TestAccountsAndFleet(t *testing.T) {
	const s1, s2 = "acmeSecret-1f4d7a9c2e", "zetaSecret-8b3c6e0f7d"
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) {
		fmt.Fprint(w, `{"users":[{"id":"u1","email":"rosa@example.com"}],"next_page_token":""}`)
	})
	e := newEnv(t, a).with("ZOOM_PHONE_TOKEN_URL=" + a.srv.URL + "/oauth/token")
	for name, s := range map[string]string{"acme": s1, "zeta": s2} {
		out := e.runIn(t, s+"\n", "accounts", "connect", name, "--account-id", "acct-"+name, "--client-id", "cid-"+name, "--client-secret-stdin")
		assertNoForm(t, "connect", out, s1, s2)
		if strings.Contains(out, bbWithheld) {
			t.Fatalf("connect was withheld:\n%s", out)
		}
		// A child runs with this account's own config, not the parent's environment.
		var cfg string
		_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.Name() == "config.toml" && strings.Contains(path, name) {
				cfg = path
			}
			return nil
		})
		f, err := os.OpenFile(cfg, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(f, "base_url = %q\ntoken_url = %q\nca_cert = %q\n", a.srv.URL+"/v2", a.srv.URL+"/oauth/token", a.caFile)
		_ = f.Close()
	}
	list := e.run(t, "accounts", "list", "--json")
	assertNoForm(t, "accounts list", list, s1, s2, bbMinted)
	for _, want := range []string{"acme", "zeta", "acct-acme"} {
		if !strings.Contains(list, want) {
			t.Errorf("accounts list lost %q:\n%s", want, list)
		}
	}
	if strings.Contains(list, bbWithheld) {
		t.Errorf("accounts list was withheld:\n%s", list)
	}
	fleet := e.run(t, "fleet", "--json", "--", "users", "list-phone", "--json")
	assertNoForm(t, "fleet", fleet, s1, s2, bbMinted)
	if !strings.Contains(fleet, "rosa@example.com") || strings.Contains(fleet, bbWithheld) {
		t.Errorf("fleet lost its result:\n%.2000s", fleet)
	}
	if a.mints.Load() < 2 {
		t.Errorf("fleet minted %d token(s), want one per account:\n%.2000s", a.mints.Load(), fleet)
	}
	scanFiles(t, e.home, func(rel string) bool {
		return strings.HasSuffix(rel, "config.toml") || strings.HasSuffix(rel, "credentials.toml")
	}, s1, s2, bbMinted)
}

// Ordinary output survives whole: the kit withholds what carries a credential, not a
// realistic listing, the doctor or a store sync.
func TestRealisticOutputSurvives(t *testing.T) {
	const users = `{"page_size":30,"next_page_token":"","total_records":1,"users":[{"id":"zAJvP9bvQ1a0nW","email":"rosa@example.com",` +
		`"name":"Rosa Diaz","extension_number":1042,"status":"activate","site":{"id":"s1","name":"HQ"},"policy":{"desk_phone":{"pin_code":false}},` +
		`"phone_numbers":[{"id":"n1","number":"+14155550123"}]}]}`
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) { fmt.Fprint(w, users) })
	e := newEnv(t, a).minting(a)
	out := e.run(t, "users", "list-phone", "--json") +
		e.mcpCalls(t, []map[string]any{read("users.list-phone", map[string]any{})})
	if strings.Contains(out, bbWithheld) {
		t.Errorf("a realistic listing was withheld:\n%.2000s", out)
	}
	for _, want := range []string{"Rosa Diaz", "rosa@example.com", "+14155550123"} {
		if strings.Count(out, want) < 2 {
			t.Errorf("%q did not survive on both binaries:\n%.2000s", want, out)
		}
	}
	doc := e.run(t, "doctor")
	if strings.Contains(doc, bbWithheld) {
		t.Errorf("doctor was withheld:\n%s", doc)
	}
	sync := e.run(t, "sync", "--full", "--resources", "users", "--json")
	if strings.Contains(sync, bbWithheld) {
		t.Errorf("sync was withheld:\n%.2000s", sync)
	}
}

// Codex pass 3 round 1, input 1: accounts connect echoes the client ID it was given.
func TestAccountsConnectDoesNotEchoTheClientID(t *testing.T) {
	const client = "ClientValue82z-4e7a"
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) { fmt.Fprint(w, `{}`) })
	e := newEnv(t, a).with("ZOOM_PHONE_CLIENT_SECRET=SecretValue91z-5d2c")
	for _, extra := range [][]string{{"--json"}, nil} {
		args := append([]string{"accounts", "connect", "acme", "--account-id", "acct-A", "--client-id", client, "--client-secret-env", "--force"}, extra...)
		out := e.run(t, args...)
		assertNoForm(t, "connect", out, client, "SecretValue91z-5d2c")
		if strings.Contains(out, bbWithheld) || !strings.Contains(out, "acct-A") {
			t.Errorf("connect lost its result:\n%s", out)
		}
	}
}

// Input 2: a sibling account's file read by accounts list, under the current
// account's home, holds its values before they can be shown.
func TestAccountsListHoldsSiblingProfile(t *testing.T) {
	const pass = "SiblingSecret91z-3b7e"
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) { fmt.Fprint(w, `{}`) })
	e := newEnv(t, a)
	root := filepath.Join(e.home, "accounts")
	for name, body := range map[string]string{
		"acme":   "client_id = \"cid-acme\"\n",
		"globex": "base_url = \"https://user:" + pass + "@api.zoom.us/v2\"\n",
	} {
		dir := filepath.Join(root, name)
		for _, d := range []string{dir, filepath.Join(dir, "data"), filepath.Join(dir, "state"), filepath.Join(dir, "cache")} {
			if err := os.MkdirAll(d, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	acme := filepath.Join(root, "acme")
	s := e.with("ZOOM_PHONE_HOME="+acme, "ZOOM_PHONE_CONFIG="+filepath.Join(acme, "config.toml"))
	for _, args := range [][]string{{"accounts", "list", "--json"}, {"accounts", "list"}} {
		assertNoForm(t, "accounts list", s.run(t, args...), pass)
	}
}

// Input 3: the MCP server's account isolation refusal names a path, and the path can
// carry a credential. It runs after the kit starts, so the refusal passes it.
func TestMCPIsolationRefusalPassesTheKit(t *testing.T) {
	const secret = "a1b2c3d4" + "e5f6" // fake; split for the secret scanner
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) { fmt.Fprint(w, `{}`) })
	e := newEnv(t, a)
	dir := filepath.Join(e.home, "accounts", secret)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte("phone_account_id = \"acct-A\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(e.home, "unrelated")
	s := e.with("ZOOM_PHONE_CLIENT_SECRET="+secret, "ZOOM_PHONE_CONFIG="+cfg, "ZOOM_PHONE_DATA_DIR="+unrelated)
	out := s.mcpCalls(t, []map[string]any{read("users.list-phone", map[string]any{})})
	assertNoForm(t, "MCP refusal", out, secret)
}

// Windows CI smoke, PR #543: Zoom sets the tracking cookie _zm_chtaid, a
// number from 0 to 999, on API and token responses. The kit holds every
// cookie value at any length, so a listing (or doctor's report) that printed
// the same number was withheld whole. The cookie authenticates nothing and
// this connector sends no cookies; credwire declares it an Identifier.
func TestZoomTrackingCookieDoesNotWithholdOutput(t *testing.T) {
	const users = `{"page_size":30,"next_page_token":"","total_records":1,"users":[{"id":"zAJvP9bvQ1a0nW","email":"rosa@example.com",` +
		`"name":"Rosa Diaz","extension_number":374,"status":"activate","phone_numbers":[{"id":"n1","number":"+14155550123"}]}]}`
	a := newAPI(t, func(w http.ResponseWriter, r *http.Request, body string) {
		w.Header().Add("Set-Cookie", "_zm_chtaid=374; Expires=Thu, 05 Nov 2026 16:02:59 GMT; Max-Age=7200; Path=/; Secure")
		fmt.Fprint(w, users)
	})
	e := newEnv(t, a).minting(a)
	out := e.run(t, "users", "list-phone", "--json") +
		e.mcpCalls(t, []map[string]any{read("users.list-phone", map[string]any{})})
	if strings.Contains(out, bbWithheld) {
		t.Errorf("a listing sharing a number with the tracking cookie was withheld:\n%.2000s", out)
	}
	if strings.Count(out, "374") < 2 {
		t.Errorf("the extension did not survive on both binaries:\n%.2000s", out)
	}
}
