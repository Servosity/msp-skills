// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class's failing input; the
// before/after runs are in the PR's review notes. Set
// CONNECTSECURE_LEAK_BIN_DIR to run them against binaries built elsewhere.

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

	"connectsecure-pp-cli/internal/credkit"
)

// Fixture credentials: random, 18 characters, and sharing no 8-character
// piece with the connector's program text or with each other.
const (
	fixJWT      = "SkwEH91W" + "CeVVcf9nxB"
	fixSecret   = "u8JW9BWA" + "ay40zzGV2K"
	fixClientID = "AD99Pw6C" + "USNeqGOpaT"
	fixTenant   = "hmamJyaK" + "1dtFFCANw7"
	fixHeader   = "ZRoOV1rD" + "XR20mwaG7w"
	fixPreToken = "vMKWL9Mg" + "v3193JC66y"
	fixProfTok  = "WxX10gVW" + "0HeNQ51hj2"
	fixCacheTok = "GTXxbCyr" + "WNnEQZHeeu"
	fixStoreTok = "6y55jjKg" + "ZMZWmPd05b"
	fixWebTok   = "Bk0HJa7f" + "NGRTdkLrPA"
	fixPlainTok = "I90dOjYK" + "Am78EXzQsk"
	fixPHToken  = "68ZhTEY7" + "N1O7txLgpr"
	fixPHSecret = "yuHxgKAw" + "MeMqhzX8dM"
	fixUserID   = "4815162342"
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
		if d := os.Getenv("CONNECTSECURE_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "connectsecure-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{exe("connectsecure-cli"), "../../cmd/connectsecure-cli"},
			{exe("connectsecure-mcp"), "../../cmd/connectsecure-mcp"},
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
	return filepath.Join(binDir, exe("connectsecure-cli")), filepath.Join(binDir, exe("connectsecure-mcp"))
}

// vendor is a TLS stub of a ConnectSecure pod: the kit never sends a
// credential over plain http, loopback included.
type vendor struct {
	url, caFile string
	hits        atomic.Int64
	conns       atomic.Int64
}

func startVendor(t *testing.T, h http.HandlerFunc) *vendor {
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

// mintEnv points the binaries at v with the mint credentials.
func (v *vendor) mintEnv() []string {
	return []string{"CONNECTSECURE_BASE_URL=" + v.url, "CONNECTSECURE_CA_CERT=" + v.caFile,
		"CONNECTSECURE_TENANT=" + fixTenant, "CONNECTSECURE_CLIENT_ID=" + fixClientID, "CONNECTSECURE_CLIENT_SECRET=" + fixSecret}
}

// tokenEnv points the binaries at v with a pre-minted JWT.
func (v *vendor) tokenEnv(token string) []string {
	return []string{"CONNECTSECURE_BASE_URL=" + v.url, "CONNECTSECURE_CA_CERT=" + v.caFile,
		"CONNECTSECURE_ACCESS_TOKEN=" + token, "CONNECTSECURE_USER_ID=" + fixUserID}
}

// closedEnv is a closed environment under one sandbox home; every press
// directory resolves under it.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "CONNECTSECURE_HOME=" + home,
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
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
			t.Errorf("%s shows %q:\n%s", label, s, out)
		}
	}
}

