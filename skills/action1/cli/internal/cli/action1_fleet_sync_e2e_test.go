// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"action1-pp-cli/internal/cliutil"
	"action1-pp-cli/internal/cliutil/testenv"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// action1 handfixes org-fan-out-sync + paging-follows-next-page: `sync` walks
// every organization for the four org-scoped lists the fleet novels read, and
// follows ResultPage.next_page. The fake answers ONE record per page whatever
// limit is asked (a server cap below the requested page size), and the same
// CVE exists in two organizations.
func TestFleetNovelsReadWhatSyncWrites(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]string{} // path -> raw query of the first request
	page := func(w http.ResponseWriter, r *http.Request, all []map[string]any) {
		from, _ := strconv.Atoi(r.URL.Query().Get("from"))
		items := []map[string]any{}
		next := ""
		if from < len(all) {
			items = all[from : from+1]
			if from+1 < len(all) {
				next = fmt.Sprintf("/API%s?from=%d&limit=1", r.URL.Path, from+1)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "ResultPage", "items": items, "total_items": len(all), "next_page": next, "prev_page": ""})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if _, ok := seen[r.URL.Path]; !ok {
			seen[r.URL.Path] = r.URL.RawQuery
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		org := ""
		if parts := strings.Split(strings.Trim(p, "/"), "/"); len(parts) >= 2 {
			org = parts[len(parts)-1]
			if strings.HasSuffix(p, "/data") {
				org = parts[1]
			}
		}
		switch {
		case p == "/organizations":
			page(w, r, []map[string]any{{"id": "org-a", "name": "Alpha"}, {"id": "org-b", "name": "Bravo"}})
		case strings.HasPrefix(p, "/endpoints/managed/"):
			eps := map[string][]map[string]any{
				"org-a": {
					{"id": "ep-1", "name": "WS-ONE", "organization_id": "org-a", "missing_updates": map[string]any{"critical": 5, "other": 1}, "reboot_required": "Yes"},
					{"id": "ep-2", "name": "WS-TWO", "organization_id": "org-a", "missing_updates": map[string]any{"critical": 1, "other": 0}, "reboot_required": "No"},
				},
				"org-b": {{"id": "ep-3", "name": "SRV-THREE", "organization_id": "org-b", "missing_updates": map[string]any{"critical": 2, "other": 2}, "reboot_required": "No"}},
			}
			page(w, r, eps[org])
		case strings.HasPrefix(p, "/vulnerabilities/"):
			v := map[string][]map[string]any{
				"org-a": {{"cve_id": "CVE-2025-0001", "cvss_score": "9.8", "cisa_kev": "Yes", "endpoints_count": 4, "organization_id": "org-a"}},
				"org-b": {
					{"cve_id": "CVE-2025-0001", "cvss_score": "9.8", "cisa_kev": "Yes", "endpoints_count": 3, "organization_id": "org-b"},
					{"cve_id": "CVE-2025-0002", "cvss_score": "5.0", "cisa_kev": "No", "endpoints_count": 1, "organization_id": "org-b"},
				},
			}
			page(w, r, v[org])
		case strings.HasPrefix(p, "/automations/instances/"):
			page(w, r, []map[string]any{{"id": "inst-" + org, "status": "Failed", "percent_completed": 50}})
		case strings.HasPrefix(p, "/installed-software/") && strings.HasSuffix(p, "/data"):
			page(w, r, []map[string]any{{"id": "app-" + org, "name": "Google Chrome", "version": "120.0"}})
		default:
			page(w, r, nil)
		}
	}))
	defer srv.Close()

	dir := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir) // Windows reads USERPROFILE, not HOME
	t.Setenv("ACTION1_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("ACTION1_BASE_URL", srv.URL)
	t.Setenv("ACTION1_OAUTH2", "e2e-static-token-0123456789")
	db := filepath.Join(dir, "fleet.db")

	run := func(args ...string) []byte {
		t.Helper()
		var out, errb bytes.Buffer
		cmd := newRootCmd(&rootFlags{})
		cmd.SetOut(&out)
		cmd.SetErr(&errb)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\nstdout: %s\nstderr: %s", args, err, out.String(), errb.String())
		}
		return out.Bytes()
	}
	run("sync", "--db", db, "--json")

	if q := seen["/endpoints/managed/org-a"]; !strings.Contains(q, "fields=%2A") && !strings.Contains(q, "fields=*") {
		t.Errorf("endpoint sync must ask for fields=* (missing_updates summaries); query was %q", q)
	}

	var posture []patchPostureRow
	if err := json.Unmarshal(run("fleet", "patch-posture", "--db", db, "--json"), &posture); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range posture {
		got[r.EndpointID] = r.OrganizationID
	}
	for ep, org := range map[string]string{"ep-1": "org-a", "ep-2": "org-a", "ep-3": "org-b"} {
		if got[ep] != org {
			t.Errorf("patch-posture: endpoint %s in %q, want %q (all pages of every org synced); rows %+v", ep, got[ep], org, posture)
		}
	}

	triage := string(run("fleet", "vuln-triage", "--db", db, "--json"))
	if !strings.Contains(triage, "CVE-2025-0002") {
		t.Errorf("vuln-triage misses org-b's second-page CVE: %s", triage)
	}
	var vrows []vulnTriageRow
	if err := json.Unmarshal([]byte(triage), &vrows); err != nil {
		t.Fatalf("vuln-triage output: %v\n%s", err, triage)
	}
	perOrg := map[string]int{}
	for _, r := range vrows {
		if r.CVEID == "CVE-2025-0001" {
			perOrg[r.OrganizationID] = r.EndpointsAffected
		}
	}
	if perOrg["org-a"] != 4 || perOrg["org-b"] != 3 {
		t.Errorf("CVE-2025-0001 exists in two organizations (4 and 3 endpoints); one org's row overwrote the other: %v", perOrg)
	}

	var auto automationHealthResult
	if err := json.Unmarshal(run("fleet", "automation-health", "--db", db, "--json"), &auto); err != nil {
		t.Fatal(err)
	}
	if auto.TotalInstances != 2 {
		t.Errorf("automation-health: want the instance of each organization (2), got %+v", auto)
	}
	var apps []softwareRollupRow
	if err := json.Unmarshal(run("fleet", "software-rollup", "--db", db, "--json"), &apps); err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].Installs != 2 || apps[0].Orgs != 2 {
		t.Errorf("software-rollup: Chrome is installed in both organizations (records carry no organization_id; the walk's parent is the org): %+v", apps)
	}
}
