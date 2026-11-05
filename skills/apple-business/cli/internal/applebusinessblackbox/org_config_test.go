// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package applebusinessblackbox

// What a save under --config keeps, what auth logout may remove, and what auth status
// says about a saved account, each driven on the built binary. Seen failing on the code
// they fix: .review/more/tests3-before.log.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fileRoutes blanks the harness's routing, so only the config file names the hosts.
var fileRoutes = append(append([]string{}, noAccountEnv...),
	"APPLE_BUSINESS_BASE_URL=", "APPLE_BUSINESS_TOKEN_URL=", "APPLE_BUSINESS_CA_CERT=")

func routedOrg(t *testing.T, f *fake, identity string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "acme.toml")
	writePrivate(t, path, "base_url = \""+f.srv.URL+"/v1\"\ntoken_url = \""+f.srv.URL+"/auth/oauth2/token\"\nca_cert = \""+
		strings.ReplaceAll(f.caFile, `\`, `\\`)+"\"\ntls_server_name = \"127.0.0.1\"\n"+identity+"\n[headers]\nX-Org-Tag = \"acme-gateway\"\n")
	return path
}

func mustContain(t *testing.T, path string, want ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range want {
		if !strings.Contains(string(data), w) {
			t.Errorf("%s lost %q:\n%s", filepath.Base(path), w, data)
		}
	}
}

// Every save rewrites the org file, and the file is that customer's whole setup: a save
// that drops its routing sends the next run, and its credential, to the default host.
func TestOrgConfigSaveKeepsItsRouting(t *testing.T) {
	bin := binaries(t)
	const account = "BUSINESSAPI.acme-customer"
	routing := func(f *fake) []string {
		return []string{"base_url = '" + f.srv.URL + "/v1'", "token_url = '" + f.srv.URL + "/auth/oauth2/token'", "ca_cert", "tls_server_name", "X-Org-Tag"}
	}
	t.Run("auth login", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		org := routedOrg(t, f, "")
		out, code := runCLIEnv(t, bin, home, key, f, fileRoutes, "--config", org, "auth", "login", "--client-id", account, "--client-secret", key, "--key-id", keyID)
		if code != 0 {
			t.Fatalf("auth login: exit %d\n%s", code, out)
		}
		mustContain(t, org, routing(f)...)
		mustContain(t, org, account)
		out, code = runCLIEnv(t, bin, home, key, f, fileRoutes, "--config", org, "blueprints", "list", "--dry-run")
		if code != 0 || !strings.Contains(out, f.srv.URL) {
			t.Errorf("the next run left the file's host: exit %d\n%s", code, out)
		}
	})
	t.Run("auth set-token", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		org := routedOrg(t, f, "")
		runStdin(t, bin, home, key, f, acmeFixture, fileRoutes, "--config", org, "auth", "set-token")
		mustContain(t, org, routing(f)...)
		mustContain(t, org, acmeFixture)
	})
	t.Run("auth logout", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		org := routedOrg(t, f, "business_client_id = \""+account+"\"\nbusiness_private_key = '"+key+"'\nbusiness_key_id = \""+keyID+"\"")
		if out, code := runCLIEnv(t, bin, home, key, f, fileRoutes, "--config", org, "auth", "logout"); code != 0 {
			t.Fatalf("auth logout: exit %d\n%s", code, out)
		}
		mustContain(t, org, routing(f)...)
		data, _ := os.ReadFile(org)
		if strings.Contains(string(data), account) || strings.Contains(string(data), "business_private_key") {
			t.Errorf("logout left the account in the file:\n%s", data)
		}
	})
	t.Run("a route from the environment is not saved", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		org := routedOrg(t, f, "")
		const other = "https://gateway.invalid"
		extra := append(append([]string{}, noAccountEnv...), "APPLE_BUSINESS_BASE_URL="+other+"/v1", "APPLE_BUSINESS_TOKEN_URL="+other+"/token")
		runStdin(t, bin, home, key, f, acmeFixture, extra, "--config", org, "auth", "set-token")
		mustContain(t, org, routing(f)...)
		if data, _ := os.ReadFile(org); strings.Contains(string(data), other) {
			t.Errorf("the file now names the environment's host:\n%s", data)
		}
	})
	t.Run("set-token drops an Authorization header that would outrank the token", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		org := filepath.Join(t.TempDir(), "acme.toml")
		writePrivate(t, org, "[headers]\nAuthorization = \"Bearer "+bravoFixture+"\"\nX-Org-Tag = \"acme-gateway\"\n")
		runStdin(t, bin, home, key, f, acmeFixture, noAccountEnv, "--config", org, "auth", "set-token")
		mustContain(t, org, acmeFixture, "X-Org-Tag")
		if data, _ := os.ReadFile(org); strings.Contains(string(data), "Authorization") {
			t.Errorf("the header is still saved:\n%s", data)
		}
	})
	t.Run("logout drops an Authorization header and keeps the rest", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		org := filepath.Join(t.TempDir(), "acme.toml")
		writePrivate(t, org, "[headers]\nAuthorization = \"Bearer "+acmeFixture+"\"\nX-Org-Tag = \"acme-gateway\"\n")
		if out, code := runCLIEnv(t, bin, home, key, f, noAccountEnv, "--config", org, "auth", "logout"); code != 0 {
			t.Fatalf("auth logout: exit %d\n%s", code, out)
		}
		mustContain(t, org, "X-Org-Tag")
		f.bearersSeen()
		runCLIEnv(t, bin, home, key, f, noAccountEnv, "--config", org, "devices", "list", "--json")
		for _, b := range f.bearersSeen() {
			if b != "" {
				t.Errorf("a logged-out file still sent a credential: %d characters", len(b))
			}
		}
	})
}

// A file other users can change does not get to choose where a credential goes, and
// rewriting it 0600 would promote what it names to trusted. The save refuses and says how
// to proceed; a loose file that names no routing is simply tightened.
func TestLooseOrgConfigIsNotPromotedBySave(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits")
	}
	bin := binaries(t)
	const foreign = "https://gateway.invalid"
	for name, args := range map[string][]string{
		"auth login":  {"auth", "login", "--client-id", "BUSINESSAPI.acme-customer", "--client-secret", "%KEY%", "--key-id", keyID},
		"auth logout": {"auth", "logout"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake(t)
			home, key := t.TempDir(), keyFile(t, f)
			org := filepath.Join(t.TempDir(), "acme.toml")
			before := []byte("base_url = \"" + foreign + "/v1\"\ntoken_url = \"" + foreign + "/token\"\n")
			if err := os.WriteFile(org, before, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(org, 0o644); err != nil {
				t.Fatal(err)
			}
			argv := []string{"--config", org}
			for _, a := range args {
				argv = append(argv, strings.ReplaceAll(a, "%KEY%", key))
			}
			out, code := runCLIEnv(t, bin, home, key, f, fileRoutes, argv...)
			if f.assertionCount() != 0 {
				t.Errorf("minted before refusing")
			}
			if code == 0 {
				t.Errorf("the save reported success:\n%s", out)
			}
			after, err := os.ReadFile(org)
			if err != nil {
				t.Fatal(err)
			}
			info, _ := os.Stat(org)
			if !bytes.Equal(before, after) || info.Mode().Perm() != 0o644 {
				t.Errorf("the loose file changed (mode %o):\n%s\n%s", info.Mode().Perm(), after, out)
			}
			if !strings.Contains(out, "chmod 600") {
				t.Errorf("the refusal does not say what to do:\n%s", out)
			}
		})
	}
	t.Run("control: a loose file naming no routing is tightened", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		org := filepath.Join(t.TempDir(), "acme.toml")
		if err := os.WriteFile(org, []byte("business_key_id = \""+keyID+"\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(org, 0o644); err != nil {
			t.Fatal(err)
		}
		if out, code := runCLIEnv(t, bin, home, key, f, noAccountEnv, "--config", org, "auth", "logout"); code != 0 {
			t.Fatalf("auth logout: exit %d\n%s", code, out)
		}
		if info, _ := os.Stat(org); info.Mode().Perm() != 0o600 {
			t.Errorf("mode %o", info.Mode().Perm())
		}
	})
}

// An account with its key and key ID is configured: every data command mints from it, so
// a status that says otherwise is a false red on a working identity.
func TestStatusSeesASavedAccount(t *testing.T) {
	bin := binaries(t)
	const account = "BUSINESSAPI.acme-customer"
	loggedIn := func(t *testing.T, f *fake, home, key string) []string {
		t.Helper()
		org := filepath.Join(t.TempDir(), "acme.toml")
		if out, code := runCLIEnv(t, bin, home, key, f, noAccountEnv, "--config", org, "auth", "login", "--client-id", account, "--client-secret", key, "--key-id", keyID); code != 0 {
			t.Fatalf("auth login: exit %d\n%s", code, out)
		}
		return []string{"--config", org}
	}
	check := func(t *testing.T, f *fake, home, key string, extra, pre []string) {
		t.Helper()
		status := func(args ...string) (string, int) {
			return runCLIEnv(t, bin, home, key, f, extra, append(append(append([]string{}, pre...), "auth", "status"), args...)...)
		}
		if out, code := status(); code != 0 || strings.Contains(out, "Not authenticated") {
			t.Errorf("auth status: exit %d\n%s", code, out)
		}
		out, code := status("--json")
		var got struct {
			Authenticated bool `json:"authenticated"`
		}
		if err := json.Unmarshal([]byte(out), &got); code != 0 || err != nil || !got.Authenticated {
			t.Errorf("auth status --json: exit %d\n%s", code, out)
		}
		// Doctor's exit also judges its probe path, which the fake does not serve.
		out, _ = runCLIEnv(t, bin, home, key, f, extra, append(append([]string{}, pre...), "doctor")...)
		if strings.Contains(out, "not configured") || strings.Contains(out, "missing required") || !strings.Contains(out, "OK Auth") {
			t.Errorf("doctor:\n%s", out)
		}
	}
	t.Run("after a login under --config", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		check(t, f, home, key, noAccountEnv, loggedIn(t, f, home, key))
	})
	t.Run("under the three account variables", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		check(t, f, home, key, nil, nil)
	})
	t.Run("control: an account with no key is not", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		out, code := runCLIEnv(t, bin, home, key, f, []string{"APPLE_BUSINESS_PRIVATE_KEY="}, "auth", "status")
		if code == 0 || !strings.Contains(out, "Not authenticated") {
			t.Errorf("auth status: exit %d\n%s", code, out)
		}
	})
}

// A default config's own token means Load never read the shared login, so a login naming
// an account is not this run's to remove. A bare token names nobody: it goes with the
// config's, as the template's legacy migration expects.
func TestLogoutKeepsASharedLoginThisRunNeverUsed(t *testing.T) {
	bin := binaries(t)
	defaultConfig := func(t *testing.T, home string) string {
		t.Helper()
		conf := findFile(t, home, "config.toml")
		if conf == "" {
			t.Fatal("no default config saved")
		}
		return conf
	}
	saved := map[string]func(t *testing.T, f *fake, home, key string){
		"a login": func(t *testing.T, f *fake, home, key string) {
			if out, code := runCLI(t, bin, home, key, f, "auth", "login"); code != 0 {
				t.Fatalf("auth login: exit %d\n%s", code, out)
			}
		},
		"a bare token": func(t *testing.T, f *fake, home, key string) {
			runStdin(t, bin, home, key, f, bravoFixture, noAccountEnv, "auth", "set-token")
		},
	}
	for name, save := range saved {
		kept := name == "a login"
		t.Run("the default config's token, the shared store holds "+name, func(t *testing.T) {
			f := newFake(t)
			home, key := t.TempDir(), keyFile(t, f)
			save(t, f, home, key)
			global := findFile(t, home, "credentials.toml")
			before, err := os.ReadFile(global)
			if err != nil {
				t.Fatal(err)
			}
			conf := defaultConfig(t, home)
			writePrivate(t, conf, "access_token = \""+acmeFixture+"\"\n")
			if out, code := runCLIEnv(t, bin, home, key, f, noAccountEnv, "auth", "logout"); code != 0 {
				t.Fatalf("auth logout: exit %d\n%s", code, out)
			}
			after, err := os.ReadFile(global)
			if kept && (err != nil || !bytes.Equal(before, after)) {
				t.Errorf("the shared login is gone or changed: %v", err)
			}
			if !kept && err == nil {
				t.Errorf("the bare token is still saved")
			}
			if data, _ := os.ReadFile(conf); strings.Contains(string(data), acmeFixture) {
				t.Errorf("the default config still holds its token:\n%s", data)
			}
		})
		t.Run("control: with no other identity logout removes "+name, func(t *testing.T) {
			f := newFake(t)
			home, key := t.TempDir(), keyFile(t, f)
			save(t, f, home, key)
			global := findFile(t, home, "credentials.toml")
			if out, code := runCLIEnv(t, bin, home, key, f, noAccountEnv, "auth", "logout"); code != 0 {
				t.Fatalf("auth logout: exit %d\n%s", code, out)
			}
			if _, err := os.Stat(global); err == nil {
				t.Errorf("the shared login is still saved")
			}
		})
	}
}

// The environment's selection stands however Load treated it: a route it blocked marks
// nothing, and a file holding a pasted token names no account to compare.
func TestLogoutRefusalReadsTheEnvironmentItself(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits")
	}
	bin := binaries(t)
	const otherClient = "BUSINESSAPI.other-customer"
	blocked := append(append([]string{}, noAccountEnv...), "APPLE_BUSINESS_BASE_URL=", "APPLE_BUSINESS_TOKEN_URL=")
	for name, extra := range map[string][]string{
		"another account": append(append([]string{}, blocked...), "APPLE_BUSINESS_CLIENT_ID="+otherClient),
		"a given token":   append(append([]string{}, blocked...), "APPLE_BUSINESS_ACCESS_TOKEN="+envFixture),
	} {
		t.Run("a loose default config names a foreign host, the environment gives "+name, func(t *testing.T) {
			f := newFake(t)
			home, key := t.TempDir(), keyFile(t, f)
			if out, code := runCLI(t, bin, home, key, f, "auth", "login"); code != 0 {
				t.Fatalf("auth login: exit %d\n%s", code, out)
			}
			global := findFile(t, home, "credentials.toml")
			conf := findFile(t, home, "config.toml")
			if err := os.WriteFile(conf, []byte("base_url = \"https://gateway.invalid/v1\"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(conf, 0o644); err != nil {
				t.Fatal(err)
			}
			out, code := runCLIEnv(t, bin, home, key, f, extra, "auth", "logout")
			if code == 0 {
				t.Errorf("auth logout reported success:\n%s", out)
			}
			if _, err := os.Stat(global); err != nil {
				t.Errorf("the saved login is gone: %v\n%s", err, out)
			}
		})
	}
	t.Run("an org file's pasted token under another account", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		org := filepath.Join(t.TempDir(), "p.toml")
		runStdin(t, bin, home, key, f, acmeFixture, noAccountEnv, "--config", org, "auth", "set-token")
		before, err := os.ReadFile(org)
		if err != nil {
			t.Fatal(err)
		}
		out, code := runCLIEnv(t, bin, home, key, f, []string{"APPLE_BUSINESS_CLIENT_ID=" + otherClient}, "--config", org, "auth", "logout")
		if code == 0 {
			t.Errorf("auth logout reported success:\n%s", out)
		}
		if after, _ := os.ReadFile(org); !bytes.Equal(before, after) {
			t.Errorf("the org file changed:\n%s", out)
		}
	})
}

// A default config other users can change gets the same refusal as an org file: rewriting
// it 0600 would promote its hosts to trusted, and the next run's bearer would go there.
func TestLooseDefaultConfigIsNotPromotedBySave(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits")
	}
	bin := binaries(t)
	const foreign = "https://gateway.invalid"
	loose := func(t *testing.T, home, content string) string {
		t.Helper()
		conf := filepath.Join(home, ".config", "apple-business-cli", "config.toml")
		if err := os.MkdirAll(filepath.Dir(conf), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(conf, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(conf, 0o644); err != nil {
			t.Fatal(err)
		}
		return conf
	}
	unchanged := func(t *testing.T, conf, content, out string) {
		t.Helper()
		data, err := os.ReadFile(conf)
		if err != nil {
			t.Fatal(err)
		}
		info, _ := os.Stat(conf)
		if string(data) != content || info.Mode().Perm() != 0o644 {
			t.Errorf("the loose file changed (mode %o):\n%s\n%s", info.Mode().Perm(), data, out)
		}
	}
	content := "base_url = \"" + foreign + "/v1\"\n"
	t.Run("auth set-token", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		conf := loose(t, home, content)
		out := runStdin(t, bin, home, key, f, acmeFixture, fileRoutes, "auth", "set-token")
		unchanged(t, conf, content, out)
		if p := anyFileHolds(t, home, acmeFixture); p != "" {
			t.Errorf("the token was saved in %s", p)
		}
	})
	t.Run("auth login under a route from the environment", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		conf := loose(t, home, content)
		out, code := runCLI(t, bin, home, key, f, "auth", "login")
		if code == 0 {
			t.Errorf("the save reported success:\n%s", out)
		}
		unchanged(t, conf, content, out)
	})
	t.Run("auth logout keeps the shared login too", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		if out, code := runCLI(t, bin, home, key, f, "auth", "login"); code != 0 {
			t.Fatalf("auth login: exit %d\n%s", code, out)
		}
		global := findFile(t, home, "credentials.toml")
		conf := loose(t, home, content)
		out, code := runCLIEnv(t, bin, home, key, f, fileRoutes, "auth", "logout")
		if code == 0 {
			t.Errorf("the save reported success:\n%s", out)
		}
		unchanged(t, conf, content, out)
		if _, err := os.Stat(global); err != nil {
			t.Errorf("the shared login is gone: %v", err)
		}
	})
	t.Run("an ordinary read that mints still works and writes nothing", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		if out, code := runCLI(t, bin, home, key, f, "auth", "login"); code != 0 {
			t.Fatalf("auth login: exit %d\n%s", code, out)
		}
		global := findFile(t, home, "credentials.toml")
		data, err := os.ReadFile(global)
		if err != nil {
			t.Fatal(err)
		}
		var kept []string
		for _, line := range strings.Split(string(data), "\n") {
			if !strings.HasPrefix(line, "access_token") && !strings.HasPrefix(line, "token_expiry") {
				kept = append(kept, line)
			}
		}
		writePrivate(t, global, strings.Join(kept, "\n"))
		const tagged = "[headers]\nX-Org-Tag = \"acme-gateway\"\n"
		conf := loose(t, home, tagged)
		minted := f.assertionCount()
		out, code := runCLIEnv(t, bin, home, key, f, noAccountEnv, "devices", "list", "--json")
		if code != 0 || f.assertionCount() != minted+1 {
			t.Errorf("exit %d, %d mints\n%s", code, f.assertionCount()-minted, out)
		}
		unchanged(t, conf, tagged, out)
	})
}

// A per-customer home is how an MSP separates tenants: a fresh one has no identity, and
// nothing done under it touches the default config's.
func TestRelocatedHomeIgnoresTheDefaultConfig(t *testing.T) {
	bin := binaries(t)
	const acme = "BUSINESSAPI.acme-customer"
	for name, relocate := range map[string]func(dir string) ([]string, []string){
		"--home":              func(dir string) ([]string, []string) { return nil, []string{"--home", dir} },
		"APPLE_BUSINESS_HOME": func(dir string) ([]string, []string) { return []string{"APPLE_BUSINESS_HOME=" + dir}, nil },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake(t)
			home, key := t.TempDir(), keyFile(t, f)
			conf := filepath.Join(home, ".config", "apple-business-cli", "config.toml")
			content := "business_client_id = \"" + acme + "\"\nbusiness_private_key = \"" + strings.ReplaceAll(key, `\`, `\\`) + "\"\nbusiness_key_id = \"" + keyID + "\"\n"
			writePrivate(t, conf, content)
			extra, pre := relocate(filepath.Join(t.TempDir(), "bravo"))
			extra = append(append([]string{}, noAccountEnv...), extra...)
			if out, code := runCLIEnv(t, bin, home, key, f, extra, append(append([]string{}, pre...), "auth", "status")...); code == 0 {
				t.Errorf("a fresh home acts as the default config's account:\n%s", out)
			}
			minted := f.assertionCount()
			runCLIEnv(t, bin, home, key, f, extra, append(append([]string{}, pre...), "devices", "list", "--json")...)
			if f.assertionCount() != minted {
				t.Errorf("a fresh home minted from the default config's key")
			}
			if out, code := runCLIEnv(t, bin, home, key, f, extra, append(append([]string{}, pre...), "auth", "logout")...); code != 0 {
				t.Fatalf("auth logout: exit %d\n%s", code, out)
			}
			if data, _ := os.ReadFile(conf); string(data) != content {
				t.Errorf("the default config changed:\n%s", data)
			}
		})
	}
}

// A saved login that cannot be read cannot be shown to be this run's, so it stays.
func TestLogoutKeepsAStoreItCannotRead(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs mode bits that bind")
	}
	bin := binaries(t)
	for name, extra := range map[string][]string{
		"no selection":    noAccountEnv,
		"another account": {"APPLE_BUSINESS_CLIENT_ID=BUSINESSAPI.other-customer"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake(t)
			home, key := t.TempDir(), keyFile(t, f)
			if out, code := runCLI(t, bin, home, key, f, "auth", "login"); code != 0 {
				t.Fatalf("auth login: exit %d\n%s", code, out)
			}
			global := findFile(t, home, "credentials.toml")
			if err := os.Chmod(global, 0o000); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(global, 0o600) })
			out, code := runCLIEnv(t, bin, home, key, f, extra, "auth", "logout")
			if code == 0 {
				t.Errorf("auth logout reported success:\n%s", out)
			}
			if _, err := os.Stat(global); err != nil {
				t.Errorf("the saved login is gone: %v\n%s", err, out)
			}
		})
	}
}
