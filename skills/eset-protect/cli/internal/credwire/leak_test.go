// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is one the review's failing cases named. Set
// ESET_PROTECT_LEAK_BIN_DIR to run them against binaries built elsewhere (how the
// pre-kit build was seen failing: .review/classes-fail-first.txt).

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
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

	"eset-protect-pp-cli/internal/cli"
	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/credkit"
	"eset-protect-pp-cli/internal/credwire"

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
		if d := os.Getenv("ESET_PROTECT_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "eset-protect-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{{"eset-protect-cli", "../../cmd/eset-protect-cli"}, {"eset-protect-mcp", "../../cmd/eset-protect-mcp"}} {
			if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, b.out), b.pkg).CombinedOutput(); err != nil {
				binErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, "eset-protect-cli"), filepath.Join(binDir, "eset-protect-mcp")
}

// vendor is a TLS stub of ESET Connect: the kit never sends a credential
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
	return []string{"ESET_PROTECT_BASE_URL=" + v.url, "ESET_PROTECT_CA_CERT=" + v.caFile, "ESET_PROTECT_TOKEN=" + key}
}

// closedEnv is a closed environment under one sandbox home.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "ESET_PROTECT_HOME=" + home, "TMPDIR=" + os.TempDir(), "NO_COLOR=1"}, extra...)
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
	base := closedEnv(home, "ESET_PROTECT_BASE_URL=https://u:"+short+"@eu.device-management.eset.systems")
	assertAbsent(t, "CLI flag error", run(t, cliBin, base, "--timeout", short, "version"), short)
	assertAbsent(t, "MCP flag error", run(t, mcpBin, base, "--transport", short), short)

	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"devices":[{"uuid":"d1","displayName":%q}],"nextPageToken":""}`, short)
	})
	home = t.TempDir()
	env := closedEnv(home, v.env(short)...)
	assertAbsent(t, "devices list", run(t, cliBin, env, "devices", "list", "--json"), short)
	assertAbsent(t, "sync", run(t, cliBin, env, "sync"), short)
	assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, nil, `{"name":"eset-protect_read","arguments":{"endpoint_id":"devices.list"}}`), short)
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
		{"comment-glued-to-quote", "protect_token = \"eset_live_glued0101\"#longcomment\nbroken = = longenoughvalue\n", "eset_live_glued0101"},
		{"inline-table-comma", "headers = { X-Api-Key = \",eset_live_comma123\" }\nbroken = = longenoughvalue\n", ",eset_live_comma123"},
		{"unterminated-array", "extra = [\"eset_live_array8910\",\nprotect_token = \"eset_live_after1112\"\n", "eset_live_array8910"},
		{"key-after-unterminated-array", "extra = [\"filler-value\",\nprotect_token = \"eset_live_after1314\"\n", "eset_live_after1314"},
		{"multi-line-string", "access_token = \"\"\"\neset_live_multi1516\n\"\"\"\nbroken = = longenoughvalue\n", "eset_live_multi1516"},
		{"unterminated-multi-line", "note = \"\"\"\nprotect_token = \"eset_live_inside1718\"\n", "eset_live_inside1718"},
		{"escaped-quote-raw", "protect_token = \"eset_live\\\"esc1920\"\nbroken = = longenoughvalue\n", `eset_live\"esc1920`},
		{"escaped-quote-decoded", "protect_token = \"eset_live\\\"esc2122\"\nbroken = = longenoughvalue\n", `eset_live"esc2122`},
		{"credential-all-punctuation", "protect_token = \"!!--!!\"\nbroken = = longenoughvalue\n", "!!--!!"},
		{"bare-value", "client_secret = eset_live_bare2526\nbroken = = longenoughvalue\n", "eset_live_bare2526"},
		{"hash-in-bare-value", "client_secret = eset_live#hash2728\nbroken = = longenoughvalue\n", "eset_live#hash2728"},
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
	const secret = "eset_live_glued0101"
	writeFile(t, filepath.Join(home, "config", "config.toml"), "protect_token = \""+secret+"\"#c\n")
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
	const secret = "eset_live_relocated2930"
	for name, args := range map[string]func(string) []string{
		"home-after-dashdash-value": func(h string) []string { return []string{"--select", "--", "--home", h, "--" + secret} },
		"home-equals":               func(h string) []string { return []string{"--home=" + h, "--" + secret} },
		"home-as-a-value":           func(h string) []string { return []string{"--select", "--home", h, "--" + secret} },
	} {
		t.Run(name, func(t *testing.T) {
			home, other := t.TempDir(), t.TempDir()
			writeFile(t, filepath.Join(other, "config", "config.toml"), `protect_token = "`+secret+`"`+"\n")
			assertAbsent(t, name, run(t, cliBin, closedEnv(home), args(other)...), secret)
		})
	}
}

