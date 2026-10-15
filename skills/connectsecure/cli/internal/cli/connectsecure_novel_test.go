// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectsecure-pp-cli/internal/client"
	"connectsecure-pp-cli/internal/cliutil/testenv"
	"connectsecure-pp-cli/internal/config"
	"connectsecure-pp-cli/internal/store"
)

var csNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func csFixture() map[string][]csRow {
	recent, old := "2026-09-30T00:00:00", "2024-01-01T00:00:00"
	return map[string][]csRow{
		csResCompanies: {
			{"id": float64(1), "name": "Acme Dental"}, {"id": float64(2), "name": "Beta Law"}, {"id": float64(3), "name": "Quiet Co"},
		},
		csResAssets: {
			{"id": float64(101), "host_name": "acme-dc01", "company_id": float64(1), "company_name": "Acme Dental", "agent_id": float64(11), "auth_scan_status": true, "last_discovered_time": recent},
			{"id": float64(102), "host_name": "acme-ws02", "company_id": float64(1), "company_name": "Acme Dental", "agent_id": float64(12), "auth_scan_status": false, "last_discovered_time": old},
			{"id": float64(201), "host_name": "beta-fs01", "company_id": float64(2), "company_name": "Beta Law", "agent_id": float64(21), "auth_scan_status": true, "last_discovered_time": recent},
			{"id": float64(202), "host_name": "beta-old", "company_id": float64(2), "is_deprecated": true, "last_discovered_time": old},
		},
		csResAgents: {
			{"id": float64(11), "last_ping_time": recent}, {"id": float64(12), "last_ping_time": old}, {"id": float64(21), "last_ping_time": recent},
		},
		csResProblemsByCo: {
			{"problem_name": "CVE-2024-3400", "severity": "Critical", "epss_score": 0.95, "base_score": 10.0, "company_id": float64(1), "company_name": "Acme Dental", "asset_ids": []any{"101", "102"}, "first_vul_discovered": recent},
			{"problem_name": "CVE-2024-3400", "severity": "Critical", "epss_score": 0.95, "base_score": 10.0, "company_id": float64(2), "asset_ids": []any{"201"}, "first_vul_discovered": old},
			{"problem_name": "CVE-2023-1111", "severity": "High", "epss_score": 0.2, "base_score": 7.5, "company_id": float64(2), "company_name": "Beta Law", "asset_ids": []any{"201", "999"}, "first_vul_discovered": old},
		},
		csResRemediations: {
			{"problem_name": "CVE-2024-3400", "company_id": float64(1), "cisa": "2024-04-12"},
			{"problem_name": "CVE-2024-3400", "company_id": float64(2), "cisa": "2024-04-12"},
			{"problem_name": "CVE-2023-1111", "company_id": float64(2), "cisa": ""},
		},
		csResSuppressions: {
			{"id": float64(1), "company_id": float64(2), "problem_name": "CVE-2023-1111", "suppressed_till": old, "requested_by": "t@msp", "approved_by": "T@msp"},
			{"id": float64(2), "company_id": float64(1), "problem_name": "CVE-2024-3400", "requested_by": "a@msp"},
			{"id": float64(3), "company_id": float64(1), "problem_name": "CVE-2023-1111", "suppressed_till": "2099-01-01", "requested_by": "a@msp", "approved_by": "lead@msp"},
			{"id": float64(4), "company_id": float64(1), "problem_name": "CVE-2023-1111", "suppressed_till": old, "unsuppressed_on": recent},
		},
		csResProblemsGlobal: {{"problem_name": "CVE-2024-3400", "epss_score": 0.95, "severity": "Critical"}, {"problem_name": "CVE-2023-1111", "epss_score": 0.2, "severity": "High"}},
		csResVelocity: {
			{"company_id": float64(1), "severity": "Critical", "affected_software_graterthan_30_days": float64(2)},
			{"company_id": float64(1), "severity": "High", "affected_software_graterthan_30_days": float64(4)},
		},
		csResRiskScore: {{"company_name": "acme dental", "risk_score": "82.5"}},
	}
}

func TestBuildExposureFiltersJoinsAndRanks(t *testing.T) {
	data := csFixture()
	rows := buildExposure(data, exposureOpts{minEPSS: 0.5, severities: csCSVSet("critical"), now: csNow})
	if len(rows) != 3 {
		t.Fatalf("want 3 critical asset rows, got %d: %+v", len(rows), rows)
	}
	for _, r := range rows {
		if r.ProblemName != "CVE-2024-3400" || !r.KEV || !r.AssetSynced {
			t.Fatalf("unexpected row %+v", r)
		}
	}
	if rows[2].CompanyName != "Beta Law" {
		t.Fatalf("company name must fall back to synced companies, got %q", rows[2].CompanyName)
	}
	// Negative: the High/0.2 problem never passes --min-epss 0.5.
	for _, r := range buildExposure(data, exposureOpts{minEPSS: 0.5, now: csNow}) {
		if r.ProblemName == "CVE-2023-1111" {
			t.Fatal("EPSS filter leaked a 0.2 finding")
		}
	}
	// Window: only Acme's finding is new within 7 days.
	recent := buildExposure(data, exposureOpts{newSince: 7 * 24 * time.Hour, now: csNow})
	if len(recent) != 2 || recent[0].CompanyID != "1" {
		t.Fatalf("new-since window wrong: %+v", recent)
	}
	// Unsynced asset ids still surface, flagged.
	all := buildExposure(data, exposureOpts{severities: csCSVSet("high"), now: csNow})
	if len(all) != 2 || all[0].AssetSynced == all[1].AssetSynced {
		t.Fatalf("want one synced and one unsynced asset row: %+v", all)
	}
	if got := buildExposure(map[string][]csRow{}, exposureOpts{now: csNow}); got == nil || len(got) != 0 {
		t.Fatal("empty store must yield an empty, non-nil slice")
	}
}

