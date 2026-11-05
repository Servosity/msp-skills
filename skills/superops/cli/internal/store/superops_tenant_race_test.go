// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// plantRawStore writes a SQLite file with the given statements and nothing
// else: no migration and no tenant record.
func plantRawStore(t *testing.T, path string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

// plantMigratedUnclaimedStore is a migrated store with no rows of its own and
// no tenant record: the schema a read-write open of a build without the
// record, or of an unknown tenant, left behind.
func plantMigratedUnclaimedStore(t *testing.T, path string) {
	t.Helper()
	previous := currentTenantSource()
	SetTenantSource(nil)
	defer SetTenantSource(previous)
	s, err := Open(path)
	if err != nil {
		t.Fatalf("plant open: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func refusalOf(t *testing.T, what string, s *Store, err error) *TenantRefusal {
	t.Helper()
	if err == nil {
		if s != nil {
			_ = s.Close()
		}
		t.Fatalf("%s was not refused", what)
	}
	var refusal *TenantRefusal
	if !errors.Is(err, ErrStoreTenant) || !errors.As(err, &refusal) {
		t.Fatalf("%s: err %v, want a tenant refusal", what, err)
	}
	return refusal
}

// Member 1 (round 2 P1): a read-only open of a store that records no tenant
// took it as this tenant's when it held no synced rows, and kept the handle;
// another tenant could then claim and fill the file before the first one ran
// its query. A store that records no tenant is never read, empty or not, and
// the refusal of an empty one names sync.
func TestTenantStampReadOnlyOpenRefusesAStoreThatRecordsNoTenant(t *testing.T) {
	for _, tc := range []struct {
		name  string
		plant func(t *testing.T, path string)
		empty bool
	}{
		{"zero-byte file", func(t *testing.T, path string) {
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"tables without rows", func(t *testing.T, path string) {
			plantRawStore(t, path, `CREATE TABLE resources (id TEXT, resource_type TEXT, data TEXT)`)
		}, true},
		{"migrated schema", plantMigratedUnclaimedStore, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "shared.db")
			tc.plant(t, path)
			for _, tenant := range []string{stampTenantA, stampTenantB} {
				runAsTenant(t, tenant, true)
				s, err := OpenReadOnly(path)
				refusal := refusalOf(t, "read-only open of a store that records no tenant", s, err)
				msg := refusal.Error()
				if !strings.Contains(msg, "superops-cli sync") || !strings.Contains(msg, "--db") {
					t.Fatalf("refusal does not name sync and --db: %s", msg)
				}
				if tc.empty && !strings.Contains(msg, "no data has been synced for this tenant") {
					t.Fatalf("refusal of an empty store does not say nothing was synced: %s", msg)
				}
				if strings.Contains(msg, "acme-") {
					t.Fatalf("refusal names a tenant: %s", msg)
				}
			}
			if got, ok := stampOf(t, path); ok {
				t.Fatalf("a read-only open recorded tenant %q", got)
			}
		})
	}
}

// Member 3 (round 2 P2) and the "any row in any table" rule: the claim used
// to look only at the synced tables, so a store holding only another tenant's
// learn rows (or rows in a table a later build adds) was claimed by the first
// read-write open and its rows were served to that tenant. One row anywhere
// stops the claim; tables without rows do not.
func TestTenantStampClaimNeedsEveryTableEmpty(t *testing.T) {
	ctx := context.Background()
	for _, table := range []string{
		"search_learnings", "entity_lookups", "search_patterns", "learning_playbooks",
		"learn_candidates", "learn_events", "resources_fts_data", "a_table_a_later_build_adds",
	} {
		t.Run(table, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "legacy.db")
			plantRawStore(t, path,
				`CREATE TABLE "`+table+`" (note TEXT)`,
				`INSERT INTO "`+table+`" (note) VALUES ('tenant A note')`)
			raw, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
			if err != nil {
				t.Fatal(err)
			}
			s := &Store{db: raw, path: path}
			err = s.claimTenant(ctx, stampTenantB)
			_ = raw.Close()
			refusal := refusalOf(t, "claim of a store holding a row in "+table, nil, err)
			if !refusal.Unproven {
				t.Fatalf("claim refusal %+v, want unproven", refusal)
			}
			runAsTenant(t, stampTenantB, true)
			s, err = Open(path)
			refusalOf(t, "read-write open of a store holding a row in "+table, s, err)
			s, err = OpenReadOnly(path)
			refusalOf(t, "read-only open of a store holding a row in "+table, s, err)
			if got, ok := stampOf(t, path); ok {
				t.Fatalf("the store was claimed for %q", got)
			}
		})
	}

	path := filepath.Join(t.TempDir(), "tables-only.db")
	plantRawStore(t, path, `CREATE TABLE a_table_a_later_build_adds (note TEXT)`, `CREATE TABLE search_learnings_old (note TEXT)`)
	runAsTenant(t, stampTenantB, true)
	s, err := Open(path)
	if err != nil {
		t.Fatalf("read-write open of a store whose tables hold no rows: %v", err)
	}
	_ = s.Close()
	if got, ok := stampOf(t, path); !ok || got != stampTenantB {
		t.Fatalf("stamp %q (recorded %v), want %q", got, ok, stampTenantB)
	}
}

