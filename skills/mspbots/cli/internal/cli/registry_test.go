// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

// runRegistry executes the registry command tree against a temp store.
func runRegistry(t *testing.T, dbPath string, args ...string) (string, error) {
	t.Helper()
	flags := &rootFlags{asJSON: true}
	cmd := newNovelRegistryCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append(args, "--db", dbPath))
	err := cmd.Execute()
	return out.String(), err
}

func TestRegistryAddListRmRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t.db")

	out, err := runRegistry(t, dbPath, "add", "open-tickets", "1534956341424005122", "--type", "dataset", "--notes", "ops board")
	if err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	var added map[string]any
	if err := json.Unmarshal([]byte(out), &added); err != nil {
		t.Fatalf("add output not JSON: %v\n%s", err, out)
	}
	if added["alias"] != "open-tickets" || added["resource_type"] != "dataset" {
		t.Fatalf("add output = %v", added)
	}

	out, err = runRegistry(t, dbPath, "list")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("list output not JSON array: %v\n%s", err, out)
	}
	if len(entries) != 1 || entries[0]["resource_id"] != "1534956341424005122" {
		t.Fatalf("list = %v", entries)
	}

	out, err = runRegistry(t, dbPath, "rm", "open-tickets")
	if err != nil {
		t.Fatalf("rm: %v\n%s", err, out)
	}
	out, err = runRegistry(t, dbPath, "list")
	if err != nil {
		t.Fatalf("list after rm: %v\n%s", err, out)
	}
	if err := json.Unmarshal([]byte(out), &entries); err != nil || len(entries) != 0 {
		t.Fatalf("registry should be empty after rm, got %s (err=%v)", out, err)
	}
}

func TestRegistryAddValidation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t.db")
	tests := []struct {
		name string
		args []string
	}{
		{"bad type", []string{"add", "x", "1534956341424005122", "--type", "report"}},
		{"alias looks like id", []string{"add", "1534956341424005122", "1534956341424005122"}},
		{"id not snowflake", []string{"add", "x", "not-an-id"}},
		{"rm unknown alias", []string{"rm", "ghost"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if out, err := runRegistry(t, dbPath, tt.args...); err == nil {
				t.Fatalf("expected error, got none\n%s", out)
			}
		})
	}
}
