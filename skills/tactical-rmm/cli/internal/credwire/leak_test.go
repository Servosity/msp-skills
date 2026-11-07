// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class's failing input, seen
// failing on the binaries built before the kit was wired. Set
// TACTICAL_RMM_LEAK_BIN_DIR to run them against binaries built elsewhere.

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

	"tactical-rmm-pp-cli/internal/credkit"
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
		if d := os.Getenv("TACTICAL_RMM_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "tactical-rmm-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{exe("tactical-rmm-cli"), "../../cmd/tactical-rmm-cli"},
			{exe("tactical-rmm-mcp"), "../../cmd/tactical-rmm-mcp"},
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
	return filepath.Join(binDir, exe("tactical-rmm-cli")), filepath.Join(binDir, exe("tactical-rmm-mcp"))
}

// vendor is a TLS stub of the Tactical RMM API: the kit never sends a credential
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
	return []string{"TACTICAL_RMM_BASE_URL=" + v.url, "TACTICAL_RMM_CA_CERT=" + v.caFile, "TRMM_API_KEY=" + token}
}

// closedEnv is a closed environment under one sandbox home; every press
// directory resolves under it.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "TACTICAL_RMM_HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg-config"), "XDG_DATA_HOME=" + filepath.Join(home, "xdg-data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "xdg-state"), "XDG_CACHE_HOME=" + filepath.Join(home, "xdg-cache"),
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

