// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"datto-rmm-pp-cli/internal/store"
)

// Codex review r1 class "one record counted in two partitions" (datto-rmm
// handfix fleet-alerts-one-record-per-uid): an alert synced while open and
// synced again after Datto resolved it sits in both account-alerts-open
// (the earlier row stays until a full sync prunes it) and
// account-alerts-resolved. The fleet analytics concatenated both partitions,
// so the alert counted twice in storm volume and still counted as open on the
// scorecard. One record per alertUid now, and a resolved record wins.
func TestFleetAlertsCountOneRecordPerAlert(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "alerts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	open := `{"alertUid":"a1","priority":"High","resolved":false,"timestamp":"` + ts + `","alertSourceInfo":{"deviceUid":"d1","deviceName":"PC1","siteUid":"s1","siteName":"Acme"}}`
	resolved := `{"alertUid":"a1","priority":"High","resolved":true,"timestamp":"` + ts + `","alertSourceInfo":{"deviceUid":"d1","deviceName":"PC1","siteUid":"s1","siteName":"Acme"}}`
	still := `{"alertUid":"a2","priority":"High","resolved":false,"timestamp":"` + ts + `","alertSourceInfo":{"deviceUid":"d1","deviceName":"PC1","siteUid":"s1","siteName":"Acme"}}`
	if _, _, err := db.UpsertBatch(fleetAlertsOpenResource, []json.RawMessage{json.RawMessage(open), json.RawMessage(still)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch(fleetAlertsResolvedResource, []json.RawMessage{json.RawMessage(resolved)}); err != nil {
		t.Fatal(err)
	}
	alerts, err := loadFleetAlerts(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 2 {
		t.Fatalf("loaded %d alert records for 2 alerts: %+v", len(alerts), alerts)
	}
	for _, a := range alerts {
		if a.AlertUID == "a1" && !a.Resolved {
			t.Fatalf("the resolved alert a1 still reads as open")
		}
	}
	card, ok := computeScorecard("Acme", []fleetDevice{{UID: "d1", Hostname: "PC1", SiteName: "Acme", SiteUID: "s1"}}, alerts, nil, time.Now())
	if !ok || card.OpenAlerts != 1 {
		t.Fatalf("scorecard open alerts = %d, want 1 (a2 only)", card.OpenAlerts)
	}
}
