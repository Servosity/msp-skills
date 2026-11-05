// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const (
	stampTenantA = "subdomain=acme-a base_url=https://api.superops.ai"
	stampTenantB = "subdomain=acme-b base_url=https://api.superops.ai"
)

// runAsTenant installs a tenant source for the test; ok false is a command
// whose tenant cannot be determined.
func runAsTenant(t *testing.T, tenant string, ok bool) {
	t.Helper()
	SetTenantSource(func() (string, bool) { return tenant, ok })
	t.Cleanup(func() { SetTenantSource(nil) })
}

// rawHandle is a store handle on path that skipped every open-time gate, so a
// test can drive claimTenant itself.
func rawHandle(t *testing.T, path string) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	return &Store{db: db, path: path}
}

func seedTicket(t *testing.T, path string) {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatalf("seed open: %v", err)
	}
	defer s.Close()
	row, _ := json.Marshal(map[string]any{"ticketId": "7000000000000001", "displayId": "T-1", "subject": "seeded"})
	if _, _, err := s.UpsertBatch("tickets", []json.RawMessage{row}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func stampOf(t *testing.T, path string) (string, bool) {
	t.Helper()
	previous := currentTenantSource()
	SetTenantSource(nil)
	defer SetTenantSource(previous)
	s, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	defer s.Close()
	tenant, ok, err := readTenantStamp(context.Background(), s.db)
	if err != nil {
		t.Fatalf("read stamp: %v", err)
	}
	return tenant, ok
}

func wantRefusal(t *testing.T, what string, s *Store, err error, unproven bool) {
	t.Helper()
	if err == nil {
		_ = s.Close()
		t.Fatalf("%s opened", what)
	}
	var refusal *TenantRefusal
	if !errors.Is(err, ErrStoreTenant) || !errors.As(err, &refusal) || refusal.Unproven != unproven {
		t.Fatalf("%s: err %v, want a tenant refusal (unproven %v)", what, err, unproven)
	}
}

// A store records the tenant of its first read-write open, before any write,
// and every later open of another tenant, read-only or read-write, is refused.
func TestTenantStampRefusesAnotherTenantOnEveryOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.db")
	runAsTenant(t, stampTenantA, true)
	seedTicket(t, path)
	if got, ok := stampOf(t, path); !ok || got != stampTenantA {
		t.Fatalf("stamp %q (recorded %v), want %q", got, ok, stampTenantA)
	}

	runAsTenant(t, stampTenantB, true)
	s, err := OpenReadOnly(path)
	wantRefusal(t, "read-only open as tenant B", s, err, false)
	s, err = Open(path)
	wantRefusal(t, "read-write open as tenant B", s, err, false)
	runAsTenant(t, "", false)
	s, err = OpenReadOnly(path)
	wantRefusal(t, "read-only open with no tenant", s, err, false)

	runAsTenant(t, stampTenantA, true)
	for _, open := range []func(string) (*Store, error){OpenReadOnly, Open} {
		s, err := open(path)
		if err != nil {
			t.Fatalf("tenant A reopen: %v", err)
		}
		if n, err := s.Count("tickets"); err != nil || n != 1 {
			t.Fatalf("tenant A reads %d tickets (err %v), want 1", n, err)
		}
		_ = s.Close()
	}
}

// A store with synced data and no stamp cannot prove its tenant: refused to
// every tenant, read-only and read-write, and never stamped by the refusal.
func TestTenantStampRefusesAnUnprovenStoreWithData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	SetTenantSource(nil)
	seedTicket(t, path)
	for _, tenant := range []string{stampTenantA, stampTenantB} {
		runAsTenant(t, tenant, true)
		s, err := OpenReadOnly(path)
		wantRefusal(t, "read-only open of an unproven store", s, err, true)
		s, err = Open(path)
		wantRefusal(t, "read-write open of an unproven store", s, err, true)
	}
	if got, ok := stampOf(t, path); ok {
		t.Fatalf("a refused open stamped the store with %q", got)
	}
}

// An empty store that records no tenant is claimed only by a read-write open
// of a known tenant. A read-only open refuses it, an unknown tenant refuses
// it, and neither records anything.
func TestTenantStampClaimsAnEmptyStoreOnFirstReadWriteOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	runAsTenant(t, stampTenantA, true)
	s, err := OpenReadOnly(path)
	wantRefusal(t, "read-only open of an empty store", s, err, false)
	if got, ok := stampOf(t, path); ok {
		t.Fatalf("a read-only open stamped %q", got)
	}

	runAsTenant(t, "", false)
	s, err = Open(path)
	wantRefusal(t, "read-write open of an empty store with no tenant", s, err, false)
	if got, ok := stampOf(t, path); ok {
		t.Fatalf("an unknown tenant stamped %q", got)
	}

	runAsTenant(t, stampTenantB, true)
	s, err = Open(path)
	if err != nil {
		t.Fatalf("first read-write open: %v", err)
	}
	_ = s.Close()
	if got, ok := stampOf(t, path); !ok || got != stampTenantB {
		t.Fatalf("stamp %q (recorded %v), want %q", got, ok, stampTenantB)
	}
	s, err = OpenReadOnly(path)
	if err != nil {
		t.Fatalf("read-only open of the claimed store: %v", err)
	}
	_ = s.Close()
}

// Two claims of one empty store, on two handles held open at once, race to a
// single stamp; the loser is refused before it can write.
func TestTenantStampRaceHasOneOwner(t *testing.T) {
	for i := 0; i < 20; i++ {
		path := filepath.Join(t.TempDir(), "race.db")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		a, b := rawHandle(t, path), rawHandle(t, path)
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for j, pair := range []struct {
			s      *Store
			tenant string
		}{{a, stampTenantA}, {b, stampTenantB}} {
			wg.Add(1)
			go func(j int, s *Store, tenant string) {
				defer wg.Done()
				errs[j] = retryOnBusy(context.Background(), time.Now().Add(10*time.Second), "claim", func() error {
					return s.claimTenant(context.Background(), tenant)
				})
			}(j, pair.s, pair.tenant)
		}
		wg.Wait()
		_ = a.Close()
		_ = b.Close()
		won := 0
		for _, err := range errs {
			if err == nil {
				won++
			} else if !errors.Is(err, ErrStoreTenant) {
				t.Fatalf("claim error %v, want a tenant refusal", err)
			}
		}
		if won != 1 {
			t.Fatalf("round %d: %d claims won (%v), want exactly one", i, won, errs)
		}
	}
}
