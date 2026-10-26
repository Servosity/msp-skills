// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored: the operator's tenant setting must scope every local-store
// novel command, not only --org. THREATLOCKER_ORG_ID is the same setting as
// --org (ResolveTenantOrg), so a command that filtered on the raw flag alone
// acted on, or showed, tenants the operator never chose. Driven through the
// real command tree against a seeded two-tenant store and a recording fixture.
// No tenant data, no real credentials.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"threatlocker-pp-cli/internal/cliutil/testenv"
)

// runTLWithOrgEnv is runTL with THREATLOCKER_ORG_ID set to orgEnv ("" unsets
// the tenant), so a test can drive the env-only tenant selection.
func runTLWithOrgEnv(t *testing.T, baseURL, orgEnv string, args ...string) (string, error) {
	t.Helper()
	testenv.Isolate(t)
	t.Setenv("THREATLOCKER_API_KEY", tlFixtureToken)
	t.Setenv(TenantOrgEnvVar, orgEnv)
	if baseURL != "" {
		t.Setenv("THREATLOCKER_BASE_URL", baseURL)
	}
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), err
}

// seedTwoTenantStore puts one row per tenant (org-1 / org-2) in every table the
// local-store novel commands read, each carrying its tenant's id or name.
func seedTwoTenantStore(t *testing.T) string {
	t.Helper()
	recent := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	return seedTLStore(t,
		`INSERT INTO organizations (id, data, organization_id, name) VALUES ('org-1','{}','org-1','Tenant 1'), ('org-2','{}','org-2','Tenant 2')`,
		`INSERT INTO computers (id, data, computer_id, computer_name, organization_id, organization_name, is_online, is_isolated) VALUES
			('c1','{}','c1','PC-1','org-1','Tenant 1',1,0), ('c2','{}','c2','PC-2','org-2','Tenant 2',1,0)`,
		tlPendingApprovals,
		`INSERT INTO audit (id, data, organization_id, date_created, username, action, full_path) VALUES
			('l1','{"organizationId":"org-1"}','org-1','`+recent+`','tech','Permit','c:\one.exe'),
			('l2','{"organizationId":"org-2"}','org-2','`+recent+`','tech','Permit','c:\two.exe')`,
		`INSERT INTO maintenance (id, data, computer_id, maintenance_type, start_date_time, end_date_time, created_by) VALUES
			('m1','{}','c1','Learning','`+recent+`','','tech'), ('m2','{}','c2','Learning','`+recent+`','','tech')`)
}

func approveBatchRequests(t *testing.T, rec *tlRecorder) map[string]bool {
	t.Helper()
	got := map[string]bool{}
	for _, r := range rec.seen() {
		if r.Method == "GET" && strings.HasSuffix(r.Path, tlStaleGetByID) {
			continue // the live re-check before each write (tl_stale_snapshot_test.go)
		}
		if !strings.HasSuffix(r.Path, "/ApprovalRequest/ApprovalRequestPermitApplication") {
			t.Fatalf("unexpected request %s %s", r.Method, r.Path)
		}
		if r.Org != r.Body["organizationId"] {
			t.Fatalf("request for %v sent under tenant header %q", r.Body["organizationId"], r.Org)
		}
		got[r.Org] = true
	}
	return got
}

type approveBatchPlan struct {
	DryRun       bool          `json:"dryRun"`
	WouldApprove int           `json:"wouldApprove"`
	Targets      []batchTarget `json:"targets"`
	Refused      string        `json:"refused"`
}

func decodeApproveBatchPlan(t *testing.T, out string) approveBatchPlan {
	t.Helper()
	var plan approveBatchPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("decode dry-run plan %s: %v", out, err)
	}
	return plan
}

