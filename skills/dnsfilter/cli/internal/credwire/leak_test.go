// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is one the review's failing cases named. Set
// DNSFILTER_LEAK_BIN_DIR to run them against binaries built elsewhere (how the
// pre-kit build was seen failing: .review/classes-fail-first.txt).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
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

	"dnsfilter-pp-cli/internal/cli"
	"dnsfilter-pp-cli/internal/credwire"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	binOnce sync.Once
	binDir  string
	binErr  error
)

func binaries(t *testing.T) (cliBin, mcpBin string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("drives Unix config-file modes; the Windows build is vetted and compiled by the gates")
	}
	binOnce.Do(func() {
		if d := os.Getenv("DNSFILTER_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "dnsfilter-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{{"dnsfilter-cli", "../../cmd/dnsfilter-cli"}, {"dnsfilter-mcp", "../../cmd/dnsfilter-mcp"}} {
			if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, b.out), b.pkg).CombinedOutput(); err != nil {
				binErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, "dnsfilter-cli"), filepath.Join(binDir, "dnsfilter-mcp")
}

// vendor is a TLS stub of the DNSFilter API: the kit never sends a credential
// over plain http, loopback included.
type vendor struct {
	url, caFile string
	hits        atomic.Int64
	auth        atomic.Value
}

func startVendor(t *testing.T, h http.HandlerFunc) *vendor {
	t.Helper()
	v := &vendor{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.hits.Add(1)
		v.auth.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	v.url = srv.URL
	v.caFile = filepath.Join(t.TempDir(), "ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(v.caFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return v
}

func (v *vendor) env(key string) []string {
	return []string{"DNSFILTER_BASE_URL=" + v.url, "DNSFILTER_CA_CERT=" + v.caFile, "DNSFILTER_API_KEY=" + key}
}

// closedEnv is a closed environment under one sandbox home.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "DNSFILTER_HOME=" + home, "TMPDIR=" + os.TempDir(), "NO_COLOR=1"}, extra...)
}

func run(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run() // exit codes vary by case; the output is judged
	return out.String()
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
			t.Errorf("%s shows %q:\n%s", label, s, out)
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
		data, rerr := os.ReadFile(p)
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
func mcpCall(t *testing.T, bin string, env []string, args []string, calls ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
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
		t.Errorf("MCP answered %d of %d calls:\n%s\n%s", answered, len(calls), out.String(), stderr.String())
	}
	return out.String() + stderr.String()
}

// TestShortHeldValueNeverPrinted is the short-held-value-floor class: a value
// of any length is held. The review's input: a six-digit password in the base
// URL, echoed by a flag error, and a six-digit key echoed by the vendor.
func TestShortHeldValueNeverPrinted(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const short = "482915"
	home := t.TempDir()
	base := closedEnv(home, "DNSFILTER_BASE_URL=https://u:"+short+"@api.dnsfilter.com")
	assertAbsent(t, "CLI flag error", run(t, cliBin, base, "--timeout", short, "version"), short)
	assertAbsent(t, "MCP flag error", run(t, mcpBin, base, "--transport", short), short)

	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"data":[{"id":"7","type":"policies","attributes":{"name":%q}}]}`, short)
	})
	home = t.TempDir()
	env := closedEnv(home, v.env(short)...)
	assertAbsent(t, "policies list", run(t, cliBin, env, "policies", "list", "--json"), short)
	assertAbsent(t, "sync", run(t, cliBin, env, "sync", "--resources", "policies"), short)
	assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, nil, `{"name":"dnsfilter_read","arguments":{"endpoint_id":"policies.all"}}`), short)
	assertNoFileCarries(t, home, nil, short)
}

// TestMalformedConfigHeldBeforeAnythingPrints is the
// malformed-config-raw-hold-not-quote-aware class plus the startup-ordering
// check: a flag error prints before any command loads config, from a config
// TOML rejects, in both binaries. Each value is shaped by the review's input.
func TestMalformedConfigHeldBeforeAnythingPrints(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	cases := []struct{ name, file, secret string }{
		{"inline-table-comma", "headers = { X-Api-Key = \",df_live_comma123\" }\nbroken = = 1\n", ",df_live_comma123"},
		{"inline-table-comma-tail", "headers = { X-Api-Key = \",df_live_tail4567\" }\nbroken = = 1\n", "df_live_tail4567"},
		{"unterminated-array", "extra = [\"df_live_array8910\",\napi_key = \"df_live_after1112\"\n", "df_live_array8910"},
		{"key-after-unterminated-array", "extra = [\"x\",\napi_key = \"df_live_after1314\"\n", "df_live_after1314"},
		{"multi-line-string", "access_token = \"\"\"\ndf_live_multi1516\n\"\"\"\nbroken = = 1\n", "df_live_multi1516"},
		{"unterminated-multi-line", "note = \"\"\"\napi_key = \"df_live_inside1718\"\n", "df_live_inside1718"},
		{"escaped-quote-raw", "api_key = \"df_live\\\"esc1920\"\nbroken = = 1\n", `df_live\"esc1920`},
		{"escaped-quote-decoded", "api_key = \"df_live\\\"esc2122\"\nbroken = = 1\n", `df_live"esc2122`},
		{"equals-in-array-string", "extra = [\n\"df=live2324\",\n]\nbroken = = 1\n", "df=live2324"},
		{"credential-all-punctuation", "api_key = \"!!--!!\"\nbroken = = 1\n", "!!--!!"},
		{"bare-value", "client_secret = df_live_bare2526\nbroken = = 1\n", "df_live_bare2526"},
		{"hash-in-bare-value", "client_secret = df_live#hash2728\nbroken = = 1\n", "df_live#hash2728"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			cfg := filepath.Join(home, "config", "config.toml")
			writeFile(t, cfg, c.file)
			env := closedEnv(home)
			assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", c.secret, "version"), c.secret)
			assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", c.secret), c.secret)
		})
	}
}

