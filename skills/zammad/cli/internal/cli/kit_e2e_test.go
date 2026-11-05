// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the credential kit adoption on the zammad
// 4.34.0 publish (2026-11-03). Pinned by this connector's handfixes.json
// under the "credkit-wiring" entry.

package cli

import (
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
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

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"zammad-pp-cli/internal/client"
)

// The kit's Start runs only in main, so its guarantees are proved on the
// built binary. ZAMMAD_E2E_BIN runs these same tests against another build,
// which is how each class's test was seen failing on the commit before the
// kit.
var (
	kitBinOnce sync.Once
	kitBinPath string
	kitBinErr  error
)

func kitBin(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the binary")
	}
	if b := os.Getenv("ZAMMAD_E2E_BIN"); b != "" {
		return b
	}
	kitBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "zammad-kit-e2e")
		if err != nil {
			kitBinErr = err
			return
		}
		kitBinPath = filepath.Join(dir, "zammad-cli")
		if runtime.GOOS == "windows" {
			kitBinPath += ".exe" // exec finds nothing without it
		}
		out, err := exec.Command("go", "build", "-o", kitBinPath, "../../cmd/zammad-cli").CombinedOutput()
		if err != nil {
			kitBinErr = fmt.Errorf("%w\n%s", err, out)
		}
	})
	if kitBinErr != nil {
		t.Fatal(kitBinErr)
	}
	return kitBinPath
}

// kitRun runs the built CLI with a home of its own, and returns stdout,
// stderr and the exit code.
func kitRun(t *testing.T, home string, env []string, args ...string) (string, string, int) {
	t.Helper()
	return kitRunStdin(t, home, "", env, args...)
}

