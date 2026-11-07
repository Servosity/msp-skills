// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. `fleet stale` behavior, plus the seeded-mirror helpers the
// triage and executables find end-to-end tests share.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/store"
)

// fleetStaleTestSeedDB writes resource rows (keyed by their "uuid") into a
// fresh SQLite mirror and returns its path.
func fleetStaleTestSeedDB(t *testing.T, rows map[string][]map[string]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	for resource, items := range rows {
		for _, item := range items {
			raw, err := json.Marshal(item)
			if err != nil {
				t.Fatal(err)
			}
			id, _ := item["uuid"].(string)
			if err := db.Upsert(resource, id, raw); err != nil {
				t.Fatalf("seed %s/%s: %v", resource, id, err)
			}
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// fleetStaleTestRun executes the real root command and returns stdout.
func fleetStaleTestRun(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func fleetStaleTestAgo(d time.Duration) string {
	return time.Now().Add(-d).UTC().Format(time.RFC3339)
}

const fleetStaleTestDay = 24 * time.Hour

func fleetStaleTestGroups() []map[string]any {
	return []map[string]any{
		{"uuid": "11111111-0000-4000-8000-000000000001", "displayName": "Servosity MSP", "linkedEntityType": "DEVICE_GROUP_ENTITY_TYPE_MSP"},
		{"uuid": "22222222-0000-4000-8000-000000000002", "displayName": "Acme Dental", "parentGroupUuid": "11111111-0000-4000-8000-000000000001", "linkedEntityType": esetCompanyEntityType},
		{"uuid": "33333333-0000-4000-8000-000000000003", "displayName": "HQ", "parentGroupUuid": "22222222-0000-4000-8000-000000000002", "linkedEntityType": "DEVICE_GROUP_ENTITY_TYPE_SITE"},
		{"uuid": "44444444-0000-4000-8000-000000000004", "displayName": "Bright Smiles Ortho", "parentGroupUuid": "11111111-0000-4000-8000-000000000001", "linkedEntityType": esetCompanyEntityType},
	}
}

const (
	fleetStaleTestAcmeHQ = "33333333-0000-4000-8000-000000000003"
	fleetStaleTestAcme   = "22222222-0000-4000-8000-000000000002"
	fleetStaleTestBright = "44444444-0000-4000-8000-000000000004"
)

func fleetStaleTestDevices() []map[string]any {
	return []map[string]any{
		{"uuid": "d0000001-aaaa-4bbb-8ccc-000000000001", "displayName": "FRONTDESK-01", "systemHostname": "frontdesk-01.acme.local", "parentGroupUuid": fleetStaleTestAcmeHQ,
			"functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_ATTENTION_REQUIRED", "lastSyncTime": fleetStaleTestAgo(40 * fleetStaleTestDay), "operatingSystem": map[string]any{"displayName": "Windows 11 Pro"}},
		{"uuid": "d0000002-aaaa-4bbb-8ccc-000000000002", "displayName": "RECEPTION-02", "parentGroupUuid": fleetStaleTestAcmeHQ,
			"functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_OK", "lastSyncTime": fleetStaleTestAgo(2 * time.Hour)},
		{"uuid": "d0000003-aaaa-4bbb-8ccc-000000000003", "displayName": "LAB-PC", "parentGroupUuid": fleetStaleTestAcme,
			"functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_ATTENTION_RECOMMENDED"},
		{"uuid": "d0000004-aaaa-4bbb-8ccc-000000000004", "displayName": "XRAY-SERVER", "parentGroupUuid": fleetStaleTestAcme, "isMuted": true,
			"functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_OK", "lastSyncTime": fleetStaleTestAgo(35 * fleetStaleTestDay)},
		{"uuid": "d0000005-aaaa-4bbb-8ccc-000000000005", "displayName": "BRIGHT-NAS", "parentGroupUuid": fleetStaleTestBright,
			"functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_OK", "lastSyncTime": fleetStaleTestAgo(20 * fleetStaleTestDay)},
		{"uuid": "d0000006-aaaa-4bbb-8ccc-000000000006", "displayName": "KIOSK-03", "parentGroupUuid": fleetStaleTestBright,
			"functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_OK", "lastSyncTime": "not-a-time"},
	}
}

func fleetStaleTestDecode(t *testing.T, out string) fleetStaleView {
	t.Helper()
	var v fleetStaleView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("output is not fleet stale JSON: %v\n%s", err, out)
	}
	return v
}

func fleetStaleTestNames(v fleetStaleView) string {
	names := make([]string, 0, len(v.Devices))
	for _, d := range v.Devices {
		names = append(names, d.Device)
	}
	return strings.Join(names, ",")
}

func TestFleetStaleEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	path := fleetStaleTestSeedDB(t, map[string][]map[string]any{
		"device-groups": fleetStaleTestGroups(),
		"devices":       fleetStaleTestDevices(),
	})

	out, err := fleetStaleTestRun(t, "fleet", "stale", "--db", path, "--json")
	if err != nil {
		t.Fatalf("fleet stale: %v\n%s", err, out)
	}
	v := fleetStaleTestDecode(t, out)
	// Never-seen first (by customer, device), then oldest check-in first.
	if got, want := fleetStaleTestNames(v), "LAB-PC,KIOSK-03,FRONTDESK-01,BRIGHT-NAS"; got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
	if v.Total != 4 || v.MutedExcluded != 1 || v.ThresholdDays != 14 || v.Customer != "" {
		t.Fatalf("totals = %+v", v)
	}
	front := v.Devices[2]
	if front.DaysSince != 40 || front.LastSeen == "" || front.Customer != "Acme Dental" ||
		front.GroupPath != "Servosity MSP / Acme Dental / HQ" || front.FunctionalityStatus != "ATTENTION_REQUIRED" ||
		front.OS != "Windows 11 Pro" || front.Hostname != "frontdesk-01.acme.local" || front.DeviceUUID != "d0000001-aaaa-4bbb-8ccc-000000000001" {
		t.Fatalf("FRONTDESK-01 row = %+v", front)
	}
	for _, never := range v.Devices[:2] {
		if never.DaysSince != -1 || never.LastSeen != "" {
			t.Fatalf("never-seen row should have days_since -1 and empty last_seen: %+v", never)
		}
	}
	if v.Devices[1].Customer != "Bright Smiles Ortho" {
		t.Fatalf("KIOSK-03 customer = %q", v.Devices[1].Customer)
	}

	cases := []struct {
		name      string
		args      []string
		want      string
		total     int
		muted     int
		customer  string
		threshold int
	}{
		{"customer filter", []string{"--customer", "acme"}, "LAB-PC,FRONTDESK-01", 2, 1, "Acme Dental", 14},
		{"include muted", []string{"--include-muted"}, "LAB-PC,KIOSK-03,FRONTDESK-01,XRAY-SERVER,BRIGHT-NAS", 5, 0, "", 14},
		{"longer threshold drops 20-day device", []string{"--days", "30"}, "LAB-PC,KIOSK-03,FRONTDESK-01", 3, 1, "", 30},
		{"limit keeps total", []string{"--limit", "1"}, "LAB-PC", 4, 1, "", 14},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"fleet", "stale", "--db", path, "--json"}, tc.args...)
			out, err := fleetStaleTestRun(t, args...)
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			v := fleetStaleTestDecode(t, out)
			if got := fleetStaleTestNames(v); got != tc.want {
				t.Fatalf("devices = %s, want %s", got, tc.want)
			}
			if v.Total != tc.total || v.MutedExcluded != tc.muted || v.Customer != tc.customer || v.ThresholdDays != tc.threshold {
				t.Fatalf("view = total %d muted %d customer %q threshold %d", v.Total, v.MutedExcluded, v.Customer, v.ThresholdDays)
			}
		})
	}

	if _, err := fleetStaleTestRun(t, "fleet", "stale", "--db", path, "--json", "--customer", "Nobody Inc"); err == nil {
		t.Fatal("an unknown customer must be an error")
	}
	if _, err := fleetStaleTestRun(t, "fleet", "stale", "--db", path, "--json", "--days", "0"); err == nil {
		t.Fatal("--days 0 must be a usage error")
	}
}

