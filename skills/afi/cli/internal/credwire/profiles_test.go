// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json profile-values-held): a saved run
// profile's base-url is a credential source. Its password and query values are
// held at startup, from the current and the legacy profiles.json, every
// duplicate key included, and are sent nowhere: the profile's URL never becomes
// an allowed origin.

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
)

var profileSecrets = []string{profilePass, profileFirst, profileQuery}

// profileStoreJSON is a run profile store whose profile "old" names base-url
// twice. encoding/json keeps the second member (password profilePass, query
// value profileQuery) and drops the first (password profileFirst), so profile
// "echo" repeats profileFirst as its description: a run that prints echo shows
// whether the dropped duplicate is held too.
func profileStoreJSON(host string) string {
	return `{"profiles":{"old":{"name":"old","values":{` +
		`"base-url":"https://operator:` + profileFirst + `@` + host + `",` +
		`"base-url":"https://operator:` + profilePass + `@` + host + `/api?key=` + profileQuery + `"}},` +
		`"echo":{"name":"echo","description":"` + profileFirst + `","values":{"json":"true"}}}}`
}

// TestSavedProfileValuesNeverPrint: profile show, use and list, and a command
// run with --profile, never print a saved base-url's password or query value,
// from whichever store the program reads: the current profiles.json under
// --home, AFI_HOME or AFI_CONFIG_DIR, or the legacy ~/.afi-cli/profiles.json.
// Each run must print the kit's notice, so the store was found and what it
// carries was there to withhold.
func TestSavedProfileValuesNeverPrint(t *testing.T) {
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
		{"current store under AFI_HOME", nil, nil, func(e *env) string {
			return filepath.Join(e.home, "afi", "config", "profiles.json")
		}},
		{"current store under AFI_CONFIG_DIR", nil, []string{"AFI_CONFIG_DIR=" + configDir}, func(*env) string {
			return filepath.Join(configDir, "profiles.json")
		}},
		{"legacy store", nil, nil, func(e *env) string {
			return filepath.Join(e.home, ".afi-cli", "profiles.json")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, nil, c.extra...)
			writeFile(t, c.store(e), profileStoreJSON("portal.example"))
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

// TestSavedProfileBaseURLIsSentNowhere: a saved profile's base-url is never
// used to send a request, so what it carries goes nowhere, not even to the
// configured base URL. The control shows the same command, under the same
// profile, reaches the fake with a harmless id.
func TestSavedProfileBaseURLIsSentNowhere(t *testing.T) {
	newProfileEnv := func(t *testing.T) *env {
		e := newEnv(t, nil, "AFI_API_KEY="+dryRunKey)
		writeFile(t, filepath.Join(e.home, "afi", "config", "profiles.json"), profileStoreJSON("portal.example"))
		return e
	}
	control := newProfileEnv(t)
	r := control.run(t, "", "--profile", "old", "tenants", fxTenant, "--data-source", "live", "--json")
	if !control.reached("/api/v1/tenants/" + fxTenant) {
		t.Fatalf("control: a harmless id never reached the fake, so this test proves nothing\nprinted:\n%s", r.out)
	}
	for _, s := range profileSecrets {
		e := newProfileEnv(t)
		r := e.run(t, "", "--profile", "old", "tenants", s, "--data-source", "live", "--json")
		if got := e.api.all(); len(got) != 0 {
			t.Errorf("a saved profile secret was sent to the configured base URL: %+v\nprinted:\n%s", got, r.out)
		}
		if got := e.proxy.all(); len(got) != 0 {
			t.Errorf("a saved profile secret left for another host: %+v", got)
		}
		if strings.Contains(r.out, s) {
			t.Errorf("the refusal printed a saved profile secret:\n%s", r.out)
		}
	}
}
