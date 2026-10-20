// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "unfinished-sync-is-not-synced" entry.

package cli

import (
	"context"
	"strings"
	"testing"

	"saas-alerts-pp-cli/internal/store"
)

// TestSaasIncompleteSyncIsNotSynced: a store whose latest events sync did not
// finish (a page-capped auto-refresh, --max-pages, an interrupted run) must not
// report synced:true, or a partial window reads as a complete digest.
func TestSaasIncompleteSyncIsNotSynced(t *testing.T) {
	saasIsolate(t)
	dbPath := saasSeedFixtureStore(t)
	rw, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := rw.SaveSyncProgress("events", "100", 100); err != nil {
		t.Fatal(err)
	}
	rw.Close()

	type env struct {
		Synced            bool     `json:"synced"`
		Note              string   `json:"note"`
		UnsyncedResources []string `json:"unsynced_resources"`
	}
	const want = "the last events sync did not finish, so newer rows may be missing from these results; run: saas-alerts-cli sync --resources events"
	for _, args := range [][]string{{"triage"}, {"cross-tenant"}, {"investigate", "jane.doe@example.com"}, {"coverage"}} {
		var got env
		saasRunJSON(t, &got, append(args, "--db", dbPath, "--json")...)
		if got.Synced || got.Note != want {
			t.Errorf("%v: synced=%v note=%q, want false/%q", args, got.Synced, got.Note, want)
		}
		if args[0] == "coverage" || args[0] == "investigate" {
			if strings.Join(got.UnsyncedResources, ",") != "events" {
				t.Errorf("%v: unsynced_resources=%v, want [events]", args, got.UnsyncedResources)
			}
		}
	}
}