func TestBuildPortfolioRollsUpPerCompany(t *testing.T) {
	rows := buildPortfolio(csFixture(), 30*24*time.Hour, csNow)
	by := map[string]portfolioRow{}
	for _, r := range rows {
		by[r.CompanyID] = r
	}
	acme, beta, quiet := by["1"], by["2"], by["3"]
	if acme.Assets != 2 || acme.StaleAssets != 1 || acme.Critical != 1 || acme.KEV != 1 || acme.OpenOver30Days != 6 || acme.RiskScore != "82.5" {
		t.Fatalf("acme rollup wrong: %+v", acme)
	}
	if acme.ActiveSuppressions != 2 {
		t.Fatalf("acme active suppressions: want 2 (open-ended + future), got %d", acme.ActiveSuppressions)
	}
	if beta.Assets != 1 || beta.High != 1 || beta.Critical != 1 || beta.ActiveSuppressions != 0 {
		t.Fatalf("beta rollup wrong (deprecated asset and expired suppression must not count): %+v", beta)
	}
	if quiet.CompanyName != "Quiet Co" || quiet.Assets != 0 {
		t.Fatalf("quiet client must still appear with zeros: %+v", quiet)
	}
	// A deleted company stays out even when other tables reference it.
	data := csFixture()
	data[csResCompanies] = append(data[csResCompanies], csRow{"id": float64(9), "name": "Gone LLC", "is_deleted": true})
	data[csResAssets] = append(data[csResAssets], csRow{"id": float64(901), "company_id": float64(9), "last_discovered_time": "2026-09-30"})
	for _, r := range buildPortfolio(data, 0, csNow) {
		if r.CompanyID == "9" {
			t.Fatal("deleted company resurfaced through an asset row")
		}
	}
	// Ties sort deterministically.
	tie := []portfolioRow{{CompanyID: "2"}, {CompanyID: "1"}, {CompanyID: "3"}}
	if _ = sortPortfolio(tie, "max-epss"); tie[0].CompanyID != "1" || tie[2].CompanyID != "3" {
		t.Fatalf("tie-break by id failed: %+v", tie)
	}
	if err := sortPortfolio(rows, "bogus"); err == nil {
		t.Fatal("unknown --sort must be a usage error")
	}
}

func TestBuildBlindSpots(t *testing.T) {
	rows := buildBlindSpots(csFixture(), blindSpotOpts{stale: 14 * 24 * time.Hour, agentStale: 48 * time.Hour, now: csNow})
	if len(rows) != 1 || rows[0].AssetID != "102" {
		t.Fatalf("want only acme-ws02, got %+v", rows)
	}
	want := "stale_discovery,unauthenticated_scan,silent_agent"
	if got := strings.Join(rows[0].Reasons, ","); got != want {
		t.Fatalf("reasons = %s, want %s", got, want)
	}
	// Boolean false (the spec's type) flags; true and missing do not.
	for v, want := range map[any]bool{false: true, true: false, nil: false, "Unauthenticated": true, "Authenticated": false, "Partial": true} {
		if got := blindSpotUnauthenticated(v, nil); got != want {
			t.Errorf("auth_scan_status %v: flagged=%v want %v", v, got, want)
		}
	}
	// An explicit --auth-ok list overrides the string heuristic.
	if blindSpotUnauthenticated("Partial", csCSVSet("partial")) {
		t.Fatal("--auth-ok value must not be flagged")
	}
}

func TestBuildSuppressionAudit(t *testing.T) {
	rows := buildSuppressionAudit(csFixture(), 14*24*time.Hour, false, csNow)
	got := map[string]string{}
	for _, r := range rows {
		got[r.ID] = strings.Join(r.Flags, ",")
	}
	if got["1"] != "expired_but_active,self_approved" || got["2"] != "open_ended,unapproved" {
		t.Fatalf("flags wrong: %v", got)
	}
	if _, ok := got["3"]; ok {
		t.Fatal("approved future suppression must not be flagged")
	}
	if _, ok := got["4"]; ok {
		t.Fatal("lifted suppression must be skipped")
	}
	if rows[0].ID != "2" {
		t.Fatalf("highest current EPSS must sort first, got %s", rows[0].ID)
	}
	if all := buildSuppressionAudit(csFixture(), 0, true, csNow); len(all) != 3 {
		t.Fatalf("--all keeps every active suppression: got %d", len(all))
	}
}

func TestBuildADHygiene(t *testing.T) {
	data := map[string][]csRow{
		csResCompanies: {{"id": float64(1), "name": "Acme Dental"}},
		csResADUsers: {
			{"company_id": float64(1), "account_disabled": false, "password_never_expires": true, "mfa_enabled": false, "last_logon_timestamp": "2024-01-01"},
			{"company_id": float64(1), "account_disabled": false, "password_not_required": true, "mfa_enabled": true, "last_logon_timestamp": "2026-09-30T00:00:00"},
			{"company_id": float64(1), "account_disabled": true, "password_never_expires": true},
		},
		csResADPolicies:       {{"company_id": float64(1), "complexity_enabled": true, "lockout_threshold": float64(5), "min_password_length": float64(8)}},
		csResAzureSecureScore: {{"company_id": float64(1), "current_score": float64(45), "max_score": float64(90)}},
	}
	rows := buildADHygiene(data, 90*24*time.Hour, 12, csNow)
	if len(rows) != 1 {
		t.Fatalf("want one company, got %d", len(rows))
	}
	r := rows[0]
	if r.EnabledUsers != 2 || r.PasswordNeverExpires != 1 || r.PasswordNotRequired != 1 || r.NoMFA != 1 || r.StaleLogon != 1 {
		t.Fatalf("user counts wrong (disabled account must be excluded): %+v", r)
	}
	if r.WeakPolicies != 1 || strings.Join(r.WeakPolicyReasons, ",") != "min_length_8" {
		t.Fatalf("policy only fails on length: %+v", r)
	}
	if r.AzureSecureScorePct == nil || *r.AzureSecureScorePct != 50 {
		t.Fatalf("azure pct wrong: %+v", r.AzureSecureScorePct)
	}
	if strong := buildADHygiene(data, 0, 8, csNow); strong[0].WeakPolicies != 0 {
		t.Fatal("min-length 8 policy must pass --min-length 8")
	}
}

