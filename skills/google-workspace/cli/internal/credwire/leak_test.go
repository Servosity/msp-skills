// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class's failing input, seen
// failing on the binaries built before the kit was wired. Set
// GOOGLE_WORKSPACE_LEAK_BIN_DIR to run them against binaries built elsewhere.
// The token-endpoint echo (the service-account grant's assertion) is proven in
// internal/mcp/credkit_token_leak_test.go, which can stand in for Google's
// pinned token endpoint.

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
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google-workspace-pp-cli/internal/credkit"
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
		if d := os.Getenv("GOOGLE_WORKSPACE_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "google-workspace-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{exe("google-workspace-cli"), "../../cmd/google-workspace-cli"},
			{exe("google-workspace-mcp"), "../../cmd/google-workspace-mcp"},
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
	return filepath.Join(binDir, exe("google-workspace-cli")), filepath.Join(binDir, exe("google-workspace-mcp"))
}

// vendor is a TLS stub of the Google API: the kit never sends a credential
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
	return []string{"GOOGLE_WORKSPACE_BASE_URL=" + v.url, "GOOGLE_WORKSPACE_CA_CERT=" + v.caFile, "GOOGLE_WORKSPACE_TOKEN=" + token}
}

// closedEnv is a closed environment under one sandbox home; every press
// directory resolves under it.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "GOOGLE_WORKSPACE_HOME=" + home,
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

// TestAPIErrorEchoOfTokenIsWithheld (class token-echo): an API error body that
// echoes part of GOOGLE_WORKSPACE_TOKEN. The generated masks replace the whole
// token and its encodings; a server that echoes a piece of it ("gwtok-Zq81Lm...")
// slipped past them. Any 8-character piece of a held value withholds the output.
func TestAPIErrorEchoOfTokenIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const token = "gwtok-" + "Zq81LmP0xV7c"
	piece := token[:12]
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":{"code":401,"message":"token %s... rejected","status":"UNAUTHENTICATED"}}`, piece)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env(token)...)
	assertAbsent(t, "users list", run(t, cliBin, env, "users", "list"), piece)
	assertAbsent(t, "users list --agent", run(t, cliBin, env, "users", "list", "--agent"), piece)
	assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, `{"name":"google-workspace_read","arguments":{"endpoint_id":"users.list"}}`), piece)
	if v.hits.Load() == 0 {
		t.Error("no request reached the API stub, so nothing was echoed")
	}
	assertNoFileCarries(t, home, nil, piece)
}

// testKey is a service-account key's shape. The PEM body is not a key; nothing
// at startup parses it, and the kit holds it by its field name.
const (
	testKeyID   = "pkid" + "9f8e7d6c5b4a39281706f5e4d3c2b1a0"
	testKeyLine = "MIIEv" + "QIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7"
)

func testKeyJSON() string {
	b, _ := json.Marshal(map[string]string{
		"type":           "service_account",
		"client_email":   "sa@proj.iam.gserviceaccount.com",
		"client_id":      "112233445566778899",
		"private_key_id": testKeyID,
		"private_key":    "-----BEGIN " + "PRIVATE KEY-----\n" + testKeyLine + "\n-----END PRIVATE KEY-----\n",
		"token_uri":      "https://oauth2.googleapis.com/token",
	})
	return string(b)
}

func writeTenant(t *testing.T, home, keyPath string) {
	t.Helper()
	writeFile(t, filepath.Join(configDir(home), "tenants.toml"), fmt.Sprintf(
		"[[tenants]]\nname = \"acme\"\ndomain = \"acme.example\"\nadmin = \"admin@acme.example\"\nkey_file = %q\nscopes = [\"https://www.googleapis.com/auth/admin.directory.user.readonly\"]\n", keyPath))
}

// TestServiceAccountKeyHeldAtStartup (class sa-key): a tenant's service-account
// key (copied into keys/, or anywhere tenants.toml names it) is held before
// anything prints, so a startup diagnostic that echoes a piece of it (a flag
// error, in both binaries) is withheld. A malformed key or config still has its
// values held: every string and word of bytes that do not parse.
func TestServiceAccountKeyHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	cases := []struct {
		name    string
		setup   func(t *testing.T, home string)
		secrets []string
	}{
		{"copied-key", func(t *testing.T, home string) {
			key := filepath.Join(configDir(home), "keys", "acme.json")
			writeFile(t, key, testKeyJSON())
			writeTenant(t, home, key)
		}, []string{testKeyID, testKeyLine}},
		{"key-named-by-registry", func(t *testing.T, home string) {
			key := filepath.Join(home, "elsewhere", "sa.json")
			writeFile(t, key, testKeyJSON())
			writeTenant(t, home, key)
		}, []string{testKeyID, testKeyLine}},
		{"malformed-key", func(t *testing.T, home string) {
			key := filepath.Join(configDir(home), "keys", "acme.json")
			writeFile(t, key, `{"private_key_id": "pkid`+`MALF0a1b2c3d4e5f6a7b", "private_key": "MIIEv`+`QIBADANBgkqhkiG9w0Bmalf`)
		}, []string{"pkidMALF0a1b2c3d4e5f6a7b", "MIIEvQIBADANBgkqhkiG9w0Bmalf"}},
		{"malformed-config", func(t *testing.T, home string) {
			writeFile(t, filepath.Join(configDir(home), "config.toml"), "workspace_token = \"gwcfgTok-77aa91\" broken\n")
		}, []string{"gwcfgTok-77aa91"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			c.setup(t, home)
			env := closedEnv(home)
			for _, s := range c.secrets {
				assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", s, "version"), s)
				assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", s), s)
			}
			assertAbsent(t, "tenants list", run(t, cliBin, env, "tenants", "list", "--json"), c.secrets...)
		})
	}
}

