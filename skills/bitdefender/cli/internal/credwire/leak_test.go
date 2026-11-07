// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class's failing input, seen
// failing on the binaries built before the kit was wired. Set
// BITDEFENDER_LEAK_BIN_DIR to run them against binaries built elsewhere.

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

	"bitdefender-pp-cli/internal/credkit"
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
		if d := os.Getenv("BITDEFENDER_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "bitdefender-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{exe("bitdefender-cli"), "../../cmd/bitdefender-cli"},
			{exe("bitdefender-mcp"), "../../cmd/bitdefender-mcp"},
		} {
			if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, b.out), b.pkg).CombinedOutput(); err != nil {
				binErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, exe("bitdefender-cli")), filepath.Join(binDir, exe("bitdefender-mcp"))
}

// vendor is a TLS stub of the GravityZone JSON-RPC API: the kit never sends a
// credential over plain http, loopback included. results maps a JSON-RPC
// method to its result member; a method it does not name answers {}.
type vendor struct {
	url, caFile string
	hits        atomic.Int64
	conns       atomic.Int64
}

type rpcCall struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	ID     json.RawMessage `json:"id"`
}

func startVendor(t *testing.T, results map[string]string) *vendor {
	t.Helper()
	return startTLS(t, func(w http.ResponseWriter, r *http.Request) {
		var c rpcCall
		_ = json.NewDecoder(r.Body).Decode(&c)
		if len(c.ID) == 0 {
			c.ID = json.RawMessage(`"1"`)
		}
		result, ok := results[c.Method]
		if !ok {
			result = `{}`
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":%s}`, c.ID, result)
	})
}

func startTLS(t *testing.T, h http.HandlerFunc) *vendor {
	t.Helper()
	v := &vendor{}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}))
	// A connection is counted before TLS: a refused request never dials.
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			v.conns.Add(1)
		}
	}
	srv.StartTLS()
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
	return []string{"BITDEFENDER_BASE_URL=" + v.url + "/api", "BITDEFENDER_CA_CERT=" + v.caFile, "BITDEFENDER_API_KEY=" + key}
}

// closedEnv is a closed environment under one sandbox home; every press
// directory resolves under it.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "BITDEFENDER_HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg-config"), "XDG_DATA_HOME=" + filepath.Join(home, "xdg-data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "xdg-state"), "XDG_CACHE_HOME=" + filepath.Join(home, "xdg-cache"),
		"TMPDIR=" + os.TempDir(), "NO_COLOR=1"}, extra...)
}

func configDir(home string) string { return filepath.Join(home, "config") }

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
	return runStdinCode(t, bin, env, nil, args...)
}

