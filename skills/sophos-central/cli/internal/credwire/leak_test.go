// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class's failing input, seen
// failing on the binaries built before the kit was wired. Set
// SOPHOS_CENTRAL_LEAK_BIN_DIR to run them against binaries built elsewhere.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/fs"
	"math/big"
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

	"sophos-central-pp-cli/internal/credkit"
)

const tokenPath = "/api/v2/oauth2/token"

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
		if d := os.Getenv("SOPHOS_CENTRAL_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "sophos-central-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{exe("sophos-central-cli"), "../../cmd/sophos-central-cli"},
			{exe("sophos-central-mcp"), "../../cmd/sophos-central-mcp"},
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
	return filepath.Join(binDir, exe("sophos-central-cli")), filepath.Join(binDir, exe("sophos-central-mcp"))
}

// vendor is a TLS stub of both the Sophos token endpoint and the API: the kit
// never sends a credential over plain http, loopback included. It records
// every request it is sent.
type vendor struct {
	url, caFile string
	hits        atomic.Int64
	mu          sync.Mutex
	sent        []string
}

func startVendor(t *testing.T, h http.HandlerFunc) *vendor {
	t.Helper()
	v := &vendor{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.hits.Add(1)
		b, _ := io.ReadAll(r.Body)
		v.mu.Lock()
		v.sent = append(v.sent, r.Method+" "+r.URL.Path+" "+string(b))
		v.mu.Unlock()
		r.Body = io.NopCloser(bytes.NewReader(b))
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

// env points the program at the stub, the token endpoint included, with this
// API credential.
func (v *vendor) env(clientID, clientSecret string) []string {
	return []string{"SOPHOS_CENTRAL_BASE_URL=" + v.url, "SOPHOS_CENTRAL_TOKEN_URL=" + v.url + tokenPath,
		"SOPHOS_CENTRAL_CA_CERT=" + v.caFile, "SOPHOS_CENTRAL_CLIENT_ID=" + clientID, "SOPHOS_CENTRAL_CLIENT_SECRET=" + clientSecret}
}

// sentWith reports whether a request whose method, path and body name want was sent.
func (v *vendor) sentWith(want ...string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return slices.ContainsFunc(v.sent, func(s string) bool {
		for _, w := range want {
			if !strings.Contains(s, w) {
				return false
			}
		}
		return true
	})
}

// mintOK answers the token endpoint with this bearer and reports whether r
// was a token request.
func mintOK(w http.ResponseWriter, r *http.Request, bearer string) bool {
	if r.URL.Path != tokenPath {
		return false
	}
	fmt.Fprintf(w, `{"access_token":"%s","token_type":"bearer","expires_in":3600}`, bearer)
	return true
}

// closedEnv is a closed environment under one sandbox home; every press
// directory resolves under it, and the API and token URLs default to a closed
// loopback port, so nothing reaches Sophos.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "SOPHOS_CENTRAL_HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg-config"), "XDG_DATA_HOME=" + filepath.Join(home, "xdg-data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "xdg-state"), "XDG_CACHE_HOME=" + filepath.Join(home, "xdg-cache"),
		"TMPDIR=" + os.TempDir(), "NO_COLOR=1",
		"SOPHOS_CENTRAL_BASE_URL=https://127.0.0.1:9", "SOPHOS_CENTRAL_TOKEN_URL=https://127.0.0.1:9" + tokenPath}, extra...)
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
	cmd.Dir = t.TempDir()
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

// leaks reports the first 8-character piece of secret that out carries.
func leaks(out, secret string) string {
	for i := 0; i+8 <= len(secret); i++ {
		if strings.Contains(out, secret[i:i+8]) {
			return secret[i : i+8]
		}
	}
	return ""
}

