// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

func localMembershipNames(ms []localMembership) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.GroupName)
	}
	return out
}

func TestLocalFootprintJoinsUserMemberships(t *testing.T) {
	localTestEnv(t)
	ts := localNewTestServer(t)
	db := localTestStore(t, localTestFootprintRows())

	stdout, stderr, err := localRun(t, newNovelFootprintCmd, "jane@example.com", "--db", db)
	if err != nil {
		t.Fatalf("footprint error: %v\nstderr: %s", err, stderr)
	}
	var fp localFootprint
	localDecode(t, stdout, &fp)

	if fp.User == nil || fp.User.ID != "u-jane" || fp.User.ExtensionNumber != "1001" || fp.User.Site == nil || fp.User.Site.Name != "San Jose HQ" {
		t.Fatalf("user = %+v, want Jane Doe ext 1001 at San Jose HQ", fp.User)
	}
	if got := strings.Join(localMembershipNames(fp.CallQueues), ","); got != "Sales,Support" {
		t.Fatalf("call_queues = %q, want Sales,Support (Billing belongs to Bob only)", got)
	}
	for _, q := range fp.CallQueues {
		if q.MemberID != "u-jane" || q.MemberExtensionID != "ext-jane" {
			t.Fatalf("queue %s member ids = %q/%q, want u-jane/ext-jane", q.GroupName, q.MemberID, q.MemberExtensionID)
		}
	}
	if len(fp.SharedLineGroups) != 0 || len(fp.GroupCallPickup) != 0 {
		t.Fatalf("Bob's shared line group / pickup memberships leaked into Jane's footprint: slg=%v pickup=%v", fp.SharedLineGroups, fp.GroupCallPickup)
	}
	if len(fp.MonitoringGroups) != 1 || fp.MonitoringGroups[0].GroupName != "Sales Floor" || fp.MonitoringGroups[0].MemberType != "monitor" {
		t.Fatalf("monitoring_groups = %+v, want Sales Floor as monitor", fp.MonitoringGroups)
	}
	if len(fp.DeskPhones) != 1 || fp.DeskPhones[0].ID != "dev-jane" || fp.DeskPhones[0].AssigneeExtensionID != "ext-jane" {
		t.Fatalf("desk_phones = %+v, want only dev-jane", fp.DeskPhones)
	}
	if len(fp.PhoneNumbers) != 1 || fp.PhoneNumbers[0].ID != "pn-jane" || fp.PhoneNumbers[0].Number != "+14155550123" || fp.PhoneNumbers[0].Source != "internal" {
		t.Fatalf("phone_numbers = %+v, want only pn-jane (internal)", fp.PhoneNumbers)
	}
	if len(fp.CallingPlans) != 1 || fp.CallingPlans[0].Type != "200" {
		t.Fatalf("calling_plans = %+v, want type 200", fp.CallingPlans)
	}
	if a := fp.EmergencyAddress; a == nil || a.Source != "personal" || a.ID != "ea-jane" || !a.OK || a.Status != "verified" {
		t.Fatalf("emergency_address = %+v, want personal ea-jane verified", fp.EmergencyAddress)
	}
	if fp.MembershipSource != "live" || len(fp.FetchFailures) != 0 || len(fp.SkippedLive) != 0 {
		t.Fatalf("membership_source=%q failures=%v skipped=%v", fp.MembershipSource, fp.FetchFailures, fp.SkippedLive)
	}
	reqs, deletes := ts.snapshot()
	if len(deletes) != 0 {
		t.Fatalf("footprint sent DELETEs: %v", deletes)
	}
	joined := strings.Join(reqs, "\n")
	if strings.Contains(joined, "gcp-empty") {
		t.Fatalf("pickup group with member_count 0 was fetched:\n%s", joined)
	}
	if strings.Contains(joined, "member_type=monitored") {
		t.Fatalf("monitored list fetched although monitored_members_count is 0:\n%s", joined)
	}
	for _, want := range []string{"GET /phone/shared_line_groups/slg-front?", "GET /phone/call_queues/cq-billing/members?"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected request %q, got:\n%s", want, joined)
		}
	}
}