// TestDiscoveryCoversEveryFileLoadReads traces config.Load's reads: under an
// explicit --config it also reads the global store and config, so a credential
// in any of them is held before a flag error prints.
func TestDiscoveryCoversEveryFileLoadReads(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "df_live_discover2930"
	files := map[string]func(home string) string{
		"explicit-config":  func(h string) string { return filepath.Join(h, "tenant", "config.toml") },
		"explicit-sibling": func(h string) string { return filepath.Join(h, "tenant", "data", "credentials.toml") },
		"global-store":     func(h string) string { return filepath.Join(h, "data", "credentials.toml") },
		"global-config":    func(h string) string { return filepath.Join(h, "config", "config.toml") },
		"legacy-config":    func(h string) string { return filepath.Join(h, ".config", "dnsfilter-cli", "config.toml") },
		"home-flag-store":  func(h string) string { return filepath.Join(h, "other", "data", "credentials.toml") },
		"home-flag-config": func(h string) string { return filepath.Join(h, "other", "config", "config.toml") },
	}
	for name, path := range files {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, path(home), `api_key = "`+secret+`"`+"\n")
			args := []string{"--config", filepath.Join(home, "tenant", "config.toml"), "--home", filepath.Join(home, "other"), "--timeout", secret, "version"}
			assertAbsent(t, name, run(t, cliBin, closedEnv(home), args...), secret)
		})
	}
}

// layers wraps s in n rounds of percent-encoding.
func layers(s string, n int) string {
	for i := 0; i < n; i++ {
		s = url.QueryEscape(s)
	}
	return s
}

