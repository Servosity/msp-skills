// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the credential
// classes the pre-kit review left open, each driven from its failing input
// against the built binaries, so the shared credential kit (and the
// CrowdStrike origin binding it does not replace) is what closes them.

package crowdstrikeblackbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Fixture values. Each is written so that no 8-character piece of it occurs in
// anything the programs print on their own (the kit withholds any output that
// shares one), and with few distinct characters, so a secret scanner reads it
// as the fixture it is.
const (
	kcPairID     = "kcpairidvaluemmmmnnnn"   // FALCON_CLIENT_ID
	kcPairValue  = "kcpairvaluepppprrrrssss" // FALCON_CLIENT_SECRET
	kcDupFirst   = "kcdupfirstvvvvwwwwyyyy"  // the first of two client_secret members
	kcDupSecond  = "kcdupsecondggggkkkkqqqq" // the second, also the unknown tool's name
	kcFileValue  = "kcfilevaluehhhhjjjjxxxx" // a client_secret in a config file that does not parse
	kcStoredUS1  = "kcstoredusonebbbbccccdd" // a token stored for one cloud
	kcHTTPBearer = "kchttpbearervalueeeeefffffggggghhhhh"
)

// runRaw runs bin with env and stdin, and returns stdout, stderr and the exit
// code.
func runRaw(t *testing.T, bin string, env []string, dir, stdin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env, cmd.Dir = env, dir
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s %v: %v", bin, args, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-time.After(120 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("%s %v: timed out", bin, args)
	}
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("%s %v: %v", bin, args, err)
	}
	return out.String(), errb.String(), code
}

// isolated is a clean environment under a fresh home, with no credential.
func isolated(t *testing.T, extra ...string) (string, []string) {
	t.Helper()
	home := t.TempDir()
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, "USERPROFILE=" + home, "CROWDSTRIKE_HOME=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"CROWDSTRIKE_CLI_PATH=" + bbCLI,
	}
	return home, append(env, extra...)
}

// TestKitMCPHTTPWithoutATokenSaysWhy: a token-less --transport http launch
// exits 1 and says which variable it needs. Before the kit, the connector's
// own stderr filter was a pipe that os.Exit cut before it delivered, so the
// launch exited 1 with nothing on stderr.
func TestKitMCPHTTPWithoutATokenSaysWhy(t *testing.T) {
	t.Parallel()
	home, env := isolated(t)
	out, errOut, code := runRaw(t, bbMCP, env, home, "", "--transport", "http", "--addr", "127.0.0.1:0")
	if code != 1 {
		t.Fatalf("exit %d, want 1\nstdout:\n%s\nstderr:\n%s", code, out, errOut)
	}
	if !strings.Contains(errOut, "CROWDSTRIKE_MCP_HTTP_TOKEN") {
		t.Fatalf("stderr does not name CROWDSTRIKE_MCP_HTTP_TOKEN:\n%q", errOut)
	}
}

// TestKitDoctorJSONIsAVerdict: doctor --json is a JSON document whose
// credential verdict follows the server (a healthy token, a rejected one).
// Before the kit, the connector's own filter withheld the whole document, so
// every case printed the same notice.
func TestKitDoctorJSONIsAVerdict(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	home, env := isolated(t, "CROWDSTRIKE_BASE_URL="+sv.url, "CROWDSTRIKE_CA_CERT="+sv.caFile,
		"FALCON_CLIENT_ID="+bbClientID, "FALCON_CLIENT_SECRET="+bbClientSecret, "CROWDSTRIKE_NO_CONFIG_WRITE=1")
	out, errOut, _ := runRaw(t, bbCLI, env, home, "", "doctor", "--json")
	var report map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &report); err != nil {
		t.Fatalf("doctor --json is not a JSON document (%v):\nstdout:\n%s\nstderr:\n%s", err, out, errOut)
	}
	cred, _ := report["credentials"].(string)
	if cred == "" || strings.Contains(strings.ToLower(cred), "withheld") {
		t.Fatalf("doctor --json carries no credential verdict: %q\n%s", cred, out)
	}
	assertNoEcho(t, "doctor --json", out+"\n"+errOut)
}

