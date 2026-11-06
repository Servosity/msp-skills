// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). A fixture value shares no 8-character piece with its test's
// name: t.TempDir() carries the name, and the kit rightly withholds a path that
// repeats a held piece. Token-shaped fixtures are split literals. Each input is
// one a review named. Set BETTERSTACK_LEAK_BIN_DIR to run them against binaries
// built elsewhere: that is how each was seen failing on the pre-kit build or on
// a build with credkit.Start removed.

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
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"betterstack-pp-cli/internal/cli"
	"betterstack-pp-cli/internal/cliutil"
	"betterstack-pp-cli/internal/config"
	"betterstack-pp-cli/internal/credkit"
	"betterstack-pp-cli/internal/credwire"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	binOnce sync.Once
	binDir  string
	binErr  error
)

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func binaries(t *testing.T) (cliBin, mcpBin string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("drives Unix config-file modes; the Windows build is vetted and compiled by the gates")
	}
	binOnce.Do(func() {
		if d := os.Getenv("BETTERSTACK_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "bs-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{{"betterstack-cli", "../../cmd/betterstack-cli"}, {"betterstack-mcp", "../../cmd/betterstack-mcp"}} {
			if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, exe(b.out)), b.pkg).CombinedOutput(); err != nil {
				binErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, exe("betterstack-cli")), filepath.Join(binDir, exe("betterstack-mcp"))
}

// vendor is a TLS stub of the Better Stack Uptime API: the kit never sends a
// credential over plain http, loopback included.
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
	v.url = srv.URL + "/api/v2"
	v.caFile = filepath.Join(t.TempDir(), "ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(v.caFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return v
}

func (v *vendor) env(token string) []string {
	return []string{"BETTERSTACK_BASE_URL=" + v.url, "BETTERSTACK_CA_CERT=" + v.caFile, "BETTERSTACK_API_TOKEN=" + token}
}

// page is a Better Stack list answer: next is the absolute next-page link, or
// "" on the last page.
func page(next string, items ...string) string {
	n := "null"
	if next != "" {
		n = fmt.Sprintf("%q", next)
	}
	return fmt.Sprintf(`{"data":[%s],"pagination":{"first":null,"last":null,"prev":null,"next":%s}}`, strings.Join(items, ","), n)
}

// monitor is one monitor record with the given attributes.
func monitor(attrs string) string {
	return `{"id":"2085431","type":"monitor","attributes":{` + attrs + `}}`
}

// closedEnv is a closed environment under one sandbox home.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "BETTERSTACK_HOME=" + home, "TMPDIR=" + os.TempDir(), "NO_COLOR=1"}, extra...)
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
	return runIn(t, bin, env, "", args...)
}

func runIn(t *testing.T, bin string, env []string, stdin string, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
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

// filesCarrying lists every file under root (but skip) whose bytes carry s.
func filesCarrying(root string, skip []string, s string) []string {
	var hits []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || slices.Contains(skip, p) {
			return nil
		}
		if data, rerr := os.ReadFile(p); rerr == nil && bytes.Contains(data, []byte(s)) {
			hits = append(hits, p)
		}
		return nil
	})
	return hits
}

func assertNoFileCarries(t *testing.T, home string, skip []string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		for _, p := range filesCarrying(home, skip, s) {
			t.Errorf("%s holds %q", p, s)
		}
	}
}

// mcpCall runs one MCP stdio session of tools/call params and returns stdout
// plus stderr, with every call answered.
func mcpCall(t *testing.T, bin string, env []string, calls ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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
		t.Errorf("MCP answered %d of %d calls:\n%s\n%s", answered, len(calls), out.String(), stderr.String())
	}
	return out.String() + stderr.String()
}

