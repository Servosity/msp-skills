// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/store"
)

// TestFilteredFullSyncKeepsRowsOutsideTheFilter (handfixes.json:
// filtered-sync-never-prunes-or-completes): `sync --resources devices --full
// --param isMuted=true` reached the last page of the FILTERED request and
// then pruned every unmuted device from the mirror, marking devices fully
// synced, so fleet reports undercounted. A narrowed run must not prune or
// touch sync_state; an unfiltered --full must still prune.
func TestFilteredFullSyncKeepsRowsOutsideTheFilter(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	// The default DB scope is process-global; leave it unscoped so a later
	// test that resolves defaultDBPath before running a command is unaffected.
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
	const muted, unmuted = "00000000-0000-0000-0000-00000000000a", "00000000-0000-0000-0000-00000000000b"
	dropUnmuted := false
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		rows := []string{`{"uuid":"` + muted + `","displayName":"M","isMuted":true}`}
		if r.URL.Query().Get("isMuted") != "true" && !dropUnmuted {
			rows = append(rows, `{"uuid":"`+unmuted+`","displayName":"U","isMuted":false}`)
		}
		_, _ = io.WriteString(w, `{"devices":[`+strings.Join(rows, ",")+`],"nextPageToken":""}`)
	})
	syncRun := func(extra ...string) {
		t.Helper()
		args := append([]string{"sync", "--resources", "devices", "--full"}, extra...)
		if _, errOut, err := captureExecuteAll(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, errOut)
		}
	}
	open := func() *store.Store {
		t.Helper()
		db, err := store.OpenWithContext(context.Background(), defaultDBPath(esetCLIName))
		if err != nil {
			t.Fatal(err)
		}
		return db
	}
	syncRun()
	db := open()
	_, before, _, _ := db.GetSyncState("devices")
	_ = db.Close()

	syncRun("--param", "isMuted=true")
	db = open()
	if _, err := db.Get("devices", unmuted); err != nil {
		t.Fatalf("a filtered --full sync pruned a device outside its filter: %v", err)
	}
	if _, after, _, _ := db.GetSyncState("devices"); !after.Equal(before) {
		t.Fatalf("a filtered sync rewrote the devices sync state: %v -> %v", before, after)
	}
	_ = db.Close()

	dropUnmuted = true
	syncRun()
	db = open()
	defer db.Close()
	if _, err := db.Get("devices", unmuted); err == nil {
		t.Fatal("an unfiltered --full sync must still prune a device ESET no longer returns")
	}
}
