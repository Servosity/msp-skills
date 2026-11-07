// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Review round 2 regressions for the slice-A ("local") commands.

package cli

import (
	"strings"
	"testing"
)

func localTestDeletesContain(deletes []string, sub string) bool {
	for _, d := range deletes {
		if strings.Contains(d, sub) {
			return true
		}
	}
	return false
}

// ---- item 1: stable ids decide membership; live user re-check ----

func TestLocalMemberMatchesStableIDsFirst(t *testing.T) {
	jane := localUser{ID: "u-jane", ExtensionID: "ext-jane", ExtensionNumber: "1001"}
	cases := []struct {
		name                     string
		id, extID, number, xtype string
		want                     bool
	}{
		{"id and extension id agree", "u-jane", "ext-jane", "1001", "user", true},
		{"id only", "u-jane", "", "", "", true},
		{"extension id only", "", "ext-jane", "", "", true},
		{"Bob now holds 1001", "u-bob", "ext-bob", "1001", "user", false},
		{"conflicting extension id, same number", "", "ext-bob", "1001", "user", false},
		{"id agrees, extension id conflicts", "u-jane", "ext-bob", "1001", "user", false},
		{"no stable id, number matches", "", "", "1001", "user", true},
		{"no stable id, number matches a common area", "", "", "1001", "commonArea", false},
		{"no stable id, other number", "", "", "1002", "user", false},
		{"nothing to compare", "", "", "", "user", false},
	}
	for _, tc := range cases {
		if got := localMemberMatches(tc.id, tc.extID, tc.number, tc.xtype, jane); got != tc.want {
			t.Errorf("%s: localMemberMatches = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestLocalOffboardReassignedExtensionDoesNotDeleteBob(t *testing.T) {
	localTestEnv(t)
	ts := localNewTestServer(t)
	// Jane's former extension number 1001 now belongs to Bob in the live
	// pickup and monitoring member lists.
	ts.getBody["/phone/group_call_pickup/gcp-1/members"] = `{"group_call_pickup_member":[{"id":"u-bob","display_name":"Bob Smith","extension_id":"ext-bob","extension_type":"user","extension_number":1001}],"total_records":1}`
	ts.getBody["/phone/monitoring_groups/mg-1/monitor_members"] = `{"members":[{"display_name":"Bob Smith","extension_id":"ext-bob","extension_number":1001,"extension_type":"user"}],"total_records":1}`
	db := localTestStore(t, localTestFootprintRows())

	stdout, stderr, err := localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db, "--apply")
	if err != nil {
		t.Fatalf("offboard --apply: %v\n%s", err, stderr)
	}
	var res localOffboardResult
	localDecode(t, stdout, &res)
	_, deletes := ts.snapshot()
	if localTestDeletesContain(deletes, "ext-bob") || localTestDeletesContain(deletes, "u-bob") {
		t.Fatalf("a DELETE targeted Bob: %v", deletes)
	}
	if localTestDeletesContain(deletes, "group_call_pickup") || localTestDeletesContain(deletes, "monitoring_groups") {
		t.Fatalf("pickup/monitoring rows that only shared Jane's old number were deleted: %v", deletes)
	}
	if len(deletes) != 5 {
		t.Fatalf("deletes = %v, want Jane's 2 queues, device, number and plan", deletes)
	}
}

func TestLocalOffboardApplyVerifiesUserLive(t *testing.T) {
	localTestEnv(t)
	db := localTestStore(t, localTestFootprintRows())

	t.Run("extension reassigned since sync", func(t *testing.T) {
		ts := localNewTestServer(t)
		ts.getBody["/phone/users/u-jane"] = `{"id":"u-jane","email":"jane@example.com","extension_id":"ext-jane","extension_number":1009,"status":"activate"}`
		stdout, _, err := localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db, "--apply")
		if code := localExitCode(err); code != 5 {
			t.Fatalf("exit = %d (%v), want 5", code, err)
		}
		var res localOffboardResult
		localDecode(t, stdout, &res)
		if !res.Refused || len(res.RefusalReasons) != 1 || !strings.Contains(res.RefusalReasons[0], `live extension number is "1009"`) {
			t.Fatalf("reasons = %v", res.RefusalReasons)
		}
		if _, deletes := ts.snapshot(); len(deletes) != 0 {
			t.Fatalf("DELETEs sent after a failed live check: %v", deletes)
		}
	})

	t.Run("user cannot be read", func(t *testing.T) {
		ts := localNewTestServer(t)
		ts.getStatus["/phone/users/u-jane"] = 404
		_, _, err := localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db, "--apply")
		if code := localExitCode(err); code != 5 || !strings.Contains(err.Error(), "could not verify user u-jane live") {
			t.Fatalf("exit = %d (%v), want 5", code, err)
		}
		if _, deletes := ts.snapshot(); len(deletes) != 0 {
			t.Fatalf("DELETEs sent without a live check: %v", deletes)
		}
	})
}

