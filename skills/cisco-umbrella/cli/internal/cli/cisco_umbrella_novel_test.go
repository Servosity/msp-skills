// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cisco-umbrella-pp-cli/internal/cliutil"
	"cisco-umbrella-pp-cli/internal/cliutil/testenv"
	"cisco-umbrella-pp-cli/internal/store"
)

type recordedReq struct {
	Method, Path, Query, Body, Auth, Org string
}

// fakeUmbrella is a scripted api.umbrella.com: routes are "METHOD /path".
type fakeUmbrella struct {
	t      *testing.T
	mu     sync.Mutex
	reqs   []recordedReq
	routes map[string]func(w http.ResponseWriter, r *http.Request, body []byte)
}

func newFake(t *testing.T) (*fakeUmbrella, *httptest.Server) {
	f := &fakeUmbrella{t: t, routes: map[string]func(http.ResponseWriter, *http.Request, []byte){}}
	f.route("POST /auth/v2/token", func(w http.ResponseWriter, r *http.Request, _ []byte) {
		org := r.Header.Get("X-Umbrella-OrgId")
		writeJSON(w, map[string]any{"token_type": "bearer", "access_token": "tok-" + org, "expires_in": 3600})
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, recordedReq{r.Method, r.URL.Path, r.URL.RawQuery, string(body), r.Header.Get("Authorization"), r.Header.Get("X-Umbrella-OrgId")})
		h := f.routes[r.Method+" "+r.URL.Path]
		f.mu.Unlock()
		if h == nil {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"no Route matched with those values"}`))
			return
		}
		h(w, r, body)
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeUmbrella) route(key string, h func(http.ResponseWriter, *http.Request, []byte)) {
	f.routes[key] = h
}

func (f *fakeUmbrella) static(key string, v any) {
	f.route(key, func(w http.ResponseWriter, _ *http.Request, _ []byte) { writeJSON(w, v) })
}

func (f *fakeUmbrella) forbid(key string) {
	f.route(key, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Access Forbidden"}`))
	})
}

func (f *fakeUmbrella) requests(method, pathPrefix string) []recordedReq {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []recordedReq{}
	for _, r := range f.reqs {
		if r.Method == method && strings.HasPrefix(r.Path, pathPrefix) {
			out = append(out, r)
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func envelope(data any) map[string]any {
	return map[string]any{"status": map[string]any{"code": 200, "text": "OK"}, "meta": map[string]any{"page": 1, "limit": 100}, "data": data}
}

// runUmbrella executes the CLI against the fake server and returns stdout.
func runUmbrella(t *testing.T, srvURL, stdin string, args ...string) (string, error) {
	t.Helper()
	testenv.Isolate(t)
	t.Setenv("CISCO_UMBRELLA_BASE_URL", srvURL)
	t.Setenv("CISCO_UMBRELLA_TOKEN_URL", srvURL+"/auth/v2/token")
	t.Setenv("CISCO_UMBRELLA_API_KEY", "test-key-id")
	t.Setenv("CISCO_UMBRELLA_API_SECRET", "test-key-secret")
	t.Setenv("CISCO_UMBRELLA_ORG_ID", "")
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err != nil {
		t.Logf("stderr: %s", errb.String())
	}
	return out.String(), err
}

func seedPolicies(f *fakeUmbrella, recent string) {
	f.static("GET /policies/v2/destinationlists", envelope([]map[string]any{
		{"id": 11, "name": "Global Block", "access": "block", "isGlobal": true},
		{"id": 22, "name": "Partners", "access": "allow", "isGlobal": false},
	}))
	f.static("GET /policies/v2/destinationlists/11/destinations", envelope([]map[string]any{
		{"id": "101", "destination": "example.com", "type": "domain"},
		{"id": "102", "destination": "198.51.100.7", "type": "ipv4"},
		{"id": "103", "destination": "evil.test", "type": "domain"},
	}))
	f.static("GET /policies/v2/destinationlists/22/destinations", envelope([]map[string]any{
		{"id": "201", "destination": "partner.example.com", "type": "domain", "comment": "vendor portal", "createdAt": recent},
		{"id": "202", "destination": "bad-allow.test", "type": "domain", "createdAt": "2020-01-01 00:00:00"},
	}))
}

func TestVerdictJoinsInvestigateListsAndResolvers(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, time.Now().UTC().Format(time.RFC3339))
	f.static("GET /investigate/v2/domains/categorization/login.example.com", map[string]any{"login.example.com": map[string]any{"status": -1, "security_categories": []string{"Phishing"}, "content_categories": []string{}}})
	f.static("GET /investigate/v2/domains/risk-score/login.example.com", map[string]any{"risk_score": 87})
	f.static("GET /investigate/v2/security/name/login.example.com", map[string]any{"threat_type": "Phishing", "securerank2": -5.5, "found": true})
	f.static("GET /reports/v2/top-identities", envelope([]map[string]any{{"requests": 12, "identity": map[string]any{"label": "laptop-7", "type": map[string]any{"type": "roaming"}}, "counts": map[string]any{"blockedrequests": 12}}}))

	out, err := runUmbrella(t, srv.URL, "", "verdict", "login.example.com", "--json")
	if err != nil {
		t.Fatalf("verdict: %v", err)
	}
	var got []domainVerdict
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	v := got[0]
	if !v.ListsChecked || !v.InvestigateChecked || !v.ResolversChecked || v.Coverage != "complete" {
		t.Fatalf("every section ran, so coverage must be complete: %+v", v)
	}
	if v.Status != "malicious" || v.RiskScore == nil || *v.RiskScore != 87 || v.ThreatType != "Phishing" {
		t.Fatalf("investigate fields wrong: %+v", v)
	}
	if v.BlockLists != 1 || v.AllowLists != 0 || len(v.Lists) != 1 || v.Lists[0].Entry != "example.com" || v.Lists[0].Match != "parent" {
		t.Fatalf("parent-domain block membership wrong: %+v", v.Lists)
	}
	if len(v.ResolvedBy) != 1 || v.ResolvedBy[0].Identity != "laptop-7" || v.ResolvedBy[0].Blocked != 12 {
		t.Fatalf("resolvers wrong: %+v", v.ResolvedBy)
	}
	if q := f.requests("GET", "/reports/v2/top-identities"); len(q) != 1 || !strings.Contains(q[0].Query, "domains=login.example.com") {
		t.Fatalf("top-identities not filtered by domain: %+v", q)
	}
}

func TestVerdictUnrelatedDomainMatchesNoList(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, "")
	f.static("GET /investigate/v2/domains/categorization/unrelated.org", map[string]any{"unrelated.org": map[string]any{"status": 1}})
	f.static("GET /investigate/v2/domains/risk-score/unrelated.org", map[string]any{"risk_score": 3})
	f.static("GET /investigate/v2/security/name/unrelated.org", map[string]any{})
	out, err := runUmbrella(t, srv.URL, "", "verdict", "unrelated.org", "--no-activity", "--json")
	if err != nil {
		t.Fatalf("verdict: %v", err)
	}
	var got []domainVerdict
	_ = json.Unmarshal([]byte(out), &got)
	// "notexample.com"-style lookalikes must not match "example.com"; unrelated must match nothing.
	if len(got) != 1 || got[0].BlockLists != 0 || got[0].AllowLists != 0 || len(got[0].Lists) != 0 || got[0].Status != "safe" {
		t.Fatalf("unrelated domain should match no list: %s", out)
	}
	if len(f.requests("GET", "/reports/")) != 0 {
		t.Fatal("--no-activity still called reports")
	}
}

func TestVerdictDegradesWhenInvestigateForbidden(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, "")
	f.forbid("GET /investigate/v2/domains/categorization/a.evil.test")
	f.forbid("GET /investigate/v2/domains/risk-score/a.evil.test")
	f.forbid("GET /investigate/v2/security/name/a.evil.test")
	out, err := runUmbrella(t, srv.URL, "", "verdict", "a.evil.test", "--no-activity", "--json")
	if err != nil {
		t.Fatalf("a 403 on investigate must degrade, not fail: %v", err)
	}
	var got []domainVerdict
	_ = json.Unmarshal([]byte(out), &got)
	if got[0].InvestigateChecked || !got[0].ListsChecked || got[0].Coverage != "partial" {
		t.Fatalf("a 403 on investigate must clear investigate_checked and keep lists_checked: %s", out)
	}
	if got[0].BlockLists != 1 || len(got[0].Notes) == 0 || !strings.Contains(strings.Join(got[0].Notes, ";"), "investigate scope") {
		t.Fatalf("want list membership plus an investigate-scope note: %s", out)
	}
}