// TestBoundedDecodeWithholds is the bounded-decode-treated-as-clean class: a
// decode that hits its depth or size limit must withhold, never pass. The
// review's inputs: double base64 in an octet-stream, 1,001 percent layers and
// deep re-encoding, on stdout, the export -o file and the response cache.
func TestBoundedDecodeWithholds(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const key = "df_live_" + "bounded3132"
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	double := b64(b64(key))
	cases := []struct {
		name, contentType, body string
		never                   []string
	}{
		{"double-base64-octet-stream", "application/octet-stream", double, []string{double, b64(key), key}},
		{"double-base64-json", "application/json", `{"data":[{"id":"1","type":"policies","attributes":{"name":"` + double + `"}}]}`, []string{double, b64(key), key}},
		{"percent-5-layers", "application/json", `{"data":[{"id":"2","type":"policies","attributes":{"name":"` + layers(key, 5) + `"}}]}`, []string{layers(key, 5), key}},
		{"percent-40-layers", "application/json", `{"data":[{"id":"3","type":"policies","attributes":{"name":"` + layers(key, 40) + `"}}]}`, []string{layers(key, 40), key}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", c.contentType)
				io.WriteString(w, c.body)
			})
			home := t.TempDir()
			env := closedEnv(home, v.env(key)...)
			assertAbsent(t, "stdout", run(t, cliBin, env, "policies", "list", "--json"), c.never...)
			exported := filepath.Join(home, "export.jsonl")
			assertAbsent(t, "export", run(t, cliBin, env, "export", "policies", "-o", exported), c.never...)
			assertAbsent(t, "MCP", mcpCall(t, mcpBin, env, nil, `{"name":"dnsfilter_read","arguments":{"endpoint_id":"policies.all"}}`), c.never...)
			assertNoFileCarries(t, home, nil, c.never...)
		})
	}
	t.Run("clean-control", func(t *testing.T) {
		// The same paths on an answer that carries nothing: every sink is written,
		// so the cases above withhold rather than fail for another reason.
		v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
			io.WriteString(w, `{"data":[{"id":"5","type":"policies","attributes":{"name":"Plain Policy"}}]}`)
		})
		home := t.TempDir()
		env := closedEnv(home, v.env(key)...)
		if out := run(t, cliBin, env, "policies", "list", "--json"); !strings.Contains(out, "Plain Policy") {
			t.Errorf("stdout lost the clean answer:\n%s", out)
		}
		exported := filepath.Join(home, "export.jsonl")
		out := run(t, cliBin, env, "export", "policies", "-o", exported)
		if data, err := os.ReadFile(exported); err != nil || !bytes.Contains(data, []byte("Plain Policy")) {
			t.Errorf("export lost the clean answer: %v %q\n%s", err, data, out)
		}
		if mcp := mcpCall(t, mcpBin, env, nil, `{"name":"dnsfilter_read","arguments":{"endpoint_id":"policies.all"}}`); !strings.Contains(mcp, "Plain Policy") {
			t.Errorf("MCP lost the clean answer:\n%s", mcp)
		}
	})
	t.Run("percent-1001-layers", func(t *testing.T) {
		// 1,001 layers of the key alone grow past any sane body: percent-encode a
		// short held value so the stub can send it.
		const short = "k7Q"
		deep := short
		for i := 0; i < 1001; i++ {
			deep = strings.ReplaceAll(deep, "%", "%25")
			if i == 0 {
				deep = "%6B7Q"
			}
		}
		v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, `{"data":[{"id":"4","type":"policies","attributes":{"name":%q}}]}`, deep)
		})
		home := t.TempDir()
		env := closedEnv(home, v.env(short)...)
		assertAbsent(t, "stdout", run(t, cliBin, env, "policies", "list", "--json"), deep)
		assertNoFileCarries(t, home, nil, deep)
	})
}

// TestCredentialFieldsReplacedByName: a list that carries a credential field
// stays usable, with the field's value replaced, in every sink.
func TestCredentialFieldsReplacedByName(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret, token = "site-secret-3334", "api-token-3536"
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "api_keys") {
			fmt.Fprintf(w, `{"data":{"id":"1","type":"api_keys","attributes":{"name":"ci","token":%q}}}`, token)
			return
		}
		fmt.Fprintf(w, `{"data":[{"id":"11","type":"networks","attributes":{"name":"Main Office","secret_key":%q}}],"links":{},"meta":{}}`, secret)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env("df_live_fields3738")...)
	out := run(t, cliBin, env, "networks", "all", "--json")
	assertAbsent(t, "networks", out, secret)
	if !strings.Contains(out, "Main Office") {
		t.Errorf("the rest of the row was lost:\n%s", out)
	}
	assertAbsent(t, "api-keys show", run(t, cliBin, env, "api-keys", "show", "1", "--json"), token)
	assertAbsent(t, "sync", run(t, cliBin, env, "sync", "--resources", "networks"), secret)
	mcp := mcpCall(t, mcpBin, env, nil, `{"name":"dnsfilter_read","arguments":{"endpoint_id":"networks.all"}}`)
	assertAbsent(t, "MCP", mcp, secret)
	if !strings.Contains(mcp, "Main Office") {
		t.Errorf("the MCP result lost the row:\n%s", mcp)
	}
	assertNoFileCarries(t, home, nil, secret, token)
}