func TestLocalFootprintNoLiveSkipsMembership(t *testing.T) {
	localTestEnv(t)
	ts := localNewTestServer(t)
	db := localTestStore(t, localTestFootprintRows())

	stdout, _, err := localRun(t, newNovelFootprintCmd, "1001", "--db", db, "--no-live")
	if err != nil {
		t.Fatalf("footprint --no-live: %v", err)
	}
	var fp localFootprint
	localDecode(t, stdout, &fp)
	if fp.User == nil || fp.User.Email != "jane@example.com" {
		t.Fatalf("extension 1001 resolved to %+v, want jane", fp.User)
	}
	if strings.Join(fp.SkippedLive, ",") != "call_queues,shared_line_groups,group_call_pickup,monitoring_groups" {
		t.Fatalf("skipped_live = %v", fp.SkippedLive)
	}
	if len(fp.CallQueues) != 0 || fp.MembershipSource != "skipped" {
		t.Fatalf("no-live still reported memberships: %+v source=%s", fp.CallQueues, fp.MembershipSource)
	}
	if len(fp.DeskPhones) != 1 {
		t.Fatalf("local desk phone missing under --no-live: %+v", fp.DeskPhones)
	}
	if reqs, _ := ts.snapshot(); len(reqs) != 0 {
		t.Fatalf("--no-live made API calls: %v", reqs)
	}
}

func TestLocalFootprintEmptyAndMissingStore(t *testing.T) {
	localTestEnv(t)
	ts := localNewTestServer(t)
	cases := []struct {
		name string
		db   string
	}{
		{"store without users", localTestStore(t, map[string][]string{"numbers": localTestNumbers})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := localRun(t, newNovelFootprintCmd, "jane@example.com", "--db", tc.db)
			if err != nil {
				t.Fatalf("want exit 0 on empty store, got %v", err)
			}
			var fp localFootprint
			localDecode(t, stdout, &fp)
			// Only numbers is synced, so nothing could be assessed.
			if fp.User != nil || len(fp.CallQueues) != 0 || len(fp.PhoneNumbers) != 0 || fp.CallQueues == nil || fp.Assessed {
				t.Fatalf("empty store fabricated data: %+v", fp)
			}
			if !strings.Contains(stderr+fp.Note, "sync --full --resources users,") {
				t.Fatalf("missing sync hint; stderr=%q note=%q", stderr, fp.Note)
			}
		})
	}
	if reqs, _ := ts.snapshot(); len(reqs) != 0 {
		t.Fatalf("empty store made API calls: %v", reqs)
	}
}

func TestLocalFootprintNotFoundAndAmbiguous(t *testing.T) {
	localTestEnv(t)
	localNewTestServer(t)
	db := localTestStore(t, localTestFootprintRows())

	_, _, err := localRun(t, newNovelFootprintCmd, "nobody@example.com", "--db", db)
	if code := localExitCode(err); code != 3 {
		t.Fatalf("unknown user exit = %d (%v), want 3", code, err)
	}
	_, _, err = localRun(t, newNovelFootprintCmd, "example.com", "--db", db)
	if code := localExitCode(err); code != 2 || !strings.Contains(err.Error(), "matches 3 phone users") {
		t.Fatalf("ambiguous query exit = %d (%v), want 2 listing candidates", code, err)
	}
}

func TestLocalFootprintRecordsFetchFailures(t *testing.T) {
	localTestEnv(t)
	ts := localNewTestServer(t)
	ts.getStatus["/phone/call_queues/cq-support/members"] = 400
	db := localTestStore(t, localTestFootprintRows())

	stdout, stderr, err := localRun(t, newNovelFootprintCmd, "jane@example.com", "--db", db)
	if err != nil {
		t.Fatalf("partial failure should still exit 0: %v", err)
	}
	var fp localFootprint
	localDecode(t, stdout, &fp)
	if len(fp.FetchFailures) != 1 || fp.FetchFailures[0].GroupID != "cq-support" {
		t.Fatalf("fetch_failures = %+v, want cq-support", fp.FetchFailures)
	}
	if got := strings.Join(localMembershipNames(fp.CallQueues), ","); got != "Sales" {
		t.Fatalf("call_queues = %q, want Sales only (Support failed)", got)
	}
	if !strings.Contains(stderr, "warning: could not read call_queues") {
		t.Fatalf("stderr missing warning: %q", stderr)
	}
}

