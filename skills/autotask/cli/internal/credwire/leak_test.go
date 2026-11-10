// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring), in the shape of blackpoint's. Set AUTOTASK_LEAK_BIN_DIR to run
// them against binaries built elsewhere (how the build with the kit's wiring
// disabled was seen failing them).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
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

	"autotask-pp-cli/internal/cli"
	"autotask-pp-cli/internal/cliutil"
	"autotask-pp-cli/internal/credkit"
	"autotask-pp-cli/internal/credwire"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const apiPath = "/atservicesrest/V1.0"

var (
	binOnce sync.Once
	binDir  string
	binErr  error
)

func binaries(t *testing.T) (cliBin, mcpBin string) {
	t.Helper()
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	binOnce.Do(func() {
		if d := os.Getenv("AUTOTASK_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "autotask-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{{"autotask-cli", "../../cmd/autotask-cli"}, {"autotask-mcp", "../../cmd/autotask-mcp"}} {
			if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, b.out+exe), b.pkg).CombinedOutput(); err != nil {
				binErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, "autotask-cli"+exe), filepath.Join(binDir, "autotask-mcp"+exe)
}

// vendor is a TLS stub of the Autotask REST API: the kit never sends a
// credential over plain http, loopback included.
type vendor struct {
	url, caFile string
	hits        atomic.Int64
	secret      atomic.Value
}

func startVendor(t *testing.T, h http.HandlerFunc) *vendor {
	t.Helper()
	v := &vendor{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.hits.Add(1)
		v.secret.Store(r.Header.Get("Secret"))
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	v.url = srv.URL
	v.caFile = filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(v.caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return v
}

// env is the three Autotask credentials against this stub, secret as the Secret.
func (v *vendor) env(secret string) []string {
	return []string{
		"AUTOTASK_BASE_URL=" + v.url + apiPath, "AUTOTASK_CA_CERT=" + v.caFile,
		"AUTOTASK_API_INTEGRATION_CODE=" + "LK4QZ7W1M9C3R8T2Y6PXB5VNDA",
		"AUTOTASK_PSA_USER_NAME=leak-api-user-7d2c@psa-api.example",
		"AUTOTASK_PSA_SECRET=" + secret,
	}
}

func item(w http.ResponseWriter, title string) {
	fmt.Fprintf(w, `{"item":{"id":1,"title":%q}}`, title)
}

// closedEnv is a closed environment under one sandbox home. A request to any
// host but loopback meets a dead proxy, so nothing leaves this machine.
func closedEnv(home string, extra ...string) []string {
	return append([]string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "AUTOTASK_HOME=" + home,
		"TMPDIR=" + os.TempDir(), "NO_COLOR=1",
		"HTTPS_PROXY=http://127.0.0.1:1", "HTTP_PROXY=http://127.0.0.1:1", "NO_PROXY=", "no_proxy=",
	}, extra...)
}

func run(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	out, code := runCode(t, bin, env, args...)
	// A refused start prints nothing, which every absence check would pass on.
	if code == credkit.ExitRefused {
		t.Errorf("%s %s: the start was refused, so this run proves nothing:\n%s", filepath.Base(bin), strings.Join(args[:min(3, len(args))], " "), out)
	}
	return out
}

func runCode(t *testing.T, bin string, env []string, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run() // exit codes vary by case; the output is judged
	return out.String(), cmd.ProcessState.ExitCode()
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertAbsent(t *testing.T, label, out string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if strings.Contains(out, s) {
			t.Errorf("%s shows %q:\n%.2000s", label, s, out)
		}
	}
}

// assertNoFileCarries walks home; skip names the files the test itself wrote.
func assertNoFileCarries(t *testing.T, home string, skip []string, secrets ...string) {
	t.Helper()
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || slices.Contains(skip, p) {
			return nil
		}
		data, rerr := os.ReadFile(p) // #nosec G304 -- a file under the test's own temp home
		if rerr != nil {
			return nil
		}
		for _, s := range secrets {
			if !synccpuStoreFile(p) && bytes.Contains(data, []byte(s)) { // synccpu: the local store is not checked
				t.Errorf("%s holds %q", p, s)
			}
		}
		return nil
	})
}

// mcpCall runs one MCP stdio session of tools/call params and returns stdout
// plus stderr, with every call answered.
func mcpCall(t *testing.T, bin string, env []string, calls ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = env
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	fmt.Fprintln(stdin, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	for i, c := range calls {
		fmt.Fprintf(stdin, `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":%s}`+"\n", i+2, c)
	}
	var out strings.Builder
	answered := 0
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for answered < len(calls) && sc.Scan() {
		out.WriteString(sc.Text() + "\n")
		var msg struct {
			ID *int `json:"id"`
		}
		if json.Unmarshal(sc.Bytes(), &msg) == nil && msg.ID != nil && *msg.ID >= 2 {
			answered++
		}
	}
	_ = stdin.Close()
	_, _ = io.Copy(&out, stdout)
	_ = cmd.Wait()
	if answered != len(calls) {
		t.Errorf("MCP answered %d of %d calls:\n%.2000s\n%.2000s", answered, len(calls), out.String(), stderr.String())
	}
	return out.String() + stderr.String()
}

const readTicket = `{"name":"autotask_read","arguments":{"endpoint_id":"tickets.query-item","params":{"id":"1"}}}`

// TestShortHeldValueNeverPrinted: a value of any length is held. A six-digit
// password in the base URL, echoed by a flag error, and a six-digit secret the
// vendor echoes, in both binaries.
func TestShortHeldValueNeverPrinted(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const short = "482915"
	home := t.TempDir()
	base := closedEnv(home, "AUTOTASK_BASE_URL=https://u:"+short+"@webservices5.autotask.net"+apiPath)
	assertAbsent(t, "CLI flag error", run(t, cliBin, base, "--timeout", short, "version"), short)
	assertAbsent(t, "MCP flag error", run(t, mcpBin, base, "--transport", short), short)

	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { item(w, "HQ "+short) })
	home = t.TempDir()
	env := closedEnv(home, v.env(short)...)
	assertAbsent(t, "query-item", run(t, cliBin, env, "tickets", "query-item", "1", "--json", "--no-learn"), short)
	assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, readTicket), short)
	if v.hits.Load() == 0 {
		t.Fatal("the vendor was never called; the echo checks above prove nothing")
	}
	assertNoFileCarries(t, home, nil, short)
}

