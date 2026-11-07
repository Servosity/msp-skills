// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// Codex review r1 class "incomplete sync reported healthy" (datto-rmm handfix
// sync-gaps-any-failure-signal-wins): `fleet sync-gaps` compared stored rows
// with the count sync recorded, which is the count of rows it stored, and
// ignored the store's own last_attempt_complete flag, so a sync that stopped
// before the documented next page read as "ok" with no gaps. A partition
// whose last attempt did not finish is a gap whatever its counts say.
func TestSyncGapsReportsAnUnfinishedPartition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v2/account/alerts/open" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("page") == "1" {
			_, _ = w.Write([]byte(`{"pageDetails":{"count":1,"totalCount":2,"prevPageUrl":null,"nextPageUrl":null},"alerts":[{"alertUid":"a2","priority":"High","resolved":false,"timestamp":"2026-10-28T10:00:00Z"}]}`))
			return
		}
		next := "http://" + r.Host + "/api/v2/account/alerts/open?page=1&max=1"
		_, _ = w.Write([]byte(`{"pageDetails":{"count":1,"totalCount":2,"prevPageUrl":null,"nextPageUrl":"` + next + `"},"alerts":[{"alertUid":"a1","priority":"High","resolved":false,"timestamp":"2026-10-28T09:00:00Z"}]}`))
	}))
	t.Cleanup(srv.Close)
	home := t.TempDir()
	db := filepath.Join(home, "gaps.db")
	env := map[string]string{"DATTO_RMM_BASE_URL": srv.URL + "/api", "DATTO_RMM_TOKEN": "gapsToken-6e6e"}
	_, stderr, _ := runDattoAt(t, home, env, "sync", "--resources", "account-alerts-open", "--resource-param", "account-alerts-open:max=1",
		"--max-pages", "1", "--json", "--db", db)
	stdout, stderr2, err := runDattoAt(t, home, env, "fleet", "sync-gaps", "--json", "--db", db)
	if err != nil {
		t.Fatalf("fleet sync-gaps: %v\n%s\n%s", err, stderr, stderr2)
	}
	var view syncGapsView
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatalf("output: %v\n%s", err, stdout)
	}
	for _, r := range view.Resources {
		if r.Resource == fleetAlertsOpenResource && r.Status == "ok" {
			t.Fatalf("a partition whose sync stopped before the linked next page reads as ok: %+v (sync: %s)", r, stderr)
		}
	}
	if view.Gaps == 0 {
		t.Fatalf("sync-gaps reports no gaps after an unfinished sync: %s", stdout)
	}

	// The other direction: a sync that walked every page reads as ok.
	full := filepath.Join(home, "full.db")
	if _, stderr, err := runDattoAt(t, home, env, "sync", "--resources", "account-alerts-open", "--full", "--json", "--db", full); err != nil {
		t.Fatalf("full sync: %v\n%s", err, stderr)
	}
	stdout, _, err = runDattoAt(t, home, env, "fleet", "sync-gaps", "--json", "--db", full)
	if err != nil {
		t.Fatal(err)
	}
	view = syncGapsView{}
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatalf("output: %v\n%s", err, stdout)
	}
	okSeen := false
	for _, r := range view.Resources {
		if r.Resource == fleetAlertsOpenResource {
			okSeen = r.Status == "ok" && r.StoredRows == 2
		}
	}
	if !okSeen {
		t.Fatalf("a complete sync does not read as ok: %s", stdout)
	}
}