func TestGroupCVEClients(t *testing.T) {
	rows := []csRow{{"host_name": "a1", "company_name": "Acme"}, {"host_name": "a2", "company_name": "Acme"}, {"host_name": "b1", "company_name": "Beta"}, {"host_name": "x"}}
	got := groupCVEClients(rows, 1)
	if len(got) != 3 || got[0].CompanyName != "Acme" || got[0].AssetCount != 2 || len(got[0].Assets) != 1 {
		t.Fatalf("grouping wrong: %+v", got)
	}
}

func TestResolveQBRCompany(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cond := r.URL.Query().Get("condition")
		switch {
		case strings.Contains(cond, "acme"):
			_, _ = w.Write([]byte(`{"status":true,"data":[{"id":1,"name":"Acme Dental"},{"id":4,"name":"Acme Dental West"}]}`))
		case strings.Contains(cond, "beta"):
			_, _ = w.Write([]byte(`{"status":true,"data":[{"id":2,"name":"Beta Law"}]}`))
		default:
			_, _ = w.Write([]byte(`{"status":true,"data":[]}`))
		}
	}))
	defer srv.Close()
	c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: "t", ConnectsecureUserId: "1"}, 5*time.Second, 0)
	c.NoCache = true
	ctx := context.Background()
	if id, name, err := resolveQBRCompany(ctx, c, "acme dental", ""); err != nil || id != "1" || name != "Acme Dental" {
		t.Fatalf("exact match must win over substring: %s %s %v", id, name, err)
	}
	if _, _, err := resolveQBRCompany(ctx, c, "acme", ""); err == nil || !strings.Contains(err.Error(), "matches 2 companies") {
		t.Fatalf("ambiguous name must list candidates, got %v", err)
	}
	if id, _, err := resolveQBRCompany(ctx, c, "beta", ""); err != nil || id != "2" {
		t.Fatalf("single substring match must resolve: %s %v", id, err)
	}
	if _, _, err := resolveQBRCompany(ctx, c, "zzz", ""); err == nil || !strings.Contains(err.Error(), "no company matches") {
		t.Fatalf("no match must be not-found, got %v", err)
	}
	if _, _, err := resolveQBRCompany(ctx, c, "", "1; DROP"); err == nil {
		t.Fatal("non-numeric --company-id must be rejected before any request")
	}
}

func TestCsTimeShapes(t *testing.T) {
	for _, v := range []any{"2026-09-30T00:00:00Z", "2026-09-30T00:00:00", "2026-09-30 00:00:00", "2026-09-30", float64(1790726400), "1790726400000"} {
		if _, ok := csTime(v); !ok {
			t.Errorf("csTime(%v) failed", v)
		}
	}
	if _, ok := csTime("not a date"); ok {
		t.Error("garbage must not parse")
	}
	// A never-logged-on account often carries 0; these must return, not recurse.
	for _, v := range []any{float64(0), "0", float64(-5), "-5", "NaN", math.Inf(1), "Inf"} {
		if _, ok := csTime(v); ok {
			t.Errorf("csTime(%v) must report false", v)
		}
	}
}

func TestCsSeveritySetRejectsTypos(t *testing.T) {
	if _, err := csSeveritySet("Critical,high"); err != nil {
		t.Fatalf("valid severities rejected: %v", err)
	}
	if _, err := csSeveritySet("Critcal,High"); err == nil || !strings.Contains(err.Error(), "critcal") {
		t.Fatalf("typo must be a usage error naming the value, got %v", err)
	}
	if set, err := csSeveritySet(""); err != nil || len(set) != 0 {
		t.Fatal("empty --severity means no filter")
	}
}

func TestADHygieneRankCountsStaleLogons(t *testing.T) {
	data := map[string][]csRow{
		csResCompanies: {{"id": float64(1), "name": "One Risky Account"}, {"id": float64(2), "name": "All Stale"}},
		csResADUsers: {
			{"company_id": float64(1), "account_disabled": false, "password_never_expires": true, "last_logon_timestamp": "2026-09-30"},
			{"company_id": float64(2), "account_disabled": false, "last_logon_timestamp": "2020-01-01"},
			{"company_id": float64(2), "account_disabled": false, "last_logon_timestamp": "2020-01-01"},
			{"company_id": float64(2), "account_disabled": false, "last_logon_timestamp": "2020-01-01"},
		},
	}
	rows := buildADHygiene(data, 90*24*time.Hour, 0, csNow)
	if len(rows) != 2 || rows[0].CompanyID != "2" || rows[0].RiskRank <= rows[1].RiskRank {
		t.Fatalf("three stale logons must outrank one never-expiring password: %+v", rows)
	}
}

func TestADHygieneNewerAzureSnapshotReplacesPercentage(t *testing.T) {
	data := map[string][]csRow{
		csResAzureSecureScore: {
			{"company_id": float64(1), "current_score": float64(45), "max_score": float64(90), "created_date_time": "2026-01-01T00:00:00Z"},
			{"company_id": float64(1), "current_score": float64(0), "max_score": float64(0), "created_date_time": "2026-06-01T00:00:00Z"},
		},
	}
	rows := buildADHygiene(data, 0, 0, csNow)
	if rows[0].AzureSecureScorePct != nil || rows[0].AzureSecureScoreRaw != "0/0" {
		t.Fatalf("the newer 0/0 snapshot must clear the old 50%%: pct=%v raw=%s", rows[0].AzureSecureScorePct, rows[0].AzureSecureScoreRaw)
	}
}

