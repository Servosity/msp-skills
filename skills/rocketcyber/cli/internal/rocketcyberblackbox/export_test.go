// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rocketcyberblackbox

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Codex r1 P2 export-hangs-after-api-error: export --output writes through the
// withholding file sink (a filter pipe). An API error after the file is open
// must still close it, or the process waits for that pipe forever on exit.
func TestExportOutputReturnsAfterAnAPIError(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// The fake answers the incident list 401.
	sv.mu.Lock()
	sv.denied["/incidents"] = true
	sv.mu.Unlock()
	cmd := exec.CommandContext(ctx, bbCLI, "export", "incidents", "--output", filepath.Join(e.home, "incidents.jsonl"))
	cmd.Env = e.env
	cmd.Dir = e.home
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("export --output did not return after RocketCyber answered 401:\n%s", out)
	}
	if err == nil {
		t.Fatalf("export --output succeeded on a 401:\n%s", out)
	}
	assertNoEcho(t, "export after a 401", string(out))
	// The export reached the API (an argument error would fail fast too).
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var asked bool
	for _, m := range sv.methods {
		asked = asked || m == "GET /v3/incidents"
	}
	if !asked {
		t.Fatalf("export never asked the API for the incidents; the test did not run:\n%s", out)
	}
}

// pagerServer is a RocketCyber fake whose agent list (GET /v3/agents) holds
// total agents, paged as documented (research rocketcyber-endpoints-detail.txt
// and the Postman collection, GET /agents: pageSize "default 1000, maximum
// 1000", page "default 1"; the list envelope {"totalCount","currentPage",
// "totalPages","data"}): pageSize records (default 1000) of the 1-based page
// (default 1), full pages until the last. Every other read is an empty list.
// It counts the requests it answered and the pages it served.
func pagerServer(t *testing.T, total int) (*server, *int) {
	t.Helper()
	var mu sync.Mutex
	asked := new(int)
	sv := newServer()
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*asked++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		if r.URL.Path != "/v3/agents" {
			fmt.Fprint(w, pagedList(q, 1000, 1000, nil, ""))
			return
		}
		sv.mu.Lock()
		sv.pages["/agents?page="+q.Get("page")+"&pageSize="+q.Get("pageSize")]++
		sv.mu.Unlock()
		var items []string
		for i := 1; i <= total; i++ {
			items = append(items, fmt.Sprintf(`{"id":"AG%05d","customerId":%s,"hostname":"paged-agent-%d.hq-record.example","connectivity":"online"}`, i, bbCustomerID, i))
		}
		fmt.Fprint(w, pagedList(q, 1000, 1000, items, ""))
	}))
	sv.url, sv.caFile = srv.URL+"/v3", caFileFor(t, srv)
	return sv, asked
}

