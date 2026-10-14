// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json per-client-report-keys.

package store

import "testing"

// TestPerClientReportRowsKeepDistinctIDs: two clients' rows that share a
// display name (every AD domain has an OU "Workstations" and a GPO "Default
// Domain Policy") must not collapse onto one storage id.
func TestPerClientReportRowsKeepDistinctIDs(t *testing.T) {
	type row = map[string]any
	cases := map[string][2]row{
		"report_queries-ad-ous-view":                                        {{"name": "Workstations", "company_id": 1, "object_guid": "g-a"}, {"name": "Workstations", "company_id": 2, "object_guid": "g-b"}},
		"report_queries-ad-gpos-view":                                       {{"name": "Default Domain Policy", "object_guid": "g-a"}, {"name": "Default Domain Policy", "object_guid": "g-b"}},
		"report_queries-ad-group-computers":                                 {{"name": "PC01", "object_guid": "g-a"}, {"name": "PC01", "object_guid": "g-b"}},
		"report_queries-get-groups-details":                                 {{"name": "Domain Admins", "company_name": "Acme Corp", "canonical_name": "acme.local/Users/Domain Admins"}, {"name": "Domain Admins", "company_name": "Northwind IT", "canonical_name": "nw.local/Users/Domain Admins"}},
		"report_queries-companies-by-problem-group":                         {{"name": "Acme", "company_id": 1}, {"name": "Acme", "company_id": 2}},
		"report_queries-ports-view":                                         {{"name": "Acme", "company_id": 1}, {"name": "Acme", "company_id": 2}},
		"report_queries-remediation-companies":                              {{"name": "Acme", "company_id": 1}, {"name": "Acme", "company_id": 2}},
		"report_queries-compliance-check-company-count":                     {{"name": "Acme Corp", "company_id": 1, "compliance_id": 7}, {"name": "Acme Corp", "company_id": 1, "compliance_id": 8}},
		"report_queries-distinct-tags":                                      {{"name": "site", "value": "east"}, {"name": "site", "value": "west"}},
		"report_queries-remediate-records":                                  {{"name": "Chrome", "solution_id": 11}, {"name": "Chrome", "solution_id": 12}},
		"report_queries-application-vulnerabilities-patching-asset-details": {{"name": "Chrome", "agent_id": "a1", "version": "1"}, {"name": "Chrome", "agent_id": "a2", "version": "1"}},
		"report_queries-asset-wise-vulnerabilities":                         {{"name": "CVE-1", "host_name": "pc1", "ip": "10.0.0.1", "vul_id": 5}, {"name": "CVE-1", "host_name": "pc2", "ip": "10.0.0.2", "vul_id": 5}},
	}
	for res, rows := range cases {
		a, b := ExtractResourceID(res, rows[0]), ExtractResourceID(res, rows[1])
		if a == "" || a == b {
			t.Errorf("%s: ids %q and %q collapse two clients' rows", res, a, b)
		}
	}
	// Rows the round-17 keys still merged (Codex round 19): one discovery
	// agent's two machines, a check's pass and fail buckets.
	more := []struct {
		res  string
		a, b row
	}{
		{"report_queries-application-vulnerabilities-patching-asset-details", row{"name": "Chrome", "agent_id": "a1", "version": "1", "host_name": "pc1", "ip": "10.0.0.1"}, row{"name": "Chrome", "agent_id": "a1", "version": "1", "host_name": "pc2", "ip": "10.0.0.2"}},
		{"report_queries-compliance-check-company-count", row{"company_id": 1, "compliance_id": 7, "compliance_status": "pass"}, row{"company_id": 1, "compliance_id": 7, "compliance_status": "fail"}},
		// A string "0" is a real tag value, not a placeholder id (Codex round 20).
		{"report_queries-distinct-tags", row{"name": "rack", "value": "0"}, row{"name": "rack", "value": "00"}},
	}
	for _, m := range more {
		a, b := ExtractResourceID(m.res, m.a), ExtractResourceID(m.res, m.b)
		if a == "" || a == b {
			t.Errorf("%s: ids %q and %q collapse two distinct rows", m.res, a, b)
		}
	}
	// asset-wise-vulnerabilities: one CVE on two ports of a host stays two
	// rows. A row with no port still keys on host, ip and CVE, never on name
	// (Codex round 17).
	aw := "report_queries-asset-wise-vulnerabilities"
	finding := func(extra row) row {
		r := row{"name": "CVE-1", "host_name": "pc1", "ip": "10.0.0.1", "vul_id": 5}
		for k, v := range extra {
			r[k] = v
		}
		return r
	}
	ids := map[string]bool{}
	// Two clients' identical builds differ by client id when the API sends
	// one, else by when the finding was first discovered (Codex round 18). A
	// row with no host name keys on its IP (adversarial round 19).
	for _, r := range []row{finding(nil), finding(row{"port": 443}), finding(row{"port": 8443}),
		finding(row{"company_id": 1}), finding(row{"company_id": 2}), finding(row{"discovered": "2026-09-01T10:00:00Z"}), finding(row{"discovered": "2026-09-02T10:00:00Z"}),
		{"name": "CVE-1", "host_name": "", "ip": "10.0.1.5", "vul_id": 5}, {"name": "CVE-1", "ip": "10.0.1.6", "vul_id": 5}} {
		id := ExtractResourceID(aw, r)
		if id == "" || id == "CVE-1" || ids[id] {
			t.Errorf("%s: row %v got id %q (empty, name, or a collision)", aw, r, id)
		}
		ids[id] = true
	}
	// Array fields (software_name, port lists, app name/version/path) are
	// per-row aggregates that change and reorder between syncs. They must not
	// be key parts, or a plain sync adds a row beside the stale one
	// (adversarial round 21).
	for res, r := range map[string][2]row{
		aw: {finding(row{"software_name": []any{"Chrome", "Edge"}}), finding(row{"software_name": []any{"Edge", "Chrome"}})},
		"report_queries-problems-summary-group-by-companies":                {{"problem_name": "CVE-1", "company_id": 1, "software_name": []any{"Chrome", "Edge"}, "port": []any{"443"}}, {"problem_name": "CVE-1", "company_id": 1, "software_name": []any{"Edge"}, "port": []any{}}},
		"report_queries-application-vulnerabilities-patching-asset-details": {{"agent_id": 1, "host_name": "pc1", "name": []any{"Chrome"}, "version": []any{"1"}, "path": []any{"/a"}}, {"agent_id": 1, "host_name": "pc1", "name": []any{"Chrome"}, "version": []any{"1"}, "path": []any{"/b"}}},
	} {
		if a, b := ExtractResourceID(res, r[0]), ExtractResourceID(res, r[1]); a == "" || a != b {
			t.Errorf("%s: a changed array field moved the row's id (%q vs %q)", res, a, b)
		}
	}
	// A row missing the key part falls back to the old key, never to nothing.
	if id := ExtractResourceID("report_queries-ad-ous-view", map[string]any{"name": "Workstations"}); id == "" {
		t.Error("ad-ous-view row without object_guid lost its id")
	}
}
