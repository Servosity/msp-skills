// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/store"
)

// acctPreRunLocalCommands are local-store commands (and sync's store writes)
// that never construct an API client before touching the store.
var acctPreRunLocalCommands = [][]string{
	{"e911", "audit", "--json"},
	{"numbers", "inventory", "--json"},
	{"footprint", "jane@example.com", "--no-live", "--json"},
	{"sync", "--resources", "users", "--json"},
	{"search", "jane", "--json"},
}

// acctSeedStore gives an account a real SQLite store so a command reaching it
// would read it or add tables, triggers or journal files.
func acctSeedStore(t *testing.T, home string) {
	t.Helper()
	for _, name := range []string{"data.db", "data-seeded.db"} {
		db, err := store.Open(filepath.Join(home, "data", name))
		if err != nil {
			t.Fatal(err)
		}
		_ = db.Close()
	}
}

// Codex round 6: with acme/data linked to globex/data and acme selected, every
// local-store command is refused (exit 10) before the generated pre-run opens,
// scopes or writes the store, and globex's store is byte-for-byte unchanged.
func TestAcctPreRunBlocksLocalStoreCommands(t *testing.T) {
	f := acctTreeSetup(t)
	acctSeedStore(t, f.globex)
	mustSymlink(t, filepath.Join(f.globex, "data"), filepath.Join(f.acme, "data"))
	before := acctSnapshot(t, f.globex)
	t.Setenv("ZOOM_PHONE_HOME", f.acme)
	t.Setenv("ZOOM_PHONE_CONFIG", f.acmeProfile)

	for _, args := range acctPreRunLocalCommands {
		t.Run(strings.Join(args[:len(args)-1], " "), func(t *testing.T) {
			_, errOut, code := acctRun(t, "", args...)
			if code != 10 || !strings.Contains(errOut, "is a symlink") {
				t.Fatalf("exit=%d stderr=%s", code, errOut)
			}
		})
	}
	if after := acctSnapshot(t, f.globex); !mapsEqual(before, after) {
		t.Fatalf("globex's store changed: %s", acctSnapshotDiff(before, after))
	}

	// --home spelling of the same selection is refused the same way.
	t.Setenv("ZOOM_PHONE_HOME", "")
	t.Cleanup(func() { _, _ = cliutil.SetHomeOverride("") })
	if _, errOut, code := acctRun(t, "", "search", "jane", "--json", "--home", f.acme, "--config", f.acmeProfile); code != 10 || !strings.Contains(errOut, "is a symlink") {
		t.Fatalf("--home <acme> search: exit=%d stderr=%s", code, errOut)
	}
	if after := acctSnapshot(t, f.globex); !mapsEqual(before, after) {
		t.Fatalf("globex's store changed after --home run")
	}
}

func TestAcctPreRunCleanProfileStillWorks(t *testing.T) {
	f := acctTreeSetup(t)
	acctSeedStore(t, f.acme)
	t.Setenv("ZOOM_PHONE_HOME", f.acme)
	t.Setenv("ZOOM_PHONE_CONFIG", f.acmeProfile)
	for _, args := range acctPreRunLocalCommands {
		if args[0] == "sync" {
			args = append(append([]string{}, args...), "--dry-run") // no network in tests
		}
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, errOut, code := acctRun(t, "", args...)
			if code == 10 {
				t.Fatalf("clean acme refused: %s", errOut)
			}
			if code != 0 {
				t.Fatalf("exit=%d stderr=%s", code, errOut)
			}
		})
	}
	// The operator's default profile (nothing selected) is untouched by the check.
	t.Setenv("ZOOM_PHONE_HOME", "")
	t.Setenv("ZOOM_PHONE_CONFIG", "")
	if _, errOut, code := acctRun(t, "", "e911", "audit", "--json"); code != 0 {
		t.Fatalf("default profile e911 audit: exit=%d stderr=%s", code, errOut)
	}
}

