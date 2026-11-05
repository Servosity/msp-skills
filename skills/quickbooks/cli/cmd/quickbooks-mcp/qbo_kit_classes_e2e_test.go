// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written. Not generated. Recorded hand-fix `credkit-wiring` in
// handfixes.json. The classes the last review stopped on, driven through the
// real program: a credential inside a URL anywhere on the command line is held
// before any flag is parsed, and every URL inside any text is read on its own.

package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"quickbooks-pp-cli/internal/cliutil"
	"quickbooks-pp-cli/internal/cliutil/testenv"
	"quickbooks-pp-cli/internal/credkit"
)

// The other direction: a link with no credential is no credential. It saves,
// and it and every profile beside it show unwithheld.
func TestQBOPlainLinkIsNotWithheld(t *testing.T) {
	testenv.Isolate(t)
	env := []string{"QUICKBOOKS_BASE_URL=" + qboE2EBaseURL}
	const link = "See https://developer.intuit.com/docs and //intuit.example.invalid/x?page=2"
	out := qboRun(t, env, "", "profile", "save", "defaults", "--json", "--no-cache", "--description", link)
	if strings.Contains(out, "withheld") || strings.Contains(out, "credential-like") {
		t.Fatalf("profile save refused a plain link: %s", out)
	}
	for _, args := range [][]string{{"profile", "show", "defaults"}, {"profile", "use", "defaults"}, {"profile", "list", "--json"}} {
		if out := qboRun(t, env, "", args...); strings.Contains(out, "withheld") || !strings.Contains(out, "defaults") {
			t.Errorf("%v: a profile with a plain link was withheld: %s", args, out)
		}
	}
	if out := qboRun(t, env, "", "profile", "show", "defaults"); !strings.Contains(out, "https://developer.intuit.com/docs") {
		t.Errorf("the plain link did not print: %s", out)
	}
}

// report --raw prints the API's bytes as they came, and still through the kit:
// an answer that repeats the held token is withheld with the kit's notice.
func TestQBORawReportThatRepeatsTheTokenIsWithheld(t *testing.T) {
	lkIsolate(t)
	const token = "lk-raw-report-token-0001"
	ts := newLKTokenServer(t, func(string) (int, string) { return 500, "{}" })
	api := lkServe(t, ts, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Header":{"ReportName":"CashFlow"},"Rows":{"Row":[{"type":"Data","ColData":[{"value":"`+token+`"},{"value":"1"}]}]}}`)
	})
	env := []string{"QUICKBOOKS_BASE_URL=" + api.URL + "/v3/company/42", "QUICKBOOKS_ACCESS_TOKEN=" + token, "QUICKBOOKS_NO_LEARN=true", lkCAEnv(t, api)}
	out, _ := lkRun(t, env, "report", "cash-flow", "--month", "2025-01", "--raw", "--json", "--no-cache")
	if qboLeaks(out, token) || !strings.Contains(out, "credkit: withheld") {
		t.Errorf("--raw did not withhold the answer: %s", out)
	}
}

// A URL-valued variable is held inside credkit.Start, before its startup
// refusal can print: the refusal below quotes the name of the first variable,
// which is the second one's credential.
func TestQBOEnvURLCredentialIsHeldBeforeTheStartupRefusal(t *testing.T) {
	testenv.Isolate(t)
	const secret = "QUICKBOOKS_ACCESS_TOKEN"
	run := func(mode string, env ...string) string {
		cmd := exec.Command(os.Args[0], "--version")
		cmd.Env = append(append(os.Environ(), "QBO_E2E_MAIN="+mode, "QUICKBOOKS_BASE_URL="+qboE2EBaseURL), env...)
		out, _ := cmd.CombinedOutput()
		return string(out)
	}
	for _, mode := range []string{"cli", "mcp"} {
		// Without the URL the same refusal does name it, so the run below proves something.
		if out := run(mode, secret+"=true"); !strings.Contains(out, secret) {
			t.Fatalf("%s: the refusal this test relies on did not print: %s", mode, out)
		}
		for _, env := range []string{
			"QUICKBOOKS_TOKEN_URL=https://example.test/?token=" + secret,
			"QUICKBOOKS_FEEDBACK_ENDPOINT=https://example.test/?token=" + secret,
			"HTTPS_PROXY=https://ops:" + secret + "@example.test",
		} {
			out := run(mode, secret+"=true", env)
			if strings.Contains(out, secret) {
				t.Errorf("%s: %s: the refusal printed the credential inside the URL: %s", mode, env, out)
			}
		}
	}
}