// The Codex P1 input: THREATLOCKER_ORG_ID=org-1, hash aaaa pending in org-1 and
// org-2, approve-batch --hash aaaa --yes with no --all-tenants. Only org-1 may
// be approved, and the dry-run must show that same one-tenant plan.
func TestApproveBatchHonoursOrgEnvVar(t *testing.T) {
	rec, base := newTLFixture(t, tlAnswerRecheckPending(func(string, map[string]any) (int, any) { return 200, map[string]any{} }))
	dbPath := seedTLStore(t, tlPendingApprovals)

	out, err := runTLWithOrgEnv(t, base, "org-1", "approvals", "approve-batch", "--hash", "aaaa", "--yes", "--db", dbPath)
	if err != nil {
		t.Fatalf("approve-batch --yes under THREATLOCKER_ORG_ID=org-1: %v\n%s", err, out)
	}
	if got := approveBatchRequests(t, rec); !got["org-1"] || len(got) != 1 {
		t.Fatalf("approvals sent for tenants %v under THREATLOCKER_ORG_ID=org-1, want only org-1\n%s", got, out)
	}

	recDry, baseDry := newTLFixture(t, func(string, map[string]any) (int, any) { return 200, map[string]any{} })
	out, err = runTLWithOrgEnv(t, baseDry, "org-1", "approvals", "approve-batch", "--hash", "aaaa", "--dry-run", "--json", "--db", dbPath)
	if err != nil {
		t.Fatalf("approve-batch --dry-run under THREATLOCKER_ORG_ID=org-1: %v\n%s", err, out)
	}
	plan := decodeApproveBatchPlan(t, out)
	if plan.WouldApprove != 1 || len(plan.Targets) != 1 || plan.Targets[0].OrganizationID != "org-1" {
		t.Fatalf("dry-run under THREATLOCKER_ORG_ID=org-1 plans %d approval(s) %+v, want only org-1 (the live run's plan)\n%s", plan.WouldApprove, plan.Targets, out)
	}
	if n := len(recDry.seen()); n != 0 {
		t.Fatalf("dry-run sent %d request(s)", n)
	}

	// --org still wins over the env var, and --all-tenants still widens it.
	rec2, base2 := newTLFixture(t, tlAnswerRecheckPending(func(string, map[string]any) (int, any) { return 200, map[string]any{} }))
	if out, err := runTLWithOrgEnv(t, base2, "org-1", "approvals", "approve-batch", "--hash", "aaaa", "--org", "org-2", "--yes", "--db", dbPath); err != nil {
		t.Fatalf("approve-batch --org org-2 --yes: %v\n%s", err, out)
	}
	if got := approveBatchRequests(t, rec2); !got["org-2"] || len(got) != 1 {
		t.Fatalf("--org org-2 over THREATLOCKER_ORG_ID=org-1 approved %v, want only org-2", got)
	}
	rec3, base3 := newTLFixture(t, tlAnswerRecheckPending(func(string, map[string]any) (int, any) { return 200, map[string]any{} }))
	if out, err := runTLWithOrgEnv(t, base3, "org-1", "approvals", "approve-batch", "--hash", "aaaa", "--all-tenants", "--yes", "--db", dbPath); err != nil {
		t.Fatalf("approve-batch --all-tenants --yes: %v\n%s", err, out)
	}
	if got := approveBatchRequests(t, rec3); !got["org-1"] || !got["org-2"] || len(got) != 2 {
		t.Fatalf("--all-tenants under THREATLOCKER_ORG_ID=org-1 approved %v, want org-1 and org-2", got)
	}
}

// With no tenant selected (no --org, no THREATLOCKER_ORG_ID) and no
// --all-tenants, a live approve-batch refuses, names every way to choose a
// scope, and sends nothing. The dry-run still exits 0 and shows that refusal
// as an empty plan rather than a cross-tenant one the live run would not run.
func TestApproveBatchWithoutTenantRefusesLiveRun(t *testing.T) {
	rec, base := newTLFixture(t, func(string, map[string]any) (int, any) { return 200, map[string]any{} })
	dbPath := seedTLStore(t, tlPendingApprovals)

	out, err := runTLWithOrgEnv(t, base, "", "approvals", "approve-batch", "--hash", "aaaa", "--yes", "--db", dbPath)
	if err == nil {
		t.Fatalf("approve-batch --yes with no tenant and no --all-tenants succeeded; output:\n%s", out)
	}
	if n := len(rec.seen()); n != 0 {
		t.Fatalf("unscoped approve-batch --yes sent %d request(s)", n)
	}
	for _, want := range []string{"--org", TenantOrgEnvVar, "--all-tenants"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal does not name %s: %v", want, err)
		}
	}

	out, err = runTLWithOrgEnv(t, base, "", "approvals", "approve-batch", "--hash", "aaaa", "--dry-run", "--json", "--db", dbPath)
	if err != nil {
		t.Fatalf("unscoped approve-batch --dry-run failed: %v\n%s", err, out)
	}
	plan := decodeApproveBatchPlan(t, out)
	if plan.WouldApprove != 0 || len(plan.Targets) != 0 {
		t.Fatalf("unscoped dry-run plans %d approval(s) %+v, but the live run would refuse\n%s", plan.WouldApprove, plan.Targets, out)
	}
	for _, want := range []string{"--org", TenantOrgEnvVar, "--all-tenants"} {
		if !strings.Contains(plan.Refused, want) {
			t.Fatalf("unscoped dry-run does not show the refusal naming %s:\n%s", want, out)
		}
	}
	if n := len(rec.seen()); n != 0 {
		t.Fatalf("dry-run sent %d request(s)", n)
	}

	// The verify probe shape (dry-run, no --hash) keeps exiting 0.
	if out, err := runTLWithOrgEnv(t, base, "", "approvals", "approve-batch", "--dry-run"); err != nil {
		t.Fatalf("approve-batch --dry-run with no --hash: %v\n%s", err, out)
	}
}

