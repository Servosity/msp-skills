// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"threatdown-pp-cli/internal/cliutil/testenv"
	"threatdown-pp-cli/internal/fleet"
	"threatdown-pp-cli/internal/store"
)

// Codex review r1 (2026-11-02, 4.35.1 reprint), two P1 classes in fleet sync:
//   - rows of an account that is no longer in the console survived a complete
//     sync (GET /v1/sites -> {"sites":[]} skipped every reconcile), so local
//     fleet stale still listed its endpoints;
//   - a full record was merged into the stored one, so a field the console
//     cleared (a site's account_id is "" without a subscription, the schema
//     default) kept its old value and customer report used the old account.
//
// Hand-fix (handfixes.json fleet-sync-mirrors-the-console).
func TestFleetSyncMirrorsTheConsole(t *testing.T) {
	testenv.Isolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	seed := map[string]string{
		tdResSites:      `{"id":"s1","account_id":"a1","company_name":"Acme"}`,
		tdResEndpoints:  `{"id":"e1","account_id":"a1","host_name":"OLD","infection_count":1,"last_day_seen":"2026-10-01T00:00:00Z"}`,
		tdResDetections: `{"id":"d1","account_id":"a1","threat_name":"x","reported_at":"2026-10-30T00:00:00Z"}`,
		tdResJobs:       `{"id":"j1","account_id":"a1","status":1,"issued_at":"2026-10-30T00:00:00Z"}`,
	}
	for res, row := range seed {
		if _, _, err := db.UpsertBatch(res, []json.RawMessage{json.RawMessage(row)}); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case strings.HasSuffix(r.URL.Path, "/v1/sites"):
			_, _ = w.Write([]byte(`{"sites":[{"id":"s1","account_id":"","company_name":"Acme"}]}`))
		default:
			_, _ = w.Write([]byte(`{"endpoints":[],"detections":[],"jobs":[],"next_cursor":""}`))
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("THREATDOWN_BASE_URL", srv.URL+"/oneview")
	t.Setenv("THREATDOWN_TOKEN_URL", srv.URL+"/oneview/oauth2/token")
	t.Setenv("THREATDOWN_CLIENT_ID", "cid")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "secret")
	if out, err := runRoot(t, "fleet", "sync", "--db", dbPath, "--json"); err != nil {
		t.Fatalf("fleet sync: %v\n%s", err, out)
	}

	db, err = store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	site, err := db.Get(tdResSites, "s1")
	if err != nil {
		t.Fatal(err)
	}
	var s fleet.Site
	if err := json.Unmarshal(site, &s); err != nil || s.AccountID != "" {
		t.Fatalf("the cleared account_id must be stored as the console sent it, got %s", site)
	}
	for _, res := range []string{tdResEndpoints, tdResDetections, tdResJobs} {
		if rows, err := db.List(res, 0); err != nil || len(rows) != 0 {
			t.Fatalf("%s rows of an account the console no longer has must be removed: %d left (%v)", res, len(rows), err)
		}
	}
}

// Codex review r1 (2026-11-02): RFC 3339 allows a lowercase t and z, which
// ParseTime refused, and respond pending then dropped a waiting job whose
// issued_at it could not read instead of listing it.
// Hand-fix (handfixes.json timestamps-never-hide-a-row).
func TestUnreadableTimestampNeverHidesAJob(t *testing.T) {
	if _, ok := fleet.ParseTime("2026-11-01t00:00:00z"); !ok {
		t.Fatal("lowercase RFC 3339 separators must parse")
	}
	now := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	jobs := []jobRow{
		{ID: "j1", AccountID: "a1", Status: 1, IssuedAt: "2026-11-01t00:00:00z"},
		{ID: "j2", AccountID: "a1", Status: 1, IssuedAt: "yesterday-ish"},
	}
	rows := selectPending(jobs, nil, now, now.Add(-7*24*time.Hour), time.Hour, "")
	if len(rows) != 2 {
		t.Fatalf("both waiting jobs must be listed, got %d", len(rows))
	}
}
