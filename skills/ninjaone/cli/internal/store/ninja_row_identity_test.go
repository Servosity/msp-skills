// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Class report-row-identity: documented report rows keep one stored row each. Rows of a
// report with no documented per-item identity are stored the way sync stores them: stamped
// with their device ordinal (stamped below), so identical rows stay distinct.

func openIdentityStore(t *testing.T) *Store {
	t.Helper()
	db, err := OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func rawRows(t *testing.T, rows ...string) []json.RawMessage {
	t.Helper()
	out := make([]json.RawMessage, len(rows))
	for i, r := range rows {
		out[i] = json.RawMessage(r)
	}
	return out
}

// stamped returns rows as one sync walk stores them.
func stamped(t *testing.T, resourceType string, rows ...string) []json.RawMessage {
	t.Helper()
	return NewReportRowOrdinals(resourceType).Stamp(rawRows(t, rows...))
}

func storedIDsOf(t *testing.T, db *Store, resourceType string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT id FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out = append(out, strings.ReplaceAll(id, "\x00", "|"))
	}
	sort.Strings(out)
	return out
}

func storedDataOf(t *testing.T, db *Store, resourceType string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			t.Fatal(err)
		}
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func TestReportRowsKeepTheirDocumentedIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, resource string
		rows           []string
	}{
		// spec.yaml DiskDriveReport / DeviceDiskDrive: no per-item id. Windows names the
		// first drive \\.\PHYSICALDRIVE0 on every device.
		{"disks-two-devices", "queries", []string{
			`{"deviceId":1,"name":"\\\\.\\PHYSICALDRIVE0","size":512}`,
			`{"deviceId":2,"name":"\\\\.\\PHYSICALDRIVE0","size":256}`,
		}},
		// spec.yaml ProcessorReport / DeviceProcessor: a dual-socket server (Codex r3).
		{"processors-identical", "queries-processors", []string{
			`{"deviceId":7,"name":"Intel Xeon","clockSpeed":2400}`,
			`{"deviceId":7,"name":"Intel Xeon","clockSpeed":2400}`,
		}},
		// spec.yaml SoftwareReport / DeviceApplication: side-by-side installs of one title.
		{"software-product-code", "queries-software", []string{
			`{"deviceId":7,"name":"Acme Agent","version":"1","productCode":"A"}`,
			`{"deviceId":7,"name":"Acme Agent","version":"2","productCode":"B"}`,
		}},
		{"software-location", "queries-software", []string{
			`{"deviceId":7,"name":"Acme Agent","version":"1","location":"C:\\Program Files (x86)\\Acme"}`,
			`{"deviceId":7,"name":"Acme Agent","version":"1","location":"C:\\Program Files\\Acme"}`,
		}},
		{"software-version-only", "queries-software", []string{
			`{"deviceId":7,"name":"Acme Agent","version":"1"}`,
			`{"deviceId":7,"name":"Acme Agent","version":"2"}`,
		}},
		// spec.yaml LoggedOnUsersReport / DeviceLoggedOnUser: one user in two sessions.
		{"logged-on-users-two-sessions", "queries-logged-on-users", []string{
			`{"deviceId":7,"userName":"acme\\jdoe","logonTime":1700000000}`,
			`{"deviceId":7,"userName":"acme\\jdoe","logonTime":1700000500}`,
		}},
		// spec.yaml VolumesReport / DeviceVolume: two unnamed volumes.
		{"volumes-blank-names", "queries-volumes", []string{
			`{"deviceId":7,"name":"","capacity":100}`,
			`{"deviceId":7,"name":"","capacity":200}`,
		}},
		// NodeAttributesDetailed.entityType enum: NODE, END_USER, LOCATION, ORGANIZATION.
		{"custom-fields-detailed-entity-type", "queries-custom-fields-detailed", []string{
			`{"entityId":7,"entityType":"NODE","fields":[]}`,
			`{"entityId":7,"entityType":"ORGANIZATION","fields":[]}`,
		}},
		// spec.yaml NodeAttributesDetailed: deviceId is writeOnly; responses carry entityId.
		{"custom-fields-detailed-entity", "queries-custom-fields-detailed", []string{
			`{"entityId":7,"entityType":"NODE","fields":[]}`,
			`{"entityId":8,"entityType":"NODE","fields":[]}`,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openIdentityStore(t)
			if _, _, err := db.UpsertBatch(tc.resource, stamped(t, tc.resource, tc.rows...)); err != nil {
				t.Fatal(err)
			}
			got := storedDataOf(t, db, tc.resource)
			if len(got) != len(tc.rows) {
				t.Fatalf("%s holds %d rows for %d documented rows: %v", tc.resource, len(got), len(tc.rows), got)
			}
		})
	}
}