// TestProfileCredentialValueHeldAtStartup (class profile-value): a value saved
// in profiles.json under a credential flag's name (--token) is held at startup.
func TestProfileCredentialValueHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "gwProf" + "Tok-5d4c3b2a19"
	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "profiles.json"),
		`{"profiles":{"watcher":{"name":"watcher","values":{"token":"`+secret+`","customer":"my_customer"}}}}`)
	env := closedEnv(home)
	assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", secret, "version"), secret)
	assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", secret), secret)
	assertAbsent(t, "profile show", run(t, cliBin, env, "profile", "show", "watcher"), secret)
}

// TestHeldCredentialRefusedOffOriginAndOverPlainHTTP (class origin): the token
// goes only to the API it is configured for, and never over plain http, not
// even to loopback. A --deliver webhook whose URL carries the token is refused
// before it dials: to another https origin, and to a plain http loopback one
// (which the program's own https rule allows for a token-free body).
func TestHeldCredentialRefusedOffOriginAndOverPlainHTTP(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "gwtok-" + "Orig1nQ7r3s5"
	api := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"users":[{"id":"u1","primaryEmail":"a@acme.example"}]}`)
	})
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	var plainHits atomic.Int64
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plainHits.Add(1)
		io.WriteString(w, `{}`)
	}))
	t.Cleanup(plain.Close)

	home := t.TempDir()
	env := closedEnv(home, api.env(token)...)
	run(t, cliBin, env, "users", "list", "--json", "--deliver", "webhook:"+other.url+"/hook/"+token)
	if n := other.conns.Load(); n != 0 {
		t.Errorf("webhook to another https origin: %d connection(s) opened for a URL that carries the token", n)
	}
	run(t, cliBin, env, "users", "list", "--json", "--deliver", "webhook:"+plain.URL+"/hook/"+token)
	if n := plainHits.Load(); n != 0 {
		t.Errorf("webhook over plain http: %d request(s) carried the token", n)
	}
	if api.hits.Load() == 0 {
		t.Error("the API stub was never read, so no webhook was attempted")
	}

	// The API base URL itself over plain http: the token is never sent.
	var bearerHits atomic.Int64
	plainAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization"), token) {
			bearerHits.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"users":[]}`)
	}))
	t.Cleanup(plainAPI.Close)
	env = closedEnv(t.TempDir(), "GOOGLE_WORKSPACE_BASE_URL="+plainAPI.URL, "GOOGLE_WORKSPACE_TOKEN="+token)
	assertAbsent(t, "plain http API", run(t, cliBin, env, "users", "list"), token)
	if n := bearerHits.Load(); n != 0 {
		t.Errorf("plain http API: %d request(s) carried the token", n)
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
// records a mistyped flag's name. A credential pasted where a flag name goes
// (--<token>) was written to state/learn/journal-*.jsonl; the journal appends
// through the kit (credkit.AppendFile), which withholds an entry carrying a held
// value.
func TestJournalNeverRecordsAHeldCredential(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "gwtok" + "JournL8x2q9"
	home := t.TempDir()
	env := closedEnv(home, "GOOGLE_WORKSPACE_TOKEN="+token)
	run(t, cliBin, env, "users", "list", "--"+token)
	run(t, cliBin, env, "users", "list", "--custmer", token)
	journals, _ := filepath.Glob(filepath.Join(home, "state", "learn", "journal-*.jsonl"))
	if len(journals) == 0 {
		t.Fatal("no journal segment was written, so nothing was checked")
	}
	assertNoFileCarries(t, home, nil, token)
}

// TestFeedbackStdinIsRecorded (class missing data): SKILL.md documents
// `feedback --stdin < notes.txt`. --stdin is a StdinFlags name, so the kit held
// the whole body and withheld the local record, while the command still said
// "feedback recorded locally". Feedback text is no credential (`feedback
// <text>` was never held), so its body is taken off stdin before the kit reads
// it; a held credential pasted into it is still withheld.
func TestFeedbackStdinIsRecorded(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "gwtokFeedbk3Lp7q"
	const text = "the users list output was confusing"
	home := t.TempDir()
	env := closedEnv(home, "GOOGLE_WORKSPACE_TOKEN="+token)
	runStdin(t, cliBin, env, text+"\n", "feedback", "--stdin")
	runStdin(t, cliBin, env, "pasted "+token+" by mistake\n", "feedback", "--stdin")
	record, err := os.ReadFile(filepath.Join(home, "data", "feedback.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(record), text) {
		t.Errorf("feedback.jsonl lost the feedback:\n%s", record)
	}
	assertNoFileCarries(t, home, nil, token)
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
