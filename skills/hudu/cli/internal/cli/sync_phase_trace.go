// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-wired sync phase accounting for issue #195 (hudu: trace disproportionate
// lightweight-resource sync latency).
//
// The reporter saw lightweight Hudu resources take disproportionately long in a
// multi-resource sync. The only number the sync emitted was `duration_ms` per
// resource, which starts AFTER a worker dequeues the resource and folds
// together five different things. This file splits it into five phases:
//
//	queue_wait_ms       enqueue -> a worker picked this resource up
//	api_fetch_ms        time on the wire (total Get time minus the two waits)
//	rate_limit_wait_ms  proactive rate-limiter wait before each request
//	retry_wait_ms       429 Retry-After + 5xx backoff sleeps
//	id_extract_ms       extracting items and the primary key from the page
//	store_upsert_ms     UpsertBatch, which serializes every worker on one
//	                    write mutex held through tx.Commit()
//
// queue_wait_ms is NOT in the issue's acceptance criteria and is the reason
// this file exists in this shape: `sync` enqueues every resource up front and
// runs a fixed worker pool, so a "slow" lightweight resource can have spent
// almost all of its wall time sitting in the queue behind heavier ones, and no
// phase the issue lists would have measured a single millisecond of it.
//
// Entirely opt-in: without `--phase-trace` no trace is installed on the
// context, every helper below is a nil-receiver no-op, and the emitted event
// stream is byte-identical to the untraced sync. Nothing recorded here is a
// credential, a URL, a header or a response body - only durations and counts.

package cli

import (
	"context"
	"encoding/json"
	"io"
	"sync/atomic"
	"time"

	"hudu-pp-cli/internal/client"
)

// syncPhaseTrace accumulates one resource's phase timings. The page loop is
// single-goroutine per resource, but the counters are atomic so a future
// intra-resource parallel fetch cannot silently corrupt them.
type syncPhaseTrace struct {
	resource     string
	queuedAt     time.Time
	queueWaitNS  atomic.Int64
	apiFetchNS   atomic.Int64
	idExtractNS  atomic.Int64
	storeUpsrtNS atomic.Int64
	pages        atomic.Int64
	request      *client.RequestTrace
}

type syncPhaseTraceKey struct{}
type syncPhaseTracingKey struct{}

// withSyncPhaseTracing marks a context as opted in to phase tracing. The
// per-resource trace is installed later, once the resource is dequeued.
func withSyncPhaseTracing(ctx context.Context) context.Context {
	return context.WithValue(ctx, syncPhaseTracingKey{}, true)
}

func syncPhaseTracingEnabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	on, _ := ctx.Value(syncPhaseTracingKey{}).(bool)
	return on
}

// newSyncPhaseTrace installs a per-resource trace (and the client-side request
// trace it wraps) on ctx. queuedAt is when the resource entered the work
// channel, which is what makes queue_wait_ms measurable at all.
func newSyncPhaseTrace(ctx context.Context, resource string, queuedAt time.Time) (context.Context, *syncPhaseTrace) {
	rt := &client.RequestTrace{}
	tr := &syncPhaseTrace{resource: resource, queuedAt: queuedAt, request: rt}
	ctx = client.WithRequestTrace(ctx, rt)
	return context.WithValue(ctx, syncPhaseTraceKey{}, tr), tr
}

func syncPhaseTraceFrom(ctx context.Context) *syncPhaseTrace {
	if ctx == nil {
		return nil
	}
	tr, _ := ctx.Value(syncPhaseTraceKey{}).(*syncPhaseTrace)
	return tr
}

// markDequeued closes the queue_wait phase. Called once, at the top of
// syncResource, which is the first instruction that runs after a worker picks
// the resource up.
func (t *syncPhaseTrace) markDequeued(now time.Time) {
	if t == nil || t.queuedAt.IsZero() {
		return
	}
	t.queueWaitNS.Store(int64(now.Sub(t.queuedAt)))
}

func (t *syncPhaseTrace) addAPIFetch(d time.Duration) {
	if t == nil {
		return
	}
	t.apiFetchNS.Add(int64(d))
	t.pages.Add(1)
}

func (t *syncPhaseTrace) addIDExtract(d time.Duration) {
	if t == nil {
		return
	}
	t.idExtractNS.Add(int64(d))
}

func (t *syncPhaseTrace) addStoreUpsert(d time.Duration) {
	if t == nil {
		return
	}
	t.storeUpsrtNS.Add(int64(d))
}

// syncPhaseTotals is the flat, emit-ready view. Durations are milliseconds.
type syncPhaseTotals struct {
	Resource        string
	QueueWaitMS     int64
	APIFetchMS      int64
	RateLimitWaitMS int64
	RetryWaitMS     int64
	IDExtractMS     int64
	StoreUpsertMS   int64
	Pages           int64
	Requests        int64
	Retries         int64
}

