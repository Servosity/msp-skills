// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Adversarial review (maxio 4.34.2 reprint) wrong-result classes, each driven
// from its recorded failing input: response-cache-read-as-live,
// line-item-kind-unmapped, incremental-window-in-utc.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"maxio-pp-cli/internal/cliutil/testenv"
	"maxio-pp-cli/internal/store"
)

type r3Fake struct {
	srv     *httptest.Server
	siteMRR atomic.Int64
	mu      sync.Mutex
	queries map[string][]url.Values
	invoice bool
}

func newR3Fake(t *testing.T) *r3Fake {
	t.Helper()
	testenv.Isolate(t)
	f := &r3Fake{queries: map[string][]url.Values{}}
	f.siteMRR.Store(49000)
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.queries[r.URL.Path] = append(f.queries[r.URL.Path], r.URL.Query())
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mrr.json":
			fmt.Fprintf(w, `{"mrr":{"amount_in_cents":%d,"at_time":"2026-11-05T00:00:00Z","currency":"USD"}}`, f.siteMRR.Load())
		case "/stats.json":
			fmt.Fprint(w, `{"stats":{"total_active_subscriptions":1,"total_subscriptions":1}}`)
		case "/subscriptions_mrr.json":
			fmt.Fprint(w, `{"subscriptions_mrr":[]}`)
		case "/mrr_movements.json":
			fmt.Fprint(w, `{"mrr":{"page":1,"per_page":50,"total_pages":1,"total_entries":0,"currency":"USD","movements":[]}}`)
		case "/invoices.json":
			if f.invoice {
				fmt.Fprint(w, `{"invoices":[{"uid":"inv_r3","issue_date":"2026-10-15","status":"open","total_amount":"100.00"}]}`)
				return
			}
			fmt.Fprint(w, `[]`)
		default:
			fmt.Fprint(w, `[]`)
		}
	}))
	t.Cleanup(f.srv.Close)
	t.Setenv("MAXIO_API_KEY", "r3Review"+"Key0001")
	t.Setenv("MAXIO_BASE_URL", f.srv.URL)
	return f
}

func (f *r3Fake) run(t *testing.T, args ...string) map[string]any {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(append(args, "--json"))
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\nstderr: %s", args, err, errb.String())
	}
	var v map[string]any
	_ = json.Unmarshal(out.Bytes(), &v)
	return v
}

// Class response-cache-read-as-live: mrr sync and mrr now answered a re-run
// within five minutes from the on-disk response cache, so a changed site MRR
// was missed and the stale figure was saved under a fresh snapshot time.
func TestRevenueCommandsNeverAnswerFromTheResponseCache(t *testing.T) {
	f := newR3Fake(t)
	db := filepath.Join(t.TempDir(), "r3.db")
	f.run(t, "mrr", "sync", "--db", db)
	f.siteMRR.Store(50000)
	if v := f.run(t, "mrr", "sync", "--db", db); num(v["site_mrr_cents"]) != 50000 {
		t.Errorf("a second mrr sync read site MRR %v, want 50000 (the API's current answer): %v", v["site_mrr_cents"], v)
	}
	f.siteMRR.Store(51000)
	if v := f.run(t, "mrr", "now", "--db", db); num(v["mrr_cents"]) != 51000 {
		t.Errorf("mrr now read %v, want 51000 (the API's current answer): %v", v["mrr_cents"], v)
	}
}

// Class line-item-kind-unmapped: spec.json LineItemKind documents
// prepaid_usage_component and event_based_component, and the /mrr_movements.json
// breakouts count prepaid usage as usage; revenue-mix called them services.
func TestEveryDocumentedUsageKindIsUsage(t *testing.T) {
	f := newR1Fixture(t)
	f.put("invoices-json", "inv_u", `{"uid":"inv_u","status":"open","issue_date":"2026-10-03","total_amount":"15.00","line_items":[`+
		`{"kind":"prepaid_usage_component","total_amount":"4.00"},{"kind":"event_based_component","total_amount":"6.00"},`+
		`{"kind":"metered_component","total_amount":"5.00"}]}`)
	v := f.run("revenue-mix", "--month", "2026-10")
	if num(v["usage_cents"]) != 1500 || num(v["services_cents"]) != 0 {
		t.Errorf("revenue-mix usage %v services %v, want 1500 and 0: %v", v["usage_cents"], v["services_cents"], v)
	}
}