// monitorGet runs `monitors get 2085431 --data-source live` with the given
// extra flags against a vendor answering one monitor with attrs.
func monitorGet(t *testing.T, cliBin, attrs string, flags ...string) (string, *vendor) {
	t.Helper()
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/monitors/2085431") {
			io.WriteString(w, `{"data":`+monitor(attrs)+`}`)
			return
		}
		io.WriteString(w, page(""))
	})
	home := t.TempDir()
	args := append([]string{"monitors", "get", "2085431", "--data-source", "live"}, flags...)
	out := run(t, cliBin, closedEnv(home, v.env("Lq4vN8xT2m"+"R6pW9sK3bZ7c")...), append(args, "--no-learn")...)
	if v.hits.Load() == 0 {
		t.Fatalf("no request reached the vendor, so this run proves nothing:\n%s", out)
	}
	assertNoFileCarries(t, home, nil, attrsValues(attrs)...)
	return out, v
}

// attrsValues are the string values of a JSON attribute list, for the file
// checks.
func attrsValues(attrs string) []string {
	var m map[string]any
	if json.Unmarshal([]byte("{"+attrs+"}"), &m) != nil {
		return nil
	}
	var vals []string
	for _, v := range m {
		if s, ok := v.(string); ok && len(s) >= 8 {
			vals = append(vals, s)
		}
	}
	return vals
}

// TestP1PairIdentityInAnyCaseNeverPrints is print-review P1 input 1
// (credential-pair-identity-unheld): a monitor's basic-auth user named
// AUTHUSER, authuser or another case of a declared name is held as a
// credential, so it prints neither in the record nor echoed by a later field.
func TestP1PairIdentityInAnyCaseNeverPrints(t *testing.T) {
	cliBin, _ := binaries(t)
	const user, pw = "svc_customer_4f7d", "DifferentPass_9k8p"
	for _, name := range []string{"AUTHUSER", "authuser", "AuthUser", "Auth_User", "AUTH_USERNAME", "AuthPassword"} {
		t.Run(name, func(t *testing.T) {
			attrs := fmt.Sprintf(`%q:%q,"auth_password":%q`, name, user, pw)
			out, _ := monitorGet(t, cliBin, attrs, "--json")
			assertAbsent(t, "monitors get --json", out, user, pw)
			// Held against a later echo: another field repeating the user.
			echo := attrs + fmt.Sprintf(`,"pronounceable_name":%q`, user)
			out, _ = monitorGet(t, cliBin, echo, "--plain", "--select", "attributes.pronounceable_name")
			assertAbsent(t, "monitors get --select", out, user)
		})
	}
	// Control: the same name with no credential beside it prints, so the
	// --select path does print a field.
	out, _ := monitorGet(t, cliBin, `"pronounceable_name":"plain-name-control"`, "--plain", "--select", "attributes.pronounceable_name")
	if !strings.Contains(out, "plain-name-control") {
		t.Fatalf("the control name did not print, so this run proves nothing:\n%s", out)
	}
}

// TestP1ShortValueInsideOtherTextNeverPrints is print-review P1 input 2
// (short-held-value-inside-other-text): a two-character password K7 spelled as
// the hex escapes 0x4b0x37 inside a monitor's name. A run decoder's output
// counts a short value anywhere in it. (The other input, Q inside the word
// xQy, is outside contract 2(a), where a value under 8 counts only as a whole
// token: .review/followups.md.)
func TestP1ShortValueInsideOtherTextNeverPrints(t *testing.T) {
	cliBin, _ := binaries(t)
	const name = "x0x4b0x37y"
	out, _ := monitorGet(t, cliBin, `"auth_password":"K7","pronounceable_name":"`+name+`"`, "--plain", "--select", "attributes.pronounceable_name")
	assertAbsent(t, "monitors get --select", out, name)
	// Control: the same name beside another password prints.
	out, _ = monitorGet(t, cliBin, `"auth_password":"Zq","pronounceable_name":"`+name+`"`, "--plain", "--select", "attributes.pronounceable_name")
	if !strings.Contains(out, name) {
		t.Fatalf("the control name did not print, so this run proves nothing:\n%s", out)
	}
}

