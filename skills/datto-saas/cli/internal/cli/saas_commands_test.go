// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"datto-saas-pp-cli/internal/cliutil/testenv"
	"datto-saas-pp-cli/internal/store"
)

// seedSaasStore writes a small fleet shaped like the public-source examples:
// one Office365 client with two domains, one Google client.
func seedSaasStore(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, d := range []string{
		`{"saasCustomerId":123456,"saasCustomerName":"Contoso Ltd","domain":"contoso.com","productType":"Office365","externalSubscriptionId":"Classic:Office365:654321","retentionType":"Infinite","seatsUsed":5,"backupStats":{"activeServicesCount":10,"activeServicesWithRecentBackupCount":9}}`,
		`{"saasCustomerId":123456,"saasCustomerName":"Contoso Ltd","domain":"contoso.co.uk","productType":"Office365","externalSubscriptionId":"Classic:Office365:654322","seatsUsed":1,"backupStats":{"activeServicesCount":2,"activeServicesWithRecentBackupCount":2}}`,
		`{"saasCustomerId":222333,"saasCustomerName":"Fabrikam Inc","domain":"fabrikam.com","productType":"GoogleApps","externalSubscriptionId":"Classic:GoogleApps:777888","seatsUsed":2,"backupStats":{"activeServicesCount":0,"activeServicesWithRecentBackupCount":0}}`,
	} {
		if err := db.UpsertDomains(json.RawMessage(d)); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{
		`{"parent_id":"123456","mainId":"adele@contoso.com","name":"Adele","seatType":"User","seatState":"Active","billable":true,"dateAdded":"2025-03-14T09:12:00Z","remoteId":"r-adele"}`,
		`{"parent_id":"123456","mainId":"site@contoso.com","name":"Site","seatType":"Site","seatState":"Active","billable":1,"dateAdded":"2025-05-01","remoteId":"r-site"}`,
		`{"parent_id":"123456","mainId":"new@contoso.com","name":"New","seatType":"User","seatState":"Unprotected","billable":false,"dateAdded":"2099-01-01T00:00:00Z","remoteId":"r-new"}`,
		`{"parent_id":"123456","mainId":"gone@contoso.com","name":"Gone","seatType":"User","seatState":"Archived","billable":"true","dateAdded":"2024-01-01","remoteId":"r-gone"}`,
		`{"parent_id":"123456","mainId":"odd@contoso.com","name":"Odd","seatType":"User","seatState":"Unprotected","billable":"maybe","dateAdded":"sometime"}`,
		`{"parent_id":"222333","mainId":"drive-1","name":"Drive","seatType":"SharedDrive","seatState":"Unprotected","billable":0,"dateAdded":"2099-01-01","remoteId":"g-drive"}`,
		`{"parent_id":"222333","mainId":"site-1","name":"Wrong","seatType":"Site","seatState":"Unprotected","billable":0,"dateAdded":"2099-01-01","remoteId":"g-site"}`,
	} {
		if err := db.UpsertSeats(json.RawMessage(s)); err != nil {
			t.Fatal(err)
		}
	}
	app := `{"parent_id":"123456","customerId":123456,"customerName":"Contoso Ltd","usedBytes":"2048","suites":[{"suiteType":"Office365","appTypes":[{"appType":"Exchange","backupHistory":[` +
		`{"timeWindow":1,"startTime":1,"activeServiceCount":7,"activeServiceWithBackupCount":7,"activeServiceWithPerfectBackupCount":7},` +
		`{"timeWindow":"2","startTime":2,"activeServiceCount":7,"activeServiceWithBackupCount":5,"activeServiceWithPerfectBackupCount":4,"status":"Partial"}]}]}]}`
	if err := db.UpsertApplications(json.RawMessage(app)); err != nil {
		t.Fatal(err)
	}
	markSynced(t, db)
	return dbPath
}

// markSynced records a completed sync for the three resources, as sync does.
func markSynced(t *testing.T, db *store.Store) {
	t.Helper()
	for _, r := range []string{"domains", "seats", "applications"} {
		if err := db.SaveSyncState(r, "", 1); err != nil {
			t.Fatal(err)
		}
	}
}

func runSaasCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(append(args, "--json", "--max-age", "0"))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.String(), err
}

func decodeRows(t *testing.T, s string) []map[string]any {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal([]byte(s), &rows); err != nil {
		t.Fatalf("not a JSON array: %v\n%s", err, s)
	}
	return rows
}