func assertAbsent(t *testing.T, label, out string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if p := leaks(out, s); p != "" {
			t.Errorf("%s shows %q:\n%s", label, p, out)
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
			if piece := leaks(string(data), s); piece != "" {
				t.Errorf("%s holds %q", strings.TrimPrefix(p, root), piece)
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

// TestTokenEndpointEchoNeverPrintsTheClientSecret (the kit note): the token
// endpoint answers a client-credentials request with HTTP 400 and an
// error_description that is the client secret. `auth login` printed it; so did
// the automatic mint (client.go) on an API command, with --agent and through
// the MCP server. The secret came only from the environment, so the
// connector's own mask, which knew the config's values, missed it.
func TestTokenEndpointEchoNeverPrintsTheClientSecret(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "EchoSecret42"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"invalid_client","error_description":"EchoSecret42"}`)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env("example", secret)...)
	assertAbsent(t, "auth login", run(t, cliBin, env, "auth", "login"), secret)
	if !v.sentWith("POST", tokenPath, "client_secret=") {
		t.Fatal("auth login never posted to the token endpoint, so nothing was echoed")
	}
	before := v.hits.Load()
	assertAbsent(t, "endpoints list (automatic mint)", run(t, cliBin, env, "endpoints", "list"), secret)
	assertAbsent(t, "endpoints list --agent", run(t, cliBin, env, "endpoints", "list", "--agent"), secret)
	assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, `{"name":"sophos-central_read","arguments":{"endpoint_id":"endpoints.list"}}`), secret)
	if v.hits.Load() == before {
		t.Error("no API command reached the token endpoint, so the automatic mint was never tried")
	}
	assertNoFileCarries(t, home, nil, secret)
}

// TestLearnJournalNeverStoresAHeldCredential (class journal): the learn
// journal records argv flag names as typed. An environment secret typed as a
// flag name, and a --client-secret value that starts with a dash, are both
// held credentials; neither lands in any file the CLI writes.
func TestLearnJournalNeverStoresAHeldCredential(t *testing.T) {
	cliBin, _ := binaries(t)
	const envSecret = "uY6p" + "Hd3JsQ9cWe2F"
	const flagSecret = "-Nk5t" + "Gb8RvM3xZa7"
	home := t.TempDir()
	env := closedEnv(home, "SOPHOS_CENTRAL_CLIENT_SECRET="+envSecret)
	run(t, cliBin, env, "auth", "status", "--journal-probe")
	run(t, cliBin, env, "auth", "status", "--"+envSecret)
	run(t, cliBin, env, "auth", "login", "--client-id", "partner-"+"id-4417", "--client-secret", flagSecret, "--journal-probe")
	var journaled bool
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if data, rerr := os.ReadFile(p); rerr == nil && bytes.Contains(data, []byte("journal-probe")) {
				journaled = true
			}
		}
		return nil
	})
	if !journaled {
		t.Fatal("no file recorded the clean invocation, so the journal was never written and this proves nothing")
	}
	assertNoFileCarries(t, home, nil, envSecret, flagSecret)
}

// TestMalformedConfigNeverPrintsItsCredential (class startup output): a
// config.toml that names the client secret and fails to parse. The TOML error
// echoes a key that is the secret pasted again; startup output never carries
// it, on the CLI or the MCP server.
func TestMalformedConfigNeverPrintsItsCredential(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "fJ2w" + "Lx9TqP4mVr6D"
	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "config.toml"),
		"client_id = \"partner-id-4417\"\nclient_secret = \""+secret+"\"\n"+secret+" = 1\n"+secret+" = 2\n")
	env := closedEnv(home)
	for _, args := range [][]string{{"doctor"}, {"auth", "status"}} {
		out := run(t, cliBin, env, args...)
		if strings.TrimSpace(out) == "" {
			t.Errorf("sophos-central-cli %s printed nothing, so this proves nothing", strings.Join(args, " "))
		}
		assertAbsent(t, "sophos-central-cli "+strings.Join(args, " "), out, secret)
	}
	assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", secret), secret)
}

// TestProfileValueHeldAtStartup (class startup credential): a run-profile
// value saved under a credential flag's name (an older or hand-edited
// profiles.json) is held before any output, on the CLI and the MCP server.
func TestProfileValueHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "scProf" + "Sec-5d4c3b2a19"
	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "profiles.json"),
		`{"profiles":{"watcher":{"name":"watcher","values":{"client-secret":"`+secret+`","limit":"5"}}}}`)
	env := closedEnv(home)
	assertAbsent(t, "profile show", run(t, cliBin, env, "profile", "show", "watcher"), secret)
	assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", secret, "version"), secret)
	assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", secret), secret)
}

// TestTamperPasswordNeverReachesAFile (class response field): an endpoint's
// tamper protection password (password, and each previousPasswords[].password)
// is a live credential Sophos returns. The connector keeps it out of the
// store, the response cache, export and tail
// (secret-responses-never-persisted), but --deliver file: wrote the answer,
// password included, to disk. The kit holds the declared response field as
// the response streams, so no file the program writes carries it.
func TestTamperPasswordNeverReachesAFile(t *testing.T) {
	cliBin, _ := binaries(t)
	const pw = "TpPass" + "Q7x2Lw9Kd3"
	const prev = "TpPrev" + "R5v8Mp1Jt6"
	const id = "59cf25c1-974a-4899-a84a-1883e68bd498"
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if mintOK(w, r, "scAccess"+"Tok7Kq2Wz8P4") {
			return
		}
		fmt.Fprintf(w, `{"enabled":true,"password":"%s","previousPasswords":[{"password":"%s","invalidatedAt":"2026-10-01T00:00:00.000Z"}]}`, pw, prev)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env("partner-"+"id-4417", "scClient"+"Sec9Hn3Vw6")...)
	sink := filepath.Join(home, "out", "tamper.json")
	run(t, cliBin, env, "endpoints", "get-tamper-protection", id, "--json", "--deliver", "file:"+sink)
	if !v.sentWith("GET", "/endpoint/v1/endpoints/"+id+"/tamper-protection") {
		t.Fatal("the tamper protection read never reached the stub, so nothing was answered")
	}
	assertNoFileCarries(t, home, nil, pw, prev)
}

// TestStdinRequestBodyAnswerIsPrinted (class missing data): on every command
// but feedback, --stdin reads a JSON request body, no credential. Declared a
// StdinFlags name, the kit held the whole body, so a write (allowed-items
// create) was sent and printed only the withheld notice, and a read (alerts
// search) lost its answer, because Sophos's answers repeat the body. The
// answer prints as it does for the same request made with flags. A body from
// `jq .` (one member per line) is read the same.
func TestStdinRequestBodyAnswerIsPrinted(t *testing.T) {
	cliBin, _ := binaries(t)
	v := startBodyVendor(t)
	home := t.TempDir()
	env := closedEnv(home, v.env("partner-"+"id-4417", "scClient"+"Sec2Lp7qX4")...)
	for _, c := range []struct {
		label, stdin string
		args         []string
		want, sent   []string
	}{
		{"allowed-items create --stdin (write)", `{"type":"path","comment":"Contoso line-of-business app","properties":{"path":"C:\\Apps\\contoso.exe"}}`,
			[]string{"allowed-items", "create", "--stdin", "--yes", "--json"},
			[]string{"Contoso line-of-business app", "10010"}, []string{"POST", "/endpoint/v1/settings/allowed-items", "Contoso line-of-business"}},
		{"allowed-items create --stdin, a jq-formatted body (write)", "{\n  \"type\": \"path\",\n  \"comment\": \"Fabrikam scanner driver\",\n  \"properties\": {\n    \"path\": \"C:\\\\Drivers\\\\fab.sys\"\n  }\n}\n",
			[]string{"allowed-items", "create", "--stdin", "--yes", "--json"},
			[]string{"Fabrikam scanner driver", "10010"}, []string{"POST", "/endpoint/v1/settings/allowed-items", "Fabrikam scanner"}},
		{"alerts search --stdin (read)", `{"category":["malware"],"severity":["high"]}`,
			[]string{"alerts", "search", "--stdin", "--json", "--no-cache"},
			[]string{"Malware detected on LT-0042", "a1b2c3d4"}, []string{"POST", "/common/v1/alerts/search", "malware"}},
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
		if !v.sentWith(c.sent...) {
			t.Errorf("%s: no request %q reached the stub", c.label, c.sent)
		}
	}
}

// startBodyVendor is an API stub for the --stdin request-body tests. A write
// is answered with its own body and an id, as Sophos answers a create, so a
// credential field in the body comes back in the answer; an alert search is
// answered with one alert.
func startBodyVendor(t *testing.T) *vendor {
	t.Helper()
	return startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if mintOK(w, r, "scAccess"+"Tok3Hn6Vq9Lm") {
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		b, _ := io.ReadAll(r.Body)
		if strings.HasSuffix(r.URL.Path, "/alerts/search") {
			io.WriteString(w, `{"items":[{"id":"a1b2c3d4-0000-4000-8000-000000000001","category":"malware","severity":"high","description":"Malware detected on LT-0042"}],"pages":{"current":1,"total":1}}`)
			return
		}
		answer := map[string]any{}
		_ = json.Unmarshal(b, &answer)
		answer["id"] = "10010"
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(answer)
	})
}

// TestStdinBodyCredentialFieldIsHeld (class body credential): a --stdin body
// is no credential source, but a credential field in it (here the archive
// password of a forensic log, and a nested client_secret) is held before
// anything prints (credkit.HoldBody), at any depth, so an answer that echoes
// it is withheld from stdout, stderr and every file.
func TestStdinBodyCredentialFieldIsHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const archive = "scArchive" + "Q7x2Lw9Kd3"
	const nested = "scBodyNest" + "R5v8Mp1Jt6"
	const id = "59cf25c1-974a-4899-a84a-1883e68bd498"
	v := startBodyVendor(t)
	home := t.TempDir()
	env := closedEnv(home, v.env("partner-"+"id-4417", "scClient"+"Sec8Wz4Pm1")...)
	out := runStdin(t, cliBin, env, `{"password":"`+archive+`"}`,
		"endpoints", "collect-forensic-log", id, "--stdin", "--yes", "--json")
	assertAbsent(t, "endpoints collect-forensic-log --stdin, password in the body", out, archive)
	out = runStdin(t, cliBin, env, `{"type":"path","comment":"Northwind","properties":{"path":"C:\\x.exe","client_secret":"`+nested+`"}}`,
		"allowed-items", "create", "--stdin", "--yes", "--json")
	assertAbsent(t, "allowed-items create --stdin, a nested client_secret", out, nested)
	if !v.sentWith("/forensic-logs", archive) || !v.sentWith("/allowed-items", "Northwind") {
		t.Errorf("the writes did not reach the stub, so nothing was echoed: %q", v.sent)
	}
	assertNoFileCarries(t, home, nil, archive, nested)
}

// TestStdinBodyDryRunPreview (class missing data, and body credential): a
// --dry-run preview of a --stdin body prints the body and sends nothing; a
// credential field in that body is withheld from the preview.
func TestStdinBodyDryRunPreview(t *testing.T) {
	cliBin, _ := binaries(t)
	const archive = "scDryPass" + "W4k9Zq2Xc7"
	const id = "59cf25c1-974a-4899-a84a-1883e68bd498"
	v := startBodyVendor(t)
	home := t.TempDir()
	env := closedEnv(home, v.env("partner-"+"id-4417", "scClient"+"Sec5Jt2Kx8")...)
	out := runStdin(t, cliBin, env, `{"type":"path","comment":"Northwind Depot","properties":{"path":"C:\\nw.exe"}}`,
		"allowed-items", "create", "--stdin", "--dry-run")
	if strings.Contains(out, "withheld") || !strings.Contains(out, "Northwind Depot") {
		t.Errorf("allowed-items create --stdin --dry-run: the preview lacks the body or was withheld:\n%s", out)
	}
	out = runStdin(t, cliBin, env, `{"password":"`+archive+`"}`, "endpoints", "collect-forensic-log", id, "--stdin", "--dry-run")
	assertAbsent(t, "endpoints collect-forensic-log --stdin --dry-run, password in the body", out, archive)
	if n := v.hits.Load(); n != 0 {
		t.Errorf("--dry-run sent %d request(s)", n)
	}
	assertNoFileCarries(t, home, nil, archive)
}

// TestFreeTextStdinIsKept (class missing data): `feedback --stdin` reads free
// text. Held whole (as StdinFlags did, and as HoldBody holds bytes that are no
// document), the body was withheld from the command's own record of it. It is
// not held; a held credential pasted into it is still withheld.
func TestFreeTextStdinIsKept(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "scFeedbk" + "Sec3Lp7qZ2"
	const text = "the endpoints list output was confusing"
	home := t.TempDir()
	env := closedEnv(home, "SOPHOS_CENTRAL_CLIENT_ID=partner-"+"id-4417", "SOPHOS_CENTRAL_CLIENT_SECRET="+secret)
	runStdin(t, cliBin, env, text+"\nOK\n", "feedback", "--stdin")
	runStdin(t, cliBin, env, "pasted "+secret+" by mistake\n", "feedback", "--stdin")
	record, err := os.ReadFile(filepath.Join(home, "data", "feedback.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(record), text) {
		t.Errorf("feedback.jsonl lost the feedback:\n%s", record)
	}
	assertNoFileCarries(t, home, nil, secret)
}

// TestMintRunKeepsItsOutput (class missing data): a run that mints a token
// posts client_id=..&client_secret=..&grant_type=client_credentials. The kit
// also reads a form body as one key=value line, so a credential named first
// held everything after it, "grant_type=client_credentials" included, and any
// output with a piece of "credentials" in it was withheld, the kit's own
// notice too ("it carried a credential"): doctor, run on environment
// credentials, printed nothing and exited 74. The token form now starts with
// grant_type (cliutil.TokenForm), so a minting run prints what it says.
func TestMintRunKeepsItsOutput(t *testing.T) {
	cliBin, _ := binaries(t)
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if mintOK(w, r, "scAccess"+"Tok4Rb8Nc2Xq") {
			return
		}
		io.WriteString(w, `{"id":"48bebbb0-f7b6-49c5-aec0-315858f73457","idType":"tenant","apiHosts":{"global":"https://api.central.sophos.com","dataRegion":"https://api-us01.central.sophos.com"}}`)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cliBin, "doctor")
	cmd.Env = closedEnv(t.TempDir(), v.env("partner-"+"id-4417", "scClient"+"Sec6Td1Ym4")...)
	out, _ := cmd.CombinedOutput()
	if !v.sentWith("POST", tokenPath, "grant_type=client_credentials") {
		t.Fatalf("doctor never minted, so this proves nothing:\n%s", out)
	}
	if code := cmd.ProcessState.ExitCode(); code != 0 {
		t.Errorf("doctor exited %d on a run that minted:\n%s", code, out)
	}
	if !strings.Contains(string(out), "Credentials: valid") {
		t.Errorf("doctor lost its credentials line on a run that minted:\n%s", out)
	}
}

// TestRegionalAPIHostStaysAllowed (class origin): the bearer goes to the
// global API and to the regional apiHost GET /whoami/v1 returns
// (https://api-<region>.central.sophos.com), and the client secret goes to
// id.sophos.com; Origins name those hosts as fixed data. Every request goes
// through an HTTPS proxy to one TLS stub whose certificate names the Sophos
// hosts, so nothing leaves this machine. Origins without the central.sophos.com
// wildcard refused the regional call, so no tenant command could read its
// region.
func TestRegionalAPIHostStaysAllowed(t *testing.T) {
	cliBin, _ := binaries(t)
	const bearer = "scAccess" + "Tok2Gm5Rv9Lc"
	const tenant = "48bebbb0-f7b6-49c5-aec0-315858f73457"
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "sophos-central leak test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		DNSNames: []string{"api.central.sophos.com", "api-us01.central.sophos.com", "id.sophos.com"},
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.Host+r.URL.Path+" auth="+strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Host == "id.sophos.com" && r.URL.Path == tokenPath:
			fmt.Fprintf(w, `{"access_token":"%s","token_type":"bearer","expires_in":3600}`, bearer)
		case r.Host == "api.central.sophos.com" && r.URL.Path == "/whoami/v1":
			fmt.Fprintf(w, `{"id":"%s","idType":"tenant","apiHosts":{"global":"https://api.central.sophos.com","dataRegion":"https://api-us01.central.sophos.com"}}`, tenant)
		case r.Host == "api-us01.central.sophos.com" && r.URL.Path == "/endpoint/v1/endpoints":
			io.WriteString(w, `{"items":[{"id":"59cf25c1-974a-4899-a84a-1883e68bd498","hostname":"LT-0042","type":"computer"}],"pages":{"current":1,"total":1}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error":"notFound"}`)
		}
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		up, err := net.Dial("tcp", srv.Listener.Addr().String())
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		c, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			up.Close()
			return
		}
		io.WriteString(c, "HTTP/1.1 200 Connection Established\r\n\r\n")
		go func() { _, _ = io.Copy(up, c); up.Close() }()
		_, _ = io.Copy(c, up)
		c.Close()
	}))
	t.Cleanup(proxy.Close)
	home := t.TempDir()
	ca := filepath.Join(home, "ca.pem")
	writeFile(t, ca, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})))
	var env []string
	for _, e := range closedEnv(home) {
		if !strings.HasPrefix(e, "SOPHOS_CENTRAL_BASE_URL=") && !strings.HasPrefix(e, "SOPHOS_CENTRAL_TOKEN_URL=") {
			env = append(env, e) // the defaults: the global API and id.sophos.com
		}
	}
	env = append(env, "HTTPS_PROXY="+proxy.URL, "SOPHOS_CENTRAL_CA_CERT="+ca,
		"SOPHOS_CENTRAL_CLIENT_ID=partner-"+"id-4417", "SOPHOS_CENTRAL_CLIENT_SECRET="+"scClient"+"Sec7Hv2Nw5")
	out := run(t, cliBin, env, "endpoints", "list", "--json", "--no-cache")
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(out, "LT-0042") || strings.Contains(out, "withheld") {
		t.Errorf("endpoints list did not print the regional answer:\n%s\nrequests: %q", out, seen)
	}
	for _, want := range []string{"POST id.sophos.com" + tokenPath + " auth=", "GET api.central.sophos.com/whoami/v1 auth=" + bearer,
		"GET api-us01.central.sophos.com/endpoint/v1/endpoints auth=" + bearer} {
		if !slices.Contains(seen, want) {
			t.Errorf("no request %q reached the stub: %q", want, seen)
		}
	}
}
