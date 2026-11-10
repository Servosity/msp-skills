// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): regression guards for handfixes.json entry
// "credkit-wiring" (class secret-echo).

package cli

import (
	"avanan-pp-cli/internal/credkit"
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"avanan-pp-cli/internal/cliutil"
)

// credentialSource is one place a credential enters the program, and the
// reflecting server that hands it back in an HTTP 200 answer.
type credentialSource struct {
	name   string
	cred   string            // the value that must reach no output, cache or store
	env    map[string]string // credential env vars
	config string            // config.toml body
	pre    [][]string        // CLI runs before the checked one
	over   map[string]string // env for the checked run only (overrides a stored value)
	tokens []string          // session tokens the handshake mints, in order
	reject bool              // answer the first data request 401 (forces a re-mint)
	echo   func(*http.Request) string
	// setup lays out files under the sandbox home before any run and returns
	// env entries for every run (a legacy config, a malformed file).
	setup func(t *testing.T, home string) []string
}

// fixedEcho answers every data request with v: a value the server learned
// earlier, which this run does not send.
func fixedEcho(v string) func(*http.Request) string {
	return func(*http.Request) string { return v }
}

// writeSandboxFile writes body at path (0600), creating its directory.
func writeSandboxFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// mixedCaseHex is hex whose letters alternate lower and upper case.
func mixedCaseHex(s string) string {
	b := []byte(hex.EncodeToString([]byte(s)))
	upper := false
	for i, c := range b {
		if c >= 'a' && c <= 'f' {
			if upper {
				b[i] = c - 'a' + 'A'
			}
			upper = !upper
		}
	}
	return string(b)
}

func headerEcho(name string) func(*http.Request) string {
	return func(r *http.Request) string { return r.Header.Get(name) }
}