func runStdinCode(t *testing.T, bin string, env []string, stdin io.Reader, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = stdin
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
			if bytes.Contains(data, []byte(s)) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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

// TestReadResultCredentialFieldsWithheld (kit note 1): a read result that
// carries a credential the vendor returns (a license key, a push receiver's
// authorization header, an uninstall password, a policy's power-user, proxy
// and password-protection passwords and its SIEM key, an inventory company's license key, an integration's
// password) reached the CLI's stdout and the bitdefender_read MCP result
// unchanged. ResponseFields holds each value as the response streams in, so
// the output that carries it is withheld.
func TestReadResultCredentialFieldsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const key = "bdkey" + "Q7m2Xv9LpR4t"
	const policy = "55828d66b1a43de92c712345"
	policyCLI := []string{"policies", "get", "--policy-id", policy}
	policyMCP := `{"endpoint_id":"policies.get","params":{"policyId":"` + policy + `"}}`
	// Each response carries one credential, so each case proves its own field.
	cases := []struct {
		name, secret, method, result string
		cli                          []string
		mcp                          string
	}{
		{"license.get", "LKEY" + "-Z8QW-4N7P-2XKD", "getLicenseInfo",
			`{"isAddon":false,"expiryDate":"2027-01-01T00:00:00","usedSlots":3,"totalSlots":10,"licenseKey":"%s"}`,
			[]string{"license", "get"}, `{"endpoint_id":"license.get","params":{}}`},
		{"push.get", "pushTok" + "9Hc3Lw2Zq", "getPushEventSettings",
			`{"status":1,"serviceType":"jsonRPC","serviceSettings":{"url":"https://siem.acme.example/hook","requireValidSslCertificate":true,"authorization":"Bearer %s"}}`,
			[]string{"push", "get"}, `{"endpoint_id":"push.get","params":{}}`},
		{"packages.get", "unPw" + "Fq83Kd0Lz2", "getPackageDetails",
			`{"packageName":"acme-default","language":"en_US","settings":{"uninstallPassword":"%s","scanMode":{"type":1}}}`,
			[]string{"packages", "get", "--package-id", "5a37b660b1a43d99117b23c6"}, `{"endpoint_id":"packages.get","params":{"packageId":"5a37b660b1a43d99117b23c6"}}`},
		{"policies.get power user", "puPw" + "Mx72Nq4Rb9", "getPolicyDetails",
			`{"id":"p1","name":"Default","settings":{"general":{"settings":{"powerUser":{"enabled":true,"password":"%s"}}}}}`,
			policyCLI, policyMCP},
		{"policies.get proxy", "pxPw" + "Hd61Ys3Tc8", "getPolicyDetails",
			`{"id":"p1","name":"Default","settings":{"update":{"proxy":{"profile":1,"settings":{"server":"proxy.acme.example","port":8080,"username":"svc","password":"%s"}}}}}`,
			policyCLI, policyMCP},
		{"policies.get password protection", "pcVal" + "Wn48Jr2Ke7", "getPolicyDetails",
			`{"id":"p1","name":"Default","settings":{"general":{"settings":{"passwordConfig":{"profile":1,"value":"%s"}}}}}`,
			policyCLI, policyMCP},
		{"policies.get SIEM key", "siemK" + "Tp93Gx5Hm1", "getPolicyDetails",
			`{"id":"p1","name":"Default","settings":{"general":{"securityTelemetry":{"enabled":true,"siem":1,"proxy":{"profile":3},"config":{"url":"https://siem.acme.example","key":"%s"}}}}}`,
			policyCLI, policyMCP},
		{"inventory.list", "INV" + "-K4R7-W2PZ-9QJM", "getNetworkInventoryItems",
			`{"page":1,"pagesCount":1,"perPage":30,"total":1,"items":[{"id":"c1","name":"Acme","type":1,"details":{"licenseKey":"%s"}}]}`,
			[]string{"inventory", "list"}, `{"endpoint_id":"inventory.list","params":{}}`},
		{"integrations.get", "itgPw" + "Vb5Kx8Lq3", "getIntegrationDetails",
			`{"id":"i1","integrationType":"connectwise","specifics":{"url":"https://psa.acme.example","username":"svc","password":"%s"}}`,
			[]string{"integrations", "get", "--integration-id", "678f89ef8fe5bb14010b3242"}, `{"endpoint_id":"integrations.get","params":{"integrationId":"678f89ef8fe5bb14010b3242"}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := startVendor(t, map[string]string{c.method: fmt.Sprintf(c.result, c.secret)})
			home := t.TempDir()
			env := closedEnv(home, v.env(key)...)
			assertAbsent(t, "CLI "+strings.Join(c.cli, " "), run(t, cliBin, env, c.cli...), c.secret)
			assertAbsent(t, "CLI --json", run(t, cliBin, env, append(c.cli, "--json")...), c.secret)
			assertAbsent(t, "MCP bitdefender_read", mcpCall(t, mcpBin, env, `{"name":"bitdefender_read","arguments":`+c.mcp+`}`), c.secret)
			if v.hits.Load() == 0 {
				t.Error("no request reached the API stub, so nothing was returned")
			}
			assertNoFileCarries(t, home, nil, c.secret)
		})
	}
}

// TestConfirmPreviewDoesNotEchoCredentialParams (kit note 2): bitdefender_execute
// without confirm answers a preview of the request body, which carried the
// caller's password verbatim. MCPArguments holds it before the handler runs.
func TestConfirmPreviewDoesNotEchoCredentialParams(t *testing.T) {
	_, mcpBin := binaries(t)
	const key = "bdkey" + "Prev1ew4Kq8z"
	const password = "acctPw" + "Rz7Qm2Xk9!"
	v := startVendor(t, nil)
	home := t.TempDir()
	env := closedEnv(home, v.env(key)...)
	out := mcpCall(t, mcpBin, env,
		`{"name":"bitdefender_execute","arguments":{"endpoint_id":"accounts.create","params":{"email":"tech@acme.example","userName":"tech","password":"`+password+`","role":1}}}`)
	assertAbsent(t, "accounts.create preview", out, password)
	if n := v.hits.Load(); n != 0 {
		t.Errorf("the preview sent %d request(s) without confirm", n)
	}
	assertNoFileCarries(t, home, nil, password)
}

// TestJournalNeverRecordsAHeldCredential (learn journal): the journal records
// the session label (BITDEFENDER_LEARN_SESSION) and a mistyped flag's name and
// value. A credential that lands in either was written to
// state/learn/journal-*.jsonl; the journal appends through the kit
// (credkit.AppendFile), which withholds an entry carrying a held value.
func TestJournalNeverRecordsAHeldCredential(t *testing.T) {
	cliBin, _ := binaries(t)
	const key = "bdkey" + "JournL8x2q9w"
	cases := []struct {
		name  string
		extra []string
		args  [][]string
	}{
		{"argv", nil, [][]string{{"license", "get", "--" + key}, {"license", "get", "--compny-id", key}}},
		{"session", []string{"BITDEFENDER_LEARN_SESSION=ops-" + key}, [][]string{{"license", "get", "--compny-id", "c1"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			env := closedEnv(home, append([]string{"BITDEFENDER_API_KEY=" + key}, c.extra...)...)
			for _, a := range c.args {
				run(t, cliBin, env, a...)
			}
			journals, _ := filepath.Glob(filepath.Join(home, "state", "learn", "journal-*.jsonl"))
			if len(journals) == 0 {
				t.Fatal("no journal segment was written, so nothing was checked")
			}
			assertNoFileCarries(t, home, nil, key)
		})
	}
}

// TestMalformedConfigCredentialHeldAtStartup (startup ordering): a config.toml
// that does not parse still has its values held before anything prints, so a
// startup diagnostic that echoes a piece of it (a flag error, in both binaries)
// is withheld. The default config, an explicit --config and BITDEFENDER_CONFIG.
func TestMalformedConfigCredentialHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "bdCfg" + "Key-77aa91Qz"
	body := "api_key = \"" + secret + "\" broken\n"
	t.Run("default", func(t *testing.T) {
		home := t.TempDir()
		writeFile(t, filepath.Join(configDir(home), "config.toml"), body)
		env := closedEnv(home)
		assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", secret, "version"), secret)
		assertAbsent(t, "CLI doctor", run(t, cliBin, env, "doctor"), secret)
		assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", secret), secret)
	})
	t.Run("explicit", func(t *testing.T) {
		home := t.TempDir()
		path := filepath.Join(home, "elsewhere", "gz.toml")
		writeFile(t, path, body)
		env := closedEnv(home)
		assertAbsent(t, "CLI --config flag error", run(t, cliBin, env, "--config", path, "--timeout", secret, "version"), secret)
		env = closedEnv(home, "BITDEFENDER_CONFIG="+path)
		assertAbsent(t, "CLI BITDEFENDER_CONFIG flag error", run(t, cliBin, env, "--timeout", secret, "version"), secret)
		assertAbsent(t, "MCP BITDEFENDER_CONFIG flag error", run(t, mcpBin, env, "--transport", secret), secret)
	})
}

// TestHeldCredentialRefusedOffOriginAndOverPlainHTTP (origin): the API key goes
// only to the base URL it is configured for, and never over plain http, not
// even to loopback. A --deliver webhook whose URL carries the key, a feedback
// endpoint the key is pasted to, and a redirect from the API to another origin
// are refused before they dial.
func TestHeldCredentialRefusedOffOriginAndOverPlainHTTP(t *testing.T) {
	cliBin, _ := binaries(t)
	const key = "bdkey" + "Orig1nQ7r3s5"
	basic := base64.StdEncoding.EncodeToString([]byte(key + ":"))
	// One off-origin server per case, so each count is that case's alone.
	offOrigin := func() *vendor {
		return startTLS(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	}
	other, fbOther, redirOther := offOrigin(), offOrigin(), offOrigin()
	var redirects atomic.Int64
	api := startVendor(t, map[string]string{"getLicenseInfo": `{"usedSlots":1,"totalSlots":5}`})
	redirector := startTLS(t, func(w http.ResponseWriter, r *http.Request) {
		redirects.Add(1)
		http.Redirect(w, r, redirOther.url+r.URL.Path, http.StatusTemporaryRedirect)
	})
	var plainHits atomic.Int64
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plainHits.Add(1)
		io.WriteString(w, `{}`)
	}))
	t.Cleanup(plain.Close)

	home := t.TempDir()
	env := closedEnv(home, api.env(key)...)
	run(t, cliBin, env, "license", "get", "--json", "--deliver", "webhook:"+other.url+"/hook/"+key)
	if n := other.conns.Load(); n != 0 {
		t.Errorf("webhook to another https origin: %d connection(s) opened for a URL that carries the key", n)
	}
	run(t, cliBin, env, "license", "get", "--json", "--deliver", "webhook:"+plain.URL+"/hook/"+key)
	if n := plainHits.Load(); n != 0 {
		t.Errorf("webhook over plain http: %d request(s) carried the key", n)
	}
	if api.hits.Load() == 0 {
		t.Error("the API stub was never read, so no webhook was attempted")
	}

	// A feedback endpoint on another origin, with the key pasted into the note.
	fbEnv := append(slices.Clone(env), "BITDEFENDER_FEEDBACK_ENDPOINT="+fbOther.url+"/feedback")
	run(t, cliBin, fbEnv, "feedback", "--send", "the key "+key+" did not work")
	if n := fbOther.conns.Load(); n != 0 {
		t.Errorf("feedback endpoint on another origin: %d connection(s) opened for a body that carries the key", n)
	}

	// The API answers with a redirect to another origin: an outcome guard. A
	// #method call never follows one (the envelope reads a 3xx as a malformed
	// answer), so this drives doctor, whose reachability probe is followed; the
	// client's redirect guard also refuses a loopback target, so on these
	// loopback stubs it passed before the kit too. The cases above are the ones
	// seen failing on the pre-kit build.
	rEnv := closedEnv(t.TempDir(), "BITDEFENDER_BASE_URL="+redirector.url+"/api", "BITDEFENDER_CA_CERT="+redirector.caFile, "BITDEFENDER_API_KEY="+key)
	run(t, cliBin, rEnv, "doctor")
	if redirects.Load() == 0 {
		t.Error("the redirecting API stub was never read")
	}
	if n := redirOther.conns.Load(); n != 0 {
		t.Errorf("redirect to another origin: %d connection(s) opened with the key", n)
	}

	// The API base URL itself over plain http: the key is never sent.
	var keyHits atomic.Int64
	plainAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization"), basic) {
			keyHits.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		var c rpcCall
		_ = json.NewDecoder(r.Body).Decode(&c)
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{}}`, c.ID)
	}))
	t.Cleanup(plainAPI.Close)
	env = closedEnv(t.TempDir(), "BITDEFENDER_BASE_URL="+plainAPI.URL+"/api", "BITDEFENDER_API_KEY="+key)
	assertAbsent(t, "plain http API", run(t, cliBin, env, "license", "get"), key, basic)
	if n := keyHits.Load(); n != 0 {
		t.Errorf("plain http API: %d request(s) carried the key", n)
	}
}

