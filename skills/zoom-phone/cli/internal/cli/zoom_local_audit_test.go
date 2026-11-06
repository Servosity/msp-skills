// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sort"
	"strconv"
	"strings"
	"testing"
)

// ---- e911 audit ----

func localTestE911Rows() map[string][]string {
	return map[string][]string{
		"users": {
			// personal level-1 address, verified: must NOT be flagged
			`{"id":"u-jane","email":"jane@example.com","name":"Jane Doe","extension_id":"ext-jane","extension_number":1001,"status":"activate","activation_status":"activated","site":` + localTestHQ + `}`,
			// personal address rejected
			`{"id":"u-bob","email":"bob@example.com","name":"Bob Smith","extension_id":"ext-bob","extension_number":1002,"status":"activate","activation_status":"activated","site":` + localTestHQ + `}`,
			// Austin has no default address and no personal one: NO_ADDRESS
			`{"id":"u-dana","email":"dana@example.com","name":"Dana Lee","extension_id":"ext-dana","extension_number":1004,"status":"activate","activation_status":"activated","site":` + localTestAustin + `}`,
			// falls back to the unverified Boston site default
			`{"id":"u-eve","email":"eve@example.com","name":"Eve Park","extension_id":"ext-eve","extension_number":1005,"status":"activate","activation_status":"ready to activate","site":` + localTestBoston + `}`,
			// deactivated: skipped even though it has no address
			`{"id":"u-carl","email":"carl@example.com","name":"Carl Gone","extension_id":"ext-carl","extension_number":1003,"status":"deactivate","activation_status":"not ready","site":` + localTestAustin + `}`,
		},
		"common_areas": {
			`{"id":"ca-lobby","display_name":"Lobby","extension_number":1900,"status":"online","site":` + localTestHQ + `,"desk_phones":[]}`,
			`{"id":"ca-austin","display_name":"Austin Break Room","extension_number":1901,"status":"online","site":` + localTestAustin + `}`,
		},
		"sites": {
			`{"id":"site-hq","name":"San Jose HQ","site_code":1,"country":{"code":"US","name":"United States"}}`,
			`{"id":"site-br","name":"Boston Branch","site_code":2}`,
			`{"id":"site-au","name":"Austin","site_code":3}`,
		},
		"emergency_addresses": {
			`{"id":"ea-hq","address_line1":"55 Almaden Blvd","city":"San Jose","state_code":"CA","zip":"95113","country":"US","is_default":true,"level":0,"status":4,"site":` + localTestHQ + `}`,
			`{"id":"ea-br","address_line1":"1 Federal St","city":"Boston","state_code":"MA","zip":"02110","country":"US","is_default":true,"level":0,"status":2,"site":` + localTestBoston + `}`,
			`{"id":"ea-jane","address_line1":"100 Main St","city":"Campbell","state_code":"CA","zip":"95008","country":"US","is_default":true,"level":1,"status":4,"owner":{"id":"u-jane","name":"Jane Doe","extension_number":1001},"site":` + localTestHQ + `}`,
			`{"id":"ea-bob","address_line1":"9 Elm St","city":"San Jose","state_code":"CA","zip":"95110","country":"US","is_default":true,"level":1,"status":5,"owner":{"id":"u-bob","name":"Bob Smith","extension_number":1002},"site":` + localTestHQ + `}`,
		},
		"locations": {
			`{"id":"loc-hq","name":"HQ Floor 1","identifier":"hq-1","site":` + localTestHQ + `,"emergency_address":{"id":"ea-hq"}}`,
		},
		"numbers": {
			`{"id":"pn-byoc","number":"+15125550100","source":"external","status":"available","site":` + localTestAustin + `,"carrier":{"code":1,"name":"Bandwidth"},"emergency_address_status":1,"emergency_address_update_time":"2026-10-01T10:00:00Z"}`,
			`{"id":"pn-byoc-ok","number":"+15125550101","source":"external","status":"available","site":` + localTestAustin + `,"carrier":{"code":1,"name":"Bandwidth"},"emergency_address_status":2}`,
		},
	}
}

func localFindingKeys(fs []localE911Finding) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Kind+"|"+f.EntityType+"|"+f.EntityID)
	}
	sort.Strings(out)
	return out
}