// credentialSources is every source of a credential the CLI and the MCP
// server hold: the config file (credential fields and [headers]), the
// environment, the credentials store (also when an env var replaces the
// stored value), a minted session token, a token
// re-minted after a 401, and a credential too short for the shared funnel's
// decoded search. (Credential flags exist only on `auth login`; see
// TestCredentialFlagsAreHeldAtParse.)
func credentialSources() []credentialSource {
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	return []credentialSource{
		{
			name:   "config-file",
			cred:   "cfgfile-app-id-7Q2",
			config: "app_id = \"cfgfile-app-id-7Q2\"\nclient_secret = \"cfgfile-secret-7Q2\"\n",
			tokens: []string{"cfgfile-session-7Q2"},
			echo:   headerEcho("x-av-app-id"),
		},
		{
			name:   "config-headers",
			cred:   "hdr-cfg-value-81",
			env:    map[string]string{"AVANAN_APP_ID": "hdr-app-id-81", "AVANAN_CLIENT_SECRET": "hdr-secret-81"},
			config: "[headers]\nX-Reflect-Me = \"hdr-cfg-value-81\"\n",
			tokens: []string{"hdr-session-81"},
			echo:   headerEcho("X-Reflect-Me"),
		},
		{
			name:   "env",
			cred:   "env-app-id-55K",
			env:    map[string]string{"AVANAN_APP_ID": "env-app-id-55K", "AVANAN_CLIENT_SECRET": "env-secret-55K"},
			tokens: []string{"env-session-55K"},
			echo:   headerEcho("x-av-app-id"),
		},
		{
			name:   "credentials-store",
			cred:   "store-app-id-77",
			env:    map[string]string{"AVANAN_CLIENT_SECRET": "store-secret-77"},
			pre:    [][]string{{"auth", "set-token", "store-app-id-77"}},
			tokens: []string{"store-session-77"},
			echo:   headerEcho("x-av-app-id"),
		},
		{
			name:   "credentials-file-overridden-by-env",
			cred:   "stored-app-id-OV1",
			env:    map[string]string{"AVANAN_CLIENT_SECRET": "stored-secret-OV1"},
			pre:    [][]string{{"auth", "set-token", "stored-app-id-OV1"}},
			over:   map[string]string{"AVANAN_APP_ID": "replacement-app-id-OV1"},
			tokens: []string{"stored-session-OV1"},
			echo:   func(*http.Request) string { return "stored-app-id-OV1" },
		},
		{
			name:   "minted-token",
			cred:   "minted-session-M4/0123+abc",
			env:    map[string]string{"AVANAN_APP_ID": "mint-app-id-M4", "AVANAN_CLIENT_SECRET": "mint-secret-M4"},
			tokens: []string{"minted-session-M4/0123+abc"},
			echo:   headerEcho("x-av-token"),
		},
		{
			name:   "refreshed-token",
			cred:   "refreshed-session-R9",
			env:    map[string]string{"AVANAN_APP_ID": "refresh-app-id-R9", "AVANAN_CLIENT_SECRET": "refresh-secret-R9"},
			config: "token = \"stale-session-R9\"\ntoken_expiry = " + future + "\n",
			tokens: []string{"refreshed-session-R9"},
			reject: true,
			echo:   headerEcho("x-av-token"),
		},
		{
			// No AVANAN_CONFIG: the default path is read, and a malformed
			// legacy config is skipped with a warning while env credentials
			// run the command.
			name:   "malformed-legacy-config",
			cred:   "legacy-app-id-LG1",
			env:    map[string]string{"AVANAN_APP_ID": "env-app-id-LG1", "AVANAN_CLIENT_SECRET": "env-secret-LG1"},
			tokens: []string{"legacy-session-LG1"},
			echo:   fixedEcho("legacy-app-id-LG1"),
			setup: func(t *testing.T, home string) []string {
				writeSandboxFile(t, filepath.Join(home, ".config", "avanan-cli", "config.toml"), "app_id = \"legacy-app-id-LG1\"\nclient_secret = \"legacy-secret-LG1\"\nbroken = \n")
				return []string{"AVANAN_CONFIG="}
			},
		},
		{
			name:   "malformed-credentials-file",
			cred:   "zz9-cred-app-MC1",
			env:    map[string]string{"AVANAN_APP_ID": "env-app-id-MC1", "AVANAN_CLIENT_SECRET": "env-secret-MC1"},
			tokens: []string{"zz9-session-MC1"},
			echo:   fixedEcho("zz9-cred-app-MC1"),
			setup: func(t *testing.T, home string) []string {
				dir, err := cliutil.DataDir()
				if err != nil {
					t.Fatal(err)
				}
				writeSandboxFile(t, filepath.Join(dir, "credentials.toml"), "app_id = \"zz9-cred-app-MC1\"\ntoken = \"zz9-cred-tok-MC1\"\nbroken = \n")
				return []string{"AVANAN_CONFIG="}
			},
		},
		{
			name:   "header-basic-password",
			cred:   "hdr-pass-HC1",
			env:    map[string]string{"AVANAN_APP_ID": "hdr-app-id-HC1", "AVANAN_CLIENT_SECRET": "hdr-secret-HC1"},
			config: "[headers]\nX-Upstream-Auth = \"Basic " + base64.StdEncoding.EncodeToString([]byte("hdr-user-HC1:hdr-pass-HC1")) + "\"\n",
			tokens: []string{"hdr-session-HC1"},
			echo:   fixedEcho("hdr-pass-HC1"),
		},
		{
			name:   "header-cookie-component",
			cred:   "cookie-sid-HC2",
			env:    map[string]string{"AVANAN_APP_ID": "cookie-app-id-HC2", "AVANAN_CLIENT_SECRET": "cookie-secret-HC2"},
			config: "[headers]\nCookie = \"theme=dark; sid=cookie-sid-HC2\"\n",
			tokens: []string{"cookie-session-HC2"},
			echo:   fixedEcho("cookie-sid-HC2"),
		},
		{
			name:   "echo-in-mixed-case-hex",
			cred:   "hexecho-app-id-HX1",
			env:    map[string]string{"AVANAN_APP_ID": "hexecho-app-id-HX1", "AVANAN_CLIENT_SECRET": "hexecho-secret-HX1"},
			tokens: []string{"hexecho-session-HX1"},
			echo:   func(r *http.Request) string { return mixedCaseHex(r.Header.Get("x-av-app-id")) },
		},
		{
			name:   "short-credential",
			cred:   "q7z",
			env:    map[string]string{"AVANAN_APP_ID": "q7z", "AVANAN_CLIENT_SECRET": "short-secret-Q7"},
			tokens: []string{"short-session-Q7"},
			echo:   headerEcho("x-av-app-id"),
		},
	}
}

