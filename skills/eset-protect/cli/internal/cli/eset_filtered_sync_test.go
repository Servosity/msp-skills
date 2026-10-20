// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

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

// TestFilteredSyncIgnoresAnotherQuerysCursor (filtered-sync-never-prunes-or-
// completes): with the --full / --latest-only reset held for a narrowed
// resource, syncResource read back the resume cursor an interrupted UNFILTERED
// sync left and sent it with the filter, skipping the filtered head.
func TestFilteredSyncIgnoresAnotherQuerysCursor(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
	var mu sync.Mutex
	var seen []string
	first := func() string {
		mu.Lock()
		defer mu.Unlock()
		if len(seen) == 0 {
			return ""
		}
		return seen[0]
	}
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.RawQuery)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("pageToken") == "" {
			_, _ = io.WriteString(w, `{"devices":[{"uuid":"00000000-0000-0000-0000-00000000000a","displayName":"A"}],"nextPageToken":"dp2"}`)
			return
		}
		_, _ = io.WriteString(w, `{"devices":[{"uuid":"00000000-0000-0000-0000-00000000000b","displayName":"B"}],"nextPageToken":""}`)
	})
	for _, args := range [][]string{
		{"sync", "--resources", "devices", "--full"},
		{"sync", "--resources", "devices", "--max-pages", "1"}, // leaves cursor dp2
	} {
		if _, errOut, err := captureExecuteAll(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, errOut)
		}
	}
	for _, args := range [][]string{
		{"sync", "--resources", "devices", "--full", "--param", "isMuted=true"},
		{"sync", "--resources", "devices", "--latest-only", "--resource-param", "devices:isMuted=true"},
	} {
		mu.Lock()
		seen = nil
		mu.Unlock()
		if _, errOut, err := captureExecuteAll(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, errOut)
		}
		if q := first(); q == "" || strings.Contains(q, "pageToken=") {
			t.Fatalf("%v resumed another query's cursor: first request %q", args, q)
		}
	}
}

// TestPolicyScopedIPSetsSyncKeepsOtherPolicies (filtered-sync-never-prunes-
// or-completes): ip-sets is one policy's list (--path-context policyUuid), but
// reconciled as flat_global, so `sync --resources ip-sets --path-context
// policyUuid=B --full` deleted policy A's mirrored rows.
func TestPolicyScopedIPSetsSyncKeepsOtherPolicies(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
	const polA, polB = "00000000-0000-0000-0000-0000000000a1", "00000000-0000-0000-0000-0000000000b1"
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := "00000000-0000-0000-0000-0000000000a2"
		if strings.Contains(r.URL.Path, polB) {
			id = "00000000-0000-0000-0000-0000000000b2"
		}
		_, _ = io.WriteString(w, `{"ipSets":[{"uuid":"`+id+`","displayName":"set"}],"nextPageToken":""}`)
	})
	for _, pol := range []string{polA, polB} {
		args := []string{"sync", "--resources", "ip-sets", "--path-context", "policyUuid=" + pol, "--full"}
		if _, errOut, err := captureExecuteAll(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, errOut)
		}
	}
	db, err := store.OpenWithContext(context.Background(), defaultDBPath(esetCLIName))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, id := range []string{"00000000-0000-0000-0000-0000000000a2", "00000000-0000-0000-0000-0000000000b2"} {
		if _, err := db.Get("ip-sets", id); err != nil {
			t.Fatalf("ip-set %s missing after syncing both policies: %v", id, err)
		}
	}
}

// TestArchiveFullResetIsNotComplete (handfixes.json: archive-full-reset-not-
// complete): `workflow archive --full` reset every resource with SaveSyncState,
// which stamps now and complete=1 before anything is fetched, so a run that
// timed out on its first resource left the untouched ones marked complete.
func TestArchiveFullResetIsNotComplete(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1500 * time.Millisecond) // past --timeout on the first resource
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"nextPageToken":""}`)
	})
	_, _, _ = captureExecuteAll(t, "workflow", "archive", "--full", "--timeout", "300ms")
	db, err := store.OpenWithContext(context.Background(), defaultDBPath(esetCLIName))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if st := esetMirrorCheck(db, "devices", "incidents"); len(st.Incomplete) != 2 {
		t.Fatalf("resources the archive never fetched read as complete: %+v", st)
	}
}

// TestDependentsAreNotMirrored (handfixes.json: dependents-not-mirrored): a
// device-groups sync used to fan out one members request per group into
// device_groups_devices, a table nothing reads whose generic reconcile could
// not cascade deleted parents or prove parent coverage.
func TestDependentsAreNotMirrored(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
	var mu sync.Mutex
	var paths []string
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"deviceGroups":[{"uuid":"00000000-0000-0000-0000-0000000000c1","displayName":"Acme"}],"nextPageToken":""}`)
	})
	if _, errOut, err := captureExecuteAll(t, "sync", "--resources", "device-groups,device-tasks", "--full"); err != nil {
		t.Fatalf("sync: %v\n%s", err, errOut)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, p := range paths {
		if strings.HasSuffix(p, "/devices") || strings.HasSuffix(p, "/runs") {
			t.Fatalf("sync fanned out to a dependent endpoint: %v", paths)
		}
	}
	if len(dependentResourceDefs()) != 0 {
		t.Fatal("dependent resources are mirrored again")
	}
}