// TestP1NestedHeaderlessDeflateNeverPrints is print-review P1 input 3
// (secret-in-headerless-compressed-stream): a monitor's name that base64- and
// raw-DEFLATE-decodes to a 12-byte text which decodes the same way again to the
// held password.
func TestP1NestedHeaderlessDeflateNeverPrints(t *testing.T) {
	cliBin, _ := binaries(t)
	const name = "AQwA8/95MGpOeWNrSEFBPT0="
	out, _ := monitorGet(t, cliBin, `"auth_password":"hello","pronounceable_name":"`+name+`"`, "--plain", "--select", "attributes.pronounceable_name")
	assertAbsent(t, "monitors get --select", out, name)
	// Control: the same name beside another password prints.
	out, _ = monitorGet(t, cliBin, `"auth_password":"other","pronounceable_name":"`+name+`"`, "--plain", "--select", "attributes.pronounceable_name")
	if !strings.Contains(out, name) {
		t.Fatalf("the control name did not print, so this run proves nothing:\n%s", out)
	}
}

// TestP1JournalHalvesFromTwoProcesses: two processes at once, each recording
// one half of the held token in the learn journal (a recall query is journaled
// as its query family and unresolved entities). No file may carry either half.
// A control query proves the journal is written.
func TestP1JournalHalvesFromTwoProcesses(t *testing.T) {
	cliBin, _ := binaries(t)
	const held = "Zx9mQ4rT7wK2" + "pL5n3v8c1b6d"
	halves := []string{held[:12], held[12:]}
	home := t.TempDir()
	env := closedEnv(home, "BETTERSTACK_API_TOKEN="+held)
	for round := 0; round < 5; round++ {
		var wg sync.WaitGroup
		for _, h := range halves {
			wg.Add(1)
			go func(h string) {
				defer wg.Done()
				_, _ = runCode(t, cliBin, env, "recall", "find "+h)
			}(h)
		}
		wg.Wait()
	}
	assertNoFileCarries(t, home, nil, halves...)
	const control = "zebrafinch7"
	cout, _ := runCode(t, cliBin, env, "recall", "find "+control)
	if len(filesCarrying(filepath.Join(home, "state"), nil, control)) == 0 {
		t.Fatalf("the control query reached no journal file, so this run proves nothing:\n%s", cout)
	}
}

// TestP1ConfigRewriteAddsNoCopyOfTheURLPassword: BETTERSTACK_BASE_URL carries a
// password that a config.toml [headers] value repeats, and a config rewrite
// (auth set-token) runs. config.toml and the credential store are credential
// SOURCES, written outside the kit: the rewrite keeps the file's own header, as
// it was, and must add no copy of the password anywhere: not in config.toml's
// base_url (handfix config-file-holds-no-credential), not in any other file but
// the store, and not in any output.
func TestP1ConfigRewriteAddsNoCopyOfTheURLPassword(t *testing.T) {
	cliBin, _ := binaries(t)
	const pw = "UrlPw-93f8d1c2"
	home := t.TempDir()
	cfg := filepath.Join(home, "config", "config.toml")
	writeFile(t, cfg, "[headers]\nX-Extra = \""+pw+"\"\n")
	env := closedEnv(home, "BETTERSTACK_BASE_URL=https://ro:"+pw+"@uptime.betterstack.com/api/v2")
	out, code := runIn(t, cliBin, env, "fresh-key-6c1d9e4b"+"72a3f058\n", "auth", "set-token")
	if code != 0 {
		t.Fatalf("auth set-token: exit %d\n%s", code, out)
	}
	assertAbsent(t, "auth set-token", out, pw)
	data, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), pw); n > 1 {
		t.Errorf("config.toml carries the URL password %d times (it had it once, as its own header):\n%s", n, data)
	}
	if strings.Contains(string(data), "ro:") {
		t.Errorf("config.toml took BETTERSTACK_BASE_URL's userinfo:\n%s", data)
	}
	store := filepath.Join(home, "data", "credentials.toml")
	if _, err := os.Stat(store); err != nil {
		t.Fatalf("auth set-token wrote no credential store; the case did not run: %v", err)
	}
	assertNoFileCarries(t, home, []string{cfg, store}, pw)
	assertAbsent(t, "doctor", run(t, cliBin, env, "doctor", "--json", "--no-learn"), pw)
}

