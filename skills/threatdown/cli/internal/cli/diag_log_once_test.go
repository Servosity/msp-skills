// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"threatdown-pp-cli/internal/cliutil/testenv"
)

// hand-fix (handfixes.json diag-log-is-endpoint-action): the CLI sends
// diag-log once, like a write. A dropped response must not trigger a retry
// that makes the endpoint generate a second log.
func TestDiagLogIsNeverRetried(t *testing.T) {
	testenv.Isolate(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth2/token") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/diag-log") {
			hits.Add(1)
			// Drop the connection after the request arrived: an ambiguous failure.
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					_ = conn.Close()
					return
				}
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	// Fresh connection per request: net/http itself replays a GET once when a
	// REUSED keep-alive connection closes before any response, which is not
	// the client retry under test.
	srv.Config.SetKeepAlivesEnabled(false)
	t.Cleanup(srv.Close)
	t.Setenv("THREATDOWN_BASE_URL", srv.URL+"/oneview")
	t.Setenv("THREATDOWN_TOKEN_URL", srv.URL+"/oneview/oauth2/token")
	t.Setenv("THREATDOWN_CLIENT_ID", "cid")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "secret")

	if _, err := runRoot(t, "endpoints", "diag-log", "a1", "e1", "--json"); err == nil {
		t.Fatal("want an error for a dropped connection")
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("diag-log reached the API %d times, want exactly 1", got)
	}
}

// --data-source local can never fire the endpoint action.
func TestDiagLogRefusesLocalDataSource(t *testing.T) {
	testenv.Isolate(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/diag-log") {
			hits.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("THREATDOWN_BASE_URL", srv.URL+"/oneview")
	t.Setenv("THREATDOWN_TOKEN_URL", srv.URL+"/oneview/oauth2/token")
	t.Setenv("THREATDOWN_CLIENT_ID", "cid")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "secret")
	if _, err := runRoot(t, "endpoints", "diag-log", "a1", "e1", "--data-source", "local", "--json"); err == nil {
		t.Fatal("want --data-source local refused")
	}
	if hits.Load() != 0 {
		t.Fatalf("diag-log reached the API %d times under --data-source local", hits.Load())
	}
}

// A job row with an odd ancillary field is kept; an unreadable one is counted.
func TestParseJobsNeverDropsSilently(t *testing.T) {
	jobs, unreadable := parseJobs([]json.RawMessage{
		json.RawMessage(`{"id":"j1","account_id":"a1","status":4,"online":"true"}`),
		json.RawMessage(`{"id":"j2","account_id":"a1","status":"failed"}`),
		json.RawMessage(`{"id":"j3","account_id":"a1","status":4}`),
	})
	if len(jobs) != 2 || jobs[0].ID != "j1" || unreadable != 1 {
		t.Fatalf("jobs=%+v unreadable=%d, want j1+j3 kept and 1 unreadable", jobs, unreadable)
	}
	if unreadableJobsNote(unreadable) == "" {
		t.Fatal("unreadable rows must produce a note")
	}
}
