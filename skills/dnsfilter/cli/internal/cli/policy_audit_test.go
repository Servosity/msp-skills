// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
)

// TestNovelPolicyAuditHelpWires smoke-tests that the policy-audit command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelPolicyAuditHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"policy-audit", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("policy-audit --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "policy-audit"} {
		if !strings.Contains(help, want) {
			t.Fatalf("policy-audit --help missing %q in output:\n%s", want, help)
		}
	}
}

// auditAPI serves the policy-audit endpoints. A global (MSP-shared) policy is
// listed only when include_global_policies=true, and then twice, the way a
// listing that repeats a shared policy per client org would.
func auditAPI(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("page[number]") != "" && q.Get("page[number]") != "1" {
			fmt.Fprint(w, `{"data":[]}`)
			return
		}
		switch r.URL.Path {
		case "/v1/organizations/1":
			fmt.Fprint(w, `{"data":{"id":"1","attributes":{"name":"Acme Dental"}}}`)
		case "/v1/categories/all":
			fmt.Fprint(w, `{"data":[{"id":"60","attributes":{"name":"Malware","security":true}}]}`)
		case "/v1/policies/10":
			fmt.Fprint(w, `{"data":`+auditClientPolicy+`}`)
		case "/v1/policies/all":
			items := []string{auditClientPolicy}
			if q.Get("include_global_policies") == "true" {
				items = append(items, auditGlobalPolicy, auditGlobalPolicy)
			}
			fmt.Fprintf(w, `{"data":[%s]}`, strings.Join(items, ","))
		case "/v1/domains/bulk_lookup":
			recs := make([]string, 0)
			for _, d := range strings.Split(q.Get("fqdns"), ",") {
				cat := ""
				if d == "evil.example.com" {
					cat = `{"id":"60"}`
				}
				recs = append(recs, fmt.Sprintf(`{"id":%q,"attributes":{"host":%q},"relationships":{"categories":{"data":[%s]}}}`, d, d, cat))
			}
			fmt.Fprintf(w, `{"data":[%s]}`, strings.Join(recs, ","))
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

const (
	auditClientPolicy = `{"id":"10","attributes":{"name":"Acme Std","organization_id":1,"is_global_policy":false,"whitelist_domains":["ok.example.com"],"blacklist_categories":[60],"allow_unknown_domains":false}}`
	auditGlobalPolicy = `{"id":"90","attributes":{"name":"MSP Shared","organization_id":9,"is_global_policy":true,"whitelist_domains":["evil.example.com"],"blacklist_categories":[],"allow_unknown_domains":false}}`
)

// policy-audit listed /v1/policies/all without include_global_policies=true,
// so a global policy allow-listing a threat domain and leaving Malware
// unblocked was never audited: findings [] and no note.
func TestPolicyAuditIncludesGlobalPoliciesOnce(t *testing.T) {
	testenv.Isolate(t)
	srv := auditAPI(t)
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")
	t.Cleanup(func() { humanFriendly = false })
	run := func(args ...string) string {
		humanFriendly = false
		cmd := RootCmd()
		cmd.SetArgs(append(args, "--rate-limit", "0"))
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s%s", args, err, out.String(), errOut.String())
		}
		return out.String()
	}
	for _, scope := range [][]string{nil, {"--org", "1"}, {"--baseline", "10"}} {
		var v auditView
		out := run(append([]string{"policy-audit", "--json"}, scope...)...)
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Fatalf("%v: %v\n%s", scope, err, out)
		}
		if v.PoliciesScanned != 2 {
			t.Fatalf("%v: policies_scanned = %d, want 2 (client policy + the global policy once)", scope, v.PoliciesScanned)
		}
		count := map[string]int{}
		for _, f := range v.Findings {
			count[f.PolicyID+":"+f.Kind]++
			if f.IsGlobal != (f.PolicyID == "90") {
				t.Fatalf("%v: finding %+v has is_global=%v", scope, f, f.IsGlobal)
			}
		}
		want := map[string]int{"90:threat_allowlisted": 1, "90:security_category_not_blocked": 1}
		if len(scope) > 0 && scope[0] == "--baseline" {
			want["90:baseline_category_missing"] = 1
		}
		for k, n := range want {
			if count[k] != n {
				t.Fatalf("%v: %s x%d, want x%d; findings %+v", scope, k, count[k], n, v.Findings)
			}
		}
	}
	human := run("policy-audit", "--human-friendly")
	if !strings.Contains(human, "2 policies scanned") || !strings.Contains(human, `policy 90 "MSP Shared": allow-list entry "evil.example.com" is now categorized "Malware" [global policy, shared across client orgs]`) {
		t.Fatalf("human output does not show the audited global policy:\n%s", human)
	}
}

