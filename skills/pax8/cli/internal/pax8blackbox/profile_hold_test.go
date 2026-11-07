// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package pax8blackbox

// A saved run profile is a credential source (credential leak contract section
// 1; handfixes.json credkit-wiring): a value under a credential's name or under
// base-url is held before anything prints, every member of a duplicate key. The
// same store's ordinary values, its names and its free text still print, or a
// profile named "p" or a saved --limit 7 would refuse every later start.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// profileStoreOf saves a profile through the binary and answers where the store
// landed, so the test does not repeat the program's path rules.
func profileStoreOf(t *testing.T, env []string, root string, flags ...string) string {
	t.Helper()
	runWith(t, env, append([]string{"profile", "save", "seed", "--compact"}, flags...)...)
	found := ""
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "profiles.json" {
			found = path
		}
		return nil
	})
	if found == "" {
		t.Fatalf("no profiles.json under %s", root)
	}
	return found
}

func oneProfile(description, values string) string {
	return `{"profiles":{"p":{"name":"p","description":"` + description + `","values":{` + values + `}}}}`
}

func TestSavedProfileCredentialsNeverPrint(t *testing.T) {
	const (
		urlPw   = "ProfilePw7xProfilePw7x" // gitleaks:allow
		queryV  = "ProfileQuery7xProfile"  // gitleaks:allow
		firstPw = "DupFirstPw7xDupFirst"   // gitleaks:allow
		lastPw  = "DupLastPw7xDupLast"     // gitleaks:allow
		named   = "NamedValue7xNamedValue" // gitleaks:allow
	)
	type tc struct {
		description, values string
		secrets             []string
	}
	cases := map[string]tc{
		"base-url password":   {"", `"base-url":"https://u:` + urlPw + `@api.pax8.com/v1"`, []string{urlPw}},
		"base-url query":      {"", `"base-url":"https://api.pax8.com/v1?k=` + queryV + `"`, []string{queryV}},
		"base_url spelling":   {"", `"base_url":"https://u:` + urlPw + `@api.pax8.com/v1"`, []string{urlPw}},
		"Base-URL case":       {"", `"Base-URL":"https://u:` + urlPw + `@api.pax8.com/v1"`, []string{urlPw}},
		"schemeless base-url": {"", `"base-url":"u:` + urlPw + `@api.pax8.com:443"`, []string{urlPw}},
		"unparseable url":     {"", `"base-url":"https://u:` + urlPw + `@bad host/%zz"`, []string{urlPw}},
		"token_url password":  {"", `"token_url":"https://u:` + urlPw + `@token-manager.pax8.com/token"`, []string{urlPw}},
		// The decoder keeps only the last member, so the first one reaches an
		// output only where something else carries it: the description here.
		"first of a duplicate base-url": {firstPw, `"base-url":"https://u:` + firstPw + `@api.pax8.com/v1","base-url":"https://u:` + lastPw + `@api.pax8.com/v1"`, []string{firstPw, lastPw}},
		"first of a duplicate name":     {firstPw, `"api-key":"` + firstPw + `","api-key":"` + lastPw + `"`, []string{firstPw, lastPw}},
		"client-id":                     {"", `"client-id":"` + named + `"`, []string{named}},
		"client-secret":                 {"", `"client-secret":"` + named + `"`, []string{named}},
		"client_secret":                 {"", `"client_secret":"` + named + `"`, []string{named}},
		"api-key":                       {"", `"api-key":"` + named + `"`, []string{named}},
		"password":                      {"", `"password":"` + named + `"`, []string{named}},
		"pwd":                           {"", `"pwd":"` + named + `"`, []string{named}},
		"token":                         {"", `"token":"` + named + `"`, []string{named}},
		"authorization":                 {"", `"authorization":"Bearer ` + named + `"`, []string{named}},
		"bearer":                        {"", `"bearer":"` + named + `"`, []string{named}},
		"cookie":                        {"", `"cookie":"sid=` + named + `"`, []string{named}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			env := cleanEnv(home)
			store := profileStoreOf(t, env, home)
			if err := os.WriteFile(store, []byte(oneProfile(c.description, c.values)), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{{"profile", "show", "p"}, {"profile", "use", "p"}, {"profile", "list"}} {
				out := runWith(t, env, args...)
				for _, s := range c.secrets {
					if strings.Contains(out, s) {
						t.Errorf("%v printed %s:\n%s", args, s, out)
					}
				}
			}
			if out := runWith(t, env, "profile", "show", "p"); !strings.Contains(out, kitNotice("stdout")) {
				t.Errorf("profile show did not withhold:\n%s", out)
			}
		})
	}
}

