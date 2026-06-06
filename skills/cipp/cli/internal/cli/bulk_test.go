// Copyright 2026 damienstevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBulkActionEndpointMapping(t *testing.T) {
	cases := map[string]bulkAction{
		"offboard":       {Method: "POST", Path: "/ExecOffboardUser"},
		"add-user":       {Method: "POST", Path: "/AddUser"},
		"remove-user":    {Method: "POST", Path: "/RemoveUser"},
		"set-forwarding": {Method: "POST", Path: "/ExecEmailForward"},
	}
	for action, want := range cases {
		got, ok := bulkActions[action]
		if !ok {
			t.Fatalf("action %q not mapped", action)
		}
		if got != want {
			t.Fatalf("action %q = %#v, want %#v", action, got, want)
		}
	}
}

func TestParseBulkCSVPlan(t *testing.T) {
	csv := strings.Join([]string{
		"action,tenant,params_json",
		`offboard,contoso.onmicrosoft.com,"{""userPrincipalName"":""bob@contoso.com""}"`,
		`add-user,fabrikam.onmicrosoft.com,"{""displayName"":""Alice""}"`,
		`bogus,contoso.onmicrosoft.com,{}`,
	}, "\n")

	rows, err := parseBulkCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("parseBulkCSV: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d: %#v", len(rows), rows)
	}

	if rows[0].Action != "offboard" || rows[0].Endpoint != "/ExecOffboardUser" || rows[0].Method != "POST" {
		t.Fatalf("row0 mapping wrong: %#v", rows[0])
	}
	if rows[0].Tenant != "contoso.onmicrosoft.com" {
		t.Fatalf("row0 tenant wrong: %q", rows[0].Tenant)
	}
	if rows[0].Params["userPrincipalName"] != "bob@contoso.com" {
		t.Fatalf("row0 params not parsed: %#v", rows[0].Params)
	}
	if rows[1].Action != "add-user" || rows[1].Endpoint != "/AddUser" {
		t.Fatalf("row1 mapping wrong: %#v", rows[1])
	}
	// Unknown action must carry a parseError, not silently map.
	if rows[2].parseError == "" {
		t.Fatalf("row2 (bogus action) should have a parse error")
	}
	if rows[2].Endpoint != "" {
		t.Fatalf("row2 (bogus action) should not map to an endpoint, got %q", rows[2].Endpoint)
	}
	fmt.Printf("bulk plan rows: %s/%s, %s/%s, skip=%q\n",
		rows[0].Action, rows[0].Endpoint, rows[1].Action, rows[1].Endpoint, rows[2].parseError)
}

func TestBulkCheckpointRoundTrip(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "changes.csv")
	cpPath := filepath.Join(dir, "bulk-changes.json")

	done := map[int]bool{0: true, 2: true}
	if err := writeBulkCheckpoint(cpPath, csvPath, done); err != nil {
		t.Fatalf("writeBulkCheckpoint: %v", err)
	}
	got, err := readBulkCheckpoint(cpPath)
	if err != nil {
		t.Fatalf("readBulkCheckpoint: %v", err)
	}
	if !got[0] || !got[2] || got[1] {
		t.Fatalf("checkpoint round-trip wrong: %#v", got)
	}

	// Missing checkpoint reads as empty.
	missing, err := readBulkCheckpoint(filepath.Join(dir, "nope.json"))
	if err != nil {
		t.Fatalf("readBulkCheckpoint(missing): %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing checkpoint should be empty: %#v", missing)
	}
}

func TestBulkDryRunPrintsPlanFromFile(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "changes.csv")
	content := "action,tenant,params_json\noffboard,contoso.onmicrosoft.com,{}\n"
	if err := os.WriteFile(csvPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	var flags rootFlags
	flags.dryRun = true
	cmd := newNovelBulkCmd(&flags)
	cmd.SetArgs([]string{"--from", csvPath})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&strings.Builder{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute --dry-run: %v", err)
	}
	if !strings.Contains(out.String(), "/ExecOffboardUser") {
		t.Fatalf("dry-run plan missing endpoint; got: %q", out.String())
	}
	fmt.Printf("bulk dry-run plan output:\n%s", out.String())
}