// A catalog category with no boolean security flag is unknown, not false. A
// "Malware" row without the flag, with a policy that blocks no categories and
// has no allow-list entries, produced findings [] and no note: an all-clear.
func TestPolicyAuditUnclassifiedCategoryIsNotAllClear(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.URL.Query().Get("page[number]"); p != "" && p != "1" {
			fmt.Fprint(w, `{"data":[]}`)
			return
		}
		switch r.URL.Path {
		case "/v1/categories/all":
			fmt.Fprint(w, `{"data":[{"id":"60","attributes":{"name":"Malware"}},{"id":"61","attributes":{"name":"Phishing","security":"yes"}},{"id":"7","attributes":{"name":"Social","security":false}}]}`)
		case "/v1/policies/all":
			fmt.Fprint(w, `{"data":[{"id":"10","attributes":{"name":"Open","organization_id":1,"is_global_policy":false,"whitelist_domains":[],"blacklist_categories":[],"allow_unknown_domains":false}}]}`)
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")
	t.Cleanup(func() { humanFriendly = false })
	run := func(args ...string) string {
		humanFriendly = false
		cmd := RootCmd()
		cmd.SetArgs(append(args, "--rate-limit", "0"))
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s%s", args, err, out.String(), errOut.String())
		}
		return out.String()
	}
	var v auditView
	if err := json.Unmarshal([]byte(run("policy-audit", "--json")), &v); err != nil {
		t.Fatal(err)
	}
	if strings.Join(v.UnclassifiedCategories, ",") != "60,61" || !strings.Contains(v.Note, "Malware (60), Phishing (61)") || !strings.Contains(v.Note, "not an all-clear") {
		t.Fatalf("unclassified categories not reported: unclassified=%v note=%q findings=%+v", v.UnclassifiedCategories, v.Note, v.Findings)
	}
	human := run("policy-audit", "--human-friendly")
	if !strings.Contains(human, "note: 2 categories in the category catalog have no boolean security flag (Malware (60), Phishing (61))") {
		t.Fatalf("human output lacks the unclassified-category note:\n%s", human)
	}
}

// An empty category catalog, or a blocked category id the catalog lacks,
// leaves security status unknown: "0 findings" with no note was an all-clear.
func TestPolicyAuditUnknownCatalogIsNotAllClear(t *testing.T) {
	testenv.Isolate(t)
	catalog := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.URL.Query().Get("page[number]"); p != "" && p != "1" {
			fmt.Fprint(w, `{"data":[]}`)
			return
		}
		switch r.URL.Path {
		case "/v1/categories/all":
			fmt.Fprintf(w, `{"data":[%s]}`, catalog)
		case "/v1/policies/all":
			fmt.Fprint(w, `{"data":[{"id":"10","attributes":{"name":"Std","organization_id":1,"is_global_policy":false,"whitelist_domains":[],"blacklist_categories":[60,99],"allow_unknown_domains":false}}]}`)
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")
	t.Cleanup(func() { humanFriendly = false })
	run := func(args ...string) string {
		humanFriendly = false
		cmd := RootCmd()
		cmd.SetArgs(append(args, "--rate-limit", "0"))
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s%s", args, err, out.String(), errOut.String())
		}
		return out.String()
	}
	audit := func() (auditView, string) {
		var v auditView
		if err := json.Unmarshal([]byte(run("policy-audit", "--json")), &v); err != nil {
			t.Fatal(err)
		}
		return v, run("policy-audit", "--human-friendly")
	}

	v, human := audit()
	if !v.CatalogEmpty || len(v.Findings) != 0 || !strings.Contains(v.Note, "returned no categories") || !strings.Contains(human, "note: The category catalog returned no categories") {
		t.Fatalf("empty catalog not reported: catalog_empty=%v note=%q\n%s", v.CatalogEmpty, v.Note, human)
	}

	catalog = `{"id":"60","attributes":{"name":"Malware","security":true}}`
	v, human = audit()
	if v.CatalogEmpty || strings.Join(v.UnknownCategoryIDs, ",") != "99" || !strings.Contains(human, "note: 1 blocked category ids are not in the category catalog (99)") {
		t.Fatalf("blocked id missing from the catalog not reported: unknown=%v note=%q\n%s", v.UnknownCategoryIDs, v.Note, human)
	}

	catalog += `,{"id":"99","attributes":{"name":"Custom","security":false}}`
	if v, human = audit(); v.CatalogEmpty || len(v.UnknownCategoryIDs) != 0 || v.Note != "" || human != "1 policies scanned, 0 allow-list domains categorized, 0 findings\n" {
		t.Fatalf("a fully known catalog must keep the plain output: %+v\n%q", v, human)
	}
}