// Class incremental-window-in-utc: an incremental sync sent the UTC date of the
// last sync as start_date, which Maxio reads as midnight in the site's time
// zone (spec.json /subscriptions.json start_date), and filtered on the
// endpoint's default date field; a change made in the evening at a site west
// of UTC, or to an older record, was never fetched.
func TestIncrementalSyncCoversEverySiteZoneAndUpdates(t *testing.T) {
	f := newR3Fake(t)
	db := filepath.Join(t.TempDir(), "r3.db")
	before := time.Now().Add(-time.Hour).UTC()
	f.run(t, "sync", "--resources", "subscriptions-json,invoices-json,customers-json", "--since", "1h", "--db", db)
	after := time.Now().Add(-time.Hour).UTC()
	want := map[string]bool{before.AddDate(0, 0, -1).Format("2006-01-02"): true, after.AddDate(0, 0, -1).Format("2006-01-02"): true}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, path := range []string{"/subscriptions.json", "/invoices.json", "/customers.json"} {
		qs := f.queries[path]
		if len(qs) == 0 {
			t.Errorf("%s was never asked", path)
			continue
		}
		if q := qs[0]; !want[q.Get("start_date")] || q.Get("date_field") != "updated_at" {
			t.Errorf("%s: start_date=%q date_field=%q, want a day before the UTC date (%v) and updated_at", path, q.Get("start_date"), q.Get("date_field"), want)
		}
	}
}

// A user's own date window keeps the meaning it had on the first run: the
// incremental updated_at switch rides only with the bound sync computed.
func TestUserDateWindowIsNeverReadAsUpdatedAt(t *testing.T) {
	f := newR3Fake(t)
	db := filepath.Join(t.TempDir(), "r3.db")
	f.run(t, "sync", "--resources", "invoices-json", "--since", "1h", "--param", "start_date=2026-10-01", "--param", "end_date=2026-10-31", "--db", db)
	f.run(t, "sync", "--resources", "customers-json", "--since", "1h", "--param", "end_date=2026-10-31", "--param", "date_field=updated_at", "--db", db)
	f.mu.Lock()
	defer f.mu.Unlock()
	if qs := f.queries["/invoices.json"]; len(qs) == 0 || qs[0].Get("start_date") != "2026-10-01" || qs[0].Has("date_field") {
		t.Errorf("invoices: %v, want the user's window with no date_field", qs)
	}
	if qs := f.queries["/customers.json"]; len(qs) == 0 || qs[0].Get("date_field") != "updated_at" {
		t.Errorf("customers: %v, want the user's own date_field kept", qs)
	}
}

// With a user's date window and no --since, the second run (which has a stored
// sync bound) sends exactly what the first run sent, for every window shape.
func TestUserDateWindowSecondRunAsksWhatTheFirstAsked(t *testing.T) {
	for _, window := range [][]string{
		{"--param", "start_date=2026-10-01", "--param", "end_date=2026-10-31"},
		{"--param", "end_date=2026-11-05"},
		{"--global-param", "end_datetime=2026-12-31 23:59:59"},
		{"--resource-param", "invoices-json:end_date=2026-10-31"},
	} {
		f := newR3Fake(t)
		f.invoice = true
		db := filepath.Join(t.TempDir(), "r3.db")
		args := append([]string{"sync", "--resources", "invoices-json", "--db", db}, window...)
		f.run(t, args...)
		f.run(t, args...)
		f.mu.Lock()
		qs := f.queries["/invoices.json"]
		f.mu.Unlock()
		if len(qs) < 2 || qs[0].Encode() != qs[len(qs)-1].Encode() {
			t.Errorf("%v: first %v, second %v; want the same query", window, qs[0], qs[len(qs)-1])
		}
	}
}

// r3Invoice is one invoice on a fake that filters the way spec.json says
// /invoices.json does: date_field defaults to issue_date, start_date and
// end_date bound it, status narrows it.
type r3Invoice struct{ uid, issued, updated, status string }

