// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class's failing input, seen
// failing on the binaries built before the kit was wired. Set
// CORK_LEAK_BIN_DIR to run them against binaries built elsewhere.

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

	"cork-pp-cli/internal/credkit"
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
		if d := os.Getenv("CORK_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "cork-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{exe("cork-cli"), "../../cmd/cork-cli"},
			{exe("cork-mcp"), "../../cmd/cork-mcp"},
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
	return filepath.Join(binDir, exe("cork-cli")), filepath.Join(binDir, exe("cork-mcp"))
}

// vendor is a TLS stub of the Cork API: the kit never sends a credential
// over plain http, loopback included.
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

func (v *vendor) env(token string) []string {
	return []string{"CORK_BASE_URL=" + v.url, "CORK_CA_CERT=" + v.caFile, "CORK_API_KEY=" + token}
}

// closedEnv is a closed environment under one sandbox home; every press
// directory resolves under it.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "CORK_HOME=" + home,
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

// TestAPIErrorEchoOfKeyIsWithheld (class key-echo, the 0.1.8 secret-echo
// funnel's job): an API error body that echoes CORK_API_KEY whole, a piece of
// it, base64 or URL-encoded. The press masks replace the whole key; a server
// echoing a piece or an encoding slipped past them. Any 8-character piece of a
// held value, after every decoding the contract names, withholds the output.
func TestAPIErrorEchoOfKeyIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const key = "corkkey-" + "Zq81LmP0xV7cR2"
	piece := key[:12]
	b64 := base64.StdEncoding.EncodeToString([]byte(key))
	esc := url.QueryEscape("k=" + key)
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":"key %s... rejected","debug":"%s","echo":"%s"}`, piece, b64, esc)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env(key)...)
	assertAbsent(t, "clients", run(t, cliBin, env, "clients"), piece, b64, key)
	assertAbsent(t, "clients --agent", run(t, cliBin, env, "clients", "--agent"), piece, b64, key)
	assertAbsent(t, "MCP clients_get", mcpCall(t, mcpBin, env, `{"name":"clients_get","arguments":{}}`), piece, b64, key)
	if v.hits.Load() == 0 {
		t.Error("no request reached the API stub, so nothing was echoed")
	}
	assertNoFileCarries(t, home, nil, piece, b64)
}

// TestMCPConnectPreviewWithholdsCredentialFields (class preview-echo): an
// unconfirmed integrations_connect returns a request preview, and that preview
// carried the third-party credentials in credential_fields verbatim. The MCP
// argument is held before the handler runs (Declarations.MCPArguments), so the
// preview is withheld, and nothing is sent without confirm.
func TestMCPConnectPreviewWithholdsCredentialFields(t *testing.T) {
	_, mcpBin := binaries(t)
	const key = "corkkey-" + "Prev1ewK3y9x"
	const third = "edr-" + "s3cr3tW9q2Lm7p"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	home := t.TempDir()
	env := closedEnv(home, v.env(key)...)
	out := mcpCall(t, mcpBin, env, `{"name":"integrations_connect","arguments":{"vendor_key":"SENTINELONE","credential_fields":{"api_token":"`+third+`"}}}`)
	assertAbsent(t, "MCP integrations_connect preview", out, third)
	if v.hits.Load() != 0 {
		t.Errorf("an unconfirmed integrations_connect reached the API (%d request(s))", v.hits.Load())
	}
	assertNoFileCarries(t, home, nil, third)
}

// TestCLICredentialFieldsFlagHeld (class preview-echo, CLI side):
// --credential-fields is a credential flag; its value is held before the flag
// parser runs, so a dry-run preview or a parse error never prints it.
func TestCLICredentialFieldsFlagHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const third = "edr-" + "fl4gS3cr3tQ8z"
	env := closedEnv(t.TempDir(), "CORK_API_KEY=corkkey-"+"Fl4gK3yZ9q1w")
	assertAbsent(t, "integrations connect --dry-run",
		run(t, cliBin, env, "integrations", "connect", "--vendor-key", "SENTINELONE", "--credential-fields", `{"api_token":"`+third+`"}`, "--dry-run"), third)
}

// TestMalformedConfigCredentialHeldAtStartup (class startup-ordering): a
// config.toml that does not parse still has every credential in it held before
// the startup diagnostic that names the parse error can print.
func TestMalformedConfigCredentialHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const key = "corkkey-" + "Bad70ml9xQ2"
	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "config.toml"), "api_key = \""+key+"\"\nbase_url = [unclosed\n")
	env := closedEnv(home)
	assertAbsent(t, "CLI clients", run(t, cliBin, env, "clients"), key)
	assertAbsent(t, "CLI doctor", run(t, cliBin, env, "doctor"), key)
	assertAbsent(t, "MCP clients_get", mcpCall(t, mcpBin, env, `{"name":"clients_get","arguments":{}}`), key)
}

