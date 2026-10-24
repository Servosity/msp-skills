// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Regression tests for the review findings on the 2026-10-23 reprint: each
// drives the failing input a reviewer named through the real command.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cipp-pp-cli/internal/cliutil/testenv"
	"cipp-pp-cli/internal/config"
	"cipp-pp-cli/internal/store"
)

// fakeCIPP serves canned bodies per path; each request pops the next body
// for its path (the last one repeats).
type fakeCIPP struct {
	mu     sync.Mutex
	bodies map[string][]string
	seen   []string
}

func (f *fakeCIPP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = append(f.seen, r.URL.Path+"?"+r.URL.RawQuery)
	q := f.bodies[r.URL.Path]
	if len(q) == 0 {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	body := q[0]
	if len(q) > 1 {
		f.bodies[r.URL.Path] = q[1:]
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func startFakeCIPP(t *testing.T, bodies map[string][]string) (*fakeCIPP, string) {
	t.Helper()
	testenv.Isolate(t)
	f := &fakeCIPP{bodies: bodies}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	t.Setenv("CIPP_BASE_URL", srv.URL)
	t.Setenv("CIPP_API_KEY", "test-static-token")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	return f, filepath.Join(t.TempDir(), "data.db")
}

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestFanoutReplacesUnwrapsAndSnapshotsEmpty(t *testing.T) {
	f, db := startFakeCIPP(t, map[string][]string{
		"/ListConditionalAccessPolicies": {
			`{"Results":[{"id":"p1","displayName":"Require MFA","state":"enabled"},{"id":"p2","displayName":"Block legacy","state":"enabled"}]}`,
			`[{"id":"p2","displayName":"Block legacy","state":"disabled"}]`,
			`[]`,
			`{"Results":"Failed to retrieve policies: tenant unreachable"}`,
		},
		"/ListUsers": {`[{"id":"u1","userPrincipalName":"a@t.example"}]`},
	})
	fan := func() {
		t.Helper()
		if _, err := runCLI(t, "fanout", "--endpoint", "/ListConditionalAccessPolicies", "--tenant", "t.example", "--save", "--db", db, "--no-cache"); err != nil {
			t.Fatalf("fanout: %v", err)
		}
	}
	count := func() int {
		t.Helper()
		s, err := store.Open(db)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		rows, err := readResourceRows(s, "conditionalaccesspolicies")
		if err != nil {
			t.Fatal(err)
		}
		return len(rows)
	}

	fan() // Results envelope: two rows, not one wrapper row
	if n := count(); n != 2 {
		t.Fatalf("envelope saved as %d rows, want 2", n)
	}
	fan() // p1 deleted upstream: the store must drop it
	if n := count(); n != 1 {
		t.Fatalf("after p1 was deleted upstream the store holds %d rows, want 1", n)
	}
	fan() // empty response clears the tenant and is still a snapshot
	if n := count(); n != 0 {
		t.Fatalf("empty response left %d rows", n)
	}
	out, err := runCLI(t, "changes", "--resource", "/ListConditionalAccessPolicies", "--db", db, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"removed"`) || !strings.Contains(out, `"p2"`) {
		t.Fatalf("deleting the last policy must show as a removal:\n%s", out)
	}
	// A CIPP failure envelope is an error for that tenant, not data.
	_, err = runCLI(t, "fanout", "--endpoint", "/ListConditionalAccessPolicies", "--tenant", "t.example", "--save", "--db", db, "--no-cache")
	if err == nil {
		t.Fatal("a fanout whose only tenant failed must not exit 0")
	}
	if _, err := runCLI(t, "fanout", "--endpoint", "/ListUsers", "--tenant", "t.example", "--db", db, "--no-cache"); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range f.seen {
		if strings.HasPrefix(s, "/ListUsers?") && strings.Contains(s, "IncludeLogonDetails=true") {
			found = true
		}
	}
	if !found {
		t.Fatalf("/ListUsers must request IncludeLogonDetails so sign-in activity is stored: %v", f.seen)
	}
}

func TestCADiffReportsTenantWithNoPolicies(t *testing.T) {
	_, db := startFakeCIPP(t, map[string][]string{
		"/ListConditionalAccessPolicies": {
			`[{"id":"p1","displayName":"Require MFA"}]`,
			`[{"id":"p1","displayName":"Require MFA"}]`,
			`[]`,
		},
	})
	for _, tenant := range []string{"a.example", "b.example", "c.example"} {
		if _, err := runCLI(t, "fanout", "--endpoint", "/ListConditionalAccessPolicies", "--tenant", tenant, "--save", "--db", db, "--no-cache"); err != nil {
			t.Fatal(err)
		}
	}
	out, err := runCLI(t, "ca", "diff", "--db", db, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rep caDiffReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if rep.Tenants != 3 || len(rep.Deviations) != 1 || rep.Deviations[0].Tenant != "c.example" || len(rep.Deviations[0].Missing) != 1 {
		t.Fatalf("the tenant with no policy at all must be reported missing the baseline: %+v", rep)
	}
}

func TestGDAPCoverageMatchesCustomerTenantID(t *testing.T) {
	_, db := startFakeCIPP(t, map[string][]string{
		"/ListGDAPRelationships": {
			`[{"id":"r1","status":"active","endDateTime":"2099-12-01T00:00:00Z","customer":{"tenantId":"11111111-1111-1111-1111-111111111111","displayName":"Contoso"}}]`,
			`[]`,
		},
	})
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveTenantRefs(s, []tenantRef{
		{DefaultDomainName: "contoso.example", DisplayName: "Contoso", CustomerID: "11111111-1111-1111-1111-111111111111"},
		{DefaultDomainName: "fabrikam.example", DisplayName: "Fabrikam", CustomerID: "22222222-2222-2222-2222-222222222222"},
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	gdap := func() gdapReport {
		t.Helper()
		out, err := runCLI(t, "gdap", "expiring", "--days", "30", "--db", db, "--data-source", "live", "--no-cache", "--json")
		if err != nil {
			t.Fatal(err)
		}
		var rep gdapReport
		if err := json.Unmarshal([]byte(out), &rep); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return rep
	}
	rep := gdap()
	if len(rep.Uncovered) != 1 || rep.Uncovered[0].Tenant != "fabrikam.example" {
		t.Fatalf("only fabrikam lacks a relationship; contoso is covered by its customer tenant id: %+v", rep.Uncovered)
	}
	rep = gdap() // no relationships at all: every stored tenant is uncovered
	if len(rep.Uncovered) != 2 {
		t.Fatalf("with no relationship at all both tenants are uncovered, got %+v", rep.Uncovered)
	}
}

func TestSyntheticIDsKeepRulesOfOneMailboxApart(t *testing.T) {
	out := stampTenant(rawRows(t,
		map[string]any{"UPN": "u@t.example", "Name": "Rule A", "Enabled": true},
		map[string]any{"UPN": "u@t.example", "Name": "Rule B", "Enabled": true},
	), "t.example")
	var a, b map[string]any
	_ = json.Unmarshal(out[0], &a)
	_ = json.Unmarshal(out[1], &b)
	if a["_id"] == b["_id"] {
		t.Fatalf("two rules of one mailbox share _id %v", a["_id"])
	}
}

func TestAuthLoginReplacesStaleHeaderAndRedactsTokenErrors(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("CIPP_API_KEY", "")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"access_token":"minted-bearer-value","expires_in":3600}`))
	}))
	defer srv.Close()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := writeFile(cfgPath, "base_url = 'https://old.example/api'\nauth_header = 'Bearer old-bearer'\n"); err != nil {
		t.Fatal(err)
	}
	login := func() error {
		_, err := runCLI(t, "--config", cfgPath, "auth", "login", "--client-id", "app", "--client-secret", "s3cret-value",
			"--tenant-id", "tenant", "--base-url", "https://new.example/api", "--authority", srv.URL)
		return err
	}
	if err := login(); err != nil {
		t.Fatalf("login: %v", err)
	}
	cfg := loadConfigForTest(t, cfgPath)
	if got := cfg.AuthHeader(); got != "Bearer minted-bearer-value" {
		t.Fatalf("after login the stale auth_header still wins: AuthHeader() = %q", got)
	}
	status = http.StatusBadRequest
	err := login()
	if err == nil || strings.Contains(err.Error(), "minted-bearer-value") || strings.Contains(err.Error(), "s3cret-value") {
		t.Fatalf("a token-bearing error body must be redacted: %v", err)
	}
}