// A query value in a URL variable is not a credential unless its name says so:
// holding every value, as the kit's URLEnv does, makes ?v=2 refuse every command
// and ?channel=ops refuse the POST the operator configured.
func TestQBOAPlainQueryInAURLVariableStaysUsable(t *testing.T) {
	testenv.Isolate(t)
	run := func(mode string, env []string, args ...string) (string, int) {
		cmd := exec.Command(os.Args[0], args...)
		cmd.Env = append(append(os.Environ(), "QBO_E2E_MAIN="+mode, "QUICKBOOKS_BASE_URL="+qboE2EBaseURL, "QUICKBOOKS_NO_LEARN=true"), env...)
		out, _ := cmd.CombinedOutput()
		return string(out), cmd.ProcessState.ExitCode()
	}
	for _, mode := range []string{"cli", "mcp"} {
		for _, name := range []string{"QUICKBOOKS_TOKEN_URL", "QUICKBOOKS_FEEDBACK_ENDPOINT"} {
			// The MCP server has no --version and exits 2 on it: past Start, as the CLI is.
			out, code := run(mode, []string{name + "=https://example.test/hook?v=2"}, "--version")
			if code == credkit.ExitRefused || out == "" {
				t.Errorf("%s: %s with ?v=2 stopped the program (exit %d): %q", mode, name, code, out)
			}
		}
	}
	out, _ := run("cli", []string{"QUICKBOOKS_FEEDBACK_ENDPOINT=https://example.test/hook?channel=ops"}, "feedback", "--send", "a note")
	if strings.Contains(out, qboGuardRefused) {
		t.Errorf("the guard refused the configured feedback endpoint for a plain query value: %s", out)
	}
}

// A credential field in an MCP call is held before the tool answers: a write's
// preview repeats the call's parameters back.
func TestQBOMCPCredentialArgumentIsHeldOnThePreview(t *testing.T) {
	testenv.Isolate(t)
	env := []string{"QUICKBOOKS_BASE_URL=" + qboE2EBaseURL, "QUICKBOOKS_NO_LEARN=true"}
	for i, name := range []string{"client_secret", "client_id", "access_token", "refresh_token", "auth_header", "id_token", "token", "password", "api_key", "authorization"} {
		secret := "McpArgument" + strconv.Itoa(1000+i)
		out := qboMCP(t, env, "quickbooks_execute", map[string]any{
			"endpoint_id": "customers.create",
			"params":      map[string]any{"DisplayName": "Acme", name: secret},
		})
		if qboLeaks(out, secret) {
			t.Errorf("the preview repeated %s: %s", name, out)
		}
	}
	// With nothing to hold, the same preview still answers.
	out := qboMCP(t, env, "quickbooks_execute", map[string]any{
		"endpoint_id": "customers.create",
		"params":      map[string]any{"DisplayName": "Acme"},
	})
	if !strings.Contains(out, "Acme") {
		t.Errorf("a clean preview lost its body: %s", out)
	}
}

// qboReceiver is a TLS origin that records every body posted to it.
type qboReceiver struct {
	mu     sync.Mutex
	bodies []string
	srv    *httptest.Server
}

func newQBOReceiver(t *testing.T) *qboReceiver {
	t.Helper()
	r := &qboReceiver{}
	r.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.bodies = append(r.bodies, req.Header.Get("Authorization")+"\n"+string(b))
		r.mu.Unlock()
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *qboReceiver) got(secret string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Contains(strings.Join(r.bodies, "\n"), secret)
}

// qboGuardRefused is the outbound guard's refusal. It comes before any dial, so
// a stub no system trust store knows still tells a refusal from a pass: a
// webhook has no CA setting of its own, and macOS ignores SSL_CERT_FILE.
const qboGuardRefused = "withheld from outbound request"