// ---- item 2: device partition completeness ----

func TestLocalDevicePartitionsCompleteness(t *testing.T) {
	localTestEnv(t)

	// offboard needs "assigned" only.
	rows := localTestFootprintRows()
	delete(rows, "unassigned")
	ts := localNewTestServer(t)
	unassignedDB := localTestStore(t, rows)
	stdout, _, err := localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", unassignedDB, "--apply")
	if err != nil {
		t.Fatalf("offboard with unassigned unsynced should apply: %v", err)
	}
	var applied localOffboardResult
	localDecode(t, stdout, &applied)
	if len(applied.MissingLocal) != 0 || !applied.Assessed {
		t.Fatalf("unassigned is irrelevant to offboard: missing=%v assessed=%v", applied.MissingLocal, applied.Assessed)
	}
	if _, deletes := ts.snapshot(); len(deletes) != 6 {
		t.Fatalf("deletes = %v", deletes)
	}
	stdout, _, err = localRun(t, newNovelFootprintCmd, "jane@example.com", "--db", unassignedDB, "--no-live")
	if err != nil {
		t.Fatalf("footprint: %v", err)
	}
	var fpu localFootprint
	localDecode(t, stdout, &fpu)
	if len(fpu.MissingLocal) != 0 || !fpu.Assessed || len(fpu.DeskPhones) != 1 {
		t.Fatalf("footprint with unassigned unsynced: missing=%v assessed=%v desk=%v", fpu.MissingLocal, fpu.Assessed, fpu.DeskPhones)
	}

	rows = localTestFootprintRows()
	delete(rows, "assigned")
	ts = localNewTestServer(t)
	stdout, _, err = localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", localTestStore(t, rows), "--apply")
	if code := localExitCode(err); code != 5 {
		t.Fatalf("offboard with assigned unsynced: exit %d (%v), want 5", code, err)
	}
	var ob localOffboardResult
	localDecode(t, stdout, &ob)
	if strings.Join(ob.MissingLocal, ",") != "assigned" || !strings.Contains(strings.Join(ob.RefusalReasons, ";"), "not synced: assigned") {
		t.Fatalf("missing=%v reasons=%v", ob.MissingLocal, ob.RefusalReasons)
	}
	if _, deletes := ts.snapshot(); len(deletes) != 0 {
		t.Fatalf("deletes = %v", deletes)
	}

	// devices health needs BOTH partitions.
	for _, drop := range []string{"assigned", "unassigned"} {
		rows := map[string][]string{
			"assigned": localTestAssignedDevices, "unassigned": localTestUnassignedDevices,
			"users": localTestUsers, "common_areas": {},
		}
		delete(rows, drop)
		stdout, _, err := localRun(t, newNovelDevicesHealthCmd, "--db", localTestStore(t, rows))
		if err != nil {
			t.Fatalf("devices health without %s: %v", drop, err)
		}
		var res localDevicesHealthResult
		localDecode(t, stdout, &res)
		if res.Assessed || strings.Join(res.MissingLocal, ",") != drop {
			t.Fatalf("devices health without %s: assessed=%v missing=%v", drop, res.Assessed, res.MissingLocal)
		}
	}
}

// ---- item 3: --apply enumerates groups live and ignores cached counts ----

func TestLocalOffboardApplyListsGroupsLive(t *testing.T) {
	localTestEnv(t)
	ts := localNewTestServer(t)
	// The live queue list has a queue the stale local cache does not, and
	// the "empty" pickup group (cached member_count 0) really has Jane.
	ts.getBody["/phone/call_queues"] = `{"call_queues":[` + strings.Join(localTestQueues, ",") + `,{"id":"cq-new","name":"Night Shift","extension_id":"ext-cq-new","extension_number":2009,"status":"active"}],"next_page_token":"","total_records":4}`
	ts.getBody["/phone/call_queues/cq-new/members"] = `{"call_queue_members":[{"id":"u-jane","level":"user","receive_call":true,"extension_id":"ext-jane"}]}`
	ts.getBody["/phone/group_call_pickup/gcp-empty/members"] = `{"group_call_pickup_member":[{"id":"u-jane","extension_id":"ext-jane","extension_type":"user","extension_number":1001}],"total_records":1}`
	db := localTestStore(t, localTestFootprintRows())

	stdout, _, err := localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db, "--apply")
	if err != nil {
		t.Fatalf("offboard --apply: %v", err)
	}
	var res localOffboardResult
	localDecode(t, stdout, &res)
	reqs, deletes := ts.snapshot()
	joined := strings.Join(reqs, "\n")
	for _, list := range []string{"/phone/call_queues?", "/phone/shared_line_groups?", "/phone/group_call_pickup?", "/phone/monitoring_groups?"} {
		if !strings.Contains(joined, "GET "+list) {
			t.Fatalf("--apply did not list %s live:\n%s", list, joined)
		}
	}
	for _, want := range []string{"/phone/call_queues/cq-new/members/u-jane", "/phone/group_call_pickup/gcp-empty/members/ext-jane"} {
		found := false
		for _, d := range deletes {
			if d == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing DELETE %s; got %v", want, deletes)
		}
	}

	// Plan mode keeps using the synced lists and cached counts.
	ts = localNewTestServer(t)
	if _, _, err := localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db); err != nil {
		t.Fatalf("plan: %v", err)
	}
	reqs, _ = ts.snapshot()
	joined = strings.Join(reqs, "\n")
	if strings.Contains(joined, "GET /phone/call_queues?") || strings.Contains(joined, "gcp-empty") {
		t.Fatalf("plan mode re-listed groups or ignored the cached count:\n%s", joined)
	}
}