// assertNoFileCarries walks home; skip reports the files a credential is
// meant to be in (a source the test wrote, the JWT token cache).
func assertNoFileCarries(t *testing.T, home string, skip func(string) bool, secrets ...string) {
	t.Helper()
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || (skip != nil && skip(p)) {
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

func isTokenCache(p string) bool {
	return strings.HasPrefix(filepath.Base(p), "connectsecure-token-") && strings.HasSuffix(p, ".json")
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

const readCompanies = `{"name":"connectsecure_read","arguments":{"endpoint_id":"companies.list"}}`

// TestMintedJWTNeverShown (class minted-token): the JWT /w/authorize mints is
// held the moment it exists and when it is read back from the token cache, so
// a pod answer that reflects it (a 200 record, an error that names a piece of
// it) never reaches stdout, stderr, an MCP result or a file other than the
// token cache. The generated masks only ever covered error text, and only the
// whole value.
func TestMintedJWTNeverShown(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	piece := fixJWT[3:15]
	mintStub := func(echo func(w http.ResponseWriter)) *vendor {
		return startVendor(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/w/authorize" {
				fmt.Fprintf(w, `{"status":true,"data":{"access_token":%q,"user_id":%s}}`, fixJWT, fixUserID)
				return
			}
			echo(w)
		})
	}
	t.Run("success-echo", func(t *testing.T) {
		v := mintStub(func(w http.ResponseWriter) {
			fmt.Fprintf(w, `{"status":true,"total":1,"data":[{"id":7,"name":"Acme Dental","session_ref":%q}]}`, fixJWT)
		})
		home := t.TempDir()
		env := closedEnv(home, v.mintEnv()...)
		assertAbsent(t, "companies list (minted)", run(t, cliBin, env, "companies", "list", "--json"), fixJWT, piece)
		// The second run reads the JWT back from the token cache.
		assertAbsent(t, "companies list (cached)", run(t, cliBin, env, "companies", "list", "--json", "--no-cache"), fixJWT, piece)
		assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, readCompanies), fixJWT, piece)
		if v.hits.Load() < 2 {
			t.Errorf("the pod stub saw %d request(s), so the JWT was never minted and reflected", v.hits.Load())
		}
		assertNoFileCarries(t, home, isTokenCache, fixJWT, piece)
	})
	t.Run("error-echo", func(t *testing.T) {
		v := mintStub(func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, `{"status":false,"message":"session %s... is not allowed here"}`, piece)
		})
		home := t.TempDir()
		env := closedEnv(home, v.mintEnv()...)
		assertAbsent(t, "companies list", run(t, cliBin, env, "companies", "list"), piece)
		assertAbsent(t, "companies list --agent", run(t, cliBin, env, "companies", "list", "--agent"), piece)
		assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, readCompanies), piece)
		if v.hits.Load() < 2 {
			t.Errorf("the pod stub saw %d request(s), so nothing was echoed", v.hits.Load())
		}
	})
}

// TestStartupCredentialHeld (class startup-source): a credential in a file the
// program reads (a malformed config, a [headers] value, the 0600 store, a
// cached JWT, a profile value under the --client-auth-token name) is held
// before anything prints, so a startup diagnostic that echoes it (a flag
// error, in both binaries) is withheld. A config that does not parse still has
// its values held: every string and word of it.
func TestStartupCredentialHeld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	cases := []struct {
		name   string
		setup  func(t *testing.T, home string)
		secret string
	}{
		{"malformed-config-client-secret", func(t *testing.T, home string) {
			writeFile(t, filepath.Join(configDir(home), "config.toml"), "client_secret = \""+fixSecret+"\" broken\n")
		}, fixSecret},
		{"config-header-value", func(t *testing.T, home string) {
			writeFile(t, filepath.Join(configDir(home), "config.toml"), "base_url = \"https://pod106.myconnectsecure.com\"\n\n[headers]\nX-Extra-Auth = \""+fixHeader+"\"\n")
		}, fixHeader},
		{"credential-store", func(t *testing.T, home string) {
			writeFile(t, filepath.Join(home, "data", "credentials.toml"), "access_token = \""+fixStoreTok+"\"\n")
		}, fixStoreTok},
		{"token-cache", func(t *testing.T, home string) {
			b, _ := json.Marshal(map[string]any{"access_token": fixCacheTok, "user_id": fixUserID, "expires_at": time.Now().Add(time.Hour)})
			writeFile(t, filepath.Join(home, "cache", "connectsecure-token-0011223344556677.json"), string(b))
		}, fixCacheTok},
		{"profile-client-auth-token", func(t *testing.T, home string) {
			writeFile(t, filepath.Join(configDir(home), "profiles.json"),
				`{"profiles":{"mint":{"name":"mint","values":{"client-auth-token":"`+fixProfTok+`"}}}}`)
		}, fixProfTok},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			c.setup(t, home)
			env := closedEnv(home)
			assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", c.secret, "version"), c.secret)
			assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", c.secret), c.secret)
		})
	}
}