// mcpToolsList runs one MCP stdio session that asks for tools/list and returns
// stdout plus stderr, and the names of the tools it lists.
func mcpToolsList(t *testing.T, bin string, env []string) (string, []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
	fmt.Fprintln(stdin, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	var out strings.Builder
	var names []string
	listed := false
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for !listed && sc.Scan() {
		out.WriteString(sc.Text() + "\n")
		var msg struct {
			ID     *int `json:"id"`
			Result struct {
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			} `json:"result"`
		}
		if json.Unmarshal(sc.Bytes(), &msg) == nil && msg.ID != nil && *msg.ID == 2 {
			listed = true
			for _, tool := range msg.Result.Tools {
				names = append(names, tool.Name)
			}
		}
	}
	_ = stdin.Close()
	_, _ = io.Copy(&out, stdout)
	_ = cmd.Wait()
	if !listed || len(names) == 0 {
		t.Errorf("MCP did not list its tools:\n%s\n%s", out.String(), stderr.String())
	}
	return out.String() + stderr.String(), names
}

// TestProfileBaseURLCredentialHeld (Codex r1 class A, ruling 8c): a URL saved
// under base-url (or base_url) in a run profile is a credential source, in the
// config dir's profiles.json and in the legacy ~/.bitdefender-cli one. The
// password and every query value of each such URL, every duplicate key
// included, are held at startup through the kit's URL reader (credkit.HoldURL),
// so profile show and profile use never print them. The MCP server lists no
// profile tool, and its tool list carries none of them either.
func TestProfileBaseURLCredentialHeld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const (
		userinfo = "Profile" + "Secret9xQ4"
		dupOne   = "DupSecret" + "One1Kq"
		dupTwo   = "DupSecret" + "Two2Wz"
		query    = "QuerySecret" + "77Lm"
		snake    = "Underscore" + "Sec5Rt"
	)
	cases := []struct {
		name, values string
		secrets      []string
	}{
		{"userinfo", `{"base-url":"https://ops:` + userinfo + `@api.example.com"}`, []string{userinfo}},
		{"duplicate key", `{"base-url":"https://a:` + dupOne + `@h.example","base-url":"https://b:` + dupTwo + `@h.example"}`, []string{dupOne, dupTwo}},
		{"query credential", `{"base-url":"https://h.example/api?token=` + query + `"}`, []string{query}},
		{"base_url spelling", `{"base_url":"https://ops:` + snake + `@api.example.com"}`, []string{snake}},
	}
	for _, c := range cases {
		for _, legacy := range []bool{false, true} {
			name := c.name
			if legacy {
				name += " legacy store"
			}
			t.Run(name, func(t *testing.T) {
				home := t.TempDir()
				path := filepath.Join(configDir(home), "profiles.json")
				if legacy {
					path = filepath.Join(home, ".bitdefender-cli", "profiles.json")
				}
				writeFile(t, path, `{"profiles":{"demo":{"name":"demo","values":`+c.values+`}}}`)
				env := closedEnv(home)
				for _, args := range [][]string{
					{"profile", "show", "demo", "--json"},
					{"profile", "show", "demo"},
					{"profile", "use", "demo"},
					{"profile", "use", "demo", "--json"},
					{"profile", "list", "--json"},
					{"profile", "list"},
				} {
					out := run(t, cliBin, env, append(args, "--no-learn")...)
					assertAbsent(t, strings.Join(args, " "), out, c.secrets...)
					if args[1] == "list" && !strings.Contains(out, "demo") {
						t.Errorf("%s does not list the saved profile, so the store was not read:\n%s", strings.Join(args, " "), out)
					}
				}
				out, tools := mcpToolsList(t, mcpBin, env)
				assertAbsent(t, "MCP tools/list", out, c.secrets...)
				for _, tool := range tools {
					if strings.Contains(strings.ToLower(tool), "profile") {
						t.Errorf("MCP lists %q: a tool that can show a saved profile needs its own leak case here", tool)
					}
				}
			})
		}
	}
}

