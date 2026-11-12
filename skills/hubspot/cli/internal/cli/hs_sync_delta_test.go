// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-written (handfixes.json sync-delta-search-walk, issue #695): a second
// sync of a CRM object walks POST .../search from the watermark instead of
// re-reading the whole list, and falls back to the list walk whenever a delta
// cannot be proven equivalent.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"hubspot-pp-cli/internal/cliutil/testenv"
	"hubspot-pp-cli/internal/store"
)

type deltaRecord struct {
	id       string
	modified time.Time
}

type deltaSearch struct {
	FilterGroups []struct {
		Filters []struct {
			PropertyName string `json:"propertyName"`
			Operator     string `json:"operator"`
			Value        string `json:"value"`
		} `json:"filters"`
	} `json:"filterGroups"`
	Sorts []struct {
		PropertyName string `json:"propertyName"`
		Direction    string `json:"direction"`
	} `json:"sorts"`
	Limit      int      `json:"limit"`
	After      string   `json:"after"`
	Properties []string `json:"properties"`
}

// deltaPortal serves one HubSpot object type the way HubSpot does: a cursor
// list (GET) and a search (POST) that filters, sorts ascending, pages by
// after and refuses after >= 10,000.
type deltaPortal struct {
	mu       sync.Mutex
	obj      string
	records  []deltaRecord
	gets     int
	searches []deltaSearch
	// listAfters is the after= of every list GET, in order.
	listAfters []string
	failAt     int // search request number (1-based) that answers 400; 0 = never
	// afterSearch runs with the lock held once search request n is answered.
	afterSearch func(p *deltaPortal, n int)
}

func (p *deltaPortal) set(recs []deltaRecord) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.records = append([]deltaRecord(nil), recs...)
}

func (p *deltaPortal) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.gets, p.searches, p.listAfters = 0, nil, nil
}

func (p *deltaPortal) counts() (int, []deltaSearch) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.gets, append([]deltaSearch(nil), p.searches...)
}

func deltaModifiedProp(obj string) string {
	if obj == "contacts" {
		return "lastmodifieddate"
	}
	return "hs_lastmodifieddate"
}

func (p *deltaPortal) row(r deltaRecord) string {
	props := map[string]any{"hs_object_id": r.id, deltaModifiedProp(p.obj): r.modified.UTC().Format("2006-01-02T15:04:05.000Z")}
	b, _ := json.Marshal(map[string]any{"id": r.id, "properties": props, "createdAt": "2026-01-01T00:00:00Z", "updatedAt": r.modified.UTC().Format(time.RFC3339Nano), "archived": false})
	return string(b)
}

func newDeltaPortal(t *testing.T, obj string, recs []deltaRecord) *deltaPortal {
	t.Helper()
	p := &deltaPortal{obj: obj}
	p.set(recs)
	base := "/crm/v3/objects/" + obj
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p.mu.Lock()
		defer p.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == base:
			p.gets++
			p.listAfters = append(p.listAfters, r.URL.Query().Get("after"))
			if r.URL.Query().Get("archived") == "true" { // every record here is active
				_, _ = fmt.Fprint(w, `{"results":[]}`)
				return
			}
			after, _ := strconv.Atoi(r.URL.Query().Get("after"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if limit <= 0 {
				limit = 10
			}
			var rows []string
			for i := after; i < len(p.records) && i < after+limit; i++ {
				rows = append(rows, p.row(p.records[i]))
			}
			paging := ""
			if after+limit < len(p.records) {
				paging = fmt.Sprintf(`,"paging":{"next":{"after":"%d"}}`, after+limit)
			}
			_, _ = fmt.Fprint(w, `{"results":[`+strings.Join(rows, ",")+`]`+paging+`}`)
		case r.Method == http.MethodPost && r.URL.Path == base+"/search":
			var q deltaSearch
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &q); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			p.searches = append(p.searches, q)
			after, _ := strconv.Atoi(q.After)
			if (p.failAt > 0 && len(p.searches) == p.failAt) || after >= 10000 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprint(w, `{"status":"error","category":"VALIDATION_ERROR","message":"rejected"}`)
				return
			}
			var lower int64
			if len(q.FilterGroups) == 1 && len(q.FilterGroups[0].Filters) == 1 {
				lower, _ = strconv.ParseInt(q.FilterGroups[0].Filters[0].Value, 10, 64)
			}
			var hits []deltaRecord
			for _, rec := range p.records {
				if rec.modified.UnixMilli() >= lower {
					hits = append(hits, rec)
				}
			}
			sort.SliceStable(hits, func(i, j int) bool { return hits[i].modified.Before(hits[j].modified) })
			limit := q.Limit
			var rows []string
			for i := after; i < len(hits) && i < after+limit; i++ {
				rows = append(rows, p.row(hits[i]))
			}
			paging := ""
			if after+limit < len(hits) {
				paging = fmt.Sprintf(`,"paging":{"next":{"after":"%d"}}`, after+limit)
			}
			_, _ = fmt.Fprintf(w, `{"total":%d,"results":[%s]%s}`, len(hits), strings.Join(rows, ","), paging)
			if p.afterSearch != nil {
				p.afterSearch(p, len(p.searches))
			}
		default:
			_, _ = fmt.Fprint(w, `{"results":[]}`)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HUBSPOT_BASE_URL", srv.URL)
	t.Setenv("HUBSPOT_ACCESS_TOKEN", "pat-na1-"+"7f3c9e2a-b14d-4c8e-9a6f-2d5e8b1c7a32")
	return p
}

