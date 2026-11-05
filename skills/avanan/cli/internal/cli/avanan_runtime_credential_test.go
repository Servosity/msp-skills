// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): regression guards for handfixes.json entries
// "credkit-wiring" (class secret-echo).

package cli

import (
	"avanan-pp-cli/internal/credkit"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"avanan-pp-cli/internal/cliutil"
	"avanan-pp-cli/internal/cliutil/testenv"
)

// credentialForms is every encoding a credential is looked for in output:
// raw, base64 (both alphabets, padded and not), URL-escaped (query and path)
// and JSON-escaped (as json.Marshal writes it and with "/" and "+" escaped).
func credentialForms(v string) []string {
	j, _ := json.Marshal(v)
	seen := map[string]bool{}
	var out []string
	for _, form := range []string{
		v,
		base64.StdEncoding.EncodeToString([]byte(v)),
		base64.RawStdEncoding.EncodeToString([]byte(v)),
		base64.URLEncoding.EncodeToString([]byte(v)),
		base64.RawURLEncoding.EncodeToString([]byte(v)),
		url.QueryEscape(v),
		url.PathEscape(v),
		strings.Trim(string(j), `"`),
		strings.NewReplacer("/", `\/`, "+", `\u002b`).Replace(v),
	} {
		if !seen[form] {
			seen[form] = true
			out = append(out, form)
		}
	}
	return out
}

// assertNoCredentialForm fails when text carries cred in any credentialForms
// encoding.
func assertNoCredentialForm(t *testing.T, where, text, cred string) {
	t.Helper()
	for _, form := range credentialForms(cred) {
		if strings.Contains(text, form) {
			t.Errorf("%s carries the credential as %q:\n%s", where, form, text)
		}
	}
}

// isolateAvananEnv sandboxes every user directory the CLI resolves (HOME,
// AVANAN_HOME and the XDG dirs, through testenv.Isolate) and clears every
// AVANAN_* variable the operator's shell might carry, so the test sees only
// the credentials it sets.
func isolateAvananEnv(t *testing.T) string {
	t.Helper()
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "AVANAN_") || strings.HasPrefix(name, "PRINTING_PRESS_") {
			t.Setenv(name, "")
		}
	}
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("AVANAN_HOME", home)
	t.Setenv("AVANAN_CONFIG", filepath.Join(home, "config.toml"))
	return home
}

// heldNow reports whether the kit withholds v from a file write.
func heldNow(t *testing.T, v string) bool {
	t.Helper()
	err := credkit.WriteFile(filepath.Join(t.TempDir(), "probe"), []byte("x "+v+" x"), 0o600)
	if err != nil && !errors.Is(err, credkit.ErrWithheld) {
		t.Fatalf("probe write: %v", err)
	}
	return errors.Is(err, credkit.ErrWithheld)
}

// serverCAFile writes srv's certificate as a PEM file for AVANAN_CA_CERT: the
// kit sends no credential over plain http, loopback included.
func serverCAFile(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub-ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// executeBuilt runs the built CLI bin (credkit.Start and every sink, as shipped)
// with the test's environment. Build bin with buildCommand before
// isolateAvananEnv: go must keep the real module cache.
func executeBuilt(bin string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	return out.String(), errb.String(), err
}

// executeCLI runs Execute in this process with os.Args set to args, so a
// test can ask what it held. It runs no credkit.Start.
func executeCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	dir := t.TempDir()
	outFile, ferr := os.Create(filepath.Join(dir, "stdout"))
	if ferr != nil {
		t.Fatal(ferr)
	}
	t.Cleanup(func() { _ = outFile.Close() })
	errFile, ferr := os.Create(filepath.Join(dir, "stderr"))
	if ferr != nil {
		t.Fatal(ferr)
	}
	t.Cleanup(func() { _ = errFile.Close() })
	origArgs, origOut, origErr := os.Args, os.Stdout, os.Stderr
	os.Args = append([]string{"avanan-cli"}, args...)
	os.Stdout, os.Stderr = outFile, errFile
	func() {
		defer func() { os.Args, os.Stdout, os.Stderr = origArgs, origOut, origErr }()
		err = Execute()
	}()
	read := func(f *os.File) string {
		if _, serr := f.Seek(0, io.SeekStart); serr != nil {
			t.Fatal(serr)
		}
		b, rerr := io.ReadAll(f)
		if rerr != nil {
			t.Fatal(rerr)
		}
		_ = f.Close()
		return string(b)
	}
	return read(outFile), read(errFile), err
}