func TestLocalFootprintListsGroupsLiveWhenNotSynced(t *testing.T) {
	localTestEnv(t)
	ts := localNewTestServer(t)
	rows := localTestFootprintRows()
	delete(rows, "call_queues")
	db := localTestStore(t, rows)

	stdout, _, err := localRun(t, newNovelFootprintCmd, "jane@example.com", "--db", db)
	if err != nil {
		t.Fatalf("footprint: %v", err)
	}
	var fp localFootprint
	localDecode(t, stdout, &fp)
	reqs, _ := ts.snapshot()
	if !strings.Contains(strings.Join(reqs, "\n"), "GET /phone/call_queues?") {
		t.Fatalf("call queue list not fetched live: %v", reqs)
	}
	var scan *localFamilyScan
	for i := range fp.MembershipScan {
		if fp.MembershipScan[i].Family == "call_queues" {
			scan = &fp.MembershipScan[i]
		}
	}
	if scan == nil || scan.ListSource != "live" || scan.Groups != 3 || scan.Scanned != 3 {
		t.Fatalf("call_queues scan = %+v, want live list of 3 groups", scan)
	}
	if got := strings.Join(localMembershipNames(fp.CallQueues), ","); got != "Sales,Support" {
		t.Fatalf("call_queues from live list = %q, want Sales,Support", got)
	}
	if len(fp.FetchFailures) != 0 {
		t.Fatalf("unexpected failures: %+v", fp.FetchFailures)
	}
}

func TestLocalOffboardPlanOrdersCallingPlanLast(t *testing.T) {
	localTestEnv(t)
	ts := localNewTestServer(t)
	db := localTestStore(t, localTestFootprintRows())

	stdout, _, err := localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db)
	if err != nil {
		t.Fatalf("offboard plan: %v", err)
	}
	var res localOffboardResult
	localDecode(t, stdout, &res)
	if res.Mode != "plan" || len(res.Receipts) != 0 {
		t.Fatalf("mode=%s receipts=%v, want plan with no receipts", res.Mode, res.Receipts)
	}
	wantPaths := []string{
		"/phone/call_queues/cq-sales/members/u-jane",
		"/phone/call_queues/cq-support/members/u-jane",
		"/phone/monitoring_groups/mg-1/monitor_members/ext-jane",
		"/phone/devices/dev-jane/extensions/ext-jane",
		"/phone/users/u-jane/phone_numbers/pn-jane",
		"/phone/users/u-jane/calling_plans/200",
	}
	if len(res.Steps) != len(wantPaths) {
		t.Fatalf("steps = %+v, want %d", res.Steps, len(wantPaths))
	}
	for i, s := range res.Steps {
		if s.Path != wantPaths[i] || s.Method != "DELETE" || s.Step != i+1 {
			t.Fatalf("step %d = %s %s, want DELETE %s", i+1, s.Method, s.Path, wantPaths[i])
		}
	}
	if last := res.Steps[len(res.Steps)-1]; last.Family != "calling_plans" {
		t.Fatalf("last step family = %s, want calling_plans", last.Family)
	}
	if res.Steps[2].Query["member_type"] != "monitor" {
		t.Fatalf("monitoring step query = %v, want member_type=monitor", res.Steps[2].Query)
	}
	families := map[string]bool{}
	for _, nc := range res.NotCovered {
		families[nc.Family] = true
	}
	for _, f := range []string{"push_to_talk", "private_directory", "roles"} {
		if !families[f] {
			t.Fatalf("not_covered missing %s: %+v", f, res.NotCovered)
		}
	}
	if _, deletes := ts.snapshot(); len(deletes) != 0 {
		t.Fatalf("plan mode sent DELETEs: %v", deletes)
	}
}

func TestLocalOffboardApplyRecordsDeletesAndReceipts(t *testing.T) {
	localTestEnv(t)
	ts := localNewTestServer(t)
	ts.deleteStatus["/phone/call_queues/cq-support/members/u-jane"] = 404
	db := localTestStore(t, localTestFootprintRows())

	stdout, stderr, err := localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db, "--apply")
	if err != nil {
		t.Fatalf("offboard --apply: %v\nstderr: %s", err, stderr)
	}
	var res localOffboardResult
	localDecode(t, stdout, &res)
	_, deletes := ts.snapshot()
	want := []string{
		"/phone/call_queues/cq-sales/members/u-jane",
		"/phone/call_queues/cq-support/members/u-jane",
		"/phone/monitoring_groups/mg-1/monitor_members/ext-jane",
		"/phone/devices/dev-jane/extensions/ext-jane",
		"/phone/users/u-jane/phone_numbers/pn-jane",
		"/phone/users/u-jane/calling_plans/200",
	}
	if strings.Join(deletes, "\n") != strings.Join(want, "\n") {
		t.Fatalf("DELETE paths:\n%s\nwant:\n%s", strings.Join(deletes, "\n"), strings.Join(want, "\n"))
	}
	reqs, _ := ts.snapshot()
	if !strings.Contains(strings.Join(reqs, "\n"), "DELETE /phone/monitoring_groups/mg-1/monitor_members/ext-jane?member_type=monitor") {
		t.Fatalf("monitoring DELETE missing member_type query: %v", reqs)
	}
	if len(res.Receipts) != 6 {
		t.Fatalf("receipts = %+v", res.Receipts)
	}
	if r := res.Receipts[1]; r.Status != "already_removed" || r.HTTPStatus != 404 {
		t.Fatalf("404 receipt = %+v, want already_removed", r)
	}
	for i, r := range res.Receipts {
		if i != 1 && r.Status != "removed" {
			t.Fatalf("receipt %d = %+v, want removed", i, r)
		}
	}
	if res.Summary.Removed != 5 || res.Summary.AlreadyRemoved != 1 || res.Summary.Failed != 0 || res.Mode != "apply" {
		t.Fatalf("summary = %+v mode=%s", res.Summary, res.Mode)
	}
}

