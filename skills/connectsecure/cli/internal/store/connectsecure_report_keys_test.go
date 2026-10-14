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
	// asset-wise-vulnerabilities: one CVE on two ports of a host, or in two
	// products on it, stays two rows. A row with no port or product still
	// keys on host, ip and CVE, never on name (Codex round 17).
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
	for _, r := range []row{finding(nil), finding(row{"port": 443}), finding(row{"port": 8443}), finding(row{"port": 443, "software_name": "Chrome"}), finding(row{"port": 443, "software_name": "Edge"}),
		finding(row{"company_id": 1}), finding(row{"company_id": 2}), finding(row{"discovered": "2026-09-01T10:00:00Z"}), finding(row{"discovered": "2026-09-02T10:00:00Z"}),
		{"name": "CVE-1", "host_name": "", "ip": "10.0.1.5", "vul_id": 5}, {"name": "CVE-1", "ip": "10.0.1.6", "vul_id": 5}} {
		id := ExtractResourceID(aw, r)
		if id == "" || id == "CVE-1" || ids[id] {
			t.Errorf("%s: row %v got id %q (empty, name, or a collision)", aw, r, id)
		}
		ids[id] = true
	}
	// A row missing the key part falls back to the old key, never to nothing.
	if id := ExtractResourceID("report_queries-ad-ous-view", map[string]any{"name": "Workstations"}); id == "" {
		t.Error("ad-ous-view row without object_guid lost its id")
	}
}
