// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Codex review round 2, class "wrong result": one exact column parser (a
// value that is not exactly an integer is not reported, never rounded and
// never zero) and one snapshot comparison rule (two snapshots are compared
// only when both carry the same known endpoint and partner tree).
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"cove-pp-cli/internal/cliutil"
	"cove-pp-cli/internal/cliutil/testenv"
	"cove-pp-cli/internal/store"
)

const r2Endpoint = "POST https://api.backup.management/jsonapi"

// legacyPair writes two snapshots whose endpoint is unknown (written before
// endpoint tracking) for the same partner, one minute apart.
func legacyPair(t *testing.T, first, second []store.CoveDeviceStat) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Unix(1792800000, 0)
	if _, err := db.InsertCoveSnapshot(t.Context(), t0, 7, first); err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertCoveSnapshot(t.Context(), t0.Add(time.Minute), 7, second); err != nil {
		t.Fatal(err)
	}
	db.Close()
	return dbPath
}

// scopedPair writes two complete snapshots of one known scope.
func scopedPair(t *testing.T, first, second []store.CoveDeviceStat) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "scoped.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	scope := store.CoveSnapshotScope{RootPartnerID: 7, Endpoint: r2Endpoint, Complete: true}
	t0 := time.Now().Add(-time.Hour)
	if _, err := db.InsertCoveSnapshotScoped(t.Context(), t0, scope, first); err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertCoveSnapshotScoped(t.Context(), t0.Add(time.Minute), scope, second); err != nil {
		t.Fatal(err)
	}
	db.Close()
	return dbPath
}

func storageRow(storage string) []store.CoveDeviceStat {
	return []store.CoveDeviceStat{{AccountID: 1, DeviceName: "dev-1", Customer: "Acme",
		SettingsJSON: store.MarshalSettings(map[string]string{"I1": "dev-1", "I14": storage})}}
}

type r2GrowthView struct {
	Compared          *bool  `json:"compared"`
	TotalDeltaBytes   *int64 `json:"total_delta_bytes"`
	Items             []any  `json:"items"`
	UnmeasuredDevices int    `json:"unmeasured_devices"`
	Note              string `json:"note"`
}

func decodeR2(t *testing.T, out string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(out), v); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
}

// Round 2 P1: two legacy snapshots (endpoint unknown) were compared, so a
// delta across two different endpoints could be reported.
func TestUnknownEndpointSnapshotsAreNotCompared(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	dbPath := legacyPair(t, storageRow("100"), storageRow("200"))
	out, err := runCoveCmd(t, "storage", "growth", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("storage growth: %v\n%s", err, out)
	}
	if strings.Contains(strings.ReplaceAll(out, " ", ""), `"total_delta_bytes":100`) {
		t.Fatalf("snapshots of an unknown endpoint were compared:\n%s", out)
	}
	var view r2GrowthView
	decodeR2(t, out, &view)
	if view.Compared == nil || *view.Compared || view.TotalDeltaBytes != nil || len(view.Items) != 0 {
		t.Fatalf("an unknown-endpoint pair must report compared=false and no delta:\n%s", out)
	}
	if !strings.Contains(view.Note, "no recorded endpoint") {
		t.Fatalf("the refusal must say why:\n%s", out)
	}
}

func TestDevicesChangesRefusesUnknownEndpointSnapshots(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	dbPath := legacyPair(t, []store.CoveDeviceStat{coveStoreRow(1, "5")}, []store.CoveDeviceStat{coveStoreRow(1, "2")})
	out, err := runCoveCmd(t, "devices", "changes", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("devices changes: %v\n%s", err, out)
	}
	var view struct {
		Compared *bool  `json:"compared"`
		Items    []any  `json:"items"`
		Note     string `json:"note"`
	}
	decodeR2(t, out, &view)
	if view.Compared == nil || *view.Compared || len(view.Items) != 0 || !strings.Contains(view.Note, "no recorded endpoint") {
		t.Fatalf("snapshots of an unknown endpoint were compared:\n%s", out)
	}
}

