// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated): the credential leak tests the shared credential
// kit closes for this connector. Each one is a Codex finding's own input
// (publish pass 2, round 1), driven through the real binaries the way an
// operator or an MCP client runs them: build both binaries, point them at a
// throwaway home and an httptest TLS server as the RDAP base_url, then search
// every byte the run printed, answered over MCP or left on disk.

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
		dir, err := os.MkdirTemp("", "dct-credkit-bin-")
		if err != nil {
			built.err = err
			return
		}
		built.dir = dir
		for _, name := range []string{"domain-cert-tracker-cli", "domain-cert-tracker-mcp"} {
			out := filepath.Join(dir, name)
			cmd := exec.Command("go", "build", "-o", out, "./cmd/"+name)
			cmd.Dir = filepath.Join("..", "..")
			if b, err := cmd.CombinedOutput(); err != nil {
				built.err = fmt.Errorf("go build %s: %v\n%s", name, err, b)
				return
			}
		}
		built.cli = filepath.Join(dir, "domain-cert-tracker-cli")
		built.mcp = filepath.Join(dir, "domain-cert-tracker-mcp")
	})
	if built.err != nil {
		t.Fatal(built.err)
	}
	return built.cli, built.mcp
}

// sandbox is one run's throwaway home, outside every real user directory.
type sandbox struct {
	root string // holds the home and the files the test writes itself
	home string // HOME and DOMAIN_CERT_TRACKER_HOME
}

func newSandbox(t *testing.T) sandbox {
	t.Helper()
	// Not t.TempDir: its path carries the test's name, and these names say
	// "Credential", an 8-character piece of the credentials the tests hold,
	// so every path in the sandbox would be withheld for that alone.
	root, err := os.MkdirTemp("", "dct-kit-")
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
			strings.HasPrefix(name, "DOMAIN_CERT_TRACKER_"), strings.HasPrefix(name, "PRINTING_PRESS_"),
			name == "PP_MCP_TRANSPORT", name == "CREDKIT_HELD":
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "HOME="+s.home, "USERPROFILE="+s.home, "DOMAIN_CERT_TRACKER_HOME="+s.home)
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

// rdapServer is an RDAP stand-in over TLS. It answers every request with
// status and body, and records each request's headers.
type rdapServer struct {
	*httptest.Server
	caFile string
	mu     sync.Mutex
	seen   []http.Header
	paths  []string
}

func newRDAPServer(t *testing.T, s sandbox, status int, body string) *rdapServer {
	t.Helper()
	r := &rdapServer{}
	r.Server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.seen = append(r.seen, req.Header.Clone())
		r.paths = append(r.paths, req.URL.Path)
		r.mu.Unlock()
		w.Header().Set("Content-Type", "application/rdap+json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	r.Server.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	r.StartTLS()
	t.Cleanup(r.Close)
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: r.Certificate().Raw})
	r.caFile = s.writePrivate(t, "rdap-ca.pem", string(ca))
	return r
}

// sent reports whether some request carried header name with value want.
func (r *rdapServer) sent(name, want string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, h := range r.seen {
		if h.Get(name) == want {
			return true
		}
	}
	return false
}

