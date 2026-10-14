// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"threatdown-pp-cli/internal/cliutil/testenv"
	"threatdown-pp-cli/internal/fleet"
	"threatdown-pp-cli/internal/store"
)

func TestAccountlessNote(t *testing.T) {
	if n := accountlessNote([]fleet.Site{{ID: "s1", AccountID: "a1", Name: "Acme"}}); n != "" {
		t.Fatalf("every site has an account_id, want no note, got %q", n)
	}
	n := accountlessNote([]fleet.Site{{ID: "s1", AccountID: "a1", Name: "Acme"}, {ID: "s2", Name: "Trial Co"}})
	if !strings.Contains(n, "1 site(s)") || !strings.Contains(n, "Trial Co") || strings.Contains(n, "Acme") {
		t.Fatalf("note should name only the account-less site, got %q", n)
	}
	if got := joinNotes("", "a", " ", "b"); got != "a; b" {
		t.Fatalf("joinNotes = %q", got)
	}
}

// seedFleetStore writes sites into a fresh store at path, the way fleet sync does.
func seedFleetStore(t *testing.T, path string, sites ...fleet.Site) {
	t.Helper()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range sites {
		raw, _ := json.Marshal(s)
		if err := db.Upsert(tdResSites, s.ID, raw); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCustomerReportWithoutSyncedSitesIsNotFound(t *testing.T) {
	testenv.Isolate(t)
	dir := t.TempDir()

	// No mirror at all: exit 3, never an all-zero profile.
	out, err := runRoot(t, "customer", "report", "Acme Corp", "--db", filepath.Join(dir, "missing.db"), "--json")
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 3 || strings.Contains(out, "attention_score") {
		t.Fatalf("no mirror must exit 3 with no profile, got err=%v out=%s", err, out)
	}

	// A store with no sites yet: same answer.
	empty := filepath.Join(dir, "empty.db")
	seedFleetStore(t, empty)
	out, err = runRoot(t, "customer", "report", "Acme Corp", "--db", empty, "--json")
	if !errors.As(err, &ce) || ce.code != 3 || strings.Contains(out, "attention_score") {
		t.Fatalf("no sites must exit 3 with no profile, got err=%v out=%s", err, out)
	}
}

func TestCustomerReportAccountlessSite(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "data.db")
	seedFleetStore(t, path, fleet.Site{ID: "s1", AccountID: "a1", Name: "Acme"}, fleet.Site{ID: "s9", Name: "Trial NoSub"})
	out, err := runRoot(t, "customer", "report", "Trial", "--db", path, "--json")
	if err != nil {
		t.Fatalf("an existing account-less site is found: %v", err)
	}
	var v customerReportView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("parse %s: %v", out, err)
	}
	if v.Site.Name != "Trial NoSub" || v.Health.Site != "Trial NoSub" || !strings.Contains(v.Note, "no account_id") {
		t.Fatalf("account-less site should be named in health and explained in note: %+v", v)
	}
}
