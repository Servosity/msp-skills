// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package exoblackbox runs the built CLI and MCP binaries against an Exchange
// Online admin API fake that echoes every credential it knows back in every
// encoding it can, whole and split across lines and adjacent values, and
// checks what a reviewer cannot read off the code (exchange-online handfixes
// credkit-wiring, store-never-keeps-secrets, no-response-cache,
// response-credentials-held, api-origin-pinned):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the bearer token from the environment or the
//     credential store, the MCP HTTP token, a password sent in a cmdlet
//     parameter (flag, --stdin body or MCP params), a credential-named flag
//     value, and the credentials a response carries;
//   - no file the program writes, except the 0600 credential store, holds
//     one either;
//   - no credential leaves for a host other than the configured one, by
//     redirect or by @odata.nextLink.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder.
package exoblackbox

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
	bbTenant    = "0550b473-9fd2-4dbb-a058-a1640b4bf458"
	bbToken     = "exoTOKEN7fb3189bQ6u3sZxW4o1n7R8v52"  // EXCHANGE_ONLINE_TOKEN
	bbStored    = "storedToken-71c3e9b05d2a4f68e1"      // auth set-token (stdin), the credential store
	bbPassword  = "ExoPassw0rd-" + "5c1e9a7d3b"         // a Password cmdlet parameter in a --stdin body
	bbMCPPass   = "McpPassw0rd-2e8a4c6f1b9d"            // a Password cmdlet parameter in MCP execute params
	bbArgvSec   = "-argvSecret-91c3e7a5"                // --client-secret on the command line (starts with "-")
	bbShortResp = "Sh0rtPw"                             // a 7-character credential a response carries
	bbBOMResp   = "BomRespSecret-6a1c"                  // a credential in a BOM-prefixed response
	bbGzipResp  = "GzipRespToken-4f2a9c1e"              // a credential in a gzip-encoded response
	bbHTTPToken = "mcpHttpToken-0123456789abcdefABCDEF" // EXCHANGE_ONLINE_MCP_HTTP_TOKEN
)

func secrets() []string {
	return []string{bbToken, bbStored, bbPassword, bbMCPPass, bbArgvSec, bbShortResp, bbBOMResp, bbGzipResp, bbHTTPToken}
}

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "exo-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "exchange-online-cli"+exe)
	bbMCP = filepath.Join(dir, "exchange-online-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/exchange-online-cli", bbMCP: "../../cmd/exchange-online-mcp"} {
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

// sentSecrets is every known secret the request carried (its headers, a
// decoded Basic credential, the URL or the body): a real API can echo only
// what it was sent.
func sentSecrets(r *http.Request) []string {
	body, _ := io.ReadAll(r.Body)
	seen := fmt.Sprint(r.Header) + "\n" + r.URL.String() + "\n" + string(body)
	if u, p, ok := r.BasicAuth(); ok {
		seen += "\n" + u + "\n" + p
	}
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
	honest  atomic.Bool       // an API that echoes nothing it was sent
	routes  map[string]string // a fixed answer per cmdlet
	hits    atomic.Int64
}

// cmdletOf reads the CmdletName from a cmdlet envelope body.
func cmdletOf(body []byte) string {
	var env struct {
		CmdletInput struct {
			CmdletName string `json:"CmdletName"`
		} `json:"CmdletInput"`
	}
	_ = json.Unmarshal(body, &env)
	return env.CmdletInput.CmdletName
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

func echoServer(t *testing.T) *server {
	t.Helper()
	sv := &server{}
	// A second host the API redirects and links to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		sent := sentSecrets(r)
		if sv.honest.Load() {
			sent = nil
		}
		echo := echoObject(sent)
		w.Header().Set("Content-Type", "application/json")
		if route, ok := sv.routes[cmdletOf(body)]; ok {
			fmt.Fprint(w, route)
			return
		}
		switch cmdlet := cmdletOf(body); cmdlet {
		case "Get-Recipient":
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":{"code":"401","message":"invalid token %s","details":[{"message":"bad token %s"}]},"echo":%s}`, strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case "Get-AcceptedDomain":
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case "Get-MobileDevice":
			fmt.Fprintf(w, `{"value":[{"Identity":"dev1","FriendlyName":"HQ-Phone"}],"@odata.nextLink":%q}`, other.URL+r.URL.Path+"?$skiptoken=2")
		case "Get-InboxRule":
			fmt.Fprintf(w, `{"value":[{"Name":"HQ-Rule","Enabled":true,"split":%s}]}`, splitObject(sent))
		case "Get-MailboxStatistics":
			fmt.Fprintf(w, "\xef\xbb\xbf"+`{"value":[{"DisplayName":%q,"Password":%q}]}`, bbBOMResp, bbBOMResp)
		case "Get-MailboxAutoReplyConfiguration":
			fmt.Fprintf(w, `{"value":[{"Identity":"rosa","InternalMessage":%q,"Password":%q}]}`, bbShortResp, bbShortResp)
		case "Get-CASMailbox":
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			fmt.Fprintf(zw, `{"value":[{"Identity":"rosa@example.com","EwsEnabled":true,"AccessToken":%q,"echo":%s}]}`, bbGzipResp, echoObject(append(sent, bbGzipResp)))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		default:
			if !strings.HasPrefix(cmdlet, "Get-") {
				fmt.Fprintf(w, `{"value":[{"Result":"done","echo":%s}]}`, echo)
				return
			}
			extra := ""
			if len(sent) > 0 {
				// A record whose identifier repeats the token it was sent.
				extra = fmt.Sprintf(`,{"Identity":%q,"Name":"Echo-ID"}`, sent[0])
			}
			fmt.Fprintf(w, `{"value":[{"Identity":"hq@example.com","UserPrincipalName":"hq@example.com","PrimarySmtpAddress":"hq@example.com","Name":"HQ-Record","RecipientTypeDetails":"UserMailbox","ReleaseStatus":"Requested","ReceivedTime":"2026-10-20T10:00:00Z","echo":%s}%s]}`, echo, extra)
		}
	}))
	sv.url = srv.URL
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

