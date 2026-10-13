// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"granola-pp-cli/internal/config"
)

// TestDoRejectsUnfollowedRedirectResponses proves that a 3xx response the
// client did NOT follow — one with no usable Location (so CheckRedirect never
// fired) or a 304 — is a terminal RedirectResponseError: never parsed as a
// success body, never re-driven through the GET retry loop, server hit once.
// Without the response-path guard each of these has status < 400 and would be
// handed back as a "success".
func TestDoRejectsUnfollowedRedirectResponses(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{"302 without Location", http.StatusFound},
		{"304 Not Modified", http.StatusNotModified},
		{"307 without Location", http.StatusTemporaryRedirect},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var hits int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
				// Deliberately omit Location so net/http returns the 3xx
				// without following it — exercising the response-path guard,
				// not CheckRedirect.
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(server.Close)

			c := New(&config.Config{BaseURL: server.URL, GranolaApiKey: "secret-token-value"}, time.Second, 0)
			c.NoCache = true

			_, err := c.Get(context.Background(), "/v1/notes/not_abc", nil)
			if err == nil {
				t.Fatalf("%s: expected a terminal redirect error, got nil (3xx parsed as success)", tc.name)
			}
			var redirectErr *RedirectResponseError
			if !errors.As(err, &redirectErr) {
				t.Fatalf("%s: error is not *RedirectResponseError: %T: %v", tc.name, err, err)
			}
			if got := atomic.LoadInt32(&hits); got != 1 {
				t.Fatalf("%s: server hit %d times, want exactly 1 (unfollowed redirect must not retry)", tc.name, got)
			}
			if strings.Contains(err.Error(), "secret-token-value") {
				t.Fatalf("%s: redirect error leaked the credential: %v", tc.name, err)
			}
		})
	}
}

// TestDoClassifiesMalformedRedirectLocationTerminal proves a redirect whose
// Location header is malformed (net/http fails the Do call while parsing it,
// BEFORE CheckRedirect fires) is classified terminal — a retryable GET must NOT
// replay it as a transient transport failure. Without the classification the
// GET would retry (server hit up to maxRetries+1 times).
func TestDoClassifiesMalformedRedirectLocationTerminal(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		// "%zz" is an invalid percent-escape: req.URL.Parse(loc) fails, so
		// net/http returns a "failed to parse Location header" error.
		w.Header().Set("Location", "%zz")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(server.Close)

	c := New(&config.Config{BaseURL: server.URL, GranolaApiKey: "secret-token-value"}, time.Second, 0)
	c.NoCache = true

	_, err := c.Get(context.Background(), "/v1/notes/not_abc", nil)
	if err == nil {
		t.Fatalf("expected a terminal redirect error for a malformed Location, got nil")
	}
	var redirectErr *RedirectResponseError
	if !errors.As(err, &redirectErr) {
		t.Fatalf("error is not *RedirectResponseError: %T: %v", err, err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("server hit %d times, want exactly 1 (malformed-Location redirect must not retry)", got)
	}
	if strings.Contains(err.Error(), "secret-token-value") {
		t.Fatalf("redirect error leaked the credential: %v", err)
	}
}
