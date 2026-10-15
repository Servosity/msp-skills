// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectsecure-pp-cli/internal/cliutil/testenv"
	"connectsecure-pp-cli/internal/store"
)

// Hand-fix (handfixes.json: report-queries-own-partition). Every generated
// report-queries command must cache into its own report's partition (the one
// sync writes), never the shared "report_queries" bucket. The exception is
// registry-problems-remediation-asset-details: sync's dependent of that name
// stores its per-asset rows under "report_queries".
func TestReportQueriesCommandsUseOwnPartition(t *testing.T) {
	files, err := filepath.Glob("report_queries_*.go")
	if err != nil || len(files) < 150 {
		t.Fatalf("found %d report_queries command files (%v)", len(files), err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		sub := strings.TrimSuffix(strings.TrimPrefix(f, "report_queries_"), ".go")
		want := `"auto", "report_queries-` + sub + `", path`
		if sub == "registry-problems-remediation-asset-details" {
			want = `"auto", "report_queries", path`
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(src), want) {
			t.Errorf("%s: want resolvePaginatedReadWithStrategy(..., %s, ...)", f, want)
		}
	}
}

func TestReportQueriesLocalReadReturnsOnlyThatReport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/report_queries/ad_ous_view":
			_, _ = w.Write([]byte(`{"status":true,"data":[{"name":"Workstations","company_id":1,"object_guid":"g-ou-1"},{"name":"Workstations","company_id":2,"object_guid":"g-ou-2"}]}`))
		case "/report_queries/ad_computers_view":
			_, _ = w.Write([]byte(`{"status":true,"data":[{"name":"RECEPTION","company_id":1,"object_guid":"g-pc-1"},{"name":"RECEPTION","company_id":2,"object_guid":"g-pc-2"}]}`))
		case "/r/report_queries/registry_problems_remediation_asset_details":
			_, _ = w.Write([]byte(`{"status":true,"data":[{"id":9001,"problem":"SMBv1 enabled"}]}`))
		case "/r/report_queries/remediation_plan_by_company":
			_, _ = w.Write([]byte(`{"status":true,"data":[{"solution_id":"SOL-1","product":"Google Chrome","company_id":1},{"solution_id":"SOL-2","product":"Mozilla Firefox","company_id":1}]}`))
		case "/report_queries/ad_group_computers":
			_, _ = w.Write([]byte(`{"status":true,"data":[{"name":"PC01","company_id":1,"object_guid":"g-member-1"}]}`))
		case "/report_queries/ad_gpos_view":
			_, _ = w.Write([]byte(`{"status":true,"data":[{"name":"Default Domain Policy","company_id":1,"object_guid":"g-gpo-1"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	testenv.Isolate(t)
	t.Setenv("CONNECTSECURE_BASE_URL", srv.URL)
	t.Setenv("CONNECTSECURE_ACCESS_TOKEN", "tok")
	t.Setenv("CONNECTSECURE_USER_ID", "1")
	run := func(args ...string) []byte {
		cmd := RootCmd()
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, errOut.String())
		}
		return out.Bytes()
	}
	run("--no-cache", "--json", "report-queries", "ad-ous-view")
	run("--no-cache", "--json", "report-queries", "ad-gpos-view")
	run("--no-cache", "--json", "report-queries", "ad-computers-view")
	run("--no-cache", "--json", "report-queries", "registry-problems-remediation-asset-details", "--asset-id", "55")
	run("--no-cache", "--json", "report-queries", "remediation-plan-by-company", "--company-id", "1")
	var got struct {
		Results []map[string]any `json:"results"`
	}
	raw := run("--json", "--data-source", "local", "report-queries", "ad-ous-view")
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("%v: %s", err, raw)
	}
	guids := map[string]bool{}
	for _, row := range got.Results {
		guids[row["object_guid"].(string)] = true
	}
	if len(got.Results) != 2 || !guids["g-ou-1"] || !guids["g-ou-2"] {
		t.Fatalf("local ad-ous-view must return both clients' OUs and nothing else, got %s", raw)
	}
	// Two clients' same-named computers both survive, and the registry rows
	// sync's dependent keeps in "report_queries" are not served as computers.
	got.Results = nil
	raw = run("--json", "--data-source", "local", "report-queries", "ad-computers-view")
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("%v: %s", err, raw)
	}
	if len(got.Results) != 2 || got.Results[0]["name"] != "RECEPTION" || got.Results[1]["name"] != "RECEPTION" {
		t.Fatalf("local ad-computers-view must return both clients' RECEPTION and nothing else, got %s", raw)
	}
	// The group's guid selects which group's members come back; each member
	// row's object_guid is its own. A local read fails closed rather than
	// report an empty group (handfixes.json: local-group-guid-not-row-field).
	run("--no-cache", "--json", "report-queries", "ad-group-computers", "--company-id", "1", "--object-guid", "g-group-1")
	{
		cmd := RootCmd()
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		cmd.SetArgs([]string{"--json", "--data-source", "local", "report-queries", "ad-group-computers", "--company-id", "1", "--object-guid", "g-group-1"})
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "could not apply object_guid") {
			t.Fatalf("local ad-group-computers must refuse the group guid, got err=%v out=%s", err, out.String())
		}
	}
	// Every row of a per-company report shares company_id; each of the
	// client's remediation plans must survive (handfixes.json: per-client-report-keys).
	got.Results = nil
	raw = run("--json", "--data-source", "local", "report-queries", "remediation-plan-by-company", "--company-id", "1")
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("%v: %s", err, raw)
	}
	if len(got.Results) != 2 || got.Results[0]["solution_id"] == got.Results[1]["solution_id"] {
		t.Fatalf("local remediation-plan-by-company must return both of the client's plans, got %s", raw)
	}
}

type csCountingGetter struct{ calls *int }

func (g csCountingGetter) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	*g.calls++
	return json.RawMessage(`{"status":true,"data":[]}`), nil
}
func (csCountingGetter) RateLimit() float64 { return 0 }

// Hand-fix (handfixes.json: curated-default-sync). A bare sync must not run
// the per-asset registry dependent (one request per asset); naming its parent
// still does.
func TestBareSyncSkipsPerAssetDependent(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, _, err := db.UpsertBatch("assets", []json.RawMessage{json.RawMessage(`{"id":55,"host_name":"a"}`)}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if res := syncDependentResources(context.Background(), csCountingGetter{&calls}, db, "", true, 0, false, false, nil, nil, io.Discard, 1); len(res) != 0 || calls != 0 {
		t.Fatalf("bare sync ran %d dependent(s) with %d request(s); want none", len(res), calls)
	}
	if res := syncDependentResources(context.Background(), csCountingGetter{&calls}, db, "", true, 0, false, false, []string{"assets"}, nil, io.Discard, 1); len(res) != 1 || calls == 0 {
		t.Fatalf("naming assets must still run the dependent: %d result(s), %d request(s)", len(res), calls)
	}
}

// Hand-fix (handfixes.json: prune-hand-keyed-reports). A --full sync prunes
// a finding the pod no longer returns from the per-asset reports whose rows
// carry a stable id.
func TestFullSyncPrunesExternalAssetFindings(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/r/report_queries/external_asset_vulnerabilities" {
			_, _ = w.Write([]byte(`{"status":true,"data":[]}`))
			return
		}
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"status":true,"data":[{"asset_id":1,"key":"CVE-1","value":"open"},{"asset_id":2,"key":"CVE-1","value":"open"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":true,"data":[{"asset_id":1,"key":"CVE-1","value":"open"}]}`))
	}))
	defer srv.Close()
	testenv.Isolate(t)
	t.Setenv("CONNECTSECURE_BASE_URL", srv.URL)
	t.Setenv("CONNECTSECURE_ACCESS_TOKEN", "tok")
	t.Setenv("CONNECTSECURE_USER_ID", "1")
	run := func(args ...string) []byte {
		cmd := RootCmd()
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, errOut.String())
		}
		return out.Bytes()
	}
	run("sync", "--full", "--resources", "report_queries-external-asset-vulnerabilities", "--json")
	run("sync", "--full", "--resources", "report_queries-external-asset-vulnerabilities", "--json")
	var got struct {
		Results []map[string]any `json:"results"`
	}
	raw := run("--json", "--data-source", "local", "report-queries", "external-asset-vulnerabilities")
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("%v: %s", err, raw)
	}
	if len(got.Results) != 1 {
		t.Fatalf("--full must prune asset 2's dropped finding, local has %s", raw)
	}
}
