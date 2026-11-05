// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (apple-business; handfixes.json credkit-wiring):
// the black-box proof of the credential rules. Both built binaries run against
// a fake Apple Business API that echoes the private key, the access token and
// the signed client assertion in several encodings, and redirects to another
// host. Every byte either binary prints, and every file it writes under its
// home, is scanned for every credential; the other host must never be
// reached, and an MCP write must send nothing until confirm is true.

package applebusinessblackbox

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/pelletier/go-toml/v2"
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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	// The client ID is the API account's public org label (BUSINESSAPI.<uuid>),
	// printed by design; the key ID is public too. Neither is scanned for.
	clientID    = "BUSINESSAPI.blackbox"
	keyID       = "kid-blackbox"
	accessToken = "tok-" + "Jp5Wc3Ze8Ha2Lm6Nq"
	// Credentials that live only in a config file the command reads: one
	// named by --config, one under --home, and a configured header.
	configToken = "Cfg-" + "Zx93Km2Pa7Ld8"
	homeToken   = "Home-" + "Wq47Lp2Ne8Rt5"
	headerToken = "Hdr-" + "Vb62Mx9Kc4Td7"
	// A --config file whose loose permissions refuse its credentials.
	looseToken = "Loose-" + "Pn38Wd5Hq2Ys6"
	// A token endpoint's error answer that carries a credential of its own.
	echoedToken = "NewToken-" + "Zx93Km2Pa7Ld8"
	// The MCP HTTP caller token.
	httpToken = "HttpSecret-0123456789abcdef0123456789"
)

// weakVendor are vendor credentials a strength filter would skip (short,
// one character class), each named in one field and repeated in another of
// the same answer. Free text in an undeclared field is not a held credential
// (docs/credential-leak-contract.md section 1, declared response fields).
var weakVendor = []struct{ id, attrs, value string }{
	{"WEAKNAMED", `"password":"weakpass9","deviceModel":"weakpass9"`, "weakpass9"},
	{"WEAKCLASS", `"passcode":"onlylowercaseletters","deviceModel":"onlylowercaseletters"`, "onlylowercaseletters"},
	{"WEAKSHORT", `"pin":"Zq7x2","deviceModel":"Zq7x2"`, "Zq7x2"},
	{"SHORT", `"pin":"Zq7x2","deviceModel":"WnE3eDI="`, "Zq7x2"}, // the same value, base64 in its own answer
}

// testKey is a fresh P-256 key: its PEM text, the base64 lines of its body,
// and the body joined.
type testKey struct {
	pem   string
	lines []string
	body  string
}

func newKey(t *testing.T) testKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	text := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	var key testKey
	key.pem = text
	for _, line := range strings.Split(text, "\n") {
		if line != "" && !strings.HasPrefix(line, "-----") {
			key.lines = append(key.lines, line)
		}
	}
	key.body = strings.Join(key.lines, "")
	return key
}

// jsonEscaped is s with every character written as a \uXXXX escape.
func jsonEscaped(s string) string {
	var b strings.Builder
	for _, r := range s {
		fmt.Fprintf(&b, `\u%04x`, r)
	}
	return b.String()
}

// forms are the encodings the scan looks for, independent of the funnel.
func forms(s string) []string {
	return []string{
		s, base64.StdEncoding.EncodeToString([]byte(s)), base64.RawStdEncoding.EncodeToString([]byte(s)),
		base64.URLEncoding.EncodeToString([]byte(s)), base64.RawURLEncoding.EncodeToString([]byte(s)),
		hex.EncodeToString([]byte(s)), strings.ToUpper(hex.EncodeToString([]byte(s))),
		url.QueryEscape(s), url.PathEscape(s), jsonEscaped(s),
	}
}

type fake struct {
	srv, evil  *httptest.Server
	caFile     string
	key        testKey
	evilHits   atomic.Int32
	activities atomic.Int32
	hostFile   atomic.Int32 // requests carrying the sentinel host file's content
	mu         sync.Mutex
	assertions []string
	bearers    []string // the Authorization of each API request
}