func TestBuildQBRCountsResolvedProblems(t *testing.T) {
	recent := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/r/report_queries/resolved_remediation" {
			_, _ = w.Write([]byte(`{"status":true,"data":[{"remediated_on":"` + recent + `","resolution_days":4,"total_problems":8},{"remediated_on":"` + recent + `","resolution_days":10,"total_problems":2}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":true,"data":[]}`))
	}))
	defer srv.Close()
	c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: "t", ConnectsecureUserId: "1"}, 5*time.Second, 0)
	c.NoCache = true
	pack, err := buildQBR(context.Background(), c, "1", "Acme", qbrOpts{since: 90 * 24 * time.Hour, sinceLabel: "90d", now: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if pack.Remediated.Remediations != 2 || pack.Remediated.ProblemsResolved != 10 || pack.Remediated.MedianResolutionDays != 7 {
		t.Fatalf("remediated = %+v; want 2 remediations closing 10 findings, median 7", pack.Remediated)
	}
}

func TestCVEImpactHumanOutputShowsFailedLookup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/r/cve/report" {
			_, _ = w.Write([]byte(`{"status":true,"data":{"cve":"CVE-2024-3400","available_fixes":[{"product":"PAN-OS"}]}}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	testenv.Isolate(t)
	t.Setenv("CONNECTSECURE_BASE_URL", srv.URL)
	t.Setenv("CONNECTSECURE_ACCESS_TOKEN", "tok")
	t.Setenv("CONNECTSECURE_USER_ID", "1")
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--no-cache", "--human-friendly", "cve-impact", "CVE-2024-3400", "--max-pages", "1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("partial failure must still print the report: %v", err)
	}
	if !strings.Contains(out.String(), "affected clients unavailable") || !strings.Contains(out.String(), "failed: get_assets_by_problem") {
		t.Fatalf("human output must show the failed lookup:\n%s", out.String())
	}
	if !strings.Contains(errOut.String(), "warning: 1 lookup(s) failed") {
		t.Fatalf("stderr must carry a warning: line:\n%s", errOut.String())
	}
}

func TestBuildQBRFailedSectionIsNullNotZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/r/report_queries/resolved_remediation", "/r/report_queries/risk_score":
			w.WriteHeader(http.StatusNotFound)
		default:
			_, _ = w.Write([]byte(`{"status":true,"data":[]}`))
		}
	}))
	defer srv.Close()
	c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: "t", ConnectsecureUserId: "1"}, 5*time.Second, 0)
	c.NoCache = true
	pack, err := buildQBR(context.Background(), c, "1", "Acme", qbrOpts{now: time.Now().UTC()})
	if err != nil {
		t.Fatalf("partial failure must not fail the pack: %v", err)
	}
	raw, _ := json.Marshal(pack)
	for _, want := range []string{`"remediated":null`, `"risk_score":null`, `"open_by_age":[]`, `"top_fixes":[]`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("want %s in %s", want, raw)
		}
	}
	if len(pack.FetchFailures) != 2 {
		t.Fatalf("want 2 fetch failures, got %+v", pack.FetchFailures)
	}
}

func TestBuildCVEImpactFailedLookupIsNull(t *testing.T) {
	assets := []csRow{{"host_name": "a1", "company_name": "Acme"}, {"host_name": "x"}}
	report := map[string]any{"available_fixes": []any{map[string]any{"product": "PAN-OS"}}, "summary": map[string]any{"total_affected_assets": float64(3)}}
	boom := fmt.Errorf("HTTP 404")

	failedReport, _ := json.Marshal(buildCVEImpact("CVE-1", nil, boom, assets, false, nil, 20, 10))
	for _, want := range []string{`"fixes":null`, `"reported_assets":null`, `"affected_software":null`, `"attributed_assets":1`, `"unattributed_assets":1`} {
		if !strings.Contains(string(failedReport), want) {
			t.Fatalf("report failure: want %s in %s", want, failedReport)
		}
	}
	failedAssets, _ := json.Marshal(buildCVEImpact("CVE-1", report, nil, nil, false, boom, 20, 10))
	for _, want := range []string{`"clients":null`, `"attributed_assets":null`, `"unattributed_assets":null`, `"reported_assets":3`} {
		if !strings.Contains(string(failedAssets), want) {
			t.Fatalf("asset failure: want %s in %s", want, failedAssets)
		}
	}
	ok := buildCVEImpact("CVE-1", report, nil, assets, false, nil, 20, 10)
	if ok.Fixes == nil || *ok.ReportedAssets != 3 || *ok.AttributedAssets != 1 || len(ok.Clients) != 2 {
		t.Fatalf("both lookups ok: %+v", ok)
	}
	if clean, _ := json.Marshal(buildCVEImpact("CVE-1", map[string]any{}, nil, []csRow{}, false, nil, 20, 10)); !strings.Contains(string(clean), `"fixes":[]`) || !strings.Contains(string(clean), `"clients":[]`) {
		t.Fatalf("a successful empty lookup is [] not null: %s", clean)
	}
}

// TestCsGetRowsRejectsUnexpectedShapes: a list endpoint that answers with a
// non-list or with non-object rows must surface an error (a fetch failure)
// instead of an empty or short result that reads as an all-clear.
func TestCsGetRowsRejectsUnexpectedShapes(t *testing.T) {
	for body, wantErr := range map[string]bool{
		`{"status":true,"data":[{"id":1}]}`:      false,
		`{"status":true,"data":[]}`:              false,
		`{"status":true,"data":null}`:            false,
		`{"status":true,"data":{"id":1}}`:        true,
		`{"status":true,"data":[{"id":1}, "x"]}`: true,
		`{"status":true,"data":"oops"}`:          true,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: "t", ConnectsecureUserId: "1"}, 5*time.Second, 0)
		c.NoCache = true
		_, _, err := csGetRows(context.Background(), c, "/r/x", nil, 100, 1)
		srv.Close()
		if (err != nil) != wantErr {
			t.Fatalf("body %s: err=%v, wantErr=%v", body, err, wantErr)
		}
	}
}

// TestBuildCVEImpactUnexpectedReportShapeIsAFailure: a CVE report that is
// not an object must land in fetch_failures with null counts, not read as
// zero affected assets and no fixes.
func TestBuildCVEImpactUnexpectedReportShapeIsAFailure(t *testing.T) {
	v := buildCVEImpact("CVE-2024-0001", "oops", nil, nil, false, nil, 20, 10)
	if len(v.FetchFailures) != 1 || v.FetchFailures[0].Section != "cve_report" || v.ReportedAssets != nil || v.Fixes != nil {
		t.Fatalf("unexpected report shape must be a fetch failure with null fields, got %+v", v)
	}
	if v := buildCVEImpact("CVE-2024-0001", map[string]any{}, nil, nil, false, nil, 20, 10); len(v.FetchFailures) != 0 || v.ReportedAssets == nil {
		t.Fatalf("an empty object is a valid no-data report, got %+v", v)
	}
}

// TestResolveQBRCompanyChecksLoneSubstringRow: a server that ignores the
// condition must not hand back another client's company.
func TestResolveQBRCompanyChecksLoneSubstringRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":true,"data":[{"id":1,"name":"Beta Law"}]}`))
	}))
	defer srv.Close()
	c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: "t", ConnectsecureUserId: "1"}, 5*time.Second, 0)
	c.NoCache = true
	if id, _, err := resolveQBRCompany(context.Background(), c, "acme", ""); err == nil {
		t.Fatalf("a lone row named Beta Law resolved for acme as id %s", id)
	}
}