// TestExportSplitAcrossPagesNeverLandsWhole: page 1 ends with the first half of
// the token after ~12 KB of padding rows, page 2 starts with the second half.
// The export file never carries the token whole, nor both halves. A control
// export of the padding alone proves the file is written.
func TestExportSplitAcrossPagesNeverLandsWhole(t *testing.T) {
	cliBin, _ := binaries(t)
	const held = "Qm7xR2kW9pT4" + "-vN8bL3cJ6"
	first, second := held[:12], held[12:]
	var split atomic.Bool
	padding := make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		padding = append(padding, fmt.Sprintf(`{"id":"%d","type":"heartbeat_group","attributes":{"name":"Padding group %03d","sort_index":%d,"paused":false,"team_name":"%s"}}`, 1000+i, i, i, strings.Repeat("p", 40)))
	}
	var origin string
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/heartbeat-groups") {
			io.WriteString(w, page(""))
			return
		}
		switch r.URL.Query().Get("page") {
		case "", "1":
			if split.Load() {
				rows := append(slices.Clone(padding), `{"id":"1500","type":"heartbeat_group","attributes":{"name":"Tail","team_name":"`+first+`"}}`)
				io.WriteString(w, page(origin+r.URL.Path+"?page=2", rows...))
				return
			}
			io.WriteString(w, page("", padding...))
		case "2":
			if split.Load() {
				io.WriteString(w, page("", `{"id":"1501","type":"heartbeat_group","attributes":{"team_name":"`+second+`","name":"Head"}}`))
				return
			}
			io.WriteString(w, page(""))
		default:
			io.WriteString(w, page(""))
		}
	})
	origin = strings.TrimSuffix(v.url, "/api/v2")
	home := t.TempDir()
	env := closedEnv(home, v.env(held)...)
	control := filepath.Join(home, "control.jsonl")
	out := run(t, cliBin, env, "export", "heartbeat-groups", "--format", "jsonl", "--output", control, "--no-learn")
	if data, err := os.ReadFile(control); err != nil || !strings.Contains(string(data), "Padding group 099") {
		t.Fatalf("the control export wrote no rows (err %v), so this run proves nothing:\n%s", err, out)
	}
	split.Store(true)
	dest := filepath.Join(home, "export.jsonl")
	out = run(t, cliBin, env, "export", "heartbeat-groups", "--format", "jsonl", "--output", dest, "--no-learn")
	assertAbsent(t, "export", out, held)
	if data, err := os.ReadFile(dest); err == nil {
		joined := strings.NewReplacer("\n", "", "\"", "", ",", "", "{", "", "}", "", ":", "").Replace(string(data))
		if strings.Contains(string(data), held) || strings.Contains(joined, held) || (strings.Contains(string(data), first) && strings.Contains(string(data), second)) {
			t.Errorf("the export file carries the token across its records")
		}
	}
	assertNoFileCarries(t, home, nil, held)
}

// TestMalformedConfigNeverPrintsAtStartup: a flag error prints before any
// command loads config, from a config TOML rejects, in both binaries.
func TestMalformedConfigNeverPrintsAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	cases := []struct{ name, file, secret string }{
		{"comment-glued-to-quote", "api_token = \"uptime_glued_0101aa\"#longcomment\nbroken = = longenoughvalue\n", "uptime_glued_0101aa"},
		{"bare-value", "access_token = uptime_bare_2526bb\nbroken = = longenoughvalue\n", "uptime_bare_2526bb"},
		{"multi-line-string", "api_token = \"\"\"\nuptime_multi_1516cc\n\"\"\"\nbroken = = longenoughvalue\n", "uptime_multi_1516cc"},
		{"header-in-inline-table", "headers = { X-Extra = \"uptime_header_77dd\" }\nbroken = = longenoughvalue\n", "uptime_header_77dd"},
		{"unterminated-array", "extra = [\"filler-value\",\napi_token = \"uptime_after_1314ee\"\n", "uptime_after_1314ee"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "config", "config.toml"), c.file)
			env := closedEnv(home)
			out, _ := runCode(t, cliBin, env, "--timeout", c.secret, "version")
			assertAbsent(t, "CLI startup flag error", out, c.secret)
			out, _ = runCode(t, mcpBin, env, "--transport", c.secret)
			assertAbsent(t, "MCP startup flag error", out, c.secret)
		})
	}
}

