// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): the failing inputs of the last review's open
// classes (handfixes.json credkit-wiring), each driven through the built CLI.

package cli

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"avanan-pp-cli/internal/cliutil"
	"avanan-pp-cli/internal/config"
	"avanan-pp-cli/internal/credkit"
	"avanan-pp-cli/internal/credwire"
)

// flagParseOutput runs the built CLI so its flag parser echoes value in an
// error, and returns what it printed.
func flagParseOutput(t *testing.T, bin, value string, flags ...string) string {
	t.Helper()
	cmd := exec.Command(bin, append(flags, "--timeout", value, "auth", "status")...)
	cmd.Env = os.Environ()
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err == nil {
		t.Fatalf("an invalid --timeout was accepted:\n%s", out.String())
	}
	return out.String()
}

// TestStartupSelectionReadsPastAFlagValueDoubleDash: pflag takes "--" as the
// value of the flag before it and keeps parsing, so a --config (or --home)
// after it is the file the run selects, and its credentials are held at start.
func TestStartupSelectionReadsPastAFlagValueDoubleDash(t *testing.T) {
	bin := buildCommand(t, "../../cmd/avanan-cli")
	for name, flags := range map[string]func(path string) []string{
		"config after config":       func(p string) []string { return []string{"--config", "--", "--config", p} },
		"config after another flag": func(p string) []string { return []string{"--data-source", "--", "--config", p} },
	} {
		t.Run(name, func(t *testing.T) {
			home := isolateAvananEnv(t)
			t.Setenv("AVANAN_CONFIG", "")
			const v = "file-secret-817"
			path := filepath.Join(home, "private", "b.toml")
			writeSandboxFile(t, path, `client_secret = "`+v+`"`+"\n")
			assertNoCredentialForm(t, "flag-parse output", flagParseOutput(t, bin, v, flags(path)...), v)
		})
	}
	t.Run("home after config", func(t *testing.T) {
		home := isolateAvananEnv(t)
		t.Setenv("AVANAN_CONFIG", "")
		const v = "file-secret-818"
		writeSandboxFile(t, tenantConfigPath(t, filepath.Join(home, "TENANT")), `client_secret = "`+v+`"`+"\n")
		assertNoCredentialForm(t, "flag-parse output", flagParseOutput(t, bin, v, "--config", "--", "--home", filepath.Join(home, "TENANT")), v)
	})
}

// TestMalformedConfigHoldsDecodedTOMLEscapes: a file that does not parse still
// gives up each string as TOML decodes it, not only as it is written.
func TestMalformedConfigHoldsDecodedTOMLEscapes(t *testing.T) {
	bin := buildCommand(t, "../../cmd/avanan-cli")
	for name, tc := range map[string]struct{ body, value string }{
		"eight-digit escape": {"app_id = \"\\U00000073ecret-817\"\nbroken =\n", "secret-817"},
		"four-digit escape":  {"app_id = \"\\u0073ecret-819\"\nbroken =\n", "secret-819"},
		"header escape":      {"[headers]\nX-Key = \"\\u0073ecret-820\"\nbroken =\n", "secret-820"},
		"unknown key escape": {"other = \"\\u0073ecret-821\"\nbroken =\n", "secret-821"},
		"line continuation":  {"app_id = \"\"\"secret-\\\n   822\"\"\"\nbroken =\n", "secret-822"},
		"short escaped":      {"app_id = \"\\u0073x\"\nbroken =\n", "sx"},
	} {
		t.Run(name, func(t *testing.T) {
			home := isolateAvananEnv(t)
			writeSandboxFile(t, filepath.Join(home, "config.toml"), tc.body)
			assertNoCredentialForm(t, "flag-parse output", flagParseOutput(t, bin, tc.value), tc.value)
		})
	}
}