// mcpLines runs one MCP stdio session: the handshake, then each raw line.
func mcpLines(t *testing.T, env []string, dir string, lines ...string) string {
	t.Helper()
	var in bytes.Buffer
	in.WriteString(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}` + "\n")
	in.WriteString(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")
	for _, l := range lines {
		in.WriteString(l + "\n")
	}
	out, errOut, _ := runRaw(t, bbMCP, env, dir, in.String())
	return out + "\n" + errOut
}

// TestKitMCPIncomingHeldBeforeDispatch: a tools/call names an unknown tool
// with the second of two client_secret members, which mcp-go repeats in its
// "not found" error before any tool handler runs; an unknown method carries
// one too. Every member of a duplicate key is held before the server reads
// the message (classes holder-incomplete-before-dispatch,
// holder-holds-every-duplicate-member, mcp-incoming-held-before-dispatch).
func TestKitMCPIncomingHeldBeforeDispatch(t *testing.T) {
	t.Parallel()
	home, env := isolated(t)
	out := mcpLines(t, env, home,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"`+kcDupSecond+`","arguments":{"client_secret":"`+kcDupFirst+`","client_secret":"`+kcDupSecond+`"}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"`+kcDupFirst+`","arguments":{"client_secret":"`+kcDupSecond+`","client_secret":"`+kcDupFirst+`"}}}`,
	)
	if !strings.Contains(out, `"id":7`) || !strings.Contains(out, `"id":8`) {
		t.Fatalf("the server answered neither call; the test did not run:\n%.3000s", out)
	}
	for _, v := range []string{kcDupFirst, kcDupSecond} {
		if strings.Contains(out, v) || strings.Contains(out, v[:12]) {
			t.Errorf("MCP output repeats a client_secret member %q:\n%.3000s", v, out)
		}
	}
}

// TestKitMalformedConfigHeldBeforeStartupOutput: a config file whose
// client_secret line does not parse gives up that value before anything
// prints, in the CLI and the MCP server (startup ordering).
func TestKitMalformedConfigHeldBeforeStartupOutput(t *testing.T) {
	t.Parallel()
	home, env := isolated(t, "CROWDSTRIKE_BASE_URL=https://127.0.0.1:9")
	cfg := filepath.Join(home, "config", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
		t.Fatal(err)
	}
	broken := "base_url = \"https://127.0.0.1:9\"\nclient_id = \"" + kcPairID + "\"\nclient_secret = \"" + kcFileValue + "\nclient_secret = " + kcFileValue + " trailing\n"
	if err := os.WriteFile(cfg, []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	// The value typed back as an unknown command is the failing input: Cobra
	// repeats it in its error, so only a hold taken from the malformed file
	// at startup keeps it out of the output.
	for _, args := range [][]string{{"doctor"}, {"doctor", "--json"}, {"auth", "status"}, {"devices", "query-by-filter", "--json"}, {kcFileValue}} {
		o, e, _ := runRaw(t, bbCLI, env, home, "", args...)
		all.WriteString(strings.ReplaceAll(strings.Join(args, " "), kcFileValue, "<the file value>") + ":\n" + o + e + "\n")
	}
	all.WriteString("mcp:\n" + mcpLines(t, env, home, `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`))
	text := all.String()
	if !strings.Contains(text, "config") {
		t.Fatalf("nothing mentions the config; the test did not run:\n%.3000s", text)
	}
	if strings.Contains(text, kcFileValue) || strings.Contains(text, kcFileValue[:10]) {
		t.Fatalf("startup output repeats the malformed config's client_secret:\n%.4000s", text)
	}
}

// TestKitJournalHoldsNoCredential: the learn journal never records a held
// credential, whichever field would carry it (here the session key, set to
// the client secret).
func TestKitJournalHoldsNoCredential(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	home, env := isolated(t, "CROWDSTRIKE_BASE_URL="+sv.url, "CROWDSTRIKE_CA_CERT="+sv.caFile,
		"FALCON_CLIENT_ID="+kcPairID, "FALCON_CLIENT_SECRET="+kcPairValue, "CROWDSTRIKE_NO_CONFIG_WRITE=1",
		"CROWDSTRIKE_LEARN_SESSION="+kcPairValue)
	for _, args := range [][]string{{"devices", "query-by-filter", "--json"}, {"no-such-command-here"}, {"doctor"}} {
		_, _, _ = runRaw(t, bbCLI, env, home, "", args...)
	}
	var journals int
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.Contains(filepath.ToSlash(p), "/learn/") {
			return nil
		}
		journals++
		if b, rerr := os.ReadFile(p); rerr == nil && (bytes.Contains(b, []byte(kcPairValue)) || bytes.Contains(b, []byte(kcPairValue[:10]))) {
			t.Errorf("%s holds the client secret:\n%.1500s", p, b)
		}
		return nil
	})
	if journals == 0 {
		t.Fatalf("no learn journal file was written; the test did not run")
	}
}