func deltaRecords(n int, at time.Time, step time.Duration) []deltaRecord {
	out := make([]deltaRecord, n)
	for i := range out {
		out[i] = deltaRecord{id: strconv.Itoa(i + 1), modified: at.Add(time.Duration(i) * step)}
	}
	return out
}

func deltaSync(t *testing.T, path, resource string, extra ...string) string {
	t.Helper()
	args := append([]string{"sync", "--resources", resource, "--db", path, "--json", "--rate-limit", "0", "--concurrency", "1"}, extra...)
	_, stderr, _ := r3dRun(t, args...)
	return stderr
}

func deltaState(t *testing.T, path, resource string) (time.Time, int) {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var at string
	var complete int
	if err := db.DB().QueryRow(`SELECT COALESCE(last_synced_at, ''), last_attempt_complete FROM sync_state WHERE resource_type = ?`, resource).Scan(&at, &complete); err != nil {
		t.Fatalf("sync_state for %s: %v", resource, err)
	}
	w, _ := time.Parse(time.RFC3339, at)
	return w, complete
}

func deltaStoredModified(t *testing.T, path, resource, id string) string {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	raw, err := db.Get(resource, id)
	if err != nil {
		return ""
	}
	var row struct {
		Properties map[string]any `json:"properties"`
	}
	_ = json.Unmarshal(raw, &row)
	s, _ := row.Properties[deltaModifiedProp(strings.TrimSuffix(strings.TrimPrefix(resource, "hubspot-"), "-crm"))].(string)
	return s
}