func TestVerdictIPSkipsInvestigate(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, "")
	out, err := runUmbrella(t, srv.URL, "", "verdict", "198.51.100.7", "--no-activity", "--json")
	if err != nil {
		t.Fatalf("verdict ip: %v", err)
	}
	var got []domainVerdict
	_ = json.Unmarshal([]byte(out), &got)
	if got[0].Kind != "ipv4" || got[0].BlockLists != 1 || got[0].Lists[0].Match != "exact" {
		t.Fatalf("IP should match its exact block entry: %s", out)
	}
	if len(f.requests("GET", "/investigate/")) != 0 {
		t.Fatal("IP lookups must not call domain Investigate endpoints")
	}
}

func TestDestinationDiffPreviewThenApply(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, "")
	// Stateful list: adds become visible to the post-add verification read.
	var mu sync.Mutex
	list11 := []map[string]any{
		{"id": "101", "destination": "example.com", "type": "domain"},
		{"id": "102", "destination": "198.51.100.7", "type": "ipv4"},
		{"id": "103", "destination": "evil.test", "type": "domain"},
	}
	f.route("GET /policies/v2/destinationlists/11/destinations", func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		mu.Lock()
		defer mu.Unlock()
		writeJSON(w, envelope(list11))
	})
	f.route("POST /policies/v2/destinationlists/11/destinations", func(w http.ResponseWriter, _ *http.Request, body []byte) {
		var adds []map[string]string
		_ = json.Unmarshal(body, &adds)
		mu.Lock()
		for i, a := range adds {
			list11 = append(list11, map[string]any{"id": fmt.Sprintf("9%02d", i), "destination": a["destination"], "type": "domain"})
		}
		mu.Unlock()
		writeJSON(w, envelope(map[string]any{"id": 11}))
	})
	f.static("DELETE /policies/v2/destinationlists/11/destinations/remove", envelope(map[string]any{"id": 11}))
	file := filepath.Join(t.TempDir(), "block.txt")
	if err := os.WriteFile(file, []byte("# managed blocklist\nEXAMPLE.com.\nhttps://new.test/\nnot a domain\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runUmbrella(t, srv.URL, "", "destination-lists", "diff", "11", "--file", file, "--json")
	if err != nil {
		t.Fatalf("diff preview: %v", err)
	}
	var plan destinationDiff
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	if strings.Join(plan.ToAdd, ",") != "new.test" || plan.Unchanged != 1 || len(plan.ToRemove) != 2 || plan.Applied || len(plan.Skipped) != 1 {
		t.Fatalf("plan wrong: %+v", plan)
	}
	if len(f.requests("POST", "/policies/")) != 0 || len(f.requests("DELETE", "/policies/")) != 0 {
		t.Fatal("preview must not write")
	}

	// --apply refuses while a line is unparsed; apply the cleaned file.
	if _, err := runUmbrella(t, srv.URL, "", "destination-lists", "diff", "11", "--file", file, "--apply", "--json"); err == nil {
		t.Fatal("--apply must refuse a file with an unparsed line")
	}
	clean := filepath.Join(t.TempDir(), "block-clean.txt")
	if err := os.WriteFile(clean, []byte("# managed blocklist\nEXAMPLE.com.\nhttps://new.test/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runUmbrella(t, srv.URL, "", "destination-lists", "diff", "11", "--file", clean, "--apply", "--json"); err != nil {
		t.Fatalf("diff apply: %v", err)
	}
	posts := f.requests("POST", "/policies/v2/destinationlists/11/destinations")
	if len(posts) != 1 || !strings.Contains(posts[0].Body, `"destination":"new.test"`) {
		t.Fatalf("add call wrong: %+v", posts)
	}
	dels := f.requests("DELETE", "/policies/v2/destinationlists/11/destinations/remove")
	if len(dels) != 1 || strings.TrimSpace(dels[0].Body) != "[102,103]" {
		t.Fatalf("remove call must send integer destination IDs: %+v", dels)
	}
}

func TestDestinationDiffNoChangesIsEmptyNotNull(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, "")
	out, err := runUmbrella(t, srv.URL, "example.com\n198.51.100.7\nevil.test\n", "destination-lists", "diff", "11", "--file", "-", "--json")
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if !strings.Contains(out, `"to_add": []`) || !strings.Contains(out, `"to_remove": []`) || !strings.Contains(out, `"unchanged": 3`) {
		t.Fatalf("an in-sync list must report empty arrays and 3 unchanged: %s", out)
	}
}

func TestDestinationDiffNoRemoveNeverDeletes(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, "")
	statefulList(f, 11, []map[string]any{{"id": "101", "destination": "example.com", "type": "domain"}})
	if _, err := runUmbrella(t, srv.URL, "brand-new.test\n", "destination-lists", "diff", "11", "--file", "-", "--no-remove", "--apply", "--json"); err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(f.requests("DELETE", "/policies/")) != 0 {
		t.Fatal("--no-remove deleted entries")
	}
}

