// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package tracker holds the hand-written domain and certificate logic: RDAP,
// Certificate Transparency (crt.sh), TLS and DNS observations, and the diff
// that turns two sweeps into events. Every network call goes through
// netguard, so nothing here can reach a non-public address or a non-443 port.
package tracker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"domain-cert-tracker-pp-cli/internal/cliutil"
	"domain-cert-tracker-pp-cli/internal/netguard"
)

// HTTPError is a non-2xx answer that is not a rate limit.
type HTTPError struct {
	URL    string
	Status int
}

func (e *HTTPError) Error() string { return fmt.Sprintf("GET %s: HTTP %d", e.URL, e.Status) }

// Web is a rate-limited, guarded GET client for one public source.
type Web struct {
	HTTP     *http.Client
	Limiter  *cliutil.AdaptiveLimiter
	MaxBytes int64
	Attempts int
	// Sleep is replaceable so tests do not wait out backoff.
	Sleep func(context.Context, time.Duration) error
}

// NewWeb returns a client that dials only through g at ratePerSec.
func NewWeb(g *netguard.Guard, ratePerSec float64, maxBytes int64) *Web {
	return &Web{
		HTTP:     g.HTTPClient(60 * time.Second),
		Limiter:  cliutil.NewAdaptiveLimiter(ratePerSec),
		MaxBytes: maxBytes,
		Attempts: 3,
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Get fetches url. 429 is retried with the server's Retry-After and returns
// *cliutil.RateLimitError once attempts run out; 502, 503 and 504 (crt.sh's
// usual overload answers) are retried with backoff and then returned as
// *HTTPError. A body larger than MaxBytes is an error, never a truncation.
func (w *Web) Get(ctx context.Context, url, accept string) ([]byte, error) {
	attempts := w.Attempts
	if attempts < 1 {
		attempts = 1
	}
	sleep := w.Sleep
	if sleep == nil {
		sleep = sleepCtx
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			wait := cliutil.Backoff(i)
			var rl *cliutil.RateLimitError
			if errors.As(lastErr, &rl) && rl.RetryAfter > 0 {
				wait = rl.RetryAfter
			}
			if wait > 30*time.Second {
				wait = 30 * time.Second
			}
			if err := sleep(ctx, wait); err != nil {
				return nil, err
			}
		}
		body, retry, err := w.once(ctx, url, accept)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, lastErr
}

func (w *Web) once(ctx context.Context, url, accept string) ([]byte, bool, error) {
	if w.Limiter != nil {
		if err := w.Limiter.Wait(ctx); err != nil {
			return nil, false, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	req.Header.Set("User-Agent", "domain-cert-tracker-cli (+https://github.com/Servosity/msp-skills)")
	resp, err := w.HTTP.Do(req)
	if err != nil {
		// A policy refusal is final; a transport error may be transient.
		return nil, !errors.Is(err, netguard.ErrRefused) && ctx.Err() == nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		if w.Limiter != nil {
			w.Limiter.OnRateLimit()
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return nil, true, &cliutil.RateLimitError{URL: url, RetryAfter: cliutil.RetryAfter(resp)}
	case resp.StatusCode == http.StatusBadGateway, resp.StatusCode == http.StatusServiceUnavailable, resp.StatusCode == http.StatusGatewayTimeout:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return nil, true, &HTTPError{URL: url, Status: resp.StatusCode}
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return nil, false, &HTTPError{URL: url, Status: resp.StatusCode}
	}
	if w.Limiter != nil {
		w.Limiter.OnSuccess()
	}
	limit := w.MaxBytes
	if limit <= 0 {
		limit = 8 << 20
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, true, err
	}
	if int64(len(body)) > limit {
		return nil, false, fmt.Errorf("GET %s: response larger than %d bytes", url, limit)
	}
	return body, false, nil
}
