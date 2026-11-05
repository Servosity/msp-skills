// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package applebusinessblackbox

// Failing inputs from the publish review, each driven on the built binary. Every
// test here was seen failing on the code it fixes: .review/review-inputs-before.log.

import (
	"bufio"
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	acmeFixture  = "Acme-" + "Hk47Rt92Ws15Zq"
	bravoFixture = "Bravo-" + "Zyxwvuts9876Pq"
	bodyFixture  = "Stdin-" + "Zk38Wq71Lp"
	hookFixture  = "HookPw-" + "Qr83Vb61Tn"
	envFixture   = "EnvTok-" + "Mn48Xc27Vb91"
	dupeFixture  = "Dupe-" + "Lk52Tg83Hs"
)

// runStdin is runCLIEnv with text on stdin.
func runStdin(t *testing.T, bin, home, keyRef string, f *fake, stdin string, extra []string, args ...string) string {
	t.Helper()
	cmd := exec.Command(filepath.Join(bin, "apple-business-cli"), args...)
	cmd.Env = append(env(home, keyRef, f), extra...)
	cmd.Stdin = strings.NewReader(stdin)
	out, _ := cmd.CombinedOutput()
	return string(out)
}

// findFile returns the first file under root with this name, or "".
func findFile(t *testing.T, root, name string) string {
	t.Helper()
	found := ""
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == name && found == "" {
			found = path
		}
		return nil
	})
	return found
}

// anyFileHolds reports the first file under root whose bytes contain text.
func anyFileHolds(t *testing.T, root, text string) string {
	t.Helper()
	found := ""
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || found != "" {
			return nil
		}
		if b, readErr := os.ReadFile(path); readErr == nil && bytes.Contains(b, []byte(text)) {
			found = path
		}
		return nil
	})
	return found
}

// APPLE_BUSINESS_HOME is how an MSP keeps one customer's login from another's.
func TestHomeVariableSelectsTheStore(t *testing.T) {
	bin := binaries(t)
	f := newFake(t)
	home := t.TempDir()
	key := keyFile(t, f)
	for fixture, dir := range map[string]string{acmeFixture: t.TempDir(), bravoFixture: t.TempDir()} {
		out := runStdin(t, bin, home, key, f, fixture, []string{"APPLE_BUSINESS_HOME=" + dir}, "auth", "set-token")
		if anyFileHolds(t, dir, fixture) == "" {
			t.Errorf("APPLE_BUSINESS_HOME=%s holds no saved login after set-token:\n%s", dir, out)
		}
		if p := anyFileHolds(t, home, fixture); p != "" {
			t.Errorf("a login under APPLE_BUSINESS_HOME landed in the shared store: %s", p)
		}
	}
}

// --dry-run changes no local file, on any command an operator can pass it to.
func TestDryRunChangesNoLocalFile(t *testing.T) {
	bin := binaries(t)
	f := newFake(t)
	home := t.TempDir()
	key := keyFile(t, f)
	runStdin(t, bin, home, key, f, acmeFixture, nil, "auth", "set-token")
	store := findFile(t, home, "credentials.toml")
	if store == "" {
		t.Fatal("set-token saved no credentials file")
	}
	before, _ := os.ReadFile(store)

	if out, _ := runCLI(t, bin, home, key, f, "auth", "logout", "--dry-run"); func() bool {
		now, err := os.ReadFile(store)
		return err != nil || !bytes.Equal(now, before)
	}() {
		t.Errorf("auth logout --dry-run changed the saved login:\n%s", out)
		_ = os.WriteFile(store, before, 0o600)
	}
	out := runStdin(t, bin, home, key, f, bravoFixture, nil, "auth", "set-token", "--dry-run")
	if now, err := os.ReadFile(store); err != nil || !bytes.Equal(now, before) {
		t.Errorf("auth set-token --dry-run changed the saved login:\n%s", out)
		_ = os.WriteFile(store, before, 0o600)
	}

	backup := filepath.Join(t.TempDir(), "backup.jsonl")
	writePrivate(t, backup, "the only copy\n")
	out, _ = runCLI(t, bin, home, key, f, "export", "devices", "--dry-run", "--output", backup)
	if got, _ := os.ReadFile(backup); string(got) != "the only copy\n" {
		t.Errorf("export --dry-run --output overwrote the file with %q:\n%s", got, out)
	}

	const webhook = "webhook:https://hooks.invalid/h"
	if out, code := runCLI(t, bin, home, key, f, "profile", "save", "keep", "--deliver", webhook); code != 0 {
		t.Fatalf("profile save: exit %d\n%s", code, out)
	}
	out, _ = runCLI(t, bin, home, key, f, "profile", "delete", "keep", "--yes", "--dry-run")
	if show, code := runCLI(t, bin, home, key, f, "profile", "show", "keep"); code != 0 {
		t.Errorf("profile delete --dry-run removed the profile:\n%s\nshow: %s", out, show)
	}
}