// TestKitStoredTokenStaysWithItsCloud: a token stored for one Falcon cloud is
// never sent to another one the environment names
// (stored-credential-bound-to-its-origin; the kit's own origin rule allows
// the configured cloud, so this binding is the connector's).
func TestKitStoredTokenStaysWithItsCloud(t *testing.T) {
	t.Parallel()
	first := echoServer(t)
	second := echoServer(t)
	home, env := isolated(t, "CROWDSTRIKE_CA_CERT="+first.caFile)
	store := filepath.Join(home, "data", "credentials.toml")
	if err := os.MkdirAll(filepath.Dir(store), 0o700); err != nil {
		t.Fatal(err)
	}
	stored := "base_url = " + strconv.Quote(first.url) + "\naccess_token = " + strconv.Quote(kcStoredUS1) + "\n"
	if err := os.WriteFile(store, []byte(stored), 0o600); err != nil {
		t.Fatal(err)
	}
	// The store is read: on its own cloud the token is sent.
	env1 := append(append([]string{}, env...), "CROWDSTRIKE_BASE_URL="+first.url)
	if o, e, _ := runRaw(t, bbCLI, env1, home, "", "devices", "query-by-filter", "--json"); func() bool {
		first.mu.Lock()
		defer first.mu.Unlock()
		return first.headers["Bearer "+kcStoredUS1] == 0
	}() {
		t.Fatalf("the stored token was not sent to its own cloud; the test did not run:\n%s\n%s", o, e)
	}
	env2 := append(append([]string{}, env...), "CROWDSTRIKE_BASE_URL="+second.url)
	// Both fakes trust one CA file each; point the second run at the second's.
	for i, kv := range env2 {
		if strings.HasPrefix(kv, "CROWDSTRIKE_CA_CERT=") {
			env2[i] = "CROWDSTRIKE_CA_CERT=" + second.caFile
		}
	}
	out, errOut, _ := runRaw(t, bbCLI, env2, home, "", "devices", "query-by-filter", "--json")
	second.mu.Lock()
	sent := second.headers["Bearer "+kcStoredUS1]
	second.mu.Unlock()
	if sent > 0 {
		t.Fatalf("the token stored for %s was sent to %s %d times:\n%s\n%s", first.url, second.url, sent, out, errOut)
	}
	if strings.Contains(out+errOut, kcStoredUS1) {
		t.Fatalf("output repeats the stored token:\n%s\n%s", out, errOut)
	}
}

// TestKitMCPHTTPTokenNeverPrinted: the MCP HTTP bearer is held from startup;
// a caller that sends it in a tool's arguments gets nothing of it back.
func TestKitMCPHTTPTokenNeverPrinted(t *testing.T) {
	t.Parallel()
	home, env := isolated(t, "CROWDSTRIKE_MCP_HTTP_TOKEN="+kcHTTPBearer)
	out := mcpLines(t, env, home,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"`+kcHTTPBearer+`","arguments":{}}}`)
	if !strings.Contains(out, `"id":4`) {
		t.Fatalf("no reply; the test did not run:\n%.2000s", out)
	}
	if strings.Contains(out, kcHTTPBearer[:12]) {
		t.Fatalf("MCP output repeats the HTTP bearer:\n%.2000s", out)
	}
}

var _ = http.StatusOK
