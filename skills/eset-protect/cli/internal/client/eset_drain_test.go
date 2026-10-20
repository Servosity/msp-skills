// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestPendingBodyThatNeverEndsDoesNotHang guards pending-body-drain-is-bounded
// (Codex round 2): a 202 with a response-id whose body never finishes must
// not hang the transport. The client here has no overall timeout and no
// context deadline, exactly like the streaming quarantine-download client
// once headers have arrived; before the fix drain blocked on the body forever.
func TestPendingBodyThatNeverEndsDoesNotHang(t *testing.T) {
	savedStart, savedWait := PendingPollStart, drainMaxWait
	PendingPollStart, drainMaxWait = 10*time.Millisecond, 200*time.Millisecond
	t.Cleanup(func() { PendingPollStart, drainMaxWait = savedStart, savedWait })

	release := make(chan struct{})
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			// Pending answer: headers, a few bytes, then a body that never ends.
			w.Header().Set("response-id", "rid-1")
			w.Header().Set("Content-Length", "1048576")
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"pending":`)
			w.(http.Flusher).Flush()
			select {
			case <-release:
			case <-r.Context().Done():
			}
			return
		}
		if r.Header.Get("response-id") != "rid-1" {
			t.Errorf("the poll must carry the response-id, got %q", r.Header.Get("response-id"))
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(func() { close(release); srv.Close() })

	hc := &http.Client{Transport: &EsetTransport{Base: &http.Transport{}}}
	done := make(chan error, 1)
	var body string
	go func() {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/v1/quarantined-objects:download", strings.NewReader(`{}`))
		resp, err := hc.Do(req)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			body = string(b)
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil || body != `{"ok":true}` {
			t.Fatalf("the poll after the stalled 202 must return the answer: err=%v body=%q", err, body)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a 202 whose body never finishes hung the transport past 10s; drain must be bounded")
	}
}