// Only the config the program uses binds where a credential may go. A file it
// never reads, or reads and distrusts, says nothing about that.
func TestQBOOnlyTheConfigInUseBindsTheOrigin(t *testing.T) {
	const token = "AccessHold9876"
	setup := func(t *testing.T) (dir string, api *lkAPI, recv *qboReceiver, env []string) {
		t.Helper()
		lkIsolate(t)
		ts := newLKTokenServer(t, func(string) (int, string) { return http.StatusBadRequest, `{"error":"invalid_grant"}` })
		api = newLKAPI(t, ts, func(tok string) bool { return tok == token })
		recv = newQBOReceiver(t)
		dir = t.TempDir()
		env = []string{"QUICKBOOKS_BASE_URL=", "QUICKBOOKS_NO_LEARN=true", lkCAEnv(t, api.srv)}
		return dir, api, recv, env
	}
	base := func(api *lkAPI) string { return `base_url = "` + api.srv.URL + `/v3/company/4620816365"` + "\n" }
	tenant := func(t *testing.T, dir string, api *lkAPI) string {
		t.Helper()
		p := filepath.Join(dir, token+".toml")
		qboWriteFile(t, p, base(api)+`access_token = "`+token+`"`+"\n")
		return p
	}
	unused := func(t *testing.T, recv *qboReceiver) {
		t.Helper()
		cfgDir, err := cliutil.ConfigDir()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(cfgDir, 0o700); err != nil {
			t.Fatal(err)
		}
		qboWriteFile(t, filepath.Join(cfgDir, "config.toml"), `base_url = "`+recv.srv.URL+`"`+"\n")
	}
	deliver := func(recv *qboReceiver) string { return "--deliver=webhook:" + recv.srv.URL + "/collect" }

	t.Run("an unused default config binds nothing", func(t *testing.T) {
		dir, api, recv, env := setup(t)
		unused(t, recv)
		cfg := tenant(t, dir, api)
		out, _ := lkRun(t, env, "--config", cfg, "auth", "status", "--json", deliver(recv))
		if recv.got(token) || !strings.Contains(out, qboGuardRefused) {
			t.Errorf("the guard let the token go to the origin an unused config names: %s", out)
		}
		// The other direction: the config in use still reaches its own origin.
		out, code := lkRun(t, env, append([]string{"--config", cfg}, lkReportArgs...)...)
		if code != 0 || strings.Contains(out, "withheld") {
			t.Errorf("the config in use could not reach its own origin (exit %d): %s", code, out)
		}
	})
	t.Run("a config in use binds its own origin", func(t *testing.T) {
		dir, api, recv, env := setup(t)
		qboWriteFile(t, filepath.Join(dir, "plain.toml"), base(api))
		cfg := tenant(t, dir, api)
		out, code := lkRun(t, env, append([]string{"--config", cfg}, lkReportArgs...)...)
		if code != 0 {
			t.Errorf("exit %d: %s", code, out)
		}
		_ = recv
	})
	// As Cobra does, the last --config wins, for the program and the guard alike.
	t.Run("an overridden config flag binds nothing", func(t *testing.T) {
		dir, api, recv, env := setup(t)
		earlier := filepath.Join(dir, "earlier.toml")
		qboWriteFile(t, earlier, `base_url = "`+recv.srv.URL+`"`+"\n")
		later := filepath.Join(dir, token+".toml")
		qboWriteFile(t, later, `access_token = "`+token+`"`+"\n")
		out, _ := lkRun(t, env, "--config", earlier, "--config", later, "auth", "status", "--json", deliver(recv))
		if recv.got(token) || !strings.Contains(out, qboGuardRefused) {
			t.Errorf("the guard let the token go to the origin an overridden --config names: %s", out)
		}
		cfg := tenant(t, dir, api)
		if out, code := lkRun(t, env, append([]string{"--config", earlier, "--config", cfg}, lkReportArgs...)...); code != 0 {
			t.Errorf("the last --config could not reach its own origin (exit %d): %s", code, out)
		}
	})
	// The review's input: the environment token shows up in agent-context's
	// directory paths, so the delivery body carries it.
	home := func(t *testing.T, dir string) []string {
		t.Helper()
		h := filepath.Join(dir, token)
		if err := os.MkdirAll(h, 0o700); err != nil {
			t.Fatal(err)
		}
		return []string{"QUICKBOOKS_HOME=" + h, "QUICKBOOKS_ACCESS_TOKEN=" + token}
	}
	t.Run("a config the program distrusts binds nothing", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("file modes do not make a config untrusted on Windows")
		}
		dir, _, recv, env := setup(t)
		loose := filepath.Join(dir, "loose.toml")
		if err := os.WriteFile(loose, []byte(`base_url = "`+recv.srv.URL+`"`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(loose, 0o644); err != nil {
			t.Fatal(err)
		}
		out, _ := lkRun(t, append(env, home(t, dir)...), "--config", loose, "agent-context", deliver(recv))
		if recv.got(token) || !strings.Contains(out, qboGuardRefused) {
			t.Errorf("the guard let the token go to the origin a distrusted config names: %s", out)
		}
	})
	// A store records the server its credential was saved for, and the program
	// does not route by it. It stays with the kit, which must not bind it,
	// in any spelling the kit reads.
	for name, key := range map[string]string{"plain": "base_url", "long s": "\"ba\u017fe_url\""} {
		t.Run("a store's server binds nothing, "+name, func(t *testing.T) {
			dir, _, recv, env := setup(t)
			env = append(env, home(t, dir)...)
			t.Setenv("QUICKBOOKS_HOME", filepath.Join(dir, token))
			store, err := cliutil.CredentialsFilePath()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(store), 0o700); err != nil {
				t.Fatal(err)
			}
			qboWriteFile(t, store, key+` = "`+recv.srv.URL+`"`+"\n")
			out, _ := lkRun(t, env, "agent-context", deliver(recv))
			if recv.got(token) || !strings.Contains(out, qboGuardRefused) {
				t.Errorf("the guard let the token go to the origin a store names: %s", out)
			}
		})
	}
}

