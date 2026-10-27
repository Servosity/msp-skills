// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"ninjaone-pp-cli/internal/store"
)

// Class backup-coverage-reads-documented-usage: /v2/queries/backup/usage
// (spec.yaml DeviceUsageReport) returns NodeWithDetailedReferences rows whose
// usage sits in references.backupUsage (BackupUsage). Coverage is proven only
// by a documented size above zero or a successful backup job; a row that
// reports zero, or no usage at all, is a gap.
func TestBackupCoverageReadsTheDocumentedUsage(t *testing.T) {
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var devs []json.RawMessage
	for _, id := range []string{"7", "8", "9", "10", "11", "12"} {
		devs = append(devs, json.RawMessage(`{"id":`+id+`,"systemName":"PC-`+id+`","organizationId":1,"nodeClass":"WINDOWS_WORKSTATION"}`))
	}
	if _, _, err := db.UpsertBatch(rtDevices, devs); err != nil {
		t.Fatal(err)
	}
	zero := `"revisionsCurrentSize":0,"revisionsPreviousSize":0,"revisionsDeletedSize":0,"localFileFolderSize":0,"localImageSize":0,"localImageV2Size":0,"cloudFileFolderSize":0,"cloudImageSize":0,"cloudImageV2Size":0`
	usage := []json.RawMessage{
		json.RawMessage(`{"id":7,"references":{"backupUsage":{` + zero + `}}}`),
		json.RawMessage(`{"id":8,"references":{"backupUsage":{"cloudFileFolderSize":1024,"localImageSize":0}}}`),
		json.RawMessage(`{"id":9,"systemName":"PC-9"}`),
		json.RawMessage(`{"id":11,"references":{"backupUsage":{` + zero + `,"lastSuccessfulBackupJob":1767225600.5}}}`),
		json.RawMessage(`{"id":12,"usedBytes":5}`),
	}
	if _, _, err := db.UpsertBatch(rtBackupUsage, usage); err != nil {
		t.Fatal(err)
	}
	devices, err := buildDeviceIndex(db)
	if err != nil {
		t.Fatal(err)
	}
	gaps, total, err := computeBackupCoverage(db, devices, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if total != 6 {
		t.Fatalf("total = %d, want 6", total)
	}
	got := map[string]string{}
	for _, g := range gaps {
		got[g.DeviceID] = g.Reason
	}
	want := map[string]string{
		"7":  "backup usage is zero",
		"9":  "backup usage not reported",
		"10": "no backup-usage record",
	}
	if len(got) != len(want) {
		t.Fatalf("gaps = %v, want %v", got, want)
	}
	for id, reason := range want {
		if got[id] != reason {
			t.Fatalf("device %s: reason %q, want %q (all gaps: %v)", id, got[id], reason, got)
		}
	}
}