// TestStoredBaseURLCredentialHeld (Codex r1 class A, the same class in the
// credential store): the store records the base_url a key was saved for, and a
// run under another --config (or BITDEFENDER_CONFIG) refuses that key and
// names the recorded URL. The password and every query value of that URL are
// held at startup through credkit.HoldURL.
func TestStoredBaseURLCredentialHeld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const (
		pw    = "Store" + "Secret8q2Zx"
		query = "StoreQuery" + "Tok3n9"
		key   = "bdkey" + "StoreBase7731"
	)
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "data", "credentials.toml"),
		"base_url = \"https://ops:"+pw+"@api.example.com/api?sig="+query+"\"\napi_key = \""+key+"\"\n")
	other := filepath.Join(home, "other", "gz.toml")
	writeFile(t, other, "base_url = \"https://api2.example.com/api\"\n")
	env := closedEnv(home)
	for _, args := range [][]string{
		{"--config", other, "doctor"},
		{"--config", other, "doctor", "--json"},
		{"--config", other, "license", "get"},
	} {
		out := run(t, cliBin, env, append(args, "--no-learn")...)
		assertAbsent(t, strings.Join(args[2:], " "), out, pw, query, key)
		// The refusal names the recorded URL, so the kit withholds it whole.
		if args[2] == "doctor" && !strings.Contains(out, "refused") && !strings.Contains(out, "withheld") {
			t.Errorf("doctor neither reports nor withholds the refused store key, so the store was not read:\n%s", out)
		}
	}
	mEnv := closedEnv(home, "BITDEFENDER_CONFIG="+other)
	assertAbsent(t, "MCP bitdefender_read", mcpCall(t, mcpBin, mEnv, `{"name":"bitdefender_read","arguments":{"endpoint_id":"license.get","params":{}}}`), pw, query, key)
}

