// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package jamfblackbox

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// profileStore is where this run's profile saves land, wherever the platform
// keeps its config.
func profileStore(t *testing.T, e *bbEnv) string {
	t.Helper()
	e.run(t, "profile", "save", "seed", "--json", "--compact")
	var found string
	_ = filepath.WalkDir(e.home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Name() == "profiles.json" {
			found = p
		}
		return nil
	})
	if found == "" {
		t.Fatal("profile save wrote no profiles.json")
	}
	return found
}

// A run acts on the identity pinned before the profile loads, so a profile that
// names an instance would run against a different client than it says. It is
// refused before anything is sent.
func TestRunProfileCannotChooseTheInstance(t *testing.T) {
	t.Parallel()
	js, e := honestEnv(t, nil)
	store := profileStore(t, e)
	body := `{"profiles":{"acme-only":{"name":"acme-only","values":{"instance":"acme"}}}}`
	if err := os.WriteFile(store, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	before := js.hits.Load()
	out := e.run(t, "--profile", "acme-only", "buildings", "delete", "7", "--yes")
	if n := js.hits.Load(); n != before {
		t.Fatalf("the profile's run reached the server (%d requests):\n%.800s", n-before, out)
	}
	if !strings.Contains(out, "--instance") {
		t.Fatalf("the refusal does not name the way to choose an instance:\n%.800s", out)
	}
	// profile save never stores one, so a profile it wrote always runs.
	saveOut := e.run(t, "--instance", "acme", "profile", "save", "saved", "--json", "--compact")
	if !strings.Contains(saveOut, `"saved"`) || strings.Contains(saveOut, `"instance"`) {
		t.Fatalf("profile save must keep the profile without the instance:\n%.800s", saveOut)
	}
}

// JAMF_PRO_BASE_URL's password stays held whichever server this run selects, and
// however the selection is spelled on the command line.
func TestBaseURLPasswordHeldWhateverIsSelected(t *testing.T) {
	t.Parallel()
	js, e := honestEnv(t, nil)
	const pw = "bbBaseUrlPass-ggkkggkkggkk"
	withURL := &bbEnv{home: e.home, env: append(without(e.env, "JAMF_PRO_SERVER="), "JAMF_PRO_BASE_URL=https://user:"+pw+"@"+js.host)}
	identity := `{"server":"https://` + js.host + `","client_id":"client-acme-0001"}`
	for _, tc := range []struct {
		env  []string
		args []string
	}{
		{nil, []string{"doctor", "--json", "--", "--instance", "acme"}},
		{nil, []string{"doctor", "--select", "--instance", "acme"}},
		{nil, []string{"jamf-pro-version", "--dry-run", "--json", "--select", "--instance=acme"}},
		{[]string{"JAMF_PRO_INSTANCE=acme"}, []string{"doctor", "--json"}},
		{[]string{"PP_JAMF_PRO_IDENTITY=" + identity}, []string{"doctor", "--json"}},
	} {
		out := withURL.with(tc.env...).run(t, tc.args...)
		for _, leak := range []string{pw, pw[:8], pw[len(pw)-8:]} {
			if strings.Contains(out, leak) {
				t.Errorf("%v %v printed the base URL's password:\n%.800s", tc.env, tc.args, out)
			}
		}
	}
}

// The local-data opt-out survives pinning an instance.
func TestNoLocalDataSurvivesPinning(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil, "JAMF_PRO_NO_LOCAL_DATA=1")
	before := localStores(e)
	out := e.run(t, "--instance", "acme", "sync", "--resources", "sites")
	if !strings.Contains(out, "JAMF_PRO_NO_LOCAL_DATA") {
		t.Fatalf("a pinned sync ignored the local-data opt-out:\n%.800s", out)
	}
	if after := localStores(e); !slices.Equal(before, after) {
		t.Fatalf("a pinned sync wrote a local store under the opt-out: %v, was %v", after, before)
	}
}

func localStores(e *bbEnv) []string {
	var stores []string
	_ = filepath.WalkDir(e.home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(d.Name(), ".db") {
			stores = append(stores, p)
		}
		return nil
	})
	return stores
}

// A server value refused for carrying credentials is not repeated in the refusal.
func TestRefusedServerIsNotRepeated(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	const pw = "bbServerPass-kkggkkggkkgg"
	out := e.run(t, "instance", "add", "typo", "--server", "https://user:"+pw+"@acme.jamfcloud.com", "--client-id", "c1")
	if strings.Contains(out, pw) {
		t.Fatalf("the refusal repeated the password:\n%.800s", out)
	}
	if !strings.Contains(out, "bare host") {
		t.Fatalf("the refusal does not say what is wrong:\n%.800s", out)
	}
}

// A URL read from the environment is this run's setting. An ordinary mint saves
// the token, never a credential inside that URL, in any file it writes.
func TestEnvironmentURLCredentialsStayOffDisk(t *testing.T) {
	t.Parallel()
	const pw = "bbUrlOnDisk-qqjjqqjjqqjj"
	for name, mk := range map[string]func(host string) []string{
		"base url password": func(host string) []string {
			return []string{"JAMF_PRO_BASE_URL=https://user:" + pw + "@" + host}
		},
		"base url query": func(host string) []string {
			return []string{"JAMF_PRO_BASE_URL=https://" + host + "?password=" + pw}
		},
		"server userinfo": func(host string) []string {
			return []string{"JAMF_PRO_SERVER=user:" + pw + "@" + host}
		},
	} {
		js, e := honestEnv(t, nil)
		out := e.with(mk(js.host)...).run(t, "jamf-pro-version", "--json", "--data-source", "live")
		wrote := false
		_ = filepath.WalkDir(e.home, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if d.Name() == "config.toml" {
				wrote = true
			}
			if data, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(data), pw) {
				t.Errorf("%s: %s holds the URL's credential", name, p)
			}
			return nil
		})
		if !wrote {
			t.Errorf("%s: the run saved no config, so this proves nothing:\n%.800s", name, out)
		}
	}
}

// With the variable gone, the next run has nothing saved that names that URL.
func TestEnvironmentServerIsNotSavedOverTheFile(t *testing.T) {
	t.Parallel()
	js, e := honestEnv(t, nil)
	e.run(t, "jamf-pro-version", "--json", "--data-source", "live")
	var cfg string
	_ = filepath.WalkDir(e.home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Name() == "config.toml" {
			cfg = p
		}
		return nil
	})
	if cfg == "" {
		t.Fatal("no config saved")
	}
	data, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), js.host) {
		t.Fatalf("the environment's server was saved:\n%s", data)
	}
}
