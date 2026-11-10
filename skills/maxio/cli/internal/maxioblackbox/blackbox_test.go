// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (maxio; handfixes.json credkit-wiring): the black-box proof
// of the credential rules with the shared credential kit wired in. Both built
// binaries run against a fake Maxio API served over https with a
// certificate made for this run (trusted through MAXIO_CA_CERT and
// SSL_CERT_FILE). The fake echoes the API key in the rows it answers, and a
// second https host stands for any other origin. Each credential class the
// factory review left open has one test in kit_classes_test.go, driven from
// its recorded failing input; every byte either binary prints, and every file
// it writes under its home, is scanned for every credential.

package maxioblackbox

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The fake's credentials. Each is built from two parts so no 8-character
// piece of it occurs in the program's own text.
const (
	apiKey = "MaxKey-" + "7Hq2Lm9Xw3Pz4Kd8"
	// The fake's customer ids. echoID answers a record that repeats the API key;
	// plainID one with no credential; provID one whose password member is the
	// value a later MCP request supplies as a credential argument; xID one whose
	// words include the documented Basic password x; redirectID a redirect to
	// the other origin.
	echoID     = "1534956341"
	plainID    = "1534956342"
	provID     = "1534956343"
	xID        = "1534956344"
	publicID   = "1534956345"
	redirectID = "1534956346"
)

// Credentials the fake's records carry.
const (
	vaultToken       = "VaultTok-" + "4f2a9c1e7b"
	customerVault    = "CustVault-" + "0b7e3d5a9c"
	selfServiceToken = "SelfSvc-" + "3e8a51c0d7"
	responseToken    = "tok_Resp" + "9d2c7b1e5a"
)

// kitNotice is the start of the text the kit puts in place of a withheld unit.
const kitNotice = "[credkit: withheld from "

// jsonEscaped is s with every character written as a \uXXXX escape.
func jsonEscaped(s string) string {
	var b strings.Builder
	for _, r := range s {
		fmt.Fprintf(&b, `\u%04x`, r)
	}
	return b.String()
}

// rot13 is the ROT13 of the ASCII letters of s.
func rot13(s string) string {
	b := []byte(s)
	for i, c := range b {
		switch {
		case c >= 'a' && c <= 'z':
			b[i] = 'a' + (c-'a'+13)%26
		case c >= 'A' && c <= 'Z':
			b[i] = 'A' + (c-'A'+13)%26
		}
	}
	return string(b)
}

// forms are the encodings a scan looks for. A value shorter than 8
// characters is looked for raw only; containsToken judges it.
func forms(s string) []string {
	if len(s) < 8 {
		return []string{s}
	}
	var out []string
	for _, f := range []string{
		s, base64.StdEncoding.EncodeToString([]byte(s)), base64.RawStdEncoding.EncodeToString([]byte(s)),
		base64.URLEncoding.EncodeToString([]byte(s)), base64.RawURLEncoding.EncodeToString([]byte(s)),
		hex.EncodeToString([]byte(s)), strings.ToUpper(hex.EncodeToString([]byte(s))),
		url.QueryEscape(s), url.PathEscape(s), jsonEscaped(s), rot13(s),
	} {
		if !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

// alnum reports whether c is an ASCII letter or digit.
func alnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// containsToken reports whether v occurs in text as a whole token: each side
// is the end of the text or a character that is not a letter or digit
// (credential leak contract 2(a), for a value under 8 characters).
func containsToken(text, v string) bool {
	for i := 0; ; {
		j := strings.Index(text[i:], v)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(v)
		if (start == 0 || !alnum(text[start-1])) && (end == len(text) || !alnum(text[end])) {
			return true
		}
		i = start + 1
	}
}

// leaksOf lists every value text repeats, in any of the forms above.
func leaksOf(text string, values ...string) []string {
	var found []string
	for _, v := range values {
		if len(v) < 8 {
			if containsToken(text, v) {
				found = append(found, fmt.Sprintf("%q as a whole token", v))
			}
			continue
		}
		for _, f := range forms(v) {
			if strings.Contains(text, f) {
				found = append(found, fmt.Sprintf("%q as %q", v, f))
			}
		}
	}
	return found
}

func assertNoneOf(t *testing.T, where, text string, values ...string) {
	t.Helper()
	if found := leaksOf(text, values...); len(found) > 0 {
		t.Errorf("%s printed a credential: %v\n%.2000s", where, found, text)
	}
}

// fake is the Maxio API over https, a second https host standing for any
// other origin, and a plain-http listener. Each counts what reached it.
type fake struct {
	srv, evil, plain *httptest.Server
	apiHits          atomic.Int32
	evilHits         atomic.Int32
	plainHits        atomic.Int32
	mu               sync.Mutex
	saw              []string // credentials evil or plain received
	methods          []string // API requests, "METHOD /path"
	auths            []string // Authorization headers the API received
	bodies           []string // request bodies the API received
	listEcho         atomic.Bool
}

// certFile is the PEM of the certificate every TLS fake serves: httptest's
// TLS servers share one, made for this run, valid for 127.0.0.1.
var certFile string

// secretsSeenBy are the values a request to a host other than the API must
// never carry.
var secretsSeenBy = []string{apiKey, "NewQuerySecretq7", "NewHookSecretq7", "redacted"}

func (f *fake) record(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var head strings.Builder
	_ = r.Header.Write(&head)
	text := r.URL.String() + "\n" + head.String() + "\n" + string(body)
	if found := leaksOf(text, secretsSeenBy...); len(found) > 0 || r.Header.Get("Authorization") != "" {
		f.mu.Lock()
		f.saw = append(f.saw, fmt.Sprintf("%s %s: %v %q", r.Method, r.URL, found, r.Header.Get("Authorization")))
		f.mu.Unlock()
	}
}

func newFake(t *testing.T) *fake {
	f := &fake{}
	f.evil = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.evilHits.Add(1)
		f.record(r)
		_, _ = io.WriteString(w, `{"stolen":true}`)
	}))
	t.Cleanup(f.evil.Close)
	f.plain = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.plainHits.Add(1)
		f.record(r)
		_, _ = io.WriteString(w, `{"stolen":true}`)
	}))
	t.Cleanup(f.plain.Close)
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.api))
	t.Cleanup(f.srv.Close)
	return f
}

