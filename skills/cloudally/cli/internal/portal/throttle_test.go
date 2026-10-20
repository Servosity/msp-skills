// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil"
)

func TestPostRetriesAfterThrottling(t *testing.T) {
	f := newFakePortal(t, "tok")
	attempts := 0
	f.handler = func(string, []byte, string) string {
		// Never reached: the throttling is injected by the wrapper below.
		return `{"success":true,"data":null}`
	}
	srv := f.server()
	defer srv.Close()

	s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "tok")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Swap in a transport that throttles twice, then succeeds.
	s.hc.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		if attempts <= 2 {
			return jsonResponse(http.StatusTooManyRequests, `{}`, "0"), nil
		}
		return jsonResponse(http.StatusOK, `{"success":true,"data":{"ok":true}}`, ""), nil
	})

	raw, err := s.CallRead(context.Background(), "getLoggedUser", map[string]any{})
	if err != nil {
		t.Fatalf("CallRead: %v", err)
	}
	if string(raw) != `{"ok":true}` {
		t.Fatalf("data = %s", raw)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestPostGivesUpAfterRepeatedThrottling(t *testing.T) {
	f := newFakePortal(t, "tok")
	f.handler = func(string, []byte, string) string { return `{"success":true}` }
	srv := f.server()
	defer srv.Close()

	s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "tok")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.hc.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusTooManyRequests, `{}`, "0"), nil
	})

	_, err = s.CallRead(context.Background(), "getLoggedUser", map[string]any{})
	if !errors.Is(err, ErrThrottled) {
		t.Fatalf("want ErrThrottled, got %v", err)
	}
}

func TestRetryAfterParsing(t *testing.T) {
	cases := []struct {
		header string
		want   time.Duration
	}{
		{"", defaultRetryAfter},
		{"5", 5 * time.Second},
		{"0", defaultRetryAfter},
		{"-3", defaultRetryAfter},
		{"not-a-number", defaultRetryAfter},
		// A wildly large value is clamped.
		{"99999", maxRetryAfter},
	}
	for _, tc := range cases {
		resp := &http.Response{Header: http.Header{}}
		if tc.header != "" {
			resp.Header.Set("Retry-After", tc.header)
		}
		if got := retryAfter(resp); got != tc.want {
			t.Fatalf("retryAfter(%q) = %s, want %s", tc.header, got, tc.want)
		}
	}
}

func TestPaceSpacesOutCalls(t *testing.T) {
	s := &Session{limiter: cliutil.NewAdaptiveLimiter(25)}
	// The first call never waits.
	start := time.Now()
	if err := s.pace(context.Background()); err != nil {
		t.Fatalf("first pace: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Fatalf("first pace waited %s", elapsed)
	}
	// The next call waits out the remaining interval.
	start = time.Now()
	if err := s.pace(context.Background()); err != nil {
		t.Fatalf("second pace: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("second pace only waited %s", elapsed)
	}
}

func TestPaceHonoursContextCancellation(t *testing.T) {
	s := &Session{limiter: cliutil.NewAdaptiveLimiter(0.001)}
	// The limiter lets the first call through at once, so prime it before
	// checking that a cancelled context cuts the wait short.
	if err := s.pace(context.Background()); err != nil {
		t.Fatalf("priming pace: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.pace(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// jsonResponse builds a canned response, optionally with Retry-After.
func jsonResponse(status int, body, retryAfterHeader string) *http.Response {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	if retryAfterHeader != "" {
		rec.Header().Set("Retry-After", retryAfterHeader)
	}
	rec.WriteHeader(status)
	_, _ = rec.WriteString(body)
	return rec.Result()
}
