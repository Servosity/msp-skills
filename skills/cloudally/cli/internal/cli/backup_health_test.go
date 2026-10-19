// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil/testenv"
)

// TestNovelBackupHealthHelpWires smoke-tests that the backup health command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelBackupHealthHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"backup", "health", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("backup health --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "health"} {
		if !strings.Contains(help, want) {
			t.Fatalf("backup health --help missing %q in output:\n%s", want, help)
		}
	}
}

// healthFixtureJSON holds synced backup-status rows in the shapes the feed
// has been seen to use: compact digits, ISO strings, numeric ids, and junk.
const healthFixtureJSON = `[
  {"userId": "5f1c2a", "taskId": "64b0e9", "source": "MS365", "entityName": "fresh@contoso.com",
   "lastBackupDate": "20261015060000", "lastBackupAttemptDate": "20261015060000",
   "backupStatus": [{"subSource": "MAIL", "status": "SUCCESS", "error": "", "errFAQLink": ""}]},
  {"userId": "5f1c2a", "taskId": "64b0e9", "source": "MS365", "entityName": "stale@contoso.com",
   "lastBackupDate": "2026-10-10T06:00:00Z", "lastBackupAttemptDate": "2026-10-10T06:00:00Z",
   "backupStatus": [{"subSource": "MAIL", "status": "SUCCESS", "error": ""}]},
  {"userId": 777, "taskId": 9001, "source": "GMAIL", "entityName": "failing@fabrikam.com",
   "lastBackupDate": "2026-10-14 06:00:00", "lastBackupAttemptDate": "2026-10-15 05:00:00",
   "backupStatus": [{"subSource": "MAIL", "status": "SUCCESS", "error": ""}]},
  {"userId": 777, "taskId": 9001, "source": "GMAIL", "entityName": "running@fabrikam.com",
   "lastBackupDate": "2026-10-14 06:00:00", "lastBackupAttemptDate": "2026-10-15 09:00:00",
   "backupStatus": [{"subSource": "MAIL", "status": "SUCCESS", "error": ""}]},
  {"userId": 777, "taskId": 9001, "source": "GMAIL", "entityName": "partial@fabrikam.com",
   "lastBackupDate": "2026-10-14 06:00:00", "lastBackupAttemptDate": "2026-10-15 10:00:00",
   "backupStatus": [{"subSource": "MAIL", "status": "PARTIAL", "error": ""}]},
  {"userId": "5f1c2a", "taskId": "64b0e9", "source": "SHAREPOINT", "entityName": "sites/hr",
   "lastBackupDate": "20261009000000", "lastBackupAttemptDate": "20261015010000",
   "backupStatus": [{"subSource": "SITE", "status": "FAILED", "error": "Access denied", "errFAQLink": "https://faq.example/denied"}]},
  {"userId": "999", "taskId": "1", "source": "ONEDRIVE", "entityName": "never@contoso.com",
   "lastBackupDate": "", "lastBackupAttemptDate": "garbage",
   "backupStatus": []}
]`

func loadHealthFixture(t *testing.T) []backupStatusRow {
	t.Helper()
	var rows []backupStatusRow
	if err := json.Unmarshal([]byte(healthFixtureJSON), &rows); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return rows
}

func TestClassifyBackupHealth(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	rows := loadHealthFixture(t)
	byEntity := map[string]healthRow{}
	for _, r := range rows {
		h := classifyBackupHealth(r, now, 2)
		byEntity[h.Entity] = h
	}
	cases := []struct {
		entity     string
		stale      bool
		failed     bool
		state      string
		lastBackup string
		ageDays    float64
		errText    string
		faq        string
	}{
		{"fresh@contoso.com", false, false, healthStateOK, "2026-10-15T06:00:00Z", 0.3, "", ""},
		{"stale@contoso.com", true, false, healthStateStale, "2026-10-10T06:00:00Z", 5.3, "", ""},
		// Attempt 7h old and newer than the last success: failing.
		{"failing@fabrikam.com", false, true, healthStateFail, "2026-10-14T06:00:00Z", 1.3, "", ""},
		// Attempt 3h old with a SUCCESS status and no error: still running.
		{"running@fabrikam.com", false, false, healthStateOK, "2026-10-14T06:00:00Z", 1.3, "", ""},
		// Attempt 2h old but with a non-success status: failing at once.
		{"partial@fabrikam.com", false, true, healthStateFail, "2026-10-14T06:00:00Z", 1.3, "", ""},
		{"sites/hr", true, true, "STALE+FAILING", "2026-10-09T00:00:00Z", 6.5, "SITE: Access denied", "https://faq.example/denied"},
		{"never@contoso.com", true, false, healthUnknownDate, healthUnknownDate, -1, "", ""},
	}
	for _, tc := range cases {
		h, ok := byEntity[tc.entity]
		if !ok {
			t.Fatalf("missing %s", tc.entity)
		}
		if h.Stale != tc.stale || h.Failed != tc.failed || h.State != tc.state || h.LastBackup != tc.lastBackup || h.Error != tc.errText || h.FAQLink != tc.faq {
			t.Fatalf("%s: got %+v", tc.entity, h)
		}
		if tc.ageDays < 0 {
			if h.AgeDays != nil {
				t.Fatalf("%s: want unknown age, got %v", tc.entity, *h.AgeDays)
			}
			if h.LastAttempt != healthUnknownDate {
				t.Fatalf("%s: unparseable attempt should be UNKNOWN_DATE, got %q", tc.entity, h.LastAttempt)
			}
			continue
		}
		if h.AgeDays == nil || *h.AgeDays != tc.ageDays {
			t.Fatalf("%s: age = %v, want %v", tc.entity, h.AgeDays, tc.ageDays)
		}
	}
	if byEntity["failing@fabrikam.com"].Account != "777" || byEntity["failing@fabrikam.com"].TaskID != "9001" {
		t.Fatalf("numeric ids should decode as strings: %+v", byEntity["failing@fabrikam.com"])
	}
}