// TestDiscoveryCoversEveryFileLoadReads: a credential in any file a run could read
// is held before a flag error prints.
func TestDiscoveryCoversEveryFileLoadReads(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "eset_live_discover3132"
	files := map[string]func(home string) string{
		"explicit-config":  func(h string) string { return filepath.Join(h, "tenant", "config.toml") },
		"global-store":     func(h string) string { return filepath.Join(h, "data", "credentials.toml") },
		"global-config":    func(h string) string { return filepath.Join(h, "config", "config.toml") },
		"legacy-config":    func(h string) string { return filepath.Join(h, ".config", "eset-protect-cli", "config.toml") },
		"home-flag-store":  func(h string) string { return filepath.Join(h, "other", "data", "credentials.toml") },
		"home-flag-config": func(h string) string { return filepath.Join(h, "other", "config", "config.toml") },
	}
	for name, path := range files {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, path(home), `protect_token = "`+secret+`"`+"\n")
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
	env := closedEnv(home, "ESET_PROTECT_TOKEN="+token)
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
		fmt.Fprintf(w, `{"devices":[{"uuid":"d1","displayName":"host","description":%q}],"nextPageToken":""}`, desc)
	})
	home := t.TempDir()
	out := run(t, cliBin, closedEnv(home, v.env(token)...), "devices", "list", "--json", "--no-cache", "--no-learn")
	assertAbsent(t, "devices list", out, enc.String())
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
	out := run(t, cliBin, closedEnv(home, v.env(token)...), "export", "devices", "d1", "--output", dest, "--no-cache")
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
		io.WriteString(w, `{"devices":[{"uuid":"d1","displayName":"Acme"}],"nextPageToken":""}`)
	})
	sink := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	home := t.TempDir()
	env := closedEnv(home, v.env(token)...)
	env = append(env, "ESET_PROTECT_FEEDBACK_ENDPOINT="+sink.url+"/receipt/"+token, "SSL_CERT_FILE="+sink.caFile)
	assertAbsent(t, "feedback", run(t, cliBin, env, "feedback", "harmless", "--send"), token)
	if n := sink.hits.Load(); n != 0 {
		t.Errorf("feedback: %d requests carried the token to another origin", n)
	}
	env = closedEnv(home, append(v.env(token), "ESET_PROTECT_USER_AGENT="+token, "SSL_CERT_FILE="+sink.caFile)...)
	assertAbsent(t, "webhook", run(t, cliBin, env, "devices", "list", "--json", "--deliver", "webhook:"+sink.url+"/hook"), token)
	if n := sink.hits.Load(); n != 0 {
		t.Errorf("webhook: %d requests carried the token to another origin", n)
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

// TestStdinBodyIsHeldBeforeAPreview: a --stdin body's credential fields are held
// before a dry run prints them, on the commands that take one.
func TestStdinBodyIsHeldBeforeAPreview(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "stdin-only-value-4567"
	home := t.TempDir()
	cmd := exec.Command(cliBin, "device-tasks", "create", "--stdin", "--dry-run")
	cmd.Env = closedEnv(home, "ESET_PROTECT_TOKEN=eset_live_token_33344455")
	cmd.Stdin = strings.NewReader(`{"displayName":"t","password":"` + secret + `"}`)
	out, _ := cmd.CombinedOutput()
	assertAbsent(t, "--stdin --dry-run", string(out), secret)
	assertNoFileCarries(t, home, nil, secret)
}

// TestCredentialFieldsReplacedByName: a device's identity token is replaced by
// name before any sink and the row's other fields stay usable; the token grant's
// fields, which no row carries, are held, so a row repeating one is withheld.
func TestCredentialFieldsReplacedByName(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const devToken = "device-identity-token-98765"
	const access = "access-token-in-a-row-4321"
	row := func(field, value string) *vendor {
		return startVendor(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"devices":[{"uuid":"d1","displayName":"FRONTDESK-01",%q:%q}],"nextPageToken":""}`, field, value)
		})
	}
	home := t.TempDir()
	env := closedEnv(home, row("deviceToken", devToken).env("eset_live_token_33344455")...)
	out := run(t, cliBin, env, "devices", "list", "--json", "--no-learn")
	assertAbsent(t, "list", out, devToken)
	if !strings.Contains(out, "FRONTDESK-01") {
		t.Errorf("the row's other fields are gone:\n%s", out)
	}
	out = mcpCall(t, mcpBin, env, nil, `{"name":"eset-protect_read","arguments":{"endpoint_id":"devices.list"}}`)
	assertAbsent(t, "MCP read", out, devToken)
	if !strings.Contains(out, "FRONTDESK-01") {
		t.Errorf("MCP: the row's other fields are gone:\n%s", out)
	}
	assertNoFileCarries(t, home, nil, devToken)

	home = t.TempDir()
	env = closedEnv(home, row("access_token", access).env("eset_live_token_33344455")...)
	assertAbsent(t, "list", run(t, cliBin, env, "devices", "list", "--json", "--no-learn"), access)
	assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, nil, `{"name":"eset-protect_read","arguments":{"endpoint_id":"devices.list"}}`), access)
	assertNoFileCarries(t, home, nil, access)
}

// TestRunProfileCarriesOnlyTheAllowList: a store's value under a credential-named
// or undeclared key never prints, and a declared credential field is held from
// startup. A URL inside a description is free text, which contract 1 does not
// name, so no reader of it is required.
func TestRunProfileCarriesOnlyTheAllowList(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "profiles.json"), `{"profiles":{"p":{"name":"p","description":"nightly","values":{
		"limit":"7",
		"password":"profile-password-1234567",
		"my-notes":"eset-free-text-1234567"
	}}}}`)
	env := closedEnv(home)
	out := run(t, cliBin, env, "profile", "show", "p", "--json", "--no-learn")
	assertAbsent(t, "profile show", out, "profile-password-1234567", "eset-free-text-1234567")
	if !strings.Contains(out, `"limit"`) {
		t.Errorf("an allowed value was dropped:\n%s", out)
	}
	assertAbsent(t, "flag error", run(t, cliBin, env, "--timeout", "profile-password-1234567", "version"), "profile-password-1234567")
	out = run(t, cliBin, env, "profile", "save", "bad", "--password", "profile-save-value-1234567", "--no-learn")
	assertAbsent(t, "profile save", out, "profile-save-value-1234567")
	assertNoFileCarries(t, home, nil, "profile-save-value-1234567")
}

// TestUsedConfigDecidesTheAllowedOrigin: the config the program really uses names
// the origin a credential may go to. A discovered default's base_url is only held,
// so a credential it holds never goes there.
func TestUsedConfigDecidesTheAllowedOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "eset_live_origin_44455566"
	used := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"devices":[],"nextPageToken":""}`)
	})
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "config.toml"), "base_url = \""+other.url+"\"\n")
	explicit := filepath.Join(home, "tenant.toml")
	writeFile(t, explicit, "base_url = \""+used.url+"\"\nprotect_token = \""+token+"\"\n")
	env := closedEnv(home, "ESET_PROTECT_CA_CERT="+used.caFile)
	out := run(t, cliBin, env, "--config", explicit, "devices", "list", "--json", "--no-learn", "--no-cache")
	assertAbsent(t, "devices list", out, token)
	if used.hits.Load() == 0 {
		t.Errorf("the explicit config's base URL was refused or unused:\n%s", out)
	}
	if n := other.hits.Load(); n != 0 {
		t.Errorf("%d requests went to the discovered default's base URL", n)
	}
}