// TestMCPHTTPWithoutTokenSaysWhy: --transport http with no
// BETTERSTACK_MCP_HTTP_TOKEN refuses to start, exits non-zero, and its stderr
// says why. An early exit that skips credkit.Exit drops the message: the kit's
// stderr pipe is never drained.
func TestMCPHTTPWithoutTokenSaysWhy(t *testing.T) {
	_, mcpBin := binaries(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, mcpBin, "--transport", "http", "--addr", "127.0.0.1:0")
	cmd.Env = closedEnv(t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	_ = cmd.Run()
	if code := cmd.ProcessState.ExitCode(); code == 0 {
		t.Errorf("exit 0, want a refusal")
	}
	if !strings.Contains(stderr.String(), "BETTERSTACK_MCP_HTTP_TOKEN") {
		t.Errorf("stderr does not name the missing token (stdout %q):\n%q", stdout.String(), stderr.String())
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
	cmd.Env = closedEnv(t.TempDir(), "BETTERSTACK_MCP_HTTP_TOKEN="+token)
	var serverOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &serverOut, &serverOut
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	post := func(auth, origin string) (int, string) {
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

// TestMonitorCredentialsNeverPrint: one monitor carries the probed site's
// basic-auth pair, another a request header under a name no built-in rule
// knows, and an escalation policy its incident_token (declared response
// fields; the declared request_headers container holds every leaf under it).
// Each record is read on its own, so each declaration is proven by itself.
// None reaches any output of either binary or any file; the store keeps the
// records with the credentials masked by name, so a sync completes.
func TestMonitorCredentialsNeverPrint(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const user, pw, header, incident = "probe-user-" + "4c1e9a7b", "Wm4xT9pL" + "2sQ8vR6n", "hdr-value-" + "8d3f2a6c71", "inc-" + "7b2d9f4e1a6c"
	basic := fmt.Sprintf(`{"id":"2085431","type":"monitor","attributes":{"pronounceable_name":"edge probe site","url":"https://status.example.net","auth_username":%q,"auth_password":%q}}`, user, pw)
	headers := fmt.Sprintf(`{"id":"2085432","type":"monitor","attributes":{"pronounceable_name":"inner probe site","url":"https://api.example.net","request_headers":[{"id":"406511","name":"X-Probe-Key","value":%q}]}}`, header)
	policy := fmt.Sprintf(`{"id":"77012","type":"policy","attributes":{"name":"night shift policy","repeat_count":3,"incident_token":%q}}`, incident)
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/monitors/2085431"):
			io.WriteString(w, `{"data":`+basic+`}`)
		case strings.HasSuffix(r.URL.Path, "/monitors/2085432"):
			io.WriteString(w, `{"data":`+headers+`}`)
		case strings.HasSuffix(r.URL.Path, "/monitors"):
			io.WriteString(w, page("", basic, headers))
		case strings.HasSuffix(r.URL.Path, "/policies"):
			io.WriteString(w, page("", policy))
		default:
			io.WriteString(w, page(""))
		}
	})
	home := t.TempDir()
	env := closedEnv(home, v.env("Hn3kP8vR"+"2wQ6xT9mZ4cB")...)
	secrets := []string{user, pw, header, incident}
	for _, id := range []string{"2085431", "2085432"} {
		assertAbsent(t, "monitors get "+id, run(t, cliBin, env, "monitors", "get", id, "--data-source", "live", "--json", "--no-learn"), secrets...)
		assertAbsent(t, "MCP monitors_get "+id, mcpCall(t, mcpBin, env, `{"name":"monitors_get","arguments":{"id":"`+id+`"}}`), secrets...)
	}
	assertAbsent(t, "policies list", run(t, cliBin, env, "policies", "list", "--data-source", "live", "--json", "--no-learn"), secrets...)
	syncOut := run(t, cliBin, env, "sync", "--resources", "monitors,policies", "--no-learn")
	assertAbsent(t, "sync", syncOut, secrets...)
	// The store keeps the records with their credentials masked by name, so the
	// kit's SQL driver lets the writes through (handfix store-never-keeps-secrets).
	for _, name := range []string{"edge probe site", "inner probe site", "night shift policy"} {
		if len(filesCarrying(home, nil, name)) == 0 {
			t.Errorf("sync stored no record named %q:\n%s", name, syncOut)
		}
	}
	assertNoFileCarries(t, home, nil, secrets...)
}

// TestStdinBodyIsHeldBeforeAPreview: a --stdin body is held whole before a dry
// run prints it, however the body carries the credential.
func TestStdinBodyIsHeldBeforeAPreview(t *testing.T) {
	const stdinHeld = "Vd2kR7mQ" + "4xT9pL3wN8cZ"
	cliBin, _ := binaries(t)
	const secret = "probe-stdin-password-4567"
	for name, body := range map[string]string{
		"field":     `{"url":"https://a.example","auth_password":"` + secret + `"}`,
		"case":      `{"url":"https://a.example","AUTH_PASSWORD":"` + secret + `"}`,
		"nested":    `{"url":"https://a.example","extra":{"value":"` + secret + `"}}`,
		"malformed": `{"url":"https://a.example","auth_password":"` + secret + `"`,
		"pretty":    "{\n  \"url\": \"https://a.example\",\n  \"auth_password\": \"" + secret + "\"\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			out, _ := runIn(t, cliBin, closedEnv(home, "BETTERSTACK_API_TOKEN="+stdinHeld), body, "monitors", "create", "--stdin", "--dry-run")
			assertAbsent(t, "--stdin --dry-run", out, secret)
			assertNoFileCarries(t, home, nil, secret)
		})
	}
}