// TestProfileCredentialValueHeldAtStartup (class profiles.json): a run-profile
// value saved under a credential flag's name is a credential source, held at
// startup.
func TestProfileCredentialValueHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "corkProf" + "Fld-5d4c3b2a19"
	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "profiles.json"),
		`{"profiles":{"watcher":{"name":"watcher","values":{"credential-fields":"`+secret+`","page-size":"50"}}}}`)
	env := closedEnv(home)
	assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", secret, "version"), secret)
	assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", secret), secret)
	assertAbsent(t, "profile show", run(t, cliBin, env, "profile", "show", "watcher"), secret)
}

// TestHeldCredentialRefusedOffOriginAndOverPlainHTTP (class origin): the key
// goes only to the API it is configured for, and never over plain http, not
// even to loopback. A --deliver webhook whose URL carries the key is refused
// before it dials.
func TestHeldCredentialRefusedOffOriginAndOverPlainHTTP(t *testing.T) {
	cliBin, _ := binaries(t)
	const key = "corkkey-" + "Orig1nQ7r3s5"
	api := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"items":[{"uuid":"c1","name":"Acme"}],"total":1}`)
	})
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	var plainHits atomic.Int64
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plainHits.Add(1)
		io.WriteString(w, `{}`)
	}))
	t.Cleanup(plain.Close)

	home := t.TempDir()
	env := closedEnv(home, api.env(key)...)
	run(t, cliBin, env, "clients", "--json", "--deliver", "webhook:"+other.url+"/hook/"+key)
	if n := other.conns.Load(); n != 0 {
		t.Errorf("webhook to another https origin: %d connection(s) opened for a URL that carries the key", n)
	}
	run(t, cliBin, env, "clients", "--json", "--deliver", "webhook:"+plain.URL+"/hook/"+key)
	if n := plainHits.Load(); n != 0 {
		t.Errorf("webhook over plain http: %d request(s) carried the key", n)
	}
	if api.hits.Load() == 0 {
		t.Error("the API stub was never read, so no webhook was attempted")
	}

	// The API base URL itself over plain http: the key is never sent.
	var bearerHits atomic.Int64
	plainAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization"), key) {
			bearerHits.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"items":[]}`)
	}))
	t.Cleanup(plainAPI.Close)
	env = closedEnv(t.TempDir(), "CORK_BASE_URL="+plainAPI.URL, "CORK_API_KEY="+key)
	assertAbsent(t, "plain http API", run(t, cliBin, env, "clients"), key)
	if n := bearerHits.Load(); n != 0 {
		t.Errorf("plain http API: %d request(s) carried the key", n)
	}
}

// TestSpacefastTokenHeld (class spacefast-token): SPACEFAST_TOKEN, the
// credential of the --deliver spacefast: sink, is held from startup in both
// binaries.
func TestSpacefastTokenHeld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "sft_" + "live_4b7c9d1e2f3a"
	env := closedEnv(t.TempDir(), "SPACEFAST_TOKEN="+secret)
	assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", secret, "version"), secret)
	assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", secret), secret)
	assertAbsent(t, "CLI deliver target error", run(t, cliBin, env, "version", "--deliver", "spacefast:"+secret+"/x"), secret)
}

// TestJournalNeverRecordsAHeldCredential (class journal): the learn journal
// records a mistyped flag's name. A key pasted where a flag name goes
// (--<key>) would be written to state/learn/journal-*.jsonl; the journal
// appends through the kit (credkit.AppendFile), which withholds an entry
// carrying a held value.
func TestJournalNeverRecordsAHeldCredential(t *testing.T) {
	cliBin, _ := binaries(t)
	const key = "corkkey" + "JournL8x2q9"
	home := t.TempDir()
	env := closedEnv(home, "CORK_API_KEY="+key)
	run(t, cliBin, env, "clients", "--"+key)
	run(t, cliBin, env, "clients", "--pag-size", key)
	journals, _ := filepath.Glob(filepath.Join(home, "state", "learn", "journal-*.jsonl"))
	if len(journals) == 0 {
		t.Fatal("no journal segment was written, so nothing was checked")
	}
	assertNoFileCarries(t, home, nil, key)
}

// TestFeedbackStdinIsRecorded (class missing data): `feedback --stdin` keeps
// its body (feedback text is no credential), while a held key pasted into it
// is still withheld.
func TestFeedbackStdinIsRecorded(t *testing.T) {
	cliBin, _ := binaries(t)
	const key = "corkkeyFeedbk3Lp7q"
	const text = "the clients output was confusing"
	home := t.TempDir()
	env := closedEnv(home, "CORK_API_KEY="+key)
	runStdin(t, cliBin, env, text+"\n", "feedback", "--stdin")
	runStdin(t, cliBin, env, "pasted "+key+" by mistake\n", "feedback", "--stdin")
	record, err := os.ReadFile(filepath.Join(home, "data", "feedback.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(record), text) {
		t.Errorf("feedback.jsonl lost the feedback:\n%s", record)
	}
	assertNoFileCarries(t, home, nil, key)
}

// runStdin is run with stdin.
func runStdin(t *testing.T, bin string, env []string, stdin string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run()
	if cmd.ProcessState.ExitCode() == credkit.ExitRefused {
		t.Errorf("%s %s: the start was refused:\n%s", filepath.Base(bin), strings.Join(args, " "), out.String())
	}
	return out.String()
}
