// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/config"
)

// TestTokenUnavailableRefusesEveryRequest: a client whose ESET transport
// carries a token outage sends nothing, serves no cached response (even a
// fresh entry for the same request), and returns the recorded failure itself
// (same text, same wrapped cause) from every method, with no retry.
func TestTokenUnavailableRefusesEveryRequest(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"devices":[{"uuid":"live"}]}`))
	}))
	t.Cleanup(srv.Close)
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.cacheDir = t.TempDir()
	tr := &EsetTransport{Base: srv.Client().Transport}
	c.HTTPClient = &http.Client{Transport: tr}
	if _, err := c.Get(context.Background(), "/v1/devices", nil); err != nil {
		t.Fatal(err)
	}
	c.writeCacheWithHeaders("/v1/devices", nil, nil, json.RawMessage(`{"devices":[{"uuid":"cached"}]}`))
	if got, ok := c.readCacheWithHeaders("/v1/devices", nil, nil); !ok || string(got) == "" {
		t.Fatalf("the test needs a fresh cache entry")
	}
	atomic.StoreInt32(&hits, 0)
	cause := errors.New("ESET Connect token request (eu region) failed: HTTP 503")
	tr.SetTokenUnavailable(cause)
	start := time.Now()
	calls := map[string]func() (json.RawMessage, error){
		"Get": func() (json.RawMessage, error) { return c.Get(context.Background(), "/v1/devices", nil) },
		"GetNoCache": func() (json.RawMessage, error) {
			return c.GetNoCache(context.Background(), "/v1/devices", nil)
		},
		"Post": func() (json.RawMessage, error) {
			d, _, err := c.Post(context.Background(), "/v1/device_tasks", map[string]any{})
			return d, err
		},
		"Delete": func() (json.RawMessage, error) {
			d, _, err := c.Delete(context.Background(), "/v1/x")
			return d, err
		},
	}
	for name, call := range calls {
		data, err := call()
		if err == nil || data != nil {
			t.Fatalf("%s: got data %s, err %v; want the token outage", name, data, err)
		}
		if !IsTokenUnavailable(err) || !errors.Is(err, cause) || err.Error() != cause.Error() {
			t.Fatalf("%s: err %v is not the recorded outage", name, err)
		}
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("%d request(s) were sent without a token", n)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("the refusal was retried: %s", d)
	}
}