type csRecordingGetter struct{ calls int }

func (g *csRecordingGetter) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	g.calls++
	return json.RawMessage(`{"status":true,"data":[]}`), nil
}
func (g *csRecordingGetter) RateLimit() float64 { return 0 }

// TestSyncRefusesSecretResourcesBeforeFetching guards handfixes.json
// sync-refuses-secret-resources.
func TestSyncRefusesSecretResourcesBeforeFetching(t *testing.T) {
	prev := humanFriendly
	humanFriendly = false
	defer func() { humanFriendly = prev }()
	for _, r := range []string{"credentials", "scheduler", "custom_domains"} {
		g := &csRecordingGetter{}
		var events bytes.Buffer
		res := syncResource(context.Background(), g, nil, r, "", true, 0, false, false, nil, &events)
		if res.Err == nil || !strings.Contains(res.Err.Error(), "never stored") {
			t.Errorf("%s: want a never-stored refusal, got %v", r, res.Err)
		}
		if !strings.Contains(events.String(), `"reason":"never_stored_by_design"`) {
			t.Errorf("%s: machine-mode event stream does not say why: %q", r, events.String())
		}
		if g.calls != 0 {
			t.Errorf("%s: fetched %d page(s) of secret rows before discarding them", r, g.calls)
		}
	}
}

// TestDependentWithNoParentsCompletes guards handfixes.json
// dependent-zero-parents-completes: an empty parent table is a complete walk,
// not an attempt doctor reads as never synced.
func TestDependentWithNoParentsCompletes(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dep := dependentResourceDef{Name: "report_queries", ParentTable: "assets", ParentIDParam: "asset_id", PathTemplate: "/r/report_queries/x", PathParams: []dependentPathParamDef{{Param: "asset_id", Field: "id"}}}
	g := &csRecordingGetter{}
	if res := syncDependentResource(context.Background(), g, db, dep, "", true, 0, false, false, nil, nil, 1); res.Err != nil {
		t.Fatal(res.Err)
	}
	if _, last, _, err := db.GetSyncState("report_queries"); err != nil || last.IsZero() {
		t.Fatalf("zero-parent dependent left no completed sync time (err %v)", err)
	}
}

// TestCredentialsPathIsNotAnAuthSignal guards handfixes.json
// auth-hint-reads-body-not-path: a 400 on /r/company/credentials is a plain
// API error, not "auth is missing" with exit 4.
func TestCredentialsPathIsNotAnAuthSignal(t *testing.T) {
	err := &client.APIError{Method: "POST", Path: "/r/company/credentials", StatusCode: 400, Body: `{"message":"name already exists"}`}
	got := classifyAPIError(io.Discard, err, &rootFlags{})
	var ce *cliError
	if !errors.As(got, &ce) || ce.code == 4 {
		t.Fatalf("400 on a credentials path classified as auth: %v", got)
	}
}

// TestResolveQBRCompanyRejectsDuplicateExactNames: two clients with the same
// name must not silently resolve to whichever the API listed first.
func TestResolveQBRCompanyRejectsDuplicateExactNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":true,"data":[{"id":1,"name":"Acme Dental"},{"id":7,"name":"ACME DENTAL"}]}`))
	}))
	defer srv.Close()
	c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: "t", ConnectsecureUserId: "1"}, 5*time.Second, 0)
	c.NoCache = true
	if id, _, err := resolveQBRCompany(context.Background(), c, "acme dental", ""); err == nil || !strings.Contains(err.Error(), "--company-id") {
		t.Fatalf("duplicate exact names resolved to id %q (err %v)", id, err)
	}
}

// TestValidationMessageIsNotAnAuthSignal: "Credential name already exists"
// names the resource; it must not be classified as missing auth (exit 4).
func TestValidationMessageIsNotAnAuthSignal(t *testing.T) {
	err := &client.APIError{Method: "POST", Path: "/w/company/credentials", StatusCode: 400, Body: `{"message":"Credential name already exists"}`}
	var ce *cliError
	if got := classifyAPIError(io.Discard, err, &rootFlags{}); !errors.As(got, &ce) || ce.code == 4 {
		t.Fatalf("validation error classified as auth: %v", got)
	}
}