// A wrong result on a documented response shape (not a credential class):
// export walks every page of the agent list, from the vendor's first page
// (page 1), and stops after the last; an MSP with more agents than one page
// (RocketCyber's default and maximum pageSize is 1000) must export every one,
// to a file documented as a backup or migration. The MCP export tool walks
// the same pages (its answer is capped at 60000 bytes, so it must return
// every agent or say it was truncated); sync walks the same list (its own
// pager) and is the control.
//
// Seen failing (seen/export-first-page-only-seen-failing.log in the run dir):
// export sends one request with no page parameters and writes the first
// 1000 agents. Fixed in generated resource_paths.go (rocketcyber handfix
// export-walks-every-page: the paged read configs carry paginationType page).
func TestExportWalksEveryPage(t *testing.T) {
	const total = 2500
	sv, asked := pagerServer(t, total)
	e := newEnv(t, sv, credEnv)
	file := filepath.Join(e.home, "agents.jsonl")
	out := e.run(t, "export", "agents", "--format", "jsonl", "--output", file)
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("export wrote no file: %v\n%s", err, out)
	}
	// countAgents counts the distinct agents s names (raw JSON or a JSON
	// string inside an MCP result).
	countAgents := func(s string) int {
		seen := map[string]bool{}
		for _, m := range regexp.MustCompile(`paged-agent-([0-9]+)\.hq-record\.example`).FindAllStringSubmatch(s, -1) {
			if n, err := strconv.Atoi(m[1]); err == nil && n >= 1 && n <= total {
				seen[m[1]] = true
			}
		}
		return len(seen)
	}
	records := func(s string) int { return strings.Count(s, ".hq-record.example") }
	if n, dup := countAgents(string(b)), records(string(b)); n != total || dup != total {
		sv.mu.Lock()
		t.Errorf("export --output wrote %d distinct of %d agents (%d records, %d requests, pages %v):\n%.600s", n, total, dup, *asked, sv.pages, out)
		sv.mu.Unlock()
	}
	if got := e.run(t, "export", "agents", "--format", "json"); countAgents(got) != total || records(got) != total {
		t.Errorf("export --format json printed %d distinct of %d agents (%d records)", countAgents(got), total, records(got))
	}
	if got := e.mcpCalls(t, []map[string]any{tool("export", map[string]any{"resource": "agents", "format": "json"})}); countAgents(got) != total && !strings.Contains(got, `_pp_truncated`) {
		t.Errorf("the MCP export tool returned %d distinct of %d agents:\n%.300s", countAgents(got), total, got)
	}
	// Control: sync walks every page of the same fake.
	syncOut := e.run(t, "sync", "--resources", "agents", "--json")
	t.Logf("sync control:\n%.1200s", syncOut)
	if !regexp.MustCompile(`"agents"[^}]*"(records|count|total_records|synced)":\s*`+strconv.Itoa(total)+`\b`).MatchString(syncOut) &&
		!strings.Contains(syncOut, fmt.Sprintf(`"total_records": %d`, total)) && !strings.Contains(syncOut, fmt.Sprintf(`"total_records":%d`, total)) {
		sv.mu.Lock()
		t.Errorf("sync did not store every agent (pages %v):\n%.1500s", sv.pages, syncOut)
		sv.mu.Unlock()
	}
}

// A documented response shape (spec.yaml reports export: GET /reportApi,
// response_format csv; research rocketcyber-endpoints-detail.txt reportApi):
// the CSV report prints its rows, from the CLI (text/csv, and a download
// answered as application/octet-stream) and from the MCP reports_export tool.
// The report here repeats no credential (the battery's report does, and is
// withheld whole). Seen failing (seen/reports-csv-refused-seen-failing.log in
// the run dir): the CLI answers "API returned a non-JSON response; expected
// JSON" for text/csv. Fixed in generated promoted_reports.go (rocketcyber
// handfix reports-csv-printed: the read skips the live-JSON guard and prints
// a non-JSON body as it came).
func TestReportsCSVPrintsItsRows(t *testing.T) {
	const row = "HQ-CSV report row"
	csvBody := "appId,deviceId,verdict,detectionDate,value\n60," + bbAgentID + ",malicious,2026-10-29T12:00:00Z," + row + "\n"
	rs := newServer()
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.mu.Lock()
		rs.methods = append(rs.methods, r.Method+" "+r.URL.Path)
		rs.countAuth(r)
		rs.mu.Unlock()
		if r.URL.Path != "/v3/reportApi" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("itemType") == "incidents" {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", `attachment; filename="report.csv"`)
		} else {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		}
		fmt.Fprint(w, csvBody)
	}))
	rs.url, rs.caFile = srv.URL+"/v3", caFileFor(t, srv)
	e := newEnv(t, rs, credEnv)
	for _, item := range []string{"events", "incidents"} {
		got := mcpReplies(e.mcpCalls(t, []map[string]any{tool("reports_export", map[string]any{"item-type": item})}))[1]
		if !strings.Contains(got, row) && !strings.Contains(got, base64.StdEncoding.EncodeToString([]byte(csvBody))) {
			t.Errorf("MCP reports_export (%s) lost the report:\n%.1500s", item, got)
		}
	}
	for _, args := range [][]string{
		{"reports", "--item-type", "events"},
		{"reports", "--item-type", "events", "--json"},
		{"reports", "--item-type", "incidents"},
	} {
		if got := e.run(t, args...); !strings.Contains(got, row) && !strings.Contains(got, base64.StdEncoding.EncodeToString([]byte(csvBody))) {
			t.Errorf("%v lost the report:\n%.1500s", args, got)
		}
	}
}
