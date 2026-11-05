// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"superops-pp-cli/internal/client"
	"superops-pp-cli/internal/store"
)

func dropStamp(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP TABLE IF EXISTS ` + stampTableName); err != nil {
		t.Fatalf("drop stamp: %v", err)
	}
}

func isTenantRefusal(err error, out string) bool {
	return errors.Is(err, store.ErrStoreTenant) || (strings.Contains(out, "local store ") && strings.Contains(out, "SuperOps tenant"))
}

// Member 1 (round 2 P1): tenant A's read-only open of a shared --db that held
// no rows and recorded no tenant succeeded without claiming it. Tenant B's
// sync then claimed and filled the file, and A's query on the handle it
// already held returned B's tickets. A read-only open of a store that records
// no tenant is refused up front, so A never holds such a handle.
func TestUnclaimedSharedDBIsNeverReadBeforeAnotherTenantFillsIt(t *testing.T) {
	for _, tc := range []struct {
		name  string
		plant func(t *testing.T, path string)
	}{
		{"empty file", func(t *testing.T, path string) {
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"schema without rows or tenant", func(t *testing.T, path string) {
			// The schema a read-write open leaves, with no row synced and
			// no tenant on record.
			s, err := store.Open(path)
			if err != nil {
				t.Fatalf("plant: %v", err)
			}
			_ = s.Close()
			dropStamp(t, path)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			twoTenantFakes(t)
			shared := filepath.Join(t.TempDir(), "shared.db")
			t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
			tc.plant(t, shared)

			// Tenant A opens the file the way the local commands do first:
			// read-only, through the store's one tenant check.
			handle, openErr := store.OpenReadOnly(shared)
			if openErr == nil {
				defer handle.Close()
			}

			// Tenant B syncs into the same file through the real command.
			t.Setenv("SUPEROPS_SUBDOMAIN", "acme-b")
			syncOut, syncErr := runSuperOpsCLI(t, "sync", "--resources", "tickets", "--db", shared)
			t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")

			if openErr == nil {
				recs, qerr := queryRecs(handle, "tickets")
				for _, r := range recs {
					if id := recStr(r, "displayId"); strings.HasPrefix(id, "B-") {
						t.Fatalf("tenant A read tenant B's ticket %s through the read-only handle it opened before B's sync (B's sync err %v)", id, syncErr)
					}
				}
				t.Fatalf("tenant A's read-only open of a store that records no tenant was not refused (query err %v; B's sync err %v):\n%s", qerr, syncErr, syncOut)
			}
			var refusal *store.TenantRefusal
			if !errors.As(openErr, &refusal) {
				t.Fatalf("tenant A's read-only open failed with %v, want a tenant refusal", openErr)
			}
			if msg := openErr.Error(); !strings.Contains(msg, "superops-cli sync") || !strings.Contains(msg, "--db") {
				t.Fatalf("refusal does not name sync with the same --db: %s", msg)
			}

			// Whatever B's sync did, A's own command never shows B's rows.
			out, err := runSuperOpsCLI(t, "stale-tickets", "--days", "1", "--db", shared, "--json")
			if strings.Contains(out, `"B-1"`) {
				t.Fatalf("tenant A's stale-tickets showed tenant B's rows (err %v):\n%s", err, out)
			}
			if err == nil {
				t.Fatalf("tenant A's stale-tickets on a file tenant A does not own succeeded:\n%s", out)
			}
		})
	}
}

// Member 2 (round 2 P1): the store tenant was read from --config again when
// the store opened, apart from the read the client was built from. A config
// that named acme-a when the client was built and acme-b when the store
// opened stamped the store for B while the client wrote A's rows into it. The
// store tenant is the tenant of the one configuration the client is built
// from.
func TestStoreTenantIsTheTenantOfTheClientsConfig(t *testing.T) {
	a, b := twoTenantFakes(t)
	t.Setenv("SUPEROPS_SUBDOMAIN", "")
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	writeSubdomain := func(sub string) {
		if err := os.WriteFile(cfgPath, []byte("subdomain = \""+sub+"\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeSubdomain("acme-a")
	shared := filepath.Join(dir, "shared.db")

	// The edit lands right after the client is built and before the sync
	// command opens its store.
	built := 0
	saved := clientHooks
	clientHooks = append(append([]func(*client.Client) error(nil), saved...), func(*client.Client) error {
		built++
		writeSubdomain("acme-b")
		return nil
	})
	t.Cleanup(func() { clientHooks = saved })

	out, err := runSuperOpsCLI(t, "sync", "--config", cfgPath, "--resources", "tickets", "--db", shared)
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if built == 0 {
		t.Fatal("the client hook never ran, so the config was never edited mid-command")
	}
	stamp, ok := readStamp(t, shared)
	if !ok || !strings.Contains(stamp, "subdomain=acme-a ") {
		t.Fatalf("the store records tenant %q (recorded %v) while the client synced acme-a's rows", stamp, ok)
	}
	a.mu.Lock()
	sent := append([]string(nil), a.subdomain...)
	a.mu.Unlock()
	if len(sent) == 0 {
		t.Fatal("tenant A's fake saw no request")
	}
	for _, s := range sent {
		if s != "acme-a" {
			t.Fatalf("a request went out with CustomerSubDomain %q, want acme-a", s)
		}
	}
	if n := fakeRequests(b); n != 0 {
		t.Fatalf("tenant B's fake saw %d requests", n)
	}
	if got := storeTicketRows(t, shared); got != 12 {
		t.Fatalf("store holds %d tickets, want tenant A's 12", got)
	}

	// The file now names acme-b, so the next command is tenant B's: refused.
	clientHooks = saved
	out, err = runSuperOpsCLI(t, "sync", "--config", cfgPath, "--resources", "tickets", "--db", shared)
	assertRefusal(t, "a sync whose config now names acme-b, into acme-a's --db", out, err)
	out, err = runSuperOpsCLI(t, "stale-tickets", "--days", "1", "--config", cfgPath, "--db", shared, "--json")
	assertRefusal(t, "a read whose config now names acme-b, of acme-a's --db", out, err, `"T-1"`)
}

// Member 3 (round 2 P2): a --db written before the tenant record, holding
// only tenant A's learn rows (teach), was claimed by tenant B's first
// read-write open, and B's recall returned A's note. A store that holds a row
// in any table and records no tenant is never claimed and never read.
func TestLearnOnlyStoreWithoutATenantIsNeverClaimed(t *testing.T) {
	twoTenantFakes(t)
	legacy := filepath.Join(t.TempDir(), "legacy.db")
	query := "which tickets are stale for the acme account"
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	if out, err := runSuperOpsCLI(t, "teach", "--query", query, "--resource-type", "tickets", "--resource", "T-77", "--no-validate", "--db", legacy); err != nil {
		t.Fatalf("tenant A teach: %v\n%s", err, out)
	}
	if out, err := runSuperOpsCLI(t, "recall", query, "--db", legacy, "--json"); err != nil || !strings.Contains(out, "T-77") {
		t.Fatalf("tenant A cannot recall its own note (err %v):\n%s", err, out)
	}
	// The file as a build before the tenant record left it: learn rows only,
	// no synced rows, no record.
	dropStamp(t, legacy)
	if got := storeTicketRows(t, legacy); got != 0 {
		t.Fatalf("the learn-only store holds %d synced tickets", got)
	}

	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-b")
	for _, args := range [][]string{
		{"recall", query, "--db", legacy, "--json"},
		{"learnings", "list", "--db", legacy, "--json"},
		{"sync", "--resources", "tickets", "--db", legacy},
	} {
		out, err := runSuperOpsCLI(t, args...)
		assertRefusal(t, "tenant B "+strings.Join(args[:2], " ")+" on a learn-only store that records no tenant", out, err, "T-77")
	}
	// teach is silent by design (its errors go to teach.log), so only its
	// exit status shows the refusal.
	if out, err := runSuperOpsCLI(t, "teach", "--query", "tenant b's own query", "--resource-type", "tickets", "--resource", "B-1", "--no-validate", "--db", legacy); err == nil {
		t.Fatalf("tenant B teach into a learn-only store that records no tenant succeeded:\n%s", out)
	}
	if stamp, ok := readStamp(t, legacy); ok {
		t.Fatalf("the learn-only store was claimed for %q", stamp)
	}
	for _, sub := range []string{"acme-a", "acme-b"} {
		t.Setenv("SUPEROPS_SUBDOMAIN", sub)
		s, err := store.OpenReadOnly(legacy)
		if err == nil {
			_ = s.Close()
			t.Fatalf("%s read-only open of a learn-only store that records no tenant succeeded", sub)
		}
		if !errors.Is(err, store.ErrStoreTenant) {
			t.Fatalf("%s read-only open: %v, want a tenant refusal", sub, err)
		}
	}
}

// Every store open in the shipped code passes its run's context: the context
// carries the one configuration snapshot of its command execution or MCP tool
// call. The context-free forms, and a bare background context, fall back to
// the process-wide tenant source.
func TestEveryShippedStoreOpenPassesAContext(t *testing.T) {
	contextFree := regexp.MustCompile(`\bstore\.(Open|OpenReadOnly)\(|\bstore\.Open(WithContext|ReadOnlyContext)\(context\.(Background|TODO)\(\)`)
	for _, dir := range []string{".", filepath.Join("..", "mcp")} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		if len(files) == 0 {
			t.Fatalf("no Go files under %s", dir)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range strings.Split(string(data), "\n") {
				code := strings.TrimSpace(line)
				if strings.HasPrefix(code, "//") {
					continue
				}
				if contextFree.MatchString(code) {
					t.Errorf("%s:%d opens a store without its run's context: %s", file, i+1, code)
				}
			}
		}
	}
}

// Positive path: on a fresh install, with the default store or a fresh --db,
// every local command still answers as before. The ones that read synced
// data say to sync; none of them meets a tenant refusal. After a sync they
// read this tenant's rows.
func TestFreshInstallLocalCommandsKeepWorking(t *testing.T) {
	twoTenantFakes(t)
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	fresh := filepath.Join(t.TempDir(), "new", "fresh.db")
	local := [][]string{
		{"stale-tickets", "--days", "1", "--json"},
		{"sla-watch", "--json"},
		{"alert-coverage", "--json"},
		{"unbilled", "--json"},
		{"at-risk-assets", "--json"},
		{"client-360", "Acme", "--json"},
		{"context-ticket", "T-1", "--json"},
		{"search", "ticket", "--data-source", "local", "--json"},
		{"learnings", "list", "--json"},
		{"recall", "stale tickets for acme", "--json"},
		{"playbook", "list", "--json"},
		{"workflow", "status", "--json"},
	}
	for _, db := range [][]string{nil, {"--db", fresh}} {
		for _, args := range local {
			args = append(append([]string(nil), args...), db...)
			if args[0] == "playbook" && len(db) > 0 {
				continue
			}
			out, err := runSuperOpsCLI(t, args...)
			if isTenantRefusal(err, out) {
				t.Fatalf("fresh install: %v met a tenant refusal:\n%s", args, out)
			}
			switch args[0] {
			case "client-360", "context-ticket":
				// Not found in an empty store; never a store error.
				if err != nil && strings.Contains(out, "opening local database") {
					t.Fatalf("fresh install: %v could not open the store: %v\n%s", args, err, out)
				}
			default:
				if err != nil {
					t.Fatalf("fresh install: %v failed: %v\n%s", args, err, out)
				}
			}
		}
	}

	// A read that needs synced data says so.
	out, err := runSuperOpsCLI(t, "tickets", "list", "--data-source", "local", "--json")
	if err == nil || isTenantRefusal(err, out) || !strings.Contains(out, "sync") {
		t.Fatalf("fresh install: tickets list --data-source local (err %v) does not point to sync:\n%s", err, out)
	}

	// doctor reports the store, not a refusal.
	out, _ = runSuperOpsCLI(t, "doctor", "--json")
	if strings.Contains(out, "records no SuperOps tenant") || strings.Contains(out, "is not read or written") {
		t.Fatalf("fresh install: doctor reports a tenant refusal:\n%s", out)
	}
	var report map[string]any
	if json.Unmarshal([]byte(out), &report) == nil {
		if cache, ok := report["cache"].(map[string]any); ok && cache["status"] == "error" {
			t.Fatalf("fresh install: doctor's store report is an error:\n%s", out)
		}
	}

	// After a sync every local read sees this tenant's rows.
	for _, db := range [][]string{nil, {"--db", fresh}} {
		if out, err := runSuperOpsCLI(t, append([]string{"sync", "--resources", "tickets"}, db...)...); err != nil {
			t.Fatalf("sync %v: %v\n%s", db, err, out)
		}
		out, err := runSuperOpsCLI(t, append([]string{"stale-tickets", "--days", "1", "--json"}, db...)...)
		if err != nil || !strings.Contains(out, `"T-1"`) {
			t.Fatalf("stale-tickets after sync %v (err %v):\n%s", db, err, out)
		}
		out, err = runSuperOpsCLI(t, append([]string{"search", "ticket", "--data-source", "local", "--json"}, db...)...)
		if err != nil || !strings.Contains(out, "T-1") {
			t.Fatalf("search after sync %v (err %v):\n%s", db, err, out)
		}
	}
	out, err = runSuperOpsCLI(t, "tickets", "list", "--data-source", "local", "--json")
	if err != nil || !strings.Contains(out, "T-1") {
		t.Fatalf("tickets list --data-source local after sync (err %v):\n%s", err, out)
	}

	// local-data purge removes the default store, and the next command
	// starts a fresh one for this tenant.
	if out, err := runSuperOpsCLI(t, "local-data", "purge", "--only", "store", "--yes"); err != nil {
		t.Fatalf("local-data purge: %v\n%s", err, out)
	}
	configureDefaultDBScope("")
	if _, err := os.Stat(defaultDBPath("superops-cli")); !os.IsNotExist(err) {
		t.Fatalf("the default store is still there after purge (stat err %v)", err)
	}
	if out, err := runSuperOpsCLI(t, "stale-tickets", "--days", "1", "--json"); err != nil {
		t.Fatalf("stale-tickets after purge: %v\n%s", err, out)
	}
	if stamp, ok := readStamp(t, defaultDBPath("superops-cli")); !ok || !strings.Contains(stamp, "subdomain=acme-a ") {
		t.Fatalf("the store after purge records %q (recorded %v)", stamp, ok)
	}
}