// bearersSeen returns the Authorization values sent so far and starts a new list.
func (f *fake) bearersSeen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.bearers
	f.bearers = nil
	return out
}

// secrets are the key's pieces, the access token and every client assertion
// the fake has been sent.
func (f *fake) secrets() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := append([]string{accessToken, configToken, homeToken, headerToken, looseToken, echoedToken, f.key.body}, f.key.lines...)
	for _, w := range weakVendor {
		out = append(out, w.value)
	}
	return append(out, f.assertions...)
}

func (f *fake) assertionCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.assertions)
}

func (f *fake) lastAssertion() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.assertions) == 0 {
		return "no-assertion-yet"
	}
	return f.assertions[len(f.assertions)-1]
}

// leaks lists every credential text repeats, in each form forms names.
func (f *fake) leaks(text string) []string {
	var found []string
	all := f.secrets()
	for _, s := range all {
		for _, form := range forms(s) {
			if strings.Contains(text, form) {
				found = append(found, fmt.Sprintf("%.16q... as %.24q...", s, form))
			}
		}
	}
	return found
}

func device(id, model string) string {
	return `{"type":"orgDevices","id":"` + id + `","attributes":{"serialNumber":"` + id + `","productFamily":"Mac","deviceModel":"` + model + `","status":"UNASSIGNED","addedToOrgDateTime":"2026-10-20T00:00:00Z","orderNumber":"ORD-1"}}`
}

