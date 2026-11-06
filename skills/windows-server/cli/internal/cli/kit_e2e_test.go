// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the shared credential kit's guarantees for
// this connector, proved on the built binaries against a fake WinRM listener.
// Pinned by this connector's handfixes.json under the "credkit-wiring" entry.

package cli

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"windows-server-pp-cli/internal/winsrv"
)

// The kit's Start runs only in main, so its guarantees are proved on the
// built binaries. WINDOWS_SERVER_E2E_BIN (and WINDOWS_SERVER_E2E_MCP_BIN, by
// default the MCP server beside it) runs these same tests against another
// build, which is how each class's test was seen failing on the commit before
// the kit.
var (
	kitBinOnce           sync.Once
	kitBinCLI, kitBinMCP string
	kitBinErr            error
	// The user cache before TestMain moves HOME into the sandbox, so the
	// build below reuses the real Go build cache instead of a cold one.
	kitUserCache, _ = os.UserCacheDir()
)

func kitExe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func kitBins(t *testing.T) (string, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the binaries")
	}
	if c := os.Getenv("WINDOWS_SERVER_E2E_BIN"); c != "" {
		m := os.Getenv("WINDOWS_SERVER_E2E_MCP_BIN")
		if m == "" {
			m = filepath.Join(filepath.Dir(c), kitExe("windows-server-mcp"))
		}
		return c, m
	}
	kitBinOnce.Do(func() {
		// Under the sandbox home TestMain made, which it removes at exit.
		dir, err := os.MkdirTemp(os.Getenv("HOME"), "kit-e2e-bin")
		if err != nil {
			kitBinErr = err
			return
		}
		cmd := exec.Command("go", "build", "-o", dir+string(filepath.Separator), "../../cmd/windows-server-cli", "../../cmd/windows-server-mcp")
		cmd.Env = os.Environ()
		if os.Getenv("GOCACHE") == "" && kitUserCache != "" {
			cmd.Env = append(cmd.Env, "GOCACHE="+filepath.Join(kitUserCache, "go-build"))
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			kitBinErr = fmt.Errorf("%w\n%s", err, out)
			return
		}
		kitBinCLI, kitBinMCP = filepath.Join(dir, kitExe("windows-server-cli")), filepath.Join(dir, kitExe("windows-server-mcp"))
	})
	if kitBinErr != nil {
		t.Fatal(kitBinErr)
	}
	return kitBinCLI, kitBinMCP
}

// kitCmd is bin with a home of its own and env.
func kitCmd(bin, home string, env []string, args ...string) *exec.Cmd {
	cmd := exec.Command(bin, args...)
	cmd.Env = append([]string{
		"HOME=" + home, "USERPROFILE=" + home, "WINDOWS_SERVER_HOME=" + home,
		"PATH=" + os.Getenv("PATH"), "SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "TMP=" + os.TempDir(),
	}, env...)
	return cmd
}