// TestChildGetsTheTokenThroughTheKit: an MCP mirror tool runs the CLI, and the kit
// starts no child with a held value in its environment. The token reaches the vendor
// through the kit's own pipe to its companion.
func TestChildGetsTheTokenThroughTheKit(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const token = "eset_live_child_4142434"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"devices":[],"nextPageToken":""}`)
	})
	home := t.TempDir()
	env := closedEnv(home, append(v.env(token), "ESET_PROTECT_CLI_PATH="+cliBin)...)
	out := mcpCall(t, mcpBin, env, nil, `{"name":"sync","arguments":{"resources":["devices"],"confirm":true}}`)
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
	item := `{"uuid":"d1","displayName":"Acme Dental","groupUuid":"g1"`
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if !echo.Load() {
			fmt.Fprintf(w, `{"devices":[%s}],"nextPageToken":""}`, item)
			return
		}
		fields := echoFields(all)
		switch p := r.URL.Path; {
		case strings.Contains(p, "/incidents"):
			w.Header().Set("X-Request-ID", base32.StdEncoding.EncodeToString([]byte(secrets[0])))
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"message":"rate limited"}`)
		case strings.Contains(p, "/detections"):
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"message":"bad request","detail":{%s}}`, fields)
		case strings.HasSuffix(p, "/device-groups"):
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, "<html><head><title>%s</title></head></html>", base32.StdEncoding.EncodeToString([]byte(secrets[0])))
		default:
			fmt.Fprintf(w, `{"devices":[%s,%s}],"nextPageToken":""}`, item, fields)
		}
	})
	home := t.TempDir()
	env := closedEnv(home, append(v.env(secrets[0]), "ESET_PROTECT_MCP_HTTP_TOKEN="+secrets[1])...)
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
	setToken.Env = closedEnv(home, "ESET_PROTECT_BASE_URL="+v.url, "ESET_PROTECT_CA_CERT="+v.caFile)
	setToken.Stdin = strings.NewReader(stored)
	out, err := setToken.CombinedOutput()
	if err != nil {
		t.Fatalf("auth set-token: %v\n%s", err, out)
	}
	scan("auth set-token", string(out))
	cfg := filepath.Join(home, "config", "config.toml")
	existing, _ := os.ReadFile(cfg)
	writeFile(t, cfg, string(existing)+fmt.Sprintf("\nauth_header = %q\n", "Bearer "+secrets[2]))

	if o := run(t, cliBin, env, "devices", "list", "--json"); !strings.Contains(o, "Acme Dental") {
		t.Fatalf("control run did not reach the API:\n%s", o)
	}
	run(t, cliBin, env, "sync")
	echo.Store(true)
	for _, args := range [][]string{
		{"doctor", "--json"},
		{"auth", "status", "--json"},
		{"devices", "list", "--json"},
		{"devices", "list", "--agent"},
		{"devices", "list", "--dry-run"},
		{"users", "list", "--json", "--deliver", "file:" + filepath.Join(home, "delivered.json")},
		{"sync"},
		{"fleet", "health", "--json"},
		{"detections", "list", "--json"},
		{"incidents", "list", "--json"},
		{"device-groups", "--json"},
		{"search", "Acme", "--json"},
		{"search", secrets[0], "--json"},
		{"recall", secrets[0], "--json"},
		{"recall", secrets[2], "--json"},
	} {
		scan(strings.Join(args[:min(2, len(args))], " "), run(t, cliBin, env, args...))
	}
	scan("MCP session", mcpCall(t, mcpBin, env, nil,
		`{"name":"eset-protect_read","arguments":{"endpoint_id":"devices.list"}}`,
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
	token := strings.Repeat("mcp-bearer-4344-", 3)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, mcpBin, "--transport", "http", "--addr", addr)
	cmd.Env = closedEnv(t.TempDir(), "ESET_PROTECT_MCP_HTTP_TOKEN="+token)
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
			// --page-token is ESET's pagination cursor, not a credential.
			if f.Value.Type() == "string" && f.Name != "page-token" && cliutil.SecretProfileKey(f.Name) && !slices.Contains(declared, "--"+f.Name) {
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
		t.Fatalf("no sources: %v", err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "io.ReadAll(os.Stdin)") {
			t.Errorf("%s reads a body without holding it: use cliutil.ReadRequestBody", f)
		}
	}
}

// TestStdinBodyPrettyAndEchoed: a pretty-printed body (one member per line) starts,
// its credential is held, and a vendor answer that repeats the body's plain fields
// still prints, because the write happened.
func TestStdinBodyPrettyAndEchoed(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "stdin-pretty-password-3210"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"deviceTask":{"uuid":"t1","displayName":"Patch Acme Dental"}}`)
	})
	home := t.TempDir()
	cmd := exec.Command(cliBin, "device-tasks", "create", "--stdin", "--json", "--yes", "--no-learn")
	cmd.Env = closedEnv(home, v.env("eset_live_token_33344455")...)
	cmd.Stdin = strings.NewReader("{\n  \"displayName\": \"Patch Acme Dental\",\n  \"password\": \"" + secret + "\"\n}\n")
	out, _ := cmd.CombinedOutput()
	if cmd.ProcessState.ExitCode() == credkit.ExitRefused {
		t.Fatalf("a pretty-printed body refused the start:\n%s", out)
	}
	assertAbsent(t, "--stdin", string(out), secret)
	if !strings.Contains(string(out), "Patch Acme Dental") {
		t.Errorf("the answer to a write that happened is gone:\n%s", out)
	}
	assertNoFileCarries(t, home, nil, secret)
}

