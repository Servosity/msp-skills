// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "write-outcome-from-wire" entry.

package client

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"saas-alerts-pp-cli/internal/config"
)

// TestWriteOutcomeUnknownFollowsTheWire: a write is a known "no" only when
// the request never reached the server or the server refused it (4xx, 429);
// a 5xx, an unreadable or malformed answer, or a dropped connection after the
// request was written leaves its outcome unknown.
func TestWriteOutcomeUnknownFollowsTheWire(t *testing.T) {
	answer := func(w http.ResponseWriter, r *http.Request) {}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { answer(w, r) }))
	defer srv.Close()
	post := func(base string) error {
		c := New(&config.Config{BaseURL: base, AuthHeaderVal: "test-key-0000", SaasAlertsApiKey: "test-key-0000"}, 5*time.Second, 0)
		_, _, err := c.Post(context.Background(), "/reportApi/api/v1/customers", map[string]any{"name": "x"})
		return err
	}
	hijack := func(reply string) func(w http.ResponseWriter, r *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			_, _ = conn.Write([]byte(reply))
			_ = conn.Close()
		}
	}
	status := func(code int) func(w http.ResponseWriter, r *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"x"}`))
		}
	}
	for _, tc := range []struct {
		name    string
		answer  func(w http.ResponseWriter, r *http.Request)
		unknown bool
	}{
		{"400 refused", status(http.StatusBadRequest), false},
		{"429 refused", status(http.StatusTooManyRequests), false},
		{"502 unknown", status(http.StatusBadGateway), true},
		{"connection dropped after the request", hijack(""), true},
		{"malformed answer after the request", hijack("garbage-not-http\r\n\r\n"), true},
	} {
		answer = tc.answer
		err := post(srv.URL)
		if err == nil || WriteOutcomeUnknown(err) != tc.unknown {
			t.Fatalf("%s: err=%v unknown=%v, want %v", tc.name, err, WriteOutcomeUnknown(err), tc.unknown)
		}
	}

	// Nothing listening: the request never reached a server.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := "http://" + ln.Addr().String()
	_ = ln.Close()
	if err := post(closed); err == nil || WriteOutcomeUnknown(err) || !IsNotSent(err) {
		t.Fatalf("a refused connection must be a known no: err=%v", err)
	}
}
