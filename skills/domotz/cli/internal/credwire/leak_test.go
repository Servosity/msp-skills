// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). A fixture value shares no 8-character piece with its test's
// name: t.TempDir() carries the name, and the kit rightly withholds a path that
// repeats a held piece. Token-shaped fixtures are split literals. Each input is
// one a review named. Set DOMOTZ_LEAK_BIN_DIR to run them against binaries
// built elsewhere: that is how each was seen failing on the pre-kit build.

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
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"domotz-pp-cli/internal/cli"
	"domotz-pp-cli/internal/cliutil"
	"domotz-pp-cli/internal/config"
	"domotz-pp-cli/internal/credkit"
	"domotz-pp-cli/internal/credwire"

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
		if d := os.Getenv("DOMOTZ_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "dz-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{{"domotz-cli", "../../cmd/domotz-cli"}, {"domotz-mcp", "../../cmd/domotz-mcp"}} {
			if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, exe(b.out)), b.pkg).CombinedOutput(); err != nil {
				binErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, exe("domotz-cli")), filepath.Join(binDir, exe("domotz-mcp"))
}

// vendor is a TLS stub of the Domotz Public API: the kit never sends a
// credential over plain http, loopback included.
type vendor struct {
	url, caFile string
	hits        atomic.Int64
	key         atomic.Value
}

func startVendor(t *testing.T, h http.HandlerFunc) *vendor {
	t.Helper()
	v := &vendor{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.hits.Add(1)
		v.key.Store(r.Header.Get("X-Api-Key"))
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	v.url = srv.URL + "/public-api/v1"
	v.caFile = filepath.Join(t.TempDir(), "ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(v.caFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return v
}

func (v *vendor) env(key string) []string {
	return []string{"DOMOTZ_BASE_URL=" + v.url, "DOMOTZ_CA_CERT=" + v.caFile, "DOMOTZ_API_KEY=" + key}
}

// closedEnv is a closed environment under one sandbox home.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "DOMOTZ_HOME=" + home,
		"TMPDIR=" + os.TempDir(), "NO_COLOR=1"}, extra...)
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

// mcpCall runs one MCP stdio session of tools/call params, one call at a time,
// and returns stdout plus stderr, with every call answered.
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
	var out strings.Builder
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	answered := 0
	for i, c := range calls {
		fmt.Fprintf(stdin, `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":%s}`+"\n", i+2, c)
		for sc.Scan() {
			out.WriteString(sc.Text() + "\n")
			var msg struct {
				ID *int `json:"id"`
			}
			if json.Unmarshal(sc.Bytes(), &msg) == nil && msg.ID != nil && *msg.ID == i+2 {
				answered++
				break
			}
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

// deviceVendor answers GET /agent/1/device/2 with device and every other path
// with an empty list.
func deviceVendor(t *testing.T, device string) *vendor {
	return startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/agent/1/device/2") {
			io.WriteString(w, device)
			return
		}
		io.WriteString(w, `[]`)
	})
}

// TestP1WhitespaceCommunityEncodedEchoNeverPrints is the open P1
// whitespace-credential-encoded-echo, its input as given: a device whose
// details carry the SNMP read community " \t " (whitespace only) and whose
// display_name is "IAkg", that community's base64. The selected name must not
// print from the CLI or from MCP. A control device whose community is another
// value prints the same name, so the select path does print a field (MCP reads
// the whole record, community included, so its control is withheld whole).
func TestP1WhitespaceCommunityEncodedEchoNeverPrints(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const key = "Lq4vN8xT2m" + "R6pW9sK3bZ7c"
	for _, c := range []struct {
		community string
		printed   bool
	}{{" \t ", false}, {"Zq", true}} {
		v := deviceVendor(t, `{"id":2,"protocol":"IP","display_name":"IAkg","details":{"snmp_read_community":`+fmt.Sprintf("%q", c.community)+`}}`)
		home := t.TempDir()
		env := closedEnv(home, append(v.env(key), "DOMOTZ_CLI_PATH="+cliBin)...)
		out := run(t, cliBin, env, "agent", "device", "get", "1", "2", "--select", "display_name", "--plain")
		if v.hits.Load() == 0 {
			t.Fatalf("no request reached the vendor, so this run proves nothing:\n%s", out)
		}
		mcp := mcpCall(t, mcpBin, env, `{"name":"domotz_read","arguments":{"endpoint_id":"agent.device.get","params":{"agent_id":"1","device_id":"2"}}}`)
		if strings.Contains(out, "IAkg") != c.printed {
			t.Errorf("community %q: agent device get --select display_name --plain printed IAkg %v, want %v:\n%.1500s", c.community, !c.printed, c.printed, out)
		}
		assertAbsent(t, "MCP domotz_read agent.device.get", mcp, "IAkg")
	}
}

