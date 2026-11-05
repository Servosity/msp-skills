// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Codex round-2 regressions for the slice-A ("local") commands.

package cli

import (
	"strings"
	"testing"

	"zoom-phone-pp-cli/internal/store"
)

// ---- item 1: the live re-check compares the email ----

func TestLocalOffboardApplyRefusesChangedEmail(t *testing.T) {
	localTestEnv(t)
	db := localTestStore(t, localTestFootprintRows())
	cases := []struct {
		name, query, liveEmail string
		wantRefused            bool
	}{
		{"matched by email, account renamed", "jane@example.com", "bob@example.com", true},
		{"matched by extension, account renamed", "1001", "bob@example.com", true},
		{"live email empty", "jane@example.com", "", true},
		{"same email, different case", "jane@example.com", "JANE@example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := localNewTestServer(t)
			ts.getBody["/phone/users/u-jane"] = `{"id":"u-jane","email":"` + tc.liveEmail + `","extension_id":"ext-jane","extension_number":1001,"status":"activate"}`
			stdout, _, err := localRun(t, newNovelOffboardCmd, tc.query, "--db", db, "--apply")
			_, deletes := ts.snapshot()
			if !tc.wantRefused {
				if err != nil || len(deletes) != 6 {
					t.Fatalf("unchanged email should apply: err=%v deletes=%v", err, deletes)
				}
				return
			}
			if code := localExitCode(err); code != 5 {
				t.Fatalf("exit = %d (%v), want 5", code, err)
			}
			var res localOffboardResult
			localDecode(t, stdout, &res)
			if !res.Refused || len(res.RefusalReasons) != 1 || !strings.Contains(res.RefusalReasons[0], "live email of user u-jane") {
				t.Fatalf("reasons = %v", res.RefusalReasons)
			}
			if len(deletes) != 0 {
				t.Fatalf("DELETEs sent for a renamed account: %v", deletes)
			}
		})
	}
}

// ---- item 2: a present address owner id is authoritative ----

func TestLocalResolveAddressOwnerIDAuthoritative(t *testing.T) {
	addrs := []localEmergencyAddress{
		// verified personal address of the extension's former holder
		{ID: "ea-old", Level: "1", IsDefault: true, Status: "4", Owner: &localAddressOwner{ID: "u-old", ExtensionNumber: "1001"}},
		{ID: "ea-hq", Level: "0", IsDefault: true, Status: "2", Site: localRef{ID: "site-hq"}},
		// legacy address with no owner id: the number is the only key
		{ID: "ea-noid", Level: "1", Status: "4", Owner: &localAddressOwner{ExtensionNumber: "1900"}},
	}
	if got := localResolveAddress(addrs, true, "u-new", "1001", "site-au"); got.Source != "none" {
		t.Fatalf("u-new at ext 1001 with no site default got %+v, want none (not u-old's address)", got)
	}
	if got := localResolveAddress(addrs, true, "u-new", "1001", "site-hq"); got.Source != "site_default" || got.ID != "ea-hq" {
		t.Fatalf("u-new at HQ got %+v, want the HQ site default", got)
	}
	if got := localResolveAddress(addrs, true, "u-old", "1001", "site-hq"); got.Source != "personal" || got.ID != "ea-old" {
		t.Fatalf("u-old got %+v, want its own personal address", got)
	}
	if got := localResolveAddress(addrs, true, "ca-lobby", "1900", ""); got.Source != "personal" || got.ID != "ea-noid" {
		t.Fatalf("owner without id got %+v, want the extension-number fallback", got)
	}
}

func TestLocalE911AuditStaleOwnerIsNoAddress(t *testing.T) {
	localTestEnv(t)
	rows := localTestE911Rows()
	rows["users"] = []string{
		`{"id":"u-new","email":"new@example.com","name":"New Hire","extension_id":"ext-new","extension_number":1004,"status":"activate","activation_status":"activated","site":` + localTestAustin + `}`,
	}
	rows["common_areas"] = []string{}
	rows["emergency_addresses"] = append(rows["emergency_addresses"],
		`{"id":"ea-dana-old","address_line1":"1 Old Rd","city":"Austin","state_code":"TX","zip":"78701","country":"US","is_default":true,"level":1,"status":4,"owner":{"id":"u-dana","name":"Dana Lee","extension_number":1004},"site":`+localTestAustin+`}`)
	stdout, _, err := localRun(t, newNovelE911AuditCmd, "--db", localTestStore(t, rows))
	if err != nil {
		t.Fatalf("e911 audit: %v", err)
	}
	var res localE911Result
	localDecode(t, stdout, &res)
	found := false
	for _, f := range res.Findings {
		if f.EntityID == "u-new" {
			found = true
			if f.Kind != "NO_ADDRESS" {
				t.Fatalf("u-new finding = %+v, want NO_ADDRESS (the verified address belongs to u-dana)", f)
			}
		}
	}
	if !found {
		t.Fatalf("u-new was not flagged; it inherited the former holder's address: %+v", res.Findings)
	}
}