// TestUsedConfigDecidesTheAllowedOrigin: the config the program really uses names
// the origin a credential may go to. A discovered default's base_url is only held,
// so a credential never goes there.
func TestUsedConfigDecidesTheAllowedOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	const held = "Ty6nW3qK" + "8vB2mR5xJ9dF"
	used := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, page("")) })
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "config.toml"), "base_url = \""+other.url+"\"\n")
	explicit := filepath.Join(home, "tenant.toml")
	writeFile(t, explicit, "base_url = \""+used.url+"\"\napi_token = \""+held+"\"\n")
	env := closedEnv(home, "BETTERSTACK_CA_CERT="+used.caFile)
	out := run(t, cliBin, env, "--config", explicit, "monitors", "list", "--data-source", "live", "--json", "--no-learn")
	assertAbsent(t, "monitors list", out, held)
	if used.hits.Load() == 0 {
		t.Errorf("the explicit config's base URL was refused or unused:\n%s", out)
	}
	if n := other.hits.Load(); n != 0 {
		t.Errorf("%d requests went to the discovered default's base URL", n)
	}
}

// TestStartupFlagTheParserDidNotUseIsRefused: startup reads --config and --home by
// every occurrence, and the kit takes the allowed origin from what it reads. A flag
// that takes a value can swallow one (--select --config x), so such a run stops
// before it acts.
func TestStartupFlagTheParserDidNotUseIsRefused(t *testing.T) {
	cliBin, _ := binaries(t)
	const held = "swallow-fixture-value-5c8e"
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var dials atomic.Int64
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			dials.Add(1)
			c.Close()
		}
	}()
	t.Cleanup(func() { ln.Close() })
	decoyURL := "https://" + ln.Addr().String()
	home := t.TempDir()
	usedFile := filepath.Join(home, "used.toml")
	decoyFile := filepath.Join(home, "decoy.toml")
	writeFile(t, usedFile, "base_url = \"https://uptime.betterstack.com/api/v2\"\n")
	writeFile(t, decoyFile, "base_url = \""+decoyURL+"\"\n")
	deliver := "webhook:" + decoyURL + "/hook"
	env := closedEnv(home, "BETTERSTACK_API_TOKEN="+held, "BETTERSTACK_USER_AGENT="+held, "HTTPS_PROXY=", "NO_PROXY=127.0.0.1")
	for name, args := range map[string][]string{
		"control-used-only": {"version", "--config", usedFile, "--deliver", deliver, "--no-learn"},
		"swallowed-last":    {"version", "--config", usedFile, "--select", "--config", decoyFile, "--deliver", deliver, "--no-learn"},
		"swallowed-only":    {"version", "--select", "--config", decoyFile, "--deliver", deliver, "--no-learn"},
		"swallowed-home":    {"version", "--select", "--home", filepath.Join(home, "other"), "--deliver", deliver, "--no-learn"},
	} {
		t.Run(name, func(t *testing.T) {
			before := dials.Load()
			out, _ := runCode(t, cliBin, env, args...)
			if n := dials.Load() - before; n != 0 {
				t.Errorf("%d connections carried the token to a host only an unused flag named:\n%s", n, out)
			}
			assertAbsent(t, "output", out, held)
		})
	}
	if out, code := runCode(t, cliBin, env, "--config", usedFile, "version", "--no-learn"); code != 0 {
		t.Errorf("an ordinary --config: exit %d\n%s", code, out)
	}
}

