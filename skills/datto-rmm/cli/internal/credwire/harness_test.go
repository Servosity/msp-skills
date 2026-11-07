// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the harness the
// built-binary tests share. Each test runs the real datto-rmm-cli and
// datto-rmm-mcp against a fake Datto RMM over https (the kit sends a credential
// only over https), with a throwaway home, and points every process at a
// recording proxy: Go never proxies a loopback host, so the fake is reached
// directly, while a request for any other host reaches the proxy, which records
// it and refuses it. Nothing ever leaves for the internet.
//
// The fake models Datto RMM: the API under <host>/api, the token endpoint at
// <host>/auth/oauth/token on the same origin (POST, the password grant as a form,
// the fixed public client public-client:public in HTTP Basic), and every list as
// {"pageDetails":{...,"nextPageUrl"},"<plural>":[...]}.

package credwire

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
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

var built struct {
	once     sync.Once
	cli, mcp string
	err      error
}

// binaries builds both programs once per test binary.
func binaries(t *testing.T) (string, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs both binaries")
	}
	built.once.Do(func() {
		dir, err := os.MkdirTemp("", "datto-rmm-credwire-")
		if err != nil {
			built.err = err
			return
		}
		exe := ""
		if runtime.GOOS == "windows" {
			exe = ".exe"
		}
		built.cli = filepath.Join(dir, "datto-rmm-cli"+exe)
		built.mcp = filepath.Join(dir, "datto-rmm-mcp"+exe)
		for out, pkg := range map[string]string{built.cli: "../../cmd/datto-rmm-cli", built.mcp: "../../cmd/datto-rmm-mcp"} {
			if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
				built.err = &buildError{err, string(b)}
				return
			}
		}
	})
	if built.err != nil {
		t.Fatal(built.err)
	}
	return built.cli, built.mcp
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

// seen is one request a fake or the proxy received.
type seen struct {
	Method, Host, Path, Query string
	Header                    http.Header
	Body                      string
}

// recorder keeps every request a server received.
type recorder struct {
	mu  sync.Mutex
	got []seen
}

func (r *recorder) add(req *http.Request) seen {
	b, _ := io.ReadAll(req.Body)
	s := seen{Method: req.Method, Host: req.Host, Path: req.URL.Path, Query: req.URL.RawQuery, Header: req.Header.Clone(), Body: string(b)}
	r.mu.Lock()
	r.got = append(r.got, s)
	r.mu.Unlock()
	return s
}

func (r *recorder) all() []seen {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]seen(nil), r.got...)
}

// The held values: low-entropy letters and dashes (secret scanners pass them), no
// 8-character piece shared between them or with program text.
const (
	heldKey    = "dkeyzzzz-jjjj-xxxx-jjjj" // DATTO_RMM_API_KEY, the password grant's username
	heldSecret = "dsecvvvv-qqqq-wwww-qqqq" // DATTO_RMM_API_SECRET_KEY, its password
	heldMinted = "dmintkkk-pppp-mmmm-pppp" // the access_token the fake mints for that pair
	heldToken  = "dtokenbb-gggg-hhhh-gggg" // DATTO_RMM_TOKEN, a pre-minted bearer
)

// keyPair is the environment of a run that mints.
var keyPair = []string{"DATTO_RMM_API_KEY=" + heldKey, "DATTO_RMM_API_SECRET_KEY=" + heldSecret}

// tokenEnv is the environment of a run that sends a pre-minted token.
var tokenEnv = []string{"DATTO_RMM_TOKEN=" + heldToken}

// env is one throwaway home, a fake Datto RMM over https, a recording proxy, and
// the variables that point a binary at them.
type env struct {
	home  string
	vars  []string
	api   *recorder // API requests the fake received, the token endpoint excepted
	mints *recorder // requests the token endpoint received
	proxy *recorder // requests for any other host
	url   string    // the fake's API base URL
	ca    string    // the fake's trust bundle
}

// handler answers one API request and reports whether it did. Anything it leaves
// gets the default answer: an empty list, or a site record for a site path.
type handler func(w http.ResponseWriter, r *http.Request, s seen) bool

func dattoList(plural string, items ...string) string {
	return fmt.Sprintf(`{"pageDetails":{"count":%d,"totalCount":%d,"prevPageUrl":null,"nextPageUrl":null},%q:[%s]}`,
		len(items), len(items), plural, strings.Join(items, ","))
}

const siteRecord = `{"id":1042,"uid":"site-1042","name":"HQ-Record","accountUid":"acct-1042"}`

