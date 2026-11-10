// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is one the review's failing cases named. Set
// DATTO_SAAS_LEAK_BIN_DIR to run them against binaries built elsewhere (how the
// pre-kit build was seen failing: .review/classes-fail-first.txt).

import (
	"bufio"
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/big"
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

	"datto-saas-pp-cli/internal/cli"
	"datto-saas-pp-cli/internal/cliutil"
	"datto-saas-pp-cli/internal/credkit"
	"datto-saas-pp-cli/internal/credwire"

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
		if d := os.Getenv("DATTO_SAAS_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "datto-saas-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{{"datto-saas-cli", "../../cmd/datto-saas-cli"}, {"datto-saas-mcp", "../../cmd/datto-saas-mcp"}} {
			if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, b.out), b.pkg).CombinedOutput(); err != nil {
				binErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, "datto-saas-cli"), filepath.Join(binDir, "datto-saas-mcp")
}

const fixturePublicKey = "fixture-public-q7x2"

// domainRow is one row of Datto's GET /saas/domains answer.
func domainRow(name string) string {
	return fmt.Sprintf(`[{"saasCustomerId":5,"domain":"a.example","saasCustomerName":%q,"productType":"Office365"}]`, name)
}

// vendor is a TLS stub of the Datto SaaS API: the kit never sends a credential
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
	return []string{"DATTO_SAAS_BASE_URL=" + v.url, "DATTO_SAAS_CA_CERT=" + v.caFile, "DATTO_SAAS_PUBLIC_KEY=" + fixturePublicKey, "DATTO_SAAS_SECRET_KEY=" + key}
}

// closedEnv is a closed environment under one sandbox home.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "DATTO_SAAS_HOME=" + home, "TMPDIR=" + os.TempDir(), "NO_COLOR=1"}, extra...)
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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

// TestMalformedConfigHeldBeforeAnythingPrints: a flag error prints before any
// command loads config, from a config TOML rejects, in both binaries. The first
// case is the review's input for the holder-coverage class: a comment glued to a
// closing quote, which a whitespace split held as tok"#c and not as tok.
func TestMalformedConfigHeldBeforeAnythingPrints(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	// A quoted span of an unterminated string can be ordinary text; holding it may
	// refuse the start, which prints nothing the file held.
	refusable := map[string]bool{"unterminated-multi-line": true}
	cases := []struct{ name, file, secret string }{
		{"comment-glued-to-quote", "saas_secret_key = \"dt_live_glued0101\"#longcomment\nbroken = = longenoughvalue\n", "dt_live_glued0101"},
		{"inline-table-comma", "headers = { X-Api-Key = \",dt_live_comma123\" }\nbroken = = longenoughvalue\n", ",dt_live_comma123"},
		{"unterminated-array", "extra = [\"dt_live_array8910\",\nsaas_secret_key = \"dt_live_after1112\"\n", "dt_live_array8910"},
		{"key-after-unterminated-array", "extra = [\"filler-value\",\nsaas_secret_key = \"dt_live_after1314\"\n", "dt_live_after1314"},
		{"multi-line-string", "access_token = \"\"\"\ndt_live_multi1516\n\"\"\"\nbroken = = longenoughvalue\n", "dt_live_multi1516"},
		{"unterminated-multi-line", "note = \"\"\"\nsaas_secret_key = \"dt_live_inside1718\"\n", "dt_live_inside1718"},
		{"escaped-quote-raw", "saas_secret_key = \"dt_live\\\"esc1920\"\nbroken = = longenoughvalue\n", `dt_live\"esc1920`},
		{"escaped-quote-decoded", "saas_secret_key = \"dt_live\\\"esc2122\"\nbroken = = longenoughvalue\n", `dt_live"esc2122`},
		{"credential-all-punctuation", "saas_secret_key = \"!!--!!\"\nbroken = = longenoughvalue\n", "!!--!!"},
		{"bare-value", "client_secret = dt_live_bare2526\nbroken = = longenoughvalue\n", "dt_live_bare2526"},
		{"hash-in-bare-value", "client_secret = dt_live#hash2728\nbroken = = longenoughvalue\n", "dt_live#hash2728"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "config", "config.toml"), c.file)
			env := closedEnv(home)
			runner := run
			if refusable[c.name] {
				runner = func(t *testing.T, bin string, env []string, args ...string) string {
					out, _ := runCode(t, bin, env, args...)
					return out
				}
			}
			assertAbsent(t, "CLI startup flag error", runner(t, cliBin, env, "--timeout", c.secret, "version"), c.secret)
			assertAbsent(t, "MCP startup flag error", runner(t, mcpBin, env, "--transport", c.secret), c.secret)
		})
	}
}

// TestCollidingMalformedConfigRefusesAndSaysSo: a malformed file's short value that
// occurs in public text stops the start with a named reason, not a leak and not a
// silent exit. The review's exact input has a one-letter comment.
func TestCollidingMalformedConfigRefusesAndSaysSo(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	const secret = "dt_live_glued0101"
	writeFile(t, filepath.Join(home, "config", "config.toml"), "saas_secret_key = \""+secret+"\"#c\n")
	cmd := exec.Command(cliBin, "--timeout", secret, "version")
	cmd.Env = closedEnv(home)
	out, _ := cmd.CombinedOutput()
	assertAbsent(t, "refused start", string(out), secret)
	if cmd.ProcessState.ExitCode() != credkit.ExitRefused || !strings.Contains(string(out), "refusing to start") {
		t.Errorf("exit %d, want a named refusal:\n%s", cmd.ProcessState.ExitCode(), out)
	}
}