// kitRun runs the CLI and returns stdout, stderr and the exit code.
func kitRun(t *testing.T, bin, home string, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := kitCmd(bin, home, env, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return stdout.String(), stderr.String(), code
}

// kitMCPCall starts the MCP server over stdio, calls one tool and returns the
// raw JSON-RPC line that answered it.
func kitMCPCall(t *testing.T, bin, home string, env []string, tool string, args map[string]any) string {
	t.Helper()
	return kitMCPSession(t, bin, home, env, kitStep{tool: tool, args: args})[0]
}

// kitStep is one tool call of an MCP session; before runs first.
type kitStep struct {
	tool   string
	args   map[string]any
	before func()
}

// kitMCPSession starts one MCP server over stdio and makes each call in turn,
// returning the raw JSON-RPC line that answered each.
func kitMCPSession(t *testing.T, bin, home string, env []string, steps ...kitStep) []string {
	t.Helper()
	cmd := kitCmd(bin, home, env)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = in.Close()
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}()
	lines := make(chan string)
	go func() {
		r := bufio.NewReader(out)
		for {
			line, err := r.ReadString('\n')
			if line != "" {
				lines <- line
			}
			if err != nil {
				close(lines)
				return
			}
		}
	}()
	send := func(v map[string]any) {
		v["jsonrpc"] = "2.0"
		b, _ := json.Marshal(v)
		if _, err := in.Write(append(b, '\n')); err != nil {
			t.Fatalf("writing to the MCP server: %v\n%s", err, stderr.String())
		}
	}
	await := func(id int) string {
		for {
			select {
			case line, ok := <-lines:
				if !ok {
					t.Fatalf("the MCP server closed stdout before answering %d:\n%s", id, stderr.String())
				}
				var msg struct {
					ID *int `json:"id"`
				}
				if json.Unmarshal([]byte(line), &msg) == nil && msg.ID != nil && *msg.ID == id {
					return line
				}
			case <-time.After(90 * time.Second):
				t.Fatalf("no MCP answer to %d:\n%s", id, stderr.String())
			}
		}
	}
	send(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "kit-e2e", "version": "0"},
	}})
	await(1)
	send(map[string]any{"method": "notifications/initialized"})
	var got []string
	for i, st := range steps {
		if st.before != nil {
			st.before()
		}
		send(map[string]any{"id": i + 2, "method": "tools/call", "params": map[string]any{"name": st.tool, "arguments": st.args}})
		got = append(got, await(i+2))
	}
	return got
}

// kitWinRM is a WinRM listener that answers each registered script with a
// fixed stdout, picked by the WSPP_SCRIPT payload of the shell it created.
type kitWinRM struct {
	srv      *httptest.Server
	password string
	outputs  map[string]string
	scripts  map[string]string // WSPP_SCRIPT payload -> script name
	mu       sync.Mutex
	shells   map[string]string // ShellId -> script name
	ran      map[string]int
	// cookie, when set, is a Set-Cookie line sent on every reply: as a
	// header, and also as a trailer when trailer is true. fault, when set,
	// makes Receive answer HTTP 500 with a SOAP fault carrying that text.
	cookie  string
	trailer bool
	fault   string
	// ntlm makes the listener answer without asking for authentication, as
	// an NTLM negotiator's first hop is answered when no challenge comes:
	// the password then never crosses the wire.
	ntlm bool
	// outputFn, when it names a script, answers it instead of outputs.
	outputFn map[string]func() string
}

var (
	kitActionRE = regexp.MustCompile(`<a:Action[^>]*>([^<]+)</a:Action>`)
	kitScriptRE = regexp.MustCompile(`<rsp:Variable Name="` + winsrv.ScriptEnv + `">([^<]*)</rsp:Variable>`)
	kitShellRE  = regexp.MustCompile(`<w:Selector Name="ShellId">([^<]+)</w:Selector>`)
)

