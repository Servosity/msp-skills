// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is one the review's failing cases named. Set
// BLACKPOINT_LEAK_BIN_DIR to run them against binaries built elsewhere (how the
// pre-kit build was seen failing: .review/classes-fail-first.txt).

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
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
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"blackpoint-pp-cli/internal/cli"
	"blackpoint-pp-cli/internal/cliutil"
	"blackpoint-pp-cli/internal/config"
	"blackpoint-pp-cli/internal/credkit"
	"blackpoint-pp-cli/internal/credwire"

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
		if d := os.Getenv("BLACKPOINT_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "blackpoint-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{{"blackpoint-cli", "../../cmd/blackpoint-cli"}, {"blackpoint-mcp", "../../cmd/blackpoint-mcp"}} {
			if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, b.out), b.pkg).CombinedOutput(); err != nil {
				binErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, "blackpoint-cli"), filepath.Join(binDir, "blackpoint-mcp")
}

// vendor is a TLS stub of the CompassOne API: the kit never sends a credential
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
	return []string{"BLACKPOINT_BASE_URL=" + v.url, "BLACKPOINT_CA_CERT=" + v.caFile, "BLACKPOINT_API_TOKEN=" + key}
}

// closedEnv is a closed environment under one sandbox home.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "BLACKPOINT_HOME=" + home, "TMPDIR=" + os.TempDir(), "NO_COLOR=1"}, extra...)
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

// TestShortHeldValueNeverPrinted: a value of any length is held. A six-digit
// password in the base URL, echoed by a flag error, and a six-digit token echoed
// by the vendor, in both binaries.
func TestShortHeldValueNeverPrinted(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const short = "482915"
	home := t.TempDir()
	base := closedEnv(home, "BLACKPOINT_BASE_URL=https://u:"+short+"@api.blackpointcyber.com/v1")
	assertAbsent(t, "CLI flag error", run(t, cliBin, base, "--timeout", short, "version"), short)
	assertAbsent(t, "MCP flag error", run(t, mcpBin, base, "--transport", short), short)

	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"data":[{"id":"t1","name":%q}],"meta":{"skip":0,"take":50,"totalItems":1}}`, short)
	})
	home = t.TempDir()
	env := closedEnv(home, v.env(short)...)
	assertAbsent(t, "tenants list", run(t, cliBin, env, "tenants", "list", "--json"), short)
	assertAbsent(t, "sync", run(t, cliBin, env, "sync"), short)
	assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, nil, `{"name":"blackpoint_read","arguments":{"endpoint_id":"tenants.list"}}`), short)
	assertNoFileCarries(t, home, nil, short)
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
		{"comment-glued-to-quote", "api_token = \"bp_live_glued0101\"#longcomment\nbroken = = longenoughvalue\n", "bp_live_glued0101"},
		{"inline-table-comma", "headers = { X-Api-Key = \",bp_live_comma123\" }\nbroken = = longenoughvalue\n", ",bp_live_comma123"},
		{"unterminated-array", "extra = [\"bp_live_array8910\",\napi_token = \"bp_live_after1112\"\n", "bp_live_array8910"},
		{"key-after-unterminated-array", "extra = [\"filler-value\",\napi_token = \"bp_live_after1314\"\n", "bp_live_after1314"},
		{"multi-line-string", "access_token = \"\"\"\nbp_live_multi1516\n\"\"\"\nbroken = = longenoughvalue\n", "bp_live_multi1516"},
		{"unterminated-multi-line", "note = \"\"\"\napi_token = \"bp_live_inside1718\"\n", "bp_live_inside1718"},
		{"escaped-quote-raw", "api_token = \"bp_live\\\"esc1920\"\nbroken = = longenoughvalue\n", `bp_live\"esc1920`},
		{"escaped-quote-decoded", "api_token = \"bp_live\\\"esc2122\"\nbroken = = longenoughvalue\n", `bp_live"esc2122`},
		{"credential-all-punctuation", "api_token = \"!!--!!\"\nbroken = = longenoughvalue\n", "!!--!!"},
		{"bare-value", "client_secret = bp_live_bare2526\nbroken = = longenoughvalue\n", "bp_live_bare2526"},
		{"hash-in-bare-value", "client_secret = bp_live#hash2728\nbroken = = longenoughvalue\n", "bp_live#hash2728"},
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
	const secret = "bp_live_glued0101"
	writeFile(t, filepath.Join(home, "config", "config.toml"), "api_token = \""+secret+"\"#c\n")
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
	const held = "bp-relocated-2930-x"
	for name, args := range map[string]func(string) []string{
		"home-after-dashdash-value": func(h string) []string { return []string{"--select", "--", "--home", h, "--" + held} },
		"home-equals":               func(h string) []string { return []string{"--home=" + h, "--" + held} },
		"home-as-a-value":           func(h string) []string { return []string{"--select", "--home", h, "--" + held} },
	} {
		t.Run(name, func(t *testing.T) {
			home, other := t.TempDir(), t.TempDir()
			writeFile(t, filepath.Join(other, "config", "config.toml"), `api_token = "`+held+`"`+"\n")
			assertAbsent(t, name, run(t, cliBin, closedEnv(home), args(other)...), held)
		})
	}
}