var deltaOld = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Run 2 with a watermark POSTs search with the right filter, sort and
// properties, fetches only the changed rows, never GETs the list, and the
// overlap is 5 minutes. Contacts filter on lastmodifieddate.
func TestSyncDeltaSecondRunSearchesFromTheWatermark(t *testing.T) {
	for _, tc := range []struct{ obj, resource string }{
		{"emails", "hubspot-emails-crm"},
		{"contacts", "hubspot-contacts-crm"},
	} {
		t.Run(tc.obj, func(t *testing.T) {
			testenv.Isolate(t)
			recs := deltaRecords(250, deltaOld, time.Minute)
			p := newDeltaPortal(t, tc.obj, recs)
			path := filepath.Join(t.TempDir(), "d.db")
			deltaSync(t, path, tc.resource)
			if gets, s := p.counts(); gets == 0 || len(s) != 0 {
				t.Fatalf("run 1: gets=%d searches=%d, want a list walk", gets, len(s))
			}
			watermark, _ := deltaState(t, path, tc.resource)
			if watermark.IsZero() {
				t.Fatal("run 1 left no watermark")
			}

			changed := time.Now().UTC().Truncate(time.Millisecond)
			recs[7].modified = changed
			recs[42].modified = changed.Add(time.Millisecond)
			p.set(recs)
			p.reset()
			events := deltaSync(t, path, tc.resource)

			gets, searches := p.counts()
			if gets != 0 {
				t.Fatalf("run 2 GET the list %d times; a delta walk must not\n%s", gets, events)
			}
			if len(searches) != 1 {
				t.Fatalf("run 2 sent %d searches, want 1\n%s", len(searches), events)
			}
			q := searches[0]
			prop := deltaModifiedProp(tc.obj)
			if len(q.FilterGroups) != 1 || len(q.FilterGroups[0].Filters) != 1 {
				t.Fatalf("filterGroups = %+v, want one filter", q.FilterGroups)
			}
			f := q.FilterGroups[0].Filters[0]
			wantFrom := watermark.Add(-5 * time.Minute).UnixMilli()
			if f.PropertyName != prop || f.Operator != "GTE" || f.Value != strconv.FormatInt(wantFrom, 10) {
				t.Fatalf("filter = %+v, want %s GTE %d (watermark minus 5 minutes)", f, prop, wantFrom)
			}
			if len(q.Sorts) != 1 || q.Sorts[0].PropertyName != prop || q.Sorts[0].Direction != "ASCENDING" {
				t.Fatalf("sorts = %+v, want %s ASCENDING", q.Sorts, prop)
			}
			if q.Limit != 200 {
				t.Fatalf("limit = %d, want 200", q.Limit)
			}
			if got, want := strings.Join(q.Properties, ","), hsCollectedProperties(tc.resource, ""); got != want {
				t.Fatalf("properties = %q, want the list read's %q", got, want)
			}
			if !strings.Contains(events, `"event":"sync_delta"`) || !strings.Contains(events, `"fetched":2`) {
				t.Fatalf("no sync_delta event naming 2 fetched rows:\n%s", events)
			}
			if strings.Contains(events, "resource_not_incremental") {
				t.Fatalf("resource_not_incremental fired on a delta walk:\n%s", events)
			}
			if got := deltaStoredModified(t, path, tc.resource, "8"); !strings.HasPrefix(got, changed.Format("2006-01-02T15:04")) {
				t.Fatalf("row 8 stored modified %q, want the changed value", got)
			}
			after, complete := deltaState(t, path, tc.resource)
			if after.Before(watermark) || complete != 1 {
				t.Fatalf("watermark after delta = %v complete=%d, want it stamped and complete", after, complete)
			}
		})
	}
}

// Every page is a fresh query from the previous page's last timestamp, never
// an after offset, so a walk past 10,000 changed rows never reaches HubSpot's
// 10,000-result window (after=10000 answers 400).
func TestSyncDeltaPagesRestartFromTheLastTimestamp(t *testing.T) {
	testenv.Isolate(t)
	p := newDeltaPortal(t, "emails", deltaRecords(1, deltaOld, 0))
	path := filepath.Join(t.TempDir(), "d.db")
	resource := "hubspot-emails-crm"
	deltaSync(t, path, resource)
	start := time.Now().UTC().Add(time.Minute).Truncate(time.Millisecond)
	recs := deltaRecords(10050, start, time.Millisecond)
	p.set(recs)
	p.reset()
	events := deltaSync(t, path, resource)
	gets, searches := p.counts()
	if gets != 0 {
		t.Fatalf("GET the list %d times\n%s", gets, events)
	}
	// 200 rows a page, each page re-reading the previous page's last row.
	if len(searches) != 51 {
		t.Fatalf("sent %d searches, want 51\n%s", len(searches), events)
	}
	for k, q := range searches {
		if q.After != "" {
			t.Fatalf("search %d paged by after=%s; every page must be a fresh query", k+1, q.After)
		}
		if k == 0 {
			continue
		}
		if want := strconv.FormatInt(recs[199*k].modified.UnixMilli(), 10); q.FilterGroups[0].Filters[0].Value != want {
			t.Fatalf("search %d GTE %s, want the previous page's last timestamp %s", k+1, q.FilterGroups[0].Filters[0].Value, want)
		}
	}
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if n, _ := db.Count(resource); n != 10050 {
		t.Fatalf("stored %d rows, want 10050", n)
	}
	if !strings.Contains(events, `"event":"sync_delta"`) {
		t.Fatalf("no sync_delta event:\n%s", events)
	}
}