// The ordinal is the row's position among its device's rows, counted across the pages of one
// walk; a report with a documented identity is not stamped; every other byte is kept.
func TestReportRowOrdinalsCountPerDeviceAcrossPages(t *testing.T) {
	o := NewReportRowOrdinals("queries-processors")
	page1 := o.Stamp(rawRows(t, `{"deviceId":7,"name":"Intel Xeon"}`, `{"deviceId":8,"name":"AMD EPYC"}`))
	page2 := o.Stamp(rawRows(t, `{"deviceId":7,"name":"Intel Xeon"}`, `not an object`, `{"name":"no device"}`))
	var got []string
	for _, it := range append(page1, page2[0]) {
		obj, err := DecodeJSONObject(it)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, strings.ReplaceAll(StorageIDFor("queries-processors", obj), "\x00", "|"))
	}
	if strings.Join(got, ",") != "7|#0,8|#0,7|#1" {
		t.Fatalf("storage ids %v, want [7|#0 8|#0 7|#1]", got)
	}
	if string(page2[1]) != `not an object` {
		t.Fatalf("a non-object row was rewritten: %s", page2[1])
	}
	if NewReportRowOrdinals("queries-os-patches") != nil {
		t.Fatal("queries-os-patches documents DeviceOSPatch.id; it must not be stamped")
	}
	raw := `{"deviceId":7,"name":"AT&T <Agent>","size":12345678901234567890}`
	out := NewReportRowOrdinals("queries-software").Stamp(rawRows(t, raw))
	if !strings.Contains(string(out[0]), `"name":"AT&T <Agent>"`) || !strings.Contains(string(out[0]), `"size":12345678901234567890`) || !strings.Contains(string(out[0]), `"`+ReportRowOrdinalField+`":0`) {
		t.Fatalf("stamping changed the row: %s", out[0])
	}
}

