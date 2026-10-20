// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
)

// TestWorkflowArchiveFullClearNeverMarksComplete guards
// sync-clear-never-marks-complete on the workflow archive path: --full used
// to clear each cursor with SaveSyncState(resource, "", 0), which stamps the
// collection complete, so when the archive timed out on its first resource
// every collection it never walked read as freshly synced. It also must not archive api-keys, whose rows
// carry API key tokens.
func TestWorkflowArchiveFullClearNeverMarksComplete(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/api_keys") {
			t.Errorf("workflow archive requested %s", r.URL.Path)
		}
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")

	dbPath := filepath.Join(t.TempDir(), "data.db")
	syncHFSeed(t, dbPath, "networks", "users", "organizations")
	_, _ = syncHFRun("workflow", "archive", "--full", "--timeout", "1s", "--db", dbPath)

	for _, r := range []string{"networks", "users", "organizations"} {
		if complete, found := syncHFComplete(t, dbPath, r); found && complete == 1 {
			t.Fatalf("%s reads complete after a --full archive that timed out before walking it:\n%s", r, syncHFState(t, dbPath))
		}
	}
	if _, found := syncHFComplete(t, dbPath, "api-keys"); found {
		t.Fatalf("workflow archive touched api-keys:\n%s", syncHFState(t, dbPath))
	}
}