// TestQBRRiskScoreSharedNameIsUnattributed: the risk-score report carries only
// company_name, so two rows for one name must not yield an arbitrary score.
func TestQBRRiskScoreSharedNameIsUnattributed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/r/report_queries/risk_score" {
			_, _ = w.Write([]byte(`{"status":true,"data":[{"company_name":"Acme","risk_score":"10"},{"company_name":"Acme","risk_score":"90"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":true,"data":[]}`))
	}))
	defer srv.Close()
	c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: "t", ConnectsecureUserId: "1"}, 5*time.Second, 0)
	c.NoCache = true
	pack, err := buildQBR(context.Background(), c, "1", "Acme", qbrOpts{now: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if pack.RiskScore != nil {
		t.Fatalf("shared name produced risk score %q", *pack.RiskScore)
	}
}

// TestPortfolioRiskScoreSharedNameIsBlank: same rule for the local rollup.
func TestPortfolioRiskScoreSharedNameIsBlank(t *testing.T) {
	data := map[string][]csRow{
		csResCompanies: {{"id": float64(1), "name": "Acme"}, {"id": float64(2), "name": "acme"}, {"id": float64(3), "name": "Beta"}},
		csResRiskScore: {{"company_name": "Acme", "risk_score": "90"}, {"company_name": "Beta", "risk_score": "40"}},
	}
	for _, r := range buildPortfolio(data, 24*time.Hour, csNow) {
		switch r.CompanyName {
		case "Acme", "acme":
			if r.RiskScore != "" {
				t.Errorf("company %s (shared name) got risk score %q", r.CompanyID, r.RiskScore)
			}
		case "Beta":
			if r.RiskScore != "40" {
				t.Errorf("Beta risk score = %q, want 40", r.RiskScore)
			}
		}
	}
}

// TestQBRRiskScoreOneRowSharedNameIsUnattributed: a single name-keyed risk
// row cannot be attributed when two companies carry that name.
func TestQBRRiskScoreOneRowSharedNameIsUnattributed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/r/report_queries/risk_score":
			_, _ = w.Write([]byte(`{"status":true,"data":[{"company_name":"Acme","risk_score":"80"}]}`))
		case "/r/company/companies":
			_, _ = w.Write([]byte(`{"status":true,"data":[{"id":11,"name":"Acme"},{"id":12,"name":"Acme"}]}`))
		default:
			_, _ = w.Write([]byte(`{"status":true,"data":[]}`))
		}
	}))
	defer srv.Close()
	c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: "t", ConnectsecureUserId: "1"}, 5*time.Second, 0)
	c.NoCache = true
	pack, err := buildQBR(context.Background(), c, "12", "Acme", qbrOpts{now: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if pack.RiskScore != nil {
		t.Fatalf("shared name with one report row produced risk score %q", *pack.RiskScore)
	}
}

// TestStatusFalseMessageWithholdsTheToken: csGetData and doctor print a 200
// status:false message; an echo of the CLI's bearer token must not survive.
func TestStatusFalseMessageWithholdsTheToken(t *testing.T) {
	const tok = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiI0MiJ9.SIGSENTINELxyz0123456789"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":false,"message":"session expired for ` + strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") + `"}`))
	}))
	defer srv.Close()
	c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: tok, ConnectsecureUserId: "1"}, 5*time.Second, 0)
	c.NoCache = true
	if _, err := csGetData(context.Background(), c, "/r/company/companies", nil); err == nil || strings.Contains(err.Error(), "SIGSENTINEL") {
		t.Fatalf("csGetData leaked the token: %v", err)
	}
	report := map[string]any{}
	doctorProbeCredentials(context.Background(), c, nil, report)
	if strings.Contains(fmt.Sprint(report["credentials"]), "SIGSENTINEL") {
		t.Fatalf("doctor leaked the token: %v", report["credentials"])
	}
}

// TestAuthMessageKeepsTheAuthHint: a 400 that names a token is still auth.
func TestAuthMessageKeepsTheAuthHint(t *testing.T) {
	err := &client.APIError{Method: "GET", Path: "/r/company/companies", StatusCode: 400, Body: `{"message":"Authorization token is required"}`}
	var ce *cliError
	if got := classifyAPIError(io.Discard, err, &rootFlags{}); !errors.As(got, &ce) || ce.code != 4 {
		t.Fatalf("auth message lost its auth classification: %v", got)
	}
}

// TestCVEImpactFlagsSharedClientNames: assets-by-problem rows carry only
// company_name, so a name two companies share is flagged, not merged silently.
func TestCVEImpactFlagsSharedClientNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("condition"), "Acme") {
			_, _ = w.Write([]byte(`{"status":true,"data":[{"id":11,"name":"Acme"},{"id":12,"name":"Acme"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":true,"data":[{"id":13,"name":"Beta"}]}`))
	}))
	defer srv.Close()
	c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: "t", ConnectsecureUserId: "1"}, 5*time.Second, 0)
	c.NoCache = true
	view := cveImpactView{Clients: []cveClient{{CompanyName: "Acme", AssetCount: 3}, {CompanyName: "Beta", AssetCount: 1}}}
	markSharedCVEClientNames(context.Background(), c, &view)
	if view.Clients[0].SharedBy != 2 || view.Clients[1].SharedBy != 0 {
		t.Fatalf("shared-name flags = %d/%d, want 2/0", view.Clients[0].SharedBy, view.Clients[1].SharedBy)
	}
}

// TestLocalReportResourcesAreReplacedOnSync keeps the store's replace list in
// step with the resources the local reports read.
func TestLocalReportResourcesAreReplacedOnSync(t *testing.T) {
	for _, r := range connectsecureDefaultSyncResources() {
		if !store.ConnectsecureReplaceOnSync(r) {
			t.Errorf("%s is read by a local report but would be keep-richer merged on sync", r)
		}
	}
}

// csOneTagGetter answers every page with one tag, id 1.
type csOneTagGetter struct{}

func (csOneTagGetter) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	return json.RawMessage(`{"status":true,"data":[{"id":1,"tag":"a"}]}`), nil
}
func (csOneTagGetter) RateLimit() float64 { return 0 }