// TestWholeAnswerCredentialsSendNothing: the two reads whose whole answer is a
// credential send no request, in the CLI and the MCP server.
func TestWholeAnswerCredentialsSendNothing(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `"4938"`) })
	home := t.TempDir()
	env := closedEnv(home, v.env("df_live_whole3940")...)
	out := run(t, cliBin, env, "user-agents", "uninstall-pin", "--organization-id", "5", "--json")
	if !strings.Contains(out, "dashboard") {
		t.Errorf("uninstall-pin not refused:\n%s", out)
	}
	out = run(t, cliBin, env, "psa-integrations", "--organization-id", "5", "--psa-name", "connectwise", "--yes", "--json")
	if !strings.Contains(out, "dashboard") {
		t.Errorf("psa-integrations not refused:\n%s", out)
	}
	mcpCall(t, mcpBin, env, nil, `{"name":"dnsfilter_execute","arguments":{"endpoint_id":"user-agents.uninstall-pin","params":{"organization_id":5},"confirm":true}}`)
	if n := v.hits.Load(); n != 0 {
		t.Errorf("%d requests reached the vendor", n)
	}
}

// TestChildGetsTheKeyOnStdin: an MCP mirror tool runs the CLI, and the kit starts
// no child with a held value in its environment. The key reaches the vendor
// through the stdin hand-off, and no child ever sees the MCP bearer.
func TestChildAuthenticatesWithTheEnvironmentKey(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const key = "df_live_child4142"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{"data":[],"links":{},"meta":{}}`) })
	home := t.TempDir()
	env := closedEnv(home, append(v.env(key), "DNSFILTER_CLI_PATH="+cliBin)...)
	out := mcpCall(t, mcpBin, env, nil, `{"name":"sync","arguments":{"resources":["policies"]}}`)
	assertAbsent(t, "sync", out, key, "not started", "withheld from child")
	if v.hits.Load() == 0 {
		t.Fatalf("the child reached no vendor endpoint:\n%s", out)
	}
	if got, _ := v.auth.Load().(string); !strings.Contains(got, key) {
		t.Errorf("the child sent Authorization %q, want the key", got)
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
	cmd.Env = closedEnv(t.TempDir(), "DNSFILTER_MCP_HTTP_TOKEN="+token)
	var serverOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &serverOut, &serverOut
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	post := func(auth string, origin string) (int, string) {
		var last error
		for i := 0; i < 50; i++ {
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
}

// TestDeclaredCredentialFlagsMatchTheCommands keeps CredentialFlags complete: a
// string flag named like a credential is declared, and a declared flag exists.
func TestDeclaredCredentialFlagsMatchTheCommands(t *testing.T) {
	declared := credwire.Declarations.CredentialFlags
	looks := func(name string) bool {
		for _, w := range []string{"password", "secret", "token"} {
			if strings.Contains(name, w) {
				return true
			}
		}
		return strings.HasSuffix(name, "-key") && !strings.HasSuffix(name, "entitlement-key")
	}
	seen := map[string]bool{}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		visit := func(f *pflag.Flag) {
			seen["--"+f.Name] = true
			if f.Value.Type() == "string" && looks(f.Name) && !slices.Contains(declared, "--"+f.Name) {
				t.Errorf("%s --%s looks like a credential and is not in credwire.Declarations.CredentialFlags", c.CommandPath(), f.Name)
			}
		}
		c.Flags().VisitAll(visit)
		c.PersistentFlags().VisitAll(visit)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(cli.RootCmd())
	for _, f := range declared {
		if !seen[f] {
			t.Errorf("credwire declares %s, which no command has", f)
		}
	}
}

func runStdin(t *testing.T, bin string, env []string, stdin string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run()
	return out.String()
}

// TestStdinTokenHeldBeforeFlagParse: a token on stdin is held before a flag error
// can echo it, and the command still gets it.
func TestStdinTokenHeldBeforeFlagParse(t *testing.T) {
	cliBin, _ := binaries(t)
	const short = "482915"
	home := t.TempDir()
	assertAbsent(t, "flag error", runStdin(t, cliBin, closedEnv(home), short, "auth", "set-token", "--timeout", short), short)

	const saved = "df-stdin-saved-7788"
	home = t.TempDir()
	if out := runStdin(t, cliBin, closedEnv(home), saved+"\n", "auth", "set-token"); strings.Contains(out, saved) {
		t.Errorf("set-token shows the token:\n%s", out)
	}
	found := false
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "credentials.toml" {
			data, _ := os.ReadFile(p)
			found = bytes.Contains(data, []byte(saved))
		}
		return nil
	})
	if !found {
		t.Error("set-token did not save the token it read from stdin")
	}
}

// TestLockFileCarriesNoHeldByte: the byte a lock file is made non-empty with goes
// through the kit, and a one-byte credential is held like any other.
func TestLockFileCarriesNoHeldByte(t *testing.T) {
	cliBin, _ := binaries(t)
	locks := func(home string) [][]byte {
		var out [][]byte
		_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(p, ".lock") {
				data, _ := os.ReadFile(p)
				out = append(out, data)
			}
			return nil
		})
		return out
	}
	home := t.TempDir()
	runStdin(t, cliBin, closedEnv(home), "\x00", "--no-learn", "auth", "set-token")
	for _, data := range locks(home) {
		if bytes.IndexByte(data, 0) >= 0 {
			t.Error("a lock file holds the one-byte credential")
		}
	}
	// With nothing held the lock still gets its byte.
	home = t.TempDir()
	runStdin(t, cliBin, closedEnv(home), "df-lock-token-9911\n", "--no-learn", "auth", "set-token")
	got := locks(home)
	if len(got) == 0 || len(got[0]) == 0 {
		t.Errorf("an ordinary set-token left lock files %q, want a non-empty one", got)
	}
}

// TestPresignedLinkIsTheAnswer: a download link in a url field is what the export
// command is for. It is no declared credential field, so it reaches the operator whole.
func TestPresignedLinkIsTheAnswer(t *testing.T) {
	cliBin, _ := binaries(t)
	const link = "https://bucket.s3.amazonaws.com/report.csv?Expires=1793664000&Signature=Zq81vPw3Mn57Rt"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"export":{"uuid":"exp1","url":%q}}`, link)
	})
	out := run(t, cliBin, closedEnv(t.TempDir(), v.env("df-presigned-key-5566")...), "unblock-requests-csv-export", "show", "exp1", "--json", "--data-source", "live")
	if !strings.Contains(out, "Zq81vPw3Mn57Rt") {
		t.Errorf("the download link was not returned:\n%s", out)
	}
}