// TestMalformedConfigHeldBeforeAnythingPrints: a flag error prints before any
// command loads config, from a config TOML rejects, in both binaries.
func TestMalformedConfigHeldBeforeAnythingPrints(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	cases := []struct{ name, file, secret string }{
		{"comment-glued-to-quote", "psa_secret = \"at_glued_0101x\"#longcomment\nbroken = = longenoughvalue\n", "at_glued_0101x"},
		{"unterminated-array", "extra = [\"at_array_8910x\",\npsa_secret = \"at_after_1112x\"\n", "at_array_8910x"},
		{"key-after-unterminated-array", "extra = [\"filler-value\",\napi_integration_code = \"AT9AFTER1314XQ\"\n", "AT9AFTER1314XQ"},
		{"multi-line-string", "psa_secret = \"\"\"\nat_multi_1516x\n\"\"\"\nbroken = = longenoughvalue\n", "at_multi_1516x"},
		{"bare-value", "psa_secret = at_bare_2526x\nbroken = = longenoughvalue\n", "at_bare_2526x"},
		{"base-url-password", "base_url = \"https://u:AtUrlPass2728x@[bad\"\n", "AtUrlPass2728x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "config", "config.toml"), c.file)
			env := closedEnv(home)
			assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", c.secret, "version"), c.secret)
			assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", c.secret), c.secret)
		})
	}
}