func TestSaasCommandsAgainstSeededStore(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	db := seedSaasStore(t)

	out, err := runSaasCmd(t, "seats", "reconcile", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out)
	if len(rows) != 2 {
		t.Fatalf("reconcile rows = %d, want 2", len(rows))
	}
	c := rows[0] // Contoso sorts first
	if c["seats_used"] != 6.0 || c["billable"] != 3.0 || c["billable_unknown"] != 1.0 || c["billable_not_active"] != 1.0 {
		t.Fatalf("contoso reconcile wrong: %v", c)
	}
	if !strings.Contains(out, "seats_used_mismatch") || !strings.Contains(out, "billable_unknown") {
		t.Fatalf("missing flags: %s", out)
	}

	out, err = runSaasCmd(t, "seats", "gaps", "--db", db, "--since", "30d")
	if err != nil {
		t.Fatal(err)
	}
	gaps := decodeRows(t, out)
	// "sometime" cannot be parsed, so it is kept (fail-open) and sorted last;
	// "2024"-era seats are outside the window.
	if len(gaps) != 4 || gaps[len(gaps)-1]["main_id"] != "odd@contoso.com" {
		t.Fatalf("gaps = %v", gaps)
	}

	out, err = runSaasCmd(t, "seats", "gaps", "--db", db, "--customer", "nobody.example")
	if err == nil {
		t.Fatalf("unknown customer should fail, got %s", out)
	}

	out, err = runSaasCmd(t, "seats", "bulk-plan", "--db", db, "--action", "License", "--customer", "fabrikam.com")
	if err != nil {
		t.Fatal(err)
	}
	var plan bulkPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Batches) != 1 || plan.Batches[0].SeatType != "SharedDrive" || plan.Batches[0].Warnings[0] != "requires_seat_management_2" {
		t.Fatalf("fabrikam batches = %+v", plan.Batches)
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].Reason != "seat_type_not_allowed_for_product" {
		t.Fatalf("fabrikam skipped = %+v", plan.Skipped)
	}

	out, err = runSaasCmd(t, "seats", "bulk-plan", "--db", db, "--action", "License", "--customer", "123456")
	if err != nil {
		t.Fatal(err)
	}
	plan = bulkPlan{}
	_ = json.Unmarshal([]byte(out), &plan)
	if len(plan.Batches) != 0 || len(plan.Blocked) == 0 || !strings.HasPrefix(plan.Blocked[0].Reason, "ambiguous_subscription") {
		t.Fatalf("two-subscription client must block, got %+v", plan)
	}
	if _, err := runSaasCmd(t, "seats", "bulk-plan", "--db", db, "--action", "License"); err == nil {
		t.Fatal("selector-less plan must be refused")
	}

	out, err = runSaasCmd(t, "backup-health", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	health := decodeRows(t, out)
	if health[0]["saas_customer_name"] != "Contoso Ltd" || health[0]["coverage_pct"] != 91.7 ||
		health[0]["worst_app_coverage_pct"] != 71.4 || health[0]["short_window_streak"] != 1.0 {
		t.Fatalf("health[0] = %v", health[0])
	}
	if health[1]["coverage_pct"] != nil {
		t.Fatalf("zero active services must give null coverage, got %v", health[1])
	}

	out, err = runSaasCmd(t, "exit-inventory", "contoso.com", "--data-source", "local", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	var inv exitInventory
	if err := json.Unmarshal([]byte(out), &inv); err != nil {
		t.Fatal(err)
	}
	if inv.Source != "local" || inv.ReadyToCancel || inv.SeatTotals.Total != 5 || *inv.Storage.UsedBytes != 2048 {
		t.Fatalf("inventory = %+v", inv)
	}
	kinds := map[string]bool{}
	for _, b := range inv.Blockers {
		kinds[b.Kind] = true
	}
	for _, k := range []string{"unprotected_seats", "services_without_recent_backup", "apps_latest_window_incomplete", "seats_without_remote_id"} {
		if !kinds[k] {
			t.Fatalf("blocker %s missing: %+v", k, inv.Blockers)
		}
	}
}

func TestSaasCommandsEmptyMirrorExitZero(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	missing := filepath.Join(t.TempDir(), "none.db")
	for _, args := range [][]string{
		{"seats", "roster"}, {"seats", "summary"}, {"seats", "reconcile"}, {"seats", "gaps"}, {"backup-health"},
	} {
		out, err := runSaasCmd(t, append(args, "--db", missing)...)
		if err != nil || strings.TrimSpace(out) != "[]" {
			t.Fatalf("%v on a missing mirror: out=%q err=%v", args, out, err)
		}
	}
}

func TestParseSeatDateAndBillable(t *testing.T) {
	for _, s := range []string{"2025-03-14T09:12:00Z", "2025-03-14", "2025-03-14 09:12:00", "1760572800", "1760572800000"} {
		if _, ok := parseSeatDate(s); !ok {
			t.Errorf("parseSeatDate(%q) failed", s)
		}
	}
	if _, ok := parseSeatDate("not-a-date"); ok {
		t.Error("garbage date parsed")
	}
	for raw, want := range map[string]any{`true`: true, `"true"`: true, `1`: true, `"0"`: false, `false`: false, `"maybe"`: nil, `null`: nil} {
		got := saasSeat{BillableJS: json.RawMessage(raw)}.billable()
		if (want == nil) != (got == nil) || (got != nil && *got != want) {
			t.Errorf("billable(%s) = %v, want %v", raw, got, want)
		}
	}
}

// A missing mirror must explain itself on a stderr line the MCP server
// forwards ("hint:" prefix), and the exit-inventory note must not point a
// caller with no key pair at a live call that can only get HTTP 401.
func TestEmptyMirrorGuidanceReachesAgents(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	t.Setenv("DATTO_SAAS_PUBLIC_KEY", "")
	t.Setenv("DATTO_SAAS_SECRET_KEY", "")
	missing := filepath.Join(t.TempDir(), "none.db")

	cmd := RootCmd()
	cmd.SetArgs([]string{"backup-health", "--db", missing, "--json", "--max-age", "0"})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("backup-health on a missing mirror: %v", err)
	}
	if !strings.HasPrefix(errOut.String(), "hint: no local mirror at ") {
		t.Fatalf("missing-mirror stderr is not a forwarded hint line: %q", errOut.String())
	}

	got, err := runSaasCmd(t, "exit-inventory", "123456", "--data-source", "local", "--db", missing)
	if err != nil {
		t.Fatalf("exit-inventory on a missing mirror: %v", err)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(got), &res); err != nil {
		t.Fatalf("decode %q: %v", got, err)
	}
	note, _ := res["note"].(string)
	if !strings.Contains(note, "DATTO_SAAS_PUBLIC_KEY") || strings.Contains(note, "--data-source live") {
		t.Fatalf("exit-inventory note without a key pair = %q", note)
	}
}
