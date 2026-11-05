// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Walk and scope reads fail closed (polish): a read error never yields a
// partial walk set or scope set that could look usable, and the writers that
// discard row events abort instead of skipping a walk.

package cli

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"zoom-phone-pp-cli/internal/store"
)

// localFailingQuerier delegates to a real database but fails every query
// whose text contains failOn, as an I/O error mid-read would.
type localFailingQuerier struct {
	db     *sql.DB
	failOn string
}

func (q localFailingQuerier) Query(query string, args ...any) (*sql.Rows, error) {
	if q.failOn != "" && strings.Contains(query, q.failOn) {
		return nil, errors.New("disk I/O error")
	}
	return q.db.Query(query, args...)
}

func (q localFailingQuerier) QueryRow(query string, args ...any) *sql.Row {
	return q.db.QueryRow(query, args...)
}

// localWalkStore returns an open store holding one completed type=byoc
// numbers walk over pn-1 and pn-2, with the event log tables created.
func localWalkStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for _, ddl := range localSyncEventDDL {
		if _, err := s.DB().Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	if err := localWriteWalk(s, "numbers", "type=byoc", true, false, "2026-10-01T00:00:00Z", "test", []string{"pn-1", "pn-2"}); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLocalReadWalksFailClosed(t *testing.T) {
	cases := []struct {
		name   string
		failOn string
	}{
		{"walk rows unreadable", "zoom_sync_walk_rows"},
		{"row events unreadable", "zoom_row_events"},
		{"walks unreadable", "FROM zoom_sync_walks"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := localWalkStore(t)
			q := localFailingQuerier{db: s.DB(), failOn: tc.failOn}
			if got := localReadWalks(q, "numbers"); len(got) != 0 {
				t.Fatalf("localReadWalks returned %d walks on a read error, want none (a half-read walk must not look usable): %+v", len(got), got)
			}
			if _, err := localReadWalksErr(q, "numbers"); err == nil || !strings.Contains(err.Error(), "disk I/O error") {
				t.Fatalf("localReadWalksErr error = %v, want the read error", err)
			}
		})
	}
}

func TestLocalReadWalksHappyPathAndAbsence(t *testing.T) {
	s := localWalkStore(t)
	walks, err := localReadWalksErr(s.DB(), "numbers")
	if err != nil {
		t.Fatal(err)
	}
	w, ok := walks["type=byoc"]
	if !ok || !w.usable() || !w.rows["pn-1"] || !w.rows["pn-2"] || len(w.rows) != 2 {
		t.Fatalf("byoc walk = %+v (ok=%v), want a usable walk over pn-1, pn-2", w, ok)
	}
	// A logged write to one of its rows supersedes it.
	if _, err := s.DB().Exec(`INSERT INTO zoom_row_events (resource_type, id) VALUES ('numbers', 'pn-2')`); err != nil {
		t.Fatal(err)
	}
	if w := localReadWalks(s.DB(), "numbers")["type=byoc"]; !w.superseded || w.usable() {
		t.Fatalf("after a logged write: %+v, want superseded", w)
	}
	// Another resource has no walks.
	if got, err := localReadWalksErr(s.DB(), "locations"); err != nil || len(got) != 0 {
		t.Fatalf("locations walks = %v, %v; want none", got, err)
	}

	// A store with no walk tables at all (never synced) has no walks and no error.
	fresh, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if got, err := localReadWalksErr(fresh.DB(), "numbers"); err != nil || len(got) != 0 {
		t.Fatalf("fresh store walks = %v, %v; want none and no error", got, err)
	}
	// Walk tables without the row-event log: walks are read, nothing superseded.
	noEvents, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer noEvents.Close()
	if err := localWriteWalk(noEvents, "numbers", "type=byoc", true, false, "2026-10-01T00:00:00Z", "test", []string{"pn-1"}); err != nil {
		t.Fatal(err)
	}
	if w, err := localReadWalksErr(noEvents.DB(), "numbers"); err != nil || !w["type=byoc"].usable() {
		t.Fatalf("walks without an event log = %+v, %v; want one usable walk", w, err)
	}
}

