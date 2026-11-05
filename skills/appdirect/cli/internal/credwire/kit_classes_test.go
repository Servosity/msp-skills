// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Black-box tests for the credential classes the shared credential kit closes
// for this connector (handfixes.json credkit-wiring), each driven by the
// failing input of the review that found it, and for the MCP server's exits
// (handfixes.json mcp-exit-delivers-stderr). They build both binaries and run
// them in a sandboxed home; the fake marketplace is served over https with a
// certificate trusted through APPDIRECT_CA_CERT and SSL_CERT_FILE.
package credwire

import (
	"bufio"
	"bytes"
	"encoding/pem"
	"flag"
	"fmt"
	"io/fs"
	"net"
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

var kitCLI, kitMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "appdirect-credwire-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	kitCLI = filepath.Join(dir, "appdirect-cli"+exe)
	kitMCP = filepath.Join(dir, "appdirect-mcp"+exe)
	for out, pkg := range map[string]string{kitCLI: "../../cmd/appdirect-cli", kitMCP: "../../cmd/appdirect-mcp"} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// sandbox is a home directory every path the program resolves lives under.
type sandbox struct {
	home string
	env  []string
}

func newSandbox(t *testing.T) *sandbox {
	t.Helper()
	home := t.TempDir()
	env := []string{
		"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "AppData"),
		"LOCALAPPDATA=" + filepath.Join(home, "LocalAppData"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg-config"), "XDG_DATA_HOME=" + filepath.Join(home, "xdg-data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "xdg-state"), "XDG_CACHE_HOME=" + filepath.Join(home, "xdg-cache"),
		"APPDIRECT_HOME=" + filepath.Join(home, "appdirect-home"),
		"APPDIRECT_NO_LEARN=true", "NO_COLOR=1", "PATH=" + os.Getenv("PATH"),
	}
	for _, name := range []string{"SYSTEMROOT", "SystemRoot", "TMPDIR", "TEMP", "TMP"} {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	return &sandbox{home: home, env: env}
}

type result struct {
	stdout, stderr string
	code           int
}

func (r result) all() string { return r.stdout + "\n" + r.stderr }

// run starts bin with the sandbox environment plus extra, stdin as given.
func (s *sandbox) run(t *testing.T, bin string, extra []string, stdin string, args ...string) result {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(append([]string{}, s.env...), extra...)
	cmd.Dir = s.home
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("%v: did not exit within 60s; stdout %q stderr %q", args, out.String(), errb.String())
	}
	return result{stdout: out.String(), stderr: errb.String(), code: cmd.ProcessState.ExitCode()}
}

// writeFile writes a 0600 file under the sandbox.
func (s *sandbox) writeFile(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(s.home, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// filesHolding names every file under the sandbox whose bytes hold any of
// values, the credential store (credentials.toml) excepted.
func (s *sandbox) filesHolding(t *testing.T, values ...string) []string {
	t.Helper()
	var hits []string
	_ = filepath.WalkDir(s.home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == "credentials.toml" {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		for _, v := range values {
			if bytes.Contains(b, []byte(v)) {
				hits = append(hits, fmt.Sprintf("%s holds %q", p, v))
			}
		}
		return nil
	})
	return hits
}

// tlsMarketplace is a fake AppDirect marketplace over https: the API under
// /api, the token endpoint at /oauth2/token. respond answers each API path.
func tlsMarketplace(t *testing.T, s *sandbox, accessToken string, respond func(path string) string) (*httptest.Server, []string) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth2/token" {
			fmt.Fprintf(w, `{"access_token":%q,"token_type":"bearer","expires_in":3600}`, accessToken)
			return
		}
		fmt.Fprint(w, respond(strings.TrimPrefix(r.URL.Path, "/api")))
	}))
	t.Cleanup(srv.Close)
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	caPath := s.writeFile(t, "fake-ca.pem", string(ca))
	return srv, []string{"APPDIRECT_CA_CERT=" + caPath, "SSL_CERT_FILE=" + caPath, "APPDIRECT_BASE_URL=" + srv.URL + "/api"}
}

