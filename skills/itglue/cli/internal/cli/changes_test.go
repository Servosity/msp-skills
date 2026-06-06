// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Seeded-store test for the changes novel feature.

package cli

import (
	"encoding/json"
	"fmt"
	"testing"
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

	// A future window matches nothing across all resources.
	out2 := runITGCmd(t, "changes", "--since", "2030-01-01", "--json")
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