// customerRow is one Maxio customer envelope.
func customerRow(id, organization string) string {
	return `{"customer":{"id":` + id + `,"organization":"` + organization + `"}}`
}

// api is the fake Maxio Advanced Billing API (spec.json: paths ending .json,
// lists that are arrays of singular envelopes, gets that wrap one envelope).
func (f *fake) api(w http.ResponseWriter, r *http.Request) {
	f.apiHits.Add(1)
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.methods = append(f.methods, r.Method+" "+r.URL.Path)
	f.auths = append(f.auths, r.Header.Get("Authorization"))
	f.bodies = append(f.bodies, string(body))
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	page := r.URL.Query().Get("page")
	switch {
	case r.Method == http.MethodPost && (r.URL.Path == "/customers.json" || r.URL.Path == "/payment_profiles.json"):
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body) // Maxio answers a create with the record it stored
	case r.URL.Path == "/customers.json":
		if page != "" && page != "1" {
			_, _ = io.WriteString(w, `[]`)
			return
		}
		if f.listEcho.Load() {
			_, _ = io.WriteString(w, `[`+customerRow("1", "key "+apiKey)+`,`+customerRow("2", "Second row")+`]`)
			return
		}
		_, _ = io.WriteString(w, `[`+customerRow("1", "Plain row")+`,`+customerRow("2", "Second row")+`]`)
	case r.URL.Path == "/customers/"+echoID+".json":
		// The answer repeats the key raw, base64 and hex.
		_, _ = fmt.Fprintf(w, `{"customer":{"id":1,"organization":%q,"enc":%q,"hex":%q}}`,
			"key "+apiKey, base64.StdEncoding.EncodeToString([]byte(apiKey)), hex.EncodeToString([]byte(apiKey)))
	case r.URL.Path == "/customers/"+plainID+".json":
		_, _ = io.WriteString(w, customerRow("2", "Second row"))
	case r.URL.Path == "/customers/"+provID+".json":
		_, _ = io.WriteString(w, `{"customer":{"id":7,"password":"`+"Qz7Wm4Rt"+"9Kv2Xp6N"+`","organization":"Second row"}}`)
	case r.URL.Path == "/customers/"+xID+".json":
		_, _ = io.WriteString(w, `{"customer":{"id":3,"organization":"x","vat_number":"size x large"}}`)
	case r.URL.Path == "/customers/"+publicID+".json":
		_, _ = io.WriteString(w, `{"customer":{"id":4,"organization":"Public fields","key":"payment_success","masked_card_number":"XXXX-XXXX-XXXX-1111",`+
			`"last_four":"4242","masked_bank_account_number":"XXXX4321","authorizer_first_name":"HQ-Authorizer","uniqueness_token":"hq-dedupe-7731",`+
			`"stored_credential_transaction_id":9988771,"secure_checkout_policy_url":"https://hq-record.example.com/secure-checkout"}}`)
	case r.URL.Path == "/customers/"+redirectID+".json":
		http.Redirect(w, r, f.evil.URL+"/stolen", http.StatusFound)
	case r.URL.Path == "/subscriptions.json":
		if page != "" && page != "1" {
			_, _ = io.WriteString(w, `[]`)
			return
		}
		_, _ = io.WriteString(w, `[{"subscription":{"id":15,"state":"active","self_service_page_token":"`+selfServiceToken+`",`+
			`"credit_card":{"id":16,"first_name":"HQ-Record","vault_token":"`+vaultToken+`","customer_vault_token":"`+customerVault+`","masked_card_number":"XXXX-XXXX-XXXX-1111"}}}]`)
	case r.URL.Path == "/payment_profiles/44.json":
		_, _ = io.WriteString(w, `{"payment_profile":{"id":44,"first_name":"HQ-Record","chargify_token":"`+responseToken+`","vault_token":"1"}}`)
	default:
		// A real API's not-found can repeat the path it was asked for.
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, `{"errors":["not found: %s"]}`, r.URL.Path)
	}
}

