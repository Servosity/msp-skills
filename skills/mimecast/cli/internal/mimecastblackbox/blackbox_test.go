// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (mimecast; handfixes.json credkit-wiring): the black-box proof
// of the credential rules with the shared credential kit wired in. Both built
// binaries run against a fake Mimecast served over https with a certificate
// made for this run (trusted through MIMECAST_CA_CERT and SSL_CERT_FILE). The
// fake echoes the held credentials in several encodings, returns journaling
// passwords in their declared field, and redirects to another host. Every
// byte either binary prints, and every file it writes under its home, is
// scanned for every credential; the other host must never receive one, and
// an MCP write must send nothing until confirm is true.

package mimecastblackbox

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
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
	clientID     = "cid-" + "7Hq2Lm9Xw3Pz4Kd"
	clientSecret = "csec+" + "Rb8/Kd2=Vn6Qs1Tz4"
	accessToken  = "tok-" + "Jp5Wc3Ze8Ha2Lm6Nq"
	smtpPassword = "SmtpPw-" + "9Gx4Kr7Bq2Ly"
	pop3Password = "MbxPw-" + "3Fy8Lt5Nd1Rw"
	// Passwords an operator or agent sends that no answer or startup source
	// holds first: a strong one, a single-class one, and a short one.
	newPassword   = "SmtpNew-123456"
	plainPassword = "lowercasepassword"
	shortPassword = "Pw4-ab"
	// The factory round-2 probe: a password only one MCP request carries.
	freshPassword = "FreshSmtp-7Qx2Vb9Lm4"
)

var allSecrets = []string{clientID, clientSecret, accessToken, smtpPassword, pop3Password, newPassword, plainPassword, shortPassword, freshPassword}

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

// forms are the encodings the scan looks for.
func forms(s string) []string {
	return []string{
		s, base64.StdEncoding.EncodeToString([]byte(s)), base64.RawStdEncoding.EncodeToString([]byte(s)),
		base64.URLEncoding.EncodeToString([]byte(s)), base64.RawURLEncoding.EncodeToString([]byte(s)),
		hex.EncodeToString([]byte(s)), strings.ToUpper(hex.EncodeToString([]byte(s))),
		url.QueryEscape(s), url.PathEscape(s), jsonEscaped(s),
	}
}

// leaks lists every credential text repeats, in any of the forms above.
func leaks(text string) []string {
	var found []string
	for _, s := range allSecrets {
		for _, f := range forms(s) {
			if strings.Contains(text, f) {
				found = append(found, fmt.Sprintf("%q as %q", s, f))
			}
		}
	}
	return found
}

type fake struct {
	srv, evil     *httptest.Server
	evilHits      atomic.Int32
	releases      atomic.Int32
	journalWrites atomic.Int32
	mu            sync.Mutex
	evilSaw       []string // credentials the other host received
}

// certFile is the PEM of the certificate every fake serves: httptest's TLS
// servers share one, made for this run, valid for 127.0.0.1.
var certFile string

