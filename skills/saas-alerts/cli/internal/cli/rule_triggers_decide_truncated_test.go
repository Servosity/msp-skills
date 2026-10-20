// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "decide-truncated-plan-surfaced", "decide-ids-never-truncated" and
// "decide-ids-never-dropped" entries.

package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestRuleTriggersDecideTruncatedPlanIsSurfaced: a bulk decision whose trigger
// list stopped at --max-pages warns on stderr, says so in human output, and a
// confirmed run exits as partial rather than success.
func TestRuleTriggersDecideTruncatedPlanIsSurfaced(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	dbPath := filepath.Join(t.TempDir(), "receipts.db")
	out, stderr, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--remarks", "fp", "--org", "ORG_1", "--max-pages", "1", "--include-expired", "--confirm", "--db", dbPath, "--json")
	if !strings.Contains(stderr, "warning: the trigger list is incomplete") {
		t.Fatalf("no truncation warning on stderr: %q\n%s", stderr, out)
	}
	if err == nil {
		t.Fatalf("a confirmed truncated bulk decision exited 0:\n%s", out)
	}
}

// TestRuleTriggersDecideIDsAreNeverTruncated: an --ids selection reads every id
// it does not find on a page directly, so a page cap is not a partial run.
func TestRuleTriggersDecideIDsAreNeverTruncated(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	dbPath := filepath.Join(t.TempDir(), "receipts.db")
	out, stderr, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--remarks", "fp", "--ids", "RT_A2,RT_D", "--max-pages", "1", "--confirm", "--db", dbPath, "--json")
	if err != nil || strings.Contains(stderr, "incomplete") {
		t.Fatalf("an --ids decision under a page cap reported partial: err=%v stderr=%q\n%s", err, stderr, out)
	}
	res := rqtDecode[rtDecideResult](t, out)
	if res.Truncated || res.Summary.Succeeded != 2 {
		t.Fatalf("want 2 decided and truncated=false: %s", out)
	}
	// Supersession is only worked out from the fetched pages, so the cut list still says so.
	if res.SupersessionUnchecked == "" || !strings.Contains(stderr, "supersession was checked only against the triggers read") {
		t.Fatalf("a capped --ids plan must say supersession was unchecked: stderr=%q\n%s", stderr, out)
	}
}

// TestRuleTriggersDecideIDsIgnoreUndecodedRows: an undecodable row cannot hide
// a requested id (each one missing from the pages is read directly), so it does
// not fail a fully decided --ids run.
func TestRuleTriggersDecideIDsIgnoreUndecodedRows(t *testing.T) {
	api := newRQTAPI(t)
	api.triggers = append(api.triggers, map[string]any{"id": "RT_JUNK", "status": "PENDING", "rule": 7})
	rqtEnv(t, api)
	dbPath := filepath.Join(t.TempDir(), "receipts.db")
	out, stderr, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--remarks", "fp", "--ids", "RT_B", "--confirm", "--db", dbPath, "--json")
	res := rqtDecode[rtDecideResult](t, out)
	if err != nil || res.Summary.Succeeded != 1 || res.SupersessionUnchecked == "" {
		t.Fatalf("an undecodable unrelated row failed an --ids run: err=%v stderr=%q\n%s", err, stderr, out)
	}
}

// TestRuleTriggersDecideIDsAccountMismatchIsReported: a requested id that the
// --account selector excludes is reported as skipped, never dropped silently.
func TestRuleTriggersDecideIDsAccountMismatchIsReported(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	out, _, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--remarks", "fp", "--ids", "RT_B", "--account", "jane.doe@example.com", "--json")
	if err != nil {
		t.Fatalf("preview: %v\n%s", err, out)
	}
	res := rqtDecode[rtDecideResult](t, out)
	if len(res.Planned) != 0 || rqtSkipReasons(res)["RT_B"] != rtSkipStatusMismatch {
		t.Fatalf("RT_B (bob@example.com) must be skipped as a selection mismatch: %s", out)
	}
}
