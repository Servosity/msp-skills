// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// hand-fix (handfixes.json transactions-wait-terminal-statuses): every
// documented end state other than complete - failed, imaged / timed_out (the
// deployment timed out) and canceled - ends the wait at once with exit 8 and
// the status reported; it is never polled to --max-wait and reported as
// still running (exit 9).
func TestTransactionsWaitEndsOnEveryTerminalStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
			return
		}
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		status := strings.TrimPrefix(id, "tx-")
		_, _ = w.Write([]byte(`[{"id":"` + id + `","device":"FB-1","status":"` + status + `"}]`))
	}))
	defer srv.Close()
	spEnv(t, srv)
	for status, want := range map[string]int{"complete": 0, "failed": 8, "imaged": 8, "timed_out": 8, "canceled": 8} {
		start := time.Now()
		doc, code := runJSON(t, "firebox-transactions", "wait", "tx-"+status, "--interval", "2s", "--max-wait", "20s", "--json")
		if code != want || doc["status"] != status || time.Since(start) > 10*time.Second {
			t.Errorf("%s: exit %d after %s, status %v; want exit %d at once", status, code, time.Since(start).Round(time.Second), doc["status"], want)
		}
	}
}