// TestArgvScanGuessesNoArity is the review's other holder-coverage input: pflag
// takes "--" as a flag's value and keeps parsing, so --home after it relocates the
// run, and the token it names must already be held when the unknown flag echoes.
func TestArgvScanGuessesNoArity(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "dt_live_" + "relocated2930"
	for name, args := range map[string]func(string) []string{
		"home-after-dashdash-value": func(h string) []string { return []string{"--select", "--", "--home", h, "--" + secret} },
		"home-equals":               func(h string) []string { return []string{"--home=" + h, "--" + secret} },
		"home-as-a-value":           func(h string) []string { return []string{"--select", "--home", h, "--" + secret} },
	} {
		t.Run(name, func(t *testing.T) {
			home, other := t.TempDir(), t.TempDir()
			writeFile(t, filepath.Join(other, "config", "config.toml"), `saas_secret_key = "`+secret+`"`+"\n")
			assertAbsent(t, name, run(t, cliBin, closedEnv(home), args(other)...), secret)
		})
	}
}

// TestDiscoveryCoversEveryFileLoadReads: a credential in any file a run could read
// is held before a flag error prints.
func TestDiscoveryCoversEveryFileLoadReads(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "dt_live_discover3132"
	files := map[string]func(home string) string{
		"explicit-config":  func(h string) string { return filepath.Join(h, "tenant", "config.toml") },
		"global-store":     func(h string) string { return filepath.Join(h, "data", "credentials.toml") },
		"global-config":    func(h string) string { return filepath.Join(h, "config", "config.toml") },
		"legacy-config":    func(h string) string { return filepath.Join(h, ".config", "datto-saas-cli", "config.toml") },
		"home-flag-store":  func(h string) string { return filepath.Join(h, "other", "data", "credentials.toml") },
		"home-flag-config": func(h string) string { return filepath.Join(h, "other", "config", "config.toml") },
	}
	for name, path := range files {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, path(home), `saas_secret_key = "`+secret+`"`+"\n")
			args := []string{"--config", filepath.Join(home, "tenant", "config.toml"), "--home", filepath.Join(home, "other"), "--timeout", secret, "version"}
			assertAbsent(t, name, run(t, cliBin, closedEnv(home), args...), secret)
		})
	}
}

// TestTLSKeyIsHeldInItsDecodedForm is the review's input: the hex of a private
// key's DER in an unknown-transport error.
func TestTLSKeyIsHeldInItsDecodedForm(t *testing.T) {
	_, mcpBin := binaries(t)
	der := "302e020100300506032b657004220420" + strings.Repeat("0", 64)
	raw, err := hex.DecodeString(der)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	key := filepath.Join(home, "key.pem")
	writeFile(t, key, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw})))
	out := run(t, mcpBin, closedEnv(home), "--tls-key", key, "--transport", der)
	assertAbsent(t, "unknown transport", out, der, base64.StdEncoding.EncodeToString(raw))
}

// TestRealTLSKeysStartTheServer: the documented remote mode starts and serves TLS with
// each kind of key an operator would make.
func TestRealTLSKeysStartTheServer(t *testing.T) {
	_, mcpBin := binaries(t)
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, edKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for name, key := range map[string]crypto.Signer{"rsa": rsaKey, "p256": ecKey, "ed25519": edKey} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}}
			der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
			if err != nil {
				t.Fatal(err)
			}
			pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				t.Fatal(err)
			}
			certFile, keyFile := filepath.Join(home, "cert.pem"), filepath.Join(home, "key.pem")
			writeFile(t, certFile, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})))
			writeFile(t, keyFile, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})))
			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			addr := l.Addr().String()
			l.Close()
			cmd := exec.Command(mcpBin, "--transport", "http", "--addr", addr, "--tls-cert", certFile, "--tls-key", keyFile)
			cmd.Env = closedEnv(home, "DATTO_SAAS_MCP_HTTP_TOKEN="+strings.Repeat("9f3a", 16))
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer cmd.Process.Kill()
			exited := make(chan error, 1)
			go func() { exited <- cmd.Wait() }()
			deadline := time.After(20 * time.Second)
			for {
				conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true}) // #nosec G402 -- a test's own self-signed certificate
				if err == nil {
					conn.Close()
					return
				}
				select {
				case err := <-exited:
					t.Fatalf("the server exited (%v) without listening:\n%s", err, out.String())
				case <-deadline:
					t.Fatalf("the server was not listening after 20s:\n%s", out.String())
				case <-time.After(100 * time.Millisecond):
				}
			}
		})
	}
}

func echoFields(secrets []string) string {
	var parts []string
	for i, s := range secrets {
		b := []byte(s)
		for j, enc := range []string{
			s, "Bearer " + s, base64.StdEncoding.EncodeToString(b), base64.RawURLEncoding.EncodeToString(b),
			base32.StdEncoding.EncodeToString(b), base32.HexEncoding.EncodeToString(b), hex.EncodeToString(b),
			url.QueryEscape(s),
		} {
			v, _ := json.Marshal(enc)
			parts = append(parts, fmt.Sprintf(`"echo_%d_%d":%s`, i, j, v))
		}
	}
	return strings.Join(parts, ",")
}

func credentialForms(v string) map[string]string {
	b := []byte(v)
	base := map[string]string{
		"raw":           v,
		"lowercase":     strings.ToLower(v),
		"uppercase":     strings.ToUpper(v),
		"base64":        base64.RawStdEncoding.EncodeToString(b),
		"base64url":     base64.RawURLEncoding.EncodeToString(b),
		"base32":        base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b),
		"base32 lower":  strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)),
		"base32hex":     base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(b),
		"hex":           hex.EncodeToString(b),
		"hex upper":     strings.ToUpper(hex.EncodeToString(b)),
		"query-escaped": url.QueryEscape(v),
		"path-escaped":  url.PathEscape(v),
	}
	out := map[string]string{}
	for form, n := range base {
		out[form] = n
		if jb, err := json.Marshal(n); err == nil {
			if j := string(jb[1 : len(jb)-1]); j != n {
				out[form+" (json-escaped)"] = j
			}
		}
	}
	return out
}