// TestMCPRefusesAConfigFlagItDoesNotHave: betterstack-mcp has no --config or
// --home, but startup reads every occurrence; one hidden from the parser
// refuses the start.
func TestMCPRefusesAConfigFlagItDoesNotHave(t *testing.T) {
	_, mcpBin := binaries(t)
	home := t.TempDir()
	used := filepath.Join(home, "used.toml")
	decoy := filepath.Join(home, "decoy.toml")
	writeFile(t, used, "base_url = \"https://uptime.betterstack.com/api/v2\"\n")
	writeFile(t, decoy, "base_url = \"https://collector.example\"\n")
	env := closedEnv(home, "BETTERSTACK_CONFIG="+used, "BETTERSTACK_API_TOKEN=mcp-fixture-value-2b7d")
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
	if out, code := runCode(t, mcpBin, env, "--transport", "stdio"); code != 0 {
		t.Errorf("an ordinary start: exit %d\n%s", code, out)
	}
}

// TestChildGetsTheKey: an MCP mirror tool runs the CLI, and the kit passes the
// child its credentials. The token reaches the vendor as the Bearer header.
func TestChildGetsTheKey(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const held = "Pk5wM8rT" + "3nX7vB2qL6dH"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, page("")) })
	home := t.TempDir()
	env := closedEnv(home, append(v.env(held), "BETTERSTACK_CLI_PATH="+cliBin)...)
	out := mcpCall(t, mcpBin, env, `{"name":"sync","arguments":{"resources":"monitors"}}`)
	assertAbsent(t, "sync", out, held, "not started", "withheld from child")
	if v.hits.Load() == 0 {
		t.Fatalf("the child reached no vendor endpoint:\n%s", out)
	}
	if got, _ := v.auth.Load().(string); got != "Bearer "+held {
		t.Errorf("the child sent Authorization %q, want the handed token", got)
	}
}

// TestAgentContextPrints: agent-context describes the CLI's own auth (env var
// names, a hint); with a real-shaped token set it still prints.
func TestAgentContextPrints(t *testing.T) {
	cliBin, _ := binaries(t)
	out := run(t, cliBin, closedEnv(t.TempDir(), "BETTERSTACK_API_TOKEN="+"Rb4nX9kW"+"2mT7qP5vC8sJ"), "agent-context", "--no-learn")
	var doc struct {
		Commands []any `json:"commands"`
	}
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&doc); err != nil || len(doc.Commands) == 0 {
		t.Errorf("agent-context is not the CLI's description (err %v):\n%.600s", err, out)
	}
}

