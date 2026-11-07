// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class's failing input, seen
// failing before the kit was wired. Set RIVERSIDE_FM_LEAK_BIN_DIR to run them
// against binaries built elsewhere.

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

	"riverside-fm-pp-cli/internal/credkit"
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
		if d := os.Getenv("RIVERSIDE_FM_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "riverside-fm-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{exe("riverside-fm-cli"), "../../cmd/riverside-fm-cli"},
			{exe("riverside-fm-mcp"), "../../cmd/riverside-fm-mcp"},
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
	return filepath.Join(binDir, exe("riverside-fm-cli")), filepath.Join(binDir, exe("riverside-fm-mcp"))
}

// vendor is a TLS stub of Riverside: the kit never sends a credential over
// plain http, loopback included.
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

func (v *vendor) env() []string {
	return []string{"RIVERSIDE_FM_BASE_URL=" + v.url, "RIVERSIDE_FM_CA_CERT=" + v.caFile}
}

// closedEnv is a closed environment under one sandbox home; every press
// directory resolves under it.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "RIVERSIDE_FM_HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg-config"), "XDG_DATA_HOME=" + filepath.Join(home, "xdg-data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "xdg-state"), "XDG_CACHE_HOME=" + filepath.Join(home, "xdg-cache"),
		"TMPDIR=" + os.TempDir(), "NO_COLOR=1"}, extra...)
}

func configDir(home string) string { return filepath.Join(home, "config") }
func dataDir(home string) string   { return filepath.Join(home, "data") }

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

// Session fixtures, shaped like Riverside's JWT cookies: three dotted segments
// of a run no ordinary output carries.
const (
	accessCookie  = "qZ8kW2vN" + "aaaa1111.Lm4xT9rP" + "bbbb2222.Yc7dF3sH" + "cccc3333"
	refreshCookie = "uJ5gB6nE" + "dddd4444.Kw2zR8tQ" + "eeee5555.Vx9mC1pL" + "ffff6666"
	bareJWT       = "eyJhbGciOiJIUzI1NiJ9." + "Rt6yU8iOzzzz9999pQ3wE5rT" + ".Gh2jK4lMyyyy8888nB7vC9xZ"
)

func cookieHeader() string {
	return "riverside_auth_access=" + accessCookie + "; riverside_auth_refresh=" + refreshCookie
}

// TestStoredCookieHeaderLeavesOrdinaryOutputAlone (class over-hold): auth login
// keeps the whole Cookie header in access_token. Held whole, its cookie NAMES
// held "riverside", so every output naming the product was withheld and every
// command with a saved session printed nothing and exited 74.
func TestStoredCookieHeaderLeavesOrdinaryOutputAlone(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "config.toml"), "access_token = \""+cookieHeader()+"\"\n")
	out, code := runCode(t, cliBin, closedEnv(home), "--help")
	if code != 0 || !strings.Contains(out, "riverside-fm-cli") {
		t.Fatalf("--help with a saved session: exit %d, output:\n%s", code, out)
	}
}

// TestStoredSessionHeldAtStartup (class session-held): every place a session
// rests is held before anything prints, so a startup diagnostic that echoes it
// (a flag error, in both binaries) is withheld.
func TestStoredSessionHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	cases := []struct {
		name    string
		setup   func(t *testing.T, home string)
		secrets []string
	}{
		{"config-cookie-header", func(t *testing.T, home string) {
			writeFile(t, filepath.Join(configDir(home), "config.toml"), "access_token = \""+cookieHeader()+"\"\n")
		}, []string{accessCookie, refreshCookie}},
		{"store-bare-jwt", func(t *testing.T, home string) {
			writeFile(t, filepath.Join(dataDir(home), "credentials.toml"), "access_token = \""+bareJWT+"\"\n")
		}, []string{bareJWT}},
		{"cookie-jar", func(t *testing.T, home string) {
			writeFile(t, filepath.Join(dataDir(home), "cookies.json"), `[{"name":"riverside_auth_access","value":"`+accessCookie+`","domain":"riverside.com","path":"/"}]`)
		}, []string{accessCookie}},
		{"malformed-config", func(t *testing.T, home string) {
			writeFile(t, filepath.Join(configDir(home), "config.toml"), "access_token = \""+bareJWT+"\" broken\n")
		}, []string{bareJWT}},
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
		})
	}
}

// TestMalformedJarIsHeldWhole (class session-held): a jar that does not parse has
// every word held, a cookie name among them, so the start is refused: nothing
// prints, and neither does its session.
func TestMalformedJarIsHeldWhole(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	writeFile(t, filepath.Join(dataDir(home), "cookies.json"), `[{"name":"riverside_auth_access","value":"`+accessCookie)
	out, code := runCode(t, cliBin, closedEnv(home), "--timeout", accessCookie, "version")
	assertAbsent(t, "CLI startup flag error", out, accessCookie)
	if code == 0 {
		t.Errorf("a malformed jar started the program and exited 0:\n%s", out)
	}
}