// Logging out of one customer leaves another customer's login alone.
func TestLogoutRemovesOnlyTheSelectedLogin(t *testing.T) {
	bin := binaries(t)
	f := newFake(t)
	home := t.TempDir()
	key := keyFile(t, f)
	runStdin(t, bin, home, key, f, bravoFixture, nil, "auth", "set-token")
	global := findFile(t, home, "credentials.toml")
	if global == "" {
		t.Fatal("set-token saved no credentials file")
	}
	config := filepath.Join(t.TempDir(), "config.toml")
	writePrivate(t, config, "access_token = \""+acmeFixture+"\"\n")

	out, code := runCLIEnv(t, bin, home, key, f, noAccountEnv, "--config", config, "auth", "logout")
	if code != 0 {
		t.Fatalf("auth logout: exit %d\n%s", code, out)
	}
	if _, err := os.Stat(global); err != nil {
		t.Errorf("logging out of the --config login deleted the other customer's login: %v", err)
	}
	if data, _ := os.ReadFile(config); strings.Contains(string(data), acmeFixture) {
		t.Errorf("the selected login is still saved after logout:\n%s", data)
	}
}

// A credential field in a request body is held before anything prints it, however
// the body arrived.
func TestPreviewHoldsRequestCredentialFields(t *testing.T) {
	bin := binaries(t)
	f := newFake(t)
	home := t.TempDir()
	key := keyFile(t, f)
	body := `{"data":{"type":"blueprints","attributes":{"name":"Fleet","password":"` + bodyFixture + `"}}}`
	file := filepath.Join(t.TempDir(), "in.jsonl")
	writePrivate(t, file, body+"\n")
	cases := []struct {
		name  string
		stdin string
		args  []string
	}{
		{"stdin body with the word feedback in a value", body, []string{"blueprints", "create", "--stdin", "--dry-run", "--json", "--data-attributes-name", "feedback"}},
		{"stdin body with the word feedback as an id", body, []string{"blueprints", "update", "feedback", "--stdin", "--dry-run", "--json"}},
		{"a JSON value on a body flag", "", []string{"configurations", "create", "--data-attributes-name", "WiFi", "--data-attributes-type", "CUSTOM_SETTING", "--data-attributes-custom-settings-values", `{"password":"` + bodyFixture + `"}`, "--dry-run"}},
		{"import from stdin", body + "\n", []string{"import", "blueprints", "--input", "-", "--dry-run", "--json"}},
		{"import from a file", "", []string{"import", "blueprints", "--input", file, "--dry-run", "--json"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := runStdin(t, bin, home, key, f, c.stdin, nil, c.args...)
			if strings.Contains(out, bodyFixture) {
				t.Errorf("the preview prints the body's password:\n%s", out)
			}
		})
	}
}

// A password in a saved --deliver URL is held on every later run.
func TestRunProfileWebhookPasswordIsHeld(t *testing.T) {
	bin := binaries(t)
	f := newFake(t)
	home := t.TempDir()
	key := keyFile(t, f)
	hook := "webhook:https://hook:" + hookFixture + "@127.0.0.1:9/h"
	if out, code := runCLI(t, bin, home, key, f, "profile", "save", "p", "--deliver", hook); code != 0 {
		t.Fatalf("profile save: exit %d\n%s", code, out)
	}
	for _, args := range [][]string{{"profile", "show", "p", "--json"}, {"--profile", "p", "devices", "list", "--dry-run", "--json"}} {
		if out, _ := runCLI(t, bin, home, key, f, args...); strings.Contains(out, hookFixture) {
			t.Errorf("%v prints the webhook password:\n%s", args, out)
		}
	}
}

