// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// End-to-end tests for the fleet commands: a fake GravityZone JSON-RPC server
// (envelopes in, envelopes out) behind the real client, transport and store.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"bitdefender-pp-cli/internal/cliutil/testenv"
)

const (
	idP = "aaaaaaaaaaaaaaaaaaaaaaa0" // partner (API key's company)
	idS = "aaaaaaaaaaaaaaaaaaaaaaa1" // sub-partner
	idA = "aaaaaaaaaaaaaaaaaaaaaaa2" // customer
	idB = "aaaaaaaaaaaaaaaaaaaaaaa3" // customer
	idC = "aaaaaaaaaaaaaaaaaaaaaaa4" // customer under the sub-partner
	idD = "aaaaaaaaaaaaaaaaaaaaaaa5" // customer in a company folder
	idF = "fffffffffffffffffffffff0" // company folder under the partner
	idE = "aaaaaaaaaaaaaaaaaaaaaaa6" // customer added between snapshots (extraCo)
)

type fakeGZ struct {
	mu          sync.Mutex
	round       int // 0 = first fleet snapshot state, 1 = second
	calls       []fakeCall
	failComp    string          // company whose getEndpointsList fails
	bravoPages  int             // >1 makes Bravo's endpoint list span pages
	failUsage   map[string]bool // "<companyId>|<mm/yyyy>" usage fetches that fail
	blockAaaaAt map[string]bool // companies whose blocklist holds hash aaaa
	failTreeFor string          // parent whose getCompaniesList fails
	incidentPg2 bool            // incidents span two pages and page 2 fails
	incidentShf bool            // page 1 counts 20 but only 19 unique arrive (one changed mid-paging)
	quarFull    bool            // Acme's quarantine is exactly two full pages; pagesCount only on page 1
	failDetail  bool            // getManagedEndpointDetails fails
	bravoShift  bool            // Bravo's page 1 counts 3 endpoints but only 2 ever arrive
	quarShift   bool            // Acme's quarantine page 1 counts 3 items but only 2 arrive
	extraCo     bool            // Echo appears under the partner
	incShort    bool            // incidents page 1 counts 20 but returns 2 (short first page)
}

// wantPath is each method's documented area and version (spec.yaml).
var wantPath = map[string]string{
	"getCompanyDetails": "/api/v1.0/jsonrpc/companies", "getCompaniesList": "/api/v1.0/jsonrpc/network",
	"getCompanyFoldersList": "/api/v1.0/jsonrpc/network", "getEndpointsList": "/api/v1.1/jsonrpc/network",
	"getManagedEndpointDetails": "/api/v1.0/jsonrpc/network", "getMonthlyUsagePerProductType": "/api/v1.0/jsonrpc/licensing",
	"getIncidentsList": "/api/v1.2/jsonrpc/incidents", "getQuarantineItemsList": "/api/v1.0/jsonrpc/quarantine/computers",
	"getBlocklistItems": "/api/v1.2/jsonrpc/incidents",
}

type fakeCall struct {
	Path   string
	Method string
	Params map[string]any
}

func (f *fakeGZ) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			JSONRPC string         `json:"jsonrpc"`
			Method  string         `json:"method"`
			Params  map[string]any `json:"params"`
			ID      string         `json:"id"`
		}
		if err := json.Unmarshal(body, &req); err != nil || req.JSONRPC != "2.0" || req.ID == "" {
			t.Errorf("not a JSON-RPC 2.0 envelope: %s", body)
		}
		if got := r.Header.Get("Authorization"); got != "Basic dGVzdC1rZXk6" { // base64("test-key:")
			t.Errorf("Authorization = %q, want Basic auth with the key as username and empty password", got)
		}
		if want := wantPath[req.Method]; want != "" && r.URL.Path != want {
			t.Errorf("%s sent to %s, want %s", req.Method, r.URL.Path, want)
		}
		if req.Method == "getIncidentsList" {
			fl, _ := req.Params["filters"].(map[string]any)
			start, hasStart := fl["changeStartDate"].(string)
			end, hasEnd := fl["changeEndDate"].(string)
			if hasStart != hasEnd {
				t.Errorf("getIncidentsList must send changeStartDate and changeEndDate together: %v", fl)
			}
			if hasStart && hasEnd {
				ts, _ := time.Parse(time.RFC3339, start)
				te, _ := time.Parse(time.RFC3339, end)
				if !te.After(ts) {
					t.Errorf("changeEndDate %s must be after changeStartDate %s", end, start)
				}
				if te.After(time.Now().UTC()) {
					t.Errorf("changeEndDate %s is in the future: later changes would be skipped", end)
				}
			}
			documented := map[string]bool{"open": true, "in_progress": true, "closed": true, "false_positive": true, "closed_mdr_reviewed": true,
				"unknown": true, "low": true, "medium": true, "high": true, "critical": true}
			for _, k := range []string{"status", "priority"} {
				vals, _ := fl[k].([]any)
				for _, v := range vals {
					if !documented[v.(string)] {
						t.Errorf("getIncidentsList %s value %v is not documented", k, v)
					}
				}
			}
		}
		f.mu.Lock()
		f.calls = append(f.calls, fakeCall{Path: r.URL.Path, Method: req.Method, Params: req.Params})
		st := fakeGZ{round: f.round, failComp: f.failComp, bravoPages: f.bravoPages, failUsage: f.failUsage, blockAaaaAt: f.blockAaaaAt, failTreeFor: f.failTreeFor, incidentPg2: f.incidentPg2, incidentShf: f.incidentShf, quarFull: f.quarFull, failDetail: f.failDetail, bravoShift: f.bravoShift, quarShift: f.quarShift, extraCo: f.extraCo, incShort: f.incShort}
		f.mu.Unlock()
		result, rpcErr := st.result(req.Method, req.Params)
		out := map[string]any{"id": req.ID, "jsonrpc": "2.0"}
		if rpcErr != nil {
			out["error"] = rpcErr
		} else {
			out["result"] = result
		}
		_ = json.NewEncoder(w).Encode(out)
	}
}

func ep(id, name string, managed, outdated, isolated bool, applied *bool, scan time.Time, mac, ip string) map[string]any {
	e := map[string]any{"id": id, "name": name, "fqdn": strings.ToLower(name) + ".corp.local", "isManaged": managed,
		"productOutdated": outdated, "isIsolated": isolated, "macs": []string{mac}, "ip": ip, "machineType": 1}
	if applied != nil {
		e["policy"] = map[string]any{"id": "p1", "name": "Default", "applied": *applied}
	}
	if !scan.IsZero() {
		e["lastSuccessfulScan"] = map[string]any{"name": "Quick Scan", "date": scan.Format("2006-01-02T15:04:05")}
	}
	return e
}