// TestDiscoveryCoversEveryFileLoadReads: a credential in any file a run could read
// is held before a flag error prints.
func TestDiscoveryCoversEveryFileLoadReads(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "bp_live_discover3132"
	files := map[string]func(home string) string{
		"explicit-config":  func(h string) string { return filepath.Join(h, "tenant", "config.toml") },
		"global-store":     func(h string) string { return filepath.Join(h, "data", "credentials.toml") },
		"global-config":    func(h string) string { return filepath.Join(h, "config", "config.toml") },
		"legacy-config":    func(h string) string { return filepath.Join(h, ".config", "blackpoint-cli", "config.toml") },
		"home-flag-store":  func(h string) string { return filepath.Join(h, "other", "data", "credentials.toml") },
		"home-flag-config": func(h string) string { return filepath.Join(h, "other", "config", "config.toml") },
	}
	for name, path := range files {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, path(home), `api_token = "`+secret+`"`+"\n")
			args := []string{"--config", filepath.Join(home, "tenant", "config.toml"), "--home", filepath.Join(home, "other"), "--timeout", secret, "version"}
			assertAbsent(t, name, run(t, cliBin, closedEnv(home), args...), secret)
		})
	}
}

// TestFillerDoesNotSplitAnEcho is the review's output-context class: filler that
// a raw-byte window counts pushed the first half of an echo out of context.
func TestFillerDoesNotSplitAnEcho(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "abcdefghijklmnop"
	const a, b = "abcdefgh", "ijklmnop"
	home := t.TempDir()
	env := closedEnv(home, "BLACKPOINT_API_TOKEN="+token)
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
	t.Run("profile-description", func(t *testing.T) {
		desc := a + `\u00` + strings.Repeat("%0A", 200_000) + "69jklmnop"
		data, err := json.Marshal(map[string]any{"profiles": map[string]any{"p": map[string]any{"name": "p", "description": desc, "values": map[string]string{}}}})
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(home, "config", "profiles.json"), string(data))
		out := run(t, cliBin, env, "profile", "show", "p", "--json", "--no-learn")
		assertAbsent(t, "profile show", out, a)
	})
}

// TestRot13BehindPercentIsHeld is the review's ROT13 input: ROT13 of the token's
// hex, percent-encoded per byte, behind 4097 dots.
func TestRot13BehindPercentIsHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "CredentialToken123456789"
	rot := func(s string) string {
		return strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z':
				return 'a' + (r-'a'+13)%26
			case r >= 'A' && r <= 'Z':
				return 'A' + (r-'A'+13)%26
			}
			return r
		}, s)
	}
	var enc strings.Builder
	for _, c := range []byte(rot(hex.EncodeToString([]byte(token)))) {
		fmt.Fprintf(&enc, "%%%02X", c)
	}
	desc := strings.Repeat(".", 4097) + enc.String()
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"data":[{"id":"a1","name":"acct","description":%q}],"meta":{"skip":0,"take":50,"totalItems":1}}`, desc)
	})
	home := t.TempDir()
	out := run(t, cliBin, closedEnv(home, v.env(token)...), "accounts", "list", "--json", "--no-cache", "--no-learn")
	assertAbsent(t, "accounts list", out, enc.String())
	assertNoFileCarries(t, home, nil, enc.String())
}

// TestBinaryResponseCannotReachAFile is the review's binary-intake input: an
// octet-stream answer repeating the token became a base64 envelope in the export.
func TestBinaryResponseCannotReachAFile(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "export-secret-123456"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		io.WriteString(w, token)
	})
	home := t.TempDir()
	dest := filepath.Join(home, "export.jsonl")
	out := run(t, cliBin, closedEnv(home, v.env(token)...), "export", "accounts", "acct-1", "--output", dest, "--no-cache")
	encoded := base64.StdEncoding.EncodeToString([]byte(token))
	assertAbsent(t, "export", out, token, encoded)
	assertNoFileCarries(t, home, nil, token, encoded)
}

// TestOutboundCarriesNoCredentialOffItsOrigin is the review's non-vendor POST
// input: a token in the feedback URL and in the webhook's User-Agent.
func TestOutboundCarriesNoCredentialOffItsOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "sentinel-token-123456"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"data":[{"id":"t1","name":"Acme"}],"meta":{"skip":0,"take":50,"totalItems":1}}`)
	})
	sink := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	home := t.TempDir()
	env := closedEnv(home, v.env(token)...)
	env = append(env, "BLACKPOINT_FEEDBACK_ENDPOINT="+sink.url+"/receipt/"+token, "SSL_CERT_FILE="+sink.caFile)
	assertAbsent(t, "feedback", run(t, cliBin, env, "feedback", "harmless", "--send"), token)
	if n := sink.hits.Load(); n != 0 {
		t.Errorf("feedback: %d requests carried the token to another origin", n)
	}
	env = closedEnv(home, append(v.env(token), "BLACKPOINT_USER_AGENT="+token, "SSL_CERT_FILE="+sink.caFile)...)
	assertAbsent(t, "webhook", run(t, cliBin, env, "tenants", "list", "--json", "--deliver", "webhook:"+sink.url+"/hook"), token)
	if n := sink.hits.Load(); n != 0 {
		t.Errorf("webhook: %d requests carried the token to another origin", n)
	}
}