// answerToken is Datto's token endpoint: it answers only the configured pair sent
// the documented way, and a refusal repeats what it read from the form.
func answerToken(w http.ResponseWriter, r *http.Request, s seen) {
	form, _ := url.ParseQuery(s.Body)
	public := "Basic " + base64.StdEncoding.EncodeToString([]byte("public-client:public"))
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost || r.Header.Get("Authorization") != public || form.Get("grant_type") != "password" ||
		form.Get("username") != heldKey || form.Get("password") != heldSecret {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":"invalid_grant","error_description":"rejected %s %s"}`, form.Get("username"), form.Get("password"))
		return
	}
	fmt.Fprintf(w, `{"access_token":%q,"token_type":"bearer","expires_in":359999,"scope":"default"}`, heldMinted)
}

// newEnv starts the fake with h (nil gives every request the default answer) and
// the refusing proxy.
func newEnv(t *testing.T, h handler, extra ...string) *env {
	t.Helper()
	binaries(t)
	e := &env{home: t.TempDir(), api: &recorder{}, mints: &recorder{}, proxy: &recorder{}}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/oauth/token" {
			s := e.mints.add(r)
			if h == nil || !h(w, r, s) {
				answerToken(w, r, s)
			}
			return
		}
		s := e.api.add(r)
		if h != nil && h(w, r, s) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/v2/site/") {
			_, _ = io.WriteString(w, siteRecord)
			return
		}
		_, _ = io.WriteString(w, dattoList("sites", siteRecord))
	}))
	t.Cleanup(srv.Close)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.proxy.add(r)
		http.Error(w, "the test proxy refuses every request for a host other than the fake", http.StatusForbidden)
	}))
	t.Cleanup(proxy.Close)
	e.ca = filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(e.ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	e.url = srv.URL + "/api"
	tmp := filepath.Join(e.home, "tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	cliBin, _ := binaries(t)
	e.vars = []string{
		"HOME=" + e.home,
		"USERPROFILE=" + e.home,
		"DATTO_RMM_HOME=" + filepath.Join(e.home, "dr"),
		"DATTO_RMM_BASE_URL=" + e.url,
		"DATTO_RMM_CA_CERT=" + e.ca,
		"DATTO_RMM_CLI_PATH=" + cliBin,
		"HTTPS_PROXY=" + proxy.URL,
		"HTTP_PROXY=" + proxy.URL,
		"NO_PROXY=",
		"TMPDIR=" + tmp,
		"TMP=" + tmp,
		"TEMP=" + tmp,
	}
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(name) {
		case "PATH", "SYSTEMROOT", "COMSPEC", "PATHEXT", "WINDIR":
			e.vars = append(e.vars, kv)
		}
	}
	e.vars = append(e.vars, extra...)
	return e
}

// without drops one variable from the run's environment.
func (e *env) without(name string) {
	kept := e.vars[:0:0]
	for _, kv := range e.vars {
		if !strings.HasPrefix(kv, name+"=") {
			kept = append(kept, kv)
		}
	}
	e.vars = kept
}

// result is what one run printed and how it ended.
type result struct {
	out  string // stdout and stderr, interleaved
	code int
}

// run runs the CLI with stdin and args, under a timeout.
func (e *env) run(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	cliBin, _ := binaries(t)
	return e.exec(t, cliBin, stdin, args...)
}

func (e *env) exec(t *testing.T, bin, stdin string, args ...string) result {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = e.vars
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("%v timed out; printed:\n%s", args, out.String())
	}
	return result{out: out.String(), code: cmd.ProcessState.ExitCode()}
}

// files is every regular file the run left under its home.
func (e *env) files(t *testing.T) map[string]string {
	t.Helper()
	got := map[string]string{}
	_ = filepath.Walk(e.home, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			b, _ := os.ReadFile(p)
			got[p] = string(b)
		}
		return nil
	})
	return got
}

func writeFile(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// mcp runs one stdio session of the MCP server: initialize, then each call in
// turn, waiting for its reply before the next. It returns every reply by its
// request id (calls are numbered from 1) and everything the server printed.
func (e *env) mcp(t *testing.T, calls ...map[string]any) (map[int]string, string) {
	t.Helper()
	_, mcpBin := binaries(t)
	cmd := exec.Command(mcpBin)
	cmd.Env = e.vars
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var out, errb lockedBuffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(stdin)
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}})
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	deadline := time.Now().Add(120 * time.Second)
	for i, c := range calls {
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": i + 1, "method": "tools/call", "params": c})
		for time.Now().Before(deadline) && mcpReplies(out.String())[i+1] == "" {
			time.Sleep(50 * time.Millisecond)
		}
	}
	_ = stdin.Close()
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	return mcpReplies(out.String()), out.String() + "\n" + errb.String()
}

// mcpReplies maps each JSON-RPC reply a session wrote to its request id.
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
