// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestJSMWireParam(t *testing.T) {
	assets := "https://api.atlassian.com/ex/jira/{cloudId}/jsm/assets/workspace/{workspaceId}/v1/objectschema/list"
	cases := []struct{ path, name, want string }{
		{"/servicedesk", "offset", "start"},
		{"/request/{issueIdOrKey}/comment", "offset", "start"},
		{"/servicedesk", "limit", "limit"},
		{assets, "offset", "startAt"},
		{assets, "per_page", "resultPerPage"},
		{assets, "maxResults", "maxResults"},
		{"/servicedesk", "per_page", "per_page"},
		{"/servicedesk", "", ""},
	}
	for _, tc := range cases {
		if got := jsmWireParam(tc.path, tc.name); got != tc.want {
			t.Errorf("jsmWireParam(%q, %q) = %q, want %q", tc.path, tc.name, got, tc.want)
		}
	}
}

func TestJSMSyncPaginationMapsServicedesk(t *testing.T) {
	p := jsmSyncPagination("servicedesk")
	if p.cursorParam != "start" || p.limitParam != "limit" || p.limit > jsmMaxPageSize {
		t.Fatalf("servicedesk sync pagination = %+v, want start/limit", p)
	}
	p = jsmSyncPagination("objectschema")
	if p.cursorParam != "startAt" {
		t.Fatalf("objectschema sync pagination = %+v, want startAt", p)
	}
}
