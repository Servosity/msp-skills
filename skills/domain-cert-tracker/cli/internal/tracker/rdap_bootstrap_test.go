// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package tracker

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Sweep looks domains up from several goroutines; the bootstrap registry is
// fetched once and read without a data race (run with -race).
func TestRDAPLookupLoadsBootstrapOnceConcurrently(t *testing.T) {
	var boots atomic.Int32
	rt := rtFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"objectClassName":"domain","ldhName":"example.com","events":[{"eventAction":"expiration","eventDate":"2030-01-01T00:00:00Z"}]}`
		if r.URL.String() == IANABootstrapURL {
			boots.Add(1)
			body = `{"services":[[["com"],["https://rdap.example/"]]]}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	r := &RDAP{Web: &Web{HTTP: &http.Client{Transport: rt}, MaxBytes: 1 << 20}}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.Lookup(context.Background(), "example.com"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := boots.Load(); n != 1 {
		t.Fatalf("bootstrap fetched %d times, want 1", n)
	}
}
