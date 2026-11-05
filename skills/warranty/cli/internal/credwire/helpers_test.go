// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated): what the credential leak tests share. They drive
// the real binaries the way an operator or an MCP client runs them: build both,
// point them at a throwaway home and an httptest TLS server as the Dell base_url,
// then search every byte the run printed, answered over MCP or left on disk.

package credwire

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	dir      string
	cli, mcp string
	err      error
}

func TestMain(m *testing.M) {
	if os.Getenv(kitMatrixEnv) != "" {
		runKitMatrix()
		return
	}
	code := m.Run()
	if built.dir != "" {
		_ = os.RemoveAll(built.dir)
	}
	os.Exit(code)
}

// binaries builds both programs once per test run.
func binaries(t *testing.T) (cli, mcp string) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs both binaries")
	}
	if runtime.GOOS == "windows" {
		t.Skip("drives the binaries through POSIX pipes and an httptest TLS server; the kit's Windows stdio path is covered by the kit's own CI")
	}
	built.once.Do(func() {
		dir, err := os.MkdirTemp("", "wty-bin-")
		if err != nil {
			built.err = err
			return
		}
		built.dir = dir
		for _, name := range []string{"warranty-cli", "warranty-mcp"} {
			out := filepath.Join(dir, name)
			cmd := exec.Command("go", "build", "-o", out, "./cmd/"+name)
			cmd.Dir = filepath.Join("..", "..")
			if b, err := cmd.CombinedOutput(); err != nil {
				built.err = fmt.Errorf("go build %s: %v\n%s", name, err, b)
				return
			}
		}
		built.cli = filepath.Join(dir, "warranty-cli")
		built.mcp = filepath.Join(dir, "warranty-mcp")
	})
	if built.err != nil {
		t.Fatal(built.err)
	}
	return built.cli, built.mcp
}

// sandbox is one run's throwaway home, outside every real user directory.
type sandbox struct {
	root string // holds the home and the files the test writes itself
	home string // HOME and WARRANTY_HOME
}

func newSandbox(t *testing.T) sandbox {
	t.Helper()
	// Not t.TempDir: its path carries the test's name, and these names say
	// "Credential", an 8-character piece of the credentials the tests hold,
	// so every path in the sandbox would be withheld for that alone.
	root, err := os.MkdirTemp("", "wty-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	return sandbox{root: root, home: home}
}

// env is the environment a run gets: the parent's, minus every variable
// that could point a lookup at the operator's real files, plus extra.
func (s sandbox) env(extra ...string) []string {
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch {
		case name == "HOME", name == "USERPROFILE", strings.HasPrefix(name, "XDG_"),
			strings.HasPrefix(name, "WARRANTY_"), strings.HasPrefix(name, "PRINTING_PRESS_"),
			name == "PP_MCP_TRANSPORT", name == "CREDKIT_HELD":
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "HOME="+s.home, "USERPROFILE="+s.home, "WARRANTY_HOME="+s.home)
	return append(out, extra...)
}

// writePrivate writes a 0600 file under the sandbox root (outside the home).
func (s sandbox) writePrivate(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(s.root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// filesHolding lists every file under dir whose bytes contain secret,
// skip excepted.
func filesHolding(t *testing.T, dir, secret string, skip ...string) []string {
	t.Helper()
	var hits []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		for _, s := range skip {
			if p == s {
				return nil
			}
		}
		if b, err := os.ReadFile(p); err == nil && bytes.Contains(b, []byte(secret)) {
			hits = append(hits, p)
		}
		return nil
	})
	return hits
}

type runResult struct {
	stdout, stderr string
	code           int
}

func run(t *testing.T, bin string, env []string, args ...string) runResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.WaitDelay = 5 * time.Second // Wait closes pipes a grandchild still holds
	cmd.Env = env
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("%s %v: %v", filepath.Base(bin), args, err)
	}
	return runResult{stdout: out.String(), stderr: errb.String(), code: code}
}

// reported is whether a run's output mentions want, or the credential kit's
// notice stands in for a unit it withheld (that unit carried the credential,
// so the run did reach it).
func reported(res runResult, want string) bool {
	out := res.stdout + res.stderr
	return strings.Contains(out, want) || strings.Contains(out, "[credkit: withheld from ")
}

// mcpCall starts the MCP server over stdio, initializes it, calls one tool
// and returns every frame the server wrote, plus its stderr.
func mcpCall(t *testing.T, bin string, env []string, tool string, args map[string]any) (frames, stderr string) {
	t.Helper()
	return mcpRequest(t, bin, env, "tools/call", map[string]any{"name": tool, "arguments": args})
}

// mcpRequest starts the server, initializes it and sends one request.
func mcpRequest(t *testing.T, bin string, env []string, method string, params map[string]any) (frames, stderr string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.WaitDelay = 5 * time.Second // Wait closes pipes a grandchild still holds
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer context.AfterFunc(ctx, func() { _ = stdout.Close() })()
	call, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 2, "method": method, "params": params})
	for _, m := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"credkit-leak-test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		string(call),
	} {
		if _, err := io.WriteString(stdin, m+"\n"); err != nil {
			t.Fatalf("write to the MCP server: %v", err)
		}
	}
	var all strings.Builder
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for sc.Scan() {
		line := sc.Text()
		all.WriteString(line + "\n")
		var msg struct {
			ID json.RawMessage `json:"id"`
		}
		if json.Unmarshal([]byte(line), &msg) == nil && string(msg.ID) == "2" {
			break
		}
	}
	_ = stdin.Close()
	_, _ = io.Copy(io.Discard, stdout)
	_ = cmd.Wait()
	if !strings.Contains(all.String(), `"id":2`) {
		t.Fatalf("the MCP server never answered %s; frames:\n%s\nstderr:\n%s", method, all.String(), errb.String())
	}
	return all.String(), errb.String()
}

// vendorServer is an vendor stand-in over TLS. It answers every request with
// status and body, and records each request's headers.
type vendorServer struct {
	*httptest.Server
	caFile string
	mu     sync.Mutex
	seen   []http.Header
	paths  []string
}

func newVendorServer(t *testing.T, s sandbox, status int, body string) *vendorServer {
	t.Helper()
	r := &vendorServer{}
	r.Server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.seen = append(r.seen, req.Header.Clone())
		r.paths = append(r.paths, req.URL.Path)
		r.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	r.Server.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	r.StartTLS()
	t.Cleanup(r.Close)
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: r.Certificate().Raw})
	r.caFile = s.writePrivate(t, "stub-ca.pem", string(ca))
	return r
}

// sent reports whether some request carried header name with value want.
func (r *vendorServer) sent(name, want string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, h := range r.seen {
		if h.Get(name) == want {
			return true
		}
	}
	return false
}

func (r *vendorServer) requests() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.paths)
}

func requireAbsent(t *testing.T, where, text, secret string) {
	t.Helper()
	if strings.Contains(text, secret) {
		t.Errorf("%s carries the credential %q:\n%s", where, secret, text)
	}
}