func newR3InvoiceFake(t *testing.T, list func() []r3Invoice) *r3Fake {
	t.Helper()
	f := newR3Fake(t)
	f.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f.mu.Lock()
		f.queries[r.URL.Path] = append(f.queries[r.URL.Path], q)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/invoices.json" {
			fmt.Fprint(w, `[]`)
			return
		}
		var kept []map[string]string
		for _, inv := range list() {
			date := inv.issued
			if q.Get("date_field") == "updated_at" {
				date = inv.updated
			}
			start := q.Get("start_date")
			if s := q.Get("start_datetime"); s != "" {
				start = s[:10]
			}
			end := q.Get("end_date")
			if s := q.Get("end_datetime"); s != "" {
				end = s[:10]
			}
			if (start != "" && date < start) || (end != "" && date > end) || (q.Get("status") != "" && q.Get("status") != inv.status) {
				continue
			}
			kept = append(kept, map[string]string{"uid": inv.uid, "issue_date": inv.issued, "updated_at": inv.updated, "status": inv.status})
		}
		page, _ := strconv.Atoi(q.Get("page"))
		perPage, _ := strconv.Atoi(q.Get("per_page"))
		if page < 1 {
			page = 1
		}
		if perPage < 1 {
			perPage = len(kept) + 1
		}
		from := min((page-1)*perPage, len(kept))
		body, _ := json.Marshal(map[string]any{"invoices": kept[from:min(from+perPage, len(kept))]})
		_, _ = w.Write(body)
	})
	return f
}

