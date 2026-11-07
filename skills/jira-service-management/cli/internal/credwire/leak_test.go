// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class's failing input, seen
// failing on the binaries built before the kit was wired. Set
// JIRA_SERVICE_MANAGEMENT_LEAK_BIN_DIR to run them against binaries built
// elsewhere.

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

	"jira-service-management-pp-cli/internal/credkit"
)

const testEmail = "agent@acme.example"

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
		if d := os.Getenv("JIRA_SERVICE_MANAGEMENT_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "jira-service-management-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{exe("jira-service-management-cli"), "../../cmd/jira-service-management-cli"},
			{exe("jira-service-management-mcp"), "../../cmd/jira-service-management-mcp"},
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
	return filepath.Join(binDir, exe("jira-service-management-cli")), filepath.Join(binDir, exe("jira-service-management-mcp"))
}

// vendor is a TLS stub of the Atlassian API: the kit never sends a credential
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
	return []string{"JIRA_SERVICE_MANAGEMENT_BASE_URL=" + v.url, "JIRA_SERVICE_MANAGEMENT_CA_CERT=" + v.caFile,
		"JIRA_SERVICE_MANAGEMENT_EMAIL=" + testEmail, "JIRA_SERVICE_MANAGEMENT_API_TOKEN=" + token}
}

// closedEnv is a closed environment under one sandbox home; every press
// directory resolves under it.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "JIRA_SERVICE_MANAGEMENT_HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg-config"), "XDG_DATA_HOME=" + filepath.Join(home, "xdg-data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "xdg-state"), "XDG_CACHE_HOME=" + filepath.Join(home, "xdg-cache"),
		"TMPDIR=" + os.TempDir(), "NO_COLOR=1"}, extra...)
}

func configDir(home string) string { return filepath.Join(home, "config") }

func run(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	return runStdin(t, bin, env, "", args...)
}