// ---- item 3: orphans are judged by users-sync completeness ----

func TestLocalOrphansUseUsersSyncCompleteness(t *testing.T) {
	localTestEnv(t)

	// Users synced and legitimately empty: every user assignee is orphaned.
	db := localTestStore(t, map[string][]string{
		"numbers": localTestNumbers, "users": {},
		"assigned": localTestAssignedDevices, "unassigned": localTestUnassignedDevices, "common_areas": {},
	})
	stdout, _, err := localRun(t, newNovelNumbersInventoryCmd, "--db", db)
	if err != nil {
		t.Fatalf("numbers inventory: %v", err)
	}
	var inv localInventoryResult
	localDecode(t, stdout, &inv)
	ids := []string{}
	for _, n := range inv.InactiveAssignee {
		if n.Reason != "not_in_phone_users" {
			t.Fatalf("reason = %q", n.Reason)
		}
		ids = append(ids, n.ID)
	}
	if strings.Join(ids, ",") != "pn-jane,pn-bob,pn-carl,pn-ghost" {
		t.Fatalf("orphaned numbers = %v, want all 4 user-assigned numbers", ids)
	}
	if !inv.Assessed {
		t.Fatalf("synced-empty users should be assessed")
	}

	stdout, _, err = localRun(t, newNovelDevicesHealthCmd, "--db", db)
	if err != nil {
		t.Fatalf("devices health: %v", err)
	}
	var dh localDevicesHealthResult
	localDecode(t, stdout, &dh)
	if dh.Counts["ASSIGNEE_INACTIVE"] != 3 || !dh.Assessed {
		t.Fatalf("ASSIGNEE_INACTIVE = %d assessed=%v, want 3 user-assigned devices", dh.Counts["ASSIGNEE_INACTIVE"], dh.Assessed)
	}

	// Users never synced: nothing can be called orphaned, and nothing is assessed.
	db = localTestStore(t, map[string][]string{
		"numbers":  localTestNumbers,
		"assigned": localTestAssignedDevices, "unassigned": localTestUnassignedDevices, "common_areas": {},
	})
	stdout, _, err = localRun(t, newNovelNumbersInventoryCmd, "--db", db)
	if err != nil {
		t.Fatalf("numbers inventory: %v", err)
	}
	inv = localInventoryResult{}
	localDecode(t, stdout, &inv)
	if len(inv.InactiveAssignee) != 0 || inv.Assessed {
		t.Fatalf("users unsynced: inactive=%v assessed=%v", inv.InactiveAssignee, inv.Assessed)
	}
	stdout, _, err = localRun(t, newNovelDevicesHealthCmd, "--db", db)
	if err != nil {
		t.Fatalf("devices health: %v", err)
	}
	dh = localDevicesHealthResult{}
	localDecode(t, stdout, &dh)
	if dh.Counts["ASSIGNEE_INACTIVE"] != 0 || dh.Assessed {
		t.Fatalf("users unsynced: ASSIGNEE_INACTIVE=%d assessed=%v", dh.Counts["ASSIGNEE_INACTIVE"], dh.Assessed)
	}
}

// ---- item 4: same-second assigned/unassigned rows are ambiguous ----

const localTestJaneDeviceUnassigned = `{"id":"dev-jane","display_name":"Jane Desk","device_type":"Yealink T54W","mac_address":"80-5e-c0-00-00-01","status":"online","firmware_version":"96.86.0.30","site":` + localTestHQ + `}`