func writeFile(path, body string) error { return os.WriteFile(path, []byte(body), 0o600) }

func loadConfigForTest(t *testing.T, path string) *config.Config {
	t.Helper()
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestFanoutFailureMessageKeepsStoredRows(t *testing.T) {
	_, db := startFakeCIPP(t, map[string][]string{
		"/ListConditionalAccessPolicies": {`[{"id":"p1","displayName":"Require MFA","state":"enabled"}]`, `{"Results":"Access denied"}`},
		"/ListUsers":                     {`{"Results":[{"id":"u1","displayName":"Error Reporting","userPrincipalName":"er@t.example"}]}`},
	})
	args := []string{"fanout", "--endpoint", "/ListConditionalAccessPolicies", "--tenant", "t.example", "--save", "--db", db, "--no-cache"}
	if _, err := runCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, args...); err == nil {
		t.Fatal(`{"Results":"Access denied"} must fail the tenant, not read as an empty collection`)
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := readResourceRows(s, "conditionalaccesspolicies")
	s.Close()
	if len(rows) != 1 {
		t.Fatalf("a failure message deleted the stored policies: %d rows", len(rows))
	}
	// A row whose name contains a failure word is still a row.
	if _, err := runCLI(t, "fanout", "--endpoint", "/ListUsers", "--tenant", "t.example", "--save", "--db", db, "--no-cache"); err != nil {
		t.Fatalf("a user named Error Reporting is data, not a failure: %v", err)
	}
}

func TestReplaceTenantRowsRollsBackOnInsertFailure(t *testing.T) {
	db, _ := openTestStore(t)
	store.TenantScopeResource("conditionalaccesspolicies")
	first := stampTenant(rawRows(t, map[string]any{"id": "p1", "state": "enabled"}), "t.example")
	if _, err := db.ReplaceTenantRows("conditionalaccesspolicies", "t.example", first); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`CREATE TRIGGER reject_policy_insert BEFORE INSERT ON resources WHEN NEW.resource_type='conditionalaccesspolicies' BEGIN SELECT RAISE(ABORT,'write failed'); END;`); err != nil {
		t.Fatal(err)
	}
	second := stampTenant(rawRows(t, map[string]any{"id": "p2", "state": "enabled"}), "t.example")
	if _, err := db.ReplaceTenantRows("conditionalaccesspolicies", "t.example", second); err == nil {
		t.Fatal("the insert failure must surface")
	}
	rows, _ := readResourceRows(db, "conditionalaccesspolicies")
	if len(rows) != 1 || rows[0]["id"] != "p1" {
		t.Fatalf("a failed replace must leave the previous rows intact, got %v", rows)
	}
}