// TestDiscoveryCoversEveryFileLoadReads: a credential in any file a run could read
// is held before a flag error prints.
func TestDiscoveryCoversEveryFileLoadReads(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "at_discover_3132x"
	files := map[string]func(home string) string{
		"explicit-config":  func(h string) string { return filepath.Join(h, "tenant", "config.toml") },
		"explicit-store":   func(h string) string { return filepath.Join(h, "tenant", "data", "credentials.toml") },
		"global-store":     func(h string) string { return filepath.Join(h, "data", "credentials.toml") },
		"global-config":    func(h string) string { return filepath.Join(h, "config", "config.toml") },
		"legacy-config":    func(h string) string { return filepath.Join(h, ".config", "autotask-cli", "config.toml") },
		"home-flag-store":  func(h string) string { return filepath.Join(h, "other", "data", "credentials.toml") },
		"home-flag-config": func(h string) string { return filepath.Join(h, "other", "config", "config.toml") },
	}
	for name, path := range files {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, path(home), `psa_secret = "`+secret+`"`+"\n")
			args := []string{"--config", filepath.Join(home, "tenant", "config.toml"), "--home", filepath.Join(home, "other"), "--timeout", secret, "version"}
			assertAbsent(t, name, run(t, cliBin, closedEnv(home), args...), secret)
		})
	}
}

// TestOutboundCarriesNoCredentialOffItsOrigin: a secret in the feedback URL and
// in a webhook delivery's User-Agent never leaves for another origin. The sink
// is plain http on loopback, so only the kit stands between the secret and it;
// a control without the secret arrives, so the sink is reachable.
func TestOutboundCarriesNoCredentialOffItsOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "at-sentin" + "el-secret-123456"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { item(w, "HQ") })
	var mu sync.Mutex
	var seen strings.Builder
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		fmt.Fprintf(&seen, "%s %s\n%v\n%s\n", r.Method, r.RequestURI, r.Header, b)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(sink.Close)
	sinkSaw := func() string {
		mu.Lock()
		defer mu.Unlock()
		return seen.String()
	}
	home := t.TempDir()
	env := closedEnv(home, append(v.env(secret), "AUTOTASK_FEEDBACK_ENDPOINT="+sink.URL+"/receipt/"+secret)...)
	assertAbsent(t, "feedback", run(t, cliBin, env, "feedback", "harmless note", "--send"), secret)
	env = closedEnv(home, append(v.env(secret), "AUTOTASK_USER_AGENT="+secret)...)
	assertAbsent(t, "webhook", run(t, cliBin, env, "tickets", "query-item", "1", "--json", "--no-learn", "--deliver", "webhook:"+sink.URL+"/hook"), secret)
	assertAbsent(t, "the other origin", sinkSaw(), secret)
	// Control: the same sink receives what carries no credential.
	env = closedEnv(home, append(v.env(secret), "AUTOTASK_FEEDBACK_ENDPOINT="+sink.URL+"/receipt/plain")...)
	run(t, cliBin, env, "feedback", "control HQ-Feedback", "--send")
	run(t, cliBin, closedEnv(home, v.env(secret)...), "tickets", "query-item", "1", "--json", "--no-learn", "--deliver", "webhook:"+sink.URL+"/hook-control")
	if got := sinkSaw(); !strings.Contains(got, "HQ-Feedback") || !strings.Contains(got, "/hook-control") {
		t.Fatalf("the sink received no control; the absence above proves nothing:\n%s", got)
	}
}