// TestFilteredFullSyncKeepsOtherRows guards handfixes.json
// filtered-sync-no-prune: `sync --full --param condition=...` walks a subset,
// so it must not prune the rows it never asked for nor advance the watermark.
func TestFilteredFullSyncKeepsOtherRows(t *testing.T) {
	prev := humanFriendly
	humanFriendly = false
	defer func() { humanFriendly = prev }()
	for _, tc := range []struct {
		name     string
		params   *syncUserParams
		wantKept bool
	}{
		{"filtered", &syncUserParams{flatGlobal: map[string]string{"condition": "company_id=12"}}, true},
		{"unfiltered", nil, false},
	} {
		db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Upsert("tags", "2", json.RawMessage(`{"id":2,"tag":"other client"}`)); err != nil {
			t.Fatal(err)
		}
		var events bytes.Buffer
		if res := syncResource(context.Background(), csOneTagGetter{}, db, "tags", "", true, 0, false, true, tc.params, &events); res.Err != nil {
			t.Fatalf("%s: %v", tc.name, res.Err)
		}
		row, _ := db.Get("tags", "2")
		if kept := row != nil; kept != tc.wantKept {
			t.Errorf("%s: other client's row kept=%v, want %v (events %s)", tc.name, kept, tc.wantKept, events.String())
		}
		_, last, _, _ := db.GetSyncState("tags")
		if advanced := !last.IsZero(); advanced == tc.wantKept {
			t.Errorf("%s: watermark advanced=%v", tc.name, advanced)
		}
		db.Close()
	}
}

// TestDoctorWritesNoCache guards handfixes.json doctor-html-probe-uncached:
// the HTML reachability probe skips the echo guard, so an HTML page echoing
// the bearer token must never land in the on-disk response cache.
func TestDoctorWritesNoCache(t *testing.T) {
	const tok = "DOCTORCACHESENTINEL_0123456789"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>Pod</title></head><body>header was ` + r.Header.Get("Authorization") + `</body></html>`))
			return
		}
		_, _ = w.Write([]byte(`{"status":true,"data":[]}`))
	}))
	defer srv.Close()
	home := t.TempDir()
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME", "CONNECTSECURE_HOME"} {
		t.Setenv(k, home)
	}
	t.Setenv("CONNECTSECURE_BASE_URL", srv.URL)
	t.Setenv("CONNECTSECURE_ACCESS_TOKEN", tok)
	t.Setenv("CONNECTSECURE_USER_ID", "7")
	root := RootCmd()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"doctor", "--json"})
	_ = root.Execute()
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, rerr := os.ReadFile(p); rerr == nil && bytes.Contains(b, []byte(tok)) {
				t.Errorf("doctor wrote the bearer token to %s", p)
			}
		}
		return nil
	})
}

// TestTailIsNotAnMCPReadOnlyTool guards handfixes.json mcp-tail-not-read-only.
func TestTailIsNotAnMCPReadOnlyTool(t *testing.T) {
	if v := newTailCmd(&rootFlags{}).Annotations["mcp:read-only"]; v == "true" {
		t.Fatal("tail streams credential resources but is annotated mcp:read-only")
	}
}

// TestDoctorEchoingRejectionIsAnError: a 200 status:false that echoes the
// token becomes a withheld APIError; doctor must still call it rejected.
func TestDoctorEchoingRejectionIsAnError(t *testing.T) {
	const tok = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiI0MiJ9.SIGSENTINELabc0123456789"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":false,"message":"token ` + strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") + ` revoked"}`))
	}))
	defer srv.Close()
	c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: tok, ConnectsecureUserId: "1"}, 5*time.Second, 0)
	c.NoCache = true
	report := map[string]any{}
	doctorProbeCredentials(context.Background(), c, nil, report)
	got := fmt.Sprint(report["credentials"])
	if !strings.HasPrefix(got, "ERROR rejected") || strings.Contains(got, "SIGSENTINEL") {
		t.Fatalf("echoing rejection: %q", got)
	}
}

// TestDoctorRequiresTheStatusEnvelope guards doctor-verdict-tracks-reality: a
// 200 that is not the {"status": ...} envelope (a gateway page, plaintext,
// `{}`) never verifies the credential.
func TestDoctorRequiresTheStatusEnvelope(t *testing.T) {
	for body, wantErr := range map[string]bool{
		`Access denied`:                         true,
		`{}`:                                    true,
		`{"status":true,"data":[{"id":1}]}`:     false,
		`{"data":[],"items":[],"results":[]}`:   false, // check_doctor_truth's healthy stub
		`{"data":null}`:                         true,
		`{"message":"Unauthorized","data":"x"}`: true,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		c := client.New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
		c.NoCache = true
		report := map[string]any{}
		doctorProbeCredentials(context.Background(), c, nil, report)
		srv.Close()
		if got := fmt.Sprint(report["credentials"]); strings.HasPrefix(got, "ERROR") != wantErr {
			t.Errorf("body %q: credentials = %q, want ERROR=%v", body, got, wantErr)
		}
	}
}

// TestWebhookDeliveryRefusesRedirect guards deliver-webhook-no-redirect: a
// 307/308 would replay the delivered body to wherever the server points.
func TestWebhookDeliveryRefusesRedirect(t *testing.T) {
	hit := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	if err := deliverWebhook(srv.URL, []byte(`{"password":"x"}`), false); err == nil || hit {
		t.Fatalf("webhook followed a redirect: hit=%v err=%v", hit, err)
	}
}

// TestExportDeliversRecordsToFile guards export-tail-honor-deliver: export
// writes through the command's writer, so --deliver file: receives the rows.
func TestExportDeliversRecordsToFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":true,"data":[{"id":1,"name":"Acme Corp"}],"total":1}`))
	}))
	defer srv.Close()
	home := t.TempDir()
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME", "CONNECTSECURE_HOME"} {
		t.Setenv(k, home)
	}
	t.Setenv("CONNECTSECURE_BASE_URL", srv.URL)
	t.Setenv("CONNECTSECURE_ACCESS_TOKEN", "tok_export_test")
	t.Setenv("CONNECTSECURE_USER_ID", "7")
	out := filepath.Join(home, "out.jsonl")
	// Delivery runs in Execute (the real entrypoint), which reads os.Args.
	prevArgs := os.Args
	os.Args = []string{"connectsecure-cli", "export", "companies", "--deliver", "file:" + out}
	defer func() { os.Args = prevArgs }()
	if err := Execute(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); !bytes.Contains(b, []byte("Acme Corp")) {
		t.Fatalf("delivered file lacks the exported record: %q", b)
	}
}

