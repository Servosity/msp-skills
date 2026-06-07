// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"fmt"
	"strings"
	"testing"
)

func seedCoverageFixture(t *testing.T) string {
	db, path := newTestStore(t)
	seed(t, db, "resources", "res1", `{"id":"res1","tenant_id":"ten1","name":"Jane","kind":"user","external_id":"x1","archived":false}`)
	seed(t, db, "resources", "res2", `{"id":"res2","tenant_id":"ten1","name":"Bob","kind":"user","external_id":"x2","archived":false}`)
	seed(t, db, "resources", "res3", `{"id":"res3","tenant_id":"ten1","name":"Old Site","kind":"site","external_id":"x3","archived":true}`)
	seed(t, db, "resources", "res4", `{"id":"res4","tenant_id":"ten2","name":"Carol","kind":"user","external_id":"x4","archived":false}`)
	seed(t, db, "protections", "prot1", `{"id":"prot1","resource_id":"res1","policy_id":"pol1","job_id":"job1","tenant_id":"ten1"}`)
	return path
}

func TestCoverageGapsFindsUnprotected(t *testing.T) {
	path := seedCoverageFixture(t)
	out, err := runNovel(t, newNovelCoverageGapsCmd, "--db", path)
	if err != nil {
		t.Fatalf("coverage-gaps: %v", err)
	}
	view := decodeView[coverageGapsView](t, out)
	if view.UnprotectedCount != 2 {
		t.Fatalf("unprotected count = %d, want 2 (res2 + res4; res1 protected, res3 archived): %s", view.UnprotectedCount, out)
	}
	ids := map[string]bool{}
	for _, it := range view.Items {
		ids[it.ResourceID] = true
	}
	if !ids["res2"] || !ids["res4"] || ids["res1"] || ids["res3"] {
		t.Errorf("wrong gap set: %v", ids)
	}
}

func TestCoverageGapsTenantAndKindFilters(t *testing.T) {
	path := seedCoverageFixture(t)
	out, err := runNovel(t, newNovelCoverageGapsCmd, "--db", path, "--tenant", "ten1", "--kind", "user")
	if err != nil {
		t.Fatalf("coverage-gaps: %v", err)
	}
	view := decodeView[coverageGapsView](t, out)
	if view.UnprotectedCount != 1 || view.Items[0].ResourceID != "res2" {
		t.Errorf("filtered gaps = %+v, want only res2", view.Items)
	}
}

func TestCoverageGapsIncludeArchived(t *testing.T) {
	path := seedCoverageFixture(t)
	out, err := runNovel(t, newNovelCoverageGapsCmd, "--db", path, "--tenant", "ten1", "--include-archived")
	if err != nil {
		t.Fatalf("coverage-gaps: %v", err)
	}
	view := decodeView[coverageGapsView](t, out)
	found := false
	for _, it := range view.Items {
		if it.ResourceID == "res3" && it.Archived {
			found = true
		}
	}
	if !found {
		t.Errorf("--include-archived should surface res3: %s", out)
	}
}

func TestCoverageGapsEmptyStoreNote(t *testing.T) {
	_, path := newTestStore(t)
	out, err := runNovel(t, newNovelCoverageGapsCmd, "--db", path)
	if err != nil {
		t.Fatalf("coverage-gaps: %v", err)
	}
	view := decodeView[coverageGapsView](t, out)
	if view.TotalResources != 0 || !strings.Contains(view.Note, "fleet-sync") {
		t.Errorf("empty store should note fleet-sync: %s", out)
	}
}

func TestCoverageGapsRejectsLiveDataSource(t *testing.T) {
	_, path := newTestStore(t)
	flags := &rootFlags{asJSON: true, dataSource: "live"}
	cmd := newNovelCoverageGapsCmd(flags)
	cmd.SetArgs([]string{"--db", path})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "no live equivalent") {
		t.Errorf("expected no-live-equivalent rejection, got %v", err)
	}
}

func TestCoverageGapsLimitBoundary(t *testing.T) {
	db, path := newTestStore(t)
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("res%d", i)
		seed(t, db, "resources", id, fmt.Sprintf(`{"id":"%s","tenant_id":"ten1","name":"R%d","kind":"user","archived":false}`, id, i))
	}
	// Exactly --limit gaps: no "capped" note.
	out, err := runNovel(t, newNovelCoverageGapsCmd, "--db", path, "--limit", "3")
	if err != nil {
		t.Fatalf("coverage-gaps: %v", err)
	}
	view := decodeView[coverageGapsView](t, out)
	if len(view.Items) != 3 || strings.Contains(view.Note, "capped") {
		t.Errorf("exactly-limit run must not claim truncation: %s", out)
	}
	// Fewer than exist: capped note fires.
	out, err = runNovel(t, newNovelCoverageGapsCmd, "--db", path, "--limit", "2")
	if err != nil {
		t.Fatalf("coverage-gaps: %v", err)
	}
	view = decodeView[coverageGapsView](t, out)
	if len(view.Items) != 2 || !strings.Contains(view.Note, "capped") {
		t.Errorf("truncated run must set the capped note: %s", out)
	}
	// --limit 0 means unlimited (matches backup-stale semantics).
	out, err = runNovel(t, newNovelCoverageGapsCmd, "--db", path, "--limit", "0")
	if err != nil {
		t.Fatalf("coverage-gaps: %v", err)
	}
	view = decodeView[coverageGapsView](t, out)
	if len(view.Items) != 3 || strings.Contains(view.Note, "capped") {
		t.Errorf("--limit 0 must return all gaps uncapped: %s", out)
	}
}
