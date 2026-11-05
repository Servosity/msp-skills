// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Sync-filter coverage (Codex round 3) for the slice-A ("local") commands.

package cli

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"zoom-phone-pp-cli/internal/store"
)

// localScopeRows reads zoom_sync_scope as resource -> "filtered|full|params".
func localScopeRows(t *testing.T, path string) map[string]string {
	t.Helper()
	db, err := store.OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	out := map[string]string{}
	for r, row := range localReadSyncScope(db.DB()) {
		f, full := "0", "0"
		if row.filtered {
			f = "1"
		}
		if row.full {
			full = "1"
		}
		out[r] = f + "|" + full + "|" + row.params
	}
	return out
}

// localRunSync runs the real `sync` command through the root (so the scope
// hook wraps it) against the test server.
func localRunSync(t *testing.T, args ...string) {
	t.Helper()
	cmd := RootCmd()
	var out, errb zoomLockedBuffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(append([]string{"sync"}, args...))
	_ = cmd.Execute() // per-resource dependent errors are tolerated; coverage is asserted below
	if strings.Contains(errb.String(), "could not record which resources this sync filtered") {
		t.Fatalf("scope hook failed: %s", errb.String())
	}
}

func TestLocalSyncScopeRecordedByRealSync(t *testing.T) {
	localTestEnv(t)
	jane := `{"id":"u-jane","email":"jane@example.com","name":"Jane Doe","extension_id":"ext-jane","extension_number":1001,"status":"activate","activation_status":"activated","site":{"id":"site-A","name":"A"}}`
	bob := `{"id":"u-bob","email":"bob@example.com","name":"Bob Smith","extension_id":"ext-bob","extension_number":1002,"status":"activate","activation_status":"activated","site":{"id":"site-B","name":"B"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch strings.TrimPrefix(r.URL.Path, "/v2") {
		case "/phone/users":
			if r.URL.Query().Get("site_id") == "site-A" {
				_, _ = w.Write([]byte(`{"users":[` + jane + `],"next_page_token":"","total_records":1}`))
				return
			}
			_, _ = w.Write([]byte(`{"users":[` + jane + `,` + bob + `],"next_page_token":"","total_records":2}`))
		case "/phone/numbers":
			_, _ = w.Write([]byte(`{"phone_numbers":[` +
				`{"id":"pn-jane","number":"+14155550123","source":"internal","status":"available","assignee":{"id":"u-jane","name":"Jane Doe","type":"user"}},` +
				`{"id":"pn-bob","number":"+14155550124","source":"internal","status":"available","assignee":{"id":"u-bob","name":"Bob Smith","type":"user"}}` +
				`],"next_page_token":"","total_records":2}`))
		default:
			_, _ = w.Write([]byte(`{"next_page_token":""}`))
		}
	}))
	defer srv.Close()
	t.Setenv("ZOOM_PHONE_BASE_URL", srv.URL+"/v2")
	db := filepath.Join(t.TempDir(), "data.db")

	// Every number, but only site-A users.
	localRunSync(t, "--full", "--resources", "users,numbers", "--db", db, "--resource-param", "users:site_id=site-A", "--json")
	scope := localScopeRows(t, db)
	if scope["users"] != "1|0|site_id=site-A" || scope["numbers"] != "0|1|" {
		t.Fatalf("scope after a filtered users sync = %v", scope)
	}
	stdout, stderr, err := localRun(t, newNovelNumbersInventoryCmd, "--db", db)
	if err != nil {
		t.Fatalf("numbers inventory: %v", err)
	}
	var inv localInventoryResult
	localDecode(t, stdout, &inv)
	if inv.Assessed || strings.Join(inv.MissingLocal, ",") != "users" || !strings.Contains(inv.MissingReasons["users"], "filtered (site_id=site-A)") {
		t.Fatalf("filtered users: assessed=%v missing=%v reasons=%v", inv.Assessed, inv.MissingLocal, inv.MissingReasons)
	}
	if len(inv.InactiveAssignee) != 0 {
		t.Fatalf("active site-B assignee flagged as not in phone users: %+v", inv.InactiveAssignee)
	}
	if !strings.Contains(stderr, "filtered (site_id=site-A)") {
		t.Fatalf("stderr hint does not explain the filter: %q", stderr)
	}

	// An unfiltered users sync clears the mark.
	localRunSync(t, "--full", "--resources", "users", "--db", db, "--json")
	if scope := localScopeRows(t, db); scope["users"] != "0|1|" {
		t.Fatalf("scope after an unfiltered users sync = %v", scope)
	}
	stdout, _, err = localRun(t, newNovelNumbersInventoryCmd, "--db", db)
	if err != nil {
		t.Fatalf("numbers inventory: %v", err)
	}
	inv = localInventoryResult{}
	localDecode(t, stdout, &inv)
	if !inv.Assessed || len(inv.MissingLocal) != 0 || len(inv.InactiveAssignee) != 0 {
		t.Fatalf("after full sync: assessed=%v missing=%v inactive=%v", inv.Assessed, inv.MissingLocal, inv.InactiveAssignee)
	}
}

// localTestStoreScoped is localTestStore plus recorded scope rows matching
// each resource's current checkpoint (as the sync hook would write them).
func localTestStoreScoped(t *testing.T, rows map[string][]string, filters map[string]string) string {
	t.Helper()
	path := localTestStore(t, rows)
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for rt, params := range filters {
		var ts sql.NullString
		if err := db.DB().QueryRow(`SELECT CAST(last_synced_at AS TEXT) FROM sync_state WHERE resource_type = ?`, rt).Scan(&ts); err != nil {
			t.Fatalf("%s checkpoint: %v", rt, err)
		}
		if err := localWriteSyncScope(db, rt, 1, params, ts.String, "now", true); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestLocalFilteredSyncDoesNotCoverTheAccount(t *testing.T) {
	localTestEnv(t)

	// e911 accepts a type=byoc numbers sync; numbers inventory does not.
	e911Rows := localTestE911Rows()
	byoc := localTestStoreScoped(t, e911Rows, map[string]string{"numbers": "type=byoc"})
	stdout, _, err := localRun(t, newNovelE911AuditCmd, "--db", byoc)
	if err != nil {
		t.Fatalf("e911: %v", err)
	}
	var e localE911Result
	localDecode(t, stdout, &e)
	if !e.Assessed || len(e.MissingLocal) != 0 || e.Counts.ByKind["BYOC_CARRIER_UPDATE_REQUIRED"] != 1 {
		t.Fatalf("e911 with a byoc numbers sync: assessed=%v missing=%v counts=%v", e.Assessed, e.MissingLocal, e.Counts)
	}
	stdout, _, err = localRun(t, newNovelNumbersInventoryCmd, "--db", byoc)
	if err != nil {
		t.Fatalf("inventory: %v", err)
	}
	var inv localInventoryResult
	localDecode(t, stdout, &inv)
	if inv.Assessed || strings.Join(inv.MissingLocal, ",") != "numbers" || !strings.Contains(inv.MissingReasons["numbers"], "filtered (type=byoc)") {
		t.Fatalf("inventory with a byoc numbers sync: assessed=%v missing=%v reasons=%v", inv.Assessed, inv.MissingLocal, inv.MissingReasons)
	}

	// Any other numbers filter does not cover numbers inventory either (e911
	// reads numbers only through its BYOC walk, so it is unaffected).
	site := localTestStoreScoped(t, e911Rows, map[string]string{"numbers": "site_id=site-hq"})
	stdout, _, err = localRun(t, newNovelNumbersInventoryCmd, "--db", site)
	if err != nil {
		t.Fatalf("inventory: %v", err)
	}
	inv = localInventoryResult{}
	localDecode(t, stdout, &inv)
	if inv.Assessed || strings.Join(inv.MissingLocal, ",") != "numbers" || !strings.Contains(inv.MissingReasons["numbers"], "filtered (site_id=site-hq)") {
		t.Fatalf("inventory with a site-filtered numbers sync: assessed=%v missing=%v reasons=%v", inv.Assessed, inv.MissingLocal, inv.MissingReasons)
	}

	// Filtered users: findings for the users that are stored stay true, but
	// the sweep is not account-wide.
	usersFiltered := localTestStoreScoped(t, e911Rows, map[string]string{"users": "site_id=site-hq"})
	stdout, _, err = localRun(t, newNovelE911AuditCmd, "--db", usersFiltered)
	if err != nil {
		t.Fatalf("e911: %v", err)
	}
	e = localE911Result{}
	localDecode(t, stdout, &e)
	if e.Assessed || strings.Join(e.MissingLocal, ",") != "users" || !strings.Contains(e.MissingReasons["users"], "filtered (site_id=site-hq)") ||
		!strings.Contains(strings.Join(e.SkippedChecks, "\n"), "only the 5 stored locally were checked") {
		t.Fatalf("e911 with filtered users: assessed=%v missing=%v reasons=%v skipped=%v", e.Assessed, e.MissingLocal, e.MissingReasons, e.SkippedChecks)
	}

	// Filtered emergency addresses: NO_ADDRESS / ADDRESS_NOT_VERIFIED would
	// be inferred from absence, so none is emitted.
	addrFiltered := localTestStoreScoped(t, e911Rows, map[string]string{"emergency_addresses": "site_id=site-hq"})
	stdout, _, err = localRun(t, newNovelE911AuditCmd, "--db", addrFiltered)
	if err != nil {
		t.Fatalf("e911: %v", err)
	}
	e = localE911Result{}
	localDecode(t, stdout, &e)
	for _, f := range e.Findings {
		if f.Kind == "NO_ADDRESS" || f.Kind == "ADDRESS_NOT_VERIFIED" || f.Kind == "SITE_NO_DEFAULT_ADDRESS" {
			t.Fatalf("address finding from a filtered emergency_addresses sync: %+v", f)
		}
	}
	if e.Assessed || strings.Join(e.MissingLocal, ",") != "emergency_addresses" {
		t.Fatalf("e911 with filtered addresses: assessed=%v missing=%v", e.Assessed, e.MissingLocal)
	}
	devRows := map[string][]string{"assigned": localTestAssignedDevices, "unassigned": localTestUnassignedDevices, "users": {}, "common_areas": {}}
	stdout, _, err = localRun(t, newNovelDevicesHealthCmd, "--db", localTestStoreScoped(t, devRows, map[string]string{"users": "site_id=site-zz"}))
	if err != nil {
		t.Fatalf("devices health: %v", err)
	}
	var dh localDevicesHealthResult
	localDecode(t, stdout, &dh)
	if dh.Assessed || dh.Counts["ASSIGNEE_INACTIVE"] != 0 || !strings.Contains(dh.MissingReasons["users"], "filtered (site_id=site-zz)") {
		t.Fatalf("devices with filtered users: assessed=%v inactive=%d reasons=%v", dh.Assessed, dh.Counts["ASSIGNEE_INACTIVE"], dh.MissingReasons)
	}

	// A scope row for an older checkpoint is ignored.
	stale := localTestStore(t, e911Rows)
	sdb, err := store.Open(stale)
	if err != nil {
		t.Fatal(err)
	}
	if err := localWriteSyncScope(sdb, "users", 1, "site_id=site-hq", "2001-01-01T00:00:00Z", "then", true); err != nil {
		t.Fatal(err)
	}
	_ = sdb.Close()
	stdout, _, err = localRun(t, newNovelE911AuditCmd, "--db", stale)
	if err != nil {
		t.Fatalf("e911: %v", err)
	}
	e = localE911Result{}
	localDecode(t, stdout, &e)
	// The older record says nothing about the current checkpoint: users are
	// not covered, for want of a recorded full walk (not "filtered").
	if e.Assessed || strings.Join(e.MissingLocal, ",") != "users" ||
		!strings.Contains(e.MissingReasons["users"], "no full reconciled sync is recorded") || strings.Contains(e.MissingReasons["users"], "filtered") {
		t.Fatalf("stale scope row: assessed=%v missing=%v reasons=%v", e.Assessed, e.MissingLocal, e.MissingReasons)
	}
}

func TestLocalFilteredUsersIsNotNotFound(t *testing.T) {
	localTestEnv(t)
	localNewTestServer(t)
	db := localTestStoreScoped(t, localTestFootprintRows(), map[string]string{"users": "site_id=site-hq"})

	// Absent from a filtered users sync is not "not found" (exit 3).
	stdout, _, err := localRun(t, newNovelFootprintCmd, "nobody@example.com", "--db", db, "--no-live")
	if err != nil {
		t.Fatalf("footprint for a user outside the filtered sync: exit %d (%v), want 0", localExitCode(err), err)
	}
	var fp localFootprint
	localDecode(t, stdout, &fp)
	if fp.User != nil || fp.Assessed || !strings.Contains(fp.Note, "users do not cover the account") {
		t.Fatalf("footprint = user %+v assessed %v note %q", fp.User, fp.Assessed, fp.Note)
	}

	// --apply on a resolvable user still refuses: users do not cover the account.
	ts := localNewTestServer(t)
	stdout, _, err = localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db, "--apply")
	if code := localExitCode(err); code != 5 {
		t.Fatalf("offboard --apply exit = %d (%v), want 5", code, err)
	}
	var res localOffboardResult
	localDecode(t, stdout, &res)
	if !strings.Contains(strings.Join(res.RefusalReasons, "\n"), "users: the latest completed sync was filtered (site_id=site-hq)") {
		t.Fatalf("reasons = %v", res.RefusalReasons)
	}
	if _, deletes := ts.snapshot(); len(deletes) != 0 {
		t.Fatalf("DELETEs sent: %v", deletes)
	}
}

// localSimulateRun opens the event log, performs the sync_state writes a run
// of the given resources would make (attempt start, then completion), and
// records scope with cmd's flags. Writes land in the same second as the
// fixture's checkpoints, which is the case timestamps cannot tell apart.
func localSimulateRun(t *testing.T, cmd *cobra.Command, path string, completed ...string) {
	t.Helper()
	mark, err := localBeginSyncEventLog(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(completed) > 0 {
		db, err := store.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, rt := range completed {
			if err := db.SaveSyncProgress(rt, "", 0); err != nil {
				t.Fatal(err)
			}
			if err := db.SaveSyncState(rt, "", 0); err != nil {
				t.Fatal(err)
			}
		}
		_ = db.Close()
	}
	if err := localRecordSyncScope(cmd, path, mark); err != nil {
		t.Fatal(err)
	}
}

// A second (unfiltered) sync inside the same second as the first must still
// clear the filtered mark: completion comes from the run's own checkpoint
// writes, not from timestamps.
func TestLocalSyncScopeSameSecondResync(t *testing.T) {
	localTestEnv(t)
	path := localTestStoreScoped(t, map[string][]string{"users": localTestUsers}, map[string]string{"users": "site_id=site-hq"})
	localSimulateRun(t, localTestSyncCommand(t, "--full", "--resources", "users", "--db", path), path, "users")
	if scope := localScopeRows(t, path); scope["users"] != "0|1|" {
		t.Fatalf("same-second unfiltered re-sync left scope %v", scope)
	}
	// A run that wrote no users checkpoint is not attributed to users.
	path2 := localTestStoreScoped(t, map[string][]string{"users": localTestUsers}, map[string]string{"users": "site_id=site-hq"})
	localSimulateRun(t, localTestSyncCommand(t, "--full", "--resources", "users", "--db", path2), path2)
	if scope := localScopeRows(t, path2); scope["users"] != "1|1|site_id=site-hq" {
		t.Fatalf("a run with no users checkpoint write rewrote the record: %v", scope)
	}
}