func TestFleetStaleMissingMirrorIsEmptyJSON(t *testing.T) {
	testenv.Isolate(t)
	out, err := fleetStaleTestRun(t, "fleet", "stale", "--db", filepath.Join(t.TempDir(), "absent.db"), "--json")
	if err != nil {
		t.Fatalf("missing mirror must exit 0: %v", err)
	}
	if !strings.Contains(out, `"devices": []`) {
		t.Fatalf("want an empty devices array, got:\n%s", out)
	}
	v := fleetStaleTestDecode(t, out)
	if v.Total != 0 || v.ThresholdDays != 14 {
		t.Fatalf("view = %+v", v)
	}
}

func TestFleetStaleRankPure(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	empty := fleetStaleRank(nil, newEsetTree(nil), fleetStaleOptions{Days: 14}, now)
	if empty.Devices == nil || len(empty.Devices) != 0 || empty.Total != 0 {
		t.Fatalf("empty input must give an empty, non-nil slice: %+v", empty)
	}

	// Single-tenant tree: every device rolls up to (tenant).
	tree := newEsetTree([]esetGroup{{UUID: "all", Name: "All"}})
	devices := []esetDevice{
		{UUID: "a", DisplayName: "OLD", ParentGroupUUID: "all", LastSyncTime: "2026-09-01T12:00:00Z"},
		{UUID: "b", DisplayName: "EDGE", ParentGroupUUID: "all", LastSyncTime: "2026-09-30T12:00:00Z"}, // exactly 14 days: not older
		{UUID: "c", DisplayName: "FUTURE", ParentGroupUUID: "all", LastSyncTime: "2026-10-20T12:00:00Z"},
		{UUID: "d", Hostname: "nameless.local", ParentGroupUUID: "all"},
	}
	v := fleetStaleRank(devices, tree, fleetStaleOptions{Days: 14}, now)
	if got := fleetStaleTestNames(v); got != "nameless.local,OLD" {
		t.Fatalf("devices = %s", got)
	}
	if v.Devices[1].Customer != esetCustomerTenant || v.Devices[1].DaysSince != 43 {
		t.Fatalf("OLD row = %+v", v.Devices[1])
	}
}