func newKitWinRM(t *testing.T, password string, outputs map[string]string) *kitWinRM {
	t.Helper()
	f := &kitWinRM{password: password, outputs: outputs, scripts: map[string]string{}, shells: map[string]string{}, ran: map[string]int{}}
	for _, name := range winsrv.Names() {
		_, _, payload, err := winsrv.CommandLine(name)
		if err != nil {
			t.Fatal(err)
		}
		f.scripts[payload] = name
	}
	f.srv = httptest.NewTLSServer(f)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *kitWinRM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, pass, ok := r.BasicAuth(); !f.ntlm && (!ok || pass != f.password) {
		w.Header().Set("WWW-Authenticate", "Basic")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	raw, _ := io.ReadAll(r.Body)
	body := string(raw)
	if f.cookie != "" {
		if f.trailer {
			w.Header().Set("Trailer", "Set-Cookie")
			defer func() { w.Header().Set("Set-Cookie", f.cookie) }()
		} else {
			w.Header().Add("Set-Cookie", f.cookie)
		}
	}
	m := kitActionRE.FindStringSubmatch(body)
	if m == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	reply := func(inner string) {
		_, _ = io.WriteString(w, `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:rsp="http://schemas.microsoft.com/wbem/wsman/1/windows/shell" xmlns:w="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd" xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:x="http://schemas.xmlsoap.org/ws/2004/09/transfer"><s:Body>`+inner+`</s:Body></s:Envelope>`)
	}
	shell := ""
	if s := kitShellRE.FindStringSubmatch(body); s != nil {
		shell = s[1]
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch path.Base(m[1]) {
	case "Create":
		name := ""
		if s := kitScriptRE.FindStringSubmatch(body); s != nil {
			name = f.scripts[s[1]]
		}
		id := fmt.Sprintf("SHELL-%d", len(f.shells)+1)
		f.shells[id] = name
		reply(`<x:ResourceCreated><a:ReferenceParameters><w:SelectorSet><w:Selector Name="ShellId">` + id + `</w:Selector></w:SelectorSet></a:ReferenceParameters></x:ResourceCreated>`)
	case "Command":
		reply(`<rsp:CommandResponse><rsp:CommandId>CMD-1</rsp:CommandId></rsp:CommandResponse>`)
	case "Receive":
		if f.fault != "" {
			w.Header().Set("Content-Type", "application/soap+xml")
			w.WriteHeader(http.StatusInternalServerError)
			reply(`<s:Fault><s:Reason><s:Text xml:lang="en-US">` + f.fault + `</s:Text></s:Reason></s:Fault>`)
			return
		}
		name := f.shells[shell]
		out, ok := f.outputs[name]
		if fn, isFn := f.outputFn[name]; isFn {
			out, ok = fn(), true
		}
		if !ok {
			out = `{"ok":false,"error":{"kind":"role_absent","message":"no fixture"}}`
		}
		f.ran[name]++
		reply(`<rsp:ReceiveResponse><rsp:Stream Name="stdout" CommandId="CMD-1">` + base64.StdEncoding.EncodeToString([]byte(out)) + `</rsp:Stream><rsp:Stream Name="stderr" CommandId="CMD-1" End="true"></rsp:Stream><rsp:CommandState CommandId="CMD-1" State="http://schemas.microsoft.com/wbem/wsman/1/windows/shell/CommandState/Done"><rsp:ExitCode>0</rsp:ExitCode></rsp:CommandState></rsp:ReceiveResponse>`)
	case "Signal":
		reply(`<rsp:SignalResponse/>`)
	case "Delete":
		reply(``)
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

func (f *kitWinRM) runs(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ran[name]
}

// server is the [[servers]] entry for name, pointed at this listener and
// pinned to its certificate.
func (f *kitWinRM) server(name string) string {
	u, _ := url.Parse(f.srv.URL)
	host, port, _ := net.SplitHostPort(u.Host)
	sum := sha256.Sum256(f.srv.Certificate().Raw)
	return fmt.Sprintf("[[servers]]\nname = %q\nhost = %q\nport = %s\nauth = \"basic\"\ncert_sha256 = %q\n\n", name, host, port, hex.EncodeToString(sum[:]))
}

// kitEnv writes a config of servers outside home and returns the environment
// that selects it with the shared WinRM pair.
func kitEnv(t *testing.T, password string, servers ...string) []string {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte(strings.Join(servers, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	return []string{"WINDOWS_SERVER_CONFIG=" + cfg, `WINDOWS_SERVER_USERNAME=CORP\svc-monitor`, "WINDOWS_SERVER_PASSWORD=" + password}
}

func kitEnvelope(v any) string {
	b, _ := json.Marshal(v)
	return `{"ok":true,"data":` + string(b) + `}`
}

// kitCodeForms spells s one character at a time, each spelling a member of
// the kit contract's character-code lists, character references or string
// escapes, the ways a script or a Windows tool echoes text it was handed.
func kitCodeForms(s string) map[string]string {
	each := func(format, sep string) string {
		var parts []string
		for _, r := range s {
			parts = append(parts, fmt.Sprintf(format, r))
		}
		return strings.Join(parts, sep)
	}
	return map[string]string{
		"decimal list":        each("%d", ","),
		"decimal list spaced": each("%d", ", "),
		"char array":          "-join [char[]](" + each("%d", ",") + ")",
		"decimal references":  each("&#%d;", ""),
		"hex references":      each("&#x%x;", ""),
		"0x list":             each("0x%02x", ","),
		"0x list spaced":      each("0x%02X", " "),
		"hex escapes":         each(`\x%02x`, ""),
		"unicode escapes":     each(`\u%04x`, ""),
		"octal escapes":       each(`\%03o`, ""),
	}
}

// kitSpellings is text as written and as a JSON string carries it.
func kitSpellings(text string) []string {
	b, _ := json.Marshal(text)
	return []string{text, strings.Trim(string(b), `"`)}
}

func kitAssertAbsent(t *testing.T, where, got string, values ...string) {
	t.Helper()
	for _, v := range values {
		for _, s := range kitSpellings(v) {
			if strings.Contains(got, s) {
				t.Errorf("%s carries %q:\n%s", where, s, got)
			}
		}
	}
}

// TestKitPerCharacterCodesNeverReachTheReader is the class
// funnel-per-character-code-decoding: the WinRM password, a piece of it or
// the password in another case, echoed back by a server one character code at
// a time, reaches neither CLI stdout nor an MCP tool result. Seen failing on
// the commit before the kit in
// .review/kit/seen-failing-funnel-per-character-code-decoding.log.
func TestKitPerCharacterCodesNeverReachTheReader(t *testing.T) {
	cli, mcp := kitBins(t)
	volumes := func(label string) map[string]string {
		return map[string]string{"volumes": kitEnvelope([]map[string]any{{"drive_letter": "C", "percent_free": 42.0, "health_status": "Healthy", "label": label}})}
	}
	echoes := map[string]string{
		"password":   testPassword,
		"piece":      testPassword[:12], // 8 or more characters of it count
		"upper case": strings.ToUpper(testPassword),
	}
	for echo, value := range echoes {
		for name, form := range kitCodeForms(value) {
			t.Run(echo+"/"+name, func(t *testing.T) {
				fake := newKitWinRM(t, testPassword, volumes("pw "+form))
				env := kitEnv(t, testPassword, fake.server("fs01"))
				home := t.TempDir()
				for _, args := range [][]string{{"volumes"}, {"volumes", "--agent"}} {
					stdout, stderr, _ := kitRun(t, cli, home, env, args...)
					kitAssertAbsent(t, "windows-server-cli "+strings.Join(args, " ")+" stdout", stdout, form, value)
					kitAssertAbsent(t, "windows-server-cli "+strings.Join(args, " ")+" stderr", stderr, form, value)
				}
				kitAssertAbsent(t, "MCP volumes_list", kitMCPCall(t, mcp, home, env, "volumes_list", map[string]any{}), form, value)
				if n := fake.runs("volumes"); n < 3 {
					t.Fatalf("the volumes script ran %d times, want 3 (two CLI runs and one MCP call)", n)
				}
			})
		}
	}
	// The other direction: the same spelling of a value nothing holds is
	// printed, so an absence above is the kit's doing.
	t.Run("unheld value is printed", func(t *testing.T) {
		form := kitCodeForms("Unheld-Label-Value")["decimal list"]
		fake := newKitWinRM(t, testPassword, volumes("pw "+form))
		env := kitEnv(t, testPassword, fake.server("fs01"))
		home := t.TempDir()
		if stdout, stderr, code := kitRun(t, cli, home, env, "volumes", "--agent"); !strings.Contains(stdout, form) {
			t.Fatalf("volumes --agent (exit %d) does not print the label %q:\n%s\n%s", code, form, stdout, stderr)
		}
		if got := kitMCPCall(t, mcp, home, env, "volumes_list", map[string]any{}); !strings.Contains(got, form) {
			t.Fatalf("MCP volumes_list does not carry the label %q:\n%s", form, got)
		}
	})
}

// kitStoredServers is every health_runs.server and inventory_runs.server
// value in the SQLite files under home.
func kitStoredServers(t *testing.T, home string) []string {
	t.Helper()
	var servers []string
	err := filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".db") {
			return err
		}
		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(p)+"?mode=ro")
		if err != nil {
			return err
		}
		defer db.Close()
		for _, table := range []string{"health_runs", "inventory_runs"} {
			rows, err := db.Query(`SELECT server FROM ` + table)
			if err != nil {
				continue // a store without the table
			}
			for rows.Next() {
				var s string
				if err := rows.Scan(&s); err != nil {
					rows.Close()
					return err
				}
				servers = append(servers, table+": "+s)
			}
			rows.Close()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return servers
}

// kitFilesHolding names every file under root that carries s in any case.
func kitFilesHolding(t *testing.T, root, s string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(strings.ToLower(string(b)), strings.ToLower(s)) {
			found = append(found, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// TestKitStoreNeverHoldsTheServerPassword is the class store-sink-filtering:
// a server named after its WinRM password (or a piece of it, in any case)
// never reaches health_runs.server, inventory_runs.server or any other file
// under home, whatever health and inventory snapshot do with the run. Seen
// failing on the commit before the kit in
// .review/kit/seen-failing-store-sink-filtering.log.
func TestKitStoreNeverHoldsTheServerPassword(t *testing.T) {
	cli, _ := kitBins(t)
	fixtures := leakyFixtures("Unrelated-Fixture-Value").Outputs
	cases := []struct{ name, password, server string }{
		{"name equals the password", "backup9-admin", "backup9-admin"},
		{"name contains the password", "backup9-admin", "site2-backup9-admin"},
		{"name is the password in another case", "Backup9-Admin", "backup9-admin"},
		{"name is a piece of the password", "backup9-admin-x1", "backup9-admin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newKitWinRM(t, tc.password, fixtures)
			env := kitEnv(t, tc.password, fake.server(tc.server))
			home := t.TempDir()
			for _, args := range [][]string{{"health", "--agent"}, {"inventory", "snapshot", "--agent"}} {
				kitRun(t, cli, home, env, args...)
			}
			if fake.runs("system") == 0 {
				t.Fatal("no script ran: the runs never reached the server")
			}
			for _, row := range kitStoredServers(t, home) {
				if strings.Contains(strings.ToLower(row), tc.server) {
					t.Errorf("the store holds the server named after its password: %s", row)
				}
			}
			if files := kitFilesHolding(t, home, tc.server); len(files) > 0 {
				t.Errorf("files under home carry the server named after its password: %v", files)
			}
		})
	}
	// The other direction: an ordinary server's runs are stored, so an
	// empty store above is the kit's doing.
	t.Run("ordinary server is stored", func(t *testing.T) {
		fake := newKitWinRM(t, testPassword, fixtures)
		env := kitEnv(t, testPassword, fake.server("fs01"))
		home := t.TempDir()
		for _, args := range [][]string{{"health", "--agent"}, {"inventory", "snapshot", "--agent"}} {
			if stdout, stderr, code := kitRun(t, cli, home, env, args...); code != 0 {
				t.Fatalf("%v exited %d:\n%s\n%s", args, code, stdout, stderr)
			}
		}
		got := strings.Join(kitStoredServers(t, home), "\n")
		for _, want := range []string{"health_runs: fs01", "inventory_runs: fs01"} {
			if !strings.Contains(got, want) {
				t.Fatalf("the store lacks %q:\n%s", want, got)
			}
		}
	})
}

// TestKitMalformedConfigValueNeverInStartupOutput drives the binary with a
// --config file TOML rejects. Its values sit under a typo of a credential key
// and, in the second case, under a credential key past the first 4 MiB of the
// file. The parse error is the only output the run can produce; no value from
// the file may appear in it.
func TestKitMalformedConfigValueNeverInStartupOutput(t *testing.T) {
	cli, _ := kitBins(t)
	cases := []struct {
		name, secret string
		body         func(secret string) string
	}{
		{"unlisted key", "Typo-Key-Startup-Value1", func(s string) string {
			return "[[servers]]\nname = \"fs01\"\nhost = \"fs01.corp.example\"\npasword = \"" + s + "\"\n[[servers]\n"
		}},
		{"past 4 MiB", "Large-File-Startup-Value2", func(s string) string {
			return "# " + strings.Repeat("x", 5<<20) + "\nserver_password = \"" + s + "\"\n[[servers]\n"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(cfg, []byte(tc.body(tc.secret)), 0o600); err != nil {
				t.Fatal(err)
			}
			stdout, stderr, code := kitRun(t, cli, t.TempDir(), nil, "--config", cfg, "volumes", "--agent")
			if code == 0 {
				t.Fatalf("a config TOML rejects was accepted:\n%s\n%s", stdout, stderr)
			}
			kitAssertAbsent(t, "startup output", stdout+stderr, tc.secret, base64.StdEncoding.EncodeToString([]byte(tc.secret)))
		})
	}
}

// TestKitWinRMCookiesNeverReachTheReader is round 1's input and its class:
// the WinRM leg is outside credkit.Transport, so every credential it receives
// (a Set-Cookie value, in a header or a trailer) must be held before the
// response reaches the program. The server then echoes the cookie value in a
// SOAP fault and in script output; neither reaches CLI stdout, stderr or an
// MCP result.
func TestKitWinRMCookiesNeverReachTheReader(t *testing.T) {
	cli, mcp := kitBins(t)
	const secret = "Qz7vKw2J" + "xN4pLr8T"
	vols := map[string]string{"volumes": kitEnvelope([]map[string]any{{"drive_letter": "C", "percent_free": 42.0, "health_status": "Healthy", "label": "pw " + secret}})}
	cases := map[string]*kitWinRM{}
	cases["header, fault"] = newKitWinRM(t, testPassword, vols)
	cases["header, fault"].cookie, cases["header, fault"].fault = "sid="+secret+"; Path=/", secret
	cases["header, output"] = newKitWinRM(t, testPassword, vols)
	cases["header, output"].cookie = "sid=" + secret + "; Path=/; HttpOnly"
	cases["trailer, output"] = newKitWinRM(t, testPassword, vols)
	cases["trailer, output"].cookie, cases["trailer, output"].trailer = "sid="+secret, true
	cases["unparsed line"] = newKitWinRM(t, testPassword, vols)
	cases["unparsed line"].cookie = secret
	for name, fake := range cases {
		t.Run(name, func(t *testing.T) {
			env := kitEnv(t, testPassword, fake.server("fs01"))
			home := t.TempDir()
			for _, args := range [][]string{{"volumes"}, {"volumes", "--agent"}} {
				stdout, stderr, _ := kitRun(t, cli, home, env, args...)
				kitAssertAbsent(t, "windows-server-cli "+strings.Join(args, " ")+" stdout", stdout, secret)
				kitAssertAbsent(t, "windows-server-cli "+strings.Join(args, " ")+" stderr", stderr, secret)
			}
			kitAssertAbsent(t, "MCP volumes_list", kitMCPCall(t, mcp, home, env, "volumes_list", map[string]any{}), secret)
		})
	}
	// The other direction: with no cookie the same label is printed, so an
	// absence above is the hold's doing.
	t.Run("no cookie, label printed", func(t *testing.T) {
		fake := newKitWinRM(t, testPassword, vols)
		env := kitEnv(t, testPassword, fake.server("fs01"))
		if stdout, stderr, code := kitRun(t, cli, t.TempDir(), env, "volumes", "--agent"); !strings.Contains(stdout, secret) {
			t.Fatalf("volumes --agent (exit %d) does not print the label:\n%s\n%s", code, stdout, stderr)
		}
	})
}

// TestKitPasswordSavedWhileMCPRunsIsHeld is the adversarial review's input:
// a server password saved with servers login after the long-lived MCP server
// started, used over NTLM (the default, which never puts the password in a
// header), and then echoed back by the server, never reaches an MCP result.
// Its class: every credential file read after Start holds what it read.
func TestKitPasswordSavedWhileMCPRunsIsHeld(t *testing.T) {
	cli, mcp := kitBins(t)
	const oldPass, newPass = "Old7Kq2vWz9xLm4P", "Rot8Nb3cYt6sHj5Q"
	var mu sync.Mutex
	label := "no echo yet"
	fake := newKitWinRM(t, oldPass, nil)
	fake.ntlm = true
	fake.outputFn = map[string]func() string{"volumes": func() string {
		mu.Lock()
		defer mu.Unlock()
		return kitEnvelope([]map[string]any{{"drive_letter": "C", "percent_free": 42.0, "health_status": "Healthy", "label": label}})
	}}
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte(strings.Replace(fake.server("fs01"), `auth = "basic"`, `auth = "ntlm"`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{"WINDOWS_SERVER_CONFIG=" + cfg}
	home := t.TempDir()
	login := func(pw string) {
		cmd := kitCmd(cli, home, env, "servers", "login", "fs01", "--username", `CORP\svc-monitor`)
		cmd.Stdin = strings.NewReader(pw + "\n")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("servers login: %v\n%s", err, out)
		}
	}
	login(oldPass)
	got := kitMCPSession(t, mcp, home, env,
		kitStep{tool: "volumes_list", args: map[string]any{}},
		kitStep{tool: "volumes_list", args: map[string]any{}, before: func() {
			login(newPass)
			mu.Lock()
			label = "pw " + newPass
			mu.Unlock()
		}},
	)
	if !strings.Contains(got[0], "no echo yet") {
		t.Fatalf("the first call did not reach the server:\n%s", got[0])
	}
	kitAssertAbsent(t, "MCP volumes_list after the rotation", got[1], newPass)
	if n := fake.runs("volumes"); n < 2 {
		t.Fatalf("the volumes script ran %d times, want 2 (one per call)", n)
	}
}

// TestAuthSaveKeepsTheServers is the adversarial review's input: auth
// set-credentials and auth logout rewrite the config file, and the rewrite
// dropped every [[servers]] entry, after which every command said "no servers
// configured". Both now keep them; a config that does not parse is not
// rewritten at all.
func TestAuthSaveKeepsTheServers(t *testing.T) {
	cli, _ := kitBins(t)
	cfg := filepath.Join(t.TempDir(), "config.toml")
	servers := "[[servers]]\nname = \"fs01\"\nhost = \"fs01.corp.example\"\n\n[[servers]]\nname = \"dc01\"\nhost = \"10.0.0.5\"\nport = 5986\n"
	if err := os.WriteFile(cfg, []byte(servers), 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{"WINDOWS_SERVER_CONFIG=" + cfg}
	home := t.TempDir()
	for _, args := range [][]string{{"auth", "set-credentials", `CORP\shared`, "Shr9Wq4vKz7xLm2P"}, {"auth", "logout"}} {
		if stdout, stderr, code := kitRun(t, cli, home, env, args...); code != 0 {
			t.Fatalf("%v: exit %d\n%s\n%s", args, code, stdout, stderr)
		}
		stdout, stderr, code := kitRun(t, cli, home, env, "servers", "list", "--json")
		if code != 0 || !strings.Contains(stdout, "fs01") || !strings.Contains(stdout, "dc01") {
			t.Fatalf("after %v, servers list (exit %d) lost a server:\n%s\n%s", args, code, stdout, stderr)
		}
	}
	if err := os.WriteFile(cfg, []byte(servers+"[[servers]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, code := kitRun(t, cli, home, env, "auth", "set-credentials", `CORP\shared`, "Shr9Wq4vKz7xLm2P"); code == 0 {
		t.Fatal("auth set-credentials rewrote a config that does not parse")
	}
	if b, _ := os.ReadFile(cfg); !strings.Contains(string(b), "dc01") {
		t.Fatalf("the unparsable config was rewritten:\n%s", b)
	}
}
