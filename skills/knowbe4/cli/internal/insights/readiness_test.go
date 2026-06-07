package insights

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"knowbe4-pp-cli/internal/store"
)

func TestReportRateWorstFirst(t *testing.T) {
	db, done := seed(t)
	defer done()
	// alice: delivered 2, clicked 2, reported 0; bob: delivered 2, clicked 1, reported 1.
	got, err := ReportRate(context.Background(), db, false, 0, true, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 ranked users, got %d: %+v", len(got), got)
	}
	if got[0].Entity != "alice@acme.com" || !got[0].NeverReported || got[0].ReportRatePct != 0 || got[0].Clicked != 2 {
		t.Fatalf("want alice worst (never reported, 2 clicks), got %+v", got[0])
	}
	if got[1].Entity != "bob@acme.com" || got[1].Reported != 1 || got[1].ReportRatePct != 50 {
		t.Fatalf("want bob second with 50%% report rate, got %+v", got[1])
	}
}

func TestReportRateBestFirstAndLimit(t *testing.T) {
	db, done := seed(t)
	defer done()
	got, err := ReportRate(context.Background(), db, false, 1, false, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Entity != "bob@acme.com" {
		t.Fatalf("want bob as the single best reporter, got %+v", got)
	}
}

func TestReportRateByGroupEmptyWithoutMembership(t *testing.T) {
	db, done := seed(t)
	defer done()
	// Seeded users carry '{}' data — no group membership — so the group
	// aggregation must return an empty set, not fabricated rows.
	got, err := ReportRate(context.Background(), db, true, 0, true, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("want no group rows without membership data, got %+v", got)
	}
}

func TestFreshnessReadiness(t *testing.T) {
	db, done := seed(t)
	defer done()
	rep, err := Freshness(context.Background(), db, 168*time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var prRows int
	for _, r := range rep.Resources {
		if r.Table == "pst_recipients" {
			prRows = r.Rows
		}
	}
	if prRows != 4 {
		t.Fatalf("want 4 pst_recipients rows in report, got %d", prRows)
	}
	ready := map[string]CommandReadiness{}
	for _, c := range rep.Commands {
		ready[c.Command] = c
	}
	if !ready["repeat-clickers"].Ready {
		t.Fatalf("repeat-clickers should be ready (pst_recipients populated): %+v", ready["repeat-clickers"])
	}
	// Readiness reflects the TYPED tables the commands read: the seeded
	// training_enrollments table is populated even though the generic
	// resources ledger is empty, and UntrainedClickers works against it.
	if !ready["untrained-clickers"].Ready {
		t.Fatalf("untrained-clickers should be ready (typed training_enrollments populated): %+v", ready["untrained-clickers"])
	}
	if !ready["risk-drift"].Ready {
		t.Fatalf("risk-drift should be ready (2 distinct snapshots): %+v", ready["risk-drift"])
	}
}

func TestFreshnessEmptyStoreNotReady(t *testing.T) {
	dir := t.TempDir()
	st, err := store.OpenWithContext(context.Background(), filepath.Join(dir, "empty.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	rep, err := Freshness(context.Background(), st.DB(), 168*time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range rep.Commands {
		if c.Ready {
			t.Fatalf("%s should NOT be ready on an empty store: %+v", c.Command, c)
		}
		if c.Hint == "" {
			t.Fatalf("not-ready command %s must carry a hint", c.Command)
		}
		if len(c.Missing) == 0 {
			t.Fatalf("not-ready command %s must name its missing inputs", c.Command)
		}
	}
}
