// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/cliutil/testenv"
	"zoom-phone-pp-cli/internal/config"
)

// acctTestHome sandboxes HOME (so cliutil.ConfigDir resolves to
// <home>/.config/zoom-phone-cli) and clears the credential and host env
// overrides.
func acctTestHome(t *testing.T) string {
	t.Helper()
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir)
	for _, group := range [][]string{acctCredentialEnvVars, acctHostEnvVars} {
		for _, k := range group {
			t.Setenv(k, "")
		}
	}
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".config", "zoom-phone-cli"); dir != want {
		t.Fatalf("ConfigDir = %q, want %q", dir, want)
	}
	return home
}

// acctRun executes the real root command and returns stdout, stderr and the
// process exit code main() would use.
func acctRun(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	return acctRunIn(t, strings.NewReader(stdin), args...)
}

// acctRunIn is acctRun with an arbitrary stdin (e.g. an *os.File pipe).
func acctRunIn(t *testing.T, stdin io.Reader, args ...string) (string, string, int) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	cmd.SetIn(stdin)
	var out, errb zoomLockedBuffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	code := 0
	if err := cmd.Execute(); err != nil {
		code = ExitCode(err)
		errb.WriteString(err.Error())
	}
	return out.String(), errb.String(), code
}

func acctMustAdd(t *testing.T, name, accountID, clientID, secret string) string {
	t.Helper()
	out, errOut, code := acctRun(t, secret+"\n", "accounts", "connect", name, "--account-id", accountID, "--client-id", clientID, "--client-secret-stdin", "--json")
	if code != 0 {
		t.Fatalf("accounts connect %s: exit %d stderr=%s", name, code, errOut)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("accounts connect output not JSON: %v\n%s", err, out)
	}
	path, _ := got["path"].(string)
	return path
}

func acctListNames(t *testing.T) []string {
	t.Helper()
	out, errOut, code := acctRun(t, "", "accounts", "list", "--json")
	if code != 0 {
		t.Fatalf("list exit=%d stderr=%s", code, errOut)
	}
	var listed []map[string]any
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatalf("list output: %v\n%s", err, out)
	}
	names := make([]string, 0, len(listed))
	for _, row := range listed {
		names = append(names, row["name"].(string))
	}
	return names
}