// A --stdin body is held before anything prints, on the preview and on the send,
// and the send still reaches the vendor with it whole.
func TestQBOStdinBodyIsHeldOnThePreviewAndTheSend(t *testing.T) {
	lkIsolate(t)
	const secret = "BodyFieldValue9zQ"
	var mu sync.Mutex
	var got string
	ts := newLKTokenServer(t, func(string) (int, string) { return http.StatusBadRequest, `{"error":"invalid_grant"}` })
	srv := lkServe(t, ts, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = string(b)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Customer":{"Id":"58","DisplayName":"Acme"}}`)
	})
	env := []string{"QUICKBOOKS_BASE_URL=" + srv.URL + "/v3/company/1", "QUICKBOOKS_ACCESS_TOKEN=StdinToken9876", "QUICKBOOKS_NO_LEARN=true", lkCAEnv(t, srv)}
	body := `{"DisplayName":"Acme","client_secret":"` + secret + `"}`
	args := []string{"customers", "create", "--stdin", "--json", "--yes"}
	if out := qboRun(t, env, body, append(args, "--dry-run")...); qboLeaks(out, secret) {
		t.Errorf("the preview printed the body: %s", out)
	}
	out := qboRun(t, env, body, args...)
	if qboLeaks(out, secret) {
		t.Errorf("the send printed the body: %s", out)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(got, secret) {
		t.Errorf("the vendor did not get the body, so this test checked nothing (%q): %s", got, out)
	}
	// The same flag on feedback reads a note the kit holds whole, so it cannot be
	// recorded. It says so rather than claim it was.
	note := qboRun(t, env, "a note from the operator", "feedback", "--stdin")
	if strings.Contains(note, "recorded locally") || !strings.Contains(note, "pass the note as an argument") {
		t.Errorf("feedback --stdin claimed a note it cannot record: %s", note)
	}
	if out := qboRun(t, env, "", "feedback", "a note from the operator"); !strings.Contains(out, "recorded locally") {
		t.Errorf("a note given as an argument was not recorded: %s", out)
	}
}

