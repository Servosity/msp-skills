// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression test for the fresh Codex round-1
// input on auth resolution, pinned by skills/cipp/handfixes.json under
// "auth-resolved-per-attempt".

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestRetryAfterWaitReMintsExpiredToken: client-credentials mode with a cold
// cache, token {"access_token":..., "expires_in":2}, first CIPP answer HTTP 429
// with Retry-After: 3. The retry must not send the expired bearer.
func TestRetryAfterWaitReMintsExpiredToken(t *testing.T) {
	clearVerifyEnv(t)
	ts := newTokenServer(t, 2)
	var mu sync.Mutex
	var auths []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auths = append(auths, r.Header.Get("Authorization"))
		n := len(auths)
		mu.Unlock()
		if n == 1 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Results":[]}`))
	}))
	defer api.Close()
	cfg := expiringConfig(t, ts, time.Time{})
	cfg.AccessToken = "" // cold cache
	cfg.BaseURL = api.URL
	c := New(cfg, 30*time.Second, 0)
	c.NoCache = true
	resetMintState()
	t.Cleanup(resetMintState)
	if _, err := c.Get(context.Background(), "/ListUsers", map[string]string{"tenantFilter": "t.example"}); err != nil {
		t.Fatalf("get: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(auths) < 2 {
		t.Fatalf("expected a retry after the 429, saw %d request(s)", len(auths))
	}
	if auths[1] == auths[0] {
		t.Fatalf("the retry after Retry-After resent the expired bearer %q", auths[0])
	}
	if got := ts.grantCount(); got < 2 {
		t.Fatalf("expected a re-mint before the retry, saw %d grant(s)", got)
	}
}