// kitRunStdin is kitRun with stdin as a pipe that carries stdin.
func kitRunStdin(t *testing.T, home, stdin string, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(kitBin(t), args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	cmd.Env = append([]string{"HOME=" + home, "USERPROFILE=" + home, "ZAMMAD_HOME=" + home, "PATH=" + os.Getenv("PATH")}, env...)
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

// kitStub is an https vendor stub (the kit sends no credential over plain
// http) and the environment that points the CLI at it.
func kitStub(t *testing.T, h http.HandlerFunc) []string {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	ca := filepath.Join(t.TempDir(), "stub-ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return []string{"ZAMMAD_BASE_URL=" + srv.URL, "ZAMMAD_CA_CERT=" + ca}
}

// kitFilesHolding names every file under root that carries s.
func kitFilesHolding(t *testing.T, root, s string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), s) {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// TestKitClassAConfigCredentialsNeverReachOutput is the class of the broken
// configuration review (Codex nb-r1-1, nb-r2): a credential a configuration
// file carries reaches no output. Each earlier fix listed one shape (the
// literal inputs are in TestAdoptBrokenConfigIsRefused, which the commit
// before the kit already passes); these are the same class on a file that
// parses, every one a leak before the kit. Every part of a held value is
// held, and every output is checked, so the shape of the value stops
// mattering.
func TestKitClassAConfigCredentialsNeverReachOutput(t *testing.T) {
	cases := map[string]struct{ body, flag string }{
		"base_url query value": {"base_url = \"https://h.example.com/api/v1?x=QueryValueSecret77\"\n", "QueryValueSecret77"},
		"base_url userinfo":    {"base_url = \"https://UserPartSecret77:pw@h.example.com/api/v1\"\n", "UserPartSecret77"},
		"api_token piece":      {"api_token = \"PieceTokenAbcdefgh1234\"\n", "PieceTokenAb"},
		"api_token whole":      {"api_token = \"PieceTokenAbcdefgh1234\"\n", "PieceTokenAbcdefgh1234"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(c.body), 0o600); err != nil {
				t.Fatal(err)
			}
			stdout, stderr, code := kitRun(t, home, nil, "--config", path, "--no-learn", "--"+c.flag)
			if strings.Contains(stdout+stderr, c.flag) {
				t.Fatalf("an unknown flag repeated the credential:\n%s%s", stdout, stderr)
			}
			if code == 0 {
				t.Fatalf("the unknown flag exited 0:\n%s%s", stdout, stderr)
			}
		})
	}
}

// TestKitClassBLearnRecordsCarryNoPieceOfAToken is the class of the journal
// session key (Codex nb-r1-4, nb-r2): a record is checked as it is written,
// not field by field before the last field is set, and a piece of a token
// counts. The literal input (the whole token as ZAMMAD_LEARN_SESSION) was
// already clean before the kit; a piece through the same stores was not: it
// reached the journal, the learn store, the profile file and the feedback
// file.
func TestKitClassBLearnRecordsCarryNoPieceOfAToken(t *testing.T) {
	const token, piece = "PieceTokenAbcdefgh1234", "PieceTokenAbc"
	home := t.TempDir()
	env := []string{"ZAMMAD_API_TOKEN=" + token, "ZAMMAD_LEARN_SESSION=" + token}
	for _, args := range [][]string{
		{"version"},
		{"teach-playbook", "--query", piece + " tickets", "--notes", piece + " notes", "--json"},
		{"recall", piece + " tickets", "--json"},
		{"profile", "save", piece, "--json"},
		{"feedback", piece + " message", "--json"},
	} {
		stdout, stderr, _ := kitRun(t, home, env, args...)
		if strings.Contains(stdout+stderr, piece) {
			t.Errorf("%v repeated a piece of the token:\n%s%s", args, stdout, stderr)
		}
	}
	if found := kitFilesHolding(t, home, piece); len(found) != 0 {
		t.Fatalf("files carry a piece of the token: %v", found)
	}
	// A record that is clean still lands, so the test cannot pass by the
	// journal writing nothing.
	stdout, stderr, code := kitRun(t, home, nil, "recall", "tickets", "--json")
	if code != 0 {
		t.Fatalf("recall without a token: exit %d\n%s%s", code, stdout, stderr)
	}
	if len(kitFilesHolding(t, filepath.Join(home, "state"), `"recall"`)) == 0 {
		t.Fatalf("a clean recall journaled nothing under %s", home)
	}
}

// TestKitLocalRowsAreChecked: a row that repeats a credential from an old
// sync reaches no output, with or without a --config that holds nothing.
func TestKitLocalRowsAreChecked(t *testing.T) {
	token := "test-secret-legacy-row-token"
	path, db := r1Store(t)
	r1UpsertTicket(t, db, map[string]any{"id": 42, "title": token, "state_id": 2, "priority_id": 2, "created_at": "2026-10-01T00:00:00Z", "updated_at": "2026-10-01T00:00:00Z"})
	r1UpsertTicket(t, db, map[string]any{"id": 43, "title": "plain title", "state_id": 2, "priority_id": 2, "created_at": "2026-10-01T00:00:00Z", "updated_at": "2026-10-01T00:00:00Z"})
	r1Complete(t, db, "tickets")
	db.Close()
	unreadable := filepath.Join(t.TempDir(), "unreadable.toml")
	if err := os.WriteFile(unreadable, []byte("base_url = \"https://zammad.example.com/api/v1\"\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	env := []string{"ZAMMAD_API_TOKEN=" + token}
	home := t.TempDir()
	for _, args := range [][]string{
		{"overdue", "--db", path, "--json"},
		{"stale", "--days", "7", "--db", path},
		{"orphans", "--db", path, "--json"},
		{"overdue", "--db", path, "--config", unreadable, "--json"},
	} {
		stdout, stderr, _ := kitRun(t, home, env, args...)
		if strings.Contains(stdout+stderr, token) {
			t.Errorf("%v printed a stored credential:\n%s%s", args, stdout, stderr)
		}
	}
	// The same store with no credential held prints the rows.
	stdout, stderr, _ := kitRun(t, home, nil, "overdue", "--db", path, "--json")
	if !strings.Contains(stdout, "plain title") {
		t.Fatalf("overdue with no credential held lost the rows:\n%s%s", stdout, stderr)
	}
}

// TestKitOffOriginRedirectCarriesNoCredential: Go forwards every header but
// Authorization on a redirect, so a [headers] credential would reach the
// new host. The kit refuses the hop.
func TestKitOffOriginRedirectCarriesNoCredential(t *testing.T) {
	secret := "adh-" + strings.Repeat("Hd5r", 12)
	var mu sync.Mutex
	offHits := 0
	off := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if strings.Contains(fmt.Sprint(r.Header), secret) {
			offHits++
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1}`)
	}))
	defer off.Close()
	sameHits := 0
	env := kitStub(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if r.Header.Get("X-Api-Key") == secret {
			sameHits++
		}
		mu.Unlock()
		http.Redirect(w, r, off.URL+"/elsewhere", http.StatusFound)
	})
	base := strings.TrimPrefix(env[0], "ZAMMAD_BASE_URL=")
	config := filepath.Join(t.TempDir(), "config.toml")
	body := fmt.Sprintf("base_url = %q\napi_token = \"tok-%s\"\n[headers]\nX-Api-Key = %q\n", base, strings.Repeat("Rd7t", 10), secret)
	if err := os.WriteFile(config, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, _ := kitRun(t, t.TempDir(), env[1:], "--config", config, "tickets", "get", "1", "--json", "--no-cache")
	mu.Lock()
	defer mu.Unlock()
	if sameHits == 0 {
		t.Fatalf("the credential never reached its own origin, so this proves nothing:\n%s%s", stdout, stderr)
	}
	if offHits != 0 {
		t.Fatalf("a credential header followed the redirect off origin:\n%s%s", stdout, stderr)
	}
}

// TestKitStdinCredentialsAreHeldFirst: a credential piped in reached output
// before the transport could hold it, in a --dry-run preview of the body and
// in a flag error (Codex kit-r1). The class: what stdin can carry is held at
// startup, before anything is parsed: the whole of it for auth set-token, the
// declared credential fields of a --stdin body, and the same fields of any
// body a dry run previews. A body with no such field is not held, or every
// answer repeating part of it would be withheld.
func TestKitStdinCredentialsAreHeldFirst(t *testing.T) {
	const pw = "NovelPassword77"
	env := []string{"ZAMMAD_API_TOKEN=" + "Qq7zZx9w" + "K2bV4nM8pL3t", "ZAMMAD_BASE_URL=https://h.example.com/api/v1"}
	body := `{"email":"a@example.org","password":"` + pw + `"}`
	input := filepath.Join(t.TempDir(), "users.jsonl")
	if err := os.WriteFile(input, []byte(body+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		stdin, secret string
		args          []string
	}{
		"dry run preview":   {body, pw, []string{"users", "create", "--stdin", "--dry-run", "--no-learn"}},
		"nested field":      {`{"email":"a@example.org","profile":{"password":"` + pw + `"}}`, pw, []string{"users", "create", "--stdin", "--dry-run", "--no-learn"}},
		"flag error":        {body, pw, []string{"users", "create", "--stdin", "--no-learn", "--" + pw}},
		"unparsable body":   {`{"password":"` + pw + `"`, pw, []string{"users", "create", "--stdin", "--no-learn", "--" + pw}},
		"set-token flag":    {"StdinSecret77\n", "StdinSecret77", []string{"auth", "set-token", "--no-learn", "--StdinSecret77"}},
		"import dry run":    {"", pw, []string{"import", "users", "--input", input, "--dry-run", "--no-learn"}},
		"import stdin dash": {body + "\n", pw, []string{"import", "users", "--input", "-", "--dry-run", "--no-learn"}},
	} {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, _ := kitRunStdin(t, t.TempDir(), c.stdin, env, c.args...)
			if strings.Contains(stdout+stderr, c.secret) {
				t.Fatalf("output carries the piped credential:\n%s%s", stdout, stderr)
			}
		})
	}
	stdout, stderr, _ := kitRunStdin(t, t.TempDir(), `{"email":"plain@example.org"}`, env, "users", "create", "--stdin", "--dry-run", "--no-learn")
	if !strings.Contains(stdout+stderr, "plain@example.org") {
		t.Fatalf("a body with no credential field was withheld:\n%s%s", stdout, stderr)
	}
}

// TestKitBodyCredentialFieldsMatchTheDeclaration: the startup holder and the
// transport read the same fields.
func TestKitBodyCredentialFieldsMatchTheDeclaration(t *testing.T) {
	if !slices.Equal(Declarations.CredentialFields, client.BodyCredentialFields) || !slices.Equal(Declarations.MCPArguments, client.BodyCredentialFields) {
		t.Fatalf("declared %v and %v, the holder reads %v", Declarations.CredentialFields, Declarations.MCPArguments, client.BodyCredentialFields)
	}
}

// kitWriteProfiles writes a run-profile store, the way an older version or a
// hand edit could have left it.
func kitWriteProfiles(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestKitProfileStoreIsACredentialSource is the class of the profile file: it
// can carry a credential, a URL or free text. Only a flag some command declares
// that takes no credential comes out of it, and the URL credentials in any of
// its fields are held from startup, from the legacy file too.
func TestKitProfileStoreIsACredentialSource(t *testing.T) {
	const store = `{"profiles":{
		"ok":{"name":"ok","values":{"select":"id,title","json":"true"}},
		"stray":{"name":"stray","values":{"select":"id","password":"PlainPassw0rd77","api_token":"PlainTokenAbc77","not-a-flag":"StrayValueAbc77"}},
		"url":{"name":"url","description":"a run profile","values":{"select":"id","base-url":"https://ops:FlagPassw0rd77@h.example/?api_key=FlagQueryAbc77"}}
	}}`
	home := t.TempDir()
	kitWriteProfiles(t, filepath.Join(home, "config"), store)

	stdout, stderr, code := kitRun(t, home, nil, "profile", "show", "ok", "--no-learn")
	if code != 0 || !strings.Contains(stdout, "id,title") {
		t.Fatalf("an allowed flag must still show (%d):\n%s%s", code, stdout, stderr)
	}
	stdout, stderr, _ = kitRun(t, home, nil, "profile", "show", "stray", "--no-learn")
	if !strings.Contains(stdout, `"select"`) {
		t.Fatalf("the allowed flag beside the dropped ones must stay:\n%s%s", stdout, stderr)
	}
	for _, leak := range []string{"PlainPassw0rd77", "PlainTokenAbc77", "StrayValueAbc77", "not-a-flag"} {
		if strings.Contains(stdout+stderr, leak) {
			t.Errorf("profile show printed %s:\n%s%s", leak, stdout, stderr)
		}
	}
	for _, args := range [][]string{{"profile", "show", "url"}, {"profile", "use", "url"}, {"profile", "list"}, {"--FlagPassw0rd77"}, {"--FlagQueryAbc77"}} {
		stdout, stderr, _ := kitRun(t, home, nil, append(args, "--no-learn")...)
		for _, leak := range []string{"FlagPassw0rd77", "FlagQueryAbc77"} {
			if strings.Contains(stdout+stderr, leak) {
				t.Errorf("%v printed %s:\n%s%s", args, leak, stdout, stderr)
			}
		}
	}

	legacyHome := t.TempDir()
	kitWriteProfiles(t, filepath.Join(legacyHome, ".zammad-cli"), store)
	stdout, stderr, _ = kitRun(t, legacyHome, nil, "--FlagPassw0rd77", "--no-learn")
	if strings.Contains(stdout+stderr, "FlagPassw0rd77") {
		t.Errorf("the legacy file was not held:\n%s%s", stdout, stderr)
	}
}

// TestKitProfileBaseURLCredentialsAreHeld: a profile value under base-url is a
// source (contract section 1), so its password and every query value are held at
// any length. The name field repeats the value, to show it is withheld.
func TestKitProfileBaseURLCredentialsAreHeld(t *testing.T) {
	for name, baseURL := range map[string]string{
		"password":             "https://ops:q7Z@wiki.test/",
		"token as the user":    "https://q7Z@wiki.test/",
		"encoded password":     "https://ops:%71%37%5A@wiki.test/",
		"credential query":     "https://h.test/?token=q7Z",
		"any query value":      "https://h.test/?v=q7Z",
		"encoded query":        "https://h.test/?token=%71%37%5A#top",
		"semicolon separated":  "https://h.test/?x=1;token=q7Z",
		"URL that fails parse": "https://h.test:bad/?token=q7Z",
	} {
		for _, key := range []string{"base-url", "base_url"} {
			t.Run(name+" "+key, func(t *testing.T) {
				home := t.TempDir()
				body, err := json.Marshal(map[string]any{"profiles": map[string]any{"p": map[string]any{"name": "q7Z", "values": map[string]string{"select": "id", key: baseURL}}}})
				if err != nil {
					t.Fatal(err)
				}
				kitWriteProfiles(t, filepath.Join(home, "config"), string(body))
				stdout, stderr, _ := kitRun(t, home, nil, "profile", "show", "p", "--select", "name", "--json", "--no-learn")
				if strings.Contains(stdout+stderr, "q7Z") {
					t.Fatalf("it was printed:\n%s%s", stdout, stderr)
				}
			})
		}
	}
}

// TestKitProfileCredentialsEveryDuplicate: a duplicate key keeps only its last
// member in a decoded tree; every member is a credential the file carries.
func TestKitProfileCredentialsEveryDuplicate(t *testing.T) {
	for name, values := range map[string]string{
		"first of two":   `"api_token":"q7Z","api_token":"later"`,
		"last of two":    `"api_token":"later","api_token":"q7Z"`,
		"private key":    `"private-key":"q7Z"`,
		"first base URL": `"base-url":"https://ops:q7Z@a.test/","base-url":"https://b.test/"`,
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			kitWriteProfiles(t, filepath.Join(home, "config"), `{"profiles":{"p":{"name":"q7Z","values":{"select":"id",`+values+`}}}}`)
			stdout, stderr, _ := kitRun(t, home, nil, "profile", "show", "p", "--select", "name", "--json", "--no-learn")
			if strings.Contains(stdout+stderr, "q7Z") {
				t.Fatalf("it was printed:\n%s%s", stdout, stderr)
			}
		})
	}
	t.Run("free text stays readable", func(t *testing.T) {
		home := t.TempDir()
		kitWriteProfiles(t, filepath.Join(home, "config"), `{"profiles":{"p":{"name":"p","description":"see https://ops:wikipage@wiki.test/?token=readme","values":{"select":"id"}}}}`)
		stdout, stderr, _ := kitRun(t, home, nil, "profile", "show", "p", "--no-learn")
		if !strings.Contains(stdout, "wikipage") {
			t.Fatalf("a description is not a source (contract section 1) and must still show:\n%s%s", stdout, stderr)
		}
	})
}

// TestKitOriginIsTheBaseURLTheProgramUses: a file the program does not read
// never chooses the origin. The selected config here names no base URL, so the
// program goes to the shipped default; a dead proxy stops that request here.
func TestKitOriginIsTheBaseURLTheProgramUses(t *testing.T) {
	const tokenField = "api_" + "token"
	var otherHits atomic.Int32
	other := kitStub(t, func(http.ResponseWriter, *http.Request) { otherHits.Add(1) })
	otherBase := strings.TrimPrefix(other[0], "ZAMMAD_BASE_URL=")
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	token := fmt.Sprintf("%s = %q\n", tokenField, "SelectedTokenAbc77")
	withBase := fmt.Sprintf("base_url = %q\n%s = %q\n", otherBase, tokenField, "UnselectedTokenAbc77")
	cases := map[string]func(t *testing.T, home string) []string{
		"an earlier home names one": func(t *testing.T, home string) []string {
			first, last := t.TempDir(), t.TempDir()
			write(filepath.Join(first, "config", "config.toml"), withBase)
			write(filepath.Join(last, "config", "config.toml"), token)
			return []string{"--home", first, "--home", last}
		},
		"the default home names one": func(t *testing.T, home string) []string {
			last := t.TempDir()
			write(filepath.Join(home, "config", "config.toml"), withBase)
			write(filepath.Join(last, "config", "config.toml"), token)
			return []string{"--home", last}
		},
		"a legacy config names one": func(t *testing.T, home string) []string {
			write(filepath.Join(home, ".config", "zammad-cli", "config.toml"), withBase)
			write(filepath.Join(home, "config", "config.toml"), token)
			return nil
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			args := append(setup(t, home), "tickets", "list", "--no-learn", "--no-cache", "--data-source", "live")
			stdout, stderr, _ := kitRun(t, home, []string{other[1], "HTTPS_PROXY=http://127.0.0.1:9"}, args...)
			if strings.Contains(stdout+stderr, "withheld from outbound") {
				t.Fatalf("the program's own base URL was refused:\n%s%s", stdout, stderr)
			}
			if otherHits.Load() != 0 {
				t.Fatalf("a request reached the file the program did not read")
			}
		})
	}
	if Declarations.BaseURL.Default != client.ShippedBaseURL {
		t.Fatalf("declared default %q, the holder uses %q", Declarations.BaseURL.Default, client.ShippedBaseURL)
	}
}

// TestKitExplicitConfigChoosesTheOrigin: the credential goes to the base URL of
// the config the program really uses. A discovered default's base URL is no
// origin, though its values are held; the environment outranks both.
func TestKitExplicitConfigChoosesTheOrigin(t *testing.T) {
	const tokenField = "api_" + "token"
	stub := func(hits *atomic.Int32) []string {
		return kitStub(t, func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			_, _ = w.Write([]byte("[]"))
		})
	}
	var usedHits, otherHits atomic.Int32
	used, other := stub(&usedHits), stub(&otherHits)
	baseOf := func(env []string) string { return strings.TrimPrefix(env[0], "ZAMMAD_BASE_URL=") }
	writeConfig := func(path, base, token string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		body := fmt.Sprintf("base_url = %q\n%s = %q\n", base, tokenField, token)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(t *testing.T, home string, env []string, args ...string) {
		t.Helper()
		usedHits.Store(0)
		otherHits.Store(0)
		// Only the chosen stub's CA is trusted, so the other stub fails its TLS check.
		stdout, stderr, code := kitRun(t, home, append(env, used[1]), append(args, "tickets", "list", "--no-learn", "--no-cache", "--data-source", "live")...)
		if code != 0 || usedHits.Load() == 0 {
			t.Fatalf("the request did not reach the chosen base URL (exit %d, %d hits):\n%s%s", code, usedHits.Load(), stdout, stderr)
		}
		if otherHits.Load() != 0 {
			t.Fatalf("a request reached the other base URL")
		}
	}

	t.Run("explicit config over a discovered default", func(t *testing.T) {
		home := t.TempDir()
		writeConfig(filepath.Join(home, "config", "config.toml"), baseOf(other), "DiscoveredTokenAbc77")
		explicit := filepath.Join(t.TempDir(), "chosen.toml")
		writeConfig(explicit, baseOf(used), "ChosenTokenAbcdef77")
		run(t, home, nil, "--config", explicit)
	})
	t.Run("config from the environment over a discovered default", func(t *testing.T) {
		home := t.TempDir()
		writeConfig(filepath.Join(home, "config", "config.toml"), baseOf(other), "DiscoveredTokenAbc77")
		explicit := filepath.Join(t.TempDir(), "chosen.toml")
		writeConfig(explicit, baseOf(used), "ChosenTokenAbcdef77")
		run(t, home, []string{"ZAMMAD_CONFIG=" + explicit})
	})
	t.Run("base URL from the environment over a config", func(t *testing.T) {
		home := t.TempDir()
		writeConfig(filepath.Join(home, "config", "config.toml"), baseOf(other), "DiscoveredTokenAbc77")
		run(t, home, []string{used[0]})
	})
	t.Run("the last of several homes", func(t *testing.T) {
		first, last := t.TempDir(), t.TempDir()
		writeConfig(filepath.Join(first, "config", "config.toml"), baseOf(other), "FirstHomeTokenAbc77")
		writeConfig(filepath.Join(last, "config", "config.toml"), baseOf(used), "LastHomeTokenAbc77")
		run(t, t.TempDir(), nil, "--home", first, "--home", last)
	})
	t.Run("a home flag that is another flag's value", func(t *testing.T) {
		chosen, decoy := t.TempDir(), t.TempDir()
		writeConfig(filepath.Join(chosen, "config", "config.toml"), baseOf(used), "ChosenHomeTokenAbc77")
		writeConfig(filepath.Join(decoy, "config", "config.toml"), baseOf(other), "DecoyHomeTokenAbc77")
		for _, tail := range [][]string{{"--home", "42"}, {"--home", decoy}} {
			usedHits.Store(0)
			args := append([]string{"--home", chosen, "tickets", "get", "--select"}, tail...)
			stdout, stderr, code := kitRun(t, t.TempDir(), []string{used[1]}, append(args, "--no-learn", "--no-cache", "--data-source", "live")...)
			if strings.Contains(stdout+stderr, "withheld from outbound") || usedHits.Load() == 0 {
				t.Fatalf("%v: the request did not reach the chosen home's base URL (exit %d, %d hits):\n%s%s", tail, code, usedHits.Load(), stdout, stderr)
			}
		}
	})
	t.Run("a config path the loader reads literally", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows drops a trailing space from a file name, so there is one file here")
		}
		dir := t.TempDir()
		trimmed, literal := filepath.Join(dir, "tenant.toml"), filepath.Join(dir, "tenant.toml ")
		writeConfig(trimmed, baseOf(other), "TrimmedPathTokenAbc77")
		writeConfig(literal, baseOf(used), "LiteralPathTokenAbc77")
		run(t, t.TempDir(), nil, "--config", literal)
		run(t, t.TempDir(), []string{"ZAMMAD_CONFIG=" + literal})
		stdout, stderr, _ := kitRun(t, t.TempDir(), nil, "--config", literal, "--LiteralPathTokenAbc77", "--no-learn")
		if strings.Contains(stdout+stderr, "LiteralPathTokenAbc77") {
			t.Fatalf("the literal file's token was not held:\n%s%s", stdout, stderr)
		}
	})
	t.Run("the last of several configs", func(t *testing.T) {
		first, last := filepath.Join(t.TempDir(), "a.toml"), filepath.Join(t.TempDir(), "b.toml")
		writeConfig(first, baseOf(other), "FirstConfigTokenAbc77")
		writeConfig(last, baseOf(used), "LastConfigTokenAbc77")
		run(t, t.TempDir(), nil, "--config", first, "--config", last)
	})
	t.Run("a discovered default's token is held under an explicit config", func(t *testing.T) {
		home := t.TempDir()
		writeConfig(filepath.Join(home, "config", "config.toml"), baseOf(other), "DiscoveredTokenAbc77")
		explicit := filepath.Join(t.TempDir(), "chosen.toml")
		writeConfig(explicit, baseOf(used), "ChosenTokenAbcdef77")
		stdout, stderr, _ := kitRun(t, home, nil, "--config", explicit, "--DiscoveredTokenAbc77", "--no-learn")
		if strings.Contains(stdout+stderr, "DiscoveredTokenAbc77") {
			t.Fatalf("the discovered token was not held:\n%s%s", stdout, stderr)
		}
	})
}

// TestKitEveryLearnFileIsChecked: the journal's offset file went beside the
// kit, so a value in it reached the file (Codex kit-r1). The class: every file
// the learn journal writes passes the kit, the cursor included.
func TestKitEveryLearnFileIsChecked(t *testing.T) {
	const secret = "journal-"
	home := t.TempDir()
	env := []string{"ZAMMAD_WEBHOOK_SECRET=" + secret}
	for _, args := range [][]string{{"version"}, {"recall", "tickets", "--json"}, {"recall", "tickets", "--json"}} {
		kitRun(t, home, env, args...)
	}
	if found := kitFilesHolding(t, home, secret); len(found) != 0 {
		t.Fatalf("files carry the credential: %v", found)
	}
}

// TestKitWebhookListenerAnswersCarryNoText: every answer from the listener is
// a status code with an empty body, so no text goes out beside the kit (the
// class of Codex kit-r1's MCP HTTP finding).
func TestKitWebhookListenerAnswersCarryNoText(t *testing.T) {
	f := newWebhookFixture(t)
	body := []byte(`{"event":"zammad.ticket.updated","ticket_id":"42","article_id":""}`)
	if w := f.post(http.MethodPost, body, sign("s3cret", body), "d0"); w.Code != http.StatusOK || w.Body.Len() != 0 {
		t.Errorf("recorded: answered %d with a body: %q", w.Code, w.Body)
	}
	for name, w := range map[string]*httptest.ResponseRecorder{
		"unsigned":    f.post(http.MethodPost, body, "sha1=00", "d1"),
		"not a POST":  f.post(http.MethodGet, body, sign("s3cret", body), "d2"),
		"no delivery": f.post(http.MethodPost, body, sign("s3cret", body), ""),
	} {
		if w.Code == http.StatusOK || w.Body.Len() != 0 {
			t.Errorf("%s: answered %d with a body: %q", name, w.Code, w.Body)
		}
	}
}

// Codex r1 [P1] kb_get.go: a 2xx answer that echoes the token reached stdout.
func TestR1KBGetDoesNotPrintAnEchoedToken(t *testing.T) {
	token := "test-secret-kb-echo-token"
	env := kitStub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"KnowledgeBaseAnswer":{"20":{"id":20}},"KnowledgeBaseAnswerTranslation":{"201":{"id":201,"answer_id":20,"title":"FAQ","content":"` + token + `"}}}`))
	})
	stdout, stderr, _ := kitRun(t, t.TempDir(), append(env, "ZAMMAD_API_TOKEN="+token), "kb", "get", "20", "--json", "--no-cache")
	if strings.Contains(stdout+stderr, token) {
		t.Fatalf("kb get printed the echoed token:\n%s%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "withheld from stdout") {
		t.Fatalf("kb get did not say its answer was withheld:\n%s%s", stdout, stderr)
	}
}

// Codex r1 [P1] doctor.go: a 200 plain-text answer read as reachable + valid.
func TestR1DoctorReadsTheAnswer(t *testing.T) {
	doctor := func(t *testing.T, contentType, body string) (creds, api string) {
		t.Helper()
		env := kitStub(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", contentType)
			_, _ = w.Write([]byte(body))
		})
		stdout, stderr, _ := kitRun(t, t.TempDir(), append(env, "ZAMMAD_API_TOKEN=doctor-truth-token-0a1b2c3d"), "doctor", "--json", "--no-cache")
		var report map[string]any
		if err := json.Unmarshal([]byte(stdout), &report); err != nil {
			t.Fatalf("doctor --json is not JSON: %v\n%s%s", err, stdout, stderr)
		}
		creds, _ = report["credentials"].(string)
		api, _ = report["api"].(string)
		return creds, api
	}
	creds, api := doctor(t, "text/plain", "Service temporarily unavailable")
	if !strings.HasPrefix(creds, "ERROR") || !strings.HasPrefix(api, "ERROR") {
		t.Fatalf("doctor must fail both rows on a non-JSON answer: api=%q credentials=%q", api, creds)
	}
	creds, api = doctor(t, "application/json", `{"id":3,"login":"agent@example.test"}`)
	if !strings.HasPrefix(creds, "valid") || !strings.HasPrefix(api, "reachable") {
		t.Fatalf("doctor no longer blesses a real JSON answer: api=%q credentials=%q", api, creds)
	}
}

