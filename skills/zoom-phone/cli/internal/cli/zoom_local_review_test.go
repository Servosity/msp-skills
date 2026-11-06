// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Review round 1 regressions for the slice-A ("local") commands.

package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"zoom-phone-pp-cli/internal/store"
)

// ---- item 1: --apply refuses on incomplete discovery ----

func TestLocalOffboardApplyRefusesIncompleteDiscovery(t *testing.T) {
	localTestEnv(t)

	t.Run("membership fetch failed", func(t *testing.T) {
		ts := localNewTestServer(t)
		ts.getStatus["/phone/call_queues/cq-support/members"] = 400
		db := localTestStore(t, localTestFootprintRows())
		stdout, _, err := localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db, "--apply")
		if code := localExitCode(err); code != 5 || !strings.Contains(err.Error(), "no DELETE sent") {
			t.Fatalf("exit = %d (%v), want 5 refusal", code, err)
		}
		var res localOffboardResult
		localDecode(t, stdout, &res)
		if !res.Refused || len(res.Receipts) != 0 || len(res.RefusalReasons) != 1 || !strings.Contains(res.RefusalReasons[0], "cq-support") {
			t.Fatalf("refused=%v receipts=%v reasons=%v", res.Refused, res.Receipts, res.RefusalReasons)
		}
		if len(res.Steps) == 0 {
			t.Fatalf("refusal should still show the plan")
		}
		if _, deletes := ts.snapshot(); len(deletes) != 0 {
			t.Fatalf("refused --apply sent DELETEs: %v", deletes)
		}
	})

	t.Run("plan dependency not synced", func(t *testing.T) {
		ts := localNewTestServer(t)
		rows := localTestFootprintRows()
		delete(rows, "numbers")
		db := localTestStore(t, rows)
		stdout, _, err := localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db, "--apply")
		if code := localExitCode(err); code != 5 {
			t.Fatalf("exit = %d (%v), want 5", code, err)
		}
		var res localOffboardResult
		localDecode(t, stdout, &res)
		if !res.Refused || len(res.RefusalReasons) != 1 || !strings.Contains(res.RefusalReasons[0], "not synced: numbers") {
			t.Fatalf("reasons = %v", res.RefusalReasons)
		}
		if _, deletes := ts.snapshot(); len(deletes) != 0 {
			t.Fatalf("refused --apply sent DELETEs: %v", deletes)
		}
	})

	t.Run("no local store", func(t *testing.T) {
		ts := localNewTestServer(t)
		localRequireNoDefaultStore(t)
		stdout, _, err := localRun(t, newNovelOffboardCmd, "jane@example.com", "--apply")
		if code := localExitCode(err); code != 5 {
			t.Fatalf("exit = %d (%v), want 5", code, err)
		}
		var res localOffboardResult
		localDecode(t, stdout, &res)
		if !res.Refused || res.Assessed || len(res.RefusalReasons) == 0 {
			t.Fatalf("no-store apply = %+v", res)
		}
		if reqs, _ := ts.snapshot(); len(reqs) != 0 {
			t.Fatalf("no-store --apply made API calls: %v", reqs)
		}
	})
}

// ---- item 2: --apply needs an exact id, email or extension ----