// TestHeaderPartsHaveNoShapeFilter: a part of a [headers] value is a credential
// whatever its characters: a punctuation-only cookie value or Basic password
// is refused by a sink that sees only that part.
func TestHeaderPartsHaveNoShapeFilter(t *testing.T) {
	bin := buildCommand(t, "../../cmd/avanan-cli")
	for name, header := range map[string]string{
		"cookie value":       `Cookie = "sid=!!!!"`,
		"second cookie":      `Cookie = "theme=dark; sid=!!!!"`,
		"quoted cookie":      `Cookie = 'sid="!!!!"'`,
		"basic password":     `Authorization = "Basic ` + base64.StdEncoding.EncodeToString([]byte("user:!!!!")) + `"`,
		"malformed cookie":   "Cookie = \"sid=!!!!\"\nbroken =",
		"other header value": `X-Upstream = "k=!!!!"`,
	} {
		t.Run(name, func(t *testing.T) {
			home := isolateAvananEnv(t)
			writeSandboxFile(t, filepath.Join(home, "config.toml"), "[headers]\n"+header+"\n")
			cmd := exec.Command(bin, "feedback", "cookie !!!!")
			cmd.Env = os.Environ()
			out, _ := cmd.CombinedOutput()
			assertStoredNowhere(t, home, "!!!!")
			if bytes.Contains(out, []byte("cookie !!!!")) {
				t.Errorf("output carries the part:\n%s", out)
			}
		})
	}
}

// tenantConfigPath is config.toml under --home tenant, as the program resolves it.
func tenantConfigPath(t *testing.T, tenant string) string {
	t.Helper()
	restore, err := cliutil.SetHomeOverride(tenant)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "config.toml")
}

// TestRequestSignatureIsHeld: x-av-sig authenticates a request, so a server
// that reflects it cannot get it printed.
func TestRequestSignatureIsHeld(t *testing.T) {
	bin := buildCommand(t, "../../cmd/avanan-cli")
	isolateAvananEnv(t)
	var mu sync.Mutex
	var sig string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.0/auth" {
			_, _ = w.Write([]byte("sig-test-session-SG1"))
			return
		}
		mu.Lock()
		sig = r.Header.Get("x-av-sig")
		mu.Unlock()
		_, _ = w.Write([]byte(`{"responseEnvelope":{"responseCode":200},"responseData":["` + r.Header.Get("x-av-sig") + `"]}`))
	}))
	defer srv.Close()
	cmd := exec.Command(bin, "scopes", "--data-source", "live", "--no-cache", "--json")
	cmd.Env = append(os.Environ(), "AVANAN_BASE_URL="+srv.URL, "AVANAN_CA_CERT="+serverCAFile(t, srv),
		"AVANAN_APP_ID=sig-test-app-SG1", "AVANAN_CLIENT_SECRET=sig-test-cs-SG1")
	out, _ := cmd.CombinedOutput()
	mu.Lock()
	defer mu.Unlock()
	if sig == "" {
		t.Fatalf("no signed request reached the server:\n%s", out)
	}
	assertNoCredentialForm(t, "scopes output", string(out), sig)
}

// TestSetTokenPositionalNeverReachesAnError: the positional of auth set-token is
// a credential, so nothing printed before the command runs can repeat it: not a
// flag parse error, not a flag check that runs before RunE.
func TestSetTokenPositionalNeverReachesAnError(t *testing.T) {
	bin := buildCommand(t, "../../cmd/avanan-cli")
	const v = "positional-app-PA1"
	for _, args := range [][]string{
		{"auth", "set-token", v, "--timeout", v},
		{"--timeout", v, "auth", "set-token", v},
		{"auth", "set-token", "--timeout", v, v},
		{"auth", "set-token", v, "--data-source", v},
		{"auth", "set-token", v, "--" + v},
		{"auth", "set-token", "-" + v},
		{"auth", "set-token", v, "--profile", v},
		{"auth", "set-token", v, "extra-positional-PA1", "--timeout", "extra-positional-PA1"},
	} {
		isolateAvananEnv(t)
		cmd := exec.Command(bin, args...)
		cmd.Env = os.Environ()
		out, _ := cmd.CombinedOutput()
		assertNoCredentialForm(t, strings.Join(args, " "), string(out), v)
		assertNoCredentialForm(t, strings.Join(args, " "), string(out), "extra-positional-PA1")
	}
}