// TestMCPHTTPRefusalsPassTheKit: the HTTP server's refusals and results leave
// through credkit.MCPHTTP, so a refused request echoes nothing held.
func TestMCPHTTPRefusalsPassTheKit(t *testing.T) {
	_, mcpBin := binaries(t)
	const token = "mcp-bear" + "er-4344-0123456789abcdef-long"
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, mcpBin, "--transport", "http", "--addr", addr)
	cmd.Env = closedEnv(t.TempDir(), "DATTO_SAAS_MCP_HTTP_TOKEN="+token)
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

// TestEveryStdinBodyIsHeldWhereItIsRead: a --stdin body has one way in.
func TestEveryStdinBodyIsHeldWhereItIsRead(t *testing.T) {
	files, err := filepath.Glob("../cli/*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob: %v %d", err, len(files))
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "io.ReadAll(os.Stdin)") {
			t.Errorf("%s reads a body without holding it: use cliutil.ReadRequestBody", f)
		}
	}
}

// TestShortHeldValueNeverPrinted: a value of any length is held. A six-digit
// password in the base URL, echoed by a flag error, and a six-digit secret key
// echoed by the vendor, in both binaries.
func TestShortHeldValueNeverPrinted(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const short = "482915"
	home := t.TempDir()
	base := closedEnv(home, "DATTO_SAAS_BASE_URL=https://u:"+short+"@api.datto.com/v1")
	assertAbsent(t, "CLI flag error", run(t, cliBin, base, "--timeout", short, "version"), short)
	assertAbsent(t, "MCP flag error", run(t, mcpBin, base, "--transport", short), short)

	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, domainRow(short)) })
	home = t.TempDir()
	env := closedEnv(home, v.env(short)...)
	assertAbsent(t, "domains", run(t, cliBin, env, "domains", "--json", "--data-source", "live"), short)
	assertAbsent(t, "sync", run(t, cliBin, env, "sync"), short)
	assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, nil, `{"name":"datto-saas_read","arguments":{"endpoint_id":"domains.list"}}`), short)
	assertNoFileCarries(t, home, nil, short)
}

// TestStartupDiagnosticsAreFiltered is the review's startup-output-unfiltered class:
// a held credential echoed by a flag or transport diagnostic before any command runs.
func TestStartupDiagnosticsAreFiltered(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "ReviewSecret0123456789abcdef"
	env := closedEnv(t.TempDir(), "DATTO_SAAS_SECRET_KEY="+secret)
	assertAbsent(t, "MCP transport", run(t, mcpBin, env, "--transport="+secret), secret)
	assertAbsent(t, "CLI timeout", run(t, cliBin, env, "--timeout="+secret, "domains"), secret)
	assertAbsent(t, "PP_MCP_TRANSPORT", run(t, mcpBin, append(env, "PP_MCP_TRANSPORT="+secret)), secret)
}

// TestInlineHeadersAreHeld is the review's holder-incomplete class: a valid inline
// table of headers sends a Basic credential, and the vendor's answer repeats it. The
// review's exact input is refused by name, because its value decodes to a word in
// public text. The other shapes run, and the value is withheld.
func TestInlineHeadersAreHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const reviewBasic = "dXNlcjpwYXNzd29yZA=="
	otherBasic := base64.StdEncoding.EncodeToString([]byte("zq7xalpha:zq7xbravo9921"))
	for name, c := range map[string]struct {
		config, basic string
		runs          bool
	}{
		"review-input": {`headers = { Authorization = "Basic ` + reviewBasic + `" }`, reviewBasic, false},
		"inline-table": {`headers = { Zeta = "Basic ` + otherBasic + `" }`, otherBasic, true},
		"table":        {"[headers]\nAuthorization = \"Basic " + otherBasic + "\"", otherBasic, true},
	} {
		t.Run(name, func(t *testing.T) {
			v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, domainRow(c.basic)) })
			home := t.TempDir()
			cfg := filepath.Join(home, "config", "config.toml")
			writeFile(t, cfg, c.config+"\n")
			env := closedEnv(home, "DATTO_SAAS_BASE_URL="+v.url, "DATTO_SAAS_CA_CERT="+v.caFile, "DATTO_SAAS_PUBLIC_KEY="+fixturePublicKey, "DATTO_SAAS_SECRET_KEY=fixture-"+"secret-q7x2")
			out, code := runCode(t, cliBin, env, "domains", "--json", "--data-source", "live")
			assertAbsent(t, "domains", out, c.basic)
			assertNoFileCarries(t, home, []string{cfg}, c.basic)
			if c.runs && (code == credkit.ExitRefused || v.hits.Load() == 0) {
				t.Errorf("exit %d after %d requests, want a run that reached the vendor:\n%s", code, v.hits.Load(), out)
			}
		})
	}
}

// TestSuccessEchoInEveryEncodingIsWithheld is the review's success-echo-encoding-gap
// class, with its inputs: a Base32 echo and six layers of percent-encoding past any
// fixed window, in a successful row, through the CLI and the MCP server.
func TestSuccessEchoInEveryEncodingIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "k3Yq8ZpW" + "2mR7vT4nL9sX6bH1cF5jD0gA"
	nested := secret
	for range 6 {
		var b strings.Builder
		for _, c := range []byte(nested) {
			fmt.Fprintf(&b, "%%%02X", c)
		}
		nested = b.String()
	}
	for name, field := range map[string]string{
		"base32":         base32.StdEncoding.EncodeToString([]byte(secret)),
		"nested-percent": nested,
		"raw":            secret,
		"base64":         base64.StdEncoding.EncodeToString([]byte(secret)),
		"hex":            hex.EncodeToString([]byte(secret)),
	} {
		t.Run(name, func(t *testing.T) {
			v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, domainRow(field)) })
			home := t.TempDir()
			env := closedEnv(home, v.env(secret)...)
			assertAbsent(t, "domains", run(t, cliBin, env, "domains", "--json", "--data-source", "live"), field)
			assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, nil, `{"name":"datto-saas_read","arguments":{"endpoint_id":"domains.list"}}`), field)
			assertNoFileCarries(t, home, nil, field)
		})
	}
	// The review's own secret, 32 of one letter: refused by name or withheld, never shown.
	t.Run("review-input", func(t *testing.T) {
		repeated := strings.Repeat("a", 32)
		encoded := base32.StdEncoding.EncodeToString([]byte(repeated))
		v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, domainRow(encoded)) })
		out, _ := runCode(t, cliBin, closedEnv(t.TempDir(), v.env(repeated)...), "domains", "--json", "--data-source", "live")
		assertAbsent(t, "domains", out, encoded, repeated)
	})
	t.Run("clean-row-still-shows", func(t *testing.T) {
		v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, domainRow("Contoso Ltd")) })
		out := run(t, cliBin, closedEnv(t.TempDir(), v.env(secret)...), "domains", "--json", "--data-source", "live")
		if !strings.Contains(out, "Contoso Ltd") {
			t.Errorf("a clean row was not shown:\n%s", out)
		}
	})
}