// A legacy snapshot is never a baseline, even for a scoped latest snapshot of
// the same partner.
func TestALegacySnapshotIsNeverABaseline(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	dbPath := filepath.Join(t.TempDir(), "mixed.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().Add(-time.Hour)
	if _, err := db.InsertCoveSnapshot(t.Context(), t0, 7, storageRow("100")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertCoveSnapshotScoped(t.Context(), t0.Add(time.Minute), store.CoveSnapshotScope{RootPartnerID: 7, Endpoint: r2Endpoint, Complete: true}, storageRow("200")); err != nil {
		t.Fatal(err)
	}
	db.Close()
	out, err := runCoveCmd(t, "storage", "growth", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("storage growth: %v\n%s", err, out)
	}
	var view r2GrowthView
	decodeR2(t, out, &view)
	if view.Compared == nil || *view.Compared || view.TotalDeltaBytes != nil || !strings.Contains(view.Note, "only one qualifying snapshot") {
		t.Fatalf("a legacy snapshot was used as a baseline:\n%s", out)
	}
}

// The comparable case still reports compared=true and the delta.
func TestScopedSnapshotsAreCompared(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	dbPath := scopedPair(t, storageRow("100"), storageRow("250"))
	out, err := runCoveCmd(t, "storage", "growth", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("storage growth: %v\n%s", err, out)
	}
	var view r2GrowthView
	decodeR2(t, out, &view)
	if view.Compared == nil || !*view.Compared || view.TotalDeltaBytes == nil || *view.TotalDeltaBytes != 150 {
		t.Fatalf("comparable snapshots not compared:\n%s", out)
	}
}

// A negative or fractional stored byte count is not a measurement: it never
// becomes a delta.
func TestStorageGrowthLeavesOutUnparseableStorage(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	for _, bad := range []string{"-100", "100.5", "1e400"} {
		dbPath := scopedPair(t, storageRow(bad), storageRow("200"))
		out, err := runCoveCmd(t, "storage", "growth", "--db", dbPath, "--json")
		if err != nil {
			t.Fatalf("storage growth: %v\n%s", err, out)
		}
		var view r2GrowthView
		decodeR2(t, out, &view)
		if view.UnmeasuredDevices != 1 || len(view.Items) != 0 || view.TotalDeltaBytes == nil || *view.TotalDeltaBytes != 0 {
			t.Fatalf("I14=%q was read as a measurement:\n%s", bad, out)
		}
	}
}

// Round 2 P2: a device missing its status in BOTH snapshots was not counted,
// and the command said "no status flips" with no warning.
func TestDevicesChangesCountsStatusMissingInBothSnapshots(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	row := []store.CoveDeviceStat{{AccountID: 1, DeviceName: "dev-1", Customer: "Acme", SettingsJSON: "{}"}}
	dbPath := scopedPair(t, row, row)
	out, err := runCoveCmd(t, "devices", "changes", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("devices changes: %v\n%s", err, out)
	}
	var view struct {
		StatusUnreported int    `json:"status_unreported"`
		Note             string `json:"note"`
	}
	decodeR2(t, out, &view)
	if view.StatusUnreported != 1 {
		t.Fatalf("status_unreported = %d, want 1:\n%s", view.StatusUnreported, out)
	}
	if !strings.Contains(view.Note, "did not report a status") {
		t.Fatalf("missing-status warning absent:\n%s", out)
	}
}

// Round 2 P1 end to end: a fractional status beside a fresh success
// timestamp is unknown, never healthy.
func TestFleetHealthCountsAFractionalStatusAsUnknown(t *testing.T) {
	now := strconv.FormatInt(time.Now().Unix(), 10)
	m := newRPCMock(t, statsRows(map[string]any{"AccountId": 1, "Settings": settingsList(map[string]string{
		"I1": "frac", "I8": "Acme", "D9F00": "4.9999999999999999", "D9F09": now,
	})}))
	fleetTestEnv(t, m.srv)
	out, err := runCoveCmd(t, "fleet", "health", "--json")
	if err != nil {
		t.Fatalf("fleet health: %v\n%s", err, out)
	}
	var h struct {
		Healthy int `json:"healthy"`
		Unknown int `json:"unknown"`
	}
	decodeR2(t, out, &h)
	if h.Healthy != 0 || h.Unknown != 1 {
		t.Fatalf("fractional status: healthy=%d unknown=%d, want 0/1\n%s", h.Healthy, h.Unknown, out)
	}
}

// Unreported used storage is null in the billing export, never 0 bytes, and
// the totals say how many devices they leave out.
func TestBillingUsageNeverReadsUnreportedStorageAsZero(t *testing.T) {
	m := newRPCMock(t, statsRows(
		map[string]any{"AccountId": 1, "Settings": settingsList(map[string]string{"I1": "a", "I14": "1000"})},
		map[string]any{"AccountId": 2, "Settings": settingsList(map[string]string{"I1": "b"})},
		map[string]any{"AccountId": 3, "Settings": settingsList(map[string]string{"I1": "c", "I14": "10.5"})},
	))
	fleetTestEnv(t, m.srv)
	out, err := runCoveCmd(t, "billing", "usage", "--json")
	if err != nil {
		t.Fatalf("billing usage: %v\n%s", err, out)
	}
	var view struct {
		Items []struct {
			DeviceName string   `json:"device_name"`
			Bytes      *int64   `json:"used_storage_bytes"`
			GB         *float64 `json:"used_storage_gb"`
		} `json:"items"`
		TotalBytes        int64  `json:"total_storage_bytes"`
		StorageUnreported int    `json:"storage_unreported"`
		Note              string `json:"note"`
	}
	decodeR2(t, out, &view)
	for _, it := range view.Items {
		switch it.DeviceName {
		case "a":
			if it.Bytes == nil || *it.Bytes != 1000 || it.GB == nil {
				t.Fatalf("reported storage lost:\n%s", out)
			}
		default:
			if it.Bytes != nil || it.GB != nil {
				t.Fatalf("device %s: unreported storage printed as a number:\n%s", it.DeviceName, out)
			}
		}
	}
	if view.TotalBytes != 1000 || view.StorageUnreported != 2 || !strings.Contains(view.Note, "did not report used storage") {
		t.Fatalf("totals must name the unreported devices:\n%s", out)
	}
}

// A device whose AccountId is not an exact integer is never stored under
// account 0 (where several would collapse into one row and be diffed as one
// device); the snapshot is marked incomplete instead.
func TestSnapshotNeverStoresAnUnreadableAccountIDAsZero(t *testing.T) {
	m := newRPCMock(t, statsRows(
		map[string]any{"AccountId": 1, "Settings": settingsList(map[string]string{"I1": "good", "D9F00": "5", "I14": "1"})},
		map[string]any{"AccountId": "x", "Settings": settingsList(map[string]string{"I1": "bad-1", "D9F00": "5", "I14": "1"})},
		map[string]any{"AccountId": 2.5, "Settings": settingsList(map[string]string{"I1": "bad-2", "D9F00": "2", "I14": "1"})},
	))
	fleetTestEnv(t, m.srv)
	dbPath := filepath.Join(t.TempDir(), "ids.db")
	out, err := runCoveCmd(t, "snapshot", "--partner-id", "7", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("snapshot: %v\n%s", err, out)
	}
	var view struct {
		Devices      int    `json:"devices"`
		Complete     bool   `json:"complete"`
		Unidentified int    `json:"unidentified_devices"`
		Note         string `json:"note"`
	}
	decodeR2(t, out, &view)
	if view.Devices != 1 || view.Complete || view.Unidentified != 2 || !strings.Contains(view.Note, "AccountId") {
		t.Fatalf("unreadable account ids must be left out and the snapshot marked incomplete:\n%s", out)
	}
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	latest, _, err := db.LatestCoveSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.CoveDeviceStats(context.Background(), latest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, zero := rows[0]; zero || len(rows) != 1 {
		t.Fatalf("stored rows %v; an unreadable id must never be stored as account 0", fmt.Sprint(rows))
	}
}