// htmlEchoServer is the Codex round-1 input: the handshake mints token, then
// every data request answers HTTP 200, Content-Type text/html, with the token
// as the page title.
func htmlEchoServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.0/auth" {
			_, _ = w.Write([]byte(token))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><title>" + token + "</title></html>"))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("AVANAN_CA_CERT", serverCAFile(t, srv))
	return srv
}

// TestMintedTokenNeverReachesCLIOutput is the Codex round-1 P1, end to end:
// with only AVANAN_APP_ID and AVANAN_CLIENT_SECRET configured, `scopes`
// mints a session token, and the API then answers 200 text/html with that
// token as the page title. maskErrorBody rewrites non-2xx bodies only and the
// client's masker knows only Config's credentials, so the minted token was
// printed in the CLI error.
func TestMintedTokenNeverReachesCLIOutput(t *testing.T) {
	bin := buildCommand(t, "../../cmd/avanan-cli")
	for _, tc := range []struct {
		name  string
		token string
		extra []string
	}{
		{"human", "minted-session-XYZ/0123456789+abc", nil},
		{"json", "minted-session-JSN/0123456789+abc", []string{"--json"}},
		{"agent", "minted-session-AGT/0123456789+abc", []string{"--agent"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateAvananEnv(t)
			srv := htmlEchoServer(t, tc.token)
			t.Setenv("AVANAN_BASE_URL", srv.URL)
			t.Setenv("AVANAN_APP_ID", "cli-runtime-app-0001")
			t.Setenv("AVANAN_CLIENT_SECRET", "cli-runtime-secret-0001")

			args := append([]string{"scopes", "--data-source", "live", "--no-cache"}, tc.extra...)
			stdout, stderr, err := executeBuilt(bin, args...)
			if err == nil {
				t.Fatalf("scopes succeeded on an HTML answer\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
			}
			assertNoCredentialForm(t, "stdout", stdout, tc.token)
			assertNoCredentialForm(t, "stderr", stderr, tc.token)
			assertNoCredentialForm(t, "the returned error", err.Error(), tc.token)
		})
	}
}

// TestAuthLoginHoldsTheMintedToken drives the Mint path (`auth login`) end to
// end. Its output is an allowlist with no token field, so the minted token
// must reach no output; and Mint must register it with the process-wide
// funnel the moment it exists, so any later text in this process that repeats
// it is withheld.
func TestAuthLoginHoldsTheMintedToken(t *testing.T) {
	const token = "minted-login-QRS/9876543210+xyz"
	bin := buildCommand(t, "../../cmd/avanan-cli")
	isolateAvananEnv(t)
	srv := htmlEchoServer(t, token)
	t.Setenv("AVANAN_BASE_URL", srv.URL)
	stdout, stderr, err := executeBuilt(bin, "auth", "login", "--base-url", srv.URL,
		"--app-id", "cli-login-app-0002", "--secret", "cli-login-secret-0002", "--json")
	if err != nil {
		t.Fatalf("auth login: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	assertNoCredentialForm(t, "auth login stdout", stdout, token)
	assertNoCredentialForm(t, "auth login stderr", stderr, token)
}

// TestCredentialArgumentsAreHeldBeforeAnyError: a credential handed to a
// hand-written command as an argument or flag is held from the moment the
// command has it, not only once a client is built from it. `auth login`
// refuses an unknown region before any client exists, and `auth set-token`
// never builds a client at all.
func TestCredentialArgumentsAreHeldBeforeAnyError(t *testing.T) {
	if !heldChild(t) {
		return
	}
	isolateAvananEnv(t)
	const (
		loginApp    = "held-login-flag-app-HHH-0004"
		loginSecret = "held-login-flag-secret-HHH-0004"
		setTokenApp = "held-set-token-app-III-0005"
	)
	_, _, err := executeCLI(t, "auth", "login", "--region", "no-such-region",
		"--app-id", loginApp, "--secret", loginSecret, "--json")
	if err == nil {
		t.Fatal("auth login accepted an unknown region")
	}
	_, _, err = executeCLI(t, "auth", "set-token", setTokenApp)
	if err != nil {
		t.Fatalf("auth set-token: %v", err)
	}
	for _, v := range []string{loginApp, loginSecret, setTokenApp} {
		if !heldNow(t, v) {
			t.Errorf("credential argument %q is not held", v)
		}
	}
}
