// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"

	"github.com/spf13/cobra"

	"veeam-pp-cli/internal/store"
)

// Review round 2 (Codex), written after the review stopped.

// Class: display name used as identity. Two affected objects that share an
// objectName are two problems (AlarmObject.instanceUid tells them apart).
func TestAlarmsTriageKeepsDistinctObjectsApart(t *testing.T) {
	dbPath, db := emptyFleetStore(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seedPath(t, db, veeamPathAlarmTemplates, map[string]any{"instanceUid": "tpl", "name": "Repository low"})
	seedPath(t, db, veeamPathActiveAlarms,
		map[string]any{"instanceUid": "al1", "alarmTemplateUid": "tpl", "repeatCount": float64(1),
			"object":         map[string]any{"instanceUid": "obj-a", "organizationUid": "o", "computerName": "srv-a", "objectName": "Repository"},
			"lastActivation": map[string]any{"status": "Error", "time": now}},
		map[string]any{"instanceUid": "al2", "alarmTemplateUid": "tpl", "repeatCount": float64(1),
			"object":         map[string]any{"instanceUid": "obj-b", "organizationUid": "o", "computerName": "srv-b", "objectName": "Repository"},
			"lastActivation": map[string]any{"status": "Error", "time": now}})
	db.Close()
	alarms := arr(runNovelJSON(t, newNovelAlarmsTriageCmd(&rootFlags{asJSON: true}), "--db", dbPath), "alarms")
	if len(alarms) != 2 {
		t.Fatalf("alarms-triage merged two affected objects: %d groups, want 2 (%v)", len(alarms), alarms)
	}
	for _, a := range alarms {
		if a["count"].(float64) != 1 {
			t.Errorf("group %v counts %v, want 1", a["object"], a["count"])
		}
	}
	if alarms[0]["object"] == alarms[1]["object"] {
		t.Errorf("both groups show the object as %q; the computer that owns each is lost", alarms[0]["object"])
	}
}

// P2: every novel declares its local strategy at runtime, so
// VEEAM_NO_LOCAL_DATA=1 keeps it on the existing mirror.
func TestNovelsDeclareLocalDataSource(t *testing.T) {
	f := &rootFlags{}
	for _, cmd := range []*cobra.Command{newNovelFleetHealthCmd(f), newNovelStaleBackupsCmd(f), newNovelAtRiskCmd(f),
		newNovelAlarmsTriageCmd(f), newNovelLicenseUsageCmd(f), newNovelCompanyOverviewCmd(f), newNovelSinceCmd(f)} {
		if got := commandDataSourceAnnotation(cmd); got != "local" {
			t.Errorf("%s pp:data-source annotation = %q, want local", cmd.Name(), got)
		}
	}
}

// P2: snapshots written before the fixed-width format still order by time.
func TestLegacySnapshotTimestampsOrderByTime(t *testing.T) {
	_, db := emptyFleetStore(t)
	defer db.Close()
	for _, row := range []struct {
		at   string
		used float64
	}{{"2026-11-02T14:00:00.1Z", 100}, {"2026-11-02T14:00:00.11Z", 120}} {
		if _, err := db.DB().Exec(`INSERT INTO veeam_usage_snapshots (org_uid, captured_at, used_points) VALUES ('o', ?, ?)`, row.at, row.used); err != nil {
			t.Fatal(err)
		}
	}
	got, _, ok, err := db.LatestUsageSnapshot(t.Context(), "o", time.Date(2026, 11, 2, 15, 0, 0, 0, time.UTC))
	if err != nil || !ok || got != 120 {
		t.Errorf("latest legacy snapshot = %v (ok=%v err=%v), want 120", got, ok, err)
	}
}

// P2 (same class as the MCP read-only write): VEEAM_NO_LOCAL_DATA=1 means no
// store writes, so license-usage records no snapshot under it.
func TestLicenseUsageHonorsNoLocalData(t *testing.T) {
	dbPath := seedFleetStore(t)
	t.Setenv("VEEAM_NO_LOCAL_DATA", "1")
	runNovelJSON(t, newNovelLicenseUsageCmd(&rootFlags{asJSON: true}), "--db", dbPath)
	db, err := store.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM veeam_usage_snapshots`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("license-usage recorded %d snapshots with VEEAM_NO_LOCAL_DATA=1", n)
	}
}
