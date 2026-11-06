// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package revenue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"maxio-pp-cli/internal/client"
	"maxio-pp-cli/internal/cliutil/testenv"
	"maxio-pp-cli/internal/config"
	"maxio-pp-cli/internal/store"
)

// fakeInsights serves Maxio's three Insights surfaces as spec.json documents
// them: /mrr.json ({"mrr":{...}}), /stats.json ({"stats":{...}}),
// /subscriptions_mrr.json (per_page capped at 200) and /mrr_movements.json
// (ListMRRResponse; per_page capped at 50, oldest first, total_pages).
type fakeInsights struct {
	subs              int   // per-subscription MRR rows, 100 cents each
	failSubsPage      int   // a /subscriptions_mrr.json page answered 404
	failMovementsPage int   // a /mrr_movements.json page answered 404
	movements         []any // Movement objects, oldest first
}

func (f *fakeInsights) handler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	per, _ := strconv.Atoi(q.Get("per_page"))
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/mrr.json":
		_, _ = fmt.Fprintf(w, `{"mrr":{"amount_in_cents":%d,"at_time":"2026-10-29T00:00:00Z","currency":"USD"}}`, f.subs*100)
	case "/stats.json":
		_, _ = fmt.Fprintf(w, `{"stats":{"total_active_subscriptions":%d,"total_subscriptions":%d}}`, f.subs, f.subs)
	case "/subscriptions_mrr.json":
		if page == f.failSubsPage {
			http.Error(w, `{"errors":["not found"]}`, http.StatusNotFound)
			return
		}
		if per < 1 || per > 200 {
			per = 200
		}
		rows := []any{}
		for i := (page-1)*per + 1; i <= f.subs && i <= page*per; i++ {
			rows = append(rows, map[string]any{"subscription_id": i, "mrr_amount_in_cents": 100,
				"breakouts": map[string]any{"plan_amount_in_cents": 100, "usage_amount_in_cents": 0}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"subscriptions_mrr": rows})
	case "/mrr_movements.json":
		if page == f.failMovementsPage {
			http.Error(w, `{"errors":["not found"]}`, http.StatusNotFound)
			return
		}
		if per < 1 || per > 50 {
			per = 50
		}
		total := (len(f.movements) + per - 1) / per
		var rows []any
		for i := (page - 1) * per; i < len(f.movements) && i < page*per; i++ {
			rows = append(rows, f.movements[i])
		}
		if rows == nil {
			rows = []any{}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"mrr": map[string]any{"page": page, "per_page": per,
			"total_pages": total, "total_entries": len(f.movements), "currency": "USD", "movements": rows}})
	default:
		http.NotFound(w, r)
	}
}

func r1SyncFixture(t *testing.T, f *fakeInsights) (*client.Client, *sql.DB) {
	t.Helper()
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	c := client.New(&config.Config{BaseURL: srv.URL, MaxioApiKey: "r1Insights" + "TestKey0001"}, 10*time.Second, 0)
	c.NoCache = true
	st, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "r1.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return c, st.DB()
}

func movement(ts string, subID int, category string, cents int64) map[string]any {
	return map[string]any{
		"timestamp": ts, "amount_in_cents": cents, "amount_formatted": Cents(cents),
		"description": "Acme signed up for Plan ($100.00/mo)", "category": category,
		"breakouts":       map[string]any{"plan_amount_in_cents": cents, "usage_amount_in_cents": 0},
		"line_items":      []any{map[string]any{"product_id": 1, "component_id": 0, "name": "Plan", "mrr": cents}},
		"subscription_id": subID, "subscriber_name": "Acme",
	}
}

func scalar(t *testing.T, db *sql.DB, q string, args ...any) int64 {
	t.Helper()
	var n sql.NullInt64
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n.Int64
}

