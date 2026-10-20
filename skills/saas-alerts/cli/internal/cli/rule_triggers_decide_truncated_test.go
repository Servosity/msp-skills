// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "decide-truncated-plan-surfaced" and "decide-ids-never-truncated" entries.

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
	if res := rqtDecode[rtDecideResult](t, out); res.Truncated || res.Summary.Succeeded != 2 {
		t.Fatalf("want 2 decided and truncated=false: %s", out)
	}
}