// TestRunProfilesCarryNoCredential: a credential flag in profiles.json, current
// or legacy, is not a credential source: no command shows it and no command
// takes it. (A kit-withheld save cannot put one there; an older version or a hand
// edit can.)
func TestRunProfilesCarryNoCredential(t *testing.T) {
	bin := buildCommand(t, "../../cmd/avanan-cli")
	const app, sec = "profile-app-RP1", "profile-sec-RP1"
	body := `{"profiles":{"auth":{"name":"auth","values":{"app-id":"` + app + `","secret":"` + sec + `","APP_ID":"` + app + `","timeout":"5s"}}}}`
	for name, path := range map[string]func(home string) string{
		"current": func(string) string {
			p, err := profileStorePath()
			if err != nil {
				t.Fatal(err)
			}
			return p
		},
		"legacy": func(home string) string { return filepath.Join(os.Getenv("HOME"), ".avanan-cli", "profiles.json") },
	} {
		t.Run(name, func(t *testing.T) {
			home := isolateAvananEnv(t)
			writeSandboxFile(t, path(home), body)
			for _, args := range [][]string{
				{"profile", "show", "auth", "--json"},
				{"profile", "use", "auth"},
				{"profile", "list", "--json"},
				{"auth", "login", "--profile", "auth", "--json"},
			} {
				cmd := exec.Command(bin, args...)
				cmd.Env = os.Environ()
				out, _ := cmd.CombinedOutput()
				assertNoCredentialForm(t, strings.Join(args, " "), string(out), app)
				assertNoCredentialForm(t, strings.Join(args, " "), string(out), sec)
			}
		})
	}
}

// TestOwnerMarkerPassesTheKit: the legacy store's owner marker holds a hash a
// held value can equal. Such a marker is not written, and the claim is declined.
func TestOwnerMarkerPassesTheKit(t *testing.T) {
	if !heldChild(t) {
		return
	}
	dir := t.TempDir()
	const hash = "ownerhash-OM1"
	credkit.Hold("config:unclassified", hash)
	unscoped := filepath.Join(dir, "data.db")
	writeSandboxFile(t, unscoped, "x")
	if owned, _ := claimLegacyUnscopedDB(dir, unscoped, hash, "test"); owned {
		t.Fatal("the claim succeeded on a marker that was withheld")
	}
	assertStoredNowhere(t, dir, hash)
}

// TestRunProfilesKeepOnlyAllowedFlags: a run profile carries output and read
// filters and nothing else, so a URL or a free-text value that routes a request
// never comes from one, and a value under a credential flag or base-url is held at
// startup, every duplicate included. A plain profile still prints and applies.
func TestRunProfilesKeepOnlyAllowedFlags(t *testing.T) {
	bin := buildCommand(t, "../../cmd/avanan-cli")
	const pass, qry, dup = "profile-pass-BU1", "profile-query-BU1", "profile-dup-BU1"
	isolateAvananEnv(t)
	path, err := profileStorePath()
	if err != nil {
		t.Fatal(err)
	}
	writeSandboxFile(t, path, `{"profiles":{"p":{"name":"p","description":"a saved set",
		"values":{"base-url":"https://operator:`+pass+`@gw.example","deliver":"webhook:https://hook.example/x?token=`+qry+`","yes":"true","limit":"7","app-id":"`+dup+`","app-id":""}},
		"q":{"name":"q","values":{"deliver":"file:out.json","yes":"true","limit":"7"}}}}`)
	for _, args := range [][]string{
		{"profile", "show", "p", "--json"},
		{"profile", "use", "p"},
		{"profile", "list", "--json"},
	} {
		cmd := exec.Command(bin, args...)
		cmd.Env = os.Environ()
		out, _ := cmd.CombinedOutput()
		for _, v := range []string{pass, qry} {
			assertNoCredentialForm(t, strings.Join(args, " "), string(out), v)
		}
	}
	cmd := exec.Command(bin, "profile", "use", "q")
	cmd.Env = os.Environ()
	out, _ := cmd.CombinedOutput()
	if s := string(out); !strings.Contains(s, "limit") || strings.Contains(s, "deliver") || strings.Contains(s, "yes") {
		t.Fatalf("profile use should keep limit and drop deliver and yes:\n%s", s)
	}

	isolateAvananEnv(t)
	save := exec.Command(bin, "profile", "save", "bad", "--deliver", "file:x")
	save.Env = os.Environ()
	if out, err := save.CombinedOutput(); err == nil {
		t.Fatalf("profile save accepted --deliver:\n%s", out)
	}
	save = exec.Command(bin, "profile", "save", "briefing", "--json", "--compact", "--data-source", "local")
	save.Env = os.Environ()
	if out, err := save.CombinedOutput(); err != nil {
		t.Fatalf("profile save refused plain flags: %v\n%s", err, out)
	}
	show := exec.Command(bin, "profile", "show", "briefing", "--json")
	show.Env = os.Environ()
	if out, _ := show.CombinedOutput(); !strings.Contains(string(out), "briefing") || strings.Contains(string(out), "withheld") {
		t.Errorf("a plain profile no longer prints:\n%s", out)
	}
	// A flag error echoes the value it was given, so it shows what the kit holds.
	for _, v := range []string{pass, dup} {
		isolatedPath, err := profileStorePath()
		if err != nil {
			t.Fatal(err)
		}
		writeSandboxFile(t, isolatedPath, `{"profiles":{"p":{"name":"p","values":{"base-url":"https://operator:`+pass+`@gw.example","app-id":"`+dup+`","app-id":""}}}}`)
		cmd := exec.Command(bin, "--timeout", v, "version")
		cmd.Env = os.Environ()
		out, _ := cmd.CombinedOutput()
		assertNoCredentialForm(t, "flag error", string(out), v)
	}
}