func TestFleetHealthPerCustomerTokensAndPartialFailure(t *testing.T) {
	old := fleetMintSpacing
	fleetMintSpacing = 0
	t.Cleanup(func() { fleetMintSpacing = old })
	f, srv := newFake(t)
	f.static("GET /admin/v2/managed/customers", []map[string]any{{"customerId": 111, "customerName": "Acme", "seats": 50}, {"customerId": 222, "customerName": "Globex", "seats": 10}})
	f.route("GET /deployments/v2/roamingcomputers", func(w http.ResponseWriter, r *http.Request, _ []byte) {
		switch r.Header.Get("Authorization") {
		case "Bearer tok-111":
			writeJSON(w, []map[string]any{
				{"name": "old-laptop", "status": "Off", "swgStatus": "Unprotected", "lastSync": "2020-01-01T00:00:00Z"},
				{"name": "new-laptop", "status": "Encrypted", "swgStatus": "Protected", "lastSync": time.Now().UTC().Format(time.RFC3339)},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	f.static("GET /deployments/v2/virtualappliances", []map[string]any{})
	f.static("GET /deployments/v2/tunnelsState", []map[string]any{{"tunnelId": "1", "status": "DOWN"}})

	out, err := runUmbrella(t, srv.URL, "", "fleet", "health", "--stale-days", "7", "--json")
	if err != nil {
		t.Fatalf("fleet health: %v", err)
	}
	var rep fleetReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	if rep.ScannedCustomers != 2 || len(rep.Customers) != 1 || len(rep.FetchFailures) != 1 || rep.FetchFailures[0].Name != "Globex" {
		t.Fatalf("want Acme row + Globex failure: %s", out)
	}
	a := rep.Customers[0]
	if a.Name != "Acme" || a.RoamingTotal != 2 || a.RoamingOff != 1 || a.RoamingStale != 1 || a.SWGUnprotected != 1 || a.TunnelsDown != 1 || a.Issues != 4 {
		t.Fatalf("Acme rollup wrong: %+v", a)
	}
	orgs := map[string]bool{}
	for _, r := range f.requests("POST", "/auth/v2/token") {
		orgs[r.Org] = true
	}
	if !orgs["111"] || !orgs["222"] {
		t.Fatalf("each customer needs its own X-Umbrella-OrgId token mint, got %v", orgs)
	}
}

func TestReportCardComputesPriorWindowDeltas(t *testing.T) {
	f, srv := newFake(t)
	isCurrent := func(r *http.Request) bool {
		var to int64
		_, _ = fmtSscan(r.URL.Query().Get("to"), &to)
		return time.Since(time.UnixMilli(to)) < time.Minute
	}
	f.route("GET /reports/v2/summary", func(w http.ResponseWriter, r *http.Request, _ []byte) {
		n := 60
		if isCurrent(r) {
			n = 100
		}
		writeJSON(w, map[string]any{"data": map[string]any{"requestsblocked": n}, "meta": map[string]any{}})
	})
	f.route("GET /reports/v2/top-threat-types", func(w http.ResponseWriter, r *http.Request, _ []byte) {
		n := 4
		if isCurrent(r) {
			n = 10
		}
		writeJSON(w, map[string]any{"data": []map[string]any{{"threattype": "Phishing", "count": n}}})
	})
	f.static("GET /reports/v2/top-categories", map[string]any{"data": []map[string]any{{"category": map[string]any{"label": "Malware"}, "count": 5}}})
	f.static("GET /reports/v2/deployment-status", map[string]any{"data": []map[string]any{{"label": "Roaming Computers", "count": 3, "activecount": 2}}})

	out, err := runUmbrella(t, srv.URL, "", "report-card", "--from", "-7days", "--json")
	if err != nil {
		t.Fatalf("report-card: %v", err)
	}
	var card reportCard
	if err := json.Unmarshal([]byte(out), &card); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	if len(card.Summary) != 1 || card.Summary[0].Delta == nil || *card.Summary[0].Delta != 40 || *card.Summary[0].Prior != 60 {
		t.Fatalf("summary delta wrong: %+v", card.Summary)
	}
	if len(card.TopThreatTypes) != 1 || card.TopThreatTypes[0].Delta == nil || *card.TopThreatTypes[0].Delta != 6 {
		t.Fatalf("threat delta wrong: %+v", card.TopThreatTypes)
	}
	if card.Window.To-card.Window.From != card.PriorWindow.To-card.PriorWindow.From || card.PriorWindow.To != card.Window.From {
		t.Fatalf("prior window must abut and match the current window: %+v %+v", card.Window, card.PriorWindow)
	}
	// The outgoing windows: current ends now and starts 7 days earlier; prior abuts it.
	sums := f.requests("GET", "/reports/v2/summary")
	if len(sums) != 2 {
		t.Fatalf("want a current and a prior summary request, got %d", len(sums))
	}
	var bounds [][2]int64
	for _, r := range sums {
		q, _ := url.ParseQuery(r.Query)
		var from, to int64
		_, _ = fmtSscan(q.Get("from"), &from)
		_, _ = fmtSscan(q.Get("to"), &to)
		bounds = append(bounds, [2]int64{from, to})
	}
	week := int64(7 * 24 * time.Hour / time.Millisecond)
	cur, prev := bounds[0], bounds[1]
	if cur[1]-cur[0] != week || prev[1] != cur[0] || prev[1]-prev[0] != week || time.Since(time.UnixMilli(cur[1])) > time.Minute {
		t.Fatalf("wrong outgoing windows: current %v prior %v", cur, prev)
	}
	cats := f.requests("GET", "/reports/v2/top-categories")
	if len(cats) == 0 || !strings.Contains(cats[0].Query, "verdict=blocked") {
		t.Fatalf("blocked categories must filter verdict=blocked: %+v", cats)
	}
}

func TestAllowlistAuditFindsRiskyConflictsAndNew(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, time.Now().UTC().Format(time.RFC3339))
	f.static("POST /investigate/v2/domains/categorization", map[string]any{
		"partner.example.com": map[string]any{"status": 1},
		"bad-allow.test":      map[string]any{"status": -1, "security_categories": []string{"Malware"}},
	})
	out, err := runUmbrella(t, srv.URL, "", "allowlist", "audit", "--since", "7d", "--json")
	if err != nil {
		t.Fatalf("allowlist audit: %v", err)
	}
	var rep allowAudit
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	if rep.AllowLists != 1 || rep.AllowEntries != 2 || !rep.Checked {
		t.Fatalf("counts wrong: %s", out)
	}
	if len(rep.Risky) != 1 || rep.Risky[0].Destination != "bad-allow.test" {
		t.Fatalf("risky wrong: %+v", rep.Risky)
	}
	if len(rep.Conflicts) != 1 || rep.Conflicts[0].AllowEntry != "partner.example.com" || rep.Conflicts[0].BlockEntry != "example.com" {
		t.Fatalf("conflicts wrong: %+v", rep.Conflicts)
	}
	if len(rep.NewAllows) != 1 || rep.NewAllows[0].Destination != "partner.example.com" {
		t.Fatalf("new allows wrong (the 2020 entry is outside 7d): %+v", rep.NewAllows)
	}
	body := f.requests("POST", "/investigate/v2/domains/categorization")
	if len(body) != 1 || body[0].Body == "" || !strings.HasPrefix(strings.TrimSpace(body[0].Body), "[") {
		t.Fatalf("bulk categorization must POST a JSON array of domains: %+v", body)
	}
}

func TestHygieneFindingsFromSyncedStore(t *testing.T) {
	testenv.Isolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.UpsertRoamingComputers(json.RawMessage(`{"originId":1,"name":"ghost","status":"Uninstalled","swgStatus":"Unprotected","lastSync":"2020-01-01T00:00:00Z"}`)))
	must(db.UpsertRoamingComputers(json.RawMessage(`{"originId":2,"name":"healthy","status":"Encrypted","swgStatus":"Protected","lastSync":"` + time.Now().UTC().Format(time.RFC3339) + `"}`)))
	must(db.UpsertVirtualAppliances(json.RawMessage(`{"originId":3,"name":"va-1","health":"Warning","isUpgradable":true}`)))
	must(db.UpsertNetworks(json.RawMessage(`{"originId":4,"name":"hq","ipAddress":"203.0.113.0","isVerified":false}`)))
	for _, res := range hygieneSyncResources {
		must(db.SaveSyncState(res, "", 1))
	}
	must(db.Close())

	out, err := runUmbrella(t, "http://127.0.0.1:9", "", "hygiene", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("hygiene: %v", err)
	}
	var rep hygieneReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	for _, k := range []string{"roaming-uninstalled", "roaming-stale", "swg-unprotected", "va-not-healthy", "va-upgrade-available", "network-unverified"} {
		if rep.Counts[k] != 1 {
			t.Fatalf("want one %s finding, counts=%v", k, rep.Counts)
		}
	}
	if rep.Note != "" {
		t.Fatalf("a populated store must not carry the sync note: %q", rep.Note)
	}
	for _, f := range rep.Findings {
		if f.Name == "healthy" {
			t.Fatalf("healthy roaming computer produced a finding: %+v", f)
		}
	}

	out, err = runUmbrella(t, "http://127.0.0.1:9", "", "hygiene", "--db", dbPath, "--kind", "va-not-healthy", "--json")
	if err != nil {
		t.Fatalf("hygiene --kind: %v", err)
	}
	_ = json.Unmarshal([]byte(out), &rep)
	if len(rep.Findings) != 1 || rep.Findings[0].Kind != "va-not-healthy" {
		t.Fatalf("--kind filter wrong: %s", out)
	}
}

func TestHygieneMissingStoreIsEmptyJSON(t *testing.T) {
	out, err := runUmbrella(t, "http://127.0.0.1:9", "", "hygiene", "--db", filepath.Join(t.TempDir(), "absent.db"), "--json")
	if err != nil {
		t.Fatalf("hygiene: %v", err)
	}
	if !strings.Contains(out, `"findings": []`) || !strings.Contains(out, `"note": "nothing synced`) {
		t.Fatalf("missing store must print an empty findings array plus a sync note: %s", out)
	}
}

func fmtSscan(s string, v *int64) (int, error) {
	n, err := json.Number(s).Int64()
	*v = n
	return 1, err
}

// A fleet sweep spans many customers; the root --timeout bounds each request,
// not the whole sweep, or a large MSP loses every row at the deadline.
func TestFleetHealthOutlivesPerRequestTimeout(t *testing.T) {
	old := fleetMintSpacing
	fleetMintSpacing = 400 * time.Millisecond
	t.Cleanup(func() { fleetMintSpacing = old })
	f, srv := newFake(t)
	f.static("GET /admin/v2/managed/customers", []map[string]any{{"customerId": 1, "customerName": "A"}, {"customerId": 2, "customerName": "B"}, {"customerId": 3, "customerName": "C"}})
	f.static("GET /deployments/v2/roamingcomputers", []map[string]any{})
	f.static("GET /deployments/v2/virtualappliances", []map[string]any{})
	f.static("GET /deployments/v2/tunnelsState", []map[string]any{})
	out, err := runUmbrella(t, srv.URL, "", "fleet", "health", "--timeout", "600ms", "--json")
	if err != nil {
		t.Fatalf("fleet health must finish a sweep longer than --timeout: %v", err)
	}
	var rep fleetReport
	_ = json.Unmarshal([]byte(out), &rep)
	if len(rep.Customers) != 3 {
		t.Fatalf("want all 3 customers, got %s", out)
	}
}

// A 403 on one deployment section must be visible, not read as zero problems.
func TestFleetHealthMarksForbiddenSections(t *testing.T) {
	old := fleetMintSpacing
	fleetMintSpacing = 0
	t.Cleanup(func() { fleetMintSpacing = old })
	f, srv := newFake(t)
	f.static("GET /admin/v2/managed/customers", []map[string]any{{"customerId": 7, "customerName": "Initech"}})
	f.static("GET /deployments/v2/roamingcomputers", []map[string]any{})
	f.static("GET /deployments/v2/virtualappliances", []map[string]any{})
	f.forbid("GET /deployments/v2/tunnelsState")
	out, err := runUmbrella(t, srv.URL, "", "fleet", "health", "--json")
	if err != nil {
		t.Fatalf("fleet health: %v", err)
	}
	var rep fleetReport
	_ = json.Unmarshal([]byte(out), &rep)
	if len(rep.Customers) != 1 || len(rep.Customers[0].Skipped) != 1 || !strings.Contains(rep.Customers[0].Skipped[0], "tunnel state") {
		t.Fatalf("forbidden tunnel state must be listed under skipped: %s", out)
	}
}

func TestHygienePartialSyncIsNamed(t *testing.T) {
	testenv.Isolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncState("roaming_computers", "", 0); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	out, err := runUmbrella(t, "http://127.0.0.1:9", "", "hygiene", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("hygiene: %v", err)
	}
	var rep hygieneReport
	_ = json.Unmarshal([]byte(out), &rep)
	if len(rep.NotSynced) != 3 || contains(rep.NotSynced, "roaming_computers") || rep.Note == "" {
		t.Fatalf("a partial sync must name the three unsynced resources: %s", out)
	}
	// The hint must not claim nothing synced, and must not ask to re-sync
	// the resource that already synced.
	if strings.Contains(rep.Note, "nothing synced") || strings.Contains(rep.Note, "--resources roaming_computers") ||
		!strings.Contains(rep.Note, "sync --resources virtual_appliances,tunnels-tunnels-state,networks") {
		t.Fatalf("a partial sync hint must name only the unsynced resources: %q", rep.Note)
	}
}

// When the store exists but none of the four resources ever synced, the note
// keeps the full "nothing synced" hint.
func TestHygieneNoneSyncedKeepsFullHint(t *testing.T) {
	testenv.Isolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncState("tags", "", 0); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	out, err := runUmbrella(t, "http://127.0.0.1:9", "", "hygiene", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("hygiene: %v", err)
	}
	var rep hygieneReport
	_ = json.Unmarshal([]byte(out), &rep)
	if len(rep.NotSynced) != 4 || !strings.Contains(rep.Note, "nothing synced: run 'cisco-umbrella-cli sync --resources roaming_computers,virtual_appliances,tunnels-tunnels-state,networks'") {
		t.Fatalf("an unsynced store must keep the full sync hint: %s", out)
	}
}

// With no credentials the gateway returns 403 for everything; verdict must fail
// as an auth error instead of reporting every section as a missing scope.
func TestNovelCommandsFailFastWithoutCredentials(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, "")
	testenv.Isolate(t)
	t.Setenv("CISCO_UMBRELLA_BASE_URL", srv.URL)
	t.Setenv("CISCO_UMBRELLA_TOKEN_URL", srv.URL+"/auth/v2/token")
	t.Setenv("CISCO_UMBRELLA_API_KEY", "")
	t.Setenv("CISCO_UMBRELLA_API_SECRET", "")
	t.Setenv("CISCO_UMBRELLA_ORG_ID", "")
	for _, args := range [][]string{{"verdict", "example.com", "--json"}, {"allowlist", "audit", "--json"}, {"fleet", "health", "--json"}, {"report-card", "--json"}} {
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		var ce *cliError
		if !errors.As(err, &ce) || ce.code != 4 {
			t.Fatalf("%v without credentials: want auth error (exit 4), got %v", args, err)
		}
	}
	if n := len(f.requests("GET", "/")); n != 0 {
		t.Fatalf("no request may be sent without credentials, saw %d", n)
	}
}

func TestVerdictURLHostAndDistinctListCount(t *testing.T) {
	f, srv := newFake(t)
	f.static("GET /policies/v2/destinationlists", envelope([]map[string]any{{"id": 11, "name": "Global Block", "access": "block"}}))
	// One list holding both the parent and the exact domain still counts as one list.
	f.static("GET /policies/v2/destinationlists/11/destinations", envelope([]map[string]any{
		{"id": "1", "destination": "example.com", "type": "domain"},
		{"id": "2", "destination": "login.example.com", "type": "domain"},
	}))
	f.static("GET /investigate/v2/domains/categorization/login.example.com", map[string]any{"login.example.com": map[string]any{"status": 1}})
	f.static("GET /investigate/v2/domains/risk-score/login.example.com", map[string]any{"risk_score": 1})
	f.static("GET /investigate/v2/security/name/login.example.com", map[string]any{})
	out, err := runUmbrella(t, srv.URL, "", "verdict", "https://login.example.com:8443/path?x=1", "--no-activity", "--json")
	if err != nil {
		t.Fatalf("verdict: %v", err)
	}
	var got []domainVerdict
	_ = json.Unmarshal([]byte(out), &got)
	if len(got) != 1 || got[0].BlockLists != 1 || len(got[0].Lists) != 2 || got[0].Status != "safe" {
		t.Fatalf("want one block list (two entry hits) and categorization of the bare host: %s", out)
	}
	if q := f.requests("GET", "/investigate/v2/domains/categorization/"); len(q) != 1 || q[0].Path != "/investigate/v2/domains/categorization/login.example.com" {
		t.Fatalf("Investigate must be asked about the bare host, not host:port?query: %+v", q)
	}
}

func TestCategorizationUnparseableIsAnError(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, "")
	f.route("POST /investigate/v2/domains/categorization", func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`"maintenance"`))
	})
	if _, err := runUmbrella(t, srv.URL, "", "allowlist", "audit", "--json"); err == nil {
		t.Fatal("an unparseable categorization body must fail, not report a clean audit")
	}
}