func TestAcctConnectListPathRemoveRoundTrip(t *testing.T) {
	home := acctTestHome(t)
	wantPath := filepath.Join(home, ".config", "zoom-phone-cli", "accounts", "demo", "config.toml")

	// The legacy "add" spelling is an alias of connect.
	out, errOut, code := acctRun(t, "s3cret\n", "accounts", "add", "demo", "--account-id", "A1", "--client-id", "C1", "--client-secret-stdin", "--json")
	if code != 0 {
		t.Fatalf("connect exit=%d stderr=%s", code, errOut)
	}
	var added map[string]any
	if err := json.Unmarshal([]byte(out), &added); err != nil {
		t.Fatalf("connect output: %v\n%s", err, out)
	}
	if added["name"] != "demo" || added["path"] != wantPath || added["account_id"] != "A1" || added["has_client_id"] != true || added["client_id"] != nil || added["has_secret"] != true {
		t.Fatalf("connect output = %v", added)
	}
	if strings.Contains(out, "s3cret") {
		t.Fatalf("connect output leaked the secret: %s", out)
	}

	// Windows reports no POSIX modes (0666/0777); there the ACL verdict is the check.
	if runtime.GOOS == "windows" {
		if err := cliutil.VerifyCredsPerms(wantPath); err != nil {
			t.Fatalf("account config is not private: %v", err)
		}
	} else {
		st, err := os.Stat(wantPath)
		if err != nil {
			t.Fatal(err)
		}
		if perm := st.Mode().Perm(); perm != 0o600 {
			t.Fatalf("config mode = %o, want 600", perm)
		}
		dst, err := os.Stat(filepath.Dir(wantPath))
		if err != nil {
			t.Fatal(err)
		}
		if perm := dst.Mode().Perm(); perm != 0o700 {
			t.Fatalf("account dir mode = %o, want 700", perm)
		}
	}

	cfg, err := config.Load(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "C1" || cfg.ClientSecret != "s3cret" || cfg.ZoomPhoneAccountId != "A1" {
		t.Fatalf("config.Load = client_id %q secret %q account %q", cfg.ClientID, cfg.ClientSecret, cfg.ZoomPhoneAccountId)
	}
	if cfg.HasCredentialRefusals() {
		t.Fatalf("credentials refused: %v", cfg.CredentialRefusalSummaries())
	}

	out, _, code = acctRun(t, "", "accounts", "list", "--json")
	if code != 0 {
		t.Fatalf("list exit=%d", code)
	}
	if strings.Contains(out, "s3cret") {
		t.Fatalf("list leaked the secret: %s", out)
	}
	var listed []map[string]any
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatalf("list output: %v\n%s", err, out)
	}
	if len(listed) != 1 || listed[0]["name"] != "demo" || listed[0]["path"] != wantPath || listed[0]["account_id"] != "A1" ||
		listed[0]["has_client_id"] != true || listed[0]["client_id"] != nil || listed[0]["has_secret"] != true || listed[0]["token_cached"] != false {
		t.Fatalf("list = %v", listed)
	}
	if _, ok := listed[0]["base_url"]; ok {
		t.Fatalf("default base_url should be omitted: %v", listed[0])
	}

	out, _, code = acctRun(t, "", "accounts", "path", "demo")
	if code != 0 || strings.TrimSpace(out) != wantPath {
		t.Fatalf("path exit=%d out=%q want %q", code, out, wantPath)
	}
	out, _, code = acctRun(t, "", "accounts", "path", "demo", "--json")
	var pathOut map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &pathOut) != nil || pathOut["name"] != "demo" || pathOut["path"] != wantPath {
		t.Fatalf("path --json exit=%d out=%s", code, out)
	}

	out, _, code = acctRun(t, "", "accounts", "remove", "demo", "--json")
	if code != 0 || !strings.Contains(out, `"removed": true`) {
		t.Fatalf("remove exit=%d out=%s", code, out)
	}
	if _, err := os.Stat(filepath.Dir(wantPath)); !os.IsNotExist(err) {
		t.Fatalf("account dir still present after remove: %v", err)
	}
	out, _, code = acctRun(t, "", "accounts", "list", "--json")
	if code != 0 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("list after remove exit=%d out=%q", code, out)
	}
	_, _, code = acctRun(t, "", "accounts", "remove", "demo")
	if code != 3 {
		t.Fatalf("remove missing exit=%d, want 3", code)
	}
}

