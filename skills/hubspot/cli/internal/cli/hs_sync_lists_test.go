// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-wired (handfixes.json: sync-never-calls-lists-idmapping).

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"hubspot-pp-cli/internal/store"
)

// GET /crm/v3/lists/idmapping translates one legacy list ID. Called as a list
// it answers HTTP 400 on every run (#693).
func TestSyncNeverCallsTheListsIDMappingLookup(t *testing.T) {
	for _, name := range append(defaultSyncResources(), knownSyncResourceNames()...) {
		if name == "hubspot-lists-crm" {
			t.Fatalf("sync still walks %s, the id-mapping lookup", name)
		}
		path, err := syncResourcePath(name)
		if err != nil {
			continue // a dependent resource has no flat path
		}
		if strings.Contains(path, "idmapping") {
			t.Fatalf("sync resource %s calls %s as a list", name, path)
		}
	}
}

// A store from before the drop must not stay partial on a row nothing finishes.
func TestSyncForgetsTheDroppedListsCheckpoint(t *testing.T) {
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("opening the local store: %v", err)
	}
	defer db.Close()
	if err := db.SaveSyncProgress("hubspot-lists-crm", "", 0); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncState("hubspot-deals-crm", "", 1); err != nil {
		t.Fatal(err)
	}
	if err := hsForgetDroppedSyncResources(db); err != nil {
		t.Fatal(err)
	}
	var dropped, kept int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM sync_state WHERE resource_type = 'hubspot-lists-crm'`).Scan(&dropped); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM sync_state WHERE resource_type = 'hubspot-deals-crm'`).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if dropped != 0 || kept != 1 {
		t.Fatalf("after forgetting: %d dropped rows, %d other rows; want 0 and 1", dropped, kept)
	}
}