// Codex round 1 P1: search pages by position, so a record updated between
// two pages moves to the end and shifts the rest one place forward. An offset
// walk then skips the row that moved into the gap and stamps a watermark past
// it. 201 changed rows; row 1 changes again once page 1 is answered: row 201
// must still be stored with its change.
func TestSyncDeltaUpdateMidWalkSkipsNoRow(t *testing.T) {
	testenv.Isolate(t)
	resource := "hubspot-emails-crm"
	p := newDeltaPortal(t, "emails", deltaRecords(1, deltaOld, 0))
	path := filepath.Join(t.TempDir(), "d.db")
	deltaSync(t, path, resource)
	start := time.Now().UTC().Add(time.Minute).Truncate(time.Millisecond)
	recs := deltaRecords(201, start, time.Millisecond)
	p.set(recs)
	p.reset()
	p.mu.Lock()
	p.afterSearch = func(p *deltaPortal, n int) {
		if n == 1 {
			p.records[0].modified = start.Add(time.Hour)
		}
	}
	p.mu.Unlock()
	events := deltaSync(t, path, resource)
	if got := deltaStoredModified(t, path, resource, "201"); got != recs[200].modified.Format("2006-01-02T15:04:05.000Z") {
		t.Fatalf("row 201 stored modified %q, want its change %s: the walk skipped it\n%s", got, recs[200].modified.Format(time.RFC3339Nano), events)
	}
	if got := deltaStoredModified(t, path, resource, "1"); got != start.Add(time.Hour).Format("2006-01-02T15:04:05.000Z") {
		t.Fatalf("row 1 stored modified %q, want its second change", got)
	}
}

// Codex round 1 P1: a list walk narrowed by --param (archived=true) must not
// leave a watermark the next unfiltered delta trusts. Contact 1 changed after
// the last full sync but more than 5 minutes before the filtered run.
func TestSyncDeltaFilteredRunDoesNotAdvanceTheWatermark(t *testing.T) {
	testenv.Isolate(t)
	resource := "hubspot-contacts-crm"
	recs := deltaRecords(3, deltaOld, time.Minute)
	p := newDeltaPortal(t, "contacts", recs)
	path := filepath.Join(t.TempDir(), "d.db")
	deltaSync(t, path, resource)
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`UPDATE sync_state SET last_synced_at = ? WHERE resource_type = ?`, time.Now().UTC().Add(-2*time.Hour).Format(time.RFC3339), resource); err != nil {
		t.Fatal(err)
	}
	db.Close()
	changed := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	recs[0].modified = changed
	p.set(recs)
	deltaSync(t, path, resource, "--param", "archived=true")
	events := deltaSync(t, path, resource)
	if got := deltaStoredModified(t, path, resource, "1"); got != changed.Format("2006-01-02T15:04:05.000Z") {
		t.Fatalf("contact 1 stored modified %q, want its change %s\n%s", got, changed.Format(time.RFC3339Nano), events)
	}
}

// A full page on one timestamp cannot advance: the resource falls back to a
// full list read.
func TestSyncDeltaOneTimestampPageFallsBackToTheList(t *testing.T) {
	testenv.Isolate(t)
	p := newDeltaPortal(t, "emails", deltaRecords(1, deltaOld, 0))
	path := filepath.Join(t.TempDir(), "d.db")
	resource := "hubspot-emails-crm"
	deltaSync(t, path, resource)
	same := time.Now().UTC().Add(time.Minute).Truncate(time.Millisecond)
	p.set(deltaRecords(10050, same, 0))
	p.reset()
	events := deltaSync(t, path, resource)
	gets, searches := p.counts()
	if len(searches) != 1 || gets == 0 {
		t.Fatalf("searches=%d gets=%d, want one search page then the list walk\n%s", len(searches), gets, events)
	}
	if !strings.Contains(events, `"reason":"page_one_timestamp"`) {
		t.Fatalf("no fall-back event naming the one-timestamp window:\n%s", events)
	}
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if n, _ := db.Count(resource); n != 10050 {
		t.Fatalf("stored %d rows, want 10050", n)
	}
}