// TestComposedEncodingsAreHeld is the review's composed-encoding input: the
// password's ROT13 in fullwidth characters, in a list row and in an error body, on
// every sink. The review's spaced form is left out: no reading in
// docs/credential-leak-contract.md removes spaces before ROT13 (a P2 at most;
// .review/kit-findings.md).
func TestComposedEncodingsAreHeld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const password = "P@ss-W0!"
	for name, shown := range map[string]string{
		"fullwidth-rot13": "\uff23\uff20\uff46\uff46\uff0d\uff2a\uff10\uff01",
	} {
		t.Run(name, func(t *testing.T) {
			v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/incidents") {
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprintf(w, `{"error":"invalid_grant","error_description":%q}`, shown)
					return
				}
				fmt.Fprintf(w, `{"devices":[{"uuid":"d1","displayName":%q}],"nextPageToken":""}`, shown)
			})
			home := t.TempDir()
			env := closedEnv(home, append(v.env("eset_live_token_33344455"), "ESET_PROTECT_USERNAME=api@msp.example", "ESET_PROTECT_PASSWORD="+password)...)
			assertAbsent(t, "devices list", run(t, cliBin, env, "devices", "list", "--json", "--no-learn"), shown, password)
			assertAbsent(t, "incidents list", run(t, cliBin, env, "incidents", "list", "--json", "--no-learn"), shown, password)
			assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, nil, `{"name":"eset-protect_read","arguments":{"endpoint_id":"devices.list"}}`), shown, password)
			assertNoFileCarries(t, home, nil, shown, password)
		})
	}
}