func TestAllowlistAuditListsUncategorized(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, "")
	f.static("POST /investigate/v2/domains/categorization", map[string]any{"partner.example.com": map[string]any{"status": 1}})
	out, err := runUmbrella(t, srv.URL, "", "allowlist", "audit", "--json")
	if err != nil {
		t.Fatalf("allowlist audit: %v", err)
	}
	var rep allowAudit
	_ = json.Unmarshal([]byte(out), &rep)
	if rep.Checked || len(rep.Uncategorized) != 1 || rep.Uncategorized[0] != "bad-allow.test" {
		t.Fatalf("a domain Investigate skipped must be listed and clear categorization_checked: %s", out)
	}
}

func TestDomainBatchesRespectCountAndBytes(t *testing.T) {
	many := make([]string, 2500)
	for i := range many {
		many[i] = "d.test"
	}
	if b := domainBatches(many); len(b) != 3 || len(b[0]) != 1000 || len(b[2]) != 500 {
		t.Fatalf("count batching wrong: %d batches", len(b))
	}
	long := make([]string, 900)
	for i := range long {
		long[i] = strings.Repeat("a", 200) + ".test"
	}
	for _, batch := range domainBatches(long) {
		raw, _ := json.Marshal(batch)
		if len(raw) >= 100_000 {
			t.Fatalf("batch of %d domains is %d bytes; the documented limit is 100KB", len(batch), len(raw))
		}
	}
}