// A stored ordinal row is replaced whole: the row now at a position never inherits a field of
// the row that held the position before (the generated merge keeps the richer blob).
func TestOrdinalReportRowIsReplacedWhole(t *testing.T) {
	db := openIdentityStore(t)
	if _, _, err := db.UpsertBatch("queries-software", stamped(t, "queries-software",
		`{"deviceId":7,"name":"Acme Agent","version":"1","productCode":"A","installDate":"2024-01-01T00:00:00Z"}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("queries-software", stamped(t, "queries-software",
		`{"deviceId":7,"name":"Zeta Tool","version":"5"}`)); err != nil {
		t.Fatal(err)
	}
	got := storedDataOf(t, db, "queries-software")
	if len(got) != 1 || strings.Contains(got[0], "productCode") || strings.Contains(got[0], "installDate") || !strings.Contains(got[0], "Zeta Tool") {
		t.Fatalf("device 7's first row is %v, want Zeta Tool alone", got)
	}
}

func TestUpgradedStoreDropsRowsStoredUnderTheShippedKey(t *testing.T) {
	db := openIdentityStore(t)
	// Rows exactly as the shipped release stored them: deviceId + display name, and the bare
	// drive name for /v2/queries/disks.
	for _, seed := range []struct{ resource, id, data string }{
		{"queries-software", "7\x00Acme Agent", `{"deviceId":7,"name":"Acme Agent","version":"1","productCode":"A"}`},
		{"queries-software", "8\x00Acme Agent", `{"deviceId":8,"name":"Acme Agent","version":"1"}`},
		{"queries-processors", "7\x00Intel Xeon", `{"deviceId":7,"name":"Intel Xeon"}`},
		{"queries", "Disk 0", `{"deviceId":1,"name":"Disk 0","size":512}`},
	} {
		if err := db.Upsert(seed.resource, seed.id, json.RawMessage(seed.data)); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := db.UpsertBatch("queries-software", stamped(t, "queries-software",
		`{"deviceId":7,"name":"Acme Agent","version":"1","productCode":"A"}`,
		`{"deviceId":7,"name":"Acme Agent","version":"2","productCode":"B"}`,
		`{"deviceId":8,"name":"Acme Agent","version":"2","productCode":"A"}`,
	)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("queries-processors", stamped(t, "queries-processors",
		`{"deviceId":7,"name":"Intel Xeon"}`,
		`{"deviceId":7,"name":"Intel Xeon"}`,
	)); err != nil {
		t.Fatal(err)
	}
	// Device 3's second disk is named "#0": its shipped key 3\x00#0 is the current key of
	// device 3's first disk, which must never be taken for a legacy row.
	if _, _, err := db.UpsertBatch("queries", stamped(t, "queries",
		`{"deviceId":1,"name":"Disk 0","size":512}`,
		`{"deviceId":2,"name":"Disk 0","size":256}`,
		`{"deviceId":3,"name":"Disk A","size":128}`,
		`{"deviceId":3,"name":"#0","size":64}`,
	)); err != nil {
		t.Fatal(err)
	}
	for resource, want := range map[string]string{
		"queries-software":   "7|#0,7|#1,8|#0",
		"queries-processors": "7|#0,7|#1",
		"queries":            "1|#0,2|#0,3|#0,3|#1",
	} {
		if got := strings.Join(storedIDsOf(t, db, resource), ","); got != want {
			t.Fatalf("%s holds %s, want %s (every shipped-key row dropped, no current row touched)", resource, got, want)
		}
	}
	if n, err := db.Count("queries"); err != nil || n != 4 {
		t.Fatalf("Count(queries) = %d, %v", n, err)
	}
	var fts int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM resources_fts WHERE resource_type = 'queries-software'`).Scan(&fts); err != nil {
		t.Fatal(err)
	}
	if fts != 3 {
		t.Fatalf("search index holds %d software rows, want 3", fts)
	}
}

func TestReconcileSnapshotKeepsExactlyTheWalkedRows(t *testing.T) {
	db := openIdentityStore(t)
	rows := stamped(t, "queries-software",
		`{"deviceId":7,"name":"Acme Agent","version":"1"}`,
		`{"deviceId":7,"name":"Acme Agent","version":"2"}`,
		`{"deviceId":9,"name":"Other","version":"5"}`,
	)
	if _, _, err := db.UpsertBatch("queries-software", rows); err != nil {
		t.Fatal(err)
	}
	var keep []string
	for _, r := range rows[1:] {
		obj, err := DecodeJSONObject(r)
		if err != nil {
			t.Fatal(err)
		}
		keep = append(keep, StorageIDFor("queries-software", obj))
	}
	n, err := db.ReconcileSnapshot("queries-software", keep, "")
	if err != nil || n != 1 {
		t.Fatalf("ReconcileSnapshot removed %d (%v), want 1 (device 7's first row, same device as a kept row)", n, err)
	}
	if got := storedDataOf(t, db, "queries-software"); len(got) != 2 {
		t.Fatalf("after reconcile: %v", got)
	}
}
