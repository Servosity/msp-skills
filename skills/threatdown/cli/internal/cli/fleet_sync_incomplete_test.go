// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"threatdown-pp-cli/internal/cliutil/testenv"
	"threatdown-pp-cli/internal/store"
)

// Codex review round 7 (2026-10-14): fleet sync dropped a failed job whose
// online field had an odd type, and the earlier freshness stamp let local
// respond pending print an all-clear over it.
func TestFleetSyncNeverLeavesASilentGap(t *testing.T) {
	odd := json.RawMessage(`{"id":"j3","account_id":"a1","status":64,"online":"true"}`)
	if kept, bad := validJobs([]json.RawMessage{odd}); len(kept) != 1 || bad != 0 {
		t.Fatalf("a job with an odd online field must be stored: kept %d bad %d", len(kept), bad)
	}

	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if stored, failed, err := db.UpsertBatch(tdResJobs, []json.RawMessage{odd}); err != nil || stored != 1 || failed != 0 {
		t.Fatalf("the store must keep the odd job row: stored %d failed %d err %v", stored, failed, err)
	}
	if raws, err := loadLocal(context.Background(), db, tdResJobs); err != nil || len(raws) != 1 {
		t.Fatalf("local readers must see the stored job: %d rows, err %v", len(raws), err)
	} else if jobs, bad := parseJobs(raws); len(jobs) != 1 || bad != 0 {
		t.Fatalf("parseJobs must read the stored job: %d rows, %d unreadable", len(jobs), bad)
	}
	if err := db.SaveSyncState(tdResJobs, "", 1); err != nil {
		t.Fatal(err)
	}
	if n := incompleteSyncNote(db, tdResJobs); n != "" {
		t.Fatalf("a complete sync needs no note, got %q", n)
	}
	r := fleetSyncResult{Resource: tdResJobs}
	recordFreshness(db, &r, false, true, false, "")
	if n := incompleteSyncNote(db, tdResJobs); n == "" {
		t.Fatal("an unconfirmed full sync must leave a note for local readers")
	}
	r = fleetSyncResult{Resource: tdResJobs}
	recordFreshness(db, &r, false, true, true, "")
	if n := incompleteSyncNote(db, tdResJobs); n != "" {
		t.Fatalf("the next complete sync must clear the note, got %q", n)
	}
	recordFreshness(db, &r, false, false, false, "")
	if n := incompleteSyncNote(db, tdResJobs); n != "" {
		t.Fatalf("a --site scoped sync must not flag the fleet-wide copy, got %q", n)
	}
}

// Adversarial review round 11 (2026-10-14): generic sync's "endpoints"
// resource was one endpoint's quarantine list stored in the fleet endpoint
// mirror's table; with --full it deleted the real endpoints.
func TestGenericSyncLeavesTheEndpointMirrorAlone(t *testing.T) {
	if _, err := syncResourcePath(tdResEndpoints); err == nil || !strings.Contains(err.Error(), "fleet sync") {
		t.Fatalf("generic sync must refuse %q and point at fleet sync, got %v", tdResEndpoints, err)
	}
	for _, n := range knownSyncResourceNames() {
		if n == tdResEndpoints {
			t.Fatalf("generic sync must not advertise %q", n)
		}
	}
}

// Adversarial review round 12 (2026-10-14): generic sync --full zero-stamped
// fleet-owned sync state before rejecting the name; a first-ever incomplete
// fleet sync left no marker; the --max-pages note was unreachable; auth login
// sent a placeholder secret to the token endpoint.
func TestRound12SyncAndLoginEdges(t *testing.T) {
	testenv.Isolate(t)
	home := t.TempDir()
	t.Setenv("THREATDOWN_HOME", home)
	dbPath := filepath.Join(home, "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	r := fleetSyncResult{Resource: tdResEndpoints}
	recordFreshness(db, &r, false, true, false, "")
	if incompleteSyncNote(db, tdResEndpoints) == "" {
		t.Fatal("a first-ever incomplete fleet sync must leave the marker")
	}
	r = fleetSyncResult{Resource: tdResDetections}
	recordFreshness(db, &r, true, true, false, "")
	if !strings.Contains(r.Note, "--max-pages") {
		t.Fatalf("a capped sync must say so, got %q", r.Note)
	}
	db.Close()

	for _, res := range []string{tdResEndpoints, tdResDetections, tdResJobs} {
		cmd := RootCmd()
		cmd.SetArgs([]string{"sync", "--resources", res, "--full", "--db", dbPath})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "fleet sync") {
			t.Fatalf("generic sync must refuse %q and point at fleet sync, got %v", res, err)
		}
	}
	db, err = store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if incompleteSyncNote(db, tdResEndpoints) == "" {
		t.Fatal("a refused generic sync must not reset fleet sync state")
	}

	minted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { minted = true }))
	defer srv.Close()
	t.Setenv("THREATDOWN_TOKEN_URL", srv.URL+"/oneview/oauth2/token")
	t.Setenv("THREATDOWN_CLIENT_ID", "admin")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "<your-client-secret>")
	cmd := RootCmd()
	cmd.SetArgs([]string{"auth", "login", "--client-id", "admin"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "$THREATDOWN_CLIENT_SECRET") || minted {
		t.Fatalf("auth login must refuse an env placeholder without minting: err=%v minted=%v", err, minted)
	}
}