// Every fall-back condition takes the unchanged list walk.
func TestSyncDeltaFallBacksTakeTheListWalk(t *testing.T) {
	cases := []struct {
		name   string
		obj    string
		before func(t *testing.T, path, resource string)
		extra  []string
	}{
		{name: "no-watermark", obj: "emails"},
		{name: "empty-table", obj: "emails", before: func(t *testing.T, path, resource string) {
			db, err := store.OpenWithContext(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.DB().Exec(`DELETE FROM resources WHERE resource_type = ?`, resource); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "full", obj: "emails", extra: []string{"--full"}},
		{name: "with-history", obj: "contacts", extra: []string{"--with-history", "lifecyclestage"}},
		{name: "user-property-added", obj: "emails", extra: []string{"--param", "properties=hs_email_direction"}},
		{name: "collected-properties-changed", obj: "emails", before: func(t *testing.T, _, resource string) {
			old := hsReportProperties[resource]
			hsReportProperties[resource] = append(append([]string(nil), old...), "hs_email_status")
			t.Cleanup(func() { hsReportProperties[resource] = old })
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			resource := "hubspot-" + tc.obj + "-crm"
			p := newDeltaPortal(t, tc.obj, deltaRecords(30, deltaOld, time.Minute))
			path := filepath.Join(t.TempDir(), "d.db")
			if tc.name != "no-watermark" {
				deltaSync(t, path, resource)
			}
			if tc.before != nil {
				tc.before(t, path, resource)
			}
			p.reset()
			events := deltaSync(t, path, resource, tc.extra...)
			gets, searches := p.counts()
			if gets == 0 || len(searches) != 0 {
				t.Fatalf("gets=%d searches=%d, want the list walk\n%s", gets, len(searches), events)
			}
			if strings.Contains(events, "resource_not_incremental") {
				t.Fatalf("resource_not_incremental fired for an object resource:\n%s", events)
			}
			if tc.name == "with-history" && !strings.Contains(events, `"reason":"with_history"`) {
				t.Fatalf("no sync_delta_fallback naming with_history:\n%s", events)
			}
		})
	}
}

// A delta walk never prunes: a record gone from HubSpot stays until the
// weekly sync --full reconciles it.
func TestSyncDeltaDoesNotPrune(t *testing.T) {
	testenv.Isolate(t)
	resource := "hubspot-emails-crm"
	recs := deltaRecords(5, deltaOld, time.Minute)
	p := newDeltaPortal(t, "emails", recs)
	path := filepath.Join(t.TempDir(), "d.db")
	deltaSync(t, path, resource)
	recs = recs[:4]
	recs[0].modified = time.Now().UTC()
	p.set(recs)
	p.reset()
	events := deltaSync(t, path, resource)
	if _, s := p.counts(); len(s) == 0 {
		t.Fatalf("run 2 was not a delta walk\n%s", events)
	}
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if n, _ := db.Count(resource); n != 5 {
		t.Fatalf("stored %d rows after a delta walk, want 5 (no prune)", n)
	}
	if strings.Contains(events, `"event":"reconcile"`) {
		t.Fatalf("a delta walk reconciled:\n%s", events)
	}
}

// A delta walk that fails part-way keeps the old watermark.
func TestSyncDeltaFailureKeepsTheWatermark(t *testing.T) {
	testenv.Isolate(t)
	resource := "hubspot-emails-crm"
	recs := deltaRecords(3, deltaOld, time.Minute)
	p := newDeltaPortal(t, "emails", recs)
	path := filepath.Join(t.TempDir(), "d.db")
	deltaSync(t, path, resource)
	watermark, _ := deltaState(t, path, resource)
	now := time.Now().UTC()
	p.set(deltaRecords(450, now, time.Millisecond))
	p.reset()
	p.mu.Lock()
	p.failAt = 2
	p.mu.Unlock()
	events := deltaSync(t, path, resource)
	if _, s := p.counts(); len(s) != 2 {
		t.Fatalf("searches=%d, want the walk to stop at the failing second page\n%s", len(s), events)
	}
	after, complete := deltaState(t, path, resource)
	if !after.Equal(watermark) || complete != 0 {
		t.Fatalf("watermark %v complete=%d after a failed delta, want %v and incomplete", after, complete, watermark)
	}
}

// deltaSetState rewrites one resource's checkpoint row.
func deltaSetState(t *testing.T, path, resource, cursor string, synced *time.Time) {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var at any
	if synced != nil {
		at = synced.UTC().Format(time.RFC3339)
	}
	if _, err := db.DB().Exec(`UPDATE sync_state SET last_cursor = ?, last_synced_at = ?, last_attempt_complete = 0 WHERE resource_type = ?`, cursor, at, resource); err != nil {
		t.Fatal(err)
	}
}

func deltaFirstGetAfter(t *testing.T, p *deltaPortal) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.listAfters) == 0 || p.listAfters[0] != "" {
		t.Fatalf("list walk GET afters %v, want the first from the top", p.listAfters)
	}
}