// Item 1: --env must clear every override that could redirect credentials or
// hosts, including any non-allowlisted ZOOM_PHONE_* variable in the shell.
func TestAcctPathEnvUnsetsOverrides(t *testing.T) {
	acctTestHome(t)
	path := acctMustAdd(t, "acme", "A1", "C1", "s3cret")
	// Truly absent (not just empty) so the fixed unset list is what is tested;
	// t.Setenv registers the restore.
	for _, group := range [][]string{acctCredentialEnvVars, acctPathEnvVars, acctHostEnvVars} {
		for _, k := range group {
			t.Setenv(k, "")
			_ = os.Unsetenv(k)
		}
	}
	t.Setenv("ZOOM_PHONE_BASE_URL", "https://attacker.example/v2")
	t.Setenv("ZOOM_PHONE_FUTURE_OVERRIDE", "x")
	t.Setenv("ZOOM_PHONE_NO_LEARN", "1")

	out, _, code := acctRun(t, "", "accounts", "path", "acme", "--env")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "unset ") {
		t.Fatalf("--env output:\n%s", out)
	}
	unset := map[string]bool{}
	for _, name := range strings.Fields(strings.TrimPrefix(lines[0], "unset ")) {
		unset[name] = true
	}
	for _, want := range []string{
		"ZOOM_PHONE_ACCOUNT_ID", "ZOOM_PHONE_CLIENT_ID", "ZOOM_PHONE_CLIENT_SECRET",
		"ZOOM_PHONE_BASE_URL", "ZOOM_PHONE_TOKEN_URL", "ZOOM_PHONE_OAUTH_SCOPE",
		"ZOOM_PHONE_CONFIG_DIR", "ZOOM_PHONE_DATA_DIR", "ZOOM_PHONE_STATE_DIR", "ZOOM_PHONE_CACHE_DIR",
		"ZOOM_PHONE_FUTURE_OVERRIDE",
	} {
		if !unset[want] {
			t.Errorf("--env does not unset %s: %s", want, lines[0])
		}
	}
	if unset["ZOOM_PHONE_NO_LEARN"] {
		t.Errorf("--env unsets allowlisted ZOOM_PHONE_NO_LEARN")
	}
	if lines[1] != "export ZOOM_PHONE_CONFIG='"+path+"'" || lines[2] != "export ZOOM_PHONE_HOME='"+filepath.Dir(path)+"'" {
		t.Fatalf("--env exports:\n%s", out)
	}
}

func TestAcctConnectRejectsBadInput(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		env   string
		args  []string
	}{
		{"path traversal", "fixture-secret-s1\n", "", []string{"../x", "--account-id", "A1", "--client-id", "C1", "--client-secret-stdin"}},
		{"slash", "fixture-secret-s1\n", "", []string{"a/b", "--account-id", "A1", "--client-id", "C1", "--client-secret-stdin"}},
		{"uppercase", "fixture-secret-s1\n", "", []string{"Acme", "--account-id", "A1", "--client-id", "C1", "--client-secret-stdin"}},
		{"leading underscore", "fixture-secret-s1\n", "", []string{"_x", "--account-id", "A1", "--client-id", "C1", "--client-secret-stdin"}},
		{"too long", "fixture-secret-s1\n", "", []string{strings.Repeat("a", 64), "--account-id", "A1", "--client-id", "C1", "--client-secret-stdin"}},
		{"no secret", "", "", []string{"acme", "--account-id", "A1", "--client-id", "C1"}},
		{"empty stdin secret", "\n", "", []string{"acme", "--account-id", "A1", "--client-id", "C1", "--client-secret-stdin"}},
		{"env secret without --client-secret-env", "", "env-secret", []string{"acme", "--account-id", "A1", "--client-id", "C1"}},
		{"--client-secret-env with empty env", "fixture-secret-s1\n", "", []string{"acme", "--account-id", "A1", "--client-id", "C1", "--client-secret-env"}},
		{"both secret sources", "fixture-secret-s1\n", "env-secret", []string{"acme", "--account-id", "A1", "--client-id", "C1", "--client-secret-stdin", "--client-secret-env"}},
		{"missing account id", "fixture-secret-s1\n", "", []string{"acme", "--client-id", "C1", "--client-secret-stdin"}},
		{"missing name", "fixture-secret-s1\n", "", []string{"--account-id", "A1", "--client-id", "C1", "--client-secret-stdin"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			acctTestHome(t)
			t.Setenv("ZOOM_PHONE_CLIENT_SECRET", tc.env)
			_, _, code := acctRun(t, tc.stdin, append([]string{"accounts", "connect"}, tc.args...)...)
			if code != 2 {
				t.Fatalf("exit=%d, want 2", code)
			}
			root, _ := acctRootDir()
			entries, _ := os.ReadDir(root)
			if len(entries) != 0 {
				t.Fatalf("rejected connect wrote %d entries under %s", len(entries), root)
			}
		})
	}
}