// TestTLSKeyServesAndItsTextIsHeld: a real private key lets the HTTPS server start,
// and the key's text is held from startup. Holding its raw bytes as well spent minutes
// of CPU and then refused the start, so the only way to bind off loopback never served.
func TestTLSKeyServesAndItsTextIsHeld(t *testing.T) {
	_, mcpBin := binaries(t)
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	key := filepath.Join(home, "key.pem")
	writeFile(t, key, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))
	start := time.Now()
	out, code := runCode(t, mcpBin, closedEnv(home), "--tls-key", key, "--help")
	if code == 78 || time.Since(start) > 30*time.Second {
		t.Fatalf("a real key refused the start: exit %d after %s:\n%s", code, time.Since(start), out)
	}
	for _, form := range []string{base64.StdEncoding.EncodeToString(der), hex.EncodeToString(der), strings.ToUpper(hex.EncodeToString(der))} {
		assertAbsent(t, "the key's text", run(t, mcpBin, closedEnv(home), "--tls-key", key, "--transport", form), form)
	}
}

// TestDuoSecretKeyIsHeldBeforeAPreview is the review's input: the secret key in a
// write's confirmation preview and in a CLI dry run.
func TestDuoSecretKeyIsHeldBeforeAPreview(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const skey = "duo-secret-only-value-7890"
	const tenant = "3f2a9c1e-7b4d-4e8a-9f10-2c6d8b5e4a71"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{"id":"o1"}`) })
	home := t.TempDir()
	env := closedEnv(home, v.env("bp_live_token_33344455")...)
	cli := run(t, cliBin, env, "cisco-onboardings", "create", "--tenant-id", tenant, "--skey", skey, "--ikey", "DIxxxxxxxxxxxxxxxxxx", "--dry-run")
	assertAbsent(t, "CLI dry run", cli, skey)
	mcp := mcpCall(t, mcpBin, env, nil, fmt.Sprintf(`{"name":"blackpoint_execute","arguments":{"endpoint_id":"cisco_onboardings.create","params":{"tenantId":%q,"skey":%q}}}`, tenant, skey))
	assertAbsent(t, "MCP preview", mcp, skey)
	if n := v.hits.Load(); n != 0 {
		t.Errorf("a preview sent %d requests", n)
	}
	assertNoFileCarries(t, home, nil, skey)
}

// TestStdinBodyIsHeldBeforeAPreview: a --stdin body's credentials are held before
// a dry run prints them, however the body hides them.
func TestStdinBodyIsHeldBeforeAPreview(t *testing.T) {
	cliBin, _ := binaries(t)
	const skey = "duo-stdin-only-value-4567"
	for name, body := range map[string]string{
		"field":         `{"ikey":"DIxxxxxxxxxxxxxxxxxx","skey":"` + skey + `"}`,
		"duplicate-key": `{"skey":"` + skey + `","skey":"other-value-123456"}`,
		"case":          `{"SKey":"` + skey + `"}`,
		"nested":        `{"credentials":{"value":"` + skey + `"}}`,
		"in-a-list":     `{"skey":["` + skey + `"]}`,
		"malformed":     `{"skey":"` + skey + `"`,
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			cmd := exec.Command(cliBin, "cisco-onboardings", "create", "--tenant-id", "3f2a9c1e-7b4d-4e8a-9f10-2c6d8b5e4a71", "--stdin", "--dry-run")
			cmd.Env = closedEnv(home, "BLACKPOINT_API_TOKEN=bp_live_token_33344455")
			cmd.Stdin = strings.NewReader(body)
			out, _ := cmd.CombinedOutput()
			assertAbsent(t, "--stdin --dry-run", string(out), skey)
			assertNoFileCarries(t, home, nil, skey)
		})
	}
	t.Run("import", func(t *testing.T) {
		home := t.TempDir()
		cmd := exec.Command(cliBin, "import", "cisco_onboardings", "--input", "-", "--dry-run")
		cmd.Env = closedEnv(home, "BLACKPOINT_API_TOKEN=bp_live_token_33344455")
		cmd.Stdin = strings.NewReader(`{"skey":"` + skey + `"}` + "\n")
		out, _ := cmd.CombinedOutput()
		assertAbsent(t, "import --dry-run", string(out), skey)
		assertNoFileCarries(t, home, nil, skey)
	})
}

// TestStdinBodyPrettyAndEchoed: a body from jq (one member per line) starts and is
// sent whole. An answer that shares no part of the body reaches the operator; one
// that repeats a value of the body is withheld, because a declared stdin is held
// whole, and the write still happened (exit 0, and the notice says so).
func TestStdinBodyPrettyAndEchoed(t *testing.T) {
	cliBin, _ := binaries(t)
	const skey = "duo-stdin-only-value-4567"
	var got atomic.Value
	var answer atomic.Value
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.Store(string(b))
		io.WriteString(w, answer.Load().(string))
	})
	for name, body := range map[string]string{
		"pretty":  "{\n  \"name\": \"Acme Dental Group\",\n  \"skey\": \"" + skey + "\"\n}\n",
		"compact": `{"name":"Acme Dental Group","skey":"` + skey + `"}`,
	} {
		for _, c := range []struct {
			name, answer string
			shown        bool
		}{
			{"plain-answer", `{"created":"g1"}`, true},
			{"echoed-answer", `{"created":"g1","name":"Acme Dental Group"}`, false},
		} {
			t.Run(name+"/"+c.name, func(t *testing.T) {
				answer.Store(c.answer)
				home := t.TempDir()
				cmd := exec.Command(cliBin, "contact-groups", "create", "acct-1", "--stdin", "--json", "--no-learn", "--yes")
				cmd.Env = closedEnv(home, v.env("bp_live_token_33344455")...)
				cmd.Stdin = strings.NewReader(body)
				out, err := cmd.CombinedOutput()
				assertAbsent(t, "answer", string(out), skey)
				if err != nil {
					t.Errorf("exit %v:\n%s", err, out)
				}
				if shown := strings.Contains(string(out), "g1"); shown != c.shown {
					t.Errorf("answer shown = %v, want %v:\n%s", shown, c.shown, out)
				}
				var sent map[string]any
				raw, _ := got.Load().(string)
				if json.Unmarshal([]byte(raw), &sent) != nil || sent["skey"] != skey {
					t.Errorf("the vendor received %q", raw)
				}
				assertNoFileCarries(t, home, nil, skey)
			})
		}
	}
}

// TestCredentialFieldsReplacedByName: a Duo skey and a breached password are
// replaced by name before any sink, and the row's other fields stay usable. The
// password is not held: a later result sharing it is not withheld.
func TestCredentialFieldsReplacedByName(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const skey = "duo-response-skey-98765"
	const breached = "hunter2hunter2"
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"data":[{"id":"e1","email":"a@acme.example","password":%q,"skey":%q,"source":"forum"}],"meta":{"skip":0,"take":50,"totalItems":1}}`, breached, skey)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env("bp_live_token_33344455")...)
	out := run(t, cliBin, env, "tenants", "list", "--json", "--no-learn")
	assertAbsent(t, "list", out, skey, breached)
	if !strings.Contains(out, "a@acme.example") {
		t.Errorf("the row's other fields are gone:\n%s", out)
	}
	out = mcpCall(t, mcpBin, env, nil, `{"name":"blackpoint_read","arguments":{"endpoint_id":"tenants.list"}}`)
	assertAbsent(t, "MCP read", out, skey, breached)
	if !strings.Contains(out, "a@acme.example") {
		t.Errorf("MCP: the row's other fields are gone:\n%s", out)
	}
	assertNoFileCarries(t, home, nil, skey, breached)
}