// wsMember is a JSON member under a credential name whose value is only
// whitespace, raw or JSON-escaped, at any level of string nesting.
var wsMember = regexp.MustCompile(`"(?:read|write)\\*"\s*:\s*\\*"(?:\\*[tnr]|[ \t\r\n])+\\*"`)

// TestWhitespaceAndNumberArgumentsNeverPrint: an MCP argument under a
// credential name is held whatever it is made of (MCPArguments): a community
// of whitespace only and a community that is a number. Neither reaches the
// confirmation preview of domotz_execute, which masks what it can and is
// withheld whole when it cannot. Each runs in a session of its own (a held
// whitespace value withholds every later result of its session:
// .review/kit-findings.md); a control community proves the preview prints.
func TestWhitespaceAndNumberArgumentsNeverPrint(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	v := deviceVendor(t, `{}`)
	env := closedEnv(t.TempDir(), append(v.env("Fs8jK2nB"+"5wR7qM3xT6vY"), "DOMOTZ_CLI_PATH="+cliBin)...)
	call := func(params string) string {
		return mcpCall(t, mcpBin, env, `{"name":"domotz_execute","arguments":{"endpoint_id":"agent.device.set-snmp-community","params":{"agent_id":"1","device_id":"2",`+params+`}}}`)
	}
	if out := call(`"read":" \t "`); wsMember.MatchString(out) {
		t.Errorf("a preview shows the whitespace community: %s\n%.2000s", wsMember.FindString(out), out)
	}
	assertAbsent(t, "MCP preview", call(`"write":30571946`), "30571946")
	out := call(`"read":"pv-comm-6e2b9d4a"`)
	assertAbsent(t, "MCP preview", out, "pv-comm-6e2b9d4a")
	if !strings.Contains(out, "confirmation_required") || !strings.Contains(out, cliutil.RedactedValue) {
		t.Errorf("the control preview did not print masked, so this run proves nothing:\n%.2000s", out)
	}
	if v.hits.Load() != 0 {
		t.Errorf("a preview sent %d request(s)", v.hits.Load())
	}
}