func TestLocalOffboardApplyRequiresExactMatch(t *testing.T) {
	localTestEnv(t)
	db := localTestStore(t, localTestFootprintRows())

	for _, q := range []string{"Jane Doe", "jane", "+14155550123", "(415) 555-0123"} {
		ts := localNewTestServer(t)
		_, _, err := localRun(t, newNovelOffboardCmd, q, "--db", db, "--apply")
		if code := localExitCode(err); code != 2 || !strings.Contains(err.Error(), "exact user id, email or extension") {
			t.Fatalf("--apply %q exit = %d (%v), want 2 refusing a fuzzy match", q, code, err)
		}
		if reqs, _ := ts.snapshot(); len(reqs) != 0 {
			t.Fatalf("--apply %q made API calls: %v", q, reqs)
		}
	}

	// Plan mode keeps fuzzy matching.
	localNewTestServer(t)
	stdout, _, err := localRun(t, newNovelOffboardCmd, "jane", "--db", db)
	if err != nil {
		t.Fatalf("plan with substring: %v", err)
	}
	var plan localOffboardResult
	localDecode(t, stdout, &plan)
	if plan.MatchedBy != "name" || plan.User == nil || plan.User.ID != "u-jane" || len(plan.Steps) != 6 {
		t.Fatalf("fuzzy plan = matched_by %q user %+v steps %d", plan.MatchedBy, plan.User, len(plan.Steps))
	}
	// Ambiguous still exits 2 in plan mode.
	if _, _, err := localRun(t, newNovelOffboardCmd, "example.com", "--db", db); localExitCode(err) != 2 {
		t.Fatalf("ambiguous plan exit = %d (%v), want 2", localExitCode(err), err)
	}

	// Exact extension under --apply proceeds and echoes the user on receipts.
	ts := localNewTestServer(t)
	stdout, _, err = localRun(t, newNovelOffboardCmd, "1001", "--db", db, "--apply")
	if err != nil {
		t.Fatalf("--apply 1001: %v", err)
	}
	var res localOffboardResult
	localDecode(t, stdout, &res)
	if res.MatchedBy != "extension" || res.User.ID != "u-jane" || res.User.Email != "jane@example.com" || res.User.ExtensionNumber != "1001" {
		t.Fatalf("resolved user = %+v by %q", res.User, res.MatchedBy)
	}
	for _, r := range res.Receipts {
		if r.UserID != "u-jane" || r.UserEmail != "jane@example.com" || r.UserExtension != "1001" {
			t.Fatalf("receipt does not echo the user: %+v", r)
		}
	}
	if _, deletes := ts.snapshot(); len(deletes) != 6 {
		t.Fatalf("deletes = %v, want 6", deletes)
	}
}

// ---- item 3: exit codes and already-removed classification ----

func TestLocalDeleteAlreadyGone(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   bool
	}{
		{404, `{"code":1001,"message":"User does not exist: u-jane."}`, true},
		{404, ``, true},
		{404, `{"code":2001,"message":"Account does not exist."}`, false},
		{400, `{"code":404,"message":"Member does not exist."}`, true},
		{400, `{"code":404}`, true}, // in-body code alone marks a gone member/group
		{400, `{"code":404,"message":"The group does not exist."}`, true},
		{400, `{"code":300,"message":"Validation Failed. The group does not exist, groupId:cq-1"}`, true},
		{400, `{"code":300,"message":"The extension is not assigned to this device."}`, true},
		{400, `{"code":400,"message":"Unable to delete manager"}`, false},
		{400, `{"code":300,"message":"Invalid user id."}`, false},
		{400, `{"code":300,"message":"Validation Failed."}`, false},
		{429, `{"code":429,"message":"not found"}`, false},
		{500, `{"code":500,"message":"does not exist"}`, false},
	}
	for _, tc := range cases {
		if got := localDeleteAlreadyGone(tc.status, tc.body); got != tc.want {
			t.Errorf("localDeleteAlreadyGone(%d, %s) = %v, want %v", tc.status, tc.body, got, tc.want)
		}
	}
}

func TestLocalOffboardApplyExitCodes(t *testing.T) {
	localTestEnv(t)
	db := localTestStore(t, localTestFootprintRows())
	paths := []string{
		"/phone/call_queues/cq-sales/members/u-jane",
		"/phone/call_queues/cq-support/members/u-jane",
		"/phone/monitoring_groups/mg-1/monitor_members/ext-jane",
		"/phone/devices/dev-jane/extensions/ext-jane",
		"/phone/users/u-jane/phone_numbers/pn-jane",
		"/phone/users/u-jane/calling_plans/200",
	}

	// A re-run: 400 "not assigned" and 400 code 404 converge to already_removed.
	ts := localNewTestServer(t)
	ts.deleteStatus[paths[3]] = 400
	ts.deleteBody[paths[3]] = `{"code":300,"message":"The extension is not assigned to this device."}`
	ts.deleteStatus[paths[2]] = 400
	ts.deleteBody[paths[2]] = `{"code":404,"message":"Member does not exist."}`
	stdout, _, err := localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db, "--apply")
	if err != nil {
		t.Fatalf("re-run exit = %d (%v), want 0", localExitCode(err), err)
	}
	var res localOffboardResult
	localDecode(t, stdout, &res)
	if res.Summary.AlreadyRemoved != 2 || res.Summary.Removed != 4 || res.Receipts[3].Status != "already_removed" || res.Receipts[2].Status != "already_removed" {
		t.Fatalf("summary = %+v receipts=%+v", res.Summary, res.Receipts)
	}

	// Every step failing is exit 5, not 6.
	ts = localNewTestServer(t)
	for _, p := range paths {
		ts.deleteStatus[p] = 400
		ts.deleteBody[p] = `{"code":300,"message":"Validation Failed."}`
	}
	stdout, _, err = localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db, "--apply")
	if code := localExitCode(err); code != 5 {
		t.Fatalf("all-failed exit = %d (%v), want 5", code, err)
	}
	res = localOffboardResult{}
	localDecode(t, stdout, &res)
	if res.Summary.Failed != 6 {
		t.Fatalf("summary = %+v", res.Summary)
	}
}

