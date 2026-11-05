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
		`"base-url":"https://operator:` + profilePass + `@` + host + `/backup-radar/v3?key=` + profileQuery + `"}},` +
		`"echo":{"name":"echo","description":"` + profileFirst + `","values":{"json":"true"}}}}`
}

// TestSavedProfileValuesNeverPrint: profile show, use and list, and a command
// run with --profile, never print a saved base-url's password or query value,
// from whichever store the program reads: the current profiles.json under
// --home, BACKUP_RADAR_HOME or BACKUP_RADAR_CONFIG_DIR, or the legacy
// ~/.backup-radar-cli/profiles.json when the current one is absent. Each run
// must print the kit's notice, so the store was found and what it carries was
// there to withhold.
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
		lead  []string            // arguments before each run
		extra []string            // environment beyond newEnv's
		store func(e *env) string // where the store is written
	}{
		{"current store under --home", []string{"--home", homeFlag}, nil, func(*env) string {
			return filepath.Join(homeFlag, "config", "profiles.json")
		}},
		{"current store under BACKUP_RADAR_HOME", nil, nil, func(e *env) string {
			return filepath.Join(e.home, "br", "config", "profiles.json")
		}},
		{"current store under BACKUP_RADAR_CONFIG_DIR", nil, []string{"BACKUP_RADAR_CONFIG_DIR=" + configDir}, func(*env) string {
			return filepath.Join(configDir, "profiles.json")
		}},
		{"legacy store", nil, nil, func(e *env) string {
			return filepath.Join(e.home, ".backup-radar-cli", "profiles.json")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, nil, c.extra...)
			writeFile(t, c.store(e), profileStoreJSON("api.scalepad.com"))
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
// used to send a request, so what it carries goes nowhere. Its own host (here
// the v2 host, which the recording proxy stands in for) never becomes an
// allowed origin, and the configured v3 base URL is not one for it either. The
// control shows the same v2 command, under the same profile, reaches the v2
// host with a harmless term.
func TestSavedProfileBaseURLIsSentNowhere(t *testing.T) {
	keys := []string{"BACKUP_RADAR_API_KEY=" + classV3Key, "BACKUP_RADAR_V2_API_KEY=" + classV2Key}
	newProfileEnv := func(t *testing.T) *env {
		e := newEnv(t, nil, keys...)
		writeFile(t, filepath.Join(e.home, "br", "config", "profiles.json"), profileStoreJSON("api.backupradar.com"))
		return e
	}

	control := newProfileEnv(t)
	r := control.run(t, "", "--profile", "old", "backups", "list", "--search-string", "acme-nightly", "--data-source", "live", "--json")
	if !reachedV2Host(control) {
		t.Fatalf("control: a harmless search never reached api.backupradar.com, so this test proves nothing\nproxy saw %+v\nprinted:\n%s", control.proxy.all(), r.out)
	}

	for _, s := range profileSecrets {
		e := newProfileEnv(t)
		r := e.run(t, "", "--profile", "old", "backups", "list", "--search-string", s, "--data-source", "live", "--json")
		if got := e.proxy.all(); len(got) != 0 {
			t.Errorf("a saved profile secret left for the host its profile names: proxy saw %+v\nprinted:\n%s", got, r.out)
		}
		r2 := e.run(t, "", "--profile", "old", "health", "get", s, "--data-source", "live", "--json")
		if got := e.api.all(); len(got) != 0 {
			t.Errorf("a saved profile secret was sent to the configured v3 base URL: %+v\nprinted:\n%s", got, r2.out)
		}
		for _, out := range []string{r.out, r2.out} {
			if strings.Contains(out, s) {
				t.Errorf("the refusal printed a saved profile secret:\n%s", out)
			}
		}
	}
}
