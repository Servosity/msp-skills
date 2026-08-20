package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"halopsa-pp-cli/internal/store"
)

// fakeFeed stands in for HaloPSA's /Feed: a time-ordered activity stream that
// knows nothing about page_no. It records every request the sync loop makes so
// the walk itself can be asserted, and deliberately ignores paging parameters
// exactly as the real endpoint would, returning the newest window every time.
type fakeFeed struct {
	calls   []map[string]string
	total   int
	pageCap int // rows returned per call; 0 means "all of them"
}

func (f *fakeFeed) Get(_ context.Context, _ string, params map[string]string) (json.RawMessage, error) {
	copied := map[string]string{}
	for k, v := range params {
		copied[k] = v
	}
	f.calls = append(f.calls, copied)

	n := f.total
	if f.pageCap > 0 && f.pageCap < n {
		n = f.pageCap
	}
	rows := make([]string, 0, n)
	for i := 0; i < n; i++ {
		// Descending ids, newest first, as an activity feed returns them.
		rows = append(rows, fmt.Sprintf(
			`{"id":%d,"datetime":"2026-04-16T16:13:52.903","entitytype":1,"outcome":"KB Viewed"}`,
			5600-i))
	}
	return json.RawMessage("[" + strings.Join(rows, ",") + "]"), nil
}

func (f *fakeFeed) RateLimit() float64 { return 0 }

func syncFeed(t *testing.T, api *fakeFeed, maxPages int) (syncResult, *store.Store) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	res := syncResource(context.Background(), api, db, "feed", "", true, maxPages, false,
		&syncUserParams{}, nil)
	if res.Err != nil {
		t.Fatalf("sync feed: %v", res.Err)
	}
	return res, db
}

// The reported defect: /Feed was walked by page number, so the sync spent a
// request per page re-reading a window it already had (#273).
func TestFeedIsFetchedInOneWindow(t *testing.T) {
	api := &fakeFeed{total: 100, pageCap: 100}
	res, db := syncFeed(t, api, 0)

	if len(api.calls) != 1 {
		t.Fatalf("feed made %d requests, want exactly 1 (it is not page-walkable)", len(api.calls))
	}
	sent := api.calls[0]
	for _, banned := range []string{"page_no", "page_size", "pageinate"} {
		if v, ok := sent[banned]; ok {
			t.Errorf("feed was sent %s=%q; /Feed has no such parameter", banned, v)
		}
	}
	if sent["count"] != "100" {
		t.Errorf("feed was sent count=%q, want the window size 100", sent["count"])
	}
	if res.Count != 100 {
		t.Errorf("feed stored %d rows, want 100", res.Count)
	}
	var stored int
	if err := db.DB().QueryRow(`SELECT count(*) FROM resources WHERE resource_type = 'feed'`).Scan(&stored); err != nil {
		t.Fatalf("counting feed rows: %v", err)
	}
	if stored != 100 {
		t.Errorf("resources holds %d feed rows, want 100", stored)
	}
}

// Re-syncing must refresh the newest window rather than accumulate copies: the
// rows carry a globally unique id, so the mirror collapses a re-fetch.
func TestFeedResyncIsIdempotent(t *testing.T) {
	api := &fakeFeed{total: 40, pageCap: 40}
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	for i := 0; i < 3; i++ {
		if res := syncResource(context.Background(), api, db, "feed", "", true, 0, false,
			&syncUserParams{}, nil); res.Err != nil {
			t.Fatalf("sync %d: %v", i, res.Err)
		}
	}
	if len(api.calls) != 3 {
		t.Fatalf("three syncs made %d requests, want 3", len(api.calls))
	}
	var stored int
	if err := db.DB().QueryRow(`SELECT count(*) FROM resources WHERE resource_type = 'feed'`).Scan(&stored); err != nil {
		t.Fatalf("counting feed rows: %v", err)
	}
	if stored != 40 {
		t.Errorf("three syncs of the same 40 rows stored %d; a re-fetch must collapse, not accumulate", stored)
	}
}

// --param must still win, so an operator who wants a wider window can ask.
func TestFeedWindowIsUserOverridable(t *testing.T) {
	api := &fakeFeed{total: 10, pageCap: 10}
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	up := &syncUserParams{flatGlobal: map[string]string{"count": "250"}}
	if res := syncResource(context.Background(), api, db, "feed", "", true, 0, false, up, nil); res.Err != nil {
		t.Fatalf("sync: %v", res.Err)
	}
	if got := api.calls[0]["count"]; got != "250" {
		t.Errorf("--param count=250 was overridden to %q; user params must win", got)
	}
}