func TestLocalE911AuditFlagsGapsNotVerified(t *testing.T) {
	localTestEnv(t)
	db := localTestStore(t, localTestE911Rows())
	stdout, stderr, err := localRun(t, newNovelE911AuditCmd, "--db", db)
	if err != nil {
		t.Fatalf("e911 audit: %v\n%s", err, stderr)
	}
	var res localE911Result
	localDecode(t, stdout, &res)
	want := []string{
		"ADDRESS_NOT_VERIFIED|site|site-br",
		"ADDRESS_NOT_VERIFIED|user|u-bob",
		"ADDRESS_NOT_VERIFIED|user|u-eve",
		"BYOC_CARRIER_UPDATE_REQUIRED|phone_number|pn-byoc",
		"NO_ADDRESS|common_area|ca-austin",
		"NO_ADDRESS|user|u-dana",
		"SITE_NO_DEFAULT_ADDRESS|site|site-au",
		"SITE_NO_ERL|site|site-au",
		"SITE_NO_ERL|site|site-br",
	}
	if got := localFindingKeys(res.Findings); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, f := range res.Findings {
		if f.EntityID == "u-jane" || f.EntityID == "ca-lobby" || f.EntityID == "u-carl" || f.EntityID == "site-hq" {
			t.Fatalf("verified/inactive entity flagged: %+v", f)
		}
		switch f.EntityID {
		case "u-bob":
			if f.Status != "rejected" || f.Severity != "high" || f.AddressSource != "personal" {
				t.Fatalf("bob finding = %+v, want rejected/high/personal", f)
			}
		case "u-eve":
			if f.Status != "unverified" || f.Severity != "medium" || f.AddressSource != "site_default" || f.AddressID != "ea-br" {
				t.Fatalf("eve finding = %+v, want unverified site default ea-br", f)
			}
		case "u-dana":
			if f.Severity != "high" || f.Site != "Austin" {
				t.Fatalf("dana finding = %+v", f)
			}
		}
	}
	if res.Counts.Total != 9 || res.Counts.ByKind["NO_ADDRESS"] != 2 || res.Counts.BySeverity["high"] != 4 {
		t.Fatalf("counts = %+v", res.Counts)
	}
	if res.Checked.Users != 4 || res.Checked.SkippedInactiveUsers != 1 || res.Checked.CommonAreas != 2 || res.Checked.Sites != 3 || res.Checked.Numbers != 2 {
		t.Fatalf("checked = %+v", res.Checked)
	}
	if len(res.SkippedChecks) != 0 || len(res.MissingLocal) != 0 {
		t.Fatalf("skipped=%v missing=%v, want none", res.SkippedChecks, res.MissingLocal)
	}
}

func TestLocalE911AuditCleanAndUnsynced(t *testing.T) {
	localTestEnv(t)

	clean := localTestE911Rows()
	clean["users"] = clean["users"][:1] // only verified Jane
	clean["common_areas"] = clean["common_areas"][:1]
	clean["sites"] = clean["sites"][:1]
	clean["numbers"] = clean["numbers"][1:] // BYOC already confirmed
	stdout, _, err := localRun(t, newNovelE911AuditCmd, "--db", localTestStore(t, clean))
	if err != nil {
		t.Fatalf("clean audit: %v", err)
	}
	var res localE911Result
	localDecode(t, stdout, &res)
	if len(res.Findings) != 0 || res.Findings == nil || res.Counts.Total != 0 {
		t.Fatalf("clean account produced findings: %+v", res.Findings)
	}

	// emergency_addresses never synced: no NO_ADDRESS may be fabricated.
	unsynced := localTestE911Rows()
	delete(unsynced, "emergency_addresses")
	delete(unsynced, "locations")
	stdout, stderr, err := localRun(t, newNovelE911AuditCmd, "--db", localTestStore(t, unsynced))
	if err != nil {
		t.Fatalf("unsynced audit: %v", err)
	}
	res = localE911Result{}
	localDecode(t, stdout, &res)
	for _, f := range res.Findings {
		if f.Kind != "BYOC_CARRIER_UPDATE_REQUIRED" {
			t.Fatalf("fabricated %s without emergency_addresses synced: %+v", f.Kind, f)
		}
	}
	if strings.Join(res.MissingLocal, ",") != "emergency_addresses,locations" || len(res.SkippedChecks) != 2 || res.Assessed {
		t.Fatalf("missing=%v skipped=%v", res.MissingLocal, res.SkippedChecks)
	}
	if !strings.Contains(stderr, "sync --full --resources emergency_addresses,locations") {
		t.Fatalf("stderr hint = %q", stderr)
	}
}