// The key is held whichever way it was given.
func TestPrivateKeyInAnyFormIsHeld(t *testing.T) {
	bin := binaries(t)
	t.Run("a path whose name contains five dashes", func(t *testing.T) {
		f := newFake(t)
		path := filepath.Join(t.TempDir(), "key-----copy.pem")
		writePrivate(t, path, f.key.pem)
		out, _ := runCLI(t, bin, t.TempDir(), path, f, "blueprints", "create", "--data-attributes-name", f.key.lines[0], "--dry-run")
		if strings.Contains(out, f.key.lines[0]) {
			t.Errorf("a line of the key printed; the file behind the path was not held:\n%s", out)
		}
	})
	flat := map[string]func(k testKey) string{
		"flattened with spaces": func(k testKey) string {
			return "-----BEGIN PRIVATE" + " KEY----- " + strings.Join(k.lines, " ") + " -----END PRIVATE KEY-----"
		},
		"flattened into one word": func(k testKey) string {
			return "-----BEGIN PRIVATE" + " KEY-----" + k.body + "-----END PRIVATE KEY-----"
		},
	}
	for name, form := range flat {
		t.Run(name, func(t *testing.T) {
			f := newFake(t)
			home := t.TempDir()
			out, _ := runCLI(t, bin, home, "", f, "auth", "login", "--client-id", clientID, "--client-secret", form(f.key), "--key-id", keyID)
			if strings.Contains(out, f.key.lines[0]) {
				t.Errorf("the output prints the key:\n%s", out)
			}
			for _, piece := range []string{f.key.body, f.key.lines[0]} {
				if p := anyFileHolds(t, home, piece); p != "" {
					t.Errorf("a failed login wrote the key to %s", p)
				}
			}
		})
	}
}

// Logging in with the PEM text itself leaves a store later runs can read.
func TestLoginWithPEMTextKeepsLaterRunsReadable(t *testing.T) {
	bin := binaries(t)
	f := newFake(t)
	home := t.TempDir()
	if out, code := runCLI(t, bin, home, "", f, "auth", "login", "--client-id", clientID, "--client-secret", f.key.pem, "--key-id", keyID); code != 0 {
		t.Fatalf("auth login: exit %d\n%s", code, out)
	}
	for _, args := range [][]string{{"auth", "status", "--json"}, {"doctor", "--json"}, {"sync", "--resources", "devices", "--json"}} {
		out, code := runCLI(t, bin, home, "", f, args...)
		if code != 0 || strings.Contains(out, "withheld") {
			t.Errorf("%v after a PEM-text login: exit %d\n%s", args, code, out)
		}
	}
}

// A read-only MCP tool claims no store.
func TestMCPReadsClaimNoStore(t *testing.T) {
	bin := binaries(t)
	f := newFake(t)
	home := t.TempDir()
	key := keyFile(t, f)
	if out, code := runCLI(t, bin, home, key, f, "sync", "--resources", "devices", "--json"); code != 0 {
		t.Fatalf("sync: exit %d\n%s", code, out)
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(findFirstStore(t, home)), "data-*.db*"))
	if len(matches) == 0 {
		t.Fatal("sync wrote no scoped store")
	}
	dir := filepath.Dir(matches[0])
	for _, m := range matches {
		name := strings.Replace(filepath.Base(m), filepath.Base(strings.SplitN(m, ".db", 2)[0]), "data", 1)
		if err := os.Rename(m, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(filepath.Join(bin, "apple-business-mcp"))
	cmd.Env = env(home, key, f)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	in, _ := cmd.StdinPipe()
	outPipe, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() })
	sc := bufio.NewScanner(outPipe)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	s := &mcpSession{t: t, in: in, out: sc, stderr: stderr}
	s.send("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}})
	_, _ = in.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"))
	answer := s.call("sql", map[string]any{"query": "SELECT 1"})
	if !strings.Contains(answer, `"result"`) {
		t.Fatalf("sql did not answer: %s", answer)
	}
	if _, err := os.Stat(filepath.Join(dir, "data.db.owner")); err == nil {
		t.Errorf("an MCP sql read assigned the unscoped store to this account: %s", answer)
	}
}

// findFirstStore returns the first scoped store under root.
func findFirstStore(t *testing.T, root string) string {
	t.Helper()
	found := ""
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && found == "" && strings.HasPrefix(d.Name(), "data-") && strings.HasSuffix(d.Name(), ".db") {
			found = path
		}
		return nil
	})
	if found == "" {
		t.Fatal("no scoped store under the home")
	}
	return found
}

