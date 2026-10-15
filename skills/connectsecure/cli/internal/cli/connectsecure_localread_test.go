// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"connectsecure-pp-cli/internal/store"
)

// Generated local reads must see synced rows under ConnectSecure's /r/ paths.
func TestLocalReadTreatsRAsRoutePrefix(t *testing.T) {
	for _, c := range [][2]string{{"companies", "/r/company/companies"}, {"companies", "/r/company/companies/7"}, {"report_queries-risk-score", "/r/report_queries/risk_score"}} {
		if parents := localReadPathParents(c[0], c[1]); len(parents) != 0 {
			t.Errorf("%s: /r/ must not become a parent scope, got %+v", c[1], parents)
		}
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows := []json.RawMessage{json.RawMessage(`{"id": 1, "name": "Acme"}`), json.RawMessage(`{"id": 2, "name": "Beta"}`)}
	if _, _, err := db.UpsertBatch("companies", rows); err != nil {
		t.Fatal(err)
	}
	items, _, _, _, sawValid, err := loadLocalList(db, "companies", "companies", "/r/company/companies", map[string]string{})
	if err != nil || !sawValid || len(items) != 2 {
		t.Fatalf("local list = %d items (sawValid=%v, err=%v); want 2", len(items), sawValid, err)
	}
}

// An interrupted or capped re-sync must not read as a complete snapshot.
func TestLocalDataHintsIncompleteLastSync(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("companies", []json.RawMessage{json.RawMessage(`{"id": 1, "name": "Acme"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncState("companies", "", 1); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncProgress("assets", "100", 1); err != nil {
		t.Fatal(err)
	}
	db.Close()
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	if _, _, err := csLocalData(cmd, &rootFlags{}, dbPath, "companies", "assets"); err != nil {
		t.Fatal(err)
	}
	if got := stderr.String(); !strings.Contains(got, "last sync of assets did not finish") || strings.Contains(got, "last sync of companies") {
		t.Fatalf("want an incomplete-sync hint for assets only, got:\n%s", got)
	}
}