// TestStdinBodyIsHeld: StdinFlags holds a --stdin body before anything prints,
// whether it parses or not, and the request still carries the body.
func TestStdinBodyIsHeld(t *testing.T) {
	bin := buildCommand(t, "../../cmd/avanan-cli")
	isolateAvananEnv(t)
	run := func(body string, env []string, args ...string) string {
		cmd := exec.Command(bin, args...)
		cmd.Env = append(os.Environ(), env...)
		cmd.Stdin = strings.NewReader(body)
		out, _ := cmd.CombinedOutput()
		return string(out)
	}
	for name, body := range map[string]string{
		"field":      `{"requestData":{"password":"stdin-body-cred-SB2"}}`,
		"multi-line": "{\n  \"requestData\": {\n    \"password\": \"stdin-body-cred-SB2\"\n  }\n}\n",
		"malformed":  `{"requestData":{"password":"stdin-body-cred-SB2"`,
	} {
		assertNoCredentialForm(t, name+" --stdin --dry-run", run(body, nil, "action", "post-entity", "--stdin", "--dry-run"), "stdin-body-cred-SB2")
	}

	const filter = `{"requestData":{"entityFilter":{"saas":"office365_emails"}}}`
	var got atomic.Value
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.0/auth" {
			_, _ = w.Write([]byte("stdin-test-session-SB3"))
			return
		}
		data, _ := io.ReadAll(r.Body)
		got.Store(string(data))
		_, _ = w.Write([]byte(`{"responseEnvelope":{"responseCode":200},"responseData":[{"entityId":"answer-SB3"}]}`))
	}))
	defer srv.Close()
	out := run(filter,
		[]string{"AVANAN_BASE_URL=" + srv.URL, "AVANAN_CA_CERT=" + serverCAFile(t, srv), "AVANAN_APP_ID=stdin-test-app-SB3", "AVANAN_CLIENT_SECRET=stdin-test-cs-SB3"},
		"event", "query", "--stdin", "--no-cache", "--json")
	if !strings.Contains(out, "answer-SB3") {
		t.Errorf("a read whose answer does not repeat its body was lost:\n%s", out)
	}
	if sent, _ := got.Load().(string); !strings.Contains(sent, "office365_emails") {
		t.Errorf("the request lost the --stdin body: %q", sent)
	}
}