// TestStartupDiagnosticsNeverShowAConfigCredential is the review's startup input:
// the parser's own errors, before any command runs, from the environment, from
// a config the argv names twice, and from a malformed file whose TOML escape
// decodes to the credential.
func TestStartupDiagnosticsNeverShowAConfigCredential(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	t.Run("env-then-mcp-transport-env", func(t *testing.T) {
		const secret = "StartupSecret123!"
		env := closedEnv(t.TempDir(), "ESET_PROTECT_PASSWORD="+secret, "PP_MCP_TRANSPORT="+secret)
		assertAbsent(t, "MCP start", run(t, mcpBin, env), secret)
	})
	t.Run("config-named-twice", func(t *testing.T) {
		const secret = "CfgOnlySecret456?"
		home := t.TempDir()
		private, public := filepath.Join(home, "private.toml"), filepath.Join(home, "public.toml")
		writeFile(t, private, "protect_password = \""+secret+"\"\n")
		writeFile(t, public, "")
		out := run(t, cliBin, closedEnv(home), "--config", private, "--select", "--config="+public, "--timeout", secret, "doctor")
		assertAbsent(t, "two configs", out, secret)
	})
	t.Run("malformed-toml-escape", func(t *testing.T) {
		const secret = "CfgOnlySecret456?"
		home := t.TempDir()
		broken := filepath.Join(home, "broken.toml")
		writeFile(t, broken, "protect_password = \"Cfg\\u004fnlySecret456?\"\nbroken = =\n")
		assertAbsent(t, "malformed", run(t, cliBin, closedEnv(home), "--config", broken, "--timeout", secret, "doctor"), secret)
	})
}

