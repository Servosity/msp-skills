// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class's failing input, seen
// failing on the binaries built before the kit was wired. Set
// VEEAM_LEAK_BIN_DIR to run them against binaries built elsewhere.

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

	"veeam-pp-cli/internal/credkit"
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
		if d := os.Getenv("VEEAM_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "veeam-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{exe("veeam-cli"), "../../cmd/veeam-cli"},
			{exe("veeam-mcp"), "../../cmd/veeam-mcp"},
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
	return filepath.Join(binDir, exe("veeam-cli")), filepath.Join(binDir, exe("veeam-mcp"))
}

// vendor is a TLS stub of the Veeam Service Provider Console API: the kit
// never sends a credential over plain http, loopback included.
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
	return []string{"VEEAM_BASE_URL=" + v.url, "VEEAM_CA_CERT=" + v.caFile, "VEEAM_TOKEN=" + token}
}

// closedEnv is a closed environment under one sandbox home; every press
// directory resolves under it.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "VEEAM_HOME=" + home,
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

// TestAPIErrorEchoOfTokenIsWithheld (class token-echo, the retired
// secret-echo-decoded-funnel's job): an API error body that echoes
// VEEAM_TOKEN whole, a piece of it, base64 or URL-encoded. Any 8-character
// piece of a held value, after every decoding the contract names, withholds
// the output.
func TestAPIErrorEchoOfTokenIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const token = "vspc-" + "Zq81LmP0xV7cR2"
	piece := token[:12]
	b64 := base64.StdEncoding.EncodeToString([]byte(token))
	esc := url.QueryEscape("k=" + token)
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"errors":[{"message":"token %s... rejected","debug":"%s","echo":"%s"}]}`, piece, b64, esc)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env(token)...)
	assertAbsent(t, "users get-current", run(t, cliBin, env, "users", "get-current"), piece, b64, token)
	assertAbsent(t, "users get-current --agent", run(t, cliBin, env, "users", "get-current", "--agent"), piece, b64, token)
	assertAbsent(t, "MCP veeam_read users.get-current",
		mcpCall(t, mcpBin, env, `{"name":"veeam_read","arguments":{"endpoint_id":"users.get-current"}}`), piece, b64, token)
	if v.hits.Load() == 0 {
		t.Error("no request reached the API stub, so nothing was echoed")
	}
	assertNoFileCarries(t, home, nil, piece, b64)
}

// TestResponseCredentialFieldsWithheld (class response-secret): the API hands
// back third-party secrets in response bodies: a backup server's stored Linux
// credentials (privateKey, passphrase), a freshly minted RSA key pair, a TOTP
// secret. Declarations.ResponseFields holds each value as it arrives, so it
// reaches no sink: not stdout, not the MCP result, not the response cache.
func TestResponseCredentialFieldsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const token = "vspc-" + "R3spF1dK9wQ2"
	const privKey = "pk" + "Qz7Lm2Wx9Rt4Vb"
	const passphrase = "pp" + "J8nK3qT6yH1sDf"
	const rsaKey = "rk" + "Mv5Gh2Xc8Pw1Ls"
	const totp = "tp" + "W4fN7bZ2kQ9eRy"
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/credentials"):
			fmt.Fprintf(w, `{"data":[{"instanceUid":"c1","username":"root","linuxCredentialsDetails":{"privateKey":"%s","passphrase":"%s"}}],"meta":{"pagingInfo":{"total":1,"count":1,"offset":0}}}`, privKey, passphrase)
		case strings.HasSuffix(r.URL.Path, "/keys/rsa"):
			fmt.Fprintf(w, `{"data":{"privateKey":"%s","publicKey":"public-half"}}`, rsaKey)
		case strings.HasSuffix(r.URL.Path, "/keys/totp-secret"):
			fmt.Fprintf(w, `{"data":{"secret":"%s","secretUrl":"otpauth://totp/x?secret=%s"}}`, totp, totp)
		default:
			io.WriteString(w, `{"data":[]}`)
		}
	})
	home := t.TempDir()
	env := closedEnv(home, v.env(token)...)
	assertAbsent(t, "get-backup-server-credentials-by-server",
		run(t, cliBin, env, "infrastructure", "get-backup-server-credentials-by-server", "bs1", "--json"), privKey, passphrase)
	assertAbsent(t, "generate-new-rsa-key-pair", run(t, cliBin, env, "authentication", "generate-new-rsa-key-pair", "--json"), rsaKey)
	assertAbsent(t, "generate-totp-secret", run(t, cliBin, env, "authentication", "generate-totp-secret", "--json"), totp)
	assertAbsent(t, "MCP veeam_read credentials",
		mcpCall(t, mcpBin, env, `{"name":"veeam_read","arguments":{"endpoint_id":"infrastructure.get-backup-server-credentials-by-server","params":{"backupServerUid":"bs1"}}}`),
		privKey, passphrase)
	if v.hits.Load() < 4 {
		t.Errorf("only %d request(s) reached the API stub", v.hits.Load())
	}
	assertNoFileCarries(t, home, nil, privKey, passphrase, rsaKey, totp)
}

// TestSyncNeverStoresACredentialField (class response-secret, store side):
// syncing public-cloud guest OS credentials returns each one's password. The
// value is held as it arrives, so the SQLite store never carries it, and the
// withheld row is reported, not dropped in silence.
func TestSyncNeverStoresACredentialField(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "vspc-" + "Sy9cTk4Lp2Wx"
	const guestPassword = "gp" + "H6rT1vB8mZ3qXn"
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"data":[{"guestOsCredentialsUid":"g1","role":"serviceProviderAdministrator","username":"administrator","description":"","password":"%s","organizationUid":"o1","siteUid":"s1"}],"meta":{"pagingInfo":{"total":1,"count":1,"offset":0}}}`, guestPassword)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env(token)...)
	out := run(t, cliBin, env, "sync", "--resources", "infrastructure-sites-public-cloud-guest-os-credentials", "--full")
	assertAbsent(t, "sync", out, guestPassword)
	if v.hits.Load() == 0 {
		t.Fatal("no request reached the API stub, so nothing was synced")
	}
	if !strings.Contains(out, "withheld") {
		t.Errorf("sync did not report the withheld row:\n%s", out)
	}
	assertNoFileCarries(t, home, nil, guestPassword)
}