// recordingProxy is an http proxy that records the first line of every
// request it is sent (a CONNECT names the host the program dialled) and
// answers each with 502.
func recordingProxy(t *testing.T) (addr string, lines func() []string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var got []string
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
				got = append(got, strings.TrimSpace(line))
				mu.Unlock()
				_, _ = c.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))
			}(c)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String(), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), got...)
	}
}

// Class credential-sent-to-other-host (review input): a blank
// APPDIRECT_BASE_URL over a 0600 config that names a tenant must never send
// the client pair to AppDirect's own marketplace.
func TestKitBlankBaseURLNeverSendsThePairToTheDefaultMarketplace(t *testing.T) {
	s := newSandbox(t)
	cfg := s.writeFile(t, "ad/config.toml", "base_url = \"https://tenant.example.net/api\"\n")
	proxy, lines := recordingProxy(t)
	env := []string{
		"APPDIRECT_BASE_URL= ", "APPDIRECT_CLIENT_ID=tenant-id", "APPDIRECT_CLIENT_SECRET=TenantSecret-71",
		"HTTPS_PROXY=http://" + proxy, "HTTP_PROXY=http://" + proxy, "NO_PROXY=", "no_proxy=",
	}
	for _, args := range [][]string{
		{"--config", cfg, "auth", "login"},
		{"--config", cfg, "account", "resource-company-read-company-get", "co-1", "--json", "--data-source", "live"},
	} {
		r := s.run(t, kitCLI, env, "", args...)
		for _, l := range lines() {
			if strings.Contains(strings.ToLower(l), "marketplace.appdirect.com") {
				t.Errorf("%v: the client pair was sent toward the default marketplace (%q); exit %d, output %q", args, l, r.code, r.all())
			}
		}
		if strings.Contains(r.all(), "TenantSecret-71") {
			t.Errorf("%v: printed the client secret: %q", args, r.all())
		}
	}
}

// Class credential-persisted-outside-store (review input): a [headers] value
// in config.toml that repeats a credential carried in APPDIRECT_BASE_URL's
// query string is never written back to config.toml by auth set-token.
func TestKitConfigHeaderRepeatingAURLCredentialIsNotWrittenBack(t *testing.T) {
	s := newSandbox(t)
	before := "base_url = \"https://tenant.example.net/api\"\n\n[headers]\nX-Trace = \"UrlSecret-7c9f2d\"\n"
	cfg := s.writeFile(t, "ad/config.toml", before)
	env := []string{"APPDIRECT_BASE_URL=https://tenant.example.net/api?api_key=UrlSecret-7c9f2d"}
	r := s.run(t, kitCLI, env, "LocalBearer-8e1f3c\n", "--config", cfg, "auth", "set-token")
	after, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != before && bytes.Contains(after, []byte("UrlSecret-7c9f2d")) {
		t.Errorf("auth set-token wrote the URL credential back into config.toml (exit %d, output %q):\n%s", r.code, r.all(), after)
	}
	for _, v := range []string{"UrlSecret-7c9f2d", "LocalBearer-8e1f3c"} {
		if strings.Contains(r.all(), v) {
			t.Errorf("auth set-token printed %q: %q", v, r.all())
		}
	}
}