// TestCredentialFieldsReplacedByName: a webhook's secretKey and a client portal
// user's password in a response are replaced by name before any sink, and the
// record's other fields stay usable.
func TestCredentialFieldsReplacedByName(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const key, pw = "webhook-response-key-98765", "hunter2hunter2"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"item":{"id":1,"title":"HQ Hook","secretKey":%q,"portal":{"password":%q}}}`, key, pw)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env("at-fields-secret-33344455")...)
	out := run(t, cliBin, env, "tickets", "query-item", "1", "--json", "--no-learn")
	assertAbsent(t, "query-item", out, key, pw)
	if !strings.Contains(out, "HQ Hook") || !strings.Contains(out, cliutil.SecretFieldWithheld) {
		t.Errorf("want the record with its credential fields replaced:\n%s", out)
	}
	out = mcpCall(t, mcpBin, env, readTicket)
	assertAbsent(t, "MCP read", out, key, pw)
	if !strings.Contains(out, "HQ Hook") {
		t.Errorf("MCP: the record's other fields are gone:\n%s", out)
	}
	assertNoFileCarries(t, home, nil, key, pw)
}

// TestUsedConfigDecidesTheAllowedOrigin: the config the program really uses names
// the origin a credential may go to. A discovered default's base_url is only held,
// so a credential never goes there.
func TestUsedConfigDecidesTheAllowedOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "at_origin_secret_44455566"
	used := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { item(w, "HQ") })
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "config.toml"), "base_url = \""+other.url+apiPath+"\"\n")
	explicit := filepath.Join(home, "tenant.toml")
	writeFile(t, explicit, "base_url = \""+used.url+apiPath+"\"\napi_integration_code = \"OR7GN5Q2W8E4R1T6Y3UZXCVBKM\"\npsa_user_name = \"origin-user@psa-api.example\"\npsa_secret = \""+secret+"\"\n")
	env := closedEnv(home, "AUTOTASK_CA_CERT="+used.caFile)
	out := run(t, cliBin, env, "--config", explicit, "tickets", "query-item", "1", "--json", "--no-learn", "--no-cache")
	assertAbsent(t, "query-item", out, secret)
	if used.hits.Load() == 0 {
		t.Errorf("the explicit config's base URL was refused or unused:\n%s", out)
	}
	if n := other.hits.Load(); n != 0 {
		t.Errorf("%d requests went to the discovered default's base URL", n)
	}
}

// TestChildGetsTheCredentials: an MCP mirror tool runs the CLI, and the kit hands
// the child its credentials. The Secret reaches the vendor.
func TestChildGetsTheCredentials(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "at-child-" + "4142434-xQ"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"items":[],"pageDetails":{"count":0,"requestCount":100,"prevPageUrl":null,"nextPageUrl":null}}`)
	})
	home := t.TempDir()
	env := closedEnv(home, append(v.env(secret), "AUTOTASK_CLI_PATH="+cliBin)...)
	out := mcpCall(t, mcpBin, env, `{"name":"export","arguments":{"resource":"tickets"}}`)
	assertAbsent(t, "export", out, secret, "withheld from child")
	if v.hits.Load() == 0 {
		t.Fatalf("the child reached no vendor endpoint:\n%.2000s", out)
	}
	if got, _ := v.secret.Load().(string); got != secret {
		t.Errorf("the child sent Secret %q, want the handed secret", got)
	}
}

// TestMCPHTTPRefusalsPassTheKit: the HTTP server's refusals and results leave
// through credkit.MCPHTTP, so a refused request echoes nothing held.
func TestMCPHTTPRefusalsPassTheKit(t *testing.T) {
	_, mcpBin := binaries(t)
	const token = "mcp-bearer-4344-" + "0123456789abcdef-long"
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, mcpBin, "--transport", "http", "--addr", addr)
	cmd.Env = closedEnv(t.TempDir(), "AUTOTASK_MCP_HTTP_TOKEN="+token)
	var serverOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &serverOut, &serverOut
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	post := func(auth, origin string) (int, string) {
		var last error
		for i := 0; i < 100; i++ {
			req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			if auth != "" {
				req.Header.Set("Authorization", "Bearer "+auth)
			}
			if origin != "" {
				req.Header.Set("Origin", origin)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				last = err
				time.Sleep(100 * time.Millisecond)
				continue
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return resp.StatusCode, string(b)
		}
		t.Fatalf("the server never answered: %v\n%s", last, serverOut.String())
		return 0, ""
	}
	if code, body := post("", ""); code != http.StatusUnauthorized || strings.Contains(body, token) {
		t.Errorf("no token: %d %s", code, body)
	}
	if code, body := post("wrong-"+token, ""); code != http.StatusUnauthorized || strings.Contains(body, token) {
		t.Errorf("wrong token echoed: %d %s", code, body)
	}
	if code, body := post(token, "https://evil.example"); code != http.StatusForbidden || strings.Contains(body, token) {
		t.Errorf("foreign origin: %d %s", code, body)
	}
	if code, body := post(token, ""); code != http.StatusOK || !strings.Contains(body, "serverInfo") || strings.Contains(body, token) {
		t.Errorf("good token: %d %s", code, body)
	}
	assertAbsent(t, "server output", serverOut.String(), token)
}