// TestRunProfileDuplicateKeysAreHeld: a decoder keeps the last of a duplicate key,
// so the holder reads every occurrence.
func TestRunProfileDuplicateKeysAreHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const held = "dupkey-9z7x5v-Qw3T"
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{
		"p":{"name":"p","values":{"skey":"`+held+`","skey":"","select":"`+held+`"}}}}`)
	assertAbsent(t, "duplicate key", run(t, cliBin, closedEnv(home), "profile", "show", "p", "--json", "--no-learn"), held)
}

// TestRunProfileCarriesOnlyTheAllowList: a store's value under a credential-named
// or undeclared key never prints.
func TestRunProfileCarriesOnlyTheAllowList(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"p":{"name":"p","description":"nightly","values":{
		"limit":"7",
		"skey":"duo-profile-skey-1234567",
		"my-notes":"bp-free-text-1234567"
	}}}}`)
	env := closedEnv(home)
	out := run(t, cliBin, env, "profile", "show", "p", "--json", "--no-learn")
	assertAbsent(t, "profile show", out, "duo-profile-skey-1234567", "bp-free-text-1234567")
	if !strings.Contains(out, `"limit"`) {
		t.Errorf("an allowed value was dropped:\n%s", out)
	}
	if out := run(t, cliBin, env, "profile", "save", "bad", "--skey", "duo-save-value-1234567", "--no-learn"); !strings.Contains(out, "credential-like") && !strings.Contains(out, "unknown flag") {
		t.Errorf("a credential flag was saved:\n%s", out)
	}
}