// TestDeviceCredentialsNeverPrint: Domotz returns the credentials of the
// devices it monitors: an SNMP v3 user and its keys, a device's SNMP v1/v2
// communities, a custom driver association's username and password (declared
// response fields; the credentials container holds every leaf under it). None
// reaches any output of either binary or any file; the store keeps the
// records with the credentials masked by name, so a sync completes.
func TestDeviceCredentialsNeverPrint(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const user, authKey, encKey = "v3-user-" + "4c1e9a7b", "Wm4xT9pL" + "2sQ8vR6n", "Ek7qB3zN" + "9tY5hJ2m"
	const readCom, writeCom = "ro-comm-" + "8d3f2a6c", "rw-comm-" + "1b6e9d4f"
	const drvUser, drvPass = "drv-login-" + "5a2c7e9b", "Dp6nX4wK" + "8mR3vT1q"
	devices := fmt.Sprintf(`[{"id":2,"protocol":"IP","display_name":"edge switch alpha","status":"ONLINE","details":{"snmp_read_community":%q,"snmp_write_community":%q}}]`, readCom, writeCom)
	snmp := fmt.Sprintf(`{"version":"V3_AUTH_PRIV","username":%q,"authentication_protocol":"SHA","authentication_key":%q,"encryption_protocol":"AES","encryption_key":%q}`, user, authKey, encKey)
	assoc := fmt.Sprintf(`[{"id":77,"custom_driver_id":4321,"device_id":2,"status":"ENABLED","credentials":{"username":%q,"password":%q},"parameters":[]}]`, drvUser, drvPass)
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		switch p := strings.TrimPrefix(r.URL.Path, "/public-api/v1"); {
		case p == "/agent":
			io.WriteString(w, `[{"id":1,"display_name":"north collector","status":{"value":"ONLINE"}}]`)
		case p == "/agent/1/device":
			io.WriteString(w, devices)
		case p == "/agent/1/device/2/snmp-authentication":
			io.WriteString(w, snmp)
		case p == "/custom-driver/agent/1/association":
			io.WriteString(w, assoc)
		default:
			io.WriteString(w, `[]`)
		}
	})
	home := t.TempDir()
	env := closedEnv(home, append(v.env("Hn3kP8vR"+"2wQ6xT9mZ4cB"), "DOMOTZ_CLI_PATH="+cliBin)...)
	secrets := []string{user, authKey, encKey, readCom, writeCom, drvUser, drvPass}
	for _, args := range [][]string{
		{"agent", "device", "list", "1"},
		{"agent", "device", "get-snmpauthentication", "1", "2"},
		{"custom-driver", "list-associations", "1"},
	} {
		label := strings.Join(args, " ")
		assertAbsent(t, label, run(t, cliBin, env, append(args, "--data-source", "live", "--json", "--no-learn")...), secrets...)
	}
	assertAbsent(t, "MCP reads", mcpCall(t, mcpBin, env,
		`{"name":"domotz_read","arguments":{"endpoint_id":"agent.device.list","params":{"agent_id":"1"}}}`,
		`{"name":"domotz_read","arguments":{"endpoint_id":"agent.device.get-snmpauthentication","params":{"agent_id":"1","device_id":"2"}}}`,
		`{"name":"domotz_read","arguments":{"endpoint_id":"custom-driver.list-associations","params":{"agent_id":"1"}}}`,
	), secrets...)
	syncOut := run(t, cliBin, env, "sync", "--resources", "agent", "--no-learn")
	assertAbsent(t, "sync", syncOut, secrets...)
	// The store keeps the device with its communities masked by name, so the
	// kit's SQL driver lets the write through (handfix store-never-keeps-secrets).
	if len(filesCarrying(home, nil, "edge switch alpha")) == 0 {
		t.Errorf("sync stored no device record:\n%s", syncOut)
	}
	assertNoFileCarries(t, home, nil, secrets...)
}

// TestConfigRewriteAddsNoCopyOfTheURLPassword: DOMOTZ_BASE_URL carries a
// password that a config.toml [headers] value repeats, and a config rewrite
// (auth set-token) runs. config.toml and the credential store are credential
// SOURCES, written outside the kit: the rewrite keeps the file's own header, as
// it was, and must add no copy of the password anywhere: not in config.toml's
// base_url (handfix config-file-holds-no-credential), not in any other file but
// the store, and not in any output.
func TestConfigRewriteAddsNoCopyOfTheURLPassword(t *testing.T) {
	cliBin, _ := binaries(t)
	const pw = "UrlPw-93f8d1c2"
	home := t.TempDir()
	cfg := filepath.Join(home, "config", "config.toml")
	writeFile(t, cfg, "[headers]\nX-Extra = \""+pw+"\"\n")
	env := closedEnv(home, "DOMOTZ_BASE_URL=https://ro:"+pw+"@api-eu-west-1-cell-1.domotz.com/public-api/v1")
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
		t.Errorf("config.toml took DOMOTZ_BASE_URL's userinfo:\n%s", data)
	}
	store := filepath.Join(home, "data", "credentials.toml")
	if _, err := os.Stat(store); err != nil {
		t.Fatalf("auth set-token wrote no credential store; the case did not run: %v", err)
	}
	assertNoFileCarries(t, home, []string{cfg, store}, pw)
	assertAbsent(t, "doctor", run(t, cliBin, env, "doctor", "--json", "--no-learn"), pw)
}