// Item 7: the environment secret is used only with --client-secret-env, and
// stdin is the default source without any flag.
func TestAcctConnectSecretSources(t *testing.T) {
	acctTestHome(t)
	t.Setenv("ZOOM_PHONE_CLIENT_SECRET", "env-secret")

	_, _, code := acctRun(t, "", "accounts", "connect", "acme", "--account-id", "A1", "--client-id", "C1", "--client-secret-env", "--dry-run")
	if code != 0 {
		t.Fatalf("dry-run exit=%d", code)
	}
	root, _ := acctRootDir()
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("dry-run created %s: %v", root, err)
	}

	out, errOut, code := acctRun(t, "", "accounts", "connect", "acme", "--account-id", "A1", "--client-id", "C1", "--client-secret-env", "--json")
	if code != 0 {
		t.Fatalf("--client-secret-env exit=%d stderr=%s", code, errOut)
	}
	if strings.Contains(out, "env-secret") || !strings.Contains(out, `"has_secret": true`) {
		t.Fatalf("--client-secret-env output = %s", out)
	}
	path, _ := acctConfigPath("acme")
	if cfg, _ := config.Load(path); cfg.ClientSecret != "env-secret" {
		t.Fatalf("env secret not saved: %q", cfg.ClientSecret)
	}

	// No secret flag: piped stdin wins and the env var is ignored.
	_, errOut, code = acctRun(t, "piped-secret\n", "accounts", "connect", "globex", "--account-id", "A2", "--client-id", "C2", "--json")
	if code != 0 {
		t.Fatalf("default stdin exit=%d stderr=%s", code, errOut)
	}
	path, _ = acctConfigPath("globex")
	if cfg, _ := config.Load(path); cfg.ClientSecret != "piped-secret" {
		t.Fatalf("default stdin secret = %q, want piped-secret", cfg.ClientSecret)
	}
}

// Item 7: credential-profile writes and deletes are not MCP tools.
func TestAcctProfileWritesAreMCPHidden(t *testing.T) {
	acctTestHome(t)
	root := RootCmd()
	for _, path := range [][]string{{"accounts", "connect"}, {"accounts", "remove"}} {
		cmd, _, err := root.Find(path)
		if err != nil || cmd.Name() != path[1] {
			t.Fatalf("find %v: %v", path, err)
		}
		if cmd.Annotations["mcp:hidden"] != "true" {
			t.Errorf("%v missing mcp:hidden: %v", path, cmd.Annotations)
		}
		if cmd.Annotations["mcp:read-only"] == "true" || cmd.Annotations["mcp:local-write"] == "true" {
			t.Errorf("%v advertises a safe MCP hint: %v", path, cmd.Annotations)
		}
	}
}

// Item 7: a symlinked accounts or account directory is refused and nothing is
// written through the link.
func TestAcctConnectRefusesSymlinkedDirs(t *testing.T) {
	t.Run("accounts dir", func(t *testing.T) {
		home := acctTestHome(t)
		elsewhere := filepath.Join(home, "elsewhere")
		if err := os.MkdirAll(elsewhere, 0o700); err != nil {
			t.Fatal(err)
		}
		root, _ := acctRootDir()
		if err := os.MkdirAll(filepath.Dir(root), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(elsewhere, root); err != nil {
			t.Fatal(err)
		}
		_, errOut, code := acctRun(t, "fixture-secret-s1\n", "accounts", "connect", "acme", "--account-id", "A1", "--client-id", "C1")
		if code == 0 || !strings.Contains(errOut, "symlink") {
			t.Fatalf("exit=%d stderr=%s", code, errOut)
		}
		if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
			t.Fatalf("wrote %d entries through the symlink", len(entries))
		}
	})
	t.Run("account dir", func(t *testing.T) {
		home := acctTestHome(t)
		elsewhere := filepath.Join(home, "elsewhere")
		if err := os.MkdirAll(elsewhere, 0o700); err != nil {
			t.Fatal(err)
		}
		root, _ := acctRootDir()
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(elsewhere, filepath.Join(root, "acme")); err != nil {
			t.Fatal(err)
		}
		_, errOut, code := acctRun(t, "fixture-secret-s1\n", "accounts", "connect", "acme", "--account-id", "A1", "--client-id", "C1", "--force")
		if code == 0 || !strings.Contains(errOut, "symlink") {
			t.Fatalf("exit=%d stderr=%s", code, errOut)
		}
		if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
			t.Fatalf("wrote %d entries through the symlink", len(entries))
		}
		_, _, code = acctRun(t, "", "accounts", "remove", "acme")
		if code == 0 {
			t.Fatalf("remove followed a symlinked account dir")
		}
		if _, err := os.Stat(elsewhere); err != nil {
			t.Fatalf("symlink target removed: %v", err)
		}
	})
}