// reflectingServer mints src.tokens on the legacy handshake and answers every
// data request HTTP 200 with src.echo(request) inside an Avanan envelope. It
// reports how many data requests it answered.
func reflectingServer(t *testing.T, src credentialSource) (*httptest.Server, func() int) {
	t.Helper()
	var (
		mu       sync.Mutex
		minted   int
		rejected bool
		hits     int
	)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/v1.0/auth" {
			tok := src.tokens[min(minted, len(src.tokens)-1)]
			minted++
			_, _ = w.Write([]byte(tok))
			return
		}
		if src.reject && !rejected {
			rejected = true
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"responseEnvelope":{"responseCode":401,"responseText":"expired"}}`))
			return
		}
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"responseEnvelope":{"requestId":"r1","responseCode":200,"responseText":"OK","recordsNumber":1},"responseData":[{"id":"t1","name":%q}]}`, "us:"+src.echo(r))
	}))
	t.Cleanup(srv.Close)
	return srv, func() int { mu.Lock(); defer mu.Unlock(); return hits }
}

// buildCommand builds one of this module's commands (a package path relative
// to this test's directory) into a temp dir, so the test runs the shipped main
// (its output funnel included).
func buildCommand(t *testing.T, pkg string) string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		goBin = filepath.Join(runtime.GOROOT(), "bin", "go")
	}
	out := filepath.Join(t.TempDir(), filepath.Base(pkg))
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	if b, err := exec.Command(goBin, "build", "-o", out, pkg).CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, b)
	}
	return out
}

// sourceEnv sandboxes the child's directories and sets src's env (every other
// AVANAN_* variable is cleared by isolateAvananEnv).
func sourceEnv(t *testing.T, src credentialSource, baseURL *httptest.Server) (env []string, home string) {
	t.Helper()
	home = isolateAvananEnv(t)
	if src.config != "" {
		if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(src.config), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	env = append(os.Environ(), "AVANAN_BASE_URL="+baseURL.URL, "AVANAN_CA_CERT="+serverCAFile(t, baseURL))
	for k, v := range src.env {
		env = append(env, k+"="+v)
	}
	return env, home
}

// assertStoredNowhere fails when any file under home other than the config
// and credentials files (where credentials belong) carries cred: the response
// cache, the local store, the journal, learn events.
func assertStoredNowhere(t *testing.T, home, cred string) {
	t.Helper()
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == "config.toml" || d.Name() == "credentials.toml" {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr == nil {
			if !synccpuStoreFile(path) { // synccpu: the local store is not checked
				assertNoCredentialForm(t, "file "+path, string(b), cred)
			}
		}
		return nil
	})
}

// TestEveryCredentialSourceIsWithheldFromCLIOutput drives each credential
// source end to end through the built CLI (a live read, and a sync into the
// local store): the server reflects the credential in a 200 answer, and no
// form of it may reach stdout, stderr, the cache or the store, while the
// withheld notice proves the answer was seen.
func TestEveryCredentialSourceIsWithheldFromCLIOutput(t *testing.T) {
	bin := buildCommand(t, "../../cmd/avanan-cli")
	for _, src := range credentialSources() {
		t.Run(src.name, func(t *testing.T) {
			srv, hits := reflectingServer(t, src)
			env, home := sourceEnv(t, src, srv)
			if src.setup != nil {
				env = append(env, src.setup(t, home)...)
			}
			run := func(args ...string) string {
				cmd := exec.Command(bin, args...)
				cmd.Env = env
				var out bytes.Buffer
				cmd.Stdout, cmd.Stderr = &out, &out
				_ = cmd.Run()
				return out.String()
			}
			for _, pre := range src.pre {
				assertNoCredentialForm(t, strings.Join(pre, " "), run(pre...), src.cred)
			}
			for k, v := range src.over {
				env = append(env, k+"="+v)
			}
			out := run("scopes", "--data-source", "live", "--json") + run("sync", "--resources", "msp-tenants", "--json")
			if hits() < 2 {
				t.Fatalf("the server answered no data request; the source was not exercised:\n%s", out)
			}
			assertNoCredentialForm(t, "CLI output", out, src.cred)
			if strings.Contains(strings.ToLower(out), hex.EncodeToString([]byte(src.cred))) {
				t.Errorf("CLI output carries the credential in hex:\n%s", out)
			}
			if !strings.Contains(out, "[credkit: withheld") {
				t.Errorf("the reflected answer was not withheld:\n%s", out)
			}
			assertStoredNowhere(t, home, src.cred)
		})
	}
}