func (f *fakeGZ) result(method string, p map[string]any) (any, map[string]any) {
	round, failComp := f.round, f.failComp
	yes, no := true, false
	recent := time.Now().UTC().Add(-24 * time.Hour)
	old := time.Now().UTC().Add(-60 * 24 * time.Hour)
	str := func(k string) string { s, _ := p[k].(string); return s }
	ctype := func() float64 {
		f, _ := p["filters"].(map[string]any)
		n, _ := f["companyType"].(float64)
		return n
	}
	switch method {
	case "getCompanyDetails":
		return map[string]any{"id": idP, "name": "Partner MSP", "type": 0}, nil
	case "getCompaniesList":
		if f.failTreeFor != "" && str("parentId") == f.failTreeFor {
			return nil, map[string]any{"code": -32000, "message": "Server error"}
		}
		kids := map[string]map[float64][]map[string]any{
			idP: {0: {{"id": idS, "name": "Sub Partner"}}, 1: {{"id": idA, "name": "Acme"}, {"id": idB, "name": "Bravo"}}},
			idS: {1: {{"id": idC, "name": "Charlie"}}},
			idF: {1: {{"id": idD, "name": "Delta"}, {"id": idA, "name": "Acme"}}},
		}
		if f.extraCo {
			kids[idP][1] = append(kids[idP][1], map[string]any{"id": idE, "name": "Echo"})
		}
		list := kids[str("parentId")][ctype()]
		if list == nil {
			list = []map[string]any{}
		}
		return list, nil
	case "getCompanyFoldersList":
		if str("parentId") == idP {
			return []map[string]any{{"id": idF, "name": "Folder One"}}, nil
		}
		return []map[string]any{}, nil
	case "getEndpointsList":
		parent := str("parentId")
		if parent == failComp {
			return nil, map[string]any{"code": -32001, "message": "Authorization error", "data": map[string]any{"details": "license"}}
		}
		var items []map[string]any
		switch parent {
		case idA:
			items = []map[string]any{ep("e-a1", "PC-A1", true, true, false, &yes, recent, "00:11:22:33:44:01", "10.0.0.1")}
			if round == 0 {
				items = append(items, ep("e-a2", "PC-A2", false, false, false, nil, time.Time{}, "00:11:22:33:44:02", "10.0.0.2"))
			} else {
				items = append(items, ep("e-a9", "PC-A9", false, false, false, nil, time.Time{}, "00-11-22-33-44-02", "10.0.0.9"))
			}
		case idB:
			items = []map[string]any{ep("e-b1", "PC-B1", true, false, round == 0, &no, old, "00:11:22:33:44:03", "10.0.1.1")}
			if f.bravoShift {
				return map[string]any{"page": 1, "pagesCount": 1, "total": 3, "perPage": 1000, "hasMoreRecords": false, "items": items}, nil
			}
			if f.bravoPages > 1 {
				page, _ := p["page"].(float64)
				if page > 1 {
					applied := true
					items = []map[string]any{ep("e-b2", "PC-B2", true, false, false, &applied, recent, "00:11:22:33:44:05", "10.0.1.2")}
				}
				return map[string]any{"page": page, "perPage": 1000, "hasMoreRecords": int(page) < f.bravoPages, "items": items}, nil
			}
		case idC:
			items = []map[string]any{ep("e-c1", "LAPTOP-7", true, false, false, &yes, time.Time{}, "00:11:22:33:44:04", "10.0.2.7")}
		default:
			items = []map[string]any{}
		}
		return map[string]any{"page": 1, "pagesCount": 1, "perPage": 1000, "total": len(items), "hasMoreRecords": false, "items": items}, nil
	case "getManagedEndpointDetails":
		if f.failDetail {
			return nil, map[string]any{"code": -32000, "message": "Server error"}
		}
		return map[string]any{"id": str("endpointId"), "lastSeen": "2026-10-01T10:00:00",
			"agent": map[string]any{"signatureOutdated": true}, "malwareStatus": map[string]any{"detection": false, "infected": false}}, nil
	case "getMonthlyUsagePerProductType":
		month := str("targetMonth")
		if f.failUsage[str("companyId")+"|"+month] {
			return nil, map[string]any{"code": -32001, "message": "Authorization error"}
		}
		cur := month == "09/2026"
		seats := map[string][2]int{idP: {30, 10}, idA: {12, 12}, idB: {8, 8}, idS: {10, 10}, idC: {10, 10}, idD: {0, 0}}
		s := seats[str("companyId")]
		n := s[1]
		if cur {
			n = s[0]
		}
		u := map[string]any{"productType": 0, "endpointMonthlyUsage": n, "edrMonthlyUsage": 0}
		if str("companyId") == idP {
			u["mspSecureMonthlyUsage"] = n
		}
		return map[string]any{"usages": []any{u}}, nil
	case "getIncidentsList":
		now := time.Now().UTC()
		if f.incShort {
			return map[string]any{"page": 1, "pagesCount": 1, "perPage": 10, "total": 20, "items": []any{
				map[string]any{"incidentId": "inc-s1", "company": map[string]any{"id": idA, "name": "Acme"}, "status": "open", "severityScore": 10,
					"created": now.Format(time.RFC3339), "lastIncidentChange": now.Format(time.RFC3339)},
			}}, nil
		}
		if f.incidentPg2 || f.incidentShf {
			page, _ := p["page"].(float64)
			if page > 1 && f.incidentPg2 {
				return nil, map[string]any{"code": -32000, "message": "Server error"}
			}
			if page > 1 {
				items := []any{}
				for i := 0; i < 9; i++ {
					items = append(items, map[string]any{"incidentId": "inc-p2-" + string(rune('a'+i)), "company": map[string]any{"id": idA, "name": "Acme"},
						"status": "open", "severityScore": 10, "created": now.Format(time.RFC3339), "lastIncidentChange": now.Format(time.RFC3339)})
				}
				return map[string]any{"page": 2, "perPage": 10, "items": items}, nil
			}
			items := []any{}
			for i := 0; i < 10; i++ {
				items = append(items, map[string]any{"incidentId": "inc-" + string(rune('a'+i)), "incidentNumber": i, "company": map[string]any{"id": idA, "name": "Acme"},
					"status": "open", "priority": "low", "severityScore": 10, "created": now.Format(time.RFC3339), "lastIncidentChange": now.Format(time.RFC3339)})
			}
			return map[string]any{"page": 1, "pagesCount": 2, "perPage": 10, "total": 20, "items": items}, nil
		}
		return map[string]any{"page": 1, "pagesCount": 1, "perPage": 1000, "total": 2, "items": []any{
			map[string]any{"incidentId": "inc-low", "incidentNumber": 2, "incidentType": "incident", "company": map[string]any{"id": idB, "name": "Bravo"},
				"status": "in_progress", "priority": "medium", "severityScore": 50, "mainAction": "blocked", "created": now.Add(-1 * time.Hour).Format(time.RFC3339),
				"lastIncidentChange": now.Add(-30 * time.Minute).Format(time.RFC3339), "assignee": "bbbbbbbbbbbbbbbbbbbbbbbb", "details": map[string]any{"computerName": "PC-B1"}},
			map[string]any{"incidentId": "inc-high", "incidentNumber": 1, "incidentType": "incident", "company": map[string]any{"id": idA, "name": "Acme"},
				"status": "open", "priority": "high", "severityScore": 90, "mainAction": "reported", "created": now.Add(-48 * time.Hour).Format(time.RFC3339),
				"lastIncidentChange": now.Add(-2 * time.Hour).Format(time.RFC3339), "details": map[string]any{"computerName": "PC-A1"}},
		}}, nil
	case "getQuarantineItemsList":
		item := func(sha, endpoint string) map[string]any {
			return map[string]any{"id": "q-" + sha + endpoint, "endpointId": endpoint, "quarantinedOn": "2026-10-10T10:00:00", "threatName": "Trojan.Gen",
				"companyId": str("companyId"), "details": map[string]any{"filePath": `C:\Users\x\evil.exe`, "fileSha256": sha}}
		}
		byCo := map[string][]any{
			idA: {item("aaaa", "e-a1"), item("bbbb", "e-a1")},
			idB: {item("AAAA", "e-b1")},
			idC: {item("cccc", "e-c1")},
		}
		items := byCo[str("companyId")]
		if items == nil {
			items = []any{}
		}
		if f.quarShift && str("companyId") == idA {
			return map[string]any{"page": 1, "pagesCount": 1, "perPage": 100, "total": 3, "items": items}, nil
		}
		if f.quarFull && str("companyId") == idA {
			page, _ := p["page"].(float64)
			full := []any{}
			for i := 0; i < 100; i++ {
				full = append(full, item("f"+string(rune('a'+i%26))+string(rune('a'+i/26))+string(rune('a'+int(page))), "e-a1"))
			}
			if page == 1 {
				return map[string]any{"page": 1, "pagesCount": 2, "perPage": 100, "total": 200, "items": full}, nil
			}
			return map[string]any{"page": page, "perPage": 100, "items": full}, nil
		}
		return map[string]any{"page": 1, "pagesCount": 1, "perPage": 100, "total": len(items), "items": items}, nil
	case "getBlocklistItems":
		items := []any{}
		if str("companyId") == idB {
			items = append(items, map[string]any{"id": "bl1", "type": "hash", "details": map[string]any{"algorithm": "sha256", "hash": "bbbb"}})
		}
		if f.blockAaaaAt[str("companyId")] {
			items = append(items, map[string]any{"id": "bl2", "type": "hash", "details": map[string]any{"algorithm": "sha256", "hash": "AAAA"}})
		}
		return map[string]any{"page": 1, "pagesCount": 1, "perPage": 100, "total": len(items), "items": items}, nil
	}
	return nil, map[string]any{"code": -32601, "message": "Method not found"}
}