func TestLocalResolveAddressFallbackChain(t *testing.T) {
	addrs := []localEmergencyAddress{
		{ID: "acct", Level: "0", IsDefault: true, Status: "4"},
		{ID: "hq", Level: "0", IsDefault: true, Status: "1", Site: localRef{ID: "site-hq"}},
		{ID: "hq-alt", Level: "0", IsDefault: false, Status: "4", Site: localRef{ID: "site-hq"}},
		{ID: "p1", Level: "1", Status: "3", Owner: &localAddressOwner{ID: "u1"}},
		{ID: "p1-default", Level: "1", IsDefault: true, Status: "6", Owner: &localAddressOwner{ID: "u1"}},
		{ID: "ext-owned", Level: "1", Status: "4", Owner: &localAddressOwner{ExtensionNumber: "1900"}},
	}
	cases := []struct {
		name, owner, ext, site string
		synced                 bool
		wantSource, wantID     string
		wantOK                 bool
	}{
		{"personal default wins", "u1", "", "site-hq", true, "personal", "p1-default", false},
		{"owner by extension", "ca-1", "1900", "", true, "personal", "ext-owned", true},
		{"site default", "u2", "", "site-hq", true, "site_default", "hq", true},
		{"account default", "u3", "", "site-zz", true, "account_default", "acct", true},
		{"not synced", "u1", "", "site-hq", false, "not_synced", "", false},
	}
	for _, tc := range cases {
		got := localResolveAddress(addrs, tc.synced, tc.owner, tc.ext, tc.site)
		if got.Source != tc.wantSource || got.ID != tc.wantID || got.OK != tc.wantOK {
			t.Errorf("%s: got %+v, want %s/%s ok=%v", tc.name, got, tc.wantSource, tc.wantID, tc.wantOK)
		}
	}
	if got := localResolveAddress(addrs[3:5], true, "u9", "", "site-hq"); got.Source != "none" {
		t.Errorf("no chain match: got %+v, want none", got)
	}
}

// ---- numbers inventory ----

func TestLocalNumbersInventoryCounts(t *testing.T) {
	localTestEnv(t)
	db := localTestStore(t, map[string][]string{"numbers": localTestNumbers, "users": localTestUsers})

	stdout, _, err := localRun(t, newNovelNumbersInventoryCmd, "--db", db)
	if err != nil {
		t.Fatalf("numbers inventory: %v", err)
	}
	var res localInventoryResult
	localDecode(t, stdout, &res)
	c := res.Counts
	if c.Total != 8 || c.Assigned != 5 || c.Unassigned != 2 || c.Pending != 1 || c.InactiveAssignee != 2 {
		t.Fatalf("counts = %+v, want total 8 assigned 5 unassigned 2 pending 1 inactive 2", c)
	}
	if c.BySource["internal"] != 6 || c.BySource["external"] != 2 || c.ByAssigneeType["user"] != 4 || c.ByAssigneeType["callQueue"] != 1 || c.ByAssigneeType["unassigned"] != 3 {
		t.Fatalf("by_source=%v by_assignee_type=%v", c.BySource, c.ByAssigneeType)
	}
	cells := []string{}
	for _, cell := range res.Breakdown {
		cells = append(cells, cell.Site+"|"+cell.Source+"|"+cell.AssigneeType+"|"+strconv.Itoa(cell.Count))
	}
	wantCells := []string{
		"Boston Branch|external|unassigned|1",
		"Boston Branch|external|user|1",
		"Boston Branch|internal|user|1",
		"San Jose HQ|internal|callQueue|1",
		"San Jose HQ|internal|unassigned|2",
		"San Jose HQ|internal|user|2",
	}
	if strings.Join(cells, "\n") != strings.Join(wantCells, "\n") {
		t.Fatalf("breakdown:\n%s\nwant:\n%s", strings.Join(cells, "\n"), strings.Join(wantCells, "\n"))
	}
	ids := func(list []localInventoryNumber) string {
		out := []string{}
		for _, n := range list {
			out = append(out, n.ID+":"+n.Reason)
		}
		return strings.Join(out, ",")
	}
	if got := ids(res.Unassigned); got != "pn-free1:,pn-free2:" {
		t.Fatalf("unassigned = %s (pending must be excluded)", got)
	}
	if got := ids(res.Pending); got != "pn-pending:" {
		t.Fatalf("pending = %s", got)
	}
	if got := ids(res.InactiveAssignee); got != "pn-carl:deactivated,pn-ghost:not_in_phone_users" {
		t.Fatalf("inactive_assignee = %s", got)
	}

	stdout, _, err = localRun(t, newNovelNumbersInventoryCmd, "--db", db, "--site", "boston branch")
	if err != nil {
		t.Fatalf("--site: %v", err)
	}
	res = localInventoryResult{}
	localDecode(t, stdout, &res)
	if res.Counts.Total != 3 || res.Counts.Unassigned != 1 || res.Counts.InactiveAssignee != 2 {
		t.Fatalf("--site boston counts = %+v", res.Counts)
	}
}

func TestLocalNumbersInventoryEmpty(t *testing.T) {
	localTestEnv(t)
	for name, db := range map[string]string{
		"synced empty": localTestStore(t, map[string][]string{"numbers": {}, "users": {}}),
	} {
		stdout, _, err := localRun(t, newNovelNumbersInventoryCmd, "--db", db)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var res localInventoryResult
		localDecode(t, stdout, &res)
		if res.Counts.Total != 0 || len(res.Breakdown) != 0 || res.Unassigned == nil || len(res.InactiveAssignee) != 0 || !res.Assessed || len(res.MissingLocal) != 0 {
			t.Fatalf("%s fabricated inventory: %+v", name, res)
		}
	}
}