// TestMCPExecutePreviewWithholdsPassword (class preview-echo): an unconfirmed
// veeam_execute returns a request preview, and that preview carried the
// encryption password verbatim. The MCP argument is held before the handler
// runs (Declarations.MCPArguments), so the preview is withheld, and nothing is
// sent without confirm.
func TestMCPExecutePreviewWithholdsPassword(t *testing.T) {
	_, mcpBin := binaries(t)
	const token = "vspc-" + "Prev1ewT0k9x"
	const password = "ep" + "s3cr3tW9q2Lm7p"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	home := t.TempDir()
	env := closedEnv(home, v.env(token)...)
	out := mcpCall(t, mcpBin, env, `{"name":"veeam_execute","arguments":{"endpoint_id":"infrastructure.create-backup-server-encryption-password","params":{"backupServerUid":"bs1","password":"`+password+`","hint":"h"}}}`)
	assertAbsent(t, "MCP veeam_execute preview", out, password)
	if v.hits.Load() != 0 {
		t.Errorf("an unconfirmed veeam_execute reached the API (%d request(s))", v.hits.Load())
	}
	assertNoFileCarries(t, home, nil, password)
}

// TestCLICredentialFlagsHeld (class preview-echo, CLI side): --password,
// --private-key, --passphrase and --api-key are credential flags; each value is
// held before the flag parser runs, so the dry-run preview of the request body
// never prints it.
func TestCLICredentialFlagsHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const password = "ep" + "fl4gS3cr3tQ8zV"
	const privKey = "pk" + "fl4gS3cr3tQ8zV"
	const passphrase = "pp" + "fl4gS3cr3tQ8zV"
	const apiKey = "ak" + "fl4gS3cr3tQ8zV"
	env := closedEnv(t.TempDir(), "VEEAM_TOKEN=vspc-"+"Fl4gT0kZ9q1w")
	assertAbsent(t, "create-backup-server-encryption-password --dry-run",
		run(t, cliBin, env, "infrastructure", "create-backup-server-encryption-password", "bs1", "--hint", "h", "--password", password, "--dry-run"), password)
	assertAbsent(t, "create-backup-server-linux-credentials --dry-run",
		run(t, cliBin, env, "infrastructure", "create-backup-server-linux-credentials", "bs1", "--username", "u",
			"--private-key", privKey, "--passphrase", passphrase, "--password", password, "--dry-run"), privKey, passphrase, password)
	assertAbsent(t, "assign-api-key --dry-run",
		run(t, cliBin, env, "plugins", "apikey", "assign-api-key", "p1", "--api-key", apiKey, "--dry-run"), apiKey)

	// A JSON-valued flag is no credential flag; its credential fields are held
	// one by one (credwire.HoldJSONArguments), in both flag spellings.
	const inner = "jp" + "Qx8Lm3Vt6Rw2Zk"
	creds := `[{"userName":"u","password":"` + inner + `"}]`
	assertAbsent(t, "install-linux-backup-agent-on-computer --credentials --dry-run",
		run(t, cliBin, env, "discovery", "install-linux-backup-agent-on-computer", "cu1", "--credentials", creds, "--dry-run"), inner)
	assertAbsent(t, "install-linux-backup-agent-on-computer --credentials= --dry-run",
		run(t, cliBin, env, "discovery", "install-linux-backup-agent-on-computer", "cu1", "--credentials="+creds, "--dry-run"), inner)
}