// TestQuarantineDownloadHoldingAPasswordIsWithheld is the review's binary input:
// a download whose bytes hold the password never reaches the delivered file.
func TestQuarantineDownloadHoldingAPasswordIsWithheld(t *testing.T) {
	cliBin, _ := binaries(t)
	const password = "longSecret123!"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		io.WriteString(w, "PK\x03\x04"+password)
	})
	home := t.TempDir()
	dest := filepath.Join(home, "sample.zip")
	env := closedEnv(home, append(v.env("eset_live_token_33344455"), "ESET_PROTECT_USERNAME=api@msp.example", "ESET_PROTECT_PASSWORD="+password)...)
	out := run(t, cliBin, env, "quarantined-objects-download", "--deliver", "file:"+dest)
	assertAbsent(t, "download", out, password)
	assertNoFileCarries(t, home, nil, password)
	if v.hits.Load() == 0 {
		t.Fatalf("the download never reached the vendor:\n%s", out)
	}
	stdout, code := runCode(t, cliBin, env, "quarantined-objects-download")
	assertAbsent(t, "download to stdout", stdout, password)
	if code == 0 || !strings.Contains(stdout, "withheld") {
		t.Errorf("a withheld download to stdout: exit %d, want nonzero, and a notice:\n%s", code, stdout)
	}
	// The control: a download that holds no credential arrives byte for byte.
	clean := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		io.WriteString(w, "PK\x03\x04clean\xff\xfe")
	})
	env = closedEnv(home, append(clean.env("eset_live_token_33344455"), "ESET_PROTECT_USERNAME=api@msp.example", "ESET_PROTECT_PASSWORD="+password)...)
	out = run(t, cliBin, env, "quarantined-objects-download", "--deliver", "file:"+dest)
	if got, err := os.ReadFile(dest); err != nil || string(got) != "PK\x03\x04clean\xff\xfe" {
		t.Errorf("a clean download did not arrive byte for byte: %q %v\n%s", got, err, out)
	}
}

// TestRegionPinKeepsATokenInItsDataCenter: a token configured for one region is
// never sent to another region's hosts, whatever a path context says.
func TestRegionPinKeepsATokenInItsDataCenter(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "eset_live_region_55566677"
	home := t.TempDir()
	env := closedEnv(home, "ESET_PROTECT_REGION=us", "ESET_PROTECT_TOKEN="+token)
	out := run(t, cliBin, env, "sync", "--resources", "devices", "--path-context", "region=eu")
	assertAbsent(t, "sync", out, token)
	if !strings.Contains(out, "region") {
		t.Errorf("the mismatch was not named:\n%s", out)
	}
}

// TestSetTokenStdinIsHeldBeforeFlagsParse is the review's input: the token read from
// the pipe repeated in a flag whose parse error echoes it, before RunE can read it.
func TestSetTokenStdinIsHeldBeforeFlagsParse(t *testing.T) {
	cliBin, _ := binaries(t)
	secret := "StdinOnly" + "Secret789!"
	home := t.TempDir()
	cmd := exec.Command(cliBin, "auth", "set-token", "--timeout", secret)
	cmd.Env = closedEnv(home)
	cmd.Stdin = strings.NewReader(secret + "\n")
	out, _ := cmd.CombinedOutput()
	if len(out) == 0 {
		t.Fatal("expected a flag error")
	}
	assertAbsent(t, "set-token flag error", string(out), secret)

	// The bytes still reach the command: set-token saves the token.
	cmd = exec.Command(cliBin, "auth", "set-token")
	cmd.Env = closedEnv(home)
	cmd.Stdin = strings.NewReader(secret + "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("set-token: %v\n%s", err, out)
	}
	var saved int
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), secret) {
				saved++
			}
		}
		return nil
	})
	if saved == 0 {
		t.Error("set-token did not save the token it read")
	}
}