func TestHealthFilterAndGrouping(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	var all []healthRow
	for _, r := range loadHealthFixture(t) {
		all = append(all, classifyBackupHealth(r, now, 2))
	}
	keep := func(f healthFilter) []string {
		var names []string
		for _, h := range all {
			if f.keep(h) {
				names = append(names, h.Entity)
			}
		}
		return names
	}
	if got := keep(healthFilter{}); len(got) != 5 {
		t.Fatalf("default filter should drop only the fresh and the still-running entity, got %v", got)
	}
	if got := keep(healthFilter{failingOnly: true}); len(got) != 3 {
		t.Fatalf("failing only = %v", got)
	}
	if got := keep(healthFilter{account: "5f1c2a", source: "ms365"}); len(got) != 1 || got[0] != "stale@contoso.com" {
		t.Fatalf("account+source filter = %v", got)
	}
	var unhealthy []healthRow
	for _, h := range all {
		if (healthFilter{}).keep(h) {
			unhealthy = append(unhealthy, h)
		}
	}
	sortHealthRows(unhealthy)
	if unhealthy[0].Entity != "sites/hr" || unhealthy[1].Entity != "failing@fabrikam.com" {
		t.Fatalf("failing rows should sort first, oldest first: %v", []string{unhealthy[0].Entity, unhealthy[1].Entity})
	}
	totals := map[string]int{}
	for _, h := range all {
		if (healthFilter{}).inScope(h) {
			totals[h.Account]++
		}
	}
	groups := groupHealthByAccount(unhealthy, totals)
	byAcct := map[string]healthGroup{}
	for _, g := range groups {
		byAcct[g.Account] = g
	}
	if len(groups) != 3 || groups[0].Account != "777" {
		t.Fatalf("groups = %+v", groups)
	}
	// 5f1c2a has 3 synced entities (fresh, stale, sites/hr); 2 are flagged.
	if g := byAcct["5f1c2a"]; g.Entities != 3 || g.Flagged != 2 || g.Stale != 2 || g.Failing != 1 {
		t.Fatalf("5f1c2a group = %+v", g)
	}
	// 777 has 3 entities; the still-running one is not flagged.
	if g := byAcct["777"]; g.Entities != 3 || g.Flagged != 2 || g.Failing != 2 {
		t.Fatalf("777 group = %+v", g)
	}
	if g := byAcct["999"]; g.Entities != 1 || g.Flagged != 1 || g.Unknown != 1 || g.OldestDays != nil {
		t.Fatalf("unknown-date group = %+v", g)
	}
	// With --source, totals only count in-scope entities.
	scoped := healthFilter{source: "GMAIL"}
	scopedTotals := map[string]int{}
	var scopedRows []healthRow
	for _, h := range all {
		if scoped.inScope(h) {
			scopedTotals[h.Account]++
		}
		if scoped.keep(h) {
			scopedRows = append(scopedRows, h)
		}
	}
	if g := groupHealthByAccount(scopedRows, scopedTotals); len(g) != 1 || g[0].Entities != 3 || g[0].Flagged != 2 {
		t.Fatalf("source-scoped groups = %+v", g)
	}
}

func TestBackupHealthUnsyncedHintNoCrash(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"backup", "health", "--stale-days", "2", "--json"})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unsynced run error = %v", err)
	}
	if !strings.Contains(errOut.String(), "hint: local store has not been synced yet") {
		t.Fatalf("want unsynced hint on stderr, got %q", errOut.String())
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Fatalf("want empty JSON array, got %q", out.String())
	}
}

func TestBackupHealthRejectsLiveDataSource(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"backup", "health", "--data-source", "live"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "no live equivalent") {
		t.Fatalf("want no-live-equivalent error, got %v", err)
	}
}
