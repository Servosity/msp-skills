// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written tests for the campaign-threats novel feature.

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"proofpoint-pp-cli/internal/store"
)

func TestNamesOf(t *testing.T) {
	pairs := []campaignIDName{{ID: "1", Name: "TA000"}, {ID: "2", Name: ""}, {ID: "3", Name: "Loader"}}
	names := namesOf(pairs)
	if len(names) != 2 || names[0] != "TA000" || names[1] != "Loader" {
		t.Fatalf("namesOf must keep non-empty names in order: %+v", names)
	}
}

func campaignDetailFixture() campaignDetail {
	var detail campaignDetail
	_ = json.Unmarshal([]byte(`{
		"id": "camp-1",
		"name": "Invoice wave",
		"actors": [{"id":"a1","name":"TA000"}],
		"campaignMembers": [
			{"id": "threat-synced", "threat": "https://evil.example.test/a", "type": "url", "subType": "COMPLETE_URL", "threatTime": "2026-06-01T00:00:00Z"},
			{"id": "threat-unsynced", "threat": "aa11bb22", "type": "attachment", "subType": "ATTACHMENT", "threatTime": "2026-06-02T00:00:00Z"}
		]
	}`), &detail)
	return detail
}

func TestEnrichThreatRowsWithLocalStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	defer db.Close()

	threat := json.RawMessage(`{"id":"threat-synced","name":"Invoice phish","severity":800,"category":"phish","status":"active","type":"url"}`)
	if err := db.UpsertThreat(threat); err != nil {
		t.Fatalf("seeding threat: %v", err)
	}

	rows, enriched := enrichThreatRows(context.Background(), db.DB(), campaignDetailFixture())
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if enriched != 1 {
		t.Fatalf("enriched = %d, want 1", enriched)
	}
	var synced, unsynced *campaignThreatRow
	for i := range rows {
		switch rows[i].ThreatID {
		case "threat-synced":
			synced = &rows[i]
		case "threat-unsynced":
			unsynced = &rows[i]
		}
	}
	if synced == nil || !synced.Enriched || synced.Severity == nil || *synced.Severity != 800 || synced.Category != "phish" {
		t.Fatalf("synced row not enriched from local threat table: %+v", synced)
	}
	if unsynced == nil || unsynced.Enriched || unsynced.Severity != nil {
		t.Fatalf("unsynced row must remain un-enriched, not fail: %+v", unsynced)
	}
}

func TestEnrichThreatRowsNilDB(t *testing.T) {
	rows, enriched := enrichThreatRows(context.Background(), nil, campaignDetailFixture())
	if len(rows) != 2 || enriched != 0 {
		t.Fatalf("nil db must still list members un-enriched: rows=%d enriched=%d", len(rows), enriched)
	}
}
