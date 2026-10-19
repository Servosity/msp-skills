// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cloudally-pp-cli/internal/cliutil/testenv"
)

// TestNovelRestorePlanHelpWires smoke-tests that the restore plan command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelRestorePlanHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"restore", "plan", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("restore plan --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "plan"} {
		if !strings.Contains(help, want) {
			t.Fatalf("restore plan --help missing %q in output:\n%s", want, help)
		}
	}
}

// planFixtureJSON is a TaskViewWithEntities response with mixed entity
// shapes: numeric and string ids, several sub-sources, and one entity with
// no backups at all.
const planFixtureJSON = `{
  "id": "64b0e9", "account": 12345, "type": "BACKUP", "source": "MS365", "status": "ACTIVE",
  "snapshotDate": 20261001120000, "numOfEntities": 3, "multiEntitiesTask": true,
  "entities": [
    {"id": 1, "userId": "5f1c2a", "taskId": "64b0e9", "entity": "jane.doe@contoso.com", "status": "ACTIVE",
     "lastBackupDates": {"MAIL": 20261001060000, "CALENDAR": 20261001120000, "CONTACTS": 20260930180000},
     "fullName": "Jane Doe", "extId": "ext-jane"},
    {"id": "2", "userId": "5f1c2a", "taskId": "64b0e9", "entity": "John.Smith@contoso.com", "status": "ACTIVE",
     "lastBackupDates": {"MAIL": 20260928230000}, "fullName": "John Smith"},
    {"id": 3, "userId": "5f1c2a", "taskId": "64b0e9", "entity": "new.hire@contoso.com", "status": "PENDING",
     "lastBackupDates": {}, "fullName": "New Hire"}
  ]
}`

func loadPlanFixture(t *testing.T) *taskWithEntities {
	t.Helper()
	var task taskWithEntities
	if err := json.Unmarshal([]byte(planFixtureJSON), &task); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return &task
}

func findPlanRow(rows []planRow, entity, requested string) *planRow {
	for i := range rows {
		if strings.EqualFold(rows[i].Entity, entity) && rows[i].Requested == requested {
			return &rows[i]
		}
	}
	return nil
}

func TestBuildPlanResolvesNewestAtOrBefore(t *testing.T) {
	task := loadPlanFixture(t)
	if task.Source != "MS365" || len(task.Entities) != 3 || string(task.Entities[0].ExtID) != "ext-jane" {
		t.Fatalf("fixture decoded unexpectedly: %+v", task)
	}
	dates, err := parseRequestedDates([]string{"2026-10-01", "2026-10-01T08:00", "2026-09-29"})
	if err != nil {
		t.Fatalf("parseRequestedDates: %v", err)
	}
	rows := buildPlan(task, dates, nil, "")
	if len(rows) != 9 {
		t.Fatalf("want 9 rows (3 entities x 3 dates), got %d", len(rows))
	}
	cases := []struct {
		entity, requested, wantStatus, wantSub string
		wantRaw                                int64
	}{
		{"jane.doe@contoso.com", "2026-10-01", planOK, "CALENDAR", 20261001120000},
		{"jane.doe@contoso.com", "2026-10-01T08:00", planOK, "MAIL", 20261001060000},
		{"jane.doe@contoso.com", "2026-09-29", planOK, "", 0},
		{"john.smith@contoso.com", "2026-10-01", planOK, "MAIL", 20260928230000},
		{"john.smith@contoso.com", "2026-09-29", planOK, "MAIL", 20260928230000},
		{"new.hire@contoso.com", "2026-10-01", planNoBackup, "", 0},
	}
	for _, tc := range cases {
		row := findPlanRow(rows, tc.entity, tc.requested)
		if row == nil {
			t.Fatalf("missing row for %s %s", tc.entity, tc.requested)
		}
		if tc.entity == "jane.doe@contoso.com" && tc.requested == "2026-09-29" {
			// 2026-09-29 has no backup: contacts was 09-30, so NO_BACKUP.
			if row.Status != planNoBackup {
				t.Fatalf("jane 09-29: want NO_BACKUP, got %+v", *row)
			}
			continue
		}
		if row.Status != tc.wantStatus || row.SubSource != tc.wantSub || row.ResolvedRaw != tc.wantRaw {
			t.Fatalf("%s %s: got %+v, want %s %s %d", tc.entity, tc.requested, *row, tc.wantStatus, tc.wantSub, tc.wantRaw)
		}
	}
	jane := findPlanRow(rows, "jane.doe@contoso.com", "2026-10-01")
	if jane.ExtID != "ext-jane" || jane.Resolved != "2026-10-01T12:00:00Z" || jane.Age != "11h59m" {
		t.Fatalf("jane row details: %+v", *jane)
	}
	john := findPlanRow(rows, "john.smith@contoso.com", "2026-10-01")
	if john.Age != "3d0h" {
		t.Fatalf("john age = %q", john.Age)
	}
}