// TestOriginIsTheConfigLoadReads: an unused config naming another host does not
// decide where the key may go.
func TestOriginIsTheConfigLoadReads(t *testing.T) {
	cliBin, _ := binaries(t)
	const key = "df-origin-key-3344"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"data":[]}`) })
	home := t.TempDir()
	unused := filepath.Join(home, "unused")
	writeFile(t, filepath.Join(unused, "config.toml"), "base_url = \"https://unused.example\"\n")
	tenant := filepath.Join(home, "tenant", "config.toml")
	writeFile(t, tenant, fmt.Sprintf("base_url = %q\napi_key = %q\n", v.url, key))
	env := closedEnv(home, "DNSFILTER_CONFIG_DIR="+unused, "DNSFILTER_CA_CERT="+v.caFile)
	out := run(t, cliBin, env, "--config", tenant, "policies", "all", "--data-source", "live", "--json")
	if v.hits.Load() == 0 || v.auth.Load() != key {
		t.Errorf("the configured API was not reached with the key (hits %d):\n%s", v.hits.Load(), out)
	}
}

// TestTLSKeyHeldBeforeAnythingPrints: the HTTP server's private key is a credential
// source, and a server error cannot echo a piece of it. An MCP child never opens
// the path, because its command line comes from a tool call.
func TestTLSKeyHeldBeforeAnythingPrints(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const piece = "QIBADANBgkqhkiG9"
	home := t.TempDir()
	key := filepath.Join(home, "server.key")
	writeFile(t, key, "-----BEGIN "+"PRIVATE KEY-----\nMIIEv"+piece+"w0BAQEFAASC\n-----END "+"PRIVATE KEY-----\n")
	assertAbsent(t, "MCP startup error", run(t, mcpBin, closedEnv(home), "--tls-key", key, "--transport", piece), piece)
	if out := run(t, cliBin, closedEnv(home, "DNSFILTER_MCP_CHILD=1"), "--timeout", piece, "--tls-key", key, "version"); !strings.Contains(out, piece) {
		t.Errorf("an MCP child held a file its tool-call command line named:\n%s", out)
	}
}

// TestUnresolvedPlaceholderIsNoCredential: an MCPB host that skips an optional key
// leaves its template text; the real key still works and nothing is withheld.
func TestUnresolvedPlaceholderIsNoCredential(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const key = "QxZ7vK2m" + "Lp9TgR4wYb6NcH3s"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"7","type":"policies","attributes":{"name":"Plain Policy"}}]}`)
	})
	env := closedEnv(t.TempDir(), append(v.env(key), "DNSFILTER_MCP_HTTP_TOKEN=${user_config.dnsfilter_mcp_http_token}")...)
	if out := run(t, cliBin, env, "policies", "all", "--data-source", "live", "--json"); !strings.Contains(out, "Plain Policy") {
		t.Errorf("CLI read:\n%s", out)
	}
	if out := mcpCall(t, mcpBin, env, nil, `{"name":"dnsfilter_read","arguments":{"endpoint_id":"policies.all"}}`); !strings.Contains(out, "Plain Policy") {
		t.Errorf("MCP read:\n%s", out)
	}
}

