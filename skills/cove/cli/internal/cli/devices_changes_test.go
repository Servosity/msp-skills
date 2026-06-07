// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cove-pp-cli/internal/store"
)

// seedSnapshots writes two snapshots into a temp store and returns its path.
func seedSnapshots(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "cove.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	base := time.Now().Add(-24 * time.Hour)
	if _, err := db.InsertCoveSnapshot(ctx, base, 9000, []store.CoveDeviceStat{
		{AccountID: 1, DeviceName: "regressor", Customer: "Acme", LastStatus: 5, UsedStorage: 1000},
		{AccountID: 2, DeviceName: "recoverer", Customer: "Acme", LastStatus: 2, UsedStorage: 5000},
		{AccountID: 3, DeviceName: "steady", Customer: "Globex", LastStatus: 5, UsedStorage: 800},
		{AccountID: 4, DeviceName: "retired", Customer: "Globex", LastStatus: 5, UsedStorage: 300},
	}); err != nil {
		t.Fatalf("seed baseline: %v", err)
	}
	if _, err := db.InsertCoveSnapshot(ctx, time.Now(), 9000, []store.CoveDeviceStat{
		{AccountID: 1, DeviceName: "regressor", Customer: "Acme", LastStatus: 2, UsedStorage: 1500},
		{AccountID: 2, DeviceName: "recoverer", Customer: "Acme", LastStatus: 5, UsedStorage: 5000},
		{AccountID: 3, DeviceName: "steady", Customer: "Globex", LastStatus: 5, UsedStorage: 800},
		{AccountID: 5, DeviceName: "newcomer", Customer: "Globex", LastStatus: 5, UsedStorage: 100},
	}); err != nil {
		t.Fatalf("seed latest: %v", err)
	}
	return dbPath
}

func TestDevicesChangesDetectsFlips(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dbPath := seedSnapshots(t)
	out, err := runCoveCmd(t, "devices", "changes", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("devices changes: %v\n%s", err, out)
	}
	var view struct {
		Items []struct {
			DeviceName string `json:"device_name"`
			FromName   string `json:"from_name"`
			ToName     string `json:"to_name"`
			Kind       string `json:"kind"`
		} `json:"items"`
		Added   []string `json:"added_devices"`
		Removed []string `json:"removed_devices"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if len(view.Items) != 2 {
		t.Fatalf("expected regression + recovery only, got %+v", view.Items)
	}
	// Regressions sort first.
	if view.Items[0].DeviceName != "regressor" || view.Items[0].Kind != "regression" ||
		view.Items[0].FromName != "Completed" || view.Items[0].ToName != "Failed" {
		t.Fatalf("regression row wrong: %+v", view.Items[0])
	}
	if view.Items[1].DeviceName != "recoverer" || view.Items[1].Kind != "recovery" {
		t.Fatalf("recovery row wrong: %+v", view.Items[1])
	}
	if len(view.Added) != 1 || view.Added[0] != "newcomer" {
		t.Fatalf("added devices wrong: %v", view.Added)
	}
	if len(view.Removed) != 1 || view.Removed[0] != "retired" {
		t.Fatalf("removed devices wrong: %v", view.Removed)
	}
	if strings.Contains(out, `"device_name": "steady"`) {
		t.Fatalf("unchanged device leaked into changes:\n%s", out)
	}
}

func TestDevicesChangesNeedsTwoSnapshots(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dbPath := filepath.Join(t.TempDir(), "cove.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := db.InsertCoveSnapshot(context.Background(), time.Now(), 9000, []store.CoveDeviceStat{
		{AccountID: 1, DeviceName: "only", Customer: "Acme", LastStatus: 5},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	db.Close()
	out, err := runCoveCmd(t, "devices", "changes", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("devices changes single-snapshot: %v\n%s", err, out)
	}
	if !strings.Contains(out, "only one qualifying snapshot") {
		t.Fatalf("expected single-snapshot guidance note:\n%s", out)
	}
}

func TestDevicesChangesEmptyStoreGuidance(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	out, err := runCoveCmd(t, "devices", "changes", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("devices changes empty store: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no snapshots yet") {
		t.Fatalf("expected no-snapshots guidance:\n%s", out)
	}
	_ = os.Remove(dbPath)
}
