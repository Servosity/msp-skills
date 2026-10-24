// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package qbrreport

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"servosity-msp-pp-cli/internal/store"
)

// TestSuccessStatsCannotProveAPastQuarterFromALaterSuccess: pp_last_success
// keeps only the latest success. A backup whose latest success is after the
// quarter ended may or may not have succeeded during it, so it is unknown,
// not stale, and stays out of the rate.
func TestSuccessStatsCannotProveAPastQuarterFromALaterSuccess(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if _, err := db.DB().ExecContext(ctx, `CREATE TABLE pp_last_success (engine TEXT, backup_id TEXT, company_id INTEGER, hostname TEXT, last_success TEXT, checked_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	// 2 succeeded in the last week of Q3; 5 succeeded mid-quarter and then went
	// quiet, so it was stale when the quarter ended.
	for id, last := range map[string]string{"1": "2026-10-22T04:00:00Z", "2": "2026-09-28T04:00:00Z", "3": "2026-05-01T04:00:00Z", "4": "", "5": "2026-08-15T04:00:00Z"} {
		if err := db.UpsertResticBackups(json.RawMessage(`{"id":` + id + `,"company":"/companies/1/"}`)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.DB().ExecContext(ctx, `INSERT INTO pp_last_success VALUES ('restic', ?, 1, '', ?, 'x')`, id, last); err != nil {
			t.Fatal(err)
		}
	}
	from, to := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	got := loadSuccessStats(ctx, db.DB(), 1, from, to)
	if got.Total != 5 || got.Unknown != 1 || got.Dirty != 2 || got.HardFail != 1 || !got.RateKnown {
		t.Fatalf("stats = %+v, want total 5, unknown 1 (latest success after Q3), dirty 2 (before Q3, and stale at its end), hard-fail 1", got)
	}
	if got.Rate != 25 {
		t.Fatalf("rate = %.2f, want 1 success of 4 assessable", got.Rate)
	}
	only := loadSuccessStats(ctx, db.DB(), 1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))
	if only.Unknown != 4 || only.HardFail != 1 || !only.RateKnown || only.Rate != 0 {
		t.Fatalf("Q1 stats = %+v, want 3 unknown and the never-succeeded backup as the only assessable one", only)
	}
}

func TestExecutiveSummaryNamesTheAssessedDenominator(t *testing.T) {
	r := &Report{Company: Company{Name: "Acme"}, Quarter: "2026-Q3",
		SuccessRate: SuccessStats{Total: 2, Unknown: 1, Rate: 100, RateKnown: true}}
	got := executiveSummary(r)
	if !strings.Contains(got, "of the 1 assessable") || !strings.Contains(got, "1 not assessable") || strings.Contains(got, "protected backups") {
		t.Fatalf("summary = %q; it must name the assessed denominator and the unknown count", got)
	}
}

// TestSuccessStatsCountsAQuietBackupAsStaleMidQuarter: while a quarter runs,
// a backup whose only success this quarter was on day one is stale, not a
// 100% success.
func TestSuccessStatsCountsAQuietBackupAsStaleMidQuarter(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if _, err := db.DB().ExecContext(ctx, `CREATE TABLE pp_last_success (engine TEXT, backup_id TEXT, company_id INTEGER, hostname TEXT, last_success TEXT, checked_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	for id, last := range map[string]string{"21": "2026-10-01T03:00:00Z", "22": "2026-10-22T03:00:00Z"} {
		if err := db.UpsertResticBackups(json.RawMessage(`{"id":` + id + `,"company":"/companies/1/"}`)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.DB().ExecContext(ctx, `INSERT INTO pp_last_success VALUES ('restic', ?, 1, '', ?, 'x')`, id, last); err != nil {
			t.Fatal(err)
		}
	}
	prev := qbrNow
	qbrNow = func() time.Time { return time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC) }
	defer func() { qbrNow = prev }()
	got := loadSuccessStats(ctx, db.DB(), 1, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if got.Total != 2 || got.Dirty != 1 || got.Unknown != 0 || got.Rate != 50 {
		t.Fatalf("stats = %+v, want 2 backups, the 22-day-quiet one stale, rate 50", got)
	}
	if !got.AssessedAt.Equal(time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("assessed at %v, want now while the quarter runs", got.AssessedAt)
	}
}

// TestSuccessStatsCountsABackupWithNoFreshnessAsUnknown: a synced backup whose
// freshness lookup failed has no pp_last_success row. It stays in the total
// as unknown instead of disappearing from the report.
func TestSuccessStatsCountsABackupWithNoFreshnessAsUnknown(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if _, err := db.DB().ExecContext(ctx, `CREATE TABLE pp_last_success (engine TEXT, backup_id TEXT, company_id INTEGER, hostname TEXT, last_success TEXT, checked_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"31", "32"} {
		if err := db.UpsertResticBackups(json.RawMessage(`{"id":` + id + `,"company":"/companies/1/"}`)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO pp_last_success VALUES ('restic', '31', 1, '', '2026-09-29T04:00:00Z', 'x')`); err != nil {
		t.Fatal(err)
	}
	got := loadSuccessStats(ctx, db.DB(), 1, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if got.Total != 2 || got.Unknown != 1 || got.Rate != 100 || !got.RateKnown {
		t.Fatalf("stats = %+v, want 2 backups, the one without freshness unknown, 100%% of the 1 assessable", got)
	}
}

// TestSuccessStatsBeforeAnyFreshnessCommand: on a store where no freshness
// command has run (no pp_last_success table), synced backups are counted and
// unknown, not reported as "no restic/DR backups synced".
func TestSuccessStatsBeforeAnyFreshnessCommand(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	for _, id := range []string{"41", "42"} {
		if err := db.UpsertResticBackups(json.RawMessage(`{"id":` + id + `,"company":"/companies/4421/"}`)); err != nil {
			t.Fatal(err)
		}
	}
	got := loadSuccessStats(ctx, db.DB(), 4421, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if got.Total != 2 || got.Unknown != 2 || got.RateKnown {
		t.Fatalf("stats = %+v, want 2 synced backups, both unknown, no rate", got)
	}
}
