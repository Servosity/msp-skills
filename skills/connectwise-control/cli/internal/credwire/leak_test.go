// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated): the credential leak tests the shared credential
// kit closes for this connector, driven through the real binaries the way an
// operator or an MCP client runs them: build both binaries, point them at a
// throwaway home (and, where a request is sent, an httptest TLS server as the
// instance), then search every byte the run printed or answered over MCP.
//
// -credwire.cli and -credwire.mcp run the tests against other binaries (the
// pre-kit build, for the seen-failing logs under .review/seen/).

package credwire

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"flag"
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

var (
	cliOverride = flag.String("credwire.cli", "", "run against this CLI binary instead of building one")
	mcpOverride = flag.String("credwire.mcp", "", "run against this MCP binary instead of building one")
)

var built struct {
	once     sync.Once
	dir      string
	cli, mcp string
	err      error
}

func TestMain(m *testing.M) {
	flag.Parse()
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
	if *cliOverride != "" || *mcpOverride != "" {
		return *cliOverride, *mcpOverride
	}
	built.once.Do(func() {
		dir, err := os.MkdirTemp("", "cwc-kit-bin-")
		if err != nil {
			built.err = err
			return
		}
		built.dir = dir
		exe := ""
		if runtime.GOOS == "windows" {
			exe = ".exe"
		}
		built.cli = filepath.Join(dir, "connectwise-control-cli"+exe)
		built.mcp = filepath.Join(dir, "connectwise-control-mcp"+exe)
		for out, pkg := range map[string]string{built.cli: "./cmd/connectwise-control-cli", built.mcp: "./cmd/connectwise-control-mcp"} {
			cmd := exec.Command("go", "build", "-o", out, pkg)
			cmd.Dir = filepath.Join("..", "..")
			if b, err := cmd.CombinedOutput(); err != nil {
				built.err = fmt.Errorf("go build %s: %v\n%s", pkg, err, b)
				return
			}
		}
	})
	if built.err != nil {
		t.Fatal(built.err)
	}
	return built.cli, built.mcp
}

// sandbox is one run's throwaway home, outside every real user directory.
type sandbox struct {
	root string // holds the home and the files the test writes itself
	home string // HOME and CONNECTWISE_CONTROL_HOME
}