// TestFilteredSyncKeepsTheResumeCursor guards filtered-sync-cursor-isolated:
// a --param walk's offsets index a subset, so it never moves the
// resource-wide resume point a later unfiltered sync continues from.
func TestFilteredSyncKeepsTheResumeCursor(t *testing.T) {
	prev := humanFriendly
	humanFriendly = false
	defer func() { humanFriendly = prev }()
	for _, tc := range []struct {
		name   string
		params *syncUserParams
		want   bool // cursor still "OLD-OFFSET"
	}{
		{"filtered", &syncUserParams{flatGlobal: map[string]string{"condition": "company_id=12"}}, true},
		{"unfiltered", nil, false},
	} {
		db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
		if err != nil {
			t.Fatal(err)
		}
		_ = db.Upsert("tags", "2", json.RawMessage(`{"id":2,"tag":"other client"}`))
		if err := db.SaveSyncProgress("tags", "OLD-OFFSET", 1); err != nil {
			t.Fatal(err)
		}
		var events bytes.Buffer
		if res := syncResource(context.Background(), csOneTagGetter{}, db, "tags", "", false, 0, false, false, tc.params, &events); res.Err != nil {
			t.Fatalf("%s: %v", tc.name, res.Err)
		}
		cur, _, _, _ := db.GetSyncState("tags")
		if kept := cur == "OLD-OFFSET"; kept != tc.want {
			t.Errorf("%s: resume cursor = %q", tc.name, cur)
		}
		db.Close()
	}
}

// csRefusedGetter answers every page the way a pod refuses a read.
type csRefusedGetter struct{}

func (csRefusedGetter) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	return json.RawMessage(`{"status":false,"message":"Permission denied","data":[]}`), nil
}
func (csRefusedGetter) RateLimit() float64 { return 0 }

// TestSyncStatusFalseIsAFailure guards sync-status-false-is-a-failure: a 200
// {"status": false, "data": []} is a refusal, not an empty resource, so a full
// sync must fail rather than prune the cached rows and mark itself complete.
func TestSyncStatusFalseIsAFailure(t *testing.T) {
	prev := humanFriendly
	humanFriendly = false
	defer func() { humanFriendly = prev }()
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_ = db.Upsert("tags", "101", json.RawMessage(`{"id":101,"tag":"cached"}`))
	var events bytes.Buffer
	res := syncResource(context.Background(), csRefusedGetter{}, db, "tags", "", true, 0, false, true, nil, &events)
	if row, _ := db.Get("tags", "101"); res.Err == nil || row == nil {
		t.Fatalf("status:false sync err=%v, cached row kept=%v", res.Err, row != nil)
	}
}

// csTwoOUGetter answers with two clients' OUs that share a name.
type csTwoOUGetter struct{}

func (csTwoOUGetter) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	return json.RawMessage(`{"status":true,"data":[{"name":"Workstations","company_id":1,"object_guid":"guid-a"},{"name":"Workstations","company_id":2,"object_guid":"guid-b"}]}`), nil
}
func (csTwoOUGetter) RateLimit() float64 { return 0 }

// TestSyncKeepsEveryClientsSameNamedOU guards per-client-report-keys on the
// sync path (Codex round 14's repro): both clients' "Workstations" OU survive.
func TestSyncKeepsEveryClientsSameNamedOU(t *testing.T) {
	prev := humanFriendly
	humanFriendly = false
	defer func() { humanFriendly = prev }()
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var events bytes.Buffer
	if res := syncResource(context.Background(), csTwoOUGetter{}, db, "report_queries-ad-ous-view", "", true, 0, false, true, nil, &events); res.Err != nil {
		t.Fatal(res.Err)
	}
	if n, _ := db.Count("report_queries-ad-ous-view"); n != 2 {
		t.Fatalf("stored %d OU rows, want 2 (events %s)", n, events.String())
	}
}

// TestQBRComplianceCountsChecksNotBuckets: compliance rows are per check x
// status buckets with asset_count. One failing asset fails the check; a
// failing bucket with zero assets does not (Codex round 21).
func TestQBRComplianceCountsChecksNotBuckets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/r/report_queries/compliance_check_company_count" {
			_, _ = w.Write([]byte(`{"status":true,"data":[
				{"compliance_id":"C1","compliance_status":"pass","asset_count":99},
				{"compliance_id":"C1","compliance_status":"fail","asset_count":1},
				{"compliance_id":"C2","compliance_status":"pass","asset_count":50},
				{"compliance_id":"C3","compliance_status":"fail","asset_count":0}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":true,"data":[]}`))
	}))
	defer srv.Close()
	c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: "t", ConnectsecureUserId: "1"}, 5*time.Second, 0)
	c.NoCache = true
	pack, err := buildQBR(context.Background(), c, "1", "Acme", qbrOpts{now: time.Now().UTC(), complianceType: "cis", platform: "windows", section: "1"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(pack.Compliance)
	want := `{"assets_not_pass":1,"assets_pass":149,"checks":3,"compliance_type":"cis","not_pass":1,"pass":2}`
	if string(got) != want {
		t.Fatalf("compliance = %s, want %s", got, want)
	}
}