// Every local-store read member scopes to THREATLOCKER_ORG_ID when --org is
// absent, and --all-tenants (where the command has it) still lifts the scope.
// TestLocalStoreReadsKeepEveryTenantUnderOrgEnvVar pins the read rule: only
// the explicit --org filters local rows. THREATLOCKER_ORG_ID scopes live calls
// but must never hide local rows, because an MSP sets it to its parent
// organization and its child tenants' rows carry the child ids.
func TestLocalStoreReadsKeepEveryTenantUnderOrgEnvVar(t *testing.T) {
	dbPath := seedTwoTenantStore(t)
	cases := []struct {
		name          string
		args          []string
		hasAllTenants bool
	}{
		{"approvals triage", []string{"approvals", "triage"}, true},
		{"devices health", []string{"devices", "health"}, true},
		{"devices health --detail", []string{"devices", "health", "--detail"}, true},
		{"audit drift", []string{"audit", "drift", "--since", "30d"}, true},
		{"audit export", []string{"audit", "export"}, true},
		{"audit retention-check", []string{"audit", "retention-check"}, false},
		{"maintenance open", []string{"maintenance", "open"}, true},
		{"organizations posture", []string{"organizations", "posture"}, false},
	}
	hasTenant := func(out, id, name string) bool {
		return strings.Contains(out, id) || strings.Contains(out, name)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string{}, tc.args...), "--json", "--db", dbPath)
			out, err := runTLWithOrgEnv(t, "", "org-1", args...)
			if err != nil {
				t.Fatalf("%s under THREATLOCKER_ORG_ID=org-1: %v\n%s", tc.name, err, out)
			}
			if !hasTenant(out, "org-1", "Tenant 1") || !hasTenant(out, "org-2", "Tenant 2") {
				t.Fatalf("%s under THREATLOCKER_ORG_ID=org-1 hid a tenant's local rows (only --org may filter them):\n%s", tc.name, out)
			}

			// --org filters, and beats the env var.
			out, err = runTLWithOrgEnv(t, "", "org-1", append(append([]string{}, args...), "--org", "org-2")...)
			if err != nil {
				t.Fatalf("%s --org org-2: %v\n%s", tc.name, err, out)
			}
			if !hasTenant(out, "org-2", "Tenant 2") || hasTenant(out, "org-1", "Tenant 1") {
				t.Fatalf("%s --org org-2 under THREATLOCKER_ORG_ID=org-1 is not scoped to org-2:\n%s", tc.name, out)
			}

			if !tc.hasAllTenants {
				return
			}
			out, err = runTLWithOrgEnv(t, "", "org-1", append(append([]string{}, args...), "--all-tenants")...)
			if err != nil {
				t.Fatalf("%s --all-tenants: %v\n%s", tc.name, err, out)
			}
			if !hasTenant(out, "org-1", "Tenant 1") || !hasTenant(out, "org-2", "Tenant 2") {
				t.Fatalf("%s --all-tenants under THREATLOCKER_ORG_ID=org-1 does not show both tenants:\n%s", tc.name, out)
			}
		})
	}
}