func TestLocalReadWalksUnreadableRowIsNotSkipped(t *testing.T) {
	s := localWalkStore(t)
	// A second walk whose completion flag cannot be decoded.
	if _, err := s.DB().Exec(`INSERT INTO zoom_sync_walks (resource, scope, walk_complete, full, synced_at, recorded_at) VALUES ('numbers', 'type=other', 'garbled', 0, '2026-10-01T00:00:00Z', 'test')`); err != nil {
		t.Fatal(err)
	}
	if got := localReadWalks(s.DB(), "numbers"); len(got) != 0 {
		t.Fatalf("an undecodable walk row left %d walks readable, want none: %+v", len(got), got)
	}
	if _, err := localReadWalksErr(s.DB(), "numbers"); err == nil {
		t.Fatal("localReadWalksErr: want a scan error")
	}
}

func TestLocalReadSyncScopeUnreadableRowIsNotSkipped(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := localWriteSyncScope(s, "users", 0, "", "2026-10-01T00:00:00Z", "test", true); err != nil {
		t.Fatal(err)
	}
	if got := localReadSyncScope(s.DB()); !got["users"].full || got["users"].filtered {
		t.Fatalf("users scope = %+v, want a full unfiltered record", got["users"])
	}
	if err := localWriteSyncScope(s, "sites", 0, "", "2026-10-01T00:00:00Z", "test", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`UPDATE zoom_sync_scope SET filtered = 'garbled' WHERE resource = 'sites'`); err != nil {
		t.Fatal(err)
	}
	if got := localReadSyncScope(s.DB()); len(got) != 0 {
		t.Fatalf("an undecodable scope row left %d records readable, want none: %+v", len(got), got)
	}
}

