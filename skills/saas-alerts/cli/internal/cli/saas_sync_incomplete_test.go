// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "unfinished-sync-is-not-synced" entry.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
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

// TestSaasStartedButUnfinishedIsNotEmpty: a first sync that stored rows but
// stopped at a page cap must not be described as "no synced <resource>".
func TestSaasStartedButUnfinishedIsNotEmpty(t *testing.T) {
	saasIsolate(t)
	dbPath := saasSeedFixtureStore(t)
	rw, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rw.DB().Exec(`DELETE FROM sync_state WHERE resource_type = 'devices'`); err != nil {
		t.Fatal(err)
	}
	if err := rw.SaveSyncProgress("devices", "", 1000); err != nil {
		t.Fatal(err)
	}
	rw.Close()
	var got struct {
		Synced            bool     `json:"synced"`
		Note              string   `json:"note"`
		UnsyncedResources []string `json:"unsynced_resources"`
	}
	saasRunJSON(t, &got, "investigate", "jane.doe@example.com", "--db", dbPath, "--json")
	want := "the last devices sync did not finish, so newer rows may be missing from these results; run: saas-alerts-cli sync --resources devices"
	if got.Synced || got.Note != want || strings.Join(got.UnsyncedResources, ",") != "devices" {
		t.Errorf("synced=%v note=%q unsynced=%v, want false/%q/[devices]", got.Synced, got.Note, got.UnsyncedResources, want)
	}
}

type unifyGetFake struct{ paths []string }

func (f *unifyGetFake) Get(_ context.Context, path string, params map[string]string) (json.RawMessage, error) {
	if params["searchAfter"] != "" {
		return nil, fmt.Errorf("searchAfter sent as one value: %q", params["searchAfter"])
	}
	f.paths = append(f.paths, path)
	return json.RawMessage(`[]`), nil
}

// TestUnifyDevicesSearchAfterRepeatedKeys: Unify v2 pages with searchAfter =
// the previous meta.sort (a string array), sent as repeated query keys.
func TestUnifyDevicesSearchAfterRepeatedKeys(t *testing.T) {
	if got := unifySortCursor(json.RawMessage(`[1760000000000,"d00999"]`)); got != `["1760000000000","d00999"]` {
		t.Fatalf("unifySortCursor = %q", got)
	}
	for _, bad := range []string{`[]`, `null`, `"x"`, `[{"a":1}]`} {
		if got := unifySortCursor(json.RawMessage(bad)); got != "" {
			t.Fatalf("unifySortCursor(%s) = %q, want no cursor", bad, got)
		}
	}
	if d := determinePaginationDefaults("devices"); d.cursorParam != "searchAfter" || d.nextCursorPath != "meta.sort" || d.limit != 1000 {
		t.Fatalf("devices pagination defaults = %+v", d)
	}
	f := &unifyGetFake{}
	if _, err := syncGet(context.Background(), f, "devices", "", "/manageApi/api/v1/unify/devices", map[string]string{"pageSize": "1000", "searchAfter": `["1760000000000","d00999"]`}); err != nil {
		t.Fatal(err)
	}
	if len(f.paths) != 1 || !strings.Contains(f.paths[0], "searchAfter=1760000000000&searchAfter=d00999") {
		t.Fatalf("request path = %v", f.paths)
	}
}

// TestSaasListRefreshAfterFullSyncStaysSynced: a page-capped head-of-list
// refresh of rule-triggers after a full sync is staleness, not a hole.
func TestSaasListRefreshAfterFullSyncStaysSynced(t *testing.T) {
	saasIsolate(t)
	dbPath := saasSeedFixtureStore(t)
	rw, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := rw.SaveSyncProgress("rule-triggers", "", 250); err != nil {
		t.Fatal(err)
	}
	rw.Close()
	var got struct {
		Synced bool   `json:"synced"`
		Note   string `json:"note"`
	}
	saasRunJSON(t, &got, "investigate", "jane.doe@example.com", "--db", dbPath, "--json")
	if !got.Synced || got.Note != "" {
		t.Errorf("synced=%v note=%q, want true and no note", got.Synced, got.Note)
	}
}

// TestSaasTabularOutputKeepsTheWarning: --csv/--plain/--quiet print only rows,
// so synced:false must reach stderr.
func TestSaasTabularOutputKeepsTheWarning(t *testing.T) {
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
	for _, format := range []string{"--csv", "--plain", "--quiet"} {
		_, stderr, _ := saasRunCLI(t, "triage", "--db", dbPath, format)
		if !strings.Contains(stderr, "warning: synced: false: the last events sync did not finish") {
			t.Errorf("triage %s: stderr %q has no completeness warning", format, stderr)
		}
	}
}