// TestStdinBodyHeldBeforeAPreview: a --stdin body is declared to the kit, so nothing
// that carries it prints: not a flag error, not a preview, not an answer that repeats
// it. The operator is told, the request still goes out, and the key is not lost.
func TestStdinBodyHeldBeforeAPreview(t *testing.T) {
	cliBin, _ := binaries(t)
	const long = "df-stdin-body-6622"
	body := "{\n  \"new_password\": \"" + long + "\"\n}\n"
	assertAbsent(t, "flag error", runStdin(t, cliBin, closedEnv(t.TempDir()), body, "users", "change-password", "--stdin", "--timeout", long), long)
	assertAbsent(t, "short preview", runStdin(t, cliBin, closedEnv(t.TempDir()), `{"new_password":"482915"}`, "users", "change-password", "--stdin", "--dry-run"), "482915")
	out := runStdin(t, cliBin, closedEnv(t.TempDir()), body, "users", "change-password", "--stdin", "--dry-run")
	assertAbsent(t, "preview", out, long)
	if !strings.Contains(out, "withheld") {
		t.Errorf("a withheld preview said nothing:\n%s", out)
	}
	plain := "{\n  \"name\": \"Acme Corp\",\n  \"enabled\": true\n}\n"
	if out := runStdin(t, cliBin, closedEnv(t.TempDir()), plain, "organizations", "create", "--stdin", "--dry-run"); !strings.Contains(out, "/v1/organizations") {
		t.Errorf("a pretty-printed body refused the start:\n%s", out)
	}
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":{"id":"1","type":"users","attributes":{"new_password":"`+long+`"}}}`)
	})
	out = runStdin(t, cliBin, closedEnv(t.TempDir(), v.env("df-body-key-1212")...), body, "users", "change-password", "--stdin", "--json")
	assertAbsent(t, "answer", out, long)
	if v.hits.Load() == 0 {
		t.Errorf("the request did not go out:\n%s", out)
	}
}

// TestFeedbackArgumentIsKept: free text given as an argument is no credential, so it is
// stored. Text on stdin goes through the declared --stdin flag, which the kit cannot
// scope to one command: it is withheld, and it says so.
func TestFeedbackArgumentIsKept(t *testing.T) {
	cliBin, _ := binaries(t)
	const text = "feedback text kept whole 5151"
	home := t.TempDir()
	run(t, cliBin, closedEnv(home), "feedback", text)
	found := false
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := os.ReadFile(p)
			found = found || bytes.Contains(data, []byte(text))
		}
		return nil
	})
	if !found {
		t.Error("feedback lost its text")
	}
}

// TestRunProfileStore: a profile value under a credential flag's name, each duplicate
// included, or under base-url is held at startup, and only a flag a command declares
// survives loading. Free text in a profile is no source.
func TestRunProfileStore(t *testing.T) {
	cliBin, _ := binaries(t)
	const pass, query, named, dup, kept = "ProfPass1234", "ProfQuery1234", "ProfNewPw1234", "ProfDupPw5678", "45s"
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"p":{"name":"p","values":{"timeout":"`+kept+`","mystery":"plain text","base-url":"https://u:`+pass+`@h.example/?x=`+query+`","new-password":"`+named+`","new-password":"`+dup+`"}}}}`)
	for _, secret := range []string{pass, query, named, dup} {
		assertAbsent(t, "flag error", run(t, cliBin, closedEnv(home), "--timeout", secret, "version"), secret)
	}
	out := run(t, cliBin, closedEnv(home), "profile", "show", "p", "--json")
	if strings.Contains(out, "mystery") || !strings.Contains(out, kept) {
		t.Errorf("profile show:\n%s", out)
	}
	home = t.TempDir()
	writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"p":{"values":{"new-password":"`+named+`",`)
	assertAbsent(t, "a store that does not parse", run(t, cliBin, closedEnv(home), "--timeout", named, "version"), named)
}