func newFake(t *testing.T) *fake {
	f := &fake{}
	f.evil = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.evilHits.Add(1)
		body, _ := io.ReadAll(r.Body)
		var head strings.Builder
		_ = r.Header.Write(&head)
		if found := leaks(r.URL.String() + "\n" + head.String() + "\n" + string(body)); len(found) > 0 {
			f.mu.Lock()
			f.evilSaw = append(f.evilSaw, found...)
			f.mu.Unlock()
		}
		_, _ = io.WriteString(w, `{"stolen":true}`)
	}))
	t.Cleanup(f.evil.Close)
	ok := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
	envelope := func(key, items string) string {
		return `{"meta":{"status":200},"data":[{"` + key + `":[` + items + `]}],"fail":[]}`
	}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		if r.URL.Path == "/oauth/token" {
			ok(w, `{"access_token":"`+accessToken+`","token_type":"Bearer","expires_in":1800}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/journaling/") && r.Method != http.MethodGet {
			f.journalWrites.Add(1)
		}
		switch r.URL.Path {
		case "/journaling/cloud-gateway/v1/services":
			ok(w, `{"services":[{"id":"jx1","description":"Exchange journal","pop3JournalingConnection":{"emailAddress":"journal@example.com","mailbox":"journal","password":"`+pop3Password+`","host":"pop.example.com","port":995},"smtpJournalingConnection":{"emailAddress":"smtp-journal@example.com","usesAuthentication":true,"password":"`+smtpPassword+`"}}]}`)
		case "/journaling/cloud-gateway/v1/services/jx-ok":
			ok(w, `{"id":"jx-ok","description":"Plain journal"}`)
		case "/journaling/cloud-gateway/v1/services/jx-redirect":
			http.Redirect(w, r, f.evil.URL+"/steal", http.StatusFound)
		case "/journaling/cloud-gateway/v1/services/jx-redirect-public":
			http.Redirect(w, r, "https://evil.invalid/steal", http.StatusFound)
		case "/journaling/cloud-gateway/v1/services/jx-echo":
			// The vendor answer repeats the client secret, base64-encoded.
			ok(w, `{"id":"jx-echo","description":"`+base64.StdEncoding.EncodeToString([]byte(clientSecret))+`"}`)
		case "/api/ttp/url/get-logs":
			ok(w, envelope("clickLogs", `{"date":"2026-10-14T10:00:00+0000","userEmailAddress":"alice@example.com","fromUserEmailAddress":"x@phish.example","url":"https://phish.example/?s=`+url.QueryEscape(clientSecret)+`","action":"allow","creationMethod":"User Click"}`))
		case "/api/ttp/attachment/get-logs":
			ok(w, envelope("attachmentLogs", `{"date":"2026-10-13T12:00:00+0000","recipientAddress":"alice@example.com","senderAddress":"x@phish.example","fileName":"`+hex.EncodeToString([]byte(accessToken))+`.html","result":"malicious"}`))
		case "/api/ttp/impersonation/get-logs":
			ok(w, envelope("impersonationLogs", `{"eventTime":"2026-10-14T07:00:00+0000","recipientAddress":"alice@example.com","senderAddress":"x@phish.example","definition":"`+jsonEscaped(clientID)+`"}`))
		case "/api/gateway/get-rejections":
			ok(w, envelope("rejections", `{"created":"2026-10-14T06:00:00+0000","fromAddress":"x@phish.example","toAddress":"alice@example.com","description":"token `+accessToken+`"}`))
		case "/api/gateway/find-held-messages":
			ok(w, envelope("heldEmails", `{"id":"h-ok","subject":"Quarterly report","heldReason":"Awaiting review","fromEnv":{"emailAddress":"cfo@example.com"},"to":[{"emailAddress":"alice@example.com"}],"heldSince":"2026-10-14T11:00:00+0000"}`))
		case "/api/gateway/hold-release":
			f.releases.Add(1)
			ok(w, `{"meta":{"status":200},"data":[{"id":"h-ok","release":true}],"fail":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			ok(w, `{"fail":[{"code":"endpoint_evaluation_failed_1","message":"Endpoint cannot be found"}]}`)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// assertEvilSawNothing fails when the other host received a credential.
func (f *fake) assertEvilSawNothing(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.evilSaw) > 0 {
		t.Fatalf("another host received a credential: %v", f.evilSaw)
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
	dir, err := os.MkdirTemp("", "mimecast-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binDir = dir
	code := func() int {
		defer os.RemoveAll(dir)
		for _, name := range []string{"mimecast-cli", "mimecast-mcp"} {
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

// baseEnv runs a binary as a fresh operator: its own home, the fakes'
// certificate trusted, every CLI, XDG and trust override cleared.
func baseEnv(home string) []string {
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "MIMECAST_") || strings.HasPrefix(name, "XDG_") || name == "HOME" || name == "USERPROFILE" ||
			name == "APPDATA" || name == "LOCALAPPDATA" || name == "SSL_CERT_FILE" || name == "SSL_CERT_DIR" {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "HOME="+home, "USERPROFILE="+home, "APPDATA="+filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA="+filepath.Join(home, "AppData", "Local"), "MIMECAST_HOME="+filepath.Join(home, "mh"),
		"MIMECAST_CA_CERT="+certFile, "SSL_CERT_FILE="+certFile)
}

// env is baseEnv with the credentials and the fake's base and token URL in
// the environment.
func env(home string, f *fake) []string {
	return append(baseEnv(home),
		"MIMECAST_CLIENT_ID="+clientID, "MIMECAST_CLIENT_SECRET="+clientSecret,
		"MIMECAST_BASE_URL="+f.srv.URL, "MIMECAST_TOKEN_URL="+f.srv.URL+"/oauth/token")
}

func runCLI(t *testing.T, home string, f *fake, args ...string) (string, int) {
	t.Helper()
	return runWith(t, env(home, f), "", args...)
}

func runCLIStdin(t *testing.T, home string, f *fake, stdin string, args ...string) (string, int) {
	t.Helper()
	return runWith(t, env(home, f), stdin, args...)
}

func runWith(t *testing.T, environ []string, stdin string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(exe("mimecast-cli"), args...)
	cmd.Env = environ
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running %v: %v", args, err)
	}
	return out.String(), code
}

func scanFiles(t *testing.T, home string) {
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
		if found := leaks(string(data)); len(found) > 0 {
			t.Errorf("file %s holds a credential: %v", path, found)
		}
		return nil
	})
	if n == 0 {
		t.Fatal("the runs wrote no file at all; the file scan proves nothing")
	}
}

func TestCLIRunLeaksNoCredential(t *testing.T) {
	f := newFake(t)
	home := t.TempDir()
	if keep := os.Getenv("MIMECAST_BLACKBOX_KEEP"); keep != "" {
		home = keep
	}

	// A credential-free answer reaches the operator whole.
	if out, code := runCLI(t, home, f, "journaling", "get", "jx-ok", "--json"); code != 0 || !strings.Contains(out, "Plain journal") {
		t.Fatalf("journaling get jx-ok must answer (exit %d):\n%s", code, out)
	}
	var all string
	for _, args := range [][]string{
		{"journaling", "list", "--json"},
		{"sync", "--resources", "journaling", "--json"},
		{"journaling", "get", "jx-echo", "--json"},
		{"journaling", "get", "jx-redirect", "--json"},
		{"journaling", "get", "jx-redirect-public", "--json"},
		{"threats", "mailbox", "alice@example.com", "--json"},
		{"held", "release-safe", "h-ok", "--ticket", "T-1", "--plan", "--json"},
		{"--dry-run", "journaling", "update", "jx1", "--json"},
		{"--dry-run", "journaling", "update", "jx1", "--smtp-journaling-connection-password", shortPassword, "--json"},
		{"doctor", "--json"},
		{"auth", "status", "--json"},
		{"journaling", "list", "--no-such-flag", clientSecret},
		{"feedback", "the secret is " + clientSecret},
	} {
		out, _ := runCLI(t, home, f, args...)
		all += "\n" + out
		if found := leaks(out); len(found) > 0 {
			t.Errorf("%v printed a credential: %v\n%s", args, found, out)
		}
	}
	if !strings.Contains(all, kitNotice) {
		t.Errorf("no output was withheld, so the echoing answers never reached the kit:\n%s", all)
	}
	if n := f.evilHits.Load(); n != 0 {
		t.Errorf("the redirect reached another host %d time(s)", n)
	}
	f.assertEvilSawNothing(t)
	if f.releases.Load() != 0 {
		t.Fatal("--plan released a held message")
	}
	if f.journalWrites.Load() != 0 {
		t.Fatal("a --dry-run sent a journaling write")
	}
	scanFiles(t, home)
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
	cmd := exec.Command(exe("mimecast-mcp"))
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

func (s *mcpSession) send(method string, params any) map[string]any {
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
				return m
			}
		case <-timeout:
			s.t.Fatalf("no answer to %s within %v; the server wrote:\n%.2000s\nstderr:\n%s", method, mcpAnswerWait, s.all.String(), s.stderr)
		}
	}
}

func (s *mcpSession) call(name string, args map[string]any) string {
	s.t.Helper()
	m := s.send("tools/call", map[string]any{"name": name, "arguments": args})
	b, _ := json.Marshal(m)
	return string(b)
}

// transcript is every frame the server wrote, and its stderr.
func (s *mcpSession) transcript() string { return s.all.String() + "\n" + s.stderr.String() }

func TestMCPRunLeaksNoCredentialAndWritesNeedConfirm(t *testing.T) {
	f := newFake(t)
	home := t.TempDir()
	s := startMCP(t, env(home, f))

	if read := s.call("mimecast_read", map[string]any{"endpoint_id": "journaling.get", "params": map[string]any{"journaling_id": "jx-ok"}}); !strings.Contains(read, "Plain journal") {
		t.Fatalf("mimecast_read journaling.get did not answer: %s", read)
	}
	s.call("mimecast_read", map[string]any{"endpoint_id": "journaling.list"})
	s.call("mimecast_read", map[string]any{"endpoint_id": "journaling.get", "params": map[string]any{"journaling_id": "jx-echo"}})
	s.call("mimecast_read", map[string]any{"endpoint_id": "journaling.get", "params": map[string]any{"journaling_id": "jx-redirect"}})
	s.call("threats_mailbox", map[string]any{"address": "alice@example.com"})
	s.call("sync", map[string]any{"resources": "journaling"})
	s.call("sql", map[string]any{"query": "SELECT data FROM resources"})

	// A write previews until confirm is boolean true, whatever else is passed.
	for _, args := range []map[string]any{
		{"id": "h-ok", "ticket": "T-1"},
		{"id": "h-ok", "ticket": "T-1", "plan": false, "dry-run": false},
		// The mirror passes a string as --flag=value, after the preview flag.
		{"id": "h-ok", "ticket": "T-1", "plan": "false", "dry-run": "false"},
		{"id": "h-ok", "ticket": "T-1", "confirm": "true"},
	} {
		res := s.call("held_release_safe", args)
		if n := f.releases.Load(); n != 0 {
			t.Fatalf("held_release_safe %v released without confirm: true (%d release call(s)): %s", args, n, res)
		}
	}
	res := s.call("held_release_safe", map[string]any{"id": "h-ok", "ticket": "T-1", "confirm": true})
	if f.releases.Load() != 1 {
		t.Fatalf("confirm: true must send the release once (the gate is not just closed): %s", res)
	}
	if s.call("held_purge_sender", map[string]any{"sender": "cfo@example.com", "plan": false}); f.releases.Load() != 1 {
		t.Fatal("held_purge_sender acted without confirm: true")
	}

	_ = s.in.Close()
	text := s.transcript()
	if found := leaks(text); len(found) > 0 {
		t.Errorf("the MCP run printed a credential: %v\n%s", found, text)
	}
	if !strings.Contains(text, kitNotice) {
		t.Errorf("no MCP result was withheld, so the echoing answers never reached the kit:\n%.3000s", text)
	}
	if n := f.evilHits.Load(); n != 0 {
		t.Errorf("the redirect reached another host %d time(s)", n)
	}
	f.assertEvilSawNothing(t)
	scanFiles(t, home)
}