// Codex r1 (maxio 4.34.2 reprint) class movement-identity-incomplete: two
// documented Movement records that differ only in subscription_id (one
// customer starting two identical subscriptions together) are two movements.
func TestMovementIdentityKeepsEveryDocumentedMember(t *testing.T) {
	f := &fakeInsights{movements: []any{
		movement("2026-10-01T00:00:00Z", 101, "new_business", 10000),
		movement("2026-10-01T00:00:00Z", 102, "new_business", 10000),
	}}
	c, db := r1SyncFixture(t, f)
	if _, err := Sync(context.Background(), c, db, SyncOptions{SnapshotAt: "2026-10-29T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, db, `SELECT COUNT(*) FROM rev_mrr_movements`); n != 2 {
		t.Fatalf("stored %d movements, want 2 (one per subscription)", n)
	}
	if s := scalar(t, db, `SELECT SUM(amount_cents) FROM rev_mrr_movements WHERE bucket = ?`, BucketNew); s != 20000 {
		t.Fatalf("new business %d cents, want 20000", s)
	}
	// A re-sync of the same log adds nothing.
	if _, err := Sync(context.Background(), c, db, SyncOptions{SnapshotAt: "2026-10-29T01:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, db, `SELECT COUNT(*) FROM rev_mrr_movements`); n != 2 {
		t.Fatalf("a re-sync stored %d movements, want 2", n)
	}
}

// Codex r1 class incomplete-backfill-read-as-complete: a capped backfill
// (--max-pages 1) stores only the newest page; the next unlimited sync must
// not stop at that stored page as though the older pages were stored too.
func TestCappedBackfillIsResumedNotReadAsComplete(t *testing.T) {
	var mv []any
	for i := 0; i < 50; i++ {
		mv = append(mv, movement(fmt.Sprintf("2026-09-%02dT00:00:%02dZ", 1+i%28, i), 1000+i, "new_business", 100))
	}
	mv = append(mv, movement("2026-10-05T00:00:00Z", 1000, "expansion", 100))
	f := &fakeInsights{movements: mv}
	c, db := r1SyncFixture(t, f)
	res, err := Sync(context.Background(), c, db, SyncOptions{MaxMovementPages: 1, SnapshotAt: "2026-10-29T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if res.MovementsComplete || MovementsComplete(context.Background(), db) {
		t.Fatalf("a capped backfill reported complete: %+v", res)
	}
	res, err = Sync(context.Background(), c, db, SyncOptions{SnapshotAt: "2026-10-29T01:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, db, `SELECT COUNT(*) FROM rev_mrr_movements`); n != 51 {
		t.Fatalf("after an unlimited re-sync %d movements are stored, want 51 (the older page was never stored)", n)
	}
	if s := scalar(t, db, `SELECT COALESCE(SUM(amount_cents),0) FROM rev_mrr_movements WHERE ts < '2026-10-01'`); s != 5000 {
		t.Fatalf("September starting MRR %d cents, want 5000", s)
	}
	if !res.MovementsComplete || !MovementsComplete(context.Background(), db) {
		t.Fatalf("the full backfill did not report complete: %+v", res)
	}
	// Caught up: a complete history stops at the first stored page, and a
	// new movement at the end of the log is added.
	f.movements = append(f.movements, movement("2026-10-20T00:00:00Z", 1001, "expansion", 300))
	res, err = Sync(context.Background(), c, db, SyncOptions{SnapshotAt: "2026-10-29T02:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if res.MovementsInserted != 1 || !res.MovementsComplete || res.MovementPagesRead > 2 {
		t.Fatalf("an incremental sync of a complete history: %+v", res)
	}
	// A failed page is reported and leaves the history partial.
	for i := 0; i < 60; i++ {
		f.movements = append(f.movements, movement(fmt.Sprintf("2026-10-21T00:%02d:00Z", i), 2000+i, "new_business", 100))
	}
	srvFail := f.failMovementsPage
	f.failMovementsPage = 2
	res, err = Sync(context.Background(), c, db, SyncOptions{SnapshotAt: "2026-10-29T03:00:00Z"})
	f.failMovementsPage = srvFail
	if err != nil {
		t.Fatal(err)
	}
	if res.MovementsComplete || res.MovementsError == "" || MovementsComplete(context.Background(), db) {
		t.Fatalf("a failed movement page was read as complete: %+v", res)
	}
}

// The 0.1.x key (no subscription_id) is claimed by the movement it names, so
// an existing store is not duplicated by the new identity.
func TestLegacyMovementKeyIsClaimedNotDuplicated(t *testing.T) {
	a := movement("2026-10-01T00:00:00Z", 101, "new_business", 10000)
	f := &fakeInsights{movements: []any{a, movement("2026-10-01T00:00:00Z", 102, "new_business", 10000)}}
	c, db := r1SyncFixture(t, f)
	if err := EnsureSchema(db); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(a)
	var m rawMovement
	_ = json.Unmarshal(raw, &m)
	if _, err := db.Exec(`INSERT INTO rev_mrr_movements (mvmt_key, ts, amount_cents, category, bucket, plan_cents, usage_cents, description, line_items) VALUES (?,?,?,?,?,?,?,?,?)`,
		movementKey(m), m.Timestamp, m.AmountInCents, m.Category, BucketNew, 10000, 0, m.Description, string(m.LineItems)); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(context.Background(), c, db, SyncOptions{SnapshotAt: "2026-10-29T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, db, `SELECT COUNT(*) FROM rev_mrr_movements`); n != 2 {
		t.Fatalf("stored %d movements after re-keying, want 2", n)
	}
	if n := scalar(t, db, `SELECT COUNT(*) FROM rev_mrr_movements WHERE mvmt_key NOT LIKE 'm2:%'`); n != 0 {
		t.Fatalf("%d rows keep the legacy key", n)
	}
}

// Codex r1 class partial-snapshot-published: a per-subscription page that
// fails leaves no part of that snapshot behind, so readers keep the last
// complete snapshot.
func TestFailedSnapshotPublishesNothing(t *testing.T) {
	f := &fakeInsights{subs: 201}
	c, db := r1SyncFixture(t, f)
	if _, err := Sync(context.Background(), c, db, SyncOptions{SnapshotAt: "2026-10-28T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	f.failSubsPage = 2
	if _, err := Sync(context.Background(), c, db, SyncOptions{SnapshotAt: "2026-10-29T00:00:00Z"}); err == nil {
		t.Fatal("a failed per-subscription page returned no error")
	}
	if n := scalar(t, db, `SELECT COUNT(*) FROM rev_sub_mrr_snapshots WHERE snapshot_at = '2026-10-29T00:00:00Z'`); n != 0 {
		t.Fatalf("the failed sync left %d per-subscription rows", n)
	}
	if n := scalar(t, db, `SELECT COUNT(*) FROM rev_site_snapshots WHERE snapshot_at = '2026-10-29T00:00:00Z'`); n != 0 {
		t.Fatal("the failed sync left a site snapshot row")
	}
	var latest string
	_ = db.QueryRow(`SELECT MAX(snapshot_at) FROM rev_sub_mrr_snapshots`).Scan(&latest)
	if s := scalar(t, db, `SELECT SUM(mrr_cents) FROM rev_sub_mrr_snapshots WHERE snapshot_at = ?`, latest); s != 20100 {
		t.Fatalf("the latest snapshot sums %d cents, want 20100 (the last complete one)", s)
	}
}

// Class movement-identity-incomplete: the ordinal of identical records counts
// across the whole walk, so two identical same-second movements on either side
// of a 50-row page boundary stay two, and a re-sync adds nothing.
func TestIdenticalMovementsAcrossAPageBoundaryStayTwo(t *testing.T) {
	var mv []any
	for i := 0; i < 49; i++ {
		mv = append(mv, movement(fmt.Sprintf("2026-09-01T00:00:%02dZ", i), 1000+i, "new_business", 100))
	}
	twin := movement("2026-10-01T00:00:00Z", 77, "expansion", 500)
	mv = append(mv, twin, twin)
	f := &fakeInsights{movements: mv}
	c, db := r1SyncFixture(t, f)
	for i := 0; i < 2; i++ {
		if _, err := Sync(context.Background(), c, db, SyncOptions{SnapshotAt: fmt.Sprintf("2026-10-29T0%d:00:00Z", i)}); err != nil {
			t.Fatal(err)
		}
		if n := scalar(t, db, `SELECT COUNT(*) FROM rev_mrr_movements`); n != 51 {
			t.Fatalf("sync %d stored %d movements, want 51 (two identical expansions on pages 1 and 2)", i+1, n)
		}
	}
}

// A new identical movement can be the whole of the log's last page: its key is
// already stored, so that page looks caught up while the excess row is on the
// page before it. A caught-up stop needs two quiet pages.
func TestNewIdenticalMovementAloneOnTheLastPageIsNotLost(t *testing.T) {
	var mv []any
	for i := 0; i < 49; i++ {
		mv = append(mv, movement(fmt.Sprintf("2026-09-01T00:00:%02dZ", i), 1000+i, "new_business", 100))
	}
	twin := movement("2026-10-01T00:00:00Z", 77, "expansion", 500)
	mv = append(mv, twin)
	f := &fakeInsights{movements: mv}
	c, db := r1SyncFixture(t, f)
	if _, err := Sync(context.Background(), c, db, SyncOptions{SnapshotAt: "2026-10-29T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	f.movements = append(f.movements, twin)
	if _, err := Sync(context.Background(), c, db, SyncOptions{SnapshotAt: "2026-10-29T01:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, db, `SELECT COUNT(*) FROM rev_mrr_movements`); n != 51 {
		t.Fatalf("stored %d movements, want 51 (the second identical expansion is new)", n)
	}
}