func newSandbox(t *testing.T) sandbox {
	t.Helper()
	// Not t.TempDir: its path carries the test's name, and an 8-character
	// piece of a held value in a path would withhold the path for that alone.
	root, err := os.MkdirTemp("", "cwc-kit-")
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
// could point a lookup at the operator's real files or credentials, plus extra.
func (s sandbox) env(extra ...string) []string {
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch {
		case name == "HOME", name == "USERPROFILE", strings.HasPrefix(name, "XDG_"),
			strings.HasPrefix(name, "CONNECTWISE_CONTROL_"), strings.HasPrefix(name, "PRINTING_PRESS_"),
			name == "PP_MCP_TRANSPORT", name == "CREDKIT_HELD",
			strings.EqualFold(name, "HTTPS_PROXY"), strings.EqualFold(name, "HTTP_PROXY"):
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "HOME="+s.home, "USERPROFILE="+s.home, "CONNECTWISE_CONTROL_HOME="+s.home)
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

type runResult struct {
	stdout, stderr string
	code           int
}

func run(t *testing.T, bin string, env []string, args ...string) runResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"cwc-kit-test","version":"1"}}}`,
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

func requireAbsent(t *testing.T, where, text, secret string) {
	t.Helper()
	if strings.Contains(text, secret) {
		t.Errorf("%s carries %q:\n%s", where, secret, text)
	}
}

// Codex r2 input 2: Cobra repeats an unknown command, and this one is the hex
// of the record user=ops, where ops is the configured user name: a short
// value (contract 2(b)) standing as a whole token once the hex is decoded
// (contract 3). It is withheld.
func TestR2HexRecordOfShortUserNameWithheld(t *testing.T) {
	cli, _ := binaries(t)
	const arg = "757365723d6f7073" // hex of "user=ops"
	s := newSandbox(t)
	res := run(t, cli, s.env("CONNECTWISE_CONTROL_USERNAME=ops"), arg)
	if res.code == 0 {
		t.Fatalf("an unknown command succeeded: stdout=%q stderr=%q", res.stdout, res.stderr)
	}
	requireAbsent(t, "stdout", res.stdout, arg)
	requireAbsent(t, "stderr", res.stderr, arg)
}

// Codex r2 input 4: the unknown command is the GBK bytes of the configured
// non-ASCII password, in hex (contract 3: hex, then a legacy charset). It is
// withheld.
func TestR2GBKHexOfNonASCIIPasswordWithheld(t *testing.T) {
	cli, _ := binaries(t)
	const arg = "c3dcc2eb" // hex of the GBK encoding of the password below
	s := newSandbox(t)
	res := run(t, cli, s.env("CONNECTWISE_CONTROL_PASSWORD=\u5bc6\u7801"), arg)
	if res.code == 0 {
		t.Fatalf("an unknown command succeeded: stdout=%q stderr=%q", res.stdout, res.stderr)
	}
	requireAbsent(t, "stdout", res.stdout, arg)
	requireAbsent(t, "stderr", res.stderr, arg)
}

// Startup ordering: a credential inside a malformed config file is held
// before either binary writes its first byte, so an error that echoes an
// argument (Cobra's unknown command, the MCP server's unknown transport)
// never shows it, whether the config comes from --config, the environment
// or the default path.
func TestStartupOutputNeverShowsMalformedConfigCredential(t *testing.T) {
	cli, mcp := binaries(t)
	const secret = "startupPass7Kq9xZ"
	const config = "control_username = \"startup.user\"\ncontrol_password = \"" + secret + "\"\nbad = [\n"

	s := newSandbox(t)
	cfg := s.writePrivate(t, "malformed.toml", config)
	res := run(t, cli, s.env(), "--config", cfg, secret)
	if res.code == 0 {
		t.Fatalf("an unknown command succeeded: stdout=%q stderr=%q", res.stdout, res.stderr)
	}
	requireAbsent(t, "CLI startup stdout", res.stdout, secret)
	requireAbsent(t, "CLI startup stderr", res.stderr, secret)

	res = run(t, cli, s.env("CONNECTWISE_CONTROL_CONFIG="+cfg), secret)
	requireAbsent(t, "CLI startup stdout (config from the environment)", res.stdout, secret)
	requireAbsent(t, "CLI startup stderr (config from the environment)", res.stderr, secret)

	res = run(t, mcp, s.env("CONNECTWISE_CONTROL_CONFIG="+cfg), "--transport", secret)
	if res.code == 0 {
		t.Fatalf("an unknown transport succeeded: stdout=%q stderr=%q", res.stdout, res.stderr)
	}
	requireAbsent(t, "MCP startup stdout", res.stdout, secret)
	requireAbsent(t, "MCP startup stderr", res.stderr, secret)

	// The default config path under CONNECTWISE_CONTROL_HOME.
	d := newSandbox(t)
	defCfg := filepath.Join(d.home, "config", "config.toml")
	if err := os.MkdirAll(filepath.Dir(defCfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defCfg, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	res = run(t, mcp, d.env(), "--transport", secret)
	requireAbsent(t, "MCP startup stdout (default config path)", res.stdout, secret)
	requireAbsent(t, "MCP startup stderr (default config path)", res.stderr, secret)
	res = run(t, cli, d.env(), secret)
	requireAbsent(t, "CLI startup stdout (default config path)", res.stdout, secret)
	requireAbsent(t, "CLI startup stderr (default config path)", res.stderr, secret)
}

// instance is a ConnectWise Control stand-in over TLS that answers
// GetAccessToken with the access grant and counts the requests it serves.
type instance struct {
	*httptest.Server
	caFile string
	mu     sync.Mutex
	paths  []string
}

func newInstance(t *testing.T, s sandbox, body string) *instance {
	t.Helper()
	in := &instance{}
	in.Server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		in.mu.Lock()
		in.paths = append(in.paths, r.URL.Path)
		in.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	in.Server.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	in.StartTLS()
	t.Cleanup(in.Close)
	in.caFile = s.writePrivate(t, "instance-ca.pem", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: in.Certificate().Raw})))
	return in
}

func (in *instance) requests() int {
	in.mu.Lock()
	defer in.mu.Unlock()
	return len(in.paths)
}

// The access grant is a credential (governance.md Access grant tier):
// GetAccessToken's accessToken, and the join url that repeats it, are
// withheld on the CLI and over MCP, while the grant is still issued.
func TestAccessGrantWithheld(t *testing.T) {
	cli, mcp := binaries(t)
	tok := "accessGrantTok-" + "7d2e9b4c1a5f"
	s := newSandbox(t)
	in := newInstance(t, s, `{"accessToken":"`+tok+`","url":"https://example.screenconnect.com/Host#Access/All%20Machines//x/Join?Token=`+tok+`"}`)
	env := s.env("CONNECTWISE_CONTROL_BASE_URL="+in.URL, "CONNECTWISE_CONTROL_CA_CERT="+in.caFile,
		"CONNECTWISE_CONTROL_USERNAME=grant.tech", "CONNECTWISE_CONTROL_PASSWORD=grantPass-"+"4c8e1a7b")
	const session = "6f1c2a9e-0d4b-4c7e-9a51-2b8f3d6e7c10"

	res := run(t, cli, env, "sessions", "get-access-token", "--session-id", session, "--yes", "--json")
	if in.requests() != 1 {
		t.Fatalf("the CLI sent %d request(s), want the one grant (exit %d): stdout=%q stderr=%q", in.requests(), res.code, res.stdout, res.stderr)
	}
	requireAbsent(t, "CLI stdout", res.stdout, tok)
	requireAbsent(t, "CLI stderr", res.stderr, tok)

	frames, stderr := mcpCall(t, mcp, env, "sessions_get-access-token", map[string]any{"sessionID": session, "confirm": true})
	if in.requests() != 2 {
		t.Fatalf("the MCP server sent %d grant request(s) in all, want 2:\n%s\nstderr:\n%s", in.requests(), frames, stderr)
	}
	requireAbsent(t, "MCP frames", frames, tok)
	requireAbsent(t, "MCP stderr", stderr, tok)
}
