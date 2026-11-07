// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): tactical-rmm handfix action-gets-need-confirm.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tactical-rmm-pp-cli/internal/config"
)

// A plain read (Get) never reaches a Tactical RMM action route, whatever path
// an id or parameter spells it with: Django decodes %2F before routing
// (Codex review r2: export agents bulkrecovery; read agents/<id>%2Fping).
// Only GetMutating, the explicit action commands' call, may.
func TestReadsNeverReachActionRoutes(t *testing.T) {
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.EscapedPath()]++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL}, time.Second, 0)
	c.NoCache = true
	ctx := context.Background()

	for _, p := range []string{
		"/agents/bulkrecovery/",
		"/core/clearcache/",
		"/core/clearcache",
		"/agents/abc/ping/",
		"/agents/abc%2Fping/",
		"/agents/abc%252Fping/",
		"/agents/abc%2fprocesses/",
		"/agents/abc/processes/4321/",
		"/agents/abc/eventlog/System/7/",
		"/agents/abc/registry/",
		"/services/abc/",
		"/services/abc%2FSpooler/",
		"/agents/x/..%2Fbulkrecovery/",
		"/agents/x/%2E%2E/bulkrecovery/",
	} {
		if _, err := c.Get(ctx, p, nil); err == nil {
			t.Errorf("Get %s reached the server; a read must not reach an action route", p)
		}
	}
	if len(hits) != 0 {
		t.Fatalf("action routes reached by plain reads: %v", hits)
	}
	for _, p := range []string{"/agents/", "/agents/abc/", "/agents/abc/checks/", "/clients/sites/", "/services/"} {
		if _, err := c.Get(ctx, p, nil); err != nil {
			t.Errorf("Get %s refused: %v", p, err)
		}
	}
	if _, err := c.GetMutating(ctx, "/agents/bulkrecovery/", nil); err != nil {
		t.Errorf("GetMutating (the explicit action command) refused: %v", err)
	}
}