func (f *fake) writes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, m := range f.methods {
		if !strings.HasPrefix(m, "GET ") {
			out = append(out, m)
		}
	}
	return out
}

// assertOthersSawNothing fails when a host other than the API received a
// credential, or any request reached the plain-http listener with one.
func (f *fake) assertOthersSawNothing(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.saw) > 0 {
		t.Errorf("a host other than the API received a credential: %v", f.saw)
	}
}

var binDir string

func exe(name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(binDir, name)
}

// TestMain builds both binaries once and writes the fakes' certificate.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "maxio-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binDir = dir
	code := func() int {
		defer os.RemoveAll(dir)
		for _, name := range []string{"maxio-cli", "maxio-mcp"} {
			out, err := exec.Command("go", "build", "-o", exe(name), "../../cmd/"+name).CombinedOutput()
			if err != nil {
				fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", name, err, out)
				return 1
			}
		}
		probe := httptest.NewTLSServer(http.NotFoundHandler())
		pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: probe.Certificate().Raw})
		probe.Close()
		certFile = filepath.Join(dir, "fake-ca.pem")
		if err := os.WriteFile(certFile, pemBytes, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return m.Run()
	}()
	os.Exit(code)
}

// baseEnv runs a binary as a fresh operator: its own home (MAXIO_HOME and
// every home variable inside it), the fakes' certificate trusted, every CLI,
// XDG, proxy and trust override cleared, and no credential.
func baseEnv(home string) []string {
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		upper := strings.ToUpper(name)
		if strings.HasPrefix(upper, "MAXIO_") || strings.HasPrefix(upper, "XDG_") || strings.HasSuffix(upper, "_PROXY") ||
			upper == "HOME" || upper == "USERPROFILE" || upper == "APPDATA" || upper == "LOCALAPPDATA" ||
			upper == "SSL_CERT_FILE" || upper == "SSL_CERT_DIR" || strings.HasPrefix(upper, "PRINTING_PRESS_") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "HOME="+home, "USERPROFILE="+home, "APPDATA="+filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA="+filepath.Join(home, "AppData", "Local"), "MAXIO_HOME="+filepath.Join(home, "mh"),
		"MAXIO_CA_CERT="+certFile, "SSL_CERT_FILE="+certFile)
}

// env is baseEnv with the API key and the fake's base URL.
func env(home string, f *fake) []string {
	return append(baseEnv(home), "MAXIO_API_KEY="+apiKey, "MAXIO_BASE_URL="+f.srv.URL)
}

// runWait bounds one run of a binary, so a hang fails the test.
const runWait = 120 * time.Second

// runWith runs the CLI and returns everything it printed and its exit code.
func runWith(t *testing.T, environ []string, stdin string, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), runWait)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe("maxio-cli"), args...)
	cmd.Env = environ
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("%v did not finish within %v:\n%s", args, runWait, out.String())
	}
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running %v: %v", args, err)
	}
	return out.String(), code
}