// totals subtracts the client's two wait phases out of the measured Get time,
// so api_fetch_ms is time on the wire rather than time inside Get(). Clamped at
// zero: the subtraction is across two clocks read at slightly different points
// and a tiny negative would be noise reported as a lie.
func (t *syncPhaseTrace) totals() syncPhaseTotals {
	if t == nil {
		return syncPhaseTotals{}
	}
	rateLimit := t.request.RateLimitWait()
	retry := t.request.RetryWait()
	fetch := time.Duration(t.apiFetchNS.Load()) - rateLimit - retry
	if fetch < 0 {
		fetch = 0
	}
	return syncPhaseTotals{
		Resource:        t.resource,
		QueueWaitMS:     time.Duration(t.queueWaitNS.Load()).Milliseconds(),
		APIFetchMS:      fetch.Milliseconds(),
		RateLimitWaitMS: rateLimit.Milliseconds(),
		RetryWaitMS:     retry.Milliseconds(),
		IDExtractMS:     time.Duration(t.idExtractNS.Load()).Milliseconds(),
		StoreUpsertMS:   time.Duration(t.storeUpsrtNS.Load()).Milliseconds(),
		Pages:           t.pages.Load(),
		Requests:        t.request.Requests(),
		Retries:         t.request.Retries(),
	}
}

// dominant names the phase that is strictly larger than every other phase,
// which is the question issue #195 asks. It returns "none" in the two cases
// where there is no such phase:
//
//   - nothing was measured (every phase 0ms) - a resource that failed before it
//     reached the API, or a trace that was never fed;
//   - two or more phases tie for the largest.
//
// Both used to answer "queue_wait": the accumulator started at -1, so 0ms beat
// it and the first entry in the list won, which made the "none" branch
// unreachable and handed every tie to whichever phase happened to be listed
// first. `dominant_phase` is the field the guide tells an operator to act on
// (raise --concurrency on queue_wait, lower it on store_upsert), so a
// list-order artifact reported as a measurement is a wrong instruction, not a
// cosmetic bug.
func (p syncPhaseTotals) dominant() string {
	var best int64
	bestName := "none"
	tied := false
	for _, c := range []struct {
		name string
		ms   int64
	}{
		{"queue_wait", p.QueueWaitMS},
		{"api_fetch", p.APIFetchMS},
		{"rate_limit_wait", p.RateLimitWaitMS},
		{"retry_wait", p.RetryWaitMS},
		{"id_extract", p.IDExtractMS},
		{"store_upsert", p.StoreUpsertMS},
	} {
		switch {
		case c.ms > best:
			best, bestName, tied = c.ms, c.name, false
		case c.ms == best && best > 0:
			tied = true
		}
	}
	if tied {
		return "none"
	}
	return bestName
}

// syncPhasesEvent is the wire shape of the sync_phases event. Field order here
// is the emitted key order, so it must stay in step with the table in
// skills/hudu/guide.md.
type syncPhasesEvent struct {
	Event           string `json:"event"`
	Resource        string `json:"resource"`
	QueueWaitMS     int64  `json:"queue_wait_ms"`
	APIFetchMS      int64  `json:"api_fetch_ms"`
	RateLimitWaitMS int64  `json:"rate_limit_wait_ms"`
	RetryWaitMS     int64  `json:"retry_wait_ms"`
	IDExtractMS     int64  `json:"id_extract_ms"`
	StoreUpsertMS   int64  `json:"store_upsert_ms"`
	Pages           int64  `json:"pages"`
	Requests        int64  `json:"requests"`
	Retries         int64  `json:"retries"`
	DominantPhase   string `json:"dominant_phase"`
}

// emit writes the sync_phases event. Emitted only when tracing was requested,
// so the default event stream is unchanged.
//
// Marshalled rather than hand-formatted: `--resources` takes the resource name
// straight from the operator, and a name containing a quote or a backslash
// interpolated with %s emits a line no JSON parser can read. SetEscapeHTML is
// off so a name containing <, > or & stays readable instead of being escaped to
// < and friends; this is a diagnostics stream, not HTML.
func (t *syncPhaseTrace) emit(w io.Writer) {
	if t == nil || w == nil {
		return
	}
	p := t.totals()
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(syncPhasesEvent{
		Event:           "sync_phases",
		Resource:        p.Resource,
		QueueWaitMS:     p.QueueWaitMS,
		APIFetchMS:      p.APIFetchMS,
		RateLimitWaitMS: p.RateLimitWaitMS,
		RetryWaitMS:     p.RetryWaitMS,
		IDExtractMS:     p.IDExtractMS,
		StoreUpsertMS:   p.StoreUpsertMS,
		Pages:           p.Pages,
		Requests:        p.Requests,
		Retries:         p.Retries,
		DominantPhase:   p.dominant(),
	})
}