// TestLearnSessionLabelIsNotACredential: a session label is not held, so MCP context,
// whose text carries the same word, is not withheld.
func TestLearnSessionLabelIsNotACredential(t *testing.T) {
	_, mcpBin := binaries(t)
	env := closedEnv(t.TempDir(), "DATTO_SAAS_LEARN_SESSION=customer")
	out := mcpCall(t, mcpBin, env, nil, `{"name":"context","arguments":{}}`)
	if strings.Contains(out, "withheld") || !strings.Contains(out, "customer") {
		t.Errorf("MCP context was withheld or empty:\n%s", out)
	}
}

// TestSetCredentialsPositionalsAreHeld: the keys arrive as positional arguments no
// flag scan sees. A leading dash, all dashes, and either key inside a path the success
// message prints stay hidden.
func TestSetCredentialsPositionalsAreHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	for name, secret := range map[string]string{"dashed": "-BetaSecret4821", "all-dashes": "----------", "plain": "BetaSecret48217"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			home := filepath.Join(root, secret)
			out, code := runCode(t, cliBin, closedEnv(root), "--home", home, "auth", "set-credentials", "--", "-AlphaSecret4821", secret)
			assertAbsent(t, "set-credentials", out, secret, "AlphaSecret4821")
			// All hyphens occurs in public text, so that one start is refused by name.
			if name != "all-dashes" && code != 0 {
				t.Errorf("exit %d, want the keys saved:\n%s", code, out)
			}
		})
	}
	root := t.TempDir()
	out, _ := runCode(t, cliBin, closedEnv(root), "auth", "set-credentials", "BetaSecret48217", "--timeout=BetaSecret48217")
	assertAbsent(t, "flag error after a positional", out, "BetaSecret48217")
}

// TestFillerDoesNotSplitAnEcho: filler that a raw-byte window counts must not push
// the first half of an echo out of context.
func TestFillerDoesNotSplitAnEcho(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "abcdefghijklmnop"
	const a, b = "abcdefgh", "ijklmnop"
	env := closedEnv(t.TempDir(), "DATTO_SAAS_SECRET_KEY="+token)
	joined := func(out string) string {
		return strings.NewReplacer("\n", "", "\r", "", "\t", "", "%0A", "").Replace(out)
	}
	for name, filler := range map[string]string{
		"lf":      strings.Repeat("\n", 100_000),
		"tab":     strings.Repeat("\t", 100_000),
		"percent": strings.Repeat("%0A", 30_000),
	} {
		t.Run(name, func(t *testing.T) {
			if out := joined(run(t, cliBin, env, "--"+a+filler+b, "version")); strings.Contains(out, token) {
				t.Errorf("the echo reached the output whole once filler is removed (%d bytes)", len(out))
			}
		})
	}
}

// TestBinaryResponseCannotReachAFile: an octet-stream answer repeating the key must not
// become a base64 envelope in an export.
func TestBinaryResponseCannotReachAFile(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "export-secret-123456"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		io.WriteString(w, token)
	})
	home := t.TempDir()
	dest := filepath.Join(home, "export.jsonl")
	out := run(t, cliBin, closedEnv(home, v.env(token)...), "export", "domains", "--output", dest, "--no-cache")
	encoded := base64.StdEncoding.EncodeToString([]byte(token))
	assertAbsent(t, "export", out, token, encoded)
	assertNoFileCarries(t, home, nil, token, encoded)
}

// TestOutboundCarriesNoCredentialOffItsOrigin: a key in the feedback URL or in a
// webhook's User-Agent goes nowhere.
func TestOutboundCarriesNoCredentialOffItsOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "sentinel-token-123456"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, domainRow("Acme")) })
	sink := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	home := t.TempDir()
	env := closedEnv(home, v.env(token)...)
	env = append(env, "DATTO_SAAS_FEEDBACK_ENDPOINT="+sink.url+"/receipt/"+token, "SSL_CERT_FILE="+sink.caFile)
	assertAbsent(t, "feedback", run(t, cliBin, env, "feedback", "harmless", "--send"), token)
	if n := sink.hits.Load(); n != 0 {
		t.Errorf("feedback: %d requests carried the key to another origin", n)
	}
	env = closedEnv(home, append(v.env(token), "DATTO_SAAS_USER_AGENT="+token, "SSL_CERT_FILE="+sink.caFile)...)
	assertAbsent(t, "webhook", run(t, cliBin, env, "domains", "--json", "--data-source", "live", "--deliver", "webhook:"+sink.url+"/hook"), token)
	if n := sink.hits.Load(); n != 0 {
		t.Errorf("webhook: %d requests carried the key to another origin", n)
	}
}