// kitInChild reports whether the calling test is in a child of this test
// binary, starting that child when it is not: the kit's held set lives for
// the process and has no reset, so a test that holds a value starts with
// `if !kitInChild(t) { return }` and its held set dies with the child.
func kitInChild(t *testing.T) bool {
	t.Helper()
	if os.Getenv("ZAMMAD_KIT_CHILD") != "" {
		return true
	}
	child := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$")
	child.Env = append(os.Environ(), "ZAMMAD_KIT_CHILD=1")
	if out, err := child.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
	return false
}

// The startup walk answers for the parser before the parser can run, so the two
// must agree on every flag of every command, or the kit allows the wrong origin.
func TestStartupFlagsAgreeWithTheParser(t *testing.T) {
	check := func(args []string) {
		t.Helper()
		cmd, rest, err := newRootCmd(&rootFlags{}).Find(args)
		if err != nil || cmd.ParseFlags(rest) != nil {
			return // refused, so nothing is sent
		}
		wantHome, _ := cmd.Flags().GetString("home")
		wantConfig, _ := cmd.Flags().GetString("config")
		if home, configFlag := client.StartupHomeAndConfig(args); home != wantHome || configFlag != wantConfig {
			t.Errorf("%q: walk says home %q config %q, parser says home %q config %q", args, home, configFlag, wantHome, wantConfig)
		}
	}
	var walk func(c *cobra.Command, path []string)
	walk = func(c *cobra.Command, path []string) {
		seen := func(f *pflag.Flag) {
			for _, name := range []string{"--" + f.Name, "-" + f.Shorthand} {
				if name == "-" {
					continue
				}
				check(append(slices.Clone(path), name, "--home", "/h", "--config", "/c"))
				check(append(slices.Clone(path), "--home", "/h", name, "--config", "/c"))
				check(append(slices.Clone(path), name+"=1", "--home", "/h"))
				check(append(slices.Clone(path), name+"1", "--home", "/h"))
			}
		}
		c.Flags().VisitAll(seen)
		c.PersistentFlags().VisitAll(seen)
		c.InheritedFlags().VisitAll(seen)
		for _, sub := range c.Commands() {
			walk(sub, append(slices.Clone(path), sub.Name()))
		}
	}
	walk(newRootCmd(&rootFlags{}), nil)
	check([]string{"--home", "/h", "--", "--home", "/other"})
}
