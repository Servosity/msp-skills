// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"threatdown-pp-cli/internal/cliutil/testenv"
	"threatdown-pp-cli/internal/fleet"
	"threatdown-pp-cli/internal/store"
)

// Codex review r2 (2026-11-02): replacing every write to a fleet resource let
// a partial mutation response (PUT /v1/sites/{id} answers without
// account_id) erase the account mapping fleet sync stored. Only fleet sync's
// own whole-record writes replace; every other writer still merges.
// Hand-fix (handfixes.json fleet-sync-mirrors-the-console).
func TestPartialSiteResponseKeepsTheAccount(t *testing.T) {
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, _, err := db.UpsertBatchReplacing(tdResSites, []json.RawMessage{json.RawMessage(`{"id":"s1","account_id":"a1","company_name":"Acme"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := db.Upsert(tdResSites, "s1", json.RawMessage(`{"id":"s1","company_name":"Acme","email":"a@example.com"}`)); err != nil {
		t.Fatal(err)
	}
	raw, err := db.Get(tdResSites, "s1")
	if err != nil {
		t.Fatal(err)
	}
	var s fleet.Site
	if json.Unmarshal(raw, &s) != nil || s.AccountID != "a1" {
		t.Fatalf("a partial write erased the synced account mapping: %s", raw)
	}
}

// Codex review r2 (2026-11-02): fleet sync read the site list through the
// 5-minute response cache, so an older empty list (cached by `sites list`)
// became deletion evidence for an account the console still has.
// Hand-fix (handfixes.json fleet-sync-mirrors-the-console).
func TestFleetSyncNeverDeletesOnACachedSiteList(t *testing.T) {
	testenv.Isolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch(tdResEndpoints, []json.RawMessage{json.RawMessage(`{"id":"e1","account_id":"a1","host_name":"PC1"}`)}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	var siteCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case strings.HasSuffix(r.URL.Path, "/v1/sites"):
			if siteCalls.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"sites":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"sites":[{"id":"s1","account_id":"a1","company_name":"Acme"}]}`))
		case strings.HasSuffix(r.URL.Path, "/v1/endpoints"):
			_, _ = w.Write([]byte(`{"endpoints":[{"id":"e1","account_id":"a1","host_name":"PC1"}],"next_cursor":""}`))
		default:
			_, _ = w.Write([]byte(`{"detections":[],"jobs":[],"next_cursor":""}`))
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("THREATDOWN_BASE_URL", srv.URL+"/oneview")
	t.Setenv("THREATDOWN_TOKEN_URL", srv.URL+"/oneview/oauth2/token")
	t.Setenv("THREATDOWN_CLIENT_ID", "cid")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "secret")
	if out, err := runRoot(t, "sites", "list", "--json"); err != nil {
		t.Fatalf("sites list: %v\n%s", err, out)
	}
	if out, err := runRoot(t, "fleet", "sync", "--db", dbPath, "--json"); err != nil {
		t.Fatalf("fleet sync: %v\n%s", err, out)
	}
	db, err = store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if rows, _ := db.List(tdResEndpoints, 0); len(rows) != 1 {
		t.Fatalf("a cached empty site list deleted the endpoint mirror: %d rows (site GETs: %d)", len(rows), siteCalls.Load())
	}
}