// TestStartupLoadIsSilent: startup reads a config without printing; a run's own Load
// warns, through the kit.
func TestStartupLoadIsSilent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the warning is about group and other mode bits")
	}
	cliBin, _ := binaries(t)
	home := t.TempDir()
	cfg := filepath.Join(home, "config", "config.toml")
	writeFile(t, cfg, "ca_cert = \"/tmp/ca.pem\"\n")
	if err := os.Chmod(cfg, 0o666); err != nil {
		t.Fatal(err)
	}
	out := run(t, cliBin, closedEnv(home), "--version") // loads no config of its own
	if strings.Contains(out, "ignoring ca_cert") {
		t.Errorf("the startup Load printed a warning:\n%s", out)
	}
}

// TestStdinHoldReadsAnyCommandLine: the raw command line decides, a superset of what the
// parser means: pflag takes "--" or a command's name as a flag's value and goes on.
func TestStdinHoldReadsAnyCommandLine(t *testing.T) {
	cliBin, _ := binaries(t)
	const short, long = "482915", "df-stdin-any-line-1234"
	body := `{"new_password":"` + long + `"}`
	for _, c := range []struct {
		stdin, secret string
		args          []string
	}{
		{short, short, []string{"--select", "--", "auth", "set-token", "--timeout", short}},
		{body, long, []string{"--select", "--", "users", "change-password", "--stdin", "--timeout", long}},
		{body, long, []string{"users", "change-password", "--select", "feedback", "--stdin", "--timeout", long}},
		{body, long, []string{"users", "change-password", "--stdin", "--select", "feedback", "--timeout", long}},
	} {
		assertAbsent(t, strings.Join(c.args, " "), runStdin(t, cliBin, closedEnv(t.TempDir()), c.stdin, c.args...), c.secret)
	}
}

// TestStdinBodyHeldInAnyForm: the kit holds the whole body and each line, parsed or not,
// so a credential of 8 or more characters is found in any of them.
func TestStdinBodyHeldInAnyForm(t *testing.T) {
	cliBin, _ := binaries(t)
	const long = "QxZ7vK2m" + "Lp9TgR4wYb6NcH3s"
	for _, body := range []string{
		`{"new_password":"` + long + `","new_password":null}`,
		`{"new_password":"` + long + `",`,
		`{"new_password":"` + long,
		`{'new_password':'` + long + `'}`,
		`{'unnamed':'` + long + `'}`,
		"{\"a\":1}\n{\"new_password\":\"" + long + "\"}",
		`{"password":{"value":"` + long + `"}}`,
		`new_password: ` + long,
	} {
		assertAbsent(t, body, runStdin(t, cliBin, closedEnv(t.TempDir()), body, "users", "change-password", "--stdin", "--timeout", long), long)
	}
}