// runStdin runs bin with stdin and returns stdout plus stderr.
func runStdin(t *testing.T, bin string, env []string, stdin string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run() // exit codes vary by case; the output is judged
	// A refused start prints nothing, which every absence check would pass on.
	if cmd.ProcessState.ExitCode() == credkit.ExitRefused {
		t.Errorf("%s %s: the start was refused, so this run proves nothing:\n%s", filepath.Base(bin), strings.Join(args[:min(3, len(args))], " "), out.String())
	}
	return out.String()
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

// assertNoFileCarries walks root; skip names the files the test itself wrote.
func assertNoFileCarries(t *testing.T, root string, skip []string, secrets ...string) {
	t.Helper()
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
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
// echoes part of JIRA_SERVICE_MANAGEMENT_API_TOKEN. The generated masks replace
// the whole token and its encodings; a server that echoes a piece of it
// slipped past them. Any 8-character piece of a held value withholds the
// output, on stdout, stderr and the MCP server.
func TestAPIErrorEchoOfTokenIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const token = "jsmtok-" + "Zq81LmP0xV7c"
	piece := token[:12]
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"errorMessage":"token %s... rejected","errors":[]}`, piece)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env(token)...)
	assertAbsent(t, "servicedesk list", run(t, cliBin, env, "servicedesk", "list"), piece)
	assertAbsent(t, "servicedesk list --agent", run(t, cliBin, env, "servicedesk", "list", "--agent"), piece)
	assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, `{"name":"jira-service-management_read","arguments":{"endpoint_id":"servicedesk.list"}}`), piece)
	if v.hits.Load() == 0 {
		t.Error("no request reached the API stub, so nothing was echoed")
	}
	assertNoFileCarries(t, home, nil, piece)
}

// TestBasicCredentialRefusedOffOriginOverPlainHTTPAndAcrossRedirect (class
// origin): the Basic credential and a config [headers] value go only to the
// API they are configured for, never over plain http (the program's own rule
// allowed plain http to loopback), and never across a redirect to another
// origin (the feedback POST followed a 307 with its body).
func TestBasicCredentialRefusedOffOriginOverPlainHTTPAndAcrossRedirect(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "jsmtok-" + "Orig1nQ7r3s5"
	const header = "hdrval-" + "K3p9Wq2Lx7Rt"

	t.Run("off origin", func(t *testing.T) {
		api := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
			io.WriteString(w, `{"size":1,"start":0,"limit":50,"isLastPage":true,"values":[{"id":"1","projectKey":"SD"}]}`)
		})
		other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
		env := closedEnv(t.TempDir(), api.env(token)...)
		run(t, cliBin, env, "servicedesk", "list", "--json", "--deliver", "webhook:"+other.url+"/hook/"+token)
		if n := other.conns.Load(); n != 0 {
			t.Errorf("webhook to another https origin: %d connection(s) opened for a URL that carries the token", n)
		}
		if api.hits.Load() == 0 {
			t.Error("the API stub was never read, so no webhook was attempted")
		}
	})

	t.Run("plain http", func(t *testing.T) {
		var carried atomic.Int64
		plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "" || r.Header.Get("X-Acme-Key") != "" {
				carried.Add(1)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"size":0,"start":0,"limit":50,"isLastPage":true,"values":[]}`)
		}))
		t.Cleanup(plain.Close)
		home := t.TempDir()
		writeFile(t, filepath.Join(configDir(home), "config.toml"), "[headers]\nX-Acme-Key = \""+header+"\"\n")
		env := closedEnv(home, "JIRA_SERVICE_MANAGEMENT_BASE_URL="+plain.URL,
			"JIRA_SERVICE_MANAGEMENT_EMAIL="+testEmail, "JIRA_SERVICE_MANAGEMENT_API_TOKEN="+token)
		assertAbsent(t, "plain http API", run(t, cliBin, env, "servicedesk", "list"), token, header)
		if n := carried.Load(); n != 0 {
			t.Errorf("plain http API: %d request(s) carried the Basic credential or the config header", n)
		}
	})

	// The feedback POST is the one redirect-following client the program's own
	// redirect guard does not cover: Go's default policy replays a 307 body,
	// and keeps every header across a change of port alone. It trusts only the
	// system roots, so both ends are plain http on loopback; the kit refuses the
	// held token at the first hop, before any redirect.
	t.Run("cross-origin redirect", func(t *testing.T) {
		var carried atomic.Int64
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if b, _ := io.ReadAll(r.Body); bytes.Contains(b, []byte(token)) {
				carried.Add(1)
			}
			io.WriteString(w, `{}`)
		}))
		t.Cleanup(other.Close)
		endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, other.URL+"/collect", http.StatusTemporaryRedirect)
		}))
		t.Cleanup(endpoint.Close)
		env := closedEnv(t.TempDir(), "JIRA_SERVICE_MANAGEMENT_EMAIL="+testEmail, "JIRA_SERVICE_MANAGEMENT_API_TOKEN="+token,
			"JIRA_SERVICE_MANAGEMENT_FEEDBACK_ENDPOINT="+endpoint.URL+"/feedback")
		run(t, cliBin, env, "feedback", "--send", "pasted "+token+" here")
		if n := carried.Load(); n != 0 {
			t.Errorf("redirect to another origin: %d request(s) carried the token", n)
		}
	})
}

// TestJournalNeverRecordsAHeldCredential (class journal): the learn journal
// records a mistyped flag's name. A credential pasted where a flag name goes
// (--<token>) was written to state/learn/journal-*.jsonl; the journal appends
// through the kit (credkit.AppendFile), which withholds an entry carrying a held
// value.
func TestJournalNeverRecordsAHeldCredential(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "jsmtok" + "JournL8x2q9"
	home := t.TempDir()
	env := closedEnv(home, "JIRA_SERVICE_MANAGEMENT_EMAIL="+testEmail, "JIRA_SERVICE_MANAGEMENT_API_TOKEN="+token)
	run(t, cliBin, env, "servicedesk", "list", "--"+token)
	run(t, cliBin, env, "servicedesk", "list", "--custmer", token)
	journals, _ := filepath.Glob(filepath.Join(home, "state", "learn", "journal-*.jsonl"))
	if len(journals) == 0 {
		t.Fatal("no journal segment was written, so nothing was checked")
	}
	assertNoFileCarries(t, home, nil, token)
}