func TestLocalAbsorbKeepsEventsWhenWalkReadFails(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, ddl := range localSyncEventDDL {
		if _, err := s.DB().Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	// A walk-rows table that cannot answer the walk read.
	if _, err := s.DB().Exec(`CREATE TABLE zoom_sync_walk_rows (resource TEXT NOT NULL, other TEXT)`); err != nil {
		t.Fatal(err)
	}
	if err := localEnsureSyncScopeTable(s); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`INSERT INTO zoom_sync_walks (resource, scope, walk_complete, full, synced_at, recorded_at) VALUES ('numbers', 'type=byoc', 1, 0, '2026-10-01T00:00:00Z', 'test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`INSERT INTO zoom_row_events (resource_type, id) VALUES ('numbers', 'pn-2')`); err != nil {
		t.Fatal(err)
	}
	if err := localAbsorbUnattributedEvents(s); err == nil {
		t.Fatal("absorb succeeded although the walk could not be read; the supersession evidence would be discarded")
	}
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM zoom_row_events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("row events after a failed absorb = %d, want 1 (kept for the next run)", n)
	}
}

func TestLocalLocationCoverageNeedsReadableSitesCheckpoint(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := localWriteWalk(s, "locations", "site_id=site-A", true, false, "2026-10-01T00:00:00Z", "test", []string{"loc-1"}); err != nil {
		t.Fatal(err)
	}
	sites := []localSite{}
	if err := json.Unmarshal([]byte(`[{"id":"site-A","name":"A"}]`), &sites); err != nil {
		t.Fatal(err)
	}
	cov := localCoverage{synced: map[string]bool{"sites": true}, reasons: map[string]string{}}

	// Sites checkpoint recorded and not newer than the walk: covered.
	if err := localWriteSyncScope(s, "sites", 0, "", "2026-09-30T00:00:00Z", "test", true); err != nil {
		t.Fatal(err)
	}
	if lc := localLocationCoverageFor(s.DB(), cov, sites); !lc.complete(sites) {
		t.Fatalf("with a readable sites checkpoint: uncovered=%v, want covered", lc.uncovered)
	}
	// Sites checkpoint unreadable: no walk is accepted.
	q := localFailingQuerier{db: s.DB(), failOn: "zoom_sync_scope"}
	if lc := localLocationCoverageFor(q, cov, sites); lc.complete(sites) || len(lc.uncovered) != 1 {
		t.Fatalf("with an unreadable sites checkpoint: uncovered=%v, want site-A uncovered", lc.uncovered)
	}
}

// A store written before the walk schema was complete is opened read-only by
// the audit, which cannot migrate it: the audit still returns a result, with
// the BYOC check not covered, instead of failing on the schema.
func TestLocalE911DegradesOnLegacyWalkSchema(t *testing.T) {
	cases := []struct {
		name string
		ddl  []string
	}{
		{"walks without superseded and pending", []string{
			`DROP TABLE zoom_sync_walks`,
			`CREATE TABLE zoom_sync_walks (resource TEXT NOT NULL, scope TEXT NOT NULL, walk_complete INTEGER NOT NULL, full INTEGER NOT NULL, synced_at TEXT NOT NULL, recorded_at TEXT NOT NULL, PRIMARY KEY (resource, scope))`,
			`INSERT INTO zoom_sync_walks VALUES ('numbers', 'type=byoc', 1, 0, '2026-10-01T00:00:00Z', 'test')`,
		}},
		{"walks without the walk rows table", []string{
			`INSERT INTO zoom_sync_walks (resource, scope, walk_complete, full, synced_at, recorded_at) VALUES ('numbers', 'type=byoc', 1, 0, '2026-10-01T00:00:00Z', 'test')`,
			`DROP TABLE zoom_sync_walk_rows`,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := localTestStore(t, map[string][]string{"users": localTestUsers, "sites": {localTestHQ}})
			s, err := store.Open(db)
			if err != nil {
				t.Fatal(err)
			}
			if err := localEnsureSyncScopeTable(s); err != nil {
				t.Fatal(err)
			}
			for _, ddl := range tc.ddl {
				if _, err := s.DB().Exec(ddl); err != nil {
					t.Fatalf("%s: %v", ddl, err)
				}
			}
			_ = s.Close()

			stdout, _, err := localRun(t, newNovelE911AuditCmd, "--db", db)
			if err != nil {
				t.Fatalf("e911 audit failed on a legacy walk schema: %v", err)
			}
			var e localE911Result
			localDecode(t, stdout, &e)
			if e.Assessed || localByocFindings(e) != "" {
				t.Fatalf("legacy walk schema: assessed=%v byoc=%q, want unassessed with no BYOC findings", e.Assessed, localByocFindings(e))
			}
		})
	}
	// A genuine read error is not a legacy schema.
	if localLegacyWalkSchema(errors.New("disk I/O error")) || !localLegacyWalkSchema(errors.New("reading numbers walks: no such column: superseded")) {
		t.Fatal("localLegacyWalkSchema misclassifies")
	}
}

// Locations are covered only by an affirmative walk per site: sites rows
// whose own sync is not usable (a plain sync, or a scope record that cannot
// be read) leave locations missing instead of covered.
func TestLocalLocationsNotCoveredWithoutUsableSitesSync(t *testing.T) {
	cases := []struct {
		name string
		sql  string
	}{
		{"sites sync was not a full walk", `UPDATE zoom_sync_scope SET full = 0 WHERE resource = 'sites'`},
		{"an unrelated scope row cannot be read", `UPDATE zoom_sync_scope SET filtered = 'garbled' WHERE resource = 'users'`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := localTestStore(t, map[string][]string{"users": localTestUsers, "sites": {localTestHQ, localTestBoston}})
			s, err := store.Open(db)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB().Exec(tc.sql); err != nil {
				t.Fatal(err)
			}
			_ = s.Close()
			e := localE911(t, db)
			if e.Assessed || !localContains(e.MissingLocal, "locations") {
				t.Fatalf("assessed=%v missing_local=%v, want locations missing", e.Assessed, e.MissingLocal)
			}
		})
	}
	// Absence of a false negative: per-site walks for every site still cover.
	lc := localLocationCoverage{sites: map[string]map[string]bool{"site-hq": {}, "site-br": {}}, uncovered: []string{}}
	sites := []localSite{}
	if err := json.Unmarshal([]byte(`[`+localTestHQ+`,`+localTestBoston+`]`), &sites); err != nil {
		t.Fatal(err)
	}
	if !lc.complete(sites) {
		t.Fatal("every site walked: want complete")
	}
	if (localLocationCoverage{sites: map[string]map[string]bool{}, uncovered: []string{}}).complete(sites) {
		t.Fatal("no site walked and nothing listed uncovered: want not complete")
	}
}