func TestLocalOffboardApplyContinuesPastFailure(t *testing.T) {
	localTestEnv(t)
	ts := localNewTestServer(t)
	ts.deleteStatus["/phone/devices/dev-jane/extensions/ext-jane"] = 400
	db := localTestStore(t, localTestFootprintRows())

	stdout, _, err := localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db, "--apply")
	if code := localExitCode(err); code != 6 {
		t.Fatalf("exit = %d (%v), want 6 (partial failure) when one step fails", code, err)
	}
	var res localOffboardResult
	localDecode(t, stdout, &res)
	if res.Summary.Failed != 1 || res.Summary.Removed != 5 {
		t.Fatalf("summary = %+v, want 1 failed 5 removed", res.Summary)
	}
	if r := res.Receipts[3]; r.Status != "failed" || r.HTTPStatus != 400 || r.Error == "" {
		t.Fatalf("device receipt = %+v, want failed 400", r)
	}
	_, deletes := ts.snapshot()
	if len(deletes) != 6 || deletes[5] != "/phone/users/u-jane/calling_plans/200" {
		t.Fatalf("later steps did not run after the failure: %v", deletes)
	}
}

func TestLocalOffboardApplyRefusedUnderHarnessAndNoLive(t *testing.T) {
	localTestEnv(t)
	ts := localNewTestServer(t)
	db := localTestStore(t, localTestFootprintRows())

	_, _, err := localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db, "--apply", "--no-live")
	if code := localExitCode(err); code != 2 {
		t.Fatalf("--apply --no-live exit = %d (%v), want 2", code, err)
	}

	t.Setenv("PRINTING_PRESS_DOGFOOD", "1")
	stdout, _, err := localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db, "--apply")
	if err != nil {
		t.Fatalf("harness refusal should exit 0: %v", err)
	}
	var refusal harnessRefusalResult
	localDecode(t, stdout, &refusal)
	if !refusal.Refused || refusal.Harness != "dogfood" {
		t.Fatalf("refusal = %+v", refusal)
	}
	if reqs, _ := ts.snapshot(); len(reqs) != 0 {
		t.Fatalf("harness --apply made API calls: %v", reqs)
	}
}

func TestLocalMatchUsers(t *testing.T) {
	users := []localUser{
		{ID: "u1", Email: "jane@example.com", Name: "Jane Doe", ExtensionNumber: "1001", PhoneNumbers: []localPhoneNumberRef{{Number: "+14155550123"}}},
		{ID: "u2", Email: "janet@example.com", Name: "Janet Roe", ExtensionNumber: "1002"},
	}
	cases := []struct {
		q, want, kind string
	}{
		{"u2", "u2", "id"},
		{"JANE@example.com", "u1", "email"},
		{"1002", "u2", "extension"},
		{"+14155550123", "u1", "phone_number"},
		{"14155550123", "u1", "phone_number"},
		{"4155550123", "u1", "phone_number"},
		{"(415) 555-0123", "u1", "phone_number"},
		{"+1 415 555 0123", "u1", "phone_number"},
		{"415.555.0123", "u1", "phone_number"},
		{"jane doe", "u1", "name"},
		{"roe", "u2", "name"},
		{"jan", "u1,u2", "name"},
		{"nobody", "", ""},
		{"555-0199", "", ""},
	}
	for _, tc := range cases {
		got := []string{}
		matches, kind := localMatchUsers(users, tc.q)
		for _, u := range matches {
			got = append(got, u.ID.String())
		}
		if strings.Join(got, ",") != tc.want || kind != tc.kind {
			t.Errorf("localMatchUsers(%q) = %v by %q, want %s by %q", tc.q, got, kind, tc.want, tc.kind)
		}
		if exact := localMatchIsExact(kind); exact != (kind == "id" || kind == "email" || kind == "extension") {
			t.Errorf("localMatchIsExact(%q) = %v", kind, exact)
		}
	}
}
