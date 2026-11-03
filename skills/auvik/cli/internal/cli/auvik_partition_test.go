// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written: the novel commands' store reads must survive a reprint that
// renames sync partitions. Press 4.35.1 moved Auvik's partitions under the
// names the 4.30 print used: "inventory" now holds COMPONENTS (it held devices),
// "asm" holds ASM CLIENTS (it held apps), and "auvik-inventory" holds v2
// DISCOVERY STATUS (it held v2 devices). These tests seed a store in that shape,
// plus rows an older print left behind, and read it the way the novels do.

package cli

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"auvik-pp-cli/internal/store"
)

type seedRow struct{ partition, id, typ, syncedAt string }

func seedPartitionStore(t *testing.T, rows []seedRow) *store.Store {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, r := range rows {
		data := `{"id":"` + r.id + `","type":"` + r.typ + `","attributes":{"deviceName":"` + r.id + `"}}`
		if _, err := db.DB().Exec(`INSERT INTO resources (id, resource_type, data, synced_at) VALUES (?, ?, ?, ?)`,
			r.id, r.partition, data, r.syncedAt); err != nil {
			t.Fatalf("seed %v: %v", r, err)
		}
	}
	return db
}

func rowIDs(rows []auvikRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	sort.Strings(out)
	return out
}

// A 4.35.1 sync writes components to "inventory" and devices to
// "inventory-device-info". Reading devices must never return a component, and
// must still find the devices an older print stored under "inventory".
func TestDeviceReadsNeverReturnComponents(t *testing.T) {
	db := seedPartitionStore(t, []seedRow{
		{"inventory", "c1", "component", "2026-11-01 10:00:00"},
		{"inventory-device-info", "d1", "device", "2026-11-01 10:00:00"},
		{"inventory", "d0", "device", "2026-10-01 10:00:00"},
	})
	got, err := loadResources(context.Background(), db, rtDevices)
	if err != nil {
		t.Fatal(err)
	}
	if ids := strings.Join(rowIDs(got), ","); ids != "d0,d1" {
		t.Fatalf("device read returned %q, want d0,d1 (a component row is not a device)", ids)
	}
}

// v2 discovery status lives in "auvik-inventory" under 4.35.1; the 4.30 name
// was "auvik-inventory-discovery-status". Both must be read, and the v2 DEVICE
// rows an older print stored under "auvik-inventory" must not be.
func TestDiscoveryStatusReadsTheCurrentPartition(t *testing.T) {
	db := seedPartitionStore(t, []seedRow{
		{"auvik-inventory", "s1", "deviceDiscoveryStatus", "2026-11-01 10:00:00"},
		{"auvik-inventory", "v2dev", "device", "2026-10-01 10:00:00"},
		{"auvik-inventory-discovery-status", "s0", "deviceDiscoveryStatus", "2026-10-01 10:00:00"},
	})
	got, err := loadResources(context.Background(), db, rtDiscoveryStatus)
	if err != nil {
		t.Fatal(err)
	}
	if ids := strings.Join(rowIDs(got), ","); ids != "s0,s1" {
		t.Fatalf("discovery-status read returned %q, want s0,s1", ids)
	}
}

// ASM apps and ASM clients swapped partitions between prints.
func TestASMReadsSeparateAppsFromClients(t *testing.T) {
	db := seedPartitionStore(t, []seedRow{
		{"asm", "client1", "asmClient", "2026-11-01 10:00:00"},
		{"asm-app-info", "app1", "asmApp", "2026-11-01 10:00:00"},
		{"asm", "app0", "asmApp", "2026-10-01 10:00:00"},
		{"asm-client-info", "client0", "asmClient", "2026-10-01 10:00:00"},
	})
	apps, err := loadResources(context.Background(), db, rtASMApps)
	if err != nil {
		t.Fatal(err)
	}
	clients, err := loadResources(context.Background(), db, rtASMClients)
	if err != nil {
		t.Fatal(err)
	}
	if ids := strings.Join(rowIDs(apps), ","); ids != "app0,app1" {
		t.Fatalf("ASM app read returned %q, want app0,app1", ids)
	}
	if ids := strings.Join(rowIDs(clients), ","); ids != "client0,client1" {
		t.Fatalf("ASM client read returned %q, want client0,client1", ids)
	}
}

// One record stored under both an old and a current partition name is one
// record, and the newer copy wins.
func TestPartitionReadsDeduplicateByIDNewestWins(t *testing.T) {
	db := seedPartitionStore(t, []seedRow{
		{"inventory", "d1", "device", "2026-10-01 10:00:00"},
		{"inventory-device-info", "d1", "device", "2026-11-01 10:00:00"},
	})
	if _, err := db.DB().Exec(`UPDATE resources SET data = json_set(data, '$.attributes.deviceName', 'new') WHERE resource_type = 'inventory-device-info'`); err != nil {
		t.Fatal(err)
	}
	got, err := loadResources(context.Background(), db, rtDevices)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].attrString("deviceName") != "new" {
		t.Fatalf("want one d1 row carrying the newer copy, got %d rows %+v", len(got), got)
	}
}

// Every resource name a novel command tells the operator to sync, or reads as
// its freshness signal, must be a partition the current syncer knows. Under
// 4.35.1, `sync --resources inventory` syncs components and
// `auvik-inventory-discovery-status` is an unknown resource.
func TestNovelSyncInstructionsNameRealPartitions(t *testing.T) {
	// Known to the syncer is not enough: "inventory" is a real partition that
	// now holds components. An instruction must name a family's CURRENT one.
	known := map[string]bool{}
	for _, f := range auvikFamilies {
		known[f.current()] = true
	}
	files := []string{"auvik_local.go", "eol.go", "changes.go", "configuration_audit.go", "inventory_diff.go",
		"usage_reconcile.go", "device_discovery_gaps.go", "alert_noise.go", "asm_shadow.go"}
	listRe := regexp.MustCompile(`--resources[ =]([a-z0-9,-]+)`)
	hintRe := regexp.MustCompile(`hint(?:IfUnsynced|IfStale)\(cmd, db, "([a-z0-9-]+)"`)
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range listRe.FindAllStringSubmatch(string(src), -1) {
			for _, name := range strings.Split(m[1], ",") {
				if !known[name] {
					t.Errorf("%s tells the operator to sync %q, which the syncer does not know", f, name)
				}
			}
		}
		for _, m := range hintRe.FindAllStringSubmatch(string(src), -1) {
			if !known[m[1]] {
				t.Errorf("%s checks freshness of %q, which the syncer does not know", f, m[1])
			}
		}
	}
}

// The current partition of every family must sync from the endpoint that
// family's records come from. A reprint that renames partitions again fails
// here instead of silently feeding a report another family's rows.
func TestAuvikFamiliesPinTheirEndpoint(t *testing.T) {
	known := map[string]bool{}
	for _, n := range knownSyncResourceNames() {
		known[n] = true
	}
	for name, f := range auvikFamilies {
		if !known[f.current()] {
			t.Errorf("family %s: current partition %q is unknown to the syncer", name, f.current())
			continue
		}
		got, err := syncResourcePath(f.current())
		if err != nil || got != f.path {
			t.Errorf("family %s: partition %q syncs from %q (err %v), want %q", name, f.current(), got, err, f.path)
		}
	}
}