func runGZ(t *testing.T, args ...string) (map[string]any, string, error) {
	t.Helper()
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(append(args, "--json", "--no-cache"))
	err := cmd.Execute()
	var v map[string]any
	if jerr := json.Unmarshal(out.Bytes(), &v); jerr != nil {
		t.Fatalf("%v: stdout is not JSON (%v): %s\nstderr: %s", args, jerr, out.String(), errb.String())
	}
	if r, ok := v["results"].(map[string]any); ok {
		v = r
	}
	return v, errb.String(), err
}

func setupFakeGZ(t *testing.T) *fakeGZ {
	t.Helper()
	testenv.Isolate(t)
	f := &fakeGZ{}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	t.Setenv("BITDEFENDER_BASE_URL", srv.URL+"/api")
	t.Setenv("BITDEFENDER_API_KEY", "test-key")
	return f
}

func TestCompaniesTreeWalksPartnersCustomersAndFolders(t *testing.T) {
	setupFakeGZ(t)
	v, _, err := runGZ(t, "companies", "tree")
	if err != nil {
		t.Fatal(err)
	}
	cos, _ := v["companies"].([]any)
	byID := map[string]map[string]any{}
	for _, c := range cos {
		m := c.(map[string]any)
		byID[m["id"].(string)] = m
	}
	if len(byID) != 6 || len(cos) != 6 {
		t.Fatalf("got %d companies (%d unique), want 6 with the folder duplicate dropped: %v", len(cos), len(byID), cos)
	}
	if byID[idD]["parent_id"] != idP {
		t.Fatalf("folder company parent = %v, want the enclosing partner %s", byID[idD]["parent_id"], idP)
	}
	if byID[idC]["parent_id"] != idS || byID[idC]["depth"].(float64) != 2 {
		t.Fatalf("sub-partner customer = %v, want parent %s depth 2", byID[idC], idS)
	}
	if !strings.Contains(byID[idD]["path"].(string), "Folder One") {
		t.Fatalf("folder company path %q lacks the folder", byID[idD]["path"])
	}
	if v["partners"].(float64) != 2 || v["customers"].(float64) != 4 {
		t.Fatalf("partners/customers = %v/%v, want 2/4", v["partners"], v["customers"])
	}
}

func TestFleetHealthRollupChangesFindAndRMMGap(t *testing.T) {
	f := setupFakeGZ(t)
	v, _, err := runGZ(t, "fleet", "health", "--scan-days", "7", "--details", "2")
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]map[string]any{}
	for _, c := range v["companies"].([]any) {
		m := c.(map[string]any)
		rows[m["name"].(string)] = m
	}
	acme, bravo, charlie := rows["Acme"], rows["Bravo"], rows["Charlie"]
	if acme["unmanaged"].(float64) != 1 || acme["outdated"].(float64) != 1 || acme["unscanned"].(float64) != 0 {
		t.Fatalf("Acme = %v, want 1 unmanaged, 1 outdated, 0 unscanned", acme)
	}
	if bravo["isolated"].(float64) != 1 || bravo["policy_not_applied"].(float64) != 1 || bravo["unscanned"].(float64) != 1 {
		t.Fatalf("Bravo = %v, want isolated, policy not applied, unscanned (60-day-old scan)", bravo)
	}
	if charlie["unscanned"].(float64) != 1 {
		t.Fatalf("Charlie = %v, want the never-scanned laptop counted unscanned", charlie)
	}
	if first := v["companies"].([]any)[0].(map[string]any)["name"]; first != "Bravo" {
		t.Fatalf("first row = %v, want Bravo (3 issues) ranked first", first)
	}
	if tot := v["totals"].(map[string]any); tot["endpoints"].(float64) != 4 || tot["issues"].(float64) != 6 {
		t.Fatalf("totals = %v, want 4 endpoints and 6 issues", tot)
	}
	dets, _ := v["details"].([]any)
	if len(dets) != 2 || dets[0].(map[string]any)["signature_outdated"] != true {
		t.Fatalf("details = %v, want 2 enriched endpoints with signature_outdated", dets)
	}
	if first := dets[0].(map[string]any)["name"]; first != "LAPTOP-7" {
		t.Fatalf("first enriched = %v, want the never-scanned LAPTOP-7 first", first)
	}

	// Only one snapshot: changes must report nothing, not fabricate drift.
	cv, _, err := runGZ(t, "fleet", "changes", "--since", "1h")
	if err != nil {
		t.Fatal(err)
	}
	if ch, _ := cv["changes"].([]any); len(ch) != 0 || !strings.Contains(cv["note"].(string), "1 fleet snapshot") {
		t.Fatalf("one snapshot: changes=%v note=%v", cv["changes"], cv["note"])
	}

	// Second run: PC-A2 re-registers as PC-A9 (same MAC, other notation), PC-B1
	// leaves isolation, and Charlie's fetch fails (must not read as removals).
	f.mu.Lock()
	f.round, f.failComp = 1, idC
	f.mu.Unlock()
	_, stderr, err := runGZ(t, "fleet", "health")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "1 of 6 company fetches failed") {
		t.Fatalf("stderr %q lacks the partial-failure warning", stderr)
	}
	cv, _, err = runGZ(t, "fleet", "changes", "--since", "1s")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, c := range cv["changes"].([]any) {
		m := c.(map[string]any)
		kinds[m["kind"].(string)]++
		if m["company_name"] == "Charlie" {
			t.Fatalf("Charlie's failed fetch was reported as a change: %v", m)
		}
	}
	if kinds["re_registered"] != 1 || kinds["released_from_isolation"] != 1 || kinds["endpoint_removed"] != 0 || kinds["endpoint_added"] != 0 {
		t.Fatalf("changes by kind = %v, want 1 re_registered + 1 released_from_isolation only", kinds)
	}
	if cv["complete"] != false || strings.Join(toStrings(cv["skipped_companies"]), ",") != "Charlie" {
		t.Fatalf("complete=%v skipped=%v, want an incomplete comparison naming Charlie", cv["complete"], cv["skipped_companies"])
	}

	// fleet locate from the snapshot: by hostname substring, MAC, IP; negative case.
	fv, _, err := runGZ(t, "fleet", "locate", "laptop-7")
	if err != nil {
		t.Fatal(err)
	}
	if fv["source"] != "snapshot" {
		t.Fatalf("find source = %v, want snapshot", fv["source"])
	}
	// The latest snapshot skipped Charlie (failed), so LAPTOP-7 is not in it.
	if m, _ := fv["matches"].([]any); len(m) != 0 {
		t.Fatalf("find laptop-7 in a snapshot without Charlie = %v, want none", m)
	}
	fv, _, _ = runGZ(t, "fleet", "locate", "00-11-22-33-44-01")
	if m, _ := fv["matches"].([]any); len(m) != 1 || m[0].(map[string]any)["name"] != "PC-A1" {
		t.Fatalf("find by MAC = %v, want PC-A1", fv["matches"])
	}
	fv, _, _ = runGZ(t, "fleet", "locate", "10.0.1.1")
	if m, _ := fv["matches"].([]any); len(m) != 1 || m[0].(map[string]any)["company_name"] != "Bravo" {
		t.Fatalf("find by IP = %v, want PC-B1 in Bravo", fv["matches"])
	}
	fv, _, _ = runGZ(t, "fleet", "locate", "no-such-host")
	if m, _ := fv["matches"].([]any); len(m) != 0 {
		t.Fatalf("find no-such-host = %v, want none", m)
	}

	// rmm-gap against the snapshot.
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "rmm.csv")
	csv := "Device Name,MAC Address\nPC-A1.corp.local,\npc-a9,\nGHOST-01,00:99:99:99:99:99\n"
	if err := os.WriteFile(csvPath, []byte(csv), 0o600); err != nil {
		t.Fatal(err)
	}
	gv, _, err := runGZ(t, "fleet", "rmm-gap", "--devices", csvPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := gv["summary"].(map[string]any)
	if sum["missing_in_gravityzone"].(float64) != 1 || sum["unmanaged_in_gravityzone"].(float64) != 1 || sum["missing_in_rmm"].(float64) != 1 {
		t.Fatalf("rmm-gap summary = %v, want GHOST-01 missing, PC-A9 unmanaged, PC-B1 missing in RMM", sum)
	}
}

