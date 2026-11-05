// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the harness the
// credential leak tests drive the real binaries with: a throwaway home, a fake
// Servosity API over TLS, and every byte a run printed, answered over MCP or
// left on disk to search. Synthetic data only.

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
		dir, err := os.MkdirTemp("", "sv-kit-bin-")
		if err != nil {
			built.err = err
			return
		}
		built.dir = dir
		for _, name := range []string{"servosity-cli", "servosity-mcp"} {
			cmd := exec.Command("go", "build", "-o", filepath.Join(dir, name), "./cmd/"+name)
			cmd.Dir = filepath.Join("..", "..")
			if b, err := cmd.CombinedOutput(); err != nil {
				built.err = fmt.Errorf("go build %s: %v\n%s", name, err, b)
				return
			}
		}
		built.cli = filepath.Join(dir, "servosity-cli")
		built.mcp = filepath.Join(dir, "servosity-mcp")
	})
	if built.err != nil {
		t.Fatal(built.err)
	}
	return built.cli, built.mcp
}

// sandbox is one run's throwaway home, outside every real user directory.
type sandbox struct {
	root string // holds the home and the files the test writes itself
	home string // HOME and SERVOSITY_MSP_HOME
}

func newSandbox(t *testing.T) sandbox {
	t.Helper()
	// Not t.TempDir: its path carries the test's name, and a piece of a
	// path that matched a held value would withhold every unit that names it.
	root, err := os.MkdirTemp("", "sv-kit-")
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

// env is the environment a run gets: the parent's, minus every variable that
// could point a lookup at the operator's real files, plus extra.
func (s sandbox) env(extra ...string) []string {
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch {
		case name == "HOME", name == "USERPROFILE", strings.HasPrefix(name, "XDG_"),
			strings.HasPrefix(name, "SERVOSITY_MSP_"), strings.HasPrefix(name, "PRINTING_PRESS_"),
			name == "PP_MCP_TRANSPORT", name == "CREDKIT_HELD":
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "HOME="+s.home, "USERPROFILE="+s.home, "SERVOSITY_MSP_HOME="+s.home)
	return append(out, extra...)
}

// writePrivate writes a 0600 file under the sandbox root (outside the home).
func (s sandbox) writePrivate(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(s.root, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// filesHolding lists every file under the sandbox home whose bytes contain
// any of forms. The sandbox's own config files live outside it. A form with
// no mixed case is compared case folded. A mixed-case form is a case-sensitive
// encoding (base64): folded, it no longer decodes to the credential, and recall
// stores its query lower-cased, so it is compared as written.
func (s sandbox) filesHolding(t *testing.T, forms ...string) []string {
	t.Helper()
	var hits []string
	_ = filepath.WalkDir(s.home, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		low := bytes.ToLower(b)
		for _, f := range forms {
			held := bytes.Contains(low, []byte(strings.ToLower(f)))
			if f != strings.ToLower(f) && f != strings.ToUpper(f) {
				held = bytes.Contains(b, []byte(f))
			}
			if held {
				hits = append(hits, filepath.Base(p)+" holds "+f)
			}
		}
		return nil
	})
	return hits
}

type runResult struct {
	stdout, stderr string
	code           int
}

func (r runResult) all() string { return r.stdout + "\n" + r.stderr }

func run(t *testing.T, bin string, env []string, args ...string) runResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
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

// mcpCall starts the MCP server over stdio, initializes it, calls one tool
// and returns every frame the server wrote, plus its stderr.
func mcpCall(t *testing.T, bin string, env []string, tool string, args map[string]any) (frames, stderr string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
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
	call, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args}})
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
		t.Fatalf("the MCP server never answered the %s call; frames:\n%s\nstderr:\n%s", tool, all.String(), errb.String())
	}
	return all.String(), errb.String()
}

// fakeAPI is a Servosity API stand-in over TLS. issues is the reseller issue
// list it answers (a DRF page with next null); every other path answers an
// empty DRF page.
type fakeAPI struct {
	*httptest.Server
	caFile string
	mu     sync.Mutex
	issues []string
	auth   []string
	status int // when set, every request answers this status and body
	body   string
}

func newFakeAPI(t *testing.T, s sandbox, issues ...string) *fakeAPI {
	t.Helper()
	f := &fakeAPI{issues: issues}
	f.Server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		list := append([]string(nil), f.issues...)
		status, body := f.status, f.body
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
			fmt.Fprint(w, body)
			return
		}
		if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/resellers/7/issues") {
			fmt.Fprintf(w, `{"count":%d,"num_pages":1,"next":null,"previous":null,"results":[%s]}`, len(list), strings.Join(list, ","))
			return
		}
		fmt.Fprint(w, `{"count":0,"num_pages":1,"next":null,"previous":null,"results":[]}`)
	}))
	f.Server.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	f.StartTLS()
	t.Cleanup(f.Close)
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.Certificate().Raw})
	f.caFile = s.writePrivate(t, "api-ca.pem", string(ca))
	return f
}

func (f *fakeAPI) setIssues(issues ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issues = issues
}

func (f *fakeAPI) setAnswer(status int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body = status, body
}

func (f *fakeAPI) sawAuth(want string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.auth {
		if a == want {
			return true
		}
	}
	return false
}

// apiEnv is the environment for a run against the fake API with config as
// the explicit config file and token in the environment.
func (s sandbox) apiEnv(t *testing.T, f *fakeAPI, config, token string, extra ...string) []string {
	t.Helper()
	cfg := s.writePrivate(t, "sv.toml", config)
	return s.env(append([]string{
		"SERVOSITY_MSP_CONFIG=" + cfg,
		"SERVOSITY_MSP_TOKEN=" + token,
		"SERVOSITY_MSP_BASE_URL=" + f.URL,
		"SERVOSITY_MSP_CA_CERT=" + f.caFile,
		"SERVOSITY_MSP_RESELLER_ID=7",
	}, extra...)...)
}

// requireAbsent fails when text carries any of forms, case folded.
func requireAbsent(t *testing.T, where, text string, forms ...string) {
	t.Helper()
	low := strings.ToLower(text)
	for _, f := range forms {
		if strings.Contains(low, strings.ToLower(f)) {
			t.Errorf("%s carries %q:\n%s", where, f, text)
		}
	}
}

// issueJSON is one issue the way the vendor's reseller list answers it.
func issueJSON(id, company string) string {
	return fmt.Sprintf(`{"id":%s,"state":"ACTIVE","severity":"ERROR","company":%s,"created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-02T03:04:05Z","last_seen_at":"2026-01-02T03:04:05Z"}`, id, company)
}
