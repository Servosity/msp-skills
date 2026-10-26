// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"ninjaone-pp-cli/internal/cliutil/testenv"
	"ninjaone-pp-cli/internal/store"
)

const gib = float64(bytesPerGiB)

func TestComputeLowDisk(t *testing.T) {
	vols := []nvVolume{
		{DeviceID: "1", DeviceName: "pc1", OrgID: "a", Volume: "C:", Capacity: 100 * gib, Free: 5 * gib, HasSizes: true},
		{DeviceID: "1", DeviceName: "pc1", OrgID: "a", Volume: "D:", Capacity: 1000 * gib, Free: 150 * gib, HasSizes: true},
		{DeviceID: "2", DeviceName: "pc2", OrgID: "b", Volume: "C:", Capacity: 100 * gib, Free: 2 * gib, HasSizes: true},
		{DeviceID: "3", DeviceName: "pc3", OrgID: "b", Volume: "E:"},
	}
	low, skipped := computeLowDisk(vols, 10, 0, "")
	if len(low) != 2 || low[0].DeviceName != "pc2" || low[1].DeviceName != "pc1" || low[1].Volume != "C:" {
		t.Fatalf("want pc2 C: then pc1 C: (worst first), got %+v", low)
	}
	if skipped != 1 {
		t.Fatalf("skipped = %d, want 1 (no sizes)", skipped)
	}
	if low[0].FreePct != 2 || low[0].FreeGB != 2 {
		t.Fatalf("pc2 C: free = %v%% %v GiB, want 2%% 2 GiB", low[0].FreePct, low[0].FreeGB)
	}
	// A GiB floor catches a big volume with a healthy percent but little room left.
	low, _ = computeLowDisk(vols, 10, 200, "a")
	if len(low) != 2 {
		t.Fatalf("with a 200 GiB floor org a has both volumes low, got %+v", low)
	}
	if low, _ = computeLowDisk(nil, 10, 0, ""); low == nil {
		t.Fatal("an empty result must marshal as [], not null")
	}
}

func TestComputeBitlockerGaps(t *testing.T) {
	prot := map[string]any{"protectionStatus": "PROTECTED", "conversionStatus": "FULLY_ENCRYPTED"}
	unprot := map[string]any{"protectionStatus": "UNPROTECTED", "conversionStatus": "FULLY_DECRYPTED"}
	vols := []nvVolume{
		{DeviceID: "1", DeviceName: "win", OrgID: "a", Org: "Acme", NodeClass: "WINDOWS_WORKSTATION", Volume: "C:", Capacity: 1, BitLocker: prot},
		{DeviceID: "1", DeviceName: "win", OrgID: "a", Org: "Acme", NodeClass: "WINDOWS_WORKSTATION", Volume: "D:", Capacity: 1, BitLocker: unprot},
		{DeviceID: "2", DeviceName: "srv", OrgID: "a", Org: "Acme", NodeClass: "WINDOWS_SERVER", Volume: "C:", Capacity: 1},
		{DeviceID: "3", DeviceName: "mac", OrgID: "b", Org: "Beta", NodeClass: "MAC", Volume: "/", Capacity: 1},
		{DeviceID: "4", DeviceName: "4", OrgID: "", Org: "(unassigned)", Volume: "C:", Capacity: 1},
		{DeviceID: "1", DeviceName: "win", OrgID: "a", Org: "Acme", NodeClass: "WINDOWS_WORKSTATION", Volume: "E:"},
	}
	gaps, skipped := computeBitlockerGaps(vols, false, "")
	if skipped != 1 {
		t.Fatalf("skipped = %d, want 1 (the Mac)", skipped)
	}
	want := map[string]string{"win D:": "UNPROTECTED", "srv C:": "NOT_REPORTED", "4 C:": "NOT_REPORTED"}
	if len(gaps) != len(want) {
		t.Fatalf("gaps = %+v, want %v", gaps, want)
	}
	for _, g := range gaps {
		if want[g.DeviceName+" "+g.Volume] != g.Protection {
			t.Fatalf("unexpected gap %+v", g)
		}
	}
	if gaps, _ = computeBitlockerGaps(vols, true, "b"); len(gaps) != 1 || gaps[0].DeviceName != "mac" {
		t.Fatalf("--all-os --org b must list the Mac volume, got %+v", gaps)
	}
}

// The commands read what sync wrote: three volumes of one device stay three rows
// (queries-composite-key-extraction), joined to the device and organization.
func TestVolumeNovelsReadTheSyncedStore(t *testing.T) {
	testenv.Isolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	seed := func(resource string, rows ...string) {
		items := make([]json.RawMessage, len(rows))
		for i, r := range rows {
			items[i] = json.RawMessage(r)
		}
		if _, _, err := db.UpsertBatch(resource, items); err != nil {
			t.Fatalf("seed %s: %v", resource, err)
		}
	}
	seed(rtOrgs, `{"id":2,"name":"Acme"}`)
	seed(rtDevices, `{"id":7,"systemName":"FRONTDESK","organizationId":2,"nodeClass":"WINDOWS_WORKSTATION"}`)
	seed(rtVolumes,
		`{"deviceId":7,"name":"C:","driveLetter":"C:","capacity":107374182400,"freeSpace":3221225472,"bitLockerStatus":{"protectionStatus":"PROTECTED"}}`,
		`{"deviceId":7,"name":"D:","driveLetter":"D:","capacity":107374182400,"freeSpace":53687091200,"bitLockerStatus":{"protectionStatus":"UNPROTECTED","conversionStatus":"FULLY_DECRYPTED"}}`,
		`{"deviceId":7,"name":"E:","driveLetter":"E:","capacity":0,"freeSpace":0}`)
	db.Close()

	run := func(args ...string) []map[string]any {
		t.Helper()
		root := newRootCmd(&rootFlags{})
		var out, errBuf bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errBuf)
		root.SetArgs(append(args, "--db", dbPath, "--json"))
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, errBuf.String())
		}
		var rows []map[string]any
		if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
			var env struct {
				Results []map[string]any `json:"results"`
			}
			if err2 := json.Unmarshal(out.Bytes(), &env); err2 != nil {
				t.Fatalf("%v output is not JSON rows: %v\n%s", args, err, out.String())
			}
			rows = env.Results
		}
		return rows
	}
	low := run("low-disk")
	if len(low) != 1 || low[0]["volume"] != "C:" || low[0]["deviceName"] != "FRONTDESK" || low[0]["org"] != "Acme" {
		t.Fatalf("low-disk = %v, want only FRONTDESK C: in Acme", low)
	}
	gaps := run("bitlocker-gaps")
	if len(gaps) != 1 || gaps[0]["volume"] != "D:" || gaps[0]["protectionStatus"] != "UNPROTECTED" {
		t.Fatalf("bitlocker-gaps = %v, want only FRONTDESK D: UNPROTECTED", gaps)
	}

	root := newRootCmd(&rootFlags{})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"low-disk", "--min-free-pct", "-1", "--db", dbPath})
	if err := root.Execute(); err == nil {
		t.Fatal("a negative --min-free-pct must be refused")
	}
}