func (r *rdapServer) requests() int {
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

// P1 1: a successful answer that echoes the configured auth_header is
// printed by the CLI, written to the response cache and returned by the MCP
// domain_get tool.
func TestKitSuccessBodyCredentialNeverShownOrCached(t *testing.T) {
	cli, mcp := binaries(t)
	const secret = "longcredential123"
	body := `{"objectClassName":"domain","ldhName":"example.com","notices":[{"description":["Bearer longcredential123"]}]}`

	s := newSandbox(t)
	srv := newRDAPServer(t, s, http.StatusOK, body)
	cfg := s.writePrivate(t, "rdap.toml", fmt.Sprintf("base_url = %q\nauth_header = \"Bearer %s\"\n", srv.URL, secret))
	res := run(t, cli, s.env(), "--config", cfg, "--ca-cert", srv.caFile, "domain", "example.com", "--json", "--data-source", "live")
	if srv.requests() == 0 {
		t.Fatalf("the CLI never reached the RDAP server (exit %d): stdout=%q stderr=%q", res.code, res.stdout, res.stderr)
	}
	if !srv.sent("Authorization", "Bearer "+secret) {
		t.Errorf("the configured auth_header was not sent to its own base_url")
	}
	requireAbsent(t, "CLI stdout", res.stdout, secret)
	requireAbsent(t, "CLI stderr", res.stderr, secret)
	if hits := filesHolding(t, s.home, secret); len(hits) > 0 {
		t.Errorf("the CLI wrote the credential to %v", hits)
	}

	m := newSandbox(t)
	msrv := newRDAPServer(t, m, http.StatusOK, body)
	mcfg := m.writePrivate(t, "rdap.toml", fmt.Sprintf("base_url = %q\nauth_header = \"Bearer %s\"\n", msrv.URL, secret))
	frames, stderr := mcpCall(t, mcp, m.env("DOMAIN_CERT_TRACKER_CONFIG="+mcfg, "DOMAIN_CERT_TRACKER_BASE_URL="+msrv.URL,
		"DOMAIN_CERT_TRACKER_CA_CERT="+msrv.caFile), "domain_get", map[string]any{"name": "example.com"})
	if msrv.requests() == 0 {
		t.Fatalf("the MCP server never reached the RDAP server: frames=%s stderr=%s", frames, stderr)
	}
	requireAbsent(t, "MCP domain_get frames", frames, secret)
	requireAbsent(t, "MCP stderr", stderr, secret)
	if hits := filesHolding(t, m.home, secret); len(hits) > 0 {
		t.Errorf("the MCP server wrote the credential to %v", hits)
	}
}

// P1 2: a [headers] value written with a TOML escape is sent decoded; an
// HTTP 400 that echoes the decoded value reaches CLI errors and MCP results.
func TestKitParsedHeaderCredentialNeverShown(t *testing.T) {
	cli, mcp := binaries(t)
	const decoded = "abcdefgh"
	const config = "headers = { X-Rdap-Key = \"abcd\\U00000065fgh\" }\n"
	body := `{"message":"abcdefgh"}`

	s := newSandbox(t)
	srv := newRDAPServer(t, s, http.StatusBadRequest, body)
	cfg := s.writePrivate(t, "rdap.toml", fmt.Sprintf("base_url = %q\n", srv.URL)+config)
	res := run(t, cli, s.env(), "--config", cfg, "--ca-cert", srv.caFile, "domain", "example.com", "--json", "--data-source", "live")
	if !srv.sent("X-Rdap-Key", decoded) {
		t.Fatalf("the decoded [headers] value never reached its base_url (exit %d): stdout=%q stderr=%q", res.code, res.stdout, res.stderr)
	}
	requireAbsent(t, "CLI stdout", res.stdout, decoded)
	requireAbsent(t, "CLI stderr", res.stderr, decoded)

	m := newSandbox(t)
	msrv := newRDAPServer(t, m, http.StatusBadRequest, body)
	mcfg := m.writePrivate(t, "rdap.toml", fmt.Sprintf("base_url = %q\n", msrv.URL)+config)
	frames, stderr := mcpCall(t, mcp, m.env("DOMAIN_CERT_TRACKER_CONFIG="+mcfg, "DOMAIN_CERT_TRACKER_BASE_URL="+msrv.URL,
		"DOMAIN_CERT_TRACKER_CA_CERT="+msrv.caFile), "domain_get", map[string]any{"name": "example.com"})
	if !msrv.sent("X-Rdap-Key", decoded) {
		t.Fatalf("the MCP server never sent the decoded [headers] value: frames=%s stderr=%s", frames, stderr)
	}
	requireAbsent(t, "MCP domain_get frames", frames, decoded)
	requireAbsent(t, "MCP stderr", stderr, decoded)
}

// P1 3: a config whose auth_header overflows the TOML integer parser puts
// the number in the parser's diagnostic: doctor --json reports it, and the
// legacy-config warning prints it on stderr.
func TestKitConfigParseErrorNeverShowsCredential(t *testing.T) {
	cli, _ := binaries(t)
	const secret = "123456789012345678901234567890"
	const config = "auth_header = " + secret + "\n"

	s := newSandbox(t)
	cfg := s.writePrivate(t, "private-rdap.toml", config)
	res := run(t, cli, s.env(), "doctor", "--config", cfg, "--json")
	if !reported(res, "config") {
		t.Fatalf("doctor never reported on the config (exit %d): stdout=%q stderr=%q", res.code, res.stdout, res.stderr)
	}
	requireAbsent(t, "doctor --json stdout", res.stdout, secret)
	requireAbsent(t, "doctor --json stderr", res.stderr, secret)

	// The legacy path: ~/.config/domain-cert-tracker-cli/config.toml, read
	// when the config directory has no config.toml.
	l := newSandbox(t)
	legacy := filepath.Join(l.home, ".config", "domain-cert-tracker-cli", "config.toml")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	res = run(t, cli, l.env("DOMAIN_CERT_TRACKER_HOME="+filepath.Join(l.home, "dct")), "doctor", "--json")
	if !reported(res, "config") {
		t.Fatalf("doctor never read the legacy config (exit %d): stdout=%q stderr=%q", res.code, res.stdout, res.stderr)
	}
	requireAbsent(t, "legacy-config stdout", res.stdout, secret)
	requireAbsent(t, "legacy-config stderr", res.stderr, secret)
}

// P1 4: recall journals its operand although --config was never loaded, and
// teach writes --notes to the learnings log (and the store).
func TestKitLearningLogsNeverPersistCredential(t *testing.T) {
	cli, _ := binaries(t)
	const secret = "lowercasecredential123"
	const config = "auth_header = \"" + secret + "\"\n"

	s := newSandbox(t)
	cfg := s.writePrivate(t, "private-rdap.toml", config)
	res := run(t, cli, s.env(), "--config", cfg, "recall", secret, "--json")
	// 74: the answer repeats the query, so the kit withheld it and reports the
	// stdout it lost (credkit.ExitOutputLost).
	if res.code != 0 && res.code != 74 {
		t.Fatalf("recall failed (exit %d): stdout=%q stderr=%q", res.code, res.stdout, res.stderr)
	}
	if hits := filesHolding(t, s.home, secret); len(hits) > 0 {
		t.Errorf("recall persisted the credential to %v", hits)
	}
	if _, err := os.Stat(filepath.Join(s.home, "state", "learn")); err != nil {
		t.Errorf("recall wrote no learn journal at all, so this test proved nothing: %v", err)
	}

	// teach, with the credential in the default config file.
	d := newSandbox(t)
	defCfg := filepath.Join(d.home, "config", "config.toml")
	if err := os.MkdirAll(filepath.Dir(defCfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defCfg, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	res = run(t, cli, d.env(), "teach", "--query", "find items", "--resource-type", "domain", "--resource", "example.com", "--notes", secret)
	if hits := synccpuNotStore(filesHolding(t, d.home, secret, defCfg)); len(hits) > 0 { // synccpu: the local store is not checked
		t.Errorf("teach persisted the credential to %v (exit %d, stderr=%q)", hits, res.code, res.stderr)
	}
	// The same teach with an ordinary note persists it, so the absence
	// above is the credential being withheld, not a teach that writes nothing.
	res = run(t, cli, d.env(), "teach", "--query", "find items", "--resource-type", "domain", "--resource", "example.com", "--notes", "an ordinary note")
	if hits := filesHolding(t, d.home, "an ordinary note", defCfg); len(hits) == 0 {
		t.Errorf("teach persists nothing even without a credential, so this test proved nothing (exit %d, stderr=%q)", res.code, res.stderr)
	}
}

// Startup ordering: a credential inside a malformed config file is held
// before either binary writes its first byte, so a parse error that echoes
// an argument (Cobra's unknown command, the MCP server's unknown transport)
// never shows it.
func TestKitStartupOutputNeverShowsMalformedConfigCredential(t *testing.T) {
	cli, mcp := binaries(t)
	const secret = "startupcred7Kq9xZ"
	const config = "auth_header = \"" + secret + "\"\nbad = [\n"

	s := newSandbox(t)
	cfg := s.writePrivate(t, "malformed.toml", config)
	res := run(t, cli, s.env(), "--config", cfg, secret)
	if res.code == 0 {
		t.Fatalf("an unknown command succeeded: stdout=%q stderr=%q", res.stdout, res.stderr)
	}
	requireAbsent(t, "CLI startup stdout", res.stdout, secret)
	requireAbsent(t, "CLI startup stderr", res.stderr, secret)

	res = run(t, cli, s.env("DOMAIN_CERT_TRACKER_CONFIG="+cfg), secret)
	requireAbsent(t, "CLI startup stdout (config from the environment)", res.stdout, secret)
	requireAbsent(t, "CLI startup stderr (config from the environment)", res.stderr, secret)

	res = run(t, mcp, s.env("DOMAIN_CERT_TRACKER_CONFIG="+cfg), "--transport", secret)
	if res.code == 0 {
		t.Fatalf("an unknown transport succeeded: stdout=%q stderr=%q", res.stdout, res.stderr)
	}
	requireAbsent(t, "MCP startup stdout", res.stdout, secret)
	requireAbsent(t, "MCP startup stderr", res.stderr, secret)

	// The default config path under DOMAIN_CERT_TRACKER_HOME.
	d := newSandbox(t)
	defCfg := filepath.Join(d.home, "config", "config.toml")
	if err := os.MkdirAll(filepath.Dir(defCfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defCfg, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	res = run(t, mcp, d.env(), "--transport", secret)
	requireAbsent(t, "MCP startup stderr (default config path)", res.stderr, secret)
	res = run(t, cli, d.env(), secret)
	requireAbsent(t, "CLI startup stderr (default config path)", res.stderr, secret)
}

// Brief item 8(c): profiles.json is a credential source. A value under a
// credential-named flag is never shown, a key the program does not declare is
// dropped on load, and a plain value stays usable. A URL in a non-credential
// flag or the description is free text (contract section 1): not checked here.
func TestRunProfileStoreHeldAtStartup(t *testing.T) {
	cli, _ := binaries(t)
	const named = "storedpassword789"
	s := newSandbox(t)
	path := filepath.Join(s.home, "config", "profiles.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"profiles":{"nightly":{"name":"nightly","values":{` +
		`"limit":"7",` +
		`"new-password":"` + named + `",` +
		`"mystery":"not-a-real-flag-value"}}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	res := run(t, cli, s.env(), "profile", "show", "nightly", "--json")
	out := res.stdout + res.stderr
	requireAbsent(t, "profile show", out, named)
	if strings.Contains(out, "not-a-real-flag-value") {
		t.Errorf("a key no command declares survived the load:\n%s", out)
	}

	// A plain profile refuses nothing and stays readable.
	clean := newSandbox(t)
	cleanPath := filepath.Join(clean.home, "config", "profiles.json")
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cleanPath, []byte(`{"profiles":{"hook":{"name":"hook","values":{"limit":"7","deliver":"webhook:https://h.example/x?id=1"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	res = run(t, cli, clean.env(), "profile", "show", "hook", "--json")
	if res.code != 0 || !strings.Contains(res.stdout, "https://h.example/x?id=1") || !strings.Contains(res.stdout, `"limit"`) {
		t.Errorf("a plain profile did not stay usable (exit %d):\nstdout=%s\nstderr=%s", res.code, res.stdout, res.stderr)
	}
}

// A feedback endpoint is a URL from the environment: its token never appears
// when the upstream POST fails or is refused.
func TestFeedbackEndpointCredentialNeverShown(t *testing.T) {
	cli, _ := binaries(t)
	const pass, query = "endpointpass123", "endpointtoken456"
	for _, endpoint := range []string{
		"https://127.0.0.1:1/x?token=" + query, // the query alone: url.Error prints it
		"https://u:" + pass + "@127.0.0.1:1/x?token=" + query,
	} {
		s := newSandbox(t)
		res := run(t, cli, s.env("DOMAIN_CERT_TRACKER_FEEDBACK_ENDPOINT="+endpoint), "feedback", "--send", "hello")
		out := res.stdout + res.stderr
		requireAbsent(t, "feedback --send", out, query)
		requireAbsent(t, "feedback --send", out, pass)
		if hits := filesHolding(t, s.home, query); len(hits) > 0 {
			t.Errorf("the endpoint token was persisted to %v", hits)
		}
	}
}

// Codex publish pass 4 round 1: a --deliver URL is shown by scheme and host only,
// wherever it is shown, and the token still reaches the operator's own webhook.
func TestDeliverURLNeverShownWhole(t *testing.T) {
	cli, _ := binaries(t)
	const query, pass, path = "UrlSecret9z", "deliverpass123", "pathtoken456"
	for _, spec := range []string{
		"webhook:https://u:" + pass + "@hooks.example/" + path + "?token=" + query,
		"webhook:https://127.0.0.1:1/" + path + "?token=" + query + "&x=" + pass,
		"webhook:ftp://u:" + pass + "@hooks.example/" + path + "?token=" + query,
	} {
		for _, args := range [][]string{{"report", "--dry-run", "--deliver", spec}, {"report", "--json", "--deliver", spec}} {
			s := newSandbox(t)
			res := run(t, cli, s.env(), args...)
			out := res.stdout + res.stderr
			for _, secret := range []string{query, pass, path} {
				requireAbsent(t, strings.Join(args, " "), out, secret)
			}
		}
	}

	var got sync.Mutex
	received := ""
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Lock()
		defer got.Unlock()
		received = r.URL.RawQuery
	}))
	t.Cleanup(hook.Close)
	s := newSandbox(t)
	res := run(t, cli, s.env(), "report", "--json", "--deliver", "webhook:"+hook.URL+"/in?token="+query)
	got.Lock()
	defer got.Unlock()
	if received != "token="+query {
		t.Errorf("the webhook did not receive its own URL (query %q, exit %d): %s", received, res.code, res.stderr)
	}
}

// Round 2, deliver-diagnostics: what a webhook or feedback endpoint answers never
// reaches a message, so a server reflecting the URL's token cannot print it. A
// message says the scheme, the host and a numeric status.
func TestDeliverDiagnosticsShowNoServerText(t *testing.T) {
	cli, _ := binaries(t)
	const token = "UrlSecret9z"
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		fmt.Fprintf(buf, "HTTP/1.1 302 Found %s\r\nLocation: /retry/%s\r\nContent-Length: 0\r\nConnection: close\r\n\r\n", token, token)
		_ = buf.Flush()
	}))
	t.Cleanup(hook.Close)
	s := newSandbox(t)
	res := run(t, cli, s.env(), "report", "--json", "--deliver", "webhook:"+hook.URL+"/in?token="+token)
	requireAbsent(t, "a redirecting webhook", res.stdout+res.stderr, token)
	if !strings.Contains(res.stderr, "302") {
		t.Errorf("the failure lost its status: %s", res.stderr)
	}
	res = run(t, cli, s.env(), "report", "--json", "--deliver", "webhook:https://user:"+token+"/x")
	requireAbsent(t, "a URL the parser rejects", res.stdout+res.stderr, token)

	res = run(t, cli, s.env("DOMAIN_CERT_TRACKER_FEEDBACK_ENDPOINT="+hook.URL+"/in/"+token), "feedback", "--send", "--json", "hello")
	requireAbsent(t, "a redirecting feedback endpoint", res.stdout+res.stderr, token)
	ok := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(ok.Close)
	for _, args := range [][]string{{"feedback", "--send", "hello"}, {"feedback", "--send", "--json", "hello"}} {
		res = run(t, cli, s.env("DOMAIN_CERT_TRACKER_FEEDBACK_ENDPOINT="+ok.URL+"/in/"+token), args...)
		requireAbsent(t, "feedback's success output", res.stdout+res.stderr, token)
	}
}

// Round 2, base-url-credentials: a credential in a base URL that does not parse,
// or that the URL parser reads as something else, is held all the same.
func TestBaseURLCredentialsHeldInAnyShape(t *testing.T) {
	cli, _ := binaries(t)
	for u, secret := range map[string]string{
		"https://admin:s3cretPa/ssw0rdAA@127.0.0.1:18543": "s3cretPa/ssw0rdAA",
		"https://admin:s3cret?Passw0rd12@127.0.0.1:18543": "Passw0rd12",
		"https:admin:s3cretPassw0rd@127.0.0.1:18543":      "s3cretPassw0rd",
	} {
		s := newSandbox(t)
		cfg := s.writePrivate(t, "c.toml", "base_url = \""+u+"\"\n")
		for name, r := range map[string]runResult{
			"environment, dry run": run(t, cli, s.env("DOMAIN_CERT_TRACKER_BASE_URL="+u), "domain", "example.invalid", "--dry-run"),
			"environment, doctor":  run(t, cli, s.env("DOMAIN_CERT_TRACKER_BASE_URL="+u), "doctor", "--json"),
			"config, dry run":      run(t, cli, s.env(), "--config", cfg, "domain", "example.invalid", "--dry-run"),
			"config, doctor":       run(t, cli, s.env(), "--config", cfg, "doctor", "--json"),
		} {
			requireAbsent(t, name+" "+u, r.stdout+r.stderr, secret)
		}
	}
}

// Round 2's other direction: what the closed classes must leave working.
func TestPlainValuesStayUsable(t *testing.T) {
	cli, _ := binaries(t)
	s := newSandbox(t)
	path := filepath.Join(s.home, "config", "profiles.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"profiles":{"hook":{"name":"hook","description":"see https://wiki.invalid/page?n=1#top","values":{"json":"true"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if res := run(t, cli, s.env(), "version"); res.code != 0 {
		t.Errorf("a plain URL in a profile stopped the program (exit %d): %s", res.code, res.stderr)
	}
	for _, spec := range []string{"webhook:https://hooks.invalid/notify/ops@acme.invalid", "webhook:https://hooks.invalid/in?channel=ops"} {
		if res := run(t, cli, s.env(), "profile", "save", "p", "--deliver", spec); res.code != 0 {
			t.Errorf("profile save refused %s (exit %d): %s", spec, res.code, res.stderr)
		}
	}
	if res := run(t, cli, s.env("DOMAIN_CERT_TRACKER_BASE_URL=https://user:pAssW0rdXYZ12@127.0.0.1:18543"), "version"); res.code != 0 {
		t.Errorf("a common word as the Basic user stopped the program (exit %d): %s", res.code, res.stderr)
	}
	if res := run(t, cli, s.env("DOMAIN_CERT_TRACKER_FEEDBACK_ENDPOINT=https://127.0.0.1:1/in?id=1"), "version"); res.code != 0 {
		t.Errorf("a plain feedback endpoint stopped the program (exit %d): %s", res.code, res.stderr)
	}
	cfg := s.writePrivate(t, "c.toml", "[headers]\nX-Tenant = \"acme_corp\"\n")
	if res := run(t, cli, s.env(), "--config", cfg, "tenant-acmecorp"); strings.Contains(res.stderr, "withheld") {
		t.Errorf("a header value with an underscore withheld its unseparated spelling: %s", res.stderr)
	}
}