// TestStdinBodyIsHeldBeforeAPreview: a --stdin body's credentials are held before a
// dry run prints them, however the body hides them.
func TestStdinBodyIsHeldBeforeAPreview(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "stdin-only-value-4567"
	for name, body := range map[string]string{
		"field":         `{"seat_type":"User","action_type":"License","ids":["a"],"password":"` + secret + `"}`,
		"duplicate-key": `{"token":"` + secret + `","token":"other-value-123456"}`,
		"case":          `{"Api_Key":"` + secret + `"}`,
		"nested":        `{"credentials":{"value":"` + secret + `"}}`,
		"malformed":     `{"secret":"` + secret + `"`,
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			cmd := exec.Command(cliBin, "seats", "bulk-change", "123456", "Classic:Office365:654321", "--stdin", "--dry-run")
			cmd.Env = closedEnv(home, "DATTO_SAAS_SECRET_KEY=fixture-"+"secret-q7x2", "DATTO_SAAS_PUBLIC_KEY="+fixturePublicKey)
			cmd.Stdin = strings.NewReader(body)
			out, _ := cmd.CombinedOutput()
			assertAbsent(t, "--stdin --dry-run", string(out), secret)
			assertNoFileCarries(t, home, nil, secret)
		})
	}
}

// TestRunProfileCarriesOnlyTheAllowList: a store's value under a credential-named or
// undeclared key never prints. A URL in free text is not a source (contract section 1).
func TestRunProfileCarriesOnlyTheAllowList(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"p":{"name":"p","description":"nightly","values":{
		"limit":"7",
		"secret_key":"profile-secret-1234567",
		"my-notes":"dt-free-text-1234567"
	}}}}`)
	env := closedEnv(home)
	out := run(t, cliBin, env, "profile", "show", "p", "--json", "--no-learn")
	assertAbsent(t, "profile show", out, "profile-secret-1234567", "dt-free-text-1234567")
	if !strings.Contains(out, `"limit"`) {
		t.Errorf("an allowed value was dropped:\n%s", out)
	}
}

// TestUsedConfigDecidesTheAllowedOrigin: the config the program really uses names the
// origin a credential may go to. A discovered default's base_url is only held, so a
// credential it holds never goes there.
func TestUsedConfigDecidesTheAllowedOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "dt_live_origin_44455566"
	used := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, domainRow("Acme")) })
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `[]`) })
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "config.toml"), "base_url = \""+other.url+"\"\n")
	explicit := filepath.Join(home, "tenant.toml")
	writeFile(t, explicit, "base_url = \""+used.url+"\"\nsaas_public_key = \""+fixturePublicKey+"\"\nsaas_secret_key = \""+token+"\"\n")
	env := closedEnv(home, "DATTO_SAAS_CA_CERT="+used.caFile)
	out := run(t, cliBin, env, "--config", explicit, "domains", "--json", "--no-learn", "--no-cache", "--data-source", "live")
	assertAbsent(t, "domains", out, token)
	if used.hits.Load() == 0 {
		t.Errorf("the explicit config's base URL was refused or unused:\n%s", out)
	}
	if n := other.hits.Load(); n != 0 {
		t.Errorf("%d requests went to the discovered default's base URL", n)
	}
}

// TestChildGetsTheKeysOnStdin: an MCP mirror tool runs the CLI, and the kit starts no
// child with a held value in its environment. The keys reach the vendor through the
// stdin hand-off.
func TestChildGetsTheKeysOnStdin(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const token = "dt_live_child_4142434"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, domainRow("Acme")) })
	home := t.TempDir()
	env := closedEnv(home, append(v.env(token), "DATTO_SAAS_CLI_PATH="+cliBin)...)
	out := mcpCall(t, mcpBin, env, nil, `{"name":"sync","arguments":{"confirm":true}}`)
	assertAbsent(t, "sync", out, token, "not started", "withheld from child")
	if v.hits.Load() == 0 {
		t.Fatalf("the child reached no vendor endpoint:\n%s", out)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(fixturePublicKey+":"+token))
	if got, _ := v.auth.Load().(string); got != want {
		t.Errorf("the child sent Authorization %q, want the handed keys", got)
	}
}

// TestMirroredWriteRunsNothingUntilConfirm: a mirrored writer starts no child and writes
// nothing until confirm is boolean true, and a host-file flag is refused by name first.
func TestMirroredWriteRunsNothingUntilConfirm(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, domainRow("Acme")) })
	home := t.TempDir()
	env := closedEnv(home, append(v.env("fixture-secret-q7x2"), "DATTO_SAAS_CLI_PATH="+cliBin)...)
	out := mcpCall(t, mcpBin, env, nil, `{"name":"sync","arguments":{}}`, `{"name":"sync","arguments":{"confirm":"true"}}`, `{"name":"sync","arguments":{"config":"/etc/hosts"}}`)
	if n := v.hits.Load(); n != 0 {
		t.Errorf("an unconfirmed sync sent %d requests:\n%s", n, out)
	}
	if !strings.Contains(out, "confirmation_required") || !strings.Contains(out, "config") {
		t.Errorf("no preview or file-flag refusal:\n%s", out)
	}
	// A bad ordinary flag reaches the child and fails, and that journals nothing either.
	bad := mcpCall(t, mcpBin, env, nil, `{"name":"export","arguments":{"resource":"domains","limit":"not-a-number"}}`)
	if !strings.Contains(bad, "limit") || strings.Contains(bad, "confirmation_required") {
		t.Errorf("a bad flag was not refused on preview:\n%s", bad)
	}
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			t.Errorf("a preview wrote %s", p)
		}
		return nil
	})
}

// TestWholeRunNeverEmitsAHeldCredential drives every source and every sink: the fake
// API repeats every credential in every encoding, and every stdout, stderr and file
// outside the credential files is scanned in every form.
func TestWholeRunNeverEmitsAHeldCredential(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	secrets := []string{
		"envS3cret+A1b2C3d4/E5f6G7h8",
		"mcpHttpT0ken+0123456789/abcdefghijKLMNOP",
		"cfgAuthH3ader-Z1x2C3v4B5n6M7",
	}
	stored := "storedS3cret_Q9w8E7r6T5y4U3i2"
	all := append(slices.Clone(secrets), stored)
	echo := atomic.Bool{}
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if !echo.Load() {
			io.WriteString(w, domainRow("Acme Dental"))
			return
		}
		fields := echoFields(all)
		switch p := r.URL.Path; {
		case strings.Contains(p, "/report/activity-log"):
			w.Header().Set("X-Request-ID", base32.StdEncoding.EncodeToString([]byte(secrets[0])))
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"message":"rate limited"}`)
		case strings.HasSuffix(p, "/seats"):
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"message":"bad request","detail":{%s}}`, fields)
		case strings.HasSuffix(p, "/applications"):
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, "<html><head><title>%s</title></head></html>", base32.StdEncoding.EncodeToString([]byte(secrets[0])))
		default:
			fmt.Fprintf(w, `[{"saasCustomerId":5,"domain":"a.example","saasCustomerName":"Acme Dental",%s}]`, fields)
		}
	})
	home := t.TempDir()
	env := closedEnv(home, append(v.env(secrets[0]), "DATTO_SAAS_MCP_HTTP_TOKEN="+secrets[1])...)
	var hits []string
	scan := func(where, text string) {
		for _, s := range all {
			for form, needle := range credentialForms(s) {
				if strings.Contains(text, needle) {
					hits = append(hits, where+": "+form)
				}
			}
		}
	}
	setCreds := exec.Command(cliBin, "auth", "set-credentials", fixturePublicKey, stored)
	setCreds.Env = closedEnv(home, "DATTO_SAAS_BASE_URL="+v.url, "DATTO_SAAS_CA_CERT="+v.caFile)
	out, err := setCreds.CombinedOutput()
	if err != nil {
		t.Fatalf("auth set-credentials: %v\n%s", err, out)
	}
	scan("auth set-credentials", string(out))
	cfg := filepath.Join(home, "config", "config.toml")
	existing, _ := os.ReadFile(cfg)
	writeFile(t, cfg, string(existing)+fmt.Sprintf("\nauth_header = %q\n", "Basic "+secrets[2]))

	if o := run(t, cliBin, env, "domains", "--json", "--data-source", "live"); !strings.Contains(o, "Acme Dental") {
		t.Fatalf("control run did not reach the API:\n%s", o)
	}
	run(t, cliBin, env, "sync")
	echo.Store(true)
	for _, args := range [][]string{
		{"doctor", "--json"},
		{"auth", "status", "--json"},
		{"domains", "--json", "--data-source", "live"},
		{"domains", "--agent", "--data-source", "live"},
		{"domains", "--dry-run"},
		{"domains", "--json", "--data-source", "live", "--deliver", "file:" + filepath.Join(home, "delivered.json")},
		{"sync"},
		{"activity-log", "--json", "--data-source", "live"},
		{"seats", "list", "5", "--json", "--data-source", "live"},
		{"applications", "5", "--json", "--data-source", "live"},
		{"search", "Acme", "--json"},
		{"search", secrets[0], "--json"},
		{"recall", secrets[0], "--json"},
		{"recall", secrets[2], "--json"},
	} {
		o, _ := runCode(t, cliBin, env, args...)
		scan(strings.Join(args[:min(2, len(args))], " "), o)
	}
	scan("MCP session", mcpCall(t, mcpBin, env, nil,
		`{"name":"datto-saas_read","arguments":{"endpoint_id":"domains.list"}}`,
		`{"name":"search","arguments":{"query":"Acme"}}`,
		`{"name":"sql","arguments":{"query":"select * from resources limit 5"}}`,
		`{"name":"context","arguments":{}}`,
	))
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Base(p) == "credentials.toml" || p == cfg {
			return nil
		}
		if synccpuStoreFile(p) { // synccpu: the store is not checked for this program's own credentials
			return nil
		}
		if data, rerr := os.ReadFile(p); rerr == nil {
			rel, _ := filepath.Rel(home, p)
			scan("file "+rel, string(data))
		}
		return nil
	})
	if len(hits) > 0 {
		t.Fatalf("%d held-credential hits:\n%s", len(hits), strings.Join(hits[:min(40, len(hits))], "\n"))
	}
}

// TestDeclarationsMatchTheHolderLists: the wiring checker reads the literal, so it
// repeats the list the config scanner uses.
func TestDeclarationsMatchTheHolderLists(t *testing.T) {
	d := credwire.Declarations
	if !slices.Equal(d.ConfigFields, cliutil.CredentialConfigKeys) || !slices.Equal(d.StoredRecords, cliutil.CredentialConfigKeys) {
		t.Errorf("config fields %v and stored records %v, want %v", d.ConfigFields, d.StoredRecords, cliutil.CredentialConfigKeys)
	}
}

// TestMCPArgumentCredentialIsHeld: a credential-named argument at any depth of any MCP
// tool call is a source, whatever the tool does with it. The preview echoes the
// arguments, and a vendor answer can repeat them.
func TestMCPArgumentCredentialIsHeld(t *testing.T) {
	_, mcpBin := binaries(t)
	const secret = "j7RmQ2vL" + "9xKa4nTb"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, domainRow(secret)) })
	for name, call := range map[string]string{
		"execute-preview": `{"name":"datto-saas_execute","arguments":{"endpoint_id":"seats.bulk-change","params":{"saasCustomerId":"123456","externalSubscriptionId":"Classic:Office365:654321","seat_type":"User","action_type":"License","ids":["r1"],"client_secret":"` + secret + `"}}}`,
		"nested":          `{"name":"datto-saas_execute","arguments":{"endpoint_id":"seats.bulk-change","params":{"saasCustomerId":"123456","externalSubscriptionId":"Classic:Office365:654321","seat_type":"User","action_type":"License","ids":["r1"],"extra":{"Api_Key":"` + secret + `"}}}}`,
		"read-echo":       `{"name":"datto-saas_read","arguments":{"endpoint_id":"domains.list","params":{"password":"` + secret + `"}}}`,
		"mirrored":        `{"name":"sync","arguments":{"token":"` + secret + `"}}`,
		// The adversarial review's input: a name spelled with another separator or in camel case.
		"separator": `{"name":"datto-saas_execute","arguments":{"endpoint_id":"seats.bulk-change","params":{"saasCustomerId":"123456","externalSubscriptionId":"Classic:Office365:654321","seat_type":"User","action_type":"License","ids":["r1"],"x-api-key":"` + secret + `","extra":{"secretKey":"` + secret + `"}}}}`,
		// Codex's input: mcp-go keeps the last arguments member of a call, so the kit reads the frame.
		"duplicate-arguments": `{"name":"datto-saas_read","arguments":{"client_secret":"` + secret + `"},"arguments":{"endpoint_id":"domains.list"}}`,
		"duplicate-key":       `{"name":"datto-saas_execute","arguments":{"endpoint_id":"seats.bulk-change","client_secret":"` + secret + `","client_secret":"other-value-123456","params":{"saasCustomerId":"123456","externalSubscriptionId":"Classic:Office365:654321","seat_type":"User","action_type":"License","ids":["` + secret + `"]}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			env := closedEnv(home, v.env("fixture-secret-q7x2")...)
			assertAbsent(t, name, mcpCall(t, mcpBin, env, nil, call), secret)
			assertNoFileCarries(t, home, nil, secret)
		})
	}
}