// TestUsedConfigDecidesTheAllowedOrigin: the config the program really uses names
// the origin a credential may go to. A discovered default's base_url is only held,
// so a credential it holds never goes there.
func TestUsedConfigDecidesTheAllowedOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "bp_live_origin_44455566"
	used := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"data":[],"meta":{"skip":0,"take":50,"totalItems":0}}`)
	})
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "config.toml"), "base_url = \""+other.url+"\"\n")
	explicit := filepath.Join(home, "tenant.toml")
	writeFile(t, explicit, "base_url = \""+used.url+"\"\napi_token = \""+token+"\"\n")
	env := closedEnv(home, "BLACKPOINT_CA_CERT="+used.caFile)
	out := run(t, cliBin, env, "--config", explicit, "tenants", "list", "--json", "--no-learn", "--no-cache")
	assertAbsent(t, "tenants list", out, token)
	if used.hits.Load() == 0 {
		t.Errorf("the explicit config's base URL was refused or unused:\n%s", out)
	}
	if n := other.hits.Load(); n != 0 {
		t.Errorf("%d requests went to the discovered default's base URL", n)
	}
}

// TestOrdinaryFilesDoNotStopTheProgram: a value that is no credential stays usable.
// A config key the program never reads, a saved profile with a plain query value,
// a body with one, and an import file with a line that does not parse must not
// refuse the start or withhold the answer.
func TestOrdinaryFilesDoNotStopTheProgram(t *testing.T) {
	cliBin, _ := binaries(t)
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"data":[{"id":"g1","name":"json export"}],"meta":{"skip":0,"take":50,"totalItems":1}}`)
	})
	base := func(home string) []string { return closedEnv(home, v.env("bp_ordinary_token_6677")...) }
	t.Run("unknown-config-keys", func(t *testing.T) {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, "config", "config.toml"), "timeout = 30\nretries = 1\noutput = \"json\"\n")
		if out := run(t, cliBin, base(home), "tenants", "list", "--json", "--no-learn", "--no-cache"); !strings.Contains(out, "json export") {
			t.Errorf("the answer is gone:\n%s", out)
		}
	})
	t.Run("profile-plain-query", func(t *testing.T) {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"p":{"name":"p","description":"see https://example.test/?id=7","values":{"deliver":"webhook:https://example.test/hook?v=1"}}}}`)
		if out := run(t, cliBin, base(home), "version"); !strings.Contains(out, "blackpoint-cli") {
			t.Errorf("version printed:\n%s", out)
		}
		if out := run(t, cliBin, base(home), "profile", "show", "p", "--json", "--no-learn"); !strings.Contains(out, "example.test") {
			t.Errorf("profile show printed:\n%s", out)
		}
	})
	t.Run("body-plain-query", func(t *testing.T) {
		home := t.TempDir()
		cmd := exec.Command(cliBin, "contact-groups", "create", "acct-1", "--stdin", "--json", "--no-learn", "--yes")
		cmd.Env = base(home)
		cmd.Stdin = strings.NewReader(`{"name":"g","site":"https://example.test/?id=7"}`)
		if out, _ := cmd.CombinedOutput(); !strings.Contains(string(out), "g1") {
			t.Errorf("the answer of a write that happened is gone:\n%s", out)
		}
	})
	t.Run("import-stray-lines", func(t *testing.T) {
		home := t.TempDir()
		cmd := exec.Command(cliBin, "import", "cisco_onboardings", "--input", "-", "--dry-run", "--no-learn")
		cmd.Env = base(home)
		cmd.Stdin = strings.NewReader("x\n{\n\"name\": \"g\"\n}\n{\"name\":\"g\"}\n")
		if out, _ := cmd.CombinedOutput(); !strings.Contains(string(out), "Import complete") {
			t.Errorf("the import summary is gone:\n%s", out)
		}
	})
}

// TestCredentialStoreNamesNoOrigin: config.Load takes no base URL from the store,
// so a base_url there must not become the host a credential may go to.
func TestCredentialStoreNamesNoOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "store-origin-credential-5512"
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "config.toml"), "# nothing here\n")
	writeFile(t, filepath.Join(home, "data", "credentials.toml"), "base_url = \""+other.url+"\"\n")
	env := closedEnv(home, "BLACKPOINT_API_TOKEN="+token, "BLACKPOINT_CA_CERT="+other.caFile,
		"BLACKPOINT_FEEDBACK_ENDPOINT="+other.url+"/receipt/"+token)
	assertAbsent(t, "feedback", run(t, cliBin, env, "feedback", "harmless", "--send"), token)
	if n := other.hits.Load(); n != 0 {
		t.Errorf("%d requests carried the token to the store's base_url", n)
	}
	// The program calls the built-in API, so that is the origin the token may go to.
	// A dead proxy ends the request before it leaves this machine.
	env = append(env, "HTTPS_PROXY=http://127.0.0.1:1")
	out := run(t, cliBin, env, "tenants", "list", "--json", "--no-learn", "--no-cache", "--timeout", "2s")
	if !strings.Contains(out, "proxyconnect") {
		t.Errorf("the request never left the kit: the store's base_url replaced the origin the program calls:\n%s", out)
	}
}

// TestChildGetsTheToken: an MCP mirror tool runs the CLI, and the kit passes the
// child its credentials outside the environment. The token reaches the vendor.
func TestChildGetsTheToken(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const token = "bp-child-4142434-x"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"data":[],"meta":{"skip":0,"take":50,"totalItems":0}}`)
	})
	home := t.TempDir()
	env := closedEnv(home, append(v.env(token), "BLACKPOINT_CLI_PATH="+cliBin)...)
	out := mcpCall(t, mcpBin, env, nil, `{"name":"sync","arguments":{"resources":["tenants"]}}`)
	assertAbsent(t, "sync", out, token, "not started", "withheld from child")
	if v.hits.Load() == 0 {
		t.Fatalf("the child reached no vendor endpoint:\n%s", out)
	}
	if got, _ := v.auth.Load().(string); !strings.Contains(got, token) {
		t.Errorf("the child sent Authorization %q, want the handed token", got)
	}
}