// TestMalformedConfigCredentialHeldAtStartup (class startup-ordering): a
// config.toml that does not parse still has every credential in it held before
// the startup diagnostic that names the parse error can print.
func TestMalformedConfigCredentialHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const token = "vspc-" + "Bad70ml9xQ2w"
	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "config.toml"), "access_token = \""+token+"\"\nbase_url = [unclosed\n")
	env := closedEnv(home)
	assertAbsent(t, "CLI users get-current", run(t, cliBin, env, "users", "get-current"), token)
	assertAbsent(t, "CLI doctor", run(t, cliBin, env, "doctor"), token)
	assertAbsent(t, "MCP veeam_read", mcpCall(t, mcpBin, env, `{"name":"veeam_read","arguments":{"endpoint_id":"users.get-current"}}`), token)

	// The config's own key is `token`. An unparsed file holds each of its words,
	// and the bare word collides with the kit's public text, so the start is
	// refused (exit 78) instead of naming the parse error. Either way the token
	// is never shown.
	writeFile(t, filepath.Join(configDir(home), "config.toml"), "token = \""+token+"\"\nbase_url = [unclosed\n")
	out, _ := runCode(t, cliBin, env, "users", "get-current")
	assertAbsent(t, "CLI users get-current (token key)", out, token)
}

// TestProfileCredentialValueHeldAtStartup (class profiles.json): a run-profile
// value saved under a credential flag's name is a credential source, held at
// startup.
func TestProfileCredentialValueHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "vbrProf" + "Fld-5d4c3b2a19"
	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "profiles.json"),
		`{"profiles":{"watcher":{"name":"watcher","values":{"password":"`+secret+`","limit":"50"}}}}`)
	env := closedEnv(home)
	assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", secret, "version"), secret)
	assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", secret), secret)
	assertAbsent(t, "profile show", run(t, cliBin, env, "profile", "show", "watcher"), secret)
}