// TestCredentialFlagsAreHeldAtParse: `auth login`'s --app-id and --secret are
// held the moment Cobra parses them, so even a run that fails on a later flag
// before the command body runs holds them, short ones included.
func TestCredentialFlagsAreHeldAtParse(t *testing.T) {
	if !heldChild(t) {
		return
	}
	isolateAvananEnv(t)
	const app, secret = "flag-app-id-Fzz1", "flag-secret-F1"
	stdout, stderr, err := executeCLI(t, "auth", "login", "--app-id", app, "--secret", secret, "--no-such-flag")
	if err == nil {
		t.Fatal("auth login accepted an unknown flag")
	}
	for _, v := range []string{app, secret} {
		assertNoCredentialForm(t, "stdout", stdout, v)
		assertNoCredentialForm(t, "stderr", stderr, v)
		if !heldNow(t, v) {
			t.Errorf("flag value %q was not held at parse", v)
		}
	}
}

// TestDeliveryWithholdsHeldValues: the --deliver copy is written outside
// stdout, so it passes the kit itself.
func TestDeliveryWithholdsHeldValues(t *testing.T) {
	if !heldChild(t) {
		return
	}
	const held = "delivered-held-Dzz8"
	credkit.Hold("config:unclassified", held)
	path := filepath.Join(t.TempDir(), "out.json")
	if err := Deliver(DeliverSink{Scheme: "file", Target: path}, []byte(`{"name":"`+held+`"}`), false); err == nil {
		t.Error("a delivered copy carrying a held value was written")
	}
	if b, err := os.ReadFile(path); err == nil {
		assertNoCredentialForm(t, "delivered file", string(b), held)
	}
}

// TestOutboundRequestsNeverCarryAHeldValueToAnotherHost: the built CLI's
// --deliver webhook carries the answer to a host the credential is not allowed
// to reach, so a body repeating a held credential never arrives there.
func TestOutboundRequestsNeverCarryAHeldValueToAnotherHost(t *testing.T) {
	bin := buildCommand(t, "../../cmd/avanan-cli")
	const held = "outbound-app-id-OB1"
	src := credentialSource{
		name: "webhook", cred: held,
		env:    map[string]string{"AVANAN_APP_ID": held, "AVANAN_CLIENT_SECRET": held + "-cs"},
		tokens: []string{"outbound-session-OB1"}, echo: headerEcho("x-av-app-id"),
	}
	srv, hits := reflectingServer(t, src)
	env, _ := sourceEnv(t, src, srv)
	var (
		mu  sync.Mutex
		got []string
	)
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, r.URL.String()+" "+fmt.Sprint(r.Header)+" "+string(b))
		mu.Unlock()
	}))
	t.Cleanup(other.Close)
	cmd := exec.Command(bin, "scopes", "--data-source", "live", "--json", "--deliver", "webhook:"+other.URL+"/hook")
	cmd.Env = env
	out, _ := cmd.CombinedOutput()
	if hits() == 0 {
		t.Fatalf("the API answered nothing:\n%s", out)
	}
	assertNoCredentialForm(t, "CLI output", string(out), held)
	mu.Lock()
	defer mu.Unlock()
	for _, g := range got {
		assertNoCredentialForm(t, "a request another host received", g, held)
	}
}