// TestAssetsMediaJwtTokenWithEscapedKeyNeverPrinted (class response field): an
// Assets object's avatar carries mediaJwtToken, a bearer JWT for Atlassian's
// media API. jsm_scrub.go drops it by its literal key; a key spelled with a
// JSON escape ("media\u004awtToken") slipped past it to stdout and the
// response cache. The kit holds the declared response field whatever its
// spelling.
func TestAssetsMediaJwtTokenWithEscapedKeyNeverPrinted(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "jsmtok-" + "Assets4Kq7Wd"
	const jwt = "mjwt" + "Q7v2Lp9x4KcRb8"
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":"88","objectKey":"ITSM-88","label":"srv-01","avatar":{"mediaClientConfig":{"clientId":"c1","media\u004awtToken":"%s"}}}`, jwt)
	})
	home := t.TempDir()
	env := closedEnv(home, append(v.env(token), "JIRA_SERVICE_MANAGEMENT_CLOUD_ID=cloud-1111", "JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID=ws-2222")...)
	assertAbsent(t, "object get", run(t, cliBin, env, "object", "get", "88"), jwt)
	assertAbsent(t, "object get --json", run(t, cliBin, env, "object", "get", "88", "--json"), jwt)
	if v.hits.Load() == 0 {
		t.Error("no request reached the Assets stub, so nothing was answered")
	}
	assertNoFileCarries(t, home, nil, jwt)
}

// TestMalformedConfigCredentialHeldAtStartup (class startup output): a config
// that does not parse still carries its API token. A flag-parse error that
// echoes the value is startup output, before any config is read. (A malformed
// file whose words occur in public text, "[headers" for one, refuses the start
// instead: the kit holds every word of it.)
func TestMalformedConfigCredentialHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "jsmtok-" + "Malf0rm3dQ8z"
	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "config.toml"),
		"service_management_email = \""+testEmail+"\"\nservice_management_api_token = \""+secret+"\"\nbroken = = \n")
	env := closedEnv(home)
	assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", secret, "version"), secret)
	assertAbsent(t, "CLI config load", run(t, cliBin, env, "doctor"), secret)
	assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", secret), secret)
}

// TestSetCredentialsAndProfileValueHeldAtStartup (class startup credential):
// the token given to `auth set-credentials` as a positional, and a run-profile
// value saved under a credential-like name (an older or hand-edited
// profiles.json), are held before any output.
func TestSetCredentialsAndProfileValueHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const tok = "jsmtok-" + "SetCr3dW9p2x"
	const secret = "jsmProf" + "Tok-5d4c3b2a19"
	home := t.TempDir()
	env := closedEnv(home)
	assertAbsent(t, "set-credentials flag error", run(t, cliBin, env, "auth", "set-credentials", "a@b.example", tok, "--timeout", tok), tok)

	writeFile(t, filepath.Join(configDir(home), "profiles.json"),
		`{"profiles":{"watcher":{"name":"watcher","values":{"api-token":"`+secret+`","limit":"5"}}}}`)
	assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", secret, "version"), secret)
	assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", secret), secret)
	assertAbsent(t, "profile show", run(t, cliBin, env, "profile", "show", "watcher"), secret)
}

