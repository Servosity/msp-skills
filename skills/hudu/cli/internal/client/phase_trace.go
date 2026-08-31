// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-wired request-phase accounting for issue #195 (hudu: trace
// disproportionate lightweight-resource sync latency).
//
// The existing per-resource `duration_ms` is one number that folds together
// server time, the proactive rate-limiter wait, retry backoff sleeps, ID
// extraction and the SQLite upsert, so it cannot say which of them a slow
// lightweight resource is actually spending its time in. This file lets the
// caller separate the two phases only the client can see: the time spent
// WAITING for the rate limiter before a request goes out, and the time spent
// sleeping between retries.
//
// Deliberately opt-in and allocation-free when off: with no trace in the
// context every method below is a nil-receiver no-op, so an untraced sync
// behaves exactly as it did before.
//
// Nothing here records a URL, a header, a credential or a response body - only
// durations and counts.

package client

import (
	"context"
	"sync/atomic"
	"time"
)

// RequestTrace accumulates the non-server time of every request made under one
// context. Safe for concurrent use: the sync worker pool shares one client.
type RequestTrace struct {
	rateLimitWaitNS atomic.Int64
	retryWaitNS     atomic.Int64
	requests        atomic.Int64
	retries         atomic.Int64
}

// AddRateLimitWait records time spent in the proactive rate limiter.
func (t *RequestTrace) AddRateLimitWait(d time.Duration) {
	if t == nil {
		return
	}
	t.rateLimitWaitNS.Add(int64(d))
}

// AddRetryWait records time spent sleeping before a retry (429 Retry-After or
// 5xx exponential backoff).
func (t *RequestTrace) AddRetryWait(d time.Duration) {
	if t == nil {
		return
	}
	t.retryWaitNS.Add(int64(d))
}

// CountRequest records one outgoing HTTP attempt.
func (t *RequestTrace) CountRequest() {
	if t == nil {
		return
	}
	t.requests.Add(1)
}

// CountRetry records one retry decision.
func (t *RequestTrace) CountRetry() {
	if t == nil {
		return
	}
	t.retries.Add(1)
}

// RateLimitWait returns the accumulated proactive rate-limiter wait.
func (t *RequestTrace) RateLimitWait() time.Duration {
	if t == nil {
		return 0
	}
	return time.Duration(t.rateLimitWaitNS.Load())
}

// RetryWait returns the accumulated retry backoff sleep.
func (t *RequestTrace) RetryWait() time.Duration {
	if t == nil {
		return 0
	}
	return time.Duration(t.retryWaitNS.Load())
}

// Requests returns the number of outgoing HTTP attempts.
func (t *RequestTrace) Requests() int64 {
	if t == nil {
		return 0
	}
	return t.requests.Load()
}

// Retries returns the number of retry decisions.
func (t *RequestTrace) Retries() int64 {
	if t == nil {
		return 0
	}
	return t.retries.Load()
}

type requestTraceKey struct{}

// WithRequestTrace returns a context whose requests accumulate into trace.
func WithRequestTrace(ctx context.Context, trace *RequestTrace) context.Context {
	if trace == nil {
		return ctx
	}
	return context.WithValue(ctx, requestTraceKey{}, trace)
}

// RequestTraceFrom returns the trace installed on ctx, or nil. A nil trace is
// usable: every method on it is a no-op.
func RequestTraceFrom(ctx context.Context) *RequestTrace {
	if ctx == nil {
		return nil
	}
	t, _ := ctx.Value(requestTraceKey{}).(*RequestTrace)
	return t
}