func TestAcctConnectOverwriteNeedsForce(t *testing.T) {
	acctTestHome(t)
	path := acctMustAdd(t, "acme", "A1", "C1", "old-secret")
	credsPath, err := cliutil.CredentialsFilePathForConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cliutil.AtomicWritePrivateFile(credsPath, []byte("access_token = 'stale'\n"), 0o600, 0o700); err != nil {
		t.Fatal(err)
	}

	_, _, code := acctRun(t, "new-secret\n", "accounts", "connect", "acme", "--account-id", "A2", "--client-id", "C2", "--client-secret-stdin")
	if code != 2 {
		t.Fatalf("overwrite without --force exit=%d, want 2", code)
	}
	if cfg, _ := config.Load(path); cfg.ClientSecret != "old-secret" {
		t.Fatalf("profile changed without --force: secret %q", cfg.ClientSecret)
	}

	_, errOut, code := acctRun(t, "new-secret\n", "accounts", "connect", "acme", "--account-id", "A2", "--client-id", "C2", "--client-secret-stdin", "--force", "--json")
	if code != 0 {
		t.Fatalf("--force exit=%d stderr=%s", code, errOut)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "C2" || cfg.ClientSecret != "new-secret" || cfg.ZoomPhoneAccountId != "A2" || cfg.AccessToken != "" {
		t.Fatalf("after --force: id %q secret %q account %q token %q", cfg.ClientID, cfg.ClientSecret, cfg.ZoomPhoneAccountId, cfg.AccessToken)
	}
	if _, err := os.Stat(credsPath); !os.IsNotExist(err) {
		t.Fatalf("stale cached token survived --force: %v", err)
	}
}

func TestAcctListEmptyIsEmptyArray(t *testing.T) {
	acctTestHome(t)
	out, _, code := acctRun(t, "", "accounts", "list", "--json")
	if code != 0 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("exit=%d out=%q, want [] and 0", code, out)
	}
}

