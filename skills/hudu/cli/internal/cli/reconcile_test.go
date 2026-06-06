// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Execution test: reconcile runs against the real store schema and emits a report.

package cli

import (
	"encoding/json"
	"testing"
)

func TestNovelReconcileRuns(t *testing.T) {
	out := runNovelCmd(t, newNovelReconcileCmd, "--integrator", "cw_manage")
	var report reconcileReport
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("output is not a reconcile report: %v\n%s", err, out)
	}
	if report.Matched != 0 || report.Potential != 0 || report.Orphaned != 0 {
		t.Errorf("expected zero counts on empty mirror, got %+v", report)
	}
}