// ---- devices health ----

func TestLocalDevicesHealthFlags(t *testing.T) {
	localTestEnv(t)
	db := localTestStore(t, map[string][]string{
		"assigned":     localTestAssignedDevices,
		"unassigned":   localTestUnassignedDevices,
		"users":        localTestUsers,
		"common_areas": {`{"id":"ca-lobby","display_name":"Lobby","extension_number":1900,"status":"online","site":` + localTestHQ + `,"desk_phones":[{"id":"dev-lobby","display_name":"Lobby Phone","device_type":"Poly VVX 450","status":"online"},{"id":"dev-kiosk","display_name":"Kiosk","device_type":"Poly VVX 150","status":"offline"}]}`},
	})
	stdout, _, err := localRun(t, newNovelDevicesHealthCmd, "--db", db)
	if err != nil {
		t.Fatalf("devices health: %v", err)
	}
	var res localDevicesHealthResult
	localDecode(t, stdout, &res)

	flags := map[string]string{}
	for _, sh := range res.BySite {
		for _, f := range sh.Findings {
			flags[f.DeviceID] = sh.Site + ":" + strings.Join(f.Flags, ",")
		}
	}
	want := map[string]string{
		"dev-carl":  "Boston Branch:OFFLINE,FIRMWARE_DRIFT,ASSIGNEE_INACTIVE",
		"dev-spare": "San Jose HQ:OFFLINE,UNASSIGNED",
		"dev-lobby": "San Jose HQ:NO_TEMPLATE",
		"dev-kiosk": "San Jose HQ:OFFLINE",
	}
	if len(flags) != len(want) {
		t.Fatalf("flagged devices = %v, want %v", flags, want)
	}
	for id, w := range want {
		if flags[id] != w {
			t.Fatalf("%s flags = %q, want %q (all flagged: %v)", id, flags[id], w, flags)
		}
	}
	if _, ok := flags["dev-jane"]; ok {
		t.Fatalf("healthy majority-firmware device flagged: %v", flags["dev-jane"])
	}
	if res.Counts["FIRMWARE_DRIFT"] != 1 || res.Counts["OFFLINE"] != 3 || res.DevicesChecked != 6 {
		t.Fatalf("counts=%v checked=%d", res.Counts, res.DevicesChecked)
	}
	var yealink *localFirmwareBaseline
	for i := range res.FirmwareBaselines {
		if res.FirmwareBaselines[i].DeviceType == "Yealink T54W" {
			yealink = &res.FirmwareBaselines[i]
		}
	}
	if yealink == nil || yealink.Baseline != "96.86.0.30" || yealink.Devices != 4 || yealink.Versions["96.85.0.5"] != 1 {
		t.Fatalf("yealink baseline = %+v", yealink)
	}

	stdout, _, err = localRun(t, newNovelDevicesHealthCmd, "--db", db, "--site", "site-br")
	if err != nil {
		t.Fatalf("--site: %v", err)
	}
	res = localDevicesHealthResult{}
	localDecode(t, stdout, &res)
	if res.DevicesChecked != 1 || len(res.BySite) != 1 || res.BySite[0].Site != "Boston Branch" || res.Counts["FIRMWARE_DRIFT"] != 1 {
		t.Fatalf("--site site-br = %+v (baseline must stay account-wide)", res)
	}
}

func TestLocalFirmwareBaselineTieAndMajority(t *testing.T) {
	mk := func(id, typ, fw string) localDevice {
		return localDevice{ID: zoomFlexString(id), DeviceType: zoomFlexString(typ), FirmwareVersion: zoomFlexString(fw)}
	}
	b := localFirmwareBaselines([]localDevice{
		mk("1", "Poly VVX 450", "6.4.0"), mk("2", "Poly VVX 450", "6.3.0"),
		mk("3", "Yealink T46U", "1.0"), mk("4", "Yealink T46U", "1.0"), mk("5", "Yealink T46U", "0.9"),
	})
	if p := b["poly vvx 450"]; !p.Tie || p.Baseline != "" {
		t.Fatalf("1-1 split must be a tie with no baseline: %+v", p)
	}
	if y := b["yealink t46u"]; y.Tie || y.Baseline != "1.0" {
		t.Fatalf("2-1 split baseline = %+v, want 1.0", y)
	}
	res := localDevicesHealth([]localDevice{mk("1", "Poly VVX 450", "6.4.0"), mk("2", "Poly VVX 450", "6.3.0")}, nil, false, nil, "")
	if res.Counts[localFlagFirmwareDrift] != 0 {
		t.Fatalf("tie flagged drift: %v", res.Counts)
	}
}
