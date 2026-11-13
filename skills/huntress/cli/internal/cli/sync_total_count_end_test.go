// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-fix (handfixes.json sync-total-count-proves-end, issue #709).

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"testing"

	"huntress-pp-cli/internal/store"
)

// syncScriptedPage is one Huntress-shaped response: {"identities":[...],"pagination":{...}}.
// total < 0 omits total_count; an empty next omits next_page_token, as the live API
// does on its last page.
type syncScriptedPage struct {
	from, to int // ids from..to inclusive
	total    int
	next     string
}

type syncScriptedFake struct{ pages map[string]syncScriptedPage }

func (f *syncScriptedFake) Get(_ context.Context, _ string, params map[string]string) (json.RawMessage, error) {
	p, ok := f.pages[params["page_token"]]
	if !ok {
		return nil, fmt.Errorf("unexpected page_token %q", params["page_token"])
	}
	items := []map[string]any{}
	for i := p.from; i <= p.to; i++ {
		items = append(items, map[string]any{"id": i, "name": "r" + strconv.Itoa(i)})
	}
	pag := map[string]any{"limit": 10, "current_page_count": len(items)}
	if p.total >= 0 {
		pag["total_count"] = p.total
	}
	if p.next != "" {
		pag["next_page_token"] = p.next
	}
	return json.Marshal(map[string]any{"identities": items, "pagination": pag})
}

func (f *syncScriptedFake) RateLimit() float64 { return 0 }

func TestSyncDeclaredTotalProvesTheEndOfACursorWalk(t *testing.T) {
	cases := []struct {
		name         string
		pages        map[string]syncScriptedPage
		wantComplete bool
	}{
		{"final page exactly full", map[string]syncScriptedPage{
			"": {1, 10, 20, "p2"}, "p2": {11, 20, 20, ""}}, true},
		{"api returns every row in one page", map[string]syncScriptedPage{
			"": {1, 25, 25, ""}}, true},
		{"short final page", map[string]syncScriptedPage{
			"": {1, 10, 15, "p2"}, "p2": {11, 15, 15, ""}}, true},
		{"fewer rows than total_count", map[string]syncScriptedPage{
			"": {1, 10, 30, "p2"}, "p2": {11, 20, 30, ""}}, false},
		{"no total_count at all", map[string]syncScriptedPage{
			"": {1, 10, -1, "p2"}, "p2": {11, 20, -1, ""}}, false},
		// Overlapping pages: 20 rows stored but only 15 distinct ids, 16-20 never seen.
		{"duplicate rows across pages", map[string]syncScriptedPage{
			"": {1, 10, 20, "p2"}, "p2": {6, 15, 20, ""}}, false},
		// The count moved mid-walk: 20 distinct ids reach the last page's count,
		// but ids 11-20 of the first listing were skipped.
		{"total_count changed mid-walk", map[string]syncScriptedPage{
			"": {1, 10, 30, "p2"}, "p2": {21, 30, 20, ""}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "sync.db"))
			if err != nil {
				t.Fatalf("opening the local store: %v", err)
			}
			defer db.Close()
			res := syncResource(context.Background(), &syncScriptedFake{pages: tc.pages}, db, "identities", "", false, 0, false, false, &syncUserParams{}, io.Discard)
			if res.Err != nil {
				t.Fatalf("sync failed: %v", res.Err)
			}
			(&syncOutcomes{}).note(db, res, false)
			cov, err := db.SyncCoverage()
			if err != nil {
				t.Fatalf("reading sync coverage: %v", err)
			}
			if complete := len(cov.Incomplete) == 0; complete != tc.wantComplete {
				t.Fatalf("complete = %v (incomplete %v), want %v", complete, cov.Incomplete, tc.wantComplete)
			}
		})
	}
}

// A count-proven end marks the walk complete but never prunes: a listing that
// changed mid-walk (ids 1-10 deleted, 21-30 added) satisfies a stable count with
// distinct ids while live ids 11-20 were skipped. A natural short-page end still prunes.
func TestSyncCountProvenEndNeverPrunes(t *testing.T) {
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("opening the local store: %v", err)
	}
	defer db.Close()
	run := func(pages map[string]syncScriptedPage) syncResult {
		t.Helper()
		res := syncResource(context.Background(), &syncScriptedFake{pages: pages}, db, "identities", "", true, 0, false, true, &syncUserParams{}, io.Discard)
		if res.Err != nil {
			t.Fatalf("sync failed: %v", res.Err)
		}
		return res
	}
	rows := func() int {
		t.Helper()
		var n int
		if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = ?`, "identities").Scan(&n); err != nil {
			t.Fatalf("counting rows: %v", err)
		}
		return n
	}
	fullWalkRecorded := func() bool {
		t.Helper()
		var full sql.NullString
		if err := db.DB().QueryRow(`SELECT full_started_at FROM walk_coverage WHERE resource_type = ? AND scope = ''`, "identities").Scan(&full); err != nil {
			t.Fatalf("reading walk coverage: %v", err)
		}
		return full.Valid
	}
	run(map[string]syncScriptedPage{"": {1, 10, 20, "p2"}, "p2": {11, 20, 20, ""}})
	if got := rows(); got != 20 {
		t.Fatalf("after the first walk: %d rows, want 20", got)
	}
	// Nor is it evidence of absence for the local views (drift, reconciled reads).
	if fullWalkRecorded() {
		t.Fatalf("a count-proven walk was recorded as a full enumeration")
	}
	res := run(map[string]syncScriptedPage{"": {1, 10, 20, "p2"}, "p2": {21, 30, 20, ""}})
	(&syncOutcomes{}).note(db, res, false)
	if cov, err := db.SyncCoverage(); err != nil || len(cov.Incomplete) != 0 {
		t.Fatalf("count-proven walk: coverage %+v err %v, want complete", cov, err)
	}
	if got := rows(); got != 30 {
		t.Fatalf("count-proven walk pruned: %d rows, want 30 (ids 11-20 are still live)", got)
	}
	run(map[string]syncScriptedPage{"": {1, 5, 5, ""}})
	if got := rows(); got != 5 {
		t.Fatalf("natural short-page end did not prune: %d rows, want 5", got)
	}
	if !fullWalkRecorded() {
		t.Fatalf("a natural end was not recorded as a full enumeration")
	}
}
