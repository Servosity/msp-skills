// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written: partition-authority class (4.35.1 reprint review, round 2).
// Rows an older print stored under a partition this build no longer writes
// the family to must follow the CURRENT partition's sync semantics: pruned by
// an unfiltered full sync that no longer returns them, and kept by a filtered
// one (which the press's own reconcile refuses to prune). Both tests drive the
// real `sync` command against a fake Auvik and read the store the way every
// novel does.

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"auvik-pp-cli/internal/store"
)

// fakeDeviceList serves /v1/inventory/device/info (auvik-v1
// #/paths/~1v1~1inventory~1device~1info/get), honoring the documented
// filter[deviceType] query parameter.
func fakeDeviceList(t *testing.T, devices map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if r.URL.Path != "/v1/inventory/device/info" {
			http.NotFound(w, r)
			return
		}
		want := r.URL.Query().Get("filter[deviceType]")
		data := []any{}
		for id, typ := range devices {
			if want != "" && typ != want {
				continue
			}
			data = append(data, map[string]any{"type": "device", "id": id,
				"attributes": map[string]any{"deviceName": id, "deviceType": typ}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func readDeviceIDs(t *testing.T, dbPath string) string {
	t.Helper()
	db, err := store.OpenReadOnlyContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	defer db.Close()
	got, err := loadResources(context.Background(), db, rtDevices)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(rowIDs(got), ",")
}

// Codex r2 input: legacy `inventory` rows d1 (switch) and d2 (router), then a
// FILTERED full sync returning only d1. d2 was not asked about, so it is still
// present: the device read must keep it.
func TestFilteredFullSyncKeepsLegacyRowsItDidNotAskAbout(t *testing.T) {
	srv := fakeDeviceList(t, map[string]string{"d1": "switch", "d2": "router"})
	isolateCLIEnv(t, srv.URL)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	seedRawRows(t, dbPath, []rawSeed{
		{"inventory", "d1", `{"id":"d1","type":"device","attributes":{"deviceName":"sw","deviceType":"switch"}}`},
		{"inventory", "d2", `{"id":"d2","type":"device","attributes":{"deviceName":"rtr","deviceType":"router"}}`},
	})

	runCLI(t, "sync", "--resources", "inventory-device-info", "--full",
		"--param", "filter[deviceType]=switch", "--db", dbPath, "--agent", "--no-cache")

	if ids := readDeviceIDs(t, dbPath); ids != "d1,d2" {
		t.Fatalf("device read after a filtered full sync = %q, want d1,d2 (d2 was outside the filter, not removed)", ids)
	}
}

// Codex r1 #4 input: legacy `inventory/d1`, then an UNFILTERED full sync of the
// current partition answering data: [] (auvik-v1 deviceInfoReadMultiple). The
// device is gone; it must not be resurrected from the legacy partition. A
// component in `inventory` (what 4.35.1 writes there) is not touched.
func TestUnfilteredFullSyncRetiresLegacyRowsItNoLongerReturns(t *testing.T) {
	srv := fakeDeviceList(t, map[string]string{})
	isolateCLIEnv(t, srv.URL)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	seedRawRows(t, dbPath, []rawSeed{
		{"inventory", "d1", `{"id":"d1","type":"device","attributes":{"deviceName":"old"}}`},
		{"inventory", "c1", `{"id":"c1","type":"component","attributes":{"componentName":"psu"}}`},
	})

	runCLI(t, "sync", "--resources", "inventory-device-info", "--full", "--db", dbPath, "--agent", "--no-cache")

	if ids := readDeviceIDs(t, dbPath); ids != "" {
		t.Fatalf("device read after a full sync returning no devices = %q, want none", ids)
	}
	db, err := store.OpenReadOnlyContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'inventory' AND id = 'c1'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("component c1 in inventory: count=%d err=%v, want it kept", n, err)
	}
}

// The move is typed and keeps the current copy: a legacy row whose id the
// current partition already holds never overwrites it, and a row of another
// family stays where it is.
func TestLegacyPartitionMoveKeepsTheCurrentCopy(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	seedRawRows(t, dbPath, []rawSeed{
		{"inventory-device-info", "d1", `{"id":"d1","type":"device","attributes":{"deviceName":"current"}}`},
		{"inventory", "d1", `{"id":"d1","type":"device","attributes":{"deviceName":"stale"}}`},
		{"asm", "app1", `{"id":"app1","type":"asmApp","attributes":{"name":"Figma"}}`},
		{"asm", "cl1", `{"id":"cl1","type":"asmClient","attributes":{"name":"acme"}}`},
	})
	db, err := store.OpenWithContext(context.Background(), dbPath) // a writable open runs the move
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	check := func(partition, id, want string) {
		t.Helper()
		var name string
		err := db.DB().QueryRow(`SELECT COALESCE(json_extract(data, '$.attributes.deviceName'), json_extract(data, '$.attributes.name'), '')
			FROM resources WHERE resource_type = ? AND id = ?`, partition, id).Scan(&name)
		if want == "" {
			if err == nil {
				t.Errorf("%s/%s still present (%q), want it moved", partition, id, name)
			}
			return
		}
		if err != nil || name != want {
			t.Errorf("%s/%s = %q (err %v), want %q", partition, id, name, err, want)
		}
	}
	check("inventory-device-info", "d1", "current")
	check("inventory", "d1", "")
	check("asm-app-info", "app1", "Figma")
	check("asm", "app1", "")
	check("asm", "cl1", "acme")
}

// Every non-current partition a family reads is either moved into the current
// partition at store open or one this build still syncs; anything else would
// be read forever and never pruned.
func TestEveryLegacyReadPartitionIsMovedOrStillSynced(t *testing.T) {
	for name, f := range auvikFamilies {
		for _, p := range f.partitions[1:] {
			moved := false
			for _, m := range auvikLegacyMoves {
				if m.From == p && m.To == f.current() {
					moved = true
				}
			}
			if !moved && resourceReconcileMode(p) == "" {
				t.Errorf("family %s reads %q, which is neither moved to %q nor synced by this build", name, p, f.current())
			}
		}
	}
}
