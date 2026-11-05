// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"superops-pp-cli/internal/cliutil"
	"superops-pp-cli/internal/cliutil/testenv"
	"superops-pp-cli/internal/store"
)

func seedSuperOpsStore(t *testing.T, path, displayID string) {
	t.Helper()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	row, _ := json.Marshal(map[string]any{
		"ticketId": "7000000000000" + displayID, "displayId": displayID, "subject": "seeded",
		"updatedTime": "2026-01-01T00:00:00", "client": map[string]any{"name": "Acme"},
	})
	if _, _, err := db.UpsertBatch("tickets", []json.RawMessage{row}); err != nil {
		t.Fatal(err)
	}
}

// A store written before the tenant-scoped key (the unscoped data.db of a 4.24
// install, or the credential-only data-<hash>.db) cannot say which tenant
// filled it. Adopting or claiming it shows one tenant's rows under another, so
// the CLI starts a fresh tenant-scoped store instead.
func TestUpgradedStoreIsNeverAdopted(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("SUPEROPS_API_TOKEN", "sop-test-token-4471b9")
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-b")
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	configureDefaultDBScope("")
	scopeHash, legacyHash, _ := currentDefaultDBScope()
	if scopeHash == "" || legacyHash == "" {
		t.Fatalf("scope %q legacy %q: the tenant must be part of the store key", scopeHash, legacyHash)
	}
	seedSuperOpsStore(t, filepath.Join(dir, "data-"+legacyHash+".db"), "LEGACY-1")
	seedSuperOpsStore(t, filepath.Join(dir, "data.db"), "UNSCOPED-1")

	if got, want := defaultDBPath("superops-cli"), filepath.Join(dir, "data-"+scopeHash+".db"); got != want {
		t.Fatalf("default store = %s, want the fresh tenant-scoped %s", got, want)
	}
	out, err := runSuperOpsCLI(t, "stale-tickets", "--days", "1", "--json")
	if err != nil {
		t.Fatalf("stale-tickets: %v\n%s", err, out)
	}
	for _, leaked := range []string{"LEGACY-1", "UNSCOPED-1"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("a store of unproven tenant was read: %s in\n%s", leaked, out)
		}
	}
	for _, kept := range []string{"data-" + legacyHash + ".db", "data.db"} {
		if _, err := os.Stat(filepath.Join(dir, kept)); err != nil {
			t.Fatalf("%s was moved or removed; it must be left alone: %v", kept, err)
		}
	}
}