// localTestDeviceTieStore puts dev-jane in both partitions and sets the two
// rows' write times: equal for a tie, or one strictly newer.
func localTestDeviceTieStore(t *testing.T, assignedAt, unassignedAt string) string {
	t.Helper()
	rows := localTestCompleteRows()
	rows["unassigned"] = append(append([]string{}, rows["unassigned"]...), localTestJaneDeviceUnassigned)
	path := localTestStore(t, rows)
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for rt, at := range map[string]string{"assigned": assignedAt, "unassigned": unassignedAt} {
		res, err := db.DB().Exec(`UPDATE resources SET updated_at = ?, synced_at = ? WHERE resource_type = ? AND json_extract(data, '$.id') = 'dev-jane'`, at, at, rt)
		if err != nil {
			t.Fatal(err)
		}
		if n, _ := res.RowsAffected(); n != 1 {
			t.Fatalf("%s dev-jane rows updated = %d", rt, n)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLocalDeviceAssignmentTieIsAmbiguous(t *testing.T) {
	localTestEnv(t)
	tie := localTestDeviceTieStore(t, "2026-10-15 10:00:00", "2026-10-15 10:00:00")

	stdout, _, err := localRun(t, newNovelFootprintCmd, "jane@example.com", "--db", tie, "--no-live")
	if err != nil {
		t.Fatalf("footprint: %v", err)
	}
	var fp localFootprint
	localDecode(t, stdout, &fp)
	if len(fp.DeskPhones) != 0 || len(fp.DeviceConflicts) != 1 || fp.DeviceConflicts[0].DeviceID != "dev-jane" ||
		strings.Join(fp.DeviceConflicts[0].Sources, ",") != "assigned,unassigned" {
		t.Fatalf("desk=%v conflicts=%+v, want dev-jane as a conflict only", fp.DeskPhones, fp.DeviceConflicts)
	}

	ts := localNewTestServer(t)
	stdout, _, err = localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", tie, "--apply")
	if code := localExitCode(err); code != 5 {
		t.Fatalf("offboard --apply exit = %d (%v), want 5", code, err)
	}
	var res localOffboardResult
	localDecode(t, stdout, &res)
	if len(res.RefusalReasons) != 1 || !strings.Contains(res.RefusalReasons[0], `device "Jane Desk" (dev-jane)`) {
		t.Fatalf("reasons = %v", res.RefusalReasons)
	}
	if _, deletes := ts.snapshot(); len(deletes) != 0 {
		t.Fatalf("DELETEs sent with an ambiguous device: %v", deletes)
	}

	stdout, _, err = localRun(t, newNovelDevicesHealthCmd, "--db", tie)
	if err != nil {
		t.Fatalf("devices health: %v", err)
	}
	var dh localDevicesHealthResult
	localDecode(t, stdout, &dh)
	flagged := ""
	for _, sh := range dh.BySite {
		for _, f := range sh.Findings {
			if f.DeviceID == "dev-jane" {
				flagged = strings.Join(f.Flags, ",")
			}
		}
	}
	if !strings.Contains(flagged, "ASSIGNMENT_AMBIGUOUS") || dh.Counts["ASSIGNMENT_AMBIGUOUS"] != 1 {
		t.Fatalf("dev-jane flags = %q counts=%v", flagged, dh.Counts)
	}
}

func TestLocalDeviceNewerRowWinsWithoutTie(t *testing.T) {
	localTestEnv(t)
	// Unassigned is strictly newer: the device was unassigned after the
	// assigned sync, so it is not Jane's desk phone and is not ambiguous.
	newerUnassigned := localTestDeviceTieStore(t, "2026-10-15 10:00:00", "2026-10-15 10:00:01")
	stdout, _, err := localRun(t, newNovelFootprintCmd, "jane@example.com", "--db", newerUnassigned, "--no-live")
	if err != nil {
		t.Fatalf("footprint: %v", err)
	}
	var fp localFootprint
	localDecode(t, stdout, &fp)
	if len(fp.DeskPhones) != 0 || len(fp.DeviceConflicts) != 0 {
		t.Fatalf("newer unassigned row: desk=%v conflicts=%v", fp.DeskPhones, fp.DeviceConflicts)
	}

	newerAssigned := localTestDeviceTieStore(t, "2026-10-15 10:00:01", "2026-10-15 10:00:00")
	stdout, _, err = localRun(t, newNovelFootprintCmd, "jane@example.com", "--db", newerAssigned, "--no-live")
	if err != nil {
		t.Fatalf("footprint: %v", err)
	}
	fp = localFootprint{}
	localDecode(t, stdout, &fp)
	if len(fp.DeskPhones) != 1 || len(fp.DeviceConflicts) != 0 {
		t.Fatalf("newer assigned row: desk=%v conflicts=%v", fp.DeskPhones, fp.DeviceConflicts)
	}
}