func TestLicensesWasteKeepsSameNameSKUsApart(t *testing.T) {
	db, path := openTestStore(t)
	for i, r := range []map[string]any{
		{"id": "l1", "_tenant": "t", "skuId": "sku-a", "License": "Microsoft 365 E3"},
		{"id": "l2", "_tenant": "t", "skuId": "sku-b", "License": "Microsoft 365 E3"},
		{"id": "u1", "_tenant": "t", "userPrincipalName": "a@t", "accountEnabled": false, "assignedLicenses": []any{map[string]any{"skuId": "sku-a"}}},
		{"id": "u2", "_tenant": "t", "userPrincipalName": "b@t", "accountEnabled": false, "assignedLicenses": []any{map[string]any{"skuId": "sku-b"}}},
	} {
		rt := "licenses"
		if i >= 2 {
			rt = "users"
		}
		raw, _ := json.Marshal(r)
		if err := db.Upsert(rt, r["id"].(string), raw); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	out, err := runCLI(t, "licenses", "waste", "--by", "sku", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []wasteRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if len(rows) != 2 || rows[0].SkuID == rows[1].SkuID || rows[0].LicenseName != "Microsoft 365 E3" {
		t.Fatalf("two SKUs sharing a display name must stay two rows: %+v", rows)
	}
}

func TestSnapshotOrderMixesSecondAndNanoPrecision(t *testing.T) {
	ts := []string{"2026-10-23T19:00:00.500000000Z", "2026-10-23T19:00:00Z"}
	sortSnapshotTimes(ts)
	if ts[0] != "2026-10-23T19:00:00Z" {
		t.Fatalf("second-precision stamp must sort before a later nanosecond stamp: %v", ts)
	}
}

func TestScorecardBaselineMatchesCADiff(t *testing.T) {
	_, db := startFakeCIPP(t, map[string][]string{
		"/ListConditionalAccessPolicies": {
			`[{"id":"p","displayName":"P"},{"id":"q","displayName":"Q"}]`,
			`[{"id":"p","displayName":"P"},{"id":"q","displayName":"Q"}]`,
			`[{"id":"p","displayName":"P"}]`,
			`[]`,
		},
	})
	for _, tenant := range []string{"a.example", "b.example", "c.example", "d.example"} {
		if _, err := runCLI(t, "fanout", "--endpoint", "/ListConditionalAccessPolicies", "--tenant", tenant, "--save", "--db", db, "--no-cache"); err != nil {
			t.Fatal(err)
		}
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := buildScorecard(s, "c.example", "", time.Now())
	s.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, sig := range rep.Signals {
		if sig.Signal == "ca.baseline_diff" {
			row := sig.Result.(caDiffRow)
			if len(row.Missing) != 0 {
				t.Fatalf("Q is held by 2 of 4 tenants (not a majority); scorecard must agree with ca diff: %+v", row)
			}
			return
		}
	}
	t.Fatal("ca.baseline_diff signal missing")
}

func TestFanoutPartialResultWithMessageKeepsRows(t *testing.T) {
	_, db := startFakeCIPP(t, map[string][]string{
		"/ListConditionalAccessPolicies": {
			`[{"id":"p1","state":"enabled"},{"id":"p2","state":"enabled"}]`,
			`{"Results":[{"id":"p1","state":"enabled"},"Failed to retrieve remaining policies"]}`,
		},
	})
	args := []string{"fanout", "--endpoint", "/ListConditionalAccessPolicies", "--tenant", "t.example", "--save", "--db", db, "--no-cache"}
	if _, err := runCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, args...); err == nil {
		t.Fatal("a partial result carrying a failure string must fail the tenant")
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rows, _ := readResourceRows(s, "conditionalaccesspolicies")
	if len(rows) != 2 {
		t.Fatalf("a partial result deleted p2: %d rows left", len(rows))
	}
}

func TestEndpointForResourceIsRunnable(t *testing.T) {
	for rt, want := range map[string]string{"roles": "/ListRoles", "mfausers": "/ListMFAUsers", "mailboxrules": "/ListMailboxRules", "oauthapps": "/ListOauthapps"} {
		got := endpointForResource(rt)
		if got != want {
			t.Errorf("endpointForResource(%q) = %q, want %q", rt, got, want)
		}
		if err := checkFanoutEndpoint(got); err != nil {
			t.Errorf("suggested endpoint %q is not accepted by fanout: %v", got, err)
		}
	}
}
