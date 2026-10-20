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

// TestSyncMalformedTokenAndSingleObject (sync-metadata-only-pages): a
// non-string nextPageToken decoded to "" and ended the walk as complete, so
// --full pruned unread pages; and the metadata-only shortcut swallowed the
// one single-object resource (quarantined-objects-count), whose zero value
// `{}` must still replace the stored count.
func TestSyncMalformedTokenAndSingleObject(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
	const a, d = "00000000-0000-0000-0000-00000000000a", "00000000-0000-0000-0000-00000000000d"
	devices := map[string]string{"": `{"devices":[{"uuid":"` + a + `"},{"uuid":"` + d + `"}],"nextPageToken":""}`}
	count := `{"objectCount":"5"}`
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/quarantined-objects/count") {
			_, _ = io.WriteString(w, count)
			return
		}
		_, _ = io.WriteString(w, devices[r.URL.Query().Get("pageToken")])
	})
	run := func(args ...string) {
		t.Helper()
		_, _, _ = captureExecuteAll(t, append([]string{"sync"}, args...)...)
	}
	run("--resources", "devices,quarantined-objects-count", "--full")
	// Page 2 is metadata-only with a non-string token; D is on a page the
	// walk never reaches.
	devices = map[string]string{
		"":  `{"devices":[{"uuid":"` + a + `"}],"nextPageToken":"B"}`,
		"B": `{"nextPageToken":42}`,
	}
	count = `{}`
	run("--resources", "devices", "--full")
	run("--resources", "quarantined-objects-count")
	db, err := store.OpenWithContext(context.Background(), defaultDBPath(esetCLIName))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Get("devices", d); err != nil {
		t.Fatalf("a malformed token let --full prune an unread device: %v", err)
	}
	var data string
	if err := db.DB().QueryRow(`SELECT data FROM resources WHERE resource_type = 'quarantined-objects-count'`).Scan(&data); err != nil {
		t.Fatalf("quarantined-objects-count row: %v", err)
	}
	if strings.Contains(data, `"5"`) {
		t.Fatalf("a zero count ({}) left the stale count in the mirror: %s", data)
	}
}
