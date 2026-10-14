// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectsecure-pp-cli/internal/cliutil/testenv"
)

// Hand-fix (handfixes.json: report-queries-own-partition). Every generated
// report-queries command must cache into its own report's partition (the one
// sync writes), never the shared "report_queries" bucket. ad-computers-view is
// the exception: sync stores its rows under "report_queries".
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
		if sub == "ad-computers-view" {
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
}