// Two first read-write opens of one file that does not exist yet, as two
// tenants, race to one owner: exactly one open succeeds and the file records
// that tenant; the other is refused.
func TestTenantStampFirstOpensRaceToOneOwner(t *testing.T) {
	t.Cleanup(func() { SetTenantSource(nil) })
	for round := 0; round < 20; round++ {
		path := filepath.Join(t.TempDir(), "race.db")
		var calls atomic.Int32
		tenants := []string{stampTenantA, stampTenantB}
		SetTenantSource(func() (string, bool) { return tenants[int(calls.Add(1)-1)%2], true })
		start := make(chan struct{})
		var wg sync.WaitGroup
		stores := make([]*Store, 2)
		errs := make([]error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				stores[i], errs[i] = Open(path)
			}(i)
		}
		close(start)
		wg.Wait()
		SetTenantSource(nil)
		won := 0
		for i := range errs {
			if errs[i] == nil {
				won++
				_ = stores[i].Close()
			} else if !errors.Is(errs[i], ErrStoreTenant) {
				t.Fatalf("round %d: open error %v, want a tenant refusal", round, errs[i])
			}
		}
		if won != 1 {
			t.Fatalf("round %d: %d opens won (%v), want exactly one", round, won, errs)
		}
		if got, ok := stampOf(t, path); !ok || (got != stampTenantA && got != stampTenantB) {
			t.Fatalf("round %d: stamp %q (recorded %v)", round, got, ok)
		}
	}
}

// A read-write open whose tenant is unknown (the configuration did not load)
// neither claims nor uses a store that records no tenant.
func TestTenantStampUnknownTenantNeverUsesAnUnclaimedStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	runAsTenant(t, "", false)
	for _, open := range []func(string) (*Store, error){Open, OpenReadOnly} {
		s, err := open(path)
		refusalOf(t, "open of an unclaimed store with no tenant", s, err)
	}
	if got, ok := stampOf(t, path); ok {
		t.Fatalf("an unknown tenant recorded %q", got)
	}
	runAsTenant(t, stampTenantA, true)
	deadline := time.Now().Add(5 * time.Second)
	var s *Store
	err := retryOnBusy(context.Background(), deadline, "open", func() error {
		var err error
		s, err = Open(path)
		return err
	})
	if err != nil {
		t.Fatalf("tenant A's first read-write open: %v", err)
	}
	_ = s.Close()
	if got, ok := stampOf(t, path); !ok || got != stampTenantA {
		t.Fatalf("stamp %q (recorded %v), want %q", got, ok, stampTenantA)
	}
}