// Every command that defines its own PersistentPreRun(E) is wrapped, so no
// subtree can bypass the root's isolation check.
func TestAcctPreRunWrapsEveryPreRun(t *testing.T) {
	f := acctTreeSetup(t)
	mustSymlink(t, filepath.Join(f.globex, "data"), filepath.Join(f.acme, "data"))
	t.Setenv("ZOOM_PHONE_HOME", f.acme)
	t.Setenv("ZOOM_PHONE_CONFIG", f.acmeProfile)

	root := RootCmd()
	var flags rootFlags
	ran := false
	child, _, err := root.Find([]string{"search"})
	if err != nil {
		t.Fatal(err)
	}
	child.PersistentPreRunE = func(*cobra.Command, []string) error { ran = true; return nil }
	acctWrapPreRuns(root, &flags)
	if err := child.PersistentPreRunE(child, nil); err == nil || ExitCode(err) != 10 {
		t.Fatalf("subcommand pre-run not guarded: err=%v", err)
	}
	if ran {
		t.Fatalf("subcommand pre-run ran before the isolation check refused")
	}
}

// acctSnapshotDiff names files added, removed or modified between snapshots.
func acctSnapshotDiff(before, after map[string]string) string {
	var out []string
	for p, v := range after {
		if old, ok := before[p]; !ok {
			out = append(out, "added "+filepath.Base(p))
		} else if old != v {
			out = append(out, "modified "+filepath.Base(p))
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			out = append(out, "removed "+filepath.Base(p))
		}
	}
	return strings.Join(out, ", ")
}

// Codex round 7: a hard link to ANY of another account's files (database, its
// -wal sidecar, a cache entry) under acme's expected filename is refused for
// local commands, sync and the MCP startup check, and globex is unchanged.
func TestAcctStorageTreeRefusesHardLinkedStoreFiles(t *testing.T) {
	cases := []struct {
		name string
		rel  string // path under both account dirs
		seed func(t *testing.T, path string)
	}{
		{"database", filepath.Join("data", "data-seeded.db"), func(t *testing.T, p string) {
			db, err := store.Open(p)
			if err != nil {
				t.Fatal(err)
			}
			_ = db.Close()
		}},
		{"-wal sidecar", filepath.Join("data", "data-seeded.db-wal"), func(t *testing.T, p string) {
			zoomAcctTestWrite(t, p, "globex wal frames")
		}},
		{"cache entry", filepath.Join("cache", "http", "resources", "phone", "users.json"), func(t *testing.T, p string) {
			zoomAcctTestWrite(t, p, `{"users":[{"id":"globex-user"}]}`)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := acctTreeSetup(t)
			src := filepath.Join(f.globex, tc.rel)
			mustMkdir(t, filepath.Dir(src))
			tc.seed(t, src)
			dst := filepath.Join(f.acme, tc.rel)
			mustMkdir(t, filepath.Dir(dst))
			if err := os.Link(src, dst); err != nil {
				t.Fatal(err)
			}
			before := acctSnapshot(t, f.globex)
			t.Setenv("ZOOM_PHONE_HOME", f.acme)
			t.Setenv("ZOOM_PHONE_CONFIG", f.acmeProfile)

			for _, args := range [][]string{{"e911", "audit", "--json"}, {"search", "globex", "--json"}, {"sync", "--resources", "users", "--json"}} {
				if _, errOut, code := acctRun(t, "", args...); code != 10 || !strings.Contains(errOut, "hard links") {
					t.Fatalf("%v: exit=%d stderr=%s", args, code, errOut)
				}
			}
			if err := ZoomCheckAccountIsolation(); err == nil || ExitCode(err) != 10 || !strings.Contains(err.Error(), "hard links") {
				t.Fatalf("MCP startup check: err=%v", err)
			}
			if after := acctSnapshot(t, f.globex); !mapsEqual(before, after) {
				t.Fatalf("globex changed: %s", acctSnapshotDiff(before, after))
			}

			// Clean: the link removed, acme works again.
			if err := os.Remove(dst); err != nil {
				t.Fatal(err)
			}
			if err := ZoomCheckAccountIsolation(); err != nil {
				t.Fatalf("clean acme refused: %v", err)
			}
			if _, errOut, code := acctRun(t, "", "e911", "audit", "--json"); code != 0 {
				t.Fatalf("clean acme e911 audit: exit=%d stderr=%s", code, errOut)
			}
		})
	}
}

func zoomAcctTestWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
