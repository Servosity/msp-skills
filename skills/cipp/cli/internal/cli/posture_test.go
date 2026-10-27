// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"cipp-pp-cli/internal/store"
)

// seedUser stores one user row stamped with a tenant and MFA state.
func seedUser(t *testing.T, db *store.Store, id, tenant string, mfa bool) {
	t.Helper()
	obj := map[string]any{
		"id":                id,
		"_tenant":           tenant,
		"userPrincipalName": id + "@" + tenant,
		"MFARegistration":   mfa,
	}
	raw, _ := json.Marshal(obj)
	// MFA registration lives on /ListMFAUsers rows, stored as mfausers.
	if err := db.Upsert("mfausers", id, raw); err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

func TestPostureMFAMatrix(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Tenant A: 2 users, 1 with MFA. Tenant B: 1 user, MFA on.
	seedUser(t, db, "a1", "contoso.onmicrosoft.com", true)
	seedUser(t, db, "a2", "contoso.onmicrosoft.com", false)
	seedUser(t, db, "b1", "fabrikam.onmicrosoft.com", true)
	db.Close()

	var flags rootFlags
	flags.asJSON = true
	cmd := newNovelPostureCmd(&flags)
	cmd.SetArgs([]string{"--dimension", "mfa", "--db", dbPath})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var rows []postureRow
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("unmarshal output %q: %v", out.String(), err)
	}
	got := map[string]map[string]int{}
	for _, r := range rows {
		got[r.Tenant] = r.Metrics
	}
	contoso, ok := got["contoso.onmicrosoft.com"]
	if !ok {
		t.Fatalf("missing contoso row; output=%s", out.String())
	}
	if contoso["users"] != 2 || contoso["mfa_enabled"] != 1 || contoso["mfa_missing"] != 1 {
		t.Fatalf("contoso metrics wrong: %#v (output=%s)", contoso, out.String())
	}
	fabrikam := got["fabrikam.onmicrosoft.com"]
	if fabrikam["users"] != 1 || fabrikam["mfa_enabled"] != 1 || fabrikam["mfa_missing"] != 0 {
		t.Fatalf("fabrikam metrics wrong: %#v", fabrikam)
	}
	fmt.Printf("posture mfa matrix: contoso=%v fabrikam=%v\n", contoso, fabrikam)
}

func TestPostureHonestEmpty(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.Close() // empty store

	var flags rootFlags
	flags.asJSON = true
	cmd := newNovelPostureCmd(&flags)
	cmd.SetArgs([]string{"--dimension", "mfa", "--db", dbPath})
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute (empty) should not error, got: %v", err)
	}

	var rows []postureRow
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("unmarshal empty output %q: %v", out.String(), err)
	}
	if len(rows) != 0 {
		t.Fatalf("empty store should yield 0 rows, got %d", len(rows))
	}
	if errBuf.Len() == 0 {
		t.Fatalf("expected honest-empty hint on stderr, got nothing")
	}
	fmt.Printf("posture honest-empty stderr: %s", errBuf.String())
}

// TestPostureDimensionsMatchFanoutResourceTypes locks the invariant whose
// violation shipped a permanently-empty dimension: every posture dimension's
// resourceType must equal what resourceTypeForEndpoint() derives from that
// dimension's own listEndpoint, because that is the key fanout --save writes.
// The pre-4.30 build declared "bestpracticeanalyser" while fanout wrote "bpa",
// so `posture --dimension bpa` could never return a row against a healthy store.
func TestPostureDimensionsMatchFanoutResourceTypes(t *testing.T) {
	for name, dim := range postureDimensions {
		want := resourceTypeForEndpoint(dim.listEndpoint)
		if dim.resourceType != want {
			t.Errorf("dimension %q: resourceType = %q, but fanout --endpoint %s writes %q; the dimension can never return a row",
				name, dim.resourceType, dim.listEndpoint, want)
		}
	}
}

// TestPostureDimensionCoverage asserts the reprint's three new compliance
// dimensions are present alongside the original four.
func TestPostureDimensionCoverage(t *testing.T) {
	for _, want := range []string{"mfa", "ca", "standards", "bpa", "dlp", "retention", "labels"} {
		if _, ok := postureDimensions[want]; !ok {
			t.Errorf("missing posture dimension %q", want)
		}
	}
}

// TestCAPolicyStateBuckets asserts a report-only Conditional Access policy is
// never counted as enforced. Conflating the two is the reason this dimension
// computes a real signal instead of a row count.
func TestCAPolicyStateBuckets(t *testing.T) {
	cases := []struct {
		state string
		want  string
	}{
		{"enabled", "enabled"},
		{"enabledForReportingButNotEnforced", "report_only"},
		{"disabled", "disabled"},
		{"", "unknown"},
		{"something-new", "unknown"},
	}
	for _, c := range cases {
		obj := map[string]any{}
		if c.state != "" {
			obj["state"] = c.state
		}
		if got := caPolicyState(obj); got != c.want {
			t.Errorf("caPolicyState(state=%q) = %q, want %q", c.state, got, c.want)
		}
	}
}

// TestPostureMetricsAlwaysEmitsTotal asserts every dimension emits a total, so a
// tenant holding data never renders as an empty row.
func TestPostureMetricsAlwaysEmitsTotal(t *testing.T) {
	totals := map[string]string{
		"mfa": "users", "ca": "policies", "standards": "standards", "bpa": "checks",
		"dlp": "policies", "retention": "policies", "labels": "policies",
	}
	for dim, totalKey := range totals {
		m := map[string]int{}
		postureMetrics(dim, map[string]any{"id": "x"}, m)
		if m[totalKey] != 1 {
			t.Errorf("dimension %q: expected %s=1, got %d (metrics=%v)", dim, totalKey, m[totalKey], m)
		}
	}
}

// TestComplianceBucketUnknownNotCountedHealthy asserts an un-inspectable payload
// buckets as unknown rather than silently passing.
func TestComplianceBucketUnknownNotCountedHealthy(t *testing.T) {
	got := complianceBucket(map[string]any{"irrelevant": "value"},
		[]string{"Compliant"}, []string{"Deviated"})
	if got != "unknown" {
		t.Errorf("complianceBucket with no known field = %q, want %q", got, "unknown")
	}
}