func TestUsageBillingOwnShareSwingAndAgreements(t *testing.T) {
	setupFakeGZ(t)
	dir := t.TempDir()
	agreements := filepath.Join(dir, "agreements.csv")
	if err := os.WriteFile(agreements, []byte("Company,Quantity\nAcme,12\nbravo,10\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v, _, err := runGZ(t, "usage", "billing", "--month", "09/2026", "--agreements", agreements)
	if err != nil {
		t.Fatal(err)
	}
	if v["prior_month"] != "08/2026" {
		t.Fatalf("prior_month = %v", v["prior_month"])
	}
	rows := map[string]map[string]any{}
	for _, r := range v["rows"].([]any) {
		m := r.(map[string]any)
		rows[m["company_name"].(string)] = m
	}
	p := rows["Partner MSP"]
	own, _ := p["own_share"].(map[string]any)
	// 30 total minus direct children Sub Partner 10, Acme 12, Bravo 8, Delta 0 = 0.
	if own == nil || own["endpointMonthlyUsage"] != nil {
		t.Fatalf("partner own_share = %v, want endpointMonthlyUsage 0 (omitted)", own)
	}
	flags := strings.Join(toStrings(p["flags"]), ",")
	// mspSecure* is a breakdown of endpointMonthlyUsage (30 of 30): not a problem.
	if !strings.Contains(flags, "seat_swing:endpointMonthlyUsage") || strings.Contains(flags, "model_breakdown") || strings.Contains(flags, "negative_own_share") {
		t.Fatalf("partner flags = %v, want only seat swings (10 -> 30)", flags)
	}
	if f := toStrings(rows["Acme"]["flags"]); len(f) != 0 {
		t.Fatalf("Acme flags = %v, want none (12 billed, 12 agreed, no swing)", f)
	}
	if f := strings.Join(toStrings(rows["Bravo"]["flags"]), ","); !strings.Contains(f, "agreement_mismatch:8_billed_vs_10_agreed") {
		t.Fatalf("Bravo flags = %v, want agreement mismatch", f)
	}
}

func TestFleetIncidentsQueueAndThreats(t *testing.T) {
	f := setupFakeGZ(t)
	v, _, err := runGZ(t, "fleet", "incident-queue", "--sla", "24h")
	if err != nil {
		t.Fatal(err)
	}
	inc := v["incidents"].([]any)
	if len(inc) != 2 || inc[0].(map[string]any)["incident_id"] != "inc-high" {
		t.Fatalf("incidents = %v, want inc-high (severity 90) first", inc)
	}
	first := inc[0].(map[string]any)
	if first["unassigned"] != true || first["sla_breached"] != true {
		t.Fatalf("inc-high = %v, want unassigned and SLA-breached (48h > 24h)", first)
	}
	if second := inc[1].(map[string]any); second["sla_breached"] != false || second["unassigned"] != false {
		t.Fatalf("inc-low = %v, want within SLA and assigned", second)
	}
	var sent map[string]any
	for _, c := range f.calls {
		if c.Method == "getIncidentsList" {
			sent = c.Params
			if c.Path != "/api/v1.2/jsonrpc/incidents" {
				t.Fatalf("getIncidentsList path = %s, want /api/v1.2/jsonrpc/incidents", c.Path)
			}
		}
	}
	opts, _ := sent["options"].(map[string]any)
	filters, _ := sent["filters"].(map[string]any)
	if opts["includeChildCompanies"] != true || len(filters["status"].([]any)) != 2 {
		t.Fatalf("getIncidentsList params = %v, want includeChildCompanies and the open,in_progress status filter", sent)
	}

	tv, _, err := runGZ(t, "fleet", "threats", "--window", "30d")
	if err != nil {
		t.Fatal(err)
	}
	threats := tv["threats"].([]any)
	if len(threats) != 1 {
		t.Fatalf("threats = %v, want only aaaa (Acme + Bravo, case-insensitive)", threats)
	}
	th := threats[0].(map[string]any)
	if th["sha256"] != "aaaa" || th["companies"].(float64) != 2 || th["blocklisted"] != false {
		t.Fatalf("threat = %v", th)
	}
	if u := toStrings(tv["unblocked_hashes"]); len(u) != 1 || u[0] != "aaaa" {
		t.Fatalf("unblocked_hashes = %v, want [aaaa]", u)
	}
	nv, _, _ := runGZ(t, "fleet", "threats", "--window", "30d", "--min-companies", "3")
	if th, _ := nv["threats"].([]any); len(th) != 0 || nv["note"] == "" {
		t.Fatalf("min-companies 3 = %v, want none with a note", nv)
	}
}

func toStrings(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		out = append(out, x.(string))
	}
	return out
}

func TestParseSpanAndNormalizers(t *testing.T) {
	for in, want := range map[string]time.Duration{"7d": 7 * 24 * time.Hour, "2w": 14 * 24 * time.Hour, "36h": 36 * time.Hour} {
		if got, err := parseSpan(in); err != nil || got != want {
			t.Fatalf("parseSpan(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := parseSpan("soon"); err == nil {
		t.Fatal("parseSpan(soon) should fail")
	}
	if normalizeMAC("00-1A-2B-3C-4D-5E") != "001a2b3c4d5e" || normalizeMAC("bad") != "" {
		t.Fatal("normalizeMAC")
	}
	if shortHost("PC-A1.corp.local") != "pc-a1" || shortHost("10.0.0.1") != "10.0.0.1" {
		t.Fatal("shortHost")
	}
	if !seatSwing(10, 30) || seatSwing(100, 104) || seatSwing(10, 14) || !seatSwing(0, 6) {
		t.Fatal("seatSwing guard (more than 50% AND more than 5 seats)")
	}
}

func TestFleetHealthMarksTruncatedCompanyIncomplete(t *testing.T) {
	f := setupFakeGZ(t)
	f.bravoPages = 2
	v, stderr, err := runGZ(t, "fleet", "health", "--max-pages", "1")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range v["companies"].([]any) {
		m := c.(map[string]any)
		if m["name"] == "Bravo" {
			if e, _ := m["error"].(string); !strings.Contains(e, "incomplete") {
				t.Fatalf("Bravo = %v, want an incomplete-fetch error instead of a silent one-page count", m)
			}
		}
	}
	if !strings.Contains(stderr, "1 of 6 company fetches failed") {
		t.Fatalf("stderr %q lacks the partial-failure warning", stderr)
	}
	// --fail-on must not certify a fleet it could not read completely (failed company or capped tree).
	if _, _, err := runGZ(t, "fleet", "health", "--max-pages", "2", "--max-companies", "3", "--max-age", "0", "--fail-on", "isolated"); err == nil || !strings.Contains(err.Error(), "cannot be certified") {
		t.Fatalf("--fail-on with a capped tree: err = %v, want a fail-closed error", err)
	}
	if _, _, err := runGZ(t, "fleet", "health", "--max-pages", "1", "--fail-on", "isolated"); err == nil || !strings.Contains(err.Error(), "cannot be certified") {
		t.Fatalf("--fail-on with a failed fetch: err = %v, want a fail-closed error", err)
	}
	v, _, _ = runGZ(t, "fleet", "health", "--max-pages", "2")
	if tot := v["totals"].(map[string]any); tot["endpoints"].(float64) != 5 {
		t.Fatalf("with enough pages totals = %v, want 5 endpoints (Bravo's second page included)", tot)
	}
}

func TestIncidentQueueNewWindowAndCursor(t *testing.T) {
	f := setupFakeGZ(t)
	// --limit drops an incident, so even the first --new run must not start the cursor.
	if _, _, err := runGZ(t, "fleet", "incident-queue", "--new", "--limit", "1"); err != nil {
		t.Fatal(err)
	}
	v, _, err := runGZ(t, "fleet", "incident-queue", "--new")
	if err != nil {
		t.Fatal(err)
	}
	if v["changed_since"] != nil {
		t.Fatalf("cursor advanced after a --limit-truncated run: changed_since=%v", v["changed_since"])
	}
	v, _, _ = runGZ(t, "fleet", "incident-queue", "--new")
	since, _ := v["changed_since"].(string)
	if since == "" {
		t.Fatal("complete --new run did not advance the cursor")
	}
	var last map[string]any
	for _, c := range f.calls {
		if c.Method == "getIncidentsList" {
			last = c.Params
		}
	}
	fl := last["filters"].(map[string]any)
	if fl["changeStartDate"] != since || fl["changeEndDate"] == nil {
		t.Fatalf("third --new filters = %v, want changeStartDate=%s plus changeEndDate", fl, since)
	}
}

func TestThreatsBlocklistIsPerCompany(t *testing.T) {
	f := setupFakeGZ(t)
	f.blockAaaaAt = map[string]bool{idB: true} // blocked at Bravo only
	v, _, err := runGZ(t, "fleet", "threats", "--window", "30d")
	if err != nil {
		t.Fatal(err)
	}
	th := v["threats"].([]any)[0].(map[string]any)
	if th["blocklisted"] != false || strings.Join(toStrings(th["blocked_in"]), ",") != "Bravo" || strings.Join(toStrings(th["not_blocked_in"]), ",") != "Acme" {
		t.Fatalf("threat = %v, want blocked in Bravo only and still unblocked overall", th)
	}
	f.blockAaaaAt = map[string]bool{idA: true, idB: true}
	v, _, _ = runGZ(t, "fleet", "threats", "--window", "30d")
	th = v["threats"].([]any)[0].(map[string]any)
	if th["blocklisted"] != true || len(toStrings(v["unblocked_hashes"])) != 0 {
		t.Fatalf("blocked everywhere: threat = %v unblocked = %v", th, v["unblocked_hashes"])
	}
}

func TestUsageBillingWithholdsMathOnFailedFetches(t *testing.T) {
	f := setupFakeGZ(t)
	f.failUsage = map[string]bool{idP + "|08/2026": true, idB + "|09/2026": true}
	v, stderr, err := runGZ(t, "usage", "billing", "--month", "09/2026")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "usage fetches failed") {
		t.Fatalf("stderr %q lacks the failure warning", stderr)
	}
	sawPartner := false
	for _, r := range v["rows"].([]any) {
		m := r.(map[string]any)
		flags := strings.Join(toStrings(m["flags"]), ",")
		switch m["company_name"] {
		case "Partner MSP":
			sawPartner = true
			if m["own_share"] != nil || !strings.Contains(flags, "own_share_unavailable") || !strings.Contains(flags, "prior_month_unavailable") || strings.Contains(flags, "seat_swing") {
				t.Fatalf("partner row = %v, want no own share and no swing when Bravo's and the prior month's data are missing", m)
			}
		case "Bravo":
			t.Fatalf("Bravo's current month failed; it must not get a row: %v", m)
		}
	}
	if !sawPartner {
		t.Fatal("partner row missing")
	}
}

func TestIncompleteTreeIsNotCached(t *testing.T) {
	f := setupFakeGZ(t)
	f.failTreeFor = idS
	v, _, err := runGZ(t, "companies", "tree")
	if err != nil {
		t.Fatal(err)
	}
	if note, _ := v["note"].(string); !strings.Contains(note, "not cached") || len(v["fetch_failures"].([]any)) == 0 {
		t.Fatalf("tree view = %v, want fetch_failures and a not-cached note", v)
	}
	f.mu.Lock()
	before := len(f.calls)
	f.failTreeFor = ""
	f.mu.Unlock()
	if _, _, err := runGZ(t, "fleet", "health"); err != nil {
		t.Fatal(err)
	}
	walked := false
	for _, c := range f.calls[before:] {
		if c.Method == "getCompaniesList" {
			walked = true
		}
	}
	if !walked {
		t.Fatal("fleet health reused an incomplete cached tree instead of walking again")
	}
}

func TestRMMGapReportsAmbiguousHostnames(t *testing.T) {
	devices := []rmmDevice{{Hostname: "server"}, {Hostname: "server", MACs: []string{"00-1b-21-dd-ee-02"}},
		// A VMware VMnet8 adapter MAC is shared by every host that has it: it must not match.
		{Hostname: "SOME-OTHER-PC", MACs: []string{"00:50:56:c0:00:08"}}}
	eps := []snapEndpoint{
		{CompanyID: "c1", CompanyName: "One", EndpointID: "e1", Name: "SERVER", IsManaged: true, MACs: []string{"00:1b:21:dd:ee:01", "00:50:56:c0:00:08"}},
		{CompanyID: "c2", CompanyName: "Two", EndpointID: "e2", Name: "SERVER", IsManaged: false, MACs: []string{"00:1b:21:dd:ee:02"}},
	}
	v := compareRMM(devices, eps)
	if len(v.Ambiguous) != 1 || len(v.Ambiguous[0].Candidates) != 2 {
		t.Fatalf("ambiguous = %+v, want the MAC-less 'server' reported with both candidates", v.Ambiguous)
	}
	if len(v.UnmanagedInGZ) != 1 || v.UnmanagedInGZ[0].EndpointID != "e2" {
		t.Fatalf("unmanaged = %+v, want e2 matched by MAC despite the managed same-name endpoint in c1", v.UnmanagedInGZ)
	}
	if len(v.MissingInRMM) != 0 {
		t.Fatalf("missing_in_rmm = %+v, want none (both claimed)", v.MissingInRMM)
	}
	if len(v.MissingInGZ) != 1 || v.MissingInGZ[0].Hostname != "SOME-OTHER-PC" {
		t.Fatalf("missing_in_gravityzone = %+v, want SOME-OTHER-PC (its only MAC is a shared adapter MAC)", v.MissingInGZ)
	}
	if normalizeMAC("DESKTOP-ABC1234567") != "" || !identifyingMAC(normalizeMAC("00-1b-21-dd-ee-02")) || identifyingMAC(normalizeMAC("02:00:4c:4f:4f:50")) {
		t.Fatal("MAC shape / identifying checks")
	}
}

func TestRMMGapConflictingMACsAreAmbiguous(t *testing.T) {
	// One RMM device whose two NICs match two different GravityZone endpoints.
	devices := []rmmDevice{{Hostname: "dual", MACs: []string{"00:1b:21:00:00:01", "00:1b:21:00:00:02"}}}
	eps := []snapEndpoint{
		{CompanyID: "c1", CompanyName: "One", EndpointID: "e1", Name: "A", IsManaged: true, MACs: []string{"00:1b:21:00:00:01"}},
		{CompanyID: "c1", CompanyName: "One", EndpointID: "e2", Name: "B", IsManaged: false, MACs: []string{"00:1b:21:00:00:02"}},
	}
	v := compareRMM(devices, eps)
	if len(v.Ambiguous) != 1 || len(v.UnmanagedInGZ) != 0 {
		t.Fatalf("conflicting MACs: ambiguous=%+v unmanaged=%+v, want one ambiguous match and no guessed coverage", v.Ambiguous, v.UnmanagedInGZ)
	}
}

func TestIncidentQueueRejectsZeroPageBudget(t *testing.T) {
	setupFakeGZ(t)
	if _, _, err := runGZ(t, "fleet", "incident-queue", "--new", "--max-pages", "0"); err == nil || !strings.Contains(err.Error(), "--max-pages") {
		t.Fatalf("--max-pages 0: err = %v, want a usage error", err)
	}
}

func TestThreatsReportsTruncatedTree(t *testing.T) {
	setupFakeGZ(t)
	v, _, err := runGZ(t, "fleet", "threats", "--window", "30d", "--max-companies", "2", "--max-age", "0")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range v["fetch_failures"].([]any) {
		if f.(map[string]any)["scope"] == "company tree" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fetch_failures = %v, want the company-tree truncation reported", v["fetch_failures"])
	}
}

func TestReadBoundedFileRejectsSpecialFiles(t *testing.T) {
	if _, err := readBoundedFile("--devices", os.DevNull); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("readBoundedFile(%s) err = %v, want a not-a-regular-file error", os.DevNull, err)
	}
}

func TestUsageBillingWithholdsOwnShareOnTruncatedTree(t *testing.T) {
	setupFakeGZ(t)
	v, _, err := runGZ(t, "usage", "billing", "--month", "09/2026", "--max-companies", "2", "--max-age", "0")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range v["rows"].([]any) {
		m := r.(map[string]any)
		if m["company_name"] == "Partner MSP" {
			if m["own_share"] != nil || !strings.Contains(strings.Join(toStrings(m["flags"]), ","), "own_share_unavailable") {
				t.Fatalf("partner row on a truncated tree = %v, want own share withheld", m)
			}
			return
		}
	}
	t.Fatal("partner row missing")
}

func TestIncidentCursorIsPerFilterSet(t *testing.T) {
	setupFakeGZ(t)
	if _, _, err := runGZ(t, "fleet", "incident-queue", "--new", "--priority", "high"); err != nil {
		t.Fatal(err)
	}
	v, _, _ := runGZ(t, "fleet", "incident-queue", "--new")
	if v["changed_since"] != nil {
		t.Fatalf("an unfiltered --new run reused the --priority high cursor: %v", v["changed_since"])
	}
	v, _, _ = runGZ(t, "fleet", "incident-queue", "--new", "--priority", "HIGH")
	if v["changed_since"] == nil {
		t.Fatal("the --priority high cursor was not found again (keys must be case/order-insensitive)")
	}
}

func TestIncidentQueueReportsFailedLaterPage(t *testing.T) {
	f := setupFakeGZ(t)
	f.incidentPg2 = true
	old := incidentsPageGap
	incidentsPageGap = 0
	defer func() { incidentsPageGap = old }()
	v, _, err := runGZ(t, "fleet", "incident-queue", "--per-page", "10", "--max-pages", "2", "--new")
	if err != nil {
		t.Fatal(err)
	}
	if len(v["fetch_failures"].([]any)) != 1 || !strings.Contains(v["note"].(string), "partial queue") {
		t.Fatalf("view = %v, want the failed page in fetch_failures and a partial-queue note", v)
	}
	v, _, _ = runGZ(t, "fleet", "incident-queue", "--new")
	if v["changed_since"] != nil {
		t.Fatal("cursor advanced after a run with a failed page")
	}
}

func TestIncidentQueueRejectsUndocumentedFilters(t *testing.T) {
	setupFakeGZ(t)
	if _, _, err := runGZ(t, "fleet", "incident-queue", "--priority", "urgent"); err == nil || !strings.Contains(err.Error(), "--priority") {
		t.Fatalf("--priority urgent: err = %v, want a usage error", err)
	}
	// Mixed case is normalized to the documented lowercase value on the wire (the fake rejects others).
	if _, _, err := runGZ(t, "fleet", "incident-queue", "--priority", "HIGH", "--new"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGZ(t, "fleet", "incident-queue", "--priority", "high", "--new"); err != nil {
		t.Fatal(err) // same cursor, and the window end is strictly after its start even within one second
	}
}

func TestReRegistrationSkipsConflictingNICs(t *testing.T) {
	removed := []snapEndpoint{
		{CompanyID: "c", CompanyName: "C", EndpointID: "old1", Name: "OLD1", MACs: []string{"00:1b:21:00:00:01"}},
		{CompanyID: "c", CompanyName: "C", EndpointID: "old2", Name: "OLD2", MACs: []string{"00:1b:21:00:00:02"}},
	}
	added := []snapEndpoint{{CompanyID: "c", CompanyName: "C", EndpointID: "new", Name: "NEW", MACs: []string{"00:1b:21:00:00:01", "00:1b:21:00:00:02"}}}
	cos := map[string]idName{"c": {ID: "c", Name: "C"}}
	ok := map[string]bool{"c": true}
	kinds := map[string]int{}
	for _, ch := range diffFleet(removed, added, cos, ok, cos, ok, true) {
		kinds[ch.Kind]++
	}
	if kinds["re_registered"] != 0 || kinds["endpoint_added"] != 1 || kinds["endpoint_removed"] != 2 {
		t.Fatalf("changes = %v, want no guessed re-registration when two NICs point at different old records", kinds)
	}
}

func TestReRegistrationSkipsOneOldTwoNew(t *testing.T) {
	removed := []snapEndpoint{{CompanyID: "c", CompanyName: "C", EndpointID: "old", Name: "OLD", MACs: []string{"00:1b:21:00:00:01", "00:1b:21:00:00:02"}}}
	added := []snapEndpoint{
		{CompanyID: "c", CompanyName: "C", EndpointID: "n1", Name: "N1", MACs: []string{"00:1b:21:00:00:01"}},
		{CompanyID: "c", CompanyName: "C", EndpointID: "n2", Name: "N2", MACs: []string{"00:1b:21:00:00:02"}},
	}
	cos := map[string]idName{"c": {ID: "c", Name: "C"}}
	ok := map[string]bool{"c": true}
	kinds := map[string]int{}
	for _, ch := range diffFleet(removed, added, cos, ok, cos, ok, true) {
		kinds[ch.Kind]++
	}
	if kinds["re_registered"] != 0 || kinds["endpoint_added"] != 2 || kinds["endpoint_removed"] != 1 {
		t.Fatalf("changes = %v, want no guessed pairing when one old record matches two new ones", kinds)
	}
}

func TestReadAgreementsRejectsMalformedRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.csv")
	if err := os.WriteFile(path, []byte("company,quantity\nAcme\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readAgreements(path); err == nil || !strings.Contains(err.Error(), "record 2") {
		t.Fatalf("err = %v, want a record-numbered error", err)
	}
}

func TestRankIncidentsOrdersByCreatedWithinSeverity(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	items := []json.RawMessage{
		json.RawMessage(`{"incidentId":"newer","severityScore":50,"created":"2026-10-14T10:59:00Z"}`),
		json.RawMessage(`{"incidentId":"older","severityScore":50,"created":"2026-10-14T10:01:00Z"}`),
		json.RawMessage(`{"incidentId":"older","severityScore":50,"created":"2026-10-14T10:01:00Z"}`),
	}
	rows := rankIncidents(items, now, 0)
	if len(rows) != 2 || rows[0].IncidentID != "older" {
		t.Fatalf("rows = %+v, want 2 unique rows with the older incident first", rows)
	}
}

func TestReadRMMDevicesRejectsRowsWithoutHostname(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.csv")
	if err := os.WriteFile(path, []byte("asset_id,hostname\n42\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readRMMDevices(path); err == nil || !strings.Contains(err.Error(), "record 2") {
		t.Fatalf("err = %v, want a record-numbered error", err)
	}
}

func TestIncidentQueueHoldsCursorWhenPagesShift(t *testing.T) {
	f := setupFakeGZ(t)
	f.incidentShf = true
	old := incidentsPageGap
	incidentsPageGap = 0
	defer func() { incidentsPageGap = old }()
	v, _, err := runGZ(t, "fleet", "incident-queue", "--per-page", "10", "--max-pages", "3", "--new")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v["note"].(string), "changed while paging") {
		t.Fatalf("note = %v, want the shifted-pages warning", v["note"])
	}
	v, _, _ = runGZ(t, "fleet", "incident-queue", "--new")
	if v["changed_since"] != nil {
		t.Fatal("cursor advanced although incidents shifted between pages")
	}
}

func TestIncidentQueueQuietPrintsIDs(t *testing.T) {
	setupFakeGZ(t)
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"fleet", "incident-queue", "--quiet", "--no-cache"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != "inc-high\ninc-low" {
		t.Fatalf("--quiet output = %q, want one incident id per line, worst first", got)
	}
}

func TestParseRMMJSONRejectsRecordsWithoutHostname(t *testing.T) {
	if _, err := parseRMMJSON([]byte(`[{"hostname":"PC1"},{"asset_id":42}]`)); err == nil || !strings.Contains(err.Error(), "record 2") {
		t.Fatalf("err = %v, want a record-numbered error", err)
	}
}

func TestThreatsFullFinalPageAtCapIsComplete(t *testing.T) {
	f := setupFakeGZ(t)
	f.quarFull = true
	v, _, err := runGZ(t, "fleet", "threats", "--window", "30d", "--max-pages", "2")
	if err != nil {
		t.Fatal(err)
	}
	for _, ff := range v["fetch_failures"].([]any) {
		if strings.Contains(ff.(map[string]any)["scope"].(string), "Acme") {
			t.Fatalf("two full pages at --max-pages 2 reported incomplete: %v", ff)
		}
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestIncidentQueueQuietWriteFailureKeepsCursor(t *testing.T) {
	setupFakeGZ(t)
	cmd := RootCmd()
	cmd.SetOut(failingWriter{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"fleet", "incident-queue", "--quiet", "--new", "--no-cache"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("a failed --quiet write must fail the command")
	}
	v, _, _ := runGZ(t, "fleet", "incident-queue", "--new")
	if v["changed_since"] != nil {
		t.Fatal("cursor advanced although the output write failed")
	}
}

func TestFleetHealthMarksShiftedEndpointListIncomplete(t *testing.T) {
	f := setupFakeGZ(t)
	f.bravoShift = true
	v, _, err := runGZ(t, "fleet", "health")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range v["companies"].([]any) {
		m := c.(map[string]any)
		if m["name"] == "Bravo" {
			if e, _ := m["error"].(string); !strings.Contains(e, "changed while paging") {
				t.Fatalf("Bravo = %v, want the shifted-pages error", m)
			}
		}
	}
}

func TestFleetHealthFailOnCountsDetailFailures(t *testing.T) {
	f := setupFakeGZ(t)
	f.failDetail = true
	if _, _, err := runGZ(t, "fleet", "health", "--details", "1", "--fail-on", "isolated"); err == nil || !strings.Contains(err.Error(), "cannot be certified") {
		t.Fatalf("err = %v, want --fail-on to fail closed on a failed detail fetch", err)
	}
}

func TestCachedTreeHonorsMaxCompanies(t *testing.T) {
	setupFakeGZ(t)
	if _, _, err := runGZ(t, "companies", "tree"); err != nil { // caches the full tree
		t.Fatal(err)
	}
	v, _, err := runGZ(t, "fleet", "health", "--max-companies", "2")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(v["companies"].([]any)); n != 2 {
		t.Fatalf("cached tree scanned %d companies, want --max-companies 2 honored", n)
	}
}

func TestLocateMACQueryMatchesOnlyMACs(t *testing.T) {
	e := snapEndpoint{Name: "PC", Label: "spare 00:1b:21:00:00:09", MACs: []string{"00:1b:21:00:00:01"}}
	if endpointMatches(e, "00:1b:21:00:00:09") {
		t.Fatal("a MAC query matched a label instead of a MAC")
	}
	if _, err := parseRMMJSON([]byte(`{"devices":null}`)); err == nil {
		t.Fatal("a null devices collection must be rejected")
	}
}

func TestThreatsMarksShiftedQuarantineIncomplete(t *testing.T) {
	f := setupFakeGZ(t)
	f.quarShift = true
	v, _, err := runGZ(t, "fleet", "threats", "--window", "30d")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ff := range v["fetch_failures"].([]any) {
		if strings.Contains(ff.(map[string]any)["error"].(string), "changed while paging") {
			found = true
		}
	}
	if !found {
		t.Fatalf("fetch_failures = %v, want Acme's shifted quarantine reported", v["fetch_failures"])
	}
}

func TestIncidentQueueQuietWarnsOnFailedPage(t *testing.T) {
	f := setupFakeGZ(t)
	f.incidentPg2 = true
	old := incidentsPageGap
	incidentsPageGap = 0
	defer func() { incidentsPageGap = old }()
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"fleet", "incident-queue", "--quiet", "--no-cache", "--per-page", "10", "--max-pages", "2"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errb.String(), "partial queue") || strings.Contains(out.String(), "partial") {
		t.Fatalf("stdout=%q stderr=%q, want the partial-queue warning on stderr only", out.String(), errb.String())
	}
}

func TestRepeatedMACOnOneEndpointStillIdentifies(t *testing.T) {
	devices := []rmmDevice{{Hostname: "renamed", MACs: []string{"00:1b:21:00:00:07"}}}
	eps := []snapEndpoint{{CompanyID: "c", CompanyName: "C", EndpointID: "e", Name: "OLDNAME", IsManaged: true,
		MACs: []string{"00:1b:21:00:00:07", "00-1B-21-00-00-07"}}}
	v := compareRMM(devices, eps)
	if len(v.MissingInGZ) != 0 || len(v.MissingInRMM) != 0 {
		t.Fatalf("missing_gz=%v missing_rmm=%v, want the repeated MAC to still match its one endpoint", v.MissingInGZ, v.MissingInRMM)
	}
}

func TestBillingChecksAgreementWithoutUsageRows(t *testing.T) {
	tree := []gzCompany{{ID: "c1", Name: "Quiet Co", Type: 1}}
	rows := buildBillingRows(tree, map[string]usageCounts{"c1": {}}, map[string]usageCounts{"c1": {}}, nil, map[string]int{"quiet co": 5}, true)
	if len(rows) != 1 || !strings.Contains(strings.Join(rows[0].Flags, ","), "agreement_mismatch:0_billed_vs_5_agreed") {
		t.Fatalf("rows = %+v, want a mismatch row for an agreement with no usage", rows)
	}
}

func TestReRegistrationWithRepeatedMAC(t *testing.T) {
	removed := []snapEndpoint{{CompanyID: "c", CompanyName: "C", EndpointID: "old", Name: "OLD", MACs: []string{"00:1b:21:00:00:01", "00-1B-21-00-00-01"}}}
	added := []snapEndpoint{{CompanyID: "c", CompanyName: "C", EndpointID: "new", Name: "NEW", MACs: []string{"00:1b:21:00:00:01"}}}
	cos := map[string]idName{"c": {ID: "c", Name: "C"}}
	ok := map[string]bool{"c": true}
	kinds := map[string]int{}
	for _, ch := range diffFleet(removed, added, cos, ok, cos, ok, true) {
		kinds[ch.Kind]++
	}
	if kinds["re_registered"] != 1 {
		t.Fatalf("changes = %v, want one re-registration despite the repeated MAC", kinds)
	}
}

func TestIncidentQueueShortFirstPageKeepsCursor(t *testing.T) {
	f := setupFakeGZ(t)
	f.incShort = true
	v, _, err := runGZ(t, "fleet", "incident-queue", "--per-page", "10", "--new")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v["note"].(string), "changed while paging") {
		t.Fatalf("note = %v, want the short first page flagged", v["note"])
	}
	v, _, _ = runGZ(t, "fleet", "incident-queue", "--new")
	if v["changed_since"] != nil {
		t.Fatal("cursor advanced past incidents page 1 counted but never returned")
	}
}

func TestChangesSkipsNewCompanyWhoseFetchFailed(t *testing.T) {
	f := setupFakeGZ(t)
	if _, _, err := runGZ(t, "fleet", "health"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.extraCo, f.failComp = true, idE
	f.mu.Unlock()
	if _, _, err := runGZ(t, "fleet", "health", "--max-age", "0"); err != nil { // re-walk the tree
		t.Fatal(err)
	}
	cv, _, err := runGZ(t, "fleet", "changes", "--since", "1s")
	if err != nil {
		t.Fatal(err)
	}
	if cv["complete"] != false || !strings.Contains(strings.Join(toStrings(cv["skipped_companies"]), ","), "Echo") {
		t.Fatalf("complete=%v skipped=%v, want Echo's failed fetch to make the comparison incomplete", cv["complete"], cv["skipped_companies"])
	}
}

func TestReRegistrationIgnoresMACSharedWithUnchangedEndpoint(t *testing.T) {
	mac := []string{"00:1b:21:00:00:01"}
	a := snapEndpoint{CompanyID: "c", CompanyName: "C", EndpointID: "a", Name: "A", MACs: mac}
	b := snapEndpoint{CompanyID: "c", CompanyName: "C", EndpointID: "b", Name: "B", MACs: mac}
	c := snapEndpoint{CompanyID: "c", CompanyName: "C", EndpointID: "c", Name: "C1", MACs: mac}
	cos := map[string]idName{"c": {ID: "c", Name: "C"}}
	ok := map[string]bool{"c": true}
	for _, ch := range diffFleet([]snapEndpoint{a, b}, []snapEndpoint{b, c}, cos, ok, cos, ok, true) {
		if ch.Kind == "re_registered" {
			t.Fatalf("paired A->C on a MAC that unchanged B also carries: %+v", ch)
		}
	}
}

func TestDedupeIgnoresFailedPartialResults(t *testing.T) {
	results := []fleetCompanyResult{
		{Company: gzCompany{ID: "parent"}, Endpoints: []gzEndpoint{{ID: "e1"}}},
		{Company: gzCompany{ID: "child"}, Endpoints: []gzEndpoint{{ID: "e1"}}, Err: errors.New("changed while paging")},
	}
	got := dedupeEndpoints(results)
	if len(got[0].Endpoints) != 1 {
		t.Fatalf("parent endpoints = %v, want e1 kept when the deeper copy came from a failed fetch", got[0].Endpoints)
	}
}

func TestChangesIgnoreOwnershipShiftThroughSkippedCompany(t *testing.T) {
	cos := map[string]idName{"p": {ID: "p", Name: "Parent"}, "ch": {ID: "ch", Name: "Child"}, "q": {ID: "q", Name: "Other"}}
	both := map[string]bool{"p": true, "ch": true, "q": true}
	childFailed := map[string]bool{"p": true, "q": true}
	ep := func(co, id string) snapEndpoint {
		return snapEndpoint{CompanyID: co, CompanyName: cos[co].Name, EndpointID: id, Name: id}
	}
	// e1 shifts between Child and Parent through the skipped Child; the rest are
	// real: "old" leaves, "new" arrives, "mv" moves between two healthy companies.
	withChild := []snapEndpoint{ep("ch", "e1"), ep("p", "old"), ep("p", "mv")}
	withParent := []snapEndpoint{ep("p", "e1"), ep("p", "new"), ep("q", "mv")}
	want := map[string]string{"p/old": "endpoint_removed", "p/new": "endpoint_added", "p/mv": "endpoint_removed", "q/mv": "endpoint_added"}
	for name, tc := range map[string]struct {
		changes []fleetChange
		flip    bool
	}{
		"child fails":    {diffFleet(withChild, withParent, cos, both, cos, childFailed, true), false},
		"child recovers": {diffFleet(withParent, withChild, cos, childFailed, cos, both, true), true},
	} {
		got := map[string]string{}
		for _, c := range tc.changes {
			got[c.CompanyID+"/"+c.EndpointID] = c.Kind
		}
		for k, kind := range want {
			if tc.flip { // recovery runs the same history backwards
				kind = map[string]string{"endpoint_removed": "endpoint_added", "endpoint_added": "endpoint_removed"}[kind]
			}
			if got[k] != kind {
				t.Errorf("%s: %s = %q, want %q (all: %v)", name, k, got[k], kind, got)
			}
		}
		if len(got) != len(want) {
			t.Errorf("%s: changes = %v, want only the real changes, not the shifted e1", name, got)
		}
	}
}

func TestBillingZeroOwnShareSerializesAsEmptyObject(t *testing.T) {
	tree := []gzCompany{{ID: "p", Name: "P", Type: 0}, {ID: "c", Name: "C", Type: 1, ParentID: "p"}}
	u := map[string]usageCounts{"p": {0: {"endpointMonthlyUsage": 5}}, "c": {0: {"endpointMonthlyUsage": 5}}}
	rows := buildBillingRows(tree, u, u, nil, nil, true)
	raw, _ := json.Marshal(rows[0])
	if rows[0].CompanyID != "p" || !strings.Contains(string(raw), `"own_share":{}`) {
		t.Fatalf("row = %s, want a computed zero own_share as {}", raw)
	}
}

func TestHumanOutputForEmptyStates(t *testing.T) {
	setupFakeGZ(t)
	run := func(args ...string) string {
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(append(args, "--human-friendly"))
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return out.String()
	}
	if out := run("fleet", "changes"); strings.Contains(out, "No changes") {
		t.Fatalf("fresh install printed %q, want only the no-snapshots note", out)
	}
	path := filepath.Join(t.TempDir(), "d.csv")
	if err := os.WriteFile(path, []byte("hostname\nPC1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := run("fleet", "rmm-gap", "--devices", path, "--data-source", "local"); strings.Contains(out, "No gaps") {
		t.Fatalf("no inventory printed %q, want it to say nothing was compared", out)
	}
	if plural(1, "company", "companies") != "company" || plural(2, "company", "companies") != "companies" {
		t.Fatal("plural")
	}
}

func TestInventoryAgentSource(t *testing.T) {
	if got := inventoryAgentSource(inventoryResult{Source: "snapshot"}); got != "local" {
		t.Fatalf("snapshot answer reported meta.source %q, want local", got)
	}
	if got := inventoryAgentSource(inventoryResult{Source: "live"}); got != "live" {
		t.Fatalf("live answer reported meta.source %q, want live", got)
	}
}

// A company whose fetch failed must carry a status that survives --compact,
// so its zero counts are never read as a clean row (fleet-health-row-status).
func TestFleetHealthFailedRowStatus(t *testing.T) {
	view := buildFleetHealth([]fleetCompanyResult{
		{Company: gzCompany{ID: "c1", Name: "Acme Corp"}},
		{Company: gzCompany{ID: "c2", Name: "Northwind IT"}, Err: errors.New("boom")},
	}, 7, time.Now())
	got := map[string]string{}
	for _, c := range view.Companies {
		got[c.Name] = c.Status
	}
	if got["Acme Corp"] != "ok" || got["Northwind IT"] != "fetch_failed" {
		t.Fatalf("row status = %v, want Acme Corp ok and Northwind IT fetch_failed", got)
	}
	if !isCompactGravityField("status") {
		t.Fatal("status no longer survives --compact")
	}
}