// TestOriginFollowsTheSelectedConfig: DNSFILTER_CONFIG and --home choose the config the
// run reads, so its base URL is the allowed origin; another config's is only held.
func TestOriginFollowsTheSelectedConfig(t *testing.T) {
	cliBin, _ := binaries(t)
	const key = "df-origin-key-8899"
	for _, c := range []struct {
		name string
		env  func(tenant, tenantHome string) []string
		args func(tenant, tenantHome string) []string
	}{
		{"DNSFILTER_CONFIG", func(tenant, _ string) []string { return []string{"DNSFILTER_CONFIG=" + tenant} }, func(_, _ string) []string { return nil }},
		{"--home", func(_, _ string) []string { return nil }, func(_, tenantHome string) []string { return []string{"--home", tenantHome} }},
	} {
		t.Run(c.name, func(t *testing.T) {
			v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"data":[]}`) })
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "config", "config.toml"), "base_url = \"https://unused.example\"\n")
			tenantHome := t.TempDir()
			tenant := filepath.Join(tenantHome, "config", "config.toml")
			writeFile(t, tenant, fmt.Sprintf("base_url = %q\napi_key = %q\n", v.url, key))
			env := closedEnv(home, append(c.env(tenant, tenantHome), "DNSFILTER_CA_CERT="+v.caFile)...)
			args := append(c.args(tenant, tenantHome), "policies", "all", "--data-source", "live", "--json")
			out := run(t, cliBin, env, args...)
			if v.hits.Load() == 0 || v.auth.Load() != key {
				t.Errorf("the selected API was not reached with the key (hits %d):\n%s", v.hits.Load(), out)
			}
		})
	}
}

// TestChildMarkerLeavesTheServerKeyHeld: the marker is an environment value any
// caller can set; the server's own --tls-key is held regardless.
func TestChildMarkerLeavesTheServerKeyHeld(t *testing.T) {
	_, mcpBin := binaries(t)
	const piece = "QIBADANBgkqhkiG9"
	home := t.TempDir()
	key := filepath.Join(home, "server.key")
	writeFile(t, key, "-----BEGIN "+"PRIVATE KEY-----\nMIIEv"+piece+"w0BAQEFAASC\n-----END "+"PRIVATE KEY-----\n")
	assertAbsent(t, "server with the marker", run(t, mcpBin, closedEnv(home, "DNSFILTER_MCP_CHILD=1"), "--tls-key", key, "--transport", piece), piece)
}

// TestOnlyAManifestPlaceholderIsDropped: ${user_config...} is an MCPB host's template;
// any other declared secret value is held whatever it looks like.
func TestOnlyAManifestPlaceholderIsDropped(t *testing.T) {
	cliBin, _ := binaries(t)
	const other = "${foo}"
	assertAbsent(t, "other template", run(t, cliBin, closedEnv(t.TempDir(), "DNSFILTER_API_KEY="+other), "version", "--timeout", other), other)
}

// TestStartupLoadKeepsItsHandsOff: a run's own Load holds a config's other values under
// the running rule, so one that reads as public text refuses nothing, and its warnings
// print once.
func TestStartupLoadKeepsItsHandsOff(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	for _, file := range []string{"retries = 1\n", "note = \"id\"\n", "region = \"null\"\n", "[custom]\nx = \"data\"\n"} {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, "config", "config.toml"), file)
		if out := run(t, cliBin, closedEnv(home), "version"); !strings.Contains(out, "dnsfilter") {
			t.Errorf("config %q refused the start:\n%s", file, out)
		}
	}
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "config.toml"), "broken = = 1\n")
	out := mcpCall(t, mcpBin, closedEnv(home), nil, `{"name":"dnsfilter_read","arguments":{"endpoint_id":"nope"}}`)
	if !strings.Contains(out, `"id":2`) {
		t.Errorf("a config that does not parse ended the MCP session:\n%s", out)
	}
	// Neither DNSFILTER_HOME nor a real API: auth status reads paths and files only.
	offline := func(home string, extra ...string) []string {
		return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "NO_COLOR=1", "DNSFILTER_BASE_URL=http://127.0.0.1:1"}, extra...)
	}
	out = run(t, cliBin, offline(t.TempDir(), "XDG_CONFIG_HOME=rel/dir"), "auth", "status")
	if !strings.Contains(out, "ignoring XDG_CONFIG_HOME") {
		t.Errorf("the startup Load used up the path warning:\n%s", out)
	}
	if runtime.GOOS != "windows" {
		home = t.TempDir()
		cfg := filepath.Join(home, ".config", "dnsfilter-cli", "config.toml")
		writeFile(t, cfg, "api_key = \"df-plain-config-key-1234\"\n")
		if err := os.Chmod(cfg, 0o644); err != nil {
			t.Fatal(err)
		}
		out = run(t, cliBin, offline(home, "DNSFILTER_API_KEY=df-env-key-value-1234"), "auth", "status")
		if !strings.Contains(out, "present but refused") {
			t.Errorf("the startup Load used up the refusal:\n%s", out)
		}
	}
}

// TestProfileFreeTextRefusesNothing: a profile URL under another flag is no source, so a
// short query value in it refuses nothing, and a truncated store does not hold its keys.
func TestProfileFreeTextRefusesNothing(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"hook":{"name":"hook","values":{"deliver":"webhook:https://h.example/x?id=1"}}}}`)
	if out := run(t, cliBin, closedEnv(home), "version"); !strings.Contains(out, "dnsfilter") {
		t.Errorf("a profile URL refused the start:\n%s", out)
	}
	home = t.TempDir()
	writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"nightly":{"name":"nightly","description":"nightly-report","values":{"json":"true"`)
	if out := run(t, cliBin, closedEnv(home), "profile", "--help"); !strings.Contains(out, "profile") || strings.Contains(out, "withheld") {
		t.Errorf("a truncated store withheld help:\n%s", out)
	}
}