// TestFreeTextStdinIsKept (class missing data): `feedback --stdin` and
// `request note --stdin` read free text. Held whole (as StdinFlags did, and
// as HoldBody holds bytes that are no document), the body was withheld from
// the command's own record of it. It is not held; a held credential pasted
// into it is still withheld.
func TestFreeTextStdinIsKept(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "jsmtokFeedbk3Lp7q"
	const text = "the servicedesk list output was confusing"
	home := t.TempDir()
	env := closedEnv(home, "JIRA_SERVICE_MANAGEMENT_EMAIL="+testEmail, "JIRA_SERVICE_MANAGEMENT_API_TOKEN="+token)
	runStdin(t, cliBin, env, text+"\nOK\n", "feedback", "--stdin")
	runStdin(t, cliBin, env, "pasted "+token+" by mistake\n", "feedback", "--stdin")
	record, err := os.ReadFile(filepath.Join(home, "data", "feedback.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(record), text) {
		t.Errorf("feedback.jsonl lost the feedback:\n%s", record)
	}
	assertNoFileCarries(t, home, nil, token)

	const note = "Vendor RMA pending"
	out := runStdin(t, cliBin, env, note+"\nOK\n", "request", "note", "SD-1", "--stdin", "--dry-run")
	if strings.Contains(out, "withheld") {
		t.Errorf("request note --stdin --dry-run was withheld:\n%s", out)
	}
}

// bodyVendor is an API stub for the --stdin request-body tests. A write is
// answered with its own body and an id, as Atlassian answers a create, so a
// credential field in the body comes back in the answer. It records every
// request it is sent.
type bodyVendor struct {
	*vendor
	mu   sync.Mutex
	sent []string
}

func startBodyVendor(t *testing.T) *bodyVendor {
	t.Helper()
	bv := &bodyVendor{}
	bv.vendor = startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bv.mu.Lock()
		bv.sent = append(bv.sent, r.Method+" "+r.URL.Path+" "+string(b))
		bv.mu.Unlock()
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/object/aql") {
			io.WriteString(w, `{"startAt":0,"maxResults":25,"total":1,"isLast":true,"values":[{"id":"1","label":"LT-0042","objectKey":"ITSM-42","objectType":{"id":"5","name":"Laptop"}}]}`)
			return
		}
		answer := map[string]any{}
		_ = json.Unmarshal(b, &answer)
		answer["id"] = "10010"
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(answer)
	})
	return bv
}

// sentWith reports whether a request whose method, path and body name want was sent.
func (bv *bodyVendor) sentWith(want ...string) bool {
	bv.mu.Lock()
	defer bv.mu.Unlock()
	return slices.ContainsFunc(bv.sent, func(s string) bool {
		for _, w := range want {
			if !strings.Contains(s, w) {
				return false
			}
		}
		return true
	})
}

// TestStdinRequestBodyAnswerIsPrinted (class missing data): on every command
// but feedback and request note, --stdin reads a JSON request body, no
// credential. Declared a StdinFlags name, the kit held the whole body, so a
// write (organization create, request comment add) was sent and printed only
// the withheld notice at exit 0, and a read (object find-by-aql) lost its
// whole answer, because Atlassian's answers repeat the body. The answer prints
// as it does for the same request made with flags. A body from `jq .` (one
// member per line) is read the same.
func TestStdinRequestBodyAnswerIsPrinted(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "jsmtok-" + "StdinBd7Kq2W"
	bv := startBodyVendor(t)
	home := t.TempDir()
	env := closedEnv(home, append(bv.env(token), "JIRA_SERVICE_MANAGEMENT_CLOUD_ID=cloud-1111", "JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID=ws-2222")...)
	for _, c := range []struct {
		label, stdin string
		args         []string
		want, sent   []string
	}{
		{"organization create --stdin (write)", `{"name":"Contoso Branch Office"}`,
			[]string{"organization", "create", "--stdin", "--yes", "--json"},
			[]string{"Contoso Branch Office", "10010"}, []string{"POST", "/organization", "Contoso Branch Office"}},
		{"request comment add --stdin, a jq-formatted body (write)", "{\n  \"body\": \"Replaced the toner, please confirm\",\n  \"public\": true\n}\n",
			[]string{"request", "comment", "add", "SD-1", "--stdin", "--yes", "--json"},
			[]string{"Replaced the toner, please confirm", "10010"}, []string{"POST", "/request/SD-1/comment", "Replaced the toner"}},
		{"object find-by-aql --stdin (read)", `{"qlQuery":"objectType = Laptop"}`,
			[]string{"object", "find-by-aql", "--stdin", "--json", "--no-cache"},
			[]string{"LT-0042", "ITSM-42"}, []string{"POST", "/object/aql", "objectType = Laptop"}},
	} {
		out := runStdin(t, cliBin, env, c.stdin, c.args...)
		if strings.Contains(out, "withheld") {
			t.Errorf("%s: output withheld:\n%s", c.label, out)
		}
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("%s: the answer lacks %q:\n%s", c.label, w, out)
			}
		}
		if !bv.sentWith(c.sent...) {
			t.Errorf("%s: no request %q reached the stub", c.label, c.sent)
		}
	}
	assertNoFileCarries(t, home, nil, token)
}