// ---- item 4: truncation and unplannable steps block --apply ----

func TestLocalOffboardApplyRefusesTruncatedMemberList(t *testing.T) {
	localTestEnv(t)
	prev := localMaxPages
	localMaxPages = 3
	t.Cleanup(func() { localMaxPages = prev })
	ts := localNewTestServer(t)
	ts.pages["/phone/call_queues/cq-billing/members"] = true
	db := localTestStore(t, localTestFootprintRows())

	// footprint surfaces the truncation.
	stdout, _, err := localRun(t, newNovelFootprintCmd, "jane@example.com", "--db", db)
	if err != nil {
		t.Fatalf("footprint: %v", err)
	}
	var fp localFootprint
	localDecode(t, stdout, &fp)
	if len(fp.Truncated) != 1 || fp.Truncated[0].GroupID != "cq-billing" {
		t.Fatalf("truncated = %+v, want cq-billing", fp.Truncated)
	}

	ts = localNewTestServer(t)
	ts.pages["/phone/call_queues/cq-billing/members"] = true
	stdout, _, err = localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db, "--apply")
	if code := localExitCode(err); code != 5 {
		t.Fatalf("exit = %d (%v), want 5", code, err)
	}
	var res localOffboardResult
	localDecode(t, stdout, &res)
	if !res.Refused || len(res.RefusalReasons) != 1 || !strings.Contains(res.RefusalReasons[0], `truncated call_queues "Billing" (cq-billing)`) {
		t.Fatalf("reasons = %v", res.RefusalReasons)
	}
	if _, deletes := ts.snapshot(); len(deletes) != 0 {
		t.Fatalf("DELETEs sent with a truncated member list: %v", deletes)
	}
}

func TestLocalOffboardApplyRefusesUnplannableSteps(t *testing.T) {
	localTestEnv(t)
	ts := localNewTestServer(t)
	rows := localTestFootprintRows()
	users := append([]string(nil), rows["users"]...)
	// A calling plan with no type cannot be turned into a DELETE.
	users[0] = strings.Replace(users[0], `"calling_plans":[{"type":200,"name":"US/CA Unlimited Calling Plan"}]`,
		`"calling_plans":[{"type":200,"name":"US/CA Unlimited Calling Plan"},{"name":"Legacy Plan"}]`, 1)
	if users[0] == rows["users"][0] {
		t.Fatal("fixture replace did not apply")
	}
	rows["users"] = users
	db := localTestStore(t, rows)

	stdout, _, err := localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db, "--apply")
	if code := localExitCode(err); code != 5 {
		t.Fatalf("exit = %d (%v), want 5", code, err)
	}
	var res localOffboardResult
	localDecode(t, stdout, &res)
	if len(res.Unplannable) != 1 || len(res.RefusalReasons) != 1 || !strings.Contains(res.RefusalReasons[0], "unplannable step: calling plan Legacy Plan") {
		t.Fatalf("unplannable=%v reasons=%v", res.Unplannable, res.RefusalReasons)
	}
	if _, deletes := ts.snapshot(); len(deletes) != 0 {
		t.Fatalf("DELETEs sent with an unplannable step: %v", deletes)
	}
	// Plan mode still reports it without refusing.
	stdout, _, err = localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	res = localOffboardResult{}
	localDecode(t, stdout, &res)
	if res.Refused || len(res.Unplannable) != 1 {
		t.Fatalf("plan mode: refused=%v unplannable=%v", res.Refused, res.Unplannable)
	}
}

// A plan dependency footprint does not track could never appear in
// missing_local, so it would silently never block --apply.
func TestLocalOffboardDependenciesAreTracked(t *testing.T) {
	tracked := map[string]bool{}
	for _, r := range localFootprintSyncResources {
		tracked[r] = true
	}
	for _, d := range localOffboardPlanDependencies {
		if !tracked[d] {
			t.Fatalf("plan dependency %q is not in localFootprintSyncResources %v", d, localFootprintSyncResources)
		}
	}
	if tracked["unassigned"] {
		t.Fatalf("unassigned devices belong to nobody; footprint/offboard must not require them")
	}
}