// TestDeclaredCredentialFlagsMatchTheCommands keeps CredentialFlags complete: a
// string flag named like a credential by the one secret-name rule is declared, and
// a declared flag exists.
func TestDeclaredCredentialFlagsMatchTheCommands(t *testing.T) {
	declared := credwire.Declarations.CredentialFlags
	seen := map[string]bool{}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		visit := func(f *pflag.Flag) {
			seen["--"+f.Name] = true
			if f.Value.Type() == "string" && cliutil.SecretFieldName(f.Name) && !slices.Contains(declared, "--"+f.Name) {
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

// documentedMonitorCredentials are the monitor fields Better Stack documents
// but its OpenAPI spec omits: the probed site's basic-auth pair, in each
// separator spelling (the kit folds case only), and its request headers.
var documentedMonitorCredentials = []string{"auth_username", "authUsername", "auth_user", "authUser", "auth_password", "authPassword", "request_headers"}

// TestDeclarationsMatchTheSpec pins the declared credential fields against
// Better Stack's own spec names (cliutil/testdata/betterstack-spec-names.json,
// handfix spec-names-pinned) both ways: every spec name the secret-name rule
// reads as a credential, but the Authorization header the kit always holds,
// plus the documented monitor credentials, is a declared response field, MCP
// argument and credential field, and nothing else is. A documented name is no
// spec name and reads as a credential. A heartbeat's url is a spec name and
// never declared.
func TestDeclarationsMatchTheSpec(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "cliutil", "testdata", "betterstack-spec-names.json"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil || len(names) < 80 {
		t.Fatalf("pinned spec names: %d read, err %v", len(names), err)
	}
	var want []string
	for _, n := range names {
		if cliutil.SecretFieldName(n) && n != "Authorization" {
			want = append(want, n)
		}
	}
	for _, n := range documentedMonitorCredentials {
		if slices.Contains(names, n) || !cliutil.SecretFieldName(n) {
			t.Errorf("documented monitor credential %q: spec name %v, reads as a credential %v", n, slices.Contains(names, n), cliutil.SecretFieldName(n))
		}
		want = append(want, n)
	}
	slices.Sort(want)
	d := credwire.Declarations
	for label, got := range map[string][]string{"ResponseFields": d.ResponseFields, "MCPArguments": d.MCPArguments, "CredentialFields": d.CredentialFields} {
		got = slices.Clone(got)
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", label, got, want)
		}
	}
	if !slices.Contains(names, "url") || slices.Contains(d.ResponseFields, "url") || slices.Contains(d.CredentialFields, "url") {
		t.Errorf("url must be a spec name and never a declared credential")
	}
}

// TestEveryCredentialFileKeyIsClassified: a key config.Load reads is a declared
// credential, a declared identifier or the [headers] table; a new field cannot go
// unclassified.
func TestEveryCredentialFileKeyIsClassified(t *testing.T) {
	known := slices.Clone(credwire.Declarations.ConfigFields)
	for _, id := range credwire.Declarations.Identifiers {
		if k, ok := strings.CutPrefix(id.Source, "config:"); ok {
			known = append(known, k)
		}
	}
	for _, typ := range []reflect.Type{reflect.TypeOf(config.Config{}), reflect.TypeOf(cliutil.Credentials{})} {
		for i := 0; i < typ.NumField(); i++ {
			tag, _, _ := strings.Cut(typ.Field(i).Tag.Get("toml"), ",")
			if tag != "" && tag != "-" && !slices.Contains(known, tag) {
				t.Errorf("%s.%s: toml key %q is neither a declared ConfigField nor a config: Identifier", typ.Name(), typ.Field(i).Name, tag)
			}
		}
	}
}