// TestProfileDuplicateKeyIsHeld: a JSON reader keeps the last of two duplicate keys, so
// the first was never held, and another field that repeats it prints.
func TestProfileDuplicateKeyIsHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const first = "j7RmQ2vL9xKa4nTb"
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"p":{"name":"p","description":"`+first+`","values":{"saas_secret_key":"`+first+`","saas_secret_key":"p8YsH3wN`+`6cDu5zFe"}}}}`)
	assertAbsent(t, "profile show", run(t, cliBin, closedEnv(home), "profile", "show", "p", "--json", "--no-learn"), first)
}

// TestFreeTextURLHoldsNothing: a link in a tool's free text is ordinary input, not a
// source (contract section 1, brief item 8): its query values and username stay usable
// and the server stays up.
func TestFreeTextURLHoldsNothing(t *testing.T) {
	_, mcpBin := binaries(t)
	home := t.TempDir()
	env := closedEnv(home, "DATTO_SAAS_SECRET_KEY=fixture-"+"secret-q7x2", "DATTO_SAAS_PUBLIC_KEY="+fixturePublicKey)
	search := func(q string) string {
		return `{"name":"search","arguments":{"query":"` + q + `"}}`
	}
	out := mcpCall(t, mcpBin, env, nil,
		search("https://kb.example/article?id=7"),
		search("https://kb.example/a?tab=seats"),
		search("https://joe@kb.example/a"),
		search("https://wiki.example/x?spaceKey=KB"),
		search("https://wiki.example/x?key=7"),
		search("https://wiki.example/viewpage.action?spaceKey=IT&title=Runbook"),
		search("https://kb.example/users/x:7@thread"),
		search("https://kb.example/forgot?reset_password=1"),
		search("after the links"))
	if strings.Contains(out, "withheld") {
		t.Errorf("a plain link withheld a result:\n%s", out)
	}
}

// TestProfileNameIsNotACredential: a profile named like a credential, or with a plain
// id in a link, leaves every later run working and can be deleted.
func TestProfileNameIsNotACredential(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	home := t.TempDir()
	env := closedEnv(home, "DATTO_SAAS_SECRET_KEY=fixture-"+"secret-q7x2", "DATTO_SAAS_PUBLIC_KEY="+fixturePublicKey)
	exit := func(bin string, args ...string) (int, string) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			var e *exec.ExitError
			if !errors.As(err, &e) {
				t.Fatal(err)
			}
			return e.ExitCode(), string(out)
		}
		return 0, string(out)
	}
	for _, args := range [][]string{{"token", "--json"}, {"hockey", "--select", "domain"}} {
		if code, out := exit(cliBin, append([]string{"profile", "save"}, args...)...); code != 0 {
			t.Fatalf("profile save %v: exit %d\n%s", args, code, out)
		}
	}
	code, out := exit(cliBin, "domains", "--dry-run", "--select", "domain", "--no-learn")
	if code != 0 || strings.Contains(out, "withheld") {
		t.Errorf("after saving the profiles, domains: exit %d\n%s", code, out)
	}
	if code, out := exit(mcpBin, "--transport", "nonsense-transport"); code == 78 {
		t.Errorf("after saving the profiles, the MCP server refused: %s", out)
	}
	if code, out := exit(cliBin, "profile", "delete", "token", "--yes"); code != 0 {
		t.Errorf("profile delete token: exit %d\n%s", code, out)
	}

	const long = "j7RmQ2vL9xKa4nTb"
	writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"p":{"name":"p","description":"https://kb.example/?id=7&tab=security&type=internal&sig=`+long+`","values":{"limit":"7"}}}}`)
	code, out = exit(cliBin, "profile", "show", "p", "--json", "--no-learn")
	if code == 78 {
		t.Errorf("a plain id in a profile's link refused the start: %s", out)
	}
	if code, out := exit(cliBin, "domains", "--dry-run", "--no-learn"); code != 0 {
		t.Errorf("after a profile with plain links, domains: exit %d\n%s", code, out)
	}
}