// shortTemp is an empty directory. Not t.TempDir: its path carries the test's name,
// which can share a piece with the tokens these tests hold.
func shortTemp(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "exo-kit-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
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
		"EXCHANGE_ONLINE_BASE_URL=" + sv.url, "EXCHANGE_ONLINE_CA_CERT=" + sv.caFile,
		"EXCHANGE_ONLINE_CLI_PATH=" + bbCLI, "EXCHANGE_ONLINE_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}
	if withEnvToken {
		env = append(env, "EXCHANGE_ONLINE_TOKEN="+bbToken)
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
	var out, errb bytes.Buffer
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
		case <-waitDone: // it refused to start, or stopped
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

// battery runs every output path; tok is the credential the process holds
// (the environment token, or the stored one when the environment has none).
func battery(e *bbEnv, deliverFile, tok string) [][]string {
	T := bbTenant
	return [][]string{
		{"mailboxes", "list", T, "--json"},
		{"mailboxes", "list", T},
		{"mailboxes", "list", T, "--csv"},
		{"mailboxes", "list", T, "--plain"},
		{"mailboxes", "list", T, "--json", "--receipt"},
		{"mailboxes", "list", T, "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		{"mailboxes", "list", T, "--json", "--deliver", "file:" + deliverFile},
		{"inbox-rules", "list", T, "--mailbox", "rosa@example.com", "--json"},
		{"inbox-rules", "list", T, "--mailbox", "rosa@example.com"},
		{"recipients", "list", T, "--json"},
		{"accepted-domains", T, "--json"},
		{"mobile-devices", "list", T, "--mailbox", "rosa@example.com", "--json"},
		{"mailbox-stats", T, "--identity", "rosa@example.com", "--json"},
		{"auto-reply", "get", T, "--identity", "rosa@example.com", "--json"},
		{"client-access", "list", T, "--json"},
		{"mailboxes", "set", T, "--identity", "rosa@example.com", "--type", "Shared", "--json"},
		{"mailboxes", "set", T, "--identity", "rosa@example.com", "--dry-run", "--json"},
		{"mailboxes", "list", T, "--client-secret", bbArgvSec},
		{"mailboxes", "list", T, "--json", "--" + tok},
		{"ews-posture", T, "--json"},
		{"forwarding-audit", T, "--json"},
		{"forwarding-audit", T, "--sweep", "--json"},
		{"inbox-rules", "sweep", T, "--json"},
		{"bec", "triage", T, "rosa@example.com", "--json"},
		{"quarantine", "snapshot", T, "--json"},
		{"quarantine", "queue", "--json"},
		{"where-is", T, "--recipient", "rosa@example.com", "--json"},
		{"access-map", T, "rosa@example.com", "--json"},
		{"offboard", T, "rosa@example.com", "--plan", "--json"},
		{"feedback", "noted the token " + tok + " in a ticket"},
		// Learnings, playbooks and learn events land in the local store's own tables.
		{"teach", "--query", "find mailboxes " + tok, "--resource", "123", "--resource-type", "mailboxes",
			"--notes", "rotated " + tok, "--playbook-notes", "use " + tok, "--no-validate"},
		{"doctor"},
		{"auth", "status"},
	}
}

func mcpBattery() []map[string]any {
	T := bbTenant
	return []map[string]any{
		{"name": "exchange-online_read", "arguments": map[string]any{"endpoint_id": "mailboxes.list", "params": map[string]any{"tenantId": T}}},
		{"name": "exchange-online_read", "arguments": map[string]any{"endpoint_id": "inbox_rules.list", "params": map[string]any{"tenantId": T, "Mailbox": "rosa@example.com"}}},
		{"name": "exchange-online_read", "arguments": map[string]any{"endpoint_id": "recipients.list", "params": map[string]any{"tenantId": T}}},
		{"name": "exchange-online_read", "arguments": map[string]any{"endpoint_id": "mailbox_stats.get", "params": map[string]any{"tenantId": T, "Identity": "rosa@example.com"}}},
		{"name": "exchange-online_read", "arguments": map[string]any{"endpoint_id": "client_access.list", "params": map[string]any{"tenantId": T}}},
		{"name": "exchange-online_execute", "arguments": map[string]any{"endpoint_id": "mailboxes.set", "params": map[string]any{"tenantId": T, "Identity": "rosa@example.com", "Password": bbMCPPass}}},
		{"name": "exchange-online_execute", "arguments": map[string]any{"endpoint_id": "mailboxes.set", "params": map[string]any{"tenantId": T, "Identity": "rosa@example.com", "Password": bbMCPPass}, "confirm": true}},
		{"name": "bec_triage", "arguments": map[string]any{"tenantId": T, "mailbox": "rosa@example.com"}},
		{"name": "where_is", "arguments": map[string]any{"tenantId": T, "recipient": "rosa@example.com"}},
		{"name": "offboard", "arguments": map[string]any{"tenantId": T, "mailbox": "rosa@example.com"}},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
	}
}

// stdinRuns feed a request body on stdin (mailboxes set --stdin): previewed
// and sent.
func stdinRuns(e *bbEnv, t *testing.T) string {
	body := fmt.Sprintf(`{"Identity":"rosa@example.com","Password":%q}`, bbPassword)
	return e.runIn(t, body, "mailboxes", "set", bbTenant, "--stdin", "--dry-run", "--json") + e.runIn(t, body, "mailboxes", "set", bbTenant, "--stdin", "--json")
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
	stdinRuns(e, t)
	e.mcpCalls(t, mcpBattery())
	var scanned int
	var sawCreds bool
	scan := func() {
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
	// Against an API that echoes nothing the same writes do write data, so the scan
	// is not vacuous; and then it runs again, over what landed.
	sv.honest.Store(true)
	e.run(t, "mailboxes", "list", bbTenant, "--json", "--deliver", "file:"+deliverFile)
	e.run(t, "quarantine", "snapshot", bbTenant, "--json")
	e.run(t, "teach", "--query", "find mailboxes", "--resource", "123", "--resource-type", "mailboxes", "--notes", "rotated on schedule", "--no-validate")
	scan()
	if b, err := os.ReadFile(deliverFile); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
		t.Errorf("%s: want the delivered record (err %v)", filepath.Base(deliverFile), err)
	}
	var sawStore, sawSnapshot, sawLearning bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(d.Name(), ".db") {
			if b, rerr := os.ReadFile(path); rerr == nil {
				sawStore = sawStore || bytes.Contains(b, []byte("HQ-Record"))
				sawSnapshot = sawSnapshot || (strings.Contains(d.Name(), "snapshots") && bytes.Contains(b, []byte("HQ-Record")))
				sawLearning = sawLearning || bytes.Contains(b, []byte("rotated on schedule"))
			}
		}
		return nil
	})
	if !sawStore {
		t.Errorf("no local store holds a fetched record")
	}
	if !sawSnapshot {
		t.Errorf("the snapshot database holds no quarantine snapshot record")
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
	assertNoEcho(t, "output of mailboxes set --stdin", stdinRuns(e, t))
	// A response that repeats a held value is withheld whole, with the kit's notice.
	if out := e.run(t, "mailboxes", "list", bbTenant, "--json"); !strings.Contains(out, bbWithheld) {
		t.Errorf("want the kit's notice for an echoing response, got:\n%.1500s", out)
	}
	out := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
		t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP session", out)
	// An API that echoes nothing: the records print, a credential member is replaced
	// by name, and JSON output stays valid JSON.
	sv.honest.Store(true)
	cmd := exec.Command(bbCLI, "mailboxes", "list", bbTenant, "--json")
	cmd.Env, cmd.Dir = e.env, e.home
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("mailboxes list --json: %v", err)
	}
	var doc any
	if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("HQ-Record")) {
		t.Fatalf("want valid JSON with the record, got:\n%.1500s", stdout)
	}
	assertNoEcho(t, "honest mailboxes list", string(stdout))
	stats := e.run(t, "mailbox-stats", bbTenant, "--identity", "rosa@example.com", "--json")
	if !strings.Contains(stats, "[redacted]") && !strings.Contains(stats, bbWithheld) {
		t.Errorf("a credential member is neither replaced nor withheld:\n%.1500s", stats)
	}
	assertNoEcho(t, "honest mailbox-stats", stats)
	honestMCP := e.mcpCalls(t, []map[string]any{{"name": "exchange-online_read", "arguments": map[string]any{"endpoint_id": "mailboxes.list", "params": map[string]any{"tenantId": bbTenant}}}})
	assertNoEcho(t, "honest MCP session", honestMCP)
	if !strings.Contains(honestMCP, "HQ-Record") {
		t.Fatalf("MCP results lost the record itself:\n%.2000s", honestMCP)
	}
}