func TestFleetHealthStopsWhenVAPagingIsIgnored(t *testing.T) {
	old := fleetMintSpacing
	fleetMintSpacing = 0
	t.Cleanup(func() { fleetMintSpacing = old })
	f, srv := newFake(t)
	f.static("GET /admin/v2/managed/customers", []map[string]any{{"customerId": 5, "customerName": "Umbrella Corp"}})
	f.static("GET /deployments/v2/roamingcomputers", []map[string]any{})
	vas := make([]map[string]any, 100)
	for i := range vas {
		vas[i] = map[string]any{"originId": 1000 + i, "health": "healthy"}
	}
	f.static("GET /deployments/v2/virtualappliances", vas) // same full page for every ?page=
	f.static("GET /deployments/v2/tunnelsState", []map[string]any{})
	out, err := runUmbrella(t, srv.URL, "", "fleet", "health", "--json")
	if err != nil {
		t.Fatalf("fleet health: %v", err)
	}
	var rep fleetReport
	_ = json.Unmarshal([]byte(out), &rep)
	if len(rep.Customers) != 1 || rep.Customers[0].VATotal != 100 || len(rep.Customers[0].Skipped) != 0 {
		t.Fatalf("a repeated VA page must stop paging without double counting: %s", out)
	}
	if n := len(f.requests("GET", "/deployments/v2/virtualappliances")); n != 2 {
		t.Fatalf("want exactly 2 VA page requests, got %d", n)
	}
}

func TestOrgFromProfileScopesTheRun(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("CISCO_UMBRELLA_ORG_ID", "")
	t.Setenv("CISCO_UMBRELLA_API_KEY", "test-key-id")
	t.Setenv("CISCO_UMBRELLA_API_SECRET", "test-key-secret")
	run := func(args ...string) {
		t.Helper()
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out.String())
		}
	}
	run("profile", "save", "acme", "--org", "8675309")
	_ = os.Unsetenv("CISCO_UMBRELLA_ORG_ID")
	run("networks", "list", "--profile", "acme", "--dry-run")
	if got := os.Getenv("CISCO_UMBRELLA_ORG_ID"); got != "8675309" {
		t.Fatalf("an org set by a saved profile must scope the run; env=%q", got)
	}
	if _, legacy, _ := currentDefaultDBScope(); legacy != "" {
		t.Fatalf("a child-org scope must never adopt the legacy store, legacy hash %q", legacy)
	}
}

func TestVerdictScoresForbiddenMakesCoveragePartial(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, "")
	f.static("GET /investigate/v2/domains/categorization/ok.test", map[string]any{"ok.test": map[string]any{"status": 1}})
	f.forbid("GET /investigate/v2/domains/risk-score/ok.test")
	f.static("GET /investigate/v2/security/name/ok.test", map[string]any{"found": true})
	out, err := runUmbrella(t, srv.URL, "", "verdict", "ok.test", "--no-activity", "--json")
	if err != nil {
		t.Fatalf("verdict: %v", err)
	}
	var got []domainVerdict
	_ = json.Unmarshal([]byte(out), &got)
	if got[0].ScoresChecked || got[0].Coverage != "partial" {
		t.Fatalf("a 403 on risk score must make coverage partial: %s", out)
	}
}

func TestVerdictUnparseableScoreFails(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, "")
	f.static("GET /investigate/v2/domains/categorization/ok.test", map[string]any{"ok.test": map[string]any{"status": 1}})
	f.route("GET /investigate/v2/domains/risk-score/ok.test", func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"risk_score":"high"}`))
	})
	f.static("GET /investigate/v2/security/name/ok.test", map[string]any{})
	if _, err := runUmbrella(t, srv.URL, "", "verdict", "ok.test", "--no-activity", "--json"); err == nil {
		t.Fatal("an unparseable risk score must fail the command")
	}
}

func TestChildOrgNeverClaimsSharedDataDB(t *testing.T) {
	home := testenv.Isolate(t)
	t.Setenv("CISCO_UMBRELLA_API_KEY", "test-key-id")
	t.Setenv("CISCO_UMBRELLA_API_SECRET", "test-key-secret")
	t.Setenv("CISCO_UMBRELLA_ORG_ID", "")
	_ = home
	dataDir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	unscoped := filepath.Join(dataDir, "data.db")
	if err := os.WriteFile(unscoped, nil, 0o600); err != nil { // an unowned shared store
		t.Fatal(err)
	}
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"networks", "list", "--org", "8675309", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	if got := defaultDBPath("cisco-umbrella-cli"); got == unscoped {
		t.Fatal("a child org resolved to the shared data.db")
	}
	if _, err := os.Stat(unscoped + ".owner"); err == nil {
		t.Fatal("a child org claimed ownership of the shared data.db")
	}
}

// Cisco: an add can return 200 yet not add. diff --apply must notice and remove nothing.
func TestDestinationDiffRefusesRemovalsWhenAddsDidNotLand(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, "")
	f.static("POST /policies/v2/destinationlists/11/destinations", envelope(map[string]any{"id": 11})) // list never changes
	f.static("DELETE /policies/v2/destinationlists/11/destinations/remove", envelope(map[string]any{"id": 11}))
	out, err := runUmbrella(t, srv.URL, "example.com\nnews.example.org/video\n", "destination-lists", "diff", "11", "--file", "-", "--apply", "--json")
	if err == nil {
		t.Fatalf("an addition that did not land must fail the apply: %s", out)
	}
	if len(f.requests("DELETE", "/policies/")) != 0 {
		t.Fatal("removals ran although an addition did not land")
	}
	if !strings.Contains(out, `"not_added"`) || !strings.Contains(out, "news.example.org/video") {
		t.Fatalf("the plan must list the entry that did not land: %s", out)
	}
}

func TestVerdictScoresNeedRealData(t *testing.T) {
	for name, tc := range map[string]struct{ risk, sec map[string]any }{
		"null risk score":    {map[string]any{"risk_score": nil}, map[string]any{"found": true}},
		"security not found": {map[string]any{"risk_score": 4}, map[string]any{"found": false}},
	} {
		t.Run(name, func(t *testing.T) {
			f, srv := newFake(t)
			seedPolicies(f, "")
			f.static("GET /investigate/v2/domains/categorization/ok.test", map[string]any{"ok.test": map[string]any{"status": 1}})
			f.static("GET /investigate/v2/domains/risk-score/ok.test", tc.risk)
			f.static("GET /investigate/v2/security/name/ok.test", tc.sec)
			out, err := runUmbrella(t, srv.URL, "", "verdict", "ok.test", "--no-activity", "--json")
			if err != nil {
				t.Fatalf("verdict: %v", err)
			}
			var got []domainVerdict
			_ = json.Unmarshal([]byte(out), &got)
			if got[0].ScoresChecked || got[0].Coverage != "partial" {
				t.Fatalf("%s must leave scores unchecked: %s", name, out)
			}
		})
	}
}

func TestVerdictNullStatusIsNotUndetermined(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, "")
	f.static("GET /investigate/v2/domains/categorization/ok.test", map[string]any{"ok.test": map[string]any{"status": nil}})
	f.static("GET /investigate/v2/domains/risk-score/ok.test", map[string]any{"risk_score": 2})
	f.static("GET /investigate/v2/security/name/ok.test", map[string]any{"found": true})
	out, err := runUmbrella(t, srv.URL, "", "verdict", "ok.test", "--no-activity", "--json")
	if err != nil {
		t.Fatalf("verdict: %v", err)
	}
	var got []domainVerdict
	_ = json.Unmarshal([]byte(out), &got)
	if got[0].InvestigateChecked || got[0].Status != "" || got[0].Coverage != "partial" {
		t.Fatalf("status:null must leave categorization unchecked: %s", out)
	}
}

func TestVerdictIPv4URLIsIPv4(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, "")
	out, err := runUmbrella(t, srv.URL, "", "verdict", "https://198.51.100.7:443/", "--no-activity", "--json")
	if err != nil {
		t.Fatalf("verdict: %v", err)
	}
	var got []domainVerdict
	_ = json.Unmarshal([]byte(out), &got)
	if got[0].Kind != "ipv4" || got[0].BlockLists != 1 || got[0].Coverage != "complete" {
		t.Fatalf("a scheme-wrapped IPv4 must match its IPv4 block entry: %s", out)
	}
	if len(f.requests("GET", "/investigate/")) != 0 {
		t.Fatal("an IP host must not be sent to domain Investigate endpoints")
	}
}

func TestHygieneCountsUnreadableRecords(t *testing.T) {
	testenv.Isolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO resources (id, resource_type, data) VALUES ('9', 'roaming_computers', '[1]')`); err != nil {
		t.Fatal(err)
	}
	for _, res := range hygieneSyncResources {
		_ = db.SaveSyncState(res, "", 1)
	}
	_ = db.Close()
	out, err := runUmbrella(t, "http://127.0.0.1:9", "", "hygiene", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("hygiene: %v", err)
	}
	var rep hygieneReport
	_ = json.Unmarshal([]byte(out), &rep)
	if rep.Unparsed != 1 || rep.Scanned["roaming_computers"] != 0 || !strings.Contains(rep.Note, "could not be read") {
		t.Fatalf("an unreadable record must be reported, not counted as clean: %s", out)
	}
}