// TestMalformedConfigNeverPrintsAtStartup: a flag error prints before any
// command loads config, from a config TOML rejects, in both binaries.
func TestMalformedConfigNeverPrintsAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	cases := []struct{ name, file, secret string }{
		{"comment-glued-to-quote", "api_key = \"netmon_glued_0101aa\"#longcomment\nbroken = = longenoughvalue\n", "netmon_glued_0101aa"},
		{"bare-value", "access_token = netmon_bare_2526bb\nbroken = = longenoughvalue\n", "netmon_bare_2526bb"},
		{"multi-line-string", "api_key = \"\"\"\nnetmon_multi_1516cc\n\"\"\"\nbroken = = longenoughvalue\n", "netmon_multi_1516cc"},
		{"header-in-inline-table", "headers = { X-Extra = \"netmon_header_77dd\" }\nbroken = = longenoughvalue\n", "netmon_header_77dd"},
		{"unterminated-array", "extra = [\"filler-value\",\napi_key = \"netmon_after_1314ee\"\n", "netmon_after_1314ee"},
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
// DOMOTZ_MCP_HTTP_TOKEN refuses to start, exits non-zero, and its stderr says
// why. An early exit that skips credkit.Exit drops the message: the kit's
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
	if !strings.Contains(stderr.String(), "DOMOTZ_MCP_HTTP_TOKEN") {
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
	cmd.Env = closedEnv(t.TempDir(), "DOMOTZ_MCP_HTTP_TOKEN="+token)
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

// TestStdinBodyIsHeldBeforeAPreview: a --stdin body is held whole before a dry
// run prints it, however the body carries the credential (stdin-body-declared).
func TestStdinBodyIsHeldBeforeAPreview(t *testing.T) {
	const stdinHeld = "Vd2kR7mQ" + "4xT9pL3wN8cZ"
	cliBin, _ := binaries(t)
	const secret = "probe-stdin-password-4567"
	for name, body := range map[string]string{
		"field":     `{"username":"hq-admin","password":"` + secret + `"}`,
		"case":      `{"username":"hq-admin","PASSWORD":"` + secret + `"}`,
		"nested":    `{"username":"hq-admin","extra":{"value":"` + secret + `"}}`,
		"malformed": `{"username":"hq-admin","password":"` + secret + `"`,
		"pretty":    "{\n  \"username\": \"hq-admin\",\n  \"password\": \"" + secret + "\"\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			out, _ := runIn(t, cliBin, closedEnv(home, "DOMOTZ_API_KEY="+stdinHeld), body, "agent", "device", "set-credentials", "1", "2", "--stdin", "--dry-run")
			assertAbsent(t, "--stdin --dry-run", out, secret)
			assertNoFileCarries(t, home, nil, secret)
		})
	}
}

// TestUsedConfigDecidesTheAllowedOrigin: the config the program really uses
// names the origin a credential may go to. A discovered default's base_url is
// only held, so a credential never goes there (used-config-decides-origin).
func TestUsedConfigDecidesTheAllowedOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	const held = "Ty6nW3qK" + "8vB2mR5xJ9dF"
	used := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `[]`) })
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `[]`) })
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "config.toml"), "base_url = \""+other.url+"\"\n")
	explicit := filepath.Join(home, "tenant.toml")
	writeFile(t, explicit, "base_url = \""+used.url+"\"\napi_key = \""+held+"\"\n")
	env := closedEnv(home, "DOMOTZ_CA_CERT="+used.caFile)
	out := run(t, cliBin, env, "--config", explicit, "agent", "list", "--data-source", "live", "--json", "--no-learn")
	assertAbsent(t, "agent list", out, held)
	if used.hits.Load() == 0 {
		t.Errorf("the explicit config's base URL was refused or unused:\n%s", out)
	}
	if n := other.hits.Load(); n != 0 {
		t.Errorf("%d requests went to the discovered default's base URL", n)
	}
}

// connectTrap is an HTTP proxy that records each CONNECT target and refuses
// it, so a request to a real Domotz host is seen leaving and never arrives.
func connectTrap(t *testing.T) (proxyURL string, targets func() []string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var mu sync.Mutex
	var seen []string
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(10 * time.Second))
				line, _ := bufio.NewReader(c).ReadString('\n')
				mu.Lock()
				seen = append(seen, strings.TrimSpace(line))
				mu.Unlock()
				io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
			}(c)
		}
	}()
	return "http://" + ln.Addr().String(), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(seen)
	}
}