// TestJSONFlagCredentialIsHeldInADryRun is the review's input: a credential member of
// a JSON flag value, shown by the dry-run preview.
func TestJSONFlagCredentialIsHeldInADryRun(t *testing.T) {
	cliBin, _ := binaries(t)
	secret := "FlagOnly" + "Secret123!"
	for _, args := range [][]string{
		{"--task-action-params", `{"password":"` + secret + `"}`},
		{`--task-action-params={"nested":{"Password":"` + secret + `"}}`},
	} {
		out, _ := runCode(t, cliBin, closedEnv(t.TempDir(), "ESET_PROTECT_TOKEN=eset_live_token_33344455"),
			append([]string{"device-tasks", "create", "--dry-run", "--no-learn"}, args...)...)
		if !strings.Contains(out, "params") && !strings.Contains(out, "withheld") {
			t.Fatalf("expected the dry-run preview or the kit's notice: %s", out)
		}
		assertAbsent(t, "dry-run", out, secret)
	}
}

// TestMCPCredentialArgumentsAreHeldInAPreview is the review's input, plus the same
// value in a JSON string and under a name MCPArguments does not list. A page cursor
// is not a credential: the answer that repeats it must still arrive.
func TestMCPCredentialArgumentsAreHeldInAPreview(t *testing.T) {
	_, mcpBin := binaries(t)
	secret := "McpOnly" + "Secret123!"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"devices":[],"nextPageToken":"cursor-41"}`)
	})
	env := closedEnv(t.TempDir(), v.env("eset_live_token_33344455")...)
	for _, params := range []string{
		`{"password":"` + secret + `"}`,
		`{"client_secret":"` + secret + `"}`,
		`{"task-action-params":"{\"password\":\"` + secret + `\"}"}`,
	} {
		out := mcpCall(t, mcpBin, env, nil,
			`{"name":"eset-protect_execute","arguments":{"endpoint_id":"device-tasks.create","params":`+params+`}}`)
		assertAbsent(t, "preview of "+params[:12], out, secret)
	}
	out := mcpCall(t, mcpBin, env, nil,
		`{"name":"eset-protect_read","arguments":{"endpoint_id":"devices.list","params":{"pageToken":"cursor-41"}}}`)
	if !strings.Contains(out, "cursor-41") {
		t.Errorf("a page cursor was withheld:\n%s", out)
	}
}

// TestMCPArgumentsAreHeldAsTheCallWroteThem is the review's inputs: decoding rounds a
// large number and keeps only the last of a duplicate key, so a hold that reads the
// decoded arguments misses the value the preview then shows. A field is a credential
// because Declarations names it, never because its name looks like one (contract 7).
func TestMCPArgumentsAreHeldAsTheCallWroteThem(t *testing.T) {
	_, mcpBin := binaries(t)
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	env := closedEnv(t.TempDir(), v.env("eset_live_token_33344455")...)
	const number = "99999999999999999999"
	dup := "DupOnly" + "Secret123!"
	for _, params := range []string{
		`{"client_secret":` + number + `,"description":"x"}`,
		`{"client_secret":"` + dup + `","client_secret":null,"description":"` + dup + `"}`,
	} {
		out := mcpCall(t, mcpBin, env, nil,
			`{"name":"eset-protect_execute","arguments":{"endpoint_id":"device-tasks.create","params":`+params+`}}`)
		assertAbsent(t, "preview of "+params, out, number, dup)
	}
}

// TestUnconfirmedMirroredWriteTouchesNothing is the review's input: a sync preview
// must not create the store or migrate it, and neither may any other mirrored write,
// so a preview starts no child at all.
func TestUnconfirmedMirroredWriteTouchesNothing(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{"devices":[]}`) })
	home := t.TempDir()
	env := closedEnv(home, append(v.env("eset_live_token_33344455"), "ESET_PROTECT_CLI_PATH="+cliBin)...)
	out := mcpCall(t, mcpBin, env, nil,
		`{"name":"sync","arguments":{}}`,
		`{"name":"sync","arguments":{"resources":["devices"],"dry-run":true}}`)
	if n := strings.Count(out, "confirmation_required"); n != 2 {
		t.Errorf("%d of 2 calls previewed:\n%s", n, out)
	}
	_ = filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(d.Name(), ".db") {
			t.Errorf("an unconfirmed sync created %s", path)
		}
		return nil
	})
	if got := v.hits.Load(); got != 0 {
		t.Errorf("a preview reached the vendor (%d requests)", got)
	}
}

