// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Issue #195 fixtures: deterministic local HTTP + real SQLite contention,
// driven through the real client and the real store so the phase numbers come
// from the code that runs in production rather than from a mock of it.
//
// Honest scope limit: these fixtures prove each phase is MEASURED and that the
// store-upsert phase is REACHABLE as a dominant phase under contention. They
// do NOT prove which phase dominates in the reporter's production tenant -
// that needs the reporter's tenant, and nothing here should be read as
// standing in for it.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"hudu-pp-cli/internal/client"
	"hudu-pp-cli/internal/config"
	"hudu-pp-cli/internal/store"
)

// slowPageServer serves one full page then one short page per resource, with a
// fixed think-time per response so api_fetch_ms is a known quantity.
func slowPageServer(t *testing.T, think time.Duration, rows int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(think)
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		if page == "" || page == "1" {
			_, _ = w.Write(makePage(1, rows))
			return
		}
		_, _ = w.Write([]byte("[]"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fixtureClient(t *testing.T, baseURL string) *client.Client {
	t.Helper()
	cfg := &config.Config{BaseURL: baseURL, AuthHeaderVal: "Bearer fixture-token"}
	c := client.New(cfg, 30*time.Second, 0)
	c.NoCache = true
	return c
}

// runTracedSync drives the same enqueue -> worker -> syncResource path `sync`
// uses, with the same fixed worker pool, and returns each resource's phases.
func runTracedSync(t *testing.T, c *client.Client, db *store.Store, resources []string, workers int) map[string]syncPhaseTotals {
	t.Helper()
	ctx := withSyncPhaseTracing(context.Background())
	work := make(chan syncWorkItem, len(resources))

	var mu sync.Mutex
	out := map[string]syncPhaseTotals{}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range work {
				rctx, tr := newSyncPhaseTrace(ctx, item.resource, item.queuedAt)
				res := syncResource(rctx, c, db, item.resource, "", true, 0, false, nil, nil)
				if res.Err != nil {
					t.Errorf("sync %s: %v", item.resource, res.Err)
				}
				mu.Lock()
				out[item.resource] = tr.totals()
				mu.Unlock()
			}
		}()
	}
	for _, r := range resources {
		work <- syncWorkItem{resource: r, queuedAt: time.Now()}
	}
	close(work)
	wg.Wait()
	return out
}

// Fixture 1: one worker, many resources, a slow server. Every resource after
// the first spends most of its wall time in the queue - the interval the
// issue's four listed phases do not measure at all, and the reason
// queue_wait_ms is here.
func TestPhaseTrace_QueueWaitIsMeasured(t *testing.T) {
	db := openSyncTestDB(t)
	defer db.Close()
	srv := slowPageServer(t, 60*time.Millisecond, 5)
	c := fixtureClient(t, srv.URL)

	resources := []string{"companies", "articles", "folders", "groups", "lists"}
	phases := runTracedSync(t, c, db, resources, 1)

	if len(phases) != len(resources) {
		t.Fatalf("got %d traces, want %d", len(phases), len(resources))
	}
	queueDominant := 0
	for _, r := range resources {
		p := phases[r]
		if p.Pages == 0 {
			t.Errorf("%s: no pages fetched", r)
		}
		if p.APIFetchMS <= 0 {
			t.Errorf("%s: api_fetch_ms = %d, want > 0 against a 60ms server", r, p.APIFetchMS)
		}
		if p.dominant() == "queue_wait" {
			queueDominant++
		}
		t.Logf("%-12s queue=%dms fetch=%dms ratelimit=%dms retry=%dms extract=%dms upsert=%dms pages=%d dominant=%s",
			r, p.QueueWaitMS, p.APIFetchMS, p.RateLimitWaitMS, p.RetryWaitMS,
			p.IDExtractMS, p.StoreUpsertMS, p.Pages, p.dominant())
	}
	// With a single worker and 5 resources at 2 requests x 60ms each, the last
	// resources wait several hundred ms in the queue.
	if queueDominant == 0 {
		t.Errorf("no resource was queue-wait dominant under a single worker; queue_wait_ms is not tracking the enqueue-to-dequeue interval")
	}
}

// Fixture 2: SQLite write contention. UpsertBatch takes the store's write
// mutex and holds it through tx.Commit(), so with a fast server and many
// concurrent workers the upsert phase is where the time goes. This is the
// mechanism the reporter's hypothesis points at; the fixture shows it is
// REACHABLE, not that it dominates in their tenant.
func TestPhaseTrace_StoreUpsertUnderContention(t *testing.T) {
	db := openSyncTestDB(t)
	defer db.Close()
	// No think time: the server answers instantly so the only serialization
	// left is the store's own write mutex.
	srv := slowPageServer(t, 0, 400)
	c := fixtureClient(t, srv.URL)

	resources := []string{"companies", "articles", "folders", "groups", "lists", "flags", "procedures", "uploads"}
	phases := runTracedSync(t, c, db, resources, 8)

	var upsertTotal, fetchTotal, extractTotal, queueTotal int64
	for _, r := range resources {
		p := phases[r]
		upsertTotal += p.StoreUpsertMS
		fetchTotal += p.APIFetchMS
		extractTotal += p.IDExtractMS
		queueTotal += p.QueueWaitMS
		t.Logf("%-12s queue=%dms fetch=%dms extract=%dms upsert=%dms pages=%d dominant=%s",
			r, p.QueueWaitMS, p.APIFetchMS, p.IDExtractMS, p.StoreUpsertMS, p.Pages, p.dominant())
	}
	t.Logf("TOTALS  queue=%dms fetch=%dms extract=%dms upsert=%dms", queueTotal, fetchTotal, extractTotal, upsertTotal)
	if upsertTotal <= 0 {
		t.Fatal("store_upsert_ms never moved, so the SQLite phase is not being measured")
	}
	if upsertTotal < fetchTotal {
		t.Errorf("with an instant server, upsert (%dms) should exceed fetch (%dms); the contention fixture is not exercising the write mutex",
			upsertTotal, fetchTotal)
	}
}

// Fixture 3: retry and rate-limit wait land in their own phases and NOT in
// api_fetch_ms. Without the split, a 429 storm reads as slow server time.
func TestPhaseTrace_RetryWaitIsNotCountedAsFetch(t *testing.T) {
	db := openSyncTestDB(t)
	defer db.Close()

	var calls int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"slow down"}`))
			return
		}
		if r.URL.Query().Get("page") == "" || r.URL.Query().Get("page") == "1" {
			_, _ = w.Write(makePage(1, 3))
			return
		}
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	phases := runTracedSync(t, fixtureClient(t, srv.URL), db, []string{"companies"}, 1)
	p := phases["companies"]
	t.Logf("companies queue=%dms fetch=%dms ratelimit=%dms retry=%dms extract=%dms upsert=%dms retries=%d dominant=%s",
		p.QueueWaitMS, p.APIFetchMS, p.RateLimitWaitMS, p.RetryWaitMS, p.IDExtractMS, p.StoreUpsertMS, p.Retries, p.dominant())

	if p.Retries < 1 {
		t.Fatalf("retries = %d, want at least 1 (the 429 was not retried)", p.Retries)
	}
	if p.RetryWaitMS < 900 {
		t.Errorf("retry_wait_ms = %d, want about 1000 from the Retry-After: 1 response", p.RetryWaitMS)
	}
	if p.APIFetchMS > 500 {
		t.Errorf("api_fetch_ms = %d: the retry sleep leaked into the fetch phase, which is exactly the conflation #195 asks to undo", p.APIFetchMS)
	}
}

// The default path must be untouched: with no --phase-trace, no trace is
// installed, nothing is measured and no sync_phases line is emitted.
func TestPhaseTrace_OffByDefault(t *testing.T) {
	db := openSyncTestDB(t)
	defer db.Close()
	srv := slowPageServer(t, 0, 3)

	var events strings.Builder
	ctx := context.Background()
	if syncPhaseTracingEnabled(ctx) {
		t.Fatal("a plain context must not be traced")
	}
	if tr := syncPhaseTraceFrom(ctx); tr != nil {
		t.Fatal("a plain context must carry no phase trace")
	}
	res := syncResource(ctx, fixtureClient(t, srv.URL), db, "companies", "", true, 0, false, nil, &events)
	if res.Err != nil {
		t.Fatalf("sync: %v", res.Err)
	}
	if strings.Contains(events.String(), "sync_phases") {
		t.Errorf("an untraced sync emitted a sync_phases event:\n%s", events.String())
	}
	// A nil trace must be safe to emit through, so the worker needs no branch.
	var nilTrace *syncPhaseTrace
	nilTrace.emit(&events)
	if strings.Contains(events.String(), "sync_phases") {
		t.Errorf("a nil trace emitted an event:\n%s", events.String())
	}
}

// The emitted event carries durations and counts only - no credential, no URL,
// no response body.
func TestPhaseTrace_EventCarriesNoSecretsOrBodies(t *testing.T) {
	db := openSyncTestDB(t)
	defer db.Close()
	srv := slowPageServer(t, 0, 3)
	c := fixtureClient(t, srv.URL)

	ctx := withSyncPhaseTracing(context.Background())
	rctx, tr := newSyncPhaseTrace(ctx, "companies", time.Now())
	if res := syncResource(rctx, c, db, "companies", "", true, 0, false, nil, nil); res.Err != nil {
		t.Fatalf("sync: %v", res.Err)
	}
	var events strings.Builder
	tr.emit(&events)
	line := strings.TrimSpace(events.String())

	var decoded map[string]any
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("sync_phases is not valid JSON (%v): %s", err, line)
	}
	for _, banned := range []string{"fixture-token", "Bearer", srv.URL, "127.0.0.1", `"id"`} {
		if strings.Contains(line, banned) {
			t.Errorf("sync_phases leaked %q: %s", banned, line)
		}
	}
	want := []string{"queue_wait_ms", "api_fetch_ms", "rate_limit_wait_ms", "retry_wait_ms",
		"id_extract_ms", "store_upsert_ms", "pages", "requests", "retries", "dominant_phase"}
	for _, key := range want {
		if _, ok := decoded[key]; !ok {
			t.Errorf("sync_phases omits %s: %s", key, line)
		}
	}
	if got := fmt.Sprint(decoded["resource"]); got != "companies" {
		t.Errorf("resource = %q, want companies", got)
	}
}

// dominant_phase is the field skills/hudu/guide.md tells the operator to act on
// ("raise --concurrency" vs "lower --concurrency"), so it must never name a
// phase that was not the largest. Two shapes used to be reported as
// queue_wait: a trace where nothing was measured at all, and any tie.
func TestPhaseTrace_DominantPhase(t *testing.T) {
	for _, tc := range []struct {
		name   string
		totals syncPhaseTotals
		want   string
	}{
		// Regression: `best` started at -1 and queue_wait was evaluated
		// first, so 0ms "won" and the "none" branch was unreachable. A
		// resource that errored before it reached the API reported
		// dominant_phase=queue_wait on an all-zero trace.
		{"nothing measured", syncPhaseTotals{}, "none"},
		// Regression: the same list-order bias decided every tie.
		{"tie between two phases",
			syncPhaseTotals{APIFetchMS: 40, StoreUpsertMS: 40}, "none"},
		{"tie including queue_wait",
			syncPhaseTotals{QueueWaitMS: 40, StoreUpsertMS: 40}, "none"},
		{"three-way tie",
			syncPhaseTotals{QueueWaitMS: 7, APIFetchMS: 7, RetryWaitMS: 7}, "none"},
		// A single strict winner is still named, wherever it sits in the list.
		{"queue_wait dominant",
			syncPhaseTotals{QueueWaitMS: 62, APIFetchMS: 49, StoreUpsertMS: 14}, "queue_wait"},
		{"store_upsert dominant (last in the list)",
			syncPhaseTotals{QueueWaitMS: 1, StoreUpsertMS: 900}, "store_upsert"},
		{"retry_wait dominant",
			syncPhaseTotals{APIFetchMS: 3, RetryWaitMS: 3001}, "retry_wait"},
		{"one phase, one millisecond",
			syncPhaseTotals{IDExtractMS: 1}, "id_extract"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.totals.dominant(); got != tc.want {
				t.Errorf("dominant() = %q, want %q (totals %+v)", got, tc.want, tc.totals)
			}
		})
	}
}

// --resources takes the resource name straight from the operator. The event was
// hand-formatted with %s, so a name carrying a quote or a backslash emitted a
// line no JSON parser could read.
func TestPhaseTrace_EmitIsValidJSONForAwkwardResourceNames(t *testing.T) {
	for _, name := range []string{
		`a"b`,
		`a\b`,
		"tab\there",
		"new\nline",
		"<script>&</script>",
		"companies",
	} {
		t.Run(name, func(t *testing.T) {
			tr := &syncPhaseTrace{resource: name, request: &client.RequestTrace{}}
			var out strings.Builder
			tr.emit(&out)
			line := strings.TrimSpace(out.String())

			var decoded map[string]any
			if err := json.Unmarshal([]byte(line), &decoded); err != nil {
				t.Fatalf("sync_phases is not valid JSON (%v): %s", err, line)
			}
			if got := fmt.Sprint(decoded["resource"]); got != name {
				t.Errorf("resource round-tripped as %q, want %q", got, name)
			}
			if got := fmt.Sprint(decoded["event"]); got != "sync_phases" {
				t.Errorf("event = %q, want sync_phases", got)
			}
		})
	}
}