// TestAPIEchoOfSessionIsWithheld (class token-echo): an API error body that
// echoes a piece of the session. Any 8-character piece of a held value
// withholds the output, on the CLI and through MCP.
func TestAPIEchoOfSessionIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	piece := accessCookie[:12]
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":"session %s... rejected"}`, piece)
	})
	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "config.toml"), "access_token = \""+cookieHeader()+"\"\n")
	env := closedEnv(home, v.env()...)
	assertAbsent(t, "user", run(t, cliBin, env, "user"), piece)
	assertAbsent(t, "user --agent", run(t, cliBin, env, "user", "--agent"), piece)
	assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, `{"name":"user","arguments":{}}`), piece)
	if v.hits.Load() == 0 {
		t.Error("no request reached the API stub, so nothing was echoed")
	}
	assertNoFileCarries(t, home, []string{filepath.Join(configDir(home), "config.toml")}, piece)
}

// TestRotatedCookieHeldBeforeOutput (class set-cookie): Riverside rotates the
// session on every response. A rotated value echoed in the same response never
// reaches an output, and only the 0600 jar keeps it.
func TestRotatedCookieHeldBeforeOutput(t *testing.T) {
	cliBin, _ := binaries(t)
	const rotated = "Pp3nM7qX" + "rota7777.Ss1vB9wD" + "ted88888.Tt4cF6hJ" + "val99999"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Set-Cookie", "riverside_auth_access="+rotated+"; Path=/; HttpOnly; Secure")
		fmt.Fprintf(w, `{"id":"u1","debug":"%s"}`, rotated)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env()...)
	assertAbsent(t, "user", run(t, cliBin, env, "user", "--json"), rotated)
	if v.hits.Load() == 0 {
		t.Error("no request reached the API stub, so nothing was rotated")
	}
	assertNoFileCarries(t, home, []string{filepath.Join(dataDir(home), "cookies.json")}, rotated)
}

// TestStudioJoinSecretsNeverReachAnySink (class join-secrets): a studio or take
// response carries its join secrets. Anyone holding one joins the studio. They
// are gone from every output, MCP result, cache file and store row, the rest
// of the response survives, and a value echoed under another name is withheld.
func TestStudioJoinSecretsNeverReachAnySink(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const (
		producer = "ProdScrt" + "4f7a2c91d0e8"
		guest    = "GuestScr" + "8b3e6d15f2a7"
		share    = "ShareTkn" + "1c9d5e7b3a60"
	)
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"studio":{"_id":"s1","name":"Northwind Studio","producerSecret":"%s","guestSecret":"%s","projects":[{"_id":"p1","shareToken":"%s"}]}}`, producer, guest, share)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env()...)
	out := run(t, cliBin, env, "studios", "get", "north", "--json")
	assertAbsent(t, "studios get", out, producer, guest, share)
	if !strings.Contains(out, "Northwind Studio") {
		t.Errorf("the studio itself is gone from the output:\n%s", out)
	}
	mcpOut := mcpCall(t, mcpBin, env, `{"name":"studios_get","arguments":{"slug":"north"}}`)
	assertAbsent(t, "MCP studios_get", mcpOut, producer, guest, share)
	if v.hits.Load() == 0 {
		t.Error("no request reached the API stub")
	}
	assertNoFileCarries(t, home, nil, producer, guest, share)
}

// TestCookiesFileLoginHoldsItsValues (class login-import): auth login
// --cookies-file writes the session to its 0600 files and prints no value of it,
// and the next command still works.
func TestCookiesFileLoginHoldsItsValues(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	env := closedEnv(home)
	src := filepath.Join(home, "import.txt")
	writeFile(t, src, cookieHeader()+"; sweetsesh=Sw33tSesh"+"abc123; cloudfront_signed_url=CfUrlVal"+"def456; dialogr-session=DlgSess1"+"ghi789; dialogr-session.sig=DlgSig22"+"jkl012\n")
	out := run(t, cliBin, env, "auth", "login", "--cookies-file", src)
	assertAbsent(t, "auth login", out, accessCookie, refreshCookie)
	next, code := runCode(t, cliBin, env, "auth", "status", "--json")
	assertAbsent(t, "auth status", next, accessCookie, refreshCookie)
	if code != 0 && strings.TrimSpace(next) == "" {
		t.Errorf("auth status after login printed nothing, exit %d", code)
	}
}

// TestHeldCredentialRefusedOffOriginAndOverPlainHTTP (class origin): a --deliver
// webhook whose URL carries the session is refused before it dials: to another
// https origin, and to a plain http loopback one.
func TestHeldCredentialRefusedOffOriginAndOverPlainHTTP(t *testing.T) {
	cliBin, _ := binaries(t)
	api := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{"id":"u1"}`) })
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	var plainHits atomic.Int64
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		plainHits.Add(1)
		io.WriteString(w, `{}`)
	}))
	t.Cleanup(plain.Close)

	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "config.toml"), "access_token = \""+cookieHeader()+"\"\n")
	env := closedEnv(home, api.env()...)
	run(t, cliBin, env, "user", "--json", "--deliver", "webhook:"+other.url+"/hook/"+accessCookie)
	if n := other.conns.Load(); n != 0 {
		t.Errorf("webhook to another https origin: %d connection(s) opened for a URL that carries the session", n)
	}
	run(t, cliBin, env, "user", "--json", "--deliver", "webhook:"+plain.URL+"/hook/"+accessCookie)
	if n := plainHits.Load(); n != 0 {
		t.Errorf("webhook over plain http: %d request(s) carried the session", n)
	}
	if api.hits.Load() == 0 {
		t.Error("the API stub was never read, so no webhook was attempted")
	}
}