// TestCredentialsGoOnlyToTheSelectedBaseURL: the one allowed origin is the base
// URL this run really uses, whether a flag, the environment or the selected config
// names it. No regional or vendor wildcard stands beside it.
func TestCredentialsGoOnlyToTheSelectedBaseURL(t *testing.T) {
	for _, o := range credwire.Declarations.Origins {
		if o.Source == "*" && !slices.Equal(o.Allowed, []string{"base_url"}) {
			t.Fatalf("default origins = %v, want only base_url", o.Allowed)
		}
	}
	bin := buildCommand(t, "../../cmd/avanan-cli")
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/v1.0/auth" {
			_, _ = w.Write([]byte("origin-test-session-OR1"))
			return
		}
		_, _ = w.Write([]byte(`{"responseEnvelope":{"responseCode":200},"responseData":[]}`))
	}))
	defer srv.Close()
	creds := []string{"AVANAN_CA_CERT=" + serverCAFile(t, srv), "AVANAN_APP_ID=origin-test-app-OR1", "AVANAN_CLIENT_SECRET=origin-test-cs-OR1"}

	reached := func(name string, env []string, args ...string) {
		t.Helper()
		hits.Store(0)
		cmd := exec.Command(bin, args...)
		cmd.Env = append(append(os.Environ(), creds...), env...)
		out, _ := cmd.CombinedOutput()
		if hits.Load() == 0 {
			t.Fatalf("%s: no request reached the selected base URL:\n%s", name, out)
		}
	}
	home := isolateAvananEnv(t)
	reached("--base-url", nil, "auth", "login", "--base-url", srv.URL)
	cmd := exec.Command(bin, "auth", "login", "--base-url", "https://u:flag-pass-BU2@127.0.0.1:1")
	cmd.Env = append(os.Environ(), creds...)
	out, _ := cmd.CombinedOutput()
	assertNoCredentialForm(t, "--base-url", string(out), "flag-pass-BU2")
	reached("environment", []string{"AVANAN_BASE_URL=" + srv.URL}, "scopes", "--data-source", "live", "--no-cache")
	selected := filepath.Join(home, "selected.toml")
	writeSandboxFile(t, selected, `base_url = "`+srv.URL+`"`+"\n")
	if err := os.Chmod(selected, 0o600); err != nil {
		t.Fatal(err)
	}
	reached("--config", nil, "--config", selected, "scopes", "--data-source", "live", "--no-cache")

	// A discovered default names another host; the explicit file has no
	// base_url, so this run's base URL is the built-in one. Only a unit can say
	// so: a live run would send to the real vendor.
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	writeSandboxFile(t, filepath.Join(dir, "config.toml"), `base_url = "`+srv.URL+`"`+"\n")
	empty := filepath.Join(home, "empty.toml")
	writeSandboxFile(t, empty, "timeout = 1\n")
	if got := config.SelectedBaseURL(empty); got != credwire.Declarations.BaseURL.Default {
		t.Errorf("explicit file without base_url: selected %q, want the built-in", got)
	}
	if got := config.SelectedBaseURL(""); got != srv.URL {
		t.Errorf("discovered default: selected %q, want %q", got, srv.URL)
	}
}

// TestProfileCredentialsHeldInEveryShape: a value under a credential flag's name
// or under base-url in profiles.json is held at startup whatever shape the file
// takes (review inputs: a file that does not parse, a value nested beneath the
// key, a base URL query pair after a semicolon). A command that never loads the
// profile prints a log line repeating it, and the line must be withheld. A plain
// profile withholds nothing.
func TestProfileCredentialsHeldInEveryShape(t *testing.T) {
	bin := buildCommand(t, "../../cmd/avanan-cli")
	isolateAvananEnv(t)
	const cred = "ProfileSecret9"
	list := func(profiles, logged string) string {
		home := t.TempDir()
		writeSandboxFile(t, filepath.Join(home, "config", "profiles.json"), profiles)
		writeSandboxFile(t, filepath.Join(home, "data", "feedback.jsonl"), `{"text":"`+logged+`"}`+"\n")
		cmd := exec.Command(bin, "feedback", "list", "--json")
		cmd.Env = append(os.Environ(), "AVANAN_HOME="+home, "AVANAN_CONFIG="+filepath.Join(home, "config.toml"))
		out, _ := cmd.CombinedOutput()
		return string(out)
	}
	for name, profiles := range map[string]string{
		"malformed":       `{"profiles":{"p":{"values":{"limit":oops,"token":"` + cred + `"}}}}`,
		"nested":          `{"profiles":{"p":{"values":{"token":{"value":"` + cred + `"}}}}}`,
		"nested in array": `{"profiles":{"p":{"values":{"secret":[{"value":"` + cred + `"}]}}}}`,
		"duplicate":       `{"profiles":{"p":{"values":{"app-id":"` + cred + `","app-id":""}}}}`,
		"semicolon":       `{"profiles":{"p":{"values":{"base-url":"https://gw.example/?token=` + cred + `;x=1"}}}}`,
		"base-url nested": `{"profiles":{"p":{"values":{"base-url":["https://op:` + cred + `@gw.example/"]}}}}`,
	} {
		assertNoCredentialForm(t, name+" profile, feedback list", list(profiles, cred), cred)
	}
	if out := list(`{"profiles":{"p":{"name":"p","values":{"limit":"7","data-source":"local"}}}}`, "plain-line-PR9"); !strings.Contains(out, "plain-line-PR9") {
		t.Errorf("a plain profile withheld an ordinary log line:\n%s", out)
	}
}