// TestRegionDecidesTheAllowedOrigin: the base URL is the template
// https://api-{region}.domotz.com/public-api/v1, and the kit allows the one
// host this run resolves it to (used-config-decides-origin): the default
// region's host and DOMOTZ_REGION's host each receive the key's request (seen
// as the CONNECT a proxy trap records), and a region that is not one DNS label
// sends nothing anywhere.
func TestRegionDecidesTheAllowedOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	const held = "Rg5mT8wQ" + "3kV7nB2xL6pJ"
	for _, c := range []struct{ name, region, host string }{
		{"default", "", "api-us-east-1-cell-1.domotz.com:443"},
		{"env", "eu-west-1-cell-1", "api-eu-west-1-cell-1.domotz.com:443"},
		{"not-a-label", "evil.example.com/x?", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			proxy, targets := connectTrap(t)
			extra := []string{"DOMOTZ_API_KEY=" + held, "HTTPS_PROXY=" + proxy, "NO_PROXY=", "no_proxy="}
			if c.region != "" {
				extra = append(extra, "DOMOTZ_REGION="+c.region)
			}
			out, _ := runCode(t, cliBin, closedEnv(t.TempDir(), extra...), "agent", "list", "--data-source", "live", "--json", "--no-learn")
			assertAbsent(t, "agent list", out, held)
			got := targets()
			switch {
			case c.host == "" && len(got) != 0:
				t.Errorf("DOMOTZ_REGION=%q: the request left for %v", c.region, got)
			case c.host != "" && (len(got) == 0 || !strings.Contains(got[0], c.host)):
				t.Errorf("want the request to leave for %s, the proxy saw %v:\n%.1500s", c.host, got, out)
			}
			if c.host != "" && strings.Contains(out, "withheld from outbound request") {
				t.Errorf("the kit refused the resolved region's host:\n%.1500s", out)
			}
		})
	}
}

// TestStartupFlagTheParserDidNotUseIsRefused: startup reads --config and --home
// by every occurrence, and the kit takes the allowed origin from what it reads.
// A flag that takes a value can swallow one (--select --config x), so such a
// run stops before it acts.
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
	writeFile(t, usedFile, "base_url = \"https://api-eu-west-1-cell-1.domotz.com/public-api/v1\"\n")
	writeFile(t, decoyFile, "base_url = \""+decoyURL+"\"\n")
	deliver := "webhook:" + decoyURL + "/hook"
	env := closedEnv(home, "DOMOTZ_API_KEY="+held, "DOMOTZ_USER_AGENT="+held, "HTTPS_PROXY=", "NO_PROXY=127.0.0.1")
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
				t.Errorf("%d connections carried the key to a host only an unused flag named:\n%s", n, out)
			}
			assertAbsent(t, "output", out, held)
		})
	}
	if out, code := runCode(t, cliBin, env, "--config", usedFile, "version", "--no-learn"); code != 0 {
		t.Errorf("an ordinary --config: exit %d\n%s", code, out)
	}
}

// TestMCPRefusesAConfigFlagItDoesNotHave: domotz-mcp has no --config or
// --home, but startup reads every occurrence; one hidden from the parser
// refuses the start.
func TestMCPRefusesAConfigFlagItDoesNotHave(t *testing.T) {
	_, mcpBin := binaries(t)
	home := t.TempDir()
	used := filepath.Join(home, "used.toml")
	decoy := filepath.Join(home, "decoy.toml")
	writeFile(t, used, "base_url = \"https://api-eu-west-1-cell-1.domotz.com/public-api/v1\"\n")
	writeFile(t, decoy, "base_url = \"https://collector.example\"\n")
	env := closedEnv(home, "DOMOTZ_CONFIG="+used, "DOMOTZ_API_KEY=mcp-fixture-value-2b7d")
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
// child its credentials. The key reaches the vendor as the X-Api-Key header.
func TestChildGetsTheKey(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const held = "Pk5wM8rT" + "3nX7vB2qL6dH"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `[]`) })
	home := t.TempDir()
	env := closedEnv(home, append(v.env(held), "DOMOTZ_CLI_PATH="+cliBin)...)
	out := mcpCall(t, mcpBin, env, `{"name":"sync","arguments":{"resources":"agent"}}`)
	assertAbsent(t, "sync", out, held, "not started", "withheld from child")
	if v.hits.Load() == 0 {
		t.Fatalf("the child reached no vendor endpoint:\n%s", out)
	}
	if got, _ := v.key.Load().(string); got != held {
		t.Errorf("the child sent X-Api-Key %q, want the handed key", got)
	}
}

