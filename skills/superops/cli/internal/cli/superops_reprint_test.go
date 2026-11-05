// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"superops-pp-cli/internal/cliutil/testenv"
	"superops-pp-cli/internal/store"
)

// fakeSuperOps serves POST /msp the way the SuperOps MSP API does: each list
// query takes input{pageSize,page} and returns the aliased entity array plus
// listInfo{hasMore,page}.
type fakeSuperOps struct {
	t        *testing.T
	data     map[string][]map[string]any // op name (getTicketList) -> records
	noPaging bool                        // omit listInfo entirely

	mu        sync.Mutex
	pages     map[string][]int
	subdomain []string
	auth      []string
}

var fakeOpRe = regexp.MustCompile(`\b(get\w+List|getKbItems|getWorklogEntries)\s*\(`)

func (f *fakeSuperOps) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if r.Method != http.MethodPost || r.URL.Path != "/msp" || json.Unmarshal(body, &req) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	m := fakeOpRe.FindStringSubmatch(req.Query)
	if m == nil {
		http.Error(w, "unknown op", http.StatusBadRequest)
		return
	}
	op := m[1]
	size, page := 50, 1
	if v, ok := req.Variables["first"].(float64); ok {
		size = int(v)
	}
	if v, ok := req.Variables["page"].(float64); ok {
		page = int(v)
	}
	f.mu.Lock()
	if f.pages == nil {
		f.pages = map[string][]int{}
	}
	f.pages[op] = append(f.pages[op], page)
	f.subdomain = append(f.subdomain, r.Header.Get("CustomerSubDomain"))
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	f.mu.Unlock()

	all := f.data[op]
	start := (page - 1) * size
	end := start + size
	if start > len(all) {
		start = len(all)
	}
	if end > len(all) {
		end = len(all)
	}
	conn := map[string]any{"nodes": all[start:end]}
	if !f.noPaging {
		conn["listInfo"] = map[string]any{"hasMore": end < len(all), "page": page, "pageSize": size, "totalCount": len(all)}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{op: conn}})
}

func runSuperOpsCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	if err != nil {
		return out.String() + errOut.String(), err
	}
	return out.String(), nil
}

func startFakeSuperOps(t *testing.T, f *fakeSuperOps) {
	t.Helper()
	f.t = t
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("SUPEROPS_BASE_URL", srv.URL)
	t.Setenv("SUPEROPS_API_TOKEN", "sop-test-token-4471b9")
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-msp")
}

