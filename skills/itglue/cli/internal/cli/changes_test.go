// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Seeded-store test for the changes novel feature.

package cli

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestNovelChangesCommand(t *testing.T) {
	seedITGStore(t)

	// Configurations changed since 2019 → the seeded config (recent updated-at).
	out := runITGCmd(t, "changes", "--since", "2019-01-01", "--resource", "configurations", "--json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(rows) != 1 || fmt.Sprint(rows[0]["id"]) != "30" {
		t.Errorf("want exactly config 30; got %s", out)
	}
	if rows[0]["resource_type"] != "configurations" {
		t.Errorf("row missing resource_type: %#v", rows[0])
	}

	// A future window matches nothing across all resources. The boundary is
	// derived from the clock rather than written as a literal: the newest
	// seeded record is now-24h, so a --since one year ahead of the current
	// day is later than every fixture on every possible run date.
	//
	// The old literal boundary was "2030-01-01", and the clock catches up
	// with it. Not on 2030-01-01 itself: the eight fixtures that carry a
	// relative timestamp are at now-24h, which is still 2029-12-31 that
	// day, so they stay outside the window and the assertion holds one
	// last time. From roughly 2030-01-02 those eight land at or after the
	// boundary, come back as rows, and the assertion inverts. The other
	// two fixtures are pinned at 2020-01-01 and 2021-06-01 and never enter
	// a 2030 window at all, so the failure is eight rows, not ten.
	future := time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02")
	out2 := runITGCmd(t, "changes", "--since", future, "--json")
	var rows2 []map[string]any
	if err := json.Unmarshal([]byte(out2), &rows2); err != nil {
		t.Fatalf("unmarshal future: %v\n%s", err, out2)
	}
	if len(rows2) != 0 {
		t.Errorf("future --since should be empty; got %s", out2)
	}

	// An invalid resource is a usage error.
	root := RootCmd()
	root.SetArgs([]string{"changes", "--resource", "bogus", "--json"})
	if err := root.Execute(); err == nil {
		t.Error("expected error for invalid --resource")
	}
}