// A token minted elsewhere is used as given: no key is read, nothing is minted and
// nothing is saved.
func TestAccessTokenFromTheEnvironmentIsUsedNotSaved(t *testing.T) {
	bin := binaries(t)
	f := newFake(t)
	home := t.TempDir()
	const notAKey = "not-a-key-and-not-a-path"
	if out, code := runCLI(t, bin, home, notAKey, f, "devices", "list", "--json"); code == 0 {
		t.Fatalf("control: a read with no usable key and no token must fail:\n%s", out)
	}
	out, code := runCLIEnv(t, bin, home, notAKey, f, []string{"APPLE_BUSINESS_ACCESS_TOKEN=" + envFixture}, "devices", "list", "--json")
	if code != 0 || !strings.Contains(out, "SERIALA001") {
		t.Fatalf("a read with the token in the environment: exit %d\n%s", code, out)
	}
	if n := f.assertionCount(); n != 0 {
		t.Errorf("minted %d token(s) while a token was given", n)
	}
	if strings.Contains(out, envFixture) {
		t.Errorf("the output prints the token:\n%s", out)
	}
	if p := anyFileHolds(t, home, envFixture); p != "" {
		t.Errorf("the token was saved to %s", p)
	}
}

// Every member of a duplicate key is held (contract section 1), so a credential field
// is read from the bytes the operator gave, before any parser keeps only the last one.
func TestDuplicateCredentialKeyIsHeldBeforeAnyParse(t *testing.T) {
	bin := binaries(t)
	f := newFake(t)
	home := t.TempDir()
	key := keyFile(t, f)
	body := `{"password":"` + dupeFixture + `","password":"","data":{"type":"blueprints","attributes":{"name":"` + dupeFixture + `"}}}`
	file := filepath.Join(t.TempDir(), "in.jsonl")
	writePrivate(t, file, body+"\n")
	value := `{"password":"` + dupeFixture + `","password":"","description":"` + dupeFixture + `"}`
	configure := []string{"configurations", "create", "--data-attributes-name", "WiFi", "--data-attributes-type", "CUSTOM_SETTING", "--dry-run"}
	cases := []struct {
		name  string
		stdin string
		args  []string
	}{
		{"import from a file", "", []string{"import", "blueprints", "--input", file, "--dry-run", "--json"}},
		{"import from stdin", body + "\n", []string{"import", "blueprints", "--input", "-", "--dry-run", "--json"}},
		{"stdin body", body, []string{"blueprints", "create", "--stdin", "--dry-run", "--json"}},
		{"a body flag", "", append(configure[:len(configure):len(configure)], "--data-attributes-custom-settings-values", value)},
		{"a body flag with =", "", append(configure[:len(configure):len(configure)], "--data-attributes-custom-settings-values="+value)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if out := runStdin(t, bin, home, key, f, c.stdin, nil, c.args...); strings.Contains(out, dupeFixture) {
				t.Errorf("the first password member printed:\n%s", out)
			}
		})
	}
}

// noAccountEnv blanks the account variables, so a config file selects the account.
var noAccountEnv = []string{"APPLE_BUSINESS_CLIENT_ID=", "APPLE_BUSINESS_PRIVATE_KEY=", "APPLE_BUSINESS_KEY_ID="}

// The shared login goes only when it holds the account this run kept.
func TestLogoutKeepsAnotherAccountsSharedLogin(t *testing.T) {
	bin := binaries(t)
	const otherClient = "BUSINESSAPI.other-customer"
	cases := []struct {
		name   string
		config func(key string) string // the default config.toml, "" for none
		extra  []string
		code   int
		kept   bool
	}{
		{name: "the default config names another account", config: func(key string) string {
			return "business_client_id = \"" + otherClient + "\"\nbusiness_private_key = '" + key + "'\nbusiness_key_id = \"" + keyID + "\"\n"
		}, extra: noAccountEnv, kept: true},
		{name: "control: no selection", extra: noAccountEnv},
		{name: "control: the environment names the saved account"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(t)
			home := t.TempDir()
			key := keyFile(t, f)
			if out, code := runCLI(t, bin, home, key, f, "auth", "login"); code != 0 {
				t.Fatalf("auth login: exit %d\n%s", code, out)
			}
			global := findFile(t, home, "credentials.toml")
			if global == "" {
				t.Fatal("no saved login")
			}
			if c.config != nil {
				dir := filepath.Join(filepath.Dir(filepath.Dir(global)), "config")
				if found := findFile(t, home, "config.toml"); found != "" {
					dir = filepath.Dir(found)
				}
				writePrivate(t, filepath.Join(dir, "config.toml"), c.config(key))
			}
			out, code := runCLIEnv(t, bin, home, key, f, c.extra, "auth", "logout")
			if code != c.code {
				t.Fatalf("auth logout: exit %d\n%s", code, out)
			}
			_, err := os.Stat(global)
			if kept := err == nil; kept != c.kept {
				t.Errorf("shared login still saved = %v, want %v\n%s", kept, c.kept, out)
			}
		})
	}
}