// TestAPIErrorEchoOfKeyIsWithheld (class key-echo): an API error body that
// echoes part of TRMM_API_KEY. Any 8-character piece of a held value withholds
// the output, on the CLI, the MCP server and every file.
func TestAPIErrorEchoOfKeyIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const key = "trmmk" + "Zq81LmP0xV7cR2"
	piece := key[:12]
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"detail":"API key %s... is not valid"}`, piece)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env(key)...)
	assertAbsent(t, "clients clients", run(t, cliBin, env, "clients", "clients"), piece)
	assertAbsent(t, "clients clients --agent", run(t, cliBin, env, "clients", "clients", "--agent"), piece)
	assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, `{"name":"tactical-rmm_read","arguments":{"endpoint_id":"clients.clients"}}`), piece)
	if v.hits.Load() == 0 {
		t.Error("no request reached the API stub, so nothing was echoed")
	}
	assertNoFileCarries(t, home, nil, piece)
}

// TestConfigKeyHeldAtStartup (class config-key): the API key in config.toml or
// the 0600 credential store is held before anything prints, so a startup
// diagnostic that echoes it (a flag error, in both binaries) is withheld. A
// malformed config still has its values held, word by word.
func TestConfigKeyHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	cases := []struct {
		name, file, body, secret string
	}{ // file is under the sandbox home
		{"config", "config/config.toml", "api_key = \"trmmCfg" + "Key-61ab93f2\"\n", "trmmCfgKey-61ab93f2"},
		{"malformed-config", "config/config.toml", "api_key = \"trmmBad" + "Key-77aa91c4\" broken\n", "trmmBadKey-77aa91c4"},
		{"auth-header", "config/config.toml", "auth_header = \"trmmHdr" + "Key-0c9d8e7f\"\n", "trmmHdrKey-0c9d8e7f"},
		{"headers-table", "config/config.toml", "[headers]\nX-API-KEY = \"trmmTbl" + "Key-3e4f5a6b\"\n", "trmmTblKey-3e4f5a6b"},
		{"credential-store", "data/credentials.toml", "api_key = \"trmmSto" + "Key-9a8b7c6d\"\n", "trmmStoKey-9a8b7c6d"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, filepath.FromSlash(c.file)), c.body)
			env := closedEnv(home)
			assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", c.secret, "version"), c.secret)
			assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", c.secret), c.secret)
		})
	}
}

// TestHeldCredentialRefusedOffOriginAndOverPlainHTTP (class origin): the key
// goes only to the Tactical RMM server it is configured for, and never over
// plain http, not even to loopback. A --deliver webhook whose URL carries the
// key is refused before it dials: to another https origin, and to a plain http
// loopback one.
func TestHeldCredentialRefusedOffOriginAndOverPlainHTTP(t *testing.T) {
	cliBin, _ := binaries(t)
	const key = "trmmk" + "Orig1nQ7r3s5x"
	api := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `[{"id":1,"name":"Acme"}]`)
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
	run(t, cliBin, env, "clients", "clients", "--json", "--deliver", "webhook:"+other.url+"/hook/"+key)
	if n := other.conns.Load(); n != 0 {
		t.Errorf("webhook to another https origin: %d connection(s) opened for a URL that carries the key", n)
	}
	run(t, cliBin, env, "clients", "clients", "--json", "--deliver", "webhook:"+plain.URL+"/hook/"+key)
	if n := plainHits.Load(); n != 0 {
		t.Errorf("webhook over plain http: %d request(s) carried the key", n)
	}
	if api.hits.Load() == 0 {
		t.Error("the API stub was never read, so no webhook was attempted")
	}

	// The API base URL itself over plain http: the key is never sent.
	var keyHits atomic.Int64
	plainAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("X-API-KEY"), key) {
			keyHits.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `[]`)
	}))
	t.Cleanup(plainAPI.Close)
	env = closedEnv(t.TempDir(), "TACTICAL_RMM_BASE_URL="+plainAPI.URL, "TRMM_API_KEY="+key)
	assertAbsent(t, "plain http API", run(t, cliBin, env, "clients", "clients"), key)
	if n := keyHits.Load(); n != 0 {
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
// (--<key>) was written to state/learn/journal-*.jsonl; the journal appends
// through the kit (credkit.AppendFile), which withholds an entry carrying a held
// value.
func TestJournalNeverRecordsAHeldCredential(t *testing.T) {
	cliBin, _ := binaries(t)
	const key = "trmmk" + "JournL8x2q9w"
	home := t.TempDir()
	env := closedEnv(home, "TRMM_API_KEY="+key)
	run(t, cliBin, env, "clients", "clients", "--"+key)
	run(t, cliBin, env, "clients", "clients", "--limt", key)
	journals, _ := filepath.Glob(filepath.Join(home, "state", "learn", "journal-*.jsonl"))
	if len(journals) == 0 {
		t.Fatal("no journal segment was written, so nothing was checked")
	}
	assertNoFileCarries(t, home, nil, key)
}

// TestSetTokenFromStdinIsNeverPrinted (class stdin-key): `auth set-token`
// reads the key from a pipe and saves it to the 0600 store; the key is
// held as soon as it is read, so a later diagnostic in the same run that
// carries it is withheld, and no file but the store holds it.
func TestSetTokenFromStdinIsNeverPrinted(t *testing.T) {
	cliBin, _ := binaries(t)
	const key = "trmmk" + "StdIn5v6w7x8y"
	home := t.TempDir()
	out := runStdin(t, cliBin, closedEnv(home), key+"\n", "auth", "set-token")
	assertAbsent(t, "auth set-token", out, key)
	store, err := filepath.Glob(filepath.Join(home, "data", "credentials*.toml"))
	if err != nil || len(store) == 0 {
		t.Fatalf("set-token wrote no credential store:\n%s", out)
	}
	assertNoFileCarries(t, home, store, key)
}

// TestFeedbackStdinIsRecorded (class missing data): `feedback --stdin` is a
// StdinFlags name, so the kit would hold the whole body and withhold the local
// record, while the command still said "feedback recorded locally". Feedback
// text is no credential, so its body is taken off stdin before the kit reads
// it; a held credential pasted into it is still withheld.
func TestFeedbackStdinIsRecorded(t *testing.T) {
	cliBin, _ := binaries(t)
	const key = "trmmkFeedbk3Lp7q"
	const text = "the clients list output was confusing"
	home := t.TempDir()
	env := closedEnv(home, "TRMM_API_KEY="+key)
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

// TestMCPShellOutChildAuthenticates (class child): mirrored MCP tools run the
// companion CLI as a child. The kit sends the child the held set instead of a
// TRMM_API_KEY in its environment, and the child's own Start sets it again, so
// the child still authenticates to the configured server and its rows come
// back; the key appears in no output and no file.
func TestMCPShellOutChildAuthenticates(t *testing.T) {
	_, mcpBin := binaries(t)
	const key = "trmmk" + "Child9k8j7h6g"
	var authed atomic.Int64
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") == key {
			authed.Add(1)
		}
		io.WriteString(w, `[{"id":7,"name":"AcmeChildRow"}]`)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env(key)...)
	out := mcpCall(t, mcpBin, env, `{"name":"sync","arguments":{"resources":"clients","full":true}}`)
	assertAbsent(t, "MCP sync", out, key)
	if authed.Load() == 0 {
		t.Errorf("the CLI child never reached the API with the key:\n%s", out)
	}
	assertNoFileCarries(t, home, nil, key)
}

// TestSavedProfileBaseURLHeld (class profile-base-url): a saved run profile's
// base-url value can carry a password or a query token. It is held from
// startup, at the current and the legacy store path and under every spelling
// and duplicate of the name, so `profile show` never prints it.
func TestSavedProfileBaseURLHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const pass, tok, dup = "ProfileP" + "ass9z", "ProfQ" + "tok7k2", "DupP" + "ass4m8"
	body := `{"profiles":{"saved":{"name":"saved","values":{"base-url":"https://ops:` + pass +
		`@rmm.example.net/api?token=` + tok + `","Base_URL":"https://a:` + dup + `@x.example.net","base-url":"https://rmm.example.net"}}}}`
	single := `{"profiles":{"saved":{"name":"saved","values":{"base-url":"https://ops:` + pass + `@rmm.example.net/api?token=` + tok + `"}}}}`
	for _, rel := range []string{"config/profiles.json", ".tactical-rmm-cli/profiles.json"} {
		for i, b := range []string{single, body} {
			t.Run(fmt.Sprintf("%s/%d", rel, i), func(t *testing.T) {
				home := t.TempDir()
				writeFile(t, filepath.Join(home, filepath.FromSlash(rel)), b)
				env := closedEnv(home)
				assertAbsent(t, "profile show", run(t, cliBin, env, "profile", "show", "saved", "--json", "--no-learn"), pass, tok, dup)
				assertAbsent(t, "profile list", run(t, cliBin, env, "profile", "list", "--json", "--no-learn"), pass, tok, dup)
			})
		}
	}
}

// TestSpacefastMintedKeyNeverPrinted (class minted-key): an anonymous Spacefast
// publish returns a new page key and a door URL that carries it. The key is
// held the moment the publish returns, so no output prints it; it is kept only
// in the private delivery record.
func TestSpacefastMintedKeyNeverPrinted(t *testing.T) {
	cliBin, _ := binaries(t)
	const key = "sfc_" + "MintedReview9z"
	sf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":{"space":{"id":"space-r1"},"claim":{"key":"`+key+
			`","url":"https://spacefast.com/r1?key=`+key+`","claimUrl":"https://spacefast.com/claim/r1"}}}`)
	}))
	t.Cleanup(sf.Close)
	home := t.TempDir()
	env := closedEnv(home, "SPACEFAST_API_URL="+sf.URL)
	assertAbsent(t, "spacefast publish", run(t, cliBin, env, "version", "--deliver", "spacefast:review-page", "--no-learn"), key)
	assertAbsent(t, "spacefast republish", run(t, cliBin, env, "version", "--deliver", "spacefast:review-page", "--no-learn"), key)
}
