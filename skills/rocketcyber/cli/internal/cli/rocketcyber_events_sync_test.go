// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// rocketcyber handfix events-not-synced: a default sync walks the published
// resources only. 4.34.2 derived a per-app events dependent from the required
// appId query param; event records carry no primary key, so the walk stored
// keyless rows and fanned out over every detection app's whole event history
// on every sync. events list and events summary stay live reads.

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"rocketcyber-pp-cli/internal/cliutil/testenv"
)

func TestDefaultSyncDoesNotWalkEvents(t *testing.T) {
	testenv.Isolate(t)
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/apps"):
			_, _ = w.Write([]byte(`{"totalCount":2,"currentPage":1,"totalPages":1,"data":[{"id":2,"name":"Advanced Breach Detection"},{"id":3,"name":"System Process Verifier"}]}`))
		case strings.HasSuffix(r.URL.Path, "/events"):
			_, _ = w.Write([]byte(`{"totalCount":1,"currentPage":1,"totalPages":1,"data":[{"appId":2,"deviceId":null,"verdict":"suspicious","detectionDate":"2026-10-01T00:00:00Z","value":"x","valueType":"y"}]}`))
		default:
			_, _ = w.Write([]byte(`{"totalCount":0,"currentPage":1,"totalPages":1,"data":[]}`))
		}
	}))
	defer srv.Close()
	t.Setenv("ROCKETCYBER_BASE_URL", srv.URL+"/v3")
	t.Setenv("ROCKETCYBER_API_TOKEN", "evtSyncTok-6b1f2c9d")

	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"sync", "--json"})
	_ = cmd.Execute()

	mu.Lock()
	defer mu.Unlock()
	if hits["/v3/apps"] == 0 {
		t.Fatalf("sync never listed apps (the test did not reach the fake API): hits=%v\n%s", hits, out.String())
	}
	if n := hits["/v3/events"]; n != 0 {
		t.Fatalf("default sync requested /events %d times; events are a live read, never a sync walk: hits=%v", n, hits)
	}
	for _, d := range dependentResourceDefs() {
		if d.Name == "events" {
			t.Fatalf("events is registered as a sync dependent of %s", d.ParentTable)
		}
	}
}