// Every store a run could mean: under --home, and the legacy path.
func TestSavedProfileCredentialsHeldInEveryStore(t *testing.T) {
	const pw = "ProfilePw7xProfilePw7x" // gitleaks:allow
	body := oneProfile("", `"base-url":"https://u:`+pw+`@api.pax8.com/v1"`)
	check := func(t *testing.T, env []string, args ...string) {
		t.Helper()
		out := runWith(t, env, append([]string{"profile", "show", "p"}, args...)...)
		if strings.Contains(out, pw) {
			t.Fatalf("printed the password:\n%s", out)
		}
		if !strings.Contains(out, kitNotice("stdout")) {
			t.Fatalf("read no profile, so this proves nothing:\n%s", out)
		}
	}
	t.Run("home flag", func(t *testing.T) {
		home, other := t.TempDir(), t.TempDir()
		env := cleanEnv(home)
		store := profileStoreOf(t, env, other, "--home", other)
		if err := os.WriteFile(store, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		check(t, env, "--home", other)
	})
	t.Run("home environment", func(t *testing.T) {
		home, other := t.TempDir(), t.TempDir()
		env := cleanEnv(home, "PAX8_HOME="+other)
		store := profileStoreOf(t, env, other)
		if err := os.WriteFile(store, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		check(t, env)
	})
	t.Run("legacy path", func(t *testing.T) {
		home := t.TempDir()
		legacy := filepath.Join(home, ".pax8-cli", "profiles.json")
		if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(legacy, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		check(t, cleanEnv(home))
	})
}

// The other direction: nothing here is a credential, so all of it still prints.
func TestSavedProfileOrdinaryValuesStillPrint(t *testing.T) {
	home := t.TempDir()
	env := cleanEnv(home)
	store := profileStoreOf(t, env, home)
	body := `{"profiles":{"p":{"name":"p","description":"nightly billing run","values":{"limit":"7","select":"id,name","json":"true","base-url":"https://api.pax8.com/v1"}},` +
		`"token-rotation":{"name":"token-rotation","description":"a name, not a credential","values":{"compact":"true"}}}}`
	if err := os.WriteFile(store, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out := runWith(t, env, "profile", "show", "p")
	if strings.Contains(out, "withheld") {
		t.Fatalf("an ordinary profile was withheld:\n%s", out)
	}
	for _, want := range []string{"nightly billing run", `"limit": "7"`, `"select": "id,name"`, "https://api.pax8.com/v1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("profile show lost %q:\n%s", want, out)
		}
	}
	if out := runWith(t, env, "profile", "list"); !strings.Contains(out, "token-rotation") || strings.Contains(out, "withheld") {
		t.Fatalf("profile list:\n%s", out)
	}
	if out := runWith(t, env, "version"); strings.Contains(out, "withheld") {
		t.Fatalf("version was withheld:\n%s", out)
	}
}

// A store that does not parse: the program refuses it and prints none of it.
func TestSavedProfileStoreThatDoesNotParseStaysHeld(t *testing.T) {
	const pw = "ProfilePw7xProfilePw7x" // gitleaks:allow
	home := t.TempDir()
	env := cleanEnv(home)
	store := profileStoreOf(t, env, home)
	if err := os.WriteFile(store, []byte(`{"profiles":{"p":{"values":{"base-url":"https://u:`+pw+`@api.pax8.com/v1"`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := runWith(t, env, "profile", "show", "p"); strings.Contains(out, pw) {
		t.Fatalf("printed the password:\n%s", out)
	}
}