// Class credential-sent-to-another-host: neither a redirect nor an
// @odata.nextLink off the configured API host reaches it.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	redirect := e.run(t, "accepted-domains", bbTenant, "--json")
	link := e.run(t, "mobile-devices", "list", bbTenant, "--mailbox", "rosa@example.com", "--json")
	e.mcpCalls(t, []map[string]any{
		{"name": "exchange-online_read", "arguments": map[string]any{"endpoint_id": "accepted_domains.list", "params": map[string]any{"tenantId": bbTenant}}},
		{"name": "exchange-online_read", "arguments": map[string]any{"endpoint_id": "mobile_devices.list", "params": map[string]any{"tenantId": bbTenant, "Mailbox": "rosa@example.com"}}},
	})
	sv.mu.Lock()
	n := sv.offHost
	sv.mu.Unlock()
	if n != 0 {
		t.Fatalf("the other host was contacted %d time(s)", n)
	}
	if !strings.Contains(redirect, "leave the configured Exchange Online API host") {
		t.Fatalf("redirect: want the pin refusal, got:\n%.1500s", redirect)
	}
	if !strings.Contains(link, "another origin") {
		t.Fatalf("nextLink: want the origin refusal, got:\n%.1500s", link)
	}
}

// MCP writes preview until confirm is boolean true: the unconfirmed execute
// and the offboard tool send no state-changing cmdlet; the confirmed execute
// sends Set-Mailbox once.
func TestMCPWritesNeedConfirm(t *testing.T) {
	var mu sync.Mutex
	var cmdlets []string
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		cmdlets = append(cmdlets, cmdletOf(body))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"Identity":"rosa@example.com","RecipientTypeDetails":"UserMailbox","HiddenFromAddressListsEnabled":false}]}`))
	}))
	ca := filepath.Join(t.TempDir(), "ca.pem")
	_ = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600)
	e := newEnv(t, &server{url: srv.URL, caFile: ca}, true)
	T := bbTenant
	unconfirmed := []map[string]any{
		{"name": "exchange-online_execute", "arguments": map[string]any{"endpoint_id": "mailboxes.set", "params": map[string]any{"tenantId": T, "Identity": "rosa@example.com", "Type": "Shared"}}},
		{"name": "exchange-online_execute", "arguments": map[string]any{"endpoint_id": "mailboxes.set", "params": map[string]any{"tenantId": T, "Identity": "rosa@example.com", "Type": "Shared"}, "confirm": "true"}},
		{"name": "offboard", "arguments": map[string]any{"tenantId": T, "mailbox": "rosa@example.com", "manager": "boss@example.com"}},
		{"name": "offboard", "arguments": map[string]any{"tenantId": T, "mailbox": "rosa@example.com", "confirm": false}},
	}
	out := e.mcpCalls(t, unconfirmed)
	mu.Lock()
	sawPlanRead := false
	for _, c := range cmdlets {
		if !strings.HasPrefix(c, "Get-") {
			t.Errorf("an unconfirmed MCP call sent %s", c)
		}
		sawPlanRead = sawPlanRead || c == "Get-Mailbox"
	}
	cmdlets = nil
	mu.Unlock()
	if !strings.Contains(out, "Preview only") {
		t.Fatalf("want previews, got:\n%.2000s", out)
	}
	// offboard's preview is its own --plan: it reads the mailbox and reports
	// each step, so the operator sees what confirm would change.
	if !sawPlanRead || !strings.Contains(out, `\"mode\": \"plan\"`) && !strings.Contains(out, `\"mode\":\"plan\"`) {
		t.Fatalf("offboard preview is not the --plan report (read the mailbox: %v):\n%.2000s", sawPlanRead, out)
	}
	e.mcpCalls(t, []map[string]any{
		{"name": "exchange-online_execute", "arguments": map[string]any{"endpoint_id": "mailboxes.set", "params": map[string]any{"tenantId": T, "Identity": "rosa@example.com", "Type": "Shared"}, "confirm": true}},
	})
	mu.Lock()
	defer mu.Unlock()
	if len(cmdlets) != 1 || cmdlets[0] != "Set-Mailbox" {
		t.Fatalf("confirm: true sent %v, want exactly [Set-Mailbox]", cmdlets)
	}
}