// statefulList serves a destination list whose adds show up on the next read,
// as the real API does, so diff --apply's post-add verification can pass.
func statefulList(f *fakeUmbrella, id int, initial []map[string]any) {
	var mu sync.Mutex
	entries := append([]map[string]any{}, initial...)
	base := fmt.Sprintf("/policies/v2/destinationlists/%d/destinations", id)
	f.route("GET "+base, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		mu.Lock()
		defer mu.Unlock()
		writeJSON(w, envelope(entries))
	})
	f.route("POST "+base, func(w http.ResponseWriter, _ *http.Request, body []byte) {
		var adds []map[string]string
		_ = json.Unmarshal(body, &adds)
		mu.Lock()
		for i, a := range adds {
			entries = append(entries, map[string]any{"id": fmt.Sprintf("8%03d", len(entries)+i), "destination": a["destination"], "type": "domain"})
		}
		mu.Unlock()
		writeJSON(w, envelope(map[string]any{"id": id}))
	})
}

// Class: unscoped tenant fallback. No credentials + --org must never read data.db.
func TestChildOrgWithoutCredentialsNeverUsesSharedDataDB(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("CISCO_UMBRELLA_API_KEY", "")
	t.Setenv("CISCO_UMBRELLA_API_SECRET", "")
	t.Setenv("CISCO_UMBRELLA_ORG_ID", "")
	dataDir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(dataDir, 0o700)
	unscoped := filepath.Join(dataDir, "data.db")
	if err := os.WriteFile(unscoped, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"hygiene", "--org", "111", "--json"})
	_ = cmd.Execute()
	if got := defaultDBPath("cisco-umbrella-cli"); got == unscoped {
		t.Fatal("--org without credentials resolved to the shared data.db")
	}
}

// Class: null payload accepted as a completed check.
func TestNullCollectionIsNotAnEmptyOne(t *testing.T) {
	f, srv := newFake(t)
	f.static("GET /policies/v2/destinationlists", map[string]any{"status": map[string]any{"code": 200}, "data": nil})
	if out, err := runUmbrella(t, srv.URL, "", "verdict", "x.test", "--no-activity", "--json"); err == nil {
		t.Fatalf("data:null must fail, not report lists_checked: %s", out)
	}
}

// Class: unusable fields treated as clean (hygiene and fleet share one evaluator).
func TestUnusableFieldsAreUnchecked(t *testing.T) {
	cutoff := time.Now().Add(-24 * time.Hour)
	uncheckedOnly := func(resource, data string, want ...string) {
		t.Helper()
		f, unchecked, ok := hygieneFindingsFor(resource, "1", json.RawMessage(data), cutoff)
		if !ok || strings.Join(unchecked, ",") != strings.Join(want, ",") || len(f) != 0 {
			t.Fatalf("%s %s: unchecked=%v ok=%v findings=%+v; want unchecked %v", resource, data, unchecked, ok, f, want)
		}
	}
	uncheckedOnly("networks", `{"originId":4,"isVerified":"false"}`, "network-unverified")
	uncheckedOnly("roaming_computers", `{"name":"x","status":"Encrypted","swgStatus":"Protected","lastSync":"yesterday"}`, "roaming-stale")
	uncheckedOnly("tunnels_tunnels_state", `{"tunnelId":"163367136"}`, "tunnel-down")
	if f, unchecked, ok := hygieneFindingsFor("roaming_computers", "1", json.RawMessage(`{"name":"x","status":"Encrypted","swgStatus":"Protected","lastSync":null}`), cutoff); !ok || len(unchecked) != 0 || len(f) != 1 || f[0].Kind != "roaming-stale" {
		t.Fatalf("a roaming computer with no sync recorded is stale: %+v %v %v", f, unchecked, ok)
	}
	// One unusable field must not hide another check's finding.
	if f, unchecked, ok := hygieneFindingsFor("roaming_computers", "1", json.RawMessage(`{"name":"x","status":"Off","swgStatus":7,"lastSync":null}`), cutoff); !ok || len(unchecked) != 1 || len(f) != 2 || f[0].Kind != "roaming-off" {
		t.Fatalf("a bad swgStatus must not hide the off finding: %+v %v", f, unchecked)
	}
	if f, unchecked, ok := hygieneFindingsFor("virtual_appliances", "3", json.RawMessage(`{"name":"va","health":"Warning","isUpgradable":"false"}`), cutoff); !ok || len(unchecked) != 1 || len(f) != 1 || f[0].Kind != "va-not-healthy" {
		t.Fatalf("a bad isUpgradable must not hide the health finding: %+v %v", f, unchecked)
	}
	if _, _, ok := hygieneFindingsFor("networks", "4", json.RawMessage(`[1]`), cutoff); ok {
		t.Fatal("a record that is not an object is unreadable")
	}

	old := fleetMintSpacing
	fleetMintSpacing = 0
	t.Cleanup(func() { fleetMintSpacing = old })
	fk, srv := newFake(t)
	fk.static("GET /admin/v2/managed/customers", []map[string]any{{"customerId": 9, "customerName": "Initrode"}})
	fk.static("GET /deployments/v2/roamingcomputers", []map[string]any{{"name": "no-status"}})
	fk.static("GET /deployments/v2/virtualappliances", []map[string]any{})
	fk.static("GET /deployments/v2/tunnelsState", []map[string]any{{"tunnelId": "163367136", "status": "UP"}})
	out, err := runUmbrella(t, srv.URL, "", "fleet", "health", "--json")
	if err != nil {
		t.Fatalf("fleet health: %v", err)
	}
	var rep fleetReport
	_ = json.Unmarshal([]byte(out), &rep)
	skipped := strings.Join(rep.Customers[0].Skipped, ";")
	if len(rep.Customers) != 1 || !strings.Contains(skipped, "roaming-off check") || !strings.Contains(skipped, "swg-unprotected check") {
		t.Fatalf("a roaming computer without status/swgStatus must list those checks as not run: %s", out)
	}
	if rep.Customers[0].TunnelsTotal != 1 {
		t.Fatalf("a string tunnelId must still be read: %s", out)
	}
}