// ---- item 4: assessed / missing_local / explicit --db ----

// localRequireNoDefaultStore proves the sandbox has no store at the
// default path, so "no --db" exercises the dogfood happy path.
func localRequireNoDefaultStore(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(defaultDBPath(localCLIName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sandbox has a default store at %s (err=%v)", defaultDBPath(localCLIName), err)
	}
}

func TestLocalNoStoreIsUnassessed(t *testing.T) {
	localTestEnv(t)
	localNewTestServer(t)
	localRequireNoDefaultStore(t)
	sorted := func(in []string) string {
		c := append([]string(nil), in...)
		sort.Strings(c)
		return strings.Join(c, ",")
	}
	cases := []struct {
		name    string
		newCmd  func(*rootFlags) *cobra.Command
		args    []string
		missing []string
		decode  func(t *testing.T, out string) (assessed bool, missing []string, skipped []string)
	}{
		{"footprint", newNovelFootprintCmd, []string{"jane@example.com"}, localFootprintSyncResources, func(t *testing.T, out string) (bool, []string, []string) {
			var r localFootprint
			localDecode(t, out, &r)
			return r.Assessed, r.MissingLocal, nil
		}},
		{"offboard", newNovelOffboardCmd, []string{"jane@example.com"}, localFootprintSyncResources, func(t *testing.T, out string) (bool, []string, []string) {
			var r localOffboardResult
			localDecode(t, out, &r)
			return r.Assessed, r.MissingLocal, nil
		}},
		{"e911 audit", newNovelE911AuditCmd, nil, localE911SyncResources, func(t *testing.T, out string) (bool, []string, []string) {
			var r localE911Result
			localDecode(t, out, &r)
			return r.Assessed, r.MissingLocal, r.SkippedChecks
		}},
		{"numbers inventory", newNovelNumbersInventoryCmd, nil, localInventorySyncResources, func(t *testing.T, out string) (bool, []string, []string) {
			var r localInventoryResult
			localDecode(t, out, &r)
			return r.Assessed, r.MissingLocal, nil
		}},
		{"devices health", newNovelDevicesHealthCmd, nil, localDevicesSyncResources, func(t *testing.T, out string) (bool, []string, []string) {
			var r localDevicesHealthResult
			localDecode(t, out, &r)
			return r.Assessed, r.MissingLocal, nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := localRun(t, tc.newCmd, tc.args...)
			if err != nil {
				t.Fatalf("default path without a store: exit %d (%v), want 0", localExitCode(err), err)
			}
			assessed, missing, skipped := tc.decode(t, stdout)
			if assessed || sorted(missing) != sorted(tc.missing) {
				t.Fatalf("assessed=%v missing=%v, want false and %v", assessed, missing, tc.missing)
			}
			if tc.name == "e911 audit" {
				if len(skipped) != len(localE911Checks) {
					t.Fatalf("skipped_checks = %v, want every check", skipped)
				}
				for i, c := range localE911Checks {
					if !strings.HasPrefix(skipped[i], c+":") {
						t.Fatalf("skipped_checks[%d] = %q, want %s", i, skipped[i], c)
					}
				}
			}
			if !strings.Contains(stderr, "no local mirror at") {
				t.Fatalf("stderr = %q", stderr)
			}

			missingPath := filepath.Join(t.TempDir(), "typo.db")
			_, _, err = localRun(t, tc.newCmd, append(append([]string{}, tc.args...), "--db", missingPath)...)
			if code := localExitCode(err); code != 2 || !strings.Contains(err.Error(), missingPath) {
				t.Fatalf("explicit missing --db exit = %d (%v), want 2 naming the path", code, err)
			}
		})
	}

	// A store where every resource any of these commands reads finished
	// syncing (empty lists still count as synced) is assessed: true.
	db := localTestStore(t, localTestCompleteRows())
	for _, tc := range cases {
		args := append(append([]string{}, tc.args...), "--db", db, "--no-live")
		if tc.name != "footprint" && tc.name != "offboard" {
			args = []string{"--db", db}
		}
		stdout, _, err := localRun(t, tc.newCmd, args...)
		if err != nil {
			t.Fatalf("%s with store: %v", tc.name, err)
		}
		if assessed, missing, _ := tc.decode(t, stdout); !assessed || len(missing) != 0 {
			t.Fatalf("%s with a fully synced store: assessed=%v missing=%v", tc.name, assessed, missing)
		}
	}
}

// localTestCompleteRows covers every resource the five commands read.
func localTestCompleteRows() map[string][]string {
	rows := localTestFootprintRows()
	rows["sites"] = []string{}
	rows["common_areas"] = []string{}
	rows["locations"] = []string{}
	return rows
}

// localTestAssessCases runs each command and returns (assessed, missing_local).
func localTestAssessCases() []struct {
	name   string
	newCmd func(*rootFlags) *cobra.Command
	args   []string
	need   []string
	decode func(t *testing.T, out string) (bool, []string)
} {
	return []struct {
		name   string
		newCmd func(*rootFlags) *cobra.Command
		args   []string
		need   []string
		decode func(t *testing.T, out string) (bool, []string)
	}{
		{"footprint", newNovelFootprintCmd, []string{"jane@example.com", "--no-live"}, localFootprintSyncResources, func(t *testing.T, out string) (bool, []string) {
			var r localFootprint
			localDecode(t, out, &r)
			return r.Assessed, r.MissingLocal
		}},
		{"offboard", newNovelOffboardCmd, []string{"jane@example.com", "--no-live"}, localFootprintSyncResources, func(t *testing.T, out string) (bool, []string) {
			var r localOffboardResult
			localDecode(t, out, &r)
			return r.Assessed, r.MissingLocal
		}},
		{"e911 audit", newNovelE911AuditCmd, nil, localE911SyncResources, func(t *testing.T, out string) (bool, []string) {
			var r localE911Result
			localDecode(t, out, &r)
			return r.Assessed, r.MissingLocal
		}},
		{"numbers inventory", newNovelNumbersInventoryCmd, nil, localInventorySyncResources, func(t *testing.T, out string) (bool, []string) {
			var r localInventoryResult
			localDecode(t, out, &r)
			return r.Assessed, r.MissingLocal
		}},
		{"devices health", newNovelDevicesHealthCmd, nil, localDevicesSyncResources, func(t *testing.T, out string) (bool, []string) {
			var r localDevicesHealthResult
			localDecode(t, out, &r)
			return r.Assessed, r.MissingLocal
		}},
	}
}

func TestLocalEmptyOrPartialStoreIsUnassessed(t *testing.T) {
	localTestEnv(t)
	localNewTestServer(t)
	sorted := func(in []string) string {
		c := append([]string(nil), in...)
		sort.Strings(c)
		return strings.Join(c, ",")
	}

	// An empty default store: the file exists, nothing was ever synced.
	localRequireNoDefaultStore(t)
	empty, err := store.Open(defaultDBPath(localCLIName))
	if err != nil {
		t.Fatal(err)
	}
	if err := empty.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range localTestAssessCases() {
		stdout, _, err := localRun(t, tc.newCmd, tc.args...)
		if err != nil {
			t.Fatalf("%s on an empty default store: exit %d (%v), want 0", tc.name, localExitCode(err), err)
		}
		assessed, missing := tc.decode(t, stdout)
		if assessed || sorted(missing) != sorted(tc.need) {
			t.Fatalf("%s on an empty store: assessed=%v missing=%v, want false and %v", tc.name, assessed, missing, tc.need)
		}
	}

	// A partial sync: everything except users is synced.
	rows := localTestCompleteRows()
	delete(rows, "users")
	db := localTestStore(t, rows)
	for _, tc := range localTestAssessCases() {
		args := append(append([]string{}, tc.args...), "--db", db)
		stdout, _, err := localRun(t, tc.newCmd, args...)
		if err != nil {
			t.Fatalf("%s partial: %v", tc.name, err)
		}
		if assessed, missing := tc.decode(t, stdout); assessed || sorted(missing) != "users" {
			t.Fatalf("%s with users unsynced: assessed=%v missing=%v, want false and [users]", tc.name, assessed, missing)
		}
	}

	// Users synced (footprint resolves Jane and builds) but
	// emergency_addresses not: only commands that read it are unassessed.
	rows = localTestCompleteRows()
	delete(rows, "emergency_addresses")
	db = localTestStore(t, rows)
	for _, tc := range localTestAssessCases() {
		args := append(append([]string{}, tc.args...), "--db", db)
		stdout, _, err := localRun(t, tc.newCmd, args...)
		if err != nil {
			t.Fatalf("%s partial: %v", tc.name, err)
		}
		reads := false
		for _, r := range tc.need {
			if r == "emergency_addresses" {
				reads = true
			}
		}
		assessed, missing := tc.decode(t, stdout)
		want := ""
		if reads {
			want = "emergency_addresses"
		}
		if assessed == reads || sorted(missing) != want {
			t.Fatalf("%s with emergency_addresses unsynced: assessed=%v missing=%v, want assessed=%v missing=[%s]", tc.name, assessed, missing, !reads, want)
		}
	}
}

// ---- item 5: truthful meta.source ----

func TestLocalFootprintMetaSourceIsTruthful(t *testing.T) {
	localTestEnv(t)
	localNewTestServer(t)
	db := localTestStore(t, localTestFootprintRows())
	source := func(args ...string) string {
		stdout, _, err := localRunWith(t, &rootFlags{asJSON: true, agent: true}, newNovelFootprintCmd, args...)
		if err != nil {
			t.Fatalf("footprint %v: %v", args, err)
		}
		var env struct {
			Meta struct {
				Source string `json:"source"`
			} `json:"meta"`
		}
		if err := json.Unmarshal([]byte(stdout), &env); err != nil {
			t.Fatalf("decode agent envelope: %v\n%s", err, stdout)
		}
		return env.Meta.Source
	}
	if got := source("jane@example.com", "--db", db); got != "live" {
		t.Fatalf("live scan meta.source = %q, want live", got)
	}
	if got := source("jane@example.com", "--db", db, "--no-live"); got != "local" {
		t.Fatalf("--no-live meta.source = %q, want local", got)
	}
	if got := source("jane@example.com"); got != "local" {
		t.Fatalf("no-store meta.source = %q, want local", got)
	}
}

// ---- item 6: devices health counts shape ----

func TestLocalDevicesHealthCountsShape(t *testing.T) {
	localTestEnv(t)
	healthy := localTestStore(t, map[string][]string{
		"assigned": localTestAssignedDevices[:2], "unassigned": {}, "users": localTestUsers, "common_areas": {},
	})
	stdout, _, err := localRun(t, newNovelDevicesHealthCmd, "--db", healthy)
	if err != nil {
		t.Fatalf("devices health: %v", err)
	}
	var res localDevicesHealthResult
	localDecode(t, stdout, &res)
	want := []string{"ASSIGNEE_INACTIVE", "ASSIGNMENT_AMBIGUOUS", "FIRMWARE_DRIFT", "NO_TEMPLATE", "OFFLINE", "UNASSIGNED", "total"}
	keys := make([]string, 0, len(res.Counts))
	for k, v := range res.Counts {
		keys = append(keys, k)
		if v != 0 {
			t.Fatalf("healthy fleet count %s = %d, want 0", k, v)
		}
	}
	sort.Strings(keys)
	if strings.Join(keys, ",") != strings.Join(want, ",") || res.DevicesChecked != 2 || !res.Assessed {
		t.Fatalf("counts keys = %v checked=%d assessed=%v, want %v", keys, res.DevicesChecked, res.Assessed, want)
	}

	flagged := localTestStore(t, map[string][]string{
		"assigned": localTestAssignedDevices, "unassigned": localTestUnassignedDevices, "users": localTestUsers, "common_areas": {},
	})
	stdout, _, err = localRun(t, newNovelDevicesHealthCmd, "--db", flagged)
	if err != nil {
		t.Fatalf("devices health: %v", err)
	}
	res = localDevicesHealthResult{}
	localDecode(t, stdout, &res)
	// dev-carl, dev-spare and dev-lobby are flagged; 5 devices checked.
	if res.Counts["total"] != 3 || res.Counts["OFFLINE"] != 2 || res.Counts["UNASSIGNED"] != 1 || res.Counts["NO_TEMPLATE"] != 1 || res.DevicesChecked != 5 {
		t.Fatalf("counts = %v checked=%d", res.Counts, res.DevicesChecked)
	}
	for _, sh := range res.BySite {
		if _, ok := sh.Counts["FIRMWARE_DRIFT"]; !ok || sh.Counts["total"] != len(sh.Findings) {
			t.Fatalf("site %s counts = %v findings=%d", sh.Site, sh.Counts, len(sh.Findings))
		}
	}
}

// ---- item 7: phone-number lookups ----

func TestLocalFootprintPhoneNumberFormats(t *testing.T) {
	localTestEnv(t)
	localNewTestServer(t)
	db := localTestStore(t, localTestFootprintRows())
	for _, q := range []string{"4155550123", "(415) 555-0123", "+1 415 555 0123", "+14155550123"} {
		stdout, _, err := localRun(t, newNovelFootprintCmd, q, "--db", db, "--no-live")
		if err != nil {
			t.Fatalf("footprint %q: %v", q, err)
		}
		var fp localFootprint
		localDecode(t, stdout, &fp)
		if fp.User == nil || fp.User.ID != "u-jane" || fp.MatchedBy != "phone_number" {
			t.Fatalf("footprint %q resolved %+v by %q, want u-jane by phone_number", q, fp.User, fp.MatchedBy)
		}
	}
}

// ---- item 8: only completed syncs count ----

func TestLocalSyncedSetNeedsCompletedSync(t *testing.T) {
	localTestEnv(t)
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	addr := []json.RawMessage{json.RawMessage(localTestAddresses[0])}
	if _, _, err := db.UpsertBatch("emergency_addresses", addr); err != nil {
		t.Fatal(err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.SaveSyncState("users", "", 3)) // completed
	localTestRecordFullScope(t, db, "users")
	must(db.SaveSyncState("sites", "", 0)) // completed, zero rows
	localTestRecordFullScope(t, db, "sites")
	must(db.SaveSyncProgress("emergency_addresses", "c", 1)) // interrupted: rows but incomplete
	must(db.SaveSyncCursor("numbers", "c"))                  // zero-time checkpoint (sync --full start)
	must(db.SaveSyncState("locations", "", 1))
	must(db.SaveSyncCursor("locations", "next")) // a later attempt started and did not finish
	must(db.Close())

	ro, err := store.OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	synced := localSyncedSet(ro.DB(), "users", "sites", "emergency_addresses", "numbers", "locations", "devices")
	_ = ro.Close()
	got := []string{}
	for k, v := range synced {
		if v {
			got = append(got, k)
		}
	}
	sort.Strings(got)
	if strings.Join(got, ",") != "sites,users" {
		t.Fatalf("synced = %v, want only sites,users", got)
	}

	// The audit treats the interrupted emergency_addresses as unsynced: no
	// address findings are fabricated from partial rows.
	stdout, _, err := localRun(t, newNovelE911AuditCmd, "--db", path)
	if err != nil {
		t.Fatalf("e911 audit: %v", err)
	}
	var res localE911Result
	localDecode(t, stdout, &res)
	if !strings.Contains(strings.Join(res.MissingLocal, ","), "emergency_addresses") || !strings.Contains(strings.Join(res.SkippedChecks, "\n"), "emergency_addresses not synced") {
		t.Fatalf("missing=%v skipped=%v", res.MissingLocal, res.SkippedChecks)
	}
}

// ---- item 9: the first API call is sequential ----

func TestLocalMembershipScanFirstCallIsSequential(t *testing.T) {
	localTestEnv(t)
	ts := localNewTestServer(t)
	ts.firstDelay = 150 * time.Millisecond
	// Every group list is synced, so the first request is a member fetch.
	db := localTestStore(t, localTestFootprintRows())
	if _, _, err := localRun(t, newNovelFootprintCmd, "jane@example.com", "--db", db); err != nil {
		t.Fatalf("footprint: %v", err)
	}
	reqs, _ := ts.snapshot()
	ts.mu.Lock()
	overlap := ts.firstOverlap
	ts.mu.Unlock()
	if len(reqs) < 3 {
		t.Fatalf("expected a fan-out, got %v", reqs)
	}
	if overlap {
		t.Fatalf("another request started while the first was in flight; the first call must be sequential")
	}
}
