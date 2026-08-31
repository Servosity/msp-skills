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
	"fmt"
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

// dominant names the largest phase, which is the question issue #195 asks.
func (p syncPhaseTotals) dominant() string {
	best, bestName := int64(-1), "none"
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
		if c.ms > best {
			best, bestName = c.ms, c.name
		}
	}
	return bestName
}

// emit writes the sync_phases event. Emitted only when tracing was requested,
// so the default event stream is unchanged.
func (t *syncPhaseTrace) emit(w io.Writer) {
	if t == nil || w == nil {
		return
	}
	p := t.totals()
	fmt.Fprintf(w, `{"event":"sync_phases","resource":"%s","queue_wait_ms":%d,"api_fetch_ms":%d,`+
		`"rate_limit_wait_ms":%d,"retry_wait_ms":%d,"id_extract_ms":%d,"store_upsert_ms":%d,`+
		`"pages":%d,"requests":%d,"retries":%d,"dominant_phase":"%s"}`+"\n",
		p.Resource, p.QueueWaitMS, p.APIFetchMS, p.RateLimitWaitMS, p.RetryWaitMS,
		p.IDExtractMS, p.StoreUpsertMS, p.Pages, p.Requests, p.Retries, p.dominant())
}