func TestBuildPlanSubSourceAndEntityFilter(t *testing.T) {
	task := loadPlanFixture(t)
	dates, _ := parseRequestedDates([]string{"2026-10-01"})
	rows := buildPlan(task, dates, map[string]bool{"john.smith@contoso.com": true}, "mail")
	if len(rows) != 1 || rows[0].Entity != "John.Smith@contoso.com" || rows[0].SubSource != "MAIL" {
		t.Fatalf("filtered rows = %+v", rows)
	}
	rows = buildPlan(task, dates, nil, "contacts")
	jane := findPlanRow(rows, "jane.doe@contoso.com", "2026-10-01")
	if jane.Status != planOK || jane.SubSource != "CONTACTS" || jane.ResolvedRaw != 20260930180000 {
		t.Fatalf("sub-source contacts: %+v", *jane)
	}
	if john := findPlanRow(rows, "john.smith@contoso.com", "2026-10-01"); john.Status != planNoBackup {
		t.Fatalf("john has no contacts backup, got %+v", *john)
	}
	missing := missingEntities(task, []string{"ghost@contoso.com", "JANE.DOE@contoso.com"})
	if len(missing) != 1 || missing[0] != "ghost@contoso.com" {
		t.Fatalf("missingEntities = %v", missing)
	}
}

func TestParseRequestedDatesRejectsBadInput(t *testing.T) {
	if _, err := parseRequestedDates(nil); err == nil {
		t.Fatal("expected error for no dates")
	}
	if _, err := parseRequestedDates([]string{"not-a-date"}); err == nil {
		t.Fatal("expected error for bad date")
	}
	dates, err := parseRequestedDates([]string{"2026-10-01,2026-10-02", "2026-10-01"})
	if err != nil || len(dates) != 2 {
		t.Fatalf("comma split and dedupe: %v %v", dates, err)
	}
}

func TestParseEntityFilterFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entities.csv")
	content := "email,department\n# comment\n\n\"Jane.Doe@contoso.com\",sales\njohn.smith@contoso.com\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	set, order, err := parseEntityFilter([]string{"extra@contoso.com, jane.doe@contoso.com"}, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(set) != 3 || !set["jane.doe@contoso.com"] || !set["john.smith@contoso.com"] || !set["extra@contoso.com"] {
		t.Fatalf("set = %v", set)
	}
	if len(order) != 3 || order[0] != "extra@contoso.com" {
		t.Fatalf("order = %v", order)
	}
}

func TestPlanFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.json")
	plan := restorePlan{Scope: scopeDistributor, AccountID: "5f1c2a", TaskID: "64b0e9", Source: "MS365", Dates: []string{"2026-10-01"},
		Rows: []planRow{{Entity: "jane.doe@contoso.com", Requested: "2026-10-01", ResolvedRaw: 20261001120000, Status: planOK}}}
	if err := writePlanFile(path, plan); err != nil {
		t.Fatal(err)
	}
	back, err := readPlanFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if back.TaskID != "64b0e9" || len(back.Rows) != 1 || back.Rows[0].ResolvedRaw != 20261001120000 {
		t.Fatalf("round trip = %+v", back)
	}
}

func TestRestoreScopePaths(t *testing.T) {
	cases := []struct {
		scope                 restoreScope
		kind, task, list, rst string
	}{
		{restoreScope{}, scopeAccount, "/v2/backup-tasks/64b0e9", "/v2/backup-tasks", "/v2/recovery/restore"},
		{restoreScope{account: "5f1c2a"}, scopeDistributor, "/v2/distributors/accounts/5f1c2a/backup-tasks/64b0e9", "/v2/distributors/accounts/5f1c2a/backup-tasks", "/v2/distributors/accounts/5f1c2a/recovery/restore"},
		{restoreScope{account: "5f1c2a", reseller: true}, scopeReseller, "/v2/resellers/accounts/5f1c2a/backup-tasks/64b0e9", "/v2/resellers/accounts/5f1c2a/backup-tasks", "/v2/resellers/accounts/5f1c2a/recovery/restore"},
	}
	for _, tc := range cases {
		if tc.scope.kind() != tc.kind || tc.scope.taskPath("64b0e9") != tc.task || tc.scope.tasksPath() != tc.list || tc.scope.restorePath() != tc.rst {
			t.Fatalf("scope %+v: %s %s %s %s", tc.scope, tc.scope.kind(), tc.scope.taskPath("64b0e9"), tc.scope.tasksPath(), tc.scope.restorePath())
		}
	}
	if err := (restoreScope{reseller: true}).validate(); err == nil {
		t.Fatal("--reseller without --account should be rejected")
	}
}

func TestRestorePlanDryRunDoesNotCallAPI(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("CLOUDALLY_BASE_URL", "http://127.0.0.1:9")
	cmd := RootCmd()
	cmd.SetArgs([]string{"restore", "plan", "--account", "5f1c2a", "--task", "64b0e9", "--date", "2026-10-01", "--dry-run"})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run error = %v\nstderr: %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "dry-run: would GET /v2/distributors/accounts/5f1c2a/backup-tasks/64b0e9") {
		t.Fatalf("unexpected dry-run output: %q", out.String())
	}
}

func TestRestorePlanMissingTaskIsUsageError(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"restore", "plan", "--date", "2026-10-01"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--task") {
		t.Fatalf("want usage error naming --task, got %v", err)
	}
	if code := ExitCode(err); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}