// Codex round 2 P1: a delta that falls back must not resume an earlier
// capped list walk's cursor and then stamp past a row that cursor skips.
// Emails 2-201 changed at T1 (one timestamp on a full page), email 1 at T2.
func TestSyncDeltaFallBackListStartsFromTheTop(t *testing.T) {
	testenv.Isolate(t)
	resource := "hubspot-emails-crm"
	p := newDeltaPortal(t, "emails", deltaRecords(201, deltaOld, time.Minute))
	path := filepath.Join(t.TempDir(), "d.db")
	deltaSync(t, path, resource)
	w := time.Now().UTC().Add(-2 * time.Hour)
	deltaSetState(t, path, resource, "100", &w)
	t1 := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	t2 := t1.Add(50 * time.Minute)
	recs := deltaRecords(201, t1, 0)
	recs[0].modified = t2
	p.set(recs)
	p.reset()
	events := deltaSync(t, path, resource)
	if !strings.Contains(events, `"reason":"page_one_timestamp"`) {
		t.Fatalf("no one-timestamp fall-back:\n%s", events)
	}
	deltaFirstGetAfter(t, p)
	if got := deltaStoredModified(t, path, resource, "1"); got != t2.Format("2006-01-02T15:04:05.000Z") {
		t.Fatalf("email 1 stored modified %q, want %s\n%s", got, t2.Format(time.RFC3339Nano), events)
	}
}

// The same class without a delta: an interrupted first sync resumed from its
// cursor would stamp a watermark newer than the rows the first attempt read.
func TestSyncDeltaResumedFirstSyncStartsFromTheTop(t *testing.T) {
	testenv.Isolate(t)
	resource := "hubspot-emails-crm"
	recs := deltaRecords(250, deltaOld, time.Minute)
	p := newDeltaPortal(t, "emails", recs)
	path := filepath.Join(t.TempDir(), "d.db")
	deltaSync(t, path, resource)
	deltaSetState(t, path, resource, "100", nil)
	changed := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	recs[0].modified = changed
	p.set(recs)
	p.reset()
	deltaSync(t, path, resource)
	deltaFirstGetAfter(t, p)
	if got := deltaStoredModified(t, path, resource, "1"); got != changed.Format("2006-01-02T15:04:05.000Z") {
		t.Fatalf("email 1 stored modified %q, want %s", got, changed.Format(time.RFC3339Nano))
	}
}

// --max-pages caps a delta walk: it stops after that many pages and stamps
// the last timestamp it read, so the next run continues from there.
func TestSyncDeltaMaxPagesCapsTheDelta(t *testing.T) {
	testenv.Isolate(t)
	resource := "hubspot-emails-crm"
	p := newDeltaPortal(t, "emails", deltaRecords(3, deltaOld, time.Minute))
	path := filepath.Join(t.TempDir(), "d.db")
	deltaSync(t, path, resource)
	start := time.Now().UTC().Add(time.Minute).Truncate(time.Second)
	recs := deltaRecords(450, start, time.Second)
	p.set(recs)
	p.reset()
	events := deltaSync(t, path, resource, "--max-pages", "1")
	gets, searches := p.counts()
	if gets != 0 || len(searches) != 1 {
		t.Fatalf("gets=%d searches=%d, want one capped search page\n%s", gets, len(searches), events)
	}
	w, complete := deltaState(t, path, resource)
	if !w.Equal(recs[199].modified) || complete != 1 {
		t.Fatalf("watermark %v complete=%d, want the last row read %v", w, complete, recs[199].modified)
	}
	p.reset()
	deltaSync(t, path, resource)
	_, searches = p.counts()
	if want := strconv.FormatInt(recs[199].modified.Add(-5*time.Minute).UnixMilli(), 10); searches[0].FilterGroups[0].Filters[0].Value != want {
		t.Fatalf("next run GTE %s, want %s", searches[0].FilterGroups[0].Filters[0].Value, want)
	}
}