func writePrivate(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// credentialFile reports whether name is one of the connector's declared
// credential sources: the config file and the 0600 credential store, which
// the kit names in Declarations.Files and holds at every start.
func credentialFile(name string) bool {
	return name == "config.toml" || name == "credentials.toml"
}

// scanFiles fails when a file under home other than a declared credential
// source holds any of values, or when a credential source is not 0600.
func scanFiles(t *testing.T, home string, values ...string) {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		n++
		if credentialFile(d.Name()) {
			if info, err := d.Info(); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 && len(leaksOf(string(data), values...)) > 0 {
				t.Errorf("credential source %s is %v, want 0600", path, info.Mode().Perm())
			}
			return nil
		}
		if found := leaksOf(string(data), values...); !synccpuStoreFile(path) && len(found) > 0 { // synccpu: the local store is not checked
			t.Errorf("file %s holds a credential: %v", path, found)
		}
		return nil
	})
	if n == 0 {
		t.Fatal("the runs wrote no file at all; the file scan proves nothing")
	}
}

// mcpSession speaks newline-delimited JSON-RPC to the MCP server over stdio.
type mcpSession struct {
	t      *testing.T
	in     io.WriteCloser
	lines  chan string // each line the server writes; closed at its end
	stderr *bytes.Buffer
	all    strings.Builder
	id     int
}

// mcpAnswerWait bounds how long one request waits for its answer: a frame the
// kit withholds never arrives, and the test must fail rather than hang.
const mcpAnswerWait = 60 * time.Second

func startMCP(t *testing.T, environ []string) *mcpSession {
	t.Helper()
	cmd := exec.Command(exe("maxio-mcp"))
	cmd.Env = environ
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	in, _ := cmd.StdinPipe()
	outPipe, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// The process this test started, by PID.
	t.Cleanup(func() { _ = in.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() })
	sc := bufio.NewScanner(outPipe)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	s := &mcpSession{t: t, in: in, lines: lines, stderr: stderr}
	s.send("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "blackbox", "version": "1"}})
	_, _ = in.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"))
	return s
}

// sendRaw writes one request and returns the answer frame with its id, as
// the server wrote it.
func (s *mcpSession) sendRaw(params map[string]any, method string) string {
	s.t.Helper()
	s.id++
	msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": s.id, "method": method, "params": params})
	if _, err := s.in.Write(append(msg, '\n')); err != nil {
		s.t.Fatal(err)
	}
	timeout := time.After(mcpAnswerWait)
	for {
		select {
		case line, open := <-s.lines:
			if !open {
				s.t.Fatalf("MCP server closed before answering %s; stderr:\n%s", method, s.stderr)
			}
			s.all.WriteString(line + "\n")
			var m map[string]any
			if json.Unmarshal([]byte(line), &m) != nil {
				continue
			}
			if id, ok := m["id"].(float64); ok && int(id) == s.id {
				return line
			}
		case <-timeout:
			s.t.Fatalf("no answer to %s within %v; the server wrote:\n%.2000s\nstderr:\n%s", method, mcpAnswerWait, s.all.String(), s.stderr)
		}
	}
}

func (s *mcpSession) send(method string, params map[string]any) string {
	s.t.Helper()
	return s.sendRaw(params, method)
}

func (s *mcpSession) call(name string, args map[string]any) string {
	s.t.Helper()
	return s.send("tools/call", map[string]any{"name": name, "arguments": args})
}

// transcript is every frame the server wrote, and its stderr.
func (s *mcpSession) transcript() string { return s.all.String() + "\n" + s.stderr.String() }

// freeAddr is a loopback address nothing listens on.
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

// TestCLIRunLeaksNoCredential drives the CLI's ordinary reads against an API
// whose answers repeat the key: every printed byte and every file written
// outside the credential store is scanned for it, and a credential-free
// answer still reaches the operator whole.
func TestCLIRunLeaksNoCredential(t *testing.T) {
	f := newFake(t)
	home := t.TempDir()
	if out, code := runWith(t, env(home, f), "", "customers", "read", plainID, "--json"); code != 0 || !strings.Contains(out, "Second row") {
		t.Fatalf("customers read %s must answer (exit %d):\n%s", plainID, code, out)
	}
	var all string
	for _, args := range [][]string{
		{"customers", "read", echoID, "--json"},
		{"doctor", "--json"},
		{"auth", "status", "--json"},
		{"customers", "read", "--no-such-flag", apiKey},
	} {
		out, _ := runWith(t, env(home, f), "", args...)
		all += "\n" + out
		assertNoneOf(t, fmt.Sprint(args), out, apiKey)
	}
	if !strings.Contains(all, kitNotice) {
		t.Errorf("no output was withheld, so the echoing answers never reached the kit:\n%.3000s", all)
	}
	f.assertOthersSawNothing(t)
	scanFiles(t, home, apiKey)
}
