// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"jira-service-management-pp-cli/internal/config"
)

func TestRequestGuard(t *testing.T) {
	calls := 0
	prev := jsmFetchTenantCloudID
	jsmFetchTenantCloudID = func(_ context.Context, _ *http.Client, site string) (string, error) {
		calls++
		return "cloud-" + site, nil
	}
	t.Cleanup(func() { jsmFetchTenantCloudID = prev; jsmTenantCloudIDs.Clear() })
	c := &Client{Config: &config.Config{TemplateVars: map[string]string{"site": "acme"}}}
	send := func(raw string) (*http.Request, error) {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		return req, jsmBeforeRequest(context.Background(), c, req)
	}

	req, err := send("https://acme.atlassian.net/rest/servicedeskapi/requesttype")
	if err != nil || req.Header.Get("X-ExperimentalApi") != "opt-in" {
		t.Fatalf("request API call: err=%v header=%q", err, req.Header.Get("X-ExperimentalApi"))
	}
	if _, err := send("https://api.atlassian.com/ex/jira/cloud-acme/jsm/assets/workspace/w/v1/object/1"); err != nil {
		t.Fatalf("own site's cloudId refused: %v", err)
	}
	req, err = send("https://api.atlassian.com/ex/jira/cloud-beta/jsm/assets/workspace/w/v1/object/1")
	if err == nil || !strings.Contains(err.Error(), "not site acme's cloudId") {
		t.Fatalf("another site's cloudId allowed: %v", err)
	}
	if req.Header.Get("X-ExperimentalApi") != "" {
		t.Fatal("Assets call got the request-API header")
	}
	if calls != 1 {
		t.Fatalf("tenant_info fetched %d times, want once per site", calls)
	}
	c.Config.TemplateVars["site"] = ""
	if _, err := send("https://api.atlassian.com/ex/jira/cloud-acme/jsm/assets/workspace/w/v1/object/1"); err == nil {
		t.Fatal("Assets call allowed with no site to check against")
	}
}