// proxyRecorder is a loopback listener every proxy variable names. A request
// the program does not refuse arrives here, as a CONNECT naming its host for
// https or as the absolute request line for plain http, and goes no further.
type proxyRecorder struct {
	ln    net.Listener
	mu    sync.Mutex
	lines []string
}

func newProxyRecorder(t *testing.T) *proxyRecorder {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &proxyRecorder{ln: ln}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				line, _ := bufio.NewReader(conn).ReadString('\n')
				p.mu.Lock()
				p.lines = append(p.lines, strings.TrimSpace(line))
				p.mu.Unlock()
				_, _ = conn.Write([]byte("HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))
			}(conn)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return p
}

func (p *proxyRecorder) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.lines...)
}

// env points every proxy variable Go reads at the recorder and empties the
// bypass lists. Loopback hosts are never proxied, so the TLS stubs are reached
// directly.
func (p *proxyRecorder) env() []string {
	u := (&url.URL{Scheme: "http", Host: p.ln.Addr().String()}).String()
	return []string{"HTTPS_PROXY=" + u, "https_proxy=" + u, "HTTP_PROXY=" + u, "http_proxy=" + u, "NO_PROXY=", "no_proxy="}
}

// TestHeldCredentialRefusedOffOriginAndOverPlainHTTP (class origin): the JWT
// goes only to the configured pod, and never over plain http. A --deliver
// webhook whose URL carries it is refused before it dials, to another https
// origin and to a plain http host; a pod base URL on plain http gets no
// credential, loopback included (the connector's own https rule allowed a
// loopback pod).
func TestHeldCredentialRefusedOffOriginAndOverPlainHTTP(t *testing.T) {
	cliBin, _ := binaries(t)
	api := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"status":true,"total":1,"data":[{"id":7,"name":"Acme Dental"}]}`)
	})
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	px := newProxyRecorder(t)

	env := closedEnv(t.TempDir(), append(api.tokenEnv(fixWebTok), px.env()...)...)
	run(t, cliBin, env, "companies", "list", "--json", "--deliver", "webhook:"+other.url+"/hook/"+fixWebTok)
	if n := other.conns.Load(); n != 0 {
		t.Errorf("webhook to another https origin: %d connection(s) opened for a URL that carries the JWT", n)
	}
	run(t, cliBin, env, "companies", "list", "--json", "--deliver", "webhook:http://hooks.connectsecure-leak.test/hook/"+fixWebTok)
	if got := px.seen(); len(got) != 0 {
		t.Errorf("webhook over plain http: %d request(s) carried the JWT: %q", len(got), got)
	}
	if api.hits.Load() == 0 {
		t.Error("the pod stub was never read, so no webhook was attempted")
	}

	// The pod base URL itself over plain http.
	var bearerHits atomic.Int64
	plainAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization"), fixPlainTok) {
			bearerHits.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":true,"data":[]}`)
	}))
	t.Cleanup(plainAPI.Close)
	podPx := newProxyRecorder(t)
	for _, base := range []string{plainAPI.URL, "http://pod106.myconnectsecure.com"} {
		env := closedEnv(t.TempDir(), append(podPx.env(), "CONNECTSECURE_BASE_URL="+base,
			"CONNECTSECURE_ACCESS_TOKEN="+fixPlainTok, "CONNECTSECURE_USER_ID="+fixUserID)...)
		assertAbsent(t, "plain http pod "+base, run(t, cliBin, env, "companies", "list"), fixPlainTok)
	}
	if n := bearerHits.Load(); n != 0 {
		t.Errorf("plain http loopback pod: %d request(s) carried the JWT", n)
	}
	if got := podPx.seen(); len(got) != 0 {
		t.Errorf("plain http pod: %d request(s) left through the proxy: %q", len(got), got)
	}
}

