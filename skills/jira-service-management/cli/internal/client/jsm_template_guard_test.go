// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import "testing"

func TestJSMTemplateGuard(t *testing.T) {
	base := "https://{site}.atlassian.net/rest/servicedeskapi"
	for _, bad := range []string{"evil.com/", "a:b@evil.com/", "x#", "x?y", "..", "a..b", "-x"} {
		if _, err := buildURL(base, "/servicedesk", map[string]string{"site": bad}); err == nil {
			t.Errorf("site %q must be rejected", bad)
		}
	}
	for _, good := range []string{"acme", "my-site", "site_placeholder", "fff89a1a-52c8-11e5-91b2-3c15c2bd8caa-ecosystem"} {
		got, err := buildURL(base, "/servicedesk", map[string]string{"site": good})
		if err != nil || got != "https://"+good+".atlassian.net/rest/servicedeskapi/servicedesk" {
			t.Errorf("site %q: got %q err %v", good, got, err)
		}
	}
	assets := "https://api.atlassian.com/ex/jira/{cloudId}/jsm/assets/workspace/{workspaceId}/v1/object/1"
	if _, err := buildURL("", assets, map[string]string{"cloudId": "..", "workspaceId": "w"}); err == nil {
		t.Error("cloudId '..' must be rejected (path traversal)")
	}
	if _, err := buildURL(base, "/x", map[string]string{}); err == nil {
		t.Error("unset site must still report the env var to set")
	}
}
