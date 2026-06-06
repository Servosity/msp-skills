// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGroupsRiskCommand(t *testing.T) {
	out := runNovel(t, seedStore(t), newNovelGroupsRiskCmd)
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("groups risk output is not valid JSON: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Orphaned Project") || !strings.Contains(out, "ownerless") {
		t.Errorf("expected ownerless 'Orphaned Project' flagged, got: %s", out)
	}
	if got := res["scannedGroups"].(float64); got != 3 {
		t.Errorf("expected 3 scanned groups, got %v", got)
	}
	if got := res["missingAssociationData"].(float64); got != 1 {
		t.Errorf("expected 1 group with missing association data, got %v", got)
	}
	// Healthy group must not be flagged; missing-data group must not be flagged.
	for _, name := range []string{"Healthy Team", "Unsynced Associations"} {
		if strings.Contains(out, name) && name == "Healthy Team" {
			t.Errorf("healthy group should not be flagged, got: %s", out)
		}
	}
}

func TestGroupsRiskCommandGuestRatioValidation(t *testing.T) {
	flags := &rootFlags{asJSON: true}
	cmd := newNovelGroupsRiskCmd(flags)
	cmd.SetArgs([]string{"--guest-ratio", "1.5"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected usage error for --guest-ratio > 1")
	} else if ExitCode(err) != 2 {
		t.Errorf("expected exit code 2 for bad --guest-ratio, got %d", ExitCode(err))
	}
}
