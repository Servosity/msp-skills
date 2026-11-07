// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json dry-run-sends-nothing): the global
// --dry-run sends nothing on any command, the token mint included, and each run
// reaches its preview, so the zero count is not a flag error's. A flag value
// carrying a credential goes through the kit's preview.

package credwire

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

const dryRunProxyPassword = "ddryrunp-cccc-llll-cccc"

func TestDryRunSendsNothing(t *testing.T) {
	t.Parallel()
	for name, creds := range map[string][]string{"minted": keyPair, "token": tokenEnv} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, nil, creds...)
			users := writeFile(t, filepath.Join(e.home, "in", "users.jsonl"), `{"name":"Dry"}`+"\n")
			for _, args := range [][]string{
				{"site", "update", "site-1042", "--name", "Dry Site", "--dry-run"},
				{"site", "create", "--name", "Dry Site", "--proxy-settings-password", dryRunProxyPassword, "--dry-run"},
				{"account", "create-variable", "--name", "dryVar", "--value", "x", "--dry-run"},
				{"device", "quickjob", "create-quick-job", "dev-5150", "--job-name", "Dry", "--job-component-component-uid", "comp-77", "--dry-run"},
				{"alert", "resolve", "alert", "alert-7731", "--dry-run"},
				{"user", "--dry-run"},
				{"import", "user", "--input", users, "--dry-run"},
				{"export", "account", "--dry-run"},
			} {
				r := e.run(t, "", args...)
				if !strings.Contains(r.out, "dry_run") && !strings.Contains(r.out, "would ") && !strings.Contains(r.out, "dry run") && !strings.Contains(r.out, "GET ") {
					t.Errorf("%v printed no preview, so the zero count proves nothing:\n%s", args, r.out)
				}
				if strings.Contains(r.out, dryRunProxyPassword) {
					t.Errorf("%v printed the proxy password:\n%s", args, r.out)
				}
			}
			for what, rec := range map[string]*recorder{"the API": e.api, "the token endpoint": e.mints, "another host": e.proxy} {
				if got := rec.all(); len(got) != 0 {
					t.Errorf("--dry-run sent %d request(s) to %s, first %s %s", len(got), what, got[0].Method, got[0].Path)
				}
			}
			r := e.run(t, "", "site", "update", "site-1042", "--name", "Dry Site")
			got := e.api.all()
			if len(got) != 1 || got[0].Method != http.MethodPost || got[0].Path != "/api/v2/site/site-1042" {
				t.Errorf("control: want the one POST, got %+v\n%s", got, r.out)
			}
		})
	}
}

// TestAuthLoginDryRunMintsNothing: auth login previews a mint and neither sends it
// nor saves anything.
func TestAuthLoginDryRunMintsNothing(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, keyPair...)
	r := e.run(t, "", "auth", "login", "--dry-run")
	if got := e.mints.all(); len(got) != 0 {
		t.Fatalf("auth login --dry-run sent %+v\n%s", got, r.out)
	}
	for path := range e.files(t) {
		if strings.HasPrefix(filepath.Base(path), "credentials") {
			t.Fatalf("auth login --dry-run saved %s", path)
		}
	}
	noHeldValue(t, "auth login --dry-run", r.out)
}