// TestPlaceholderHostGetsNoCredential (class credential-sent-to-another-host):
// the shipped default base URL, https://your-pod.myconnectsecure.com, names a
// host on the vendor's own domain. No credential goes there from any source
// (the built-in default, CONNECTSECURE_BASE_URL in any spelling, config.toml,
// a run profile) on any path (a generated command, sync, doctor, an MCP tool):
// every proxy variable names a loopback recorder, so a request the connector
// does not refuse arrives there as a CONNECT, and nothing reaches the host.
func TestPlaceholderHostGetsNoCredential(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	px := newProxyRecorder(t)
	secrets := []string{fixPHToken, fixPHSecret, fixHeader, fixProfTok}
	mint := []string{"CONNECTSECURE_TENANT=" + fixTenant, "CONNECTSECURE_CLIENT_ID=" + fixClientID, "CONNECTSECURE_CLIENT_SECRET=" + fixPHSecret}
	pre := []string{"CONNECTSECURE_ACCESS_TOKEN=" + fixPHToken, "CONNECTSECURE_USER_ID=" + fixUserID}
	type source struct {
		name  string
		env   []string
		setup func(t *testing.T, home string)
	}
	sources := []source{
		{"default-pre-minted", pre, nil},
		{"default-mint", mint, nil},
		{"env", append(slices.Clone(mint), "CONNECTSECURE_BASE_URL=https://your-pod.myconnectsecure.com"), nil},
		{"env-respelled", append(slices.Clone(pre), "CONNECTSECURE_BASE_URL=HTTPS://Your-Pod.MyConnectSecure.com.:443/"), nil},
		{"config", nil, func(t *testing.T, home string) {
			writeFile(t, filepath.Join(configDir(home), "config.toml"),
				"base_url = \"https://your-pod.myconnectsecure.com\"\naccess_token = \""+fixPHToken+"\"\nuser_id = \""+fixUserID+"\"\n\n[headers]\nX-Extra-Auth = \""+fixHeader+"\"\n")
		}},
	}
	for _, s := range sources {
		t.Run(s.name, func(t *testing.T) {
			home := t.TempDir()
			if s.setup != nil {
				s.setup(t, home)
			}
			env := closedEnv(home, append(px.env(), s.env...)...)
			for _, args := range [][]string{{"companies", "list", "--json"}, {"sync", "--resources", "companies"}} {
				out := run(t, cliBin, env, args...)
				if !strings.Contains(out, "CONNECTSECURE_BASE_URL") {
					t.Errorf("%v: the refusal does not name CONNECTSECURE_BASE_URL:\n%s", args, out)
				}
				assertAbsent(t, strings.Join(args, " "), out, secrets...)
			}
			assertAbsent(t, "doctor", run(t, cliBin, env, "doctor", "--json"), secrets...)
			out := mcpCall(t, mcpBin, env, readCompanies, `{"name":"sync","arguments":{"resources":"companies"}}`)
			if !strings.Contains(out, "CONNECTSECURE_BASE_URL") {
				t.Errorf("MCP read and sync: the refusal does not name CONNECTSECURE_BASE_URL:\n%s", out)
			}
			assertAbsent(t, "MCP read and sync", out, secrets...)
		})
	}
	// A run profile carries no base URL (the CLI has no --base-url flag); its
	// credential is a saved --client-auth-token, which only session sends.
	t.Run("profile", func(t *testing.T) {
		home := t.TempDir()
		writeFile(t, filepath.Join(configDir(home), "profiles.json"),
			`{"profiles":{"mint":{"name":"mint","values":{"client-auth-token":"`+fixProfTok+`"}}}}`)
		assertAbsent(t, "session --profile", run(t, cliBin, closedEnv(home, px.env()...), "session", "--profile", "mint", "--json"), secrets...)
	})
	if got := px.seen(); len(got) != 0 {
		t.Errorf("the program sent %d request(s) toward the placeholder host: %q", len(got), got)
	}
}