// TestImportRecordIsHeldBeforeItsPreview: an import record read from stdin or a
// file is a request body, so its credential fields are held before the dry-run
// preview prints it, and a plain record's preview stays readable.
func TestImportRecordIsHeldBeforeItsPreview(t *testing.T) {
	bin := buildCommand(t, "../../cmd/avanan-cli")
	isolateAvananEnv(t)
	run := func(record string, args ...string) string {
		cmd := exec.Command(bin, args...)
		cmd.Env = os.Environ()
		cmd.Stdin = strings.NewReader(record + "\n")
		out, _ := cmd.CombinedOutput()
		return string(out)
	}
	const cred = "ImportStdinSecret9"
	assertNoCredentialForm(t, "import --input -", run(`{"accessKey":"`+cred+`"}`, "import", "action", "--input", "-", "--dry-run"), cred)
	file := filepath.Join(t.TempDir(), "records.jsonl")
	writeSandboxFile(t, file, `{"requestData":{"accessKey":"`+cred+`"}}`+"\n")
	assertNoCredentialForm(t, "import --input file", run("", "import", "action", "--input", file, "--dry-run"), cred)
	if out := run(`{"entityIds":["plain-entity-IM1"]}`, "import", "action", "--input", "-", "--dry-run"); !strings.Contains(out, "plain-entity-IM1") {
		t.Errorf("a plain record's preview was lost:\n%s", out)
	}
}

// TestFeedbackStdinIsRecorded (missing data): --stdin is a StdinFlags name, so
// the kit held a `feedback --stdin` body whole and withheld its local record
// while the command said it was recorded. Feedback text is no credential
// (`feedback <text>` is never held), so its body is taken off stdin before the
// kit reads it; a held credential pasted into it is still withheld.
func TestFeedbackStdinIsRecorded(t *testing.T) {
	bin := buildCommand(t, "../../cmd/avanan-cli")
	home := isolateAvananEnv(t)
	const token = "feedback-held-token-FB7"
	const text = "the sidebar is slow"
	// Global flags may stand before the command, a value flag among them.
	for i, lead := range [][]string{nil, {"--no-learn"}, {"--timeout", "30s"}, {"--select=x", "--no-learn"}} {
		note := fmt.Sprintf("%s %d", text, i)
		for _, body := range []string{note, "pasted " + token + " by mistake"} {
			cmd := exec.Command(bin, append(slices.Clone(lead), "feedback", "--stdin")...)
			cmd.Env = append(os.Environ(), "AVANAN_TOKEN="+token)
			cmd.Stdin = strings.NewReader(body + "\n")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v feedback --stdin: %v\n%s", lead, err, out)
			}
		}
		path, err := feedbackFilePath()
		if err != nil {
			t.Fatal(err)
		}
		record, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no feedback record under %s: %v", home, err)
		}
		if !strings.Contains(string(record), note) {
			t.Errorf("%v feedback --stdin: feedback.jsonl lost the note:\n%s", lead, record)
		}
		assertNoCredentialForm(t, "feedback.jsonl", string(record), token)
	}

	// A flag value that reads "feedback" does not make another command's
	// --stdin body skip the kit: the body stays held whole.
	const cred = "stdin-only-cred-FB8"
	cmd := exec.Command(bin, "--select", "feedback", "action", "post-entity", "--stdin", "--dry-run")
	cmd.Env = os.Environ()
	cmd.Stdin = strings.NewReader(`{"requestData":{"password":"` + cred + `"}}` + "\n")
	out, _ := cmd.CombinedOutput()
	assertNoCredentialForm(t, "--select feedback action post-entity --stdin --dry-run", string(out), cred)
}