// TestConfigCommentIsNotAValue: a URL in the comment after a bare value holds nothing.
func TestConfigCommentIsNotAValue(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "config.toml"), "debug = true # see https://kb.example/a?id=7\n")
	cmd := exec.Command(cliBin, "domains", "--dry-run", "--no-learn")
	cmd.Env = closedEnv(home, "DATTO_SAAS_SECRET_KEY=fixture-"+"secret-q7x2", "DATTO_SAAS_PUBLIC_KEY="+fixturePublicKey)
	if out, err := cmd.CombinedOutput(); err != nil || strings.Contains(string(out), "withheld") {
		t.Errorf("a comment changed the run: %v\n%s", err, out)
	}
}

// TestStoreRefusalCarriesNoFileText: a store line that does not parse can put a
// credential where a key name belongs, and a refused start prints its source.
func TestStoreRefusalCarriesNoFileText(t *testing.T) {
	cliBin, _ := binaries(t)
	const piece = "zq7xbravo9921"
	for name, line := range map[string]string{
		"toml": "saas_secret_key = " + piece + "=Q9\n",
		"yaml": "saas_secret_key: " + piece + "=Q9\n",
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "data", "credentials.toml"), line)
			cmd := exec.Command(cliBin, "doctor")
			cmd.Env = closedEnv(home)
			out, _ := cmd.CombinedOutput()
			assertAbsent(t, "doctor", string(out), piece)
		})
	}
}