// TestStdinBodyCredentialFieldIsHeld (class body credential): a --stdin body
// is no credential source, but a credential field in it is held before
// anything prints (credkit.HoldBody), at any depth, so an answer that echoes
// it is withheld from stdout, stderr and every file.
func TestStdinBodyCredentialFieldIsHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "jsmtok-" + "StdinCr4Wz8P"
	const auth = "jsmBodyAuth" + "Q7x2Lw9Kd3"
	const nested = "jsmBodyNest" + "R5v8Mp1Jt6"
	bv := startBodyVendor(t)
	home := t.TempDir()
	env := closedEnv(home, bv.env(token)...)
	out := runStdin(t, cliBin, env, `{"name":"Fabrikam Field Office","authorization":"`+auth+`"}`,
		"organization", "create", "--stdin", "--yes", "--json")
	assertAbsent(t, "organization create --stdin, authorization in the body", out, auth)
	out = runStdin(t, cliBin, env, `{"serviceDeskId":"10","requestTypeId":"25","requestFieldValues":{"summary":"Scanner offline","service_management_api_token":"`+nested+`"}}`,
		"request", "create", "--stdin", "--yes", "--json")
	assertAbsent(t, "request create --stdin, a nested service_management_api_token", out, nested)
	if !bv.sentWith("/organization", "Fabrikam Field Office") || !bv.sentWith("/request", "Scanner offline") {
		t.Errorf("the writes did not reach the stub, so nothing was echoed: %q", bv.sent)
	}
	assertNoFileCarries(t, home, nil, auth, nested)
}

// TestStdinBodyDryRunPreview (class missing data, and body credential): a
// --dry-run preview of a --stdin body prints the body and sends nothing; a
// credential field in that body is withheld from the preview.
func TestStdinBodyDryRunPreview(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "jsmtok-" + "StdinDr6Hn3V"
	const auth = "jsmDryAuth" + "W4k9Zq2Xc7"
	bv := startBodyVendor(t)
	home := t.TempDir()
	env := closedEnv(home, bv.env(token)...)
	out := runStdin(t, cliBin, env, `{"name":"Northwind Depot"}`, "organization", "create", "--stdin", "--dry-run")
	if strings.Contains(out, "withheld") || !strings.Contains(out, "Northwind Depot") {
		t.Errorf("organization create --stdin --dry-run: the preview lacks the body or was withheld:\n%s", out)
	}
	out = runStdin(t, cliBin, env, `{"name":"Northwind Depot","authorization":"`+auth+`"}`, "organization", "create", "--stdin", "--dry-run")
	assertAbsent(t, "organization create --stdin --dry-run, authorization in the body", out, auth)
	if n := bv.hits.Load(); n != 0 {
		t.Errorf("--dry-run sent %d request(s)", n)
	}
	assertNoFileCarries(t, home, nil, auth)
}
