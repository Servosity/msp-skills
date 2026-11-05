// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zoom-phone-pp-cli/internal/cliutil"
)

type acctTreeFixture struct {
	root, acme, globex string // accounts dir and the two account homes
	acmeProfile        string
}

// acctTreeSetup saves acme and globex and gives globex the files a minted,
// synced account has, so a link from acme into globex has something to reach.
func acctTreeSetup(t *testing.T) acctTreeFixture {
	t.Helper()
	acctTestHome(t)
	acmeProfile := acctMustAdd(t, "acme", "ACCT_A", "CID_A", "acme-secret")
	globexProfile := acctMustAdd(t, "globex", "ACCT_B", "CID_B", "globex-secret")
	f := acctTreeFixture{root: filepath.Dir(filepath.Dir(acmeProfile)), acme: filepath.Dir(acmeProfile), globex: filepath.Dir(globexProfile), acmeProfile: acmeProfile}
	creds := "client_id = 'CID_B'\nclient_secret = 'globex-secret'\nphone_account_id = 'ACCT_B'\naccess_token = 'tok-globex'\n"
	if err := cliutil.AtomicWritePrivateFile(filepath.Join(f.globex, "data", "credentials.toml"), []byte(creds), 0o600, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"state", filepath.Join("cache", "http")} {
		if err := os.MkdirAll(filepath.Join(f.globex, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(f.globex, "cache", "http", "entry.json"), []byte(`{"globex":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

// acctSnapshot records every file under dir (following no links).
func acctSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			out[p] = string(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Codex round 5: no profile may reach another account through a link in its
// storage tree; every entry point refuses with exit 10 before touching files.
func TestAcctStorageTreeRefusesLinks(t *testing.T) {
	cases := []struct {
		name   string
		tamper func(t *testing.T, f acctTreeFixture)
		want   string
	}{
		{"data dir symlinked to another account", func(t *testing.T, f acctTreeFixture) {
			mustSymlink(t, filepath.Join(f.globex, "data"), filepath.Join(f.acme, "data"))
		}, "symlink"},
		{"credentials file symlinked to another account's", func(t *testing.T, f acctTreeFixture) {
			mustMkdir(t, filepath.Join(f.acme, "data"))
			mustSymlink(t, filepath.Join(f.globex, "data", "credentials.toml"), filepath.Join(f.acme, "data", "credentials.toml"))
		}, "symlink"},
		{"credentials file hard-linked to another account's", func(t *testing.T, f acctTreeFixture) {
			mustMkdir(t, filepath.Join(f.acme, "data"))
			if err := os.Link(filepath.Join(f.globex, "data", "credentials.toml"), filepath.Join(f.acme, "data", "credentials.toml")); err != nil {
				t.Fatal(err)
			}
		}, "hard links"},
		{"state dir symlinked", func(t *testing.T, f acctTreeFixture) {
			mustSymlink(t, filepath.Join(f.globex, "state"), filepath.Join(f.acme, "state"))
		}, "symlink"},
		{"cache dir symlinked", func(t *testing.T, f acctTreeFixture) {
			mustSymlink(t, filepath.Join(f.globex, "cache"), filepath.Join(f.acme, "cache"))
		}, "symlink"},
		{"nested cache entry symlinked", func(t *testing.T, f acctTreeFixture) {
			mustMkdir(t, filepath.Join(f.acme, "cache"))
			mustSymlink(t, filepath.Join(f.globex, "cache", "http"), filepath.Join(f.acme, "cache", "http"))
		}, "symlink"},
		{"accounts dir itself is a symlink", func(t *testing.T, f acctTreeFixture) {
			real := filepath.Join(filepath.Dir(f.root), "real-accounts")
			if err := os.Rename(f.root, real); err != nil {
				t.Fatal(err)
			}
			mustSymlink(t, real, f.root)
		}, "symlink"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := acctTreeSetup(t)
			tc.tamper(t, f)
			globexReal, err := filepath.EvalSymlinks(f.globex)
			if err != nil {
				t.Fatal(err)
			}
			before := acctSnapshot(t, globexReal)
			rec := fleetStubSpawn(t, nil)
			t.Setenv("ZOOM_PHONE_HOME", f.acme)
			t.Setenv("ZOOM_PHONE_CONFIG", f.acmeProfile)

			// Client hook (every API command).
			if err := acctGuardNewClientErr(t, f.acmeProfile); err == nil || ExitCode(err) != 10 || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("client hook: err=%v, want exit 10 mentioning %q", err, tc.want)
			}
			// auth login / set-token / logout wrappers.
			if _, errOut, code := acctRun(t, "", "auth", "logout"); code != 10 || !strings.Contains(errOut, tc.want) {
				t.Fatalf("auth logout: exit=%d stderr=%s", code, errOut)
			}
			// Any CLI command with acme selected is refused by the pre-run.
			if _, errOut, code := acctRun(t, "", "fleet", "--", "doctor"); code != 10 || !strings.Contains(errOut, tc.want) {
				t.Fatalf("fleet with acme selected: exit=%d stderr=%s", code, errOut)
			}

			// From the operator's own shell (no account selected) each entry point
			// still refuses the tampered account on its own.
			t.Setenv("ZOOM_PHONE_HOME", "")
			t.Setenv("ZOOM_PHONE_CONFIG", "")
			// fleet, before spawning anything.
			if _, errOut, code := acctRun(t, "", "fleet", "--", "doctor"); code != 10 || !strings.Contains(errOut, "fleet refuses to start") {
				t.Fatalf("fleet: exit=%d stderr=%s", code, errOut)
			}
			if n := len(rec.calls()); n != 0 {
				t.Fatalf("fleet spawned %d children for a tampered account", n)
			}
			// accounts connect --force and accounts remove.
			if _, errOut, code := acctRun(t, "new-secret\n", "accounts", "connect", "acme", "--account-id", "ACCT_A", "--client-id", "CID_A", "--force"); code != 10 {
				t.Fatalf("connect --force: exit=%d stderr=%s", code, errOut)
			}
			if _, errOut, code := acctRun(t, "", "accounts", "remove", "acme"); code != 10 {
				t.Fatalf("remove: exit=%d stderr=%s", code, errOut)
			}
			// accounts list does not read through the link.
			out, _, code := acctRun(t, "", "accounts", "list", "--json")
			var rows []acctInfo
			if code != 0 || json.Unmarshal([]byte(out), &rows) != nil {
				t.Fatalf("list exit=%d out=%s", code, out)
			}
			for _, r := range rows {
				if r.Name == "acme" && (r.Error == "" || r.HasClientID || r.TokenCached) {
					t.Fatalf("list read acme through the link: %+v", r)
				}
			}

			if after := acctSnapshot(t, globexReal); !mapsEqual(before, after) {
				t.Fatalf("globex's files changed:\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

func TestAcctStorageTreeCleanProfileStillWorks(t *testing.T) {
	f := acctTreeSetup(t)
	if err := acctVerifyStorageTree("acme", f.acme); err != nil {
		t.Fatalf("clean acme refused: %v", err)
	}
	if err := acctVerifyStorageTree("globex", f.globex); err != nil {
		t.Fatalf("clean minted globex refused: %v", err)
	}
	if err := acctVerifyStorageTree("newco", filepath.Join(f.root, "newco")); err != nil {
		t.Fatalf("not-yet-created account refused: %v", err)
	}
	t.Setenv("ZOOM_PHONE_HOME", f.acme)
	t.Setenv("ZOOM_PHONE_CONFIG", f.acmeProfile)
	if err := acctGuardNewClientErr(t, f.acmeProfile); err != nil {
		t.Fatalf("clean profile refused by the client hook: %v", err)
	}
	rec := fleetStubSpawn(t, nil)
	if _, errOut, code := acctRun(t, "", "fleet", "--", "doctor"); code != 0 || len(rec.calls()) != 2 {
		t.Fatalf("fleet on clean accounts: exit=%d spawns=%d stderr=%s", code, len(rec.calls()), errOut)
	}
	out, _, _ := acctRun(t, "", "accounts", "list", "--json")
	if strings.Contains(out, `"error"`) {
		t.Fatalf("clean accounts listed with an error: %s", out)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// A symlink OUTSIDE the tree that points at the profile is refused too: the
// generated save rewrites the path it was given, which is outside the account.
func TestAcctStorageTreeRefusesProfileReachedFromOutside(t *testing.T) {
	f := acctTreeSetup(t)
	link := filepath.Join(filepath.Dir(filepath.Dir(f.root)), "acme-link.toml")
	mustSymlink(t, f.acmeProfile, link)
	t.Setenv("ZOOM_PHONE_HOME", f.acme)
	err := acctGuardNewClientErr(t, link)
	if err == nil || ExitCode(err) != 10 || !strings.Contains(err.Error(), "from outside its account directory") {
		t.Fatalf("symlinked profile path: err=%v", err)
	}
	if err := acctGuardNewClientErr(t, f.acmeProfile); err != nil {
		t.Fatalf("the profile itself refused: %v", err)
	}
}