// Item 9: non-default hosts persisted into a profile are visible.
func TestAcctListShowsNonDefaultHosts(t *testing.T) {
	acctTestHome(t)
	acmePath := acctMustAdd(t, "acme", "A1", "C1", "s1")
	globexPath := acctMustAdd(t, "globex", "A2", "C2", "s2")
	appendTOML := func(path, extra string) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := cliutil.AtomicWritePrivateFile(path, append(data, []byte(extra)...), 0o600, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	appendTOML(acmePath, "base_url = 'http://127.0.0.1:9999/v2'\ntoken_url = 'http://127.0.0.1:9999/oauth/token'\n")
	appendTOML(globexPath, "base_url = 'https://api.zoom.us/v2'\n")

	out, _, code := acctRun(t, "", "accounts", "list", "--json")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	var listed []map[string]any
	if err := json.Unmarshal([]byte(out), &listed); err != nil || len(listed) != 2 {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if listed[0]["base_url"] != "http://127.0.0.1:9999/v2" || listed[0]["token_url"] != "http://127.0.0.1:9999/oauth/token" {
		t.Fatalf("acme hosts not shown: %v", listed[0])
	}
	if _, ok := listed[1]["base_url"]; ok {
		t.Fatalf("globex default base_url shown: %v", listed[1])
	}
}

// Item 3: after `eval "$(accounts path acme --env)"` the accounts directory is
// still the operator's, so list (and fleet) see every account.
func TestAcctRootIgnoresSelectedAccountHome(t *testing.T) {
	home := acctTestHome(t)
	acmePath := acctMustAdd(t, "acme", "A1", "C1", "s1")
	acctMustAdd(t, "globex", "A2", "C2", "s2")

	t.Setenv("ZOOM_PHONE_HOME", filepath.Dir(acmePath))
	t.Setenv("ZOOM_PHONE_CONFIG", acmePath)
	if got := acctListNames(t); strings.Join(got, ",") != "acme,globex" {
		t.Fatalf("with acme selected, list = %v, want acme,globex", got)
	}
	out, _, code := acctRun(t, "", "accounts", "path", "globex")
	if want := filepath.Join(home, ".config", "zoom-phone-cli", "accounts", "globex", "config.toml"); code != 0 || strings.TrimSpace(out) != want {
		t.Fatalf("path globex with acme selected = %q (exit %d), want %q", out, code, want)
	}

	// A general-purpose ZOOM_PHONE_HOME (not an account home) still relocates
	// the accounts directory like every other config path.
	general := filepath.Join(home, "zp-home")
	t.Setenv("ZOOM_PHONE_HOME", general)
	t.Setenv("ZOOM_PHONE_CONFIG", "")
	root, err := acctRootDir()
	if err != nil || root != filepath.Join(general, "config", "accounts") {
		t.Fatalf("general home root = %q (%v)", root, err)
	}
	if got := acctListNames(t); len(got) != 0 {
		t.Fatalf("general home list = %v, want empty", got)
	}
}

// TestAcctTokenMintStaysInAccountHome proves the isolation fleet relies on:
// with ZOOM_PHONE_HOME set to the account directory, the generated token
// save lands in that account's data/credentials.toml, never in the default
// credentials file, and a reload of the (now scrubbed) config still resolves
// that account's own credentials.
func TestAcctTokenMintStaysInAccountHome(t *testing.T) {
	home := acctTestHome(t)
	defaultCreds, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	path := acctMustAdd(t, "acme", "A1", "C1", "s3cret")
	t.Setenv("ZOOM_PHONE_HOME", filepath.Dir(path))
	t.Setenv("ZOOM_PHONE_CONFIG", path) // the pairing `accounts path --env` exports

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, "tok-acme", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(defaultCreds); !os.IsNotExist(err) {
		t.Fatalf("token mint wrote the default credentials file %s (err=%v)", defaultCreds, err)
	}
	if !strings.HasPrefix(defaultCreds, home) {
		t.Fatalf("default credentials path %s escaped the sandbox", defaultCreds)
	}
	accountCreds := filepath.Join(filepath.Dir(path), "data", "credentials.toml")
	data, err := os.ReadFile(accountCreds)
	if err != nil {
		t.Fatalf("account credentials file: %v", err)
	}
	if !strings.Contains(string(data), "tok-acme") || !strings.Contains(string(data), "phone_account_id = 'A1'") {
		t.Fatalf("account credentials file missing token or account id:\n%s", data)
	}

	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ClientID != "C1" || reloaded.ZoomPhoneAccountId != "A1" || reloaded.AccessToken != "tok-acme" {
		t.Fatalf("reload: id %q account %q token %q", reloaded.ClientID, reloaded.ZoomPhoneAccountId, reloaded.AccessToken)
	}

	info, err := acctLoadInfo("acme", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if info.AccountID != "A1" || !info.HasClientID || !info.HasSecret || !info.TokenCached {
		t.Fatalf("list view after mint = %+v", info)
	}
	out, _, code := acctRun(t, "", "accounts", "list", "--json")
	if code != 0 || strings.Contains(out, "tok-acme") || strings.Contains(out, "s3cret") {
		t.Fatalf("list after mint exit=%d leaked a credential: %s", code, out)
	}
	if !strings.Contains(out, `"name": "acme"`) || !strings.Contains(out, `"token_cached": true`) || !strings.Contains(out, `"has_secret": true`) {
		t.Fatalf("list after mint lost the account's scrubbed credentials: %s", out)
	}
}

// Round 2 item 4: without a secret flag, stdin is read only when it is a
// non-empty file or a pipe that delivers the secret promptly; an idle pipe,
// /dev/null or an empty file fails fast with the flag hint instead of hanging.
func TestAcctConnectImplicitStdin(t *testing.T) {
	prev := acctImplicitStdinWait
	acctImplicitStdinWait = 150 * time.Millisecond
	t.Cleanup(func() { acctImplicitStdinWait = prev })
	connect := func(t *testing.T, in io.Reader, name string, extra ...string) (string, int) {
		t.Helper()
		args := append([]string{"accounts", "connect", name, "--account-id", "A1", "--client-id", "C1"}, extra...)
		_, errOut, code := acctRunIn(t, in, args...)
		return errOut, code
	}
	savedSecret := func(t *testing.T, name string) string {
		t.Helper()
		path, _ := acctConfigPath(name)
		cfg, err := config.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return cfg.ClientSecret
	}

	t.Run("idle pipe fails fast", func(t *testing.T) {
		acctTestHome(t)
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		defer r.Close()
		start := time.Now()
		errOut, code := connect(t, r, "acme")
		if code != 2 || !strings.Contains(errOut, "--client-secret-stdin") || !strings.Contains(errOut, "nothing arrived") {
			t.Fatalf("exit=%d stderr=%s", code, errOut)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("idle pipe took %s", elapsed)
		}
		if ok, _ := acctExists("acme"); ok {
			t.Fatalf("profile written from an idle pipe")
		}
	})
	t.Run("pipe with a line, writer still open", func(t *testing.T) {
		acctTestHome(t)
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		defer r.Close()
		if _, err := w.WriteString("pipe-secret\n"); err != nil {
			t.Fatal(err)
		}
		if errOut, code := connect(t, r, "acme"); code != 0 {
			t.Fatalf("exit=%d stderr=%s", code, errOut)
		}
		if got := savedSecret(t, "acme"); got != "pipe-secret" {
			t.Fatalf("secret = %q", got)
		}
	})
	t.Run("explicit --client-secret-stdin waits for EOF", func(t *testing.T) {
		acctTestHome(t)
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		go func() {
			time.Sleep(300 * time.Millisecond) // longer than the implicit wait
			_, _ = w.WriteString("slow-secret")
			_ = w.Close()
		}()
		if errOut, code := connect(t, r, "acme", "--client-secret-stdin"); code != 0 {
			t.Fatalf("exit=%d stderr=%s", code, errOut)
		}
		if got := savedSecret(t, "acme"); got != "slow-secret" {
			t.Fatalf("secret = %q", got)
		}
	})
	t.Run("regular file", func(t *testing.T) {
		home := acctTestHome(t)
		p := filepath.Join(home, "secret.txt")
		if err := os.WriteFile(p, []byte("file-secret\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if errOut, code := connect(t, f, "acme"); code != 0 {
			t.Fatalf("exit=%d stderr=%s", code, errOut)
		}
		if got := savedSecret(t, "acme"); got != "file-secret" {
			t.Fatalf("secret = %q", got)
		}
	})
	t.Run("empty file and /dev/null", func(t *testing.T) {
		home := acctTestHome(t)
		empty := filepath.Join(home, "empty.txt")
		if err := os.WriteFile(empty, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		for _, p := range []string{empty, os.DevNull} {
			f, err := os.Open(p)
			if err != nil {
				t.Fatal(err)
			}
			errOut, code := connect(t, f, "acme")
			_ = f.Close()
			if code != 2 || !strings.Contains(errOut, "--client-secret-stdin") {
				t.Fatalf("%s: exit=%d stderr=%s", p, code, errOut)
			}
		}
	})
}