func r3Exec(t *testing.T, db, query string, args ...any) {
	t.Helper()
	st, err := store.OpenWithContext(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.DB().Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func r3Stored(t *testing.T, db, uid string) (status string, syncedAt time.Time) {
	t.Helper()
	st, err := store.OpenReadOnlyContext(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	raw, err := st.Get("invoices-json", uid)
	if err != nil {
		t.Fatalf("%s is not stored: %v", uid, err)
	}
	var row struct{ Status string }
	if err := json.Unmarshal(raw, &row); err != nil {
		t.Fatal(err)
	}
	_, syncedAt, _, err = st.GetSyncState("invoices-json")
	if err != nil {
		t.Fatal(err)
	}
	return row.Status, syncedAt
}

// Class user-window-advances-sync-bound: the stored sync time says every change
// since it was seen, so only a run that covered everything since it may move it.
// The recorded failing input: plain sync; void an older invoice; a narrowed run
// that does not fetch it; plain sync. Every narrowing below left the void unseen
// for good.
func TestOnlyARunCoveringTheStoredBoundMovesIt(t *testing.T) {
	day := func(n int) string { return time.Now().UTC().AddDate(0, 0, -n).Format("2006-01-02") }
	for _, tc := range []struct {
		name    string
		narrow  []string
		advance bool
	}{
		{"start_date", []string{"--param", "start_date=" + day(12)}, false},
		{"per-resource start and end", []string{"--resource-param", "invoices-json:start_date=" + day(12), "--resource-param", "invoices-json:end_date=" + day(1)}, false},
		{"--since shorter than the gap", []string{"--since", "1h"}, false},
		{"--since older than the bound", []string{"--since", "30d"}, true},
		{"a plain sync", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv := r3Invoice{"inv_x", day(20), day(20), "open"}
			f := newR3InvoiceFake(t, func() []r3Invoice { return []r3Invoice{inv} })
			db := filepath.Join(t.TempDir(), "r3.db")
			f.run(t, "sync", "--resources", "invoices-json", "--db", db)
			stored := time.Now().UTC().AddDate(0, 0, -10).Truncate(time.Second)
			r3Exec(t, db, `UPDATE sync_state SET last_synced_at = ? WHERE resource_type = 'invoices-json'`, stored.Format(time.RFC3339))
			inv.updated, inv.status = day(5), "voided"

			f.run(t, append([]string{"sync", "--resources", "invoices-json", "--db", db}, tc.narrow...)...)
			_, at := r3Stored(t, db, "inv_x")
			if moved := !at.Equal(stored); moved != tc.advance {
				t.Errorf("stored sync time %v after the run (was %v), want moved=%v", at, stored, tc.advance)
			}

			f.run(t, "sync", "--resources", "invoices-json", "--db", db)
			if status, _ := r3Stored(t, db, "inv_x"); status != "voided" {
				t.Errorf("after the plain sync inv_x is %q, want voided", status)
			}
		})
	}
}

// Every shape of narrowing, without the cost of a sync per shape.
func TestSyncRunReach(t *testing.T) {
	stored := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	window := map[string]string{"start_date": "2026-10-20"}
	filter := map[string]string{"status": "open"}
	const before, after = "2026-10-01T00:00:00Z", "2026-11-01T00:00:00Z"
	for _, tc := range []struct {
		name          string
		p             *syncUserParams
		since, param  string
		plain, covers bool
	}{
		{"plain", nil, "", "start_date", true, true},
		{"no user params", &syncUserParams{}, "", "start_date", true, true},
		{"--since on a resource with no bound param", nil, after, "", true, true},
		{"--since older than the stored time", nil, before, "start_date", false, true},
		{"--since equal to the stored time", nil, stored.Format(time.RFC3339), "start_date", false, true},
		{"--since later than the stored time", nil, after, "start_date", false, false},
		{"--param window", &syncUserParams{flatGlobal: window}, "", "start_date", false, false},
		{"--global-param window", &syncUserParams{trueGlobal: window}, "", "start_date", false, false},
		{"--resource-param window", &syncUserParams{perResource: map[string]map[string]string{"invoices-json": window}}, "", "start_date", false, false},
		{"--param end only", &syncUserParams{flatGlobal: map[string]string{"end_date": "2026-10-31"}}, "", "start_date", false, false},
		{"--param non-date filter", &syncUserParams{flatGlobal: filter}, "", "start_date", false, false},
		{"--param with an older --since", &syncUserParams{flatGlobal: window}, before, "start_date", false, false},
		{"another resource's param", &syncUserParams{perResource: map[string]map[string]string{"customers-json": window}}, "", "start_date", true, true},
	} {
		plain, covers := syncRunReach(tc.p, "invoices-json", tc.since, tc.param, stored)
		if plain != tc.plain || covers != tc.covers {
			t.Errorf("%s: plain=%v covers=%v, want %v %v", tc.name, plain, covers, tc.plain, tc.covers)
		}
	}
	if _, covers := syncRunReach(nil, "invoices-json", before, "start_date", time.Time{}); covers {
		t.Error("--since on a store with no stored time reads as covering it")
	}
}

// A narrowed run on a store with no sync time must not create one: the next
// plain sync still walks everything the narrowed run skipped.
func TestANarrowedFirstRunLeavesNoStoredBound(t *testing.T) {
	old := r3Invoice{"inv_old", "2020-01-15", "2020-01-15", "paid"}
	today := time.Now().UTC().Format("2006-01-02")
	cur := r3Invoice{"inv_cur", today, today, "open"}
	f := newR3InvoiceFake(t, func() []r3Invoice { return []r3Invoice{old, cur} })
	db := filepath.Join(t.TempDir(), "r3.db")
	f.run(t, "sync", "--resources", "invoices-json", "--param", "status=open", "--db", db)
	if _, at := r3Stored(t, db, "inv_cur"); !at.IsZero() {
		t.Errorf("a narrowed first run wrote a stored sync time %v", at)
	}
	f.run(t, "sync", "--resources", "invoices-json", "--db", db)
	if status, _ := r3Stored(t, db, "inv_old"); status != "paid" {
		t.Errorf("after a narrowed run and a plain sync inv_old is %q, want paid", status)
	}
}

// A resume cursor belongs to the query that made it. A narrowed run neither
// leaves one for a plain sync nor starts from a plain sync's, or the first pages
// of the other query are skipped and the walk still reads as complete.
func TestANarrowedRunStaysOffTheResumeCursor(t *testing.T) {
	const perPage = 20 // determinePaginationDefaults for invoices-json
	var all []r3Invoice
	for i := range perPage + 5 {
		all = append(all, r3Invoice{fmt.Sprintf("inv_%03d", i), "2026-10-15", "2026-10-15", "open"})
	}
	t.Run("a narrowed run leaves no cursor", func(t *testing.T) {
		f := newR3InvoiceFake(t, func() []r3Invoice { return all })
		db := filepath.Join(t.TempDir(), "r3.db")
		f.run(t, "sync", "--resources", "invoices-json", "--param", "status=open", "--max-pages", "1", "--db", db)
		r3Exec(t, db, `DELETE FROM resources WHERE resource_type = 'invoices-json' AND id = 'inv_000'`)
		f.run(t, "sync", "--resources", "invoices-json", "--db", db)
		if status, _ := r3Stored(t, db, "inv_000"); status != "open" {
			t.Errorf("inv_000 on the first page is %q after the plain sync, want open", status)
		}
	})
	t.Run("a narrowed run starts at its own first page", func(t *testing.T) {
		f := newR3InvoiceFake(t, func() []r3Invoice { return all })
		db := filepath.Join(t.TempDir(), "r3.db")
		f.run(t, "sync", "--resources", "invoices-json", "--max-pages", "1", "--db", db)
		f.mu.Lock()
		f.queries = map[string][]url.Values{}
		f.mu.Unlock()
		f.run(t, "sync", "--resources", "invoices-json", "--param", "status=open", "--max-pages", "1", "--db", db)
		f.mu.Lock()
		defer f.mu.Unlock()
		if qs := f.queries["/invoices.json"]; len(qs) != 1 || qs[0].Get("page") != "1" {
			t.Errorf("the narrowed run asked %v, want page 1", qs)
		}
	})
}