func newFake(t *testing.T) *fake {
	f := &fake{key: newKey(t)}
	f.evil = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.evilHits.Add(1)
		_, _ = io.WriteString(w, `{"stolen":true}`)
	}))
	t.Cleanup(f.evil.Close)
	ok := func(w http.ResponseWriter, code int, body string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(r.URL.String()+string(body), "SENTINELHOSTFILE") {
			f.hostFile.Add(1)
		}
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			f.mu.Lock()
			f.bearers = append(f.bearers, r.Header.Get("Authorization"))
			f.mu.Unlock()
		}
		if r.URL.Path == "/auth/oauth2/token-echo" {
			// A refused mint whose error answer carries a credential.
			ok(w, 400, `{"error":"invalid_client","access_token":"`+echoedToken+`"}`)
			return
		}
		for _, wv := range weakVendor {
			if r.URL.Path == "/v1/orgDevices/"+wv.id {
				ok(w, 200, `{"data":{"type":"orgDevices","id":"`+wv.id+`","attributes":{"serialNumber":"`+wv.id+`",`+wv.attrs+`}},"links":{}}`)
				return
			}
		}
		if r.URL.Path == "/v1/orgDevices/BINARYNEW" {
			// A non-textual answer that introduces a plainly named credential.
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, "password="+binaryCredential)
			return
		}
		if r.URL.Path == "/v1/orgDevices/BINTOKEN" {
			// A non-textual answer that repeats the access token.
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, accessToken)
			return
		}
		if r.URL.Path == "/auth/oauth2/token" {
			form, _ := url.ParseQuery(string(body))
			a := form.Get("client_assertion")
			if a == "" {
				ok(w, 400, `{"error":"invalid_client"}`)
				return
			}
			f.mu.Lock()
			f.assertions = append(f.assertions, a)
			f.mu.Unlock()
			ok(w, 200, `{"access_token":"`+accessToken+`","token_type":"Bearer","expires_in":3600}`)
			return
		}
		switch r.URL.Path {
		case "/v1/orgDevices":
			ok(w, 200, `{"data":[`+device("SERIALA001", "MacBook Air")+`],"links":{"self":"/v1/orgDevices"},"meta":{"paging":{"limit":100}}}`)
		case "/v1/orgDevices/ECHOTOKEN":
			// The vendor answer repeats the access token, base64-encoded.
			ok(w, 200, `{"data":`+device("ECHOTOKEN", base64.StdEncoding.EncodeToString([]byte(accessToken)))+`}`)
		case "/v1/orgDevices/ECHOKEY":
			// One line of the private key, hex-encoded.
			ok(w, 200, `{"data":`+device("ECHOKEY", hex.EncodeToString([]byte(f.key.lines[1])))+`}`)
		case "/v1/orgDevices/ECHOBODY":
			// The whole key body as JSON \u escapes: decoding the answer yields it raw.
			ok(w, 200, `{"data":`+device("ECHOBODY", jsonEscaped(f.key.body))+`}`)
		case "/v1/orgDevices/PUBLICID":
			// The public client ID, which the program prints by design.
			ok(w, 200, `{"data":`+device("PUBLICID", clientID)+`}`)
		case "/v1/orgDevices/ECHOASSERT":
			// The signed client assertion, percent-encoded in an error detail.
			ok(w, 404, `{"errors":[{"status":"404","detail":"no device for `+url.QueryEscape(f.lastAssertion())+`"}]}`)
		case "/v1/orgDevices/REDIRECT":
			http.Redirect(w, r, f.evil.URL+"/steal", http.StatusFound)
		case "/v1/orgDevices/REDIRECTPUB":
			// A public name passes the loopback dial guard; only the host pin stops it.
			http.Redirect(w, r, "https://evil.invalid/steal", http.StatusFound)
		case "/v1/mdmServers":
			ok(w, 200, `{"data":[{"type":"mdmServers","id":"S1","attributes":{"serverName":"Jamf Prod","status":"ACTIVE","serverType":"MDM"}}],"meta":{"paging":{"limit":100}}}`)
		case "/v1/orgDeviceActivities":
			if r.Method == http.MethodPost {
				f.activities.Add(1)
				ok(w, 201, `{"data":{"type":"orgDeviceActivities","id":"act-1","attributes":{"status":"IN_PROGRESS","subStatus":"SUBMITTED"}}}`)
				return
			}
			ok(w, 405, `{"errors":[{"status":"405"}]}`)
		case "/v1/auditEvents":
			// Apple's documented payload shape: eventDataPropertyKey names the
			// eventData<Type> object (AuditEventCommonAttributes).
			ok(w, 200, `{"data":[{"type":"auditEvents","id":"e1","attributes":{"eventDateTime":"`+time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)+`","type":"BATCH_PASSWORD_RESET_INITIATED","category":"ACCOUNTS","outcome":"SUCCESS","actorType":"USER","actorName":"admin@example.com","subjectType":"USER","subjectName":"Pat","eventDataPropertyKey":"eventDataBatchPasswordResetInitiated","eventDataBatchPasswordResetInitiated":{"userCount":"7"}}}],"meta":{"paging":{"limit":100}}}`)
		default:
			ok(w, 404, `{"errors":[{"status":"404","detail":"not found"}]}`)
		}
	}))
	t.Cleanup(f.srv.Close)
	// The credential kit refuses to send a credential over plain http, so the
	// fake is https and the binaries trust its certificate.
	f.caFile = filepath.Join(t.TempDir(), "fake-ca.pem")
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.srv.Certificate().Raw})
	if err := os.WriteFile(f.caFile, ca, 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

var (
	buildOnce sync.Once
	binDir    string
	buildErr  error
)

func binaries(t *testing.T) string {
	t.Helper()
	// Point at another build's binaries to see a test fail on code it predates.
	if dir := os.Getenv("APPLE_BUSINESS_BLACKBOX_BIN"); dir != "" {
		return dir
	}
	buildOnce.Do(func() {
		binDir, buildErr = os.MkdirTemp("", "apple-business-blackbox-bin-")
		if buildErr != nil {
			return
		}
		ext := ""
		if runtime.GOOS == "windows" {
			ext = ".exe"
		}
		for _, name := range []string{"apple-business-cli", "apple-business-mcp"} {
			out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, name+ext), "../../cmd/"+name).CombinedOutput()
			if err != nil {
				buildErr = fmt.Errorf("go build %s: %v\n%s", name, err, out)
				return
			}
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return binDir
}

// env runs a binary as a fresh operator: its own home, the credentials in
// the environment, every CLI and XDG override cleared. keyRef is the private
// key reference: a path or the PEM text itself.
func env(home, keyRef string, f *fake) []string {
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "APPLE_BUSINESS_") || strings.HasPrefix(name, "XDG_") || name == "HOME" || name == "USERPROFILE" || name == "APPDATA" || name == "LOCALAPPDATA" {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "HOME="+home, "USERPROFILE="+home, "APPDATA="+filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA="+filepath.Join(home, "AppData", "Local"),
		"APPLE_BUSINESS_CLIENT_ID="+clientID, "APPLE_BUSINESS_KEY_ID="+keyID, "APPLE_BUSINESS_PRIVATE_KEY="+keyRef,
		"APPLE_BUSINESS_BASE_URL="+f.srv.URL+"/v1", "APPLE_BUSINESS_TOKEN_URL="+f.srv.URL+"/auth/oauth2/token",
		"APPLE_BUSINESS_CA_CERT="+f.caFile, "SSL_CERT_FILE="+f.caFile)
}

// keyFile writes the key outside home: the operator's own .pem, which the
// program reads and must never copy.
func keyFile(t *testing.T, f *fake) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "AuthKey.pem")
	if err := os.WriteFile(p, []byte(f.key.pem), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func runCLI(t *testing.T, bin, home, keyRef string, f *fake, args ...string) (string, int) {
	t.Helper()
	return runCLIEnv(t, bin, home, keyRef, f, nil, args...)
}

// runCLIEnv is runCLI with extra environment entries (later entries win).
func runCLIEnv(t *testing.T, bin, home, keyRef string, f *fake, extra []string, args ...string) (string, int) {
	t.Helper()
	// A run that hangs (a deadlocked token mint) fails the test, not the suite.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(bin, "apple-business-cli"), args...)
	cmd.Env = append(env(home, keyRef, f), extra...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Errorf("%v did not finish within 90s (hung):\n%s", args, out.String())
		return out.String(), -1
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
	// A fixture that does not parse makes the binary fail with a config error whose text the
	// kit may withhold, or pass vacuously; a Windows path in a basic string ("C:\Users") is one.
	if strings.HasSuffix(path, ".toml") {
		var probe map[string]any
		if err := toml.Unmarshal([]byte(content), &probe); err != nil {
			t.Fatalf("fixture %s does not parse as TOML (quote paths as 'literal' strings): %v", filepath.Base(path), err)
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func scanFiles(t *testing.T, home string, f *fake) {
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
		// The one file allowed to hold a credential is the private credential store.
		if d.Name() == "credentials.toml" {
			if info, err := d.Info(); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
				t.Errorf("credential store %s is %v, want 0600", path, info.Mode().Perm())
			}
			return nil
		}
		if found := f.leaks(string(data)); len(found) > 0 {
			t.Errorf("file %s holds a credential: %v", path, found)
		}
		return nil
	})
	if n == 0 {
		t.Fatal("the runs wrote no file at all; the file scan proves nothing")
	}
}

func TestCLIRunLeaksNoCredential(t *testing.T) {
	bin := binaries(t)
	f := newFake(t)
	home := t.TempDir()
	if keep := os.Getenv("APPLE_BUSINESS_BLACKBOX_KEEP"); keep != "" {
		home = keep
	}
	keyRef := keyFile(t, f)

	out, code := runCLI(t, bin, home, keyRef, f, "devices", "list", "--json")
	if code != 0 || !strings.Contains(out, "SERIALA001") {
		t.Fatalf("devices list must answer (exit %d):\n%s", code, out)
	}
	if f.assertionCount() == 0 {
		t.Fatal("no client assertion reached the fake; the token mint was never exercised")
	}
	all := out
	for _, args := range [][]string{
		{"sync", "--resources", "devices,mdm_servers", "--json"},
		{"devices", "get", "ECHOTOKEN", "--json"},
		{"devices", "get", "ECHOKEY", "--json"},
		{"devices", "get", "ECHOBODY", "--json"},
		{"devices", "get", "ECHOASSERT", "--json"},
		{"devices", "get", "REDIRECT", "--json"},
		{"devices", "get", "REDIRECTPUB", "--json"},
		{"devices", "assign", "--server", "Jamf Prod", "SERIALA001", "--dry-run", "--json"},
		{"--dry-run", "device-activities", "create", "--json"},
		{"doctor", "--json"},
		{"auth", "status", "--json"},
		{"devices", "list", "--no-such-flag", f.key.lines[0]},
		{"feedback", "the key is " + f.key.lines[2]},
	} {
		out, _ := runCLI(t, bin, home, keyRef, f, args...)
		all += "\n" + out
		if found := f.leaks(out); len(found) > 0 {
			t.Errorf("%v printed a credential: %v\n%s", args, found, out)
		}
	}
	// Vendor credentials of any strength, a binary answer (printed and
	// exported to a file), and a token endpoint error that carries a token.
	runs := [][]string{
		{"devices", "get", "BINTOKEN", "--json"},
		{"export", "devices", "BINTOKEN", "--output", filepath.Join(home, "export.jsonl")},
	}
	for _, w := range weakVendor {
		runs = append(runs, []string{"devices", "get", w.id, "--json"})
	}
	for _, args := range runs {
		out, _ := runCLI(t, bin, home, keyRef, f, args...)
		all += "\n" + out
		if found := f.leaks(out); len(found) > 0 {
			t.Errorf("%v printed a credential: %v\n%s", args, found, out)
		}
	}
	echoURL := []string{"APPLE_BUSINESS_TOKEN_URL=" + f.srv.URL + "/auth/oauth2/token-echo"}
	for _, args := range [][]string{{"auth", "login"}, {"devices", "list", "--json"}} {
		out, code := runCLIEnv(t, bin, home, keyRef, f, echoURL, args...)
		all += "\n" + out
		if code == 0 {
			t.Errorf("%v must fail against a refusing token endpoint:\n%s", args, out)
		}
		if found := f.leaks(out); len(found) > 0 {
			t.Errorf("%v printed a credential from the token endpoint's error answer: %v\n%s", args, found, out)
		}
	}
	// Credentials only a config file holds, read after the flags parse: a
	// --config file, a --home config and configured headers (in a --config
	// file and in the default config). A flag error must not print them.
	cfgFile := filepath.Join(t.TempDir(), "customer.toml")
	writePrivate(t, cfgFile, "access_token = \""+configToken+"\"\n[headers]\nAuthorization = \"Bearer "+headerToken+"\"\n")
	homeDir := t.TempDir()
	writePrivate(t, filepath.Join(homeDir, "config", "config.toml"), "access_token = \""+homeToken+"\"\n")
	looseFile := filepath.Join(t.TempDir(), "loose.toml")
	writePrivate(t, looseFile, "access_token = \""+looseToken+"\"\n")
	if runtime.GOOS != "windows" {
		if err := os.Chmod(looseFile, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	defHome := t.TempDir() // not scanned: its config.toml holds the header by design
	writePrivate(t, filepath.Join(defHome, ".config", "apple-business-cli", "config.toml"), "[headers]\nAuthorization = \"Bearer "+headerToken+"\"\n")
	for _, run := range []struct {
		home string
		args []string
	}{
		{home, []string{"--config", cfgFile, "devices", "list", "--" + configToken}},
		{home, []string{"--config=" + cfgFile, "devices", "list", "--" + headerToken}},
		{home, []string{"--home", homeDir, "devices", "list", "--" + homeToken}},
		{home, []string{"--config", looseFile, "devices", "list", "--" + looseToken}},
		{defHome, []string{"devices", "list", "--" + headerToken}},
	} {
		out, _ := runCLI(t, bin, run.home, keyRef, f, run.args...)
		all += "\n" + out
		if found := f.leaks(out); len(found) > 0 {
			t.Errorf("%v printed a credential its config holds: %v\n%s", run.args, found, out)
		}
	}
	// A documented audit-event payload is data, not a credential.
	out, _ = runCLI(t, bin, home, keyRef, f, "audit-events", "digest", "--since", "7d", "--json")
	all += "\n" + out
	if !strings.Contains(out, "userCount") {
		t.Errorf("an audit event's documented eventData payload was withheld or redacted:\n%s", out)
	}
	// The client ID is public: an answer that carries it, after a token mint
	// in the same run, is shown, not withheld.
	out, _ = runCLI(t, bin, home, keyRef, f, "devices", "get", "PUBLICID", "--json")
	all += "\n" + out
	if !strings.Contains(out, clientID) {
		t.Errorf("an answer carrying the public client ID was withheld or redacted:\n%s", out)
	}
	if !strings.Contains(all, "withheld") {
		t.Errorf("no answer was withheld, so the echo answers never reached the funnel:\n%s", all)
	}
	if n := f.evilHits.Load(); n != 0 {
		t.Fatalf("the redirect reached another host %d time(s)", n)
	}
	if !strings.Contains(all, "refusing a request to") {
		t.Errorf("the off-host redirect was not refused by the host pin:\n%s", all)
	}
	if n := f.activities.Load(); n != 0 {
		t.Fatalf("a --dry-run sent %d device activit(ies)", n)
	}
	scanFiles(t, home, f)
}

// mcpSession speaks newline-delimited JSON-RPC to the MCP server.
type mcpSession struct {
	t      *testing.T
	in     io.WriteCloser
	out    *bufio.Scanner
	stderr *bytes.Buffer
	all    strings.Builder
	id     int
}

func (s *mcpSession) send(method string, params any) map[string]any {
	s.t.Helper()
	s.id++
	msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": s.id, "method": method, "params": params})
	if _, err := s.in.Write(append(msg, '\n')); err != nil {
		s.t.Fatal(err)
	}
	for s.out.Scan() {
		line := s.out.Text()
		s.all.WriteString(line + "\n")
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) != nil {
			continue
		}
		if id, ok := m["id"].(float64); ok && int(id) == s.id {
			return m
		}
	}
	s.t.Fatalf("MCP server closed before answering %s; stderr:\n%s", method, s.stderr)
	return nil
}

func (s *mcpSession) call(name string, args map[string]any) string {
	s.t.Helper()
	m := s.send("tools/call", map[string]any{"name": name, "arguments": args})
	b, _ := json.Marshal(m)
	return string(b)
}

func TestMCPRunLeaksNoCredentialAndWritesNeedConfirm(t *testing.T) {
	bin := binaries(t)
	f := newFake(t)
	home := t.TempDir()
	cmd := exec.Command(filepath.Join(bin, "apple-business-mcp"))
	// The PEM text itself in the environment, with literal \n as MCP configs carry it.
	cmd.Env = env(home, strings.ReplaceAll(strings.TrimSpace(f.key.pem), "\n", `\n`), f)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	in, _ := cmd.StdinPipe()
	outPipe, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() })
	sc := bufio.NewScanner(outPipe)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	s := &mcpSession{t: t, in: in, out: sc, stderr: stderr}
	s.send("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "blackbox", "version": "1"}})
	_, _ = in.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"))

	read := s.call("apple-business_read", map[string]any{"endpoint_id": "devices.list"})
	if !strings.Contains(read, "SERIALA001") {
		t.Fatalf("apple-business_read devices.list did not answer: %s", read)
	}
	for _, id := range []string{"ECHOTOKEN", "ECHOKEY", "ECHOBODY", "ECHOASSERT", "REDIRECT", "REDIRECTPUB"} {
		s.call("apple-business_read", map[string]any{"endpoint_id": "devices.get", "params": map[string]any{"id": id}})
	}
	if res := s.call("sync", map[string]any{"resources": "devices", "confirm": true}); strings.Contains(res, `\"isError\":true`) || strings.Contains(res, "Preview only") {
		t.Errorf("a confirmed sync must run: %.400s", res)
	}
	if res := s.call("sql", map[string]any{"query": "SELECT data FROM resources"}); !strings.Contains(res, "SERIALA001") {
		t.Errorf("the synced device must come back through sql: %.400s", res)
	}
	// The protocol layer itself echoes request text (an unknown tool name)
	// before any tool middleware runs.
	if res := s.call(f.key.lines[0], map[string]any{}); !strings.Contains(res, `"error"`) {
		t.Errorf("an unknown tool must still answer with a JSON-RPC error: %s", res)
	}

	// A write previews until confirm is boolean true, whatever else is passed.
	for _, args := range []map[string]any{
		{"serial": []any{"SERIALA001"}, "server": "Jamf Prod"},
		{"serial": []any{"SERIALA001"}, "server": "Jamf Prod", "dry-run": false},
		{"serial": []any{"SERIALA001"}, "server": "Jamf Prod", "dry-run": "false"},
		{"serial": []any{"SERIALA001"}, "server": "Jamf Prod", "confirm": "true"},
	} {
		res := s.call("devices_assign", args)
		if n := f.activities.Load(); n != 0 {
			t.Fatalf("devices_assign %v sent without confirm: true (%d activit(ies)): %s", args, n, res)
		}
	}
	s.call("devices_unassign", map[string]any{"serial": []any{"SERIALA001"}})
	s.call("apple-business_execute", map[string]any{"endpoint_id": "device_activities.create",
		"params": map[string]any{"data": map[string]any{"type": "orgDeviceActivities"}}})
	if n := f.activities.Load(); n != 0 {
		t.Fatalf("a write without confirm: true sent %d activit(ies)", n)
	}
	// A host-path flag is not a tool argument: no MCP call reads a host file.
	sentinel := filepath.Join(t.TempDir(), "serials.txt")
	if err := os.WriteFile(sentinel, []byte("SENTINELHOSTFILE01\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"devices_lookup", "devices_unassign"} {
		s.call(tool, map[string]any{"file": sentinel, "confirm": true})
	}
	if n := f.hostFile.Load(); n != 0 {
		t.Fatalf("an MCP tool argument read a host file: %d request(s) carried its content", n)
	}
	res := s.call("devices_assign", map[string]any{"serial": []any{"SERIALA001"}, "server": "Jamf Prod", "confirm": true})
	if f.activities.Load() != 1 {
		t.Fatalf("confirm: true must send the activity once (the gate is not just closed): %s", res)
	}

	_ = in.Close()
	text := s.all.String() + "\n" + stderr.String()
	if found := f.leaks(text); len(found) > 0 {
		t.Errorf("the MCP run printed a credential: %v\n%s", found, text)
	}
	if n := f.evilHits.Load(); n != 0 {
		t.Fatalf("the redirect reached another host %d time(s)", n)
	}
	scanFiles(t, home, f)
}

// syncBuffer is a bytes.Buffer safe to read while a process writes it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestMCPHTTPLogsPassTheChokepoint: the streamable-HTTP transport logs
// through Go's default logger (slog.Default), which was bound to the real
// stderr before the chokepoint replaced os.Stderr. A session ID that repeats
// the caller token is logged; that line must reach stderr withheld.
func TestMCPHTTPLogsPassTheChokepoint(t *testing.T) {
	bin := binaries(t)
	f := newFake(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	cmd := exec.Command(filepath.Join(bin, "apple-business-mcp"), "--transport", "http", "--addr", addr)
	cmd.Env = append(env(t.TempDir(), keyFile(t, f), f), "APPLE_BUSINESS_MCP_HTTP_TOKEN="+httpToken)
	stderr := &syncBuffer{}
	cmd.Stdout, cmd.Stderr = io.Discard, stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	deadline := time.Now().Add(15 * time.Second)
	for {
		if c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
			_ = c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("MCP HTTP server never listened on %s; stderr:\n%s", addr, stderr)
		}
		time.Sleep(50 * time.Millisecond)
	}
	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":{"x":1}}`))
	req.Header.Set("Authorization", "Bearer "+httpToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", httpToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	// The log line is the positive signal: wait until it shows, withheld or not.
	for deadline = time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if s := stderr.String(); strings.Contains(s, "withheld") || strings.Contains(s, "sampling response") {
			break
		}
	}
	text := stderr.String()
	for _, form := range forms(httpToken) {
		if strings.Contains(text, form) {
			t.Fatalf("the HTTP transport logged the caller token past the chokepoint:\n%s", text)
		}
	}
	if !strings.Contains(text, "withheld") {
		t.Fatalf("the transport's log line never reached stderr through the chokepoint:\n%s", text)
	}
}