// TestSpacefastAndHTTPTokensHeld (class env-token): the two secret environment
// variables are held from startup in both binaries.
func TestSpacefastAndHTTPTokensHeld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const spacefast = "sft_" + "live_4b7c9d1e2f3a"
	const httpTok = "mcpHttp" + "Tok9f8e7d6c5b4a3928170f"
	env := closedEnv(t.TempDir(), "SPACEFAST_TOKEN="+spacefast, "RIVERSIDE_FM_MCP_HTTP_TOKEN="+httpTok)
	for _, s := range []string{spacefast, httpTok} {
		assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", s, "version"), s)
		assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", s), s)
	}
}

// TestJournalNeverRecordsAHeldCredential (class journal): the learn journal
// records a mistyped flag's name. A session pasted where a flag name goes was
// written to state/learn/journal-*.jsonl; the journal appends through the kit.
func TestJournalNeverRecordsAHeldCredential(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	cfg := filepath.Join(configDir(home), "config.toml")
	writeFile(t, cfg, "access_token = \""+cookieHeader()+"\"\n")
	env := closedEnv(home)
	run(t, cliBin, env, "user", "--"+accessCookie)
	journals, _ := filepath.Glob(filepath.Join(home, "state", "learn", "journal-*.jsonl"))
	if len(journals) == 0 {
		t.Fatal("no journal segment was written, so nothing was checked")
	}
	assertNoFileCarries(t, home, []string{cfg}, accessCookie)
}

// A long episode's transcript is a large text unit, and the kit scans every unit
// it passes. Prove a three-hour recording (27,000 words) reaches stdout and a file
// whole, with a session held, instead of being withheld or cut.
func TestLongEpisodeTranscriptReachesStdoutAndFile(t *testing.T) {
	cliBin, _ := binaries(t)
	const words, perSentence = 27000, 15
	var body strings.Builder
	body.WriteString(`{"success":true,"data":{"speakers":[{"id":"s1","name":"Host","sentences":[`)
	for i := 0; i < words/perSentence; i++ {
		if i > 0 {
			body.WriteByte(',')
		}
		fmt.Fprintf(&body, `{"index":%d,"words":[`, i)
		for j := 0; j < perSentence; j++ {
			if j > 0 {
				body.WriteByte(',')
			}
			n := i*perSentence + j
			fmt.Fprintf(&body, `["w%dx",%d,400]`, n, n*400)
		}
		body.WriteString(`]}`)
	}
	body.WriteString(`]}]}}`)
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body.String())
	})
	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "config.toml"), "access_token = \""+cookieHeader()+"\"\n")
	env := closedEnv(home, v.env()...)
	last := fmt.Sprintf("w%dx", words-1)

	out, code := runCode(t, cliBin, env, "transcripts", "convert", "take-1", "--format", "txt")
	if code != 0 || !strings.Contains(out, last) {
		t.Fatalf("stdout: exit %d, %d bytes, last word present: %v", code, len(out), strings.Contains(out, last))
	}
	path := filepath.Join(home, "ep.vtt")
	if out, code := runCode(t, cliBin, env, "transcripts", "convert", "take-1", "--format", "vtt", "--out", path); code != 0 {
		t.Fatalf("--out: exit %d: %s", code, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), last) {
		t.Fatalf("--out file: %d bytes, last word missing", len(data))
	}
}