// TestMCPRefusesAConfigFlagItDoesNotHave: autotask-mcp has no --config or --home,
// but startup reads every occurrence and the kit takes the allowed origin from it.
// A flag that takes a value (--addr --config x) or a stray positional hides one
// from the parser, so the server refuses to start rather than run on a file it
// never uses.
func TestMCPRefusesAConfigFlagItDoesNotHave(t *testing.T) {
	_, mcpBin := binaries(t)
	home := t.TempDir()
	used := filepath.Join(home, "used.toml")
	decoy := filepath.Join(home, "decoy.toml")
	writeFile(t, used, "base_url = \"https://webservices5.autotask.net"+apiPath+"\"\n")
	writeFile(t, decoy, "base_url = \"https://collector.example"+apiPath+"\"\n")
	env := closedEnv(home, "AUTOTASK_CONFIG="+used, "AUTOTASK_PSA_SECRET=at-mcp-fixture-value")
	for name, args := range map[string][]string{
		"swallowed-config": {"--addr", "--config", decoy},
		"after-positional": {"x", "--config", decoy},
		"swallowed-home":   {"--addr", "--home", filepath.Join(home, "other")},
	} {
		t.Run(name, func(t *testing.T) {
			out, code := runCode(t, mcpBin, env, args...)
			if code != 2 || !strings.Contains(out, "takes no --config or --home") {
				t.Errorf("exit %d, want a refusal (2):\n%s", code, out)
			}
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, mcpBin, "--transport", "stdio")
	cmd.Env, cmd.Stdin = env, strings.NewReader("")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("an ordinary start: %v\n%s", err, out)
	}
}

// TestDeclaredFlagsExist keeps the flag declarations honest: every credential
// flag and stdin flag the kit is told about is a flag some command has.
func TestDeclaredFlagsExist(t *testing.T) {
	seen := map[string]bool{}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		visit := func(f *pflag.Flag) { seen["--"+f.Name] = true }
		c.Flags().VisitAll(visit)
		c.PersistentFlags().VisitAll(visit)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(cli.RootCmd())
	for _, f := range append(slices.Clone(credwire.Declarations.CredentialFlags), credwire.Declarations.StdinFlags...) {
		if !seen[f] {
			t.Errorf("credwire declares %s, which no command has", f)
		}
	}
}

// TestDeclarationsMatchTheMaskedFields: the client replaces the same credential
// members by name that the kit holds from a response.
func TestDeclarationsMatchTheMaskedFields(t *testing.T) {
	d := credwire.Declarations
	for _, got := range [][]string{d.CredentialFields, d.ResponseFields} {
		if !slices.Equal(got, cliutil.MaskedResponseFields) {
			t.Errorf("declared fields %v, want %v", got, cliutil.MaskedResponseFields)
		}
	}
	if !slices.Equal(d.ConfigFields, d.StoredRecords) {
		t.Errorf("config fields %v and stored records %v differ", d.ConfigFields, d.StoredRecords)
	}
}

// TestRunProfileCredentialValuesAreHeld: a run profile's value under a
// declared credential name (secret) or under base-url is held, every
// duplicate included; a value under any other flag stays usable. zone's
// --user is no such name: the API user name is an identifier.
func TestRunProfileCredentialValuesAreHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const (
		sec   = "profile-secret-9z7x5vQ2w4"
		first = "profile-dup-first-3c1bZk"
		pass  = "profile-base-pass-8m2n4p"
	)
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"p":{"name":"p","values":{
		"secret":"`+sec+`","secret":"`+first+`","secret":"","base-url":"https://u:`+pass+`@webservices5.autotask.net/atservicesrest/V1.0",
		"select":"`+sec+` `+first+` `+pass+`","limit":"nightly-plain-value-77"}}}}`)
	out := run(t, cliBin, closedEnv(home), "profile", "show", "p", "--json", "--no-learn")
	assertAbsent(t, "profile show", out, sec, first, pass)
	writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"p":{"name":"p","values":{"limit":"nightly-plain-value-77"}}}}`)
	if out := run(t, cliBin, closedEnv(home), "profile", "show", "p", "--json", "--no-learn"); !strings.Contains(out, "nightly-plain-value-77") {
		t.Errorf("a plain value is gone:\n%s", out)
	}
}