// While the environment selects an identity no saved login holds, auth logout has
// nothing of its own to clear: it changes nothing and says so.
func TestLogoutRefusedWhileTheEnvironmentSelectsAnother(t *testing.T) {
	bin := binaries(t)
	const otherClient = "BUSINESSAPI.other-customer"
	cases := map[string][]string{
		"a given token":   {"APPLE_BUSINESS_ACCESS_TOKEN=" + envFixture},
		"another account": {"APPLE_BUSINESS_CLIENT_ID=" + otherClient},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFake(t)
			home := t.TempDir()
			key := keyFile(t, f)
			if out, code := runCLI(t, bin, home, key, f, "auth", "login"); code != 0 {
				t.Fatalf("auth login: exit %d\n%s", code, out)
			}
			global := findFile(t, home, "credentials.toml")
			before, err := os.ReadFile(global)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" { // mode bits; a store Load refuses is still someone's login
				if err := os.Chmod(global, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			out, code := runCLIEnv(t, bin, home, key, f, extra, "auth", "logout")
			if code == 0 {
				t.Errorf("auth logout reported success:\n%s", out)
			}
			after, err := os.ReadFile(global)
			if err != nil {
				t.Fatalf("the saved login is gone: %v\n%s", err, out)
			}
			if !bytes.Equal(before, after) {
				t.Errorf("the saved login changed:\n%s", out)
			}
		})
	}
}