func superopsStoreRows(t *testing.T, resource string) int {
	t.Helper()
	db, err := store.OpenReadOnly(defaultDBPath("superops-cli"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = ?`, resource).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", resource, err)
	}
	return n
}

func fakeTickets(n int, now time.Time) []map[string]any {
	out := make([]map[string]any, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, map[string]any{
			"ticketId":          fmt.Sprintf("70000000000000%04d", i),
			"displayId":         fmt.Sprintf("T-%d", i),
			"subject":           fmt.Sprintf("ticket %d", i),
			"status":            "Open",
			"client":            map[string]any{"accountId": "1", "name": "Acme"},
			"technician":        map[string]any{"userId": "9", "name": "Dana"},
			"resolutionDueTime": now.Add(-time.Hour).UTC().Format("2006-01-02T15:04:05"),
			"updatedTime":       now.Add(-30 * 24 * time.Hour).UTC().Format("2006-01-02T15:04:05"),
		})
	}
	return out
}

// Before the fix the loop read only Relay pageInfo, which SuperOps never
// returns: 120 tickets synced as 50 and the resource read as complete.
func TestSyncWalksEverySuperOpsListPage(t *testing.T) {
	f := &fakeSuperOps{data: map[string][]map[string]any{"getTicketList": fakeTickets(120, time.Now())}}
	startFakeSuperOps(t, f)
	if out, err := runSuperOpsCLI(t, "sync", "--resources", "tickets", "--full"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := superopsStoreRows(t, "tickets"); got != 120 {
		t.Fatalf("stored %d tickets, want 120 (pages requested %v)", got, f.pages["getTicketList"])
	}
	if got := fmt.Sprint(f.pages["getTicketList"]); got != "[1 2 3]" {
		t.Fatalf("pages requested %s, want [1 2 3]", got)
	}
	for i, sd := range f.subdomain {
		if sd != "acme-msp" {
			t.Fatalf("request %d sent CustomerSubDomain %q, want acme-msp", i, sd)
		}
		if f.auth[i] != "Bearer sop-test-token-4471b9" {
			t.Fatalf("request %d sent Authorization %q", i, f.auth[i])
		}
	}
}

// A page that carries no pagination signal at all must not read as the end.
func TestSyncWithoutPaginationSignalIsIncomplete(t *testing.T) {
	f := &fakeSuperOps{noPaging: true, data: map[string][]map[string]any{"getTicketList": fakeTickets(10, time.Now())}}
	startFakeSuperOps(t, f)
	out, err := runSuperOpsCLI(t, "sync", "--resources", "tickets", "--full")
	if err == nil {
		t.Fatalf("sync without listInfo or pageInfo succeeded; want an incomplete error\n%s", out)
	}
}

// The local novels read the same store sync writes (default path, no --db),
// over a store filled by a multi-page sync.
func TestLocalNovelsRunOnASyncedStore(t *testing.T) {
	now := time.Now()
	tickets := fakeTickets(60, now)
	// One ticket carries its client as a JSON-encoded string (a JSON scalar).
	tickets[0]["client"] = `{"accountId":"2","name":"Globex"}`
	worklogs := []map[string]any{
		{"worklogEntryId": "w1", "billable": true, "timespent": 90, "startTime": now.Add(-48 * time.Hour).UTC().Format("2006-01-02T15:04:05"), "ticket": map[string]any{"ticketId": tickets[1]["ticketId"]}},
		{"worklogEntryId": "w2", "billable": true, "timespent": 30, "startTime": now.Add(-24 * time.Hour).UTC().Format("2006-01-02T15:04:05"), "ticket": map[string]any{"ticketId": tickets[0]["ticketId"]}},
		{"worklogEntryId": "w3", "billable": false, "timespent": 600, "startTime": now.Add(-24 * time.Hour).UTC().Format("2006-01-02T15:04:05"), "ticket": map[string]any{"ticketId": tickets[1]["ticketId"]}},
	}
	assets := []map[string]any{{"assetId": "a1", "name": "SRV-01", "hostName": "srv-01", "patchStatus": "Critical patches missing", "client": map[string]any{"accountId": "1", "name": "Acme"}}}
	alerts := []map[string]any{{"id": "al1", "status": "Open", "asset": map[string]any{"assetId": "a1"}}}
	clients := []map[string]any{{"accountId": "1", "name": "Acme"}}
	f := &fakeSuperOps{data: map[string][]map[string]any{
		"getTicketList": tickets, "getWorklogEntries": worklogs, "getAssetList": assets, "getAlertList": alerts, "getClientList": clients,
	}}
	startFakeSuperOps(t, f)
	if out, err := runSuperOpsCLI(t, "sync", "--resources", "tickets,worklogs,assets,alerts,clients", "--full"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := superopsStoreRows(t, "tickets"); got != 60 {
		t.Fatalf("stored %d tickets, want 60", got)
	}
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"sla-watch", "--json"}, []string{`"breached"`, `"Dana"`}},
		{[]string{"sla-watch", "--by", "client", "--json"}, []string{`"Globex"`, `"Acme"`}},
		{[]string{"unbilled", "--json"}, []string{`"Acme"`, `"Globex"`, `"billableMinutes": 90`}},
		{[]string{"stale-tickets", "--days", "7", "--json"}, []string{`"T-60"`}},
		{[]string{"at-risk-assets", "--json"}, []string{`"SRV-01"`}},
		{[]string{"alert-coverage", "--json"}, []string{`"Acme"`}},
		{[]string{"client-360", "Acme", "--json"}, []string{`"Acme"`, `"T-60"`}},
		{[]string{"context-ticket", "T-2", "--json"}, []string{`"T-2"`, `"w1"`}},
	}
	for _, tc := range cases {
		out, err := runSuperOpsCLI(t, tc.args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", tc.args, err, out)
		}
		for _, w := range tc.want {
			if !strings.Contains(out, w) {
				t.Fatalf("%v output missing %s:\n%s", tc.args, w, out)
			}
		}
	}
}