// Class: missing metrics coerced to zero; and the documented type.label field.
func TestReportCardMissingNumbersAreNull(t *testing.T) {
	f, srv := newFake(t)
	f.route("GET /reports/v2/summary", func(w http.ResponseWriter, r *http.Request, _ []byte) {
		var to int64
		_, _ = fmtSscan(r.URL.Query().Get("to"), &to)
		if time.Since(time.UnixMilli(to)) < time.Minute {
			writeJSON(w, map[string]any{"data": map[string]any{"requests": 100}})
			return
		}
		writeJSON(w, map[string]any{"data": map[string]any{"requests": nil}})
	})
	f.static("GET /reports/v2/top-threat-types", map[string]any{"data": []map[string]any{}})
	f.static("GET /reports/v2/top-categories", map[string]any{"data": []map[string]any{}})
	f.static("GET /reports/v2/deployment-status", map[string]any{"data": []map[string]any{{"type": map[string]any{"label": "Sites"}, "activecount": 1, "count": 2}}})
	out, err := runUmbrella(t, srv.URL, "", "report-card", "--from", "-7days", "--json")
	if err != nil {
		t.Fatalf("report-card: %v", err)
	}
	var card reportCard
	_ = json.Unmarshal([]byte(out), &card)
	if len(card.Summary) != 1 || card.Summary[0].Prior != nil || card.Summary[0].Delta != nil {
		t.Fatalf("a null prior metric must give null prior and delta, not 0 and +100: %s", out)
	}
	if len(card.DeploymentStatus) != 1 || card.DeploymentStatus[0].Label != "Sites" || *card.DeploymentStatus[0].Count != 2 {
		t.Fatalf("deployment status must read the documented type.label: %s", out)
	}
}

// Class: inconsistent IP-host filtering.
func TestAllowlistAuditNeverCategorizesIPHosts(t *testing.T) {
	f, srv := newFake(t)
	f.static("GET /policies/v2/destinationlists", envelope([]map[string]any{{"id": 22, "name": "Partners", "access": "allow"}}))
	f.static("GET /policies/v2/destinationlists/22/destinations", envelope([]map[string]any{
		{"id": "1", "destination": "http://198.51.100.7/path", "type": "url"},
		{"id": "2", "destination": "partner.example.com", "type": "domain"},
	}))
	f.static("GET /investigate/v2/domains/categorization/partner.example.com", map[string]any{"partner.example.com": map[string]any{"status": 1}})
	out, err := runUmbrella(t, srv.URL, "", "allowlist", "audit", "--json")
	if err != nil {
		t.Fatalf("allowlist audit: %v", err)
	}
	for _, r := range f.requests("GET", "/investigate/") {
		if strings.Contains(r.Path, "198.51.100.7") {
			t.Fatalf("an IP host was sent to domain Investigate: %s", r.Path)
		}
	}
	for _, r := range f.requests("POST", "/investigate/") {
		if strings.Contains(r.Body, "198.51.100.7") {
			t.Fatalf("an IP host was sent to bulk categorization: %s", r.Body)
		}
	}
	var rep allowAudit
	_ = json.Unmarshal([]byte(out), &rep)
	if !rep.Checked {
		t.Fatalf("the one domain was categorized, so the audit is checked: %s", out)
	}
}

func TestAllowlistAuditListsUndatedAllows(t *testing.T) {
	f, srv := newFake(t)
	f.static("GET /policies/v2/destinationlists", envelope([]map[string]any{{"id": 22, "name": "Partners", "access": "allow"}}))
	f.static("GET /policies/v2/destinationlists/22/destinations", envelope([]map[string]any{{"id": "1", "destination": "nodate.example.com", "type": "domain", "createdAt": ""}}))
	f.static("GET /investigate/v2/domains/categorization/nodate.example.com", map[string]any{"nodate.example.com": map[string]any{"status": 1}})
	out, err := runUmbrella(t, srv.URL, "", "allowlist", "audit", "--since", "7d", "--json")
	if err != nil {
		t.Fatalf("allowlist audit: %v", err)
	}
	var rep allowAudit
	_ = json.Unmarshal([]byte(out), &rep)
	if len(rep.UndatedAllows) != 1 || len(rep.NewAllows) != 0 {
		t.Fatalf("an allow with no createdAt is undated, not silently old: %s", out)
	}
}

// Class: null or invalid collection members must fail closed, never act.
func TestNullListMembersFailClosed(t *testing.T) {
	f, srv := newFake(t)
	f.static("GET /policies/v2/destinationlists", envelope([]map[string]any{{"id": 11, "name": "Global Block", "access": "block"}}))
	f.route("GET /policies/v2/destinationlists/11/destinations", func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":{"code":200},"data":[null]}`))
	})
	f.static("DELETE /policies/v2/destinationlists/11/destinations/remove", envelope(map[string]any{"id": 11}))
	if _, err := runUmbrella(t, srv.URL, "keep.example.com\n", "destination-lists", "diff", "11", "--file", "-", "--apply", "--json"); err == nil {
		t.Fatal("a null destination row must fail the diff")
	}
	if len(f.requests("DELETE", "/policies/")) != 0 {
		t.Fatal("a null destination row produced a DELETE")
	}
	f2, srv2 := newFake(t)
	f2.route("GET /policies/v2/destinationlists", func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[null]}`))
	})
	if _, err := runUmbrella(t, srv2.URL, "", "verdict", "x.test", "--no-activity", "--json"); err == nil {
		t.Fatal("a null destination-list row must fail verdict")
	}
	if len(f2.requests("GET", "/policies/v2/destinationlists/0")) != 0 {
		t.Fatal("a null list row produced a request for list 0")
	}
}

// Class: per-check unusable fields.
func TestPerCheckMissingFieldsAreUnchecked(t *testing.T) {
	cutoff := time.Now().Add(-24 * time.Hour)
	now := time.Now().UTC().Format(time.RFC3339)
	_, unchecked, ok := hygieneFindingsFor("roaming_computers", "1", json.RawMessage(`{"name":"x","status":"Encrypted","swgStatus":null,"lastSync":"`+now+`"}`), cutoff)
	if !ok || len(unchecked) != 1 || unchecked[0] != "swg-unprotected" {
		t.Fatalf("a null swgStatus must leave the SWG check unchecked: %v %v", unchecked, ok)
	}
	if _, unchecked, ok := hygieneFindingsFor("roaming_computers", "1", json.RawMessage(`{"name":"x","status":"Bogus","swgStatus":"Protected","lastSync":"`+now+`"}`), cutoff); !ok || strings.Join(unchecked, ",") != "roaming-off,roaming-uninstalled" {
		t.Fatalf("an undocumented roaming status must leave the status checks unchecked: %v", unchecked)
	}
	_, unchecked, ok = hygieneFindingsFor("virtual_appliances", "3", json.RawMessage(`{"name":"va","health":"healthy"}`), cutoff)
	if !ok || len(unchecked) != 1 || unchecked[0] != "va-upgrade-available" {
		t.Fatalf("a VA without isUpgradable must leave the upgrade check unchecked: %v %v", unchecked, ok)
	}
}

// Class: missing resolver metrics.
func TestResolverRowsNeedCounts(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, "")
	f.static("GET /investigate/v2/domains/categorization/ok.test", map[string]any{"ok.test": map[string]any{"status": 1}})
	f.static("GET /investigate/v2/domains/risk-score/ok.test", map[string]any{"risk_score": 2})
	f.static("GET /investigate/v2/security/name/ok.test", map[string]any{"found": true})
	f.static("GET /reports/v2/top-identities", envelope([]map[string]any{{"identity": map[string]any{"label": "laptop-7"}}}))
	out, err := runUmbrella(t, srv.URL, "", "verdict", "ok.test", "--json")
	if err != nil {
		t.Fatalf("verdict: %v", err)
	}
	var got []domainVerdict
	_ = json.Unmarshal([]byte(out), &got)
	if got[0].ResolversChecked || got[0].Coverage != "partial" || len(got[0].ResolvedBy) != 0 {
		t.Fatalf("a resolver row without counts must not be reported as zero counts: %s", out)
	}
}

// Class: IP literals. IPv6 is not an Umbrella destination type.
func TestIPv6IsRejected(t *testing.T) {
	for _, in := range []string{"2001:db8::1", "[2001:db8::1]", "https://[2001:db8::1]/path", "http://[2001:db8::1]:8080/"} {
		if v, k, ok := normalizeDestination(in); ok {
			t.Errorf("normalizeDestination(%q) = %q,%q; IPv6 must be rejected", in, v, k)
		}
	}
	if hasDomainHost("[2001:db8::1]:443/x", "url") {
		t.Error("a bracketed IPv6 host is not a DNS name")
	}
}

func TestRound6Classes(t *testing.T) {
	// IPv6 with a port and no scheme is still IPv6.
	for _, in := range []string{"[2001:db8::1]:443", "[2001:db8::1]:443/path"} {
		if v, _, ok := normalizeDestination(in); ok {
			t.Errorf("normalizeDestination(%q) = %q; IPv6 must be rejected", in, v)
		}
	}
	// Exact identifiers.
	if got := rawID(json.RawMessage(`9007199254740993`)); got != "9007199254740993" {
		t.Errorf("rawID lost precision: %s", got)
	}
	if got := rawID(json.RawMessage(`"163367136"`)); got != "163367136" {
		t.Errorf("rawID string: %s", got)
	}
}

func TestFleetCustomerIDsStayExact(t *testing.T) {
	old := fleetMintSpacing
	fleetMintSpacing = 0
	t.Cleanup(func() { fleetMintSpacing = old })
	f, srv := newFake(t)
	f.route("GET /admin/v2/managed/customers", func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"customerId":9007199254740993,"customerName":"Big Org","seats":5}]`))
	})
	f.static("GET /deployments/v2/roamingcomputers", []map[string]any{})
	f.static("GET /deployments/v2/virtualappliances", []map[string]any{})
	f.static("GET /deployments/v2/tunnelsState", []map[string]any{})
	if _, err := runUmbrella(t, srv.URL, "", "fleet", "health", "--json"); err != nil {
		t.Fatalf("fleet health: %v", err)
	}
	var childOrgs []string
	for _, m := range f.requests("POST", "/auth/v2/token") {
		if m.Org != "" { // the other mint is the parent's own token for the customer list
			childOrgs = append(childOrgs, m.Org)
		}
	}
	if len(childOrgs) != 1 || childOrgs[0] != "9007199254740993" {
		t.Fatalf("the child-org token must be minted for the exact customer id: %v", childOrgs)
	}
}

