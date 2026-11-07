// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json profile-values-held): a saved run
// profile is a credential source. A base-url's password and query values and a
// credential flag's value are held at startup, from the current and the legacy
// profiles.json, every duplicate key included, and are sent nowhere: the profile's
// URL never becomes an allowed origin.

package credwire

import (
	"path/filepath"
	"strings"
	"testing"
)

const (
	profilePass  = "ProfPassKw7Rt3Yx9Lm2Dh" // password of the base-url the program shows
	profileFirst = "ProfFirstQz4Hv8Np6JsCe" // password of the duplicate the program's decoder drops
	profileQuery = "ProfQueryBc5Mf1Wg7TdAr" // a query value of the base-url the program shows
	profileProxy = "ProfProxyTg3Ls9Vk2HxBn" // a saved --proxy-settings-password value
)

var profileSecrets = []string{profilePass, profileFirst, profileQuery, profileProxy}

// profileStoreJSON is a run profile store whose profile "old" names base-url twice.
// encoding/json keeps the second member and drops the first, so profile "echo"
// repeats the dropped password as its description: a run that prints echo shows
// whether the dropped duplicate is held too.
func profileStoreJSON(host string) string {
	return `{"profiles":{"old":{"name":"old","values":{` +
		`"base-url":"https://operator:` + profileFirst + `@` + host + `",` +
		`"base-url":"https://operator:` + profilePass + `@` + host + `/api?key=` + profileQuery + `",` +
		`"proxy-settings-password":"` + profileProxy + `"}},` +
		`"echo":{"name":"echo","description":"` + profileFirst + `","values":{"json":"true"}}}}`
}

// TestSavedProfileValuesNeverPrint: profile show, use and list, and a command run
// with --profile, never print a saved secret, from whichever store the program
// reads. Each run must print the kit's notice, so the store was found and what it
// carries was there to withhold.
func TestSavedProfileValuesNeverPrint(t *testing.T) {
	t.Parallel()
	runs := [][]string{
		{"profile", "show", "old", "--json"},
		{"profile", "use", "old"},
		{"profile", "list", "--json"},
		{"profile", "show", "echo", "--json"},
		{"--profile", "old", "profile", "use", "old", "--json"},
	}
	homeFlag := t.TempDir()
	configDir := t.TempDir()
	cases := []struct {
		name  string
		lead  []string
		extra []string
		store func(e *env) string
	}{
		{"current store under --home", []string{"--home", homeFlag}, nil, func(*env) string {
			return filepath.Join(homeFlag, "config", "profiles.json")
		}},
		{"current store under DATTO_RMM_HOME", nil, nil, func(e *env) string {
			return filepath.Join(e.home, "dr", "config", "profiles.json")
		}},
		{"current store under DATTO_RMM_CONFIG_DIR", nil, []string{"DATTO_RMM_CONFIG_DIR=" + configDir}, func(*env) string {
			return filepath.Join(configDir, "profiles.json")
		}},
		{"legacy store", nil, nil, func(e *env) string {
			return filepath.Join(e.home, ".datto-rmm-cli", "profiles.json")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, nil, c.extra...)
			writeFile(t, c.store(e), profileStoreJSON("merlot-api.centrastage.net"))
			for _, run := range runs {
				args := append(append([]string{}, c.lead...), run...)
				r := e.run(t, "", args...)
				for _, s := range profileSecrets {
					if strings.Contains(r.out, s) {
						t.Errorf("%v printed the saved profile secret %s:\n%s", args, s, r.out)
					}
				}
				if !strings.Contains(r.out, "withheld") {
					t.Errorf("%v printed no notice, so the store was not read or held nothing:\n%s", args, r.out)
				}
			}
		})
	}
}

// TestSavedProfileValueIsSentNowhere: what a saved profile carries is never sent,
// to its own host or to the configured API. The control shows the same command,
// under the same profile, reaches the fake with a harmless value.
func TestSavedProfileValueIsSentNowhere(t *testing.T) {
	t.Parallel()
	control := newEnv(t, nil, tokenEnv...)
	writeFile(t, filepath.Join(control.home, "dr", "config", "profiles.json"), profileStoreJSON("merlot-api.centrastage.net"))
	r := control.run(t, "", "--profile", "old", "site", "get", "acme-nightly", "--data-source", "live", "--json")
	if len(control.api.all()) != 1 {
		t.Fatalf("control: a harmless get never reached the fake, so this test proves nothing\nsaw %+v\nprinted:\n%s", control.api.all(), r.out)
	}
	for _, s := range profileSecrets {
		e := newEnv(t, nil, tokenEnv...)
		writeFile(t, filepath.Join(e.home, "dr", "config", "profiles.json"), profileStoreJSON("merlot-api.centrastage.net"))
		r := e.run(t, "", "--profile", "old", "site", "get", s, "--data-source", "live", "--json")
		if got := e.proxy.all(); len(got) != 0 {
			t.Errorf("a saved profile secret left for the host its profile names: %+v\nprinted:\n%s", got, r.out)
		}
		if got := e.api.all(); len(got) != 0 {
			t.Errorf("a saved profile secret was sent to the configured API: %+v\nprinted:\n%s", got, r.out)
		}
		if strings.Contains(r.out, s) {
			t.Errorf("the refusal printed a saved profile secret:\n%s", r.out)
		}
	}
}