// TestAgentContextPrints: agent-context describes the CLI's own auth (env var
// names, a hint); with a real-shaped key set it still prints.
func TestAgentContextPrints(t *testing.T) {
	cliBin, _ := binaries(t)
	out := run(t, cliBin, closedEnv(t.TempDir(), "DOMOTZ_API_KEY="+"Rb4nX9kW"+"2mT7qP5vC8sJ"), "agent-context", "--no-learn")
	var doc struct {
		Commands []any `json:"commands"`
	}
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&doc); err != nil || len(doc.Commands) == 0 {
		t.Errorf("agent-context is not the CLI's description (err %v):\n%.600s", err, out)
	}
}

// TestP1JournalHalvesFromTwoProcesses: two processes at once, each recording
// one half of the held key in the learn journal (a recall query is journaled
// as its query family and unresolved entities). No file may carry either half.
// A control query proves the journal is written.
func TestP1JournalHalvesFromTwoProcesses(t *testing.T) {
	cliBin, _ := binaries(t)
	const held = "Zx9mQ4rT7wK2" + "pL5n3v8c1b6d"
	halves := []string{held[:12], held[12:]}
	home := t.TempDir()
	env := closedEnv(home, "DOMOTZ_API_KEY="+held)
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

// TestDeclaredCredentialFlagsMatchTheCommands keeps CredentialFlags complete: a
// string flag named like a credential by the one secret-name rule is declared,
// and a declared flag exists.
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

// TestDeclarationsMatchTheSpec pins the declared credential fields against
// Domotz's own spec names (cliutil/testdata/domotz-spec-names.json, handfix
// domotz-credential-names-exact) both ways: every spec name the secret-name
// rule reads as a credential, but the X-Api-Key header the kit holds as a
// CredentialHeader, is a declared response field and credential field, and
// nothing else is. MCP arguments are those names plus each credential flag's
// spelling, since the MCP mirror names a tool argument after its flag. Product
// data under credential-like names (an inventory field's key, an activity
// log's user, a device's authentication_status) is a spec name and never
// declared.
func TestDeclarationsMatchTheSpec(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "cliutil", "testdata", "domotz-spec-names.json"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil || len(names) < 80 {
		t.Fatalf("pinned spec names: %d read, err %v", len(names), err)
	}
	var want []string
	for _, n := range names {
		if cliutil.SecretFieldName(n) && n != "X-Api-Key" {
			want = append(want, n)
		}
	}
	slices.Sort(want)
	d := credwire.Declarations
	for label, got := range map[string][]string{"ResponseFields": d.ResponseFields, "CredentialFields": d.CredentialFields} {
		got = slices.Clone(got)
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", label, got, want)
		}
	}
	wantMCP := slices.Clone(want)
	for _, f := range d.CredentialFlags {
		if n := strings.TrimPrefix(f, "--"); !slices.Contains(wantMCP, n) {
			wantMCP = append(wantMCP, n)
		}
	}
	slices.Sort(wantMCP)
	got := slices.Clone(d.MCPArguments)
	slices.Sort(got)
	if !slices.Equal(got, wantMCP) {
		t.Errorf("MCPArguments = %v, want %v", got, wantMCP)
	}
	for _, public := range []string{"key", "user", "authentication_status", "authentication_protocol", "requires_credentials"} {
		if !slices.Contains(names, public) || slices.Contains(d.ResponseFields, public) || slices.Contains(d.MCPArguments, public) {
			t.Errorf("%s must be a spec name and never a declared credential", public)
		}
	}
	if !slices.Contains(d.CredentialHeaders, "X-Api-Key") {
		t.Errorf("X-Api-Key is not a declared credential header")
	}
}

// TestEveryCredentialFileKeyIsClassified: a key config.Load reads is a declared
// credential, a declared identifier or the [headers] table; a new field cannot
// go unclassified.
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