// A saved profile can carry a credential, a URL or free text in any field, from
// any release. All of it is held from the start of the run; what a profile saves
// that is the program's own setting is not, so a clean profile still shows.
func TestQBOProfileStoreIsHeldAtStartup(t *testing.T) {
	testenv.Isolate(t)
	env := []string{"QUICKBOOKS_BASE_URL=" + qboE2EBaseURL}
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	profiles := filepath.Join(dir, "profiles.json")
	// One store per case: a shared 8-character piece would withhold them all,
	// and a case could then pass for another case's hold.
	for _, tc := range []struct{ name, secret, store string }{
		{"credential flag", "Rf5ProfileVal", `{"profiles":{"p":{"name":"p","values":{"refresh-token":"Rf5ProfileVal"}}}}`},
		{"short credential value", "77", `{"profiles":{"p":{"name":"p","values":{"Client_Secret":"77"}}}}`},
		{"a duplicate in another profile", "Dp8ProfileVal", `{"profiles":{"a":{"name":"a","values":{"limit":"5"}},"p":{"name":"p","values":{"api-key":"Dp8ProfileVal"}}}}`},
		{"base-url password", "Bu3PasswordUz", `{"profiles":{"p":{"name":"p","values":{"base-url":"https://u:Bu3PasswordUz@host.example.invalid/"}}}}`},
		{"both members of a duplicate key", "Dk4FirstMember", `{"profiles":{"p":{"name":"p","description":"Dk4FirstMember","values":{"refresh-token":"Dk4FirstMember","refresh-token":"Xk5SecondMembe"}}}}`},
		{"a duplicate values object", "Dv7FirstValues", `{"profiles":{"p":{"name":"p","description":"Dv7FirstValues","values":{"api-key":"Dv7FirstValues"},"values":{"limit":"5"}}}}`},
		{"a duplicate profile", "Dq2FirstProfil", `{"profiles":{"p":{"name":"p","values":{"api-key":"Dq2FirstProfil"}},"p":{"name":"p","description":"Dq2FirstProfil","values":{}}}}`},
		{"a duplicate base-url", "Dz3FirstBaseUr", `{"profiles":{"p":{"name":"p","description":"Dz3FirstBaseUr","values":{"base-url":"https://u:Dz3FirstBaseUr@host.example.invalid/","base-url":"https://host.example.invalid/"}}}}`},
		{"base-url query", "Bq6QueryValZ", `{"profiles":{"p":{"name":"p","values":{"base_url":"https://host.example.invalid/?%74oken=Bq6QueryValZ"}}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			qboWriteFile(t, profiles, tc.store)
			for _, args := range [][]string{{"profile", "list", "--json"}, {"profile", "show", "p"}} {
				if out := qboRun(t, env, "", args...); qboLeaks(out, tc.secret) {
					t.Errorf("%v printed it: %s", args, out)
				}
			}
		})
	}

	qboWriteFile(t, profiles, `{"profiles":{"defaults":{"name":"defaults","description":"month end run, see https://developer.intuit.com/docs","values":{"json":"true","limit":"50","select":"Id,DisplayName","notes":"free text stays usable"}}}}`)
	for _, args := range [][]string{{"profile", "show", "defaults"}, {"profile", "list", "--json"}} {
		if out := qboRun(t, env, "", args...); strings.Contains(out, "withheld") || !strings.Contains(out, "defaults") {
			t.Errorf("%v: a clean profile was withheld: %s", args, out)
		}
	}
	if out := qboRun(t, env, "", "profile", "show", "defaults"); !strings.Contains(out, "DisplayName") || !strings.Contains(out, "month end run") {
		t.Errorf("a clean profile lost its own settings: %s", out)
	}
}

// A webhook or feedback URL is its own key, in every error about it.
func TestQBOEndpointErrorsNameNoURL(t *testing.T) {
	testenv.Isolate(t)
	env := []string{"QUICKBOOKS_BASE_URL=" + qboE2EBaseURL, "QUICKBOOKS_NO_LEARN=true"}
	const key = "hook-path-9q"
	bad := "https://hooks.example.invalid/" + key + "%zz"
	for name, out := range map[string]string{
		"webhook":  qboRun(t, env, "", "profile", "list", "--json", "--deliver=webhook:"+bad),
		"feedback": qboRun(t, append(env, "QUICKBOOKS_FEEDBACK_ENDPOINT="+bad), "", "feedback", "a note", "--send"),
	} {
		if strings.Contains(out, key) {
			t.Errorf("%s: the error names the URL: %s", name, out)
		}
	}
}
