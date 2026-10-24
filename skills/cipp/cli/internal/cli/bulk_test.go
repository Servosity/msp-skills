// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

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
	if err := writeBulkCheckpoint(cpPath, []byte("csv-bytes"), csvPath, done, nil); err != nil {
		t.Fatalf("writeBulkCheckpoint: %v", err)
	}
	got, _, err := readBulkCheckpoint(cpPath)
	if err != nil {
		t.Fatalf("readBulkCheckpoint: %v", err)
	}
	if !got[0] || !got[2] || got[1] {
		t.Fatalf("checkpoint round-trip wrong: %#v", got)
	}

	// Missing checkpoint reads as empty.
	missing, _, err := readBulkCheckpoint(filepath.Join(dir, "nope.json"))
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

// TestCollapseBulkRowsMergesMultiTenantDeploys asserts that identical deploys
// across tenants become ONE server-side fan-out call, while per-user mutations
// stay one call per row. Collapsing is what keeps a large deploy batch from
// tripping Microsoft Graph throttling.
func TestCollapseBulkRowsMergesMultiTenantDeploys(t *testing.T) {
	rows := []bulkRow{
		{Index: 0, Action: "deploy-dlp-policy", Tenant: "a.example.com", Endpoint: "/AddDlpCompliancePolicy", Method: "POST", Params: map[string]any{"name": "PII"}},
		{Index: 1, Action: "deploy-dlp-policy", Tenant: "b.example.com", Endpoint: "/AddDlpCompliancePolicy", Method: "POST", Params: map[string]any{"name": "PII"}},
		{Index: 2, Action: "deploy-dlp-policy", Tenant: "c.example.com", Endpoint: "/AddDlpCompliancePolicy", Method: "POST", Params: map[string]any{"name": "PII"}},
		{Index: 3, Action: "offboard", Tenant: "a.example.com", Endpoint: "/ExecOffboardUser", Method: "POST", Params: map[string]any{"user": "x@a.example.com"}},
		{Index: 4, Action: "offboard", Tenant: "b.example.com", Endpoint: "/ExecOffboardUser", Method: "POST", Params: map[string]any{"user": "y@b.example.com"}},
	}
	batches := collapseBulkRows(rows)

	// 3 identical DLP deploys -> 1 batch; 2 offboards -> 2 batches.
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches (1 merged deploy + 2 offboards), got %d", len(batches))
	}
	if !batches[0].MultiTenant {
		t.Error("the DLP deploy batch must be MultiTenant")
	}
	if len(batches[0].Tenants) != 3 {
		t.Errorf("merged deploy should carry 3 tenants, got %d", len(batches[0].Tenants))
	}
	if len(batches[0].Indexes) != 3 {
		t.Errorf("merged deploy should track 3 row indexes, got %v", batches[0].Indexes)
	}
	for _, b := range batches[1:] {
		if b.MultiTenant {
			t.Error("offboard is a per-user mutation and must NOT be MultiTenant")
		}
		if len(b.Tenants) != 1 {
			t.Errorf("per-user batch should carry exactly 1 tenant, got %d", len(b.Tenants))
		}
	}
}

// TestCollapseBulkRowsKeepsDistinctParamsApart asserts deploys that differ in
// payload are never merged — merging them would silently apply one tenant's
// policy to another.
func TestCollapseBulkRowsKeepsDistinctParamsApart(t *testing.T) {
	rows := []bulkRow{
		{Index: 0, Action: "deploy-dlp-policy", Tenant: "a.example.com", Params: map[string]any{"name": "PII"}},
		{Index: 1, Action: "deploy-dlp-policy", Tenant: "b.example.com", Params: map[string]any{"name": "HIPAA"}},
	}
	batches := collapseBulkRows(rows)
	if len(batches) != 2 {
		t.Fatalf("deploys with different params must not merge; expected 2 batches, got %d", len(batches))
	}
}

// TestCollapseBulkRowsSkipsParseErrors asserts unparseable rows never reach the
// wire.
func TestCollapseBulkRowsSkipsParseErrors(t *testing.T) {
	rows := []bulkRow{
		{Index: 0, Action: "bogus", Tenant: "a.example.com", parseError: "unknown action"},
		{Index: 1, Action: "offboard", Tenant: "a.example.com", Endpoint: "/ExecOffboardUser", Method: "POST"},
	}
	batches := collapseBulkRows(rows)
	if len(batches) != 1 {
		t.Fatalf("rows with parse errors must be skipped; expected 1 batch, got %d", len(batches))
	}
	if batches[0].Action != "offboard" {
		t.Errorf("surviving batch should be the offboard row, got %q", batches[0].Action)
	}
}

// TestBulkMultiTenantActionsMatchSpec asserts every MultiTenant action points at
// an endpoint the spec actually declares selectedTenants on. A wrong entry here
// would send selectedTenants to an endpoint that ignores it, silently applying
// the change to one tenant instead of all of them.
func TestBulkMultiTenantActionsMatchSpec(t *testing.T) {
	// The 21 endpoints whose OpenAPI request body declares `selectedTenants`.
	selectedTenantsEndpoints := map[string]bool{
		"/AddAutopilotConfig": true, "/AddChocoApp": true, "/AddConnectionFilter": true,
		"/AddDefenderDeployment": true, "/AddDlpCompliancePolicy": true, "/AddEnrollment": true,
		"/AddExConnector": true, "/AddMSPApp": true, "/AddNamedLocation": true,
		"/AddOfficeApp": true, "/AddQuarantinePolicy": true, "/AddRetentionCompliancePolicy": true,
		"/AddSafeLinksPolicyFromTemplate": true, "/AddSensitiveInfoType": true,
		"/AddSensitivityLabel": true, "/AddSpamFilter": true, "/AddStoreApp": true,
		"/AddTransportRule": true, "/AddWin32ScriptApp": true, "/DeployContactTemplates": true,
		"/ExecDeployAppTemplate": true,
	}
	for name, def := range bulkActions {
		if def.MultiTenant && !selectedTenantsEndpoints[def.Path] {
			t.Errorf("action %q is marked MultiTenant but %s does not declare selectedTenants", name, def.Path)
		}
	}
}