// TestProfileShortCredentialIsHeld: a value under a credential-named key has no
// strength floor: seven punctuation characters are a credential.
func TestProfileShortCredentialIsHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const short = "!$%^~+|"
	for name, values := range map[string]string{
		"key": `"saas_secret_key":"` + short + `"`,
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"p":{"name":"p","description":"`+short+`","values":{`+values+`}}}}`)
			assertAbsent(t, "profile show", run(t, cliBin, closedEnv(home), "profile", "show", "p", "--json", "--no-learn"), short)
		})
	}
}

// TestCredentialStoreURLIsNotAnOrigin: the program never sends to a base_url in the
// credential store, so it cannot allow a credential to go there either. Nothing listens
// on the port, and an https origin is the only kind the kit would allow, so the guard's
// verdict is in the error: withheld, or passed and then refused by the closed port.
func TestCredentialStoreURLIsNotAnOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "j7RmQ2vL" + "9xKa4nTb"
	const closed = "https://127.0.0.1:9"
	const key = "saas_public_key = \"" + fixturePublicKey + "\"\nsaas_secret_key = \"" + secret + "\"\n"
	deliver := func(config, store string) string {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, "config", "config.toml"), config)
		writeFile(t, filepath.Join(home, "data", "credentials.toml"), store)
		writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"p":{"name":"p","description":"`+secret+`","values":{}}}}`)
		out := run(t, cliBin, closedEnv(home, "DATTO_SAAS_SECRET_KEY="+secret), "profile", "show", "p", "--json", "--no-learn", "--deliver", "webhook:"+closed+"/receive")
		assertAbsent(t, "profile show", out, secret)
		return out
	}
	// The control: the config's own base URL is allowed, so the guard lets it pass.
	if out := deliver("base_url = \""+closed+"\"\n", key); !strings.Contains(out, "connection refused") {
		t.Fatalf("the config's base URL was not allowed:\n%s", out)
	}
	if out := deliver("\n", "base_url = \""+closed+"\"\n"+key); !strings.Contains(out, "withheld from outbound") {
		t.Errorf("the credential store's base URL was allowed:\n%s", out)
	}
}

// TestProfileBaseURLIsHeld: a saved profile value under base-url is a source, as
// config.toml's base_url is: its password and credential query values are held from
// startup, in every duplicate. The first input is Codex's.
func TestProfileBaseURLIsHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "ProfileOnly9x7"
	for name, values := range map[string]string{
		"password":  `"base-url":"https://operator:` + secret + `@api.datto.com/v1"`,
		"query":     `"base_url":"https://api.datto.com/v1?api_key=` + secret + `"`,
		"duplicate": `"base-url":"https://operator:` + secret + `@api.datto.com/v1","base-url":"https://api.datto.com/v1"`,
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"p":{"name":"p","description":"`+secret+`","values":{`+values+`}}}}`)
			assertAbsent(t, "profile show", run(t, cliBin, closedEnv(home), "profile", "show", "p", "--json", "--no-learn"), secret)
		})
	}
}

// TestHandoffDuplicateIsHeld: the MCP child's stdin hand-off holds every value it
// carries before a duplicate collapses, and a hand-off that does not parse is held
// too. The first input is Codex's.
func TestHandoffDuplicateIsHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "HandOffHidden9x7"
	for name, stdin := range map[string]string{
		"duplicate": `{"DATTO_SAAS_SECRET_KEY":"` + secret + `","DATTO_SAAS_SECRET_KEY":"OtherValue4q2"}`,
		"malformed": `{"DATTO_SAAS_SECRET_KEY":"` + secret + `"`,
		// Codex round 2's input: a second value after the first.
		"trailing":         `{} {"DATTO_SAAS_SECRET_KEY":"` + secret + `"}`,
		"trailing-garbage": `{"DATTO_SAAS_SECRET_KEY":"` + secret + `"} x`,
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"p":{"name":"p","description":"`+secret+`","values":{}}}}`)
			cmd := exec.Command(cliBin, "profile", "show", "p", "--json", "--no-learn")
			cmd.Env = closedEnv(home, cliutil.MCPChildEnvVar+"=1")
			cmd.Stdin = strings.NewReader(stdin)
			out, _ := cmd.CombinedOutput()
			// A hand-off that does not parse may refuse the start (fail closed); a
			// well-formed one has to run, or the absence check proves nothing.
			if cmd.ProcessState.ExitCode() == credkit.ExitRefused && !strings.Contains(name, "garbage") && name != "malformed" {
				t.Fatalf("the start was refused, so this run proves nothing:\n%s", out)
			}
			assertAbsent(t, "profile show", string(out), secret)
		})
	}
}

// synccpuStoreFile reports whether path is the local SQLite store or one of its
// side files. synccpu (owner GO 2026-11-09) dropped the store's credential check
// except for held vendor response values, so a credential this program holds from
// its own configuration may sit in the store; every other file is still checked.
func synccpuStoreFile(path string) bool {
	for _, s := range []string{".db", ".db-wal", ".db-shm", ".db-journal"} {
		if strings.HasSuffix(path, s) {
			return true
		}
	}
	return false
}