// TestSpacefastPageKeyNeverPrinted (Codex r1 class B, contract 1): an anonymous
// Spacefast publish answers with the page's key and two links that carry it.
// The key is held the moment the receipt is decoded, and an anonymous page's
// links go only to its 0600 page record, whose path the run prints instead. The
// update reads the key back from that record; it prints no key either.
func TestSpacefastPageKeyNeverPrinted(t *testing.T) {
	cliBin, _ := binaries(t)
	key := "sfc_" + "Q7m2Xv9LpR4tZZ"
	var hits atomic.Int64
	sf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":{"space":{"id":"spc_demo"},"claim":{"key":"%s","url":"https://demo-page.space.fast/__/%s","claimUrl":"https://spacefast.com/claim#%s"}}}`, key, key, key)
	}))
	t.Cleanup(sf.Close)
	home := t.TempDir()
	env := closedEnv(home, "SPACEFAST_API_URL="+sf.URL)
	first := run(t, cliBin, env, "version", "--deliver", "spacefast:demo-page", "--no-learn")
	assertAbsent(t, "anonymous publish", first, key)
	if hits.Load() != 1 {
		t.Fatalf("the Spacefast stub answered %d request(s), want 1:\n%s", hits.Load(), first)
	}
	records, _ := filepath.Glob(filepath.Join(home, "state", "spacefast-deliveries", "*.json"))
	if len(records) != 1 {
		t.Fatalf("got %d page records, want 1:\n%s", len(records), first)
	}
	data, err := os.ReadFile(records[0])
	if err != nil || !bytes.Contains(data, []byte(key)) {
		t.Errorf("the page record does not hold the key (err %v):\n%s", err, data)
	}
	if runtime.GOOS != "windows" { // Windows reports 0666; its ACL is the press store's concern
		if info, err := os.Stat(records[0]); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("the page record is not 0600: %v %v", info, err)
		}
	}
	if !strings.Contains(first, records[0]) || !strings.Contains(first, "SPACEFAST_TOKEN") {
		t.Errorf("the publish does not name the page record and SPACEFAST_TOKEN:\n%s", first)
	}
	second := run(t, cliBin, env, "version", "--deliver", "spacefast:demo-page", "--no-learn")
	assertAbsent(t, "anonymous update", second, key)
	assertNoFileCarries(t, home, records, key)
}