// A --config file is one customer's whole saved identity: saves stay in it, and it
// never falls back to the shared login, which may be another customer's.
func TestOrgConfigIsItsWholeIdentity(t *testing.T) {
	bin := binaries(t)
	const acmeClient, bravoClient = "BUSINESSAPI.acme-customer", "BUSINESSAPI.bravo-customer"
	org := func(t *testing.T, dir, client, key string) string {
		t.Helper()
		path := filepath.Join(dir, client+".toml")
		writePrivate(t, path, "business_client_id = \""+client+"\"\nbusiness_private_key = '"+key+"'\nbusiness_key_id = \""+keyID+"\"\n")
		return path
	}
	// A run under path, whose credential resolution must not reach the shared login.
	noShared := func(t *testing.T, f *fake, bin, home, key, path string) {
		t.Helper()
		f.bearersSeen()
		minted := f.assertionCount()
		// The fake answers without a credential, as Apple would not: what it is sent is the proof.
		runCLIEnv(t, bin, home, key, f, noAccountEnv, "--config", path, "devices", "list", "--json")
		if got := f.assertionCount(); got != minted {
			t.Errorf("minted %d token(s) from the shared login", got-minted)
		}
		for _, b := range f.bearersSeen() {
			if b != "" {
				t.Errorf("a config with no login sent a credential: %d characters", len(b))
			}
		}
	}
	t.Run("login keeps the account in its own file and out of the shared store", func(t *testing.T) {
		f := newFake(t)
		home := t.TempDir()
		key := keyFile(t, f)
		dir := t.TempDir()
		acme, bravo := org(t, dir, acmeClient, key), org(t, dir, bravoClient, key)
		for _, c := range []struct{ path, client string }{{acme, acmeClient}, {bravo, bravoClient}} {
			out, code := runCLIEnv(t, bin, home, key, f, noAccountEnv, "--config", c.path, "auth", "login", "--client-id", c.client, "--client-secret", key, "--key-id", keyID)
			if code != 0 {
				t.Fatalf("auth login: exit %d\n%s", code, out)
			}
			data, err := os.ReadFile(c.path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), c.client) {
				t.Errorf("%s no longer names its account:\n%s", c.path, data)
			}
			if strings.Contains(string(data), accessToken) {
				t.Errorf("%s holds the minted token", c.path)
			}
		}
		if p := findFile(t, home, "credentials.toml"); p != "" {
			t.Errorf("a login under --config wrote the shared store: %s", p)
		}
		if p := findFile(t, dir, "credentials.toml"); p != "" {
			t.Errorf("a login under --config wrote a shared sibling: %s", p)
		}
		minted := f.assertionCount()
		out, code := runCLIEnv(t, bin, home, key, f, noAccountEnv, "--config", acme, "devices", "list", "--json")
		if code != 0 || !strings.Contains(out, "SERIALA001") {
			t.Errorf("the config no longer works: exit %d\n%s", code, out)
		}
		if f.assertionCount() != minted+1 {
			t.Errorf("a run under --config did not mint from its own key")
		}
	})
	t.Run("logout clears only its own file", func(t *testing.T) {
		f := newFake(t)
		home := t.TempDir()
		key := keyFile(t, f)
		if out, code := runCLI(t, bin, home, key, f, "auth", "login"); code != 0 {
			t.Fatalf("auth login: exit %d\n%s", code, out)
		}
		global := findFile(t, home, "credentials.toml")
		saved, err := os.ReadFile(global)
		if err != nil {
			t.Fatal(err)
		}
		bravo := org(t, t.TempDir(), bravoClient, key)
		if out, code := runCLIEnv(t, bin, home, key, f, noAccountEnv, "--config", bravo, "auth", "logout"); code != 0 {
			t.Fatalf("auth logout: exit %d\n%s", code, out)
		}
		now, err := os.ReadFile(global)
		if err != nil || !bytes.Equal(saved, now) {
			t.Errorf("logging out of a --config changed the shared login: %v", err)
		}
		noShared(t, f, bin, home, key, bravo)
	})
	t.Run("the shared store under the same home is not this config's", func(t *testing.T) {
		f := newFake(t)
		root := t.TempDir()
		key := keyFile(t, f)
		extra := []string{"APPLE_BUSINESS_HOME=" + root}
		if out, code := runCLIEnv(t, bin, root, key, f, extra, "auth", "login"); code != 0 {
			t.Fatalf("auth login: exit %d\n%s", code, out)
		}
		global := findFile(t, root, "credentials.toml")
		bravo := org(t, root, bravoClient, key)
		if out, code := runCLIEnv(t, bin, root, key, f, append(extra[:1:1], noAccountEnv...), "--config", bravo, "auth", "logout"); code != 0 {
			t.Fatalf("auth logout: exit %d\n%s", code, out)
		}
		if _, err := os.Stat(global); err != nil {
			t.Errorf("the shared login is gone: %v", err)
		}
	})
	t.Run("set-token saves to its own file", func(t *testing.T) {
		f := newFake(t)
		home := t.TempDir()
		key := keyFile(t, f)
		if out, code := runCLI(t, bin, home, key, f, "auth", "login"); code != 0 {
			t.Fatalf("auth login: exit %d\n%s", code, out)
		}
		global := findFile(t, home, "credentials.toml")
		saved, err := os.ReadFile(global)
		if err != nil {
			t.Fatal(err)
		}
		bravo := org(t, t.TempDir(), bravoClient, key)
		runStdin(t, bin, home, key, f, bravoFixture, noAccountEnv, "--config", bravo, "auth", "set-token")
		if now, err := os.ReadFile(global); err != nil || !bytes.Equal(saved, now) {
			t.Errorf("set-token under --config changed the shared login: %v", err)
		}
		f.bearersSeen()
		out, code := runCLIEnv(t, bin, home, key, f, noAccountEnv, "--config", bravo, "devices", "list", "--json")
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, out)
		}
		for _, b := range f.bearersSeen() {
			if b != "Bearer "+bravoFixture {
				t.Errorf("sent another credential, %d characters", len(b))
			}
		}
	})
	t.Run("a config that never had a login stays without one", func(t *testing.T) {
		f := newFake(t)
		home := t.TempDir()
		key := keyFile(t, f)
		if out, code := runCLI(t, bin, home, key, f, "auth", "login"); code != 0 {
			t.Fatalf("auth login: exit %d\n%s", code, out)
		}
		empty := filepath.Join(t.TempDir(), "empty.toml")
		writePrivate(t, empty, "\n")
		noShared(t, f, bin, home, key, empty)
	})
}

