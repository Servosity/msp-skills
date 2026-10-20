// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"io"
	"net/http"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/store"
)

// TestSyncMetadataOnlyPages (handfixes.json: sync-metadata-only-pages):
// protobuf JSON can omit an empty repeated field, so a mid-walk page may be
// `{"nextPageToken":"C"}`. The generated fallback stored it as a resource row,
// lost the token and ended the walk, and `sync --full` then pruned devices on
// the pages it never read. A `{}` page is an empty natural end, not a row.
func TestSyncMetadataOnlyPages(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
	const a, d, stale = "00000000-0000-0000-0000-00000000000a", "00000000-0000-0000-0000-00000000000d", "00000000-0000-0000-0000-0000000000ff"
	pages := map[string]string{
		"": `{"devices":[{"uuid":"` + stale + `","displayName":"STALE"},{"uuid":"` + a + `","displayName":"A"},{"uuid":"` + d + `","displayName":"D"}],"nextPageToken":""}`,
	}
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, pages[r.URL.Query().Get("pageToken")])
	})
	syncFull := func() {
		t.Helper()
		if _, errOut, err := captureExecuteAll(t, "sync", "--resources", "devices", "--full"); err != nil {
			t.Fatalf("sync: %v\n%s", err, errOut)
		}
	}
	syncFull() // mirror holds STALE, A, D
	pages = map[string]string{
		"":  `{"devices":[{"uuid":"` + a + `","displayName":"A"}],"nextPageToken":"B"}`,
		"B": `{"nextPageToken":"C"}`,
		"C": `{"devices":[{"uuid":"` + d + `","displayName":"D"}],"nextPageToken":""}`,
	}
	syncFull()
	db, err := store.OpenWithContext(context.Background(), defaultDBPath(esetCLIName))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Get("devices", d); err != nil {
		t.Fatalf("a device after a metadata-only page was pruned: %v", err)
	}
	if _, err := db.Get("devices", stale); err == nil {
		t.Fatal("a device ESET no longer returns was not pruned")
	}
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'devices'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("want exactly A and D mirrored (no metadata row), got %d (%v)", n, err)
	}
}