// TestWholeRunNeverEmitsAHeldCredential drives every source and every sink: the
// fake API repeats every credential in every encoding, and every stdout, stderr
// and file outside the credential files is scanned in every form.
func TestWholeRunNeverEmitsAHeldCredential(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	secrets := []string{
		"envTok3n+A1b2C3d4/E5f6G7h8",
		"mcpHttpT0ken+0123456789/abcdefghijKLMNOP",
		"cfgAuthH3ader-Z1x2C3v4B5n6M7",
	}
	stored := "storedT0ken_Q9w8E7r6T5y4U3i2"
	all := append(slices.Clone(secrets), stored)
	echo := atomic.Bool{}
	item := `{"id":"t1","name":"Acme Dental","accountId":"a1","status":"OPEN","type":"MDR","alertCount":1,"created":"2026-10-01T00:00:00Z"`
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if !echo.Load() {
			fmt.Fprintf(w, `{"data":[%s}],"meta":{"skip":0,"take":50,"totalItems":1},"count":1}`, item)
			return
		}
		fields := echoFields(all)
		switch p := r.URL.Path; {
		case strings.Contains(p, "/vulnerability-management/scans"):
			w.Header().Set("X-Request-ID", base32.StdEncoding.EncodeToString([]byte(secrets[0])))
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"message":"rate limited"}`)
		case strings.Contains(p, "/vulnerability-management/vulnerabilities"):
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"message":"bad request","detail":{%s}}`, fields)
		case strings.HasSuffix(p, "/assets"):
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, "<html><head><title>%s</title></head></html>", base32.StdEncoding.EncodeToString([]byte(secrets[0])))
		default:
			fmt.Fprintf(w, `{"data":[%s,%s}],"meta":{"skip":0,"take":50,"totalItems":1},"count":1}`, item, fields)
		}
	})
	home := t.TempDir()
	env := closedEnv(home, append(v.env(secrets[0]), "BLACKPOINT_MCP_HTTP_TOKEN="+secrets[1])...)
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
	setToken := exec.Command(cliBin, "auth", "set-token")
	setToken.Env = closedEnv(home, "BLACKPOINT_BASE_URL="+v.url, "BLACKPOINT_CA_CERT="+v.caFile)
	setToken.Stdin = strings.NewReader(stored)
	out, err := setToken.CombinedOutput()
	if err != nil {
		t.Fatalf("auth set-token: %v\n%s", err, out)
	}
	scan("auth set-token", string(out))
	cfg := filepath.Join(home, "config", "config.toml")
	existing, _ := os.ReadFile(cfg)
	writeFile(t, cfg, string(existing)+fmt.Sprintf("\nauth_header = %q\n", "Bearer "+secrets[2]))

	if o := run(t, cliBin, env, "tenants", "list", "--json"); !strings.Contains(o, "Acme Dental") {
		t.Fatalf("control run did not reach the API:\n%s", o)
	}
	run(t, cliBin, env, "sync")
	echo.Store(true)
	for _, args := range [][]string{
		{"doctor", "--json"},
		{"auth", "status", "--json"},
		{"tenants", "list", "--json"},
		{"tenants", "list", "--agent"},
		{"tenants", "list", "--dry-run"},
		{"accounts", "list", "--json", "--deliver", "file:" + filepath.Join(home, "delivered.json")},
		{"sync"},
		{"fleet-sync", "--kinds", "detections", "--json"},
		{"detections", "list", "--x-tenant-id", "t1", "--json"},
		{"scans", "list", "--x-tenant-id", "t1", "--json"},
		{"vulnerabilities", "list", "--x-tenant-id", "t1", "--json"},
		{"assets", "list", "--x-tenant-id", "t1", "--json"},
		{"search", "Acme", "--json"},
		{"search", secrets[0], "--json"},
		{"recall", secrets[0], "--json"},
		{"recall", secrets[2], "--json"},
	} {
		scan(strings.Join(args[:min(2, len(args))], " "), run(t, cliBin, env, args...))
	}
	scan("MCP session", mcpCall(t, mcpBin, env, nil,
		`{"name":"blackpoint_read","arguments":{"endpoint_id":"tenants.list"}}`,
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
	cmd.Env = closedEnv(t.TempDir(), "BLACKPOINT_MCP_HTTP_TOKEN="+token)
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

// TestDeclarationsMatchTheHolderLists: the wiring checker reads the literal, so it
// repeats the lists the config scanner and the client's replacement use.
func TestDeclarationsMatchTheHolderLists(t *testing.T) {
	d := credwire.Declarations
	if !slices.Equal(d.ConfigFields, cliutil.CredentialConfigKeys) || !slices.Equal(d.StoredRecords, cliutil.CredentialConfigKeys) {
		t.Errorf("config fields %v and stored records %v, want %v", d.ConfigFields, d.StoredRecords, cliutil.CredentialConfigKeys)
	}
	for _, got := range [][]string{d.CredentialFields, d.ResponseFields, d.MCPArguments} {
		if !slices.Equal(got, cliutil.CredentialResponseFields) {
			t.Errorf("response fields %v, want %v", got, cliutil.CredentialResponseFields)
		}
	}
}

// TestCredentialKeySpellingIsHeld: go-toml matches a key to its field without case,
// so the holder reads names the same way. A credential under any spelling the
// decoder loads is held, in a file the run uses and in one it only holds.
func TestCredentialKeySpellingIsHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "CaseSpellSecret9z"
	for name, c := range map[string]struct{ file, body string }{
		"store-field":   {"data/credentials.toml", "AUTH_HEADER = \"Bearer " + secret + "\"\n"},
		"store-token":   {"data/credentials.toml", "Api_Token = \"" + secret + "\"\n"},
		"store-url":     {"data/credentials.toml", "BASE_URL = \"https://u:" + secret + "@api.example.test/v1\"\n"},
		"store-header":  {"data/credentials.toml", "[HEADERS]\nX-Custom = \"" + secret + "\"\n"},
		"config-field":  {"config/config.toml", "API_Token = \"" + secret + "\"\n"},
		"config-url":    {"config/config.toml", "BASE_URL = \"https://u:" + secret + "@api.example.test/v1\"\n"},
		"config-header": {"config/config.toml", "[HEADERS]\nX-Custom = \"" + secret + "\"\n"},
		"config-dotted": {"config/config.toml", "Headers.X-Custom = \"" + secret + "\"\n"},
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, filepath.FromSlash(c.file)), c.body)
			writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"p":{"name":"p","description":"`+secret+`","values":{}}}}`)
			env := closedEnv(home)
			assertAbsent(t, "this run's files", run(t, cliBin, env, "profile", "show", "p", "--json", "--no-learn"), secret)
			// With --config naming another file, the default config is only held.
			used := filepath.Join(t.TempDir(), "used.toml")
			writeFile(t, used, "# used\n")
			assertAbsent(t, "held only", run(t, cliBin, env, "--config", used, "profile", "show", "p", "--json", "--no-learn"), secret)
		})
	}
}

// TestEveryCredentialFileKeyIsClassified: a key config.Load reads is a credential,
// a named non-credential or a [headers] entry; a new field cannot go unread.
func TestEveryCredentialFileKeyIsClassified(t *testing.T) {
	known := append(slices.Clone(cliutil.CredentialConfigKeys), cliutil.NotCredentials...)
	known = append(known, "headers")
	for _, typ := range []reflect.Type{reflect.TypeOf(config.Config{}), reflect.TypeOf(cliutil.Credentials{})} {
		for i := 0; i < typ.NumField(); i++ {
			tag, _, _ := strings.Cut(typ.Field(i).Tag.Get("toml"), ",")
			if tag != "" && tag != "-" && !slices.Contains(known, tag) {
				t.Errorf("%s.%s: toml key %q is in neither cliutil.CredentialConfigKeys nor cliutil.NotCredentials", typ.Name(), typ.Field(i).Name, tag)
			}
		}
	}
}

// TestProfileNameIsData: a profile's name is the operator's data, whatever it reads
// like. Saving one named after a secret neither refuses later starts nor loses the
// profile.
func TestProfileNameIsData(t *testing.T) {
	cliBin, _ := binaries(t)
	for _, name := range []string{"auth", "acme-key", "secret", "token"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			env := closedEnv(home)
			if out, code := runCode(t, cliBin, env, "profile", "save", name, "--json", "--no-learn"); code != 0 {
				t.Fatalf("save: exit %d\n%s", code, out)
			}
			if out, code := runCode(t, cliBin, env, "version"); code != 0 || !strings.Contains(out, "blackpoint-cli") {
				t.Fatalf("version after save: exit %d\n%s", code, out)
			}
			if out, code := runCode(t, cliBin, env, "profile", "delete", name, "--yes"); code != 0 {
				t.Fatalf("delete: exit %d\n%s", code, out)
			}
		})
	}
}

// TestProfileHoldsOnlyWhatContractNames: a value under a credential flag's name or
// base-url is held, every duplicate included; a value under any other flag is not.
func TestProfileHoldsOnlyWhatContractNames(t *testing.T) {
	cliBin, _ := binaries(t)
	const (
		skey  = "profile-skey-value-9z7x5v"
		first = "profile-dup-first-3c1bZk"
		base  = "profile-base-pass-8m2n4p"
	)
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"Profiles":{"secret":{"name":"secret","VALUES":{
		"skey":"`+skey+`","SKEY":"`+first+`","SKEY":"","base-url":"https://u:`+base+`@h.example/",
		"select":"`+skey+` `+first+` `+base+`","limit":"nightly-plain-value-77"}}}}`)
	out := run(t, cliBin, closedEnv(home), "profile", "show", "secret", "--json", "--no-learn")
	assertAbsent(t, "profile show", out, skey, first, base)
	writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"p":{"name":"p","values":{"limit":"nightly-plain-value-77"}}}}`)
	if out := run(t, cliBin, closedEnv(home), "profile", "show", "p", "--json", "--no-learn"); !strings.Contains(out, "nightly-plain-value-77") {
		t.Errorf("a plain value is gone:\n%s", out)
	}
}

// TestStartupFlagTheParserDidNotUseIsRefused: startup reads --config and --home by
// every occurrence, and the kit takes the allowed origin from what it reads. A flag
// that takes a value can swallow one (--select --config x), so a file the run never
// uses could name the host a credential may go to. Such a run stops before it acts.
func TestStartupFlagTheParserDidNotUseIsRefused(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "swallow-fixture-value"
	// Any connection to the decoy means the kit let a request carrying the token go
	// to a host only an unused flag named.
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
	writeFile(t, usedFile, "base_url = \"https://api.blackpointcyber.com/v1\"\n")
	writeFile(t, decoyFile, "base_url = \""+decoyURL+"\"\n")
	deliver := "webhook:" + decoyURL + "/hook"
	env := closedEnv(home, "BLACKPOINT_API_TOKEN="+token, "BLACKPOINT_USER_AGENT="+token, "HTTPS_PROXY=", "NO_PROXY=127.0.0.1")
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
			assertAbsent(t, "output", out, token)
		})
	}
	// A --config the parser uses, written once, is not refused.
	if out, code := runCode(t, cliBin, env, "--config", usedFile, "version", "--no-learn"); code != 0 {
		t.Errorf("an ordinary --config: exit %d\n%s", code, out)
	}
}

// TestMCPRefusesAConfigFlagItDoesNotHave: blackpoint-mcp has no --config or --home,
// but startup reads every occurrence and the kit takes the allowed origin from it. A
// flag that takes a value (--addr --config x) or a stray positional hides one from
// the parser, so the server refuses to start rather than run on a file it never uses.
func TestMCPRefusesAConfigFlagItDoesNotHave(t *testing.T) {
	_, mcpBin := binaries(t)
	home := t.TempDir()
	used := filepath.Join(home, "used.toml")
	decoy := filepath.Join(home, "decoy.toml")
	writeFile(t, used, "base_url = \"https://api.blackpointcyber.com/v1\"\n")
	writeFile(t, decoy, "base_url = \"https://collector.example\"\n")
	env := closedEnv(home, "BLACKPOINT_CONFIG="+used, "BLACKPOINT_API_TOKEN=mcp-fixture-value")
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