func TestDestinationDiffRejectsNonIntegerRemovalIDs(t *testing.T) {
	f, srv := newFake(t)
	f.static("GET /policies/v2/destinationlists/11/destinations", envelope([]map[string]any{{"id": "abc", "destination": "old.test", "type": "domain"}}))
	f.static("POST /policies/v2/destinationlists/11/destinations", envelope(map[string]any{"id": 11}))
	f.static("DELETE /policies/v2/destinationlists/11/destinations/remove", envelope(map[string]any{"id": 11}))
	if _, err := runUmbrella(t, srv.URL, "new.test\n", "destination-lists", "diff", "11", "--file", "-", "--apply", "--json"); err == nil {
		t.Fatal("a non-integer destination id must stop the apply")
	}
	if len(f.requests("POST", "/policies/")) != 0 || len(f.requests("DELETE", "/policies/")) != 0 {
		t.Fatal("no write may happen when a removal id is invalid")
	}
}

func TestReportCardRankedRowsNeedNames(t *testing.T) {
	f, srv := newFake(t)
	f.static("GET /reports/v2/summary", map[string]any{"data": map[string]any{"requests": 1}})
	f.route("GET /reports/v2/top-threat-types", func(w http.ResponseWriter, r *http.Request, _ []byte) {
		var to int64
		_, _ = fmtSscan(r.URL.Query().Get("to"), &to)
		if time.Since(time.UnixMilli(to)) < time.Minute {
			writeJSON(w, map[string]any{"data": []map[string]any{{"threattype": "Phishing", "count": 10}}})
			return
		}
		writeJSON(w, map[string]any{"data": []map[string]any{{"count": 5}}}) // unnamed prior row
	})
	f.static("GET /reports/v2/top-categories", map[string]any{"data": []map[string]any{}})
	f.static("GET /reports/v2/deployment-status", map[string]any{"data": []map[string]any{}})
	out, err := runUmbrella(t, srv.URL, "", "report-card", "--from", "-7days", "--json")
	if err != nil {
		t.Fatalf("report-card: %v", err)
	}
	var card reportCard
	_ = json.Unmarshal([]byte(out), &card)
	if len(card.TopThreatTypes) != 1 || card.TopThreatTypes[0].Prior != nil {
		t.Fatalf("an unnamed prior row makes the prior unknown, not 0: %s", out)
	}
}

// A file line the CLI cannot parse must never cause its list twin to be deleted.
func TestDestinationDiffApplyRefusesUnparsedLines(t *testing.T) {
	f, srv := newFake(t)
	f.static("GET /policies/v2/destinationlists/11/destinations", envelope([]map[string]any{{"id": "5", "destination": "weird$entry", "type": "domain"}}))
	f.static("DELETE /policies/v2/destinationlists/11/destinations/remove", envelope(map[string]any{"id": 11}))
	if _, err := runUmbrella(t, srv.URL, "weird$entry\n", "destination-lists", "diff", "11", "--file", "-", "--apply", "--json"); err == nil {
		t.Fatal("--apply with an unparsed file line must refuse")
	}
	if len(f.requests("DELETE", "/policies/")) != 0 || len(f.requests("POST", "/policies/")) != 0 {
		t.Fatal("no write may happen while a file line is unparsed")
	}
	// The same entry, parseable now, is kept, not deleted.
	f2, srv2 := newFake(t)
	f2.static("GET /policies/v2/destinationlists/11/destinations", envelope([]map[string]any{{"id": "5", "destination": "redirect.test/go?next=https://target.test", "type": "url"}}))
	f2.static("DELETE /policies/v2/destinationlists/11/destinations/remove", envelope(map[string]any{"id": 11}))
	out, err := runUmbrella(t, srv2.URL, "redirect.test/go?next=https://target.test\n", "destination-lists", "diff", "11", "--file", "-", "--apply", "--json")
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(f2.requests("DELETE", "/policies/")) != 0 || !strings.Contains(out, `"unchanged": 1`) {
		t.Fatalf("an identical URL entry must stay: %s", out)
	}
}

func TestVerdictAgentKeepsSparseScores(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, "")
	cats := map[string]any{}
	for _, d := range []string{"a.test", "b.test", "c.test", "d.test"} {
		cats[d] = map[string]any{"status": 1}
		score := any(nil)
		if d == "a.test" {
			score = 42
		}
		f.static("GET /investigate/v2/domains/risk-score/"+d, map[string]any{"risk_score": score})
		f.static("GET /investigate/v2/security/name/"+d, map[string]any{"found": true})
	}
	f.static("POST /investigate/v2/domains/categorization", cats)
	out, err := runUmbrella(t, srv.URL, "", "verdict", "a.test", "b.test", "c.test", "d.test", "--no-activity", "--agent")
	if err != nil {
		t.Fatalf("verdict: %v", err)
	}
	if !strings.Contains(strings.ReplaceAll(out, " ", ""), `"risk_score":42`) {
		t.Fatalf("--agent dropped the one domain's risk score: %s", out)
	}
}

// --entries carries the source of truth inline (the MCP form, where --file is
// refused); exactly one of --file and --entries is accepted.
func TestDestinationDiffEntriesInline(t *testing.T) {
	f, srv := newFake(t)
	seedPolicies(f, "")
	out, err := runUmbrella(t, srv.URL, "", "destination-lists", "diff", "11", "--entries", "EXAMPLE.com., https://new.test/\nevil.test", "--json")
	if err != nil {
		t.Fatalf("diff --entries: %v", err)
	}
	var plan destinationDiff
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	if strings.Join(plan.ToAdd, ",") != "new.test" || plan.Unchanged != 2 || len(plan.ToRemove) != 1 || plan.File != "--entries" {
		t.Fatalf("plan wrong: %+v", plan)
	}
	if _, err := runUmbrella(t, srv.URL, "", "destination-lists", "diff", "11", "--entries", "a.test", "--file", "-", "--json"); err == nil {
		t.Fatal("--entries with --file must be refused")
	}
	if _, err := runUmbrella(t, srv.URL, "", "destination-lists", "diff", "11", "--json"); err == nil {
		t.Fatal("neither --entries nor --file must be refused")
	}
}
