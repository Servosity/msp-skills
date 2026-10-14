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
	// A row missing the key part falls back to the old key, never to nothing.
	if id := ExtractResourceID("report_queries-ad-ous-view", map[string]any{"name": "Workstations"}); id == "" {
		t.Error("ad-ous-view row without object_guid lost its id")
	}
}