// Class holder-incomplete, in its declared-source forms (the review's input is
// free text inside a non-credential field, which contract section 1 leaves out;
// see .review/followups.md): a credential with spaces in a declared response
// field, and a short bearer in a declared config field, are held, so a
// non-credential field that repeats either prints withheld.
func TestKitDeclaredCredentialsWithSpacesAndShortBearerAreHeld(t *testing.T) {
	const spaced = "Qw7 horse battery staple!"
	const short = "pQ7"
	t.Run("response field with spaces", func(t *testing.T) {
		s := newSandbox(t)
		_, env := tlsMarketplace(t, s, "MintedAccess-Zq4vX81k", func(p string) string {
			return fmt.Sprintf(`{"id":"co-1","name":%q,"password":%q,"status":"ACTIVE"}`, spaced, spaced)
		})
		env = append(env, "APPDIRECT_CLIENT_ID=PairId-Wm3c7Hq2", "APPDIRECT_CLIENT_SECRET=PairSecret-Jt6r1Ky9")
		r := s.run(t, kitCLI, env, "", "account", "resource-company-read-company-get", "co-1", "--json", "--data-source", "live")
		if strings.Contains(r.all(), spaced) {
			t.Errorf("a declared response credential with spaces was printed (exit %d): %q", r.code, r.all())
		}
		if hits := s.filesHolding(t, spaced); len(hits) > 0 {
			t.Errorf("written to a file: %v", hits)
		}
	})
	t.Run("short bearer in config auth_header", func(t *testing.T) {
		s := newSandbox(t)
		srv, env := tlsMarketplace(t, s, "unused", func(p string) string {
			return fmt.Sprintf(`{"id":"co-1","name":%q,"status":"ACTIVE"}`, short)
		})
		cfg := s.writeFile(t, "ad/config.toml", fmt.Sprintf("base_url = %q\nauth_header = \"Bearer %s\"\n", srv.URL+"/api", short))
		r := s.run(t, kitCLI, env, "", "--config", cfg, "account", "resource-company-read-company-get", "co-1", "--json", "--data-source", "live")
		for _, line := range strings.Split(r.all(), "\n") {
			if strings.Contains(line, `"`+short+`"`) || strings.Contains(line, " "+short+" ") {
				t.Errorf("a short bearer from config auth_header was printed (exit %d): %q", r.code, r.all())
				break
			}
		}
	})
}

// Class secret-in-encoded-output (review input): a numeric spelling of a held
// number longer than 400 characters is withheld, not skipped.
func TestKitLongNumericSpellingOfAHeldNumberIsWithheld(t *testing.T) {
	spelling := "123.4" + strings.Repeat("0", 400) + "e1" // 1234
	for _, form := range []struct {
		name  string
		stdin string
		args  []string
	}{
		{"stdin", spelling + "\n", []string{"feedback", "--stdin", "--json"}},
		{"argument", "", []string{"feedback", "note " + spelling, "--json"}},
	} {
		t.Run(form.name, func(t *testing.T) {
			s := newSandbox(t)
			r := s.run(t, kitCLI, []string{"APPDIRECT_CLIENT_SECRET=1234"}, form.stdin, form.args...)
			if strings.Contains(r.all(), spelling) {
				t.Errorf("the numeric spelling of the held secret was printed (exit %d): %.300q", r.code, r.all())
			}
			if hits := s.filesHolding(t, spelling); len(hits) > 0 {
				t.Errorf("the numeric spelling of the held secret was written: %v", hits)
			}
		})
	}
}

// mcp-exit-delivers-stderr: every exit of the MCP server delivers what it
// printed first: -h exits 0 with the usage, a bad flag 2 naming it, and the
// HTTP-mode refusal without a caller token exits 1 with its reason.
func TestKitMCPExitDeliversItsMessage(t *testing.T) {
	s := newSandbox(t)
	if r := s.run(t, kitMCP, []string{"APPDIRECT_MCP_HTTP_TOKEN="}, "", "--transport", "http", "--addr", "127.0.0.1:0"); r.code != 1 || !strings.Contains(r.stderr, "APPDIRECT_MCP_HTTP_TOKEN must be set") {
		t.Errorf("http with no token: exit %d, want 1 and the reason on stderr; stderr %q stdout %q", r.code, r.stderr, r.stdout)
	}
	if r := s.run(t, kitMCP, nil, "", "-h"); r.code != 0 || !strings.Contains(r.stderr, "-transport") {
		t.Errorf("-h: exit %d, want 0 and the usage on stderr; stderr %q stdout %q", r.code, r.stderr, r.stdout)
	}
	if r := s.run(t, kitMCP, nil, "", "--no-such-flag"); r.code != 2 || !strings.Contains(r.stderr, "no-such-flag") {
		t.Errorf("bad flag: exit %d, want 2 and the flag named on stderr; stderr %q stdout %q", r.code, r.stderr, r.stdout)
	}
}