// A saved profile's base-url is a credential source, every duplicate member included.
func TestProfileStoreHoldsEveryDuplicateMember(t *testing.T) {
	bin := binaries(t)
	f := newFake(t)
	home := t.TempDir()
	key := keyFile(t, f)
	store := `{"profiles":{"p":{"name":"p","description":"` + dupeFixture + `","values":{"base-url":"https://api-business.apple.com/v1?token=` + dupeFixture + `","base-url":""}}}}`
	extra := []string{"APPLE_BUSINESS_HOME=" + home}
	if out, code := runCLIEnv(t, bin, home, key, f, extra, "profile", "save", "seed", "--deliver", "webhook:https://127.0.0.1:9/h"); code != 0 {
		t.Fatalf("profile save: exit %d\n%s", code, out)
	}
	path := findFile(t, home, "profiles.json")
	if path == "" {
		t.Fatal("no profile store")
	}
	writePrivate(t, path, store)
	if out, _ := runCLIEnv(t, bin, home, key, f, extra, "profile", "list", "--json"); strings.Contains(out, dupeFixture) {
		t.Errorf("the first base-url member's credential printed:\n%s", out)
	}
}

func dataStores(t *testing.T, home string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "data") && strings.HasSuffix(d.Name(), ".db") {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// APPLE_BUSINESS_ACCESS_TOKEN is the run's whole identity: nothing a config or a saved
// login names may send in its place, and its inventory never lands in their store.
func TestEnvAccessTokenIsTheWholeIdentity(t *testing.T) {
	bin := binaries(t)
	token := []string{"APPLE_BUSINESS_ACCESS_TOKEN=" + envFixture}
	want := "Bearer " + envFixture
	onlyToken := func(t *testing.T, f *fake, out string) {
		t.Helper()
		seen := f.bearersSeen()
		if len(seen) == 0 {
			t.Fatalf("no API request was sent:\n%s", out)
		}
		for _, b := range seen {
			if b != want {
				t.Errorf("an API request carried another account's credential, not the given token: %d characters", len(b))
			}
		}
	}
	for name, config := range map[string]string{
		"auth_header":           "auth_header = \"Bearer " + acmeFixture + "\"\n",
		"[headers]":             "[headers]\nAuthorization = \"Bearer " + acmeFixture + "\"\n",
		"auth_header + account": "business_client_id = \"" + clientID + "\"\nauth_header = \"Bearer " + acmeFixture + "\"\n",
	} {
		t.Run("config "+name, func(t *testing.T) {
			f := newFake(t)
			path := filepath.Join(t.TempDir(), "config.toml")
			writePrivate(t, path, config)
			extra := append(token[:1:1], "APPLE_BUSINESS_CLIENT_ID=", "APPLE_BUSINESS_PRIVATE_KEY=", "APPLE_BUSINESS_KEY_ID=")
			out, code := runCLIEnv(t, bin, t.TempDir(), keyFile(t, f), f, extra, "--config", path, "devices", "list", "--json")
			if code != 0 {
				t.Fatalf("exit %d\n%s", code, out)
			}
			onlyToken(t, f, out)
		})
	}
	t.Run("a saved login keeps its store", func(t *testing.T) {
		f := newFake(t)
		home := t.TempDir()
		key := keyFile(t, f)
		if out, code := runCLI(t, bin, home, key, f, "auth", "login"); code != 0 {
			t.Fatalf("auth login: exit %d\n%s", code, out)
		}
		if out, code := runCLI(t, bin, home, key, f, "sync"); code != 0 {
			t.Fatalf("sync as the saved login: exit %d\n%s", code, out)
		}
		saved := dataStores(t, home)
		if len(saved) != 1 {
			t.Fatalf("stores after the saved login's sync: %v", saved)
		}
		f.bearersSeen()
		out, code := runCLIEnv(t, bin, home, key, f, token, "sync")
		if code != 0 {
			t.Fatalf("sync with the token: exit %d\n%s", code, out)
		}
		onlyToken(t, f, out)
		if got := dataStores(t, home); len(got) != 2 {
			t.Errorf("the given token's inventory shares the saved login's store: %v", got)
		}
	})
}
