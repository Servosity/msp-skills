// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Execution test: layout-drift audit runs against the real store schema.

package cli

import (
	"encoding/json"
	"testing"
)

func TestNovelAuditLayoutDriftRuns(t *testing.T) {
	out := runNovelCmd(t, newNovelAuditLayoutDriftCmd)
	var rows []driftRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, out)
	}
	if len(rows) != 0 {
		t.Errorf("expected empty result on empty mirror, got %d", len(rows))
	}
}
