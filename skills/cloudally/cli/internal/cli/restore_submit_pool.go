// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptrace"
	"sync"
	"sync/atomic"
	"time"

	"cloudally-pp-cli/internal/client"
)

// rateLimiter spaces requests at a fixed interval across workers.
type rateLimiter struct {
	mu       sync.Mutex
	next     time.Time
	interval time.Duration
}

func newRateLimiter(perSecond float64) *rateLimiter {
	if perSecond <= 0 {
		return &rateLimiter{}
	}
	return &rateLimiter{interval: time.Duration(float64(time.Second) / perSecond)}
}

func (r *rateLimiter) wait(ctx context.Context) error {
	if r.interval <= 0 {
		return ctx.Err()
	}
	r.mu.Lock()
	now := time.Now()
	if r.next.Before(now) {
		r.next = now
	}
	slot := r.next
	r.next = slot.Add(r.interval)
	r.mu.Unlock()
	if d := time.Until(slot); d > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
	return ctx.Err()
}

// classifySubmitFailure decides what a failed restore POST means for the
// ledger. httpStatus is the response status when one came back (0 when
// none did); connected reports whether any connection to the API was
// obtained during the call, which is the earliest point request bytes can
// leave this process.
//
//   - 4xx other than 408 and 429 is a clean rejection (FAILED); 429 is a
//     clean rejection too, since the API refused before doing the work.
//   - 408, 5xx, and any other response code are UNKNOWN: the API may have
//     accepted the restore before answering.
//   - With no response, a call that never obtained a connection (DNS
//     failure, connection refused, a local error before dialing) provably
//     sent nothing (FAILED); anything after a connection (timeout, reset,
//     cancel while in flight) is UNKNOWN.
func classifySubmitFailure(httpStatus int, connected bool) string {
	switch {
	case httpStatus == http.StatusRequestTimeout:
		return jobUnknown
	case httpStatus == http.StatusTooManyRequests:
		return jobFailed
	case httpStatus >= 400 && httpStatus < 500:
		return jobFailed
	case httpStatus != 0:
		return jobUnknown
	case connected:
		return jobUnknown
	default:
		return jobFailed
	}
}

// submitOne posts a single restore and classifies the outcome.
func submitOne(ctx context.Context, c *client.Client, path string, body map[string]any, job restoreJob) submitResult {
	res := submitResult{job: job, at: time.Now().UTC()}
	var connected atomic.Bool
	trace := &httptrace.ClientTrace{
		GotConn: func(httptrace.GotConnInfo) { connected.Store(true) },
	}
	raw, status, err := c.Post(httptrace.WithClientTrace(ctx, trace), path, body)
	res.httpStatus = status
	if err != nil {
		var apiE *client.APIError
		if errors.As(err, &apiE) && apiE.StatusCode != 0 {
			res.httpStatus = apiE.StatusCode
		}
		res.authFailure = res.httpStatus == http.StatusUnauthorized || res.httpStatus == http.StatusForbidden
		res.status = classifySubmitFailure(res.httpStatus, connected.Load())
		res.errText = err.Error()
		return res
	}
	res.response = string(raw)
	var parsed restoreResponse
	_ = json.Unmarshal(raw, &parsed)
	if parsed.NeedRedirect {
		res.status = jobFailedRedirect
		res.errText = "restore requires a browser redirect: " + parsed.RedirectURL
		return res
	}
	res.status = jobSubmitted
	return res
}

// runSubmitPool posts every job with a bounded worker pool and rate limit.
// beforeSend runs on the worker right before each request and must commit
// the row as SUBMITTING; when it fails the row is not sent and is reported
// with notSent set. Results are delivered on the calling goroutine. A
// 401/403 stops dispatch: rows not yet sent stay PLANNED, while requests
// already on the wire run to completion so their outcome is recorded
// instead of being cut off.
func runSubmitPool(ctx context.Context, c *client.Client, path, taskID, source string, jobs []restoreJob, opts submitOptions, concurrency int, rate float64, beforeSend func(restoreJob) error, onResult func(submitResult)) {
	dispatchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if concurrency < 1 {
		concurrency = 1
	}
	work := make(chan restoreJob)
	results := make(chan submitResult)
	limiter := newRateLimiter(rate)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range work {
				if err := limiter.wait(dispatchCtx); err != nil {
					return
				}
				if err := beforeSend(job); err != nil {
					results <- submitResult{job: job, status: job.Status, notSent: true, errText: "not sent: " + err.Error(), at: time.Now().UTC()}
					continue
				}
				body := restoreBody(taskID, source, job.Entity, job.ExtID, job.ResolvedRaw, opts)
				res := submitOne(ctx, c, path, body, job)
				if res.authFailure {
					// Stop dispatch before this worker can pick up another
					// row, rather than after the result is recorded.
					cancel()
				}
				results <- res
			}
		}()
	}
	go func() {
		defer close(work)
		for _, job := range jobs {
			select {
			case <-dispatchCtx.Done():
				return
			case work <- job:
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()
	for res := range results {
		onResult(res)
		if res.authFailure {
			cancel()
		}
	}
}
