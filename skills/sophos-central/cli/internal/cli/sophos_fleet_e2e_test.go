// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"sophos-central-pp-cli/internal/cliutil/testenv"
)

// fakeSophosAPI serves just enough of Sophos Central for fleet refresh: token,
// whoami, a partner tenant list, key-paged endpoints, alerts, health check
// and licences. Tenant T3 answers 403 like a tenant without Partner Assistance.
func fakeSophosAPI(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	write := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := r.Header.Get("X-Tenant-ID")
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path+" tenant="+tenant+" key="+r.URL.Query().Get("pageFromKey")+" cursor="+r.URL.Query().Get("cursor"))
		mu.Unlock()
		switch {
		case r.URL.Path == "/token":
			_ = r.ParseForm()
			if r.PostForm.Get("client_id") == "" {
				write(w, 500, map[string]string{"errorCode": "SERVER_ERROR"})
				return
			}
			write(w, 200, map[string]any{"access_token": "tok", "token_type": "bearer", "expires_in": 3600})
		case r.URL.Path == "/whoami/v1":
			write(w, 200, map[string]any{"id": "P1", "idType": "partner", "apiHosts": map[string]string{"global": "https://api.central.sophos.com"}})
		case r.URL.Path == "/partner/v1/tenants":
			write(w, 200, map[string]any{"items": []map[string]any{
				{"id": "T1", "name": "Acme Dental", "billingType": "term", "apiHost": "https://api-us01.central.sophos.com"},
				{"id": "T2", "name": "Trial Co", "billingType": "trial", "apiHost": "https://api-us01.central.sophos.com"},
				{"id": "T3", "name": "Locked Ltd", "billingType": "term", "apiHost": "https://api-eu01.central.sophos.com"},
			}, "pages": map[string]int{"current": 1, "total": 1}})
		case tenant == "T3":
			write(w, 403, map[string]string{"error": "forbidden", "message": "Forbidden"})
		case r.URL.Path == "/endpoint/v1/endpoints":
			ep := func(id, host, health string) map[string]any {
				return map[string]any{"id": id, "hostname": host, "tenant": map[string]any{"id": tenant}, "health": map[string]any{"overall": health},
					"tamperProtectionSupported": true, "tamperProtectionEnabled": true, "isolation": map[string]any{"status": "notIsolated"}}
			}
			if r.URL.Query().Get("pageFromKey") == "" {
				write(w, 200, map[string]any{"items": []any{ep("aaaaaaaa-0000-0000-0000-000000000001", "WS-FIN-07", "bad")}, "pages": map[string]any{"nextKey": "k2"}})
			} else {
				write(w, 200, map[string]any{"items": []any{ep("aaaaaaaa-0000-0000-0000-000000000002", "WS-FIN-08", "good")}, "pages": map[string]any{}})
			}
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/endpoint/v1/endpoints/aaaaaaaa-"):
			write(w, 200, map[string]any{"id": strings.TrimPrefix(r.URL.Path, "/endpoint/v1/endpoints/"), "hostname": "WS-FIN-07", "isolation": map[string]any{"status": "notIsolated"}, "health": map[string]any{"overall": "bad"}})
		case r.Method == "POST" && r.URL.Path == "/endpoint/v1/endpoints/isolation":
			write(w, 202, map[string]any{"items": []any{map[string]any{"id": "aaaaaaaa-0000-0000-0000-000000000001", "isolation": map[string]any{"enabled": true}}}})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/scans"):
			write(w, 201, map[string]any{"id": "scan-1", "status": "requested"})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/actions"):
			write(w, 201, map[string]any{"id": "act-1", "action": "acknowledge"})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/tamper-protection"):
			write(w, 201, map[string]any{"enabled": true, "password": "SECRET-TP-PASSWORD"})
		case r.URL.Path == "/endpoint/v1/settings/tamper-protection":
			write(w, 200, map[string]any{"enabled": true})
		case r.URL.Path == "/siem/v1/events" || r.URL.Path == "/siem/v1/alerts":
			if r.URL.Query().Get("cursor") == "" {
				write(w, 200, map[string]any{"has_more": true, "next_cursor": "c2", "items": []any{map[string]any{"id": "ev1"}}})
			} else {
				write(w, 200, map[string]any{"has_more": false, "next_cursor": "c3", "items": []any{map[string]any{"id": "ev2"}}})
			}
		case strings.HasPrefix(r.URL.Path, "/partner/v1/billing/usage/"):
			write(w, 200, map[string]any{"items": []any{
				map[string]any{"accountId": "T1", "billableQuantity": 5, "unitPrice": "4.00", "unitPriceCurrency": "USD", "productDescription": "Intercept X"},
			}, "pages": map[string]any{}})
		case r.URL.Path == "/common/v1/alerts":
			write(w, 200, map[string]any{"items": []any{map[string]any{"id": "bbbbbbbb-0000-0000-0000-000000000001", "severity": "high", "description": "Malware detected",
				"raisedAt": "2026-10-13T10:00:00Z", "tenant": map[string]any{"id": tenant, "name": "Acme Dental"},
				"managedAgent": map[string]any{"id": "aaaaaaaa-0000-0000-0000-000000000001"}, "allowedActions": []string{"acknowledge"}}}, "pages": map[string]any{}})
		case r.URL.Path == "/account-health-check/v1/health-check":
			write(w, 200, map[string]any{"endpoint": map[string]any{"protection": map[string]any{"computer": map[string]any{"score": 72, "snoozed": false}}}})
		case r.URL.Path == "/licenses/v1/licenses":
			write(w, 200, map[string]any{"licenses": []any{}})
		default:
			write(w, 404, map[string]string{"error": "notFound"})
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func runRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := RootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append(args, "--rate-limit", "0"))
	err := root.Execute()
	if err != nil {
		t.Logf("stderr: %s", errOut.String())
	}
	return out.String(), err
}

func TestFleetSyncThenTriageEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	srv, seen := fakeSophosAPI(t)
	t.Setenv("SOPHOS_CENTRAL_BASE_URL", srv.URL)
	t.Setenv("SOPHOS_CENTRAL_TOKEN_URL", srv.URL+"/token")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_ID", "cid")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_SECRET", "secret")
	t.Setenv(sophosTenantEnv, "")
	sophosTenantFlag = ""
	db := filepath.Join(t.TempDir(), "fleet.db")

	out, err := runRoot(t, "fleet", "refresh", "--db", db, "--json")
	if err != nil {
		t.Fatalf("fleet refresh: %v\n%s", err, out)
	}
	var summary fleetSyncSummary
	if err := json.Unmarshal([]byte(out), &summary); err != nil {
		t.Fatalf("fleet refresh output is not the summary JSON: %v\n%s", err, out)
	}
	// Trial tenant skipped by default; T3's 403 recorded, not fatal.
	if summary.TenantsTotal != 2 || summary.OK != 1 || summary.Forbidden != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if summary.Stored["endpoints"] != 2 {
		t.Fatalf("both key pages of endpoints should be stored, got %d (requests %v)", summary.Stored["endpoints"], *seen)
	}
	joined := strings.Join(*seen, "\n")
	if !strings.Contains(joined, "/endpoint/v1/endpoints tenant=T1 key=k2") {
		t.Fatalf("second key page never requested:\n%s", joined)
	}
	if strings.Contains(joined, "tenant=T2") {
		t.Fatalf("trial tenant T2 was called without --include-trial:\n%s", joined)
	}

	out, err = runRoot(t, "fleet", "triage", "--db", db, "--json")
	if err != nil {
		t.Fatalf("fleet triage: %v", err)
	}
	var view triageView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("triage output: %v\n%s", err, out)
	}
	kinds := map[string]bool{}
	for _, r := range view.Items {
		kinds[r.Kind] = true
		if r.TenantID != "T1" {
			t.Fatalf("row from unexpected tenant: %+v", r)
		}
	}
	if !kinds["alert"] || !kinds["unhealthy"] {
		t.Fatalf("triage should show the high alert and the bad endpoint, got %+v", view.Items)
	}
	if len(view.BlindTenants) != 1 || view.BlindTenants[0].TenantID != "T3" {
		t.Fatalf("blind tenants = %+v, want T3", view.BlindTenants)
	}

	out, err = runRoot(t, "fleet", "health", "--db", db, "--json")
	if err != nil {
		t.Fatalf("fleet health: %v", err)
	}
	var hv healthView
	if err := json.Unmarshal([]byte(out), &hv); err != nil || len(hv.Items) != 2 || hv.Items[0].Overall == nil || *hv.Items[0].Overall != 72 {
		t.Fatalf("fleet health = %+v err=%v\n%s", hv, err, out)
	}

	// contain: preview changes nothing, --yes executes with receipts.
	out, err = runRoot(t, "contain", "WS-FIN-07", "--scan", "--ack-alerts", "--db", db, "--json")
	if err != nil {
		t.Fatalf("contain preview: %v\n%s", err, out)
	}
	var plan containPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil || plan.Mode != "preview" || len(plan.Targets) != 1 || plan.Targets[0].TenantID != "T1" {
		t.Fatalf("contain preview = %+v err=%v", plan, err)
	}
	actions := map[string]int{}
	for _, a := range plan.Actions {
		actions[a.Action]++
		if a.Status != "planned" {
			t.Fatalf("preview must not execute: %+v", a)
		}
	}
	if actions["isolate"] != 1 || actions["scan"] != 1 || actions["ack_alert"] != 1 {
		t.Fatalf("planned actions = %v", actions)
	}
	if strings.Contains(strings.Join(*seen, "\n"), "POST /endpoint/v1/endpoints/isolation") {
		t.Fatal("preview sent an isolation request")
	}
	out, err = runRoot(t, "contain", "WS-FIN-07", "--scan", "--ack-alerts", "--db", db, "--json", "--yes")
	if err != nil {
		t.Fatalf("contain --yes: %v\n%s", err, out)
	}
	plan = containPlan{}
	_ = json.Unmarshal([]byte(out), &plan)
	for _, a := range plan.Actions {
		if a.Status != "ok" || len(a.Receipt) == 0 {
			t.Fatalf("executed action without receipt: %+v", a)
		}
	}
	if !strings.Contains(strings.Join(*seen, "\n"), "POST /endpoint/v1/endpoints/isolation tenant=T1") {
		t.Fatal("isolation request did not reach tenant T1")
	}
	// Unknown host: not found (exit 3), not a silent success.
	if _, err := runRoot(t, "contain", "NO-SUCH-HOST", "--db", db, "--json"); err == nil {
		t.Fatal("unknown host must fail")
	}

	// fleet tamper --yes enables and never prints the password.
	out, err = runRoot(t, "fleet", "tamper", "--json", "--yes")
	if err != nil {
		t.Fatalf("fleet tamper: %v\n%s", err, out)
	}
	if strings.Contains(out, "SECRET-TP-PASSWORD") {
		t.Fatal("tamper password leaked into output")
	}
	var tv tamperView
	if err := json.Unmarshal([]byte(out), &tv); err != nil || tv.Mode != "executed" || tv.Unreadable != 1 {
		t.Fatalf("fleet tamper = %+v err=%v", tv, err)
	}

	// fleet siem-collect: NDJSON, cursor saved, second run resumes from it.
	out, err = runRoot(t, "fleet", "siem-collect", "--stream", "events", "--db", db)
	if err != nil {
		t.Fatalf("fleet siem-collect: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	events := 0
	for _, l := range lines {
		var rec siemRecord
		if json.Unmarshal([]byte(l), &rec) != nil {
			t.Fatalf("non-JSON line %q", l)
		}
		if rec.RecordType == "event" {
			events++
		}
	}
	if events != 2 {
		t.Fatalf("want 2 events across 2 pages, got %d:\n%s", events, out)
	}
	before := len(*seen)
	if _, err := runRoot(t, "fleet", "siem-collect", "--stream", "events", "--db", db); err != nil {
		t.Fatal(err)
	}
	firstResume := ""
	for _, s := range (*seen)[before:] {
		if strings.Contains(s, "/siem/v1/events tenant=T1") {
			firstResume = s
			break
		}
	}
	// The first run ended on page 2 with next_cursor c3; the next run must
	// start exactly there, not from a from_date window.
	if !strings.HasSuffix(firstResume, "cursor=c3") {
		t.Fatalf("second siem run should resume at the saved cursor c3, first request was %q", firstResume)
	}

	// fleet revenue joins accountId to the synced tenant.
	out, err = runRoot(t, "fleet", "revenue", "--month", "2026-09", "--db", db, "--json")
	if err != nil {
		t.Fatalf("fleet revenue: %v\n%s", err, out)
	}
	var bv billingView
	if err := json.Unmarshal([]byte(out), &bv); err != nil || len(bv.Items) != 1 || !bv.Items[0].Matched || bv.Items[0].Revenue["USD"] != 20 {
		t.Fatalf("fleet revenue = %+v err=%v", bv, err)
	}
	if bv.Items[0].Variance == nil || *bv.Items[0].Variance != 3 {
		t.Fatalf("billable 5 - deployed 2 = 3, got %v", bv.Items[0].Variance)
	}

	// The generated commands carry the key-paging hand-fix: --all walks both
	// pages, and generic sync follows pages.nextKey too.
	t.Setenv(sophosTenantEnv, "T1")
	before = len(*seen)
	out, err = runRoot(t, "endpoints", "list", "--all", "--json")
	if err != nil {
		t.Fatalf("endpoints list --all: %v\n%s", err, out)
	}
	if !strings.Contains(out, "WS-FIN-07") || !strings.Contains(out, "WS-FIN-08") {
		t.Fatalf("endpoints list --all should return both key pages:\n%s", out)
	}
	before = len(*seen)
	if out, err = runRoot(t, "sync", "--resources", "endpoints", "--db", filepath.Join(t.TempDir(), "sync.db"), "--json"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	paged := false
	for _, s := range (*seen)[before:] {
		if strings.Contains(s, "/endpoint/v1/endpoints tenant=T1 key=k2") {
			paged = true
		}
	}
	if !paged {
		t.Fatalf("generic sync did not follow pages.nextKey:\n%s", strings.Join((*seen)[before:], "\n"))
	}
}
