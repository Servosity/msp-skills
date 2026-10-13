// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"granola-pp-cli/internal/config"
)

// TestCheckRedirectRefusesAndDoesNotRetry proves a 302 yields the typed
// RedirectRefusedError, that it is terminal (the server is hit exactly once,
// never re-driven through the retry loop even for a retryable GET), and that
// it is not a false success.
func TestCheckRedirectRefusesAndDoesNotRetry(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		// Same-host redirect: Go's default policy would re-add Authorization
		// here; the client must refuse instead of following.
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	t.Cleanup(server.Close)

	// New() installs the refuse-all CheckRedirect; do NOT overwrite HTTPClient
	// or the policy under test would be replaced.
	c := New(&config.Config{BaseURL: server.URL, GranolaApiKey: "secret-token-value"}, time.Second, 0)
	c.NoCache = true

	_, err := c.Get(context.Background(), "/v1/notes/not_abc", nil)
	if err == nil {
		t.Fatalf("expected a redirect-refusal error, got nil (a redirect was treated as success)")
	}
	var redirectErr *RedirectRefusedError
	if !errors.As(err, &redirectErr) {
		t.Fatalf("error is not *RedirectRefusedError: %T: %v", err, err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("server hit %d times, want exactly 1 (refused redirect must not retry)", got)
	}
	if strings.Contains(err.Error(), "secret-token-value") {
		t.Fatalf("redirect error leaked the credential: %v", err)
	}
}

// zeroReader streams filler bytes without allocating a large buffer, so the
// oversized-body server can produce >64 MiB cheaply.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

// TestDoRejectsOversizedResponseBody proves a response body larger than the
// cap is REJECTED with an error rather than silently truncated.
func TestDoRejectsOversizedResponseBody(t *testing.T) {
	if testing.Short() {
		t.Skip("skips the >64 MiB transfer under -short")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.CopyN(w, zeroReader{}, maxResponseBytes+1)
	}))
	t.Cleanup(server.Close)

	c := New(&config.Config{BaseURL: server.URL}, 30*time.Second, 0)
	c.HTTPClient = server.Client()
	c.NoCache = true

	_, err := c.Get(context.Background(), "/big", nil)
	if err == nil {
		t.Fatalf("expected an error for an oversized response body, got nil (body was silently truncated)")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want a body-size-limit error", err)
	}
}

// TestReadCappedBody exercises the exact boundary logic do() relies on.
func TestReadCappedBody(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		body    []byte
		max     int64
		wantErr bool
	}{
		{"under cap", []byte("hello"), 10, false},
		{"exactly cap", []byte("0123456789"), 10, false},
		{"one over cap", []byte("0123456789x"), 10, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := readCappedBody(bytes.NewReader(tc.body), tc.max)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error for %d-byte body over %d cap, got body %q", len(tc.body), tc.max, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tc.body) {
				t.Fatalf("body = %q, want %q", got, tc.body)
			}
		})
	}
}