// TestOwnerMarkerPassesTheKit is the review's input: a password equal to the computed
// scope hash must not land in the legacy database's owner marker.
func TestOwnerMarkerPassesTheKit(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	unscoped := filepath.Join(home, "data", "data.db")
	if err := os.MkdirAll(filepath.Dir(unscoped), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unscoped, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	const password = "ac5619c0a13a"
	runCode(t, cliBin, closedEnv(home, append(v.env("eset_live_token_33344455"), "ESET_PROTECT_PASSWORD="+password)...),
		"fleet", "health", "--no-learn")
	if b, err := os.ReadFile(unscoped + ".owner"); err == nil && strings.Contains(string(b), password) {
		t.Errorf("the owner marker holds the password")
	}
	assertNoFileCarries(t, home, nil, password)
}

// TestMirroredWritesPreviewOnTheRealBinary: without confirm, learnings_forget --all
// (the review's input) deletes nothing, and no mirrored write reaches the vendor.
func TestMirroredWritesPreviewOnTheRealBinary(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{"devices":[]}`) })
	home := t.TempDir()
	env := closedEnv(home, v.env("eset_live_token_33344455")...)
	run(t, cliBin, env, "teach", "--query", "find stale devices", "--resource-type", "devices", "--resource", "d1")
	before := run(t, cliBin, env, "learnings", "list", "--json")
	if !strings.Contains(before, "find stale devices") {
		t.Fatalf("seed: %s", before)
	}
	hits := v.hits.Load()
	calls := []string{
		`{"name":"learnings_forget","arguments":{"query":"find stale devices","all":true}}`,
		`{"name":"sync","arguments":{}}`,
		`{"name":"teach","arguments":{"query":"another query","resource-type":"devices","resource":"d2"}}`,
	}
	out := mcpCall(t, mcpBin, env, nil, calls...)
	if n := strings.Count(out, "confirmation_required"); n != len(calls) {
		t.Errorf("%d of %d calls previewed:\n%s", n, len(calls), out)
	}
	if got := v.hits.Load(); got != hits {
		t.Errorf("a preview reached the vendor (%d requests)", got-hits)
	}
	after := run(t, cliBin, env, "learnings", "list", "--json")
	if !strings.Contains(after, "find stale devices") || strings.Contains(after, "another query") {
		t.Errorf("a preview changed the learn store: %s", after)
	}
	mcpCall(t, mcpBin, env, nil, `{"name":"learnings_forget","arguments":{"query":"find stale devices","all":true,"confirm":true}}`)
	if after := run(t, cliBin, env, "learnings", "list", "--json"); strings.Contains(after, "find stale devices") {
		t.Errorf("confirm:true did not run it: %s", after)
	}
}

// TestHTTPSModeStarts: a real TLS key must not refuse the server. Holding the key's
// raw DER did, on every key form, with nothing on stderr.
func TestHTTPSModeStarts(t *testing.T) {
	_, mcpBin := binaries(t)
	home := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}}
	cert, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certFile, keyFile := filepath.Join(home, "c.pem"), filepath.Join(home, "k.pem")
	writeFile(t, certFile, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert})))
	writeFile(t, keyFile, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, mcpBin, "--transport", "http", "--addr", addr, "--tls-cert", certFile, "--tls-key", keyFile)
	cmd.Env = closedEnv(home, "ESET_PROTECT_MCP_HTTP_TOKEN="+strings.Repeat("https-mode-bearer-", 3))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	pool := x509.NewCertPool()
	parsed, _ := x509.ParseCertificate(cert)
	pool.AddCert(parsed)
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-exited:
			t.Fatalf("the HTTPS server exited: %v", err)
		default:
		}
		resp, err := client.Post("https://"+addr+"/mcp", "application/json", strings.NewReader(`{}`))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("no token: %d", resp.StatusCode)
			}
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("the HTTPS server never answered")
}

// TestDocumentedRegionConfigStarts: governance.md's [template_vars] region setting is
// an identifier; held as a credential it collides with public text and refuses start.
func TestDocumentedRegionConfigStarts(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	cfg := filepath.Join(home, "config", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, cfg, "[template_vars]\nregion = \"us\"\n")
	out, code := runCode(t, cliBin, closedEnv(home), "version")
	if code == credkit.ExitRefused || !strings.Contains(out, "eset-protect-cli") {
		t.Errorf("rc %d: %s", code, out)
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