// TestHeldCredentialRefusedOffOriginAndOverPlainHTTP (class origin): the token
// goes only to the API it is configured for, and never over plain http, not
// even to loopback. A --deliver webhook whose URL carries the token is refused
// before it dials.
func TestHeldCredentialRefusedOffOriginAndOverPlainHTTP(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "vspc-" + "Orig1nQ7r3s5"
	api := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"data":{"instanceUid":"u1","userName":"acme"}}`)
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
	run(t, cliBin, env, "users", "get-current", "--json", "--deliver", "webhook:"+other.url+"/hook/"+token)
	if n := other.conns.Load(); n != 0 {
		t.Errorf("webhook to another https origin: %d connection(s) opened for a URL that carries the token", n)
	}
	run(t, cliBin, env, "users", "get-current", "--json", "--deliver", "webhook:"+plain.URL+"/hook/"+token)
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
		io.WriteString(w, `{"data":{}}`)
	}))
	t.Cleanup(plainAPI.Close)
	env = closedEnv(t.TempDir(), "VEEAM_BASE_URL="+plainAPI.URL, "VEEAM_TOKEN="+token)
	assertAbsent(t, "plain http API", run(t, cliBin, env, "users", "get-current"), token)
	if n := bearerHits.Load(); n != 0 {
		t.Errorf("plain http API: %d request(s) carried the token", n)
	}
}

// TestSinkTokensHeld (class sink-token): SPACEFAST_TOKEN, the credential of
// the --deliver spacefast: sink, and VEEAM_MCP_HTTP_TOKEN, the MCP HTTP bearer,
// are held from startup in both binaries.
func TestSinkTokensHeld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const spacefast = "sft_" + "live_4b7c9d1e2f3a"
	env := closedEnv(t.TempDir(), "SPACEFAST_TOKEN="+spacefast)
	assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", spacefast, "version"), spacefast)
	assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", spacefast), spacefast)
	assertAbsent(t, "CLI deliver target error", run(t, cliBin, env, "version", "--deliver", "spacefast:"+spacefast+"/x"), spacefast)

	const bearer = "mcpb-" + "Ht7pK2wQ9zLx4v"
	env = closedEnv(t.TempDir(), "VEEAM_MCP_HTTP_TOKEN="+bearer)
	assertAbsent(t, "CLI startup flag error (MCP HTTP token)", run(t, cliBin, env, "--timeout", bearer, "version"), bearer)
	assertAbsent(t, "MCP startup flag error (MCP HTTP token)", run(t, mcpBin, env, "--transport", bearer), bearer)
}

// TestJournalNeverRecordsAHeldCredential (class journal): the learn journal
// records a mistyped flag's name. A token pasted where a flag name goes
// (--<token>) would be written to state/learn/journal-*.jsonl; the journal
// appends through the kit (credkit.AppendFile), which withholds an entry
// carrying a held value.
func TestJournalNeverRecordsAHeldCredential(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "vspctok" + "JournL8x2q9"
	home := t.TempDir()
	env := closedEnv(home, "VEEAM_TOKEN="+token)
	run(t, cliBin, env, "users", "get-current", "--"+token)
	run(t, cliBin, env, "users", "get-current", "--x-request-idd", token)
	journals, _ := filepath.Glob(filepath.Join(home, "state", "learn", "journal-*.jsonl"))
	if len(journals) == 0 {
		t.Fatal("no journal segment was written, so nothing was checked")
	}
	assertNoFileCarries(t, home, nil, token)
}

// TestFeedbackStdinIsRecorded (class missing data): `feedback --stdin` keeps
// its body (feedback text is no credential), while a held token pasted into it
// is still withheld.
func TestFeedbackStdinIsRecorded(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "vspctokFeedbk3Lp7q"
	const text = "the users output was confusing"
	home := t.TempDir()
	env := closedEnv(home, "VEEAM_TOKEN="+token)
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
